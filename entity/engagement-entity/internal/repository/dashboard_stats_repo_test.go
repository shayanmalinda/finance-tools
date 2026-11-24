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
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

var costTripCols = []string{"id", "ref", "job", "date", "cur", "amt", "ct", "t_job", "t_email", "t_country", "t_agent_id", "t_agent", "t_alloc", "t_customer", "t_type"}

func TestDashboardInvoiceLines(t *testing.T) {
	m := newDashboardMocks(t)
	f := domain.DashboardCostLinesFilter{From: "2026-01-01", To: "2026-11-05", JobNumbers: []string{"TRV-1"}, Employees: []string{"jane@example.com"}, Limit: 100}
	m.travel.ExpectQuery(re(`FROM invoice AS i
		LEFT JOIN inv_cost_type AS ct ON i.inv_cost_type_id = ct.id
		LEFT JOIN travel AS t ON i.job_number = t.job_number
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE i.added_date >= ? AND i.added_date < DATE_ADD(?, INTERVAL 1 DAY)
			AND COALESCE(i.status, '') NOT IN ('Rejected by travel', 'Rejected by finance')
			AND COALESCE(ct.name, '') != 'Currency' AND i.job_number IN (?) AND t.traveller_email IN (?) ORDER BY i.added_date, i.id LIMIT ?`)).
		WithArgs("2026-01-01", "2026-11-05", "TRV-1", "jane@example.com", 101).
		WillReturnRows(sqlmock.NewRows(costTripCols).
			AddRow(10764, "INV/1", " TRV-1 ", "2026-10-14", "LKR", 152000.0, "Air Ticket", "TRV-1", "jane@example.com", "India", 3, "Classic", "A-1", "Acme", "Paid").
			AddRow(10765, nil, nil, "2026-10-15", "USD", 10.0, "Other", nil, nil, nil, nil, nil, nil, nil, nil))
	got, err := m.repo.InvoiceLines(context.Background(), f)
	if err != nil || len(got) != 2 {
		t.Fatalf("InvoiceLines = %+v, %v", got, err)
	}
	a := got[0]
	if a.ID != "INV-10764" || a.Source != "invoice" || *a.JobNumber != "TRV-1" || a.Currency != "LKR" || a.Amount != 152000 ||
		a.CostType != "Air Ticket" || *a.Trip.TravellerEmail != "jane@example.com" || *a.Trip.AgentID != 3 || *a.Trip.AllocationID != "A-1" || *a.Trip.EngagementType != "Paid" {
		t.Errorf("first line = %+v", a)
	}
	if got[1].JobNumber != nil || got[1].Trip.JobNumber != nil || got[1].Trip.AgentID != nil {
		t.Errorf("unlinked line = %+v", got[1])
	}
	m.verify(t)
}

func TestDashboardPaymentLines(t *testing.T) {
	m := newDashboardMocks(t)
	f := domain.DashboardCostLinesFilter{From: "2026-01-01", To: "2026-11-05"}
	m.travel.ExpectQuery(re(`FROM perdiem_payment AS pp
		LEFT JOIN travel AS t ON pp.job_number = t.job_number
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE pp.added_date >= ? AND pp.added_date < DATE_ADD(?, INTERVAL 1 DAY) ORDER BY pp.added_date, pp.id LIMIT ?`)).
		WithArgs("2026-01-01", "2026-11-05", domain.DashboardCostLinesMax+1).
		WillReturnRows(sqlmock.NewRows(costTripCols[:6]).AddRow(7, "PD/1", "TRV-2", "2026-10-01", "EUR", 220.5))
	// The scan expects the trip columns too: a short row is a scan error.
	if _, err := m.repo.PaymentLines(context.Background(), f); err == nil {
		t.Fatal("a row without the trip columns scanned")
	}

	m = newDashboardMocks(t)
	m.travel.ExpectQuery(re(`FROM perdiem_payment AS pp`)).
		WillReturnRows(sqlmock.NewRows(append(append([]string{}, costTripCols[:6]...), costTripCols[7:]...)).
			AddRow(7, "PD/1", "TRV-2", "2026-10-01", "EUR", 220.5, "TRV-2", "jane@example.com", "France", nil, nil, "A-2", "Acme", "Paid Post Sale"))
	got, err := m.repo.PaymentLines(context.Background(), f)
	if err != nil || len(got) != 1 || got[0].ID != "PD-7" || got[0].CostType != "Per Diem" || got[0].Currency != "EUR" || got[0].Amount != 220.5 {
		t.Fatalf("PaymentLines = %+v, %v", got, err)
	}
	m.verify(t)
}

