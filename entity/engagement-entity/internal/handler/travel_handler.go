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

// TravelHandler serves travel requests, visas and travel documents.
type TravelHandler struct {
	requests  service.TravelRequestService
	documents service.TravelDocumentService
}

// NewTravelHandler constructs a TravelHandler.
func NewTravelHandler(requests service.TravelRequestService, documents service.TravelDocumentService) *TravelHandler {
	return &TravelHandler{requests: requests, documents: documents}
}

func travelWriteCreated(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(v)
}

// travelID parses a positive integer path value.
func travelID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(name))
	if err != nil || id <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive integer"})
		return 0, false
	}
	return id, true
}

// travelQueryList returns the non-empty values of a repeated query parameter.
func travelQueryList(r *http.Request, key string) []string {
	var out []string
	for _, v := range r.URL.Query()[key] {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SearchTravelRequests handles POST /travel-requests/search.
func (h *TravelHandler) SearchTravelRequests(w http.ResponseWriter, r *http.Request) {
	var f domain.TravelRequestSearch
	if !decodeRequest(w, r, &f) {
		return
	}
	resp, err := h.requests.Search(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// GetTravelRequest handles GET /travel-requests/{jobNumber}.
func (h *TravelHandler) GetTravelRequest(w http.ResponseWriter, r *http.Request) {
	resp, err := h.requests.Get(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateTravelRequest handles PATCH /travel-requests/{jobNumber}.
func (h *TravelHandler) UpdateTravelRequest(w http.ResponseWriter, r *http.Request) {
	var in domain.UpdateTravelRequestInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.requests.Update(r.Context(), r.PathValue("jobNumber"), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ActivePerDiems handles GET /travel-requests/{jobNumber}/active-per-diems.
func (h *TravelHandler) ActivePerDiems(w http.ResponseWriter, r *http.Request) {
	resp, err := h.requests.ActivePerDiems(r.Context(), r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListTravelAgencies handles GET /travel-agencies?status=Active.
func (h *TravelHandler) ListTravelAgencies(w http.ResponseWriter, r *http.Request) {
	resp, err := h.requests.Agents(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// InsertEmailAudit handles POST /travel-email-audit-logs.
func (h *TravelHandler) InsertEmailAudit(w http.ResponseWriter, r *http.Request) {
	var in domain.TravelEmailAuditInput
	if !decodeRequest(w, r, &in) {
		return
	}
	if err := h.requests.InsertEmailAudit(r.Context(), in); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListVisas handles GET /visas.
func (h *TravelHandler) ListVisas(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var exclude []int
	for _, v := range travelQueryList(r, "excludeId") {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "excludeId must be a positive integer"})
			return
		}
		exclude = append(exclude, id)
	}
	resp, err := h.documents.SearchVisas(r.Context(), domain.TravelVisaFilter{
		Emails: travelQueryList(r, "email"), Countries: travelQueryList(r, "country"),
		Status: q.Get("status"), Validity: q.Get("validity"), Today: q.Get("today"),
		CoversFrom: q.Get("coversFrom"), CoversTo: q.Get("coversTo"), Search: q.Get("search"),
		ExcludeIDs: exclude, Limit: limit, Offset: offset,
		IncludeMarkedExpired: q.Get("includeMarkedExpired") == "true",
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// GetVisa handles GET /visas/{id}.
func (h *TravelHandler) GetVisa(w http.ResponseWriter, r *http.Request) {
	id, ok := travelID(w, r, "id")
	if !ok {
		return
	}
	resp, err := h.documents.GetVisa(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// CreateVisa handles POST /visas.
func (h *TravelHandler) CreateVisa(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateTravelVisaInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.documents.CreateVisa(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	travelWriteCreated(w, resp)
}

// UpdateVisa handles PUT /visas/{id}.
func (h *TravelHandler) UpdateVisa(w http.ResponseWriter, r *http.Request) {
	id, ok := travelID(w, r, "id")
	if !ok {
		return
	}
	var in domain.UpdateTravelVisaInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.documents.UpdateVisa(r.Context(), id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListTravelDocuments handles GET /travel-documents.
func (h *TravelHandler) ListTravelDocuments(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	resp, err := h.documents.SearchDocuments(r.Context(), domain.TravelDocumentFilter{
		Emails: travelQueryList(r, "email"), Countries: travelQueryList(r, "country"),
		DocumentTypes: travelQueryList(r, "documentType"), Status: q.Get("status"),
		Validity: q.Get("validity"), Today: q.Get("today"), Search: q.Get("search"),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// GetTravelDocument handles GET /travel-documents/{id}.
func (h *TravelHandler) GetTravelDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := travelID(w, r, "id")
	if !ok {
		return
	}
	resp, err := h.documents.GetDocument(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// CreateTravelDocument handles POST /travel-documents.
func (h *TravelHandler) CreateTravelDocument(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateTravelDocumentInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.documents.CreateDocument(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	travelWriteCreated(w, resp)
}

// UpdateTravelDocument handles PUT /travel-documents/{id}.
func (h *TravelHandler) UpdateTravelDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := travelID(w, r, "id")
	if !ok {
		return
	}
	var in domain.UpdateTravelDocumentInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.documents.UpdateDocument(r.Context(), id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
