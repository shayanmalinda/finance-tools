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

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

// travelRouteEntity serves the role table and the few travel calls the
// route tests reach.
type travelRouteEntity struct {
	engagemententity.Client
}

func (travelRouteEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (travelRouteEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if app == domain.AppAllocations {
		return map[string][]int{"wso2-employees": {1}}, nil
	}
	return map[string][]int{
		"wso2-employees":          {19, 20, 21, 22, 23, 24, 36, 37, 38, 47, 48, 49},
		"app-travel-manager-po":   {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 25, 26, 27, 28, 30, 32, 41, 42, 45, 46},
		"app-travel-user-finance": {2, 3, 8, 28, 30, 35, 41, 42, 45},
	}, nil
}

func (travelRouteEntity) TravelAgency(context.Context, string) (*domain.TravelAgency, error) {
	return nil, nil
}

func (travelRouteEntity) SearchTravelRequests(context.Context, engagemententity.TravelRequestSearch) (engagemententity.TravelRequestPage, error) {
	return engagemententity.TravelRequestPage{
		Items: []engagemententity.TravelRequestRecord{{JobNumber: "TRV-2026-10-1246", Status: "New", TravellerEmail: "jane@wso2.com"}},
		Total: 1, StatusCounts: map[string]int{"New": 1},
	}, nil
}

func (travelRouteEntity) GetTravelRequest(context.Context, string) (engagemententity.TravelRequestRecord, error) {
	return engagemententity.TravelRequestRecord{JobNumber: "TRV-2026-10-1246", Status: "New", TravellerEmail: "jane@wso2.com"}, nil
}

func newTravelRouter(groups ...string) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "jane@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   travelRouteEntity{},
	})
}

func travelDo(h http.Handler, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("X-Jwt-Assertion", "valid")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func travelMultipart(t *testing.T, fields map[string]string, image []byte) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if image != nil {
		fw, err := mw.CreateFormFile("image", "visa.png")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(image)
	}
	_ = mw.Close()
	return mw.FormDataContentType(), &buf
}

func TestTravelRoutes_Gates(t *testing.T) {
	employee := newTravelRouter("wso2-employees")
	team := newTravelRouter("app-travel-manager-po")
	finance := newTravelRouter("app-travel-user-finance")
	tests := []struct {
		name   string
		h      http.Handler
		method string
		path   string
		want   int
	}{
		{"employee cannot list all requests", employee, http.MethodGet, "/travel-requests", http.StatusForbidden},
		{"team lists requests", team, http.MethodGet, "/travel-requests?segment=new", http.StatusOK},
		{"finance has no New segment", finance, http.MethodGet, "/travel-requests?segment=new", http.StatusForbidden},
		{"employee lists own requests", employee, http.MethodGet, "/me/travel-requests", http.StatusOK},
		{"traveller opens own request", employee, http.MethodGet, "/travel-requests/TRV-2026-10-1246", http.StatusOK},
		{"finance cannot update", finance, http.MethodPatch, "/travel-requests/TRV-2026-10-1246", http.StatusForbidden},
		{"employee cannot list visas", employee, http.MethodGet, "/visas", http.StatusForbidden},
		{"finance cannot open a visa", finance, http.MethodGet, "/visas/1", http.StatusForbidden},
		{"employee cannot list documents", employee, http.MethodGet, "/travel-documents", http.StatusForbidden},
		{"finance cannot see visa options", finance, http.MethodGet, "/travel-requests/TRV-2026-10-1246/visa-options", http.StatusForbidden},
		{"bad search date", team, http.MethodGet, "/travel-requests?startFrom=05-10-2026", http.StatusBadRequest},
		{"bad visa id", team, http.MethodGet, "/visas/abc", http.StatusBadRequest},
		{"bad document id", team, http.MethodGet, "/travel-documents/0/image", http.StatusBadRequest},
		{"bad limit", team, http.MethodGet, "/travel-documents?limit=0", http.StatusBadRequest},
		{"bad job number", team, http.MethodGet, "/travel-requests/E07201", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := travelDo(tc.h, tc.method, tc.path, "application/json", strings.NewReader(`{}`)); rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestTravelRoutes_ListBody(t *testing.T) {
	rec := travelDo(newTravelRouter("app-travel-manager-po"), http.MethodGet, "/travel-requests", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	var page struct {
		Items []struct {
			JobNumber string `json:"jobNumber"`
			Actions   struct {
				Assign bool `json:"assign"`
			} `json:"actions"`
		} `json:"items"`
		Total         int            `json:"total"`
		SegmentCounts map[string]int `json:"segmentCounts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || !page.Items[0].Actions.Assign || page.SegmentCounts["new"] != 1 {
		t.Errorf("page = %+v", page)
	}
}

func TestTravelRoutes_Uploads(t *testing.T) {
	team := newTravelRouter("app-travel-manager-po")
	employee := newTravelRouter("wso2-employees")

	// Over 5 MB: 413.
	ct, body := travelMultipart(t, map[string]string{"country": "GB"}, make([]byte, filestore.MaxImageBytes+10))
	if rec := travelDo(team, http.MethodPost, "/visas", ct, body); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("large: status %d %s", rec.Code, rec.Body)
	}
	// Not multipart: 400.
	if rec := travelDo(team, http.MethodPost, "/visas", "application/json", strings.NewReader(`{}`)); rec.Code != http.StatusBadRequest {
		t.Errorf("json body: status %d", rec.Code)
	}
	// Missing fields: 400 with the legacy message.
	ct, body = travelMultipart(t, map[string]string{"travellerEmail": "jane@wso2.com"}, nil)
	rec := travelDo(employee, http.MethodPost, "/me/visas", ct, body)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Please check the form again") {
		t.Errorf("missing fields: status %d %s", rec.Code, rec.Body)
	}
	// The employee may not add a visa for someone else (Travel 7).
	ct, body = travelMultipart(t, map[string]string{"country": "GB"}, nil)
	if rec := travelDo(employee, http.MethodPost, "/visas", ct, body); rec.Code != http.StatusForbidden {
		t.Errorf("employee add for others: status %d", rec.Code)
	}
	// A document without file storage configured: 503.
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 32)...)
	ct, body = travelMultipart(t, map[string]string{
		"documentType": "Other ID Document", "documentNumber": "ID-1", "country": "LK", "issuedDate": "2020-01-01",
	}, png)
	if rec := travelDo(employee, http.MethodPost, "/me/travel-documents", ct, body); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no file storage: status %d %s", rec.Code, rec.Body)
	}
}
