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

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// withBudgetTables sets whether the budget tables exist for one test.
func withBudgetTables(t *testing.T, present bool) {
	t.Helper()
	was := HasBudgetTables()
	SetBudgetTables(present)
	t.Cleanup(func() { SetBudgetTables(was) })
}

func newBudgetRepo(t *testing.T, withExpense bool) (*budgetRepo, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	tr, tm := mockDB(t)
	r := &budgetRepo{travel: tr}
	var em sqlmock.Sqlmock
	if withExpense {
		r.expense, em = mockDB(t)
	}
	return r, tm, em
}

func str(s string) *string { return &s }

func TestDetectBudgetTables(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectQuery(re("FROM information_schema.tables")).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))
	ok, err := DetectBudgetTables(context.Background(), db)
	if err != nil || !ok {
		t.Fatalf("both tables: %v, %v", ok, err)
	}
	mock.ExpectQuery(re("FROM information_schema.tables")).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	if ok, err := DetectBudgetTables(context.Background(), db); err != nil || ok {
		t.Fatalf("one table: %v, %v", ok, err)
	}
	SetBudgetTables(false)
	if HasBudgetTables() {
		t.Error("HasBudgetTables should follow SetBudgetTables")
	}
}

func TestBudgetCreateLine_InsertsActiveAndDuplicateIsConflict(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	in := domain.BudgetLineCreateRequest{Year: 2027, Code: "X", FunctionName: "Sales", SubFunction: "X", AmountUSD: 10,
		SortOrder: 5, CreatedBy: "fin@example.com"}
	tm.ExpectBegin()
	tm.ExpectExec(re("INSERT INTO budget_line (")).
		WithArgs(2027, "X", "Sales", "X", nil, nil, 10.0, "Active", 5, "fin@example.com").
		WillReturnResult(sqlmock.NewResult(7, 1))
	tm.ExpectCommit()
	id, err := r.CreateLine(context.Background(), in)
	if err != nil || id != 7 {
		t.Fatalf("id = %d, err = %v", id, err)
	}

	tm.ExpectBegin()
	tm.ExpectExec(re("INSERT INTO budget_line (")).WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	tm.ExpectRollback()
	_, err = r.CreateLine(context.Background(), in)
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("duplicate: err = %v, want ConflictError", err)
	}
}

