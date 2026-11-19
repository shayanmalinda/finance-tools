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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardClient_PathsAndDecoding(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch r.URL.Path {
		case "/dashboard/allocations-today":
			_, _ = w.Write([]byte(`{"byEngagementType":[{"engagementTypeId":1,"engagementType":"Internal Engagement","count":4}],"paidByAllocationType":[],"nonPaidByAllocationType":[],"consultants":3}`))
		case "/dashboard/my-work":
			_, _ = w.Write([]byte(`{"allocations":[],"teamAllocation":null,"trips":[],"tripCount":0}`))
		case "/dashboard/queues":
			_, _ = w.Write([]byte(`{"pendingApproval":{"5":2},"clearance":{"Ready for Clearance":1},"invoicesFailed":3}`))
		case "/dashboard/travel":
			_, _ = w.Write([]byte(`{"from":"2025-12-01","to":"2026-12-01","cost":[{"source":"card","month":"2026-10","costType":"","currency":"USD","amount":48.2}]}`))
		case "/dashboard/engagements":
			_, _ = w.Write([]byte(`{"startingSoon":[],"startingSoonTotal":0,"trend":{"byMonth":[],"byBusinessUnit":[]}}`))
		case "/dashboard/agency":
			_, _ = w.Write([]byte(`{"inProgress":2,"upcoming":[],"invoicesByStatus":[],"tripsByMonth":[],"perDiemsOnOpenTrips":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	ctx := context.Background()

	at, err := c.DashboardAllocationsToday(ctx, "2026-10-01", 4)
	if err != nil || at.Consultants != 3 || at.ByEngagementType[0].Count != 4 {
		t.Fatalf("DashboardAllocationsToday = %+v, %v", at, err)
	}
	if _, err := c.DashboardAllocationsToday(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DashboardMyWork(ctx, DashboardMyWorkFilter{Email: "jane@wso2.com", Today: "2026-11-05", Trips: true, TripLimit: 3}); err != nil {
		t.Fatal(err)
	}
	q, err := c.DashboardQueues(ctx, DashboardQueuesFilter{PendingApprovalTypes: []int{1, 5}, ClearanceTypes: []int{3}, Invoices: true})
	if err != nil || q.PendingApproval[5] != 2 || q.Clearance["Ready for Clearance"] != 1 || q.InvoicesFailed != 3 {
		t.Fatalf("DashboardQueues = %+v, %v", q, err)
	}
	tr, err := c.DashboardTravel(ctx, DashboardTravelFilter{Today: "2026-11-05", Trips: true, CostCards: true})
	if err != nil || tr.Trips != nil || len(tr.Cost) != 1 || tr.Cost[0].Source != DashboardCostCard {
		t.Fatalf("DashboardTravel = %+v, %v", tr, err)
	}
	e, err := c.DashboardEngagements(ctx, DashboardEngagementsFilter{EngagementTypes: []int{1, 5}, Bench: true, Trend: true})
	if err != nil || e.Trend == nil {
		t.Fatalf("DashboardEngagements = %+v, %v", e, err)
	}
	a, err := c.DashboardAgency(ctx, 3, "2026-11-05")
	if err != nil || a.InProgress != 2 {
		t.Fatalf("DashboardAgency = %+v, %v", a, err)
	}

	want := []string{
		"/dashboard/allocations-today?businessUnitId=4&date=2026-10-01",
		"/dashboard/allocations-today",
		"/dashboard/my-work?email=jane%40wso2.com&today=2026-11-05&tripLimit=3&trips=true",
		"/dashboard/queues?clearanceType=3&invoices=true&pendingApprovalType=1&pendingApprovalType=5",
		"/dashboard/travel?costCards=true&today=2026-11-05&trips=true",
		"/dashboard/engagements?bench=true&engagementType=1&engagementType=5&trend=true",
		"/dashboard/agency?agencyId=3&today=2026-11-05",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, paths[i], want[i])
		}
	}
}
