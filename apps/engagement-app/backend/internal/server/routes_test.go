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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

type fakeVerifier struct{ claims oidc.Claims }

func (f fakeVerifier) Verify(_ context.Context, raw string) (oidc.Claims, error) {
	if raw != "valid" {
		return oidc.Claims{}, errors.New("bad token")
	}
	return f.claims, nil
}

type fakeEntity struct {
	engagemententity.Client
	down bool
}

func (fakeEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (f fakeEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if f.down {
		return nil, &apierror.UpstreamError{Service: "engagement-entity", Msg: "unreachable"}
	}
	if app == domain.AppAllocations {
		return map[string][]int{"wso2-employees": {1}, "app-allocations-admin-super": {1, 2, 23}}, nil
	}
	return map[string][]int{"wso2-employees": {1, 2}}, nil
}

func newRouter(entity fakeEntity) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "Jane@wso2.com", Groups: []string{"wso2-employees", "app-allocations-admin-super"}, GroupsPresent: true}},
		Entity:   entity,
	})
}

func get(h http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("X-Jwt-Assertion", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthNeedsNoToken(t *testing.T) {
	if rec := get(newRouter(fakeEntity{}), "/health", ""); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestUserInfo(t *testing.T) {
	rec := get(newRouter(fakeEntity{}), "/user-info", "valid")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	const want = `{"email":"jane@wso2.com","allocations":{"privileges":[1,2,23],"ownPrivileges":[1],"roles":["app-allocations-admin-super","wso2-employees"]},"travel":{"privileges":[1,2],"ownPrivileges":[1,2],"roles":["wso2-employees"],"agency":null},"workQueues":{"invoices":[],"perDiemPayments":[],"netsuiteSyncFailures":false}}` + "\n"
	if rec.Body.String() != want {
		t.Errorf("body = %s, want %s", rec.Body, want)
	}
}

func TestUserInfoRequiresAValidToken(t *testing.T) {
	for _, token := range []string{"", "forged"} {
		if rec := get(newRouter(fakeEntity{}), "/user-info", token); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401", token, rec.Code)
		}
	}
}

// The entity being down is 503 (the gate's "Failed" rung), never 403 or an
// empty privilege list (the "Denied" rung).
func TestUserInfoEntityDownIs503(t *testing.T) {
	rec := get(newRouter(fakeEntity{down: true}), "/user-info", "valid")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Code != 503 || body.Message == "" {
		t.Errorf("body = %+v, %v", body, err)
	}
}

func TestUnknownRouteIsAuthenticatedFirst(t *testing.T) {
	h := newRouter(fakeEntity{})
	if rec := get(h, "/no-such-screen", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", rec.Code)
	}
	if rec := get(h, "/no-such-screen", "valid"); rec.Code != http.StatusNotFound {
		t.Errorf("valid token: status = %d, want 404", rec.Code)
	}
}

// An unknown path or a wrong method answers with the same {code, message}
// JSON as every other error, not ServeMux's plain text.
func TestMuxErrorsAreJSON(t *testing.T) {
	h := newRouter(fakeEntity{})
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/no-such-screen", http.StatusNotFound},
		{http.MethodDelete, "/user-info", http.StatusMethodNotAllowed},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-Jwt-Assertion", "valid")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if rec.Code != tc.want || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s %s: status %d, content type %q", tc.method, tc.path, rec.Code, rec.Header().Get("Content-Type"))
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Code != tc.want || body.Message == "" {
			t.Errorf("%s %s: body = %+v, %v", tc.method, tc.path, body, err)
		}
		if tc.want == http.StatusMethodNotAllowed && rec.Header().Get("Allow") == "" {
			t.Error("405 lost the Allow header")
		}
	}
}
