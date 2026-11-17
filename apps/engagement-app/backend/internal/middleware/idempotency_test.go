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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

func idemRequest(key, email string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/engagements", nil)
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	return req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: email}))
}

func TestIdempotency_ReplaysCompletedRequest(t *testing.T) {
	var calls int32
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"E1"}`))
	}))
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
		if rec.Code != http.StatusCreated || rec.Body.String() != `{"id":"E1"}` {
			t.Fatalf("attempt %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	if calls != 1 {
		t.Errorf("handler ran %d times, want 1", calls)
	}
	// Another caller with the same key is a different request.
	h.ServeHTTP(httptest.NewRecorder(), idemRequest("k1", "b@wso2.com"))
	// No key: always runs.
	h.ServeHTTP(httptest.NewRecorder(), idemRequest("", "a@wso2.com"))
	if calls != 3 {
		t.Errorf("handler ran %d times, want 3", calls)
	}
}

// The same key with a different body is refused; the handler still reads
// the full body.
func TestIdempotency_DifferentBodyIs422(t *testing.T) {
	var got []string
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, string(b))
		w.WriteHeader(http.StatusCreated)
	}))
	send := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/engagements", strings.NewReader(body))
		req.Header.Set(IdempotencyKeyHeader, "k1")
		req = req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: "a@wso2.com"}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := send(`{"a":1}`); c != http.StatusCreated {
		t.Fatalf("first = %d", c)
	}
	if c := send(`{"a":1}`); c != http.StatusCreated {
		t.Fatalf("replay = %d", c)
	}
	if c := send(`{"a":2}`); c != http.StatusUnprocessableEntity {
		t.Fatalf("different body = %d", c)
	}
	if len(got) != 1 || got[0] != `{"a":1}` {
		t.Errorf("handler saw %v", got)
	}
}

func TestIdempotency_InFlightIsConflict(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusCreated)
	}))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.ServeHTTP(httptest.NewRecorder(), idemRequest("k", "a@wso2.com"))
	}()
	<-started
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("k", "a@wso2.com"))
	close(release)
	wg.Wait()
	if rec.Code != http.StatusConflict {
		t.Errorf("second submit status = %d, want 409", rec.Code)
	}
}

func TestIdempotency_FailureIsNotRemembered(t *testing.T) {
	var calls int32
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	for i := 0; i < 2; i++ {
		h.ServeHTTP(httptest.NewRecorder(), idemRequest("k", "a@wso2.com"))
	}
	if calls != 2 {
		t.Errorf("handler ran %d times, want 2", calls)
	}
}

func TestIdempotency_Expires(t *testing.T) {
	s := NewIdempotency()
	now := time.Now()
	s.now = func() time.Time { return now }
	var calls int32
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { atomic.AddInt32(&calls, 1) }))
	h.ServeHTTP(httptest.NewRecorder(), idemRequest("k", "a@wso2.com"))
	now = now.Add(idempotencyTTL + time.Second)
	h.ServeHTTP(httptest.NewRecorder(), idemRequest("k", "a@wso2.com"))
	if calls != 2 {
		t.Errorf("handler ran %d times, want 2", calls)
	}
}

// A panicking handler releases its key: the retry runs instead of getting 409.
func TestIdempotency_PanicReleasesTheKey(t *testing.T) {
	var calls int32
	h := Recovery(NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			panic("boom")
		}
		w.WriteHeader(http.StatusCreated)
	})))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("first: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
	if rec.Code != http.StatusCreated || calls != 2 {
		t.Errorf("retry: %d after %d calls, want 201 after 2", rec.Code, calls)
	}
}
