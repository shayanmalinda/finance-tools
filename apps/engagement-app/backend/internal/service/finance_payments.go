// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Payment tabs of Invoices & Payments.
const (
	FinancePaymentTabPush = "push"
	FinancePaymentTabAll  = "all"
)

// Legacy UI messages of the Per Diem Payments tab.
const (
	FinanceNoPerDiemsMsg      = "No per diems available."
	FinanceMultiCurrencyMsg   = "Multiple currencies detected for per diem. Please recheck per diem information"
	FinanceNoBalanceMsg       = "No available balance for per diem payments. Please verify the details and try again."
	FinanceInsufficientMsg    = "Insufficient balance! The entered amount is greater than the available remaining balance."
	financeTripNotOpenMsg     = "The travel request is not In Progress or Extended."
	financePaymentFailedError = "The last push did not return a NetSuite status. Re-sync to look it up."
)

func (s *FinanceService) paymentOf(p domain.Perms, pay engagemententity.FinancePayment) domain.PerDiemPayment {
	out := domain.PerDiemPayment{
		ID: pay.PerDiemNumber, JobNumber: pay.JobNumber, Traveller: financePerson(pay.TravellerEmail),
		Currency: financeDeref(pay.PaymentCurrency), ConvertedAmount: pay.ConvertedAmount, ConvertedCurrency: pay.ConvertedCurrency,
		Status: financeNSPending, NetSuite: domain.NetSuiteState{Status: domain.NetSuitePending},
		Campaign: financeCampaign(pay.Campaign), AddedOn: financeDate(pay.AddedDate), AddedBy: pay.AddedBy,
	}
	if pay.PaymentAmount != nil {
		out.Amount = *pay.PaymentAmount
	}
	status := financeDeref(pay.NSStatus)
	switch {
	case strings.EqualFold(status, financeNSComplete):
		out.Status, out.NetSuite.Status = financeNSComplete, domain.NetSuitePushed
	case strings.EqualFold(status, financeNSPushing):
		out.Status, out.NetSuite.Status = financeNSComplete, domain.NetSuitePushing
	case strings.EqualFold(status, financeNSFailed):
		out.NetSuite.Status, out.NetSuite.Error = domain.NetSuiteFailed, financeStr(financePaymentFailedError)
	}
	pending := strings.EqualFold(status, financeNSPending)
	open := financeOpen(pay.TravelStatus)
	out.Actions.Edit = pending && open && p.Travel.Has(domain.TravelPrivEditPerDiemPayment)
	out.Actions.Push = pending && open && p.Travel.Has(domain.TravelPrivPushPerDiemPayments)
	out.Actions.Resync = out.NetSuite.Status == domain.NetSuiteFailed && p.Travel.Has(domain.TravelPrivPushPerDiemPayments)
	return out
}

func (s *FinanceService) paymentsOf(ctx context.Context, p domain.Perms, rows []engagemententity.FinancePayment) []domain.PerDiemPayment {
	out := make([]domain.PerDiemPayment, 0, len(rows))
	refs := make([]*domain.CampaignRef, 0, len(rows))
	people := make([]*domain.Person, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.paymentOf(p, r))
	}
	for i := range out {
		refs = append(refs, out[i].Campaign)
		people = append(people, out[i].Traveller)
	}
	s.Campaigns.Fill(ctx, refs...)
	s.people.fillNames(ctx, people...)
	return out
}

// paymentBalance is the legacy UI Payment Summary: per diem totals (every
// currency added up, the first one's currency shown), Complete Currency
// invoices and travel cards converted to that currency at their added
// dates, payments added up as entered; remaining = total - the three,
// rounded to cents.
func (s *FinanceService) paymentBalance(ctx context.Context, basis engagemententity.FinancePaymentBasis) (*domain.PaymentSummary, error) {
	if len(basis.PerDiemTotals) == 0 {
		return nil, nil
	}
	sum := &domain.PaymentSummary{Currency: basis.PerDiemTotals[0].Currency}
	for _, t := range basis.PerDiemTotals {
		sum.TotalPerDiemAmount += t.TotalAmount
	}
	fx := newFinanceFX(s.currency)
	for _, a := range basis.CurrencyInvoices {
		v, err := fx.convert(ctx, a.Currency, sum.Currency, a.Amount, a.AddedDate)
		if err != nil {
			return nil, err
		}
		sum.InitialCurrencyPaid += v
	}
	for _, a := range basis.TravelCards {
		v, err := fx.convert(ctx, a.Currency, sum.Currency, a.Amount, a.AddedDate)
		if err != nil {
			return nil, err
		}
		sum.TotalTravelCardAmount += v
	}
	for _, a := range basis.Payments {
		sum.TotalPaymentAmount += a.Amount
	}
	sum.TotalPerDiemAmount = financeRound2(sum.TotalPerDiemAmount)
	sum.InitialCurrencyPaid = financeRound2(sum.InitialCurrencyPaid)
	sum.TotalTravelCardAmount = financeRound2(sum.TotalTravelCardAmount)
	sum.TotalPaymentAmount = financeRound2(sum.TotalPaymentAmount)
	sum.RemainingBalance = financeRound2(sum.TotalPerDiemAmount - sum.InitialCurrencyPaid - sum.TotalTravelCardAmount - sum.TotalPaymentAmount)
	return sum, nil
}

