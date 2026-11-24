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

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

// dashboardEntity is a fake entity with a role table and canned dashboard data.
type dashboardEntity struct {
	engagemententity.Client
	roles     map[domain.App]map[string][]int
	allocDate string
	buID      int
}

func (*dashboardEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (f *dashboardEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	return f.roles[app], nil
}
func (f *dashboardEntity) TravelAgency(context.Context, string) (*domain.TravelAgency, error) {
	return nil, nil
}
func (f *dashboardEntity) DashboardAllocationsToday(_ context.Context, date string, bu int) (*engagemententity.DashboardAllocationsToday, error) {
	f.allocDate, f.buID = date, bu
	return &engagemententity.DashboardAllocationsToday{
		ByEngagementType: []engagemententity.DashboardEngagementTypeCount{{EngagementTypeID: 5, EngagementType: "Marketing Event", Count: 3}},
	}, nil
}
func (f *dashboardEntity) DashboardCostLines(context.Context, engagemententity.DashboardCostLinesFilter) (*engagemententity.DashboardCostLines, error) {
	cust := "=SUM(A1)"
	return &engagemententity.DashboardCostLines{Items: []engagemententity.DashboardCostEntry{
		{ID: "INV-1", Source: "invoice", Date: "2026-10-01", OriginalCurrency: "USD", OriginalAmount: 12.5, CostType: "Hotel", Customer: &cust, Paid: "paid"},
	}}, nil
}
func (f *dashboardEntity) DashboardMyVisas(context.Context, string, string) (*engagemententity.DashboardMyVisas, error) {
	return &engagemententity.DashboardMyVisas{}, nil
}
func (f *dashboardEntity) DashboardMyWork(context.Context, engagemententity.DashboardMyWorkFilter) (*engagemententity.DashboardMyWork, error) {
	return &engagemententity.DashboardMyWork{
		Trips: []engagemententity.DashboardTrip{{JobNumber: "TRV-2026-11-0001", TravellerEmail: "jane@wso2.com", StartDate: "2026-11-10", EndDate: "2026-11-12"}},
	}, nil
}

func newDashboardRouter(groups []string, roles map[domain.App]map[string][]int) (http.Handler, *dashboardEntity) {
	e := &dashboardEntity{roles: roles}
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   e,
		Now:      func() time.Time { return time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC) },
	}), e
}

var dashboardEmployeeRoles = map[domain.App]map[string][]int{
	domain.AppAllocations: {"wso2-employees": {1}},
	domain.AppTravel:      {"wso2-employees": {19, 20, 21, 22, 23, 24, 36, 37, 38, 47, 48, 49}},
}

func TestDashboardRoutes_Gates(t *testing.T) {
	// A signed-in person outside every role: 403 on all three.
	h, _ := newDashboardRouter([]string{"someone-else"}, dashboardEmployeeRoles)
	for _, path := range []string{"/dashboard", "/dashboard/allocations-today", "/me/overview"} {
		if rec := get(h, path, "valid"); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", path, rec.Code)
		}
	}
	// An employee passes all three.
	h, _ = newDashboardRouter([]string{"wso2-employees"}, dashboardEmployeeRoles)
	for _, path := range []string{"/dashboard", "/dashboard/allocations-today", "/me/overview"} {
		if rec := get(h, path, "valid"); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body %s", path, rec.Code, rec.Body)
		}
	}
	// No token: 401.
	if rec := get(h, "/dashboard", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d", rec.Code)
	}
}

func TestDashboardRoutes_AllocationsToday(t *testing.T) {
	h, e := newDashboardRouter([]string{"wso2-employees"}, dashboardEmployeeRoles)
	for _, path := range []string{
		"/dashboard/allocations-today?date=05-11-2026",
		"/dashboard/allocations-today?date=2026-11-06",
		"/dashboard/allocations-today?businessUnitId=IAM",
	} {
		if rec := get(h, path, "valid"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", path, rec.Code)
		}
	}
	rec := get(h, "/dashboard/allocations-today?date=2026-10-01&businessUnitId=4", "valid")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if e.allocDate != "2026-10-01" || e.buID != 4 {
		t.Errorf("entity got date %q bu %d", e.allocDate, e.buID)
	}
	var body domain.DashboardAllocationsToday
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Allocations != 3 || len(body.ByEngagementType) != 1 || body.ByEngagementType[0].Percent != 100 ||
		body.PaidByAllocationType == nil || body.NonPaidByAllocationType == nil {
		t.Errorf("body = %+v", body)
	}
}

