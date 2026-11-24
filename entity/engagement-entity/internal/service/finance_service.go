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
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// FinanceService is the travel finance area: per diems, travel cards,
// invoices, per diem payments and their NetSuite pushes. Privileges are the
// backend's; this layer validates input, keeps the legacy data rules and
// owns the push state machine (one push in flight, lookup before push,
// re-sync by lookup).
type FinanceService interface {
	Trip(ctx context.Context, jobNumber string) (domain.FinanceTrip, error)

	PerDiems(ctx context.Context, jobNumber string) ([]domain.FinancePerDiem, error)
	PerDiem(ctx context.Context, jobNumber string, id int) (domain.FinancePerDiem, error)
	AddPerDiem(ctx context.Context, jobNumber string, in domain.FinancePerDiemCreate) (domain.FinancePerDiem, error)
	DeletePerDiem(ctx context.Context, jobNumber string, id int) error
	PaymentBasis(ctx context.Context, jobNumber string) (domain.FinancePaymentBasis, error)

	TravelCards(ctx context.Context, jobNumber string) ([]domain.FinanceTravelCard, error)
	TripExpenses(ctx context.Context, jobNumber string) (domain.FinanceTripExpenses, error)
	AddTravelCard(ctx context.Context, jobNumber string, in domain.FinanceTravelCardCreate) (domain.FinanceTravelCard, error)

	CostTypes(ctx context.Context) ([]domain.FinanceCostType, error)
	Invoices(ctx context.Context, f domain.FinanceInvoiceFilter) ([]domain.FinanceInvoice, int, error)
	Invoice(ctx context.Context, id int) (domain.FinanceInvoice, error)
	InvoiceCounts(ctx context.Context, agencyID int) (domain.FinanceInvoiceCounts, error)
	AddInvoices(ctx context.Context, jobNumber string, in domain.FinanceInvoicesCreate) ([]domain.FinanceInvoice, error)
	DecideInvoices(ctx context.Context, d domain.FinanceInvoiceDecision) ([]domain.FinanceDecisionResult, error)
	PushInvoice(ctx context.Context, id int, actor string) (domain.FinancePushResult, error)
	ResyncInvoice(ctx context.Context, id int, actor string) (domain.FinancePushResult, error)
	InvoiceReference(ctx context.Context, id int) (domain.FinanceReference, error)

	Payments(ctx context.Context, f domain.FinancePaymentFilter) ([]domain.FinancePayment, int, error)
	Payment(ctx context.Context, perDiemNumber string) (domain.FinancePayment, error)
	PaymentCounts(ctx context.Context) (domain.FinancePaymentCounts, error)
	AddPayment(ctx context.Context, jobNumber string, in domain.FinancePaymentCreate) (domain.FinancePayment, error)
	UpdatePayment(ctx context.Context, perDiemNumber string, in domain.FinancePaymentUpdate) (domain.FinancePayment, error)
	PushPayment(ctx context.Context, perDiemNumber string, req domain.FinancePushRequest) (domain.FinancePushResult, error)
	ResyncPayment(ctx context.Context, perDiemNumber, actor string) (domain.FinancePushResult, error)
	PaymentReference(ctx context.Context, perDiemNumber string) (domain.FinanceReference, error)

	Subsidiary(ctx context.Context, company string) (domain.FinanceSubsidiary, error)
}

// FinanceNetSuite is the NetSuite client the finance service uses
// (*netsuite.Client).
type FinanceNetSuite interface {
	Enabled() bool
	PurchaseOrderTaxCode() string
	LookupPurchaseOrders(ctx context.Context, transactionID string) ([]netsuite.Record, error)
	LookupVendorBills(ctx context.Context, transactionID string) ([]netsuite.Record, error)
	CreatePurchaseOrder(ctx context.Context, po netsuite.PurchaseOrder) (string, error)
	CreateVendorBill(ctx context.Context, vb netsuite.VendorBill) (string, error)
}