// travellerSubsidiary resolves the traveller's subsidiary (HR company →
// expense_app subsidiaries), the currency payments are converted to.
func (s *FinanceService) travellerSubsidiary(ctx context.Context, traveller string) (engagemententity.FinanceSubsidiary, error) {
	if traveller == "" {
		return engagemententity.FinanceSubsidiary{}, financeBad("Error while retrieving traveller email.")
	}
	e, err := s.hr.Employee(ctx, traveller)
	if err != nil {
		return engagemententity.FinanceSubsidiary{}, err
	}
	if e == nil || e.Company == nil || strings.TrimSpace(*e.Company) == "" {
		return engagemententity.FinanceSubsidiary{}, financeBad("The traveller's company could not be found in HR.")
	}
	sub, err := s.entity.FinanceSubsidiary(ctx, strings.TrimSpace(*e.Company))
	if isFinanceNotFound(err) {
		return sub, financeBad("No Subsidiary found under the name: %s", strings.TrimSpace(*e.Company))
	}
	return sub, err
}

// TripPayments is GET /travel-requests/{jobNumber}/per-diem-payments: the
// payments and the Payment Summary (null without per diems).
func (s *FinanceService) TripPayments(ctx context.Context, p domain.Perms, jobNumber string) (domain.TravelRequestPayments, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.TravelRequestPayments{}, err
	}
	if _, err := viewScope(p, trip, false, []int{domain.TravelPrivViewPerDiemPayments}, domain.TravelPrivViewOwnPayments); err != nil {
		return domain.TravelRequestPayments{}, err
	}
	rows, _, err := s.entity.FinancePayments(ctx, engagemententity.FinancePaymentFilter{JobNumber: jobNumber})
	if err != nil {
		return domain.TravelRequestPayments{}, err
	}
	basis, err := s.entity.FinancePaymentBasis(ctx, jobNumber)
	if err != nil {
		return domain.TravelRequestPayments{}, err
	}
	sum, err := s.paymentBalance(ctx, basis)
	if err != nil {
		return domain.TravelRequestPayments{}, err
	}
	if sum != nil && p.Travel.Any(domain.TravelPrivAddPerDiemPayments, domain.TravelPrivEditPerDiemPayment) {
		if sub, err := s.travellerSubsidiary(ctx, trip.TravellerEmail); err == nil && sub.CurrencyCode != "" {
			sum.ConvertedCurrency = financeStr(sub.CurrencyCode)
		}
	}
	return domain.TravelRequestPayments{Items: s.paymentsOf(ctx, p, rows), Summary: sum}, nil
}

func (s *FinanceService) today() string { return s.now().UTC().Format("2006-01-02") }

