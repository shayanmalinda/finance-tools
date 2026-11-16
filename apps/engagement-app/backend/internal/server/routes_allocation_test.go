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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

type allocRouteEntity struct {
	engagemententity.Client
}

func (allocRouteEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (allocRouteEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if app == domain.AppAllocations {
		return map[string][]int{"alloc-mgr": {6, 9, 16, 20, 21, 22, 32}, "wso2-employees": {1}}, nil
	}
	return map[string][]int{}, nil
}

func (allocRouteEntity) AllocationEngagement(_ context.Context, id string) (engagemententity.AllocationEngagement, error) {
	s := func(v string) *string { return &v }
	return engagemententity.AllocationEngagement{ID: id, EngagementTypeID: 6, StartDate: s("2026-11-01"), EndDate: s("2026-11-30"),
		RequestStatus: s("In Progress"), EngagementCode: s("NWB-CO")}, nil
}

func (allocRouteEntity) AllocationTimeline(context.Context, engagemententity.AllocationTimelineRequest) ([]engagemententity.AllocationRecord, error) {
	s := func(v string) *string { return &v }
	return []engagemententity.AllocationRecord{{ID: "A1", Email: "jane@wso2.com", StartDate: "2026-11-01", EndDate: "2026-11-30",
		EngagementID: s("E1"), ClearanceStatus: s("Confirmed"), Engagement: &engagemententity.AllocationEngagementRef{ID: "E1"}}}, nil
}

func (allocRouteEntity) ConsultantAllocations(context.Context, engagemententity.ConsultantAllocationFilter) ([]engagemententity.ConsultantAllocation, error) {
	s := func(v string) *string { return &v }
	return []engagemententity.ConsultantAllocation{{ID: "A1", Email: "jane@wso2.com", StartDate: "2026-11-02", EndDate: "2026-11-02",
		EngagementID: s("E1"), InitialEngagementCode: s("=CMD"), AllocationTypeName: s("Consulting"), EngagementTypeName: s("Paid")}}, nil
}

func (allocRouteEntity) ConsultantNonAllocated(context.Context, engagemententity.ConsultantAllocationFilter) ([]engagemententity.ConsultantNonAllocatedEmployee, error) {
	return nil, nil
}

func (allocRouteEntity) ConsultantTeamDefaults(context.Context) ([]engagemententity.ConsultantTeamDefault, error) {
	return nil, nil
}

func (allocRouteEntity) ConsultantEngagementCodes(context.Context) ([]engagemententity.ConsultantEngagementCode, error) {
	return nil, nil
}

type allocRouteHR struct{ hrentity.Unconfigured }

func (allocRouteHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	return &hrentity.Employee{WorkEmail: e}, nil
}

func newAllocationRouter(groups ...string) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "mgr@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   allocRouteEntity{},
		HR:       allocRouteHR{},
		Now:      func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) },
	})
}

func allocDo(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req.Header.Set("X-Jwt-Assertion", "valid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAllocationRoutes_Gates(t *testing.T) {
	employee := newAllocationRouter("wso2-employees")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/engagements/E1/allocations", `{}`},
		{http.MethodGet, "/allocation-timeline?email=a@wso2.com&from=2026-11-01&to=2026-11-30", ""},
		{http.MethodGet, "/allocations", ""},
		{http.MethodPatch, "/allocations/A1/status", `{}`},
		{http.MethodGet, "/team-allocations?team=T&from=2026-11-01&to=2026-11-30", ""},
		{http.MethodPost, "/team-allocations", `{}`},
		{http.MethodPatch, "/team-allocations/A1", `{}`},
		{http.MethodDelete, "/team-allocations/A1", ""},
		{http.MethodPost, "/team-allocations/bulk/validate", `{"csv":""}`},
		{http.MethodPost, "/team-allocations/bulk", `{"csv":""}`},
		{http.MethodGet, "/reports/consultant-allocations?from=2026-11-01&to=2026-11-30", ""},
	} {
		if rec := allocDo(employee, c.method, c.path, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", c.method, c.path, rec.Code)
		}
	}
}

func TestAllocationRoutes_ValidationOverlapAndCSV(t *testing.T) {
	h := newAllocationRouter("alloc-mgr")
	if rec := allocDo(h, http.MethodGet, "/allocation-timeline?from=2026-11-01", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("timeline without to: %d", rec.Code)
	}
	if rec := allocDo(h, http.MethodGet, "/allocations?travelRequired=maybe", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad travelRequired: %d", rec.Code)
	}
	if rec := allocDo(h, http.MethodPost, "/engagements/E1/allocations", `{"bogus":1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rec.Code)
	}

	rec := allocDo(h, http.MethodPost, "/engagements/E1/allocations",
		`{"consultantEmail":"jane@wso2.com","startDate":"2026-11-10","endDate":"2026-11-20","travelRequired":false}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("overlap: %d %s", rec.Code, rec.Body)
	}
	var body domain.AllocationOverlapError
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Code != 409 || len(body.Overlaps) != 1 ||
		!strings.HasPrefix(body.Message, "This person is already allocated") {
		t.Errorf("409 body = %+v, %v", body, err)
	}

	rec = allocDo(h, http.MethodGet, "/reports/consultant-allocations?from=2026-11-02&to=2026-11-02&format=csv", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "consultant-allocations-2026-11-02-to-2026-11-02.csv") {
		t.Fatalf("csv: %d %v", rec.Code, rec.Header())
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if !strings.HasPrefix(lines[0], "Engagement Type,Engagement Location,Country") || !strings.Contains(lines[1], "'=CMD") {
		t.Errorf("csv body = %s", rec.Body)
	}
	rec = allocDo(h, http.MethodGet, "/reports/consultant-allocations?from=2026-11-02&to=2026-11-02", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[`) {
		t.Errorf("json report: %d %s", rec.Code, rec.Body)
	}
	if rec := allocDo(h, http.MethodGet, "/reports/consultant-allocations?from=2026-11-02&to=2026-11-02&format=xml", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown format: %d", rec.Code)
	}
	if rec := allocDo(h, http.MethodGet, "/team-allocations?from=2026-11-01&to=2026-11-30", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("team search without org: %d", rec.Code)
	}
}
