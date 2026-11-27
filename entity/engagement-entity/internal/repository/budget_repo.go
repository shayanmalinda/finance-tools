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
	"log"
	"math"
	"sort"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ErrBudgetNotFound is returned when a budget line or trip does not exist.
var ErrBudgetNotFound = errors.New("budget record not found")

// budgetLedgerTripMax caps the trips of one ledger call.
const budgetLedgerTripMax = domain.DashboardCostLinesMax

// BudgetRepository reads and writes the travel budget tables of travelapp,
// and reads trip costs of travelapp and expense_app. Every write records its
// audit_event rows.
type BudgetRepository interface {
	Years(ctx context.Context) ([]domain.BudgetYear, error)
	// Lines lists the lines of year (0: every year), ordered by sort_order, id.
	Lines(ctx context.Context, year int, includeRetired bool) ([]domain.BudgetLine, error)
	Line(ctx context.Context, id int) (domain.BudgetLine, error)
	LineCodeExists(ctx context.Context, code string) (bool, error)
	CreateLine(ctx context.Context, in domain.BudgetLineCreateRequest) (int, error)
	UpdateLine(ctx context.Context, id int, in domain.BudgetLineUpdateRequest) error
	RetireLine(ctx context.Context, id int, actor string) error

	AllocationTypeRules(ctx context.Context) ([]domain.BudgetAllocationTypeRule, error)
	SaveAllocationTypeRule(ctx context.Context, id int, in domain.BudgetAllocationTypeRuleRequest) error
	RegionRules(ctx context.Context) ([]domain.BudgetRegionRule, error)
	TeamRules(ctx context.Context) ([]domain.BudgetTeamRule, error)

	EngagementLine(ctx context.Context, engagementID string) (*string, error)
	// SetEngagementLine upserts the engagement's line; nil deletes it.
	SetEngagementLine(ctx context.Context, engagementID string, code *string, actor string) error

	// Trip returns ErrBudgetNotFound when the trip does not exist.
	Trip(ctx context.Context, jobNumber string) (domain.BudgetTrip, error)
	// SetTripLine upserts the trip's row with chosen_by USER. year is the
	// trip's budget year (nil when none holds its start date).
	SetTripLine(ctx context.Context, jobNumber string, year, lineID *int, actor string) error
	NeedsLine(ctx context.Context, year, limit, offset int) ([]domain.BudgetTrip, int, error)
	Ledger(ctx context.Context, f domain.BudgetLedgerFilter) (domain.BudgetLedger, error)
}

type budgetRepo struct{ travel, expense *sql.DB }

// NewBudgetRepository constructs a BudgetRepository. travel is the travelapp
// pool; expense is the expense_app pool (read-only), nil when not configured.
func NewBudgetRepository(travel, expense *sql.DB) BudgetRepository {
	return &budgetRepo{travel: travel, expense: expense}
}

func budgetNullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func budgetRound(v float64) float64 { return math.Round(v*100) / 100 }

func budgetIsDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func (r *budgetRepo) Years(ctx context.Context) ([]domain.BudgetYear, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT budget_year, DATE_FORMAT(start_date, '%Y-%m-%d'),
			DATE_FORMAT(end_date, '%Y-%m-%d'), is_active
		FROM budget_year ORDER BY budget_year`)
	if err != nil {
		return nil, fmt.Errorf("budget.Years: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetYear{}
	for rows.Next() {
		var y domain.BudgetYear
		if err := rows.Scan(&y.Year, &y.StartDate, &y.EndDate, &y.IsActive); err != nil {
			return nil, fmt.Errorf("budget.Years scan: %w", err)
		}
		out = append(out, y)
	}
	return out, rows.Err()
}

const budgetLineCols = `l.id, l.budget_year, l.code, l.function_name, l.sub_function, l.holder_name, l.holder_email,
	l.amount_usd, l.status, l.sort_order, l.updated_by, DATE_FORMAT(l.updated_at, '%Y-%m-%dT%H:%i:%s')`

func scanBudgetLine(sc interface{ Scan(...any) error }) (domain.BudgetLine, error) {
	var (
		l                           domain.BudgetLine
		holder, email, updBy, updAt sql.NullString
	)
	if err := sc.Scan(&l.ID, &l.Year, &l.Code, &l.FunctionName, &l.SubFunction, &holder, &email,
		&l.AmountUSD, &l.Status, &l.SortOrder, &updBy, &updAt); err != nil {
		return l, err
	}
	l.HolderName, l.HolderEmail, l.UpdatedBy, l.UpdatedAt = nullStr(holder), nullStr(email), nullStr(updBy), nullStr(updAt)
	return l, nil
}

func (r *budgetRepo) Lines(ctx context.Context, year int, includeRetired bool) ([]domain.BudgetLine, error) {
	q := `SELECT ` + budgetLineCols + ` FROM budget_line AS l WHERE 1 = 1`
	var args []any
	if year > 0 {
		q += ` AND l.budget_year = ?`
		args = append(args, year)
	}
	if !includeRetired {
		q += ` AND l.status = ?`
		args = append(args, domain.BudgetLineActive)
	}
	rows, err := r.travel.QueryContext(ctx, q+` ORDER BY l.sort_order, l.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("budget.Lines: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetLine{}
	for rows.Next() {
		l, err := scanBudgetLine(rows)
		if err != nil {
			return nil, fmt.Errorf("budget.Lines scan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *budgetRepo) Line(ctx context.Context, id int) (domain.BudgetLine, error) {
	l, err := scanBudgetLine(r.travel.QueryRowContext(ctx, `SELECT `+budgetLineCols+` FROM budget_line AS l WHERE l.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrBudgetNotFound
	}
	if err != nil {
		return l, fmt.Errorf("budget.Line: %w", err)
	}
	return l, nil
}

func (r *budgetRepo) LineCodeExists(ctx context.Context, code string) (bool, error) {
	var n int
	if err := r.travel.QueryRowContext(ctx, `SELECT COUNT(*) FROM budget_line WHERE code = ?`, code).Scan(&n); err != nil {
		return false, fmt.Errorf("budget.LineCodeExists: %w", err)
	}
	return n > 0, nil
}

func budgetAudit(action audit.Action, entity, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: entity, Actor: actor}
}

// CreateLine inserts an Active line; a duplicate (year, code) is a conflict.
func (r *budgetRepo) CreateLine(ctx context.Context, in domain.BudgetLineCreateRequest) (int, error) {
	var id int64
	err := WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO budget_line (budget_year, code, function_name, sub_function,
				holder_name, holder_email, amount_usd, status, sort_order, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.Year, in.Code, in.FunctionName, in.SubFunction, budgetNullable(in.HolderName), budgetNullable(in.HolderEmail),
			in.AmountUSD, domain.BudgetLineActive, in.SortOrder, nullableActor(in.CreatedBy))
		if err != nil {
			if budgetIsDuplicate(err) {
				return &apierror.ConflictError{Msg: "A budget line with this code already exists for the year."}
			}
			return fmt.Errorf("budget.CreateLine: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("budget.CreateLine id: %w", err)
		}
		return audit.Inserted("budget_line", "id", id).Record(ctx, tx, budgetAudit(audit.Create, "budget_line", in.CreatedBy))
	})
	return int(id), err
}

func nullableActor(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (r *budgetRepo) UpdateLine(ctx context.Context, id int, in domain.BudgetLineUpdateRequest) error {
	return WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "budget_line", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE budget_line SET function_name = ?, sub_function = ?, holder_name = ?,
				holder_email = ?, amount_usd = ?, sort_order = ?, status = ?, updated_by = ?, updated_at = NOW()
			WHERE id = ?`,
			in.FunctionName, in.SubFunction, budgetNullable(in.HolderName), budgetNullable(in.HolderEmail),
			in.AmountUSD, in.SortOrder, in.Status, nullableActor(in.UpdatedBy), id); err != nil {
			return fmt.Errorf("budget.UpdateLine: %w", err)
		}
		return t.Record(ctx, tx, budgetAudit(audit.Update, "budget_line", in.UpdatedBy))
	})
}

// RetireLine is the soft delete: status Retired. Trips keep the line.
func (r *budgetRepo) RetireLine(ctx context.Context, id int, actor string) error {
	return WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "budget_line", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE budget_line SET status = ?, updated_by = ?, updated_at = NOW()
			WHERE id = ?`, domain.BudgetLineRetired, nullableActor(actor), id); err != nil {
			return fmt.Errorf("budget.RetireLine: %w", err)
		}
		return t.Record(ctx, tx, budgetAudit(audit.Update, "budget_line", actor))
	})
}

func (r *budgetRepo) AllocationTypeRules(ctx context.Context) ([]domain.BudgetAllocationTypeRule, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT allocation_type_id, rule_kind, line_code
		FROM budget_allocation_type_rule ORDER BY allocation_type_id`)
	if err != nil {
		return nil, fmt.Errorf("budget.AllocationTypeRules: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetAllocationTypeRule{}
	for rows.Next() {
		var (
			ru   domain.BudgetAllocationTypeRule
			code sql.NullString
		)
		if err := rows.Scan(&ru.AllocationTypeID, &ru.RuleKind, &code); err != nil {
			return nil, fmt.Errorf("budget.AllocationTypeRules scan: %w", err)
		}
		ru.LineCode = nullStr(code)
		out = append(out, ru)
	}
	return out, rows.Err()
}

// upsert runs update (with its audit tracker) when the row exists, else
// insert, in tx. exists counts the row with a lock.
func budgetUpsert(ctx context.Context, tx *sql.Tx, table, key string, keyVal any, actor string,
	insert func() error, update func() error) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+key+` = ? FOR UPDATE`, keyVal).Scan(&n); err != nil {
		return fmt.Errorf("budget.%s exists: %w", table, err)
	}
	if n == 0 {
		if err := insert(); err != nil {
			return err
		}
		return audit.Inserted(table, key, keyVal).Record(ctx, tx, budgetAudit(audit.Create, table, actor))
	}
	t, err := audit.Track(ctx, tx, table, key, key+` = ?`, keyVal)
	if err != nil {
		return err
	}
	if err := update(); err != nil {
		return err
	}
	return t.Record(ctx, tx, budgetAudit(audit.Update, table, actor))
}

func (r *budgetRepo) SaveAllocationTypeRule(ctx context.Context, id int, in domain.BudgetAllocationTypeRuleRequest) error {
	return WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		return budgetUpsert(ctx, tx, "budget_allocation_type_rule", "allocation_type_id", id, in.UpdatedBy,
			func() error {
				_, err := tx.ExecContext(ctx, `INSERT INTO budget_allocation_type_rule
						(allocation_type_id, rule_kind, line_code, updated_by, updated_at) VALUES (?, ?, ?, ?, NOW())`,
					id, in.RuleKind, budgetNullable(in.LineCode), nullableActor(in.UpdatedBy))
				return wrapBudget("budget.SaveAllocationTypeRule", err)
			},
			func() error {
				_, err := tx.ExecContext(ctx, `UPDATE budget_allocation_type_rule SET rule_kind = ?, line_code = ?,
						updated_by = ?, updated_at = NOW() WHERE allocation_type_id = ?`,
					in.RuleKind, budgetNullable(in.LineCode), nullableActor(in.UpdatedBy), id)
				return wrapBudget("budget.SaveAllocationTypeRule", err)
			})
	})
}

