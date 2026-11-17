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

// auditSchema names the schema an entity type's audit events go to.
type auditSchema int

const (
	auditAllocationDB auditSchema = iota
	auditTravelApp
)

// auditSchemas maps entity types to their schema.
var auditSchemas = map[string]auditSchema{
	"engagement": auditAllocationDB, "allocation": auditAllocationDB, "allocation_type": auditAllocationDB,
	"travel_request": auditTravelApp, "visa": auditTravelApp, "travel_document": auditTravelApp,
	"per_diem": auditTravelApp, "per_diem_payment": auditTravelApp, "invoice": auditTravelApp,
	"travel_card": auditTravelApp,
}

type auditRepo struct{ alloc, travel *sql.DB }

// NewAuditRepository constructs an AuditRepository over both pools.
func NewAuditRepository(alloc, travel *sql.DB) AuditRepository {
	return &auditRepo{alloc: alloc, travel: travel}
}

func (r *auditRepo) Record(ctx context.Context, e audit.Event) (bool, error) {
	schema, ok := auditSchemas[e.EntityType]
	if !ok {
		return false, nil
	}
	db := r.travel
	if schema == auditAllocationDB {
		db = r.alloc
	}
	return true, audit.Record(ctx, db, e)
}
