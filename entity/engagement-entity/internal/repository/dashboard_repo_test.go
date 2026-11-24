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
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

type dashboardMocks struct {
	alloc, travel, expense sqlmock.Sqlmock
	repo                   DashboardRepository
}

func newDashboardMocks(t *testing.T) dashboardMocks {
	t.Helper()
	open := func() (*sql.DB, sqlmock.Sqlmock) {
		db, mock := mockDB(t)
		return db, mock
	}
	a, am := open()
	tr, tm := open()
	e, em := open()
	return dashboardMocks{alloc: am, travel: tm, expense: em, repo: NewDashboardRepository(a, tr, e)}
}

func (m dashboardMocks) verify(t *testing.T) {
	t.Helper()
	for name, mock := range map[string]sqlmock.Sqlmock{"alloc": m.alloc, "travel": m.travel, "expense": m.expense} {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

var dashboardTestWindow = DashboardWindow{
	Today: "2026-11-05", Tomorrow: "2026-11-06", In7Days: "2026-11-13", In30Days: "2026-12-06",
	MonthStart: "2026-11-01", NextMonth: "2026-12-01", YearStart: "2025-12-01", BenchEndDay: "2026-11-18",
}

// The "today" variant is the legacy query verbatim: curdate(), the today
// predicates, no date arguments.
func TestDashboardAllocationsToday_TodayVariantIsLegacy(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`allocation.start_date <= curdate() AND allocation.end_date >= curdate()
			AND (engagement.engagement_status in ('Confirmed', 'In Progress') OR engagement.status in ('New', 'In Progress'))
			AND allocation.status = 'Active' AND allocation.allocation_status in ('Active', 'Confirmed') GROUP BY engagement_type.id`)).
		WithArgs().
		WillReturnRows(sqlmock.NewRows([]string{"allocationCount", "engagementType", "engagementTypeId"}).
			AddRow(12, "Internal Engagement", 1).AddRow(4, "Paid Post Sale", 6))
	m.alloc.ExpectQuery(re(`AND engagement_type.name IN ('Paid', 'Paid Pre Sale', 'Paid Post Sale') GROUP BY allocation_type.allocation_type, allocation_type.color`)).
		WithArgs().
		WillReturnRows(sqlmock.NewRows([]string{"allocationCount", "allocationTypeName", "allocationTypeColor"}).
			AddRow(4, "Implementation", "#123456"))
	m.alloc.ExpectQuery(re(`AND engagement_type.name NOT IN ('Paid', 'Paid Pre Sale', 'Paid Post Sale') GROUP BY allocation_type.allocation_type, allocation_type.color`)).
		WithArgs().
		WillReturnRows(sqlmock.NewRows([]string{"allocationCount", "allocationTypeName", "allocationTypeColor"}).
			AddRow(12, "R&D", nil))
	m.alloc.ExpectQuery(re(`SELECT count(DISTINCT allocation.email) AS consultantCount`)).
		WithArgs().
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(14))

	got, err := m.repo.AllocationsToday(context.Background(), domain.DashboardAllocationsTodayFilter{})
	if err != nil {
		t.Fatalf("AllocationsToday: %v", err)
	}
	if len(got.ByEngagementType) != 2 || got.ByEngagementType[1].EngagementTypeID != 6 || got.ByEngagementType[0].Count != 12 {
		t.Errorf("byEngagementType = %+v", got.ByEngagementType)
	}
	if len(got.PaidByAllocationType) != 1 || *got.PaidByAllocationType[0].Color != "#123456" {
		t.Errorf("paid = %+v", got.PaidByAllocationType)
	}
	if len(got.NonPaidByAllocationType) != 1 || got.NonPaidByAllocationType[0].Color != nil {
		t.Errorf("nonPaid = %+v", got.NonPaidByAllocationType)
	}
	if got.Consultants != 14 {
		t.Errorf("consultants = %d", got.Consultants)
	}
	m.verify(t)
}

