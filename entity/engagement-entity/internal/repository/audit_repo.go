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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
)

// AuditRepository records audit events that are not part of a write.
type AuditRepository interface {
	// Record writes e to the schema that holds its entity type, reporting
	// false for an unknown type.
	Record(ctx context.Context, e audit.Event) (bool, error)
}

// auditSchemas maps entity types to their schema: true is allocation_db,
// false travelapp.
var auditSchemas = map[string]bool{
	"engagement": true, "allocation": true, "allocation_type": true,
	"travel_request": false, "visa": false, "travel_document": false, "per_diem": false,
	"per_diem_payment": false, "invoice": false, "travel_card": false,
}

type auditRepo struct{ alloc, travel *sql.DB }

// NewAuditRepository constructs an AuditRepository over both pools.
func NewAuditRepository(alloc, travel *sql.DB) AuditRepository {
	return &auditRepo{alloc: alloc, travel: travel}
}

func (r *auditRepo) Record(ctx context.Context, e audit.Event) (bool, error) {
	inAlloc, ok := auditSchemas[e.EntityType]
	if !ok {
		return false, nil
	}
	db := r.travel
	if inAlloc {
		db = r.alloc
	}
	return true, audit.Record(ctx, db, e)
}
