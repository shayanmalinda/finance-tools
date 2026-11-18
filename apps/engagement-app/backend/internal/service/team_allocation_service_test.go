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
	"reflect"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

var teamTypes = []engagemententity.AllocationType{
	{ID: 40, AllocationTypeName: "Digital", Status: "ACTIVE", EngagementTypeID: 1, DefaultEngagementCode: sp("DIG-GEN"), Color: sp("#00aabb")},
	{ID: 41, AllocationTypeName: "Old", Status: "Deleted", EngagementTypeID: 1, DefaultEngagementCode: sp("OLD")},
	{ID: 12, AllocationTypeName: "Consulting", Status: "Active", EngagementTypeID: 6},
}

func TestTeamAllocationCreate(t *testing.T) {
	ent := &allocFakeEntity{types: teamTypes}
	svc := NewTeamAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	ctx := context.Background()
	p := allocPerms(21)
	req := domain.TeamAllocationCreate{Email: "jane@wso2.com", AllocationTypeID: 40, StartDate: "2026-11-10", EndDate: "2026-11-20", Comment: sp("bench")}

	for name, mut := range map[string]func(*domain.TeamAllocationCreate){
		"customer type": func(r *domain.TeamAllocationCreate) { r.AllocationTypeID = 12 },
		"deleted type":  func(r *domain.TeamAllocationCreate) { r.AllocationTypeID = 41 },
		"past 30":       func(r *domain.TeamAllocationCreate) { r.StartDate = "2026-10-05" },
		"order":         func(r *domain.TeamAllocationCreate) { r.StartDate = "2026-11-21" },
		"required":      func(r *domain.TeamAllocationCreate) { r.Email = "" },
		"unknown":       func(r *domain.TeamAllocationCreate) { r.Email = "ghost@wso2.com" },
	} {
		r := req
		mut(&r)
		if _, err := svc.Create(ctx, p, r); !isAllocValidation(err, "") {
			t.Errorf("%s: %v", name, err)
		}
	}
	r := req
	r.StartDate = "2026-10-05"
	if _, err := svc.Create(ctx, p, r); !isAllocValidation(err, "Dates cannot be less than  30 days from today") {
		t.Errorf("legacy past text: %v", err)
	}

	// Any overlap with an active allocation is refused (Team Allocations page).
	ent.timeline = []engagemententity.AllocationRecord{{ID: "A1", Email: "jane@wso2.com", StartDate: "2026-11-19", EndDate: "2026-11-25",
		EngagementID: sp("-1"), ClearanceStatus: sp("Confirmed")}}
	if _, ok := isAllocOverlap(must(svc.Create(ctx, p, req)), "This person is already allocated for the selected period of time."); !ok {
		t.Error("overlap should be refused")
	}
	ent.timeline = nil
	item, err := svc.Create(ctx, p, req)
	if err != nil {
		t.Fatal(err)
	}
	m := ent.bulk[0].NewAllocations[0]
	if m.EngagementID != "-1" || m.EngagementCode != "DIG-GEN" || *m.ClearanceStatus != "Confirmed" || *m.Comment != "bench" ||
		item.Label != "DIG-GEN: Digital" || !item.Editable || item.ID != "A0000000099" {
		t.Errorf("saved %+v / item %+v", m, item)
	}
}

func TestTeamAllocationUpdateAndDelete(t *testing.T) {
	ent := &allocFakeEntity{allocations: map[string]engagemententity.AllocationRecord{
		"D1":  {ID: "D1", Email: "jane@wso2.com", StartDate: "2026-11-01", EndDate: "2026-11-30", EngagementID: sp("-1"), Status: sp("Active")},
		"OLD": {ID: "OLD", Email: "jane@wso2.com", StartDate: "2026-09-01", EndDate: "2026-11-30", EngagementID: sp("-1.0"), Status: sp("Active")},
		"E1":  {ID: "E1", Email: "jane@wso2.com", StartDate: "2026-11-01", EndDate: "2026-11-30", EngagementID: sp("E07201"), Status: sp("Active")},
	}}
	svc := NewTeamAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	ctx := context.Background()
	p := allocPerms(21)

	if _, err := svc.Update(ctx, p, "OLD", domain.TeamAllocationUpdate{StartDate: sp("2026-11-02")}); !isAllocValidation(err, teamAllocMsgTooOld) {
		t.Errorf("old start: %v", err)
	}
	if _, err := svc.Update(ctx, p, "OLD", domain.TeamAllocationUpdate{EndDate: sp("2026-11-20"), Comment: sp("x")}); err != nil {
		t.Errorf("old allocation, recent end date: %v", err)
	}
	if _, err := svc.Update(ctx, p, "E1", domain.TeamAllocationUpdate{Comment: sp("x")}); err == nil {
		t.Error("engagement allocation: want 404")
	}
	if _, err := svc.Update(ctx, p, "D1", domain.TeamAllocationUpdate{StartDate: sp("2026-12-01")}); !isAllocValidation(err, allocMsgEndBeforeStart) {
		t.Errorf("order: %v", err)
	}
	if _, err := svc.Update(ctx, p, "D1", domain.TeamAllocationUpdate{StartDate: sp("2026-11-03")}); err != nil {
		t.Fatal(err)
	}
	last := ent.updates[len(ent.updates)-1]
	if *last.StartDate != "2026-11-03" || last.EndDate != nil || last.UpdatedBy != "mgr@wso2.com" {
		t.Errorf("update = %+v", last)
	}
	if err := svc.Delete(ctx, p, "OLD"); !isAllocValidation(err, teamAllocMsgTooOld) {
		t.Errorf("delete old: %v", err)
	}
	if err := svc.Delete(ctx, p, "D1"); err != nil || !reflect.DeepEqual(ent.deleted, []string{"D1"}) {
		t.Errorf("delete: %v %v", err, ent.deleted)
	}
	if _, err := svc.Search(ctx, domain.AllocationTimelineFilter{From: "2026-11-01", To: "2026-11-30"}); !isAllocValidation(err, "Select a Business Unit, Department or Team") {
		t.Errorf("search: %v", err)
	}
}

