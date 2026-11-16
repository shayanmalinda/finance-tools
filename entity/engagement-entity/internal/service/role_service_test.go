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
	"reflect"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type roleFakeRepo struct {
	replaced []int
	missing  bool
}

func (f *roleFakeRepo) Roles(context.Context) ([]domain.RoleWithPrivileges, error) {
	return []domain.RoleWithPrivileges{{ID: 1, Name: "r", PrivilegeIDs: []int{1}}}, nil
}
func (f *roleFakeRepo) Role(_ context.Context, id int) (domain.RoleWithPrivileges, error) {
	if f.missing {
		return domain.RoleWithPrivileges{}, repository.ErrRoleNotFound
	}
	return domain.RoleWithPrivileges{ID: id, Name: "r", PrivilegeIDs: f.replaced}, nil
}
func (f *roleFakeRepo) ReplacePrivileges(_ context.Context, _ int, ids []int, _ string) error {
	f.replaced = ids
	return nil
}

type roleFakeAccess struct{ repository.AccessRepository }

func (roleFakeAccess) ListPrivileges(context.Context) ([]domain.Privilege, error) {
	return []domain.Privilege{{ID: 1}, {ID: 2}, {ID: 29}}, nil
}

func roleTestService(r *roleFakeRepo) RoleService {
	return NewRoleService(map[domain.App]repository.RoleRepository{domain.AppTravel: r},
		map[domain.App]repository.AccessRepository{domain.AppTravel: roleFakeAccess{}})
}

func TestRoleService(t *testing.T) {
	r := &roleFakeRepo{}
	svc := roleTestService(r)
	s, err := svc.Settings(context.Background(), domain.AppTravel)
	if err != nil || len(s.Roles) != 1 || len(s.Privileges) != 3 {
		t.Fatalf("settings %+v err %v", s, err)
	}
	var nf *apierror.NotFoundError
	if _, err := svc.Settings(context.Background(), domain.AppAllocations); !errors.As(err, &nf) {
		t.Fatalf("unwired app: %v", err)
	}
	role, err := svc.ReplacePrivileges(context.Background(), domain.AppTravel, 3,
		domain.RolePrivilegesUpdateRequest{PrivilegeIDs: []int{29, 1, 29}, UpdatedBy: "me"})
	if err != nil || !reflect.DeepEqual(r.replaced, []int{1, 29}) || role.ID != 3 {
		t.Fatalf("role %+v replaced %v err %v", role, r.replaced, err)
	}
	var ve *apierror.ValidationError
	if _, err := svc.ReplacePrivileges(context.Background(), domain.AppTravel, 3,
		domain.RolePrivilegesUpdateRequest{PrivilegeIDs: []int{99}, UpdatedBy: "me"}); !errors.As(err, &ve) {
		t.Fatalf("unknown privilege: %v", err)
	}
	if _, err := svc.ReplacePrivileges(context.Background(), domain.AppTravel, 3,
		domain.RolePrivilegesUpdateRequest{}); !errors.As(err, &ve) {
		t.Fatalf("no updatedBy: %v", err)
	}
	if _, err := roleTestService(&roleFakeRepo{missing: true}).ReplacePrivileges(context.Background(), domain.AppTravel, 3,
		domain.RolePrivilegesUpdateRequest{UpdatedBy: "me"}); !errors.As(err, &nf) {
		t.Fatalf("missing role: %v", err)
	}
}
