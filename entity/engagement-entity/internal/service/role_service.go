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
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// RoleService is Role Settings of each legacy app: roles with their
// privileges, and replacing a role's privileges.
type RoleService interface {
	Settings(ctx context.Context, app domain.App) (domain.RoleSettingsResponse, error)
	ReplacePrivileges(ctx context.Context, app domain.App, roleID int, req domain.RolePrivilegesUpdateRequest) (domain.RoleWithPrivileges, error)
}

type roleService struct {
	roles  map[domain.App]repository.RoleRepository
	access map[domain.App]repository.AccessRepository
}

// NewRoleService constructs a RoleService from one role and one access
// repository per app.
func NewRoleService(roles map[domain.App]repository.RoleRepository, access map[domain.App]repository.AccessRepository) RoleService {
	return &roleService{roles: roles, access: access}
}

func (s *roleService) repos(app domain.App) (repository.RoleRepository, repository.AccessRepository, error) {
	r, ok1 := s.roles[app]
	a, ok2 := s.access[app]
	if !app.Valid() || !ok1 || !ok2 {
		return nil, nil, &apierror.NotFoundError{Msg: fmt.Sprintf("unknown app %q: must be allocations or travel", app)}
	}
	return r, a, nil
}

func (s *roleService) Settings(ctx context.Context, app domain.App) (domain.RoleSettingsResponse, error) {
	r, a, err := s.repos(app)
	if err != nil {
		return domain.RoleSettingsResponse{}, err
	}
	roles, err := r.Roles(ctx)
	if err != nil {
		return domain.RoleSettingsResponse{}, err
	}
	privs, err := a.ListPrivileges(ctx)
	if err != nil {
		return domain.RoleSettingsResponse{}, err
	}
	if privs == nil {
		privs = []domain.Privilege{}
	}
	return domain.RoleSettingsResponse{Roles: roles, Privileges: privs}, nil
}

// ReplacePrivileges replaces the role's privileges with the given set. Ids
// must be active privileges of the app (the legacy screen offered only
// those); repeats are dropped.
func (s *roleService) ReplacePrivileges(ctx context.Context, app domain.App, roleID int, req domain.RolePrivilegesUpdateRequest) (domain.RoleWithPrivileges, error) {
	r, a, err := s.repos(app)
	if err != nil {
		return domain.RoleWithPrivileges{}, err
	}
	if strings.TrimSpace(req.UpdatedBy) == "" {
		return domain.RoleWithPrivileges{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	current, err := r.Role(ctx, roleID)
	if err != nil {
		if errors.Is(err, repository.ErrRoleNotFound) {
			return domain.RoleWithPrivileges{}, &apierror.NotFoundError{Msg: fmt.Sprintf("Role %d not found.", roleID)}
		}
		return domain.RoleWithPrivileges{}, err
	}
	privs, err := a.ListPrivileges(ctx)
	if err != nil {
		return domain.RoleWithPrivileges{}, err
	}
	known := map[int]bool{}
	for _, p := range privs {
		known[p.ID] = true
	}
	// A role may keep privileges it already holds even if they are now inactive.
	for _, id := range current.PrivilegeIDs {
		known[id] = true
	}
	seen := map[int]bool{}
	ids := []int{}
	for _, id := range req.PrivilegeIDs {
		if !known[id] {
			return domain.RoleWithPrivileges{}, &apierror.ValidationError{Msg: fmt.Sprintf("Privilege %d is not an active privilege of %s.", id, app)}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	if err := r.ReplacePrivileges(ctx, roleID, ids, req.UpdatedBy); err != nil {
		return domain.RoleWithPrivileges{}, err
	}
	return r.Role(ctx, roleID)
}