func TestDashboardCardAndClaimLines(t *testing.T) {
	m := newDashboardMocks(t)
	f := domain.DashboardCostLinesFilter{From: "2026-01-01", To: "2026-11-05", JobNumbers: []string{"TRV-1"}, Employees: []string{"a@example.com"}, Limit: 5}
	cols := []string{"id", "ref", "job", "date", "cur", "amt", "cc", "cc_amt", "email", "country", "type"}
	m.expense.ExpectQuery(re(`FROM cc_txn AS c
		LEFT JOIN expense_type AS et ON et.id = c.expense_type_id
		LEFT JOIN expense_category AS ec ON ec.id = et.expense_category_id
		WHERE c.txn_date >= ? AND c.txn_date < DATE_ADD(?, INTERVAL 1 DAY)
			AND COALESCE(c.status, '') != 'invalid'
			AND ((c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != '') OR ec.expense_category LIKE ?) AND TRIM(c.travel_job_number) IN (?) AND c.employee_email IN (?) ORDER BY c.txn_date, c.id LIMIT ?`)).
		WithArgs("2026-01-01", "2026-11-05", "%Travel%", "TRV-1", "a@example.com", 6).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, "REF1", "TRV-1", "2026-10-02", "LKR", 15000.0, "USD", 48.2, "a@example.com", "India", "CC-Travel - Airfare").
			AddRow(2, "REF2", "", "2026-10-03", "EUR", 90.0, "", 0.0, "a@example.com", "", nil))
	m.expense.ExpectQuery(re(`FROM expense_claims AS c`)).
		WithArgs("2026-01-01", "2026-11-05", "%Travel%", "TRV-1", "a@example.com", 6).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(9, "RPT-1", "TRV-1", "2026-10-04", "LKR", 1000.0, "", 0.0, "a@example.com", "India", "Meals"))

	card, err := m.repo.CardLines(context.Background(), f)
	if err != nil || len(card) != 2 {
		t.Fatalf("CardLines = %+v, %v", card, err)
	}
	if card[0].ID != "CC-1" || card[0].CostType != "Card" || card[0].CCCurrency != "USD" || card[0].CCAmount != 48.2 || *card[0].JobNumber != "TRV-1" ||
		*card[0].EmployeeEmail != "a@example.com" || *card[0].TravelCountry != "India" || *card[0].SourceType != "CC-Travel - Airfare" {
		t.Errorf("card[0] = %+v", card[0])
	}
	if card[1].JobNumber != nil || card[1].TravelCountry != nil || card[1].SourceType != nil {
		t.Errorf("card[1] = %+v", card[1])
	}
	claims, err := m.repo.ClaimLines(context.Background(), f)
	if err != nil || len(claims) != 1 || claims[0].ID != "EC-9" || claims[0].CostType != "Claim" || claims[0].Source != "claim" {
		t.Fatalf("ClaimLines = %+v, %v", claims, err)
	}
	m.verify(t)

	r := NewDashboardRepository(nil, nil, nil)
	if _, err := r.CardLines(context.Background(), f); !errors.Is(err, ErrDashboardNoExpenseDB) {
		t.Errorf("card without pool: %v", err)
	}
	if _, err := r.ClaimLines(context.Background(), f); !errors.Is(err, ErrDashboardNoExpenseDB) {
		t.Errorf("claim without pool: %v", err)
	}
}

// The line cap asks for one row more, to see whether it was hit.
func TestDashboardCostLimit(t *testing.T) {
	for in, want := range map[int]int{0: 20001, -3: 20001, 5: 6, 20000: 20001, 99999: 20001} {
		if got := dashboardCostLimit(domain.DashboardCostLinesFilter{Limit: in}); got != want {
			t.Errorf("limit %d -> %d, want %d", in, got, want)
		}
	}
}