// AddPayment is POST /travel-requests/{jobNumber}/per-diem-payments: number
// {jobNumber}-NNN (payment count + 1), in the per diem currency, within the
// remaining balance, converted to the traveller's subsidiary currency at
// today's rate (amount / rate).
func (s *FinanceService) AddPayment(ctx context.Context, p domain.Perms, jobNumber string, amount float64) (domain.PerDiemPayment, error) {
	if s.currency == nil {
		return domain.PerDiemPayment{}, errFinanceNoConverter
	}
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	if p.IsAgent() {
		return domain.PerDiemPayment{}, financeForbidden()
	}
	if !financeOpen(trip.Status) {
		return domain.PerDiemPayment{}, financeBad(financeTripNotOpenMsg)
	}
	if amount <= 0 {
		return domain.PerDiemPayment{}, financeBad("Please re-check the information. Payment Amount is mandatory.")
	}
	if !financeCents(amount) {
		return domain.PerDiemPayment{}, financeBad(FinancePaymentCentsMsg)
	}
	basis, err := s.entity.FinancePaymentBasis(ctx, jobNumber)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	switch {
	case len(basis.PerDiemTotals) == 0:
		return domain.PerDiemPayment{}, financeBad(FinanceNoPerDiemsMsg)
	case len(basis.PerDiemTotals) > 1:
		return domain.PerDiemPayment{}, financeBad(FinanceMultiCurrencyMsg)
	}
	sum, err := s.paymentBalance(ctx, basis)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	if sum.RemainingBalance <= 0 {
		return domain.PerDiemPayment{}, financeBad(FinanceNoBalanceMsg)
	}
	if financeRound2(sum.RemainingBalance-amount) < 0 {
		return domain.PerDiemPayment{}, financeBad(FinanceInsufficientMsg)
	}
	sub, err := s.travellerSubsidiary(ctx, trip.TravellerEmail)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	converted, err := newFinanceFX(s.currency).convert(ctx, sum.Currency, sub.CurrencyCode, amount, s.today())
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	created, err := s.entity.FinanceAddPayment(ctx, jobNumber, engagemententity.FinancePaymentCreate{
		PerDiemNumber: fmt.Sprintf("%s-%03d", jobNumber, basis.PaymentCount+1), PaymentCurrency: sum.Currency,
		PaymentAmount: amount, ConvertedCurrency: sub.CurrencyCode, ConvertedAmount: converted, CreatedBy: p.Email,
	})
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	out := s.paymentOf(p, created)
	s.Campaigns.Fill(ctx, out.Campaign)
	s.people.fillNames(ctx, out.Traveller)
	return out, nil
}

// paymentView checks a payment's record rule: any with Travel 41 or 42, the
// traveller's own with 47; agents never see payments.
func paymentView(p domain.Perms, pay engagemententity.FinancePayment) error {
	switch {
	case p.IsAgent():
		return financeForbidden()
	case p.Travel.Any(domain.TravelPrivViewAllPaymentsPage, domain.TravelPrivViewPerDiemPayments):
		return nil
	case p.Travel.Has(domain.TravelPrivViewOwnPayments) && strings.EqualFold(financeDeref(pay.TravellerEmail), p.Email):
		return nil
	}
	return financeForbidden()
}

func (s *FinanceService) payment(ctx context.Context, number string) (engagemententity.FinancePayment, error) {
	pay, err := s.entity.FinancePayment(ctx, number)
	if isFinanceNotFound(err) {
		return pay, &apierror.NotFoundError{Msg: "Per diem payment not found."}
	}
	return pay, err
}

// Payment is GET /per-diem-payments/{paymentId}; a pushed payment's vendor
// bill is looked up live.
func (s *FinanceService) Payment(ctx context.Context, p domain.Perms, number string) (domain.PerDiemPayment, error) {
	pay, err := s.payment(ctx, number)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	if err := paymentView(p, pay); err != nil {
		return domain.PerDiemPayment{}, err
	}
	out := s.paymentOf(p, pay)
	s.Campaigns.Fill(ctx, out.Campaign)
	s.people.fillNames(ctx, out.Traveller)
	if out.NetSuite.Status == domain.NetSuitePushed {
		if ref, err := s.entity.FinancePaymentReference(ctx, number); err == nil {
			out.NetSuite.Reference = ref
		}
	}
	return out, nil
}

