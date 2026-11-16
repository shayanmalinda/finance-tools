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
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
)

// The NetSuite push state machine (LEGACY-ISSUES L2), within the legacy
// columns:
//
//  1. Claim: the legacy wrote "Complete" before calling NetSuite; the port
//     does so with a conditional UPDATE (… WHERE status = 'Accepted by
//     finance' / ns_status = 'Pending'), so of two concurrent pushes only
//     one wins and the other is told the record is already being pushed.
//     While the push runs, an invoice carries InvoiceNSPushingMarker and a
//     payment ns_status "Pushing"; a push stuck for 15 minutes is moved to
//     NS Sync Failures (repository.ExpireStalePushes).
//  2. Lookup before push: the winner looks the invoice number (purchase
//     orders) or per diem number (vendor bills) up in NetSuite. Found: the
//     record stays Complete and nothing is sent. A failed lookup releases
//     the claim (nothing was sent).
//  3. Push. An id back: Complete. Anything else (an error, a timeout, an
//     unreadable answer) may or may not have created the record, so it is
//     marked as an NS Sync Failure: an invoice goes back to "Accepted by
//     finance" with the error in reject_reason (InvoiceNSFailurePrefix); a
//     payment gets ns_status "Failed".
//  4. Re-sync (NS Sync Failures only): a lookup, never a push. Found:
//     Complete. Not found: back in the push queue.
//
// Steps 2 to 4 run detached from the request's context, so a caller that
// gives up mid-way never leaves a claimed record unsettled.

func financeResult(id, number, job string) domain.FinancePushResult {
	return domain.FinancePushResult{ID: id, Number: number, JobNumber: job}
}

func financeSkip(r domain.FinancePushResult, msg string) domain.FinancePushResult {
	r.OK, r.Outcome, r.Message = false, domain.NSOutcomeSkipped, &msg
	return r
}

func (s *financeService) requireNetSuite() error {
	if !s.ns.Enabled() {
		return &apierror.ServiceUnavailableError{Msg: "NetSuite push is not enabled"}
	}
	return nil
}

// settleCtx is the context of the steps after a claim: detached from the
// caller, bounded by the NetSuite timeout plus finalTimeout.
func (s *financeService) settleCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), netsuite.Timeout+s.finalTimeout)
}

func financeFunding(rows []domain.FinanceFundingSource) []netsuite.FundingSource {
	out := make([]netsuite.FundingSource, 0, len(rows))
	for _, f := range rows {
		out = append(out, netsuite.FundingSource{
			BusinessUnit: f.BusinessUnit, ProductUnit: f.ProductUnit, Region: f.SubRegion, Percentage: f.Percentage,
		})
	}
	return out
}

func financeDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// financeEngagementOf resolves a trip's engagement for a push (legacy:
// travel → allocation → engagement, with funding sources).
func (s *financeService) financeEngagementOf(ctx context.Context, jobNumber string) (*domain.FinanceTrip, *domain.FinanceEngagement, []domain.FinanceFundingSource, string, error) {
	trip, err := s.repo.Trip(ctx, jobNumber)
	if err != nil {
		return nil, nil, nil, "", err
	}
	if trip == nil {
		return nil, nil, nil, "Travel request not found.", nil
	}
	if trip.AllocationID == nil || *trip.AllocationID == "" {
		return nil, nil, nil, "No allocation Id found for the travel request: " + jobNumber, nil
	}
	e, err := s.alloc.EngagementOfAllocation(ctx, *trip.AllocationID)
	if err != nil {
		return nil, nil, nil, "", err
	}
	if e == nil {
		return nil, nil, nil, "An error occurred while retrieving engagement data", nil
	}
	funding, err := s.alloc.FundingSources(ctx, e.ID)
	if err != nil {
		return nil, nil, nil, "", err
	}
	return trip, e, funding, "", nil
}

// purchaseOrderMemo ports createMemo.
func purchaseOrderMemo(costTypeName, travellerEmail, convertedCurrency, convertedAmount, customerName, jobNumber string) string {
	if strings.EqualFold(costTypeName, domain.InvoiceCostTypeCurrency) {
		return fmt.Sprintf("%s - %s - (%s %s) - %s - %s", costTypeName, travellerEmail, convertedCurrency, convertedAmount, customerName, jobNumber)
	}
	return fmt.Sprintf("%s - %s - %s - %s", costTypeName, travellerEmail, customerName, jobNumber)
}

