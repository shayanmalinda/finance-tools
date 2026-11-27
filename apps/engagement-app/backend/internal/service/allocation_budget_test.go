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
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// budgetAllocSetup is an internal engagement of 2027 whose allocation type
// (12) maps to SALES_APAC, with the budget service over a fake entity.
func budgetAllocSetup(enabled bool) (AllocationService, *allocFakeEntity, *budgetFake, *BudgetService, *allocFakeSender) {
	eng := allocTestEngagement(1)
	eng.StartDate, eng.EndDate = sp("2027-01-01"), sp("2027-12-31")
	ent := &allocFakeEntity{engagement: eng}
	bf := newBudgetFake()
	snd := &allocFakeSender{}
	bs := newBudgetSvc(bf, nil, snd, 0)
	deps := allocTestDeps(ent, allocTestHR(), &allocFakeSender{})
	if enabled {
		deps.Budgets = bs
	}
	return NewAllocationService(deps), ent, bf, bs, snd
}

func budgetCreateReq(line *int) domain.AllocationCreateRequest {
	return domain.AllocationCreateRequest{ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2027-02-01"), EndDate: sp("2027-02-05"),
		TravelRequired: bp(true), ClearanceStatus: sp("Confirmed"), BudgetLineID: line}
}

func TestAllocationCreate_BudgetLineChosenAndSuggested(t *testing.T) {
	svc, ent, _, _, _ := budgetAllocSetup(true)
	ctx := context.Background()

	out, err := svc.Create(ctx, allocPerms(16), "E07201", budgetCreateReq(ip(1)))
	if err != nil {
		t.Fatal(err)
	}
	tb := ent.bulk[0].TravelBudget
	if tb == nil || *tb.BudgetLineID != 1 || *tb.SuggestedLineID != 2 || tb.SuggestionSource != "ALLOCATION_TYPE" || *tb.ChosenBy != "USER" {
		t.Errorf("chosen: %+v", tb)
	}
	v, _ := out.Budget.(*domain.TripBudgetView)
	if v == nil || v.JobNumber != "TRV-2026-11-0001" || v.Line.Name != "Sales: EU" || v.Suggested.Name != "Sales: APAC" || v.NeedsLine || *v.Year != 2027 {
		t.Errorf("view = %+v", v)
	}

	// An old client sends none: the suggestion is applied by the system.
	if _, err = svc.Create(ctx, allocPerms(16), "E07201", budgetCreateReq(nil)); err != nil {
		t.Fatal(err)
	}
	tb = ent.bulk[1].TravelBudget
	if *tb.BudgetLineID != 2 || *tb.SuggestedLineID != 2 || *tb.ChosenBy != "SYSTEM" || tb.SuggestionSource != "ALLOCATION_TYPE" {
		t.Errorf("suggested: %+v", tb)
	}
}

func TestAllocationCreate_BudgetLineOfAnotherYearIsRefused(t *testing.T) {
	svc, ent, bf, _, _ := budgetAllocSetup(true)
	bf.lines[0].Status = "Retired"
	for _, id := range []int{99, 1} {
		_, err := svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(ip(id)))
		if !isAllocValidation(err, "Choose a budget line of the trip's year.") {
			t.Errorf("line %d: %v", id, err)
		}
	}
	if len(ent.bulk) != 0 {
		t.Error("nothing should be written")
	}
}

func TestAllocationCreate_NoSuggestionAsksTheBudgetOwnerOnce(t *testing.T) {
	svc, ent, bf, bs, snd := budgetAllocSetup(true)
	ent.engagement.AllocationTypeID = ip(14) // ENGAGEMENT rule with no line on the engagement
	out, err := svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(nil))
	if err != nil {
		t.Fatal(err)
	}
	bs.wait()
	tb := ent.bulk[0].TravelBudget
	if tb.BudgetLineID != nil || tb.SuggestedLineID != nil || tb.SuggestionSource != "NONE" || tb.ChosenBy != nil {
		t.Errorf("payload = %+v", tb)
	}
	if v, _ := out.Budget.(*domain.TripBudgetView); v == nil || !v.NeedsLine || v.Line != nil {
		t.Errorf("view = %+v", out.Budget)
	}
	if len(snd.sent) != 1 || snd.sent[0].To[0] != "owner@example.com" || snd.sent[0].NotificationType != "travelBudgetLineNeeded" ||
		snd.sent[0].ReferenceID != "TRV-2026-11-0001" {
		t.Errorf("emails = %+v", snd.sent)
	}
	// The engagement's own line is used when set.
	bf.engLine = map[string]string{"E07201": "GA_BOARD"}
	if _, err = svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(nil)); err != nil {
		t.Fatal(err)
	}
	bs.wait()
	if tb = ent.bulk[1].TravelBudget; *tb.BudgetLineID != 3 || tb.SuggestionSource != "ENGAGEMENT" || len(snd.sent) != 1 {
		t.Errorf("engagement line: %+v, %d emails", tb, len(snd.sent))
	}
}

