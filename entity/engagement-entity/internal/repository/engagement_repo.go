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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ErrEngagementNotFound is returned when no engagement has the id.
var ErrEngagementNotFound = errors.New("engagement not found")

// ErrEngagementAllocationTypeNotFound is returned by EngagementTypeOf for an
// unknown allocation type id.
var ErrEngagementAllocationTypeNotFound = errors.New("allocation type not found")

// EngagementBeforeCommit runs inside the create transaction after the row
// and its funding sources are written (the legacy NetSuite marketing event
// call). An error rolls the insert back.
type EngagementBeforeCommit func(ctx context.Context, engagementID string, engagementTypeID int) error

// EngagementRepository reads and writes allocation_db.engagement and, for the
// cancel cascade, travelapp.travel. Every query is ported from the legacy
// Finance Entity (entity/modules/database/engagement.bal, funding_source.bal).
type EngagementRepository interface {
	Search(ctx context.Context, req domain.EngagementSearchRequest) ([]domain.Engagement, int, error)
	TypeCounts(ctx context.Context, req domain.EngagementSearchRequest) (map[int]int, error)
	Facets(ctx context.Context, engagementTypeIDs []int) (domain.EngagementFacetsResponse, error)
	Get(ctx context.Context, id string) (domain.Engagement, error)
	FundingSources(ctx context.Context, ids []string) (map[string][]domain.EngagementFundingSource, error)
	Consultants(ctx context.Context, ids []string) (map[string][]string, error)
	// EngagementTypeOf returns the engagement type of an allocation type.
	EngagementTypeOf(ctx context.Context, allocationTypeID int) (int, error)
	// Create inserts the engagement and returns its trigger-generated id.
	Create(ctx context.Context, req domain.EngagementCreateRequest, beforeCommit EngagementBeforeCommit) (string, error)
	// Update applies req; a Cancelled engagement status cascades to the
	// allocations and their travel requests. It returns the Active
	// allocations read before a cancellation.
	Update(ctx context.Context, id string, req domain.EngagementUpdateRequest) ([]domain.EngagementAllocation, error)
	// Allocations returns the engagement's allocations (Active only when
	// activeOnly) with their travel requests.
	Allocations(ctx context.Context, id string, activeOnly bool) ([]domain.EngagementAllocation, error)
}

type engagementRepo struct {
	alloc  *sql.DB
	travel *sql.DB
}

// NewEngagementRepository constructs an EngagementRepository over the
// allocation_db and travelapp pools.
func NewEngagementRepository(alloc, travel *sql.DB) EngagementRepository {
	return &engagementRepo{alloc: alloc, travel: travel}
}

// engagementSelect() is the legacy getEngagement column list plus the
// creation time (last_updated_date, set on insert only) and the Release 1
// campaign_id. Dates are read as YYYY-MM-DD, as the legacy DATE() did.
func engagementSelect() string {
	return `SELECT e.id, aT.engagement_type, eT.name, e.allocation_type, aT.allocation_type,
		e.customer_code, e.customer_name, e.project_name, e.opportunity_id, e.opportunity_line_item_id,
		e.product_name, REPLACE(e.location, 'BR()', CHAR(10)), e.country, e.business_domain, e.username, e.contracts_signed,
		e.engagement_code, DATE_FORMAT(e.start_date, '%Y-%m-%d'), DATE_FORMAT(e.end_date, '%Y-%m-%d'),
		e.status, e.engagement_scope, e.engagement_nature, e.engagement_status, e.customer_contact,
		e.owner, COALESCE(e.tentative_dates = 1, 0) AS tentative_dates, e.technical_owner, e.comment, e.description, e.updated_by,
		e.last_updated_date, ` + campaignCol("e") + `
	FROM engagement AS e
		LEFT OUTER JOIN allocation_type AS aT ON e.allocation_type = aT.id
		LEFT JOIN engagement_type AS eT ON aT.engagement_type = eT.id`
}

type engagementScanner interface {
	Scan(dest ...any) error
}

