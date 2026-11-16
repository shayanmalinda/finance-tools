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

// Package service holds the backend's business logic between handlers and
// the outbound clients.
package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/cache"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// rolePrivilegeTTL bounds how long a role-privilege map is reused. The legacy
// backends read the tables on every request; a minute keeps an edit made in
// Role Settings close to that while sparing the entity a call per request.
// Each replica holds its own copy, so a change shows on every replica within
// this TTL.
const rolePrivilegeTTL = time.Minute

// agencyTTL bounds how long a person's travel agency is reused.
const agencyTTL = time.Minute

// AccessService answers what a caller may do, per legacy app, from the
// caller's Asgardeo groups and the app's role tables.
type AccessService interface {
	// UserInfo is the GET /user-info answer for caller.
	UserInfo(ctx context.Context, caller reqcontext.Caller) (domain.UserInfoResponse, error)
	// Perms resolves everything caller may do. Services take it as input.
	Perms(ctx context.Context, caller reqcontext.Caller) (domain.Perms, error)
	// Privileges returns the privilege ids caller's groups grant in app,
	// ascending, never nil.
	Privileges(ctx context.Context, app domain.App, groups []string) ([]int, error)
	// HasAnyPrivilege reports whether caller's groups grant at least one of
	// ids in app. Every screen endpoint checks its legacy privilege with it.
	HasAnyPrivilege(ctx context.Context, app domain.App, groups []string, ids ...int) (bool, error)
	// InvalidateRoles drops app's cached role table, so a Role Settings save
	// applies on this replica at once (other replicas within a minute).
	InvalidateRoles(app domain.App)
}

type roleTable struct {
	// byRole maps a lower-cased role name to its privilege ids.
	byRole map[string][]int
	// names maps a lower-cased role name to the name as stored.
	names map[string]string
}

type accessService struct {
	entity engagemententity.AccessAPI
	// maps caches each app's role table.
	maps     *cache.Cache[domain.App, roleTable]
	agencies *cache.Cache[string, *domain.TravelAgency]
}

// NewAccessService constructs an AccessService reading role tables through
// entity.
func NewAccessService(entity engagemententity.AccessAPI) AccessService {
	return &accessService{
		entity:   entity,
		maps:     cache.New[domain.App, roleTable](rolePrivilegeTTL),
		agencies: cache.New[string, *domain.TravelAgency](agencyTTL),
	}
}

// rolePrivileges returns app's role table keyed by lower-cased role name.
// Role names are compared case-insensitively because the legacy queries
// compare them in MySQL (roles.name IN (...)) under a case-insensitive
// collation.
func (s *accessService) rolePrivileges(ctx context.Context, app domain.App) (roleTable, error) {
	if m, ok := s.maps.Get(app); ok {
		return m, nil
	}
	raw, err := s.entity.RolePrivileges(ctx, app)
	if err != nil {
		return roleTable{}, err
	}
	t := roleTable{byRole: make(map[string][]int, len(raw)), names: make(map[string]string, len(raw))}
	for role, ids := range raw {
		key := strings.ToLower(strings.TrimSpace(role))
		t.byRole[key] = append(t.byRole[key], ids...)
		t.names[key] = strings.TrimSpace(role)
	}
	s.maps.Set(app, t)
	return t, nil
}

