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
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// TravelRequestRepository reads and writes travelapp.travel and the tables
// the travel request screens need from travelapp (travel_agents,
// visa_status, perdiem_allocation, email_auditlog).
type TravelRequestRepository interface {
	// Search returns one page of matches and the total.
	Search(ctx context.Context, f domain.TravelRequestSearch) ([]domain.TravelRequest, int, error)
	// StatusCounts returns the matches per stored status, ignoring f.Statuses.
	StatusCounts(ctx context.Context, f domain.TravelRequestSearch) (map[string]int, error)
	// Get returns the request with this job number, or nil.
	Get(ctx context.Context, jobNumber string) (*domain.TravelRequest, error)
	// Update applies in to the request (legacy updateTravel) and reports
	// whether a row matched.
	Update(ctx context.Context, jobNumber string, in domain.UpdateTravelRequestInput) (bool, error)
	// Agents lists travel agents; status "" means all.
	Agents(ctx context.Context, status string) ([]domain.TravelAgent, error)
	// ActivePerDiems lists the Active per diems of a request.
	ActivePerDiems(ctx context.Context, jobNumber string) ([]domain.TravelActivePerDiem, error)
	// InsertEmailAudit writes one email_auditlog row.
	InsertEmailAudit(ctx context.Context, in domain.TravelEmailAuditInput) error
}

type travelRequestRepo struct{ db *sql.DB }

// NewTravelRequestRepository constructs a TravelRequestRepository. db must be
// the travelapp pool.
func NewTravelRequestRepository(db *sql.DB) TravelRequestRepository {
	return &travelRequestRepo{db: db}
}

// travelRequestColumns are the columns of a TravelRequest, in scan order.
const travelRequestColumns = `t.job_number, COALESCE(t.status, ''), t.traveller_email, REPLACE(t.city, 'BR()', CHAR(10)), t.country,
		t.customer_name, t.start_date, t.end_date, t.allocation_id, t.engagement_id, t.engagement_code,
		t.engagement_type, t.travel_agent_id, ta.name_of_travel_agent, t.visa_id, t.visa,
		v.email, v.country, v.visa_type, v.entry_type, v.start_date, v.end_date,
		t.last_updated_by, t.last_updated_date`

const travelRequestFrom = ` FROM travel AS t
		LEFT JOIN travel_agents AS ta ON ta.id = t.travel_agent_id
		LEFT JOIN visa_status AS v ON v.id = t.visa_id`

// travelWhere builds the WHERE clause of a search. The base condition is
// that of the legacy get_sorted_all_travel_requests view: no Deleted, empty
// or NULL status ("<>" is never true for NULL).
func travelWhere(f domain.TravelRequestSearch, withStatuses bool) (string, []any) {
	var b strings.Builder
	var args []any
	b.WriteString(` WHERE t.status NOT IN ('Deleted', '')`)
	in := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		b.WriteString(` AND ` + col + ` IN (` + Placeholders(len(vals)) + `)`)
		args = append(args, Args(vals)...)
	}
	if withStatuses {
		in("t.status", f.Statuses)
	}
	if f.TravelAgentID != nil {
		b.WriteString(` AND t.travel_agent_id = ?`)
		args = append(args, *f.TravelAgentID)
	}
	in("t.traveller_email", f.TravellerEmails)
	in("t.country", f.Countries)
	in("ta.name_of_travel_agent", f.AgencyNames)
	if f.VisaID != nil {
		b.WriteString(` AND t.visa_id = ?`)
		args = append(args, *f.VisaID)
	}
	if f.StartFrom != "" {
		b.WriteString(` AND DATE(t.start_date) >= ?`)
		args = append(args, f.StartFrom)
	}
	if f.EndTo != "" {
		b.WriteString(` AND DATE(t.end_date) <= ?`)
		args = append(args, f.EndTo)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := LikeContains(s)
		b.WriteString(` AND (t.job_number LIKE ? OR t.traveller_email LIKE ? OR t.city LIKE ? OR t.country LIKE ?`)
		args = append(args, like, like, like, like)
		if f.SearchEngagements {
			b.WriteString(` OR t.customer_name LIKE ?`)
			args = append(args, like)
			if len(f.EngagementIDs) > 0 {
				b.WriteString(` OR t.engagement_id IN (` + Placeholders(len(f.EngagementIDs)) + `)`)
				args = append(args, Args(f.EngagementIDs)...)
			}
		}
		if f.SearchAgency {
			b.WriteString(` OR ta.name_of_travel_agent LIKE ?`)
			args = append(args, like)
		}
		b.WriteString(`)`)
	}
	return b.String(), args
}

