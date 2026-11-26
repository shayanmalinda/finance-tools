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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// FinanceRepository reads and writes the travel finance tables of travelapp:
// invoice, perdiem_allocation, perdiem_payment, travel_card and their
// lookups. Every statement ports the legacy Finance Entity SQL
// (travel-finance-port-spec §3); differences are noted per method.
type FinanceRepository interface {
	Trip(ctx context.Context, jobNumber string) (*domain.FinanceTrip, error)

	PerDiems(ctx context.Context, jobNumber string) ([]domain.FinancePerDiem, error)
	PerDiem(ctx context.Context, jobNumber string, id int) (*domain.FinancePerDiem, error)
	InsertPerDiem(ctx context.Context, jobNumber, engagementCode string, in domain.FinancePerDiemCreate) (int, error)
	DeletePerDiem(ctx context.Context, jobNumber string, id int) (bool, error)
	PaymentBasis(ctx context.Context, jobNumber string) (domain.FinancePaymentBasis, error)
	PaymentCount(ctx context.Context, jobNumber string) (int, error)

	TravelCards(ctx context.Context, jobNumber string) ([]domain.FinanceTravelCard, error)
	InsertTravelCard(ctx context.Context, jobNumber string, in domain.FinanceTravelCardCreate) (int, error)

	CostTypes(ctx context.Context) ([]domain.FinanceCostType, error)
	CostTypeIDByName(ctx context.Context, name string) (*int, error)
	ClassificationID(ctx context.Context, engagementCodeLike, suffixLike string, costTypeID *int) (*int, error)
	ClassificationData(ctx context.Context, id int) (department, internalID *string, err error)
	InvoiceNumberExists(ctx context.Context, invoiceNumber string) (bool, error)
	InsertInvoices(ctx context.Context, jobNumber, createdBy string, rows []FinanceInvoiceInsert) ([]int, error)
	Invoices(ctx context.Context, f domain.FinanceInvoiceFilter) ([]domain.FinanceInvoice, int, error)
	Invoice(ctx context.Context, id int) (*domain.FinanceInvoice, error)
	InvoiceCounts(ctx context.Context, agencyID int) (domain.FinanceInvoiceCounts, error)
	DecideInvoice(ctx context.Context, id int, d FinanceInvoiceTransition) (bool, error)
	ClaimInvoicePush(ctx context.Context, id int, actor string) (bool, error)
	ReleaseInvoicePush(ctx context.Context, id int, actor string) error
	FailInvoicePush(ctx context.Context, id int, actor, msg string) error
	CompleteInvoicePush(ctx context.Context, id int, ref string) error
	ResolveInvoiceFailure(ctx context.Context, id int, actor, ref string) (bool, error)

	Payments(ctx context.Context, f domain.FinancePaymentFilter) ([]domain.FinancePayment, int, error)
	Payment(ctx context.Context, perDiemNumber string) (*domain.FinancePayment, error)
	PaymentCounts(ctx context.Context) (domain.FinancePaymentCounts, error)
	PaymentNumberExists(ctx context.Context, perDiemNumber string) (bool, error)
	InsertPayment(ctx context.Context, jobNumber string, in domain.FinancePaymentCreate) (int, error)
	UpdatePayment(ctx context.Context, id int, in domain.FinancePaymentUpdate) (bool, error)
	ClaimPaymentPush(ctx context.Context, id int, actor string) (bool, error)
	ReleasePaymentPush(ctx context.Context, id int, actor string) error
	CompletePaymentPush(ctx context.Context, id int, actor string) error
	FailPaymentPush(ctx context.Context, id int, actor string) error
	ResolvePaymentFailure(ctx context.Context, id int, actor string, found bool) (bool, error)

	// AuditPush records the outcome of a NetSuite push or re-sync.
	AuditPush(ctx context.Context, e audit.Event) error
}

// FinanceInvoiceInsert is one invoice row to insert (status Pending).
type FinanceInvoiceInsert struct {
	domain.FinanceInvoiceCreate
	ClassificationID int
}

// FinanceInvoiceTransition is one approve or reject: from status to status,
// the approver column written (hr_approved_by or finance_approved_by) and the
// reason of a rejection.
type FinanceInvoiceTransition struct {
	From, To string
	// ApproverColumn is "hr_approved_by" or "finance_approved_by".
	ApproverColumn string
	Actor          string
	Reason         *string
}

type financeRepo struct{ db *sql.DB }

// NewFinanceRepository constructs a FinanceRepository. db must be the
// travelapp pool.
func NewFinanceRepository(db *sql.DB) FinanceRepository { return &financeRepo{db: db} }

const financeDateTime = "2006-01-02 15:04:05"

// financeMaxRows bounds the lists the legacy entity paged by 100 at a time
// until a short page ("load all").
const financeMaxRows = 10000

func financeStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func financeTime(nt sql.NullTime) *string {
	if !nt.Valid {
		return nil
	}
	s := nt.Time.UTC().Format(financeDateTime)
	return &s
}

func financeInt(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	n := int(ni.Int64)
	return &n
}

func financeFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	f := nf.Float64
	return &f
}

// financeDecimal is a DECIMAL column as text, as Ballerina's decimal
// toString renders it (scale kept: "155100.00").
func financeDecimal(ns sql.NullString) (string, float64) {
	if !ns.Valid {
		return "", 0
	}
	f, _ := strconv.ParseFloat(ns.String, 64)
	return ns.String, f
}

