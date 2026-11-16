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
	"fmt"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// allocationEmploymentWindow is the legacy employment filter of the report
// queries: employees active (or marked leaver) who joined before the end of
// the window, or leavers whose final date is after its start.
const allocationEmploymentWindow = ` AND ((employee.Employee_Status IN ('Active' , 'Marked leaver') AND
			employee.JoinedDate < ?) OR (employee.Employee_Status = 'Left' AND
			employee.FinalDate > ?))`

// allocationOrgFilter appends the BU / department / team filters on the
// employee table.
func allocationOrgFilter(f domain.ConsultantAllocationFilter, q string, args []any) (string, []any) {
	if f.BusinessUnit != "" {
		q += ` AND employee.BusinessUnit = ?`
		args = append(args, f.BusinessUnit)
	}
	if f.Department != "" {
		q += ` AND employee.Department = ?`
		args = append(args, f.Department)
	}
	if f.Team != "" {
		q += ` AND employee.Team = ?`
		args = append(args, f.Team)
	}
	return q, args
}

// ConsultantAllocations ports getConsultantAllocations: Active, Confirmed
// allocations (engagement and default) that touch the window, of employees
// in the HR copy allocation_db.employee (which also holds the partner
// consultants under business unit PARTNER), with the engagements' funding
// sources. The legacy BFF read it in pages of 1000 ordered by id until an
// empty page; this reads it in one query with the same order.
func (r *allocationRepo) ConsultantAllocations(ctx context.Context, f domain.ConsultantAllocationFilter) ([]domain.ConsultantAllocation, error) {
	q := `SELECT
			allocation.id AS id, allocation.email AS email, allocation_type.allocation_type AS allocationTypeName,
			allocation.start_date AS startDate, allocation.end_date AS endDate,
			allocation.engagement_id AS engagementId, allocation.isRecurring, allocation.start_time AS startTime,
			allocation.end_time AS endTime, allocation.time_zone AS timeZone,
			engagement.engagement_code AS engagementCode, engagement.engagement_nature AS engagementNature,
			engagement.country AS country, allocation.engagement_code AS initialEngagementCode,
			allocation_type.default_engagement_code AS defaultEngagementCodeForAllocType,
			engagement_type.name AS engagementTypeName,
			datediff(allocation.end_date, allocation.start_date) AS numOfDdays,
			employee.Department AS department, employee.Team AS team, employee.BusinessUnit AS businessUnit,
			employee.Location AS location, employee.Company AS company
		FROM allocation
			LEFT JOIN engagement ON allocation.engagement_id = engagement.id
			LEFT JOIN allocation_type ON allocation.allocation_type = allocation_type.id
			LEFT JOIN engagement_type ON allocation_type.engagement_type = engagement_type.id
			LEFT JOIN employee ON allocation.email = employee.Work_Email
		WHERE allocation.status = 'Active'
			AND allocation.clearance_status = 'Confirmed'
			AND datediff(allocation.end_date, allocation.start_date) >= 0
			AND allocation.status != 'Deleted'` +
		allocationEmploymentWindow +
		` AND (((allocation.start_date BETWEEN ? AND ?) OR (allocation.end_date BETWEEN ? AND ?)) OR
			(allocation.start_date <= ? AND allocation.end_date >= ?))`
	args := []any{f.EndDate, f.StartDate, f.StartDate, f.EndDate, f.StartDate, f.EndDate, f.StartDate, f.EndDate}
	q, args = allocationOrgFilter(f, q, args)
	if f.Email != "" {
		q += ` AND allocation.email IN (?)`
		args = append(args, f.Email)
	}
	if len(f.AllocationTypeIDs) > 0 {
		q += ` AND allocation.allocation_type IN (` + Placeholders(len(f.AllocationTypeIDs)) + `)`
		args = append(args, Args(f.AllocationTypeIDs)...)
	}
	q += ` ORDER BY id`

	rows, err := r.alloc.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("consultantAllocations: %w", err)
	}
	defer rows.Close()
	out := []domain.ConsultantAllocation{}
	var engagementIDs []string
	seen := map[string]bool{}
	for rows.Next() {
		var (
			c          domain.ConsultantAllocation
			start, end allocationDate
			startTime  *string
			endTime    *string
		)
		if err := rows.Scan(&c.ID, &c.Email, &c.AllocationTypeName, &start, &end, &c.EngagementID, &c.IsRecurring,
			&startTime, &endTime, &c.TimeZone, &c.EngagementCode, &c.EngagementNature, &c.Country,
			&c.InitialEngagementCode, &c.DefaultEngagementCodeForAllocType, &c.EngagementTypeName, &c.NumOfDays,
			&c.Department, &c.Team, &c.BusinessUnit, &c.Location, &c.Company); err != nil {
			return nil, fmt.Errorf("consultantAllocations scan: %w", err)
		}
		c.StartDate, c.EndDate = start.str(), end.str()
		c.StartTime, c.EndTime = allocationTimeOf(startTime), allocationTimeOf(endTime)
		c.FundingSources = []domain.AllocationFundingSource{}
		if c.EngagementID != nil && !domain.AllocationIsDefault(*c.EngagementID) && !seen[*c.EngagementID] {
			seen[*c.EngagementID] = true
			engagementIDs = append(engagementIDs, *c.EngagementID)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fs, err := r.fundingSources(ctx, engagementIDs)
	if err != nil {
		return nil, err
	}
	byEngagement := map[string][]domain.AllocationFundingSource{}
	for _, s := range fs {
		byEngagement[s.EngagementID] = append(byEngagement[s.EngagementID], s)
	}
	for i := range out {
		if id := out[i].EngagementID; id != nil {
			if s := byEngagement[*id]; s != nil {
				out[i].FundingSources = s
			}
		}
	}
	return out, nil
}

// NonAllocatedEmployees ports getNonAllocatedEmployees: employees (not in
// department PARTNER) employed in the window, in the org filter, without an
// Active allocation touching the window. The legacy query sent the allocated
// emails as a NOT IN list; this reads the two sets (the legacy
// getAllocatedEmployeeData query and the employee query) and subtracts in
// Go, which returns the same rows without a list of thousands of bind
// parameters.
func (r *allocationRepo) NonAllocatedEmployees(ctx context.Context, f domain.ConsultantAllocationFilter) ([]domain.ConsultantNonAllocatedEmployee, error) {
	aq := `SELECT DISTINCT employee.Work_Email AS workEmail
		FROM allocation
			LEFT JOIN employee ON allocation.email = employee.Work_Email
		WHERE allocation.status = 'Active' AND datediff(allocation.end_date, allocation.start_date) >= 0` +
		allocationEmploymentWindow +
		` AND (((allocation.start_date BETWEEN ? AND ?) OR (allocation.end_date BETWEEN ? AND ?)) OR
			(allocation.start_date <= ? AND allocation.end_date >= ?))`
	aargs := []any{f.EndDate, f.StartDate, f.StartDate, f.EndDate, f.StartDate, f.EndDate, f.StartDate, f.EndDate}
	aq, aargs = allocationOrgFilter(f, aq, aargs)
	rows, err := r.alloc.QueryContext(ctx, aq, aargs...)
	if err != nil {
		return nil, fmt.Errorf("nonAllocatedEmployees allocated: %w", err)
	}
	allocated := map[string]bool{}
	for rows.Next() {
		var email *string
		if err := rows.Scan(&email); err != nil {
			rows.Close()
			return nil, fmt.Errorf("nonAllocatedEmployees allocated scan: %w", err)
		}
		if email != nil {
			allocated[*email] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	eq := `SELECT DISTINCT Work_Email AS workEmail, Team AS team, BusinessUnit AS businessUnit,
			Department AS department, Job_Role AS jobRole, Company AS company
		FROM employee
		WHERE department != 'PARTNER'` +
		` AND ((employee.Employee_Status IN ('Active' , 'Marked leaver')
			AND employee.JoinedDate < ?)
			OR (employee.Employee_Status = 'Left' AND employee.FinalDate > ?))`
	eargs := []any{f.EndDate, f.StartDate}
	if f.BusinessUnit != "" {
		eq += ` AND employee.BusinessUnit = ?`
		eargs = append(eargs, f.BusinessUnit)
	}
	if f.Department != "" && f.Department != "PARTNER" {
		eq += ` AND employee.Department = ?`
		eargs = append(eargs, f.Department)
	}
	if f.Team != "" {
		eq += ` AND employee.Team = ?`
		eargs = append(eargs, f.Team)
	}
	eq += ` AND employee.Work_Email != '' ORDER BY employee.Work_Email`
	erows, err := r.alloc.QueryContext(ctx, eq, eargs...)
	if err != nil {
		return nil, fmt.Errorf("nonAllocatedEmployees: %w", err)
	}
	defer erows.Close()
	out := []domain.ConsultantNonAllocatedEmployee{}
	for erows.Next() {
		var e domain.ConsultantNonAllocatedEmployee
		if err := erows.Scan(&e.WorkEmail, &e.Team, &e.BusinessUnit, &e.Department, &e.JobRole, &e.Company); err != nil {
			return nil, fmt.Errorf("nonAllocatedEmployees scan: %w", err)
		}
		if e.WorkEmail != nil && allocated[*e.WorkEmail] {
			continue
		}
		out = append(out, e)
	}
	return out, erows.Err()
}

// TeamDefaults ports getDefaultAllocationsQuery.
func (r *allocationRepo) TeamDefaults(ctx context.Context) ([]domain.ConsultantTeamDefault, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT
			alft.business_unit AS businessUnit, alft.department, alft.team,
			aT.allocation_type AS defaultAllocTypeName, engagement_type.name AS engagementTypeName,
			aT.default_engagement_code AS defaultEngagementCode
		FROM default_allocation_for_team AS alft
			INNER JOIN allocation_type AS aT ON alft.default_allocation_type = aT.id
			INNER JOIN engagement_type ON aT.engagement_type = engagement_type.id
		ORDER BY businessUnit, department, team`)
	if err != nil {
		return nil, fmt.Errorf("teamDefaults: %w", err)
	}
	defer rows.Close()
	out := []domain.ConsultantTeamDefault{}
	for rows.Next() {
		var d domain.ConsultantTeamDefault
		if err := rows.Scan(&d.BusinessUnit, &d.Department, &d.Team, &d.DefaultAllocTypeName,
			&d.EngagementTypeName, &d.DefaultEngagementCode); err != nil {
			return nil, fmt.Errorf("teamDefaults scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// EngagementCodes ports getEngCodeDataQuery without a code (every row, in
// table order).
func (r *allocationRepo) EngagementCodes(ctx context.Context) ([]domain.ConsultantEngagementCode, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT
			eng_code AS engagementCode, eng_group AS engGroup, NTST_AccountName AS accountName, GL_code AS glCode
		FROM engcode_grouping_gl_codes`)
	if err != nil {
		return nil, fmt.Errorf("engagementCodes: %w", err)
	}
	defer rows.Close()
	out := []domain.ConsultantEngagementCode{}
	for rows.Next() {
		var c domain.ConsultantEngagementCode
		if err := rows.Scan(&c.EngagementCode, &c.EngGroup, &c.AccountName, &c.GLCode); err != nil {
			return nil, fmt.Errorf("engagementCodes scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
