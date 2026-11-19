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

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

type fakeVerifier struct {
	claims oidc.Claims
	err    error
	got    string
}

func (f *fakeVerifier) Verify(_ context.Context, raw string) (oidc.Claims, error) {
	f.got = raw
	return f.claims, f.err
}

type fakeUserinfo struct {
	claims oidc.Claims
	err    error
	calls  int
}

func (f *fakeUserinfo) Fetch(context.Context, string) (oidc.Claims, error) {
	f.calls++
	return f.claims, f.err
}

func TestAuth(t *testing.T) {
	full := oidc.Claims{Email: "Jane@WSO2.com", Groups: []string{"wso2-employees"}, GroupsPresent: true}
	tests := []struct {
		name          string
		header        map[string]string
		verifier      *fakeVerifier
		userinfo      *fakeUserinfo
		wantStatus    int
		wantUserinfo  int
		wantGroupsLen int
	}{
		{"valid assertion", map[string]string{"X-Jwt-Assertion": "tok"}, &fakeVerifier{claims: full}, &fakeUserinfo{}, http.StatusOK, 0, 1},
		{"valid bearer", map[string]string{"Authorization": "Bearer tok"}, &fakeVerifier{claims: full}, &fakeUserinfo{}, http.StatusOK, 0, 1},
		{"bearer without groups asks userinfo", map[string]string{"Authorization": "bearer tok"},
			&fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com"}},
			&fakeUserinfo{claims: oidc.Claims{Email: "jane@wso2.com", Groups: []string{"a", "b"}, GroupsPresent: true}}, http.StatusOK, 1, 2},
		{"bearer without email asks userinfo", map[string]string{"Authorization": "Bearer tok"},
			&fakeVerifier{claims: oidc.Claims{GroupsPresent: true, Groups: []string{"a"}}},
			&fakeUserinfo{claims: oidc.Claims{Email: "jane@wso2.com"}}, http.StatusOK, 1, 1},
		// The groups are unknown, not empty: no access would be the wrong answer.
		{"userinfo down is unavailable, not no access", map[string]string{"Authorization": "Bearer tok"},
			&fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com"}}, &fakeUserinfo{err: errors.New("down")},
			http.StatusServiceUnavailable, 1, 0},
		// Userinfo refusing the token is a sign-in problem, not an outage.
		{"userinfo rejecting the token is 401", map[string]string{"Authorization": "Bearer tok"},
			&fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com"}}, &fakeUserinfo{err: oidc.ErrUserinfoRejected},
			http.StatusUnauthorized, 1, 0},
		{"signing keys unavailable is not sign in again", map[string]string{"X-Jwt-Assertion": "tok"},
			&fakeVerifier{err: fmt.Errorf("oidc: validating token: %w", oidc.ErrKeysUnavailable)}, &fakeUserinfo{},
			http.StatusServiceUnavailable, 0, 0},
		{"gateway JWT never goes to userinfo", map[string]string{"X-Jwt-Assertion": "tok"},
			&fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com"}}, &fakeUserinfo{}, http.StatusOK, 0, 0},
		{"missing header", nil, &fakeVerifier{}, &fakeUserinfo{}, http.StatusUnauthorized, 0, 0},
		{"basic auth is not a token", map[string]string{"Authorization": "Basic abc"}, &fakeVerifier{claims: full}, &fakeUserinfo{}, http.StatusUnauthorized, 0, 0},
		{"invalid token", map[string]string{"X-Jwt-Assertion": "tok"}, &fakeVerifier{err: errors.New("expired")}, &fakeUserinfo{}, http.StatusUnauthorized, 0, 0},
		{"no email anywhere", map[string]string{"Authorization": "Bearer tok"},
			&fakeVerifier{claims: oidc.Claims{GroupsPresent: true}}, &fakeUserinfo{}, http.StatusUnauthorized, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var caller reqcontext.Caller
			h := Auth(tc.verifier, tc.userinfo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				caller, _ = reqcontext.CallerFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/user-info", nil)
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				if tc.userinfo.calls != tc.wantUserinfo {
					t.Errorf("userinfo calls = %d, want %d", tc.userinfo.calls, tc.wantUserinfo)
				}
				var body struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Code != tc.wantStatus || body.Message == "" {
					t.Errorf("body = %+v, %v", body, err)
				}
				return
			}
			if caller.Email != "jane@wso2.com" || len(caller.Groups) != tc.wantGroupsLen {
				t.Errorf("caller = %+v, want lower-cased email and %d groups", caller, tc.wantGroupsLen)
			}
			if tc.userinfo.calls != tc.wantUserinfo {
				t.Errorf("userinfo calls = %d, want %d", tc.userinfo.calls, tc.wantUserinfo)
			}
		})
	}
}

