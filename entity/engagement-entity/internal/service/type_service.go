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

type typeService struct {
	repo repository.TypeRepository
}

// NewTypeService constructs a TypeService.
func NewTypeService(repo repository.TypeRepository) TypeService {
	return &typeService{repo: repo}
}

// canonical returns the member of allowed that equals v ignoring case, or
// false when there is none.
func canonical(v string, allowed ...string) (string, bool) {
	for _, a := range allowed {
		if strings.EqualFold(v, a) {
			return a, true
		}
	}
	return "", false
}

func (s *typeService) ListEngagementTypes(ctx context.Context, f domain.EngagementTypeFilter) (domain.ListEngagementTypesResponse, error) {
	if f.Type != "" {
		t, ok := canonical(f.Type, domain.EngagementKindInternal, domain.EngagementKindCustomer)
		if !ok {
			return domain.ListEngagementTypesResponse{}, &apierror.ValidationError{Msg: "type must be Internal or Customer"}
		}
		f.Type = t
	}
	types, err := s.repo.ListEngagementTypes(ctx, f)
	if err != nil {
		return domain.ListEngagementTypesResponse{}, err
	}
	if types == nil {
		types = []domain.EngagementType{}
	}
	return domain.ListEngagementTypesResponse{EngagementTypes: types}, nil
}

func (s *typeService) ListAllocationTypes(ctx context.Context, f domain.AllocationTypeFilter) (domain.ListAllocationTypesResponse, error) {
	if f.EngagementTypeID < 0 {
		return domain.ListAllocationTypesResponse{}, &apierror.ValidationError{Msg: "engagementTypeId must be a positive integer"}
	}
	if f.Status != "" {
		st, ok := canonical(f.Status, domain.AllocationTypeStatusActive, domain.AllocationTypeStatusDeleted)
		if !ok {
			return domain.ListAllocationTypesResponse{}, &apierror.ValidationError{Msg: "status must be Active or Deleted"}
		}
		f.Status = st
	}
	types, err := s.repo.ListAllocationTypes(ctx, f)
	if err != nil {
		return domain.ListAllocationTypesResponse{}, err
	}
	if types == nil {
		types = []domain.AllocationType{}
	}
	return domain.ListAllocationTypesResponse{AllocationTypes: types}, nil
}