// The past-date variant binds the date twice and the business unit id.
func TestDashboardAllocationsToday_PastDateAndBusinessUnit(t *testing.T) {
	m := newDashboardMocks(t)
	past := `AND allocation.start_date <= ? AND allocation.end_date >= ?
			AND engagement.engagement_status != 'Cancelled' AND engagement.status != 'Cancelled'
			AND allocation.status != 'Deleted'`
	m.alloc.ExpectQuery(re(past+` AND engagement_funding_source.business_unit_id = ? GROUP BY engagement_type.id`)).
		WithArgs("2026-10-01", "2026-10-01", 7).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c"}))
	m.alloc.ExpectQuery(re(past+dashboardPaidTypes+dashboardBUFilter)).
		WithArgs("2026-10-01", "2026-10-01", 7).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c"}))
	m.alloc.ExpectQuery(re(past+dashboardNonPaidTypes+dashboardBUFilter)).
		WithArgs("2026-10-01", "2026-10-01", 7).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c"}))
	m.alloc.ExpectQuery(re(past+dashboardBUFilter)).
		WithArgs("2026-10-01", "2026-10-01", 7).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))

	got, err := m.repo.AllocationsToday(context.Background(), domain.DashboardAllocationsTodayFilter{Date: "2026-10-01", BusinessUnitID: 7})
	if err != nil {
		t.Fatalf("AllocationsToday: %v", err)
	}
	if got.ByEngagementType == nil || got.PaidByAllocationType == nil || got.NonPaidByAllocationType == nil {
		t.Errorf("empty lists must be [], got %+v", got)
	}
	m.verify(t)
}

func TestDashboardOwnAllocationsAndTeamAllocation(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`WHERE a.email = ? AND a.engagement_id NOT IN ('-1', '-1.0') AND a.status = 'Active'`)).
		WithArgs("jane@example.com", "2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"id", "eid", "type", "acc", "title", "s", "e", "cl"}).
			AddRow("A1", "E1", 6, "Acme", "Acme API rollout", "2026-11-01", "2026-11-20", "Confirmed").
			AddRow("A2", "E2", nil, nil, nil, "2026-12-01", "2026-12-05", nil))
	m.alloc.ExpectQuery(re(`a.engagement_id IN ('-1', '-1.0') AND a.status != 'Deleted'
			AND a.start_date < ? AND a.end_date >= ?
			AND COALESCE(aT.allocation_type, '') != 'Engagement Overhead'`)).
		WithArgs("jane@example.com", "2026-11-06", "2026-11-05").
		WillReturnError(sql.ErrNoRows)

	allocs, err := m.repo.OwnAllocations(context.Background(), "jane@example.com", "2026-11-05")
	if err != nil || len(allocs) != 2 || *allocs[0].EngagementTypeID != 6 || allocs[1].EngagementTypeID != nil {
		t.Fatalf("OwnAllocations = %+v, %v", allocs, err)
	}
	team, err := m.repo.TeamAllocationOn(context.Background(), "jane@example.com", "2026-11-05", "2026-11-06")
	if err != nil || team != nil {
		t.Fatalf("TeamAllocationOn = %+v, %v", team, err)
	}
	m.verify(t)
}

func TestDashboardPendingApprovalAndClearanceCounts(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`WHERE e.status = 'Pending Approval' AND aT.engagement_type IN (?, ?)`)).
		WithArgs(1, 5).
		WillReturnRows(sqlmock.NewRows([]string{"t", "n"}).AddRow(1, 3).AddRow(5, 2))
	m.alloc.ExpectQuery(re(`WHERE e.status != 'Deleted' AND a.status = 'Active' AND a.clearance_status IN ('Ready for Clearance', 'Clearance In Progress')
			AND aT.engagement_type IN (?, ?, ?)`)).
		WithArgs(3, 4, 6).
		WillReturnRows(sqlmock.NewRows([]string{"s", "n"}).AddRow("Ready for Clearance", 7))

	pend, err := m.repo.PendingApprovalCounts(context.Background(), []int{1, 5})
	if err != nil || pend[1] != 3 || pend[5] != 2 {
		t.Fatalf("PendingApprovalCounts = %v, %v", pend, err)
	}
	cl, err := m.repo.ClearanceCounts(context.Background(), []int{3, 4, 6})
	if err != nil || cl["Ready for Clearance"] != 7 || cl["Clearance In Progress"] != 0 {
		t.Fatalf("ClearanceCounts = %v, %v", cl, err)
	}
	// No types: no query (no IN ()).
	if got, err := m.repo.PendingApprovalCounts(context.Background(), nil); err != nil || len(got) != 0 {
		t.Errorf("empty types = %v, %v", got, err)
	}
	if got, err := m.repo.ClearanceCounts(context.Background(), nil); err != nil || len(got) != 0 {
		t.Errorf("empty types = %v, %v", got, err)
	}
	m.verify(t)
}