// UpdatePayment is PATCH /per-diem-payments/{paymentId} (Edit Payment):
// pending only, trip open, within the remaining balance (the edited
// payment's old amount given back), reconverted at today's rate.
func (s *FinanceService) UpdatePayment(ctx context.Context, p domain.Perms, number string, amount float64) (domain.PerDiemPayment, error) {
	if s.currency == nil {
		return domain.PerDiemPayment{}, errFinanceNoConverter
	}
	if amount <= 0 {
		return domain.PerDiemPayment{}, financeBad("Please re-check the information. Payment Amount is mandatory.")
	}
	if !financeCents(amount) {
		return domain.PerDiemPayment{}, financeBad(FinancePaymentCentsMsg)
	}
	pay, err := s.payment(ctx, number)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	if !strings.EqualFold(financeDeref(pay.NSStatus), financeNSPending) {
		return domain.PerDiemPayment{}, &apierror.ConflictError{Msg: "Only a pending payment can be edited."}
	}
	if !financeOpen(pay.TravelStatus) {
		return domain.PerDiemPayment{}, financeBad(financeTripNotOpenMsg)
	}
	basis, err := s.entity.FinancePaymentBasis(ctx, pay.JobNumber)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	sum, err := s.paymentBalance(ctx, basis)
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	if sum == nil {
		return domain.PerDiemPayment{}, financeBad(FinanceNoPerDiemsMsg)
	}
	old := 0.0
	if pay.PaymentAmount != nil {
		old = *pay.PaymentAmount
	}
	if financeRound2(sum.RemainingBalance+old-amount) < 0 {
		return domain.PerDiemPayment{}, financeBad(FinanceInsufficientMsg)
	}
	sub, err := s.travellerSubsidiary(ctx, financeDeref(pay.TravellerEmail))
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	converted, err := newFinanceFX(s.currency).convert(ctx, financeDeref(pay.PaymentCurrency), sub.CurrencyCode, amount, s.today())
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	updated, err := s.entity.FinanceUpdatePayment(ctx, number, engagemententity.FinancePaymentUpdate{
		PaymentAmount: amount, ConvertedCurrency: sub.CurrencyCode, ConvertedAmount: converted, UpdatedBy: p.Email})
	if err != nil {
		return domain.PerDiemPayment{}, err
	}
	out := s.paymentOf(p, updated)
	s.Campaigns.Fill(ctx, out.Campaign)
	s.people.fillNames(ctx, out.Traveller)
	return out, nil
}

func paymentTabs(p domain.Perms) []string {
	var tabs []string
	if p.TravelOwn.Has(domain.TravelPrivPushPerDiemPayments) {
		tabs = append(tabs, FinancePaymentTabPush)
	}
	if p.Travel.Has(domain.TravelPrivViewAllPaymentsPage) {
		tabs = append(tabs, FinancePaymentTabAll)
	}
	return tabs
}

// financePaymentAllStatuses is every payment ns_status the screens filter on.
var financePaymentAllStatuses = []string{financeNSPending, financeNSPushing, financeNSComplete, financeNSFailed}

// Payments is GET /per-diem-payments: Per Diem Payments to Push (own
// Travel 40: Pending) or All Per Diem Payments (Travel 41).
func (s *FinanceService) Payments(ctx context.Context, p domain.Perms, q domain.PaymentQuery) (domain.PerDiemPaymentPage, error) {
	tabs := paymentTabs(p)
	if q.Tab == "" {
		if financeHas(tabs, FinancePaymentTabAll) || len(tabs) == 0 {
			q.Tab = FinancePaymentTabAll
		} else {
			q.Tab = tabs[0]
		}
	}
	if q.Tab != FinancePaymentTabPush && q.Tab != FinancePaymentTabAll {
		return domain.PerDiemPaymentPage{}, financeBad("tab must be push or all.")
	}
	// A tab the caller does not have: no rows, only their own tab counts.
	allowed := financeHas(tabs, q.Tab)
	if len(tabs) == 0 || p.IsAgent() {
		return domain.PerDiemPaymentPage{}, financeForbidden()
	}
	statuses := financePaymentAllStatuses
	if q.Tab == FinancePaymentTabPush {
		statuses = []string{financeNSPending}
	}
	if len(q.NetSuiteStatus) > 0 {
		var keep []string
		for _, st := range statuses {
			ns := domain.NetSuitePending
			if st == financeNSComplete || st == financeNSPushing {
				ns = domain.NetSuitePushed
			}
			if financeHas(q.NetSuiteStatus, ns) {
				keep = append(keep, st)
			}
		}
		statuses = keep
	}
	page := domain.PerDiemPaymentPage{Items: []domain.PerDiemPayment{}, TabCounts: map[string]int{}}
	if allowed && len(statuses) > 0 {
		// The full set on All sends no filter, so a payment whose ns_status
		// is NULL or unexpected (paymentOf maps it) still shows, as counts.All
		// includes it.
		if q.Tab == FinancePaymentTabAll && len(statuses) == len(financePaymentAllStatuses) {
			statuses = nil
		}
		rows, total, err := s.entity.FinancePayments(ctx, engagemententity.FinancePaymentFilter{
			NSStatuses: statuses, Search: strings.TrimSpace(q.Search), Travellers: q.Travellers,
			ConvertedCurrency: q.ConvertedCurrency, CampaignIDs: q.CampaignIDs,
			Sort: domain.KnownSort(q.Sort, domain.PaymentSorts), Limit: q.Limit, Offset: q.Offset,
		})
		if err != nil {
			return domain.PerDiemPaymentPage{}, err
		}
		page.Items, page.Total = s.paymentsOf(ctx, p, rows), total
	}
	counts, err := s.entity.FinancePaymentCounts(ctx)
	if err != nil {
		return domain.PerDiemPaymentPage{}, err
	}
	for _, t := range tabs {
		page.TabCounts[t] = map[string]int{FinancePaymentTabPush: counts.Pending, FinancePaymentTabAll: counts.All}[t]
	}
	return page, nil
}

