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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

var (
	// ErrTravelRecordNotFound: no visa or travel document with that id.
	ErrTravelRecordNotFound = errors.New("travel record not found")
	// ErrTravelDuplicateActive: another Active visa (same email and country)
	// or travel document (same email, type and country) exists.
	ErrTravelDuplicateActive = errors.New("another active travel record exists")
)

// TravelDocumentRepository reads and writes travelapp.visa_status and
// travelapp.employee_travel_document.
type TravelDocumentRepository interface {
	SearchVisas(ctx context.Context, f domain.TravelVisaFilter) ([]domain.TravelVisa, int, error)
	GetVisa(ctx context.Context, id int) (*domain.TravelVisa, error)
	// CreateVisa sets the Active visa of the same email and country Inactive
	// and inserts the new one, in one transaction. It returns the new id.
	CreateVisa(ctx context.Context, in domain.CreateTravelVisaInput) (int, error)
	// UpdateVisa fails with ErrTravelRecordNotFound or ErrTravelDuplicateActive.
	UpdateVisa(ctx context.Context, id int, in domain.UpdateTravelVisaInput) error

	SearchDocuments(ctx context.Context, f domain.TravelDocumentFilter) ([]domain.TravelDocument, int, error)
	GetDocument(ctx context.Context, id int) (*domain.TravelDocument, error)
	// CreateDocument sets the Active document of the same email, type and
	// country Inactive and inserts the new one, in one transaction.
	CreateDocument(ctx context.Context, in domain.CreateTravelDocumentInput) (int, error)
	// UpdateDocument fails with ErrTravelRecordNotFound or ErrTravelDuplicateActive.
	UpdateDocument(ctx context.Context, id int, in domain.UpdateTravelDocumentInput) error
}

type travelDocumentRepo struct{ db *sql.DB }

// NewTravelDocumentRepository constructs a TravelDocumentRepository. db must
// be the travelapp pool.
func NewTravelDocumentRepository(db *sql.DB) TravelDocumentRepository {
	return &travelDocumentRepo{db: db}
}

// ── Visas ─────────────────────────────────────────────────────────────────

const travelVisaSelect = `SELECT v.id, v.email, v.country, v.visa_type, v.entry_type, v.start_date, v.end_date,
		v.issued_date, v.status, v.image_id, v.passport_id, p.issued_country, p.document_no, p.issued_date,
		p.expiry_date, v.added_by, v.updated_by
	FROM visa_status AS v
	LEFT JOIN employee_travel_document AS p ON p.id = v.passport_id`

