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
)

// Anything that may write clears the dashboard cache; reads do not.
func TestInvalidateOnWrite(t *testing.T) {
	cleared := 0
	h := invalidateOnWrite(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // even a failed write may have changed data
	}), func() { cleared++ })
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, "/x", nil))
	}
	if cleared != 0 {
		t.Fatalf("reads cleared the cache %d times", cleared)
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, "/x", nil))
	}
	if cleared != 4 {
		t.Errorf("writes cleared the cache %d times, want 4", cleared)
	}
}