func dedupeStrings(v []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// PushPayments is POST /per-diem-payments/push: each pending payment of an
// open trip in turn, with the traveller's HR record (the vendor bill
// "employee"). No email (legacy).
func (s *FinanceService) PushPayments(ctx context.Context, p domain.Perms, numbers []string) (domain.BatchResult, error) {
	numbers = dedupeStrings(numbers)
	if len(numbers) == 0 || len(numbers) > 200 {
		return domain.BatchResult{}, financeBad("Select 1 to 200 payments.")
	}
	employees := map[string]*engagemententity.FinanceEmployee{}
	out := domain.BatchResult{Results: []domain.BatchItem{}}
	for i, n := range numbers {
		if s.financeOutOfTime(ctx) {
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr(financeOutOfTimeMsg)})
			continue
		}
		pay, err := s.payment(ctx, n)
		if err != nil {
			msg, stop := financeRecordErr(err)
			if stop != nil {
				return financeBatchStop(out, n, false, numbers[i+1:], stop)
			}
			var nf *apierror.NotFoundError
			if asNotFound(err, &nf) {
				msg = financeStr(nf.Msg)
			}
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: msg})
			continue
		}
		if !financeOpen(pay.TravelStatus) {
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr(financeTripNotOpenMsg)})
			continue
		}
		traveller := strings.ToLower(financeDeref(pay.TravellerEmail))
		emp, ok := employees[traveller]
		if !ok {
			emp, err = s.financeEmployee(ctx, traveller)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return financeBatchStop(out, n, false, numbers[i+1:], err)
				}
				out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr("Not pushed: the traveller's HR record could not be read. Try again.")})
				continue
			}
			employees[traveller] = emp
		}
		if emp == nil {
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr("Not pushed: the traveller or their company was not found in HR.")})
			continue
		}
		// The lookups may have used up the reserve: a push that starts now
		// could hit the deadline with an unknown outcome.
		if s.financeOutOfTime(ctx) {
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr(financeOutOfTimeMsg)})
			continue
		}
		// Canceled with time to spare: stop before the push, so the payment
		// is reported as not started rather than outcome unknown.
		if err := ctx.Err(); err != nil {
			return financeBatchStop(out, n, false, numbers[i+1:], err)
		}
		r, err := s.entity.FinancePushPayment(ctx, n, p.Email, *emp)
		if err != nil {
			msg, stop := financeRecordErr(err)
			if stop != nil {
				return financeBatchStop(out, n, true, numbers[i+1:], stop)
			}
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: msg})
			continue
		}
		out.Results = append(out.Results, pushItem(n, r))
	}
	return out, nil
}

func asNotFound(err error, target **apierror.NotFoundError) bool {
	nf, ok := err.(*apierror.NotFoundError)
	if ok {
		*target = nf
	}
	return ok
}

// financeEmployee is the traveller's HR record for a vendor bill; nil when
// HR has no such employee or no company.
func (s *FinanceService) financeEmployee(ctx context.Context, traveller string) (*engagemententity.FinanceEmployee, error) {
	if traveller == "" {
		return nil, nil
	}
	e, err := s.hr.Employee(ctx, traveller)
	if err != nil {
		return nil, err
	}
	if e == nil || financeDeref(e.Company) == "" {
		return nil, nil
	}
	return &engagemententity.FinanceEmployee{
		EmployeeID: e.EmployeeID, Email: e.WorkEmail, FirstName: e.FirstName, LastName: e.LastName,
		JobRole: financeDeref(e.JobRole), Department: financeDeref(e.Department), Company: strings.TrimSpace(*e.Company),
	}, nil
}

