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
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return &buf
}

// A decoded newline in the path cannot start a forged log line.
func TestLogger_QuotesThePath(t *testing.T) {
	buf := captureLog(t)
	h := Logger(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/a%0Astatus=200%20forged", nil))
	if out := strings.TrimSuffix(buf.String(), "\n"); strings.Contains(out, "\n") || !strings.Contains(out, `"/a\nstatus=200 forged"`) {
		t.Errorf("log = %q", buf.String())
	}
}

func TestCorrelationID_ReplacesUnsafeIDs(t *testing.T) {
	for id, keep := range map[string]bool{
		"abc-123_X.y:z":          true,
		"":                       false,
		"a b":                    false,
		"a\nforged=1":            false,
		strings.Repeat("a", 129): false,
		strings.Repeat("a", 128): true,
	} {
		var got string
		h := CorrelationID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = CorrelationIDFromContext(r.Context())
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Correlation-ID", id)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if (got == id) != keep || got == "" || rec.Header().Get("X-Correlation-ID") != got {
			t.Errorf("%q: got %q (keep %v)", id, got, keep)
		}
	}
}

// Recovery logs the stack and the route, and lets http.ErrAbortHandler
// through for net/http.
func TestRecovery_LogsStackAndRepanicsAbort(t *testing.T) {
	buf := captureLog(t)
	h := Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/engagements", nil))
	if out := buf.String(); rec.Code != http.StatusInternalServerError || !strings.Contains(out, `method=POST path="/engagements"`) ||
		!strings.Contains(out, "goroutine ") {
		t.Errorf("%d, log = %s", rec.Code, out)
	}

	defer func() {
		if p := recover(); !errors.Is(asError(p), http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", p)
		}
	}()
	Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Error("http.ErrAbortHandler was swallowed")
}

func asError(p any) error {
	err, _ := p.(error)
	return err
}