type financeService struct {
	repo    repository.FinanceRepository
	alloc   repository.FinanceAllocationRepository
	expense repository.FinanceExpenseRepository
	ns      FinanceNetSuite
	now     func() time.Time
	// finalTimeout bounds the writes that settle a push after NetSuite has
	// been called; they run detached from the request so a client timeout
	// never leaves a record half pushed.
	finalTimeout time.Duration
}

// NewFinanceService constructs a FinanceService.
func NewFinanceService(repo repository.FinanceRepository, alloc repository.FinanceAllocationRepository,
	expense repository.FinanceExpenseRepository, ns FinanceNetSuite) FinanceService {
	return &financeService{repo: repo, alloc: alloc, expense: expense, ns: ns, now: time.Now, finalTimeout: 30 * time.Second}
}

var (
	financeJobNumberRe     = regexp.MustCompile(`^TRV-\d{4}-\d{2}-\d{4}$`)
	financePerDiemNumberRe = regexp.MustCompile(`^TRV-\d{4}-\d{2}-\d{4}-\d{3}$`)
	financeInvoiceNumberRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	financeCurrencyRe      = regexp.MustCompile(`^[A-Z]{3}$`)
	financeDateTimeRe      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
)

var financeMonths = map[string]bool{
	"January": true, "February": true, "March": true, "April": true, "May": true, "June": true,
	"July": true, "August": true, "September": true, "October": true, "November": true, "December": true,
}

func financeBad(format string, a ...any) error {
	return &apierror.ValidationError{Msg: fmt.Sprintf(format, a...)}
}

func financeCheckJob(jobNumber string) error {
	if !financeJobNumberRe.MatchString(jobNumber) {
		return financeBad("Invalid job number format")
	}
	return nil
}

func financeCheckActor(actor string) error {
	if strings.TrimSpace(actor) == "" || !strings.Contains(actor, "@") {
		return financeBad("the acting user's email is required")
	}
	return nil
}

func financeMsg(s string) *string { return &s }

// ── Trip ────────────────────────────────────────────────────────────────

func (s *financeService) Trip(ctx context.Context, jobNumber string) (domain.FinanceTrip, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinanceTrip{}, err
	}
	t, err := s.repo.Trip(ctx, jobNumber)
	if err != nil {
		return domain.FinanceTrip{}, err
	}
	if t == nil {
		return domain.FinanceTrip{}, &apierror.NotFoundError{Msg: "Travel request not found."}
	}
	if t.AllocationID != nil && *t.AllocationID != "" {
		e, err := s.alloc.EngagementOfAllocation(ctx, *t.AllocationID)
		if err != nil {
			return domain.FinanceTrip{}, err
		}
		if e != nil {
			if e.EngagementCode != "" {
				code := e.EngagementCode
				t.EngagementCode = &code
			}
			if e.CampaignID != nil {
				t.Campaign = &domain.FinanceCampaign{ID: *e.CampaignID, Name: e.ProjectName}
			}
		}
	}
	return *t, nil
}

// ── Per diems ───────────────────────────────────────────────────────────

func (s *financeService) PerDiems(ctx context.Context, jobNumber string) ([]domain.FinancePerDiem, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return nil, err
	}
	return s.repo.PerDiems(ctx, jobNumber)
}

func (s *financeService) PerDiem(ctx context.Context, jobNumber string, id int) (domain.FinancePerDiem, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinancePerDiem{}, err
	}
	p, err := s.repo.PerDiem(ctx, jobNumber, id)
	if err != nil {
		return domain.FinancePerDiem{}, err
	}
	if p == nil {
		return domain.FinancePerDiem{}, &apierror.NotFoundError{Msg: "Per diem not found."}
	}
	return *p, nil
}

