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
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// AuditService records the backend's view, download and export events.
type AuditService interface {
	RecordRead(ctx context.Context, in domain.AuditReadEvent) error
}

type auditService struct{ repo repository.AuditRepository }

// NewAuditService constructs an AuditService.
func NewAuditService(repo repository.AuditRepository) AuditService { return &auditService{repo: repo} }

func (s *auditService) RecordRead(ctx context.Context, in domain.AuditReadEvent) error {
	action := audit.Action(in.Action)
	if action != audit.View && action != audit.Download && action != audit.Export {
		return &apierror.ValidationError{Msg: "action must be View, Download or Export"}
	}
	e := audit.Event{
		Action: action, EntityType: strings.TrimSpace(in.EntityType), EntityID: strings.TrimSpace(in.EntityID),
		ParentType: strings.TrimSpace(in.ParentType), ParentID: strings.TrimSpace(in.ParentID), Metadata: in.Metadata,
	}
	if len(e.EntityID) > 64 || len(e.ParentType) > 64 || len(e.ParentID) > 255 {
		return &apierror.ValidationError{Msg: "entityId and parentType must be at most 64 characters, parentId 255"}
	}
	ok, err := s.repo.Record(ctx, e)
	if err != nil {
		return err
	}
	if !ok {
		return &apierror.ValidationError{Msg: "unknown entityType " + in.EntityType}
	}
	return nil
}
