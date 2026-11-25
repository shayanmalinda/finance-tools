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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

type fakeSearch struct {
	q     string
	limit int
}

func (f *fakeSearch) Search(_ context.Context, _ domain.Perms, q string, limit int) (domain.SearchResponse, error) {
	f.q, f.limit = q, limit
	if len(strings.TrimSpace(q)) < service.SearchMinQuery {
		return domain.SearchResponse{}, &apierror.ValidationError{Msg: "q must be at least 2 characters."}
	}
	return domain.SearchResponse{Query: q, Groups: []domain.SearchGroup{{Kind: "travelRequests", Total: 1,
		Items: []domain.SearchItem{{ID: "TRV-1", Title: "TRV-1"}}}}, Failed: []string{"engagements"}}, nil
}

func TestSearchHandler(t *testing.T) {
	for _, tc := range []struct {
		name      string
		url       string
		withPerms bool
		status    int
		limit     int
		body      string
	}{
		{"ok", "/search?q=2026-11", true, 200, 0, `{"query":"2026-11","groups":[{"kind":"travelRequests","total":1,"items":[{"id":"TRV-1","title":"TRV-1","subtitle":"","status":""}]}],"failed":["engagements"]}`},
		{"limit passed through", "/search?q=abc&limit=7", true, 200, 7, ""},
		{"limit above max reaches the service to clamp", "/search?q=abc&limit=99", true, 200, 99, ""},
		{"missing q", "/search", true, 400, 0, "at least 2"},
		{"short q", "/search?q=a", true, 400, 0, "at least 2"},
		{"bad limit", "/search?q=abc&limit=x", true, 400, 0, "limit must be"},
		{"zero limit", "/search?q=abc&limit=0", true, 400, 0, "limit must be"},
		{"no perms in context is refused", "/search?q=abc", false, 403, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSearch{}
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.withPerms {
				req = req.WithContext(reqcontext.WithPerms(req.Context(), domain.Perms{}))
			}
			rec := httptest.NewRecorder()
			NewSearchHandler(f).Search(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.status, rec.Body)
			}
			if tc.limit != 0 && f.limit != tc.limit {
				t.Errorf("limit = %d, want %d", f.limit, tc.limit)
			}
			if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
				t.Errorf("body = %s, want it to contain %s", rec.Body, tc.body)
			}
		})
	}
}
