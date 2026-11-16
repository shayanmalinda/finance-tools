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

// TypeRepository reads the engagement and allocation type lookups in
// allocation_db.
type TypeRepository interface {
	ListEngagementTypes(ctx context.Context, f domain.EngagementTypeFilter) ([]domain.EngagementType, error)
	ListAllocationTypes(ctx context.Context, f domain.AllocationTypeFilter) ([]domain.AllocationType, error)
}

type typeRepo struct{ db *sql.DB }

// NewTypeRepository constructs a TypeRepository. db must be the allocation_db pool.
func NewTypeRepository(db *sql.DB) TypeRepository { return &typeRepo{db: db} }

// ListEngagementTypes ports the legacy getEngagementTypeQuery. ORDER BY id is
// added so the order no longer depends on the storage engine.
func (r *typeRepo) ListEngagementTypes(ctx context.Context, f domain.EngagementTypeFilter) ([]domain.EngagementType, error) {
	q := `SELECT id, name, type, is_active FROM engagement_type WHERE 1 = 1`
	var args []any
	if f.Type != "" {
		q += ` AND type = ?`
		args = append(args, f.Type)
	}
	if f.IsActive != nil {
		q += ` AND is_active = ?`
		args = append(args, *f.IsActive)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("engagementType.List: %w", err)
	}
	defer rows.Close()

	var out []domain.EngagementType
	for rows.Next() {
		var et domain.EngagementType
		if err := rows.Scan(&et.ID, &et.Name, &et.Type, &et.IsActive); err != nil {
			return nil, fmt.Errorf("engagementType.List scan: %w", err)
		}
		out = append(out, et)
	}
	return out, rows.Err()
}

// ListAllocationTypes ports the legacy getAllocationTypesQuery: only Active
// and Deleted rows, joined to their engagement type, ordered by name. The
// status comparison relies on the column's case-insensitive collation, as the
// legacy query does (rows hold "ACTIVE" as well as "Active").
func (r *typeRepo) ListAllocationTypes(ctx context.Context, f domain.AllocationTypeFilter) ([]domain.AllocationType, error) {
	q := `SELECT aT.id, aT.allocation_type, aT.status, aT.color, aT.department,
			aT.emailToAllocations, aT.default_engagement_code, aT.engagement_code_suffix,
			eT.id, eT.name
		FROM allocation_type AS aT
		INNER JOIN engagement_type AS eT ON aT.engagement_type = eT.id
		WHERE aT.status IN ('Active', 'Deleted')`
	var args []any
	if f.EngagementTypeID != 0 {
		q += ` AND eT.id = ?`
		args = append(args, f.EngagementTypeID)
	}
	if f.Status != "" {
		q += ` AND aT.status = ?`
		args = append(args, f.Status)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY aT.allocation_type`, args...)
	if err != nil {
		return nil, fmt.Errorf("allocationType.List: %w", err)
	}
	defer rows.Close()

	var out []domain.AllocationType
	for rows.Next() {
		var at domain.AllocationType
		if err := rows.Scan(&at.ID, &at.AllocationTypeName, &at.Status, &at.Color, &at.Department,
			&at.EmailToAllocations, &at.DefaultEngagementCode, &at.EngagementCodeSuffix,
			&at.EngagementTypeID, &at.EngagementTypeName); err != nil {
			return nil, fmt.Errorf("allocationType.List scan: %w", err)
		}
		out = append(out, at)
	}
	return out, rows.Err()
}
