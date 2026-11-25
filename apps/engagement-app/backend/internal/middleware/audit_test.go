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
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

type auditRecorderFunc func(ctx context.Context, e domain.AuditEvent) error

func (f auditRecorderFunc) RecordAuditEvent(ctx context.Context, e domain.AuditEvent) error {
	return f(ctx, e)
}

func TestAudit_RecordsSuccessOnlyAndNeverFailsTheResponse(t *testing.T) {
	got := make(chan string, 4)
	rec := auditRecorderFunc(func(ctx context.Context, e domain.AuditEvent) error {
		c, _ := reqcontext.CallerFrom(ctx)
		got <- c.Email + " " + e.Action + " " + e.EntityID
		return errors.New("entity down")
	})
	event := func(r *http.Request) (domain.AuditEvent, bool) {
		return domain.AuditEvent{Action: domain.AuditDownload, EntityType: "visa", EntityID: r.PathValue("id")}, true
	}
	mux := http.NewServeMux()
	mux.Handle("GET /visas/{id}", Audit(rec, event)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "404" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("image"))
	})))
	serve := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(reqcontext.WithCaller(req.Context(), reqcontext.Caller{Email: "jane@wso2.com"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	if w := serve("/visas/5"); w.Code != http.StatusOK || w.Body.String() != "image" {
		t.Errorf("download: %d %q", w.Code, w.Body)
	}
	select {
	case s := <-got:
		if s != "jane@wso2.com Download 5" {
			t.Errorf("event %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event recorded")
	}
	if w := serve("/visas/404"); w.Code != http.StatusNotFound {
		t.Errorf("missing: %d", w.Code)
	}
	// Wait for every background write to finish, then the channel must be
	// empty: no fixed sleep for an event that must not arrive.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitAudits(ctx); err != nil {
		t.Fatalf("WaitAudits: %v", err)
	}
	select {
	case s := <-got:
		t.Errorf("a failed request recorded %q", s)
	default:
	}
}

// A panicking recorder is recovered (the process lives on) and WaitAudits
// waits for the background writes.
func TestAudit_RecoversPanicsAndWaitAuditsDrains(t *testing.T) {
	release := make(chan struct{})
	rec := auditRecorderFunc(func(context.Context, domain.AuditEvent) error {
		<-release
		panic("recorder bug")
	})
	event := func(*http.Request) (domain.AuditEvent, bool) {
		return domain.AuditEvent{Action: domain.AuditView, EntityType: "visa"}, true
	}
	h := Audit(rec, event)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/visas", nil))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := WaitAudits(ctx); err == nil {
		t.Fatal("WaitAudits returned before the write finished")
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if err := WaitAudits(ctx2); err != nil {
		t.Errorf("WaitAudits = %v", err)
	}
}

// HEAD runs the GET handler but sends no body: nothing is recorded.
func TestAudit_SkipsHEAD(t *testing.T) {
	var calls atomic.Int32
	rec := auditRecorderFunc(func(context.Context, domain.AuditEvent) error { calls.Add(1); return nil })
	event := func(*http.Request) (domain.AuditEvent, bool) {
		return domain.AuditEvent{Action: domain.AuditExport}, true
	}
	h := Audit(rec, event)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("a,b")) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodHead, "/reports/invoices?format=csv", nil))
	if err := WaitAudits(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("HEAD recorded %d audit events", n)
	}
}

// The audit write is detached from the request: a request context that is
// already cancelled (client gone, request timeout) still records the event,
// and the write's own context carries a deadline.
func TestAudit_CancelledRequestContextStillRecords(t *testing.T) {
	type result struct {
		ctxErr      error
		hasDeadline bool
		email       string
	}
	got := make(chan result, 1)
	rec := auditRecorderFunc(func(ctx context.Context, e domain.AuditEvent) error {
		c, _ := reqcontext.CallerFrom(ctx)
		_, dl := ctx.Deadline()
		got <- result{ctxErr: ctx.Err(), hasDeadline: dl, email: c.Email}
		return nil
	})
	event := func(r *http.Request) (domain.AuditEvent, bool) {
		return domain.AuditEvent{Action: domain.AuditView, EntityType: "travel_cost"}, true
	}
	reqCtx, cancelReq := context.WithCancel(reqcontext.WithCaller(context.Background(), reqcontext.Caller{Email: "jane@wso2.com"}))
	h := Audit(rec, event)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancelReq() // the request ends before the audit is sent
		_, _ = w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(reqCtx))

	select {
	case r := <-got:
		if r.ctxErr != nil {
			t.Errorf("audit context was cancelled: %v", r.ctxErr)
		}
		if !r.hasDeadline {
			t.Error("audit context has no timeout")
		}
		if r.email != "jane@wso2.com" {
			t.Errorf("caller lost: %q", r.email)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event recorded after the request context was cancelled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = WaitAudits(ctx)
}
