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

// BudgetHandler serves the Travel Budgets screens and settings.
type BudgetHandler struct{ svc service.BudgetScreen }

// NewBudgetHandler constructs a BudgetHandler.
func NewBudgetHandler(svc service.BudgetScreen) *BudgetHandler { return &BudgetHandler{svc: svc} }

// Preview handles GET /budgets/preview.
func (h *BudgetHandler) Preview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typeID, err := engagementQueryInt(r, "allocationTypeId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Preview(r.Context(), service.BudgetPreviewQuery{
		EngagementID: strings.TrimSpace(q.Get("engagementId")), TravellerEmail: strings.TrimSpace(q.Get("travellerEmail")),
		StartDate: strings.TrimSpace(q.Get("startDate")), EndDate: strings.TrimSpace(q.Get("endDate")),
		Country: strings.TrimSpace(q.Get("country")), AllocationTypeID: typeID})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Summary handles GET /budgets/summary?year=.
func (h *BudgetHandler) Summary(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	year, err := engagementQueryInt(r, "year")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Summary(r.Context(), p, year)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Lines handles GET /budgets/lines?year=&includeRetired=.
func (h *BudgetHandler) Lines(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	year, err := engagementQueryInt(r, "year")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	retired := false
	if v := strings.TrimSpace(r.URL.Query().Get("includeRetired")); v != "" {
		if retired, err = strconv.ParseBool(v); err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "includeRetired must be true or false."})
			return
		}
	}
	items, err := h.svc.Lines(r.Context(), p, year, retired)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, domain.BudgetLines{Lines: items})
}

// CreateLine handles POST /budgets/lines.
func (h *BudgetHandler) CreateLine(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.BudgetLineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.CreateLine(r.Context(), p, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// UpdateLine handles PUT /budgets/lines/{id}.
func (h *BudgetHandler) UpdateLine(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := engagementPathInt(w, r, "id")
	if !ok {
		return
	}
	var in domain.BudgetLineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.UpdateLine(r.Context(), p, id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// RetireLine handles DELETE /budgets/lines/{id}.
func (h *BudgetHandler) RetireLine(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := engagementPathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.RetireLine(r.Context(), p, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AllocationTypeRules handles GET /budgets/allocation-type-rules.
func (h *BudgetHandler) AllocationTypeRules(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.AllocationTypeRules(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, domain.BudgetAllocationTypeRules{Rules: items})
}

// SaveAllocationTypeRule handles PUT /budgets/allocation-type-rules/{allocationTypeId}.
func (h *BudgetHandler) SaveAllocationTypeRule(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := engagementPathInt(w, r, "allocationTypeId")
	if !ok {
		return
	}
	var in domain.BudgetRuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.SaveAllocationTypeRule(r.Context(), p, id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// NeedsLine handles GET /budgets/needs-line?year=&limit=&offset=.
func (h *BudgetHandler) NeedsLine(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	year, err := engagementQueryInt(r, "year")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.NeedsLine(r.Context(), p, year, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// SetTripLine handles PUT /travel-requests/{jobNumber}/budget-line.
func (h *BudgetHandler) SetTripLine(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.TripBudgetSet
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.SetTripLine(r.Context(), p, r.PathValue("jobNumber"), in.BudgetLineID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
