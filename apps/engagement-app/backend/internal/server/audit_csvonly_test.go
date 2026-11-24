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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/middleware"
)

// With the report cap full, a dashboard view still loads and only a CSV download waits.
func TestCSVOnly_CapsDownloadsNotViews(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	hold, release := make(chan struct{}), make(chan struct{})
	blocking := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { close(hold); <-release })
	limit := middleware.Limit(1)
	// Take the only slot with a CSV download that does not finish yet.
	go csvOnly(limit(blocking), ok).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x?format=csv", nil))
	<-hold
	defer close(release)
	h := csvOnly(limit(ok), ok)
	for _, tc := range []struct {
		url  string
		want int
	}{
		{"/x", http.StatusOK},
		{"/x?format=json", http.StatusOK},
		{"/x?format=CSV", http.StatusServiceUnavailable},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.url, rec.Code, tc.want)
		}
	}
}
