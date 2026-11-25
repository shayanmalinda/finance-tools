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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// dashboardNotCancelled keeps every trip that is not Cancelled (status may be
// NULL on old rows).
const dashboardNotCancelled = `COALESCE(t.status, '') != 'Cancelled'`

// UpcomingTrips lists trips that are not Cancelled and end today or later,
// by start date, for a traveller, an agency or everyone, with the total.
func (r *dashboardRepo) UpcomingTrips(ctx context.Context, f DashboardTripFilter) ([]domain.DashboardTrip, int, error) {
	q := `SELECT t.job_number, t.status, t.traveller_email, REPLACE(t.city, 'BR()', CHAR(10)), t.country,
			DATE_FORMAT(t.start_date, '%Y-%m-%d'), DATE_FORMAT(t.end_date, '%Y-%m-%d'),
			ta.name_of_travel_agent, COUNT(*) OVER () AS total
		FROM travel AS t
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE ` + dashboardNotCancelled + ` AND t.end_date >= ?`
	args := []any{f.Today}
	if f.Email != "" {
		q += ` AND t.traveller_email = ?`
		args = append(args, f.Email)
	}
	if f.AgencyID > 0 {
		q += ` AND t.travel_agent_id = ?`
		args = append(args, f.AgencyID)
	}
	q += ` ORDER BY t.start_date, t.job_number LIMIT ?`
	args = append(args, f.Limit)
	rows, err := r.travel.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("dashboard.UpcomingTrips: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardTrip{}
	total := 0
	for rows.Next() {
		var t domain.DashboardTrip
		if err := rows.Scan(&t.JobNumber, &t.Status, &t.TravellerEmail, &t.City, &t.Country,
			&t.StartDate, &t.EndDate, &t.Agency, &total); err != nil {
			return nil, 0, fmt.Errorf("dashboard.UpcomingTrips scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("dashboard.UpcomingTrips rows: %w", err)
	}
	return out, total, nil
}

// NewTravelRequestCount counts travel requests with status New (the Travel
// Requests list, segment new).
func (r *dashboardRepo) NewTravelRequestCount(ctx context.Context) (int, error) {
	var n int
	if err := r.travel.QueryRowContext(ctx, `SELECT COUNT(*) FROM travel WHERE status = 'New'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("dashboard.NewTravelRequestCount: %w", err)
	}
	return n, nil
}

// InvoiceQueueCounts counts invoices per work queue: Pending (PO approval),
// Accepted by travel (finance approval), Accepted by finance not failed (to
// push) and failed pushes (NS Sync Failures).
func (r *dashboardRepo) InvoiceQueueCounts(ctx context.Context) (DashboardInvoiceQueues, error) {
	q := `SELECT
			COALESCE(SUM(CASE WHEN i.status = 'Pending' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.status = 'Accepted by travel' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.status = 'Accepted by finance' AND NOT ` + dashboardInvoiceFailed + ` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN i.status = 'Accepted by finance' AND ` + dashboardInvoiceFailed + ` THEN 1 ELSE 0 END), 0)
		FROM invoice AS i
		WHERE i.status IN ('Pending', 'Accepted by travel', 'Accepted by finance')`
	var c DashboardInvoiceQueues
	if err := r.travel.QueryRowContext(ctx, q).Scan(&c.Pending, &c.AcceptedByTravel, &c.ToPush, &c.Failed); err != nil {
		return c, fmt.Errorf("dashboard.InvoiceQueueCounts: %w", err)
	}
	return c, nil
}

// PaymentQueueCounts counts Pending per diem payments (to push) and failed
// pushes (ns_status 'Failed').
func (r *dashboardRepo) PaymentQueueCounts(ctx context.Context) (toPush, failed int, err error) {
	q := `SELECT
			COALESCE(SUM(CASE WHEN NOT ` + dashboardPaymentFailed + ` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardPaymentFailed + ` THEN 1 ELSE 0 END), 0)
		FROM perdiem_payment AS pp
		WHERE pp.ns_status IN ('Pending', 'Failed')`
	if err := r.travel.QueryRowContext(ctx, q).Scan(&toPush, &failed); err != nil {
		return 0, 0, fmt.Errorf("dashboard.PaymentQueueCounts: %w", err)
	}
	return toPush, failed, nil
}

// TravelKPIs computes the Travel tiles in one pass over travel.
func (r *dashboardRepo) TravelKPIs(ctx context.Context, w DashboardWindow) (DashboardTravelKPIs, error) {
	q := `SELECT
			COALESCE(SUM(CASE WHEN t.status IN ('In Progress', 'Extended') AND t.start_date < ? AND t.end_date >= ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.status = 'Completed' AND t.end_date >= ? AND t.end_date < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.end_date >= ? THEN 1 ELSE 0 END), 0)
		FROM travel AS t`
	var k DashboardTravelKPIs
	err := r.travel.QueryRowContext(ctx, q,
		w.Tomorrow, w.Today, // in progress today
		w.Tomorrow, w.In30Days, // today < start <= today + 30
		w.MonthStart, w.NextMonth, // completed, ending this month
		w.YearStart, w.Tomorrow, // started in the 12-month window, up to today
		w.Today, // upcoming
	).Scan(&k.InProgress, &k.StartingIn30Days, &k.CompletedThisMonth, &k.Last12Months, &k.Upcoming)
	if err != nil {
		return k, fmt.Errorf("dashboard.TravelKPIs: %w", err)
	}
	return k, nil
}

// TravelCells counts trips not Cancelled that started in the 12-month window
// (up to today) by start month, country and agency.
func (r *dashboardRepo) TravelCells(ctx context.Context, w DashboardWindow) ([]DashboardTravelCell, error) {
	q := `SELECT DATE_FORMAT(t.start_date, '%Y-%m') AS d_month, REGEXP_REPLACE(t.country, '^[[:space:]]+|[[:space:]]+$', '') AS d_country, ta.name_of_travel_agent, COUNT(*)
		FROM travel AS t
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ?
		GROUP BY d_month, d_country, ta.name_of_travel_agent`
	rows, err := r.travel.QueryContext(ctx, q, w.YearStart, w.Tomorrow)
	if err != nil {
		return nil, fmt.Errorf("dashboard.TravelCells: %w", err)
	}
	defer rows.Close()
	out := []DashboardTravelCell{}
	for rows.Next() {
		var c DashboardTravelCell
		var country sql.NullString
		if err := rows.Scan(&c.Month, &country, &c.Agency, &c.Count); err != nil {
			return nil, fmt.Errorf("dashboard.TravelCells scan: %w", err)
		}
		c.Country = country.String
		out = append(out, c)
	}
	return out, rows.Err()
}

// InvoiceCost sums invoices that are not rejected and not of cost type
// Currency, added in the 12-month window, by month, cost type and original
// currency (invoice_currency). The amount is the original invoice_amount: the
// stored converted_* columns are not read, the BFF converts at NetSuite's
// month-end rate.
func (r *dashboardRepo) InvoiceCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	q := `SELECT DATE_FORMAT(i.added_date, '%Y-%m') AS d_month, COALESCE(ct.name, '') AS d_cost_type,
			UPPER(TRIM(COALESCE(i.invoice_currency, ''))) AS d_currency,
			SUM(i.invoice_amount) AS amount
		FROM invoice AS i
		LEFT JOIN inv_cost_type AS ct ON i.inv_cost_type_id = ct.id
		WHERE i.added_date >= ? AND i.added_date < ?
			AND COALESCE(i.status, '') NOT IN ('Rejected by travel', 'Rejected by finance')
			AND COALESCE(ct.name, '') != 'Currency'
		GROUP BY d_month, d_cost_type, d_currency`
	rows, err := r.travel.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.InvoiceCost: %w", err)
	}
	return dashboardCostRows(rows, domain.DashboardCostInvoice, "dashboard.InvoiceCost")
}

// PaymentCost sums per diem payments (every NetSuite status) added in the
// 12-month window, by month and original currency (payment_currency), with
// the original payment_amount. The stored converted_* columns are not read.
func (r *dashboardRepo) PaymentCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	q := `SELECT DATE_FORMAT(pp.added_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			UPPER(TRIM(COALESCE(pp.payment_currency, ''))) AS d_currency,
			SUM(pp.payment_amount) AS amount
		FROM perdiem_payment AS pp
		WHERE pp.added_date >= ? AND pp.added_date < ?
		GROUP BY d_month, d_currency`
	rows, err := r.travel.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.PaymentCost: %w", err)
	}
	return dashboardCostRows(rows, domain.DashboardCostPerDiem, "dashboard.PaymentCost")
}

// ── Travel agent dashboard ──────────────────────────────────────────────────

// dashboardAgencyInvoice joins an invoice to the agency of the person who
// added it, as the legacy invoice queries do (invoice.added_by =
// travel_agent_employee.wso2_email).
const dashboardAgencyInvoice = `FROM invoice AS i
		INNER JOIN travel_agent_employee AS tae ON i.added_by = tae.wso2_email
		WHERE tae.travel_agent_id = ?`

// AgencyKPIs computes the agent's tiles over the agency's trips.
func (r *dashboardRepo) AgencyKPIs(ctx context.Context, agencyID int, w DashboardWindow) (DashboardAgencyKPIs, error) {
	q := `SELECT
			COALESCE(SUM(CASE WHEN t.status IN ('In Progress', 'Extended', 'On Hold') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.status = 'Completed' AND NOT EXISTS (
				SELECT 1 ` + dashboardAgencyInvoice + ` AND i.job_number = t.job_number) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN ` + dashboardNotCancelled + ` AND t.end_date >= ? THEN 1 ELSE 0 END), 0)
		FROM travel AS t
		WHERE t.travel_agent_id = ?`
	var k DashboardAgencyKPIs
	err := r.travel.QueryRowContext(ctx, q,
		w.Today, w.In7Days, // today <= start <= today + 7
		w.Today, w.In30Days, // today <= start <= today + 30
		agencyID, // invoices of this agency
		w.Today,  // upcoming
		agencyID,
	).Scan(&k.InProgress, &k.StartingIn7Days, &k.StartingIn30Days, &k.CompletedWithoutInvoice, &k.Upcoming)
	if err != nil {
		return k, fmt.Errorf("dashboard.AgencyKPIs: %w", err)
	}
	return k, nil
}

// AgencyInvoicesByStatus counts the agency's invoices by legacy status.
func (r *dashboardRepo) AgencyInvoicesByStatus(ctx context.Context, agencyID int) ([]domain.DashboardCount, error) {
	q := `SELECT CASE COALESCE(i.status, 'Pending')
				WHEN 'Completed' THEN 'Complete'
				WHEN 'Approve by finance' THEN 'Accepted by finance'
				ELSE COALESCE(i.status, 'Pending') END AS d_status, COUNT(*) ` + dashboardAgencyInvoice + `
		GROUP BY d_status
		ORDER BY COUNT(*) DESC, d_status`
	rows, err := r.travel.QueryContext(ctx, q, agencyID)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AgencyInvoicesByStatus: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.AgencyInvoicesByStatus")
}

// AgencyTripsByMonth counts the agency's trips not Cancelled that started in
// the 12-month window (up to today), by start month.
func (r *dashboardRepo) AgencyTripsByMonth(ctx context.Context, agencyID int, w DashboardWindow) ([]domain.DashboardCount, error) {
	q := `SELECT DATE_FORMAT(t.start_date, '%Y-%m') AS d_month, COUNT(*)
		FROM travel AS t
		WHERE t.travel_agent_id = ? AND ` + dashboardNotCancelled + ` AND t.start_date >= ? AND t.start_date < ?
		GROUP BY d_month
		ORDER BY d_month`
	rows, err := r.travel.QueryContext(ctx, q, agencyID, w.YearStart, w.Tomorrow)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AgencyTripsByMonth: %w", err)
	}
	return dashboardCountRows(rows, "dashboard.AgencyTripsByMonth")
}

// ── expense_app ─────────────────────────────────────────────────────────────

// CardCost sums the card transactions (expense_app.cc_txn) tagged with a
// travel job number, except invalid ones, by transaction month, original
// currency (txn_currency) and whether the card was billed in USD. Amount is
// the sum of txn_amount; CCAmount is the sum of cc_amount and CCCurrency is
// "USD" for the groups billed in USD (a USD card row with no billed amount
// counts as not billed in USD), blank otherwise. The tag is free text;
// it is not checked against travelapp.
func (r *dashboardRepo) CardCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	if r.expense == nil {
		return nil, ErrDashboardNoExpenseDB
	}
	q := `SELECT DATE_FORMAT(c.txn_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			UPPER(TRIM(COALESCE(c.txn_currency, ''))) AS d_currency,
			CASE WHEN UPPER(TRIM(COALESCE(c.cc_currency, ''))) = 'USD' AND c.cc_amount IS NOT NULL THEN 'USD' ELSE '' END AS d_cc_currency,
			SUM(c.txn_amount) AS amount, SUM(c.cc_amount) AS cc_amount
		FROM cc_txn AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND COALESCE(c.status, '') != 'invalid'
		GROUP BY d_month, d_currency, d_cc_currency`
	rows, err := r.expense.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.CardCost: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardCostLine{}
	for rows.Next() {
		var month, costType, currency, ccCurrency sql.NullString
		var amount, ccAmount sql.NullFloat64
		if err := rows.Scan(&month, &costType, &currency, &ccCurrency, &amount, &ccAmount); err != nil {
			return nil, fmt.Errorf("dashboard.CardCost scan: %w", err)
		}
		out = append(out, domain.DashboardCostLine{
			Source: domain.DashboardCostCard, Month: month.String, CostType: costType.String,
			Currency: currency.String, Amount: amount.Float64,
			CCCurrency: ccCurrency.String, CCAmount: ccAmount.Float64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.CardCost rows: %w", err)
	}
	return out, nil
}

// ClaimCost sums the expense claims (expense_app.expense_claims) tagged with a
// travel job number, except deleted (-1) and rejected (1) ones (so claims
// not yet approved count), by transaction month and original currency
// (txn_currency), with the original txn_amount. The reimbursement_* columns
// are not read.
func (r *dashboardRepo) ClaimCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	if r.expense == nil {
		return nil, ErrDashboardNoExpenseDB
	}
	q := `SELECT DATE_FORMAT(c.txn_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			UPPER(TRIM(COALESCE(c.txn_currency, ''))) AS d_currency,
			SUM(c.txn_amount) AS amount
		FROM expense_claims AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND COALESCE(c.status, '') NOT IN ('-1', '1')
		GROUP BY d_month, d_currency`
	rows, err := r.expense.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.ClaimCost: %w", err)
	}
	return dashboardCostRows(rows, domain.DashboardCostClaim, "dashboard.ClaimCost")
}
