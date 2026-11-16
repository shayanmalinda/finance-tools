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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// ReferenceService returns every lookup list of the two legacy schemas in
// one answer.
type ReferenceService interface {
	ReferenceData(ctx context.Context) (domain.ReferenceDataResponse, error)
}

type referenceService struct {
	types repository.TypeRepository
	refs  repository.ReferenceRepository
}

// NewReferenceService constructs a ReferenceService.
func NewReferenceService(types repository.TypeRepository, refs repository.ReferenceRepository) ReferenceService {
	return &referenceService{types: types, refs: refs}
}

func referenceNonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// ReferenceData reads every list. Engagement types are all of them (the
// filters show the inactive "Paid" too); allocation types are the legacy
// Active and Deleted rows.
func (s *referenceService) ReferenceData(ctx context.Context) (domain.ReferenceDataResponse, error) {
	var (
		out domain.ReferenceDataResponse
		err error
	)
	if out.EngagementTypes, err = s.types.ListEngagementTypes(ctx, domain.EngagementTypeFilter{}); err != nil {
		return out, err
	}
	if out.AllocationTypes, err = s.types.ListAllocationTypes(ctx, domain.AllocationTypeFilter{}); err != nil {
		return out, err
	}
	if out.BusinessUnits, err = s.refs.BusinessUnits(ctx); err != nil {
		return out, err
	}
	if out.ProductUnits, err = s.refs.ProductUnits(ctx); err != nil {
		return out, err
	}
	if out.Regions, err = s.refs.Regions(ctx); err != nil {
		return out, err
	}
	if out.SubRegions, err = s.refs.SubRegions(ctx); err != nil {
		return out, err
	}
	if out.TravelAgencies, err = s.refs.TravelAgencies(ctx); err != nil {
		return out, err
	}
	if out.CostTypes, err = s.refs.CostTypes(ctx); err != nil {
		return out, err
	}
	if out.PerDiemRates, err = s.refs.PerDiemRates(ctx); err != nil {
		return out, err
	}
	out.EngagementTypes = referenceNonNil(out.EngagementTypes)
	out.AllocationTypes = referenceNonNil(out.AllocationTypes)
	return out, nil
}
