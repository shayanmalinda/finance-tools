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

// Package middleware provides composable HTTP middleware for the Engagement App Backend.
package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

const correlationIDHeader = "X-Correlation-ID"

// CorrelationID reads the X-Correlation-ID request header (sent by the
// caller) or generates a UUID v4 if it is absent or not a safe ID. The ID is stored in the
// request context and echoed in the response header for end-to-end tracing.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(correlationIDHeader)
		if !validCorrelationID(id) {
			id = newCorrelationID()
		}
		w.Header().Set(correlationIDHeader, id)
		next.ServeHTTP(w, r.WithContext(reqcontext.WithCorrelationID(r.Context(), id)))
	})
}

// maxCorrelationID caps a caller's correlation ID.
const maxCorrelationID = 128

// validCorrelationID reports whether a caller's ID is safe to log and echo:
// 1-128 letters, digits, '-', '_', '.' or ':'.
func validCorrelationID(id string) bool {
	if id == "" || len(id) > maxCorrelationID {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_.:", c)) {
			return false
		}
	}
	return true
}

// CorrelationIDFromContext returns the correlation ID stored in ctx, or "".
func CorrelationIDFromContext(ctx context.Context) string {
	return reqcontext.CorrelationID(ctx)
}

func newCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("correlationid: failed to read random bytes: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