func TestDashboardEngagementsStarting(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`WHERE e.status NOT IN ('Pending Approval', 'Cancelled') AND COALESCE(e.engagement_status, '') != 'Cancelled'
			AND aT.engagement_type IN (?, ?)
			AND e.start_date >= ? AND e.start_date < ?
		ORDER BY e.start_date, e.id
		LIMIT ?`)).
		WithArgs(3, 7, "2026-11-05", "2026-12-06", 6).
		WillReturnRows(sqlmock.NewRows([]string{"id", "t", "tn", "acc", "title", "at", "code", "loc", "c", "s", "e", "st", "es", "n", "total"}).
			AddRow("E7", 7, "Paid Pre Sale", "Acme", "Acme POC", "PoC", "SA-1", "London", "United Kingdom",
				"2026-11-10", "2026-11-14", "Approved", "Confirmed", 2, 9))

	got, total, err := m.repo.EngagementsStarting(context.Background(), []int{3, 7}, "2026-11-05", "2026-12-06", 6)
	if err != nil || len(got) != 1 || total != 9 || got[0].Consultants != 2 || *got[0].EngagementTypeID != 7 {
		t.Fatalf("EngagementsStarting = %+v, %d, %v", got, total, err)
	}
	m.verify(t)
}

func TestDashboardBenchAndTrend(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`FROM employee AS emp`)).
		WithArgs("2026-11-18", "2026-11-05", "2026-11-18", "2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"bu", "c", "a"}).AddRow("IAM", 10, 4))
	m.alloc.ExpectQuery(re(`SELECT DATE_FORMAT(a.start_date, '%Y-%m') AS d_month, COUNT(*)`)).
		WithArgs("2025-12-01", "2026-11-06").
		WillReturnRows(sqlmock.NewRows([]string{"m", "n"}).AddRow("2026-10", 40))
	m.alloc.ExpectQuery(re(`INNER JOIN business_unit AS bu ON efs.business_unit_id = bu.id`)).
		WithArgs("2025-12-01", "2026-11-06").
		WillReturnRows(sqlmock.NewRows([]string{"bu", "n"}).AddRow("IAM", 12))

	bench, err := m.repo.Bench(context.Background(), "2026-11-05", "2026-11-18")
	if err != nil || len(bench) != 1 || bench[0] != (domain.DashboardBenchRow{BusinessUnit: "IAM", Consultants: 10, Allocated: 4}) {
		t.Fatalf("Bench = %+v, %v", bench, err)
	}
	byMonth, err := m.repo.AllocationsByStartMonth(context.Background(), "2025-12-01", "2026-11-06")
	if err != nil || len(byMonth) != 1 || byMonth[0].Count != 40 {
		t.Fatalf("AllocationsByStartMonth = %+v, %v", byMonth, err)
	}
	byBU, err := m.repo.AllocationsByBusinessUnit(context.Background(), "2025-12-01", "2026-11-06")
	if err != nil || len(byBU) != 1 || byBU[0].Label != "IAM" {
		t.Fatalf("AllocationsByBusinessUnit = %+v, %v", byBU, err)
	}
	m.verify(t)
}

