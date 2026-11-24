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
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func TestAllocationInsertEmailAudit_Sent(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO email_auditlog`)).
		WithArgs("[Engagement App] Consultant request", "consultantRequest", "{}", "a@example.com", "b@example.com", "Sent", nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewAllocationEmailAuditRepository(db).InsertEmailAudit(context.Background(), domain.EmailAuditInput{
		Subject: "[Engagement App] Consultant request", TemplateID: "consultantRequest", ContentKeyValPairs: "{}",
		Recipients: "a@example.com", CCRecipients: "b@example.com", Status: "Sent",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAllocationInsertEmailAudit_FailedWithErrorCutToTheColumn(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`(subject, templateId, contentKeyValPairs, recipients, cc_recipients, datetime, status, error_message)`)).
		WithArgs("subj", "tpl", "{}", "a@example.com", "", "Failed", strings.Repeat("é", 500)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewAllocationEmailAuditRepository(db).InsertEmailAudit(context.Background(), domain.EmailAuditInput{
		Subject: "subj", TemplateID: "tpl", ContentKeyValPairs: "{}", Recipients: "a@example.com",
		Status: "Failed", ErrorMessage: strings.Repeat("é", 700),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAllocationInsertEmailAudit_MissingTableIsAnError(t *testing.T) {
	db, mock := mockDB(t)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO email_auditlog`)).WillReturnError(context.DeadlineExceeded)
	if err := NewAllocationEmailAuditRepository(db).InsertEmailAudit(context.Background(), domain.EmailAuditInput{
		Subject: "s", TemplateID: "t", Status: "Sent",
	}); err == nil {
		t.Fatal("want the insert error returned for the caller to log")
	}
}
