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
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// BudgetHandler serves the /budgets endpoints.
type BudgetHandler struct{ svc service.BudgetService }

// NewBudgetHandler constructs a BudgetHandler.
func NewBudgetHandler(svc service.BudgetService) *BudgetHandler { return &BudgetHandler{svc: svc} }

// fail maps an error to its response. The missing-tables refusal keeps its
// message (run the DDL), unlike other 503s.
func (h *BudgetHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, repository.ErrBudgetNoTables) {
		apierror.WriteJSON(w, http.StatusServiceUnavailable, repository.ErrBudgetNoTables.Msg)
		return
	}
	writeServiceError(w, r, err)
}

func (h *BudgetHandler) reply(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, v)
}

func budgetPathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	v, err := strconv.Atoi(r.PathValue(name))
	if err != nil || v <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive integer"})
		return 0, false
	}
	return v, true
}

func budgetPathText(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := strings.TrimSpace(r.PathValue(name))
	if v == "" || len(v) > 255 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "invalid " + name})
		return "", false
	}
	return v, true
}

func budgetQueryInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return 0, true
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be an integer"})
		return 0, false
	}
	return v, true
}

func budgetQueryBool(w http.ResponseWriter, r *http.Request, name string) (bool, bool) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return false, true
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be true or false"})
		return false, false
	}
	return v, true
}

// Years handles GET /budgets/years.
func (h *BudgetHandler) Years(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Years(r.Context())
	h.reply(w, r, resp, err)
}

// Lines handles GET /budgets/lines?year=&includeRetired=.
func (h *BudgetHandler) Lines(w http.ResponseWriter, r *http.Request) {
	year, ok := budgetQueryInt(w, r, "year")
	if !ok {
		return
	}
	retired, ok := budgetQueryBool(w, r, "includeRetired")
	if !ok {
		return
	}
	resp, err := h.svc.Lines(r.Context(), year, retired)
	h.reply(w, r, resp, err)
}

// CreateLine handles POST /budgets/lines.
func (h *BudgetHandler) CreateLine(w http.ResponseWriter, r *http.Request) {
	var req domain.BudgetLineCreateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.CreateLine(r.Context(), req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	engagementWriteCreated(w, resp)
}

// UpdateLine handles PUT /budgets/lines/{id}.
func (h *BudgetHandler) UpdateLine(w http.ResponseWriter, r *http.Request) {
	id, ok := budgetPathInt(w, r, "id")
	if !ok {
		return
	}
	var req domain.BudgetLineUpdateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.UpdateLine(r.Context(), id, req)
	h.reply(w, r, resp, err)
}

// RetireLine handles DELETE /budgets/lines/{id}?updatedBy=.
func (h *BudgetHandler) RetireLine(w http.ResponseWriter, r *http.Request) {
	id, ok := budgetPathInt(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.RetireLine(r.Context(), id, r.URL.Query().Get("updatedBy")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AllocationTypeRules handles GET /budgets/allocation-type-rules.
func (h *BudgetHandler) AllocationTypeRules(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.AllocationTypeRules(r.Context())
	h.reply(w, r, resp, err)
}

// SaveAllocationTypeRule handles PUT /budgets/allocation-type-rules/{allocationTypeId}.
func (h *BudgetHandler) SaveAllocationTypeRule(w http.ResponseWriter, r *http.Request) {
	id, ok := budgetPathInt(w, r, "allocationTypeId")
	if !ok {
		return
	}
	var req domain.BudgetAllocationTypeRuleRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SaveAllocationTypeRule(r.Context(), id, req)
	h.reply(w, r, resp, err)
}

// RegionRules handles GET /budgets/region-rules.
func (h *BudgetHandler) RegionRules(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.RegionRules(r.Context())
	h.reply(w, r, resp, err)
}

// TeamRules handles GET /budgets/team-rules.
func (h *BudgetHandler) TeamRules(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.TeamRules(r.Context())
	h.reply(w, r, resp, err)
}

// EngagementLine handles GET /budgets/engagements/{engagementId}/line.
func (h *BudgetHandler) EngagementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := budgetPathText(w, r, "engagementId")
	if !ok {
		return
	}
	resp, err := h.svc.EngagementLine(r.Context(), id)
	h.reply(w, r, resp, err)
}

// SetEngagementLine handles PUT /budgets/engagements/{engagementId}/line.
func (h *BudgetHandler) SetEngagementLine(w http.ResponseWriter, r *http.Request) {
	id, ok := budgetPathText(w, r, "engagementId")
	if !ok {
		return
	}
	var req domain.BudgetEngagementLineRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SetEngagementLine(r.Context(), id, req)
	h.reply(w, r, resp, err)
}

// Trip handles GET /budgets/trips/{jobNumber}.
func (h *BudgetHandler) Trip(w http.ResponseWriter, r *http.Request) {
	job, ok := budgetPathText(w, r, "jobNumber")
	if !ok {
		return
	}
	resp, err := h.svc.Trip(r.Context(), job)
	h.reply(w, r, resp, err)
}

// SetTripLine handles PUT /budgets/trips/{jobNumber}.
func (h *BudgetHandler) SetTripLine(w http.ResponseWriter, r *http.Request) {
	job, ok := budgetPathText(w, r, "jobNumber")
	if !ok {
		return
	}
	var req domain.BudgetTripRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SetTripLine(r.Context(), job, req)
	h.reply(w, r, resp, err)
}

// NeedsLine handles GET /budgets/needs-line?year=&limit=&offset=.
func (h *BudgetHandler) NeedsLine(w http.ResponseWriter, r *http.Request) {
	year, ok := budgetQueryInt(w, r, "year")
	if !ok {
		return
	}
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.NeedsLine(r.Context(), year, limit, offset)
	h.reply(w, r, resp, err)
}

// Ledger handles GET /budgets/ledger?from=&to=&unassigned=.
func (h *BudgetHandler) Ledger(w http.ResponseWriter, r *http.Request) {
	un, ok := budgetQueryBool(w, r, "unassigned")
	if !ok {
		return
	}
	q := r.URL.Query()
	resp, err := h.svc.Ledger(r.Context(), domain.BudgetLedgerFilter{From: q.Get("from"), To: q.Get("to"), Unassigned: un})
	h.reply(w, r, resp, err)
}
