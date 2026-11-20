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
	"sync"
	"testing"
	"time"
)

// With every slot busy, the next request is refused at once (503 and
// Retry-After), not queued; a freed slot is reused.
func TestLimit(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	h := Limit(2)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/reports/invoices", nil))
			codes[i] = w.Code
		}()
	}
	<-started
	<-started

	// The third must be answered at once, not wait for a slot.
	third := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/reports/invoices", nil))
		third <- w
	}()
	select {
	case w := <-third:
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != LimitRetryAfter {
			t.Errorf("third while two run: status %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
		}
	case <-started:
		t.Error("third request ran while two were running")
	case <-time.After(2 * time.Second):
		t.Error("third request waited for a slot instead of being refused")
	}

	close(release)
	wg.Wait()
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Errorf("running requests: %v", codes)
	}

	h2 := Limit(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	w0 := httptest.NewRecorder()
	h2.ServeHTTP(w0, httptest.NewRequest(http.MethodGet, "/", nil))
	if w0.Code != http.StatusOK {
		t.Errorf("n=0 is treated as 1: status %d", w0.Code)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/reports/invoices", nil))
	if w.Code != http.StatusOK {
		t.Errorf("after release: status %d", w.Code)
	}
}
