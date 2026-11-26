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

package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// FinanceHandler serves the finance screen area.
type FinanceHandler struct{ svc *service.FinanceService }

// NewFinanceHandler constructs a FinanceHandler.
func NewFinanceHandler(svc *service.FinanceService) *FinanceHandler { return &FinanceHandler{svc: svc} }

func financePathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil || id <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive whole number."})
		return 0, false
	}
	return id, true
}

// PerDiems handles GET /travel-requests/{jobNumber}/per-diems.
func (h *FinanceHandler) PerDiems(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	items, err := h.svc.PerDiems(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

// AddPerDiem handles POST /travel-requests/{jobNumber}/per-diems.
func (h *FinanceHandler) AddPerDiem(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.PerDiemCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	pd, err := h.svc.AddPerDiem(r.Context(), p, r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, pd)
}

// DeletePerDiem handles DELETE /travel-requests/{jobNumber}/per-diems/{perDiemId}.
func (h *FinanceHandler) DeletePerDiem(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := financePathInt(w, r, "perDiemId")
	if !ok {
		return
	}
	if err := h.svc.DeletePerDiem(r.Context(), p, r.PathValue("jobNumber"), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TripInvoices handles GET /travel-requests/{jobNumber}/invoices.
func (h *FinanceHandler) TripInvoices(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	page, err := h.svc.TripInvoices(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, page)
}

// AddInvoices handles POST /travel-requests/{jobNumber}/invoices.
func (h *FinanceHandler) AddInvoices(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in struct {
		Invoices []domain.InvoiceCreate `json:"invoices"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	page, err := h.svc.AddInvoices(r.Context(), p, r.PathValue("jobNumber"), in.Invoices)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, page)
}

// TripExpenses handles GET /travel-requests/{jobNumber}/expenses.
func (h *FinanceHandler) TripExpenses(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	out, err := h.svc.TripExpenses(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, out)
}

// TravelCards handles GET /travel-requests/{jobNumber}/travel-cards.
func (h *FinanceHandler) TravelCards(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	items, err := h.svc.TravelCards(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

// AddTravelCard handles POST /travel-requests/{jobNumber}/travel-cards.
func (h *FinanceHandler) AddTravelCard(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.TravelCardCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.AddTravelCard(r.Context(), p, r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, c)
}

// TripPayments handles GET /travel-requests/{jobNumber}/per-diem-payments.
func (h *FinanceHandler) TripPayments(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	out, err := h.svc.TripPayments(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, out)
}

type financeAmountBody struct {
	Amount float64 `json:"amount"`
}

// AddPayment handles POST /travel-requests/{jobNumber}/per-diem-payments.
func (h *FinanceHandler) AddPayment(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in financeAmountBody
	if !decodeJSON(w, r, &in) {
		return
	}
	pay, err := h.svc.AddPayment(r.Context(), p, r.PathValue("jobNumber"), in.Amount)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, pay)
}

// Invoices handles GET /invoices.
func (h *FinanceHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	q := r.URL.Query()
	page, err := h.svc.Invoices(r.Context(), p, domain.InvoiceQuery{
		Tab: strings.TrimSpace(q.Get("tab")), Search: q.Get("search"),
		PoApproval: queryList(r, "poApproval"), FinanceApproval: queryList(r, "financeApproval"),
		NetSuiteStatus: queryList(r, "netsuiteStatus"), CostTypes: queryList(r, "costType"), Agencies: queryList(r, "agency"),
		Months: queryList(r, "month"), CampaignIDs: queryList(r, "campaignId"),
		Sort: strings.TrimSpace(q.Get("sort")), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, page)
}

// Invoice handles GET /invoices/{invoiceId}.
func (h *FinanceHandler) Invoice(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := financePathInt(w, r, "invoiceId")
	if !ok {
		return
	}
	inv, err := h.svc.Invoice(r.Context(), p, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, inv)
}

type financeInvoiceIDs struct {
	InvoiceIDs []int `json:"invoiceIds"`
}

func (h *FinanceHandler) invoiceBatch(w http.ResponseWriter, r *http.Request,
	run func(p domain.Perms, ids []int) (domain.BatchResult, error)) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in financeInvoiceIDs
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := run(p, in.InvoiceIDs)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// ApproveInvoices handles POST /invoices/approve.
func (h *FinanceHandler) ApproveInvoices(w http.ResponseWriter, r *http.Request) {
	h.invoiceBatch(w, r, func(p domain.Perms, ids []int) (domain.BatchResult, error) {
		return h.svc.ApproveInvoices(r.Context(), p, ids)
	})
}

// RejectInvoices handles POST /invoices/reject.
func (h *FinanceHandler) RejectInvoices(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in struct {
		InvoiceIDs []int  `json:"invoiceIds"`
		Reason     string `json:"reason"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.RejectInvoices(r.Context(), p, in.InvoiceIDs, in.Reason)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// PushInvoices handles POST /invoices/push.
func (h *FinanceHandler) PushInvoices(w http.ResponseWriter, r *http.Request) {
	h.invoiceBatch(w, r, func(p domain.Perms, ids []int) (domain.BatchResult, error) {
		return h.svc.PushInvoices(r.Context(), p, ids)
	})
}

// ResyncInvoices handles POST /invoices/resync.
func (h *FinanceHandler) ResyncInvoices(w http.ResponseWriter, r *http.Request) {
	h.invoiceBatch(w, r, func(p domain.Perms, ids []int) (domain.BatchResult, error) {
		return h.svc.ResyncInvoices(r.Context(), p, ids)
	})
}

// Payments handles GET /per-diem-payments.
func (h *FinanceHandler) Payments(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	q := r.URL.Query()
	page, err := h.svc.Payments(r.Context(), p, domain.PaymentQuery{
		Tab: strings.TrimSpace(q.Get("tab")), Search: q.Get("search"), NetSuiteStatus: queryList(r, "netsuiteStatus"),
		Travellers: queryList(r, "traveller"), ConvertedCurrency: queryList(r, "convertedCurrency"),
		CampaignIDs: queryList(r, "campaignId"),
		Sort:        strings.TrimSpace(q.Get("sort")), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, page)
}

// Payment handles GET /per-diem-payments/{paymentId}.
func (h *FinanceHandler) Payment(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	pay, err := h.svc.Payment(r.Context(), p, r.PathValue("paymentId"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, pay)
}

// UpdatePayment handles PATCH /per-diem-payments/{paymentId}.
func (h *FinanceHandler) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in financeAmountBody
	if !decodeJSON(w, r, &in) {
		return
	}
	pay, err := h.svc.UpdatePayment(r.Context(), p, r.PathValue("paymentId"), in.Amount)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, pay)
}

func (h *FinanceHandler) paymentBatch(w http.ResponseWriter, r *http.Request,
	run func(p domain.Perms, ids []string) (domain.BatchResult, error)) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in struct {
		PaymentIDs []string `json:"paymentIds"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := run(p, in.PaymentIDs)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// PushPayments handles POST /per-diem-payments/push.
func (h *FinanceHandler) PushPayments(w http.ResponseWriter, r *http.Request) {
	h.paymentBatch(w, r, func(p domain.Perms, ids []string) (domain.BatchResult, error) {
		return h.svc.PushPayments(r.Context(), p, ids)
	})
}

// ResyncPayments handles POST /per-diem-payments/resync.
func (h *FinanceHandler) ResyncPayments(w http.ResponseWriter, r *http.Request) {
	h.paymentBatch(w, r, func(p domain.Perms, ids []string) (domain.BatchResult, error) {
		return h.svc.ResyncPayments(r.Context(), p, ids)
	})
}

// SyncFailures handles GET /netsuite-sync-failures.
func (h *FinanceHandler) SyncFailures(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	page, err := h.svc.SyncFailures(r.Context(), p, r.URL.Query().Get("search"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, page)
}

func financeReportRange(r *http.Request) (string, string, error) {
	from, err := queryDate(r, "from")
	if err != nil {
		return "", "", err
	}
	to, err := queryDate(r, "to")
	return from, to, err
}

func financeMoney(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func financeOptMoney(f *float64) string {
	if f == nil {
		return ""
	}
	return financeMoney(*f)
}

func financeVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func financeCampaignCols(c *domain.CampaignRef) (string, string) {
	if c == nil {
		return "", ""
	}
	return c.ID, financeVal(c.Name)
}

// InvoiceReport handles GET /reports/invoices (format=csv for the legacy
// columns plus Campaign ID and Campaign Name).
func (h *FinanceHandler) InvoiceReport(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	from, to, err := financeReportRange(r)
	asCSV := false
	if err == nil {
		asCSV, err = reportCSV(r)
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	page, err := h.svc.InvoiceReport(r.Context(), p, from, to)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if !asCSV {
		writeJSON(w, page)
		return
	}
	rows := make([][]string, 0, len(page.Items))
	for _, inv := range page.Items {
		cid, cname := financeCampaignCols(inv.Campaign)
		rows = append(rows, []string{
			// Legacy: "Traveller WSO2 Email" holds the agent who added the
			// invoice (travel_agent_employee.wso2_email).
			inv.InvoiceNumber, financeVal(inv.AgentEmail), inv.JobNumber, inv.Status,
			inv.Currency, financeMoney(inv.Amount), inv.PaidCurrency, financeMoney(inv.PaidAmount),
			inv.Month, inv.CostType, cid, cname, financeVal(inv.RejectReason), financeVal(inv.AddedOn),
		})
	}
	writeCSV(w, from+"_to_"+to+"_invoices.csv", []string{
		"Invoice Number", "Traveller WSO2 Email", "Job Number", "Invoice Status", "Invoice Currency", "Invoice Amount",
		"Paid Currency", "Paid Amount", "Month", "Cost Type", "Campaign ID", "Campaign Name", "Reject Reason", "Added Date",
	}, rows)
}

// PaymentReport handles GET /reports/per-diem-payments.
func (h *FinanceHandler) PaymentReport(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	from, to, err := financeReportRange(r)
	asCSV := false
	if err == nil {
		asCSV, err = reportCSV(r)
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	page, err := h.svc.PaymentReport(r.Context(), p, from, to)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if !asCSV {
		writeJSON(w, page)
		return
	}
	rows := make([][]string, 0, len(page.Items))
	for _, pay := range page.Items {
		cid, cname := financeCampaignCols(pay.Campaign)
		traveller := ""
		if pay.Traveller != nil {
			traveller = pay.Traveller.Email
		}
		ns := "Pending"
		if pay.NetSuite.Status == domain.NetSuitePushed || pay.NetSuite.Status == domain.NetSuitePushing {
			ns = "Pushed"
		}
		rows = append(rows, []string{
			pay.ID, traveller, pay.JobNumber, ns, pay.Currency, financeMoney(pay.Amount),
			financeVal(pay.ConvertedCurrency), financeOptMoney(pay.ConvertedAmount), cid, cname, financeVal(pay.AddedOn),
		})
	}
	writeCSV(w, from+"_to_"+to+"_perdiem.csv", []string{
		"Per Diem Number", "Traveller Email", "Job Number", "NetSuite Status", "Payment Currency", "Payment Amount",
		"Converted Currency", "Converted Amount", "Campaign ID", "Campaign Name", "Added Date",
	}, rows)
}