// travelOrder returns the ORDER BY of a whitelisted sort.
func travelOrder(f domain.TravelRequestSearch) (string, []any) {
	if f.Sort == domain.TravelSortStartDesc {
		return ` ORDER BY t.start_date DESC, t.job_number`, nil
	}
	today := f.Today
	if today == "" {
		today = time.Now().UTC().Format(time.DateOnly)
	}
	return ` ORDER BY CASE WHEN DATE(t.end_date) >= ? THEN 0 ELSE 1 END,
		CASE WHEN DATE(t.end_date) >= ? THEN t.start_date END ASC,
		t.start_date DESC, t.job_number`, []any{today, today}
}

func (r *travelRequestRepo) Search(ctx context.Context, f domain.TravelRequestSearch) ([]domain.TravelRequest, int, error) {
	where, args := travelWhere(f, true)

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+travelRequestFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("travel.Count: %w", err)
	}
	if total == 0 {
		return []domain.TravelRequest{}, 0, nil
	}

	order, orderArgs := travelOrder(f)
	q := `SELECT ` + travelRequestColumns + travelRequestFrom + where + order + ` LIMIT ? OFFSET ?`
	all := append(append(append([]any{}, args...), orderArgs...), f.Limit, f.Offset)
	rows, err := r.db.QueryContext(ctx, q, all...)
	if err != nil {
		return nil, 0, fmt.Errorf("travel.Search: %w", err)
	}
	defer rows.Close()

	out := []domain.TravelRequest{}
	for rows.Next() {
		t, err := scanTravelRequest(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("travel.Search scan: %w", err)
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (r *travelRequestRepo) StatusCounts(ctx context.Context, f domain.TravelRequestSearch) (map[string]int, error) {
	where, args := travelWhere(f, false)
	rows, err := r.db.QueryContext(ctx,
		`SELECT COALESCE(t.status, ''), COUNT(*)`+travelRequestFrom+where+` GROUP BY t.status`, args...)
	if err != nil {
		return nil, fmt.Errorf("travel.StatusCounts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var (
			status string
			n      int
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("travel.StatusCounts scan: %w", err)
		}
		out[status] += n
	}
	return out, rows.Err()
}

// Get ports the legacy getTravel (travel(jobNumber)): no status condition,
// so a Deleted request still opens.
func (r *travelRequestRepo) Get(ctx context.Context, jobNumber string) (*domain.TravelRequest, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+travelRequestColumns+travelRequestFrom+` WHERE t.job_number = ? LIMIT 1`, jobNumber)
	t, err := scanTravelRequest(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("travel.Get: %w", err)
	}
	return &t, nil
}

// Update ports the legacy updateTravel: last_updated_by and
// last_updated_date = NOW() always, then only the given fields.
func (r *travelRequestRepo) Update(ctx context.Context, jobNumber string, in domain.UpdateTravelRequestInput) (bool, error) {
	q := `UPDATE travel SET last_updated_by = ?, last_updated_date = NOW()`
	args := []any{textnorm.Latin1(in.UpdatedBy)}
	if in.Status != nil {
		q += `, status = ?`
		args = append(args, *in.Status)
	}
	if in.TravelAgentID != nil {
		q += `, travel_agent_id = ?`
		args = append(args, *in.TravelAgentID)
	}
	if in.VisaID != nil {
		q += `, visa_id = ?`
		args = append(args, *in.VisaID)
	}
	q += ` WHERE job_number = ?`
	args = append(args, jobNumber)
	var n int64
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "travel", "job_number", `job_number = ?`, jobNumber)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return fmt.Errorf("travel.Update: %w", err)
		}
		if n, err = res.RowsAffected(); err != nil {
			return fmt.Errorf("travel.Update rows: %w", err)
		}
		return t.Record(ctx, tx, travelAuditEvent(audit.Update, in.UpdatedBy))
	})
	// The pools set clientFoundRows, so n counts matched rows: 0 means no
	// such job number.
	return n > 0, err
}