// vendorBillMemo ports createMemoForPerDiemVendorBill, including the line
// break and four spaces of the legacy two-line template literal.
func vendorBillMemo(travellerEmail, currency, amount, jobNumber, customerName, engagementCode string) string {
	return fmt.Sprintf("Per Diem for %s - %s %s - %s - \n    %s - %s", travellerEmail, currency, amount, jobNumber, customerName, engagementCode)
}

// legacyAmount formats a float as legacy's float-to-decimal string does
// (10 → "10.0", 4.48 → "4.48").
func legacyAmount(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// ── Invoices ────────────────────────────────────────────────────────────

// buildPurchaseOrder ports the payload of pushInvoicesToNetSuite*. A
// non-empty message means the invoice cannot be pushed (nothing changes);
// the legacy failed the whole batch on such an invoice.
func (s *financeService) buildPurchaseOrder(ctx context.Context, inv domain.FinanceInvoice) (netsuite.PurchaseOrder, string, error) {
	// LKR rule (isValidCurrencyConversion): paid in LKR is pushed only when
	// the invoice is in LKR too.
	if !(inv.ConvertedCurrency != "LKR" || inv.InvoiceCurrency == "LKR") {
		return netsuite.PurchaseOrder{}, "Not pushed: invoices paid in LKR are pushed only when the invoice currency is LKR.", nil
	}
	switch {
	case inv.VendorID == nil:
		return netsuite.PurchaseOrder{}, "Not pushed: the travel agency that added this invoice has no NetSuite vendor id.", nil
	case inv.Department == nil || inv.AccountInternalID == nil:
		return netsuite.PurchaseOrder{}, "Not pushed: the invoice has no account classification department or account.", nil
	case inv.CostTypeName == nil:
		return netsuite.PurchaseOrder{}, "Not pushed: the invoice has no cost type.", nil
	case inv.FinanceApprovedBy == nil:
		return netsuite.PurchaseOrder{}, "Not pushed: the invoice has no finance approver.", nil
	case inv.AddedDate == nil:
		return netsuite.PurchaseOrder{}, "Not pushed: the invoice has no added date.", nil
	}
	trip, eng, funding, msg, err := s.financeEngagementOf(ctx, inv.JobNumber)
	if err != nil || msg != "" {
		return netsuite.PurchaseOrder{}, msg, err
	}
	date, _, _ := strings.Cut(*inv.AddedDate, " ")
	return netsuite.PurchaseOrder{
		TransactionID:   inv.InvoiceNumber,
		Department:      *inv.Department,
		EngagementCode:  eng.EngagementCode,
		TransactionDate: date,
		VendorID:        strconv.Itoa(*inv.VendorID),
		TotalAmount:     inv.InvoiceAmountText,
		BillCurrency:    inv.InvoiceCurrency,
		Memo: purchaseOrderMemo(*inv.CostTypeName, trip.TravellerEmail, inv.ConvertedCurrency, inv.ConvertedAmountText,
			financeDeref(eng.CustomerName), inv.JobNumber),
		Account:        *inv.AccountInternalID,
		TaxCode:        s.ns.PurchaseOrderTaxCode(),
		FundingSources: financeFunding(funding),
		POType:         "Travel-PO",
		JobNumber:      inv.JobNumber,
		InvoiceNumber:  inv.InvoiceNumber,
		ApproverEmail:  *inv.FinanceApprovedBy,
	}, "", nil
}

// PushInvoice pushes one finance-approved invoice as a purchase order.
func (s *financeService) PushInvoice(ctx context.Context, id int, actor string) (out domain.FinancePushResult, err error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinancePushResult{}, err
	}
	if err := financeCheckActor(actor); err != nil {
		return domain.FinancePushResult{}, err
	}
	inv, err := s.repo.Invoice(ctx, id)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if inv == nil {
		return domain.FinancePushResult{}, &apierror.NotFoundError{Msg: "Invoice not found."}
	}
	res := financeResult(strconv.Itoa(id), inv.InvoiceNumber, inv.JobNumber)
	switch {
	case inv.NetSuiteFailed:
		return financeSkip(res, "Its last push did not finish. Re-sync it from NS Sync Failures."), nil
	case strings.EqualFold(inv.Status, domain.InvoiceStatusComplete):
		return financeSkip(res, "Already pushed to NetSuite, or being pushed."), nil
	case !strings.EqualFold(inv.Status, domain.InvoiceStatusAcceptedByFinance):
		return financeSkip(res, "Only invoices accepted by finance can be pushed."), nil
	}
	po, msg, err := s.buildPurchaseOrder(ctx, *inv)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if msg != "" {
		return financeSkip(res, msg), nil
	}
	won, err := s.repo.ClaimInvoicePush(ctx, id, actor)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if !won {
		return financeSkip(res, "Already pushed to NetSuite, or being pushed."), nil
	}

	sctx, cancel := s.settleCtx(ctx)
	defer cancel()
	defer func() { s.auditPush(sctx, "invoice", strconv.Itoa(id), actor, "push", out, err) }()
	found, err := s.ns.LookupPurchaseOrders(sctx, inv.InvoiceNumber)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite lookup before push failed", "invoice", inv.InvoiceNumber, "err", err)
		if rerr := s.repo.ReleaseInvoicePush(sctx, id, actor); rerr != nil {
			return domain.FinancePushResult{}, fmt.Errorf("release invoice %d after a failed lookup: %w", id, rerr)
		}
		return financeSkip(res, "Could not check NetSuite before pushing; nothing was pushed. Please try again."), nil
	}
	if len(found) > 0 {
		ref := found[0].InternalID
		s.storeInvoiceRef(sctx, id, ref)
		res.OK, res.Outcome, res.Reference = true, domain.NSOutcomeFound, &ref
		res.Message = financeMsg("Already in NetSuite; marked as pushed without pushing again.")
		return res, nil
	}
	poID, err := s.ns.CreatePurchaseOrder(sctx, po)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite purchase order push failed", "invoice", inv.InvoiceNumber, "err", err)
		if ferr := s.repo.FailInvoicePush(sctx, id, actor, err.Error()); ferr != nil {
			return domain.FinancePushResult{}, fmt.Errorf("record the failed push of invoice %d: %w", id, ferr)
		}
		res.Outcome, res.Message = domain.NSOutcomeFailed, financeMsg(err.Error())
		return res, nil
	}
	s.storeInvoiceRef(sctx, id, poID)
	res.OK, res.Outcome, res.Reference = true, domain.NSOutcomePushed, &poID
	return res, nil
}

