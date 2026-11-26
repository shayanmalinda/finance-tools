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
)

func TestPlaceholdersAndLike(t *testing.T) {
	if got := Placeholders(3); got != "?, ?, ?" {
		t.Errorf("Placeholders(3) = %q", got)
	}
	if got := Placeholders(0); got != "" {
		t.Errorf("Placeholders(0) = %q", got)
	}
	if got := LikeContains(`50%_a\b`); got != `%50\%\_a\\b%` {
		t.Errorf("LikeContains = %q", got)
	}
}

func TestAgencyOf(t *testing.T) {
	db, mock := mockDB(t)
	r := NewAgencyRepository(db)
	mock.ExpectQuery(re("WHERE tae.wso2_email = ? AND tae.status = 'Active'")).WithArgs("agent@classic.example").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "Birch Travels"))
	a, err := r.AgencyOf(context.Background(), "agent@classic.example")
	if err != nil || a == nil || a.ID != 2 || a.Name != "Birch Travels" {
		t.Fatalf("AgencyOf = %+v, %v", a, err)
	}
	mock.ExpectQuery(re("FROM travel_agent_employee")).WithArgs("x@example.com").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
	if a, err := r.AgencyOf(context.Background(), "x@example.com"); err != nil || a != nil {
		t.Errorf("not an agent: %+v, %v", a, err)
	}
}

func TestSortColumnsOrder(t *testing.T) {
	cols := SortColumns{
		"id":      {`x.id`},
		"country": {`x.country`, `x.city`},
	}
	tests := []struct {
		sort   string
		want   string
		wantOK bool
	}{
		{"id", ` ORDER BY x.id`, true},
		{"-id", ` ORDER BY x.id DESC`, true},
		{"country", ` ORDER BY x.country, x.city, x.id`, true},
		{"-country", ` ORDER BY x.country DESC, x.city DESC, x.id`, true},
		{"", "", false},
		{"-", "", false},
		{"--id", "", false},
		{"unknown", "", false},
		{"ID", "", false},
		{"id; DROP TABLE engagement", "", false},
		{"id, (SELECT SLEEP(5))", "", false},
		{"-id --", "", false},
	}
	for _, tc := range tests {
		got, ok := cols.Order(tc.sort, `x.id`)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Order(%q) = %q, %v; want %q, %v", tc.sort, got, ok, tc.want, tc.wantOK)
		}
	}
}
