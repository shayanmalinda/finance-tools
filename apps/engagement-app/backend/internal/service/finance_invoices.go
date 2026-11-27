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
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// Invoice tabs of Invoices & Payments.
const (
	FinanceTabApprovePO = "approve-po"
	FinanceTabApprove   = "approve"
	FinanceTabPush      = "push"
	FinanceTabAll       = "all"
)

func poApprovalOf(status string) string {
	switch {
	case strings.EqualFold(status, financeStatusPending):
		return domain.ApprovalPending
	case strings.EqualFold(status, financeStatusRejectedByTravel):
		return domain.ApprovalRejected
	}
	return domain.ApprovalApproved
}

func financeApprovalOf(status string) string {
	switch {
	case strings.EqualFold(status, financeStatusRejectedByFinance):
		return domain.ApprovalRejected
	case strings.EqualFold(status, financeStatusAcceptedByFinance), strings.EqualFold(status, financeStatusComplete):
		return domain.ApprovalApproved
	}
	return domain.ApprovalPending
}

// financeStep is the approval step the caller acts on: PO approval with
// Travel 32, finance approval with 33 and not 32 (legacy: the HR branch
// wins, so a holder of both can never finance-approve).
func financeStep(p domain.Perms) string {
	switch {
	case p.Travel.Has(domain.TravelPrivInvoiceHRApproval):
		return "travel"
	case p.Travel.Has(domain.TravelPrivInvoiceFinance):
		return "finance"
	}
	return ""
}

func (s *FinanceService) invoiceOf(p domain.Perms, inv engagemententity.FinanceInvoice) domain.Invoice {
	out := domain.Invoice{
		ID: inv.ID, InvoiceNumber: inv.InvoiceNumber, JobNumber: inv.JobNumber, Traveller: financePerson(inv.TravellerEmail),
		Agency: inv.AgencyName, Month: inv.Month, CostType: financeDeref(inv.CostTypeName),
		Amount: inv.InvoiceAmount, Currency: inv.InvoiceCurrency, PaidAmount: inv.ConvertedAmount, PaidCurrency: inv.ConvertedCurrency,
		Status: inv.Status, PoApproval: poApprovalOf(inv.Status), FinanceApproval: financeApprovalOf(inv.Status),
		PoApprovedBy: inv.HRApprovedBy, FinanceApprovedBy: inv.FinanceApprovedBy, RejectReason: inv.RejectReason,
		Campaign: financeCampaign(inv.Campaign), TravelStartDate: financeDate(inv.TravelStartDate), TravelEndDate: financeDate(inv.TravelEndDate),
		AddedOn: financeDate(inv.AddedDate), AddedBy: inv.AddedBy, AgentEmail: inv.AgentEmail,
		NetSuite: domain.NetSuiteState{Status: domain.NetSuitePending},
	}
	if p.IsAgent() {
		out.Agency = nil // their own agency
	}
	var place []string
	for _, v := range []*string{inv.City, inv.Country} {
		if v != nil && strings.TrimSpace(*v) != "" {
			place = append(place, strings.TrimSpace(*v))
		}
	}
	if len(place) > 0 {
		out.Destination = financeStr(strings.Join(place, ", "))
	}
	switch {
	case inv.NetSuitePushing:
		out.NetSuite.Status = domain.NetSuitePushing
	case strings.EqualFold(inv.Status, financeStatusComplete):
		out.NetSuite.Status, out.NetSuite.Reference = domain.NetSuitePushed, inv.NetSuiteReference
	case inv.NetSuiteFailed:
		out.NetSuite.Status, out.NetSuite.Error = domain.NetSuiteFailed, inv.NetSuiteError
	}
	step := financeStep(p)
	out.Actions.ApprovePo = step == "travel" && strings.EqualFold(inv.Status, financeStatusPending)
	out.Actions.Approve = step == "finance" && strings.EqualFold(inv.Status, financeStatusAcceptedByTravel)
	out.Actions.Reject = out.Actions.ApprovePo || out.Actions.Approve
	canPush := p.Travel.Has(domain.TravelPrivPushInvoices)
	out.Actions.Push = canPush && strings.EqualFold(inv.Status, financeStatusAcceptedByFinance) && !inv.NetSuiteFailed
	out.Actions.Resync = canPush && inv.NetSuiteFailed
	return out
}