type fakeChecker struct {
	allow bool
	err   error
}

func (f fakeChecker) HasAnyPrivilege(context.Context, domain.App, []string, ...int) (bool, error) {
	return f.allow, f.err
}

func TestRequireAnyPrivilege(t *testing.T) {
	tests := []struct {
		name       string
		checker    fakeChecker
		withCaller bool
		want       int
	}{
		{"allowed", fakeChecker{allow: true}, true, http.StatusOK},
		{"denied", fakeChecker{}, true, http.StatusForbidden},
		{"role tables unreadable", fakeChecker{err: errors.New("down")}, true, http.StatusServiceUnavailable},
		{"outside Auth", fakeChecker{allow: true}, false, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireAnyPrivilege(tc.checker, domain.AppAllocations, 2)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			req := httptest.NewRequest(http.MethodPost, "/engagements", nil)
			if tc.withCaller {
				req = req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: "a@wso2.com"}))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://one.wso2.com/"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	pre := httptest.NewRequest(http.MethodOptions, "/user-info", nil)
	pre.Header.Set("Origin", "https://one.wso2.com")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pre)
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Errorf("allow-headers = %q", rec.Header().Get("Access-Control-Allow-Headers"))
	}
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://one.wso2.com" {
		t.Errorf("preflight: status %d, allow-origin %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	other := httptest.NewRequest(http.MethodGet, "/user-info", nil)
	other.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, other)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code != http.StatusTeapot {
		t.Errorf("unlisted origin: allow-origin %q, status %d", rec.Header().Get("Access-Control-Allow-Origin"), rec.Code)
	}
}

type fakeResolver struct {
	perms domain.Perms
	err   error
}

func (f fakeResolver) Perms(context.Context, reqcontext.Caller) (domain.Perms, error) {
	return f.perms, f.err
}

func TestRequire(t *testing.T) {
	travel18 := domain.Perms{Email: "a@wso2.com", Alloc: domain.PrivilegeSet{}, Travel: domain.NewPrivilegeSet([]int{18})}
	tests := []struct {
		name       string
		resolver   fakeResolver
		gate       domain.Gate
		withCaller bool
		want       int
	}{
		{"travel gate passes", fakeResolver{perms: travel18}, domain.Gate{domain.AppTravel: {1, 18}}, true, http.StatusOK},
		{"same id in the other app does not pass", fakeResolver{perms: travel18}, domain.Gate{domain.AppAllocations: {18}}, true, http.StatusForbidden},
		{"either app", fakeResolver{perms: travel18}, domain.Gate{domain.AppAllocations: {6}, domain.AppTravel: {18}}, true, http.StatusOK},
		{"resolver down", fakeResolver{err: errors.New("down")}, domain.Gate{domain.AppTravel: {18}}, true, http.StatusServiceUnavailable},
		{"outside Auth", fakeResolver{perms: travel18}, domain.Gate{domain.AppTravel: {18}}, false, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got domain.Perms
			h := Require(tc.resolver, tc.gate)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got, _ = reqcontext.PermsFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/travel-requests", nil)
			if tc.withCaller {
				req = req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: "a@wso2.com"}))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusOK && got.Email != "a@wso2.com" {
				t.Error("perms not attached to the context")
			}
		})
	}
}
