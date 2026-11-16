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
	"fmt"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/cache"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// engagementTypeTTL bounds how stale a cached engagement type list can be.
// Neither legacy app edits engagement_type (a new type is a data change made
// by the team), so ten minutes only delays such a change.
const engagementTypeTTL = 10 * time.Minute

// cachedTypeService caches engagement types, which every create and filter
// screen reads. Allocation types are not cached: the Allocation Types
// settings screen edits them, and an edit must show on the next read.
type cachedTypeService struct {
	TypeService
	engagementTypes *cache.Cache[string, domain.ListEngagementTypesResponse]
}

// NewCachedTypeService wraps next with an engagement type cache.
func NewCachedTypeService(next TypeService) TypeService {
	return &cachedTypeService{
		TypeService:     next,
		engagementTypes: cache.New[string, domain.ListEngagementTypesResponse](engagementTypeTTL),
	}
}

func (s *cachedTypeService) ListEngagementTypes(ctx context.Context, f domain.EngagementTypeFilter) (domain.ListEngagementTypesResponse, error) {
	active := "any"
	if f.IsActive != nil {
		active = fmt.Sprint(*f.IsActive)
	}
	key := f.Type + "|" + active
	if v, ok := s.engagementTypes.Get(key); ok {
		return v, nil
	}
	v, err := s.TypeService.ListEngagementTypes(ctx, f)
	if err != nil {
		return v, err
	}
	s.engagementTypes.Set(key, v)
	return v, nil
}
