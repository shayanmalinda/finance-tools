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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// dashboardStrings reads a repeated or comma-joined string query parameter.
func dashboardStrings(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// CostLines handles GET /dashboard/cost-lines?from=&to=&source=&jobNumber=&employee=&limit=.
func (h *DashboardHandler) CostLines(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.DashboardCostLinesFilter{
		From: strings.TrimSpace(q.Get("from")), To: strings.TrimSpace(q.Get("to")),
		Sources: dashboardStrings(q["source"]), JobNumbers: dashboardStrings(q["jobNumber"]),
		Employees: dashboardStrings(q["employee"]),
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > domain.DashboardCostLinesMax {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "limit must be between 1 and 20000"})
			return
		}
		f.Limit = n
	}
	resp, err := h.svc.CostLines(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// TravelTeam handles GET /dashboard/travel-team?today=.
func (h *DashboardHandler) TravelTeam(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.TravelTeam(r.Context(), r.URL.Query().Get("today"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Manager handles GET /dashboard/manager?today=&engagementType=.
func (h *DashboardHandler) Manager(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	types, err := dashboardInts(q, "engagementType")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Manager(r.Context(), domain.DashboardManagerFilter{Today: q.Get("today"), Types: types})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// AccountManager handles GET /dashboard/account-manager?today=&email=.
func (h *DashboardHandler) AccountManager(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := h.svc.AccountManager(r.Context(), q.Get("email"), q.Get("today"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Events handles GET /dashboard/events?today=.
func (h *DashboardHandler) Events(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Events(r.Context(), r.URL.Query().Get("today"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// MyVisas handles GET /dashboard/my-visas?today=&email=.
func (h *DashboardHandler) MyVisas(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := h.svc.MyVisas(r.Context(), q.Get("email"), q.Get("today"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