// auditPush records the outcome of a push or re-sync that reached NetSuite;
// a failure is only logged.
func (s *financeService) auditPush(ctx context.Context, entityType, id, actor, op string, res domain.FinancePushResult, err error) {
	md := map[string]any{"operation": op, "number": res.Number, "outcome": res.Outcome, "ok": res.OK}
	if res.Reference != nil {
		md["reference"] = *res.Reference
	}
	if res.Message != nil {
		md["message"] = *res.Message
	}
	if err != nil {
		md["error"] = err.Error()
	}
	e := audit.Event{Action: audit.Push, EntityType: entityType, EntityID: id, ParentType: "travel_request",
		ParentID: res.JobNumber, Actor: actor, Metadata: md}
	if aerr := s.repo.AuditPush(ctx, e); aerr != nil {
		slog.ErrorContext(ctx, "could not record the netsuite outcome", "entity", entityType, "id", id, "err", aerr)
	}
}

// storeInvoiceRef records the purchase order id; the invoice is already
// Complete, so a failure here is only logged.
func (s *financeService) storeInvoiceRef(ctx context.Context, id int, ref string) {
	if err := s.repo.CompleteInvoicePush(ctx, id, ref); err != nil {
		slog.ErrorContext(ctx, "could not store the purchase order id", "invoice", id, "ref", ref, "err", err)
	}
}