func engagementScan(s engagementScanner) (domain.Engagement, error) {
	var (
		e                                                                          domain.Engagement
		engType, allocType                                                         sql.NullInt64
		engTypeName, allocTypeName, custCode, custName, project, oppID, lineItem   sql.NullString
		product, location, country, domainName, username, code, start, end         sql.NullString
		status, scope, nature, engStatus, contact, owner, techOwner, comment, desc sql.NullString
		updatedBy, campaign                                                        sql.NullString
		contracts, tentative                                                       sql.NullBool
		created                                                                    sql.NullTime
	)
	if err := s.Scan(&e.ID, &engType, &engTypeName, &allocType, &allocTypeName,
		&custCode, &custName, &project, &oppID, &lineItem,
		&product, &location, &country, &domainName, &username, &contracts,
		&code, &start, &end,
		&status, &scope, &nature, &engStatus, &contact,
		&owner, &tentative, &techOwner, &comment, &desc, &updatedBy,
		&created, &campaign); err != nil {
		return domain.Engagement{}, err
	}
	e.EngagementTypeID = int(engType.Int64)
	e.EngagementTypeName = engagementStr(engTypeName)
	e.AllocationTypeID = int(allocType.Int64)
	e.AllocationTypeName = engagementStr(allocTypeName)
	e.CustomerCode, e.CustomerName, e.ProjectName = engagementStr(custCode), engagementStr(custName), engagementStr(project)
	e.OpportunityID, e.OpportunityLineItemID = engagementStr(oppID), engagementStr(lineItem)
	e.ProductName, e.Address, e.Country = engagementStr(product), engagementStr(location), engagementStr(country)
	e.BusinessDomain, e.CreatedBy = engagementStr(domainName), engagementStr(username)
	e.ContractsSigned, e.TentativeDates = engagementBool(contracts), engagementBool(tentative)
	e.EngagementCode, e.StartDate, e.EndDate = engagementStr(code), engagementStr(start), engagementStr(end)
	e.RequestStatus, e.EngagementScope, e.EngagementNature = engagementStr(status), engagementStr(scope), engagementStr(nature)
	e.EngagementStatus, e.CustomerContact, e.OpportunityOwner = engagementStr(engStatus), engagementStr(contact), engagementStr(owner)
	e.TechnicalOwner, e.Comment, e.Description = engagementStr(techOwner), engagementStr(comment), engagementStr(desc)
	e.UpdatedBy, e.CampaignID = engagementStr(updatedBy), engagementStr(campaign)
	if created.Valid {
		s := created.Time.UTC().Format(time.RFC3339)
		e.CreatedOn = &s
	}
	e.FundingSources = []domain.EngagementFundingSource{}
	return e, nil
}

func engagementStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func engagementBool(nb sql.NullBool) *bool {
	if !nb.Valid {
		return nil
	}
	b := nb.Bool
	return &b
}

// engagementBase is the status condition every search starts from: only
// Pending Approval (approvals, optionally the creator's own), everything but
// Pending Approval (the legacy list), or, with IncludePending, no condition.
func engagementBase(req domain.EngagementSearchRequest) (string, []any) {
	switch {
	case req.PendingApproval && req.PendingCreator != "":
		return `e.status = 'Pending Approval' AND LOWER(e.username) = LOWER(?)`, []any{req.PendingCreator}
	case req.PendingApproval:
		return `e.status = 'Pending Approval'`, nil
	case req.IncludePending:
		return "", nil
	}
	return `e.status != 'Pending Approval'`, nil
}

