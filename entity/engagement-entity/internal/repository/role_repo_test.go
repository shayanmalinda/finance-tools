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

func TestRoleRoles_GroupsPrivilegesPerRole(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &roleRepo{db: er.alloc, app: domain.AppAllocations}
	mock.ExpectQuery(re(`SELECT id, name FROM roles ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "wso2-employees").AddRow(2, "app-allocations-admin-super"))
	mock.ExpectQuery(re(`SELECT DISTINCT role_id, privilege_id FROM role_privilege ORDER BY role_id, privilege_id`)).
		WillReturnRows(sqlmock.NewRows([]string{"r", "p"}).AddRow(1, 1).AddRow(2, 1).AddRow(2, 23).AddRow(99, 4))
	roles, err := repo.Roles(context.Background())
	if err != nil || len(roles) != 2 || len(roles[0].PrivilegeIDs) != 1 || len(roles[1].PrivilegeIDs) != 2 {
		t.Fatalf("roles %+v err %v", roles, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRoleRole_NotFound(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &roleRepo{db: er.alloc, app: domain.AppTravel}
	mock.ExpectQuery(re(`SELECT id, name FROM roles WHERE id = ?`)).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	if _, err := repo.Role(context.Background(), 7); !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("err %v", err)
	}
}

// Allocations: delete + insert in a transaction, then the audit_log row with
// the legacy description.
func TestRoleReplacePrivileges_Allocations(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &roleRepo{db: er.alloc, app: domain.AppAllocations}
	mock.ExpectBegin()
	mock.ExpectExec(re(`DELETE FROM role_privilege WHERE role_id = ?`)).WithArgs(3).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(re(`INSERT INTO role_privilege (role_id, privilege_id) VALUES (?, ?), (?, ?)`)).
		WithArgs(3, 1, 3, 23).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	mock.ExpectExec(re(`INSERT INTO audit_log (resource, action, user, description, timestamp)`)).
		WithArgs("ROLE", "UPDATE", "me@wso2.com",
			`The role with 3 as the ID is updated by me@wso2.com with the following privileges: ["1","23"]`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.ReplacePrivileges(context.Background(), 3, []int{1, 23}, "me@wso2.com"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Travel: delete, insert and privilege_auditlog in one transaction; an empty
// set inserts nothing.
func TestRoleReplacePrivileges_TravelEmpty(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &roleRepo{db: er.alloc, app: domain.AppTravel}
	mock.ExpectBegin()
	mock.ExpectExec(re(`DELETE FROM role_privilege WHERE role_id = ?`)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(re(`INSERT INTO privilege_auditlog (role_id, privilege_ids, updated_by, date)`)).
		WithArgs(4, "[]", "me@wso2.com").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.ReplacePrivileges(context.Background(), 4, nil, "me@wso2.com"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if got := roleIntList([]int{1, 2, 3}); got != "[1,2,3]" {
		t.Errorf("roleIntList = %s", got)
	}
}

// An audit failure on the Allocations side is only logged.
func TestRoleReplacePrivileges_AuditFailureIgnored(t *testing.T) {
	mock, _, er := engagementMocks(t)
	repo := &roleRepo{db: er.alloc, app: domain.AppAllocations}
	mock.ExpectBegin()
	mock.ExpectExec(re(`DELETE FROM role_privilege`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectExec(re(`INSERT INTO audit_log`)).
		WithArgs("ROLE", "UPDATE", "me", "The role with 4 as the ID is updated by me to have no privileges").
		WillReturnError(errors.New("audit down"))
	if err := repo.ReplacePrivileges(context.Background(), 4, []int{}, "me"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
