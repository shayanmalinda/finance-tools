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
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// AllocationRepository is the SQL of the allocation screen area: engagement
// allocations, default (team) allocations, the travel rows they create, the
// partner consultants they name and the Consultant Allocations report. Each
// method uses one pool per query; nothing crosses schemas.
type AllocationRepository interface {
	// Engagement returns the engagement with its funding sources, or nil.
	Engagement(ctx context.Context, id string) (*domain.AllocationEngagement, error)
	// Get returns one allocation in any status, or nil.
	Get(ctx context.Context, id string) (*domain.AllocationRecord, error)
	// Timeline returns the active allocations of people over a window.
	Timeline(ctx context.Context, req domain.AllocationTimelineRequest) ([]domain.AllocationRecord, error)
	// Search returns one page of the Engagement Allocations list, its total
	// and the travel rows of the page's allocations.
	Search(ctx context.Context, req domain.AllocationSearchRequest) ([]domain.AllocationRecord, int, error)
	// Bulk inserts and updates allocations in one transaction.
	Bulk(ctx context.Context, req domain.AllocationBulkRequest) (domain.AllocationBulkResponse, error)
	// Update changes one allocation and creates or cancels its travel row.
	Update(ctx context.Context, pre domain.AllocationRecord, req domain.AllocationUpdateRequest) (domain.AllocationUpdateResponse, error)
	// Delete sets the allocation's status to Deleted.
	Delete(ctx context.Context, id string) error
	// Visas returns the person's visas valid over the dates.
	Visas(ctx context.Context, email, startDate, endDate string) ([]domain.AllocationVisa, error)
	// ConsultantAllocations is the Consultant Allocations report query.
	ConsultantAllocations(ctx context.Context, f domain.ConsultantAllocationFilter) ([]domain.ConsultantAllocation, error)
	// NonAllocatedEmployees are employees with no active allocation in the window.
	NonAllocatedEmployees(ctx context.Context, f domain.ConsultantAllocationFilter) ([]domain.ConsultantNonAllocatedEmployee, error)
	// TeamDefaults are the default allocation types per team.
	TeamDefaults(ctx context.Context) ([]domain.ConsultantTeamDefault, error)
	// EngagementCodes are the engagement code groups and GL codes.
	EngagementCodes(ctx context.Context) ([]domain.ConsultantEngagementCode, error)
}

type allocationRepo struct {
	alloc  *sql.DB // allocation_db
	travel *sql.DB // travelapp
}

// NewAllocationRepository constructs an AllocationRepository over the
// allocation_db and travelapp pools.
func NewAllocationRepository(alloc, travel *sql.DB) AllocationRepository {
	return &allocationRepo{alloc: alloc, travel: travel}
}

// allocationDate scans a DATE or DATETIME column (time.Time with
// parseTime, or text without) as "YYYY-MM-DD"; NULL stays nil.
type allocationDate struct{ s *string }

func (d *allocationDate) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		d.s = nil
	case time.Time:
		s := t.Format("2006-01-02")
		d.s = &s
	case []byte:
		s := allocationDatePart(string(t))
		d.s = &s
	case string:
		s := allocationDatePart(t)
		d.s = &s
	default:
		return fmt.Errorf("allocationDate: unsupported type %T", v)
	}
	return nil
}

func (d allocationDate) str() string {
	if d.s == nil {
		return ""
	}
	return *d.s
}

func allocationDatePart(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// allocationTimestamp scans a DATETIME as RFC 3339 UTC.
type allocationTimestamp struct{ s *string }

func (d *allocationTimestamp) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		d.s = nil
	case time.Time:
		s := t.UTC().Format(time.RFC3339)
		d.s = &s
	case []byte, string:
		raw := fmt.Sprint(t)
		if b, ok := t.([]byte); ok {
			raw = string(b)
		}
		s := raw
		if ts, err := time.Parse("2006-01-02 15:04:05", allocationTrimFraction(raw)); err == nil {
			s = ts.UTC().Format(time.RFC3339)
		}
		d.s = &s
	default:
		return fmt.Errorf("allocationTimestamp: unsupported type %T", v)
	}
	return nil
}

func allocationTrimFraction(s string) string {
	if i := strings.IndexByte(s, '.'); i > 0 {
		return s[:i]
	}
	return s
}

