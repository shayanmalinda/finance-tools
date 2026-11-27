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
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// FinanceHandler serves the travel finance routes (/finance/...).
type FinanceHandler struct{ svc service.FinanceService }

// NewFinanceHandler constructs a FinanceHandler.
func NewFinanceHandler(svc service.FinanceService) *FinanceHandler { return &FinanceHandler{svc: svc} }

func financeWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func financeList(r *http.Request, key string) []string {
	var out []string
	for _, v := range r.URL.Query()[key] {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// financeRawList keeps commas (agency and campaign names may hold them).
func financeRawList(r *http.Request, key string) []string {
	var out []string
	for _, v := range r.URL.Query()[key] {
		if p := strings.TrimSpace(v); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func financeDateParam(r *http.Request, key string) (string, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return "", &apierror.ValidationError{Msg: key + " must be a date (YYYY-MM-DD)"}
	}
	return v, nil
}

func financePathID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil || id <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive integer"})
		return 0, false
	}
	return id, true
}

// Trip handles GET /finance/travel-requests/{jobNumber}.
func (h *FinanceHandler) Trip(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Trip(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, t)
}

// PerDiems handles GET /finance/travel-requests/{jobNumber}/per-diems.
func (h *FinanceHandler) PerDiems(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.PerDiems(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"perDiems": items})
}

// PerDiem handles GET /finance/travel-requests/{jobNumber}/per-diems/{id}.
func (h *FinanceHandler) PerDiem(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	p, err := h.svc.PerDiem(r.Context(), r.PathValue("jobNumber"), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, p)
}

// AddPerDiem handles POST /finance/travel-requests/{jobNumber}/per-diems.
func (h *FinanceHandler) AddPerDiem(w http.ResponseWriter, r *http.Request) {
	var in domain.FinancePerDiemCreate
	if !decodeRequest(w, r, &in) {
		return
	}
	p, err := h.svc.AddPerDiem(r.Context(), r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	financeWrite(w, http.StatusCreated, p)
}

// DeletePerDiem handles DELETE /finance/travel-requests/{jobNumber}/per-diems/{id}.
func (h *FinanceHandler) DeletePerDiem(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.DeletePerDiem(r.Context(), r.PathValue("jobNumber"), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PaymentBasis handles GET /finance/travel-requests/{jobNumber}/payment-basis.
func (h *FinanceHandler) PaymentBasis(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.PaymentBasis(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, b)
}

// TravelCards handles GET /finance/travel-requests/{jobNumber}/travel-cards.
func (h *FinanceHandler) TravelCards(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.TravelCards(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"travelCards": items})
}

// TripExpenses handles GET /finance/travel-requests/{jobNumber}/expenses.
func (h *FinanceHandler) TripExpenses(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.TripExpenses(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, out)
}

// TripCounts handles GET /finance/travel-requests/{jobNumber}/counts?agencyId=&include=.
func (h *FinanceHandler) TripCounts(w http.ResponseWriter, r *http.Request) {
	agencyID := 0
	if v := r.URL.Query().Get("agencyId"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "agencyId must be a positive integer"})
			return
		}
		agencyID = n
	}
	out, err := h.svc.TripCounts(r.Context(), r.PathValue("jobNumber"), agencyID, financeList(r, "include"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, out)
}

// AddTravelCard handles POST /finance/travel-requests/{jobNumber}/travel-cards.
func (h *FinanceHandler) AddTravelCard(w http.ResponseWriter, r *http.Request) {
	var in domain.FinanceTravelCardCreate
	if !decodeRequest(w, r, &in) {
		return
	}
	c, err := h.svc.AddTravelCard(r.Context(), r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	financeWrite(w, http.StatusCreated, c)
}

// CostTypes handles GET /finance/invoice-cost-types.
func (h *FinanceHandler) CostTypes(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.CostTypes(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"costTypes": items})
}

// AddInvoices handles POST /finance/travel-requests/{jobNumber}/invoices.
func (h *FinanceHandler) AddInvoices(w http.ResponseWriter, r *http.Request) {
	var in domain.FinanceInvoicesCreate
	if !decodeRequest(w, r, &in) {
		return
	}
	items, err := h.svc.AddInvoices(r.Context(), r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	financeWrite(w, http.StatusCreated, map[string]any{"invoices": items})
}

// Invoices handles GET /finance/invoices.
func (h *FinanceHandler) Invoices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	f := domain.FinanceInvoiceFilter{
		JobNumber: strings.TrimSpace(q.Get("jobNumber")), Statuses: financeRawList(r, "status"),
		Failed: q.Get("failed"), Search: q.Get("search"), CostTypes: financeRawList(r, "costType"),
		Agencies: financeRawList(r, "agency"), Months: financeList(r, "month"), CampaignIDs: financeList(r, "campaignId"),
		Sort: q.Get("sort"), Limit: limit, Offset: offset,
	}
	var err error
	if f.AddedFrom, err = financeDateParam(r, "addedFrom"); err == nil {
		f.AddedTo, err = financeDateParam(r, "addedTo")
	}
	if err == nil && q.Get("agencyId") != "" {
		if f.AgencyID, err = strconv.Atoi(q.Get("agencyId")); err != nil || f.AgencyID <= 0 {
			err = &apierror.ValidationError{Msg: "agencyId must be a positive integer"}
		}
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, total, err := h.svc.Invoices(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"invoices": items, "total": total})
}