func wrapBudget(what string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

func (r *budgetRepo) RegionRules(ctx context.Context) ([]domain.BudgetRegionRule, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT region, sub_region, line_code FROM budget_region_rule ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("budget.RegionRules: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetRegionRule{}
	for rows.Next() {
		var (
			ru        domain.BudgetRegionRule
			sub, code sql.NullString
		)
		if err := rows.Scan(&ru.Region, &sub, &code); err != nil {
			return nil, fmt.Errorf("budget.RegionRules scan: %w", err)
		}
		ru.SubRegion, ru.LineCode = nullStr(sub), nullStr(code)
		out = append(out, ru)
	}
	return out, rows.Err()
}

func (r *budgetRepo) TeamRules(ctx context.Context) ([]domain.BudgetTeamRule, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT id, business_unit, department, team, line_code
		FROM budget_team_rule ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("budget.TeamRules: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetTeamRule{}
	for rows.Next() {
		var (
			ru         domain.BudgetTeamRule
			bu, dep, t sql.NullString
		)
		if err := rows.Scan(&ru.ID, &bu, &dep, &t, &ru.LineCode); err != nil {
			return nil, fmt.Errorf("budget.TeamRules scan: %w", err)
		}
		ru.BusinessUnit, ru.Department, ru.Team = nullStr(bu), nullStr(dep), nullStr(t)
		out = append(out, ru)
	}
	return out, rows.Err()
}

func (r *budgetRepo) EngagementLine(ctx context.Context, engagementID string) (*string, error) {
	var code sql.NullString
	err := r.travel.QueryRowContext(ctx, `SELECT line_code FROM engagement_budget_line WHERE engagement_id = ?`,
		engagementID).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("budget.EngagementLine: %w", err)
	}
	return nullStr(code), nil
}

func (r *budgetRepo) SetEngagementLine(ctx context.Context, engagementID string, code *string, actor string) error {
	return WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		if code == nil {
			t, err := audit.Track(ctx, tx, "engagement_budget_line", "engagement_id", `engagement_id = ?`, engagementID)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM engagement_budget_line WHERE engagement_id = ?`, engagementID); err != nil {
				return fmt.Errorf("budget.SetEngagementLine delete: %w", err)
			}
			return t.Record(ctx, tx, budgetAudit(audit.Delete, "engagement_budget_line", actor))
		}
		return budgetUpsert(ctx, tx, "engagement_budget_line", "engagement_id", engagementID, actor,
			func() error {
				_, err := tx.ExecContext(ctx, `INSERT INTO engagement_budget_line (engagement_id, line_code, updated_by, updated_at)
					VALUES (?, ?, ?, NOW())`, engagementID, *code, nullableActor(actor))
				return wrapBudget("budget.SetEngagementLine", err)
			},
			func() error {
				_, err := tx.ExecContext(ctx, `UPDATE engagement_budget_line SET line_code = ?, updated_by = ?, updated_at = NOW()
					WHERE engagement_id = ?`, *code, nullableActor(actor), engagementID)
				return wrapBudget("budget.SetEngagementLine", err)
			})
	})
}

const budgetTripCols = `t.job_number, t.traveller_email, ` + dashboardTripCountry + `,
	DATE_FORMAT(t.start_date, '%Y-%m-%d'), DATE_FORMAT(t.end_date, '%Y-%m-%d'), t.status, t.engagement_id,
	t.engagement_type, t.customer_name,
	COALESCE(tb.budget_year, (SELECT y.budget_year FROM budget_year AS y
		WHERE DATE(t.start_date) BETWEEN y.start_date AND y.end_date AND y.is_active = 1 LIMIT 1)),
	tb.budget_line_id, tb.suggested_line_id, tb.suggestion_source, tb.chosen_by,
	DATE_FORMAT(COALESCE(tb.rate_date, DATE(t.start_date)), '%Y-%m-%d'), tb.job_number IS NOT NULL`

const budgetTripFrom = ` FROM travel AS t LEFT JOIN travel_budget AS tb ON tb.job_number = t.job_number`

func scanBudgetTrip(sc interface{ Scan(...any) error }) (domain.BudgetTrip, error) {
	var (
		b                                          domain.BudgetTrip
		email, country, status, eng, engType, cust sql.NullString
		src, by                                    sql.NullString
		year, line, sug                            sql.NullInt64
	)
	if err := sc.Scan(&b.JobNumber, &email, &country, &b.StartDate, &b.EndDate, &status, &eng, &engType, &cust,
		&year, &line, &sug, &src, &by, &b.RateDate, &b.HasRow); err != nil {
		return b, err
	}
	b.TravellerEmail, b.Country, b.Status, b.EngagementID = nullStr(email), nullStr(country), nullStr(status), nullStr(eng)
	b.EngagementType, b.CustomerName, b.SuggestionSource, b.ChosenBy = nullStr(engType), nullStr(cust), nullStr(src), nullStr(by)
	b.BudgetYear, b.BudgetLineID, b.SuggestedLineID = nullInt(year), nullInt(line), nullInt(sug)
	return b, nil
}

func nullInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func (r *budgetRepo) Trip(ctx context.Context, jobNumber string) (domain.BudgetTrip, error) {
	b, err := scanBudgetTrip(r.travel.QueryRowContext(ctx, `SELECT `+budgetTripCols+budgetTripFrom+` WHERE t.job_number = ?`, jobNumber))
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrBudgetNotFound
	}
	if err != nil {
		return b, fmt.Errorf("budget.Trip: %w", err)
	}
	return b, nil
}

// SetTripLine keeps the suggestion fields and rate_date of an existing row;
// a new row takes rate_date = DATE(travel.start_date).
func (r *budgetRepo) SetTripLine(ctx context.Context, jobNumber string, year, lineID *int, actor string) error {
	return WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		return budgetUpsert(ctx, tx, "travel_budget", "job_number", jobNumber, actor,
			func() error {
				_, err := tx.ExecContext(ctx, `INSERT INTO travel_budget (job_number, budget_year, budget_line_id, chosen_by,
						rate_date, updated_by, updated_at)
					SELECT t.job_number, ?, ?, ?, DATE(t.start_date), ?, NOW() FROM travel AS t WHERE t.job_number = ?`,
					intPtr(year), intPtr(lineID), domain.BudgetChosenUser, nullableActor(actor), jobNumber)
				return wrapBudget("budget.SetTripLine", err)
			},
			func() error {
				_, err := tx.ExecContext(ctx, `UPDATE travel_budget SET budget_year = ?, budget_line_id = ?, chosen_by = ?,
						updated_by = ?, updated_at = NOW() WHERE job_number = ?`,
					intPtr(year), intPtr(lineID), domain.BudgetChosenUser, nullableActor(actor), jobNumber)
				return wrapBudget("budget.SetTripLine", err)
			})
	})
}

func intPtr(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// NeedsLine lists the trips of year, not Cancelled, with no budget row or no
// line, by start date.
func (r *budgetRepo) NeedsLine(ctx context.Context, year, limit, offset int) ([]domain.BudgetTrip, int, error) {
	where := ` JOIN budget_year AS y ON y.budget_year = ? AND DATE(t.start_date) BETWEEN y.start_date AND y.end_date
		WHERE COALESCE(t.status, '') != 'Cancelled' AND (tb.job_number IS NULL OR tb.budget_line_id IS NULL)`
	var total int
	if err := r.travel.QueryRowContext(ctx, `SELECT COUNT(*)`+budgetTripFrom+where, year).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("budget.NeedsLine count: %w", err)
	}
	rows, err := r.travel.QueryContext(ctx, `SELECT `+budgetTripCols+budgetTripFrom+where+
		` ORDER BY t.start_date, t.job_number LIMIT ? OFFSET ?`, year, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("budget.NeedsLine: %w", err)
	}
	defer rows.Close()
	out := []domain.BudgetTrip{}
	for rows.Next() {
		b, err := scanBudgetTrip(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("budget.NeedsLine scan: %w", err)
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

type budgetCostKey struct{ job, source, currency string }

// Ledger lists the trips starting in f's range with their summed costs per
// source and currency (cancelled trips included). Card and claim costs and
// the unassigned travel spend come from expense_app; without that pool, or
// when it fails, ExpenseAvailable is false and they are left out.
func (r *budgetRepo) Ledger(ctx context.Context, f domain.BudgetLedgerFilter) (domain.BudgetLedger, error) {
	out := domain.BudgetLedger{From: f.From, To: f.To, Trips: []domain.BudgetLedgerTrip{}, Costs: []domain.BudgetLedgerCost{}}
	rows, err := r.travel.QueryContext(ctx, `SELECT t.job_number, tb.budget_line_id, t.status, `+dashboardTripCountry+`,
			DATE_FORMAT(t.start_date, '%Y-%m-%d'), DATE_FORMAT(t.end_date, '%Y-%m-%d'),
			DATE_FORMAT(COALESCE(tb.rate_date, DATE(t.start_date)), '%Y-%m-%d')`+budgetTripFrom+`
		WHERE DATE(t.start_date) BETWEEN ? AND ? ORDER BY t.start_date, t.job_number LIMIT ?`,
		f.From, f.To, budgetLedgerTripMax+1)
	if err != nil {
		return out, fmt.Errorf("budget.Ledger trips: %w", err)
	}
	var jobs []string
	for rows.Next() {
		var (
			tr              domain.BudgetLedgerTrip
			line            sql.NullInt64
			status, country sql.NullString
		)
		if err := rows.Scan(&tr.JobNumber, &line, &status, &country, &tr.StartDate, &tr.EndDate, &tr.RateDate); err != nil {
			rows.Close()
			return out, fmt.Errorf("budget.Ledger trips scan: %w", err)
		}
		tr.BudgetLineID, tr.Status, tr.Country = nullInt(line), nullStr(status), nullStr(country)
		out.Trips = append(out.Trips, tr)
		jobs = append(jobs, tr.JobNumber)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, fmt.Errorf("budget.Ledger trips rows: %w", err)
	}
	if len(out.Trips) > budgetLedgerTripMax {
		out.Truncated = true
		out.Trips, jobs = out.Trips[:budgetLedgerTripMax], jobs[:budgetLedgerTripMax]
	}

	sums := map[budgetCostKey]float64{}
	if err := r.travelCosts(ctx, f, sums); err != nil {
		return out, err
	}
	var unassigned []domain.BudgetLedgerUnassigned
	if r.expense != nil {
		if unassigned, err = r.expenseCosts(ctx, f, jobs, sums); err != nil {
			// Card and claim spend is an add-on: the ledger still serves
			// invoices and per diems.
			log.Printf("budget.Ledger: expense_app unavailable: %v", err)
			for k := range sums {
				if k.source == domain.DashboardCostCard || k.source == domain.DashboardCostClaim {
					delete(sums, k)
				}
			}
			unassigned = nil
		} else {
			out.ExpenseAvailable = true
		}
	}
	keys := make([]budgetCostKey, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.job != b.job {
			return a.job < b.job
		}
		if a.source != b.source {
			return a.source < b.source
		}
		return a.currency < b.currency
	})
	for _, k := range keys {
		out.Costs = append(out.Costs, domain.BudgetLedgerCost{JobNumber: k.job, Source: k.source, Currency: k.currency, Amount: budgetRound(sums[k])})
	}
	if f.Unassigned {
		if unassigned == nil {
			unassigned = []domain.BudgetLedgerUnassigned{}
		}
		out.Unassigned = &unassigned
	}
	return out, nil
}

// travelCosts adds the invoice and per diem sums of the range's trips.
func (r *budgetRepo) travelCosts(ctx context.Context, f domain.BudgetLedgerFilter, sums map[budgetCostKey]float64) error {
	queries := []struct{ source, q string }{
		{domain.DashboardCostInvoice, `SELECT t.job_number, UPPER(TRIM(COALESCE(i.invoice_currency, ''))), SUM(i.invoice_amount)
			FROM invoice AS i
			JOIN travel AS t ON i.job_number = t.job_number
			LEFT JOIN inv_cost_type AS ct ON i.inv_cost_type_id = ct.id
			WHERE DATE(t.start_date) BETWEEN ? AND ?
				AND ` + costInvoiceStatusSQL + `
				AND ` + costInvoiceTypeSQL + `
			GROUP BY t.job_number, UPPER(TRIM(COALESCE(i.invoice_currency, '')))`},
		{domain.DashboardCostPerDiem, `SELECT t.job_number, UPPER(TRIM(COALESCE(pp.payment_currency, ''))), SUM(pp.payment_amount)
			FROM perdiem_payment AS pp
			JOIN travel AS t ON pp.job_number = t.job_number
			WHERE DATE(t.start_date) BETWEEN ? AND ?
			GROUP BY t.job_number, UPPER(TRIM(COALESCE(pp.payment_currency, '')))`},
	}
	for _, s := range queries {
		rows, err := r.travel.QueryContext(ctx, s.q, f.From, f.To)
		if err != nil {
			return fmt.Errorf("budget.Ledger %s: %w", s.source, err)
		}
		for rows.Next() {
			var (
				job, cur string
				amt      sql.NullFloat64
			)
			if err := rows.Scan(&job, &cur, &amt); err != nil {
				rows.Close()
				return fmt.Errorf("budget.Ledger %s scan: %w", s.source, err)
			}
			sums[budgetCostKey{job, s.source, cur}] += amt.Float64
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("budget.Ledger %s rows: %w", s.source, err)
		}
	}
	return nil
}

// budgetExpenseSources are the expense_app tables behind each source.
var budgetExpenseSources = []struct{ source, table, status, ccCols string }{
	{domain.DashboardCostCard, "cc_txn", costCardStatusSQL, costCardCCCols},
	{domain.DashboardCostClaim, "expense_claims", costClaimStatusSQL, costClaimCCCols},
}

// expenseCosts adds the card and claim sums of jobs to sums and returns the
// travel spend with no job number, by source, month and currency.
func (r *budgetRepo) expenseCosts(ctx context.Context, f domain.BudgetLedgerFilter, jobs []string, sums map[budgetCostKey]float64) ([]domain.BudgetLedgerUnassigned, error) {
	// A card billed in USD reports USD and its billed amount.
	money := func(cur string, amt float64, ccCur string, ccAmt float64) (string, float64) {
		if ccCur == "USD" {
			return "USD", ccAmt
		}
		return cur, amt
	}
	for _, s := range budgetExpenseSources {
		for start := 0; start < len(jobs); start += travelInChunk {
			chunk := jobs[start:min(start+travelInChunk, len(jobs))]
			rows, err := r.expense.QueryContext(ctx, `SELECT TRIM(c.travel_job_number), UPPER(TRIM(COALESCE(c.txn_currency, ''))),
					c.txn_amount, `+s.ccCols+`
				FROM `+s.table+` AS c
				WHERE TRIM(c.travel_job_number) IN (`+Placeholders(len(chunk))+`) AND `+s.status, Args(chunk)...)
			if err != nil {
				return nil, fmt.Errorf("budget.Ledger %s: %w", s.source, err)
			}
			for rows.Next() {
				var (
					job, cur, ccCur string
					amt, ccAmt      sql.NullFloat64
				)
				if err := rows.Scan(&job, &cur, &amt, &ccCur, &ccAmt); err != nil {
					rows.Close()
					return nil, fmt.Errorf("budget.Ledger %s scan: %w", s.source, err)
				}
				c, a := money(cur, amt.Float64, ccCur, ccAmt.Float64)
				sums[budgetCostKey{job, s.source, c}] += a
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, fmt.Errorf("budget.Ledger %s rows: %w", s.source, err)
			}
		}
	}
	if !f.Unassigned {
		return nil, nil
	}
	type key struct{ source, month, currency string }
	acc := map[key]float64{}
	rangeSQL, rangeArgs := dashboardRangeWhere("c.txn_date", domain.DashboardCostLinesFilter{From: f.From, To: f.To})
	for _, s := range budgetExpenseSources {
		args := append(append([]any{}, rangeArgs...), dashboardTravelCategory)
		rows, err := r.expense.QueryContext(ctx, `SELECT DATE_FORMAT(c.txn_date, '%Y-%m'), UPPER(TRIM(COALESCE(c.txn_currency, ''))),
				c.txn_amount, `+s.ccCols+`
			FROM `+s.table+` AS c
			LEFT JOIN expense_type AS et ON et.id = c.expense_type_id
			LEFT JOIN expense_category AS ec ON ec.id = et.expense_category_id
			WHERE `+rangeSQL+` AND `+s.status+`
				AND (c.travel_job_number IS NULL OR TRIM(c.travel_job_number) = '')
				AND ec.expense_category LIKE ?`, args...)
		if err != nil {
			return nil, fmt.Errorf("budget.Ledger unassigned %s: %w", s.source, err)
		}
		for rows.Next() {
			var (
				month, cur, ccCur string
				amt, ccAmt        sql.NullFloat64
			)
			if err := rows.Scan(&month, &cur, &amt, &ccCur, &ccAmt); err != nil {
				rows.Close()
				return nil, fmt.Errorf("budget.Ledger unassigned %s scan: %w", s.source, err)
			}
			c, a := money(cur, amt.Float64, ccCur, ccAmt.Float64)
			acc[key{s.source, month, c}] += a
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("budget.Ledger unassigned %s rows: %w", s.source, err)
		}
	}
	out := make([]domain.BudgetLedgerUnassigned, 0, len(acc))
	for k, v := range acc {
		out = append(out, domain.BudgetLedgerUnassigned{Source: k.source, Month: k.month, Currency: k.currency, Amount: budgetRound(v)})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Month != b.Month {
			return a.Month < b.Month
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Currency < b.Currency
	})
	return out, nil
}
