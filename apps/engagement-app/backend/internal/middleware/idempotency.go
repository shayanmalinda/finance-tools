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
)

type idemEntry struct {
	bodySum [32]byte
	done    bool
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

// Idempotency replays the first 2xx response for a repeated Idempotency-Key
// (per caller), answers 409 while it is in flight and 422 when the key comes
// back with a different body. Per replica.
type Idempotency struct {
	mu      sync.Mutex
	entries map[string]*idemEntry
	now     func() time.Time
}

// NewIdempotency returns an empty Idempotency store.
func NewIdempotency() *Idempotency {
	return &Idempotency{entries: map[string]*idemEntry{}, now: time.Now}
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
		sum := sha256.Sum256(head)

		now := s.now()
		s.mu.Lock()
		for k, e := range s.entries {
			if now.After(e.expires) {
				delete(s.entries, k)
			}
		}
		if e, ok := s.entries[id]; ok {
			// Snapshot under the lock: the original request fills the entry
			// under it too.
			done, bodySum, status, header, body := e.done, e.bodySum, e.status, e.header, e.body
			s.mu.Unlock()
			if !done {
				apierror.WriteJSON(w, http.StatusConflict, "This request is already being processed.")
				return
			}
			if bodySum != sum {
				apierror.WriteJSON(w, http.StatusUnprocessableEntity, "This Idempotency-Key was already used for a different request.")
				return
			}
			for k, v := range header {
				w.Header()[k] = v
			}
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}
		entry := &idemEntry{bodySum: sum, expires: now.Add(idempotencyTTL)}
		s.entries[id] = entry
		s.mu.Unlock()

		rec := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
		finished := false
		// Deferred so a panicking handler releases the key too: otherwise
		// every retry would get 409 until the entry expires.
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if finished && rec.status >= 200 && rec.status < 300 {
				entry.done = true
				entry.status = rec.status
				entry.header = w.Header().Clone()
				entry.body = rec.buf.Bytes()
			} else if s.entries[id] == entry {
				delete(s.entries, id)
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
