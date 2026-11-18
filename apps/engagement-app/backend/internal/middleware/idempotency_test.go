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
	"strconv"
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

// The query is part of the request: the same key on another query string
// is a different request (422), never a fresh run or a replay.
func TestIdempotency_QueryIsPartOfTheRequest(t *testing.T) {
	var calls int32
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	send := func(target string) int {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		req.Header.Set(IdempotencyKeyHeader, "k1")
		req = req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: "a@wso2.com"}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := send("/payments?notify=true"); code != http.StatusCreated {
		t.Fatalf("first: %d", code)
	}
	if code := send("/payments?notify=true&retry=1"); code != http.StatusUnprocessableEntity {
		t.Errorf("same key, other query: %d, want 422", code)
	}
	if code := send("/payments?notify=true"); code != http.StatusCreated {
		t.Errorf("same key, same query: %d, want the replayed 201", code)
	}
	if calls != 1 {
		t.Errorf("handler ran %d times, want 1", calls)
	}
}

// A replay keeps the retry's own correlation ID, not the original's.
func TestIdempotency_ReplayKeepsItsCorrelationID(t *testing.T) {
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/engagements/E1")
		w.WriteHeader(http.StatusCreated)
	}))
	send := func(corrID string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rec.Header().Set(correlationIDHeader, corrID) // as CorrelationID sets it
		h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
		return rec
	}
	send("first")
	rec := send("second")
	if got := rec.Header().Get(correlationIDHeader); got != "second" {
		t.Errorf("replayed correlation ID = %q, want second", got)
	}
	if rec.Header().Get("Idempotent-Replayed") != "true" || rec.Header().Get("Location") != "/engagements/E1" {
		t.Errorf("replay headers = %v", rec.Header())
	}
}

// One caller can hold only so many live keys; another caller is unaffected,
// and a key frees up once it expires.
func TestIdempotency_CapsKeysPerCaller(t *testing.T) {
	s := NewIdempotency()
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	send := func(key, email string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, idemRequest(key, email))
		return rec.Code
	}
	for i := range maxKeysPerCaller {
		if code := send(strconv.Itoa(i), "a@wso2.com"); code != http.StatusCreated {
			t.Fatalf("key %d: %d", i, code)
		}
	}
	if code := send("one-more", "a@wso2.com"); code != http.StatusTooManyRequests {
		t.Errorf("over the cap: %d, want 429", code)
	}
	// A key already held still replays.
	if code := send("0", "a@wso2.com"); code != http.StatusCreated {
		t.Errorf("replay at the cap: %d", code)
	}
	if code := send("one-more", "b@wso2.com"); code != http.StatusCreated {
		t.Errorf("another caller: %d", code)
	}
	now = now.Add(idempotencyTTL + sweepInterval)
	if code := send("one-more", "a@wso2.com"); code != http.StatusCreated {
		t.Errorf("after expiry: %d", code)
	}
}

// A response too large to keep is not remembered: a retry runs again.
func TestIdempotency_LargeResponseIsNotKept(t *testing.T) {
	var calls int32
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write(make([]byte, maxStoredBody+1))
	}))
	for range 2 {
		h.ServeHTTP(httptest.NewRecorder(), idemRequest("k1", "a@wso2.com"))
	}
	if calls != 2 {
		t.Errorf("handler ran %d times, want 2", calls)
	}
}

// A different body is 422 even while the first request is still running.
func TestIdempotency_DifferentBodyIs422WhileInFlight(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
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
	done := make(chan struct{})
	go func() { defer close(done); send(`{"a":1}`) }()
	<-started
	if code := send(`{"a":2}`); code != http.StatusUnprocessableEntity {
		t.Errorf("different body in flight: %d, want 422", code)
	}
	if code := send(`{"a":1}`); code != http.StatusConflict {
		t.Errorf("same body in flight: %d, want 409", code)
	}
	close(release)
	<-done
}

// A replay keeps the headers the current request already carries, such as
// CORS for its own origin.
func TestIdempotency_ReplayKeepsCurrentCORS(t *testing.T) {
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	send := func(origin string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rec.Header().Set("Access-Control-Allow-Origin", origin) // as CORS sets it
		h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
		return rec
	}
	send("https://a.example")
	rec := send("https://b.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://b.example" || rec.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("replay: origin %q, headers %v", got, rec.Header())
	}
}

// At the cap, the caller's own expired keys are reclaimed before refusing,
// even when the periodic sweep has not run yet.
func TestIdempotency_CapReclaimsExpiredKeys(t *testing.T) {
	s := NewIdempotency()
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	for i := range maxKeysPerCaller {
		h.ServeHTTP(httptest.NewRecorder(), idemRequest(strconv.Itoa(i), "a@wso2.com"))
	}
	// Expired, but less than a sweep interval since the last sweep.
	now = now.Add(idempotencyTTL + time.Second)
	s.lastSweep = now
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("fresh", "a@wso2.com"))
	if rec.Code != http.StatusCreated {
		t.Errorf("fresh key after expiry: %d, want 201", rec.Code)
	}
}

// A replay answers the retry's own origin: CORS headers of the original
// response are not replayed.
func TestIdempotency_ReplayDoesNotCarryCORS(t *testing.T) {
	h := NewIdempotency().Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	first := httptest.NewRecorder()
	first.Header().Set("Access-Control-Allow-Origin", "https://one.wso2.com") // as CORS sets it
	first.Header().Set("Access-Control-Expose-Headers", "X-Correlation-ID")
	h.ServeHTTP(first, idemRequest("k1", "a@wso2.com"))
	retry := httptest.NewRecorder()
	h.ServeHTTP(retry, idemRequest("k1", "a@wso2.com"))
	if retry.Header().Get("Idempotent-Replayed") != "true" || retry.Header().Get("Access-Control-Allow-Origin") != "" ||
		retry.Header().Get("Access-Control-Expose-Headers") != "" {
		t.Errorf("replay headers = %v", retry.Header())
	}
}

// The replay window starts when the response is ready: a push that ran for
// most of the TTL is still replayed shortly after it finished.
func TestIdempotency_TTLStartsWhenTheResponseIsReady(t *testing.T) {
	s := NewIdempotency()
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	var calls int32
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		now = now.Add(idempotencyTTL - time.Minute) // a long push
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), idemRequest("k1", "a@wso2.com"))
	now = now.Add(2 * time.Minute) // past the TTL from the start, not from the end
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
	if calls != 1 || rec.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("calls = %d, replayed = %q", calls, rec.Header().Get("Idempotent-Replayed"))
	}
}

// A key still in flight never expires, even past the TTL: a retry gets 409,
// not a second run.
func TestIdempotency_InFlightKeyDoesNotExpire(t *testing.T) {
	s := NewIdempotency()
	now := time.Unix(0, 0)
	var mu sync.Mutex
	s.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	release, started := make(chan struct{}), make(chan struct{})
	h := s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	done := make(chan struct{})
	go func() { h.ServeHTTP(httptest.NewRecorder(), idemRequest("k1", "a@wso2.com")); close(done) }()
	<-started
	mu.Lock()
	now = now.Add(idempotencyTTL + sweepInterval)
	mu.Unlock()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, idemRequest("k1", "a@wso2.com"))
	close(release)
	<-done
	if rec.Code != http.StatusConflict {
		t.Errorf("retry while in flight past the TTL: %d, want 409", rec.Code)
	}
}