func TestBudgetUpdateAndRetireLine(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectBegin()
	tm.ExpectExec(re("UPDATE budget_line SET function_name = ?")).
		WithArgs("Sales", "EU", str("Ricardo"), nil, 75000.0, 40, "Active", "fin@example.com", 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.UpdateLine(context.Background(), 7, domain.BudgetLineUpdateRequest{FunctionName: "Sales", SubFunction: "EU",
		HolderName: str("Ricardo"), AmountUSD: 75000, SortOrder: 40, Status: "Active", UpdatedBy: "fin@example.com"}); err != nil {
		t.Fatal(err)
	}
	// Retire is a soft delete: the row stays, trips keep the line.
	tm.ExpectBegin()
	tm.ExpectExec(re("UPDATE budget_line SET status = ?, updated_by = ?, updated_at = NOW() WHERE id = ?")).
		WithArgs("Retired", "fin@example.com", 7).WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.RetireLine(context.Background(), 7, "fin@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetLines_FilterRetiredAndYear(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	cols := []string{"id", "y", "code", "f", "s", "h", "e", "amt", "st", "so", "ub", "ua"}
	tm.ExpectQuery(re("WHERE 1 = 1 AND l.budget_year = ? AND l.status = ? ORDER BY l.sort_order, l.id")).
		WithArgs(2027, "Active").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(7, 2027, "SALES_EU", "Sales", "EU", "Ricardo", nil, 75000.0, "Active", 40, nil, nil))
	lines, err := r.Lines(context.Background(), 2027, false)
	if err != nil || len(lines) != 1 || lines[0].Code != "SALES_EU" || lines[0].HolderName == nil || lines[0].HolderEmail != nil {
		t.Fatalf("lines = %+v, err = %v", lines, err)
	}
	tm.ExpectQuery(re("WHERE 1 = 1 ORDER BY l.sort_order, l.id")).WillReturnRows(sqlmock.NewRows(cols))
	if lines, err := r.Lines(context.Background(), 0, true); err != nil || lines == nil || len(lines) != 0 {
		t.Fatalf("all years with retired: %+v, %v", lines, err)
	}
}

func TestBudgetSaveAllocationTypeRule_InsertsThenUpdates(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	code := "SALES_EU"
	in := domain.BudgetAllocationTypeRuleRequest{RuleKind: "LINE", LineCode: &code, UpdatedBy: "fin@example.com"}
	tm.ExpectBegin()
	tm.ExpectQuery(re("FROM budget_allocation_type_rule WHERE allocation_type_id = ? FOR UPDATE")).WithArgs(12).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	tm.ExpectExec(re("INSERT INTO budget_allocation_type_rule")).
		WithArgs(12, "LINE", "SALES_EU", "fin@example.com").WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.SaveAllocationTypeRule(context.Background(), 12, in); err != nil {
		t.Fatal(err)
	}
	tm.ExpectBegin()
	tm.ExpectQuery(re("FROM budget_allocation_type_rule")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	tm.ExpectExec(re("UPDATE budget_allocation_type_rule SET rule_kind = ?, line_code = ?")).
		WithArgs("REGION", nil, "fin@example.com", 12).WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	in.RuleKind, in.LineCode = "REGION", nil
	if err := r.SaveAllocationTypeRule(context.Background(), 12, in); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetEngagementLine_SetAndClear(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectQuery(re("SELECT line_code FROM engagement_budget_line")).WithArgs("1234").
		WillReturnRows(sqlmock.NewRows([]string{"c"}))
	if c, err := r.EngagementLine(context.Background(), "1234"); err != nil || c != nil {
		t.Fatalf("unset: %v, %v", c, err)
	}
	code := "GA_BOARD"
	tm.ExpectBegin()
	tm.ExpectQuery(re("FROM engagement_budget_line WHERE engagement_id = ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	tm.ExpectExec(re("INSERT INTO engagement_budget_line")).WithArgs("1234", "GA_BOARD", "a@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.SetEngagementLine(context.Background(), "1234", &code, "a@example.com"); err != nil {
		t.Fatal(err)
	}
	tm.ExpectBegin()
	tm.ExpectExec(re("DELETE FROM engagement_budget_line WHERE engagement_id = ?")).WithArgs("1234").
		WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.SetEngagementLine(context.Background(), "1234", nil, "a@example.com"); err != nil {
		t.Fatal(err)
	}
}

var budgetTripTestCols = []string{"job", "email", "country", "start", "end", "status", "eng", "type", "cust",
	"year", "line", "sug", "src", "by", "rate", "has"}

func TestBudgetTrip_ReadsAndNotFound(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectQuery(re("WHERE t.job_number = ?")).WithArgs("TRV-1").WillReturnRows(sqlmock.NewRows(budgetTripTestCols).
		AddRow("TRV-1", "a@wso2.com", "Germany", "2027-01-18", "2027-01-22", "New", "1234", "Pre-Sale", "Acme",
			2027, nil, nil, "NONE", nil, "2026-12-10", true))
	b, err := r.Trip(context.Background(), "TRV-1")
	if err != nil || b.BudgetLineID != nil || b.BudgetYear == nil || *b.BudgetYear != 2027 || !b.HasRow || b.RateDate != "2026-12-10" {
		t.Fatalf("trip = %+v, err = %v", b, err)
	}
	tm.ExpectQuery(re("WHERE t.job_number = ?")).WillReturnRows(sqlmock.NewRows(budgetTripTestCols))
	if _, err := r.Trip(context.Background(), "TRV-2"); !errors.Is(err, ErrBudgetNotFound) {
		t.Fatalf("missing trip: %v", err)
	}
}

// An existing row keeps its suggestion fields and rate_date: the UPDATE does
// not list them.
func TestBudgetSetTripLine_UpdateKeepsSuggestionAndRateDate(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	year, line := 2027, 7
	tm.ExpectBegin()
	tm.ExpectQuery(re("FROM travel_budget WHERE job_number = ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	tm.ExpectExec(re("UPDATE travel_budget SET budget_year = ?, budget_line_id = ?, chosen_by = ?, updated_by = ?, updated_at = NOW() WHERE job_number = ?")).
		WithArgs(2027, 7, "USER", "a@example.com", "TRV-1").WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.SetTripLine(context.Background(), "TRV-1", &year, &line, "a@example.com"); err != nil {
		t.Fatal(err)
	}
}

// A trip with no row gets one with rate_date = DATE(travel.start_date).
func TestBudgetSetTripLine_NewRowRateDateIsTheStartDate(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectBegin()
	tm.ExpectQuery(re("FROM travel_budget WHERE job_number = ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	tm.ExpectExec(re("DATE(t.start_date), ?, NOW() FROM travel AS t WHERE t.job_number = ?")).
		WithArgs(nil, nil, "USER", "a@example.com", "TRV-1").WillReturnResult(sqlmock.NewResult(0, 1))
	tm.ExpectCommit()
	if err := r.SetTripLine(context.Background(), "TRV-1", nil, nil, "a@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetNeedsLine_ExcludesCancelledAndCounts(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectQuery(re("SELECT COUNT(*) FROM travel AS t LEFT JOIN travel_budget")).WithArgs(2027).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(12))
	tm.ExpectQuery(`COALESCE\(t\.status, ''\) != 'Cancelled' AND \(tb\.job_number IS NULL OR tb\.budget_line_id IS NULL\)[\s\S]*ORDER BY t\.start_date`).
		WithArgs(2027, 50, 0).
		WillReturnRows(sqlmock.NewRows(budgetTripTestCols).
			AddRow("TRV-1", "a@wso2.com", "Germany", "2027-01-18", "2027-01-22", "New", "1234", "Pre-Sale", "Acme",
				2027, nil, nil, nil, nil, "2027-01-18", false))
	items, total, err := r.NeedsLine(context.Background(), 2027, 50, 0)
	if err != nil || total != 12 || len(items) != 1 || items[0].HasRow || items[0].SuggestionSource != nil {
		t.Fatalf("items = %+v, total = %d, err = %v", items, total, err)
	}
}

func TestBudgetLedger_GroupsCostsPerTripSourceCurrency(t *testing.T) {
	r, tm, em := newBudgetRepo(t, true)
	tm.ExpectQuery(re("WHERE DATE(t.start_date) BETWEEN ? AND ? ORDER BY t.start_date, t.job_number LIMIT ?")).
		WithArgs("2027-01-01", "2027-12-31", budgetLedgerTripMax+1).
		WillReturnRows(sqlmock.NewRows([]string{"j", "l", "s", "c", "sd", "ed", "rd"}).
			AddRow("TRV-1", 7, "Cancelled", "Germany", "2027-01-18", "2027-01-22", "2026-12-10").
			AddRow("TRV-2", nil, "New", "India", "2027-02-01", "2027-02-03", "2027-02-01"))
	tm.ExpectQuery(re("FROM invoice AS i")).WithArgs("2027-01-01", "2027-12-31").
		WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}).AddRow("TRV-1", "LKR", 412000.0).AddRow("TRV-1", "USD", 10.5))
	tm.ExpectQuery(re("FROM perdiem_payment AS pp")).
		WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}).AddRow("TRV-2", "USD", 90.0))
	// Card: one billed in USD reports USD and the billed amount.
	em.ExpectQuery(re("FROM cc_txn AS c")).WithArgs("TRV-1", "TRV-2").
		WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a", "cc", "cca"}).
			AddRow("TRV-1", "EUR", 90.0, "USD", 100.0).AddRow("TRV-1", "EUR", 50.0, "", nil))
	em.ExpectQuery(re("FROM expense_claims AS c")).WithArgs("TRV-1", "TRV-2").
		WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a", "cc", "cca"}).AddRow("TRV-2", "LKR", 1000.0, "", 0.0))
	// Unassigned: blank job number, travel category.
	em.ExpectQuery(re("ec.expense_category LIKE ?")).WithArgs("2027-01-01", "2027-12-31", "%Travel%").
		WillReturnRows(sqlmock.NewRows([]string{"m", "c", "a", "cc", "cca"}).
			AddRow("2027-03", "USD", 800.0, "", nil).AddRow("2027-03", "USD", 12.4, "", nil))
	em.ExpectQuery(re("FROM expense_claims AS c")).WithArgs("2027-01-01", "2027-12-31", "%Travel%").
		WillReturnRows(sqlmock.NewRows([]string{"m", "c", "a", "cc", "cca"}))

	l, err := r.Ledger(context.Background(), domain.BudgetLedgerFilter{From: "2027-01-01", To: "2027-12-31", Unassigned: true})
	if err != nil {
		t.Fatal(err)
	}
	if !l.ExpenseAvailable || l.Truncated || len(l.Trips) != 2 || l.Trips[0].BudgetLineID == nil || *l.Trips[0].BudgetLineID != 7 {
		t.Fatalf("ledger = %+v", l)
	}
	want := []domain.BudgetLedgerCost{
		{JobNumber: "TRV-1", Source: "card", Currency: "EUR", Amount: 50},
		{JobNumber: "TRV-1", Source: "card", Currency: "USD", Amount: 100},
		{JobNumber: "TRV-1", Source: "invoice", Currency: "LKR", Amount: 412000},
		{JobNumber: "TRV-1", Source: "invoice", Currency: "USD", Amount: 10.5},
		{JobNumber: "TRV-2", Source: "claim", Currency: "LKR", Amount: 1000},
		{JobNumber: "TRV-2", Source: "perDiem", Currency: "USD", Amount: 90},
	}
	if len(l.Costs) != len(want) {
		t.Fatalf("costs = %+v", l.Costs)
	}
	for i := range want {
		if l.Costs[i] != want[i] {
			t.Errorf("cost %d = %+v, want %+v", i, l.Costs[i], want[i])
		}
	}
	if l.Unassigned == nil || len(*l.Unassigned) != 1 || (*l.Unassigned)[0] != (domain.BudgetLedgerUnassigned{Source: "card", Month: "2027-03", Currency: "USD", Amount: 812.4}) {
		t.Errorf("unassigned = %+v", l.Unassigned)
	}
}

func TestBudgetLedger_WithoutExpensePoolHasNoCardClaimOrUnassigned(t *testing.T) {
	r, tm, _ := newBudgetRepo(t, false)
	tm.ExpectQuery(re("FROM travel AS t LEFT JOIN travel_budget")).
		WillReturnRows(sqlmock.NewRows([]string{"j", "l", "s", "c", "sd", "ed", "rd"}).
			AddRow("TRV-1", nil, "New", "Germany", "2027-01-18", "2027-01-22", "2027-01-18"))
	tm.ExpectQuery(re("FROM invoice AS i")).WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}).AddRow("TRV-1", "USD", 5.0))
	tm.ExpectQuery(re("FROM perdiem_payment AS pp")).WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}))
	l, err := r.Ledger(context.Background(), domain.BudgetLedgerFilter{From: "2027-01-01", To: "2027-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	if l.ExpenseAvailable || len(l.Costs) != 1 || l.Costs[0].Source != "invoice" || l.Unassigned != nil {
		t.Errorf("ledger = %+v", l)
	}
}

func TestBudgetLedger_ExpenseFailureDegrades(t *testing.T) {
	r, tm, em := newBudgetRepo(t, true)
	tm.ExpectQuery(re("FROM travel AS t LEFT JOIN travel_budget")).
		WillReturnRows(sqlmock.NewRows([]string{"j", "l", "s", "c", "sd", "ed", "rd"}).
			AddRow("TRV-1", nil, "New", "Germany", "2027-01-18", "2027-01-22", "2027-01-18"))
	tm.ExpectQuery(re("FROM invoice AS i")).WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}).AddRow("TRV-1", "USD", 5.0))
	tm.ExpectQuery(re("FROM perdiem_payment AS pp")).WillReturnRows(sqlmock.NewRows([]string{"j", "c", "a"}))
	em.ExpectQuery(re("FROM cc_txn AS c")).WillReturnError(errors.New("down"))
	l, err := r.Ledger(context.Background(), domain.BudgetLedgerFilter{From: "2027-01-01", To: "2027-12-31", Unassigned: true})
	if err != nil || l.ExpenseAvailable || len(l.Costs) != 1 || l.Unassigned == nil || len(*l.Unassigned) != 0 {
		t.Fatalf("ledger = %+v, err = %v", l, err)
	}
}
