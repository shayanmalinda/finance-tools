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
	"reflect"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// fakeEntity serves fixed role-privilege maps per app and counts calls. A
// nil map for an app makes that app's call fail.
type fakeEntity struct {
	engagemententity.Client // unimplemented methods panic if called
	maps                    map[domain.App]map[string][]int
	calls                   map[domain.App]int
	agencyCalls             int
}

func (f *fakeEntity) TravelAgency(_ context.Context, email string) (*domain.TravelAgency, error) {
	f.agencyCalls++
	if email == "agent@classic.example" {
		return &domain.TravelAgency{ID: 2, Name: "Classic Travels"}, nil
	}
	return nil, nil
}

func (f *fakeEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if f.calls == nil {
		f.calls = map[domain.App]int{}
	}
	f.calls[app]++
	m, ok := f.maps[app]
	if !ok {
		return nil, &apierror.UpstreamError{Service: "engagement-entity", Msg: "unreachable"}
	}
	return m, nil
}

// Production-like maps (role_privilege as of the 2026-10-05 dump, trimmed).
// Privilege 1 exists in both apps and means different things in each.
func legacyMaps() map[domain.App]map[string][]int {
	return map[domain.App]map[string][]int{
		domain.AppAllocations: {
			"wso2-employees":                            {1},
			"app-allocations-account-manager":           {1, 2, 3, 5, 8},
			"app-allocations-manager-cust-eng-pre-sale": {33},
			"app-allocations-admin-super":               {1, 2, 3, 4, 5, 6, 7, 8},
		},
		domain.AppTravel: {
			"wso2-employees":         {1, 2},
			"app-travel-admin-super": {1, 2, 3, 4},
			"app-travel-manager-po":  {2, 3},
		},
	}
}

func TestUserInfo_PrivilegesPerAppNeverMerged(t *testing.T) {
	tests := []struct {
		name      string
		groups    []string
		wantAlloc []int
		wantTrav  []int
	}{
		{"employee only", []string{"wso2-employees"}, []int{1}, []int{1, 2}},
		{"allocations role only adds to allocations", []string{"wso2-employees", "app-allocations-manager-cust-eng-pre-sale"},
			[]int{1, 33}, []int{1, 2}},
		{"travel role only adds to travel", []string{"wso2-employees", "app-travel-manager-po"}, []int{1}, []int{1, 2, 3}},
		{"overlapping roles are de-duplicated and sorted", []string{"app-allocations-admin-super", "app-allocations-account-manager", "wso2-employees"},
			[]int{1, 2, 3, 4, 5, 6, 7, 8}, []int{1, 2}},
		{"group names match case-insensitively, as MySQL IN does", []string{"WSO2-Employees", " app-travel-admin-super "},
			[]int{1}, []int{1, 2, 3, 4}},
		{"unknown groups grant nothing", []string{"some-other-app-group"}, []int{}, []int{}},
		{"no groups", nil, []int{}, []int{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewAccessService(&fakeEntity{maps: legacyMaps()})
			got, err := svc.UserInfo(context.Background(), reqcontext.Caller{Email: "jane@wso2.com", Groups: tc.groups})
			if err != nil {
				t.Fatalf("UserInfo: %v", err)
			}
			if got.Email != "jane@wso2.com" {
				t.Errorf("email = %q", got.Email)
			}
			if !reflect.DeepEqual(got.Allocations.Privileges, tc.wantAlloc) {
				t.Errorf("allocations = %v, want %v", got.Allocations.Privileges, tc.wantAlloc)
			}
			if !reflect.DeepEqual(got.Travel.Privileges, tc.wantTrav) {
				t.Errorf("travel = %v, want %v", got.Travel.Privileges, tc.wantTrav)
			}
		})
	}
}

