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
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type accessService struct {
	repos    map[domain.App]repository.AccessRepository
	agencies repository.AgencyRepository
}

// NewAccessService constructs an AccessService from one repository per app.
// An app without a repository answers 404.
func NewAccessService(repos map[domain.App]repository.AccessRepository, agencies repository.AgencyRepository) AccessService {
	return &accessService{repos: repos, agencies: agencies}
}

func (s *accessService) TravelAgency(ctx context.Context, email string) (domain.TravelAgencyResponse, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return domain.TravelAgencyResponse{}, &apierror.ValidationError{Msg: "email is required"}
	}
	a, err := s.agencies.AgencyOf(ctx, email)
	if err != nil {
		return domain.TravelAgencyResponse{}, err
	}
	return domain.TravelAgencyResponse{Agency: a}, nil
}

func (s *accessService) repo(app domain.App) (repository.AccessRepository, error) {
	if r, ok := s.repos[app]; ok && app.Valid() {
		return r, nil
	}
	return nil, &apierror.NotFoundError{Msg: fmt.Sprintf("unknown app %q: must be allocations or travel", app)}
}

func (s *accessService) ListRoles(ctx context.Context, app domain.App) (domain.ListRolesResponse, error) {
	r, err := s.repo(app)
	if err != nil {
		return domain.ListRolesResponse{}, err
	}
	roles, err := r.ListRoles(ctx)
	if err != nil {
		return domain.ListRolesResponse{}, err
	}
	if roles == nil {
		roles = []domain.Role{}
	}
	return domain.ListRolesResponse{Roles: roles}, nil
}

func (s *accessService) ListPrivileges(ctx context.Context, app domain.App) (domain.ListPrivilegesResponse, error) {
	r, err := s.repo(app)
	if err != nil {
		return domain.ListPrivilegesResponse{}, err
	}
	privileges, err := r.ListPrivileges(ctx)
	if err != nil {
		return domain.ListPrivilegesResponse{}, err
	}
	if privileges == nil {
		privileges = []domain.Privilege{}
	}
	return domain.ListPrivilegesResponse{Privileges: privileges}, nil
}

func (s *accessService) RolePrivilegeMap(ctx context.Context, app domain.App) (domain.RolePrivilegeMapResponse, error) {
	r, err := s.repo(app)
	if err != nil {
		return domain.RolePrivilegeMapResponse{}, err
	}
	m, err := r.RolePrivilegeMap(ctx)
	if err != nil {
		return domain.RolePrivilegeMapResponse{}, err
	}
	// An empty map serialises as {} rather than null: a caller must be able to
	// tell "no grants configured" from a malformed response.
	if m == nil {
		m = map[string][]int{}
	}
	return domain.RolePrivilegeMapResponse{RolePrivileges: m}, nil
}
