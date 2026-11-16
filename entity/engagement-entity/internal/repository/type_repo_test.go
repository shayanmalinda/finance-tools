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
	"database/sql/driver"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func TestListEngagementTypes_Filters(t *testing.T) {
	active := true
	tests := []struct {
		name   string
		filter domain.EngagementTypeFilter
		sql    string
		args   []driver.Value
	}{
		{"no filter", domain.EngagementTypeFilter{}, "WHERE 1 = 1 ORDER BY id", nil},
		{"type", domain.EngagementTypeFilter{Type: "Internal"}, "WHERE 1 = 1 AND type = ? ORDER BY id", []driver.Value{"Internal"}},
		{"type and active", domain.EngagementTypeFilter{Type: "Customer", IsActive: &active},
			"WHERE 1 = 1 AND type = ? AND is_active = ? ORDER BY id", []driver.Value{"Customer", true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock, _, types := newMock(t)
			q := mock.ExpectQuery(re(tc.sql))
			if tc.args != nil {
				q = q.WithArgs(tc.args...)
			}
			q.WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "is_active"}).
				AddRow(5, "Marketing Event", "Internal", 1))

			got, err := types().ListEngagementTypes(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("ListEngagementTypes: %v", err)
			}
			if len(got) != 1 || got[0] != (domain.EngagementType{ID: 5, Name: "Marketing Event", Type: "Internal", IsActive: true}) {
				t.Errorf("got %+v", got)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestListAllocationTypes_LegacyShapeAndFilters(t *testing.T) {
	mock, _, types := newMock(t)
	mock.ExpectQuery(re("WHERE aT.status IN ('Active', 'Deleted') AND eT.id = ? AND aT.status = ? ORDER BY aT.allocation_type")).
		WithArgs(1, "Active").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "allocation_type", "status", "color", "department", "emailToAllocations",
			"default_engagement_code", "engagement_code_suffix", "eT.id", "eT.name",
		}).AddRow(3, "Pre-sale Support", "ACTIVE", "#d030eb", "SOLUTIONS ARCHITECTURE", nil, "PRE-GEN", nil, 1, "Internal Engagement"))

	got, err := types().ListAllocationTypes(context.Background(),
		domain.AllocationTypeFilter{EngagementTypeID: 1, Status: "Active"})
	if err != nil {
		t.Fatalf("ListAllocationTypes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	at := got[0]
	if at.AllocationTypeName != "Pre-sale Support" || at.Status != "ACTIVE" || at.EngagementTypeName != "Internal Engagement" ||
		at.Color == nil || *at.Color != "#d030eb" || at.EmailToAllocations != nil || at.EngagementCodeSuffix != nil {
		t.Errorf("got %+v", at)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