// IN lists over 500 are split into several queries.
func TestDashboardTripsByJob_Chunks(t *testing.T) {
	m := newDashboardMocks(t)
	jobs := make([]string, 0, 1001)
	for i := 0; i < 1001; i++ {
		jobs = append(jobs, fmt.Sprintf("TRV-%d", i))
	}
	cols := []string{"job", "email", "country", "agent_id", "agent", "alloc", "customer", "type"}
	m.travel.ExpectQuery(re(`FROM travel AS t LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id WHERE t.job_number IN (`)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("trv-1", "a@example.com", "India", 3, "Classic", "A-1", "Acme", "Paid"))
	m.travel.ExpectQuery(re(`WHERE t.job_number IN (`)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("TRV-600", "b@example.com", nil, nil, nil, nil, nil, nil))
	m.travel.ExpectQuery(re(`WHERE t.job_number IN (?)`)).
		WithArgs("TRV-1000").
		WillReturnRows(sqlmock.NewRows(cols))
	got, err := m.repo.TripsByJob(context.Background(), jobs)
	if err != nil || len(got) != 2 || *got["TRV-1"].AgentName != "Classic" || got["TRV-600"].Country != nil {
		t.Fatalf("TripsByJob = %+v, %v", got, err)
	}
	m.verify(t)
}

func TestDashboardCostEngagements_Chunks(t *testing.T) {
	m := newDashboardMocks(t)
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = fmt.Sprintf("A-%d", i)
	}
	cols := []string{"aid", "eid", "customer", "type", "atype"}
	m.alloc.ExpectQuery(re(`JOIN allocation_type AS aT ON e.allocation_type = aT.id
			JOIN engagement_type AS eT ON aT.engagement_type = eT.id
			WHERE e.status != 'Deleted' AND a.id IN (`)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow("A-1", "E-1", "Acme", "Paid Post Sale", "Onsite"))
	m.alloc.ExpectQuery(re(`a.id IN (?)`)).WithArgs("A-500").
		WillReturnRows(sqlmock.NewRows(cols).AddRow("A-500", "E-2", nil, "Internal Engagement", nil))
	got, err := m.repo.CostEngagements(context.Background(), ids)
	if err != nil || len(got) != 2 || got["A-1"].EngagementID != "E-1" || *got["A-1"].AllocationType != "Onsite" || got["A-500"].CustomerName != nil {
		t.Fatalf("CostEngagements = %+v, %v", got, err)
	}
	m.verify(t)
}

func TestDashboardTravelTeam(t *testing.T) {
	m := newDashboardMocks(t)
	w := dashboardTestWindow
	w.In6Days, w.Minus3, w.Minus90, w.In60Days = "2026-11-12", "2026-11-03", "2026-08-07", "2027-01-05"
	m.travel.ExpectQuery(re(`WHERE COALESCE(t.status, '') != 'Cancelled' AND t.status NOT IN ('Deleted', '')
			AND t.start_date < ? AND t.end_date >= ?`)).
		WithArgs("2026-11-12", "2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"a", "b"}).AddRow(12, 5))
	m.travel.ExpectQuery(re(`WHERE t.status IN ('New', 'In Progress', 'Extended', 'On Hold')
			AND t.end_date < ? AND t.end_date >= ?`)).
		WithArgs("2026-11-03", "2026-08-07").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(4))
	m.travel.ExpectQuery(re(`FROM (SELECT DATEDIFF(?, i.added_date) AS d FROM invoice AS i WHERE i.status = 'Pending') AS x`)).
		WithArgs("2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e", "f"}).AddRow(2, 3, 3, 1, 120, 9))
	m.travel.ExpectQuery(re(`GROUP BY t.traveller_email ORDER BY COUNT(*) DESC, t.traveller_email LIMIT 8`)).
		WithArgs(w.YearStart, w.Tomorrow).
		WillReturnRows(sqlmock.NewRows([]string{"e", "n"}).AddRow("jane@example.com", 7))
	m.travel.ExpectQuery(re(`COALESCE(ta.name_of_travel_agent, 'No travel agent yet') AS d_agent`)).
		WithArgs("2026-11-05", "2027-01-05").
		WillReturnRows(sqlmock.NewRows([]string{"a", "n"}).AddRow("Classic", 5).AddRow("No travel agent yet", 2))

	tr, co, err := m.repo.TravelTeamAway(context.Background(), w)
	if err != nil || tr != 12 || co != 5 {
		t.Fatalf("TravelTeamAway = %d %d %v", tr, co, err)
	}
	if n, err := m.repo.TravelEndedNotCompleted(context.Background(), w); err != nil || n != 4 {
		t.Fatalf("TravelEndedNotCompleted = %d %v", n, err)
	}
	by, total, oldest, err := m.repo.POQueueByAge(context.Background(), "2026-11-05")
	if err != nil || by != [4]int{2, 3, 3, 1} || total != 9 || oldest != 120 {
		t.Fatalf("POQueueByAge = %v %d %d %v", by, total, oldest, err)
	}
	if f, err := m.repo.FrequentTravellers(context.Background(), w); err != nil || len(f) != 1 || f[0].Count != 7 {
		t.Fatalf("FrequentTravellers = %+v %v", f, err)
	}
	if a, err := m.repo.AgentWorkload(context.Background(), w); err != nil || len(a) != 2 || a[1].Label != "No travel agent yet" {
		t.Fatalf("AgentWorkload = %+v %v", a, err)
	}
	m.verify(t)
}

