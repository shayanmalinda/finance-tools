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
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// dashboardTravelCategory is the text an expense category name must contain
// for a card or claim without a job number to count as travel spend (the
// categories are "CC-Travel", "EXP-Lanka-Travel", "EXP-USnOther-Travel").
const dashboardTravelCategory = "%Travel%"

// DashboardTripRef is the trip a cost line belongs to (travelapp.travel),
// with its agent. JobNumber is nil when no trip matched.
type DashboardTripRef struct {
	JobNumber      *string
	TravellerEmail *string
	Country        *string
	AgentID        *int
	AgentName      *string
	AllocationID   *string
	CustomerName   *string
	EngagementType *string
}

// DashboardCostRow is one cost line as read from its source table. Trip is
// joined in SQL for invoices and per diem payments; the service joins it in
// Go for card and claim lines.
type DashboardCostRow struct {
	ID            string
	Source        string
	Reference     *string
	JobNumber     *string
	Date          string
	Currency      string
	Amount        float64
	CCCurrency    string
	CCAmount      float64
	CostType      string
	SourceType    *string
	EmployeeEmail *string // card and claim lines
	TravelCountry *string // card and claim lines
	Trip          DashboardTripRef
}

// DashboardCostEngagement is the engagement of an allocation.
type DashboardCostEngagement struct {
	EngagementID   string
	CustomerName   *string
	TypeName       *string
	AllocationType *string
}

// dashboardTripCountry trims the country of a trip.
const dashboardTripCountry = `REGEXP_REPLACE(t.country, '^[[:space:]]+|[[:space:]]+$', '')`

const dashboardTripCols = `t.job_number, t.traveller_email, ` + dashboardTripCountry + `, t.travel_agent_id,
			ta.name_of_travel_agent, t.allocation_id, t.customer_name, t.engagement_type`

// dashboardRangeWhere is the inclusive day range From..To: To is bound once more
// and the database adds a day, so DATE and DATETIME columns compare alike.
func dashboardRangeWhere(col string) string {
	return col + ` >= ? AND ` + col + ` < DATE_ADD(?, INTERVAL 1 DAY)`
}

// dashboardInClause adds ` AND col IN (...)` for a non-empty list.
func dashboardInClause(b *strings.Builder, args *[]any, col string, vals []string) {
	if len(vals) == 0 {
		return
	}
	b.WriteString(` AND ` + col + ` IN (` + Placeholders(len(vals)) + `)`)
	*args = append(*args, Args(vals)...)
}

func dashboardCostLimit(f domain.DashboardCostLinesFilter) int {
	n := f.Limit
	if n <= 0 || n > domain.DashboardCostLinesMax {
		n = domain.DashboardCostLinesMax
	}
	return n + 1 // one more than the cap, to detect truncation
}