// ResyncPayments is POST /per-diem-payments/resync (lookup only).
func (s *FinanceService) ResyncPayments(ctx context.Context, p domain.Perms, numbers []string) (domain.BatchResult, error) {
	numbers = dedupeStrings(numbers)
	if len(numbers) == 0 || len(numbers) > 200 {
		return domain.BatchResult{}, financeBad("Select 1 to 200 payments.")
	}
	out := domain.BatchResult{Results: []domain.BatchItem{}}
	for i, n := range numbers {
		if s.financeOutOfTime(ctx) {
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: financeStr(financeOutOfTimeMsg)})
			continue
		}
		r, err := s.entity.FinanceResyncPayment(ctx, n, p.Email)
		if err != nil {
			msg, stop := financeRecordErr(err)
			if stop != nil {
				return financeBatchStop(out, n, true, numbers[i+1:], stop)
			}
			out.Results = append(out.Results, domain.BatchItem{ID: n, Message: msg})
			continue
		}
		out.Results = append(out.Results, pushItem(n, r))
	}
	return out, nil
}

// SyncFailures is GET /netsuite-sync-failures: failed invoices with own
// Travel 34, failed payments with own Travel 40.
func (s *FinanceService) SyncFailures(ctx context.Context, p domain.Perms, search string) (domain.SyncFailurePage, error) {
	out := domain.SyncFailurePage{Items: []domain.SyncFailure{}}
	search = strings.TrimSpace(search)
	if p.TravelOwn.Has(domain.TravelPrivPushInvoices) {
		rows, _, err := s.entity.FinanceInvoices(ctx, engagemententity.FinanceInvoiceFilter{Failed: "true", Search: search})
		if err != nil {
			return out, err
		}
		for _, inv := range rows {
			out.Items = append(out.Items, domain.SyncFailure{
				Record: "invoice", ID: strconv.Itoa(inv.ID), Number: inv.InvoiceNumber, Agency: inv.AgencyName,
				JobNumber: inv.JobNumber, Traveller: financePerson(inv.TravellerEmail), Campaign: financeCampaign(inv.Campaign),
				Amount: inv.InvoiceAmount, Currency: inv.InvoiceCurrency,
				NetSuite: domain.NetSuiteState{Status: domain.NetSuiteFailed, Error: inv.NetSuiteError},
			})
		}
	}
	if p.TravelOwn.Has(domain.TravelPrivPushPerDiemPayments) {
		rows, _, err := s.entity.FinancePayments(ctx, engagemententity.FinancePaymentFilter{NSStatuses: []string{financeNSFailed}, Search: search})
		if err != nil {
			return out, err
		}
		for _, pay := range rows {
			item := domain.SyncFailure{
				Record: "payment", ID: pay.PerDiemNumber, Number: pay.PerDiemNumber, JobNumber: pay.JobNumber,
				Traveller: financePerson(pay.TravellerEmail), Campaign: financeCampaign(pay.Campaign),
				Currency: financeDeref(pay.PaymentCurrency),
				NetSuite: domain.NetSuiteState{Status: domain.NetSuiteFailed, Error: financeStr(financePaymentFailedError)},
			}
			if pay.PaymentAmount != nil {
				item.Amount = *pay.PaymentAmount
			}
			out.Items = append(out.Items, item)
		}
	}
	refs := make([]*domain.CampaignRef, 0, len(out.Items))
	people := make([]*domain.Person, 0, len(out.Items))
	for i := range out.Items {
		refs = append(refs, out.Items[i].Campaign)
		people = append(people, out.Items[i].Traveller)
	}
	s.Campaigns.Fill(ctx, refs...)
	s.people.fillNames(ctx, people...)
	out.Total = len(out.Items)
	return out, nil
}

// PaymentReport is GET /reports/per-diem-payments: payments added from..to
// (inclusive), newest first.
func (s *FinanceService) PaymentReport(ctx context.Context, p domain.Perms, from, to string) (domain.PerDiemPaymentPage, error) {
	if err := financeRange(from, to); err != nil {
		return domain.PerDiemPaymentPage{}, err
	}
	rows, total, err := s.entity.FinancePayments(ctx, engagemententity.FinancePaymentFilter{AddedFrom: from, AddedTo: to})
	if err != nil {
		return domain.PerDiemPaymentPage{}, err
	}
	return domain.PerDiemPaymentPage{Items: s.paymentsOf(ctx, p, rows), Total: total}, nil
}
