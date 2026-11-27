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
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

// budgetRouteEntity serves empty budget data: the routes' gates are what is tested.
type budgetRouteEntity struct{ engagemententity.Client }

func (budgetRouteEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (budgetRouteEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if app == domain.AppAllocations {
		return map[string][]int{"wso2-employees": {1}, "allocator": {16}, "app-allocations-admin-super": {23}}, nil
	}
	return map[string][]int{"travel-finance": {35}, "travel-team": {6}, "app-travel-admin-super": {29}}, nil
}

func (budgetRouteEntity) RetireBudgetLine(context.Context, int, string) error { return nil }

func (budgetRouteEntity) BudgetYears(context.Context) ([]engagemententity.BudgetYear, error) {
	return []engagemententity.BudgetYear{{Year: 2027, StartDate: "2027-01-01", EndDate: "2027-12-31", IsActive: true}}, nil
}

func (budgetRouteEntity) BudgetLines(context.Context, int, bool) ([]engagemententity.BudgetLine, error) {
	return nil, nil
}

func (budgetRouteEntity) BudgetLedger(context.Context, string, string, bool) (engagemententity.BudgetLedger, error) {
	return engagemententity.BudgetLedger{}, nil
}

func (budgetRouteEntity) BudgetNeedsLine(context.Context, int, int, int) (engagemententity.BudgetNeedsLinePage, error) {
	return engagemententity.BudgetNeedsLinePage{}, nil
}

func (budgetRouteEntity) BudgetAllocationTypeRules(context.Context) ([]engagemententity.BudgetAllocationTypeRule, error) {
	return nil, nil
}

func (budgetRouteEntity) AllocationTypes(context.Context, engagemententity.AllocationTypeFilter) ([]engagemententity.AllocationType, error) {
	return nil, nil
}

func (budgetRouteEntity) AllocationEngagement(context.Context, string) (engagemententity.AllocationEngagement, error) {
	return engagemententity.AllocationEngagement{}, nil
}

func (budgetRouteEntity) SetBudgetTrip(_ context.Context, job string, _ *int, _ string) (engagemententity.BudgetTrip, error) {
	return engagemententity.BudgetTrip{JobNumber: job}, nil
}

func newBudgetRouter(enabled bool, groups ...string) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "mgr@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   budgetRouteEntity{},
		Budgets:  config.BudgetConfig{Enabled: enabled, CacheTTL: time.Minute},
		Now:      func() time.Time { return time.Date(2027, 3, 10, 0, 0, 0, 0, time.UTC) },
	})
}

var budgetRoutes = []struct{ method, path, body string }{
	{http.MethodGet, "/budgets/preview?engagementId=E1&travellerEmail=a@wso2.com&startDate=2027-02-01&endDate=2027-02-02", ""},
	{http.MethodGet, "/budgets/summary?year=2027", ""},
	{http.MethodGet, "/budgets/lines?year=2027", ""},
	{http.MethodPost, "/budgets/lines", `{}`},
	{http.MethodPut, "/budgets/lines/1", `{}`},
	{http.MethodDelete, "/budgets/lines/1", ""},
	{http.MethodGet, "/budgets/allocation-type-rules", ""},
	{http.MethodPut, "/budgets/allocation-type-rules/12", `{"ruleKind":"NONE"}`},
	{http.MethodGet, "/budgets/needs-line?year=2027", ""},
	{http.MethodPut, "/travel-requests/TRV-2027-01-0001/budget-line", `{"budgetLineId":1}`},
}

func TestBudgetRoutes_OffRegistersNothing(t *testing.T) {
	h := newBudgetRouter(false, "wso2-employees", "app-allocations-admin-super", "app-travel-admin-super", "travel-team")
	for _, c := range budgetRoutes {
		if rec := allocDo(h, c.method, c.path, c.body); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 404", c.method, c.path, rec.Code)
		}
	}
}

func TestBudgetRoutes_Gates(t *testing.T) {
	employee := newBudgetRouter(true, "wso2-employees")
	for _, c := range budgetRoutes {
		if rec := allocDo(employee, c.method, c.path, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("employee %s %s: status = %d, want 403", c.method, c.path, rec.Code)
		}
	}
	status := func(h http.Handler, method, path, body string) int { return allocDo(h, method, path, body).Code }

	// Travel 35 sees the summary, the list and the pickers, and cannot write.
	viewer := newBudgetRouter(true, "travel-finance")
	for _, p := range []string{"/budgets/summary?year=2027", "/budgets/needs-line?year=2027", "/budgets/lines?year=2027"} {
		if got := status(viewer, http.MethodGet, p, ""); got != http.StatusOK {
			t.Errorf("viewer %s = %d", p, got)
		}
	}
	for _, c := range budgetRoutes[3:5] {
		if got := status(viewer, c.method, c.path, c.body); got != http.StatusForbidden {
			t.Errorf("viewer %s %s = %d", c.method, c.path, got)
		}
	}
	if got := status(viewer, http.MethodPut, "/travel-requests/TRV-1/budget-line", `{"budgetLineId":1}`); got != http.StatusForbidden {
		t.Errorf("viewer set line = %d", got)
	}
	// An allocator gets the preview and the picker only.
	allocator := newBudgetRouter(true, "allocator")
	if got := status(allocator, http.MethodGet, budgetRoutes[0].path, ""); got != http.StatusOK {
		t.Errorf("allocator preview = %d", got)
	}
	if got := status(allocator, http.MethodGet, "/budgets/lines", ""); got != http.StatusOK {
		t.Errorf("allocator lines = %d", got)
	}
	if got := status(allocator, http.MethodGet, "/budgets/summary", ""); got != http.StatusForbidden {
		t.Errorf("allocator summary = %d", got)
	}
	// The Travel Team sets a line, and the budget admin needs both groups.
	team := newBudgetRouter(true, "travel-team")
	if got := status(team, http.MethodPut, "/travel-requests/TRV-1/budget-line", `{"budgetLineId":1}`); got != http.StatusOK {
		t.Errorf("team set line = %d", got)
	}
	one := newBudgetRouter(true, "app-travel-admin-super")
	if got := status(one, http.MethodDelete, "/budgets/lines/1", ""); got != http.StatusForbidden {
		t.Errorf("one group delete = %d, want the service 403", got)
	}
	both := newBudgetRouter(true, "app-travel-admin-super", "app-allocations-admin-super")
	if got := status(both, http.MethodDelete, "/budgets/lines/1", ""); got != http.StatusNoContent {
		t.Errorf("admin delete = %d", got)
	}
	if got := status(both, http.MethodGet, "/budgets/allocation-type-rules", ""); got != http.StatusOK {
		t.Errorf("admin rules = %d", got)
	}
	if rec := allocDo(both, http.MethodPost, "/budgets/lines", `{"bogus":1}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Unknown field") {
		t.Errorf("unknown field = %d %s", rec.Code, rec.Body)
	}
}