// No privileges serialise as [], never null: the webapp tells "no access"
// from a malformed answer.
func TestUserInfo_EmptyIsArrayNotNull(t *testing.T) {
	svc := NewAccessService(&fakeEntity{maps: legacyMaps()})
	got, err := svc.UserInfo(context.Background(), reqcontext.Caller{Email: "x@wso2.com"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	const want = `{"email":"x@wso2.com","allocations":{"privileges":[],"ownPrivileges":[],"roles":[]},"travel":{"privileges":[],"ownPrivileges":[],"roles":[],"agency":null},"workQueues":{"invoices":[],"perDiemPayments":[],"netsuiteSyncFailures":false}}`
	if string(b) != want {
		t.Errorf("JSON = %s, want %s", b, want)
	}
}

// If either app cannot be read the whole answer fails, so the webapp shows
// "could not check access" with Retry rather than "no access".
func TestUserInfo_EntityFailureFailsTheWholeAnswer(t *testing.T) {
	maps := legacyMaps()
	delete(maps, domain.AppTravel)
	svc := NewAccessService(&fakeEntity{maps: maps})
	_, err := svc.UserInfo(context.Background(), reqcontext.Caller{Email: "jane@wso2.com", Groups: []string{"wso2-employees"}})
	var ue *apierror.UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UpstreamError", err)
	}
}

func TestUserInfo_RequiresEmail(t *testing.T) {
	svc := NewAccessService(&fakeEntity{maps: legacyMaps()})
	_, err := svc.UserInfo(context.Background(), reqcontext.Caller{Groups: []string{"wso2-employees"}})
	var ue *apierror.UnauthorizedError
	if !errors.As(err, &ue) {
		t.Errorf("err = %v, want UnauthorizedError", err)
	}
}

func TestRolePrivilegeMapsAreCachedPerApp(t *testing.T) {
	entity := &fakeEntity{maps: legacyMaps()}
	svc := NewAccessService(entity)
	caller := reqcontext.Caller{Email: "jane@wso2.com", Groups: []string{"wso2-employees"}}
	for i := 0; i < 3; i++ {
		if _, err := svc.UserInfo(context.Background(), caller); err != nil {
			t.Fatal(err)
		}
	}
	for _, app := range domain.Apps {
		if entity.calls[app] != 1 {
			t.Errorf("%s map fetched %d times, want 1", app, entity.calls[app])
		}
	}
}

// A failed fetch is not cached: the next request tries again.
func TestRolePrivilegeFailureIsNotCached(t *testing.T) {
	entity := &fakeEntity{maps: map[domain.App]map[string][]int{}}
	svc := NewAccessService(entity)
	for i := 0; i < 2; i++ {
		if _, err := svc.Privileges(context.Background(), domain.AppTravel, []string{"wso2-employees"}); err == nil {
			t.Fatal("want an error")
		}
	}
	if entity.calls[domain.AppTravel] != 2 {
		t.Errorf("fetched %d times, want 2", entity.calls[domain.AppTravel])
	}
}

func TestHasAnyPrivilege(t *testing.T) {
	svc := NewAccessService(&fakeEntity{maps: legacyMaps()})
	groups := []string{"wso2-employees", "app-allocations-manager-cust-eng-pre-sale"}
	tests := []struct {
		app  domain.App
		ids  []int
		want bool
	}{
		{domain.AppAllocations, []int{33}, true},
		{domain.AppAllocations, []int{32, 33}, true},
		{domain.AppAllocations, []int{32}, false},
		{domain.AppAllocations, nil, false},
		// Travel privilege 3 is not Allocations privilege 3, and vice versa.
		{domain.AppTravel, []int{33}, false},
		{domain.AppTravel, []int{2}, true},
	}
	for _, tc := range tests {
		got, err := svc.HasAnyPrivilege(context.Background(), tc.app, groups, tc.ids...)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("HasAnyPrivilege(%s, %v) = %v, want %v", tc.app, tc.ids, got, tc.want)
		}
	}
}

