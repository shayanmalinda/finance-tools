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

// Package service contains business logic between HTTP handlers and the repository layer.
package service

import (
	"context"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// AccessService exposes each legacy app's roles and privileges. Every method
// takes the app explicitly: the two apps' privilege ids are never mixed.
type AccessService interface {
	ListRoles(ctx context.Context, app domain.App) (domain.ListRolesResponse, error)
	ListPrivileges(ctx context.Context, app domain.App) (domain.ListPrivilegesResponse, error)
	RolePrivilegeMap(ctx context.Context, app domain.App) (domain.RolePrivilegeMapResponse, error)
	// TravelAgency returns the travel agency email works for, if any.
	TravelAgency(ctx context.Context, email string) (domain.TravelAgencyResponse, error)
}

// TypeService exposes the engagement and allocation type lookups.
type TypeService interface {
	ListEngagementTypes(ctx context.Context, f domain.EngagementTypeFilter) (domain.ListEngagementTypesResponse, error)
	ListAllocationTypes(ctx context.Context, f domain.AllocationTypeFilter) (domain.ListAllocationTypesResponse, error)
}
