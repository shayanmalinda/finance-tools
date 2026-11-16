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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// AllocationHandler serves the allocation screen area's entity endpoints.
type AllocationHandler struct{ svc service.AllocationService }

// NewAllocationHandler constructs an AllocationHandler.
func NewAllocationHandler(svc service.AllocationService) *AllocationHandler {
	return &AllocationHandler{svc: svc}
}

func allocationWriteStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// allocationPathID returns the {id} path value, or writes a 400.
func allocationPathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || len(id) > 64 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "invalid id"})
		return "", false
	}
	return id, true
}

// Engagement handles GET /allocations/engagements/{id}.
func (h *AllocationHandler) Engagement(w http.ResponseWriter, r *http.Request) {
	id, ok := allocationPathID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Engagement(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Get handles GET /allocations/{id}.
func (h *AllocationHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := allocationPathID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Timeline handles POST /allocations/timeline.
func (h *AllocationHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	var req domain.AllocationTimelineRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Timeline(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Search handles POST /allocations/search.
func (h *AllocationHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req domain.AllocationSearchRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Search(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Bulk handles POST /allocations/bulk.
func (h *AllocationHandler) Bulk(w http.ResponseWriter, r *http.Request) {
	var req domain.AllocationBulkRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Bulk(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	allocationWriteStatus(w, http.StatusCreated, resp)
}

// Update handles PATCH /allocations/{id}.
func (h *AllocationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := allocationPathID(w, r)
	if !ok {
		return
	}
	var req domain.AllocationUpdateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Delete handles DELETE /allocations/{id} (status Deleted).
func (h *AllocationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := allocationPathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Visas handles GET /allocations/visas?email=&startDate=&endDate=.
func (h *AllocationHandler) Visas(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := h.svc.Visas(r.Context(), q.Get("email"), q.Get("startDate"), q.Get("endDate"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// consultantFilter parses the report filter query parameters.
func consultantFilter(w http.ResponseWriter, r *http.Request) (domain.ConsultantAllocationFilter, bool) {
	q := r.URL.Query()
	f := domain.ConsultantAllocationFilter{
		StartDate:    q.Get("startDate"),
		EndDate:      q.Get("endDate"),
		Email:        strings.TrimSpace(q.Get("email")),
		BusinessUnit: strings.TrimSpace(q.Get("businessUnit")),
		Department:   strings.TrimSpace(q.Get("department")),
		Team:         strings.TrimSpace(q.Get("team")),
	}
	for _, v := range q["allocationTypeId"] {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "allocationTypeId must be a positive integer"})
			return f, false
		}
		f.AllocationTypeIDs = append(f.AllocationTypeIDs, n)
	}
	return f, true
}

// ConsultantAllocations handles GET /consultant-allocations.
func (h *AllocationHandler) ConsultantAllocations(w http.ResponseWriter, r *http.Request) {
	f, ok := consultantFilter(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.ConsultantAllocations(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// NonAllocatedEmployees handles GET /consultant-allocations/non-allocated.
func (h *AllocationHandler) NonAllocatedEmployees(w http.ResponseWriter, r *http.Request) {
	f, ok := consultantFilter(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.NonAllocatedEmployees(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// TeamDefaults handles GET /consultant-allocations/team-defaults.
func (h *AllocationHandler) TeamDefaults(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.TeamDefaults(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// EngagementCodes handles GET /consultant-allocations/engagement-codes.
func (h *AllocationHandler) EngagementCodes(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.EngagementCodes(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
