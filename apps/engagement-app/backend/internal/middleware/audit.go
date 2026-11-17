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
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// AuditRecorder records an audit event. engagemententity.Client implements it.
type AuditRecorder interface {
	RecordAuditEvent(ctx context.Context, e domain.AuditEvent) error
}

const auditTimeout = 5 * time.Second

// auditsInFlight counts the background audit writes so shutdown can wait for
// them (WaitAudits).
var auditsInFlight sync.WaitGroup

// WaitAudits waits for the background audit writes to finish, or for ctx to
// end. The server calls it on shutdown, after the listener has drained, so a
// restart does not drop the audit row of a response already sent.
func WaitAudits(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		auditsInFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Audit records the event that event builds for a request that succeeded
// (2xx), in the background: a failure is only logged and never holds up or
// fails the response.
func Audit(rec AuditRecorder, event func(r *http.Request) (domain.AuditEvent, bool)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			if rw.status < 200 || rw.status >= 300 {
				return
			}
			e, ok := event(r)
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), auditTimeout)
			auditsInFlight.Add(1)
			go func() {
				defer auditsInFlight.Done()
				defer cancel()
				// Outside Recovery: a panic here would end the process.
				defer func() {
					if p := recover(); p != nil {
						slog.ErrorContext(ctx, "audit event panicked", "action", e.Action, "entityType", e.EntityType,
							"entityId", e.EntityID, "panic", p, "stack", string(debug.Stack()))
					}
				}()
				if err := rec.RecordAuditEvent(ctx, e); err != nil {
					slog.ErrorContext(ctx, "audit event not recorded", "action", e.Action, "entityType", e.EntityType,
						"entityId", e.EntityID, "err", err)
				}
			}()
		})
	}
}
