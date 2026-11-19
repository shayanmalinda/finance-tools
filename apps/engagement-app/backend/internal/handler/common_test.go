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

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
)

func TestPagination(t *testing.T) {
	for q, ok := range map[string]bool{"": true, "limit=10&offset=20": true, "limit=0": false, "limit=501": false, "offset=-1": false, "limit=x": false} {
		r := httptest.NewRequest(http.MethodGet, "/x?"+q, nil)
		if _, _, err := pagination(r); (err == nil) != ok {
			t.Errorf("%q: err = %v", q, err)
		}
	}
}

func TestWriteCSVNeutralisesFormulas(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCSV(rec, "x.csv", []string{"a", "b"}, [][]string{{"=HYPERLINK(1)", "-12.5"}, {"a,b", `say "hi"`}})
	got := rec.Body.String()
	want := "a,b\n'=HYPERLINK(1),-12.5\n\"a,b\",\"say \"\"hi\"\"\"\n"
	if got != want {
		t.Errorf("csv = %q, want %q", got, want)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), `"x.csv"`) {
		t.Error("no filename")
	}
}

func TestQueryHelpers(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?s=a&s=+b+&s=&n=1&n=2&d=2026-11-05&bad=5", nil)
	if got := queryList(r, "s"); len(got) != 2 || got[1] != "b" {
		t.Errorf("queryList = %v", got)
	}
	if got, err := queryInts(r, "n"); err != nil || len(got) != 2 {
		t.Errorf("queryInts = %v %v", got, err)
	}
	if _, err := queryInts(httptest.NewRequest(http.MethodGet, "/x?n=a", nil), "n"); err == nil {
		t.Error("queryInts accepted a")
	}
	if d, err := queryDate(r, "d"); err != nil || d != "2026-11-05" {
		t.Errorf("queryDate = %q %v", d, err)
	}
	if _, err := queryDate(httptest.NewRequest(http.MethodGet, "/x?d=05/11/2026", nil), "d"); err == nil {
		t.Error("queryDate accepted 05/11/2026")
	}
}

// A canceled request answers non-2xx, so Idempotency and Audit do not take
// it for a success.
func TestWriteServiceError_CanceledIsNot2xx(t *testing.T) {
	rec := httptest.NewRecorder()
	writeServiceError(rec, httptest.NewRequest(http.MethodPost, "/x", nil), fmt.Errorf("load: %w", context.Canceled))
	if rec.Code != 499 || !strings.Contains(rec.Body.String(), `"code":499`) {
		t.Errorf("canceled: %d %s", rec.Code, rec.Body)
	}
}

// writeServiceError decides what the browser sees for every failure: an
// upstream 4xx passes its message through, but a dependency refusing our
// credentials, a dependency outage and an unknown error never echo upstream
// or internal detail, and never look like the user's permission problem.
func TestWriteServiceError_Mapping(t *testing.T) {
	const secret = "SELECT * FROM travel WHERE secret"
	tests := []struct {
		name     string
		err      error
		status   int
		wantText string
	}{
		{"validation", &apierror.ValidationError{Msg: "Bad date"}, 400, "Bad date"},
		{"unauthorized", &apierror.UnauthorizedError{Msg: "Sign in again"}, 401, "Sign in again"},
		{"forbidden", &apierror.ForbiddenError{Msg: "No access"}, 403, "No access"},
		{"not found", &apierror.NotFoundError{Msg: "No such trip"}, 404, "No such trip"},
		{"conflict", &apierror.ConflictError{Msg: "Already exists"}, 409, "Already exists"},
		{"upstream 4xx", &apierror.UpstreamError{Service: "entity", StatusCode: 422, Msg: "Invoice number taken"}, 422,
			"Invoice number taken"},
		{"upstream 401", &apierror.UpstreamError{Service: "entity", StatusCode: 401, Msg: secret}, 503,
			"A connected service refused the request"},
		{"upstream 403", &apierror.UpstreamError{Service: "entity", StatusCode: 403, Msg: secret}, 503,
			"A connected service refused the request"},
		{"upstream 5xx", &apierror.UpstreamError{Service: "entity", StatusCode: 500, Msg: secret}, 503,
			"Service temporarily unavailable"},
		{"unavailable", fmt.Errorf("wrap: %w", &apierror.ServiceUnavailableError{Msg: secret}), 503,
			"Service temporarily unavailable"},
		{"deadline", fmt.Errorf("load: %w", context.DeadlineExceeded), 504, "The request timed out"},
		{"unknown", errors.New(secret), 500, "Something went wrong"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeServiceError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), tc.err)
			body := rec.Body.String()
			if rec.Code != tc.status || !strings.Contains(body, tc.wantText) {
				t.Errorf("got %d %s, want %d containing %q", rec.Code, body, tc.status, tc.wantText)
			}
			if strings.Contains(body, secret) {
				t.Errorf("body echoes upstream or internal detail: %s", body)
			}
		})
	}
}

// An image download is never sniffed, cached or offered as a page.
func TestTravelWriteImage_Headers(t *testing.T) {
	rec := httptest.NewRecorder()
	travelWriteImage(rec, "application/octet-stream", []byte("<html>"))
	h := rec.Header()
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, no-store" ||
		h.Get("Content-Disposition") != "inline" || h.Get("Content-Type") != "application/octet-stream" ||
		h.Get("Content-Length") != "6" {
		t.Errorf("headers = %v", h)
	}
}
