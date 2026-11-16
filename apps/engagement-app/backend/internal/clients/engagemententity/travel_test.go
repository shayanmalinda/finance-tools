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

package engagemententity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
)

func TestTravelClient_PathsMethodsAndBodies(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []string
		body  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPost && r.URL.Path == "/travel-requests/search" {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
		}
		mu.Unlock()
		switch r.URL.Path {
		case "/travel-requests/search":
			_, _ = w.Write([]byte(`{"items":[{"jobNumber":"TRV-2026-10-1246","status":"New","visaDetails":{"id":301}}],"total":1,"statusCounts":{"New":1}}`))
		case "/travel-requests/TRV-2026-10-0000":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"travel request TRV-2026-10-0000 not found"}`))
		case "/travel-email-audit-logs":
			w.WriteHeader(http.StatusNoContent)
		case "/travel-agencies":
			_, _ = w.Write([]byte(`{"items":[{"id":2,"name":"Acorn Travels","status":"Active"}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	ctx := context.Background()

	agency := 2
	page, err := c.SearchTravelRequests(ctx, TravelRequestSearch{Statuses: []string{"New"}, TravelAgentID: &agency, Limit: 10})
	if err != nil || page.Total != 1 || page.Items[0].Visa.ID != 301 || page.StatusCounts["New"] != 1 {
		t.Fatalf("search = %+v, %v", page, err)
	}
	if body["travelAgentId"] != float64(2) || body["limit"] != float64(10) {
		t.Errorf("search body = %v", body)
	}
	_, err = c.GetTravelRequest(ctx, "TRV-2026-10-0000")
	var up *apierror.UpstreamError
	if !errors.As(err, &up) || up.StatusCode != 404 {
		t.Errorf("missing: %v", err)
	}
	status := "On Hold"
	_, _ = c.UpdateTravelRequest(ctx, "TRV-2026-10-1246", TravelRequestWrite{Status: &status, UpdatedBy: "a@wso2.com"})
	_, _ = c.TravelActivePerDiems(ctx, "TRV-2026-10-1246")
	if agents, err := c.TravelAgencies(ctx, "Active"); err != nil || agents[0].Name != "Acorn Travels" {
		t.Errorf("agencies = %+v %v", agents, err)
	}
	if err := c.InsertTravelEmailAudit(ctx, TravelEmailAudit{Subject: "s", TemplateID: "noContent"}); err != nil {
		t.Errorf("audit: %v", err)
	}
	_, _ = c.SearchVisas(ctx, TravelVisaFilter{Emails: []string{"a@wso2.com"}, Status: "Active", CoversFrom: "2026-10-06", Limit: 5})
	_, _ = c.UpdateVisa(ctx, 7, TravelVisaUpdate{})
	_, _ = c.SearchTravelDocuments(ctx, TravelDocumentFilter{DocumentTypes: []string{"Passport"}, Offset: 10})
	_, _ = c.UpdateTravelDocument(ctx, 8, TravelDocumentUpdate{})
	_, _ = c.CreateVisa(ctx, TravelVisaCreate{})
	_, _ = c.GetTravelDocument(ctx, 9)

	want := []string{
		"POST /travel-requests/search",
		"GET /travel-requests/TRV-2026-10-0000",
		"PATCH /travel-requests/TRV-2026-10-1246",
		"GET /travel-requests/TRV-2026-10-1246/active-per-diems",
		"GET /travel-agencies?status=Active",
		"POST /travel-email-audit-logs",
		"GET /visas?coversFrom=2026-10-06&email=a%40wso2.com&limit=5&status=Active",
		"PUT /visas/7",
		"GET /travel-documents?documentType=Passport&offset=10",
		"PUT /travel-documents/8",
		"POST /visas",
		"GET /travel-documents/9",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}