// ResyncInvoice settles an invoice on NS Sync Failures by a lookup.
func (s *financeService) ResyncInvoice(ctx context.Context, id int, actor string) (out domain.FinancePushResult, err error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinancePushResult{}, err
	}
	if err := financeCheckActor(actor); err != nil {
		return domain.FinancePushResult{}, err
	}
	inv, err := s.repo.Invoice(ctx, id)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if inv == nil {
		return domain.FinancePushResult{}, &apierror.NotFoundError{Msg: "Invoice not found."}
	}
	res := financeResult(strconv.Itoa(id), inv.InvoiceNumber, inv.JobNumber)
	if !inv.NetSuiteFailed {
		return financeSkip(res, "Only invoices on NS Sync Failures can be re-synced."), nil
	}
	sctx, cancel := s.settleCtx(ctx)
	defer cancel()
	defer func() { s.auditPush(sctx, "invoice", strconv.Itoa(id), actor, "resync", out, err) }()
	found, err := s.ns.LookupPurchaseOrders(sctx, inv.InvoiceNumber)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite re-sync lookup failed", "invoice", inv.InvoiceNumber, "err", err)
		return financeSkip(res, "Could not reach NetSuite. Please try again."), nil
	}
	ref := ""
	if len(found) > 0 {
		ref = found[0].InternalID
	}
	ok, err := s.repo.ResolveInvoiceFailure(sctx, id, actor, ref)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if !ok {
		return financeSkip(res, "The invoice changed while it was being re-synced."), nil
	}
	if len(found) > 0 {
		ref := found[0].InternalID
		res.OK, res.Outcome, res.Reference = true, domain.NSOutcomeFound, &ref
		return res, nil
	}
	res.Outcome = domain.NSOutcomeNotFound
	res.Message = financeMsg("Not found in NetSuite. The invoice is back in Invoices to Push.")
	return res, nil
}

// InvoiceReference is the stored purchase order id, else a lookup (invoices
// pushed by legacy).
func (s *financeService) InvoiceReference(ctx context.Context, id int) (domain.FinanceReference, error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinanceReference{}, err
	}
	inv, err := s.repo.Invoice(ctx, id)
	if err != nil {
		return domain.FinanceReference{}, err
	}
	if inv == nil {
		return domain.FinanceReference{}, &apierror.NotFoundError{Msg: "Invoice not found."}
	}
	if inv.NetSuiteReference != nil {
		return domain.FinanceReference{Reference: inv.NetSuiteReference}, nil
	}
	found, err := s.ns.LookupPurchaseOrders(ctx, inv.InvoiceNumber)
	if err != nil {
		return domain.FinanceReference{}, &apierror.ServiceUnavailableError{Msg: err.Error()}
	}
	if len(found) == 0 {
		return domain.FinanceReference{}, nil
	}
	return domain.FinanceReference{Reference: &found[0].InternalID}, nil
}

// ── Per diem payments ───────────────────────────────────────────────────