func TestDashboardManagerQueries(t *testing.T) {
	m := newDashboardMocks(t)
	engCols := []string{"id", "type", "tn", "cust", "title", "at", "code", "city", "country", "s", "e", "status", "es", "consultants", "total"}
	m.alloc.ExpectQuery(re(`SELECT x.*, COUNT(*) OVER () AS total FROM (SELECT e.id AS e_id`)).
		WithArgs(3, 4, "2026-11-05", "2026-12-06", 6).
		WillReturnRows(sqlmock.NewRows(engCols).AddRow("E-1", 3, "Paid Pre Sale", "Acme", "Rollout", "Onsite", nil, nil, nil, "2026-11-10", "2026-12-10", "New", nil, 0, 3))
	got, total, err := m.repo.EngagementsStartingWithout(context.Background(), []int{3, 4}, "2026-11-05", "2026-12-06", 6)
	if err != nil || total != 3 || len(got) != 1 || got[0].Consultants != 0 || *got[0].EngagementTypeID != 3 {
		t.Fatalf("EngagementsStartingWithout = %+v %d %v", got, total, err)
	}

	m.alloc.ExpectQuery(re(`WHERE x.consultants = 0`)).WithArgs("jane@example.com", "2026-11-05", "2026-11-05", 6).
		WillReturnRows(sqlmock.NewRows(engCols))
	if got, total, err := m.repo.OwnEngagementsRunningWithout(context.Background(), "jane@example.com", "2026-11-05", 6); err != nil || total != 0 || len(got) != 0 {
		t.Fatalf("OwnEngagementsRunningWithout = %+v %d %v", got, total, err)
	}

	m.alloc.ExpectQuery(re(`a.isTravelRequired, COUNT(*) OVER () AS total`)).
		WithArgs(3, "2026-11-06", "2026-11-20", 6).
		WillReturnRows(sqlmock.NewRows([]string{"a", "em", "e", "t", "c", "p", "s", "en", "tr", "total"}).
			AddRow("AL-1", "dev@example.com", "E-1", 3, "Acme", "Rollout", "2026-11-10", "2026-12-10", "Yes", 8))
	starting, total, err := m.repo.AllocationsStarting(context.Background(), []int{3}, "2026-11-06", "2026-11-20", 6)
	if err != nil || total != 8 || len(starting) != 1 || !starting[0].TravelRequired || starting[0].Email != "dev@example.com" {
		t.Fatalf("AllocationsStarting = %+v %d %v", starting, total, err)
	}

	// Window 2026-08-07 to 2026-11-05 (91 days). Only the days inside count.
	m.alloc.ExpectQuery(re(`a.start_date < ? AND a.end_date >= ?`)).
		WithArgs(3, "2026-11-06", "2026-08-07").
		WillReturnRows(sqlmock.NewRows([]string{"e", "s", "en"}).
			AddRow("dev@example.com", "2026-08-01", "2026-08-10"). // straddles the start: 4 days
			AddRow("dev@example.com", "2026-11-01", "2026-11-30"). // straddles the end: 5 days
			AddRow("dev@example.com", "2024-01-01", "2027-01-01"). // spans the window: 91 days
			AddRow("dev@example.com", "2026-09-01", "2026-09-10")) // inside: 10 days
	if got, err := m.repo.MostAllocated(context.Background(), []int{3}, "2026-08-07", "2026-11-06", 8); err != nil || len(got) != 1 || got[0].Count != 110 {
		t.Fatalf("MostAllocated = %+v %v", got, err)
	}

	// A customer type among the caller's types groups by customer, over the customer types only.
	m.alloc.ExpectQuery(re(`SELECT TRIM(e.customer_name) AS d_label`)).
		WithArgs(3, 4, "2026-11-06", "2025-12-01").
		WillReturnRows(sqlmock.NewRows([]string{"l", "s", "en"}).AddRow("Acme", "2025-12-01", "2025-12-10"))
	by, top, err := m.repo.TopCustomers(context.Background(), []int{1, 3, 4, 5}, "2025-12-01", "2026-11-06", 8)
	if err != nil || by != "customer" || len(top) != 1 || top[0].Label != "Acme" {
		t.Fatalf("TopCustomers = %q %+v %v", by, top, err)
	}
	// None of them: group by country over all the types.
	m.alloc.ExpectQuery(re(`SELECT TRIM(e.country) AS d_label`)).
		WithArgs(1, 5, "2026-11-06", "2025-12-01").
		WillReturnRows(sqlmock.NewRows([]string{"l", "s", "en"}).AddRow("India", "2025-12-01", "2025-12-10"))
	by, top, err = m.repo.TopCustomers(context.Background(), []int{1, 5}, "2025-12-01", "2026-11-06", 8)
	if err != nil || by != "country" || len(top) != 1 || top[0].Label != "India" {
		t.Fatalf("TopCustomers by country = %q %+v %v", by, top, err)
	}
	m.verify(t)
}

