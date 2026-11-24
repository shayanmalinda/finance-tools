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
	"errors"
	"fmt"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ── Allocations Today: the legacy Allocations dashboard ──────────────────────
//
// apps/allocation/backend/modules/database/db_queries.bal
// getAllocationsWithEngagementTypeQuery, getPaidAllocationsWithAllocationTypeQuery
// and getNonPaidAllocationsWithAllocationTypeQuery.
//
// One rule for every date, today included: the legacy "today" predicate. The
// legacy screen also had a looser "past date" variant (only Cancelled and
// Deleted left out), which counted Inactive (removed) allocations and so gave
// 17 instead of 9 for the same day depending on whether the date was typed.
// The "today" predicate is the definition of an allocation that counts (an
// Active allocation, Active or Confirmed, of a Confirmed or In Progress
// engagement), is what the screen showed by default, and matches the
// codebase's "active allocation" rule. It reads current statuses, so a past
// date shows today's view of who was allocated then.
//
// Counts are distinct allocations: the join to the funding sources (for the
// business unit filter) repeats an allocation once per funding row, and
// count(allocation.id) as in the legacy code counted a multi-funded
// engagement's allocations once per funding row. Engagements without funding
// rows are still left out, as in the legacy screen.

const dashboardTodayFrom = `
		FROM
			allocation
		INNER JOIN
			allocation_type ON allocation.allocation_type = allocation_type.id
		INNER JOIN
			engagement_type ON allocation_type.engagement_type = engagement_type.id
		INNER JOIN
			engagement ON allocation.engagement_id = engagement.id
		INNER JOIN
			engagement_funding_source ON allocation.engagement_id = engagement_funding_source.engagement_id
		WHERE
			allocation.engagement_id != '-1' AND allocation.engagement_id != '-1.0'`

// dashboardDayWhere is the one rule; its two ? are the day.
const dashboardDayWhere = `
			AND allocation.start_date <= ? AND allocation.end_date >= ?
			AND (engagement.engagement_status in ('Confirmed', 'In Progress') OR engagement.status in ('New', 'In Progress'))
			AND allocation.status = 'Active' AND allocation.allocation_status in ('Active', 'Confirmed')`

const (
	dashboardPaidTypes    = ` AND engagement_type.name IN ('Paid', 'Paid Pre Sale', 'Paid Post Sale')`
	dashboardNonPaidTypes = ` AND engagement_type.name NOT IN ('Paid', 'Paid Pre Sale', 'Paid Post Sale')`
	dashboardBUFilter     = ` AND engagement_funding_source.business_unit_id = ?`
)

// dashboardTodayQuery builds one dashboard query: sel is the select list,
// extra the paid / non-paid predicate, group the GROUP BY clause. f.Date is
// the day (the service fills it in).
func dashboardTodayQuery(f domain.DashboardAllocationsTodayFilter, sel, extra, group string) (string, []any) {
	q := `SELECT ` + sel + dashboardTodayFrom + dashboardDayWhere
	args := []any{f.Date, f.Date}
	q += extra
	if f.BusinessUnitID > 0 {
		q += dashboardBUFilter
		args = append(args, f.BusinessUnitID)
	}
	return q + group, args
}