// buildVendorBill ports getPerDiemTravelInfo + getEmployeeData and the
// vendor bill payload of pushPerDiemPaymentsToNetSuite*.
func (s *financeService) buildVendorBill(ctx context.Context, p domain.FinancePayment, emp domain.FinanceEmployee) (netsuite.VendorBill, string, error) {
	trip, eng, funding, msg, err := s.financeEngagementOf(ctx, p.JobNumber)
	if err != nil || msg != "" {
		return netsuite.VendorBill{}, msg, err
	}
	costType, err := s.repo.CostTypeIDByName(ctx, domain.InvoiceCostTypeCurrency)
	if err != nil {
		return netsuite.VendorBill{}, "", err
	}
	if costType == nil {
		return netsuite.VendorBill{}, "Not pushed: the Currency cost type is missing.", nil
	}
	cls, err := s.repo.ClassificationID(ctx, "%"+eng.EngagementCode+"%", "", costType)
	if err != nil {
		return netsuite.VendorBill{}, "", err
	}
	if cls == nil {
		parts := strings.Split(eng.EngagementCode, "-")
		if len(parts) < 2 {
			return netsuite.VendorBill{}, "Not pushed: Invalid engagement code structure for invoice classification type retrieval", nil
		}
		if cls, err = s.repo.ClassificationID(ctx, "", "%"+parts[1]+"%", costType); err != nil {
			return netsuite.VendorBill{}, "", err
		}
		if cls == nil {
			return netsuite.VendorBill{}, "Not pushed: No valid invoice classification ID found", nil
		}
	}
	dept, account, err := s.repo.ClassificationData(ctx, *cls)
	if err != nil {
		return netsuite.VendorBill{}, "", err
	}
	if dept == nil || account == nil {
		return netsuite.VendorBill{}, "Not pushed: the account classification has no department or account.", nil
	}
	if strings.TrimSpace(emp.Company) == "" {
		return netsuite.VendorBill{}, "Not pushed: Invalid Company Data", nil
	}
	sub, err := s.expense.Subsidiary(ctx, emp.Company)
	if err != nil {
		return netsuite.VendorBill{}, "", err
	}
	if sub == nil {
		return netsuite.VendorBill{}, "Not pushed: No Subsidiary found under the name: " + emp.Company, nil
	}
	if p.ConvertedAmountText == nil || p.ConvertedCurrency == nil || p.PaymentAmount == nil || p.PaymentCurrency == nil {
		return netsuite.VendorBill{}, "Not pushed: the payment has no amount or converted amount.", nil
	}
	now := s.now().UTC()
	return netsuite.VendorBill{
		TransactionID:  p.PerDiemNumber,
		Department:     *dept,
		EngagementCode: eng.EngagementCode,
		// getCurrentDateString: today, not zero padded ("2026-11-5").
		TransactionDate: fmt.Sprintf("%d-%d-%d", now.Year(), int(now.Month()), now.Day()),
		TotalAmount:     *p.ConvertedAmountText,
		BillCurrency:    *p.ConvertedCurrency,
		Memo: vendorBillMemo(trip.TravellerEmail, *p.PaymentCurrency, legacyAmount(*p.PaymentAmount),
			p.JobNumber, financeDeref(trip.CustomerName), eng.EngagementCode),
		Employee: netsuite.Employee{
			Email: emp.Email, FirstName: emp.FirstName, LastName: emp.LastName, JobRole: emp.JobRole,
			EmployeeID: emp.EmployeeID, Department: emp.Department, Subsidiary: sub.LegalName,
		},
		Account:        *account,
		TaxCode:        sub.TaxCodeInternalID,
		FundingSources: financeFunding(funding),
	}, "", nil
}

// PushPayment pushes one Pending payment as a vendor bill. req.Employee is
// the traveller's HR record (the legacy vendor bill "employee").
func (s *financeService) PushPayment(ctx context.Context, perDiemNumber string, req domain.FinancePushRequest) (out domain.FinancePushResult, err error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinancePushResult{}, err
	}
	if err := financeCheckActor(req.UpdatedBy); err != nil {
		return domain.FinancePushResult{}, err
	}
	if req.Employee == nil {
		return domain.FinancePushResult{}, financeBad("employee (the traveller's HR record) is required")
	}
	p, err := s.Payment(ctx, perDiemNumber)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	res := financeResult(perDiemNumber, perDiemNumber, p.JobNumber)
	switch status := financeDeref(p.NSStatus); {
	case strings.EqualFold(status, domain.PaymentNSFailed):
		return financeSkip(res, "Its last push did not finish. Re-sync it from NS Sync Failures."), nil
	case strings.EqualFold(status, domain.PaymentNSComplete), strings.EqualFold(status, domain.PaymentNSPushing):
		return financeSkip(res, "Already pushed to NetSuite, or being pushed."), nil
	case !strings.EqualFold(status, domain.PaymentNSPending):
		return financeSkip(res, "Only pending payments can be pushed."), nil
	}
	vb, msg, err := s.buildVendorBill(ctx, p, *req.Employee)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if msg != "" {
		return financeSkip(res, msg), nil
	}
	won, err := s.repo.ClaimPaymentPush(ctx, p.ID, req.UpdatedBy)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if !won {
		return financeSkip(res, "Already pushed to NetSuite, or being pushed."), nil
	}

	sctx, cancel := s.settleCtx(ctx)
	defer cancel()
	defer func() { s.auditPush(sctx, "per_diem_payment", strconv.Itoa(p.ID), req.UpdatedBy, "push", out, err) }()
	found, err := s.ns.LookupVendorBills(sctx, perDiemNumber)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite lookup before push failed", "perDiemNumber", perDiemNumber, "err", err)
		if rerr := s.repo.ReleasePaymentPush(sctx, p.ID, req.UpdatedBy); rerr != nil {
			return domain.FinancePushResult{}, fmt.Errorf("release payment %s after a failed lookup: %w", perDiemNumber, rerr)
		}
		return financeSkip(res, "Could not check NetSuite before pushing; nothing was pushed. Please try again."), nil
	}
	if len(found) > 0 {
		ref := found[0].InternalID
		s.completePayment(sctx, p.ID, req.UpdatedBy)
		res.OK, res.Outcome, res.Reference = true, domain.NSOutcomeFound, &ref
		res.Message = financeMsg("Already in NetSuite; marked as pushed without pushing again.")
		return res, nil
	}
	vbID, err := s.ns.CreateVendorBill(sctx, vb)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite vendor bill push failed", "perDiemNumber", perDiemNumber, "err", err)
		if ferr := s.repo.FailPaymentPush(sctx, p.ID, req.UpdatedBy); ferr != nil {
			return domain.FinancePushResult{}, fmt.Errorf("record the failed push of payment %s: %w", perDiemNumber, ferr)
		}
		res.Outcome, res.Message = domain.NSOutcomeFailed, financeMsg(err.Error())
		return res, nil
	}
	s.completePayment(sctx, p.ID, req.UpdatedBy)
	res.OK, res.Outcome, res.Reference = true, domain.NSOutcomePushed, &vbID
	return res, nil
}

