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
	"net/http"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

func searchRouter(groups ...string) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   fakeEntity{},
	})
}

func TestSearchRoute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		router http.Handler
		path   string
		token  string
		want   int
	}{
		{"no token", searchRouter("wso2-employees"), "/search?q=abc", "", http.StatusUnauthorized},
		// wso2-employees holds Travel 1 and 2 in fakeEntity: passes the gate.
		{"gated caller, short q", searchRouter("wso2-employees"), "/search?q=a", "valid", http.StatusBadRequest},
		{"no source privilege", searchRouter("nobody"), "/search?q=abc", "valid", http.StatusForbidden},
		{"no source privilege, bad q is still 403", searchRouter("nobody"), "/search?q=a", "valid", http.StatusForbidden},
	} {
		if rec := get(tc.router, tc.path, tc.token); rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body)
		}
	}
}