// Agents ports the legacy getTravelAgentsQuery (ordered by name).
func (r *travelRequestRepo) Agents(ctx context.Context, status string) ([]domain.TravelAgent, error) {
	q := `SELECT id, name_of_travel_agent, status FROM travel_agents`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY name_of_travel_agent`, args...)
	if err != nil {
		return nil, fmt.Errorf("travelAgents.List: %w", err)
	}
	defer rows.Close()
	out := []domain.TravelAgent{}
	for rows.Next() {
		var a domain.TravelAgent
		if err := rows.Scan(&a.ID, &a.Name, &a.Status); err != nil {
			return nil, fmt.Errorf("travelAgents.List scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActivePerDiems ports getPerDiemAllocations({jobNumber, status: "Active"})
// as the legacy Completed email reads it. ORDER BY id is added for a stable
// table.
func (r *travelRequestRepo) ActivePerDiems(ctx context.Context, jobNumber string) ([]domain.TravelActivePerDiem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT start_date, end_date, no_of_days, currency, perdiem_value, total
		FROM perdiem_allocation
		WHERE job_number = ? AND status = 'Active'
		ORDER BY id`, jobNumber)
	if err != nil {
		return nil, fmt.Errorf("perdiemAllocation.Active: %w", err)
	}
	defer rows.Close()
	out := []domain.TravelActivePerDiem{}
	for rows.Next() {
		var (
			p          domain.TravelActivePerDiem
			start, end sql.NullTime
			days       sql.NullInt64
			total      sql.NullFloat64
		)
		if err := rows.Scan(&start, &end, &days, &p.Currency, &p.PerDiemValue, &total); err != nil {
			return nil, fmt.Errorf("perdiemAllocation.Active scan: %w", err)
		}
		p.StartDate, p.EndDate = travelDateTime(start), travelDateTime(end)
		if days.Valid {
			d := int(days.Int64)
			p.NoOfDays = &d
		}
		if total.Valid {
			v := total.Float64
			p.Total = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// travelAuditLimits are the email_auditlog column sizes; longer values are
// cut so that a strict-mode insert never fails on length.
const (
	travelAuditShort   = 255
	travelAuditContent = 3000
)

// InsertEmailAudit ports the legacy insertEmailAuditLogQuery.
func (r *travelRequestRepo) InsertEmailAudit(ctx context.Context, in domain.TravelEmailAuditInput) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO email_auditlog
		(subject, templateId, contentKeyValPairs, recipients, cc_recipients, datetime)
		VALUES (?, ?, ?, ?, ?, NOW())`,
		textnorm.Truncate(textnorm.Latin1(in.Subject), travelAuditShort),
		textnorm.Truncate(textnorm.Latin1(in.TemplateID), travelAuditShort),
		textnorm.Truncate(textnorm.Latin1(in.ContentKeyValPairs), travelAuditContent),
		textnorm.Truncate(textnorm.Latin1(in.Recipients), travelAuditShort),
		textnorm.Truncate(textnorm.Latin1(in.CCRecipients), travelAuditShort))
	if err != nil {
		return fmt.Errorf("emailAuditlog.Insert: %w", err)
	}
	return nil
}

// travelScanner is a *sql.Row or *sql.Rows.
type travelScanner interface{ Scan(dest ...any) error }

func scanTravelRequest(s travelScanner) (domain.TravelRequest, error) {
	var (
		t                         domain.TravelRequest
		start, end, updated       sql.NullTime
		agentID, visaID           sql.NullInt64
		vEmail, vCountry, vType   sql.NullString
		vEntry                    sql.NullString
		vStart, vEnd              sql.NullTime
		engagementCode, engagType sql.NullString
		country, customer         sql.NullString
	)
	if err := s.Scan(&t.JobNumber, &t.Status, &t.TravellerEmail, &t.City, &country,
		&customer, &start, &end, &t.AllocationID, &t.EngagementID, &engagementCode,
		&engagType, &agentID, &t.TravelAgentName, &visaID, &t.VisaText,
		&vEmail, &vCountry, &vType, &vEntry, &vStart, &vEnd,
		&t.LastUpdatedBy, &updated); err != nil {
		return domain.TravelRequest{}, err
	}
	t.Country, t.CustomerName = country.String, customer.String
	t.EngagementCode, t.EngagementType = engagementCode.String, engagType.String
	if v := travelDate(start); v != nil {
		t.StartDate = *v
	}
	if v := travelDate(end); v != nil {
		t.EndDate = *v
	}
	t.LastUpdatedDate = travelDateTime(updated)
	if agentID.Valid {
		id := int(agentID.Int64)
		t.TravelAgentID = &id
	}
	if visaID.Valid {
		id := int(visaID.Int64)
		t.VisaID = &id
		t.Visa = &domain.TravelVisaRef{
			ID: id, Email: travelNullString(vEmail), Country: travelNullString(vCountry),
			VisaType: travelNullString(vType), EntryType: travelNullString(vEntry),
			StartDate: travelDate(vStart), EndDate: travelDate(vEnd),
		}
	}
	return t, nil
}

func travelNullString(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

// travelDate formats a DATETIME as YYYY-MM-DD.
func travelDate(t sql.NullTime) *string {
	if !t.Valid {
		return nil
	}
	v := t.Time.Format(time.DateOnly)
	return &v
}

// travelDateTime formats a DATETIME as YYYY-MM-DD HH:MM:SS.
func travelDateTime(t sql.NullTime) *string {
	if !t.Valid {
		return nil
	}
	v := t.Time.Format(time.DateTime)
	return &v
}
