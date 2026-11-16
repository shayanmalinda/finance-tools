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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/middleware"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// engagementRouteEntity is a Client plus EngagementAPI: the role table
// (group = privileges) and a few engagement answers.
type engagementRouteEntity struct {
	engagemententity.Client
}

func (engagementRouteEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (engagementRouteEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if app == domain.AppAllocations {
		return map[string][]int{"alloc-admin": {2, 3, 5, 23, 24, 25, 26, 28, 4}, "alloc-viewer": {3}, "app-allocations-admin-super-stg": {}}, nil
	}
	return map[string][]int{"travel-roles": {29}, "app-travel-admin-super-stg": {}}, nil
}

func (engagementRouteEntity) TravelAgency(context.Context, string) (*domain.TravelAgency, error) {
	return nil, nil
}

func (engagementRouteEntity) ReferenceData(context.Context) (engagemententity.ReferenceLists, error) {
	return engagemententity.ReferenceLists{}, nil
}

func (engagementRouteEntity) EngagementSearch(context.Context, engagemententity.EngagementSearchQuery) (engagemententity.EngagementSearchResult, error) {
	return engagemententity.EngagementSearchResult{Items: []engagemententity.EngagementRecord{}, TypeCounts: map[int]int{}}, nil
}

func (engagementRouteEntity) EngagementGet(_ context.Context, id string) (engagemententity.EngagementRecord, error) {
	return engagemententity.EngagementRecord{}, &apierror.UpstreamError{Service: "engagement-entity", StatusCode: 404, Msg: "Engagement " + id + " not found."}
}

func (engagementRouteEntity) RoleSettings(context.Context, domain.App) (engagemententity.RoleSettingsLists, error) {
	return engagemententity.RoleSettingsLists{}, nil
}

// engagementRouter is the authenticated mux with only this area's routes;
// the token value is the caller's comma-separated groups.
func engagementRouter() http.Handler {
	d := Deps{Entity: engagementRouteEntity{}, Sales: salesentity.Unconfigured{}, HR: hrentity.Unconfigured{},
		Email: email.LogSender{}, Now: time.Now}
	api := http.NewServeMux()
	a := &area{Deps: d, Access: service.NewAccessService(d.Entity), Idem: middleware.NewIdempotency(), mux: api}
	registerEngagementRoutes(a)
	return middleware.Auth(engagementGroupsVerifier{}, nil)(api)
}

type engagementGroupsVerifier struct{}

func (engagementGroupsVerifier) Verify(_ context.Context, raw string) (oidc.Claims, error) {
	return oidc.Claims{Email: "jane@wso2.com", Groups: strings.Split(raw, ","), GroupsPresent: true}, nil
}

const bothSuper = "app-allocations-admin-super-stg,app-travel-admin-super-stg"

func engagementDo(h http.Handler, method, path, groups, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Jwt-Assertion", groups)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEngagementRoutes(t *testing.T) {
	h := engagementRouter()
	tests := []struct {
		name, method, path, groups, body string
		want                             int
	}{
		{"reference data for anyone", "GET", "/reference-data", "nobody", "", 200},
		{"list allowed", "GET", "/engagements?segment=customer", "alloc-viewer", "", 200},
		{"list gate", "GET", "/engagements?segment=customer", "nobody", "", 403},
		{"list segment required", "GET", "/engagements", "alloc-viewer", "", 400},
		{"list bad date", "GET", "/engagements?segment=customer&startFrom=05-11-2026", "alloc-viewer", "", 400},
		{"list bad limit", "GET", "/engagements?segment=customer&limit=0", "alloc-viewer", "", 400},
		{"viewer may not see the internal segment", "GET", "/engagements?segment=internal", "alloc-viewer", "", 403},
		{"engagement not found", "GET", "/engagements/E404", "alloc-viewer", "", 404},
		{"create gate", "POST", "/engagements", "alloc-viewer", `{}`, 403},
		{"create body", "POST", "/engagements", "alloc-admin", `{"bogus": 1}`, 400},
		{"patch gate", "PATCH", "/engagements/E1", "alloc-viewer", `{}`, 403},
		{"approve gate", "POST", "/engagements/E1/approve", "alloc-viewer", "", 403},
		{"request status body", "POST", "/engagements/E1/request-status", "alloc-viewer", `{"status":"Completed"}`, 403},
		{"approvals segment", "GET", "/approvals?segment=all", "alloc-admin", "", 400},
		{"approvals allowed", "GET", "/approvals?segment=internal", "alloc-admin", "", 200},
		{"accounts gate", "GET", "/accounts?search=ac", "alloc-viewer", "", 403},
		{"accounts too short", "GET", "/accounts?search=a", "alloc-admin", "", 400},
		{"accounts sales unconfigured", "GET", "/accounts?search=acme", "alloc-admin", "", 503},
		{"role settings need both super admin groups", "GET", "/role-settings/travel", "travel-roles", "", 403},
		{"role settings travel by travel 29", "GET", "/role-settings/travel", "travel-roles," + bothSuper, "", 200},
		{"role settings allocations needs 23", "GET", "/role-settings/allocations", "travel-roles," + bothSuper, "", 403},
		{"role settings bad role id", "PUT", "/role-settings/allocations/roles/x/privileges", "alloc-admin", `{"privilegeIds":[]}`, 400},
		{"allocation types gate", "GET", "/settings/allocation-types", "alloc-viewer", "", 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := engagementDo(h, tc.method, tc.path, tc.groups, tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
