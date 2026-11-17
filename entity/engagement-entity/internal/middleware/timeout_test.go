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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeout_LongRoutes(t *testing.T) {
	var left time.Duration
	var ok bool
	h := Timeout(30*time.Second, 5*time.Minute, func(r *http.Request) bool { return r.URL.Path == "/push" })(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			var d time.Time
			d, ok = r.Context().Deadline()
			left = time.Until(d)
		}))
	// Each deadline must exist and sit within a scheduling tolerance of its limit.
	const tolerance = 5 * time.Second
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/push", nil))
	if !ok || left <= 5*time.Minute-tolerance || left > 5*time.Minute {
		t.Errorf("push deadline set %v, in %v", ok, left)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/other", nil))
	if !ok || left <= 30*time.Second-tolerance || left > 30*time.Second {
		t.Errorf("other deadline set %v, in %v", ok, left)
	}
}
