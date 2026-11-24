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

package service

import (
	"context"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// normalizeEmailAudit validates an email_auditlog row: subject and templateId
// are required, status is "Sent" (also when empty) or "Failed", and a Sent
// row carries no error.
func normalizeEmailAudit(in domain.EmailAuditInput) (domain.EmailAuditInput, error) {
	if strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.TemplateID) == "" {
		return in, &apierror.ValidationError{Msg: "subject and templateId are required"}
	}
	switch in.Status {
	case "":
		in.Status = domain.EmailAuditSent
	case domain.EmailAuditSent, domain.EmailAuditFailed:
	default:
		return in, &apierror.ValidationError{Msg: "status must be Sent or Failed"}
	}
	if in.Status == domain.EmailAuditSent {
		in.ErrorMessage = ""
	}
	return in, nil
}

// AllocationEmailAuditService serves POST /allocation-email-audit-logs.
type AllocationEmailAuditService interface {
	InsertEmailAudit(ctx context.Context, in domain.EmailAuditInput) error
}

type allocationEmailAuditService struct {
	repo repository.AllocationEmailAuditRepository
}

// NewAllocationEmailAuditService constructs the service.
func NewAllocationEmailAuditService(repo repository.AllocationEmailAuditRepository) AllocationEmailAuditService {
	return &allocationEmailAuditService{repo: repo}
}

func (s *allocationEmailAuditService) InsertEmailAudit(ctx context.Context, in domain.EmailAuditInput) error {
	in, err := normalizeEmailAudit(in)
	if err != nil {
		return err
	}
	return s.repo.InsertEmailAudit(ctx, in)
}