// scanTripRef scans the dashboardTripCols into t.
func scanTripRef(sc interface{ Scan(...any) error }, t *DashboardTripRef, lead ...any) error {
	var (
		job, email, country, agentName, alloc, customer, engType sql.NullString
		agentID                                                  sql.NullInt64
	)
	dest := append(lead, &job, &email, &country, &agentID, &agentName, &alloc, &customer, &engType)
	if err := sc.Scan(dest...); err != nil {
		return err
	}
	*t = DashboardTripRef{
		JobNumber: nullStr(job), TravellerEmail: nullStr(email), Country: nullStr(country),
		AgentName: nullStr(agentName), AllocationID: nullStr(alloc), CustomerName: nullStr(customer),
		EngagementType: nullStr(engType),
	}
	if agentID.Valid {
		v := int(agentID.Int64)
		t.AgentID = &v
	}
	return nil
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

// InvoiceLines lists invoices (not rejected, not of cost type Currency) added
// in f's range, with the trip, agent and the original amount. Reads travelapp.
func (r *dashboardRepo) InvoiceLines(ctx context.Context, f domain.DashboardCostLinesFilter) ([]DashboardCostRow, error) {
	var b strings.Builder
	b.WriteString(`SELECT i.id, i.invoice_number, i.job_number, DATE_FORMAT(i.added_date, '%Y-%m-%d'),
			UPPER(TRIM(COALESCE(i.invoice_currency, ''))), i.invoice_amount, COALESCE(NULLIF(ct.name, ''), 'Other'),
			` + dashboardTripCols + `
		FROM invoice AS i
		LEFT JOIN inv_cost_type AS ct ON i.inv_cost_type_id = ct.id
		LEFT JOIN travel AS t ON i.job_number = t.job_number
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE ` + dashboardRangeWhere("i.added_date") + `
			AND COALESCE(i.status, '') NOT IN ('Rejected by travel', 'Rejected by finance')
			AND COALESCE(ct.name, '') != 'Currency'`)
	args := []any{f.From, f.To}
	dashboardInClause(&b, &args, "i.job_number", f.JobNumbers)
	dashboardInClause(&b, &args, "t.traveller_email", f.Employees)
	b.WriteString(` ORDER BY i.added_date, i.id LIMIT ?`)
	args = append(args, dashboardCostLimit(f))
	rows, err := r.travel.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard.InvoiceLines: %w", err)
	}
	defer rows.Close()
	out := []DashboardCostRow{}
	for rows.Next() {
		var (
			c        DashboardCostRow
			id       sql.NullInt64
			ref, job sql.NullString
			currency sql.NullString
			amount   sql.NullFloat64
			costType sql.NullString
			date     sql.NullString
		)
		if err := scanTripRef(rows, &c.Trip, &id, &ref, &job, &date, &currency, &amount, &costType); err != nil {
			return nil, fmt.Errorf("dashboard.InvoiceLines scan: %w", err)
		}
		c.ID, c.Source = fmt.Sprintf("INV-%d", id.Int64), domain.DashboardCostInvoice
		c.Reference, c.JobNumber, c.Date = nullStr(ref), trimmedStr(job), date.String
		c.Currency, c.Amount, c.CostType = currency.String, amount.Float64, costType.String
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.InvoiceLines rows: %w", err)
	}
	return out, nil
}

// PaymentLines lists per diem payments (every NetSuite status) added in f's
// range, with the trip, agent and the original amount. Reads travelapp.
func (r *dashboardRepo) PaymentLines(ctx context.Context, f domain.DashboardCostLinesFilter) ([]DashboardCostRow, error) {
	var b strings.Builder
	b.WriteString(`SELECT pp.id, pp.per_diem_number, pp.job_number, DATE_FORMAT(pp.added_date, '%Y-%m-%d'),
			UPPER(TRIM(COALESCE(pp.payment_currency, ''))), pp.payment_amount,
			` + dashboardTripCols + `
		FROM perdiem_payment AS pp
		LEFT JOIN travel AS t ON pp.job_number = t.job_number
		LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
		WHERE ` + dashboardRangeWhere("pp.added_date"))
	args := []any{f.From, f.To}
	dashboardInClause(&b, &args, "pp.job_number", f.JobNumbers)
	dashboardInClause(&b, &args, "t.traveller_email", f.Employees)
	b.WriteString(` ORDER BY pp.added_date, pp.id LIMIT ?`)
	args = append(args, dashboardCostLimit(f))
	rows, err := r.travel.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard.PaymentLines: %w", err)
	}
	defer rows.Close()
	out := []DashboardCostRow{}
	for rows.Next() {
		var (
			c        DashboardCostRow
			id       sql.NullInt64
			ref, job sql.NullString
			currency sql.NullString
			amount   sql.NullFloat64
			date     sql.NullString
		)
		if err := scanTripRef(rows, &c.Trip, &id, &ref, &job, &date, &currency, &amount); err != nil {
			return nil, fmt.Errorf("dashboard.PaymentLines scan: %w", err)
		}
		c.ID, c.Source = fmt.Sprintf("PD-%d", id.Int64), domain.DashboardCostPerDiem
		c.Reference, c.JobNumber, c.Date = nullStr(ref), trimmedStr(job), date.String
		c.Currency, c.Amount, c.CostType = currency.String, amount.Float64, "Per Diem"
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard.PaymentLines rows: %w", err)
	}
	return out, nil
}