// AddPerDiem ports POST /travels/{jn}/per-diems for one row: the engagement
// code is the trip's live engagement code (the legacy client sent it).
func (s *financeService) AddPerDiem(ctx context.Context, jobNumber string, in domain.FinancePerDiemCreate) (domain.FinancePerDiem, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinancePerDiem{}, err
	}
	switch {
	case !financeCurrencyRe.MatchString(in.Currency):
		return domain.FinancePerDiem{}, financeBad("currency must be a three-letter currency code")
	case in.PerDiemValue <= 0:
		return domain.FinancePerDiem{}, financeBad("perDiemValue must be more than 0")
	case in.Total < 0:
		return domain.FinancePerDiem{}, financeBad("total must not be negative")
	case in.NoOfDays < 1:
		return domain.FinancePerDiem{}, financeBad("noOfDays must be 1 or more")
	case !financeDateTimeRe.MatchString(in.StartDate) || !financeDateTimeRe.MatchString(in.EndDate):
		return domain.FinancePerDiem{}, financeBad("startDate and endDate must be YYYY-MM-DD HH:MM:SS")
	case in.EndDate <= in.StartDate:
		return domain.FinancePerDiem{}, financeBad("Please check the dates again")
	}
	if err := financeCheckActor(in.CreatedBy); err != nil {
		return domain.FinancePerDiem{}, err
	}
	trip, err := s.Trip(ctx, jobNumber)
	if err != nil {
		return domain.FinancePerDiem{}, err
	}
	if trip.EngagementCode == nil || *trip.EngagementCode == "" {
		return domain.FinancePerDiem{}, financeBad("The travel request has no engagement code.")
	}
	id, err := s.repo.InsertPerDiem(ctx, jobNumber, *trip.EngagementCode, in)
	if err != nil {
		return domain.FinancePerDiem{}, err
	}
	return s.PerDiem(ctx, jobNumber, id)
}

func (s *financeService) DeletePerDiem(ctx context.Context, jobNumber string, id int) error {
	if err := financeCheckJob(jobNumber); err != nil {
		return err
	}
	ok, err := s.repo.DeletePerDiem(ctx, jobNumber, id)
	if err != nil {
		return err
	}
	if !ok {
		return &apierror.NotFoundError{Msg: "Per diem not found."}
	}
	return nil
}

func (s *financeService) PaymentBasis(ctx context.Context, jobNumber string) (domain.FinancePaymentBasis, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinancePaymentBasis{}, err
	}
	return s.repo.PaymentBasis(ctx, jobNumber)
}

// TripExpenses is a travel's card spend and expense claims (expense_app).
func (s *financeService) TripExpenses(ctx context.Context, jobNumber string) (domain.FinanceTripExpenses, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinanceTripExpenses{}, err
	}
	return s.expense.TripExpenses(ctx, jobNumber)
}

// ── Travel cards ────────────────────────────────────────────────────────

func (s *financeService) TravelCards(ctx context.Context, jobNumber string) ([]domain.FinanceTravelCard, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return nil, err
	}
	return s.repo.TravelCards(ctx, jobNumber)
}

// FinanceCardsBlockedMsg is the legacy UI text shown when payments exist
// (the legacy server said "Payments are being processed. travel cards cannot
// be added.").
const FinanceCardsBlockedMsg = "The finance team has started processing the payments. Adding travel cards is not permitted at this time."