func TestAllocationCreate_OutsideEveryBudgetYearSendsNoEmail(t *testing.T) {
	svc, ent, _, bs, snd := budgetAllocSetup(true)
	ent.engagement.StartDate = sp("2026-11-01")
	req := budgetCreateReq(ip(1))
	req.StartDate, req.EndDate = sp("2026-11-10"), sp("2026-11-12")
	out, err := svc.Create(context.Background(), allocPerms(16), "E07201", req)
	if err != nil {
		t.Fatal(err)
	}
	bs.wait()
	tb := ent.bulk[0].TravelBudget
	if tb == nil || tb.BudgetLineID != nil || tb.SuggestionSource != "NONE" || len(snd.sent) != 0 {
		t.Errorf("payload %+v emails %d", tb, len(snd.sent))
	}
	if b, _ := json.Marshal(out); !strings.Contains(string(b), `"budget":null`) {
		t.Errorf("response = %s", b)
	}
}

func TestAllocationCreate_BudgetFailureNeverFailsTheAllocation(t *testing.T) {
	svc, ent, bf, bs, snd := budgetAllocSetup(true)
	bf.failLines = errors.New("entity down")
	out, err := svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(ip(1)))
	if err != nil {
		t.Fatalf("allocation failed over budgets: %v", err)
	}
	bs.wait()
	tb := ent.bulk[0].TravelBudget
	if tb == nil || tb.BudgetLineID != nil || tb.SuggestedLineID != nil || tb.SuggestionSource != "NONE" || tb.ChosenBy != nil || len(snd.sent) != 1 {
		t.Errorf("payload = %+v, %d emails (the owner is asked to choose)", tb, len(snd.sent))
	}
	if out.TravelRequest == nil {
		t.Error("trip lost")
	}
	// Rules unreadable: the allocator's own choice still stands.
	bf.failLines, bf.failRules = nil, errors.New("down")
	if _, err = svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(ip(2))); err != nil {
		t.Fatal(err)
	}
	if tb = ent.bulk[1].TravelBudget; *tb.BudgetLineID != 2 || *tb.ChosenBy != "USER" || tb.SuggestionSource != "NONE" {
		t.Errorf("rules down: %+v", tb)
	}
}

// Flag off: the entity request and the response are exactly what they were
// before travel budgets existed, even when the client sends a budgetLineId.
func TestAllocationCreate_BudgetsOffIsByteIdentical(t *testing.T) {
	svc, ent, _, _, _ := budgetAllocSetup(false)
	out, err := svc.Create(context.Background(), allocPerms(16), "E07201", budgetCreateReq(ip(1)))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(ent.bulk[0])
	const wantReq = `{"newAllocations":[{"email":"jane@wso2.com","allocationTypeId":12,"status":"Active","allocationStatus":"Active",` +
		`"clearanceStatus":"Confirmed","startDate":"2027-02-01","endDate":"2027-02-05","startTime":null,"endTime":null,"timeZone":null,` +
		`"engagementId":"E07201","engagementCode":"NWB-CO","isTravelRequired":"Yes","isRecurring":"No","comment":null}],` +
		`"updateAllocations":null,"allocationLogs":null,"requestStatusInProgress":"E07201","createdBy":"mgr@wso2.com"}`
	if string(req) != wantReq {
		t.Errorf("entity request changed:\n%s\nwant\n%s", req, wantReq)
	}
	resp, _ := json.Marshal(out)
	const wantResp = `{"allocation":{"id":"A0000000099","engagementId":"E07201","consultant":{"email":"jane@wso2.com","name":null},"partner":null,` +
		`"startDate":"2027-02-01","endDate":"2027-02-05","hourly":null,"travelRequired":true,"clearanceStatus":"Confirmed","comment":null,` +
		`"addedBy":"mgr@wso2.com","addedOn":null,"travelRequest":{"jobNumber":"TRV-2026-11-0001","canOpen":false},"actions":{"changeStatus":false}},` +
		`"travelRequest":"TRV-2026-11-0001","emailsSent":true}`
	if string(resp) != wantResp {
		t.Errorf("response changed:\n%s\nwant\n%s", resp, wantResp)
	}
}

