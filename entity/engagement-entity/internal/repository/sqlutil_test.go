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
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
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

// TestListSortWhitelists checks every list's whitelist: each key sorts
// ascending and descending with the id tiebreak, and an empty, unknown or
// injection-looking sort is rejected so the list keeps its default order.
func TestListSortWhitelists(t *testing.T) {
	lists := []struct {
		name     string
		cols     SortColumns
		tiebreak string
		keys     []string
	}{
		{"engagements", engagementSortColumns, `e.id`, []string{"id", "type", "country", "startDate", "requestStatus", "engagementStatus", "created"}},
		{"approvals", approvalSortColumns, `e.id`, []string{"id", "type", "country", "startDate", "businessUnit", "created"}},
		{"allocations", allocationSortColumns, `a.id`, []string{"id", "consultant", "engagement", "type", "startDate", "status", "created"}},
		{"invoices", financeInvoiceSortColumns, `i.id`, []string{"invoiceNumber", "jobNumber", "costType", "addedDate", "status"}},
		{"payments", financePaymentSortColumns, `pp.id`, []string{"number", "jobNumber", "addedDate", "status"}},
		{"visas", travelVisaSortColumns, `v.id`, []string{"traveller", "country", "visaType", "validFrom", "expiry"}},
		{"documents", travelDocumentSortColumns, `d.id`, []string{"traveller", "type", "country", "issued", "expiry"}},
	}
	for _, l := range lists {
		if len(l.cols) != len(l.keys) {
			t.Errorf("%s: whitelist has %d keys, want %d", l.name, len(l.cols), len(l.keys))
		}
		for _, k := range l.keys {
			for _, sort := range []string{k, "-" + k} {
				o, ok := l.cols.Order(sort, l.tiebreak)
				if !ok || !strings.HasPrefix(o, " ORDER BY ") || !strings.Contains(o, l.tiebreak) {
					t.Errorf("%s %q = %q, %v", l.name, sort, o, ok)
				}
				if strings.HasPrefix(sort, "-") && !strings.Contains(o, " DESC") {
					t.Errorf("%s %q is not descending: %q", l.name, sort, o)
				}
			}
		}
		for _, bad := range []string{"", "current", "amount", "id; DROP TABLE x", "id,(SELECT 1)", "-", "created DESC"} {
			if o, ok := l.cols.Order(bad, l.tiebreak); ok {
				t.Errorf("%s %q accepted: %q", l.name, bad, o)
			}
		}
	}
}

// TestDefaultOrdersUnchanged checks that an empty sort leaves each list on
// the order it had before sorting was added.
func TestDefaultOrdersUnchanged(t *testing.T) {
	if o, _ := engagementOrder(domain.EngagementSearchRequest{Today: "2026-01-01"}); !strings.Contains(o, "CASE WHEN DATE(e.end_date) >= ? THEN 0 ELSE 1 END") {
		t.Errorf("engagements default = %q", o)
	}
	if o, _ := engagementOrder(domain.EngagementSearchRequest{PendingApproval: true, Sort: "created"}); o != ` ORDER BY e.last_updated_date, e.id` {
		t.Errorf("approvals default = %q", o)
	}
	if o, _ := engagementOrder(domain.EngagementSearchRequest{Sort: "-requestStatus"}); o != ` ORDER BY e.status DESC, e.id` {
		t.Errorf("engagement sort = %q", o)
	}
}
