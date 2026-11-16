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
	"encoding/json"
	"errors"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type fakeAccessRepo struct {
	m map[string][]int
}

func (f *fakeAccessRepo) ListRoles(context.Context) ([]domain.Role, error) { return nil, nil }
func (f *fakeAccessRepo) ListPrivileges(context.Context) ([]domain.Privilege, error) {
	return nil, nil
}
func (f *fakeAccessRepo) RolePrivilegeMap(context.Context) (map[string][]int, error) {
	return f.m, nil
}

func TestAccessService_UnknownAppIsNotFound(t *testing.T) {
	svc := NewAccessService(map[domain.App]repository.AccessRepository{domain.AppAllocations: &fakeAccessRepo{}}, nil)
	for _, app := range []domain.App{"finance", domain.AppTravel} { // travel: no repository wired
		_, err := svc.RolePrivilegeMap(context.Background(), app)
		var nf *apierror.NotFoundError
		if !errors.As(err, &nf) {
			t.Errorf("app %q: err = %v, want NotFoundError", app, err)
		}
	}
}

// Empty results serialise as [] and {}, never null.
func TestAccessService_EmptyResultsAreNotNull(t *testing.T) {
	svc := NewAccessService(map[domain.App]repository.AccessRepository{domain.AppTravel: &fakeAccessRepo{}}, nil)
	ctx := context.Background()

	roles, _ := svc.ListRoles(ctx, domain.AppTravel)
	privs, _ := svc.ListPrivileges(ctx, domain.AppTravel)
	m, _ := svc.RolePrivilegeMap(ctx, domain.AppTravel)
	for name, v := range map[string]any{"roles": roles, "privileges": privs, "rolePrivileges": m} {
		b, _ := json.Marshal(v)
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(b, &raw)
		if string(raw[name]) == "null" {
			t.Errorf("%s serialised as null: %s", name, b)
		}
	}
}

type fakeTypeRepo struct {
	engagementCalls  int
	lastEngagement   domain.EngagementTypeFilter
	lastAllocation   domain.AllocationTypeFilter
	allocationCalled bool
}

func (f *fakeTypeRepo) ListEngagementTypes(_ context.Context, fl domain.EngagementTypeFilter) ([]domain.EngagementType, error) {
	f.engagementCalls++
	f.lastEngagement = fl
	return []domain.EngagementType{{ID: 1, Name: "Internal Engagement", Type: "Internal", IsActive: true}}, nil
}

func (f *fakeTypeRepo) ListAllocationTypes(_ context.Context, fl domain.AllocationTypeFilter) ([]domain.AllocationType, error) {
	f.allocationCalled = true
	f.lastAllocation = fl
	return nil, nil
}

func TestListEngagementTypes_TypeValidation(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"Internal", "Internal", false},
		{"customer", "Customer", false},
		{"Partner", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			repo := &fakeTypeRepo{}
			_, err := NewTypeService(repo).ListEngagementTypes(context.Background(), domain.EngagementTypeFilter{Type: tc.in})
			if tc.wantErr {
				var ve *apierror.ValidationError
				if !errors.As(err, &ve) || repo.engagementCalls != 0 {
					t.Fatalf("err = %v, calls = %d; want ValidationError and no query", err, repo.engagementCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if repo.lastEngagement.Type != tc.want {
				t.Errorf("repository got type %q, want %q", repo.lastEngagement.Type, tc.want)
			}
		})
	}
}

func TestListAllocationTypes_Validation(t *testing.T) {
	tests := []struct {
		name    string
		filter  domain.AllocationTypeFilter
		want    domain.AllocationTypeFilter
		wantErr bool
	}{
		{"no filter", domain.AllocationTypeFilter{}, domain.AllocationTypeFilter{}, false},
		{"status canonicalised", domain.AllocationTypeFilter{Status: "ACTIVE"}, domain.AllocationTypeFilter{Status: "Active"}, false},
		{"engagement type", domain.AllocationTypeFilter{EngagementTypeID: 5}, domain.AllocationTypeFilter{EngagementTypeID: 5}, false},
		{"unknown status", domain.AllocationTypeFilter{Status: "Archived"}, domain.AllocationTypeFilter{}, true},
		{"negative id", domain.AllocationTypeFilter{EngagementTypeID: -1}, domain.AllocationTypeFilter{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeTypeRepo{}
			resp, err := NewTypeService(repo).ListAllocationTypes(context.Background(), tc.filter)
			if tc.wantErr {
				var ve *apierror.ValidationError
				if !errors.As(err, &ve) || repo.allocationCalled {
					t.Fatalf("err = %v, called = %v; want ValidationError and no query", err, repo.allocationCalled)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if repo.lastAllocation != tc.want {
				t.Errorf("repository got %+v, want %+v", repo.lastAllocation, tc.want)
			}
			if resp.AllocationTypes == nil {
				t.Error("AllocationTypes is nil; want an empty slice")
			}
		})
	}
}

func TestCachedTypeService_EngagementTypesHitRepositoryOnce(t *testing.T) {
	repo := &fakeTypeRepo{}
	svc := NewCachedTypeService(NewTypeService(repo))
	active := true
	for i := 0; i < 3; i++ {
		if _, err := svc.ListEngagementTypes(context.Background(), domain.EngagementTypeFilter{Type: "Internal", IsActive: &active}); err != nil {
			t.Fatal(err)
		}
	}
	if repo.engagementCalls != 1 {
		t.Errorf("repository called %d times, want 1", repo.engagementCalls)
	}
	// A different filter is a different entry.
	if _, err := svc.ListEngagementTypes(context.Background(), domain.EngagementTypeFilter{}); err != nil {
		t.Fatal(err)
	}
	if repo.engagementCalls != 2 {
		t.Errorf("repository called %d times, want 2", repo.engagementCalls)
	}
}