func TestAllocationUpdateStatus_BudgetLineOnTheTripItCreates(t *testing.T) {
	svc, ent, _, bs, _ := budgetAllocSetup(true)
	ent.allocations = map[string]engagemententity.AllocationRecord{"A1": {ID: "A1", Email: "jane@wso2.com", StartDate: "2027-02-01", EndDate: "2027-02-05",
		ClearanceStatus: sp("Accepted by Consultant"), IsTravelRequired: sp("Yes"), EngagementID: sp("E07201"),
		Engagement: &engagemententity.AllocationEngagementRef{ID: "E07201", EngagementTypeID: 1, StartDate: sp("2027-01-01"), EndDate: sp("2027-12-31")}}}
	ent.travelJob = sp("TRV-2027-02-0001")
	out, err := svc.UpdateStatus(context.Background(), allocPerms(20), "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed"), BudgetLineID: ip(1)})
	if err != nil {
		t.Fatal(err)
	}
	bs.wait()
	tb := ent.updates[0].TravelBudget
	if tb == nil || *tb.BudgetLineID != 1 || *tb.ChosenBy != "USER" || *tb.SuggestedLineID != 2 {
		t.Errorf("update payload = %+v", tb)
	}
	if v, _ := out.Budget.(*domain.TripBudgetView); v == nil || v.JobNumber != "TRV-2027-02-0001" {
		t.Errorf("view = %+v", out.Budget)
	}
	// A status that creates no trip carries no budget and answers null.
	ent.allocations["A1"] = func() engagemententity.AllocationRecord {
		a := ent.allocations["A1"]
		a.IsTravelRequired = sp("No")
		return a
	}()
	ent.travelJob = nil
	if _, err = svc.UpdateStatus(context.Background(), allocPerms(20), "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed")}); err != nil {
		t.Fatal(err)
	}
	if ent.updates[1].TravelBudget != nil {
		t.Error("no trip, no travelBudget")
	}
	// Off: nothing is sent.
	off, ent2, _, _, _ := budgetAllocSetup(false)
	ent2.allocations = ent.allocations
	ent2.allocations["A1"] = engagemententity.AllocationRecord{ID: "A1", Email: "jane@wso2.com", StartDate: "2027-02-01", EndDate: "2027-02-05",
		ClearanceStatus: sp("Accepted by Consultant"), IsTravelRequired: sp("Yes"), EngagementID: sp("E07201"),
		Engagement: &engagemententity.AllocationEngagementRef{ID: "E07201", EngagementTypeID: 1, StartDate: sp("2027-01-01"), EndDate: sp("2027-12-31")}}
	o, err := off.UpdateStatus(context.Background(), allocPerms(20), "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed"), BudgetLineID: ip(1)})
	if err != nil || ent2.updates[0].TravelBudget != nil || o.Budget != nil {
		t.Errorf("off: %v %+v %v", err, ent2.updates[0].TravelBudget, o.Budget)
	}
	if b, _ := json.Marshal(ent2.updates[0]); strings.Contains(string(b), "travelBudget") {
		t.Errorf("payload = %s", b)
	}
}

func TestAllocStatusCreatesTrip(t *testing.T) {
	a := engagemententity.AllocationRecord{Email: "j@wso2.com", IsTravelRequired: sp("Yes"), Engagement: &engagemententity.AllocationEngagementRef{}}
	for _, c := range []struct {
		cur, next string
		want      bool
	}{
		{"Accepted by Consultant", "Confirmed", true}, {"Confirmed - Visa Pending", "Confirmed", false},
		{"Accepted by Consultant", "Confirmed - Visa Pending", true}, {"Confirmed", "Confirmed", false},
		{"Confirmed", "Allocation Cancelled", false},
	} {
		if got := allocStatusCreatesTrip(a, c.cur, c.next); got != c.want {
			t.Errorf("%s to %s = %v", c.cur, c.next, got)
		}
	}
	a.IsTravelRequired = sp("No")
	if allocStatusCreatesTrip(a, "Accepted by Consultant", "Confirmed") {
		t.Error("travel not required")
	}
}
