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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// SettingsService is the Settings screen: Allocation Roles and Travel
// Roles (Role Settings), Partners and Allocation Types.
type SettingsService interface {
	RoleSettings(ctx context.Context, p domain.Perms, app domain.App) (domain.RoleSettings, error)
	UpdateRolePrivileges(ctx context.Context, p domain.Perms, app domain.App, roleID int, privilegeIDs []int) (domain.RoleSettingsRole, error)

	AllocationTypes(ctx context.Context, engagementTypeID int, search string) ([]domain.ReferenceAllocationType, error)
	CreateAllocationType(ctx context.Context, p domain.Perms, in domain.SettingsAllocationTypeInput) (domain.ReferenceAllocationType, error)
	UpdateAllocationType(ctx context.Context, p domain.Perms, id int, in domain.SettingsAllocationTypeInput) (domain.ReferenceAllocationType, error)
	DeleteAllocationType(ctx context.Context, p domain.Perms, id int) error
}

type settingsService struct {
	entity         EngagementEntityAPI
	refs           ReferenceService
	onRolesChanged func(domain.App)
}

// NewSettingsService constructs a SettingsService. refs is invalidated
// after an allocation type write.
func NewSettingsService(entity EngagementEntityAPI, refs ReferenceService) SettingsService {
	return &settingsService{entity: entity, refs: refs}
}

// NewSettingsServiceWithRoleHook is NewSettingsService plus a hook run after
// a role's privileges are saved (the access service drops its cached role
// table, so the change applies at once).
func NewSettingsServiceWithRoleHook(entity EngagementEntityAPI, refs ReferenceService, onRolesChanged func(domain.App)) SettingsService {
	return &settingsService{entity: entity, refs: refs, onRolesChanged: onRolesChanged}
}

// settingsRolePrivilege is each app's Role Settings privilege: Allocations
// 23, Travel 29, and only for a person in both super admin groups.
func settingsRoleAccess(p domain.Perms, app domain.App) error {
	if !domain.HasRole(p.AllocRoles, domain.AllocationsSuperAdminGroup) || !domain.HasRole(p.TravelRoles, domain.TravelSuperAdminGroup) {
		return engagementForbidden()
	}
	switch app {
	case domain.AppAllocations:
		if p.Alloc.Has(23) {
			return nil
		}
	case domain.AppTravel:
		if p.Travel.Has(29) {
			return nil
		}
	default:
		return &apierror.NotFoundError{Msg: "Unknown app: must be allocations or travel."}
	}
	return engagementForbidden()
}

func (s *settingsService) RoleSettings(ctx context.Context, p domain.Perms, app domain.App) (domain.RoleSettings, error) {
	if err := settingsRoleAccess(p, app); err != nil {
		return domain.RoleSettings{}, err
	}
	l, err := s.entity.RoleSettings(ctx, app)
	if err != nil {
		return domain.RoleSettings{}, err
	}
	out := domain.RoleSettings{Roles: make([]domain.RoleSettingsRole, 0, len(l.Roles)), Privileges: make([]domain.RoleSettingsPrivilege, 0, len(l.Privileges))}
	for _, r := range l.Roles {
		ids := r.PrivilegeIDs
		if ids == nil {
			ids = []int{}
		}
		out.Roles = append(out.Roles, domain.RoleSettingsRole{ID: r.ID, Name: r.Name, PrivilegeIDs: ids})
	}
	for _, pr := range l.Privileges {
		out.Privileges = append(out.Privileges, domain.RoleSettingsPrivilege(pr))
	}
	return out, nil
}

// UpdateRolePrivileges replaces a role's privileges (legacy PATCH
// role-privileges with the privilege audit log). The backend's
// role-privilege cache keeps the old grants for up to a minute.
func (s *settingsService) UpdateRolePrivileges(ctx context.Context, p domain.Perms, app domain.App, roleID int, privilegeIDs []int) (domain.RoleSettingsRole, error) {
	if err := settingsRoleAccess(p, app); err != nil {
		return domain.RoleSettingsRole{}, err
	}
	if privilegeIDs == nil {
		return domain.RoleSettingsRole{}, engagementValidation("privilegeIds is required.")
	}
	r, err := s.entity.RoleReplacePrivileges(ctx, app, roleID, privilegeIDs, p.Email)
	if err != nil {
		return domain.RoleSettingsRole{}, err
	}
	if s.onRolesChanged != nil {
		s.onRolesChanged(app)
	}
	if r.PrivilegeIDs == nil {
		r.PrivilegeIDs = []int{}
	}
	return domain.RoleSettingsRole{ID: r.ID, Name: r.Name, PrivilegeIDs: r.PrivilegeIDs}, nil
}

// ── Partners ─────────────────────────────────────────────────────────────────

// ── Allocation types ─────────────────────────────────────────────────────────

// AllocationTypes is the settings list: Active types (the legacy status
// filter), optionally of one engagement type, matching search on name,
// department or code.
func (s *settingsService) AllocationTypes(ctx context.Context, engagementTypeID int, search string) ([]domain.ReferenceAllocationType, error) {
	ts, err := s.entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{EngagementTypeID: engagementTypeID, Status: "Active"})
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(search))
	out := []domain.ReferenceAllocationType{}
	for _, t := range ts {
		if q != "" && !strings.Contains(strings.ToLower(t.AllocationTypeName), q) &&
			!strings.Contains(strings.ToLower(engagementVal(t.Department)), q) &&
			!strings.Contains(strings.ToLower(engagementVal(t.DefaultEngagementCode)), q) {
			continue
		}
		out = append(out, domain.ReferenceAllocationType(t))
	}
	return out, nil
}

func settingsTypeFields(p domain.Perms, in domain.SettingsAllocationTypeInput) engagemententity.SettingsAllocationTypeFields {
	return engagemententity.SettingsAllocationTypeFields{
		EngagementTypeID: in.EngagementTypeID, AllocationTypeName: in.AllocationTypeName, DefaultEngagementCode: in.DefaultEngagementCode,
		EngagementCodeSuffix: in.EngagementCodeSuffix, Department: in.Department, EmailToAllocations: in.EmailToAllocations,
		Color: in.Color, Actor: p.Email,
	}
}

func (s *settingsService) CreateAllocationType(ctx context.Context, p domain.Perms, in domain.SettingsAllocationTypeInput) (domain.ReferenceAllocationType, error) {
	t, err := s.entity.SettingsCreateAllocationType(ctx, settingsTypeFields(p, in))
	if err != nil {
		return domain.ReferenceAllocationType{}, err
	}
	s.refs.Invalidate()
	return domain.ReferenceAllocationType(t), nil
}

func (s *settingsService) UpdateAllocationType(ctx context.Context, p domain.Perms, id int, in domain.SettingsAllocationTypeInput) (domain.ReferenceAllocationType, error) {
	t, err := s.entity.SettingsUpdateAllocationType(ctx, id, settingsTypeFields(p, in))
	if err != nil {
		return domain.ReferenceAllocationType{}, err
	}
	s.refs.Invalidate()
	return domain.ReferenceAllocationType(t), nil
}

func (s *settingsService) DeleteAllocationType(ctx context.Context, p domain.Perms, id int) error {
	if err := s.entity.SettingsDeleteAllocationType(ctx, id, p.Email); err != nil {
		return err
	}
	s.refs.Invalidate()
	return nil
}
