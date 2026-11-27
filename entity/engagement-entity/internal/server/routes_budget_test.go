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
	"net/http"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

func withBudgetTables(t *testing.T, present bool) {
	t.Helper()
	was := repository.HasBudgetTables()
	repository.SetBudgetTables(present)
	t.Cleanup(func() { repository.SetBudgetTables(was) })
}

func TestBudgetRoutes_WithoutTablesAnswer503(t *testing.T) {
	withBudgetTables(t, false)
	h, _, _ := newTestRouter(t)
	for _, c := range []struct{ method, target, body string }{
		{"GET", "/budgets/years", ""},
		{"GET", "/budgets/lines?year=2027", ""},
		{"POST", "/budgets/lines", `{}`},
		{"PUT", "/budgets/lines/7", `{}`},
		{"DELETE", "/budgets/lines/7?updatedBy=a@b.c", ""},
		{"GET", "/budgets/allocation-type-rules", ""},
		{"PUT", "/budgets/allocation-type-rules/12", `{"ruleKind":"NONE"}`},
		{"GET", "/budgets/region-rules", ""},
		{"GET", "/budgets/team-rules", ""},
		{"GET", "/budgets/engagements/1234/line", ""},
		{"PUT", "/budgets/engagements/1234/line", `{"lineCode":null}`},
		{"GET", "/budgets/trips/TRV-1", ""},
		{"PUT", "/budgets/trips/TRV-1", `{"budgetLineId":null}`},
		{"GET", "/budgets/needs-line?year=2027", ""},
		{"GET", "/budgets/ledger?from=2027-01-01&to=2027-12-31", ""},
	} {
		rec := engagementServe(h, c.method, c.target, c.body)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Resources/travelapp_table_update_v1.2.0.sql") {
			t.Errorf("%s %s: status %d, body %s", c.method, c.target, rec.Code, rec.Body)
		}
	}
}

func TestBudgetRoutes_Validation(t *testing.T) {
	withBudgetTables(t, true)
	h, _, _ := newTestRouter(t)
	for _, c := range []struct{ method, target, body string }{
		{"GET", "/budgets/lines?year=abc", ""},
		{"GET", "/budgets/lines?includeRetired=maybe", ""},
		{"POST", "/budgets/lines", `{"unknown":1}`},
		{"PUT", "/budgets/lines/0", `{}`},
		{"PUT", "/budgets/allocation-type-rules/12", `{"ruleKind":"MAYBE"}`},
		{"GET", "/budgets/needs-line", ""},
		{"GET", "/budgets/ledger", ""},
		{"GET", "/budgets/ledger?from=2027-01-01&to=2027-12-31&unassigned=x", ""},
	} {
		if rec := engagementServe(h, c.method, c.target, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status %d, want 400 (%s)", c.method, c.target, rec.Code, rec.Body)
		}
	}
}