func TestDashboardUpcomingTrips_Filters(t *testing.T) {
	cols := []string{"job", "st", "who", "city", "country", "s", "e", "agency", "total"}
	tests := []struct {
		name string
		f    DashboardTripFilter
		sql  string
		args []driver.Value
	}{
		{"everyone", DashboardTripFilter{Today: "2026-11-05", Limit: 5},
			`WHERE COALESCE(t.status, '') != 'Cancelled' AND t.end_date >= ? ORDER BY t.start_date, t.job_number LIMIT ?`,
			[]driver.Value{"2026-11-05", 5}},
		{"traveller", DashboardTripFilter{Today: "2026-11-05", Email: "jane@example.com", Limit: 3},
			`AND t.end_date >= ? AND t.traveller_email = ? ORDER BY`, []driver.Value{"2026-11-05", "jane@example.com", 3}},
		{"agency", DashboardTripFilter{Today: "2026-11-05", AgencyID: 2, Limit: 5},
			`AND t.end_date >= ? AND t.travel_agent_id = ? ORDER BY`, []driver.Value{"2026-11-05", 2, 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newDashboardMocks(t)
			m.travel.ExpectQuery(re(tc.sql)).WithArgs(tc.args...).
				WillReturnRows(sqlmock.NewRows(cols).
					AddRow("TRV-2026-11-0001", "New", "jane@example.com", "Paris", "France", "2026-11-10", "2026-11-12", nil, 4))
			got, total, err := m.repo.UpcomingTrips(context.Background(), tc.f)
			if err != nil || len(got) != 1 || total != 4 || got[0].Agency != nil {
				t.Fatalf("UpcomingTrips = %+v, %d, %v", got, total, err)
			}
			m.verify(t)
		})
	}
}