// trimmedStr is nullStr with the value trimmed; blank is nil.
func trimmedStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := strings.TrimSpace(s.String)
	if v == "" {
		return nil
	}
	return &v
}

// expenseLines reads card or claim lines: rows tagged with a job number, or
// whose expense category is travel. Reads expense_app.
func (r *dashboardRepo) expenseLines(ctx context.Context, f domain.DashboardCostLinesFilter, source, what string) ([]DashboardCostRow, error) {
	if r.expense == nil {
		return nil, ErrDashboardNoExpenseDB
	}
	table, prefix, refCol, status, ccCols := "cc_txn", "CC-", "c.txn_reference_number", `COALESCE(c.status, '') != 'invalid'`,
		`CASE WHEN UPPER(TRIM(COALESCE(c.cc_currency, ''))) = 'USD' THEN 'USD' ELSE '' END, c.cc_amount`
	if source == domain.DashboardCostClaim {
		table, prefix, refCol, status, ccCols = "expense_claims", "EC-", "c.report_seq_number", `COALESCE(c.status, '') NOT IN ('-1', '1')`, `'', 0`
	}
	var b strings.Builder
	b.WriteString(`SELECT c.id, ` + refCol + `, TRIM(COALESCE(c.travel_job_number, '')), DATE_FORMAT(c.txn_date, '%Y-%m-%d'),
			UPPER(TRIM(COALESCE(c.txn_currency, ''))), c.txn_amount, ` + ccCols + `, c.employee_email,
			TRIM(COALESCE(c.travel_country, '')), et.expense_type
		FROM ` + table + ` AS c
		LEFT JOIN expense_type AS et ON et.id = c.expense_type_id
		LEFT JOIN expense_category AS ec ON ec.id = et.expense_category_id
		WHERE ` + dashboardRangeWhere("c.txn_date") + `
			AND ` + status + `
			AND ((c.travel_job_number IS NOT NULL AND TRIM(c.travel_job_number) != '') OR ec.expense_category LIKE ?)`)
	args := []any{f.From, f.To, dashboardTravelCategory}
	dashboardInClause(&b, &args, "TRIM(c.travel_job_number)", f.JobNumbers)
	dashboardInClause(&b, &args, "c.employee_email", f.Employees)
	b.WriteString(` ORDER BY c.txn_date, c.id LIMIT ?`)
	args = append(args, dashboardCostLimit(f))
	rows, err := r.expense.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()
	out := []DashboardCostRow{}
	for rows.Next() {
		var (
			c                          DashboardCostRow
			id                         sql.NullInt64
			ref, job, date, cur, ccCur sql.NullString
			email, country, typ        sql.NullString
			amount, ccAmount           sql.NullFloat64
		)
		if err := rows.Scan(&id, &ref, &job, &date, &cur, &amount, &ccCur, &ccAmount, &email, &country, &typ); err != nil {
			return nil, fmt.Errorf("%s scan: %w", what, err)
		}
		c.ID, c.Source = fmt.Sprintf("%s%d", prefix, id.Int64), source
		c.Reference, c.JobNumber, c.Date = nullStr(ref), trimmedStr(job), date.String
		c.Currency, c.Amount = cur.String, amount.Float64
		c.CCCurrency, c.CCAmount = ccCur.String, ccAmount.Float64
		c.CostType = "Card"
		if source == domain.DashboardCostClaim {
			c.CostType = "Claim"
		}
		c.SourceType, c.EmployeeEmail, c.TravelCountry = nullStr(typ), nullStr(email), trimmedStr(country)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", what, err)
	}
	return out, nil
}

