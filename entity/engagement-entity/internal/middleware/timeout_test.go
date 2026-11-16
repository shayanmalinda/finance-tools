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
	h := Timeout(30*time.Second, 5*time.Minute, func(r *http.Request) bool { return r.URL.Path == "/push" })(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			d, _ := r.Context().Deadline()
			left = time.Until(d)
		}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/push", nil))
	if left < 4*time.Minute {
		t.Errorf("push deadline in %v", left)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/other", nil))
	if left > 30*time.Second {
		t.Errorf("other deadline in %v", left)
	}
}
