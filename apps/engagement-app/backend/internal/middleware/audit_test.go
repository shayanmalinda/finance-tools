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
	select {
	case s := <-got:
		t.Errorf("a failed request recorded %q", s)
	case <-time.After(100 * time.Millisecond):
	}
}
