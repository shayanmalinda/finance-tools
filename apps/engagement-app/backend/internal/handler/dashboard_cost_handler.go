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

// costQuery reads the range, comparison and filters shared by the three
// Travel Costs endpoints. Values of source, employee, jobNumber, travelAgent
// and paid may be repeated or comma-joined; country, customer and costType
// may contain commas, so they are repeated only.
func costQuery(r *http.Request) (service.CostQuery, error) {
	q := service.CostQuery{Compare: true}
	var err error
	if q.From, err = queryDate(r, "from"); err != nil {
		return q, err
	}
	if q.To, err = queryDate(r, "to"); err != nil {
		return q, err
	}
	if v := strings.TrimSpace(r.URL.Query().Get("compare")); v != "" {
		if q.Compare, err = strconv.ParseBool(v); err != nil {
			return q, &apierror.ValidationError{Msg: "compare must be true or false."}
		}
	}
	f := &q.Filter
	f.Sources = queryMulti(r, "source")
	f.Employees = queryMulti(r, "employee")
	f.JobNumbers = queryMulti(r, "jobNumber")
	f.Paid = queryMulti(r, "paid")
	f.Countries = queryList(r, "country")
	f.Customers = queryList(r, "customer")
	f.CostTypes = queryList(r, "costType")
	for _, v := range queryMulti(r, "travelAgent") {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return q, &apierror.ValidationError{Msg: "travelAgent must be whole numbers, 0 or more."}
		}
		f.TravelAgents = append(f.TravelAgents, n)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("unlinked")); v != "" {
		if f.Unlinked, err = strconv.ParseBool(v); err != nil {
			return q, &apierror.ValidationError{Msg: "unlinked must be true or false."}
		}
	}
	return q, nil
}

func writeCostCSV(w http.ResponseWriter, c *service.CostCSV) {
	if len(c.Rows) == 0 {
		writeCSVFile(w, c.Filename, nil, nil, true)
		return
	}
	writeCSVFile(w, c.Filename, c.Rows[0], c.Rows[1:], true)
}

// TravelCosts handles GET /dashboard/travel-costs.
func (h *DashboardHandler) TravelCosts(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	csvOut, err := reportCSV(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	q, err := costQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if csvOut {
		c, err := h.svc.TravelCostsCSV(r.Context(), p, q, r.URL.Query().Get("section"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeCostCSV(w, c)
		return
	}
	resp, err := h.svc.TravelCosts(r.Context(), p, q)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// CostTransactions handles GET /dashboard/travel-costs/transactions.
func (h *DashboardHandler) CostTransactions(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	csvOut, err := reportCSV(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	q, err := costQuery(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if csvOut {
		c, err := h.svc.CostTransactionsCSV(r.Context(), p, q)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeCostCSV(w, c)
		return
	}
	limit, offset, err := costPaging(r)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	resp, err := h.svc.CostTransactions(r.Context(), p, q, service.CostTransactionsQuery{
		GroupBy: r.URL.Query().Get("groupBy"), Sort: r.URL.Query().Get("sort"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// costPaging reads limit (1 to 100, default 25) and offset.
func costPaging(r *http.Request) (limit, offset int, err error) {
	limit = 25
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			return 0, 0, &apierror.ValidationError{Msg: "limit must be between 1 and 100."}
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return 0, 0, &apierror.ValidationError{Msg: "offset must be 0 or more."}
		}
		offset = n
	}
	return limit, offset, nil
}

// JobCostSummary handles GET /travel-requests/{jobNumber}/cost-summary.
func (h *DashboardHandler) JobCostSummary(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.JobCostSummary(r.Context(), p, r.PathValue("jobNumber"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
