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
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func financeMock(t *testing.T) (sqlmock.Sqlmock, *financeRepo, *financeAllocRepo, *financeExpenseRepo) {
	t.Helper()
	db, mock := mockDB(t)
	return mock, &financeRepo{db: db}, &financeAllocRepo{db: db}, &financeExpenseRepo{db: db}
}

func financeDone(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

var financeInvoiceCols = []string{"id", "invoice_number", "month", "inv_cost_type_id", "inv_acc_classification_type_id",
	"invoice_currency", "invoice_amount", "converted_currency", "converted_amount", "status",
	"added_by", "added_date", "updated_by", "updated_date", "reject_reason", "job_number",
	"hr_approved_by", "finance_approved_by", "account_internal_id", "department", "cost_type_name",
	"wso2_email", "vendor_id", "tA.id", "name_of_travel_agent",
	"traveller_email", "t.status", "city", "country", "start_date", "end_date", "allocation_id"}

func financeInvoiceRow(status string, reason any) []driver.Value {
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	return []driver.Value{7, "INV-1", "October", 1, 3, "USD", "155100.00", "LKR", "46530000.00", status,
		"agent@example.com", at, nil, nil, reason, "TRV-2026-11-0001",
		"po@example.com", "fin@example.com", "6001", "Sales", "Air Ticket",
		"agent@example.com", 9001, 4, "Birch Travels",
		"jane@example.com", "In Progress", "London", "United Kingdom", at, at, "A0000000001"}
}

func TestFinanceInvoices_FiltersOrderAndFailureMarker(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	f := domain.FinanceInvoiceFilter{
		Statuses: []string{"Accepted by finance"}, Failed: "false", Search: "50%", AgencyID: 4,
		AllocationIDs: []string{"A0000000001"}, AddedFrom: "2026-01-01", AddedTo: "2026-11-05", Limit: 10, Offset: 20,
	}
	where := "WHERE 1 = 1 AND i.status IN (?) AND NOT " + financeFailedInvoice
	mock.ExpectQuery(re("SELECT COUNT(*)"+financeInvoiceFrom+" "+where)).
		WithArgs("Accepted by finance", `%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`, "A0000000001", 4, "2026-01-01", "2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(31))
	mock.ExpectQuery(re("ORDER BY i.added_date DESC, i.invoice_number ASC, i.id DESC LIMIT ? OFFSET ?")).
		WillReturnRows(sqlmock.NewRows(financeInvoiceCols).
			AddRow(financeInvoiceRow("Accepted by finance", "NetSuite push failed: timeout")...))

	items, total, err := repo.Invoices(context.Background(), f)
	if err != nil {
		t.Fatalf("Invoices: %v", err)
	}
	if total != 31 || len(items) != 1 {
		t.Fatalf("total %d items %d", total, len(items))
	}
	inv := items[0]
	if !inv.NetSuiteFailed || inv.NetSuiteError == nil || *inv.NetSuiteError != "timeout" || inv.RejectReason != nil {
		t.Errorf("failure marker not split: %+v", inv)
	}
	if inv.InvoiceAmountText != "155100.00" || inv.InvoiceAmount != 155100 || *inv.VendorID != 9001 || *inv.AddedDate != "2026-11-04 10:00:00" {
		t.Errorf("invoice = %+v", inv)
	}
	financeDone(t, mock)
}

// A rejection reason on a rejected invoice stays a reject reason.
func TestFinanceInvoice_RejectReasonKept(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectQuery(re("WHERE i.id = ?")).WithArgs(7).
		WillReturnRows(sqlmock.NewRows(financeInvoiceCols).AddRow(financeInvoiceRow("Rejected by finance", "NetSuite push failed: x")...))
	inv, err := repo.Invoice(context.Background(), 7)
	if err != nil || inv == nil || inv.NetSuiteFailed || inv.RejectReason == nil {
		t.Fatalf("inv = %+v, err = %v", inv, err)
	}
	financeDone(t, mock)
}

func TestFinanceInvoiceCounts(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectQuery(re("COALESCE(SUM(i.status = 'Pending'), 0)")).WithArgs(4).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(1, 2, 3, 4, 10))
	c, err := repo.InvoiceCounts(context.Background(), 4)
	if err != nil || c != (domain.FinanceInvoiceCounts{Pending: 1, AcceptedByTravel: 2, AcceptedByFinance: 3, Failed: 4, All: 10}) {
		t.Errorf("counts = %+v, %v", c, err)
	}
	financeDone(t, mock)
}

func TestFinanceDecideInvoice(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	reason := "Wrong amount → fix"
	mock.ExpectBegin()
	mock.ExpectExec(re("UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = ?, finance_approved_by = ?, reject_reason = ? WHERE id = ? AND status = ?")).
		WithArgs("fin@example.com", "Rejected by finance", "fin@example.com", "Wrong amount -> fix", 7, "Accepted by travel").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	ok, err := repo.DecideInvoice(context.Background(), 7, FinanceInvoiceTransition{
		From: "Accepted by travel", To: "Rejected by finance", ApproverColumn: "finance_approved_by", Actor: "fin@example.com", Reason: &reason,
	})
	if err != nil || !ok {
		t.Errorf("ok = %v, err = %v", ok, err)
	}
	if _, err := repo.DecideInvoice(context.Background(), 7, FinanceInvoiceTransition{ApproverColumn: "status"}); err == nil {
		t.Error("a bad approver column must be refused")
	}
	financeDone(t, mock)
}

func TestFinanceInvoicePushTransitions(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	ctx := context.Background()
	mock.ExpectBegin()
	mock.ExpectExec(re("status = 'Complete', reject_reason = ? WHERE id = ? AND status = 'Accepted by finance' AND COALESCE(reject_reason, '') = ''")).
		WithArgs("p@example.com", "NetSuite push in progress", 7).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	won, err := repo.ClaimInvoicePush(ctx, 7, "p@example.com")
	if err != nil || won {
		t.Errorf("claim = %v, %v; want lost", won, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(re("status = 'Accepted by finance', reject_reason = NULL WHERE id = ? AND status = 'Complete' AND reject_reason = ?")).
		WithArgs("p@example.com", 7, "NetSuite push in progress").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.ReleaseInvoicePush(ctx, 7, "p@example.com"); err != nil {
		t.Error(err)
	}

	long := strings.Repeat("x", 400)
	mock.ExpectBegin()
	mock.ExpectExec(re("status = 'Accepted by finance', reject_reason = ? WHERE id = ? AND status = 'Complete'")).
		WithArgs("p@example.com", (domain.InvoiceNSFailurePrefix + long)[:domain.InvoiceMaxRejectReason], 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.FailInvoicePush(ctx, 7, "p@example.com", long); err != nil {
		t.Error(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(re("reject_reason = ? WHERE id = ? AND status = 'Accepted by finance' AND COALESCE(reject_reason, '') LIKE 'NetSuite push failed:%'")).
		WithArgs("p@example.com", "Complete", "NetSuite PO: 4400", 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if ok, err := repo.ResolveInvoiceFailure(ctx, 7, "p@example.com", "4400"); err != nil || !ok {
		t.Errorf("resolve = %v, %v", ok, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re("UPDATE invoice SET reject_reason = ? WHERE id = ? AND status = 'Complete'")).
		WithArgs("NetSuite PO: 5501", 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.CompleteInvoicePush(ctx, 7, "5501"); err != nil {
		t.Error(err)
	}
	financeDone(t, mock)
}

func TestFinanceClassificationIDBranches(t *testing.T) {
	ct := 1
	for name, tc := range map[string]struct {
		code, suffix string
		ct           *int
		sql          string
		args         []driver.Value
	}{
		"code and type": {"%E07201%", "", &ct, "AND engagement_code LIKE ? AND inv_cost_type_id=? ORDER BY id LIMIT 1",
			[]driver.Value{"%E07201%", 1}},
		"suffix and type": {"", "%07201%", &ct, "AND engagement_code_suffix LIKE ? AND inv_cost_type_id=? ORDER BY id LIMIT 1",
			[]driver.Value{"%07201%", 1}},
		"travel insurance": {"ALL", "", nil, "WHERE status='active' AND engagement_code LIKE ? ORDER BY id LIMIT 1",
			[]driver.Value{"ALL"}},
	} {
		t.Run(name, func(t *testing.T) {
			mock, repo, _, _ := financeMock(t)
			mock.ExpectQuery(re(tc.sql)).WithArgs(tc.args...).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
			id, err := repo.ClassificationID(context.Background(), tc.code, tc.suffix, tc.ct)
			if err != nil || id == nil || *id != 9 {
				t.Errorf("id = %v, err = %v", id, err)
			}
			financeDone(t, mock)
		})
	}
}

func TestFinanceInsertPerDiemWritesAuditInOneTransaction(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(re("INSERT into perdiem_allocation")).
		WithArgs("E07201", "TRV-2026-11-0001", "GBP", 70.0, 770.0, "2026-11-06 09:00:00", "2026-11-16 18:00:00", 11, "Active").
		WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectExec(re("INSERT INTO perdiem_auditlog VALUES (?, ?, ?, ?, ?, NOW())")).
		WithArgs("Perdiem Allocation", "po@example.com", "E07201", "TRV-2026-11-0001", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	id, err := repo.InsertPerDiem(context.Background(), "TRV-2026-11-0001", "E07201", domain.FinancePerDiemCreate{
		Currency: "GBP", PerDiemValue: 70, Total: 770, StartDate: "2026-11-06 09:00:00", EndDate: "2026-11-16 18:00:00",
		NoOfDays: 11, CreatedBy: "po@example.com",
	})
	if err != nil || id != 42 {
		t.Errorf("id = %d, err = %v", id, err)
	}
	financeDone(t, mock)
}

func TestFinanceInsertInvoicesTransaction(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(re("INSERT into invoice")).
		WithArgs("INV-1", "October", 1, 3, "USD", "1850.5", "USD", "1850.5", "Pending", "agent@example.com", "TRV-2026-11-0001").
		WillReturnResult(sqlmock.NewResult(8, 1))
	mock.ExpectCommit()
	ids, err := repo.InsertInvoices(context.Background(), "TRV-2026-11-0001", "agent@example.com", []FinanceInvoiceInsert{{
		FinanceInvoiceCreate: domain.FinanceInvoiceCreate{InvoiceNumber: "INV-1", Month: "October", CostTypeID: 1,
			InvoiceCurrency: "USD", InvoiceAmount: 1850.5, ConvertedCurrency: "USD", ConvertedAmount: 1850.5},
		ClassificationID: 3,
	}})
	if err != nil || len(ids) != 1 || ids[0] != 8 {
		t.Errorf("ids = %v, err = %v", ids, err)
	}
	financeDone(t, mock)
}

func TestFinancePaymentBasis(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	at := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(re("WHERE job_number = ? AND status = 'ACTIVE' GROUP BY currency ORDER BY MIN(id)")).WithArgs("TRV-2026-11-0001").
		WillReturnRows(sqlmock.NewRows([]string{"t", "c"}).AddRow(770.0, "GBP"))
	mock.ExpectQuery(re("status = 'Complete' AND COALESCE(reject_reason, '') <> ? AND inv_cost_type_id = (SELECT id FROM inv_cost_type WHERE status='active' AND name='Currency' LIMIT 1)")).
		WithArgs("TRV-2026-11-0001", domain.InvoiceNSPushingMarker).WillReturnRows(sqlmock.NewRows([]string{"a", "c", "d"}).AddRow("100.00", "USD", at))
	mock.ExpectQuery(re("FROM travel_card WHERE job_number = ?")).WithArgs("TRV-2026-11-0001").
		WillReturnRows(sqlmock.NewRows([]string{"a", "c", "d"}).AddRow(50.0, "GBP", at))
	mock.ExpectQuery(re("FROM perdiem_payment WHERE job_number = ?")).WithArgs("TRV-2026-11-0001").
		WillReturnRows(sqlmock.NewRows([]string{"a", "c", "d"}))
	b, err := repo.PaymentBasis(context.Background(), "TRV-2026-11-0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.PerDiemTotals) != 1 || b.CurrencyInvoices[0].Amount != 100 || b.TravelCards[0].AddedDate != "2026-11-01 08:00:00" ||
		b.PaymentCount != 0 || b.Payments == nil {
		t.Errorf("basis = %+v", b)
	}
	financeDone(t, mock)
}

func TestFinancePaymentsAndTransitions(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	ctx := context.Background()
	cols := []string{"id", "job", "num", "cur", "amt", "ns", "ccur", "camt", "by", "at", "uby", "uat", "trav", "ts", "alloc"}
	mock.ExpectQuery(re("SELECT COUNT(*)"+financePaymentFrom+" WHERE 1 = 1 AND pp.ns_status IN (?, ?) AND t.traveller_email IN (?)")).
		WithArgs("Pending", "Failed", "jane@example.com").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery(re("ORDER BY pp.added_date DESC")).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(3, "TRV-2026-11-0001", "TRV-2026-11-0001-001", "GBP", 400.0, "Pending",
			"LKR", "152000.50", "fin@example.com", time.Now(), nil, nil, "jane@example.com", "In Progress", "A1"))
	items, total, err := repo.Payments(ctx, domain.FinancePaymentFilter{NSStatuses: []string{"Pending", "Failed"}, Travellers: []string{"jane@example.com"}})
	if err != nil || total != 1 || *items[0].ConvertedAmountText != "152000.50" {
		t.Fatalf("items = %+v, %v", items, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(re("SET ns_status = 'Pushing', last_updated_by = ?, last_updated_date = NOW() WHERE id = ? AND ns_status = 'Pending'")).
		WithArgs("p@example.com", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if won, err := repo.ClaimPaymentPush(ctx, 3, "p@example.com"); err != nil || !won {
		t.Errorf("claim = %v, %v", won, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re("SET ns_status = 'Failed', last_updated_by = ?, last_updated_date = NOW() WHERE id = ? AND ns_status = 'Pushing'")).
		WithArgs("p@example.com", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.FailPaymentPush(ctx, 3, "p@example.com"); err != nil {
		t.Error(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re("SET ns_status = 'Complete', last_updated_by = ?, last_updated_date = NOW() WHERE id = ? AND ns_status = 'Pushing'")).
		WithArgs("p@example.com", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.CompletePaymentPush(ctx, 3, "p@example.com"); err != nil {
		t.Error(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re("WHERE id = ? AND ns_status = 'Failed'")).WithArgs("Pending", "p@example.com", 3).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if ok, err := repo.ResolvePaymentFailure(ctx, 3, "p@example.com", false); err != nil || !ok {
		t.Errorf("resolve = %v, %v", ok, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(re("WHERE id = ? AND ns_status = 'Pending'")).
		WithArgs(3, 500.0, "LKR", "190000.25", "e@example.com", 3).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if ok, err := repo.UpdatePayment(ctx, 3, domain.FinancePaymentUpdate{PaymentAmount: 500, ConvertedCurrency: "LKR",
		ConvertedAmount: 190000.25, UpdatedBy: "e@example.com"}); err != nil || ok {
		t.Errorf("update = %v, %v; want not pending", ok, err)
	}
	financeDone(t, mock)
}

func TestFinanceTrip(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectQuery(re("FROM travel t LEFT JOIN travel_agents ta ON ta.id = t.travel_agent_id WHERE t.job_number = ?")).
		WithArgs("TRV-2026-11-0001").
		WillReturnRows(sqlmock.NewRows([]string{"j", "s", "e", "a", "n", "al", "c", "co", "sd", "ed", "cu", "ec"}).
			AddRow("TRV-2026-11-0001", "In Progress", "jane@example.com", 4, "Birch Travels", "A1", "London", "UK", nil, nil, "Acme", "E1"))
	trip, err := repo.Trip(context.Background(), "TRV-2026-11-0001")
	if err != nil || trip == nil || *trip.AgencyID != 4 || *trip.Status != "In Progress" {
		t.Fatalf("trip = %+v, %v", trip, err)
	}
	mock.ExpectQuery(re("WHERE t.job_number = ?")).WillReturnRows(sqlmock.NewRows([]string{"j"}))
	if trip, err := repo.Trip(context.Background(), "TRV-2026-11-0002"); err != nil || trip != nil {
		t.Errorf("missing trip = %+v, %v", trip, err)
	}
	financeDone(t, mock)
}

func TestFinanceAllocationQueries(t *testing.T) {
	mock, _, alloc, expense := financeMock(t)
	ctx := context.Background()
	if m, err := alloc.Campaigns(ctx, nil); err != nil || len(m) != 0 {
		t.Errorf("empty campaigns = %v, %v", m, err)
	}
	mock.ExpectQuery(re("WHERE a.id IN (?, ?) AND e.campaign_id IS NOT NULL AND e.campaign_id <> ''")).WithArgs("A1", "A2").
		WillReturnRows(sqlmock.NewRows([]string{"a", "c", "n"}).AddRow("A1", "701000000000001AAA", "Integration Summit"))
	m, err := alloc.Campaigns(ctx, []string{"A1", "A2"})
	if err != nil || m["A1"].ID != "701000000000001AAA" || *m["A1"].Name != "Integration Summit" {
		t.Errorf("campaigns = %v, %v", m, err)
	}
	mock.ExpectQuery(re("WHERE e.campaign_id IN (?)")).WithArgs("C1").WillReturnRows(sqlmock.NewRows([]string{"a"}).AddRow("A1"))
	if ids, err := alloc.AllocationsOfCampaigns(ctx, []string{"C1"}); err != nil || len(ids) != 1 {
		t.Errorf("ids = %v, %v", ids, err)
	}
	mock.ExpectQuery(re("INNER JOIN engagement e ON e.id = a.engagement_id WHERE a.id = ? LIMIT 1")).WithArgs("A1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "c", "cu", "p", "ca"}).AddRow("E1", "E-SALES-1", "Acme", "Summit", ""))
	e, err := alloc.EngagementOfAllocation(ctx, "A1")
	if err != nil || e.EngagementCode != "E-SALES-1" || e.CampaignID != nil {
		t.Errorf("engagement = %+v, %v", e, err)
	}
	mock.ExpectQuery(re("WHERE eng_fs.engagement_id IN (?)")).WithArgs("E1").
		WillReturnRows(sqlmock.NewRows([]string{"bu", "pu", "sr", "r", "p"}).AddRow("BU", nil, "UK", "Europe", "100.00"))
	if fs, err := alloc.FundingSources(ctx, "E1"); err != nil || fs[0].Percentage != "100.00" || fs[0].ProductUnit != nil {
		t.Errorf("funding = %+v, %v", fs, err)
	}
	mock.ExpectQuery(re("FROM subsidiaries WHERE status = 'active' AND code = ? LIMIT 1")).WithArgs("WSO2 - UK").
		WillReturnRows(sqlmock.NewRows([]string{"id", "l", "c", "co", "t"}).AddRow(3, "WSO2 UK Ltd", "GBP", "UK", "12"))
	if s, err := expense.Subsidiary(ctx, "WSO2 - UK"); err != nil || s.TaxCodeInternalID != "12" || s.CurrencyCode != "GBP" {
		t.Errorf("subsidiary = %+v, %v", s, err)
	}
	financeDone(t, mock)
}

func TestExpireStalePushes(t *testing.T) {
	mock, repo, _, _ := financeMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(re("status = 'Accepted by finance', reject_reason = ? WHERE status = 'Complete' AND reject_reason = ? AND updated_date < NOW() - INTERVAL ? SECOND")).
		WithArgs(sqlmock.AnyArg(), "NetSuite push in progress", 900).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(re("SET ns_status = 'Failed', last_updated_date = NOW() WHERE ns_status = 'Pushing' AND last_updated_date < NOW() - INTERVAL ? SECOND")).
		WithArgs(900).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := ExpireStalePushes(context.Background(), repo.db, 15*time.Minute); err != nil {
		t.Error(err)
	}
	financeDone(t, mock)
}

func TestTripExpenses(t *testing.T) {
	db, mock := mockDB(t)
	r := NewFinanceExpenseRepository(db)
	mock.ExpectQuery(re(`FROM cc_txn AS t
			LEFT JOIN expense_type AS et ON et.id = t.expense_type_id
		WHERE t.travel_job_number = ? AND COALESCE(t.status, '') != 'invalid'`)).WithArgs("TRV-2026-11-0001", financeMaxRows).
		WillReturnRows(sqlmock.NewRows([]string{"id", "date", "type", "ref", "amount", "cur", "email", "status"}).
			AddRow(4, "2026-11-02", "Taxi", "UBER", 18.4, "USD", "e@example.com", "submitted"))
	mock.ExpectQuery(re(`FROM expense_claims AS c
			LEFT JOIN expense_type AS et ON et.id = c.expense_type_id
		WHERE c.travel_job_number = ? AND COALESCE(c.status, '') != '-1'`)).WithArgs("TRV-2026-11-0001", financeMaxRows).
		WillReturnRows(sqlmock.NewRows([]string{"id", "num", "date", "type", "amount", "cur", "ramount", "rcur", "email", "status"}).
			AddRow(9, "1201", "2026-11-03", "Meals", 3000.0, "LKR", 10.1, "USD", "e@example.com", "3").
			AddRow(10, "1201", nil, nil, nil, nil, nil, nil, nil, "0"))
	got, err := r.TripExpenses(context.Background(), "TRV-2026-11-0001")
	if err != nil || len(got.CardExpenses) != 1 || len(got.ExpenseClaims) != 2 {
		t.Fatalf("TripExpenses = %+v, %v", got, err)
	}
	if c := got.CardExpenses[0]; *c.Amount != 18.4 || *c.ExpenseType != "Taxi" {
		t.Errorf("card = %+v", c)
	}
	if c := got.ExpenseClaims[0]; *c.ClaimNumber != "1201" || *c.ReimbursementCurrency != "USD" || *c.Status != "3" {
		t.Errorf("claim = %+v", c)
	}
	if c := got.ExpenseClaims[1]; c.Amount != nil || c.Date != nil {
		t.Errorf("nulls: %+v", c)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