// engagementWhere builds the search WHERE clause. The base condition is the
// legacy one: getEngagements excludes Pending Approval, the approval search
// lists only it.
func engagementWhere(req domain.EngagementSearchRequest) (string, []any) {
	conds := []string{`e.id != ''`}
	var args []any
	if c, a := engagementBase(req); c != "" {
		conds = append(conds, c)
		args = append(args, a...)
	}
	in := func(col string, vals []string) {
		if len(vals) > 0 {
			conds = append(conds, col+` IN (`+Placeholders(len(vals))+`)`)
			args = append(args, Args(vals)...)
		}
	}
	if len(req.EngagementTypeIDs) > 0 {
		conds = append(conds, `aT.engagement_type IN (`+Placeholders(len(req.EngagementTypeIDs))+`)`)
		args = append(args, Args(req.EngagementTypeIDs)...)
	}
	in(`e.customer_name`, req.Accounts)
	in(`aT.allocation_type`, req.AllocationTypes)
	in(`e.status`, req.RequestStatuses)
	in(`e.engagement_status`, req.EngagementStatuses)
	in(`e.country`, req.Countries)
	if len(req.BusinessUnits) > 0 {
		conds = append(conds, `EXISTS (SELECT 1 FROM engagement_funding_source AS eFS
			JOIN business_unit AS bu ON eFS.business_unit_id = bu.id
			WHERE eFS.engagement_id = e.id AND bu.name IN (`+Placeholders(len(req.BusinessUnits))+`))`)
		args = append(args, Args(req.BusinessUnits)...)
	}
	switch req.Consultants {
	case "allocated":
		conds = append(conds, `EXISTS (SELECT 1 FROM allocation AS a WHERE a.engagement_id = e.id AND a.status = 'Active')`)
	case "none":
		conds = append(conds, `NOT EXISTS (SELECT 1 FROM allocation AS a WHERE a.engagement_id = e.id AND a.status = 'Active')`)
	}
	if req.StartFrom != "" {
		conds = append(conds, `DATE(e.start_date) >= ?`)
		args = append(args, req.StartFrom)
	}
	if req.StartTo != "" {
		conds = append(conds, `DATE(e.start_date) <= ?`)
		args = append(args, req.StartTo)
	}
	if req.EndTo != "" {
		conds = append(conds, `DATE(e.end_date) <= ?`)
		args = append(args, req.EndTo)
	}
	if s := strings.TrimSpace(req.Search); s != "" {
		cols := []string{`e.id`, `e.customer_name`, `e.project_name`, `e.engagement_code`, `e.country`, `aT.allocation_type`}
		if req.PendingApproval {
			cols = append(cols, `e.username`)
		}
		parts := make([]string, len(cols))
		like := LikeContains(s)
		for i, c := range cols {
			parts[i] = c + ` LIKE ?`
			args = append(args, like)
		}
		conds = append(conds, `(`+strings.Join(parts, ` OR `)+`)`)
	}
	return ` WHERE ` + strings.Join(conds, ` AND `), args
}

// engagementSortColumns whitelists the column sorts of the engagements list.
// "created" is the creation time (last_updated_date, which is set on insert
// only); the Approvals list has always used it as its oldest-first order.
var engagementSortColumns = SortColumns{
	"id":               {`e.id`},
	"type":             {`eT.name`},
	"country":          {`e.country`},
	"startDate":        {`e.start_date`},
	"requestStatus":    {`e.status`},
	"engagementStatus": {`e.engagement_status`},
	"created":          {`e.last_updated_date`},
}

// approvalSortColumns whitelists the column sorts of the approvals list. The
// business unit is the first of the engagement's funding sources by name.
var approvalSortColumns = SortColumns{
	"id":        {`e.id`},
	"type":      {`eT.name`},
	"country":   {`e.country`},
	"startDate": {`e.start_date`},
	"businessUnit": {`(SELECT MIN(bu.name) FROM engagement_funding_source AS eFS
			JOIN business_unit AS bu ON eFS.business_unit_id = bu.id WHERE eFS.engagement_id = e.id)`},
	"created": {`e.last_updated_date`},
}

// engagementOrder returns the ORDER BY of the engagement search and its bind
// arguments. A whitelisted sort wins; "created" alone is the Approvals
// default; anything else (empty, "current", unknown) is the current-first
// order, which needs req.Today.
func engagementOrder(req domain.EngagementSearchRequest) (string, []any) {
	cols := engagementSortColumns
	if req.PendingApproval {
		cols = approvalSortColumns
	}
	if o, ok := cols.Order(req.Sort, `e.id`); ok {
		return o, nil
	}
	return ` ORDER BY CASE WHEN DATE(e.end_date) >= ? THEN 0 ELSE 1 END,
			CASE WHEN DATE(e.end_date) >= ? THEN e.start_date END ASC,
			e.start_date DESC, e.id ASC`, []any{req.Today, req.Today}
}