// CardLines lists card transactions (expense_app.cc_txn), not invalid, with
// a job number or a travel expense category.
func (r *dashboardRepo) CardLines(ctx context.Context, f domain.DashboardCostLinesFilter) ([]DashboardCostRow, error) {
	return r.expenseLines(ctx, f, domain.DashboardCostCard, "dashboard.CardLines")
}

// ClaimLines lists expense claims (expense_app.expense_claims), except
// deleted (-1) and rejected (1), with a job number or a travel expense
// category.
func (r *dashboardRepo) ClaimLines(ctx context.Context, f domain.DashboardCostLinesFilter) ([]DashboardCostRow, error) {
	return r.expenseLines(ctx, f, domain.DashboardCostClaim, "dashboard.ClaimLines")
}

// TripsByJob returns the trips with the given (upper-cased) job numbers,
// keyed by upper-cased job number. Reads travelapp, 500 job numbers a query.
func (r *dashboardRepo) TripsByJob(ctx context.Context, jobNumbers []string) (map[string]DashboardTripRef, error) {
	out := map[string]DashboardTripRef{}
	for start := 0; start < len(jobNumbers); start += travelInChunk {
		end := min(start+travelInChunk, len(jobNumbers))
		chunk := jobNumbers[start:end]
		rows, err := r.travel.QueryContext(ctx, `SELECT `+dashboardTripCols+`
			FROM travel AS t
			LEFT JOIN travel_agents AS ta ON t.travel_agent_id = ta.id
			WHERE t.job_number IN (`+Placeholders(len(chunk))+`)`, Args(chunk)...)
		if err != nil {
			return nil, fmt.Errorf("dashboard.TripsByJob: %w", err)
		}
		for rows.Next() {
			var t DashboardTripRef
			if err := scanTripRef(rows, &t); err != nil {
				rows.Close()
				return nil, fmt.Errorf("dashboard.TripsByJob scan: %w", err)
			}
			if t.JobNumber != nil {
				out[strings.ToUpper(strings.TrimSpace(*t.JobNumber))] = t
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("dashboard.TripsByJob rows: %w", err)
		}
	}
	return out, nil
}

// CostEngagements returns, per allocation id, the engagement id, its
// customer, engagement type name and allocation type name. Reads
// allocation_db, 500 ids a query.
func (r *dashboardRepo) CostEngagements(ctx context.Context, allocationIDs []string) (map[string]DashboardCostEngagement, error) {
	out := map[string]DashboardCostEngagement{}
	for start := 0; start < len(allocationIDs); start += travelInChunk {
		end := min(start+travelInChunk, len(allocationIDs))
		chunk := allocationIDs[start:end]
		rows, err := r.alloc.QueryContext(ctx, `SELECT a.id, e.id, e.customer_name, eT.name, aT.allocation_type
			FROM allocation AS a
			JOIN engagement AS e ON a.engagement_id = e.id
			JOIN allocation_type AS aT ON e.allocation_type = aT.id
			JOIN engagement_type AS eT ON aT.engagement_type = eT.id
			WHERE e.status != 'Deleted' AND a.id IN (`+Placeholders(len(chunk))+`)`, Args(chunk)...)
		if err != nil {
			return nil, fmt.Errorf("dashboard.CostEngagements: %w", err)
		}
		for rows.Next() {
			var (
				aid, eid                 string
				customer, typeName, aTyp sql.NullString
			)
			if err := rows.Scan(&aid, &eid, &customer, &typeName, &aTyp); err != nil {
				rows.Close()
				return nil, fmt.Errorf("dashboard.CostEngagements scan: %w", err)
			}
			out[aid] = DashboardCostEngagement{EngagementID: eid, CustomerName: nullStr(customer), TypeName: nullStr(typeName), AllocationType: nullStr(aTyp)}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("dashboard.CostEngagements rows: %w", err)
		}
	}
	return out, nil
}