func TestBudgetRoutes_ReadsAndWrites(t *testing.T) {
	withBudgetTables(t, true)
	h, _, tm := newTestRouter(t)
	lineCols := []string{"id", "y", "code", "f", "s", "h", "e", "amt", "st", "so", "ub", "ua"}

	tm.ExpectQuery("FROM budget_year").WillReturnRows(sqlmock.NewRows([]string{"y", "s", "e", "a"}).AddRow(2027, "2027-01-01", "2027-12-31", true))
	rec := engagementServe(h, "GET", "/budgets/years", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"years":[{"year":2027,"startDate":"2027-01-01","endDate":"2027-12-31","isActive":true}]}` {
		t.Errorf("years: %d %s", rec.Code, rec.Body)
	}

	tm.ExpectQuery("FROM budget_line AS l").WillReturnRows(sqlmock.NewRows(lineCols).
		AddRow(7, 2027, "SALES_APAC", "Sales", "APAC", "Genie", nil, 60000.0, "Active", 30, nil, nil))
	rec = engagementServe(h, "GET", "/budgets/lines?year=2027", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"code":"SALES_APAC"`) || !strings.Contains(rec.Body.String(), `"holderEmail":null`) {
		t.Errorf("lines: %d %s", rec.Code, rec.Body)
	}

	// POST: years check, insert, read back.
	tm.ExpectQuery("FROM budget_year").WillReturnRows(sqlmock.NewRows([]string{"y", "s", "e", "a"}).AddRow(2027, "2027-01-01", "2027-12-31", true))
	tm.ExpectBegin()
	tm.ExpectExec("INSERT INTO budget_line").WillReturnResult(sqlmock.NewResult(8, 1))
	tm.ExpectCommit()
	tm.ExpectQuery("FROM budget_line AS l WHERE l.id = ?").WillReturnRows(sqlmock.NewRows(lineCols).
		AddRow(8, 2027, "NEW", "Sales", "New", nil, nil, 10.0, "Active", 0, nil, nil))
	rec = engagementServe(h, "POST", "/budgets/lines", `{"year":2027,"code":"NEW","functionName":"Sales","subFunction":"New","amountUsd":10,"createdBy":"a@b.c"}`)
	if rec.Code != http.StatusCreated {
		t.Errorf("create: %d %s", rec.Code, rec.Body)
	}

	// PUT unknown line: 404.
	tm.ExpectQuery("FROM budget_line AS l WHERE l.id = ?").WillReturnRows(sqlmock.NewRows(lineCols))
	rec = engagementServe(h, "PUT", "/budgets/lines/99", `{"functionName":"S","subFunction":"S","amountUsd":1,"status":"Active"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("update unknown: %d %s", rec.Code, rec.Body)
	}

	// DELETE: soft retire, 204.
	tm.ExpectQuery("FROM budget_line AS l WHERE l.id = ?").WillReturnRows(sqlmock.NewRows(lineCols).
		AddRow(7, 2027, "SALES_APAC", "Sales", "APAC", "Genie", nil, 60000.0, "Active", 30, nil, nil))
	tm.ExpectBegin()
	tm.ExpectExec("UPDATE budget_line SET status = ?").WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if rec = engagementServe(h, "DELETE", "/budgets/lines/7?updatedBy=a@b.c", ""); rec.Code != http.StatusNoContent {
		t.Errorf("retire: %d %s", rec.Code, rec.Body)
	}

	tm.ExpectQuery("FROM budget_allocation_type_rule").WillReturnRows(sqlmock.NewRows([]string{"i", "k", "c"}).AddRow(12, "LINE", "SALES_APAC"))
	rec = engagementServe(h, "GET", "/budgets/allocation-type-rules", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"rules":[{"allocationTypeId":12,"ruleKind":"LINE","lineCode":"SALES_APAC"}]}` {
		t.Errorf("rules: %d %s", rec.Code, rec.Body)
	}

	tm.ExpectQuery("FROM budget_region_rule").WillReturnRows(sqlmock.NewRows([]string{"r", "s", "c"}).AddRow("EU", nil, "SALES_EU"))
	if rec = engagementServe(h, "GET", "/budgets/region-rules", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"subRegion":null`) {
		t.Errorf("region rules: %d %s", rec.Code, rec.Body)
	}
	tm.ExpectQuery("FROM budget_team_rule").WillReturnRows(sqlmock.NewRows([]string{"i", "b", "d", "t", "c"}).AddRow(1, nil, "MARKETING", nil, "MKT_TRAVEL"))
	if rec = engagementServe(h, "GET", "/budgets/team-rules", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"department":"MARKETING"`) {
		t.Errorf("team rules: %d %s", rec.Code, rec.Body)
	}

	// Engagement line: unset reads as null, never 404.
	tm.ExpectQuery("FROM engagement_budget_line").WillReturnRows(sqlmock.NewRows([]string{"c"}))
	rec = engagementServe(h, "GET", "/budgets/engagements/1234/line", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"engagementId":"1234","lineCode":null}` {
		t.Errorf("engagement line: %d %s", rec.Code, rec.Body)
	}
	tm.ExpectBegin()
	tm.ExpectExec("DELETE FROM engagement_budget_line").WillReturnResult(sqlmock.NewResult(0, 0))
	tm.ExpectCommit()
	if rec = engagementServe(h, "PUT", "/budgets/engagements/1234/line", `{"lineCode":null,"updatedBy":"a@b.c"}`); rec.Code != 200 {
		t.Errorf("clear engagement line: %d %s", rec.Code, rec.Body)
	}

	// Trip: 404 when missing.
	tripCols := []string{"job", "email", "country", "start", "end", "status", "eng", "type", "cust", "year", "line", "sug", "src", "by", "rate", "has"}
	tm.ExpectQuery("WHERE t.job_number = ?").WillReturnRows(sqlmock.NewRows(tripCols))
	if rec = engagementServe(h, "GET", "/budgets/trips/TRV-404", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing trip: %d %s", rec.Code, rec.Body)
	}
	tm.ExpectQuery("WHERE t.job_number = ?").WillReturnRows(sqlmock.NewRows(tripCols).
		AddRow("TRV-1", "a@wso2.com", "Germany", "2027-01-18", "2027-01-22", "New", "1234", "Pre-Sale", "Acme", 2027, nil, nil, "NONE", nil, "2026-12-10", true))
	if rec = engagementServe(h, "GET", "/budgets/trips/TRV-1", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"hasRow":true`) {
		t.Errorf("trip: %d %s", rec.Code, rec.Body)
	}

	// Needs line and ledger.
	tm.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	tm.ExpectQuery("ORDER BY t.start_date, t.job_number LIMIT").WillReturnRows(sqlmock.NewRows(tripCols))
	if rec = engagementServe(h, "GET", "/budgets/needs-line?year=2027&limit=5", ""); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"items":[],"total":0}` {
		t.Errorf("needs-line: %d %s", rec.Code, rec.Body)
	}
	tm.ExpectQuery("FROM travel AS t LEFT JOIN travel_budget").WillReturnRows(sqlmock.NewRows([]string{"j", "l", "s", "c", "sd", "ed", "rd"}))
	tm.ExpectQuery("FROM invoice AS i").WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}))
	tm.ExpectQuery("FROM perdiem_payment").WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}))
	rec = engagementServe(h, "GET", "/budgets/ledger?from=2027-01-01&to=2027-12-31&unassigned=true", "")
	want := `{"from":"2027-01-01","to":"2027-12-31","expenseAvailable":false,"truncated":false,"trips":[],"costs":[],"unassigned":[]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("ledger: %d %s", rec.Code, rec.Body)
	}
}
