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
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// dashboardCustomerTypes are the engagement types that have a customer
// (the retired Paid type and the pre and post sale types).
var dashboardCustomerTypes = []int{2, 3, 4, 6, 7}

// ── Travel Team (travelapp) ─────────────────────────────────────────────────

// TravelTeamAway counts the travellers and the distinct countries of trips
// not Cancelled that are under way or start within the next 6 days.
func (r *dashboardRepo) TravelTeamAway(ctx context.Context, w DashboardWindow) (travellers, countries int, err error) {
	q := `SELECT COUNT(DISTINCT t.traveller_email),
			COUNT(DISTINCT NULLIF(` + dashboardTripCountry + `, ''))
		FROM travel AS t
		WHERE ` + dashboardNotCancelled + ` AND t.status NOT IN ('Deleted', '')
			AND t.start_date < ? AND t.end_date >= ?`
	if err := r.travel.QueryRowContext(ctx, q, w.In6Days, w.Today).Scan(&travellers, &countries); err != nil {
		return 0, 0, fmt.Errorf("dashboard.TravelTeamAway: %w", err)
	}
	return travellers, countries, nil
}

// TravelEndedNotCompleted counts trips still New, In Progress, Extended or On
// Hold that ended 3 to 90 days ago.
func (r *dashboardRepo) TravelEndedNotCompleted(ctx context.Context, w DashboardWindow) (int, error) {
	var n int
	err := r.travel.QueryRowContext(ctx, `SELECT COUNT(*) FROM travel AS t
		WHERE t.status IN ('New', 'In Progress', 'Extended', 'On Hold')
			AND t.end_date < ? AND t.end_date >= ?`, w.Minus3, w.Minus90).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("dashboard.TravelEndedNotCompleted: %w", err)
	}
	return n, nil
}

// POQueueByAge buckets the invoices waiting for PO approval (status Pending,
// the approve-po queue) by days since they were added: under 7, 7 to 30, 31
// to 90, over 90. It also returns the total and the age of the oldest.
func (r *dashboardRepo) POQueueByAge(ctx context.Context, today string) (byAge [4]int, total, oldest int, err error) {
	q := `SELECT COALESCE(SUM(d < 7), 0), COALESCE(SUM(d BETWEEN 7 AND 30), 0),
			COALESCE(SUM(d BETWEEN 31 AND 90), 0), COALESCE(SUM(d > 90), 0),
			COALESCE(MAX(d), 0), COUNT(*)
		FROM (SELECT DATEDIFF(?, i.added_date) AS d FROM invoice AS i WHERE i.status = 'Pending') AS x`
	if err = r.travel.QueryRowContext(ctx, q, today).Scan(&byAge[0], &byAge[1], &byAge[2], &byAge[3], &oldest, &total); err != nil {
		return byAge, 0, 0, fmt.Errorf("dashboard.POQueueByAge: %w", err)
	}
	return byAge, total, oldest, nil
}

// FrequentTravellers lists the 8 travellers with the most trips (not
// Cancelled) that started in the 12-month window.
func (r *dashboardRepo) FrequentTravellers(ctx context.Context, w DashboardWindow) ([]domain.DashboardCount, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT t.traveller_email, COUNT(*)
		FROM travel AS t
		WHERE `+dashboardNotCancelled+` AND t.start_date >= ? AND t.start_date < ?
		GROUP BY t.traveller_email
		ORDER BY COUNT(*) DESC, t.traveller_email
		LIMIT 8`, w.YearStart, w.Tomorrow)
	if err != nil {
		return nil, fmt.Errorf("dashboard.FrequentTravellers: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.FrequentTravellers")
}

// AgentWorkload counts, per travel agent (or "No travel agent yet"), the trips
// not Cancelled under way or starting within 60 days; the top 5.
func (r *dashboardRepo) AgentWorkload(ctx context.Context, w DashboardWindow) ([]domain.DashboardCount, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT COALESCE(ta.name_of_travel_agent, 'No travel agent yet') AS d_agent, COUNT(*)
		FROM travel AS t
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE `+dashboardNotCancelled+` AND t.end_date >= ? AND t.start_date < ?
		GROUP BY d_agent
		ORDER BY COUNT(*) DESC, d_agent
		LIMIT 5`, w.Today, w.In60Days)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AgentWorkload: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.AgentWorkload")
}

