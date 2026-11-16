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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// EngagementHandler serves the engagement endpoints.
type EngagementHandler struct{ svc service.EngagementService }

// NewEngagementHandler constructs an EngagementHandler.
func NewEngagementHandler(svc service.EngagementService) *EngagementHandler {
	return &EngagementHandler{svc: svc}
}

// engagementWriteCreated writes v as a 201 JSON response.
func engagementWriteCreated(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(v)
}

// Search handles POST /engagements/search.
func (h *EngagementHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req domain.EngagementSearchRequest
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

// Facets handles GET /engagements/facets?engagementTypeId=1&engagementTypeId=5.
func (h *EngagementHandler) Facets(w http.ResponseWriter, r *http.Request) {
	var ids []int
	for _, s := range r.URL.Query()["engagementTypeId"] {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "engagementTypeId must be a positive integer"})
			return
		}
		ids = append(ids, v)
	}
	resp, err := h.svc.Facets(r.Context(), ids)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Get handles GET /engagements/{id}.
func (h *EngagementHandler) Get(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Allocations handles GET /engagements/{id}/roster?activeOnly=true|false.
func (h *EngagementHandler) Allocations(w http.ResponseWriter, r *http.Request) {
	activeOnly := false
	if s := r.URL.Query().Get("activeOnly"); s != "" {
		v, err := strconv.ParseBool(s)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "activeOnly must be true or false"})
			return
		}
		activeOnly = v
	}
	resp, err := h.svc.Allocations(r.Context(), r.PathValue("id"), activeOnly)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Create handles POST /engagements.
func (h *EngagementHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.EngagementCreateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Create(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	engagementWriteCreated(w, resp)
}

// Update handles PATCH /engagements/{id}.
func (h *EngagementHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req domain.EngagementUpdateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.Update(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