func (s *FinanceService) invoicesOf(ctx context.Context, p domain.Perms, rows []engagemententity.FinanceInvoice) []domain.Invoice {
	out := make([]domain.Invoice, 0, len(rows))
	refs := make([]*domain.CampaignRef, 0, len(rows))
	people := make([]*domain.Person, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.invoiceOf(p, r))
	}
	for i := range out {
		refs = append(refs, out[i].Campaign)
		people = append(people, out[i].Traveller)
	}
	s.Campaigns.Fill(ctx, refs...)
	s.people.fillNames(ctx, people...)
	return out
}

func financeAgencyID(p domain.Perms) int {
	if p.IsAgent() {
		return p.Agency.ID
	}
	return 0
}

// financeTripInvoiceLimit is the most invoices the trip list returns.
const financeTripInvoiceLimit = 1000

// TripInvoices is GET /travel-requests/{jobNumber}/invoices (legacy
// travelInvoices(filter:{jobNumber}, limit 1000)); agents see only their
// agency's invoices.
func (s *FinanceService) TripInvoices(ctx context.Context, p domain.Perms, jobNumber string) (domain.InvoicePage, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.InvoicePage{}, err
	}
	if _, err := viewScope(p, trip, true, []int{domain.TravelPrivViewInvoices}, domain.TravelPrivViewOwnInvoices); err != nil {
		return domain.InvoicePage{}, err
	}
	rows, total, err := s.entity.FinanceInvoices(ctx, engagemententity.FinanceInvoiceFilter{
		JobNumber: jobNumber, AgencyID: financeAgencyID(p), Limit: financeTripInvoiceLimit})
	if err != nil {
		return domain.InvoicePage{}, err
	}
	return domain.InvoicePage{Items: s.invoicesOf(ctx, p, rows), Total: total}, nil
}

// AddInvoices is POST /travel-requests/{jobNumber}/invoices: trip In
// Progress, Extended or Cancelled (the legacy UI rule); cost types by name;
// one "New travel invoices have been added" email per invoice (legacy).
func (s *FinanceService) AddInvoices(ctx context.Context, p domain.Perms, jobNumber string, rows []domain.InvoiceCreate) (domain.InvoicePage, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.InvoicePage{}, err
	}
	if err := writeScope(p, trip); err != nil {
		return domain.InvoicePage{}, err
	}
	if !financeOpen(trip.Status) && !strings.EqualFold(financeDeref(trip.Status), "Cancelled") {
		return domain.InvoicePage{}, financeBad("Invoices can be added only while the travel request is In Progress, Extended or Cancelled.")
	}
	if len(rows) == 0 || len(rows) > 50 {
		return domain.InvoicePage{}, financeBad("Add 1 to 50 invoices at a time.")
	}
	types, err := s.entity.FinanceCostTypes(ctx)
	if err != nil {
		return domain.InvoicePage{}, err
	}
	in := engagemententity.FinanceInvoicesCreate{CreatedBy: p.Email}
	for _, r := range rows {
		id := 0
		for _, t := range types {
			if strings.EqualFold(strings.TrimSpace(r.CostType), t.Name) {
				id = t.ID
			}
		}
		var missing []string
		paidAmt, paidCur := 0.0, ""
		if r.PaidAmount != nil {
			paidAmt = *r.PaidAmount
		}
		if r.PaidCurrency != nil {
			paidCur = strings.ToUpper(strings.TrimSpace(*r.PaidCurrency))
		}
		for name, v := range map[string]string{"Invoice Number": r.InvoiceNumber, "Month": r.Month, "Cost Type": r.CostType,
			"Invoice Currency": r.Currency} {
			if strings.TrimSpace(v) == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 || r.Amount <= 0 || paidAmt < 0 || (paidAmt > 0 && paidCur == "") {
			return domain.InvoicePage{}, financeBad("Please re-check the information. Invoice Number, Month, Cost Type, Invoice Amount and Invoice Currency are mandatory; a Paid Amount needs its Paid Currency.")
		}
		if id == 0 {
			return domain.InvoicePage{}, financeBad("Unknown cost type %q.", r.CostType)
		}
		cur := strings.ToUpper(strings.TrimSpace(r.Currency))
		if cur != "LKR" && cur != "USD" {
			return domain.InvoicePage{}, financeBad("Invoice Currency must be LKR or USD.")
		}
		in.Invoices = append(in.Invoices, engagemententity.FinanceInvoiceCreate{
			InvoiceNumber: strings.TrimSpace(r.InvoiceNumber), Month: strings.TrimSpace(r.Month), CostTypeID: id,
			InvoiceCurrency: cur, InvoiceAmount: r.Amount,
			ConvertedCurrency: paidCur, ConvertedAmount: paidAmt,
		})
	}
	created, err := s.entity.FinanceAddInvoices(ctx, jobNumber, in)
	if err != nil {
		return domain.InvoicePage{}, err
	}
	typeNames := make(map[int]string, len(types))
	for _, t := range types {
		typeNames[t.ID] = t.Name
	}
	for _, c := range created {
		s.notify(ctx, email.FinanceInvoicesAdded, trip, "invoices",
			email.FinanceItems{Invoices: []email.FinanceInvoiceItem{financeInvoiceItem(c, typeNames)}})
	}
	return domain.InvoicePage{Items: s.invoicesOf(ctx, p, created), Total: len(created)}, nil
}

