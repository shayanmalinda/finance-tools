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

// FinanceAllocationRepository reads what the travel finance screens need
// from allocation_db: a trip's engagement (through its allocation), its
// funding sources, and the Marketing Event campaigns.
type FinanceAllocationRepository interface {
	// EngagementOfAllocation is the engagement of an allocation, or nil.
	EngagementOfAllocation(ctx context.Context, allocationID string) (*domain.FinanceEngagement, error)
	// Campaigns maps each allocation id whose engagement has a campaign to
	// that campaign (id, name = engagement.project_name).
	Campaigns(ctx context.Context, allocationIDs []string) (map[string]domain.FinanceCampaign, error)
	// AllocationsOfCampaigns lists the allocations of the engagements linked
	// to any of campaignIDs.
	AllocationsOfCampaigns(ctx context.Context, campaignIDs []string) ([]string, error)
	FundingSources(ctx context.Context, engagementID string) ([]domain.FinanceFundingSource, error)
}

type financeAllocRepo struct{ db *sql.DB }

// NewFinanceAllocationRepository constructs a FinanceAllocationRepository.
// db must be the allocation_db pool.
func NewFinanceAllocationRepository(db *sql.DB) FinanceAllocationRepository {
	return &financeAllocRepo{db: db}
}

// EngagementOfAllocation ports the legacy engagementAllocations(filter:
// {allocationId}) → engagements(filter:{id}) pair as one statement.
func (r *financeAllocRepo) EngagementOfAllocation(ctx context.Context, allocationID string) (*domain.FinanceEngagement, error) {
	var (
		e                           domain.FinanceEngagement
		code, customer, name, campn sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `SELECT e.id, e.engagement_code, e.customer_name, e.project_name, `+campaignCol("e")+`
		FROM allocation a
		INNER JOIN engagement e ON e.id = a.engagement_id
		WHERE a.id = ?
		LIMIT 1`, allocationID).Scan(&e.ID, &code, &customer, &name, &campn)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.EngagementOfAllocation: %w", err)
	}
	e.EngagementCode = code.String
	e.CustomerName, e.ProjectName = financeStr(customer), financeStr(name)
	if campn.Valid && campn.String != "" {
		e.CampaignID = &campn.String
	}
	return &e, nil
}