// financeAmount binds a float to a DECIMAL column: the decimal text, so
// MySQL rounds it (half away from zero) exactly as it rounded the legacy
// Ballerina decimal.
func financeAmount(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func financeIn(col string, vals []string, where *[]string, args *[]any) {
	if len(vals) == 0 {
		return
	}
	*where = append(*where, col+" IN ("+Placeholders(len(vals))+")")
	*args = append(*args, Args(vals)...)
}

// ── Trip ────────────────────────────────────────────────────────────────

// Trip reads the travel row the finance rules need (legacy GET
// /travels/{jn} subset) with the agency's name. The live engagement code and
// the campaign come from allocation_db (FinanceAllocationRepository).
func (r *financeRepo) Trip(ctx context.Context, jobNumber string) (*domain.FinanceTrip, error) {
	var (
		t                                                        domain.FinanceTrip
		status, alloc, city, country, customer, code, agencyName sql.NullString
		agencyID                                                 sql.NullInt64
		start, end                                               sql.NullTime
		traveller                                                sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `SELECT t.job_number, t.status, t.traveller_email, t.travel_agent_id,
			ta.name_of_travel_agent, t.allocation_id, REPLACE(t.city, 'BR()', CHAR(10)), t.country, t.start_date, t.end_date,
			t.customer_name, t.engagement_code
		FROM travel t
		LEFT JOIN travel_agents ta ON ta.id = t.travel_agent_id
		WHERE t.job_number = ?`, jobNumber).Scan(&t.JobNumber, &status, &traveller, &agencyID,
		&agencyName, &alloc, &city, &country, &start, &end, &customer, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.Trip: %w", err)
	}
	t.Status, t.AgencyID, t.AgencyName, t.AllocationID = financeStr(status), financeInt(agencyID), financeStr(agencyName), financeStr(alloc)
	t.TravellerEmail = traveller.String
	t.City, t.Country, t.CustomerName, t.EngagementCode = financeStr(city), financeStr(country), financeStr(customer), financeStr(code)
	t.StartDate, t.EndDate = financeTime(start), financeTime(end)
	return &t, nil
}

// ── Per diems ───────────────────────────────────────────────────────────

const financePerDiemCols = `SELECT id, engagement_code, job_number, currency, perdiem_value, total,
			start_date, end_date, no_of_days, status
		FROM perdiem_allocation`

func scanFinancePerDiem(sc interface{ Scan(...any) error }) (domain.FinancePerDiem, error) {
	var (
		p          domain.FinancePerDiem
		cur        sql.NullString
		total      sql.NullFloat64
		start, end sql.NullTime
		days       sql.NullInt64
	)
	if err := sc.Scan(&p.ID, &p.EngagementCode, &p.JobNumber, &cur, &p.PerDiemValue, &total,
		&start, &end, &days, &p.Status); err != nil {
		return p, err
	}
	p.Currency, p.Total, p.StartDate, p.EndDate, p.NoOfDays = financeStr(cur), total.Float64, financeTime(start), financeTime(end), financeInt(days)
	return p, nil
}

// PerDiems ports getPerDiemAllocationDetails({jobNumber, status: "Active"}).
// The legacy entity's default LIMIT 100 is raised to the finance maximum and
// an ORDER BY added so the rows read in the order they were added.
func (r *financeRepo) PerDiems(ctx context.Context, jobNumber string) ([]domain.FinancePerDiem, error) {
	rows, err := r.db.QueryContext(ctx, financePerDiemCols+` WHERE job_number = ? AND status = ? ORDER BY start_date, id LIMIT ?`,
		jobNumber, domain.PerDiemStatusActive, financeMaxRows)
	if err != nil {
		return nil, fmt.Errorf("finance.PerDiems: %w", err)
	}
	defer rows.Close()
	out := []domain.FinancePerDiem{}
	for rows.Next() {
		p, err := scanFinancePerDiem(rows)
		if err != nil {
			return nil, fmt.Errorf("finance.PerDiems scan: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PerDiem ports GET /per-diems/{id} (id = ? LIMIT 1), also requiring the
// per diem to be an active one of jobNumber.
func (r *financeRepo) PerDiem(ctx context.Context, jobNumber string, id int) (*domain.FinancePerDiem, error) {
	p, err := scanFinancePerDiem(r.db.QueryRowContext(ctx, financePerDiemCols+` WHERE id = ? AND job_number = ? AND status = ? LIMIT 1`,
		id, jobNumber, domain.PerDiemStatusActive))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.PerDiem: %w", err)
	}
	return &p, nil
}

// InsertPerDiem ports insertPerDiemAllocation and the BFF's perdiem_auditlog
// write (subject "Perdiem Allocation", description = the input as JSON), in
// one transaction: the legacy wrote them separately.
func (r *financeRepo) InsertPerDiem(ctx context.Context, jobNumber, engagementCode string, in domain.FinancePerDiemCreate) (int, error) {
	legacyLog, err := json.Marshal(struct {
		EngagementCode string  `json:"engagementCode"`
		JobNumber      string  `json:"jobNumber"`
		Currency       string  `json:"currency"`
		PerDiemValue   float64 `json:"perDiemValue"`
		Total          float64 `json:"total"`
		StartDate      string  `json:"startDate"`
		EndDate        string  `json:"endDate"`
		NoOfDays       int     `json:"noOfDays"`
		Status         string  `json:"status"`
	}{engagementCode, jobNumber, in.Currency, in.PerDiemValue, in.Total, in.StartDate, in.EndDate, in.NoOfDays, domain.PerDiemStatusActive})
	if err != nil {
		return 0, fmt.Errorf("finance.InsertPerDiem audit: %w", err)
	}
	var id int64
	err = WithTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT into perdiem_allocation
			(engagement_code, job_number, currency, perdiem_value, total, start_date, end_date, no_of_days, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			textnorm.Latin1(engagementCode), jobNumber, textnorm.Latin1(in.Currency), in.PerDiemValue, in.Total,
			in.StartDate, in.EndDate, in.NoOfDays, domain.PerDiemStatusActive)
		if err != nil {
			return fmt.Errorf("finance.InsertPerDiem: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("finance.InsertPerDiem id: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO perdiem_auditlog VALUES (?, ?, ?, ?, ?, NOW())`,
			"Perdiem Allocation", in.CreatedBy, textnorm.Latin1(engagementCode), jobNumber, textnorm.Latin1(string(legacyLog))); err != nil {
			return fmt.Errorf("finance.InsertPerDiem auditlog: %w", err)
		}
		return audit.Inserted("perdiem_allocation", "id", id).Record(ctx, tx, financeAuditEvent(audit.Create, "per_diem", in.CreatedBy))
	})
	return int(id), err
}

// DeletePerDiem ports updatePerDiemAllocation({id, status: "Deleted"}), the
// legacy soft delete, limited to an active per diem of jobNumber.
func (r *financeRepo) DeletePerDiem(ctx context.Context, jobNumber string, id int) (bool, error) {
	return r.trackedUpdate(ctx, "perdiem_allocation", `id = ? AND job_number = ? AND status = ?`,
		[]any{id, jobNumber, domain.PerDiemStatusActive}, financeAuditEvent(audit.Delete, "per_diem", ""),
		`UPDATE perdiem_allocation SET id = ?, status = ? WHERE id = ? AND job_number = ? AND status = ?`,
		id, domain.PerDiemStatusDeleted, id, jobNumber, domain.PerDiemStatusActive)
}

// trackedUpdate runs one UPDATE in a transaction with its audit events (rows
// of table matching where before the update) and reports whether exactly
// one row matched.
func (r *financeRepo) trackedUpdate(ctx context.Context, table, where string, whereArgs []any, e audit.Event, q string, args ...any) (bool, error) {
	var n int64
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, table, "id", where, whereArgs...)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		if n, err = res.RowsAffected(); err != nil {
			return err
		}
		return t.Record(ctx, tx, e)
	})
	return n == 1, err
}

// insertTracked runs one INSERT in a transaction with its Create event.
func (r *financeRepo) insertTracked(ctx context.Context, table string, e audit.Event, q string, args ...any) (int, error) {
	var id int64
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return audit.Inserted(table, "id", id).Record(ctx, tx, e)
	})
	return int(id), err
}

func financeAuditEvent(action audit.Action, entityType, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: entityType, ParentType: "travel_request", ParentColumn: "job_number", Actor: actor}
}

func financePushStep(e audit.Event, step string) audit.Event {
	e.Metadata = map[string]any{"netsuite": step}
	return e
}

func (r *financeRepo) AuditPush(ctx context.Context, e audit.Event) error {
	return audit.Record(ctx, r.db, e)
}

// PaymentBasis reads the inputs of the legacy per diem balance
// (getConvertedPerDiemPaymentSummary): per diem totals per currency (legacy
// "status = 'ACTIVE'", which matches the stored "Active" through the
// case-insensitive collation; ordered so the first added currency comes
// first, where legacy relied on the engine's order), Complete invoices of
// cost type Currency (getCurrencyCostTypeInvoiceAmount, the cost type id
// looked up by name in the same statement), travel cards and payments.
func (r *financeRepo) PaymentBasis(ctx context.Context, jobNumber string) (domain.FinancePaymentBasis, error) {
	b := domain.FinancePaymentBasis{
		PerDiemTotals: []domain.FinancePerDiemTotal{}, CurrencyInvoices: []domain.FinanceAmount{},
		TravelCards: []domain.FinanceAmount{}, Payments: []domain.FinanceAmount{},
	}
	rows, err := r.db.QueryContext(ctx, `SELECT SUM(total) as totalPerDiemAmount, currency as perDiemCurrency
		FROM perdiem_allocation
		WHERE job_number = ? AND status = 'ACTIVE'
		GROUP BY currency
		ORDER BY MIN(id)`, jobNumber)
	if err != nil {
		return b, fmt.Errorf("finance.PaymentBasis totals: %w", err)
	}
	for rows.Next() {
		var t domain.FinancePerDiemTotal
		var total sql.NullFloat64
		var cur sql.NullString
		if err := rows.Scan(&total, &cur); err != nil {
			rows.Close()
			return b, fmt.Errorf("finance.PaymentBasis totals scan: %w", err)
		}
		t.TotalAmount, t.Currency = total.Float64, cur.String
		b.PerDiemTotals = append(b.PerDiemTotals, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return b, err
	}

	amounts := func(q string, dst *[]domain.FinanceAmount, extra ...any) error {
		rows, err := r.db.QueryContext(ctx, q, append([]any{jobNumber}, extra...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var amt sql.NullFloat64
			var cur sql.NullString
			var at sql.NullTime
			if err := rows.Scan(&amt, &cur, &at); err != nil {
				return err
			}
			a := domain.FinanceAmount{Amount: amt.Float64, Currency: cur.String}
			if s := financeTime(at); s != nil {
				a.AddedDate = *s
			}
			*dst = append(*dst, a)
		}
		return rows.Err()
	}
	if err := amounts(`SELECT converted_amount as invoiceAmount, converted_currency as invoiceCurrency, added_date as addedDate
		FROM invoice
		WHERE job_number = ? AND status = 'Complete' AND COALESCE(reject_reason, '') <> ?
			AND inv_cost_type_id = (SELECT id FROM inv_cost_type WHERE status='active' AND name='Currency' LIMIT 1)`,
		&b.CurrencyInvoices, domain.InvoiceNSPushingMarker); err != nil {
		return b, fmt.Errorf("finance.PaymentBasis invoices: %w", err)
	}
	if err := amounts(`SELECT travel_card_amount, travel_card_currency, added_date FROM travel_card WHERE job_number = ?`, &b.TravelCards); err != nil {
		return b, fmt.Errorf("finance.PaymentBasis cards: %w", err)
	}
	if err := amounts(`SELECT payment_amount, payment_currency, added_date FROM perdiem_payment WHERE job_number = ?`, &b.Payments); err != nil {
		return b, fmt.Errorf("finance.PaymentBasis payments: %w", err)
	}
	b.PaymentCount = len(b.Payments)
	return b, nil
}

// PaymentCount ports getPerDiemPaymentCount.
func (r *financeRepo) PaymentCount(ctx context.Context, jobNumber string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) as numberOfPayments FROM perdiem_payment WHERE job_number = ?`, jobNumber).Scan(&n); err != nil {
		return 0, fmt.Errorf("finance.PaymentCount: %w", err)
	}
	return n, nil
}

// ── Travel cards ────────────────────────────────────────────────────────

// TravelCards ports getTravelCards({jobNumber}); the legacy default LIMIT 100
// is raised to the finance maximum, ordered by id.
func (r *financeRepo) TravelCards(ctx context.Context, jobNumber string) ([]domain.FinanceTravelCard, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, job_number AS jobNumber, travel_card_currency AS travelCardCurrency,
			travel_card_amount AS travelCardAmount, added_by, added_date AS addedDate
		FROM travel_card WHERE job_number = ? ORDER BY id LIMIT ?`, jobNumber, financeMaxRows)
	if err != nil {
		return nil, fmt.Errorf("finance.TravelCards: %w", err)
	}
	defer rows.Close()
	out := []domain.FinanceTravelCard{}
	for rows.Next() {
		var c domain.FinanceTravelCard
		var at sql.NullTime
		if err := rows.Scan(&c.ID, &c.JobNumber, &c.Currency, &c.Amount, &c.AddedBy, &at); err != nil {
			return nil, fmt.Errorf("finance.TravelCards scan: %w", err)
		}
		if s := financeTime(at); s != nil {
			c.AddedDate = *s
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertTravelCard ports insertTravelCards for one card.
func (r *financeRepo) InsertTravelCard(ctx context.Context, jobNumber string, in domain.FinanceTravelCardCreate) (int, error) {
	id, err := r.insertTracked(ctx, "travel_card", financeAuditEvent(audit.Create, "travel_card", in.CreatedBy),
		`INSERT INTO travel_card (job_number, travel_card_currency, travel_card_amount, added_by, added_date)
		VALUES (?, ?, ?, ?, NOW())`, jobNumber, textnorm.Latin1(in.Currency), in.Amount, in.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("finance.InsertTravelCard: %w", err)
	}
	return id, nil
}

// ── Cost types and classification ───────────────────────────────────────

// CostTypes ports getInvoiceCostTypes (BFF): active cost types.
func (r *financeRepo) CostTypes(ctx context.Context) ([]domain.FinanceCostType, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM inv_cost_type WHERE status='active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("finance.CostTypes: %w", err)
	}
	defer rows.Close()
	out := []domain.FinanceCostType{}
	for rows.Next() {
		var c domain.FinanceCostType
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, fmt.Errorf("finance.CostTypes scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CostTypeIDByName ports getInvoiceCostTypeIdByName; nil when none.
func (r *financeRepo) CostTypeIDByName(ctx context.Context, name string) (*int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM inv_cost_type WHERE status='active' AND name=? LIMIT 1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.CostTypeIDByName: %w", err)
	}
	return &id, nil
}

// ClassificationID ports getInvoiceAccountClassificationType (BFF): the
// first active classification whose engagement_code (or suffix) matches the
// LIKE pattern, for the cost type when given. The legacy query has no ORDER
// BY and takes the first row; ORDER BY id makes "first" stable. nil when no
// row matches.
func (r *financeRepo) ClassificationID(ctx context.Context, engagementCodeLike, suffixLike string, costTypeID *int) (*int, error) {
	q := `SELECT id FROM inv_acc_classification_type WHERE status='active'`
	var args []any
	switch {
	case engagementCodeLike != "" && suffixLike == "" && costTypeID != nil:
		q += ` AND engagement_code LIKE ? AND inv_cost_type_id=?`
		args = append(args, engagementCodeLike, *costTypeID)
	case engagementCodeLike == "" && suffixLike != "" && costTypeID != nil:
		q += ` AND engagement_code_suffix LIKE ? AND inv_cost_type_id=?`
		args = append(args, suffixLike, *costTypeID)
	case engagementCodeLike != "" && suffixLike == "" && costTypeID == nil:
		q += ` AND engagement_code LIKE ?`
		args = append(args, engagementCodeLike)
	}
	var id int
	err := r.db.QueryRowContext(ctx, q+` ORDER BY id LIMIT 1`, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.ClassificationID: %w", err)
	}
	return &id, nil
}

// ClassificationData ports getEmployeeDataByInvAccClassificationTypeId.
func (r *financeRepo) ClassificationData(ctx context.Context, id int) (*string, *string, error) {
	var dept, internal sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT department, internal_id AS internalId
		FROM inv_acc_classification_type
		WHERE status='active' AND id=?`, id).Scan(&dept, &internal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("finance.ClassificationData: %w", err)
	}
	return financeStr(dept), financeStr(internal), nil
}

// ── Invoices ────────────────────────────────────────────────────────────

// financeInvoiceFrom is the legacy getTravelInvoices SELECT and joins, plus
// the trip (travel) and the agency's id and name.
const financeInvoiceFrom = ` FROM invoice AS i
			LEFT JOIN inv_acc_classification_type AS iACT ON i.inv_acc_classification_type_id = iACT.id
			LEFT JOIN inv_cost_type AS iCT ON i.inv_cost_type_id = iCT.id
			LEFT JOIN travel_agent_employee AS tAE ON i.added_by = tAE.wso2_email
			LEFT JOIN travel_agents AS tA ON tAE.travel_agent_id = tA.id
			LEFT JOIN travel AS t ON i.job_number = t.job_number`

const financeInvoiceSelect = `SELECT i.id, i.invoice_number, i.month, i.inv_cost_type_id, i.inv_acc_classification_type_id,
			i.invoice_currency, i.invoice_amount, i.converted_currency, i.converted_amount, i.status,
			i.added_by, i.added_date, i.updated_by, i.updated_date, i.reject_reason, i.job_number,
			i.hr_approved_by, i.finance_approved_by,
			iACT.internal_id AS account_internal_id, iACT.department, iCT.name AS cost_type_name,
			tAE.wso2_email, tA.vendor_id, tA.id, tA.name_of_travel_agent,
			t.traveller_email, t.status, REPLACE(t.city, 'BR()', CHAR(10)), t.country, t.start_date, t.end_date, t.allocation_id` + financeInvoiceFrom

// financeFailedInvoice is true for an invoice on NS Sync Failures.
const financeFailedInvoice = `(i.status = 'Accepted by finance' AND COALESCE(i.reject_reason, '') LIKE 'NetSuite push failed:%')`

func scanFinanceInvoice(sc interface{ Scan(...any) error }) (domain.FinanceInvoice, error) {
	var (
		inv                                              domain.FinanceInvoice
		status, addedBy, updatedBy, reason, job, hr, fin sql.NullString
		internal, dept, costName, agent, agencyName      sql.NullString
		traveller, travelStatus, city, country, alloc    sql.NullString
		invAmt, convAmt                                  sql.NullString
		addedAt, updatedAt, start, end                   sql.NullTime
		vendor, agencyID                                 sql.NullInt64
	)
	if err := sc.Scan(&inv.ID, &inv.InvoiceNumber, &inv.Month, &inv.CostTypeID, &inv.AccountClassificationID,
		&inv.InvoiceCurrency, &invAmt, &inv.ConvertedCurrency, &convAmt, &status,
		&addedBy, &addedAt, &updatedBy, &updatedAt, &reason, &job,
		&hr, &fin, &internal, &dept, &costName,
		&agent, &vendor, &agencyID, &agencyName,
		&traveller, &travelStatus, &city, &country, &start, &end, &alloc); err != nil {
		return inv, err
	}
	inv.InvoiceAmountText, inv.InvoiceAmount = financeDecimal(invAmt)
	inv.ConvertedAmountText, inv.ConvertedAmount = financeDecimal(convAmt)
	inv.Status = status.String
	if !status.Valid {
		inv.Status = domain.InvoiceStatusPending // the column's default
	}
	inv.AddedBy, inv.AddedDate, inv.UpdatedBy, inv.UpdatedDate = financeStr(addedBy), financeTime(addedAt), financeStr(updatedBy), financeTime(updatedAt)
	inv.RejectReason, inv.JobNumber = financeStr(reason), job.String
	inv.HRApprovedBy, inv.FinanceApprovedBy = financeStr(hr), financeStr(fin)
	inv.AccountInternalID, inv.Department, inv.CostTypeName = financeStr(internal), financeStr(dept), financeStr(costName)
	inv.AgentEmail, inv.VendorID, inv.AgencyID, inv.AgencyName = financeStr(agent), financeInt(vendor), financeInt(agencyID), financeStr(agencyName)
	inv.TravellerEmail, inv.TravelStatus, inv.City, inv.Country = financeStr(traveller), financeStr(travelStatus), financeStr(city), financeStr(country)
	inv.TravelStartDate, inv.TravelEndDate, inv.AllocationID = financeTime(start), financeTime(end), financeStr(alloc)
	inv.SplitInvoiceRejectReason()
	return inv, nil
}

// financeInvoiceWhere builds the filters of the invoice list. The legacy
// filters are kept (job number, status, the invoice/job number search) and
// the Release 1 screen filters added.
func financeInvoiceWhere(f domain.FinanceInvoiceFilter) (string, []any) {
	where := []string{"1 = 1"}
	var args []any
	if f.JobNumber != "" {
		where = append(where, "i.job_number = ?")
		args = append(args, f.JobNumber)
	}
	financeIn("i.status", f.Statuses, &where, &args)
	switch f.Failed {
	case "true":
		where = append(where, financeFailedInvoice)
	case "false":
		where = append(where, "NOT "+financeFailedInvoice)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := LikeContains(s)
		where = append(where, `(i.invoice_number LIKE ? OR i.job_number LIKE ? OR iCT.name LIKE ? OR tA.name_of_travel_agent LIKE ?
			OR i.month LIKE ? OR t.traveller_email LIKE ?)`)
		args = append(args, like, like, like, like, like, like)
	}
	financeIn("iCT.name", f.CostTypes, &where, &args)
	financeIn("tA.name_of_travel_agent", f.Agencies, &where, &args)
	financeIn("i.month", f.Months, &where, &args)
	financeIn("t.allocation_id", f.AllocationIDs, &where, &args)
	if f.AgencyID > 0 {
		where = append(where, "tA.id = ?")
		args = append(args, f.AgencyID)
	}
	if f.AddedFrom != "" {
		where = append(where, "i.added_date >= ?")
		args = append(args, f.AddedFrom)
	}
	if f.AddedTo != "" {
		// Inclusive of the whole end day; the legacy BETWEEN 'from' AND
		// 'to' dropped the end day after 00:00:00.
		where = append(where, "i.added_date < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.AddedTo)
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// financeInvoiceSortColumns whitelists the invoice list sorts. There is no
// "amount": invoice_amount is in the invoice's own currency and
// converted_amount in the subsidiary's, so neither sorts meaningfully.
var financeInvoiceSortColumns = SortColumns{
	"invoiceNumber": {`i.invoice_number`},
	"jobNumber":     {`i.job_number`},
	"costType":      {`iCT.name`},
	"addedDate":     {`i.added_date`},
	"status":        {`i.status`},
}

// Invoices ports getTravelInvoices with the Release 1 filters, newest first
// (the legacy list had no ORDER BY), and the total of matches.
func (r *financeRepo) Invoices(ctx context.Context, f domain.FinanceInvoiceFilter) ([]domain.FinanceInvoice, int, error) {
	where, args := financeInvoiceWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+financeInvoiceFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("finance.Invoices count: %w", err)
	}
	limit := f.Limit
	if limit <= 0 || limit > financeMaxRows {
		limit = financeMaxRows
	}
	order, ok := financeInvoiceSortColumns.Order(f.Sort, `i.id`)
	if !ok {
		order = ` ORDER BY i.added_date DESC, i.invoice_number ASC, i.id DESC`
	}
	rows, err := r.db.QueryContext(ctx, financeInvoiceSelect+where+order+` LIMIT ? OFFSET ?`,
		append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, fmt.Errorf("finance.Invoices: %w", err)
	}
	defer rows.Close()
	out := []domain.FinanceInvoice{}
	for rows.Next() {
		inv, err := scanFinanceInvoice(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("finance.Invoices scan: %w", err)
		}
		out = append(out, inv)
	}
	return out, total, rows.Err()
}

// Invoice ports getTravelInvoices({id}) (legacy travelInvoice).
func (r *financeRepo) Invoice(ctx context.Context, id int) (*domain.FinanceInvoice, error) {
	inv, err := scanFinanceInvoice(r.db.QueryRowContext(ctx, financeInvoiceSelect+` WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.Invoice: %w", err)
	}
	return &inv, nil
}

// InvoiceCounts counts the invoice work queues: waiting for PO approval,
// for finance approval, to push (not failed), on NS Sync Failures, and all.
func (r *financeRepo) InvoiceCounts(ctx context.Context, agencyID int) (domain.FinanceInvoiceCounts, error) {
	q := `SELECT
			COALESCE(SUM(i.status = 'Pending'), 0),
			COALESCE(SUM(i.status = 'Accepted by travel'), 0),
			COALESCE(SUM(i.status = 'Accepted by finance' AND NOT ` + financeFailedInvoice + `), 0),
			COALESCE(SUM(` + financeFailedInvoice + `), 0),
			COUNT(*)` + financeInvoiceFrom
	var args []any
	if agencyID > 0 {
		q += ` WHERE tA.id = ?`
		args = append(args, agencyID)
	}
	var c domain.FinanceInvoiceCounts
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&c.Pending, &c.AcceptedByTravel, &c.AcceptedByFinance, &c.Failed, &c.All); err != nil {
		return c, fmt.Errorf("finance.InvoiceCounts: %w", err)
	}
	return c, nil
}

// InvoiceNumberExists ports the add-invoice duplicate check
// (travelInvoices(filter:{invoiceNumber}, limit 1)): any status. The
// column's collation makes it case-insensitive.
func (r *financeRepo) InvoiceNumberExists(ctx context.Context, invoiceNumber string) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice WHERE invoice_number = ?`, invoiceNumber).Scan(&n); err != nil {
		return false, fmt.Errorf("finance.InvoiceNumberExists: %w", err)
	}
	return n > 0, nil
}

// InsertInvoices ports insertTravelInvoice for each row (status Pending,
// added_date NOW()), in one transaction: the legacy committed row by row.
func (r *financeRepo) InsertInvoices(ctx context.Context, jobNumber, createdBy string, rows []FinanceInvoiceInsert) ([]int, error) {
	ids := make([]int, 0, len(rows))
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		for _, in := range rows {
			res, err := tx.ExecContext(ctx, `INSERT into invoice
				(invoice_number, month, inv_cost_type_id, inv_acc_classification_type_id, invoice_currency, invoice_amount,
				 converted_currency, converted_amount, status, added_by, added_date, job_number)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), ?)`,
				in.InvoiceNumber, textnorm.Latin1(in.Month), in.CostTypeID, in.ClassificationID,
				textnorm.Latin1(in.InvoiceCurrency), financeAmount(in.InvoiceAmount),
				textnorm.Latin1(in.ConvertedCurrency), financeAmount(in.ConvertedAmount),
				domain.InvoiceStatusPending, createdBy, jobNumber)
			if err != nil {
				return fmt.Errorf("finance.InsertInvoices: %w", err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("finance.InsertInvoices id: %w", err)
			}
			if err := audit.Inserted("invoice", "id", id).Record(ctx, tx, financeAuditEvent(audit.Create, "invoice", createdBy)); err != nil {
				return err
			}
			ids = append(ids, int(id))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// DecideInvoice ports updateTravelInvoice for an approve or reject: it sets
// the new status, the approver column, the reason of a rejection and
// updated_by/updated_date, but only while the invoice is still at d.From (the
// legacy read the status first and wrote unconditionally). False when the
// invoice is no longer at d.From.
func (r *financeRepo) DecideInvoice(ctx context.Context, id int, d FinanceInvoiceTransition) (bool, error) {
	if d.ApproverColumn != "hr_approved_by" && d.ApproverColumn != "finance_approved_by" {
		return false, fmt.Errorf("finance.DecideInvoice: bad approver column %q", d.ApproverColumn)
	}
	q := `UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = ?, ` + d.ApproverColumn + ` = ?`
	args := []any{d.Actor, d.To, d.Actor}
	if d.Reason != nil {
		q += `, reject_reason = ?`
		args = append(args, textnorm.Latin1(*d.Reason))
	}
	q += ` WHERE id = ? AND status = ?`
	args = append(args, id, d.From)
	ok, err := r.trackedUpdate(ctx, "invoice", `id = ? AND status = ?`, []any{id, d.From},
		financeAuditEvent(audit.StatusChange, "invoice", d.Actor), q, args...)
	if err != nil {
		return false, fmt.Errorf("finance.DecideInvoice: %w", err)
	}
	return ok, nil
}

// ClaimInvoicePush is the one-push-in-flight guard. Like the legacy push it
// sets status Complete before NetSuite is called, but only when the invoice
// is still Accepted by finance and not on NS Sync Failures: of two
// concurrent pushes only one matches the row.
func (r *financeRepo) ClaimInvoicePush(ctx context.Context, id int, actor string) (bool, error) {
	ok, err := r.trackedUpdate(ctx, "invoice", `id = ? AND status = 'Accepted by finance' AND COALESCE(reject_reason, '') = ''`,
		[]any{id}, financePushStep(financeAuditEvent(audit.StatusChange, "invoice", actor), "claim"),
		`UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = 'Complete', reject_reason = ?
		WHERE id = ? AND status = 'Accepted by finance' AND COALESCE(reject_reason, '') = ''`, actor, domain.InvoiceNSPushingMarker, id)
	if err != nil {
		return false, fmt.Errorf("finance.ClaimInvoicePush: %w", err)
	}
	return ok, nil
}

// ReleaseInvoicePush undoes a claim when nothing was sent to NetSuite.
func (r *financeRepo) ReleaseInvoicePush(ctx context.Context, id int, actor string) error {
	_, err := r.trackedUpdate(ctx, "invoice", `id = ? AND status = 'Complete' AND reject_reason = ?`, []any{id, domain.InvoiceNSPushingMarker},
		financePushStep(financeAuditEvent(audit.StatusChange, "invoice", actor), "release"),
		`UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = 'Accepted by finance', reject_reason = NULL
		WHERE id = ? AND status = 'Complete' AND reject_reason = ?`, actor, id, domain.InvoiceNSPushingMarker)
	if err != nil {
		return fmt.Errorf("finance.ReleaseInvoicePush: %w", err)
	}
	return nil
}

// FailInvoicePush is the legacy revert to Accepted by finance after a failed
// push, plus the NS Sync Failures marker with the error in reject_reason.
func (r *financeRepo) FailInvoicePush(ctx context.Context, id int, actor, msg string) error {
	reason := textnorm.Latin1(domain.InvoiceNSFailurePrefix + msg)
	reason = textnorm.Truncate(reason, domain.InvoiceMaxRejectReason)
	_, err := r.trackedUpdate(ctx, "invoice", `id = ? AND status = 'Complete'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "invoice", actor), "failed"),
		`UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = 'Accepted by finance', reject_reason = ?
		WHERE id = ? AND status = 'Complete'`, actor, reason, id)
	if err != nil {
		return fmt.Errorf("finance.FailInvoicePush: %w", err)
	}
	return nil
}

// CompleteInvoicePush stores the purchase order id of a claimed (Complete)
// invoice.
func (r *financeRepo) CompleteInvoicePush(ctx context.Context, id int, ref string) error {
	_, err := r.trackedUpdate(ctx, "invoice", `id = ? AND status = 'Complete'`, []any{id},
		financePushStep(financeAuditEvent(audit.Update, "invoice", ""), "reference"),
		`UPDATE invoice SET reject_reason = ? WHERE id = ? AND status = 'Complete'`,
		domain.InvoiceNSReferencePrefix+ref, id)
	if err != nil {
		return fmt.Errorf("finance.CompleteInvoicePush: %w", err)
	}
	return nil
}

// ResolveInvoiceFailure ends an NS Sync Failure after a lookup: found in
// NetSuite (ref, the purchase order id) → Complete with the id; not found
// (ref "") → back in the push queue (marker cleared). False when the invoice
// is no longer on NS Sync Failures.
func (r *financeRepo) ResolveInvoiceFailure(ctx context.Context, id int, actor, ref string) (bool, error) {
	status, reason := domain.InvoiceStatusAcceptedByFinance, sql.NullString{}
	if ref != "" {
		status, reason = domain.InvoiceStatusComplete, sql.NullString{String: domain.InvoiceNSReferencePrefix + ref, Valid: true}
	}
	where := `id = ? AND status = 'Accepted by finance' AND COALESCE(reject_reason, '') LIKE 'NetSuite push failed:%'`
	ok, err := r.trackedUpdate(ctx, "invoice", where, []any{id},
		financePushStep(financeAuditEvent(audit.Update, "invoice", actor), "resync"),
		`UPDATE invoice SET updated_by = ?, updated_date = NOW(), status = ?, reject_reason = ? WHERE `+where, actor, status, reason, id)
	if err != nil {
		return false, fmt.Errorf("finance.ResolveInvoiceFailure: %w", err)
	}
	return ok, nil
}

// ── Per diem payments ───────────────────────────────────────────────────

const financePaymentFrom = ` FROM perdiem_payment pp
		LEFT JOIN travel t ON pp.job_number = t.job_number`

const financePaymentSelect = `SELECT pp.id, pp.job_number AS jobNumber, pp.per_diem_number AS perDiemNumber,
			pp.payment_currency AS paymentCurrency, pp.payment_amount AS paymentAmount, pp.ns_status AS netsuiteStatus,
			pp.converted_currency AS convertedCurrency, pp.converted_amount AS convertedAmount,
			pp.added_by, pp.added_date AS addedDate, pp.last_updated_by, pp.last_updated_date,
			t.traveller_email AS travellerEmail, t.status AS travelStatus, t.allocation_id` + financePaymentFrom

func scanFinancePayment(sc interface{ Scan(...any) error }) (domain.FinancePayment, error) {
	var (
		p                                                domain.FinancePayment
		job, num, cur, status, convCur, convAmt, addedBy sql.NullString
		updBy, traveller, travelStatus, alloc            sql.NullString
		amt                                              sql.NullFloat64
		addedAt, updAt                                   sql.NullTime
	)
	if err := sc.Scan(&p.ID, &job, &num, &cur, &amt, &status, &convCur, &convAmt, &addedBy, &addedAt,
		&updBy, &updAt, &traveller, &travelStatus, &alloc); err != nil {
		return p, err
	}
	p.JobNumber, p.PerDiemNumber = job.String, num.String
	p.PaymentCurrency, p.PaymentAmount, p.NSStatus = financeStr(cur), financeFloat(amt), financeStr(status)
	p.ConvertedCurrency = financeStr(convCur)
	if convAmt.Valid {
		text, f := financeDecimal(convAmt)
		p.ConvertedAmountText, p.ConvertedAmount = &text, &f
	}
	p.AddedBy, p.AddedDate, p.LastUpdatedBy, p.LastUpdatedDate = financeStr(addedBy), financeTime(addedAt), financeStr(updBy), financeTime(updAt)
	p.TravellerEmail, p.TravelStatus, p.AllocationID = financeStr(traveller), financeStr(travelStatus), financeStr(alloc)
	return p, nil
}

func financePaymentWhere(f domain.FinancePaymentFilter) (string, []any) {
	where := []string{"1 = 1"}
	var args []any
	if f.JobNumber != "" {
		where = append(where, "pp.job_number = ?")
		args = append(args, f.JobNumber)
	}
	financeIn("pp.ns_status", f.NSStatuses, &where, &args)
	financeIn("t.traveller_email", f.Travellers, &where, &args)
	financeIn("pp.converted_currency", f.ConvertedCurrency, &where, &args)
	financeIn("t.allocation_id", f.AllocationIDs, &where, &args)
	if s := strings.TrimSpace(f.Search); s != "" {
		like := LikeContains(s)
		where = append(where, `(pp.per_diem_number LIKE ? OR pp.job_number LIKE ? OR t.traveller_email LIKE ?
			OR pp.payment_currency LIKE ? OR pp.converted_currency LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}
	if f.AddedFrom != "" {
		where = append(where, "pp.added_date >= ?")
		args = append(args, f.AddedFrom)
	}
	if f.AddedTo != "" {
		where = append(where, "pp.added_date < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.AddedTo)
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// financePaymentSortColumns whitelists the per diem payment list sorts; no
// "amount" for the reason given at financeInvoiceSortColumns. "status" is the
// NetSuite status, the only status the table stores.
var financePaymentSortColumns = SortColumns{
	"number":    {`pp.per_diem_number`},
	"jobNumber": {`pp.job_number`},
	"addedDate": {`pp.added_date`},
	"status":    {`pp.ns_status`},
}

// Payments ports getPerDiemPaymentDetails with the Release 1 filters,
// newest first, and the total of matches.
func (r *financeRepo) Payments(ctx context.Context, f domain.FinancePaymentFilter) ([]domain.FinancePayment, int, error) {
	where, args := financePaymentWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+financePaymentFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("finance.Payments count: %w", err)
	}
	limit := f.Limit
	if limit <= 0 || limit > financeMaxRows {
		limit = financeMaxRows
	}
	order, ok := financePaymentSortColumns.Order(f.Sort, `pp.id`)
	if !ok {
		order = ` ORDER BY pp.added_date DESC, pp.per_diem_number ASC, pp.id DESC`
	}
	rows, err := r.db.QueryContext(ctx, financePaymentSelect+where+order+` LIMIT ? OFFSET ?`,
		append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, fmt.Errorf("finance.Payments: %w", err)
	}
	defer rows.Close()
	out := []domain.FinancePayment{}
	for rows.Next() {
		p, err := scanFinancePayment(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("finance.Payments scan: %w", err)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// Payment reads one payment by its per diem number (the first, by id: the
// column has no unique key in staging).
func (r *financeRepo) Payment(ctx context.Context, perDiemNumber string) (*domain.FinancePayment, error) {
	p, err := scanFinancePayment(r.db.QueryRowContext(ctx, financePaymentSelect+` WHERE pp.per_diem_number = ? ORDER BY pp.id LIMIT 1`, perDiemNumber))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finance.Payment: %w", err)
	}
	return &p, nil
}

// PaymentCounts counts the payment work queues.
func (r *financeRepo) PaymentCounts(ctx context.Context) (domain.FinancePaymentCounts, error) {
	var c domain.FinancePaymentCounts
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(ns_status = 'Pending'), 0), COALESCE(SUM(ns_status = 'Failed'), 0), COUNT(*)
		FROM perdiem_payment`).Scan(&c.Pending, &c.Failed, &c.All)
	if err != nil {
		return c, fmt.Errorf("finance.PaymentCounts: %w", err)
	}
	return c, nil
}

// PaymentNumberExists ports the add-payment duplicate check (payments search
// {perDiemNumber}, limit 1).
func (r *financeRepo) PaymentNumberExists(ctx context.Context, perDiemNumber string) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM perdiem_payment WHERE per_diem_number = ?`, perDiemNumber).Scan(&n); err != nil {
		return false, fmt.Errorf("finance.PaymentNumberExists: %w", err)
	}
	return n > 0, nil
}

// InsertPayment ports insertPerDiemPayment (ns_status Pending).
func (r *financeRepo) InsertPayment(ctx context.Context, jobNumber string, in domain.FinancePaymentCreate) (int, error) {
	id, err := r.insertTracked(ctx, "perdiem_payment", financeAuditEvent(audit.Create, "per_diem_payment", in.CreatedBy),
		`INSERT into perdiem_payment
		(job_number, per_diem_number, payment_currency, payment_amount, ns_status, converted_currency, converted_amount, added_by, added_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
		jobNumber, in.PerDiemNumber, textnorm.Latin1(in.PaymentCurrency), in.PaymentAmount, domain.PaymentNSPending,
		textnorm.Latin1(in.ConvertedCurrency), financeAmount(in.ConvertedAmount), in.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("finance.InsertPayment: %w", err)
	}
	return id, nil
}

// UpdatePayment ports updatePerDiemPayment for an edit (amount, its
// conversion, last_updated_by/date), only while the payment is Pending (the
// legacy read the status first and wrote unconditionally).
func (r *financeRepo) UpdatePayment(ctx context.Context, id int, in domain.FinancePaymentUpdate) (bool, error) {
	ok, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Pending'`, []any{id},
		financeAuditEvent(audit.Update, "per_diem_payment", in.UpdatedBy),
		`UPDATE perdiem_payment SET id = ?, payment_amount = ?, converted_currency = ?, converted_amount = ?,
			last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Pending'`,
		id, in.PaymentAmount, textnorm.Latin1(in.ConvertedCurrency), financeAmount(in.ConvertedAmount), in.UpdatedBy, id)
	if err != nil {
		return false, fmt.Errorf("finance.UpdatePayment: %w", err)
	}
	return ok, nil
}

// ClaimPaymentPush is the one-push-in-flight guard: Complete before the
// NetSuite call, as the legacy did, only from Pending.
func (r *financeRepo) ClaimPaymentPush(ctx context.Context, id int, actor string) (bool, error) {
	ok, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Pending'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", actor), "claim"),
		`UPDATE perdiem_payment SET ns_status = 'Pushing', last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Pending'`, actor, id)
	if err != nil {
		return false, fmt.Errorf("finance.ClaimPaymentPush: %w", err)
	}
	return ok, nil
}

// ReleasePaymentPush undoes a claim when nothing was sent to NetSuite.
func (r *financeRepo) ReleasePaymentPush(ctx context.Context, id int, actor string) error {
	_, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Pushing'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", actor), "release"),
		`UPDATE perdiem_payment SET ns_status = 'Pending', last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Pushing'`, actor, id)
	if err != nil {
		return fmt.Errorf("finance.ReleasePaymentPush: %w", err)
	}
	return nil
}

// FailPaymentPush marks a push that returned no vendor bill id (NS Sync
// Failures). The legacy reverted to Pending.
func (r *financeRepo) FailPaymentPush(ctx context.Context, id int, actor string) error {
	_, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Pushing'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", actor), "failed"),
		`UPDATE perdiem_payment SET ns_status = 'Failed', last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Pushing'`, actor, id)
	if err != nil {
		return fmt.Errorf("finance.FailPaymentPush: %w", err)
	}
	return nil
}

// CompletePaymentPush ends a push that NetSuite has: Pushing → Complete.
func (r *financeRepo) CompletePaymentPush(ctx context.Context, id int, actor string) error {
	_, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Pushing'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", actor), "complete"),
		`UPDATE perdiem_payment SET ns_status = 'Complete', last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Pushing'`, actor, id)
	if err != nil {
		return fmt.Errorf("finance.CompletePaymentPush: %w", err)
	}
	return nil
}

// ExpireStalePushes moves pushes still in progress after maxAge (the
// process stopped mid-push) to NS Sync Failures, where a re-sync settles
// them by a lookup.
func ExpireStalePushes(ctx context.Context, db *sql.DB, maxAge time.Duration) error {
	r := &financeRepo{db: db}
	secs := int(maxAge.Seconds())
	reason := domain.InvoiceNSFailurePrefix + "the push was interrupted. Re-sync to check NetSuite."
	inv := `status = 'Complete' AND reject_reason = ? AND updated_date < NOW() - INTERVAL ? SECOND`
	if _, err := r.trackedUpdate(ctx, "invoice", inv, []any{domain.InvoiceNSPushingMarker, secs},
		financePushStep(financeAuditEvent(audit.StatusChange, "invoice", ""), "expired"),
		`UPDATE invoice SET updated_date = NOW(), status = 'Accepted by finance', reject_reason = ? WHERE `+inv,
		reason, domain.InvoiceNSPushingMarker, secs); err != nil {
		return fmt.Errorf("finance.ExpireStalePushes invoices: %w", err)
	}
	pay := `ns_status = 'Pushing' AND last_updated_date < NOW() - INTERVAL ? SECOND`
	if _, err := r.trackedUpdate(ctx, "perdiem_payment", pay, []any{secs},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", ""), "expired"),
		`UPDATE perdiem_payment SET ns_status = 'Failed', last_updated_date = NOW() WHERE `+pay, secs); err != nil {
		return fmt.Errorf("finance.ExpireStalePushes payments: %w", err)
	}
	return nil
}

// ResolvePaymentFailure ends an NS Sync Failure after a lookup: Complete when
// NetSuite has the vendor bill, else back to Pending.
func (r *financeRepo) ResolvePaymentFailure(ctx context.Context, id int, actor string, found bool) (bool, error) {
	status := domain.PaymentNSPending
	if found {
		status = domain.PaymentNSComplete
	}
	ok, err := r.trackedUpdate(ctx, "perdiem_payment", `id = ? AND ns_status = 'Failed'`, []any{id},
		financePushStep(financeAuditEvent(audit.StatusChange, "per_diem_payment", actor), "resync"),
		`UPDATE perdiem_payment SET ns_status = ?, last_updated_by = ?, last_updated_date = NOW()
		WHERE id = ? AND ns_status = 'Failed'`, status, actor, id)
	if err != nil {
		return false, fmt.Errorf("finance.ResolvePaymentFailure: %w", err)
	}
	return ok, nil
}