// AddTravelCard ports POST /travels/{jn}/travel-cards for one card: refused
// once a per diem payment exists.
func (s *financeService) AddTravelCard(ctx context.Context, jobNumber string, in domain.FinanceTravelCardCreate) (domain.FinanceTravelCard, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinanceTravelCard{}, err
	}
	if !financeCurrencyRe.MatchString(in.Currency) {
		return domain.FinanceTravelCard{}, financeBad("currency must be a three-letter currency code")
	}
	if in.Amount <= 0 {
		return domain.FinanceTravelCard{}, financeBad("amount must be more than 0")
	}
	if err := financeCheckActor(in.CreatedBy); err != nil {
		return domain.FinanceTravelCard{}, err
	}
	if _, err := s.Trip(ctx, jobNumber); err != nil {
		return domain.FinanceTravelCard{}, err
	}
	n, err := s.repo.PaymentCount(ctx, jobNumber)
	if err != nil {
		return domain.FinanceTravelCard{}, err
	}
	if n > 0 {
		return domain.FinanceTravelCard{}, financeBad(FinanceCardsBlockedMsg)
	}
	id, err := s.repo.InsertTravelCard(ctx, jobNumber, in)
	if err != nil {
		return domain.FinanceTravelCard{}, err
	}
	cards, err := s.repo.TravelCards(ctx, jobNumber)
	if err != nil {
		return domain.FinanceTravelCard{}, err
	}
	for _, c := range cards {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.FinanceTravelCard{ID: id, JobNumber: jobNumber, Currency: in.Currency, Amount: in.Amount, AddedBy: in.CreatedBy}, nil
}

// ── Invoices ────────────────────────────────────────────────────────────

func (s *financeService) CostTypes(ctx context.Context) ([]domain.FinanceCostType, error) {
	return s.repo.CostTypes(ctx)
}

func (s *financeService) withInvoiceCampaigns(ctx context.Context, items []domain.FinanceInvoice) error {
	var ids []string
	seen := map[string]bool{}
	for _, it := range items {
		if it.AllocationID != nil && *it.AllocationID != "" && !seen[*it.AllocationID] {
			seen[*it.AllocationID] = true
			ids = append(ids, *it.AllocationID)
		}
	}
	camps, err := s.alloc.Campaigns(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if a := items[i].AllocationID; a != nil {
			if c, ok := camps[*a]; ok {
				items[i].Campaign = &c
			}
		}
	}
	return nil
}

