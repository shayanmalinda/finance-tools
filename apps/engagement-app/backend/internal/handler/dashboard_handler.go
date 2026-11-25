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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// DashboardHandler serves GET /dashboard, GET /dashboard/allocations-today
// and GET /me/overview.
type DashboardHandler struct{ svc service.DashboardService }

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(svc service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// Get handles GET /dashboard.
func (h *DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	// sections: nil when absent (every section behind a default-on widget).
	var sections []string
	if _, asked := r.URL.Query()["sections"]; asked {
		sections = queryMulti(r, "sections")
		if sections == nil {
			sections = []string{}
		}
	}
	resp, err := h.svc.Dashboard(r.Context(), p, sections)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// AllocationsToday handles GET /dashboard/allocations-today?date=&businessUnitId=.
func (h *DashboardHandler) AllocationsToday(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	date, err := queryDate(r, "date")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	bu := 0
	if v := strings.TrimSpace(r.URL.Query().Get("businessUnitId")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "businessUnitId must be a positive whole number."})
			return
		}
		bu = n
	}
	resp, err := h.svc.AllocationsToday(r.Context(), p, date, bu)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// MeOverview handles GET /me/overview.
func (h *DashboardHandler) MeOverview(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.MeOverview(r.Context(), p)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
