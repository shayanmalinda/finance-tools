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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestDashboardRoutes_Validation(t *testing.T) {
	h, _, _ := newTestRouter(t)
	for _, target := range []string{
		"/dashboard/allocations-today?date=yesterday",
		"/dashboard/allocations-today?businessUnitId=abc",
		"/dashboard/my-work",
		"/dashboard/my-work?email=jane@example.com&trips=maybe",
		"/dashboard/queues?pendingApprovalType=x",
		"/dashboard/travel?trips=1x",
		"/dashboard/engagements?engagementType=-1",
		"/dashboard/agency",
		"/dashboard/agency?agencyId=0",
		"/dashboard/cost-lines",
		"/dashboard/cost-lines?from=2026-01-01",
		"/dashboard/cost-lines?from=2026-01-01&to=2026-02-01&limit=0",
		"/dashboard/cost-lines?from=2026-01-01&to=2026-02-01&source=cash",
		"/dashboard/cost-lines?from=2026-03-01&to=2026-02-01",
		"/dashboard/travel-team?today=yesterday",
		"/dashboard/manager?engagementType=x",
		"/dashboard/account-manager",
		"/dashboard/events?today=x",
		"/dashboard/my-visas?email=nobody",
	} {
		if rec := serve(h, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", target, rec.Code, rec.Body)
		}
	}
}

func TestDashboardRoutes_AllocationsTodayReadsAllocationDB(t *testing.T) {
	h, allocMock, travelMock := newTestRouter(t)
	allocMock.ExpectQuery("GROUP BY engagement_type.id").WithArgs("2026-10-01", "2026-10-01", 3).
		WillReturnRows(sqlmock.NewRows([]string{"c", "n", "id"}).AddRow(2, "Marketing Event", 5))
	allocMock.ExpectQuery("engagement_type.name IN").
		WillReturnRows(sqlmock.NewRows([]string{"c", "n", "color"}))
	allocMock.ExpectQuery("engagement_type.name NOT IN").
		WillReturnRows(sqlmock.NewRows([]string{"c", "n", "color"}).AddRow(2, "Event", "#fff"))
	allocMock.ExpectQuery("count\\(DISTINCT allocation.email\\)").
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(2))

	rec := serve(h, http.MethodGet, "/dashboard/allocations-today?date=2026-10-01&businessUnitId=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := `{"byEngagementType":[{"engagementTypeId":5,"engagementType":"Marketing Event","count":2}],` +
		`"paidByAllocationType":[],"nonPaidByAllocationType":[{"allocationType":"Event","color":"#fff","count":2}],"consultants":2}`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("body = %s\nwant   %s", rec.Body, want)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDashboardRoutes_QueuesReadOnlyWhatIsAsked(t *testing.T) {
	h, allocMock, travelMock := newTestRouter(t)
	allocMock.ExpectQuery("Pending Approval").WithArgs(1, 5).
		WillReturnRows(sqlmock.NewRows([]string{"t", "n"}).AddRow(5, 4))
	travelMock.ExpectQuery("FROM perdiem_payment").
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow(2, 1))

	rec := serve(h, http.MethodGet, "/dashboard/queues?pendingApprovalType=1&pendingApprovalType=5&payments=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		PendingApproval map[string]int `json:"pendingApproval"`
		PaymentsToPush  int            `json:"paymentsToPush"`
		PaymentsFailed  int            `json:"paymentsFailed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.PendingApproval["5"] != 4 || body.PaymentsToPush != 2 || body.PaymentsFailed != 1 {
		t.Errorf("body = %+v", body)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// The test router has no expense_app pool: card spend is a 503.
func TestDashboardRoutes_CardSpendWithoutExpensePoolIs503(t *testing.T) {
	h, _, _ := newTestRouter(t)
	if rec := serve(h, http.MethodGet, "/dashboard/travel?costCards=true"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
