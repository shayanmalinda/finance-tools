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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// AllocationHandler serves the allocation screen area: Allocate, the
// Engagement Allocations list and status popup, timelines, Team Allocations
// and the Consultant Allocations report.
type AllocationHandler struct {
	svc    service.AllocationService
	team   service.TeamAllocationService
	report service.ConsultantReportService
}

// NewAllocationHandler constructs an AllocationHandler.
func NewAllocationHandler(svc service.AllocationService, team service.TeamAllocationService, report service.ConsultantReportService) *AllocationHandler {
	return &AllocationHandler{svc: svc, team: team, report: report}
}

// allocationError writes an overlap as its 409 body, anything else as usual.
func allocationError(w http.ResponseWriter, r *http.Request, err error) {
	var oe *domain.AllocationOverlapError
	if errors.As(err, &oe) {
		writeJSONStatus(w, http.StatusConflict, oe)
		return
	}
	writeServiceError(w, r, err)
}

func allocationPathID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	id := strings.TrimSpace(r.PathValue(key))
	if id == "" || len(id) > 64 {
		apierror.WriteJSON(w, http.StatusBadRequest, "Invalid id.")
		return "", false
	}
	return id, true
}

// Create handles POST /engagements/{id}/allocations.
func (h *AllocationHandler) Create(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := allocationPathID(w, r, "id")
	if !ok {
		return
	}
	var req domain.AllocationCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.svc.Create(r.Context(), p, id, req)
	if err != nil {
		allocationError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// timelineFilter parses email / org, from, to and allocationTypeId.
func timelineFilter(r *http.Request) (domain.AllocationTimelineFilter, error) {
	q := r.URL.Query()
	f := domain.AllocationTimelineFilter{
		Emails:       queryList(r, "email"),
		BusinessUnit: strings.TrimSpace(q.Get("businessUnit")),
		Department:   strings.TrimSpace(q.Get("department")),
		Team:         strings.TrimSpace(q.Get("team")),
	}
	if len(f.Emails) > 100 {
		return f, &apierror.ValidationError{Msg: "At most 100 emails."}
	}
	var err error
	if f.From, err = queryDate(r, "from"); err != nil {
		return f, err
	}
	if f.To, err = queryDate(r, "to"); err != nil {
		return f, err
	}
	if f.From == "" || f.To == "" {
		return f, &apierror.ValidationError{Msg: "from and to are required."}
	}
	f.AllocationTypeIDs, err = queryInts(r, "allocationTypeId")
	return f, err
}

// Timeline handles GET /allocation-timeline.
func (h *AllocationHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	f, err := timelineFilter(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Timeline(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// List handles GET /allocations.
func (h *AllocationHandler) List(w http.ResponseWriter, r *http.Request) { h.list(w, r, false) }

// ListMine handles GET /me/allocations.
func (h *AllocationHandler) ListMine(w http.ResponseWriter, r *http.Request) { h.list(w, r, true) }

func (h *AllocationHandler) list(w http.ResponseWriter, r *http.Request, mine bool) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	limit, offset, err := pagination(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	f := domain.AllocationListFilter{
		Search: strings.TrimSpace(r.URL.Query().Get("search")), Kinds: queryList(r, "kind"),
		ClearanceStatuses: queryList(r, "clearanceStatus"), Consultants: queryList(r, "consultant"),
		AllocationTypes: queryList(r, "allocationType"), Limit: limit, Offset: offset,
	}
	if len([]rune(f.Search)) > 200 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "search can have at most 200 characters."})
		return
	}
	if f.EngagementTypeIDs, err = queryInts(r, "engagementTypeId"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if v := r.URL.Query().Get("travelRequired"); v != "" {
		b, perr := strconv.ParseBool(v)
		if perr != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "travelRequired must be true or false."})
			return
		}
		f.TravelRequired = &b
	}
	if f.StartFrom, err = queryDate(r, "startFrom"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	if f.EndTo, err = queryDate(r, "endTo"); err != nil {
		writeServiceError(w, r, err)
		return
	}
	list := h.svc.List
	if mine {
		list = h.svc.ListMine
	}
	resp, err := list(r.Context(), p, f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateStatus handles PATCH /allocations/{id}/status.
func (h *AllocationHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := allocationPathID(w, r, "id")
	if !ok {
		return
	}
	var req domain.AllocationStatusUpdate
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.svc.UpdateStatus(r.Context(), p, id, req)
	if err != nil {
		allocationError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// SearchTeam handles GET /team-allocations.
func (h *AllocationHandler) SearchTeam(w http.ResponseWriter, r *http.Request) {
	f, err := timelineFilter(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.team.Search(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// CreateTeam handles POST /team-allocations.
func (h *AllocationHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var req domain.TeamAllocationCreate
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.team.Create(r.Context(), p, req)
	if err != nil {
		allocationError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// UpdateTeam handles PATCH /team-allocations/{id}.
func (h *AllocationHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := allocationPathID(w, r, "id")
	if !ok {
		return
	}
	var req domain.TeamAllocationUpdate
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.team.Update(r.Context(), p, id, req)
	if err != nil {
		allocationError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// DeleteTeam handles DELETE /team-allocations/{id}.
func (h *AllocationHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := allocationPathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.team.Delete(r.Context(), p, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ValidateBulk handles POST /team-allocations/bulk/validate.
func (h *AllocationHandler) ValidateBulk(w http.ResponseWriter, r *http.Request) {
	var req domain.TeamAllocationBulkRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.team.ValidateBulk(r.Context(), req.CSV)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UploadBulk handles POST /team-allocations/bulk.
func (h *AllocationHandler) UploadBulk(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var req domain.TeamAllocationBulkRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resp, err := h.team.UploadBulk(r.Context(), p, req.CSV)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Report handles GET /reports/consultant-allocations (format=csv for the
// 18-column legacy CSV).
func (h *AllocationHandler) Report(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.ConsultantReportFilter{
		Consultant:   strings.ToLower(strings.TrimSpace(q.Get("consultant"))),
		BusinessUnit: strings.TrimSpace(q.Get("businessUnit")),
		Department:   strings.TrimSpace(q.Get("department")),
		Team:         strings.TrimSpace(q.Get("team")),
	}
	var err error
	if f.From, err = queryDate(r, "from"); err == nil {
		f.To, err = queryDate(r, "to")
	}
	if err == nil && (f.From == "" || f.To == "") {
		err = &apierror.ValidationError{Msg: "from and to are required."}
	}
	if err == nil {
		f.AllocationTypeIDs, err = queryInts(r, "allocationTypeId")
	}
	asCSV := false
	if err == nil {
		asCSV, err = reportCSV(r)
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	rows, err := h.report.Report(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if asCSV {
		writeCSV(w, "consultant-allocations-"+f.From+"-to-"+f.To+".csv", service.ConsultantReportCSVHeader,
			service.ConsultantReportCSVRows(rows))
		return
	}
	writeJSON(w, domain.ConsultantReport{Items: rows})
}