func TestDashboardAccountManagerQueries(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`ELSE COALESCE(NULLIF(e.engagement_status, ''), 'New') END AS d_status`)).
		WithArgs("jane@example.com", "2025-12-01").
		WillReturnRows(sqlmock.NewRows([]string{"s", "n"}).AddRow("In Progress", 4).AddRow("Pending Approval", 1))
	m.alloc.ExpectQuery(re(`WHERE LOWER(e.username) = ? AND e.status NOT IN ('Cancelled', 'Deleted')
			AND e.start_date >= ? AND e.start_date < ?`)).
		WithArgs("jane@example.com", "2026-11-01", "2027-05-01").
		WillReturnRows(sqlmock.NewRows([]string{"m", "n"}).AddRow("2026-12", 2))
	if got, err := m.repo.OwnEngagementsByStatus(context.Background(), "jane@example.com", "2025-12-01"); err != nil || len(got) != 2 {
		t.Fatalf("OwnEngagementsByStatus = %+v %v", got, err)
	}
	if got, err := m.repo.OwnEngagementsByMonth(context.Background(), "jane@example.com", "2026-11-01", "2027-05-01"); err != nil || len(got) != 1 || got[0].Label != "2026-12" {
		t.Fatalf("OwnEngagementsByMonth = %+v %v", got, err)
	}
	m.verify(t)
}

func TestDashboardEventQueries(t *testing.T) {
	m := newDashboardMocks(t)
	m.alloc.ExpectQuery(re(`WHERE aT.engagement_type = 5 AND e.status NOT IN ('Cancelled', 'Deleted')
			AND e.start_date >= ? AND e.start_date < ?
		GROUP BY d_month`)).WithArgs("2025-12-01", "2026-12-01").
		WillReturnRows(sqlmock.NewRows([]string{"m", "n"}).AddRow("2026-10", 3))
	m.alloc.ExpectQuery(re(`JOIN region AS rg ON efs.region_id = rg.id`)).WithArgs("2025-12-01", "2026-12-01", 8).
		WillReturnRows(sqlmock.NewRows([]string{"n", "c"}).AddRow("AMER", 5))
	m.alloc.ExpectQuery(re(`JOIN business_unit AS bu ON efs.business_unit_id = bu.id`)).WithArgs("2025-12-01", "2026-12-01", 6).
		WillReturnRows(sqlmock.NewRows([]string{"n", "c"}).AddRow("Choreo", 2))
	m.alloc.ExpectQuery(re(`COALESCE(SUM(COALESCE(e.campaign_id, '') != ''), 0)`)).WithArgs("2026-08-07").
		WillReturnRows(sqlmock.NewRows([]string{"u", "t", "c"}).AddRow("jane@example.com", 6, 5))

	if got, err := m.repo.EventsByMonth(context.Background(), "2025-12-01", "2026-12-01"); err != nil || len(got) != 1 {
		t.Fatalf("EventsByMonth = %+v %v", got, err)
	}
	if got, err := m.repo.EventsByRegion(context.Background(), "2025-12-01", "2026-12-01", 8); err != nil || got[0].Label != "AMER" {
		t.Fatalf("EventsByRegion = %+v %v", got, err)
	}
	if got, err := m.repo.EventsByBusinessUnit(context.Background(), "2025-12-01", "2026-12-01", 6); err != nil || got[0].Count != 2 {
		t.Fatalf("EventsByBusinessUnit = %+v %v", got, err)
	}
	cr, ok, err := m.repo.EventCampaignCoverage(context.Background(), "2026-08-07")
	if err != nil || !ok || len(cr) != 1 || cr[0].Total != 6 || cr[0].WithCampaign != 5 {
		t.Fatalf("EventCampaignCoverage = %+v %v %v", cr, ok, err)
	}
	m.verify(t)

	// Without the column nothing is queried.
	SetEngagementCampaignColumn(false)
	defer SetEngagementCampaignColumn(true)
	if cr, ok, err := m.repo.EventCampaignCoverage(context.Background(), "2026-08-07"); err != nil || ok || cr != nil {
		t.Errorf("without the column: %+v %v %v", cr, ok, err)
	}
}

