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

// EngagementHandler serves the Engagements, engagement page and Approvals
// endpoints.
type EngagementHandler struct{ svc service.EngagementService }

// NewEngagementHandler constructs an EngagementHandler.
func NewEngagementHandler(svc service.EngagementService) *EngagementHandler {
	return &EngagementHandler{svc: svc}
}

// List handles GET /engagements.
func (h *EngagementHandler) List(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	f := service.EngagementListFilter{
		Segment: r.URL.Query().Get("segment"), Search: strings.TrimSpace(r.URL.Query().Get("search")),
		Accounts: queryList(r, "account"), AllocationTypes: queryList(r, "allocationType"),
		RequestStatuses: queryList(r, "requestStatus"), EngagementStatuses: queryList(r, "engagementStatus"),
		BusinessUnits: queryList(r, "businessUnit"), Countries: queryList(r, "country"),
		Consultants: r.URL.Query().Get("consultants"), Sort: strings.TrimSpace(r.URL.Query().Get("sort")),
		// Optional and additive: only the exact value "true" turns it on.
		IncludePending: r.URL.Query().Get("includePending") == "true",
	}
	var err error
	if len(f.Search) > 200 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "search can have at most 200 characters."})
		return
	}
	if f.EngagementTypeIDs, err = queryInts(r, "engagementTypeId"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	for key, dst := range map[string]*string{"startFrom": &f.StartFrom, "startTo": &f.StartTo, "endTo": &f.EndTo} {
		if *dst, err = queryDate(r, key); err != nil {
			writeServiceError(w, r, err)
			return
		}
	}
	if f.Limit, f.Offset, err = pagination(r); err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.List(r.Context(), p, f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Facets handles GET /engagements/facets.
func (h *EngagementHandler) Facets(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Facets(r.Context(), p, r.URL.Query().Get("segment"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Approvals handles GET /approvals.
func (h *EngagementHandler) Approvals(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Approvals(r.Context(), p, r.URL.Query().Get("segment"), strings.TrimSpace(r.URL.Query().Get("search")),
		strings.TrimSpace(r.URL.Query().Get("sort")), limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Get handles GET /engagements/{id}.
func (h *EngagementHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Get(r.Context(), p, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Create handles POST /engagements.
func (h *EngagementHandler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.EngagementCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.Create(r.Context(), p, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// Update handles PATCH /engagements/{id}.
func (h *EngagementHandler) Update(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.EngagementUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.Update(r.Context(), p, r.PathValue("id"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// SetRequestStatus handles POST /engagements/{id}/request-status.
func (h *EngagementHandler) SetRequestStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.EngagementRequestStatusChange
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.SetRequestStatus(r.Context(), p, r.PathValue("id"), in.Status)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// SetEngagementStatus handles POST /engagements/{id}/engagement-status.
func (h *EngagementHandler) SetEngagementStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.EngagementStatusChange
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Comment != nil && len(*in.Comment) > 1000 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "comment can have at most 1000 characters."})
		return
	}
	resp, err := h.svc.SetEngagementStatus(r.Context(), p, r.PathValue("id"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Approve handles POST /engagements/{id}/approve.
func (h *EngagementHandler) Approve(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Approve(r.Context(), p, r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// engagementPathInt reads a positive integer path value, writing the 400.
func engagementPathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	v, err := strconv.Atoi(r.PathValue(name))
	if err != nil || v <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive whole number."})
		return 0, false
	}
	return v, true
}

// engagementQueryInt reads an optional non-negative integer query value.
func engagementQueryInt(r *http.Request, name string) (int, error) {
	s := strings.TrimSpace(r.URL.Query().Get(name))
	if s == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0, &apierror.ValidationError{Msg: name + " must be a whole number."}
	}
	return v, nil
}
