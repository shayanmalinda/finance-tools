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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ErrSettingsAllocationTypeNotFound is returned when no allocation type has
// the id (or it is already deleted, for update and delete).
var ErrSettingsAllocationTypeNotFound = errors.New("allocation type not found")

// SettingsRepository writes allocation_db.allocation_type for the Allocation
// Types settings screen. Every write adds the legacy audit_log row.
type SettingsRepository interface {
	AllocationType(ctx context.Context, id int) (domain.AllocationType, error)
	EngagementTypeExists(ctx context.Context, id int) (bool, error)
	// AllocationTypeNameTaken reports whether the engagement type already
	// has a non-deleted allocation type with the name (other than exceptID).
	AllocationTypeNameTaken(ctx context.Context, engagementTypeID int, name string, exceptID int) (bool, error)
	CreateAllocationType(ctx context.Context, in domain.SettingsAllocationTypeInput) (int, error)
	UpdateAllocationType(ctx context.Context, id int, in domain.SettingsAllocationTypeInput) error
	DeleteAllocationType(ctx context.Context, id int, actor string) error
}

type settingsRepo struct{ db *sql.DB }

// NewSettingsRepository constructs a SettingsRepository. db must be the
// allocation_db pool.
func NewSettingsRepository(db *sql.DB) SettingsRepository { return &settingsRepo{db: db} }

func (r *settingsRepo) AllocationType(ctx context.Context, id int) (domain.AllocationType, error) {
	var at domain.AllocationType
	err := r.db.QueryRowContext(ctx, `SELECT aT.id, aT.allocation_type, aT.status, aT.color, aT.department,
			aT.emailToAllocations, aT.default_engagement_code, aT.engagement_code_suffix, eT.id, eT.name
		FROM allocation_type AS aT
		INNER JOIN engagement_type AS eT ON aT.engagement_type = eT.id
		WHERE aT.id = ?`, id).Scan(&at.ID, &at.AllocationTypeName, &at.Status, &at.Color, &at.Department,
		&at.EmailToAllocations, &at.DefaultEngagementCode, &at.EngagementCodeSuffix, &at.EngagementTypeID, &at.EngagementTypeName)
	if errors.Is(err, sql.ErrNoRows) {
		return at, ErrSettingsAllocationTypeNotFound
	}
	if err != nil {
		return at, fmt.Errorf("allocationType.Get: %w", err)
	}
	return at, nil
}

func (r *settingsRepo) EngagementTypeExists(ctx context.Context, id int) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM engagement_type WHERE id = ?`, id).Scan(&n); err != nil {
		return false, fmt.Errorf("engagementType.Exists: %w", err)
	}
	return n > 0, nil
}

func (r *settingsRepo) AllocationTypeNameTaken(ctx context.Context, engagementTypeID int, name string, exceptID int) (bool, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM allocation_type
		WHERE engagement_type = ? AND allocation_type = ? AND status != 'Deleted' AND id != ?`,
		engagementTypeID, name, exceptID).Scan(&n); err != nil {
		return false, fmt.Errorf("allocationType.NameTaken: %w", err)
	}
	return n > 0, nil
}

func settingsNullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// CreateAllocationType ports addNewAllocationTypeQuery (status 'ACTIVE') and
// its audit row.
func (r *settingsRepo) CreateAllocationType(ctx context.Context, in domain.SettingsAllocationTypeInput) (int, error) {
	var id int64
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO allocation_type (engagement_type, department, allocation_type, color,
				status, default_engagement_code, engagement_code_suffix, emailToAllocations)
			VALUES (?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`,
			in.EngagementTypeID, settingsNullable(in.Department), in.AllocationTypeName, settingsNullable(in.Color),
			in.DefaultEngagementCode, settingsNullable(in.EngagementCodeSuffix), settingsNullable(in.EmailToAllocations))
		if err != nil {
			return fmt.Errorf("allocationType.Insert: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return fmt.Errorf("allocationType.Insert id: %w", err)
		}
		return audit.Inserted("allocation_type", "id", id).Record(ctx, tx, settingsAuditEvent(audit.Create, in.Actor))
	})
	if err != nil {
		return 0, err
	}
	SettingsAuditLog(ctx, r.db, "ALLOCATION_TYPE", "INSERT",
		fmt.Sprintf("A new allocation type is added as %s by %s", in.AllocationTypeName, in.Actor), in.Actor)
	return int(id), nil
}

// UpdateAllocationType ports updateAllocationTypeQuery and its audit row.
func (r *settingsRepo) UpdateAllocationType(ctx context.Context, id int, in domain.SettingsAllocationTypeInput) error {
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "allocation_type", "id", `id = ? AND status != 'Deleted'`, id)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE allocation_type SET engagement_type = ?, department = ?, allocation_type = ?,
				color = ?, default_engagement_code = ?, engagement_code_suffix = ?, emailToAllocations = ?
			WHERE id = ? AND status != 'Deleted'`,
			in.EngagementTypeID, settingsNullable(in.Department), in.AllocationTypeName, settingsNullable(in.Color),
			in.DefaultEngagementCode, settingsNullable(in.EngagementCodeSuffix), settingsNullable(in.EmailToAllocations), id)
		if err != nil {
			return fmt.Errorf("allocationType.Update: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrSettingsAllocationTypeNotFound
		}
		return t.Record(ctx, tx, settingsAuditEvent(audit.Update, in.Actor))
	})
	if err != nil {
		return err
	}
	SettingsAuditLog(ctx, r.db, "ALLOCATION_TYPE", "UPDATE",
		fmt.Sprintf("Allocation type of ID %d is updated by %s", id, in.Actor), in.Actor)
	return nil
}

// DeleteAllocationType ports deleteAllocationTypeQuery (soft delete) and its
// audit row. There is no in-use check, as in legacy.
func (r *settingsRepo) DeleteAllocationType(ctx context.Context, id int, actor string) error {
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "allocation_type", "id", `id = ? AND status != 'Deleted'`, id)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE allocation_type SET status = 'Deleted' WHERE id = ? AND status != 'Deleted'`, id)
		if err != nil {
			return fmt.Errorf("allocationType.Delete: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrSettingsAllocationTypeNotFound
		}
		return t.Record(ctx, tx, settingsAuditEvent(audit.Delete, actor))
	})
	if err != nil {
		return err
	}
	SettingsAuditLog(ctx, r.db, "ALLOCATION_TYPE", "DELETE",
		fmt.Sprintf("Allocation type of ID %d is deleted (status is updated to Deleted) by %s", id, actor), actor)
	return nil
}

func settingsAuditEvent(action audit.Action, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: "allocation_type", Actor: actor}
}
