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

func TestSettingsAllocationTypeWrites(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &settingsRepo{db: er.alloc}
	ctx := context.Background()
	in := domain.SettingsAllocationTypeInput{EngagementTypeID: 1, AllocationTypeName: "Digital Operations",
		DefaultEngagementCode: "DIG-GEN", Department: "DIGITAL", EmailToAllocations: "No", Actor: "me@wso2.com"}

	mock.ExpectQuery(re(`SELECT COUNT(*) FROM engagement_type WHERE id = ?`)).WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	if ok, err := repo.EngagementTypeExists(ctx, 1); err != nil || !ok {
		t.Fatalf("exists %v %v", ok, err)
	}
	mock.ExpectQuery(re(`WHERE engagement_type = ? AND allocation_type = ? AND status != 'Deleted' AND id != ?`)).
		WithArgs(1, "Digital Operations", 0).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	if taken, err := repo.AllocationTypeNameTaken(ctx, 1, "Digital Operations", 0); err != nil || taken {
		t.Fatalf("taken %v %v", taken, err)
	}
	// Empty optional fields are NULL; status ACTIVE as the legacy insert.
	mock.ExpectBegin()
	mock.ExpectExec(re(`VALUES (?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`)).
		WithArgs(1, "DIGITAL", "Digital Operations", nil, "DIG-GEN", nil, "No").WillReturnResult(sqlmock.NewResult(77, 1))
	mock.ExpectCommit()
	mock.ExpectExec(re(`INSERT INTO audit_log`)).
		WithArgs("ALLOCATION_TYPE", "INSERT", "me@wso2.com", "A new allocation type is added as Digital Operations by me@wso2.com").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if id, err := repo.CreateAllocationType(ctx, in); err != nil || id != 77 {
		t.Fatalf("id %d err %v", id, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re(`UPDATE allocation_type SET engagement_type = ?`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(re(`INSERT INTO audit_log`)).
		WithArgs("ALLOCATION_TYPE", "UPDATE", "me@wso2.com", "Allocation type of ID 77 is updated by me@wso2.com").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.UpdateAllocationType(ctx, 77, in); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re(`UPDATE allocation_type SET status = 'Deleted' WHERE id = ? AND status != 'Deleted'`)).
		WithArgs(77).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(re(`INSERT INTO audit_log`)).
		WithArgs("ALLOCATION_TYPE", "DELETE", "me@wso2.com", "Allocation type of ID 77 is deleted (status is updated to Deleted) by me@wso2.com").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.DeleteAllocationType(ctx, 77, "me@wso2.com"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re(`UPDATE allocation_type SET status = 'Deleted'`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if err := repo.DeleteAllocationType(ctx, 78, "me@wso2.com"); !errors.Is(err, ErrSettingsAllocationTypeNotFound) {
		t.Fatalf("err %v", err)
	}
	mock.ExpectQuery(re(`WHERE aT.id = ?`)).WithArgs(77).WillReturnRows(sqlmock.NewRows([]string{
		"id", "n", "s", "c", "d", "e", "code", "suf", "etid", "etn"}).
		AddRow(77, "Digital Operations", "ACTIVE", nil, "DIGITAL", "No", "DIG-GEN", nil, 1, "Internal Engagement"))
	if at, err := repo.AllocationType(ctx, 77); err != nil || at.EngagementTypeName != "Internal Engagement" {
		t.Fatalf("at %+v err %v", at, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
