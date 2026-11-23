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
// Currency, added in the 12-month window, by month, cost type and currency.
// The amount is the USD one when either side is USD (invoice_currency, or
// converted_currency, which the Travel Team fills in, USD by default);
// otherwise the original amount in its currency.
func (r *dashboardRepo) InvoiceCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	q := `SELECT DATE_FORMAT(i.added_date, '%Y-%m') AS d_month, COALESCE(ct.name, '') AS d_cost_type,
			CASE WHEN i.invoice_currency = 'USD' OR i.converted_currency = 'USD' THEN 'USD' ELSE i.invoice_currency END AS d_currency,
			SUM(CASE WHEN i.invoice_currency = 'USD' THEN i.invoice_amount
				WHEN i.converted_currency = 'USD' THEN i.converted_amount
				ELSE i.invoice_amount END) AS amount
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
// 12-month window, by month and currency: USD when the payment or its
// subsidiary conversion is in USD, else the payment currency.
func (r *dashboardRepo) PaymentCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	q := `SELECT DATE_FORMAT(pp.added_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			CASE WHEN pp.payment_currency = 'USD' OR pp.converted_currency = 'USD' THEN 'USD' ELSE COALESCE(pp.payment_currency, '') END AS d_currency,
			SUM(CASE WHEN pp.payment_currency = 'USD' THEN pp.payment_amount
				WHEN pp.converted_currency = 'USD' THEN pp.converted_amount
				ELSE pp.payment_amount END) AS amount
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

// AgencyPerDiems lists the active per diem rows on the agency's open (In
// Progress or Extended) trips. perdiem_allocation.status holds 'Active' /
// 'Deleted'; the comparison is case-insensitive as in legacy.
func (r *dashboardRepo) AgencyPerDiems(ctx context.Context, agencyID, limit int) ([]domain.DashboardAgencyPerDiem, error) {
	q := `SELECT pa.job_number, t.traveller_email,
			DATE_FORMAT(pa.start_date, '%Y-%m-%d'), DATE_FORMAT(pa.end_date, '%Y-%m-%d'),
			pa.no_of_days, pa.currency, pa.total
		FROM perdiem_allocation AS pa
		INNER JOIN travel AS t ON pa.job_number = t.job_number
		WHERE t.travel_agent_id = ? AND t.status IN ('In Progress', 'Extended') AND pa.status = 'Active'
		ORDER BY t.start_date, pa.start_date, pa.id
		LIMIT ?`
	rows, err := r.travel.QueryContext(ctx, q, agencyID, limit)
	if err != nil {
		return nil, fmt.Errorf("dashboard.AgencyPerDiems: %w", err)
	}
	defer rows.Close()
	out := []domain.DashboardAgencyPerDiem{}
	for rows.Next() {
		var p domain.DashboardAgencyPerDiem
		var days sql.NullInt64
		var total sql.NullFloat64
		if err := rows.Scan(&p.JobNumber, &p.TravellerEmail, &p.StartDate, &p.EndDate, &days, &p.Currency, &total); err != nil {
			return nil, fmt.Errorf("dashboard.AgencyPerDiems scan: %w", err)
		}
		if days.Valid {
			v := int(days.Int64)
			p.Days = &v
		}
		if total.Valid {
			v := total.Float64
			p.Total = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ── expense_app ─────────────────────────────────────────────────────────────

// CardCost sums the card transactions (expense_app.cc_txn) tagged with a
// travel job number, except invalid ones, by transaction month and currency:
// USD when the transaction or the card is in USD, else the transaction
// currency. The tag is free text; it is not checked against travelapp.
func (r *dashboardRepo) CardCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	if r.expense == nil {
		return nil, ErrDashboardNoExpenseDB
	}
	q := `SELECT DATE_FORMAT(c.txn_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			CASE WHEN c.txn_currency = 'USD' OR c.cc_currency = 'USD' THEN 'USD' ELSE c.txn_currency END AS d_currency,
			SUM(CASE WHEN c.txn_currency = 'USD' THEN c.txn_amount
				WHEN c.cc_currency = 'USD' THEN c.cc_amount
				ELSE c.txn_amount END) AS amount
		FROM cc_txn AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND COALESCE(c.status, '') != 'invalid'
		GROUP BY d_month, d_currency`
	rows, err := r.expense.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.CardCost: %w", err)
	}
	return dashboardCostRows(rows, domain.DashboardCostCard, "dashboard.CardCost")
}

// ClaimCost sums the expense claims (expense_app.expense_claims) tagged with a
// travel job number, except deleted (-1) and rejected (1) ones, by
// transaction month and currency, the same way as CardCost: USD when the
// transaction or the reimbursement is in USD.
func (r *dashboardRepo) ClaimCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error) {
	if r.expense == nil {
		return nil, ErrDashboardNoExpenseDB
	}
	q := `SELECT DATE_FORMAT(c.txn_date, '%Y-%m') AS d_month, '' AS d_cost_type,
			CASE WHEN c.txn_currency = 'USD' OR c.reimbursement_currency = 'USD' THEN 'USD' ELSE c.txn_currency END AS d_currency,
			SUM(CASE WHEN c.txn_currency = 'USD' THEN c.txn_amount
				WHEN c.reimbursement_currency = 'USD' THEN c.reimbursement_amount
				ELSE c.txn_amount END) AS amount
		FROM expense_claims AS c
		WHERE c.txn_date >= ? AND c.txn_date < ?
			AND c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != ''
			AND c.status NOT IN ('-1', '1')
		GROUP BY d_month, d_currency`
	rows, err := r.expense.QueryContext(ctx, q, w.YearStart, w.NextMonth)
	if err != nil {
		return nil, fmt.Errorf("dashboard.ClaimCost: %w", err)
	}
	return dashboardCostRows(rows, domain.DashboardCostClaim, "dashboard.ClaimCost")
}