// invoiceTabs are the tabs the caller sees: the work queues by the
// caller's own privileges (super admin grants excluded), All with Travel 39.
// The finance step follows the legacy rule on every privilege held: a caller
// who holds Invoice HR Approval (32) through any group acts at the PO step
// only, so the finance queue is shown only to holders of own 33 without 32.
func invoiceTabs(p domain.Perms) []string {
	var tabs []string
	own := p.TravelOwn
	if own.Has(domain.TravelPrivInvoiceHRApproval) {
		tabs = append(tabs, FinanceTabApprovePO)
	}
	if own.Has(domain.TravelPrivInvoiceFinance) && !p.Travel.Has(domain.TravelPrivInvoiceHRApproval) {
		tabs = append(tabs, FinanceTabApprove)
	}
	if own.Has(domain.TravelPrivPushInvoices) {
		tabs = append(tabs, FinanceTabPush)
	}
	if p.Travel.Has(domain.TravelPrivViewAllInvoicesPage) {
		tabs = append(tabs, FinanceTabAll)
	}
	return tabs
}

func financeHas(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// financeStatusFilter intersects the statuses a set of chip filters allows.
func financeStatusFilter(base []string, q domain.InvoiceQuery) []string {
	keep := func(status string) bool {
		if len(q.PoApproval) > 0 && !financeHas(q.PoApproval, poApprovalOf(status)) {
			return false
		}
		if len(q.FinanceApproval) > 0 && !financeHas(q.FinanceApproval, financeApprovalOf(status)) {
			return false
		}
		if len(q.NetSuiteStatus) > 0 {
			ns := domain.NetSuitePending
			if status == financeStatusComplete {
				ns = domain.NetSuitePushed
			}
			if !financeHas(q.NetSuiteStatus, ns) {
				return false
			}
		}
		return true
	}
	out := []string{}
	for _, st := range base {
		if keep(st) {
			out = append(out, st)
		}
	}
	return out
}

// Invoices is GET /invoices: the tab's rows (own queues or All) with the
// size of every tab the caller sees.
func (s *FinanceService) Invoices(ctx context.Context, p domain.Perms, q domain.InvoiceQuery) (domain.InvoicePage, error) {
	tabs := invoiceTabs(p)
	if q.Tab == "" {
		if financeHas(tabs, FinanceTabAll) || len(tabs) == 0 {
			q.Tab = FinanceTabAll
		} else {
			q.Tab = tabs[0]
		}
	}
	if !financeHas([]string{FinanceTabApprovePO, FinanceTabApprove, FinanceTabPush, FinanceTabAll}, q.Tab) {
		return domain.InvoicePage{}, financeBad("tab must be approve-po, approve, push or all.")
	}
	// A tab the caller does not have: no rows, only their own tab counts.
	allowed := financeHas(tabs, q.Tab)
	if !allowed && len(tabs) == 0 {
		return domain.InvoicePage{}, financeForbidden()
	}
	f := engagemententity.FinanceInvoiceFilter{
		Search: strings.TrimSpace(q.Search), CostTypes: q.CostTypes, Agencies: q.Agencies, Months: q.Months,
		CampaignIDs: q.CampaignIDs, AgencyID: financeAgencyID(p), Sort: domain.KnownSort(q.Sort, domain.InvoiceSorts),
		Limit: q.Limit, Offset: q.Offset,
	}
	base := financeAllStatuses
	switch q.Tab {
	case FinanceTabApprovePO:
		base = []string{financeStatusPending}
	case FinanceTabApprove:
		base = []string{financeStatusAcceptedByTravel}
	case FinanceTabPush:
		base, f.Failed = []string{financeStatusAcceptedByFinance}, "false"
	}
	f.Statuses = financeStatusFilter(base, q)
	page := domain.InvoicePage{Items: []domain.Invoice{}, TabCounts: map[string]int{}}
	if allowed && len(f.Statuses) > 0 {
		if len(f.Statuses) == len(financeAllStatuses) {
			f.Statuses = nil
		}
		rows, total, err := s.entity.FinanceInvoices(ctx, f)
		if err != nil {
			return domain.InvoicePage{}, err
		}
		page.Items, page.Total = s.invoicesOf(ctx, p, rows), total
	}
	counts, err := s.entity.FinanceInvoiceCounts(ctx, f.AgencyID)
	if err != nil {
		return domain.InvoicePage{}, err
	}
	for _, t := range tabs {
		page.TabCounts[t] = map[string]int{FinanceTabApprovePO: counts.Pending, FinanceTabApprove: counts.AcceptedByTravel,
			FinanceTabPush: counts.AcceptedByFinance, FinanceTabAll: counts.All}[t]
	}
	return page, nil
}

// Invoice is GET /invoices/{invoiceId}: any invoice with Travel 30 or 39
// (agents: their agency's), the traveller's own with 38. A pushed
// invoice's purchase order is looked up live (not stored).
func (s *FinanceService) Invoice(ctx context.Context, p domain.Perms, id int) (domain.Invoice, error) {
	inv, err := s.entity.FinanceInvoice(ctx, id)
	if isFinanceNotFound(err) {
		return domain.Invoice{}, &apierror.NotFoundError{Msg: "Invoice not found."}
	}
	if err != nil {
		return domain.Invoice{}, err
	}
	switch {
	case p.IsAgent():
		if inv.AgencyID == nil || *inv.AgencyID != p.Agency.ID {
			return domain.Invoice{}, financeForbidden()
		}
	case p.Travel.Any(domain.TravelPrivViewInvoices, domain.TravelPrivViewAllInvoicesPage):
	case p.Travel.Has(domain.TravelPrivViewOwnInvoices) && strings.EqualFold(financeDeref(inv.TravellerEmail), p.Email):
	default:
		return domain.Invoice{}, financeForbidden()
	}
	out := s.invoiceOf(p, inv)
	s.Campaigns.Fill(ctx, out.Campaign)
	s.people.fillNames(ctx, out.Traveller)
	if strings.EqualFold(inv.Status, financeStatusComplete) && out.NetSuite.Reference == nil {
		if ref, err := s.entity.FinanceInvoiceReference(ctx, id); err == nil {
			out.NetSuite.Reference = ref
		}
	}
	return out, nil
}

// decide runs an approve or reject at the caller's step and sends the
// legacy single email for the batch (job number of the last invoice
// updated).
func (s *FinanceService) decide(ctx context.Context, p domain.Perms, ids []int, decision, reason string) (domain.BatchResult, error) {
	step := financeStep(p)
	if step == "" {
		return domain.BatchResult{}, financeForbidden()
	}
	if len(ids) == 0 || len(ids) > 200 {
		return domain.BatchResult{}, financeBad("Select 1 to 200 invoices.")
	}
	res, err := s.entity.FinanceDecideInvoices(ctx, engagemententity.FinanceInvoiceDecision{
		InvoiceIDs: ids, Decision: decision, Step: step, Reason: reason, UpdatedBy: p.Email})
	if err != nil {
		return domain.BatchResult{}, err
	}
	out := domain.BatchResult{Results: make([]domain.BatchItem, 0, len(res))}
	lastJob := ""
	for _, r := range res {
		out.Results = append(out.Results, domain.BatchItem{ID: strconv.Itoa(r.ID), OK: r.OK, Message: r.Message})
		if r.OK && r.JobNumber != nil {
			lastJob = *r.JobNumber
		}
	}
	// The email is about lastJob: list the invoices of this batch that were
	// decided on that trip.
	decided := map[int]bool{}
	for _, r := range res {
		if r.OK && r.JobNumber != nil && *r.JobNumber == lastJob {
			decided[r.ID] = true
		}
	}
	if lastJob != "" {
		kind := email.FinanceInvoicesApproved
		if decision == "reject" {
			kind = email.FinanceInvoicesRejected
		}
		trip := engagemententity.FinanceTrip{JobNumber: lastJob}
		if t, err := s.entity.FinanceTrip(ctx, lastJob); err == nil {
			trip = t
			trip.JobNumber = lastJob
		}
		items := email.FinanceItems{}
		if decision == "reject" {
			items.Reason = strings.TrimSpace(reason)
		}
		// Page through every invoice of the trip: a long trip can have more
		// than one page, and the email lists every invoice the batch decided.
		const pageSize = 500
		for offset := 0; ; offset += pageSize {
			rows, total, err := s.entity.FinanceInvoices(ctx, engagemententity.FinanceInvoiceFilter{JobNumber: lastJob, Limit: pageSize, Offset: offset})
			if err != nil {
				// The action stands: send the email without the invoice list.
				slog.WarnContext(ctx, "finance email: invoices not listed", "job", lastJob, "err", err)
				items.Invoices = nil
				break
			}
			for _, row := range rows {
				if decided[row.ID] {
					items.Invoices = append(items.Invoices, financeInvoiceItem(row, nil))
					// Once only: a row inserted between reads shifts the page
					// boundary, so the same invoice can come back on the next page.
					delete(decided, row.ID)
				}
			}
			if len(decided) == 0 || len(rows) == 0 || offset+len(rows) >= total {
				break
			}
		}
		s.notify(ctx, kind, trip, "invoices", items)
	}
	return out, nil
}

// ApproveInvoices is POST /invoices/approve.
func (s *FinanceService) ApproveInvoices(ctx context.Context, p domain.Perms, ids []int) (domain.BatchResult, error) {
	return s.decide(ctx, p, ids, "approve", "")
}

// RejectInvoices is POST /invoices/reject.
func (s *FinanceService) RejectInvoices(ctx context.Context, p domain.Perms, ids []int, reason string) (domain.BatchResult, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.BatchResult{}, financeBad("Please enter a reject reason")
	}
	if len(reason) > 255 {
		return domain.BatchResult{}, financeBad("The reject reason must be 255 characters or fewer.")
	}
	return s.decide(ctx, p, ids, "reject", reason)
}