// allocationBool scans a TINYINT flag (0/1) or a BOOL.
type allocationBool struct{ b bool }

func (d *allocationBool) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		d.b = false
	case bool:
		d.b = t
	case int64:
		d.b = t != 0
	case []byte:
		d.b = string(t) != "0" && string(t) != ""
	case string:
		d.b = t != "0" && t != ""
	default:
		return fmt.Errorf("allocationBool: unsupported type %T", v)
	}
	return nil
}

// Engagement ports the legacy getEngagement (by id), adding the allocation
// type's emailToAllocations and the engagement type's name and kind, plus
// getFundingSources for the one id.
func (r *allocationRepo) Engagement(ctx context.Context, id string) (*domain.AllocationEngagement, error) {
	var (
		e         domain.AllocationEngagement
		typeID    *int
		contracts allocationBool
		start     allocationDate
		end       allocationDate
	)
	err := r.alloc.QueryRowContext(ctx, `SELECT
			e.id, aT.engagement_type, eT.name, eT.type, e.allocation_type, aT.allocation_type, aT.emailToAllocations,
			e.customer_name, e.customer_code, e.project_name, e.opportunity_id, e.opportunity_line_item_id,
			e.product_name, REPLACE(e.location, 'BR()', CHAR(10)), e.country, e.business_domain, e.username, e.contracts_signed,
			e.engagement_code, DATE(e.start_date) AS startDate, DATE(e.end_date) AS endDate, e.status,
			e.engagement_status, e.engagement_scope, e.engagement_nature, e.customer_contact, e.owner,
			e.technical_owner, e.comment, e.description
		FROM engagement AS e
			LEFT OUTER JOIN allocation_type AS aT ON e.allocation_type = aT.id
			LEFT JOIN engagement_type AS eT ON aT.engagement_type = eT.id
		WHERE e.id != '' AND e.id = ?`, id).Scan(
		&e.ID, &typeID, &e.EngagementTypeName, &e.EngagementKind, &e.AllocationTypeID, &e.AllocationTypeName,
		&e.EmailToAllocations, &e.CustomerName, &e.CustomerCode, &e.Title, &e.OpportunityID, &e.ProductID,
		&e.ProductName, &e.Address, &e.Country, &e.BusinessDomain, &e.CreatedBy, &contracts,
		&e.EngagementCode, &start, &end, &e.RequestStatus,
		&e.EngagementStatus, &e.EngagementScope, &e.EngagementNature, &e.CustomerContact, &e.OpportunityOwner,
		&e.TechnicalOwner, &e.Comment, &e.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("allocation.Engagement: %w", err)
	}
	if typeID != nil {
		e.EngagementTypeID = *typeID
	}
	e.ContractsSigned = contracts.b
	e.StartDate, e.EndDate = start.s, end.s
	fs, err := r.fundingSources(ctx, []string{e.ID})
	if err != nil {
		return nil, err
	}
	e.FundingSources = fs
	return &e, nil
}

