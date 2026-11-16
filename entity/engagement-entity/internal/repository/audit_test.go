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
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func auditOn(t *testing.T) {
	audit.SetEnabled(true)
	t.Cleanup(func() { audit.SetEnabled(false) })
}

// An allocation confirmed with travel: the allocation's event in the
// allocation_db transaction, the new travel request's in the travelapp one.
func TestAudit_AllocationUpdateAndTravelInsertInTheirTransactions(t *testing.T) {
	auditOn(t)
	m := newAllocationMocks(t)
	next := "Confirmed"
	cols := []string{"Id", "email", "clearance_status", "engagement_id"}
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT * FROM allocation WHERE Id = ? FOR UPDATE")).WithArgs("A0000000001").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("A0000000001", "jane@wso2.com", "Ready for Clearance", "E07201"))
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT * FROM allocation WHERE id IN (?)")).WithArgs("A0000000001").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("A0000000001", "jane@wso2.com", "Confirmed", "E07201"))
	m.alloc.ExpectExec(re("INSERT INTO audit_event")).
		WithArgs("mgr@wso2.com", "user", "Status Change", "allocation", "A0000000001", "engagement", "E07201", nil, "engagement-app",
			`{"clearance_status":{"before":"Ready for Clearance","after":"Confirmed"}}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-1"))
	m.travel.ExpectQuery(re("SELECT * FROM travel WHERE job_number IN (?)")).WithArgs("TRV-1").
		WillReturnRows(sqlmock.NewRows([]string{"job_number", "status", "allocation_id"}).AddRow("TRV-1", "New", "A0000000001"))
	m.travel.ExpectExec(re("INSERT INTO audit_event")).
		WithArgs("adder@wso2.com", "user", "Create", "travel_request", "TRV-1", "allocation", "A0000000001", nil, "engagement-app",
			`{"allocation_id":{"before":null,"after":"A0000000001"},"job_number":{"before":null,"after":"TRV-1"},"status":{"before":null,"after":"New"}}`, nil).
		WillReturnResult(sqlmock.NewResult(2, 1))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow("Confirmed", "Yes")...))

	if _, err := m.repo.Update(context.Background(), preRecord("Ready for Clearance", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &next, UpdatedBy: "mgr@wso2.com"}); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}

// A failed audit insert rolls the change back.
func TestAudit_FailureRollsTheWriteBack(t *testing.T) {
	auditOn(t)
	m := newAllocationMocks(t)
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT * FROM allocation WHERE id = ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("A1", "Active"))
	m.alloc.ExpectExec(re("UPDATE allocation SET status = 'Deleted'")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT * FROM allocation WHERE id IN (?)")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("A1", "Deleted"))
	m.alloc.ExpectExec(re("INSERT INTO audit_event")).WillReturnError(context.DeadlineExceeded)
	m.alloc.ExpectRollback()
	if err := m.repo.Delete(context.Background(), "A1"); err == nil {
		t.Fatal("want the audit error")
	}
	m.verify(t)
}

func TestAuditRepository_PicksTheSchemaOfTheEntity(t *testing.T) {
	auditOn(t)
	m := newAllocationMocks(t)
	repo := NewAuditRepository(m.repo.alloc, m.repo.travel)
	m.travel.ExpectExec(re("INSERT INTO audit_event")).WithArgs(sqlmock.AnyArg(), "system", "Download", "visa", "5",
		nil, nil, nil, "engagement-app", nil, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	m.alloc.ExpectExec(re("INSERT INTO audit_event")).WithArgs(sqlmock.AnyArg(), "system", "Export", "allocation", nil,
		nil, nil, nil, "engagement-app", nil, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	ctx := context.Background()
	for _, e := range []audit.Event{
		{Action: audit.Download, EntityType: "visa", EntityID: "5"},
		{Action: audit.Export, EntityType: "allocation"},
	} {
		if ok, err := repo.Record(ctx, e); err != nil || !ok {
			t.Fatalf("%s: ok %v err %v", e.EntityType, ok, err)
		}
	}
	if ok, err := repo.Record(ctx, audit.Event{Action: audit.View, EntityType: "nope"}); err != nil || ok {
		t.Errorf("unknown type: ok %v err %v", ok, err)
	}
	m.verify(t)
}