// ── Engagement Manager and Account Manager (allocation_db) ──────────────────

// dashboardEngagementCols are the columns of a DashboardEngagement, in scan
// order, followed by the total. The inner select aliases them so the outer
// one can filter on consultants.
const dashboardEngagementInner = `SELECT e.id AS e_id, aT.engagement_type AS e_type, eT.name AS e_type_name,
			e.customer_name AS e_customer, e.project_name AS e_title, aT.allocation_type AS e_alloc_type,
			e.engagement_code AS e_code, REPLACE(e.location, 'BR()', CHAR(10)) AS e_city, e.country AS e_country,
			DATE_FORMAT(e.start_date, '%Y-%m-%d') AS e_start, DATE_FORMAT(e.end_date, '%Y-%m-%d') AS e_end,
			e.status AS e_status, e.engagement_status AS e_eng_status,
			(SELECT COUNT(DISTINCT a.email) FROM allocation AS a
				WHERE a.engagement_id = e.id AND ` + dashboardActiveAllocation + `) AS consultants
		FROM engagement AS e
		LEFT JOIN allocation_type AS aT ON e.allocation_type = aT.id
		LEFT JOIN engagement_type AS eT ON aT.engagement_type = eT.id`

func dashboardEngagementRows(rows *sql.Rows, what string) ([]domain.DashboardEngagement, int, error) {
	defer rows.Close()
	out := []domain.DashboardEngagement{}
	total := 0
	for rows.Next() {
		var e domain.DashboardEngagement
		var typeID sql.NullInt64
		if err := rows.Scan(&e.ID, &typeID, &e.EngagementTypeName, &e.Account, &e.Title, &e.AllocationType,
			&e.EngagementCode, &e.City, &e.Country, &e.StartDate, &e.EndDate, &e.RequestStatus,
			&e.EngagementStatus, &e.Consultants, &total); err != nil {
			return nil, 0, fmt.Errorf("%s scan: %w", what, err)
		}
		if typeID.Valid {
			v := int(typeID.Int64)
			e.EngagementTypeID = &v
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s rows: %w", what, err)
	}
	return out, total, nil
}

// EngagementsStartingWithout lists the engagements of the given types
// starting in [from, to) (not Pending Approval or Cancelled) that have no
// consultant on an active allocation, by start date, with the total.
func (r *dashboardRepo) EngagementsStartingWithout(ctx context.Context, engagementTypes []int, from, to string, limit int) ([]domain.DashboardEngagement, int, error) {
	if len(engagementTypes) == 0 {
		return []domain.DashboardEngagement{}, 0, nil
	}
	q := `SELECT x.*, COUNT(*) OVER () AS total FROM (` + dashboardEngagementInner + `
		WHERE e.status NOT IN ('Pending Approval', 'Cancelled') AND COALESCE(e.engagement_status, '') != 'Cancelled'
			AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
			AND e.start_date >= ? AND e.start_date < ?) AS x
		WHERE x.consultants = 0
		ORDER BY x.e_start, x.e_id
		LIMIT ?`
	rows, err := r.alloc.QueryContext(ctx, q, append(Args(engagementTypes), from, to, limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("dashboard.EngagementsStartingWithout: %w", err)
	}
	return dashboardEngagementRows(rows, "dashboard.EngagementsStartingWithout")
}

// AllocationsStarting lists active engagement allocations of the given types
// starting in [from, to), by start date, with the total.
func (r *dashboardRepo) AllocationsStarting(ctx context.Context, engagementTypes []int, from, to string, limit int) ([]domain.DashboardStartingAllocation, int, error) {
	out := []domain.DashboardStartingAllocation{}
	if len(engagementTypes) == 0 {
		return out, 0, nil
	}
	q := `SELECT a.id, a.email, e.id, aT.engagement_type, e.customer_name, e.project_name,
			DATE_FORMAT(a.start_date, '%Y-%m-%d'), DATE_FORMAT(a.end_date, '%Y-%m-%d'), a.isTravelRequired,
			COUNT(*) OVER () AS total
		FROM allocation AS a
		JOIN engagement AS e ON a.engagement_id = e.id
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE ` + dashboardActiveAllocation + ` AND ` + dashboardEngagementAllocation + `
			AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
			AND a.start_date >= ? AND a.start_date < ?
		ORDER BY a.start_date, a.id
		LIMIT ?`
	rows, err := r.alloc.QueryContext(ctx, q, append(Args(engagementTypes), from, to, limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("dashboard.AllocationsStarting: %w", err)
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var a domain.DashboardStartingAllocation
		var typeID sql.NullInt64
		var travel sql.NullString
		if err := rows.Scan(&a.AllocationID, &a.Email, &a.EngagementID, &typeID, &a.Account, &a.Title,
			&a.StartDate, &a.EndDate, &travel, &total); err != nil {
			return nil, 0, fmt.Errorf("dashboard.AllocationsStarting scan: %w", err)
		}
		if typeID.Valid {
			v := int(typeID.Int64)
			a.EngagementTypeID = &v
		}
		a.TravelRequired = travel.String == "Yes"
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("dashboard.AllocationsStarting rows: %w", err)
	}
	return out, total, nil
}

// MostAllocated sums, per consultant, the calendar days that the active
// engagement allocations of the given types spend inside [from, to), so an
// allocation that starts before the window or ends after it counts only its
// days in the window; the top limit. Label is the consultant's email.
func (r *dashboardRepo) MostAllocated(ctx context.Context, engagementTypes []int, from, to string, limit int) ([]domain.DashboardCount, error) {
	if len(engagementTypes) == 0 {
		return []domain.DashboardCount{}, nil
	}
	q := `SELECT a.email AS d_label, DATE_FORMAT(a.start_date, '%Y-%m-%d'), DATE_FORMAT(a.end_date, '%Y-%m-%d')
		FROM allocation AS a
		JOIN engagement AS e ON a.engagement_id = e.id
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE ` + dashboardActiveAllocation + ` AND ` + dashboardEngagementAllocation + `
			AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
			AND a.start_date < ? AND a.end_date >= ?`
	rows, err := r.alloc.QueryContext(ctx, q, append(Args(engagementTypes), to, from)...)
	if err != nil {
		return nil, fmt.Errorf("dashboard.MostAllocated: %w", err)
	}
	return dashboardWindowDays(rows, "dashboard.MostAllocated", from, to, limit)
}

// TopCustomers sums the calendar days of the same allocations inside the
// window per customer (over the customer types among engagementTypes) or,
// when there is none, per country over all of them. It returns what it
// grouped by.
func (r *dashboardRepo) TopCustomers(ctx context.Context, engagementTypes []int, from, to string, limit int) (by string, _ []domain.DashboardCount, _ error) {
	var customer []int
	for _, t := range engagementTypes {
		for _, c := range dashboardCustomerTypes {
			if t == c {
				customer = append(customer, t)
			}
		}
	}
	types, col, by := customer, "e.customer_name", "customer"
	if len(customer) == 0 {
		types, col, by = engagementTypes, "e.country", "country"
	}
	if len(types) == 0 {
		return by, []domain.DashboardCount{}, nil
	}
	q := `SELECT TRIM(` + col + `) AS d_label, DATE_FORMAT(a.start_date, '%Y-%m-%d'), DATE_FORMAT(a.end_date, '%Y-%m-%d')
		FROM allocation AS a
		JOIN engagement AS e ON a.engagement_id = e.id
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE ` + dashboardActiveAllocation + ` AND ` + dashboardEngagementAllocation + `
			AND aT.engagement_type IN (` + Placeholders(len(types)) + `)
			AND a.start_date < ? AND a.end_date >= ?
			AND TRIM(COALESCE(` + col + `, '')) != ''`
	rows, err := r.alloc.QueryContext(ctx, q, append(Args(types), to, from)...)
	if err != nil {
		return by, nil, fmt.Errorf("dashboard.TopCustomers: %w", err)
	}
	out, err := dashboardWindowDays(rows, "dashboard.TopCustomers", from, to, limit)
	return by, out, err
}

// dashboardWindowDays scans (label, start, end) allocation rows, adds up per
// label the days each allocation has inside [from, to) and returns the top
// limit, largest first and by label on a tie. Each row is one allocation, so a
// multi-funded engagement is not counted twice.
func dashboardWindowDays(rows *sql.Rows, what, from, to string, limit int) ([]domain.DashboardCount, error) {
	defer rows.Close()
	total := map[string]int{}
	for rows.Next() {
		var label sql.NullString
		var start, end string
		if err := rows.Scan(&label, &start, &end); err != nil {
			return nil, fmt.Errorf("%s scan: %w", what, err)
		}
		d, err := daysInWindow(start, end, from, to)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		if d > 0 {
			total[label.String] += d
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", what, err)
	}
	out := make([]domain.DashboardCount, 0, len(total))
	for l, c := range total {
		out = append(out, domain.DashboardCount{Label: l, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// daysInWindow is max(0, min(end, windowEnd) - max(start, windowStart) + 1)
// in calendar days, where the window is [from, to) so windowEnd is the day
// before to. All values are YYYY-MM-DD.
func daysInWindow(start, end, from, to string) (int, error) {
	const layout = "2006-01-02"
	var d [4]time.Time
	for i, v := range []string{start, end, from, to} {
		t, err := time.Parse(layout, v)
		if err != nil {
			return 0, fmt.Errorf("bad date %q: %w", v, err)
		}
		d[i] = t
	}
	lo, hi := d[0], d[1]
	if d[2].After(lo) {
		lo = d[2]
	}
	if last := d[3].AddDate(0, 0, -1); last.Before(hi) {
		hi = last
	}
	n := int(math.Round(hi.Sub(lo).Hours()/24)) + 1
	if n < 0 {
		return 0, nil
	}
	return n, nil
}

// dashboardOwnEngagement keeps the engagements the person created and that
// are not Deleted.
const dashboardOwnEngagement = `LOWER(e.username) = ? AND e.status != 'Deleted'`

// OwnEngagementsRunningWithout lists the person's engagements that are
// running today (not Pending Approval, Cancelled or Completed) without a
// consultant on an active allocation, with the total.
func (r *dashboardRepo) OwnEngagementsRunningWithout(ctx context.Context, email, today string, limit int) ([]domain.DashboardEngagement, int, error) {
	q := `SELECT x.*, COUNT(*) OVER () AS total FROM (` + dashboardEngagementInner + `
		WHERE LOWER(e.username) = ? AND e.start_date <= ? AND e.end_date >= ?
			AND e.status NOT IN ('Pending Approval', 'Cancelled', 'Deleted')
			AND COALESCE(e.engagement_status, '') NOT IN ('Cancelled', 'Completed')) AS x
		WHERE x.consultants = 0
		ORDER BY x.e_start, x.e_id
		LIMIT ?`
	rows, err := r.alloc.QueryContext(ctx, q, email, today, today, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("dashboard.OwnEngagementsRunningWithout: %w", err)
	}
	return dashboardEngagementRows(rows, "dashboard.OwnEngagementsRunningWithout")
}

// OwnEngagementsByStatus counts the person's engagements that end in the
// 12-month window or later, by status.
func (r *dashboardRepo) OwnEngagementsByStatus(ctx context.Context, email, yearStart string) ([]domain.DashboardCount, error) {
	q := `SELECT CASE WHEN e.status = 'Pending Approval' THEN 'Pending Approval'
				WHEN e.status = 'Cancelled' OR e.engagement_status = 'Cancelled' THEN 'Cancelled'
				ELSE COALESCE(NULLIF(e.engagement_status, ''), 'New') END AS d_status, COUNT(*)
		FROM engagement AS e
		WHERE ` + dashboardOwnEngagement + ` AND e.end_date >= ?
		GROUP BY d_status
		ORDER BY COUNT(*) DESC, d_status`
	rows, err := r.alloc.QueryContext(ctx, q, email, yearStart)
	if err != nil {
		return nil, fmt.Errorf("dashboard.OwnEngagementsByStatus: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.OwnEngagementsByStatus")
}

// OwnEngagementsByMonth counts the person's engagements, not Cancelled, that
// start in [from, to) by start month.
func (r *dashboardRepo) OwnEngagementsByMonth(ctx context.Context, email, from, to string) ([]domain.DashboardCount, error) {
	q := `SELECT DATE_FORMAT(e.start_date, '%Y-%m') AS d_month, COUNT(*)
		FROM engagement AS e
		WHERE LOWER(e.username) = ? AND e.status NOT IN ('Cancelled', 'Deleted')
			AND e.start_date >= ? AND e.start_date < ?
		GROUP BY d_month
		ORDER BY d_month`
	rows, err := r.alloc.QueryContext(ctx, q, email, from, to)
	if err != nil {
		return nil, fmt.Errorf("dashboard.OwnEngagementsByMonth: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.OwnEngagementsByMonth")
}

// ── Marketing Events (allocation_db) ────────────────────────────────────────

// dashboardEventFrom is the events (engagement type 5, not Cancelled or
// Deleted) in a start range.
const dashboardEventFrom = `FROM engagement AS e
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE aT.engagement_type = 5 AND e.status NOT IN ('Cancelled', 'Deleted')
			AND e.start_date >= ? AND e.start_date < ?`

// EventsByMonth counts events starting in [from, to) by start month.
func (r *dashboardRepo) EventsByMonth(ctx context.Context, from, to string) ([]domain.DashboardCount, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT DATE_FORMAT(e.start_date, '%Y-%m') AS d_month, COUNT(*) `+dashboardEventFrom+`
		GROUP BY d_month
		ORDER BY d_month`, from, to)
	if err != nil {
		return nil, fmt.Errorf("dashboard.EventsByMonth: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.EventsByMonth")
}

// eventsByFunding counts events per funding region or business unit; an
// event counts once in each one that funds it.
func (r *dashboardRepo) eventsByFunding(ctx context.Context, join, nameCol, what string, from, to string, limit int) ([]domain.DashboardCount, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT `+nameCol+` AS d_name, COUNT(DISTINCT e.id)
		FROM engagement AS e
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		JOIN engagement_funding_source AS efs ON efs.engagement_id = e.id
		`+join+`
		WHERE aT.engagement_type = 5 AND e.status NOT IN ('Cancelled', 'Deleted')
			AND e.start_date >= ? AND e.start_date < ?
		GROUP BY d_name
		ORDER BY COUNT(DISTINCT e.id) DESC, d_name
		LIMIT ?`, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return dashboardCountRows(rows, what)
}

// EventsByRegion counts events per funding region; the top limit.
func (r *dashboardRepo) EventsByRegion(ctx context.Context, from, to string, limit int) ([]domain.DashboardCount, error) {
	return r.eventsByFunding(ctx, `JOIN region AS rg ON efs.region_id = rg.id`, `rg.name`, "dashboard.EventsByRegion", from, to, limit)
}

// EventsByBusinessUnit counts events per funding business unit; the top limit.
func (r *dashboardRepo) EventsByBusinessUnit(ctx context.Context, from, to string, limit int) ([]domain.DashboardCount, error) {
	return r.eventsByFunding(ctx, `JOIN business_unit AS bu ON efs.business_unit_id = bu.id`, `bu.name`, "dashboard.EventsByBusinessUnit", from, to, limit)
}

// EventCampaignCoverage counts, per creator, the events starting on or after
// from and how many carry a campaign id. Without the campaign_id column it
// returns ok false.
func (r *dashboardRepo) EventCampaignCoverage(ctx context.Context, from string) (creators []domain.DashboardCampaignCreator, ok bool, err error) {
	if !HasEngagementCampaignColumn() {
		return nil, false, nil
	}
	rows, err := r.alloc.QueryContext(ctx, `SELECT LOWER(COALESCE(e.username, '')), COUNT(*),
			COALESCE(SUM(COALESCE(e.campaign_id, '') != ''), 0)
		FROM engagement AS e
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE aT.engagement_type = 5 AND e.status NOT IN ('Cancelled', 'Deleted') AND e.start_date >= ?
		GROUP BY LOWER(COALESCE(e.username, ''))
		ORDER BY COUNT(*) DESC, 1`, from)
	if err != nil {
		return nil, false, fmt.Errorf("dashboard.EventCampaignCoverage: %w", err)
	}
	defer rows.Close()
	creators = []domain.DashboardCampaignCreator{}
	for rows.Next() {
		var c domain.DashboardCampaignCreator
		if err := rows.Scan(&c.Email, &c.Total, &c.WithCampaign); err != nil {
			return nil, false, fmt.Errorf("dashboard.EventCampaignCoverage scan: %w", err)
		}
		creators = append(creators, c)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("dashboard.EventCampaignCoverage rows: %w", err)
	}
	return creators, true, nil
}

// ── My visas (travelapp) ────────────────────────────────────────────────────

// OwnTripVisas lists the person's trips (not Cancelled, not Deleted or blank)
// that end today or later, by start date, with the visa picked on each; the
// first limit.
func (r *dashboardRepo) OwnTripVisas(ctx context.Context, email, today string, limit int) ([]domain.DashboardMyVisaTrip, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT t.job_number, `+dashboardTripCountry+`, REPLACE(t.city, 'BR()', CHAR(10)),
			DATE_FORMAT(t.start_date, '%Y-%m-%d'), DATE_FORMAT(t.end_date, '%Y-%m-%d'),
			t.visa_id, v.visa_type, DATE_FORMAT(v.end_date, '%Y-%m-%d')
		FROM travel AS t
		LEFT JOIN visa_status AS v ON v.id = t.visa_id
		WHERE t.traveller_email = ? AND `+dashboardNotCancelled+` AND t.status NOT IN ('Deleted', '')
			AND t.end_date >= ?
		ORDER BY t.start_date, t.job_number
		LIMIT ?`, email, today, limit)
	if err != nil {
		return nil, fmt.Errorf("dashboard.OwnTripVisas: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardMyVisaTrip{}
	for rows.Next() {
		var t domain.DashboardMyVisaTrip
		var visaID sql.NullInt64
		if err := rows.Scan(&t.JobNumber, &t.Country, &t.City, &t.StartDate, &t.EndDate, &visaID, &t.VisaType, &t.VisaEnd); err != nil {
			return nil, fmt.Errorf("dashboard.OwnTripVisas scan: %w", err)
		}
		if visaID.Valid {
			v := int(visaID.Int64)
			t.VisaID = &v
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.OwnTripVisas rows: %w", err)
	}
	return out, nil
}

// OwnActiveVisas lists the person's Active visas that end on or after today.
func (r *dashboardRepo) OwnActiveVisas(ctx context.Context, email, today string) ([]domain.DashboardOwnVisa, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT country, visa_type, DATE_FORMAT(start_date, '%Y-%m-%d'), DATE_FORMAT(end_date, '%Y-%m-%d')
		FROM visa_status
		WHERE email = ? AND status = 'Active' AND end_date >= ?
		ORDER BY end_date, id`, email, today)
	if err != nil {
		return nil, fmt.Errorf("dashboard.OwnActiveVisas: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardOwnVisa{}
	for rows.Next() {
		var v domain.DashboardOwnVisa
		var country, typ, start, end sql.NullString
		if err := rows.Scan(&country, &typ, &start, &end); err != nil {
			return nil, fmt.Errorf("dashboard.OwnActiveVisas scan: %w", err)
		}
		v.Country, v.VisaType, v.StartDate, v.EndDate = country.String, typ.String, start.String, end.String
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.OwnActiveVisas rows: %w", err)
	}
	return out, nil
}
