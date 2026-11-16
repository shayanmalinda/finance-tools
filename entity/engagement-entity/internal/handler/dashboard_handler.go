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
	"net/url"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// DashboardHandler serves the dashboard aggregates.
type DashboardHandler struct{ svc service.DashboardService }

// NewDashboardHandler constructs a DashboardHandler.
func NewDashboardHandler(svc service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// dashboardBool parses an optional boolean query parameter (default false).
func dashboardBool(q url.Values, key string) (bool, error) {
	v := strings.TrimSpace(q.Get(key))
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, &apierror.ValidationError{Msg: key + " must be true or false"}
	}
	return b, nil
}

// dashboardInt parses an optional positive integer query parameter (default 0).
func dashboardInt(q url.Values, key string) (int, error) {
	v := strings.TrimSpace(q.Get(key))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, &apierror.ValidationError{Msg: key + " must be a positive integer"}
	}
	return n, nil
}

// dashboardInts parses a repeated positive integer query parameter.
func dashboardInts(q url.Values, key string) ([]int, error) {
	var out []int
	for _, v := range q[key] {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			n, err := strconv.Atoi(part)
			if err != nil || n <= 0 {
				return nil, &apierror.ValidationError{Msg: key + " must be positive integers"}
			}
			out = append(out, n)
		}
	}
	return out, nil
}

// dashboardFlags parses several boolean parameters into their targets.
func dashboardFlags(q url.Values, flags map[string]*bool) error {
	for key, dst := range flags {
		v, err := dashboardBool(q, key)
		if err != nil {
			return err
		}
		*dst = v
	}
	return nil
}

// AllocationsToday handles GET /dashboard/allocations-today?date=&businessUnitId=.
func (h *DashboardHandler) AllocationsToday(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bu, err := dashboardInt(q, "businessUnitId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.AllocationsToday(r.Context(), domain.DashboardAllocationsTodayFilter{
		Date: strings.TrimSpace(q.Get("date")), BusinessUnitID: bu,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// MyWork handles GET /dashboard/my-work?email=&today=&trips=&tripLimit=.
func (h *DashboardHandler) MyWork(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.DashboardMyWorkFilter{Email: q.Get("email"), Today: q.Get("today")}
	var err error
	if f.Trips, err = dashboardBool(q, "trips"); err == nil {
		f.TripLimit, err = dashboardInt(q, "tripLimit")
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.MyWork(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Queues handles GET /dashboard/queues.
func (h *DashboardHandler) Queues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var f domain.DashboardQueuesFilter
	var err error
	if f.PendingApprovalTypes, err = dashboardInts(q, "pendingApprovalType"); err == nil {
		if f.ClearanceTypes, err = dashboardInts(q, "clearanceType"); err == nil {
			err = dashboardFlags(q, map[string]*bool{
				"travelRequests": &f.TravelRequests, "invoices": &f.Invoices, "payments": &f.Payments,
			})
		}
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Queues(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Travel handles GET /dashboard/travel.
func (h *DashboardHandler) Travel(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.DashboardTravelFilter{Today: q.Get("today")}
	if err := dashboardFlags(q, map[string]*bool{
		"trips": &f.Trips, "costInvoices": &f.CostInvoices, "costPayments": &f.CostPayments, "costCards": &f.CostCards,
	}); err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Travel(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Engagements handles GET /dashboard/engagements.
func (h *DashboardHandler) Engagements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.DashboardEngagementsFilter{Today: q.Get("today")}
	var err error
	if f.EngagementTypes, err = dashboardInts(q, "engagementType"); err == nil {
		err = dashboardFlags(q, map[string]*bool{"bench": &f.Bench, "trend": &f.Trend})
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Engagements(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Agency handles GET /dashboard/agency?agencyId=&today=.
func (h *DashboardHandler) Agency(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, err := dashboardInt(q, "agencyId")
	if err == nil && id == 0 {
		err = &apierror.ValidationError{Msg: "agencyId is required"}
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.Agency(r.Context(), id, q.Get("today"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