// completePayment ends the Pushing state; on a failure the stale-push sweep
// moves the payment to NS Sync Failures, so it is only logged.
func (s *financeService) completePayment(ctx context.Context, id int, actor string) {
	if err := s.repo.CompletePaymentPush(ctx, id, actor); err != nil {
		slog.ErrorContext(ctx, "could not mark the payment as pushed", "payment", id, "err", err)
	}
}

// ResyncPayment settles a payment on NS Sync Failures by a lookup.
func (s *financeService) ResyncPayment(ctx context.Context, perDiemNumber, actor string) (out domain.FinancePushResult, err error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinancePushResult{}, err
	}
	if err := financeCheckActor(actor); err != nil {
		return domain.FinancePushResult{}, err
	}
	p, err := s.Payment(ctx, perDiemNumber)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	res := financeResult(perDiemNumber, perDiemNumber, p.JobNumber)
	if !strings.EqualFold(financeDeref(p.NSStatus), domain.PaymentNSFailed) {
		return financeSkip(res, "Only payments on NS Sync Failures can be re-synced."), nil
	}
	sctx, cancel := s.settleCtx(ctx)
	defer cancel()
	defer func() { s.auditPush(sctx, "per_diem_payment", strconv.Itoa(p.ID), actor, "resync", out, err) }()
	found, err := s.ns.LookupVendorBills(sctx, perDiemNumber)
	if err != nil {
		slog.ErrorContext(sctx, "netsuite re-sync lookup failed", "perDiemNumber", perDiemNumber, "err", err)
		return financeSkip(res, "Could not reach NetSuite. Please try again."), nil
	}
	ok, err := s.repo.ResolvePaymentFailure(sctx, p.ID, actor, len(found) > 0)
	if err != nil {
		return domain.FinancePushResult{}, err
	}
	if !ok {
		return financeSkip(res, "The payment changed while it was being re-synced."), nil
	}
	if len(found) > 0 {
		ref := found[0].InternalID
		res.OK, res.Outcome, res.Reference = true, domain.NSOutcomeFound, &ref
		return res, nil
	}
	res.Outcome = domain.NSOutcomeNotFound
	res.Message = financeMsg("Not found in NetSuite. The payment is back in Per Diem Payments to Push.")
	return res, nil
}

// PaymentReference looks a pushed payment's vendor bill up.
func (s *financeService) PaymentReference(ctx context.Context, perDiemNumber string) (domain.FinanceReference, error) {
	if err := s.requireNetSuite(); err != nil {
		return domain.FinanceReference{}, err
	}
	if !financePerDiemNumberRe.MatchString(perDiemNumber) {
		return domain.FinanceReference{}, financeBad("Invalid per diem number format")
	}
	found, err := s.ns.LookupVendorBills(ctx, perDiemNumber)
	if err != nil {
		return domain.FinanceReference{}, &apierror.ServiceUnavailableError{Msg: err.Error()}
	}
	if len(found) == 0 {
		return domain.FinanceReference{}, nil
	}
	return domain.FinanceReference{Reference: &found[0].InternalID}, nil
}