// travelVisaWhere ports the filters of the legacy getVisaDetails that the
// screens use; the free-text search ORs its columns in one group.
func travelVisaWhere(f domain.TravelVisaFilter) (string, []any) {
	var conds []string
	var args []any
	in := func(col string, vals []string) {
		if len(vals) > 0 {
			conds = append(conds, col+` IN (`+Placeholders(len(vals))+`)`)
			args = append(args, Args(vals)...)
		}
	}
	in("v.email", f.Emails)
	in("v.country", f.Countries)
	if len(f.ExcludeIDs) > 0 {
		conds = append(conds, `v.id NOT IN (`+Placeholders(len(f.ExcludeIDs))+`)`)
		args = append(args, Args(f.ExcludeIDs)...)
	}
	if f.Status != "" {
		conds = append(conds, `v.status = ?`)
		args = append(args, f.Status)
	}
	switch f.Validity {
	case domain.TravelValidityActive:
		conds = append(conds, `DATE(v.end_date) >= ?`)
		args = append(args, f.Today)
	case domain.TravelValidityExpired:
		conds = append(conds, `DATE(v.end_date) < ?`)
		args = append(args, f.Today)
	}
	if f.CoversFrom != "" {
		conds = append(conds, `DATE(v.start_date) <= ?`)
		args = append(args, f.CoversFrom)
	}
	if f.CoversTo != "" {
		conds = append(conds, `DATE(v.end_date) >= ?`)
		args = append(args, f.CoversTo)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := LikeContains(s)
		conds = append(conds, `(v.email LIKE ? OR v.country LIKE ? OR v.visa_type LIKE ? OR v.entry_type LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return ` WHERE ` + strings.Join(conds, ` AND `), args
}

func (r *travelDocumentRepo) SearchVisas(ctx context.Context, f domain.TravelVisaFilter) ([]domain.TravelVisa, int, error) {
	where, args := travelVisaWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM visa_status AS v`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("visaStatus.Count: %w", err)
	}
	if total == 0 {
		return []domain.TravelVisa{}, 0, nil
	}
	rows, err := r.db.QueryContext(ctx, travelVisaSelect+where+` ORDER BY v.end_date ASC, v.id LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("visaStatus.Search: %w", err)
	}
	defer rows.Close()
	out := []domain.TravelVisa{}
	for rows.Next() {
		v, err := scanTravelVisa(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("visaStatus.Search scan: %w", err)
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

func (r *travelDocumentRepo) GetVisa(ctx context.Context, id int) (*domain.TravelVisa, error) {
	v, err := scanTravelVisa(r.db.QueryRowContext(ctx, travelVisaSelect+` WHERE v.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("visaStatus.Get: %w", err)
	}
	return &v, nil
}

// CreateVisa ports the legacy insertVisa: the Active visa of the same email
// and country is set Inactive, then the visa is inserted Active. The legacy
// code made the two calls without a transaction and inactivated only the
// first match; here both run in one transaction and every match is set
// Inactive.
func (r *travelDocumentRepo) CreateVisa(ctx context.Context, in domain.CreateTravelVisaInput) (int, error) {
	var id int
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		email, country, by := textnorm.Latin1(in.Email), textnorm.Country(in.Country), textnorm.Latin1(in.CreatedBy)
		t, err := audit.Track(ctx, tx, "visa_status", "id", `email = ? AND country = ? AND status = 'Active'`, email, country)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE visa_status SET status = 'Inactive', updated_by = ?, updated_date = NOW()
			WHERE email = ? AND country = ? AND status = 'Active'`, by, email, country); err != nil {
			return fmt.Errorf("visaStatus.Inactivate: %w", err)
		}
		if err := t.Record(ctx, tx, visaAuditEvent(audit.StatusChange, by)); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO visa_status
			(email, country, visa_type, start_date, end_date, added_by, added_date, status, entry_type,
			 issued_date, image_id, passport_id)
			VALUES (?, ?, ?, ?, ?, ?, NOW(), 'Active', ?, ?, ?, ?)`,
			email, country, textnorm.Latin1(in.VisaType), in.StartDate, in.EndDate, by,
			textnorm.Latin1(in.EntryType), in.IssuedDate, textnorm.Latin1(in.ImageID), in.PassportID)
		if err != nil {
			return fmt.Errorf("visaStatus.Insert: %w", err)
		}
		last, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("visaStatus.Insert id: %w", err)
		}
		id = int(last)
		return audit.Inserted("visa_status", "id", id).Record(ctx, tx, visaAuditEvent(audit.Create, by))
	})
	return id, err
}