// financeOutOfTime reports whether a bulk action should start no new
// record: less than batchReserve is left before the request's deadline.
func (s *FinanceService) financeOutOfTime(ctx context.Context) bool {
	dl, ok := ctx.Deadline()
	return ok && time.Until(dl) < s.batchReserve
}

const financeOutOfTimeMsg = "Not started: the request ran out of time. Try this record again."

// financeOutcomeUnknownMsg is a record whose call was in flight when the
// batch stopped (the request ended, or an ambiguous 503 came back), so it
// may or may not have run.
const financeOutcomeUnknownMsg = "Outcome unknown: no answer was received for this record. Check its NetSuite status before trying again."

// financeNetSuiteOffMsg is a record not started because NetSuite push was
// switched off during the batch.
const financeNetSuiteOffMsg = "Not started: NetSuite push is not enabled."

// financeEntityDownMsg is a record not started because the Engagement
// Entity was unavailable.
const financeEntityDownMsg = "Not started: the Engagement Entity is unavailable. Try this record again."

// errNetSuiteOff is the stop error for the entity's NetSuite-disabled 503,
// told apart from any other 503 by the entity's message.
var errNetSuiteOff = &apierror.ServiceUnavailableError{Msg: "NetSuite push is not enabled"}

// errEntityDown is the stop error for any other 503 from the entity.
var errEntityDown = &apierror.ServiceUnavailableError{Msg: "The Engagement Entity is unavailable"}