func TestDashboardMyVisaQueries(t *testing.T) {
	m := newDashboardMocks(t)
	m.travel.ExpectQuery(re(`LEFT JOIN visa_status AS v ON v.id = t.visa_id
		WHERE t.traveller_email = ? AND COALESCE(t.status, '') != 'Cancelled' AND t.status NOT IN ('Deleted', '')
			AND t.end_date >= ?
		ORDER BY t.start_date, t.job_number
		LIMIT ?`)).WithArgs("jane@example.com", "2026-11-05", 20).
		WillReturnRows(sqlmock.NewRows([]string{"j", "c", "city", "s", "e", "vid", "vt", "ve"}).
			AddRow("TRV-1", "India", "Pune", "2026-12-01", "2026-12-10", 12, "B1/B2", "2027-03-01").
			AddRow("TRV-2", nil, nil, "2027-01-01", "2027-01-05", nil, nil, nil))
	m.travel.ExpectQuery(re(`FROM visa_status
		WHERE email = ? AND status = 'Active' AND end_date >= ?`)).WithArgs("jane@example.com", "2026-11-05").
		WillReturnRows(sqlmock.NewRows([]string{"c", "t", "s", "e"}).AddRow("India", "B1/B2", "2026-01-01", "2027-03-01"))
	trips, err := m.repo.OwnTripVisas(context.Background(), "jane@example.com", "2026-11-05", 20)
	if err != nil || len(trips) != 2 || *trips[0].VisaID != 12 || *trips[0].VisaType != "B1/B2" || trips[1].VisaID != nil {
		t.Fatalf("OwnTripVisas = %+v %v", trips, err)
	}
	visas, err := m.repo.OwnActiveVisas(context.Background(), "jane@example.com", "2026-11-05")
	if err != nil || len(visas) != 1 || visas[0].Country != "India" || visas[0].EndDate != "2027-03-01" {
		t.Fatalf("OwnActiveVisas = %+v %v", visas, err)
	}
	m.verify(t)
}

func TestDaysInWindow(t *testing.T) {
	// Window 2026-08-07 to 2026-11-05 inclusive, given as [from, to).
	const from, to = "2026-08-07", "2026-11-06"
	cases := []struct {
		name, start, end string
		want             int
	}{
		{"straddles the window start", "2026-08-01", "2026-08-10", 4},
		{"straddles the window end", "2026-11-01", "2026-11-30", 5},
		{"spans the whole window", "2024-01-01", "2027-01-01", 91},
		{"outside, before", "2026-01-01", "2026-08-06", 0},
		{"outside, after", "2026-11-06", "2026-12-31", 0},
		{"inside", "2026-09-01", "2026-09-10", 10},
		{"single day on the last window day", "2026-11-05", "2026-11-05", 1},
		{"end before start", "2026-09-10", "2026-09-01", 0},
	}
	for _, c := range cases {
		got, err := daysInWindow(c.start, c.end, from, to)
		if err != nil || got != c.want {
			t.Errorf("%s: daysInWindow = %d %v, want %d", c.name, got, err, c.want)
		}
	}
	if _, err := daysInWindow("bad", "2026-01-01", from, to); err == nil {
		t.Error("a bad date must fail")
	}
}