func (r *engagementRepo) Search(ctx context.Context, req domain.EngagementSearchRequest) ([]domain.Engagement, int, error) {
	where, args := engagementWhere(req)
	const from = ` FROM engagement AS e
		LEFT OUTER JOIN allocation_type AS aT ON e.allocation_type = aT.id
		LEFT JOIN engagement_type AS eT ON aT.engagement_type = eT.id`
	var total int
	if err := r.alloc.QueryRowContext(ctx, `SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("engagement.Search count: %w", err)
	}
	order, pageArgs := engagementOrder(req)
	pageArgs = append(append([]any{}, args...), pageArgs...)
	pageArgs = append(pageArgs, req.Limit, req.Offset)
	rows, err := r.alloc.QueryContext(ctx, engagementSelect()+where+order+` LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("engagement.Search: %w", err)
	}
	defer rows.Close()
	out := []domain.Engagement{}
	for rows.Next() {
		e, err := engagementScan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("engagement.Search scan: %w", err)
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *engagementRepo) TypeCounts(ctx context.Context, req domain.EngagementSearchRequest) (map[int]int, error) {
	base, args := engagementBase(req)
	if base != "" {
		base = ` AND ` + base
	}
	rows, err := r.alloc.QueryContext(ctx, `SELECT aT.engagement_type, COUNT(*)
		FROM engagement AS e
		JOIN allocation_type AS aT ON e.allocation_type = aT.id
		WHERE e.id != ''`+base+`
		GROUP BY aT.engagement_type`, args...)
	if err != nil {
		return nil, fmt.Errorf("engagement.TypeCounts: %w", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var t sql.NullInt64
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return nil, fmt.Errorf("engagement.TypeCounts scan: %w", err)
		}
		if t.Valid {
			out[int(t.Int64)] = n
		}
	}
	return out, rows.Err()
}

func (r *engagementRepo) Facets(ctx context.Context, engagementTypeIDs []int) (domain.EngagementFacetsResponse, error) {
	out := domain.EngagementFacetsResponse{Accounts: []string{}, AllocationTypes: []string{}, Countries: []string{}}
	if len(engagementTypeIDs) == 0 {
		return out, nil
	}
	typeIn := Placeholders(len(engagementTypeIDs))
	for _, f := range []struct {
		col  string
		dest *[]string
	}{
		{`e.customer_name`, &out.Accounts},
		{`aT.allocation_type`, &out.AllocationTypes},
		{`e.country`, &out.Countries},
	} {
		q := `SELECT DISTINCT ` + f.col + ` FROM engagement AS e
			JOIN allocation_type AS aT ON e.allocation_type = aT.id
			WHERE e.id != '' AND e.status != 'Pending Approval' AND aT.engagement_type IN (` + typeIn + `)
			AND ` + f.col + ` IS NOT NULL AND ` + f.col + ` != ''
			ORDER BY ` + f.col
		rows, err := r.alloc.QueryContext(ctx, q, Args(engagementTypeIDs)...)
		if err != nil {
			return out, fmt.Errorf("engagement.Facets: %w", err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return out, fmt.Errorf("engagement.Facets scan: %w", err)
			}
			*f.dest = append(*f.dest, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, fmt.Errorf("engagement.Facets rows: %w", err)
		}
	}
	return out, nil
}

func (r *engagementRepo) Get(ctx context.Context, id string) (domain.Engagement, error) {
	e, err := engagementScan(r.alloc.QueryRowContext(ctx, engagementSelect()+` WHERE e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Engagement{}, ErrEngagementNotFound
	}
	if err != nil {
		return domain.Engagement{}, fmt.Errorf("engagement.Get: %w", err)
	}
	return e, nil
}

// FundingSources ports getFundingSources for the given engagement ids. An
// empty list reads nothing (the legacy query read the whole table).
func (r *engagementRepo) FundingSources(ctx context.Context, ids []string) (map[string][]domain.EngagementFundingSource, error) {
	out := map[string][]domain.EngagementFundingSource{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.alloc.QueryContext(ctx, `SELECT eng_fs.engagement_id, business_unit.name, product_unit.name,
			sub_region.name, region.name, eng_fs.percentage
		FROM engagement_funding_source AS eng_fs
			LEFT JOIN business_unit ON eng_fs.business_unit_id = business_unit.id
			LEFT JOIN product_unit ON eng_fs.product_unit_id = product_unit.id
			LEFT JOIN region ON eng_fs.region_id = region.id
			LEFT JOIN sub_region ON eng_fs.sub_region_id = sub_region.id
		WHERE eng_fs.engagement_id IN (`+Placeholders(len(ids))+`)`, Args(ids)...)
	if err != nil {
		return nil, fmt.Errorf("engagementFundingSource.List: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id              string
			bu, pu, sub, rg sql.NullString
			pct             sql.NullFloat64
		)
		if err := rows.Scan(&id, &bu, &pu, &sub, &rg, &pct); err != nil {
			return nil, fmt.Errorf("engagementFundingSource.List scan: %w", err)
		}
		out[id] = append(out[id], domain.EngagementFundingSource{
			BusinessUnit: bu.String, ProductUnit: pu.String, SubRegion: sub.String, Region: rg.String, Percentage: pct.Float64,
		})
	}
	return out, rows.Err()
}

// Consultants ports getEngagementConsultants: the Active allocations'
// emails, distinct per engagement.
func (r *engagementRepo) Consultants(ctx context.Context, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.alloc.QueryContext(ctx, `SELECT DISTINCT engagement_id, email FROM allocation
		WHERE status = 'Active' AND engagement_id IN (`+Placeholders(len(ids))+`)
		ORDER BY engagement_id, email`, Args(ids)...)
	if err != nil {
		return nil, fmt.Errorf("engagement.Consultants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, fmt.Errorf("engagement.Consultants scan: %w", err)
		}
		out[id] = append(out[id], email)
	}
	return out, rows.Err()
}

// EngagementTypeOf ports getEngagementTypeFromAllocationTypeId.
func (r *engagementRepo) EngagementTypeOf(ctx context.Context, allocationTypeID int) (int, error) {
	var t sql.NullInt64
	err := r.alloc.QueryRowContext(ctx, `SELECT engagement_type FROM allocation_type WHERE id = ?`, allocationTypeID).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !t.Valid) {
		return 0, ErrEngagementAllocationTypeNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("allocationType.EngagementType: %w", err)
	}
	return int(t.Int64), nil
}

// engagementInsertFundingSource ports addFundingSourcesQuery: the ids are
// resolved by name with a cross join, so unknown names insert nothing.
const engagementInsertFundingSource = `INSERT INTO engagement_funding_source
		(engagement_id, business_unit_id, product_unit_id, region_id, sub_region_id, percentage)
	SELECT ?, business_unit.id, product_unit.id, region.id, sub_region.id, ?
	FROM business_unit, product_unit, region, sub_region
	WHERE business_unit.name = ? AND product_unit.name = ? AND region.name = ? AND sub_region.name = ?`

func engagementInsertFunding(ctx context.Context, tx *sql.Tx, id string, fs []domain.EngagementFundingSource) error {
	for _, f := range fs {
		if _, err := tx.ExecContext(ctx, engagementInsertFundingSource,
			id, f.Percentage, f.BusinessUnit, f.ProductUnit, f.Region, f.SubRegion); err != nil {
			return fmt.Errorf("engagementFundingSource.Insert: %w", err)
		}
	}
	return nil
}

// Create ports addEngagement: insert, re-read by uuid (the id comes from a
// trigger), insert the funding sources, run beforeCommit (the NetSuite
// marketing event), commit. technical_contact, opportunity_size and
// additional_material are written empty, as the legacy webapp sent them.
func (r *engagementRepo) Create(ctx context.Context, req domain.EngagementCreateRequest, beforeCommit EngagementBeforeCommit) (string, error) {
	engType, err := r.EngagementTypeOf(ctx, req.AllocationTypeID)
	if err != nil {
		return "", err
	}
	if req.CampaignID != nil && !HasEngagementCampaignColumn() {
		return "", errEngagementNoCampaignColumn
	}
	campaignColumn, campaignValue := ", campaign_id", ", ?"
	if !HasEngagementCampaignColumn() {
		campaignColumn, campaignValue = "", ""
	}
	insertArgs := []any{req.AllocationTypeID, req.CustomerName, req.CustomerCode, req.Address,
		req.BusinessDomain, req.ContractsSigned, req.EngagementCode, req.StartDate, req.EndDate,
		req.RequestStatus, req.ProjectName, req.OpportunityID, req.OpportunityLineItemID, req.OpportunityOwner, req.ProductName,
		req.EngagementScope, req.EngagementStatus, req.CreatedBy, req.TentativeDates, req.EngagementNature, req.CustomerContact,
		req.TechnicalOwner, req.Comment, req.Country, req.UUID, req.Description}
	if HasEngagementCampaignColumn() {
		insertArgs = append(insertArgs, req.CampaignID)
	}
	var id string
	err = WithTx(ctx, r.alloc, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO engagement (
				allocation_type, customer_name, customer_code, location, technical_contact, opportunity_size,
				business_domain, additional_material, contracts_signed, engagement_code, start_date, end_date,
				status, project_name, opportunity_id, opportunity_line_item_id, owner, last_updated_date, product_name,
				engagement_scope, engagement_status, username, tentative_dates, engagement_nature, customer_contact,
				technical_owner, comment, country, uuid, description`+campaignColumn+`)
			VALUES (?, ?, ?, ?, '', '', ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP(), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`+campaignValue+`)`,
			insertArgs...); err != nil {
			return fmt.Errorf("engagement.Insert: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM engagement WHERE uuid = ?`, req.UUID).Scan(&id); err != nil {
			return fmt.Errorf("engagement.Insert re-read: %w", err)
		}
		if err := engagementInsertFunding(ctx, tx, id, req.FundingSources); err != nil {
			return err
		}
		if err := audit.Inserted("engagement", "id", id).Record(ctx, tx,
			engagementAuditEvent(audit.Create, req.CreatedBy, req.FundingSources)); err != nil {
			return err
		}
		if beforeCommit != nil {
			return beforeCommit(ctx, id, engType)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// engagementSet builds the legacy updateEngagement SET list (same order),
// plus the Release 1 columns description, project_name and campaign_id.
func engagementSet(req domain.EngagementUpdateRequest) (string, []any) {
	set := `updated_by = ?`
	args := []any{req.UpdatedBy}
	add := func(col string, v any) {
		set += `, ` + col + ` = ?`
		args = append(args, v)
	}
	if req.Address != nil {
		add(`location`, *req.Address)
	}
	if req.AllocationTypeID != nil {
		add(`allocation_type`, *req.AllocationTypeID)
	}
	if req.EngagementCode != nil {
		add(`engagement_code`, *req.EngagementCode)
	}
	if req.EngagementScope != nil {
		add(`engagement_scope`, *req.EngagementScope)
	}
	if req.EngagementNature != nil {
		add(`engagement_nature`, *req.EngagementNature)
	}
	if req.Comment != nil {
		add(`comment`, *req.Comment)
	}
	if req.StartDate != nil {
		add(`start_date`, *req.StartDate)
	}
	if req.EndDate != nil {
		add(`end_date`, *req.EndDate)
	}
	if req.EngagementStatus != nil {
		add(`engagement_status`, *req.EngagementStatus)
	}
	if req.RequestStatus != nil {
		add(`status`, *req.RequestStatus)
	}
	if req.Country != nil {
		add(`country`, *req.Country)
	}
	if req.CustomerContact != nil {
		add(`customer_contact`, *req.CustomerContact)
	}
	if req.Description != nil {
		add(`description`, *req.Description)
	}
	if req.ProjectName != nil {
		add(`project_name`, *req.ProjectName)
	}
	if req.TentativeDates != nil {
		add(`tentative_dates`, *req.TentativeDates)
	}
	if req.CampaignID != nil && HasEngagementCampaignColumn() {
		add(`campaign_id`, *req.CampaignID)
	}
	return set, args
}

// Update ports updateEngagement. For a cancellation the Active allocations
// are read first (outside the transaction, as legacy); then, in one
// allocation_db transaction: the engagement UPDATE, the funding source
// replace (only when rows are sent), every allocation of the engagement set
// to Allocation Cancelled / Inactive, and, before that commits, a travelapp
// transaction cancelling the travel requests of the pre-read allocations
// that were Confirmed or Confirmed - Visa Pending with travel required. A
// travel failure rolls the allocation_db changes back; the two commits are
// separate (not XA), as in legacy.
func (r *engagementRepo) Update(ctx context.Context, id string, req domain.EngagementUpdateRequest) ([]domain.EngagementAllocation, error) {
	cancel := req.EngagementStatus != nil && *req.EngagementStatus == domain.EngagementStatusCancelled
	var before []domain.EngagementAllocation
	if cancel {
		var err error
		if before, err = r.allocations(ctx, id, true); err != nil {
			return nil, err
		}
	}
	set, args := engagementSet(req)
	err := WithTx(ctx, r.alloc, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "engagement", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE engagement SET `+set+` WHERE id = ?`, append(args, id)...)
		if err != nil {
			return fmt.Errorf("engagement.Update: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrEngagementNotFound
		}
		if len(req.FundingSources) > 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM engagement_funding_source WHERE engagement_id = ?`, id); err != nil {
				return fmt.Errorf("engagementFundingSource.Delete: %w", err)
			}
			if err := engagementInsertFunding(ctx, tx, id, req.FundingSources); err != nil {
				return err
			}
		}
		if err := t.Record(ctx, tx, engagementAuditEvent(audit.Update, req.UpdatedBy, req.FundingSources)); err != nil {
			return err
		}
		if !cancel {
			return nil
		}
		ta, err := audit.Track(ctx, tx, "allocation", "id", `engagement_id = ?`, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE allocation SET updated_by = ?, clearance_status = ?, status = ?
			WHERE engagement_id = ?`,
			req.UpdatedBy, domain.EngagementClearanceAllocationCancelled, domain.EngagementAllocationInactive, id); err != nil {
			return fmt.Errorf("allocation.CancelForEngagement: %w", err)
		}
		if err := ta.Record(ctx, tx, allocationAuditEvent(audit.StatusChange, req.UpdatedBy)); err != nil {
			return err
		}
		travels := engagementTravelToCancel(before)
		if len(travels) == 0 {
			return nil
		}
		return WithTx(ctx, r.travel, func(ttx *sql.Tx) error {
			for _, a := range travels {
				tt, err := audit.Track(ctx, ttx, "travel", "job_number", `allocation_id = ? AND traveller_email = ?`, a.ID, a.Email)
				if err != nil {
					return err
				}
				if _, err := ttx.ExecContext(ctx, `UPDATE travel SET status = ?, last_updated_by = ?, last_updated_date = NOW()
					WHERE allocation_id = ? AND traveller_email = ?`,
					domain.EngagementTravelStatusCancelled, req.UpdatedBy, a.ID, a.Email); err != nil {
					return fmt.Errorf("travel.CancelForEngagement: %w", err)
				}
				if err := tt.Record(ctx, ttx, travelAuditEvent(audit.StatusChange, req.UpdatedBy)); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	if before == nil {
		before = []domain.EngagementAllocation{}
	}
	return before, nil
}

func engagementAuditEvent(action audit.Action, actor string, funding []domain.EngagementFundingSource) audit.Event {
	e := audit.Event{Action: action, EntityType: "engagement", Actor: actor}
	if len(funding) > 0 {
		e.Metadata = map[string]any{"fundingSources": funding}
	}
	return e
}

// engagementTravelToCancel returns the allocations whose travel the legacy
// cascade cancels: clearance Confirmed or Confirmed - Visa Pending and
// travel required Yes. A NULL clearance or travel flag does not match (the
// legacy cast panicked on it and failed the whole cancellation).
func engagementTravelToCancel(allocs []domain.EngagementAllocation) []domain.EngagementAllocation {
	var out []domain.EngagementAllocation
	for _, a := range allocs {
		if a.ClearanceStatus == nil || a.IsTravelRequired == nil {
			continue
		}
		c := *a.ClearanceStatus
		if (c == domain.EngagementClearanceConfirmed || c == domain.EngagementClearanceConfirmedVisaPend) &&
			*a.IsTravelRequired == domain.EngagementTravelRequiredYes {
			out = append(out, a)
		}
	}
	return out
}

func (r *engagementRepo) allocations(ctx context.Context, id string, activeOnly bool) ([]domain.EngagementAllocation, error) {
	q := `SELECT a.id, a.email, a.allocation_type, DATE_FORMAT(a.start_date, '%Y-%m-%d'),
			DATE_FORMAT(a.end_date, '%Y-%m-%d'), a.comment, a.clearance_status, a.isTravelRequired,
			a.added_by, a.added_date, a.isRecurring, a.start_time, a.end_time, a.time_zone, a.status
		FROM allocation AS a
		WHERE a.engagement_id = ?`
	if activeOnly {
		q += ` AND a.status = 'Active'`
	}
	rows, err := r.alloc.QueryContext(ctx, q+` ORDER BY a.id`, id)
	if err != nil {
		return nil, fmt.Errorf("engagement.Allocations: %w", err)
	}
	defer rows.Close()
	out := []domain.EngagementAllocation{}
	for rows.Next() {
		var (
			a                                                                  domain.EngagementAllocation
			allocType                                                          sql.NullInt64
			start, end, comment, clearance, travel, addedBy, recurring, status sql.NullString
			startTime, endTime, tz                                             sql.NullString
			added                                                              sql.NullTime
		)
		if err := rows.Scan(&a.ID, &a.Email, &allocType, &start, &end, &comment, &clearance, &travel,
			&addedBy, &added, &recurring, &startTime, &endTime, &tz, &status); err != nil {
			return nil, fmt.Errorf("engagement.Allocations scan: %w", err)
		}
		if allocType.Valid {
			v := int(allocType.Int64)
			a.AllocationTypeID = &v
		}
		a.StartDate, a.EndDate, a.Comment = engagementStr(start), engagementStr(end), engagementStr(comment)
		a.ClearanceStatus, a.IsTravelRequired, a.AddedBy = engagementStr(clearance), engagementStr(travel), engagementStr(addedBy)
		a.IsRecurring, a.StartTime, a.EndTime = engagementStr(recurring), engagementStr(startTime), engagementStr(endTime)
		a.TimeZone, a.Status = engagementStr(tz), engagementStr(status)
		if added.Valid {
			s := added.Time.UTC().Format(time.RFC3339)
			a.AddedDate = &s
		}
		a.TravelRequests = []domain.EngagementTravelRef{}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Allocations returns the engagement's allocations with the travel requests
// raised for each (travelapp.travel rows with the allocation id and the
// consultant's email; one query per pool, combined here).
func (r *engagementRepo) Allocations(ctx context.Context, id string, activeOnly bool) ([]domain.EngagementAllocation, error) {
	allocs, err := r.allocations(ctx, id, activeOnly)
	if err != nil || len(allocs) == 0 {
		return allocs, err
	}
	ids := make([]string, len(allocs))
	for i, a := range allocs {
		ids[i] = a.ID
	}
	rows, err := r.travel.QueryContext(ctx, `SELECT job_number, allocation_id, traveller_email, status
		FROM travel WHERE allocation_id IN (`+Placeholders(len(ids))+`) ORDER BY job_number`, Args(ids)...)
	if err != nil {
		return nil, fmt.Errorf("travel.ForAllocations: %w", err)
	}
	defer rows.Close()
	type key struct{ alloc, email string }
	refs := map[key][]domain.EngagementTravelRef{}
	for rows.Next() {
		var (
			job, email    string
			allocID, stat sql.NullString
		)
		if err := rows.Scan(&job, &allocID, &email, &stat); err != nil {
			return nil, fmt.Errorf("travel.ForAllocations scan: %w", err)
		}
		k := key{allocID.String, strings.ToLower(email)}
		refs[k] = append(refs[k], domain.EngagementTravelRef{JobNumber: job, Status: engagementStr(stat)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("travel.ForAllocations rows: %w", err)
	}
	for i := range allocs {
		if r, ok := refs[key{allocs[i].ID, strings.ToLower(allocs[i].Email)}]; ok {
			allocs[i].TravelRequests = r
		}
	}
	return allocs, nil
}
