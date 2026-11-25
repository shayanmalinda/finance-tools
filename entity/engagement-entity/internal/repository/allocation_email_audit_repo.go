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
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// AllocationEmailAuditRepository writes allocation_db.email_auditlog.
type AllocationEmailAuditRepository interface {
	// InsertEmailAudit writes one email_auditlog row.
	InsertEmailAudit(ctx context.Context, in domain.EmailAuditInput) error
}

type allocationEmailAuditRepo struct{ db *sql.DB }

// NewAllocationEmailAuditRepository returns the repository on the
// allocation_db pool.
func NewAllocationEmailAuditRepository(db *sql.DB) AllocationEmailAuditRepository {
	return &allocationEmailAuditRepo{db: db}
}

// InsertEmailAudit writes the row; the table is created by
// Resources/allocation_db_table_update_v1.2.0.sql. Until it is applied the
// insert fails and the caller logs it. The table is utf8mb4, so no Latin-1
// normalisation.
func (r *allocationEmailAuditRepo) InsertEmailAudit(ctx context.Context, in domain.EmailAuditInput) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO email_auditlog
		(subject, templateId, contentKeyValPairs, recipients, cc_recipients, datetime, status, error_message)
		VALUES (?, ?, ?, ?, ?, NOW(), ?, ?)`,
		textnorm.Truncate(in.Subject, emailAuditShort),
		textnorm.Truncate(in.TemplateID, emailAuditShort),
		textnorm.Truncate(in.ContentKeyValPairs, emailAuditContent),
		textnorm.Truncate(in.Recipients, emailAuditShort),
		textnorm.Truncate(in.CCRecipients, emailAuditShort),
		in.Status, emailAuditErrorValue(in.ErrorMessage, func(s string) string { return s }))
	if err != nil {
		return fmt.Errorf("allocation emailAuditlog.Insert: %w", err)
	}
	return nil
}