func TestDashboardQueueCounts(t *testing.T) {
	m := newDashboardMocks(t)
	m.travel.ExpectQuery(re(`SELECT COUNT(*) FROM travel WHERE status = 'New'`)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(5))
	m.travel.ExpectQuery(re(`WHEN i.status = 'Accepted by finance' AND ` + dashboardInvoiceFailed + ` THEN 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d"}).AddRow(1, 2, 3, 4))
	m.travel.ExpectQuery(re(`FROM perdiem_payment AS pp
		WHERE pp.ns_status IN ('Pending', 'Failed')`)).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow(6, 1))

	n, err := m.repo.NewTravelRequestCount(context.Background())
	if err != nil || n != 5 {
		t.Fatalf("NewTravelRequestCount = %d, %v", n, err)
	}
	inv, err := m.repo.InvoiceQueueCounts(context.Background())
	if err != nil || inv != (DashboardInvoiceQueues{1, 2, 3, 4}) {
		t.Fatalf("InvoiceQueueCounts = %+v, %v", inv, err)
	}
	push, failed, err := m.repo.PaymentQueueCounts(context.Background())
	if err != nil || push != 6 || failed != 1 {
		t.Fatalf("PaymentQueueCounts = %d, %d, %v", push, failed, err)
	}
	m.verify(t)
}

func TestDashboardTravelKPIsAndCells(t *testing.T) {
	m := newDashboardMocks(t)
	w := dashboardTestWindow
	m.travel.ExpectQuery(re(`WHEN t.status IN ('In Progress', 'Extended') AND t.start_date < ? AND t.end_date >= ? THEN 1`)).
		WithArgs(w.Tomorrow, w.Today, w.Tomorrow, w.In30Days, w.MonthStart, w.NextMonth, w.YearStart, w.Tomorrow, w.Today).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(1, 2, 3, 4, 5))
	m.travel.ExpectQuery(re(`GROUP BY d_month, d_country, ta.name_of_travel_agent`)).
		WithArgs(w.YearStart, w.Tomorrow).
		WillReturnRows(sqlmock.NewRows([]string{"m", "c", "a", "n"}).
			AddRow("2026-10", "France", "Birch Travels", 2).AddRow("2026-11", nil, nil, 1))

	k, err := m.repo.TravelKPIs(context.Background(), w)
	if err != nil || k != (DashboardTravelKPIs{1, 2, 3, 4, 5}) {
		t.Fatalf("TravelKPIs = %+v, %v", k, err)
	}
	cells, err := m.repo.TravelCells(context.Background(), w)
	if err != nil || len(cells) != 2 || *cells[0].Agency != "Birch Travels" || cells[1].Agency != nil || cells[1].Country != "" {
		t.Fatalf("TravelCells = %+v, %v", cells, err)
	}
	m.verify(t)
}

func TestDashboardCostLines(t *testing.T) {
	m := newDashboardMocks(t)
	w := dashboardTestWindow
	cols := []string{"m", "t", "c", "a"}
	m.travel.ExpectQuery(re(`UPPER(TRIM(COALESCE(i.invoice_currency, ''))) AS d_currency,
			SUM(i.invoice_amount) AS amount
		FROM invoice AS i
		LEFT JOIN inv_cost_type AS ct ON i.inv_cost_type_id = ct.id
		WHERE i.added_date >= ? AND i.added_date < ?
			AND COALESCE(i.status, '') NOT IN ('Rejected by travel', 'Rejected by finance')
			AND COALESCE(ct.name, '') != 'Currency'`)).
		WithArgs(w.YearStart, w.NextMonth).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("2026-10", "Air Ticket", "USD", 1850.5).AddRow("2026-10", "Hotel", "LKR", 90000))
	m.travel.ExpectQuery(re(`UPPER(TRIM(COALESCE(pp.payment_currency, ''))) AS d_currency,
			SUM(pp.payment_amount) AS amount
		FROM perdiem_payment AS pp
		WHERE pp.added_date >= ? AND pp.added_date < ?`)).
		WithArgs(w.YearStart, w.NextMonth).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("2026-10", "", "USD", 400))
	m.expense.ExpectQuery(re(`UPPER(TRIM(COALESCE(c.txn_currency, ''))) AS d_currency,
			CASE WHEN UPPER(TRIM(COALESCE(c.cc_currency, ''))) = 'USD' THEN 'USD' ELSE '' END AS d_cc_currency,
			SUM(c.txn_amount) AS amount, SUM(c.cc_amount) AS cc_amount
		FROM cc_txn AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND COALESCE(c.status, '') != 'invalid'
		GROUP BY d_month, d_currency, d_cc_currency`)).
		WithArgs(w.YearStart, w.NextMonth).
		WillReturnRows(sqlmock.NewRows([]string{"m", "t", "c", "cc", "a", "ca"}).
			AddRow("2026-09", "", "LKR", "USD", 15000.0, 48.2).
			AddRow("2026-09", "", "EUR", "", 90.0, nil))

	inv, err := m.repo.InvoiceCost(context.Background(), w)
	if err != nil || len(inv) != 2 || inv[0].Source != domain.DashboardCostInvoice || inv[0].Amount != 1850.5 || inv[1].Currency != "LKR" || inv[1].Amount != 90000 {
		t.Fatalf("InvoiceCost = %+v, %v", inv, err)
	}
	pay, err := m.repo.PaymentCost(context.Background(), w)
	if err != nil || len(pay) != 1 || pay[0].Source != domain.DashboardCostPerDiem {
		t.Fatalf("PaymentCost = %+v, %v", pay, err)
	}
	card, err := m.repo.CardCost(context.Background(), w)
	if err != nil || len(card) != 2 || card[0].Source != domain.DashboardCostCard || card[0].Month != "2026-09" ||
		card[0].Currency != "LKR" || card[0].Amount != 15000 || card[0].CCCurrency != "USD" || card[0].CCAmount != 48.2 ||
		card[1].Currency != "EUR" || card[1].CCCurrency != "" || card[1].CCAmount != 0 {
		t.Fatalf("CardCost = %+v, %v", card, err)
	}
	m.verify(t)
}

func TestDashboardCardCost_NoExpensePool(t *testing.T) {
	r := NewDashboardRepository(nil, nil, nil)
	if _, err := r.CardCost(context.Background(), dashboardTestWindow); !errors.Is(err, ErrDashboardNoExpenseDB) {
		t.Errorf("err = %v", err)
	}
}

func TestDashboardAgency(t *testing.T) {
	m := newDashboardMocks(t)
	w := dashboardTestWindow
	m.travel.ExpectQuery(re(`INNER JOIN travel_agent_employee AS tae ON i.added_by = tae.wso2_email
		WHERE tae.travel_agent_id = ? AND i.job_number = t.job_number`)).
		WithArgs(w.Today, w.In7Days, w.Today, w.In30Days, 2, w.Today, 2).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(3, 1, 2, 1, 6))
	m.travel.ExpectQuery(re(`WHEN 'Completed' THEN 'Complete'`)).
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"s", "n"}).AddRow("Pending", 2).AddRow("Rejected by travel", 1))
	m.travel.ExpectQuery(re(`WHERE t.travel_agent_id = ? AND COALESCE(t.status, '') != 'Cancelled' AND t.start_date >= ? AND t.start_date < ?`)).
		WithArgs(2, w.YearStart, w.Tomorrow).
		WillReturnRows(sqlmock.NewRows([]string{"m", "n"}).AddRow("2026-10", 4))
	m.travel.ExpectQuery(re(`WHERE t.travel_agent_id = ? AND t.status IN ('In Progress', 'Extended') AND pa.status = 'Active'`)).
		WithArgs(2, 5).
		WillReturnRows(sqlmock.NewRows([]string{"j", "e", "s", "en", "d", "c", "t"}).
			AddRow("TRV-2026-10-1246", "jane@example.com", "2026-10-06", "2026-10-16", 11, "GBP", 770.0).
			AddRow("TRV-2026-10-1247", "member@example.com", nil, nil, nil, nil, nil))

	k, err := m.repo.AgencyKPIs(context.Background(), 2, w)
	if err != nil || k != (DashboardAgencyKPIs{3, 1, 2, 1, 6}) {
		t.Fatalf("AgencyKPIs = %+v, %v", k, err)
	}
	st, err := m.repo.AgencyInvoicesByStatus(context.Background(), 2)
	if err != nil || len(st) != 2 {
		t.Fatalf("AgencyInvoicesByStatus = %+v, %v", st, err)
	}
	bm, err := m.repo.AgencyTripsByMonth(context.Background(), 2, w)
	if err != nil || len(bm) != 1 {
		t.Fatalf("AgencyTripsByMonth = %+v, %v", bm, err)
	}
	pd, err := m.repo.AgencyPerDiems(context.Background(), 2, 5)
	if err != nil || len(pd) != 2 || *pd[0].Days != 11 || *pd[0].Total != 770 || pd[1].Total != nil {
		t.Fatalf("AgencyPerDiems = %+v, %v", pd, err)
	}
	m.verify(t)
}

func TestDashboardClaimCost(t *testing.T) {
	m := newDashboardMocks(t)
	w := dashboardTestWindow
	m.expense.ExpectQuery(re(`UPPER(TRIM(COALESCE(c.txn_currency, ''))) AS d_currency,
			SUM(c.txn_amount) AS amount
		FROM expense_claims AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND COALESCE(c.status, '') NOT IN ('-1', '1')`)).
		WithArgs(w.YearStart, w.NextMonth).
		WillReturnRows(sqlmock.NewRows([]string{"d_month", "d_cost_type", "d_currency", "amount"}).AddRow("2026-10", "", "LKR", 120.5))
	got, err := m.repo.ClaimCost(context.Background(), w)
	if err != nil || len(got) != 1 || got[0].Source != domain.DashboardCostClaim || got[0].Amount != 120.5 || got[0].Currency != "LKR" {
		t.Fatalf("ClaimCost = %+v, %v", got, err)
	}
	m.verify(t)
	if _, err := NewDashboardRepository(nil, nil, nil).ClaimCost(context.Background(), w); !errors.Is(err, ErrDashboardNoExpenseDB) {
		t.Errorf("no expense pool: err = %v", err)
	}
}
