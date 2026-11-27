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
package repository

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func budgetBulkRequest(b *domain.AllocationTravelBudget) domain.AllocationBulkRequest {
	return domain.AllocationBulkRequest{CreatedBy: "mgr@example.com", TravelBudget: b,
		NewAllocations: []domain.AllocationInput{{Email: "jane@example.com", AllocationTypeID: 12, Status: "Active", ClearanceStatus: confirmed(),
			StartDate: "2027-02-01", EndDate: "2027-02-05", EngagementID: "E1", EngagementCode: "NWB",
			IsTravelRequired: "Yes", IsRecurring: "No"}}}
}

// expectBulkToTravelInsert expects everything up to the travel row and its job number.
func expectBulkToTravelInsert(m allocationMocks) {
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*)")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A1"))
	m.alloc.ExpectQuery(re("SELECT aT.allocation_type")).WithArgs("A1").WillReturnRows(
		sqlmock.NewRows([]string{"t", "c", "l", "cn", "p"}).AddRow("T", "DE", nil, "Acme", nil))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-2027-02-0001"))
}

func TestAllocationBulk_TravelBudgetInsertsRowInTheTravelTransaction(t *testing.T) {
	withBudgetTables(t, true)
	m := newAllocationMocks(t)
	line, sug, src, by := 7, 7, "ALLOCATION_TYPE", "USER"
	expectBulkToTravelInsert(m)
	m.travel.ExpectExec(re("INSERT INTO travel_budget (job_number, budget_year, budget_line_id, suggested_line_id")).
		WithArgs("TRV-2027-02-0001", "2027-02-01", 7, "2027-02-01", 7, "ALLOCATION_TYPE", "USER", "mgr@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	resp, err := m.repo.Bulk(context.Background(), budgetBulkRequest(&domain.AllocationTravelBudget{
		BudgetLineID: &line, SuggestedLineID: &sug, SuggestionSource: &src, ChosenBy: &by}))
	if err != nil || resp.TravelJobNumber == nil || *resp.TravelJobNumber != "TRV-2027-02-0001" {
		t.Fatalf("resp = %+v, err = %v", resp, err)
	}
	m.verify(t)
}

// A line that is not Active or not of the trip's year becomes NULL in SQL, and
// null ids are sent as NULL: the allocation still succeeds.
func TestAllocationBulk_TravelBudgetLineIsCheckedInSQLAndMayBeNull(t *testing.T) {
	withBudgetTables(t, true)
	m := newAllocationMocks(t)
	src := "NONE"
	expectBulkToTravelInsert(m)
	m.travel.ExpectExec(`l\.status = 'Active'\s+AND l\.budget_year = \(SELECT y\.budget_year`).
		WithArgs("TRV-2027-02-0001", "2027-02-01", nil, "2027-02-01", nil, "NONE", nil, "mgr@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	if _, err := m.repo.Bulk(context.Background(), budgetBulkRequest(&domain.AllocationTravelBudget{SuggestionSource: &src})); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}

// No travelBudget (old backend) or no budget tables: the legacy statements only.
func TestAllocationBulk_NoTravelBudgetOrNoTablesIssuesNoBudgetSQL(t *testing.T) {
	line := 7
	for name, tc := range map[string]struct {
		tables bool
		b      *domain.AllocationTravelBudget
	}{
		"absent":        {true, nil},
		"tables absent": {false, &domain.AllocationTravelBudget{BudgetLineID: &line}},
	} {
		t.Run(name, func(t *testing.T) {
			withBudgetTables(t, tc.tables)
			m := newAllocationMocks(t)
			expectBulkToTravelInsert(m)
			m.travel.ExpectCommit()
			m.alloc.ExpectCommit()
			if _, err := m.repo.Bulk(context.Background(), budgetBulkRequest(tc.b)); err != nil {
				t.Fatal(err)
			}
			m.verify(t)
		})
	}
}

func TestAllocationBulk_TravelBudgetFailureRollsTheAllocationBack(t *testing.T) {
	withBudgetTables(t, true)
	m := newAllocationMocks(t)
	expectBulkToTravelInsert(m)
	m.travel.ExpectExec(re("INSERT INTO travel_budget (")).WillReturnError(errors.New("boom"))
	m.travel.ExpectRollback()
	m.alloc.ExpectRollback()
	if _, err := m.repo.Bulk(context.Background(), budgetBulkRequest(&domain.AllocationTravelBudget{})); err == nil {
		t.Error("want error")
	}
	m.verify(t)
}

// Path B: the clearance change that creates the trip writes the line too.
func TestAllocationUpdate_ConfirmWritesTheTravelBudgetRow(t *testing.T) {
	withBudgetTables(t, true)
	m := newAllocationMocks(t)
	next := "Confirmed"
	line, by := 7, "SYSTEM"
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-1"))
	m.travel.ExpectExec(re("INSERT INTO travel_budget (")).
		WithArgs("TRV-1", "2026-11-10", 7, "2026-11-10", 7, nil, "SYSTEM", "adder@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow("Confirmed", "Yes")...))
	if _, err := m.repo.Update(context.Background(), preRecord("Ready for Clearance", "Yes", "jane@example.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &next, UpdatedBy: "mgr@example.com",
			TravelBudget: &domain.AllocationTravelBudget{BudgetLineID: &line, SuggestedLineID: &line, ChosenBy: &by}}); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}
