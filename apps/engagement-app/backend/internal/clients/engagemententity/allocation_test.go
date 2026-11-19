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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllocationClient_PathsMethodsAndBodies(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" "+strings.TrimSpace(string(body)))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/allocations/timeline"):
			_, _ = w.Write([]byte(`{"allocations":[{"id":"A1","email":"a@wso2.com","startDate":"2026-11-01","endDate":"2026-11-02"}]}`))
		case strings.HasPrefix(r.URL.Path, "/consultant-allocations"):
			_, _ = w.Write([]byte(`{"allocations":[],"employees":[],"defaults":[],"codes":[]}`))
		case strings.HasPrefix(r.URL.Path, "/allocations/visas"):
			_, _ = w.Write([]byte(`{"visas":[{"visaType":"Business"}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	ctx := context.Background()

	rows, err := c.AllocationTimeline(ctx, AllocationTimelineRequest{Emails: []string{"a@wso2.com"}, StartDate: "2026-11-01", EndDate: "2026-11-30"})
	if err != nil || len(rows) != 1 || rows[0].ID != "A1" {
		t.Fatalf("timeline = %+v, %v", rows, err)
	}
	status := "Confirmed"
	if _, err := c.UpdateAllocation(ctx, "A 1", AllocationUpdateRequest{ClearanceStatus: &status, UpdatedBy: "m@wso2.com"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteAllocation(ctx, "A1"); err != nil {
		t.Fatal(err)
	}
	if v, err := c.AllocationVisas(ctx, "a+b@wso2.com", "2026-11-01", "2026-11-02"); err != nil || len(v) != 1 {
		t.Fatalf("visas = %v, %v", v, err)
	}
	if _, err := c.ConsultantAllocations(ctx, ConsultantAllocationFilter{StartDate: "2026-11-01", EndDate: "2026-11-30",
		AllocationTypeIDs: []int{3, 4}, BusinessUnit: "PARTNER"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`POST /allocations/timeline {"emails":["a@wso2.com"],"startDate":"2026-11-01","endDate":"2026-11-30","allocationTypeIds":null}`,
		`PATCH /allocations/A%201 {"clearanceStatus":"Confirmed","updatedBy":"m@wso2.com"}`,
		`DELETE /allocations/A1 `,
		`GET /allocations/visas?email=a%2Bb%40wso2.com&endDate=2026-11-02&startDate=2026-11-01 `,
		`GET /consultant-allocations?allocationTypeId=3&allocationTypeId=4&businessUnit=PARTNER&endDate=2026-11-30&startDate=2026-11-01 `,
	}
	for i, w := range want {
		if i >= len(seen) || strings.TrimSpace(seen[i]) != strings.TrimSpace(w) {
			got := ""
			if i < len(seen) {
				got = seen[i]
			}
			t.Errorf("call %d = %q, want %q", i, got, w)
		}
	}
}