// fundingSources ports getFundingSources for a non-empty id list.
func (r *allocationRepo) fundingSources(ctx context.Context, engagementIDs []string) ([]domain.AllocationFundingSource, error) {
	out := []domain.AllocationFundingSource{}
	if len(engagementIDs) == 0 {
		return out, nil
	}
	rows, err := r.alloc.QueryContext(ctx, `SELECT
			eng_fs.engagement_id AS engagementId, business_unit.name AS businessUnit,
			product_unit.name AS productUnit, sub_region.name AS subRegion, region.name AS region,
			eng_fs.percentage
		FROM engagement_funding_source AS eng_fs
			LEFT JOIN business_unit ON eng_fs.business_unit_id = business_unit.id
			LEFT JOIN product_unit ON eng_fs.product_unit_id = product_unit.id
			LEFT JOIN region ON eng_fs.region_id = region.id
			LEFT JOIN sub_region ON eng_fs.sub_region_id = sub_region.id
		WHERE eng_fs.engagement_id IN (`+Placeholders(len(engagementIDs))+`)`, Args(engagementIDs)...)
	if err != nil {
		return nil, fmt.Errorf("allocation.fundingSources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f domain.AllocationFundingSource
		if err := rows.Scan(&f.EngagementID, &f.BusinessUnit, &f.ProductUnit, &f.SubRegion, &f.Region, &f.Percentage); err != nil {
			return nil, fmt.Errorf("allocation.fundingSources scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// allocationSelect is the column list every allocation read shares: the
// allocation (a), its own allocation type (aT), and, for engagement
// allocations, the engagement (e), the engagement's allocation type (eaT)
// and engagement type (eT).
const allocationSelect = `SELECT
			a.id, a.email, a.allocation_type, aT.allocation_type, aT.color,
			DATE(a.start_date) AS startDate, DATE(a.end_date) AS endDate,
			a.isRecurring, a.start_time, a.end_time, a.time_zone, a.comment, a.clearance_status,
			a.isTravelRequired, a.status, a.added_by, a.added_date, a.engagement_id, a.engagement_code,
			e.id, eaT.engagement_type, eT.name, eT.type, eaT.allocation_type, e.customer_name, e.project_name,
			e.engagement_code, e.country, REPLACE(e.location, 'BR()', CHAR(10)), e.status, e.engagement_status,
			DATE(e.start_date), DATE(e.end_date), e.opportunity_line_item_id`

type allocationScanner interface {
	Scan(dest ...any) error
}

func scanAllocationRecord(s allocationScanner) (domain.AllocationRecord, error) {
	var (
		a          domain.AllocationRecord
		start, end allocationDate
		added      allocationTimestamp
		startTime  *string
		endTime    *string
		engID      *string
		engTypeID  *int
		ref        domain.AllocationEngagementRef
		eStart     allocationDate
		eEnd       allocationDate
	)
	if err := s.Scan(
		&a.ID, &a.Email, &a.AllocationTypeID, &a.AllocationTypeName, &a.AllocationTypeColor,
		&start, &end,
		&a.IsRecurring, &startTime, &endTime, &a.TimeZone, &a.Comment, &a.ClearanceStatus,
		&a.IsTravelRequired, &a.Status, &a.AddedBy, &added, &a.EngagementID, &a.EngagementCode,
		&engID, &engTypeID, &ref.EngagementTypeName, &ref.EngagementKind, &ref.AllocationTypeName, &ref.CustomerName, &ref.Title,
		&ref.EngagementCode, &ref.Country, &ref.Address, &ref.RequestStatus, &ref.EngagementStatus,
		&eStart, &eEnd, &ref.ProductID,
	); err != nil {
		return a, err
	}
	a.StartDate, a.EndDate = start.str(), end.str()
	a.StartTime, a.EndTime = allocationTimeOf(startTime), allocationTimeOf(endTime)
	a.AddedDate = added.s
	if engID != nil && *engID != "" {
		ref.ID = *engID
		if engTypeID != nil {
			ref.EngagementTypeID = *engTypeID
		}
		ref.StartDate, ref.EndDate = eStart.s, eEnd.s
		a.Engagement = &ref
	}
	return a, nil
}

// allocationTimeOf trims a TIME value to HH:MM (legacy getTimeWithoutSecs).
func allocationTimeOf(t *string) *string {
	if t == nil {
		return nil
	}
	s := *t
	if len(s) >= 5 && s[2] == ':' {
		s = s[:5]
	}
	return &s
}

// allocationFrom is the FROM clause of single-allocation and timeline reads:
// every allocation (engagement or default) with its engagement when it has one.
const allocationFrom = `
		FROM allocation AS a
			LEFT JOIN allocation_type AS aT ON a.allocation_type = aT.id
			LEFT JOIN engagement AS e ON a.engagement_id = e.id
			LEFT JOIN allocation_type AS eaT ON e.allocation_type = eaT.id
			LEFT JOIN engagement_type AS eT ON eaT.engagement_type = eT.id`

// Get reads one allocation by id in any status (legacy GET allocations/{id}
// read only Active ones; the status and team edits need every status).
func (r *allocationRepo) Get(ctx context.Context, id string) (*domain.AllocationRecord, error) {
	return r.getWith(ctx, r.alloc, id)
}

type allocationQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *allocationRepo) getWith(ctx context.Context, q allocationQueryer, id string) (*domain.AllocationRecord, error) {
	a, err := scanAllocationRecord(q.QueryRowContext(ctx, allocationSelect+allocationFrom+`
		WHERE a.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("allocation.Get: %w", err)
	}
	return &a, nil
}

// Timeline ports getAllocationsByEmail: the people's Active allocations that
// touch the window, ordered by start date. The legacy query also right-joined
// allocation_db.employee, which only dropped allocations of people missing
// from that copy of HR; people come from the HR Entity now, so the join is
// left out. The engagement is joined for the timeline label.
func (r *allocationRepo) Timeline(ctx context.Context, req domain.AllocationTimelineRequest) ([]domain.AllocationRecord, error) {
	out := []domain.AllocationRecord{}
	if len(req.Emails) == 0 {
		return out, nil
	}
	q := allocationSelect + `
		FROM allocation AS a
			JOIN allocation_type AS aT ON a.allocation_type = aT.id
			LEFT JOIN engagement AS e ON a.engagement_id = e.id
			LEFT JOIN allocation_type AS eaT ON e.allocation_type = eaT.id
			LEFT JOIN engagement_type AS eT ON eaT.engagement_type = eT.id
		WHERE a.email IN (` + Placeholders(len(req.Emails)) + `) AND (
				((a.start_date BETWEEN ? AND ?) OR (a.end_date BETWEEN ? AND ?))
				OR ((? BETWEEN a.start_date AND a.end_date) AND (? BETWEEN a.start_date AND a.end_date))
				OR (a.start_date = ?)
				OR (a.end_date = ?)
				OR (a.start_date IS NULL)
			) AND a.status = 'Active'`
	args := Args(req.Emails)
	s, e := req.StartDate, req.EndDate
	args = append(args, s, e, s, e, s, e, s, e)
	if len(req.AllocationTypeIDs) > 0 {
		q += ` AND a.allocation_type IN (` + Placeholders(len(req.AllocationTypeIDs)) + `)`
		args = append(args, Args(req.AllocationTypeIDs)...)
	}
	q += ` ORDER BY a.start_date, a.id`
	rows, err := r.alloc.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("allocation.Timeline: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAllocationRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("allocation.Timeline scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// allocationSearchWhere builds the WHERE of the Engagement Allocations list
// (legacy getEngagementAllocations with statuses ["Active"], plus the
// Release 1 list filters).
func allocationSearchWhere(req domain.AllocationSearchRequest) (string, []any) {
	w := ` WHERE e.status != 'Deleted' AND a.status IN ('Active')` +
		` AND eaT.engagement_type IN (` + Placeholders(len(req.EngagementTypeIDs)) + `)`
	args := Args(req.EngagementTypeIDs)
	if len(req.ClearanceStatuses) > 0 {
		w += ` AND a.clearance_status IN (` + Placeholders(len(req.ClearanceStatuses)) + `)`
		args = append(args, Args(req.ClearanceStatuses)...)
	}
	if len(req.Consultants) > 0 {
		w += ` AND a.email IN (` + Placeholders(len(req.Consultants)) + `)`
		args = append(args, Args(req.Consultants)...)
	}
	if len(req.AllocationTypes) > 0 {
		w += ` AND eaT.allocation_type IN (` + Placeholders(len(req.AllocationTypes)) + `)`
		args = append(args, Args(req.AllocationTypes)...)
	}
	if req.TravelRequired != nil {
		if *req.TravelRequired {
			w += ` AND a.isTravelRequired = 'Yes'`
		} else {
			w += ` AND (a.isTravelRequired IS NULL OR a.isTravelRequired != 'Yes')`
		}
	}
	if req.StartFrom != "" {
		w += ` AND a.start_date >= ?`
		args = append(args, req.StartFrom)
	}
	if req.EndTo != "" {
		w += ` AND a.end_date <= ?`
		args = append(args, req.EndTo+" 23:59:59")
	}
	if req.Search != "" {
		like := LikeContains(req.Search)
		w += ` AND (a.id LIKE ? OR a.email LIKE ? OR e.id LIKE ? OR e.project_name LIKE ?` +
			` OR e.customer_name LIKE ? OR e.engagement_code LIKE ?)`
		args = append(args, like, like, like, like, like, like)
	}
	return w, args
}

const allocationSearchFrom = `
		FROM allocation AS a
			JOIN engagement AS e ON a.engagement_id = e.id
			JOIN allocation_type AS eaT ON e.allocation_type = eaT.id
			JOIN engagement_type AS eT ON eaT.engagement_type = eT.id
			LEFT JOIN allocation_type AS aT ON a.allocation_type = aT.id`

// allocationSortColumns whitelists the column sorts of the allocations list.
// "status" is the allocation status (clearance_status) and "created" the
// added date.
var allocationSortColumns = SortColumns{
	"id":         {`a.id`},
	"consultant": {`a.email`},
	"engagement": {`e.id`},
	"type":       {`eaT.allocation_type`},
	"startDate":  {`a.start_date`},
	"status":     {`a.clearance_status`},
	"created":    {`a.added_date`},
}

// Search returns one page of the Engagement Allocations list, current
// allocations first (ending today or later, by start date), then finished
// ones (latest start first), with the total and each row's travel rows.
func (r *allocationRepo) Search(ctx context.Context, req domain.AllocationSearchRequest) ([]domain.AllocationRecord, int, error) {
	out := []domain.AllocationRecord{}
	if len(req.EngagementTypeIDs) == 0 {
		return out, 0, nil
	}
	where, args := allocationSearchWhere(req)

	var total int
	if err := r.alloc.QueryRowContext(ctx, `SELECT COUNT(*)`+allocationSearchFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("allocation.Search count: %w", err)
	}
	if total == 0 {
		return out, 0, nil
	}

	order := ` ORDER BY (a.end_date >= ?) DESC, CASE WHEN a.end_date >= ? THEN a.start_date END ASC, a.start_date DESC, a.id ASC`
	orderArgs := []any{req.Today, req.Today}
	if o, ok := allocationSortColumns.Order(req.Sort, `a.id`); ok {
		order, orderArgs = o, nil
	}
	q := allocationSelect + allocationSearchFrom + where + order + ` LIMIT ? OFFSET ?`
	pageArgs := append(append(append([]any{}, args...), orderArgs...), req.Limit, req.Offset)
	rows, err := r.alloc.QueryContext(ctx, q, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("allocation.Search: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAllocationRecord(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("allocation.Search scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	ids := make([]string, len(out))
	for i, a := range out {
		ids[i] = a.ID
	}
	trips, err := r.travelRowsOf(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].TravelRequests = trips[out[i].ID]
	}
	return out, total, nil
}

// travelRowsOf reads the travelapp.travel rows created for the allocations.
func (r *allocationRepo) travelRowsOf(ctx context.Context, allocationIDs []string) (map[string][]domain.AllocationTravelRef, error) {
	out := map[string][]domain.AllocationTravelRef{}
	if len(allocationIDs) == 0 {
		return out, nil
	}
	rows, err := r.travel.QueryContext(ctx, `SELECT job_number, allocation_id, traveller_email, status
		FROM travel WHERE allocation_id IN (`+Placeholders(len(allocationIDs))+`) ORDER BY job_number`,
		Args(allocationIDs)...)
	if err != nil {
		return nil, fmt.Errorf("allocation.travelRows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t domain.AllocationTravelRef
		if err := rows.Scan(&t.JobNumber, &t.AllocationID, &t.TravellerEmail, &t.Status); err != nil {
			return nil, fmt.Errorf("allocation.travelRows scan: %w", err)
		}
		out[t.AllocationID] = append(out[t.AllocationID], t)
	}
	return out, rows.Err()
}

// Visas ports the travel readiness lookup of the Confirmed email
// (getVisaDetails with {email, minStartDate, minEndDate}: the legacy filter's
// countryCode is not a VisaFilter field, so visas of every country count),
// with the entity's default page of 100.
func (r *allocationRepo) Visas(ctx context.Context, email, startDate, endDate string) ([]domain.AllocationVisa, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT country, visa_type, entry_type, status, start_date, end_date
		FROM visa_status
		WHERE email = ? AND end_date >= ? AND start_date <= ?
		LIMIT 100 OFFSET 0`, email, endDate, startDate)
	if err != nil {
		return nil, fmt.Errorf("allocation.Visas: %w", err)
	}
	defer rows.Close()
	out := []domain.AllocationVisa{}
	for rows.Next() {
		var (
			v          domain.AllocationVisa
			start, end allocationDate
		)
		if err := rows.Scan(&v.Country, &v.VisaType, &v.EntryType, &v.Status, &start, &end); err != nil {
			return nil, fmt.Errorf("allocation.Visas scan: %w", err)
		}
		v.StartDate, v.EndDate = start.s, end.s
		out = append(out, v)
	}
	return out, rows.Err()
}