// AllocationsToday runs the three dashboard queries and the distinct
// consultants under the same predicates (the one addition: the legacy screen
// had no consultant count). f.Date must be set.
func (r *dashboardRepo) AllocationsToday(ctx context.Context, f domain.DashboardAllocationsTodayFilter) (domain.DashboardAllocationsToday, error) {
	out := domain.DashboardAllocationsToday{
		ByEngagementType:        []domain.DashboardEngagementTypeCount{},
		PaidByAllocationType:    []domain.DashboardAllocationTypeCount{},
		NonPaidByAllocationType: []domain.DashboardAllocationTypeCount{},
	}

	q, args := dashboardTodayQuery(f, `
			count(DISTINCT allocation.id) AS allocationCount,
			engagement_type.name AS engagementType,
			engagement_type.id AS engagementTypeId `, "", ` GROUP BY engagement_type.id`)
	rows, err := r.alloc.QueryContext(ctx, q, args...)
	if err != nil {
		return out, fmt.Errorf("dashboard.AllocationsByEngagementType: %w", err)
	}
	for rows.Next() {
		var c domain.DashboardEngagementTypeCount
		if err := rows.Scan(&c.Count, &c.EngagementType, &c.EngagementTypeID); err != nil {
			rows.Close()
			return out, fmt.Errorf("dashboard.AllocationsByEngagementType scan: %w", err)
		}
		out.ByEngagementType = append(out.ByEngagementType, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("dashboard.AllocationsByEngagementType rows: %w", err)
	}

	for _, part := range []struct {
		extra string
		dst   *[]domain.DashboardAllocationTypeCount
		what  string
	}{
		{dashboardPaidTypes, &out.PaidByAllocationType, "dashboard.PaidAllocations"},
		{dashboardNonPaidTypes, &out.NonPaidByAllocationType, "dashboard.NonPaidAllocations"},
	} {
		q, args := dashboardTodayQuery(f, `
			count(DISTINCT allocation.id) AS allocationCount,
			allocation_type.allocation_type AS allocationTypeName,
			allocation_type.color AS allocationTypeColor `, part.extra,
			` GROUP BY allocation_type.allocation_type, allocation_type.color`)
		rows, err := r.alloc.QueryContext(ctx, q, args...)
		if err != nil {
			return out, fmt.Errorf("%s: %w", part.what, err)
		}
		for rows.Next() {
			var c domain.DashboardAllocationTypeCount
			if err := rows.Scan(&c.Count, &c.AllocationType, &c.Color); err != nil {
				rows.Close()
				return out, fmt.Errorf("%s scan: %w", part.what, err)
			}
			*part.dst = append(*part.dst, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, fmt.Errorf("%s rows: %w", part.what, err)
		}
	}

	q, args = dashboardTodayQuery(f, `count(DISTINCT allocation.email) AS consultantCount `, "", "")
	if err := r.alloc.QueryRowContext(ctx, q, args...).Scan(&out.Consultants); err != nil {
		return out, fmt.Errorf("dashboard.AllocatedConsultants: %w", err)
	}
	return out, nil
}

// ── My Work ─────────────────────────────────────────────────────────────────

// OwnAllocations returns email's active engagement allocations that end
// today or later, by start date.
func (r *dashboardRepo) OwnAllocations(ctx context.Context, email, today string) ([]domain.DashboardOwnAllocation, error) {
	q := `SELECT a.id, a.engagement_id, aT.engagement_type, e.customer_name, e.project_name,
			DATE_FORMAT(a.start_date, '%Y-%m-%d'), DATE_FORMAT(a.end_date, '%Y-%m-%d'), a.clearance_status
		FROM allocation AS a
		INNER JOIN engagement AS e ON a.engagement_id = e.id
		LEFT JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE a.email = ? AND ` + dashboardEngagementAllocation + ` AND ` + dashboardActiveAllocation + `
			AND a.end_date >= ?
		ORDER BY a.start_date, a.id
		LIMIT 20`
	rows, err := r.alloc.QueryContext(ctx, q, email, today)
	if err != nil {
		return nil, fmt.Errorf("dashboard.OwnAllocations: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardOwnAllocation{}
	for rows.Next() {
		var a domain.DashboardOwnAllocation
		var typeID sql.NullInt64
		if err := rows.Scan(&a.ID, &a.EngagementID, &typeID, &a.Account, &a.Title,
			&a.StartDate, &a.EndDate, &a.ClearanceStatus); err != nil {
			return nil, fmt.Errorf("dashboard.OwnAllocations scan: %w", err)
		}
		if typeID.Valid {
			v := int(typeID.Int64)
			a.EngagementTypeID = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// TeamAllocationOn returns email's team (default) allocation covering day,
// other than Engagement Overhead and not Deleted, or nil.
func (r *dashboardRepo) TeamAllocationOn(ctx context.Context, email, day, nextDay string) (*domain.DashboardTeamAllocation, error) {
	q := `SELECT a.id, aT.allocation_type, aT.color, a.engagement_code,
			DATE_FORMAT(a.start_date, '%Y-%m-%d'), DATE_FORMAT(a.end_date, '%Y-%m-%d'), a.clearance_status, a.comment
		FROM allocation AS a
		LEFT JOIN allocation_type AS aT ON a.allocation_type = aT.id
		WHERE a.email = ? AND a.engagement_id IN ('-1', '-1.0') AND a.status != 'Deleted'
			AND a.start_date < ? AND a.end_date >= ?
			AND COALESCE(aT.allocation_type, '') != 'Engagement Overhead'
		ORDER BY a.start_date, a.id
		LIMIT 1`
	var t domain.DashboardTeamAllocation
	err := r.alloc.QueryRowContext(ctx, q, email, nextDay, day).Scan(&t.ID, &t.AllocationType, &t.Color,
		&t.EngagementCode, &t.StartDate, &t.EndDate, &t.ClearanceStatus, &t.Comment)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard.TeamAllocationOn: %w", err)
	}
	return &t, nil
}

// ── Waiting on You ──────────────────────────────────────────────────────────

// PendingApprovalCounts counts Pending Approval engagements per engagement
// type, as the Approvals list selects them (legacy
// getEngagementsForApprovalProcess: e.status = 'Pending Approval',
// aT.engagement_type = ?).
func (r *dashboardRepo) PendingApprovalCounts(ctx context.Context, engagementTypes []int) (map[int]int, error) {
	out := map[int]int{}
	if len(engagementTypes) == 0 {
		return out, nil
	}
	q := `SELECT aT.engagement_type, COUNT(*)
		FROM engagement AS e
		INNER JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE e.status = 'Pending Approval' AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
		GROUP BY aT.engagement_type`
	rows, err := r.alloc.QueryContext(ctx, q, Args(engagementTypes)...)
	if err != nil {
		return nil, fmt.Errorf("dashboard.PendingApprovalCounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t, n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, fmt.Errorf("dashboard.PendingApprovalCounts scan: %w", err)
		}
		out[t] = n
	}
	return out, rows.Err()
}

// ClearanceCounts counts Active engagement allocations at Ready for
// Clearance and Clearance In Progress on engagements of the given types (the
// Allocations list with the clearance filter and those engagement types).
func (r *dashboardRepo) ClearanceCounts(ctx context.Context, engagementTypes []int) (map[string]int, error) {
	out := map[string]int{}
	if len(engagementTypes) == 0 {
		return out, nil
	}
	q := `SELECT a.clearance_status, COUNT(*)
		FROM allocation AS a
		INNER JOIN engagement AS e ON a.engagement_id = e.id
		INNER JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE e.status != 'Deleted' AND a.status = 'Active' AND a.clearance_status IN ('Ready for Clearance', 'Clearance In Progress')
			AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
		GROUP BY a.clearance_status`
	rows, err := r.alloc.QueryContext(ctx, q, Args(engagementTypes)...)
	if err != nil {
		return nil, fmt.Errorf("dashboard.ClearanceCounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, fmt.Errorf("dashboard.ClearanceCounts scan: %w", err)
		}
		out[s] = n
	}
	return out, rows.Err()
}

// ── Engagements ─────────────────────────────────────────────────────────────

// EngagementsStarting lists engagements of the given types starting in
// [from, to), not Pending Approval or Cancelled, by start date, with the
// number of distinct consultants on active allocations, and the total.
func (r *dashboardRepo) EngagementsStarting(ctx context.Context, engagementTypes []int, from, to string, limit int) ([]domain.DashboardEngagement, int, error) {
	out := []domain.DashboardEngagement{}
	if len(engagementTypes) == 0 {
		return out, 0, nil
	}
	q := `SELECT e.id, aT.engagement_type, eT.name, e.customer_name, e.project_name, aT.allocation_type,
			e.engagement_code, REPLACE(e.location, 'BR()', CHAR(10)), e.country,
			DATE_FORMAT(e.start_date, '%Y-%m-%d'), DATE_FORMAT(e.end_date, '%Y-%m-%d'), e.status, e.engagement_status,
			(SELECT COUNT(DISTINCT a.email) FROM allocation AS a
				WHERE a.engagement_id = e.id AND ` + dashboardActiveAllocation + `) AS consultants,
			COUNT(*) OVER () AS total
		FROM engagement AS e
		LEFT JOIN allocation_type AS aT ON e.allocation_type = aT.id
		LEFT JOIN engagement_type AS eT ON aT.engagement_type = eT.id
		WHERE e.status NOT IN ('Pending Approval', 'Cancelled') AND COALESCE(e.engagement_status, '') != 'Cancelled'
			AND aT.engagement_type IN (` + Placeholders(len(engagementTypes)) + `)
			AND e.start_date >= ? AND e.start_date < ?
		ORDER BY e.start_date, e.id
		LIMIT ?`
	args := append(Args(engagementTypes), from, to, limit)
	rows, err := r.alloc.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("dashboard.EngagementsStarting: %w", err)
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var e domain.DashboardEngagement
		var typeID sql.NullInt64
		if err := rows.Scan(&e.ID, &typeID, &e.EngagementTypeName, &e.Account, &e.Title, &e.AllocationType,
			&e.EngagementCode, &e.City, &e.Country, &e.StartDate, &e.EndDate, &e.RequestStatus,
			&e.EngagementStatus, &e.Consultants, &total); err != nil {
			return nil, 0, fmt.Errorf("dashboard.EngagementsStarting scan: %w", err)
		}
		if typeID.Valid {
			v := int(typeID.Int64)
			e.EngagementTypeID = &v
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("dashboard.EngagementsStarting rows: %w", err)
	}
	return out, total, nil
}

// Bench counts, per business unit, the consultant pool and those with an
// active engagement allocation overlapping [from, to] (inclusive days). The
// pool is the legacy non-allocated-engineers report's (getNonAllocatedEmployees):
// the employee table without PARTNER, Active or Marked leaver joined before
// the window ends, or Left after it starts.
func (r *dashboardRepo) Bench(ctx context.Context, from, to string) ([]domain.DashboardBenchRow, error) {
	q := `SELECT COALESCE(NULLIF(emp.BusinessUnit, ''), 'Other') AS d_business_unit,
			COUNT(DISTINCT emp.Work_Email) AS consultants,
			COUNT(DISTINCT busy.email) AS allocated
		FROM employee AS emp
		LEFT JOIN (
			SELECT DISTINCT a.email FROM allocation AS a
			WHERE ` + dashboardEngagementAllocation + ` AND ` + dashboardActiveAllocation + `
				AND a.start_date < DATE_ADD(?, INTERVAL 1 DAY) AND a.end_date >= ?
		) AS busy ON busy.email = emp.Work_Email
		WHERE emp.Department != 'PARTNER' AND emp.Work_Email != ''
			AND ((emp.Employee_Status IN ('Active', 'Marked leaver') AND emp.JoinedDate < ?)
				OR (emp.Employee_Status = 'Left' AND emp.FinalDate > ?))
		GROUP BY d_business_unit
		ORDER BY d_business_unit`
	rows, err := r.alloc.QueryContext(ctx, q, to, from, to, from)
	if err != nil {
		return nil, fmt.Errorf("dashboard.Bench: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardBenchRow{}
	for rows.Next() {
		var b domain.DashboardBenchRow
		if err := rows.Scan(&b.BusinessUnit, &b.Consultants, &b.Allocated); err != nil {
			return nil, fmt.Errorf("dashboard.Bench scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// dashboardTrendWhere selects active engagement allocations starting in
// [from, to).
const dashboardTrendWhere = dashboardEngagementAllocation + ` AND ` + dashboardActiveAllocation + `
			AND a.start_date >= ? AND a.start_date < ?`

// AllocationsByStartMonth counts active engagement allocations by start month.
func (r *dashboardRepo) AllocationsByStartMonth(ctx context.Context, from, to string) ([]domain.DashboardCount, error) {
	q := `SELECT DATE_FORMAT(a.start_date, '%Y-%m') AS d_month, COUNT(*)
		FROM allocation AS a
		INNER JOIN engagement AS e ON a.engagement_id = e.id
		WHERE ` + dashboardTrendWhere + `
		GROUP BY d_month
		ORDER BY d_month`
	rows, err := r.alloc.QueryContext(ctx, q, from, to)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AllocationsByStartMonth: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.AllocationsByStartMonth")
}

// AllocationsByBusinessUnit counts the same allocations per funding business
// unit (an allocation counts once in each business unit that funds it).
func (r *dashboardRepo) AllocationsByBusinessUnit(ctx context.Context, from, to string) ([]domain.DashboardCount, error) {
	q := `SELECT bu.name, COUNT(DISTINCT a.id)
		FROM allocation AS a
		INNER JOIN engagement_funding_source AS efs ON a.engagement_id = efs.engagement_id
		INNER JOIN business_unit AS bu ON efs.business_unit_id = bu.id
		WHERE ` + dashboardTrendWhere + `
		GROUP BY bu.name
		ORDER BY COUNT(DISTINCT a.id) DESC, bu.name`
	rows, err := r.alloc.QueryContext(ctx, q, from, to)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AllocationsByBusinessUnit: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.AllocationsByBusinessUnit")
}
