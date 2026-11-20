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
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
)

// The router refuses to start without an auth gate, and the gate runs
// before any handler (and before X-User-Email is read).
func TestRouterRequiresAndAppliesAuth(t *testing.T) {
	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewRouter without Deps.Auth: want a panic")
			}
		}()
		NewRouter(Deps{Pools: &db.Pools{}})
	}()

	deny := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	}
	h := NewRouter(Deps{Pools: &db.Pools{}, Auth: deny})
	r := httptest.NewRequest(http.MethodPut, "/role-settings/travel/roles/1/privileges", nil)
	r.Header.Set("X-User-Email", "anyone@wso2.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