// pushItem maps an entity push or re-sync outcome.
func pushItem(id string, r engagemententity.FinancePushResult) domain.BatchItem {
	return domain.BatchItem{ID: id, OK: r.OK, Message: r.Message, NetSuiteReference: r.Reference}
}

// financeRecordErr turns a per-record error into its message; a 503
// (NetSuite off) or a context error stops the batch.
func financeRecordErr(err error) (*string, error) {
	var sue *apierror.ServiceUnavailableError
	var up *apierror.UpstreamError
	if errors.As(err, &sue) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if errors.As(err, &up) {
		if up.StatusCode == 503 {
			if up.Msg == errNetSuiteOff.Msg {
				return nil, errNetSuiteOff
			}
			return nil, errEntityDown
		}
		if up.ClientError() {
			return financeStr(up.Msg), nil
		}
	}
	return financeStr("No answer from the Engagement Entity. Check the record's NetSuite status before trying again."), nil
}

func dedupeInts(ids []int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// PushInvoices is POST /invoices/push: each invoice in turn (one push in
// flight, lookup before push, failures to NS Sync Failures: all in the
// entity). Records not started before the request's time runs out are
// reported and stay in the queue.
func (s *FinanceService) PushInvoices(ctx context.Context, p domain.Perms, ids []int) (domain.BatchResult, error) {
	return s.invoiceBatch(ctx, p, ids, s.entity.FinancePushInvoice)
}

// ResyncInvoices is POST /invoices/resync (lookup only).
func (s *FinanceService) ResyncInvoices(ctx context.Context, p domain.Perms, ids []int) (domain.BatchResult, error) {
	return s.invoiceBatch(ctx, p, ids, s.entity.FinanceResyncInvoice)
}

func (s *FinanceService) invoiceBatch(ctx context.Context, p domain.Perms, ids []int,
	call func(context.Context, int, string) (engagemententity.FinancePushResult, error)) (domain.BatchResult, error) {
	ids = dedupeInts(ids)
	if len(ids) == 0 || len(ids) > 200 {
		return domain.BatchResult{}, financeBad("Select 1 to 200 invoices.")
	}
	out := domain.BatchResult{Results: []domain.BatchItem{}}
	for i, id := range ids {
		sid := strconv.Itoa(id)
		if s.financeOutOfTime(ctx) {
			out.Results = append(out.Results, domain.BatchItem{ID: sid, Message: financeStr(financeOutOfTimeMsg)})
			continue
		}
		rest := func() []string {
			r := make([]string, 0, len(ids)-i-1)
			for _, n := range ids[i+1:] {
				r = append(r, strconv.Itoa(n))
			}
			return r
		}
		// Canceled with time to spare: stop before the call, so the record
		// is reported as not started rather than outcome unknown.
		if err := ctx.Err(); err != nil {
			return financeBatchStop(out, sid, false, rest(), err)
		}
		r, err := call(ctx, id, p.Email)
		if err != nil {
			msg, stop := financeRecordErr(err)
			if stop != nil {
				return financeBatchStop(out, sid, true, rest(), stop)
			}
			out.Results = append(out.Results, domain.BatchItem{ID: sid, Message: msg})
			continue
		}
		out.Results = append(out.Results, pushItem(sid, r))
	}
	return out, nil
}

// financeBatchStop ends a push or re-sync batch on a stop error (NetSuite
// off, the entity unavailable, or the request's context ended). When
// nothing has a result yet and current's call is known not to have run (no
// call in flight, or the entity's own NetSuite-off answer), the error is
// the batch's answer. Otherwise the records already answered keep their
// results, so the caller knows what was pushed: current is reported
// (outcome unknown when its call was in flight and may have run, even for
// the first record) and the rest as not started.
func financeBatchStop(out domain.BatchResult, current string, inFlight bool, rest []string,
	stop error) (domain.BatchResult, error) {
	ended := errors.Is(stop, context.Canceled) || errors.Is(stop, context.DeadlineExceeded)
	mayHaveRun := inFlight && (ended || errors.Is(stop, errEntityDown))
	if len(out.Results) == 0 && !mayHaveRun {
		return domain.BatchResult{}, stop
	}
	var cur, next *string
	switch {
	case ended:
		cur, next = financeStr(financeOutOfTimeMsg), financeStr(financeOutOfTimeMsg)
		if inFlight {
			cur = financeStr(financeOutcomeUnknownMsg)
		}
	case errors.Is(stop, errNetSuiteOff):
		cur, next = financeStr(financeNetSuiteOffMsg), financeStr(financeNetSuiteOffMsg)
	default:
		cur, next = financeStr(financeEntityDownMsg), financeStr(financeEntityDownMsg)
		if inFlight {
			// A 503 in front of the entity can come after the push started.
			cur = financeStr(financeOutcomeUnknownMsg)
		}
	}
	out.Results = append(out.Results, domain.BatchItem{ID: current, Message: cur})
	for _, id := range rest {
		out.Results = append(out.Results, domain.BatchItem{ID: id, Message: next})
	}
	return out, nil
}

// InvoiceReport is GET /reports/invoices: invoices added from..to
// (inclusive), newest first.
func (s *FinanceService) InvoiceReport(ctx context.Context, p domain.Perms, from, to string) (domain.InvoicePage, error) {
	if err := financeRange(from, to); err != nil {
		return domain.InvoicePage{}, err
	}
	rows, total, err := s.entity.FinanceInvoices(ctx, engagemententity.FinanceInvoiceFilter{
		AddedFrom: from, AddedTo: to, AgencyID: financeAgencyID(p)})
	if err != nil {
		return domain.InvoicePage{}, err
	}
	return domain.InvoicePage{Items: s.invoicesOf(ctx, p, rows), Total: total}, nil
}

func financeRange(from, to string) error {
	if from == "" || to == "" {
		return financeBad("from and to are required.")
	}
	if from > to {
		return financeBad("Start Date must be on or before End Date")
	}
	return nil
}

// financeInvoiceItem is the email view of an invoice; the cost type name comes
// from the row, else from names (by cost type id).
func financeInvoiceItem(inv engagemententity.FinanceInvoice, names map[int]string) email.FinanceInvoiceItem {
	costType := financeDeref(inv.CostTypeName)
	if costType == "" {
		costType = names[inv.CostTypeID]
	}
	return email.FinanceInvoiceItem{Number: inv.InvoiceNumber, CostType: costType, Month: inv.Month,
		Currency: inv.InvoiceCurrency, Amount: inv.InvoiceAmount, Agency: financeDeref(inv.AgencyName),
		PaidCurrency: inv.ConvertedCurrency, PaidAmount: inv.ConvertedAmount}
}