func (s *financeService) Invoices(ctx context.Context, f domain.FinanceInvoiceFilter) ([]domain.FinanceInvoice, int, error) {
	if f.JobNumber != "" {
		if err := financeCheckJob(f.JobNumber); err != nil {
			return nil, 0, err
		}
	}
	if f.Failed != "" && f.Failed != "true" && f.Failed != "false" {
		return nil, 0, financeBad("failed must be true or false")
	}
	if len(f.CampaignIDs) > 0 {
		allocs, err := s.alloc.AllocationsOfCampaigns(ctx, f.CampaignIDs)
		if err != nil {
			return nil, 0, err
		}
		if len(allocs) == 0 {
			return []domain.FinanceInvoice{}, 0, nil
		}
		f.AllocationIDs = allocs
	}
	items, total, err := s.repo.Invoices(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	if err := s.withInvoiceCampaigns(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *financeService) Invoice(ctx context.Context, id int) (domain.FinanceInvoice, error) {
	inv, err := s.repo.Invoice(ctx, id)
	if err != nil {
		return domain.FinanceInvoice{}, err
	}
	if inv == nil {
		return domain.FinanceInvoice{}, &apierror.NotFoundError{Msg: "Invoice not found."}
	}
	items := []domain.FinanceInvoice{*inv}
	if err := s.withInvoiceCampaigns(ctx, items); err != nil {
		return domain.FinanceInvoice{}, err
	}
	return items[0], nil
}

func (s *financeService) InvoiceCounts(ctx context.Context, agencyID int) (domain.FinanceInvoiceCounts, error) {
	return s.repo.InvoiceCounts(ctx, agencyID)
}

// FinanceCurrencyInvoiceBlockedMsg is the legacy UI text for a Currency
// invoice once payments exist (the legacy server reused the travel card
// text).
const FinanceCurrencyInvoiceBlockedMsg = "The finance team has started processing the payments. You cannot add currency cost type invoices at this time."

// classificationFor ports the add-invoice account classification choice:
// Travel Insurance uses the "ALL" classification of any cost type; others
// the classification whose engagement_code contains the engagement code,
// else whose suffix contains the part after the first "-". The legacy
// indexed that part after checking only for "at least one part" and failed
// on a code without "-"; that is a plain "not found" here.
func (s *financeService) classificationFor(ctx context.Context, costTypeID int, engagementCode string) (int, error) {
	notFound := financeBad("No valid invoice classification Id found")
	if costTypeID == domain.InvoiceCostTypeTravelInsuranceID {
		id, err := s.repo.ClassificationID(ctx, "ALL", "", nil)
		if err != nil {
			return 0, err
		}
		if id == nil {
			return 0, notFound
		}
		return *id, nil
	}
	if engagementCode == "" {
		return 0, notFound
	}
	ct := costTypeID
	id, err := s.repo.ClassificationID(ctx, "%"+engagementCode+"%", "", &ct)
	if err != nil {
		return 0, err
	}
	if id != nil {
		return *id, nil
	}
	parts := strings.Split(engagementCode, "-")
	if len(parts) < 2 {
		return 0, notFound
	}
	id, err = s.repo.ClassificationID(ctx, "", "%"+parts[1]+"%", &ct)
	if err != nil {
		return 0, err
	}
	if id == nil {
		return 0, notFound
	}
	return *id, nil
}

// AddInvoices ports POST /travels/{jn}/invoices: every row is validated
// first (number format, unique in the Travel DB and in NetSuite purchase
// orders, the Currency rule), then all are inserted together.
func (s *financeService) AddInvoices(ctx context.Context, jobNumber string, in domain.FinanceInvoicesCreate) ([]domain.FinanceInvoice, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return nil, err
	}
	if len(in.Invoices) == 0 || len(in.Invoices) > 50 {
		return nil, financeBad("invoices must have 1 to 50 rows")
	}
	if err := financeCheckActor(in.CreatedBy); err != nil {
		return nil, err
	}
	trip, err := s.Trip(ctx, jobNumber)
	if err != nil {
		return nil, err
	}
	costTypes, err := s.repo.CostTypes(ctx)
	if err != nil {
		return nil, err
	}
	costName := map[int]string{}
	for _, c := range costTypes {
		costName[c.ID] = c.Name
	}
	seen := map[string]bool{}
	paymentCount := -1
	for _, row := range in.Invoices {
		switch {
		case !financeInvoiceNumberRe.MatchString(row.InvoiceNumber):
			return nil, financeBad("'%s' is an invalid invoice number. Only numbers, alphabets, hyphen and underscore are allowed.", row.InvoiceNumber)
		case len(row.InvoiceNumber) > 50:
			return nil, financeBad("Invoice number %s is too long (50 characters at most).", row.InvoiceNumber)
		case !financeMonths[row.Month]:
			return nil, financeBad("month must be a month name such as October")
		case costName[row.CostTypeID] == "":
			return nil, financeBad("costTypeId %d is not an active cost type", row.CostTypeID)
		case !financeCurrencyRe.MatchString(row.InvoiceCurrency) ||
			(row.ConvertedCurrency != "" && !financeCurrencyRe.MatchString(row.ConvertedCurrency)):
			return nil, financeBad("invoiceCurrency and convertedCurrency must be three-letter currency codes")
		case row.InvoiceAmount <= 0 || row.ConvertedAmount < 0:
			return nil, financeBad("invoiceAmount must be more than 0 and convertedAmount 0 or more")
		}
		key := strings.ToLower(row.InvoiceNumber)
		if seen[key] {
			return nil, &apierror.ConflictError{Msg: fmt.Sprintf("Invoice number %s already exists.", row.InvoiceNumber)}
		}
		seen[key] = true
		exists, err := s.repo.InvoiceNumberExists(ctx, row.InvoiceNumber)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, &apierror.ConflictError{Msg: fmt.Sprintf("Invoice number %s already exists.", row.InvoiceNumber)}
		}
		if strings.EqualFold(costName[row.CostTypeID], domain.InvoiceCostTypeCurrency) {
			if paymentCount < 0 {
				if paymentCount, err = s.repo.PaymentCount(ctx, jobNumber); err != nil {
					return nil, err
				}
			}
			if paymentCount > 0 {
				return nil, financeBad(FinanceCurrencyInvoiceBlockedMsg)
			}
		}
		if s.ns.Enabled() {
			found, err := s.ns.LookupPurchaseOrders(ctx, row.InvoiceNumber)
			if err != nil {
				slog.ErrorContext(ctx, "netsuite purchase order lookup failed", "invoice", row.InvoiceNumber, "err", err)
				return nil, &apierror.ServiceUnavailableError{Msg: "Error while validating invoices"}
			}
			if len(found) > 0 {
				return nil, &apierror.ConflictError{Msg: fmt.Sprintf("Invoice number %s already exists in NetSuite.", row.InvoiceNumber)}
			}
		} else {
			slog.WarnContext(ctx, "NetSuite is disabled: invoice number not checked against NetSuite", "invoice", row.InvoiceNumber)
		}
	}
	code := ""
	if trip.EngagementCode != nil {
		code = *trip.EngagementCode
	}
	rows := make([]repository.FinanceInvoiceInsert, 0, len(in.Invoices))
	for _, row := range in.Invoices {
		cls, err := s.classificationFor(ctx, row.CostTypeID, code)
		if err != nil {
			return nil, err
		}
		rows = append(rows, repository.FinanceInvoiceInsert{FinanceInvoiceCreate: row, ClassificationID: cls})
	}
	ids, err := s.repo.InsertInvoices(ctx, jobNumber, in.CreatedBy, rows)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FinanceInvoice, 0, len(ids))
	for _, id := range ids {
		inv, err := s.Invoice(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

// FinanceInvoiceStatusMsg is the legacy error for an invoice at the wrong
// step.
const FinanceInvoiceStatusMsg = "Problem in Travel Invoice Status"

// DecideInvoices ports PATCH /approve-invoices and /reject-invoices for the
// step the backend chose (travel = PO approval with Travel 32, finance =
// 33 without 32). Each invoice must be at that step; one at another step
// fails alone (the legacy stopped the batch half way with a 500).
func (s *financeService) DecideInvoices(ctx context.Context, d domain.FinanceInvoiceDecision) ([]domain.FinanceDecisionResult, error) {
	if len(d.InvoiceIDs) == 0 || len(d.InvoiceIDs) > 200 {
		return nil, financeBad("invoiceIds must have 1 to 200 ids")
	}
	if err := financeCheckActor(d.UpdatedBy); err != nil {
		return nil, err
	}
	t := repository.FinanceInvoiceTransition{Actor: d.UpdatedBy}
	switch d.Step {
	case domain.FinanceStepTravel:
		t.From, t.ApproverColumn = domain.InvoiceStatusPending, "hr_approved_by"
	case domain.FinanceStepFinance:
		t.From, t.ApproverColumn = domain.InvoiceStatusAcceptedByTravel, "finance_approved_by"
	default:
		return nil, financeBad("step must be travel or finance")
	}
	switch d.Decision {
	case domain.FinanceDecisionApprove:
		t.To = map[string]string{domain.FinanceStepTravel: domain.InvoiceStatusAcceptedByTravel,
			domain.FinanceStepFinance: domain.InvoiceStatusAcceptedByFinance}[d.Step]
	case domain.FinanceDecisionReject:
		// Measured as it will be saved: Latin1 can lengthen it ("→" is "->").
		reason := strings.TrimSpace(textnorm.Latin1(d.Reason))
		if reason == "" {
			return nil, financeBad("Please enter a reject reason")
		}
		if utf8.RuneCountInString(reason) > domain.InvoiceMaxRejectReason {
			return nil, financeBad("The reject reason must be %d characters or fewer.", domain.InvoiceMaxRejectReason)
		}
		t.Reason = &reason
		t.To = map[string]string{domain.FinanceStepTravel: domain.InvoiceStatusRejectedByTravel,
			domain.FinanceStepFinance: domain.InvoiceStatusRejectedByFinance}[d.Step]
	default:
		return nil, financeBad("decision must be approve or reject")
	}
	out := make([]domain.FinanceDecisionResult, 0, len(d.InvoiceIDs))
	for _, id := range d.InvoiceIDs {
		r := domain.FinanceDecisionResult{ID: id}
		inv, err := s.repo.Invoice(ctx, id)
		if err != nil {
			return nil, err
		}
		if inv == nil {
			r.Message = financeMsg("Invoice not found.")
			out = append(out, r)
			continue
		}
		job := inv.JobNumber
		r.JobNumber = &job
		if !strings.EqualFold(inv.Status, t.From) {
			r.Message = financeMsg(FinanceInvoiceStatusMsg)
			out = append(out, r)
			continue
		}
		ok, err := s.repo.DecideInvoice(ctx, id, t)
		if err != nil {
			return nil, err
		}
		r.OK = ok
		if !ok {
			r.Message = financeMsg(FinanceInvoiceStatusMsg)
		}
		out = append(out, r)
	}
	return out, nil
}

// ── Payments ────────────────────────────────────────────────────────────

func (s *financeService) withPaymentCampaigns(ctx context.Context, items []domain.FinancePayment) error {
	var ids []string
	seen := map[string]bool{}
	for _, it := range items {
		if it.AllocationID != nil && *it.AllocationID != "" && !seen[*it.AllocationID] {
			seen[*it.AllocationID] = true
			ids = append(ids, *it.AllocationID)
		}
	}
	camps, err := s.alloc.Campaigns(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if a := items[i].AllocationID; a != nil {
			if c, ok := camps[*a]; ok {
				items[i].Campaign = &c
			}
		}
	}
	return nil
}

func (s *financeService) Payments(ctx context.Context, f domain.FinancePaymentFilter) ([]domain.FinancePayment, int, error) {
	if f.JobNumber != "" {
		if err := financeCheckJob(f.JobNumber); err != nil {
			return nil, 0, err
		}
	}
	if len(f.CampaignIDs) > 0 {
		allocs, err := s.alloc.AllocationsOfCampaigns(ctx, f.CampaignIDs)
		if err != nil {
			return nil, 0, err
		}
		if len(allocs) == 0 {
			return []domain.FinancePayment{}, 0, nil
		}
		f.AllocationIDs = allocs
	}
	items, total, err := s.repo.Payments(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	if err := s.withPaymentCampaigns(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *financeService) Payment(ctx context.Context, perDiemNumber string) (domain.FinancePayment, error) {
	if !financePerDiemNumberRe.MatchString(perDiemNumber) {
		return domain.FinancePayment{}, financeBad("Invalid per diem number format")
	}
	p, err := s.repo.Payment(ctx, perDiemNumber)
	if err != nil {
		return domain.FinancePayment{}, err
	}
	if p == nil {
		return domain.FinancePayment{}, &apierror.NotFoundError{Msg: "Per diem payment not found."}
	}
	items := []domain.FinancePayment{*p}
	if err := s.withPaymentCampaigns(ctx, items); err != nil {
		return domain.FinancePayment{}, err
	}
	return items[0], nil
}

func (s *financeService) PaymentCounts(ctx context.Context) (domain.FinancePaymentCounts, error) {
	return s.repo.PaymentCounts(ctx)
}

// AddPayment ports POST /travels/{jn}/per-diems/payments for one row, already
// converted by the backend: the per diem number must be new in the Travel DB
// and in NetSuite vendor bills.
func (s *financeService) AddPayment(ctx context.Context, jobNumber string, in domain.FinancePaymentCreate) (domain.FinancePayment, error) {
	if err := financeCheckJob(jobNumber); err != nil {
		return domain.FinancePayment{}, err
	}
	switch {
	case !financePerDiemNumberRe.MatchString(in.PerDiemNumber) || !strings.HasPrefix(in.PerDiemNumber, jobNumber+"-"):
		return domain.FinancePayment{}, financeBad("perDiemNumber must be %s-NNN", jobNumber)
	case !financeCurrencyRe.MatchString(in.PaymentCurrency) || !financeCurrencyRe.MatchString(in.ConvertedCurrency):
		return domain.FinancePayment{}, financeBad("paymentCurrency and convertedCurrency must be three-letter currency codes")
	case in.PaymentAmount <= 0 || in.ConvertedAmount < 0:
		return domain.FinancePayment{}, financeBad("paymentAmount must be more than 0")
	}
	if err := financeCheckActor(in.CreatedBy); err != nil {
		return domain.FinancePayment{}, err
	}
	if _, err := s.Trip(ctx, jobNumber); err != nil {
		return domain.FinancePayment{}, err
	}
	exists, err := s.repo.PaymentNumberExists(ctx, in.PerDiemNumber)
	if err != nil {
		return domain.FinancePayment{}, err
	}
	if exists {
		return domain.FinancePayment{}, &apierror.ConflictError{Msg: "Per Diem Number already exists in Travel DB"}
	}
	if s.ns.Enabled() {
		found, err := s.ns.LookupVendorBills(ctx, in.PerDiemNumber)
		if err != nil {
			slog.ErrorContext(ctx, "netsuite vendor bill lookup failed", "perDiemNumber", in.PerDiemNumber, "err", err)
			return domain.FinancePayment{}, &apierror.ServiceUnavailableError{Msg: "Error while validating per diem payments"}
		}
		if len(found) > 0 {
			return domain.FinancePayment{}, &apierror.ConflictError{Msg: "Per Diem Number already exists in Net Suite"}
		}
	} else {
		slog.WarnContext(ctx, "NetSuite is disabled: per diem number not checked against NetSuite", "perDiemNumber", in.PerDiemNumber)
	}
	if _, err := s.repo.InsertPayment(ctx, jobNumber, in); err != nil {
		return domain.FinancePayment{}, err
	}
	return s.Payment(ctx, in.PerDiemNumber)
}

// UpdatePayment ports PATCH /travels/{jn}/per-diems/payments for one row:
// only a Pending payment can be edited.
func (s *financeService) UpdatePayment(ctx context.Context, perDiemNumber string, in domain.FinancePaymentUpdate) (domain.FinancePayment, error) {
	if in.PaymentAmount <= 0 || in.ConvertedAmount < 0 {
		return domain.FinancePayment{}, financeBad("paymentAmount must be more than 0")
	}
	if !financeCurrencyRe.MatchString(in.ConvertedCurrency) {
		return domain.FinancePayment{}, financeBad("convertedCurrency must be a three-letter currency code")
	}
	if err := financeCheckActor(in.UpdatedBy); err != nil {
		return domain.FinancePayment{}, err
	}
	p, err := s.Payment(ctx, perDiemNumber)
	if err != nil {
		return domain.FinancePayment{}, err
	}
	ok, err := s.repo.UpdatePayment(ctx, p.ID, in)
	if err != nil {
		return domain.FinancePayment{}, err
	}
	if !ok {
		return domain.FinancePayment{}, &apierror.ConflictError{Msg: "Only a pending payment can be edited."}
	}
	return s.Payment(ctx, perDiemNumber)
}

func (s *financeService) Subsidiary(ctx context.Context, company string) (domain.FinanceSubsidiary, error) {
	if strings.TrimSpace(company) == "" {
		return domain.FinanceSubsidiary{}, financeBad("company is required")
	}
	sub, err := s.expense.Subsidiary(ctx, company)
	if err != nil {
		return domain.FinanceSubsidiary{}, err
	}
	if sub == nil {
		return domain.FinanceSubsidiary{}, &apierror.NotFoundError{Msg: "No Subsidiary found under the name: " + company}
	}
	return *sub, nil
}