func TestDashboardRoutes_MeOverview(t *testing.T) {
	h, _ := newDashboardRouter([]string{"wso2-employees"}, dashboardEmployeeRoles)
	rec := get(h, "/me/overview", "valid")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	const want = `{"upcomingAllocations":[],"upcomingTrips":[{"jobNumber":"TRV-2026-11-0001","status":null,"traveller":{"email":"jane@wso2.com","name":null},"city":null,"country":null,"startDate":"2026-11-10","endDate":"2026-11-12","agency":null}]}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %s\nwant   %s", rec.Body, want)
	}
}

var dashboardFinanceRoles = map[domain.App]map[string][]int{
	domain.AppAllocations: {"wso2-employees": {1}},
	domain.AppTravel: {
		"wso2-employees":              {19, 20, 21, 22, 23, 24, 36, 37, 38, 47, 48, 49},
		"app-travel-approver-finance": {39, 41},
	},
}

func TestDashboardRoutes_TravelCosts(t *testing.T) {
	costPaths := []string{"/dashboard/travel-costs", "/dashboard/travel-costs/transactions", "/dashboard/travel-costs/duplicates"}
	// An employee is not allowed: 403 on all three.
	h, _ := newDashboardRouter([]string{"wso2-employees"}, dashboardFinanceRoles)
	for _, path := range costPaths {
		if rec := get(h, path, "valid"); rec.Code != http.StatusForbidden {
			t.Errorf("employee %s: status = %d, want 403", path, rec.Code)
		}
		if rec := get(h, path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("no token %s: status = %d, want 401", path, rec.Code)
		}
	}
	h, _ = newDashboardRouter([]string{"wso2-employees", "app-travel-approver-finance"}, dashboardFinanceRoles)
	for _, path := range costPaths {
		if rec := get(h, path+"?from=2026-01-01&to=2026-11-05", "valid"); rec.Code != http.StatusOK {
			t.Errorf("finance %s: status = %d, body %s", path, rec.Code, rec.Body)
		}
	}
	// Parameter validation.
	for _, q := range []string{
		"?from=2026-01-01", "?from=x&to=2026-01-02", "?from=2026-02-01&to=2026-01-01", "?from=2024-01-01&to=2026-11-05",
		"?compare=maybe", "?source=cash", "?paid=maybe", "?travelAgent=x", "?travelAgent=-1", "?unlinked=2", "?format=xml",
	} {
		if rec := get(h, "/dashboard/travel-costs"+q, "valid"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", q, rec.Code, rec.Body)
		}
	}
	for _, q := range []string{"?groupBy=colour", "?sort=size", "?limit=0", "?limit=101", "?offset=-1"} {
		if rec := get(h, "/dashboard/travel-costs/transactions"+q, "valid"); rec.Code != http.StatusBadRequest {
			t.Errorf("transactions %s: status = %d, want 400", q, rec.Code)
		}
	}
	if rec := get(h, "/dashboard/travel-costs?section=nope&format=csv", "valid"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad csv section: status = %d", rec.Code)
	}
}

// The CSV total equals the JSON total, and a cell that starts like a formula is neutralised.
func TestDashboardRoutes_TravelCostsCSV(t *testing.T) {
	h, _ := newDashboardRouter([]string{"wso2-employees", "app-travel-approver-finance"}, dashboardFinanceRoles)
	rec := get(h, "/dashboard/travel-costs/transactions?from=2026-01-01&to=2026-11-05&format=csv", "valid")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "Travel_Costs_Transactions_2026-01-01_2026-11-05.csv") {
		t.Fatalf("csv: %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Total USD,12.50") || !strings.Contains(body, "'=SUM(A1)") || strings.Contains(body, ",=SUM(A1)") {
		t.Errorf("csv body:\n%s", body)
	}
	rec = get(h, "/dashboard/travel-costs/transactions?from=2026-01-01&to=2026-11-05", "valid")
	var page struct {
		TotalUSD float64 `json:"totalUsd"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil || page.TotalUSD != 12.5 {
		t.Errorf("json total = %v %v", page.TotalUSD, err)
	}
	rec = get(h, "/dashboard/travel-costs?from=2026-01-01&to=2026-11-05&format=csv&section=kpis", "valid")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Total Spend (USD),12.50") {
		t.Errorf("summary csv: %d %s", rec.Code, rec.Body)
	}
}

func TestDashboardRoutes_Sections(t *testing.T) {
	h, _ := newDashboardRouter([]string{"wso2-employees"}, dashboardEmployeeRoles)
	if rec := get(h, "/dashboard?sections=myWork,nope", "valid"); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown section: status = %d", rec.Code)
	}
	rec := get(h, "/dashboard?sections=myWork&sections=travelTeam", "valid")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"widgets":[{"id":"my-work","defaultOn":true}`) || strings.Contains(rec.Body.String(), "travelTeam") {
		t.Errorf("sections: %d %s", rec.Code, rec.Body)
	}
}
