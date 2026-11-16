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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func TestEngagementAPI_PathsMethodsAndBodies(t *testing.T) {
	type call struct{ method, uri, body string }
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, call{r.Method, r.URL.RequestURI(), string(b)})
		switch r.URL.Path {
		case "/engagements/E%201/roster", "/engagements/E 1/roster":
			_, _ = w.Write([]byte(`{"allocations":[{"id":"A1","email":"a@wso2.com","travelRequests":[]}]}`))
		case "/engagements/E1":
			_, _ = w.Write([]byte(`{"engagement":{"id":"E1"},"cancelledAllocations":[]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	ctx := context.Background()

	if _, err := c.EngagementFacets(ctx, []int{1, 5}); err != nil {
		t.Fatal(err)
	}
	allocs, err := c.EngagementRoster(ctx, "E 1", true)
	if err != nil || len(allocs) != 1 {
		t.Fatalf("roster %v err %v", allocs, err)
	}
	status := "Cancelled"
	res, err := c.EngagementUpdate(ctx, "E1", EngagementUpdateInput{EngagementStatus: &status, UpdatedBy: "me"})
	if err != nil || res.Engagement.ID != "E1" {
		t.Fatalf("update %+v err %v", res, err)
	}
	if _, err := c.RoleReplacePrivileges(ctx, domain.AppTravel, 4, nil, "me"); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{"GET", "/engagements/facets?engagementTypeId=1&engagementTypeId=5", ""},
		{"GET", "/engagements/E%201/roster?activeOnly=true", ""},
		{"PATCH", "/engagements/E1", `{"engagementStatus":"Cancelled","updatedBy":"me"}`},
		{"PUT", "/role-settings/travel/roles/4/privileges", `{"privilegeIds":[],"updatedBy":"me"}`},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v", calls)
	}
	for i, w := range want {
		got := calls[i]
		if got.method != w.method || got.uri != w.uri {
			t.Errorf("call %d = %s %s, want %s %s", i, got.method, got.uri, w.method, w.uri)
		}
		if w.body != "" {
			var a, b any
			_ = json.Unmarshal([]byte(got.body), &a)
			_ = json.Unmarshal([]byte(w.body), &b)
			ja, _ := json.Marshal(a)
			jb, _ := json.Marshal(b)
			if string(ja) != string(jb) {
				t.Errorf("call %d body = %s, want %s", i, got.body, w.body)
			}
		}
	}
}
