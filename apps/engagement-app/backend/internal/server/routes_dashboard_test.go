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