func TestTeamAllocationBulk_RowsMeetTheCreateRules(t *testing.T) {
	hr := allocTestHR()
	hr.org = []hrentity.OrgEmployee{{WorkEmail: "jane@wso2.com"}, {WorkEmail: "bob@wso2.com"},
		{WorkEmail: "engineer@wso2.com"}, {WorkEmail: "member@wso2.com"}, {WorkEmail: "eve@wso2.com"}}
	types := append([]engagemententity.AllocationType{
		{ID: 42, AllocationTypeName: "NoCode", Status: "Active", EngagementTypeID: 1, DefaultEngagementCode: sp(" ")}},
		teamTypes...)
	ent := &allocFakeEntity{types: types}
	svc := NewTeamAllocationService(allocTestDeps(ent, hr, &allocFakeSender{}))

	// Today is 2026-11-05, so 2026-10-06 is the earliest date allowed.
	res, err := svc.ValidateBulk(context.Background(), `jane,Dev,2026-10-01,2026-11-10,Digital
bob,Dev,2026-11-10,2026-11-12,Old
engineer,Dev,2026-11-10,2026-11-12,Consulting
member,Dev,2026-11-10,2026-11-12,NoCode
eve,Dev,2026-10-06,2026-11-12,Digital
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"jane@wso2.com: " + teamAllocMsgPast30,
		"The allocation type for allocation of bob@wso2.com is wrong",
		"The allocation type for allocation of engineer@wso2.com is wrong",
		"The allocation type for allocation of member@wso2.com has no default engagement code",
	}
	if !reflect.DeepEqual(res.Errors, want) {
		t.Errorf("errors =\n%q\nwant\n%q", res.Errors, want)
	}
	if len(res.Valid) != 1 || res.Valid[0].Email != "eve@wso2.com" {
		t.Errorf("valid = %+v", res.Valid)
	}
}

const teamCSV = `jane,Dev,2026-11-10,2026-11-12,Digital
jane,Dev,2026-11-11,2026-11-13,Digital
ghost,Dev,2026-11-10,2026-11-12,Digital
bob,Dev,2026-11-20,2026-11-10,Digital
bob,Dev,2026-11-01,2026-11-05,Nope
only,three,cols

engineer,Dev,2026-12-01,2026-12-02,Digital
`

func TestTeamAllocationBulk(t *testing.T) {
	hr := allocTestHR()
	hr.org = []hrentity.OrgEmployee{{WorkEmail: "jane@wso2.com"}, {WorkEmail: "bob@wso2.com"}, {WorkEmail: "engineer@wso2.com"}}
	ent := &allocFakeEntity{types: teamTypes, timeline: []engagemententity.AllocationRecord{
		{ID: "X", Email: "bob@wso2.com", StartDate: "2026-11-03", EndDate: "2026-11-04", AllocationTypeName: sp("Leave"), EngagementID: sp("-1")}}}
	svc := NewTeamAllocationService(allocTestDeps(ent, hr, &allocFakeSender{}))
	ctx := context.Background()

	res, err := svc.ValidateBulk(ctx, teamCSV)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Incorrect number of columns / empty data in these lines: 6",
		"The allocation is overlapped for jane@wso2.com 2026-11-10 <-> 2026-11-12 in csv",
		"The allocation is overlapped for jane@wso2.com 2026-11-11 <-> 2026-11-13 in csv",
		"The email id is invalid for ghost@wso2.com",
		"bob@wso2.com: Start Date [2026-11-20] is greater than End Date [2026-11-10]",
		"The allocation type for allocation of bob@wso2.com is wrong",
		"bob@wso2.com: New Allocation overlap at middle [2026-11-03 and 2026-11-04] with existing Leave allocation.",
	}
	if !reflect.DeepEqual(res.Errors, want) {
		t.Errorf("errors =\n%q\nwant\n%q", res.Errors, want)
	}
	if len(res.Valid) != 1 || res.Valid[0].Email != "engineer@wso2.com" || res.Valid[0].Line != 8 || len(ent.bulk) != 0 {
		t.Errorf("valid = %+v", res.Valid)
	}
	if hr.orgFilter.Statuses[1] != "Marked Leaver" {
		t.Errorf("employee filter = %+v", hr.orgFilter)
	}

	res, err = svc.UploadBulk(ctx, allocPerms(21), teamCSV)
	if err != nil || res.Saved == nil || *res.Saved != 1 {
		t.Fatalf("upload: %+v %v", res, err)
	}
	b := ent.bulk[0]
	if len(b.NewAllocations) != 1 || b.NewAllocations[0].EngagementID != "-1.0" || b.NewAllocations[0].EngagementCode != "DIG-GEN" ||
		*b.NewAllocations[0].ClearanceStatus != "Confirmed" || len(b.AllocationLogs) != 1 || b.AllocationLogs[0].AllocationTypeID != 40 {
		t.Errorf("bulk = %+v", b)
	}
	if _, err := svc.ValidateBulk(ctx, "a,b\n"); !isAllocValidation(err, "Cannot proceed. Please recheck the csv file for empty data / incorrect number of columns") {
		t.Errorf("all bad: %v", err)
	}
}
