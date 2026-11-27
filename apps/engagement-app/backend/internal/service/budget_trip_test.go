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
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func TestBudgetTripViewAndSetLine(t *testing.T) {
	f := newBudgetFake()
	f.trips = map[string]engagemententity.BudgetTrip{
		"TRV-1": {JobNumber: "TRV-1", Status: sp("New"), StartDate: sp("2027-02-01"), HasRow: false},
		"TRV-2": {JobNumber: "TRV-2", Status: sp("New"), StartDate: sp("2027-02-01"), BudgetYear: ip(2027), BudgetLineID: ip(1), SuggestedLineID: ip(2),
			SuggestionSource: sp("REGION"), ChosenBy: sp("USER"), HasRow: true},
		"TRV-3": {JobNumber: "TRV-3", Status: sp("New"), StartDate: sp("2026-02-01")},
	}
	svc := newBudgetSvc(f, nil, &allocFakeSender{}, 0)
	ctx := context.Background()
	// A trip with no row, in a year: needs a line.
	if v := svc.TripView(ctx, budgetPerms(35), "TRV-1"); v == nil || !v.NeedsLine || v.Source != "NONE" || v.CanEdit || *v.Year != 2027 {
		t.Errorf("no row = %+v", v)
	}
	if v := svc.TripView(ctx, budgetPerms(6), "TRV-2"); v == nil || v.Line.Name != "Sales: EU" || v.Suggested.Name != "Sales: APAC" || v.NeedsLine || !v.CanEdit || v.Source != "REGION" {
		t.Errorf("with row = %+v", v)
	}
	if v := svc.TripView(ctx, budgetPerms(35), "TRV-3"); v != nil {
		t.Errorf("out of every year = %+v", v)
	}
	if v := svc.TripView(ctx, budgetPerms(2), "TRV-2"); v != nil {
		t.Errorf("no budget privilege = %+v", v)
	}
	if v := svc.TripView(ctx, budgetPerms(35), "TRV-9"); v != nil {
		t.Errorf("unknown trip = %+v", v)
	}
	v, err := svc.SetTripLine(ctx, budgetPerms(29), "TRV-1", ip(2))
	if err != nil || v.Line == nil || v.Line.LineID != 2 || *v.ChosenBy != "USER" || v.NeedsLine {
		t.Errorf("set = %+v %v", v, err)
	}
	if v, err = svc.SetTripLine(ctx, budgetPerms(29), "TRV-1", nil); err != nil || !v.NeedsLine {
		t.Errorf("clear = %+v %v", v, err)
	}
}

func TestBudgetNeedsLine(t *testing.T) {
	f := newBudgetFake()
	f.needs = engagemententity.BudgetNeedsLinePage{Total: 7, Items: []engagemententity.BudgetTrip{
		{JobNumber: "TRV-1", TravellerEmail: sp("jane@wso2.com"), SuggestedLineID: ip(1), SuggestionSource: sp("REGION"), BudgetYear: ip(2027)}}}
	svc := NewBudgetService(f, allocTestHR(), &allocFakeSender{}, nil, BudgetOptions{Now: func() time.Time { return budgetNow }})
	page, err := svc.NeedsLine(context.Background(), budgetPerms(35), 0, 50, 0)
	if err != nil || page.Total != 7 || len(page.Items) != 1 || *page.Items[0].TravellerName != "Jane Doe" || page.Items[0].Suggested.Name != "Sales: EU" {
		t.Errorf("page = %+v %v", page, err)
	}
	if _, err := svc.NeedsLine(context.Background(), budgetPerms(29), 0, 50, 0); !errorsIsForbidden(err) {
		t.Errorf("travel 29 view: %v", err)
	}
}

func TestBudgetEngagementLine(t *testing.T) {
	f := newBudgetFake()
	svc := newBudgetSvc(f, nil, &allocFakeSender{}, 0)
	ctx := context.Background()
	if svc.EngagementLine(ctx, "E1") != nil {
		t.Error("unset line")
	}
	if err := svc.CheckEngagementLine(ctx, "NOPE"); !isAllocValidation(err, "Choose a valid travel budget line.") {
		t.Errorf("unknown code: %v", err)
	}
	if err := svc.CheckEngagementLine(ctx, ""); err != nil {
		t.Errorf("blank clears: %v", err)
	}
	if err := svc.SetEngagementLine(ctx, budgetPerms(), "E1", "GA_BOARD"); err != nil {
		t.Fatal(err)
	}
	if l := svc.EngagementLine(ctx, "E1"); l == nil || l.Code != "GA_BOARD" || l.Name != "G&A: Board meeting travel" {
		t.Errorf("line = %+v", l)
	}
	if err := svc.SetEngagementLine(ctx, budgetPerms(), "E1", ""); err != nil || svc.EngagementLine(ctx, "E1") != nil {
		t.Errorf("clear: %v", err)
	}
}

// With budgets off the pages carry no budget key at all.
func TestBudgetFieldsAreOmittedWhenOff(t *testing.T) {
	for _, v := range []any{domain.EngagementDetail{}, domain.TravelRequestDetail{}, domain.AllocationCreated{}} {
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), `"budget`) {
			t.Errorf("%T has a budget key: %s", v, b)
		}
	}
	// On, a missing budget is written as null.
	var line *domain.EngagementBudgetLine
	b, _ := json.Marshal(domain.EngagementDetail{BudgetLine: line})
	if !strings.Contains(string(b), `"budgetLine":null`) {
		t.Errorf("on = %s", b)
	}
}