// resolve returns the privileges groups grant in app, the same without the
// excluded groups, and the role names held.
func (s *accessService) resolve(ctx context.Context, app domain.App, groups []string, exclude string) (all, own domain.PrivilegeSet, roles []string, err error) {
	t, err := s.rolePrivileges(ctx, app)
	if err != nil {
		return nil, nil, nil, err
	}
	all, own, roles = domain.PrivilegeSet{}, domain.PrivilegeSet{}, []string{}
	seen := map[string]bool{}
	for _, g := range groups {
		key := strings.ToLower(strings.TrimSpace(g))
		ids, ok := t.byRole[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		roles = append(roles, t.names[key])
		for _, id := range ids {
			all[id] = true
			if !domain.IsGroup(key, exclude) {
				own[id] = true
			}
		}
	}
	sort.Strings(roles)
	return all, own, roles, nil
}

func (s *accessService) Privileges(ctx context.Context, app domain.App, groups []string) ([]int, error) {
	all, _, _, err := s.resolve(ctx, app, groups, "")
	if err != nil {
		return nil, err
	}
	return all.IDs(), nil
}

func (s *accessService) HasAnyPrivilege(ctx context.Context, app domain.App, groups []string, ids ...int) (bool, error) {
	all, _, _, err := s.resolve(ctx, app, groups, "")
	if err != nil {
		return false, err
	}
	return all.Any(ids...), nil
}

// Perms resolves both apps separately. If either app's tables cannot be read
// the whole answer fails (503) rather than returning a partial one: the
// webapp would otherwise show "no access" for an app that is merely
// unreachable (One WSO2 gate ladder: Failed is not Denied).
func (s *accessService) Perms(ctx context.Context, caller reqcontext.Caller) (domain.Perms, error) {
	if caller.Email == "" {
		return domain.Perms{}, &apierror.UnauthorizedError{Msg: "no signed-in user"}
	}
	p := domain.Perms{Email: caller.Email}
	var err error
	if p.Alloc, p.AllocOwn, p.AllocRoles, err = s.resolve(ctx, domain.AppAllocations, caller.Groups, domain.AllocationsSuperAdminGroup); err != nil {
		return domain.Perms{}, err
	}
	if p.Travel, p.TravelOwn, p.TravelRoles, err = s.resolve(ctx, domain.AppTravel, caller.Groups, domain.TravelSuperAdminGroup); err != nil {
		return domain.Perms{}, err
	}
	// Agency membership is a table lookup, not a group (legacy
	// getTravelAgentIdByEmailQuery); only agents can need it.
	if p.Travel.Has(18) {
		if a, ok := s.agencies.Get(caller.Email); ok {
			p.Agency = a
		} else {
			a, err := s.entity.TravelAgency(ctx, caller.Email)
			if err != nil {
				return domain.Perms{}, err
			}
			s.agencies.Set(caller.Email, a)
			p.Agency = a
		}
	}
	return p, nil
}

func (s *accessService) UserInfo(ctx context.Context, caller reqcontext.Caller) (domain.UserInfoResponse, error) {
	p, err := s.Perms(ctx, caller)
	if err != nil {
		return domain.UserInfoResponse{}, err
	}
	resp := domain.UserInfoResponse{
		Email: p.Email,
		Allocations: domain.AppAccess{
			Privileges: p.Alloc.IDs(), OwnPrivileges: p.AllocOwn.IDs(), Roles: p.AllocRoles,
		},
		Travel: domain.TravelAccess{AppAccess: domain.AppAccess{
			Privileges: p.Travel.IDs(), OwnPrivileges: p.TravelOwn.IDs(), Roles: p.TravelRoles,
		}},
	}
	if p.Agency != nil {
		a := *p.Agency
		resp.Travel.Agency = &a
	}
	resp.WorkQueues = WorkQueuesFor(p)
	return resp, nil
}

func (s *accessService) InvalidateRoles(app domain.App) { s.maps.Delete(app) }

// WorkQueuesFor is the caller's Invoices & Payments tabs (the finance
// area's rules), for /user-info.
func WorkQueuesFor(p domain.Perms) domain.WorkQueues {
	q := domain.WorkQueues{Invoices: invoiceTabs(p), PerDiemPayments: paymentTabs(p)}
	if q.Invoices == nil {
		q.Invoices = []string{}
	}
	if q.PerDiemPayments == nil {
		q.PerDiemPayments = []string{}
	}
	q.NetSuiteSyncFailures = p.TravelOwn.Any(domain.TravelPrivPushInvoices, domain.TravelPrivPushPerDiemPayments)
	return q
}