// Campaigns implements FinanceAllocationRepository. An empty list reads
// nothing.
func (r *financeAllocRepo) Campaigns(ctx context.Context, allocationIDs []string) (map[string]domain.FinanceCampaign, error) {
	out := map[string]domain.FinanceCampaign{}
	if len(allocationIDs) == 0 || !HasEngagementCampaignColumn() {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id, e.campaign_id, e.project_name
		FROM allocation a
		INNER JOIN engagement e ON e.id = a.engagement_id
		WHERE a.id IN (`+Placeholders(len(allocationIDs))+`) AND e.campaign_id IS NOT NULL AND e.campaign_id <> ''`,
		Args(allocationIDs)...)
	if err != nil {
		return nil, fmt.Errorf("finance.Campaigns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, campaign string
		var name sql.NullString
		if err := rows.Scan(&id, &campaign, &name); err != nil {
			return nil, fmt.Errorf("finance.Campaigns scan: %w", err)
		}
		out[id] = domain.FinanceCampaign{ID: campaign, Name: financeStr(name)}
	}
	return out, rows.Err()
}

// AllocationsOfCampaigns implements FinanceAllocationRepository. An empty
// list matches nothing.
func (r *financeAllocRepo) AllocationsOfCampaigns(ctx context.Context, campaignIDs []string) ([]string, error) {
	out := []string{}
	if len(campaignIDs) == 0 || !HasEngagementCampaignColumn() {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id
		FROM allocation a
		INNER JOIN engagement e ON e.id = a.engagement_id
		WHERE e.campaign_id IN (`+Placeholders(len(campaignIDs))+`)`, Args(campaignIDs)...)
	if err != nil {
		return nil, fmt.Errorf("finance.AllocationsOfCampaigns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("finance.AllocationsOfCampaigns scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FundingSources ports the legacy getFundingSources for one engagement.
func (r *financeAllocRepo) FundingSources(ctx context.Context, engagementID string) ([]domain.FinanceFundingSource, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
			business_unit.name AS businessUnit,
			product_unit.name AS productUnit,
			sub_region.name AS subRegion,
			region.name AS region,
			eng_fs.percentage
		FROM engagement_funding_source AS eng_fs
			LEFT JOIN business_unit ON eng_fs.business_unit_id = business_unit.id
			LEFT JOIN product_unit ON eng_fs.product_unit_id = product_unit.id
			LEFT JOIN region ON eng_fs.region_id = region.id
			LEFT JOIN sub_region ON eng_fs.sub_region_id = sub_region.id
		WHERE eng_fs.engagement_id IN (?)`, engagementID)
	if err != nil {
		return nil, fmt.Errorf("finance.FundingSources: %w", err)
	}
	defer rows.Close()
	out := []domain.FinanceFundingSource{}
	for rows.Next() {
		var bu, pu, sub, region, pct sql.NullString
		if err := rows.Scan(&bu, &pu, &sub, &region, &pct); err != nil {
			return nil, fmt.Errorf("finance.FundingSources scan: %w", err)
		}
		out = append(out, domain.FinanceFundingSource{
			BusinessUnit: financeStr(bu), ProductUnit: financeStr(pu), SubRegion: financeStr(sub),
			Region: financeStr(region), Percentage: pct.String,
		})
	}
	return out, rows.Err()
}

// FinanceExpenseRepository reads expense_app (read-only).
type FinanceExpenseRepository interface {
	// Subsidiary is the active subsidiary whose code is the HR company
	// name, or nil.
	Subsidiary(ctx context.Context, company string) (*domain.FinanceSubsidiary, error)
	// TripExpenses is the card spend and expense claims tagged with
	// jobNumber, except invalid card spend and deleted claims.
	TripExpenses(ctx context.Context, jobNumber string) (domain.FinanceTripExpenses, error)
}

type financeExpenseRepo struct{ db *sql.DB }

// NewFinanceExpenseRepository constructs a FinanceExpenseRepository. db must
// be the expense_app pool.
func NewFinanceExpenseRepository(db *sql.DB) FinanceExpenseRepository {
	return &financeExpenseRepo{db: db}
}

// Subsidiary ports getSubsidiaries (claimDbClient).
func (r *financeExpenseRepo) Subsidiary(ctx context.Context, company string) (*domain.FinanceSubsidiary, error) {
	var (
		s                         domain.FinanceSubsidiary
		legal, cur, country, code sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `SELECT id, legal_name AS legalName, currency_code AS currencyCode, country,
			tax_code_internal_id AS taxCodeInternalID
		FROM subsidiaries
		WHERE status = 'active' AND code = ?
		LIMIT 1`, company).Scan(&s.ID, &legal, &cur, &country, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.Subsidiary: %w", err)
	}
	s.LegalName, s.CurrencyCode, s.Country, s.TaxCodeInternalID = legal.String, cur.String, country.String, code.String
	return &s, nil
}

// TripExpenses implements FinanceExpenseRepository. The job number is matched
// exactly, as the expense apps store the one picked from the travel list.
// status is nullable in both tables, so a NULL status is kept.
func (r *financeExpenseRepo) TripExpenses(ctx context.Context, jobNumber string) (domain.FinanceTripExpenses, error) {
	out := domain.FinanceTripExpenses{CardExpenses: []domain.FinanceCardExpense{}, ExpenseClaims: []domain.FinanceExpenseClaim{}}
	rows, err := r.db.QueryContext(ctx, `SELECT t.id, DATE_FORMAT(t.txn_date, '%Y-%m-%d'), et.expense_type, t.txn_reference,
			t.txn_amount, t.txn_currency, t.employee_email, t.status
		FROM cc_txn AS t
			LEFT JOIN expense_type AS et ON et.id = t.expense_type_id
		WHERE t.travel_job_number = ? AND COALESCE(t.status, '') != 'invalid'
		ORDER BY t.txn_date, t.id LIMIT ?`, jobNumber, financeMaxRows)
	if err != nil {
		return out, fmt.Errorf("finance.TripExpenses cards: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.FinanceCardExpense
		var date, typ, ref, cur, email, status sql.NullString
		var amount sql.NullFloat64
		if err := rows.Scan(&c.ID, &date, &typ, &ref, &amount, &cur, &email, &status); err != nil {
			return out, fmt.Errorf("finance.TripExpenses cards scan: %w", err)
		}
		c.Date, c.ExpenseType, c.Reference, c.Currency = financeStr(date), financeStr(typ), financeStr(ref), financeStr(cur)
		c.EmployeeEmail, c.Status, c.Amount = financeStr(email), financeStr(status), financeFloat(amount)
		out.CardExpenses = append(out.CardExpenses, c)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("finance.TripExpenses cards: %w", err)
	}
	claims, err := r.db.QueryContext(ctx, `SELECT c.id, c.report_seq_number, DATE_FORMAT(c.txn_date, '%Y-%m-%d'), et.expense_type,
			c.txn_amount, c.txn_currency, c.reimbursement_amount, c.reimbursement_currency, c.employee_email, c.status
		FROM expense_claims AS c
			LEFT JOIN expense_type AS et ON et.id = c.expense_type_id
		WHERE c.travel_job_number = ? AND COALESCE(c.status, '') != '-1'
		ORDER BY c.txn_date, c.id LIMIT ?`, jobNumber, financeMaxRows)
	if err != nil {
		return out, fmt.Errorf("finance.TripExpenses claims: %w", err)
	}
	defer claims.Close()
	for claims.Next() {
		var c domain.FinanceExpenseClaim
		var num, date, typ, cur, rcur, email, status sql.NullString
		var amount, ramount sql.NullFloat64
		if err := claims.Scan(&c.ID, &num, &date, &typ, &amount, &cur, &ramount, &rcur, &email, &status); err != nil {
			return out, fmt.Errorf("finance.TripExpenses claims scan: %w", err)
		}
		c.ClaimNumber, c.Date, c.ExpenseType, c.Currency = financeStr(num), financeStr(date), financeStr(typ), financeStr(cur)
		c.ReimbursementCurrency, c.EmployeeEmail, c.Status = financeStr(rcur), financeStr(email), financeStr(status)
		c.Amount, c.ReimbursementAmount = financeFloat(amount), financeFloat(ramount)
		out.ExpenseClaims = append(out.ExpenseClaims, c)
	}
	return out, claims.Err()
}