// InvoiceCounts handles GET /finance/invoices/counts?agencyId=.
func (h *FinanceHandler) InvoiceCounts(w http.ResponseWriter, r *http.Request) {
	agency := 0
	if v := r.URL.Query().Get("agencyId"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "agencyId must be a positive integer"})
			return
		}
		agency = n
	}
	c, err := h.svc.InvoiceCounts(r.Context(), agency)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, c)
}

// Invoice handles GET /finance/invoices/{id}.
func (h *FinanceHandler) Invoice(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	inv, err := h.svc.Invoice(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, inv)
}

// DecideInvoices handles POST /finance/invoices/decisions.
func (h *FinanceHandler) DecideInvoices(w http.ResponseWriter, r *http.Request) {
	var in domain.FinanceInvoiceDecision
	if !decodeRequest(w, r, &in) {
		return
	}
	res, err := h.svc.DecideInvoices(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"results": res})
}

// PushInvoice handles POST /finance/invoices/{id}/push.
func (h *FinanceHandler) PushInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	var in domain.FinancePushRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	res, err := h.svc.PushInvoice(r.Context(), id, in.UpdatedBy)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// ResyncInvoice handles POST /finance/invoices/{id}/resync.
func (h *FinanceHandler) ResyncInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	var in domain.FinancePushRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	res, err := h.svc.ResyncInvoice(r.Context(), id, in.UpdatedBy)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// InvoiceReference handles GET /finance/invoices/{id}/netsuite-reference.
func (h *FinanceHandler) InvoiceReference(w http.ResponseWriter, r *http.Request) {
	id, ok := financePathID(w, r, "id")
	if !ok {
		return
	}
	ref, err := h.svc.InvoiceReference(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, ref)
}

// AddPayment handles POST /finance/travel-requests/{jobNumber}/per-diem-payments.
func (h *FinanceHandler) AddPayment(w http.ResponseWriter, r *http.Request) {
	var in domain.FinancePaymentCreate
	if !decodeRequest(w, r, &in) {
		return
	}
	p, err := h.svc.AddPayment(r.Context(), r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	financeWrite(w, http.StatusCreated, p)
}

// Payments handles GET /finance/per-diem-payments.
func (h *FinanceHandler) Payments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	f := domain.FinancePaymentFilter{
		JobNumber: strings.TrimSpace(q.Get("jobNumber")), NSStatuses: financeList(r, "nsStatus"),
		Search: q.Get("search"), Travellers: financeList(r, "traveller"),
		ConvertedCurrency: financeList(r, "convertedCurrency"), CampaignIDs: financeList(r, "campaignId"),
		Sort: q.Get("sort"), Limit: limit, Offset: offset,
	}
	var err error
	if f.AddedFrom, err = financeDateParam(r, "addedFrom"); err == nil {
		f.AddedTo, err = financeDateParam(r, "addedTo")
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, total, err := h.svc.Payments(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, map[string]any{"payments": items, "total": total})
}

// PaymentCounts handles GET /finance/per-diem-payments/counts.
func (h *FinanceHandler) PaymentCounts(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.PaymentCounts(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, c)
}

// Payment handles GET /finance/per-diem-payments/{perDiemNumber}.
func (h *FinanceHandler) Payment(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Payment(r.Context(), r.PathValue("perDiemNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, p)
}

// UpdatePayment handles PATCH /finance/per-diem-payments/{perDiemNumber}.
func (h *FinanceHandler) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	var in domain.FinancePaymentUpdate
	if !decodeRequest(w, r, &in) {
		return
	}
	p, err := h.svc.UpdatePayment(r.Context(), r.PathValue("perDiemNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, p)
}

// PushPayment handles POST /finance/per-diem-payments/{perDiemNumber}/push.
func (h *FinanceHandler) PushPayment(w http.ResponseWriter, r *http.Request) {
	var in domain.FinancePushRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	res, err := h.svc.PushPayment(r.Context(), r.PathValue("perDiemNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// ResyncPayment handles POST /finance/per-diem-payments/{perDiemNumber}/resync.
func (h *FinanceHandler) ResyncPayment(w http.ResponseWriter, r *http.Request) {
	var in domain.FinancePushRequest
	if !decodeRequest(w, r, &in) {
		return
	}
	res, err := h.svc.ResyncPayment(r.Context(), r.PathValue("perDiemNumber"), in.UpdatedBy)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, res)
}

// PaymentReference handles GET /finance/per-diem-payments/{perDiemNumber}/netsuite-reference.
func (h *FinanceHandler) PaymentReference(w http.ResponseWriter, r *http.Request) {
	ref, err := h.svc.PaymentReference(r.Context(), r.PathValue("perDiemNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, ref)
}

// Subsidiary handles GET /finance/subsidiaries/{company}.
func (h *FinanceHandler) Subsidiary(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Subsidiary(r.Context(), r.PathValue("company"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, s)
}
