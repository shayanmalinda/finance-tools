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
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

type settingsFakeEntity struct {
	engagementFakeEntity
	replaced   []int
	typeFilter engagemententity.AllocationTypeFilter
}

func (f *settingsFakeEntity) RoleSettings(context.Context, domain.App) (engagemententity.RoleSettingsLists, error) {
	return engagemententity.RoleSettingsLists{Roles: []engagemententity.RoleWithPrivilegeIDs{{ID: 1, Name: "wso2-employees"}},
		Privileges: []engagemententity.Privilege{{ID: 1, Name: "Dashboard Page"}}}, nil
}

func (f *settingsFakeEntity) RoleReplacePrivileges(_ context.Context, _ domain.App, id int, ids []int, _ string) (engagemententity.RoleWithPrivilegeIDs, error) {
	f.replaced = ids
	return engagemententity.RoleWithPrivilegeIDs{ID: id, Name: "r", PrivilegeIDs: ids}, nil
}

func (f *settingsFakeEntity) AllocationTypes(_ context.Context, flt engagemententity.AllocationTypeFilter) ([]engagemententity.AllocationType, error) {
	f.typeFilter = flt
	return engagementTestRef().AllocationTypes, nil
}

func TestSettingsRolesAndTypes(t *testing.T) {
	f := &settingsFakeEntity{}
	svc := NewSettingsService(f, NewReferenceService(f, engagementFakeSales{}, engagementFakeHR{}))
	ctx := context.Background()
	// Role Settings need both super admin groups, then each app's privilege: Allocations 23, Travel 29.
	super := func(p domain.Perms) domain.Perms {
		p.AllocRoles = []string{"app-allocations-admin-super-stg"}
		p.TravelRoles = []string{"app-travel-admin-super-stg"}
		return p
	}
	var fe *apierror.ForbiddenError
	onlyAlloc := engagementPerms("a@wso2.com", []int{23}, []int{29})
	onlyAlloc.AllocRoles = []string{"app-allocations-admin-super"}
	if _, err := svc.RoleSettings(ctx, onlyAlloc, domain.AppAllocations); !errors.As(err, &fe) {
		t.Fatalf("one super admin group: %v", err)
	}
	if _, err := svc.RoleSettings(ctx, super(engagementPerms("a@wso2.com", []int{23}, nil)), domain.AppTravel); !errors.As(err, &fe) {
		t.Fatalf("travel with alloc 23: %v", err)
	}
	rs, err := svc.RoleSettings(ctx, super(engagementPerms("a@wso2.com", nil, []int{29})), domain.AppTravel)
	if err != nil || len(rs.Roles) != 1 || rs.Roles[0].PrivilegeIDs == nil {
		t.Fatalf("rs %+v err %v", rs, err)
	}
	var nf *apierror.NotFoundError
	if _, err := svc.RoleSettings(ctx, super(engagementPerms("a@wso2.com", []int{23}, nil)), "finance"); !errors.As(err, &nf) {
		t.Fatalf("unknown app: %v", err)
	}
	var ve *apierror.ValidationError
	if _, err := svc.UpdateRolePrivileges(ctx, super(engagementPerms("a@wso2.com", []int{23}, nil)), domain.AppAllocations, 3, nil); !errors.As(err, &ve) {
		t.Fatalf("nil ids: %v", err)
	}
	if r, err := svc.UpdateRolePrivileges(ctx, super(engagementPerms("a@wso2.com", []int{23}, nil)), domain.AppAllocations, 3, []int{}); err != nil || r.PrivilegeIDs == nil {
		t.Fatalf("empty ids: %+v %v", r, err)
	}
	ts, err := svc.AllocationTypes(ctx, 1, "dig")
	if err != nil || len(ts) != 1 || ts[0].ID != 10 || f.typeFilter.Status != "Active" || f.typeFilter.EngagementTypeID != 1 {
		t.Fatalf("types %+v filter %+v err %v", ts, f.typeFilter, err)
	}
}

func TestReferenceLookups(t *testing.T) {
	f := &engagementFakeEntity{}
	refs := NewReferenceService(f, engagementFakeSales{}, engagementFakeHR{})
	ctx := context.Background()
	rd, err := refs.ReferenceData(ctx)
	if err != nil || len(rd.Countries) != 247 || len(rd.TimeZones) != 424 || rd.CostTypes[0] != "Air Ticket" ||
		len(rd.ClearanceTransitions["Allocation Cancelled"]) != 4 || rd.VisaTypes[2] != "Permenent Residence" {
		t.Fatalf("reference data err %v", err)
	}
	var ve *apierror.ValidationError
	if _, err := refs.Accounts(ctx, "a", 0); !errors.As(err, &ve) {
		t.Fatalf("1 char: %v", err)
	}
	if _, err := refs.Accounts(ctx, "acme", 26); !errors.As(err, &ve) {
		t.Fatalf("limit: %v", err)
	}
	accts, err := refs.Accounts(ctx, "acme", 0)
	if err != nil || len(accts) != 1 || *accts[0].Country != "United States of America" || *accts[0].Address != "1 Main St\nUnited States of America" {
		t.Fatalf("accounts %+v err %v", accts, err)
	}
	opps, err := refs.Opportunities(ctx, "001A", 6)
	if err != nil || len(opps) != 1 || len(opps[0].LineItems) != 1 || *opps[0].Owner != "Alex" {
		t.Fatalf("opps %+v err %v", opps, err)
	}
	if opps, _ := refs.Opportunities(ctx, "001A", 3); len(opps[0].LineItems) != 0 {
		t.Errorf("pre sale needs no line items")
	}
	if c, err := refs.Campaign(ctx, "7015g00000ApD26AAF", "WSO2Con"); err != nil || c == nil || c.Name != "WSO2Con Asia 2027" {
		t.Fatalf("campaign %+v err %v", c, err)
	}
}