// UpdateVisa ports the legacy update-visa: another Active visa of the same
// email and the (new) country refuses the edit; otherwise updated_by,
// updated_date and the fields are written (passport and image only when
// given).
func (r *travelDocumentRepo) UpdateVisa(ctx context.Context, id int, in domain.UpdateTravelVisaInput) error {
	return WithTx(ctx, r.db, func(tx *sql.Tx) error {
		var email sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT email FROM visa_status WHERE id = ?`, id).Scan(&email)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTravelRecordNotFound
		}
		if err != nil {
			return fmt.Errorf("visaStatus.Get: %w", err)
		}
		country := textnorm.Country(in.Country)
		var other int
		err = tx.QueryRowContext(ctx, `SELECT id FROM visa_status
			WHERE email = ? AND country = ? AND status = 'Active' AND id <> ? LIMIT 1`,
			email.String, country, id).Scan(&other)
		if err == nil {
			return ErrTravelDuplicateActive
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("visaStatus.Duplicate: %w", err)
		}
		t, err := audit.Track(ctx, tx, "visa_status", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		q := `UPDATE visa_status SET updated_by = ?, updated_date = NOW(), country = ?, visa_type = ?,
			start_date = ?, end_date = ?, status = ?, entry_type = ?, issued_date = ?`
		args := []any{textnorm.Latin1(in.UpdatedBy), country, textnorm.Latin1(in.VisaType),
			in.StartDate, in.EndDate, in.Status, textnorm.Latin1(in.EntryType), in.IssuedDate}
		if in.ImageID != nil {
			q += `, image_id = ?`
			args = append(args, textnorm.Latin1(*in.ImageID))
		}
		if in.PassportID != nil {
			q += `, passport_id = ?`
			args = append(args, *in.PassportID)
		}
		if _, err := tx.ExecContext(ctx, q+` WHERE id = ?`, append(args, id)...); err != nil {
			return fmt.Errorf("visaStatus.Update: %w", err)
		}
		return t.Record(ctx, tx, visaAuditEvent(audit.Update, in.UpdatedBy))
	})
}

func scanTravelVisa(s travelScanner) (domain.TravelVisa, error) {
	var (
		v                             domain.TravelVisa
		start, end, issued            sql.NullTime
		passportID                    sql.NullInt64
		pCountry, pNumber             sql.NullString
		pIssued, pExpiry              sql.NullTime
		email, country, vType, eType  sql.NullString
		status, imageID, added, updBy sql.NullString
	)
	if err := s.Scan(&v.ID, &email, &country, &vType, &eType, &start, &end, &issued, &status, &imageID,
		&passportID, &pCountry, &pNumber, &pIssued, &pExpiry, &added, &updBy); err != nil {
		return domain.TravelVisa{}, err
	}
	v.Email, v.Country, v.VisaType, v.EntryType = travelNullString(email), travelNullString(country),
		travelNullString(vType), travelNullString(eType)
	v.Status, v.ImageID, v.AddedBy, v.UpdatedBy = travelNullString(status), travelNullString(imageID),
		travelNullString(added), travelNullString(updBy)
	v.StartDate, v.EndDate, v.IssuedDate = travelDate(start), travelDate(end), travelDate(issued)
	if passportID.Valid {
		id := int(passportID.Int64)
		v.PassportID = &id
		if pNumber.Valid || pCountry.Valid {
			v.Passport = &domain.TravelPassportRef{
				ID: id, Country: travelNullString(pCountry), DocumentNo: travelNullString(pNumber),
				IssuedDate: travelDate(pIssued), ExpiryDate: travelDate(pExpiry),
			}
		}
	}
	return v, nil
}

// ── Travel documents ──────────────────────────────────────────────────────

const travelDocumentSelect = `SELECT d.id, d.email, d.document_no, d.document_type, d.issued_country, d.issued_date,
		d.expiry_date, d.status, d.image_id, d.added_by, d.updated_by
	FROM employee_travel_document AS d`

func travelDocumentWhere(f domain.TravelDocumentFilter) (string, []any) {
	var conds []string
	var args []any
	in := func(col string, vals []string) {
		if len(vals) > 0 {
			conds = append(conds, col+` IN (`+Placeholders(len(vals))+`)`)
			args = append(args, Args(vals)...)
		}
	}
	in("d.email", f.Emails)
	in("d.issued_country", f.Countries)
	in("d.document_type", f.DocumentTypes)
	if f.Status != "" {
		conds = append(conds, `d.status = ?`)
		args = append(args, f.Status)
	}
	switch f.Validity {
	case domain.TravelValidityActive:
		conds = append(conds, `(d.expiry_date IS NULL OR DATE(d.expiry_date) >= ?)`)
		args = append(args, f.Today)
	case domain.TravelValidityExpired:
		conds = append(conds, `DATE(d.expiry_date) < ?`)
		args = append(args, f.Today)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := LikeContains(s)
		conds = append(conds, `(d.email LIKE ? OR d.document_no LIKE ? OR d.document_type LIKE ? OR d.issued_country LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return ` WHERE ` + strings.Join(conds, ` AND `), args
}

func (r *travelDocumentRepo) SearchDocuments(ctx context.Context, f domain.TravelDocumentFilter) ([]domain.TravelDocument, int, error) {
	where, args := travelDocumentWhere(f)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM employee_travel_document AS d`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("travelDocument.Count: %w", err)
	}
	if total == 0 {
		return []domain.TravelDocument{}, 0, nil
	}
	rows, err := r.db.QueryContext(ctx,
		travelDocumentSelect+where+` ORDER BY d.expiry_date IS NULL, d.expiry_date ASC, d.id LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("travelDocument.Search: %w", err)
	}
	defer rows.Close()
	out := []domain.TravelDocument{}
	for rows.Next() {
		d, err := scanTravelDocument(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("travelDocument.Search scan: %w", err)
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (r *travelDocumentRepo) GetDocument(ctx context.Context, id int) (*domain.TravelDocument, error) {
	d, err := scanTravelDocument(r.db.QueryRowContext(ctx, travelDocumentSelect+` WHERE d.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("travelDocument.Get: %w", err)
	}
	return &d, nil
}

// CreateDocument ports the legacy insertTravelDocument (override rule as in
// CreateVisa, by email, document type and issued country).
func (r *travelDocumentRepo) CreateDocument(ctx context.Context, in domain.CreateTravelDocumentInput) (int, error) {
	var id int
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		email, docType := textnorm.Latin1(in.Email), textnorm.Latin1(in.DocumentType)
		country, by := textnorm.Country(in.IssuedCountry), textnorm.Latin1(in.CreatedBy)
		t, err := audit.Track(ctx, tx, "employee_travel_document", "id",
			`email = ? AND document_type = ? AND issued_country = ? AND status = 'Active'`, email, docType, country)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE employee_travel_document
			SET status = 'Inactive', updated_by = ?, updated_date = NOW()
			WHERE email = ? AND document_type = ? AND issued_country = ? AND status = 'Active'`,
			by, email, docType, country); err != nil {
			return fmt.Errorf("travelDocument.Inactivate: %w", err)
		}
		if err := t.Record(ctx, tx, documentAuditEvent(audit.StatusChange, by)); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO employee_travel_document
			(email, document_no, document_type, issued_country, issued_date, expiry_date, added_by, added_date,
			 image_id, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), ?, 'Active')`,
			email, textnorm.Latin1(in.DocumentNo), docType, country, in.IssuedDate, in.ExpiryDate, by,
			textnorm.Latin1(in.ImageID))
		if err != nil {
			return fmt.Errorf("travelDocument.Insert: %w", err)
		}
		last, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("travelDocument.Insert id: %w", err)
		}
		id = int(last)
		return audit.Inserted("employee_travel_document", "id", id).Record(ctx, tx, documentAuditEvent(audit.Create, by))
	})
	return id, err
}

// UpdateDocument ports the legacy update-travel-document: another Active
// document of the same email, type and country refuses the edit (legacy
// 409); document number, expiry date and image are written only when given.
func (r *travelDocumentRepo) UpdateDocument(ctx context.Context, id int, in domain.UpdateTravelDocumentInput) error {
	return WithTx(ctx, r.db, func(tx *sql.Tx) error {
		var email sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT email FROM employee_travel_document WHERE id = ?`, id).Scan(&email)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTravelRecordNotFound
		}
		if err != nil {
			return fmt.Errorf("travelDocument.Get: %w", err)
		}
		docType, country := textnorm.Latin1(in.DocumentType), textnorm.Country(in.IssuedCountry)
		var other int
		err = tx.QueryRowContext(ctx, `SELECT id FROM employee_travel_document
			WHERE email = ? AND document_type = ? AND issued_country = ? AND status = 'Active' AND id <> ? LIMIT 1`,
			email.String, docType, country, id).Scan(&other)
		if err == nil {
			return ErrTravelDuplicateActive
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("travelDocument.Duplicate: %w", err)
		}
		t, err := audit.Track(ctx, tx, "employee_travel_document", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		q := `UPDATE employee_travel_document SET updated_by = ?, updated_date = NOW(), document_type = ?,
			issued_country = ?, issued_date = ?, status = ?`
		args := []any{textnorm.Latin1(in.UpdatedBy), docType, country, in.IssuedDate, in.Status}
		if in.DocumentNo != nil {
			q += `, document_no = ?`
			args = append(args, textnorm.Latin1(*in.DocumentNo))
		}
		if in.ExpiryDate != nil {
			q += `, expiry_date = ?`
			args = append(args, *in.ExpiryDate)
		}
		if in.ImageID != nil {
			q += `, image_id = ?`
			args = append(args, textnorm.Latin1(*in.ImageID))
		}
		if _, err := tx.ExecContext(ctx, q+` WHERE id = ?`, append(args, id)...); err != nil {
			return fmt.Errorf("travelDocument.Update: %w", err)
		}
		return t.Record(ctx, tx, documentAuditEvent(audit.Update, in.UpdatedBy))
	})
}

func visaAuditEvent(action audit.Action, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: "visa", ParentType: "employee", ParentColumn: "email", Actor: actor}
}

func documentAuditEvent(action audit.Action, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: "travel_document", ParentType: "employee", ParentColumn: "email", Actor: actor}
}

func scanTravelDocument(s travelScanner) (domain.TravelDocument, error) {
	var (
		d                                   domain.TravelDocument
		issued, expiry                      sql.NullTime
		email, number, docType, country     sql.NullString
		status, imageID, addedBy, updatedBy sql.NullString
	)
	if err := s.Scan(&d.ID, &email, &number, &docType, &country, &issued, &expiry, &status, &imageID,
		&addedBy, &updatedBy); err != nil {
		return domain.TravelDocument{}, err
	}
	d.Email, d.DocumentNo, d.DocumentType, d.IssuedCountry = travelNullString(email), travelNullString(number),
		travelNullString(docType), travelNullString(country)
	d.Status = status.String
	d.ImageID, d.AddedBy, d.UpdatedBy = travelNullString(imageID), travelNullString(addedBy), travelNullString(updatedBy)
	d.IssuedDate, d.ExpiryDate = travelDate(issued), travelDate(expiry)
	return d, nil
}
