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
	"reflect"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func re(s string) string { return regexp.QuoteMeta(s) }

func newMock(t *testing.T) (sqlmock.Sqlmock, func() *accessRepo, func() *typeRepo) {
	t.Helper()
	db, mock := mockDB(t)
	return mock,
		func() *accessRepo { return &accessRepo{db: db, app: domain.AppAllocations} },
		func() *typeRepo { return &typeRepo{db: db} }
}

func TestRolePrivilegeMap_GroupsByRoleAndDropsRepeats(t *testing.T) {
	mock, access, _ := newMock(t)
	repo := access()

	mock.ExpectQuery(re("FROM role_privilege rp")).WillReturnRows(
		sqlmock.NewRows([]string{"name", "privilege_id"}).
			AddRow("app-allocations-admin-super", 1).
			AddRow("app-allocations-admin-super", 2).
			AddRow("app-allocations-admin-super", 2). // duplicate pair in role_privilege
			AddRow("wso2-employees", 1))

	got, err := repo.RolePrivilegeMap(context.Background())
	if err != nil {
		t.Fatalf("RolePrivilegeMap: %v", err)
	}
	want := map[string][]int{
		"app-allocations-admin-super": {1, 2},
		"wso2-employees":              {1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Each app keeps its legacy query: Travel inner-joins privileges, Allocations
// does not.
func TestRolePrivilegeQuery_PerApp(t *testing.T) {
	joinPrivileges := regexp.MustCompile(`JOIN privileges p ON p.id = rp.privilege_id`)
	alloc := &accessRepo{app: domain.AppAllocations}
	travel := &accessRepo{app: domain.AppTravel}
	if joinPrivileges.MatchString(alloc.rolePrivilegeQuery()) {
		t.Error("allocations query joins privileges; the legacy Allocations backend does not")
	}
	if !joinPrivileges.MatchString(travel.rolePrivilegeQuery()) {
		t.Error("travel query does not join privileges; the legacy Travel backend does")
	}
}

func TestListPrivileges_ActiveOnly(t *testing.T) {
	mock, access, _ := newMock(t)
	desc := "View Dashboard Page"
	mock.ExpectQuery(re("FROM privileges WHERE status = 'active'")).WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "description"}).
			AddRow(1, "Dashboard Page", desc).
			AddRow(2, "Customer Engagement Create Page", nil))

	got, err := access().ListPrivileges(context.Background())
	if err != nil {
		t.Fatalf("ListPrivileges: %v", err)
	}
	if len(got) != 2 || got[0].Description == nil || *got[0].Description != desc || got[1].Description != nil {
		t.Errorf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListRoles(t *testing.T) {
	mock, access, _ := newMock(t)
	mock.ExpectQuery(re("SELECT id, name FROM roles ORDER BY id")).WillReturnRows(
		sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "wso2-employees").AddRow(11, "app-allocations-manager-finance"))

	got, err := access().ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	want := []domain.Role{{ID: 1, Name: "wso2-employees"}, {ID: 11, Name: "app-allocations-manager-finance"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