// Work queues ignore the super admin group: ownPrivileges leave its grants
// out, and roles name the groups held.
func TestPerms_OwnPrivilegesAndRoles(t *testing.T) {
	svc := NewAccessService(&fakeEntity{maps: legacyMaps()})
	p, err := svc.Perms(context.Background(), reqcontext.Caller{Email: "a@wso2.com",
		Groups: []string{"wso2-employees", "app-travel-admin-super", "app-travel-manager-po"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Travel.IDs(), []int{1, 2, 3, 4}) || !reflect.DeepEqual(p.TravelOwn.IDs(), []int{1, 2, 3}) {
		t.Errorf("travel = %v own = %v", p.Travel.IDs(), p.TravelOwn.IDs())
	}
	if !reflect.DeepEqual(p.TravelRoles, []string{"app-travel-admin-super", "app-travel-manager-po", "wso2-employees"}) {
		t.Errorf("roles = %v", p.TravelRoles)
	}
	if !p.Passes(domain.Gate{domain.AppTravel: {4}}) || p.Passes(domain.Gate{domain.AppAllocations: {4}}) {
		t.Error("Passes mixes apps")
	}
}

// Staging role names carry "-stg"; the super admin group is still left out
// of the own privileges.
func TestPerms_OwnPrivilegesIgnoreEnvironmentSuffix(t *testing.T) {
	svc := NewAccessService(&fakeEntity{maps: map[domain.App]map[string][]int{
		domain.AppAllocations: {"wso2-employees": {1}, "app-allocations-admin-super-stg": {1, 2, 3}},
		domain.AppTravel:      {"wso2-employees": {49}, "app-travel-admin-super-stg": {32, 34, 40, 49}},
	}})
	p, err := svc.Perms(context.Background(), reqcontext.Caller{Email: "a@wso2.com",
		Groups: []string{"wso2-employees", "app-allocations-admin-super-stg", "app-travel-admin-super-stg"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.AllocOwn.IDs(), []int{1}) || !reflect.DeepEqual(p.TravelOwn.IDs(), []int{49}) || !p.Travel.Has(34) {
		t.Errorf("alloc own %v travel own %v travel %v", p.AllocOwn.IDs(), p.TravelOwn.IDs(), p.Travel.IDs())
	}
}

// The agency is looked up (and cached) only for holders of Travel 18.
func TestPerms_AgencyOnlyForAgents(t *testing.T) {
	maps := legacyMaps()
	maps[domain.AppTravel]["app-travel-agent-classic"] = []int{3, 4, 18}
	e := &fakeEntity{maps: maps}
	svc := NewAccessService(e)
	p, err := svc.Perms(context.Background(), reqcontext.Caller{Email: "jane@wso2.com", Groups: []string{"wso2-employees"}})
	if err != nil || p.Agency != nil || e.agencyCalls != 0 {
		t.Fatalf("employee: agency %v, calls %d, err %v", p.Agency, e.agencyCalls, err)
	}
	for i := 0; i < 2; i++ {
		p, err = svc.Perms(context.Background(), reqcontext.Caller{Email: "agent@classic.example", Groups: []string{"app-travel-agent-classic"}})
		if err != nil || p.Agency == nil || p.Agency.Name != "Classic Travels" || !p.IsAgent() {
			t.Fatalf("agent: %+v, %v", p, err)
		}
	}
	if e.agencyCalls != 1 {
		t.Errorf("agency looked up %d times, want 1", e.agencyCalls)
	}
	info, _ := svc.UserInfo(context.Background(), reqcontext.Caller{Email: "agent@classic.example", Groups: []string{"app-travel-agent-classic"}})
	if info.Travel.Agency == nil || info.Travel.Agency.Name != "Classic Travels" || info.Travel.Agency.ID != 2 {
		t.Errorf("user-info agency = %v", info.Travel.Agency)
	}
}

// The invoice queues follow the legacy rule: the Travel super admin (32, 33
// and 34 through the super admin group only) gets no queue; a finance
// approver who also holds 32 through any group acts at the PO step only.
func TestWorkQueuesFor(t *testing.T) {
	mk := func(all, own []int) domain.Perms {
		return domain.Perms{Travel: domain.NewPrivilegeSet(all), TravelOwn: domain.NewPrivilegeSet(own)}
	}
	tests := []struct {
		name    string
		p       domain.Perms
		inv     []string
		pay     []string
		failure bool
	}{
		{"super admin only", mk([]int{32, 33, 34, 39, 40, 41}, nil), []string{"all"}, []string{"all"}, false},
		{"travel team (PO)", mk([]int{32, 39, 41}, []int{32}), []string{"approve-po", "all"}, []string{"all"}, false},
		{"finance approver", mk([]int{33, 34, 39, 40, 41}, []int{33, 34, 39, 40, 41}), []string{"approve", "push", "all"}, []string{"push", "all"}, true},
		{"finance approver who is also super admin", mk([]int{32, 33, 34, 39, 40, 41}, []int{33, 34, 39, 40, 41}), []string{"push", "all"}, []string{"push", "all"}, true},
	}
	for _, tc := range tests {
		q := WorkQueuesFor(tc.p)
		if !reflect.DeepEqual(q.Invoices, tc.inv) || !reflect.DeepEqual(q.PerDiemPayments, tc.pay) || q.NetSuiteSyncFailures != tc.failure {
			t.Errorf("%s: %+v", tc.name, q)
		}
	}
}
