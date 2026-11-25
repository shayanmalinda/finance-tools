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

package service

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// emailAuditTimeout bounds one log write, which runs after the send and
// outside the request's own deadline.
const emailAuditTimeout = 5 * time.Second

// emailAuditErrorMax is the error_message column size.
const emailAuditErrorMax = 500

// EmailAuditAPI is the entity's email log: travelapp.email_auditlog for the
// Travel app's emails, allocation_db.email_auditlog for the Allocations app's.
type EmailAuditAPI interface {
	InsertTravelEmailAudit(ctx context.Context, in engagemententity.TravelEmailAudit) error
	InsertAllocationEmailAudit(ctx context.Context, in engagemententity.EmailAudit) error
}

// EmailAuditor returns the email.AuditFunc that logs every send attempt to
// the entity: Sent, or Failed with the error. A failed log write is logged
// only (the user's action and the email's outcome are unchanged), so a
// missing table or column on a database not yet migrated costs only the log.
func EmailAuditor(entity EmailAuditAPI, now func() time.Time) email.AuditFunc {
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, m email.Message, sendErr error) {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), emailAuditTimeout)
		defer cancel()
		subject := strings.TrimSpace(m.Subject)
		if subject == "" {
			// The entity rejects a blank subject, and a failed send must still be logged.
			subject = "(no subject)"
		}
		row := engagemententity.EmailAudit{
			Subject: subject, TemplateID: m.TemplateID,
			ContentKeyValPairs: email.AuditContent(m, now().UTC().Year()),
			Recipients:         joinNonEmpty(m.To), CCRecipients: joinNonEmpty(m.CC),
			Status: engagemententity.EmailAuditSent,
		}
		if sendErr != nil {
			row.Status = engagemententity.EmailAuditFailed
			row.ErrorMessage = EmailAuditErrorText(sendErr)
		}
		var err error
		if m.App == email.AppTravel {
			err = entity.InsertTravelEmailAudit(ctx, row)
		} else {
			err = entity.InsertAllocationEmailAudit(ctx, row)
		}
		if err != nil {
			slog.ErrorContext(ctx, "email_auditlog not written", "app", string(m.App), "type", m.NotificationType,
				"ref", m.ReferenceID, "status", row.Status, "err", err)
		}
	}
}

func joinNonEmpty(list []string) string {
	var out []string
	for _, a := range list {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return strings.Join(out, ", ")
}

// unsafeInError matches what must not reach a table people can read: URLs
// (they can carry tokens), bearer tokens and key=value credentials.
var unsafeInError = regexp.MustCompile(`(?i)https?://\S+|bearer\s+\S+|(?:token|secret|password|passwd|authorization|api[_-]?key)\s*[=:]\s*\S+`)

// EmailAuditErrorText is err as the error_message of a Failed row: one line,
// URLs and credentials replaced by [redacted], cut to the column size.
func EmailAuditErrorText(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	s = unsafeInError.ReplaceAllString(s, "[redacted]")
	if r := []rune(s); len(r) > emailAuditErrorMax {
		s = string(r[:emailAuditErrorMax])
	}
	return s
}
