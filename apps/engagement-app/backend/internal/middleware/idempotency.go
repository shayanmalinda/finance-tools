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
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// IdempotencyKeyHeader names a create or allocate request. One WSO2 sends a
// fresh key per form submission and the same key when it retries it.
const IdempotencyKeyHeader = "Idempotency-Key"

const (
	idempotencyTTL    = 10 * time.Minute
	maxIdempotencyKey = 128
	maxHashedBody     = 8 << 20
	// maxKeysPerCaller caps the live keys one caller holds, so one caller
	// sending many fresh keys can't grow the store without bound. One WSO2
	// sends one key per form submission.
	maxKeysPerCaller = 100
	// maxStoredBody caps a remembered response. A larger one is not kept
	// (a retry runs again); creates and allocations answer far less.
	maxStoredBody = 256 << 10
	// sweepInterval spaces out the scan for expired entries, so it doesn't
	// run under the lock on every keyed request.
	sweepInterval = time.Minute
)

type idemEntry struct {
	caller  string
	bodySum [32]byte
	done    bool
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

// Idempotency replays the first 2xx response for a repeated Idempotency-Key
// (per caller), answers 409 while it is in flight and 422 when the key comes
// back with a different body. Per replica: a retry that lands on another
// replica is not caught here.
type Idempotency struct {
	mu      sync.Mutex
	entries map[string]*idemEntry
	// perCaller indexes entries by caller, so the quota check looks only
	// at that caller's keys.
	perCaller map[string]map[string]*idemEntry
	lastSweep time.Time
	now       func() time.Time
}

// NewIdempotency returns an empty Idempotency store.
func NewIdempotency() *Idempotency {
	return &Idempotency{entries: map[string]*idemEntry{}, perCaller: map[string]map[string]*idemEntry{}, now: time.Now}
}

// add stores e under id. The caller holds s.mu.
func (s *Idempotency) add(id string, e *idemEntry) {
	s.entries[id] = e
	if s.perCaller[e.caller] == nil {
		s.perCaller[e.caller] = map[string]*idemEntry{}
	}
	s.perCaller[e.caller][id] = e
}

// drop removes the entry under id. The caller holds s.mu.
func (s *Idempotency) drop(id string, e *idemEntry) {
	delete(s.entries, id)
	if keys := s.perCaller[e.caller]; keys != nil {
		delete(keys, id)
		if len(keys) == 0 {
			delete(s.perCaller, e.caller)
		}
	}
}

// expired reports whether e can be dropped: a completed entry past its
// replay window. An entry still in flight stays until its handler exits.
func (e *idemEntry) expired(now time.Time) bool {
	return e.done && now.After(e.expires)
}

// Wrap applies the protection to next. It must run inside Auth.
func (s *Idempotency) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(IdempotencyKeyHeader))
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		if len(key) > maxIdempotencyKey {
			apierror.WriteJSON(w, http.StatusBadRequest, "Idempotency-Key is too long.")
			return
		}
		caller, _ := reqcontext.CallerFrom(r.Context())
		id := caller.Email + "\x00" + r.Method + " " + r.URL.Path + "\x00" + key
		head, err := io.ReadAll(io.LimitReader(r.Body, maxHashedBody))
		if err != nil {
			apierror.WriteJSON(w, http.StatusBadRequest, "Could not read the request body.")
			return
		}
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
		// The query is hashed with the body: the same key with another
		// query is a different request (422), never a fresh run or a replay.
		h := sha256.New()
		h.Write([]byte(r.URL.RawQuery))
		h.Write([]byte{0})
		h.Write(head)
		var sum [32]byte
		h.Sum(sum[:0])

		now := s.now()
		s.mu.Lock()
		if now.Sub(s.lastSweep) >= sweepInterval {
			s.lastSweep = now
			for k, e := range s.entries {
				if e.expired(now) {
					s.drop(k, e)
				}
			}
		}
		if e, ok := s.entries[id]; ok && e.expired(now) {
			s.drop(id, e)
		}
		if e, ok := s.entries[id]; ok {
			// Snapshot under the lock: the original request fills the entry
			// under it too.
			done, bodySum, status, header, body := e.done, e.bodySum, e.status, e.header, e.body
			s.mu.Unlock()
			if bodySum != sum {
				apierror.WriteJSON(w, http.StatusUnprocessableEntity, "This Idempotency-Key was already used for a different request.")
				return
			}
			if !done {
				apierror.WriteJSON(w, http.StatusConflict, "This request is already being processed.")
				return
			}
			// Headers this request already carries (CORS for its own origin,
			// its correlation ID) stay; the rest come from the original.
			for k, v := range header {
				if _, set := w.Header()[k]; !set {
					w.Header()[k] = v
				}
			}
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}
		if len(s.perCaller[caller.Email]) >= maxKeysPerCaller {
			// The periodic sweep may not have run yet: reclaim this
			// caller's expired keys before refusing.
			for k, e := range s.perCaller[caller.Email] {
				if e.expired(now) {
					s.drop(k, e)
				}
			}
		}
		if len(s.perCaller[caller.Email]) >= maxKeysPerCaller {
			s.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			apierror.WriteJSON(w, http.StatusTooManyRequests, "Too many requests. Please try again in a minute.")
			return
		}
		entry := &idemEntry{caller: caller.Email, bodySum: sum}
		s.add(id, entry)
		s.mu.Unlock()

		rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
		finished := false
		// Deferred so a panicking handler releases the key too: otherwise
		// every retry would get 409, as an entry in flight never expires.
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if finished && rec.status >= 200 && rec.status < 300 && rec.buf.Len() <= maxStoredBody {
				entry.done = true
				// The replay window starts when the response is ready, so a
				// long push is replayable for the full TTL.
				entry.expires = s.now().Add(idempotencyTTL)
				entry.status = rec.status
				entry.header = w.Header().Clone()
				// The correlation ID names the original request and CORS
				// answers the original origin; a replay gets its own.
				entry.header.Del(correlationIDHeader)
				for k := range entry.header {
					if strings.HasPrefix(k, "Access-Control-") {
						entry.header.Del(k)
					}
				}
				entry.body = rec.buf.Bytes()
			} else if s.entries[id] == entry {
				s.drop(id, entry)
			}
		}()
		next.ServeHTTP(rec, r)
		finished = true
	})
}

type recordingWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	buf         bytes.Buffer
}

func (r *recordingWriter) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recordingWriter) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}
