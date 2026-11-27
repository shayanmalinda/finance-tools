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
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// budgetFake is an in-memory entity implementing section 2 of the design.
type budgetFake struct {
	engagemententity.Client
	mu          sync.Mutex
	years       []engagemententity.BudgetYear
	lines       []engagemententity.BudgetLine
	rules       []engagemententity.BudgetAllocationTypeRule
	regionRules []engagemententity.BudgetRegionRule
	teamRules   []engagemententity.BudgetTeamRule
	engLine     map[string]string
	trips       map[string]engagemententity.BudgetTrip
	ledger      engagemententity.BudgetLedger
	ledgerCalls int
	needs       engagemententity.BudgetNeedsLinePage
	types       []engagemententity.AllocationType
	engagement  engagemententity.AllocationEngagement
	failLines   error
	failRules   error
	setTrip     []*int
}

func (f *budgetFake) BudgetYears(context.Context) ([]engagemententity.BudgetYear, error) {
	return f.years, nil
}

func (f *budgetFake) BudgetLines(_ context.Context, year int, _ bool) ([]engagemententity.BudgetLine, error) {
	if f.failLines != nil {
		return nil, f.failLines
	}
	var out []engagemententity.BudgetLine
	for _, l := range f.lines {
		if l.Year == year {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *budgetFake) CreateBudgetLine(_ context.Context, in engagemententity.BudgetLineCreate) (engagemententity.BudgetLine, error) {
	l := engagemententity.BudgetLine{ID: len(f.lines) + 1, Year: in.Year, Code: in.Code, FunctionName: in.FunctionName,
		SubFunction: in.SubFunction, AmountUSD: in.AmountUSD, Status: "Active"}
	f.lines = append(f.lines, l)
	return l, nil
}

func (f *budgetFake) UpdateBudgetLine(_ context.Context, id int, in engagemententity.BudgetLineUpdate) (engagemententity.BudgetLine, error) {
	for i := range f.lines {
		if f.lines[i].ID == id {
			f.lines[i].AmountUSD, f.lines[i].Status = in.AmountUSD, in.Status
			return f.lines[i], nil
		}
	}
	return engagemententity.BudgetLine{}, &apierror.UpstreamError{Service: "e", StatusCode: 404, Msg: "not found"}
}

func (f *budgetFake) RetireBudgetLine(_ context.Context, id int, _ string) error {
	for i := range f.lines {
		if f.lines[i].ID == id {
			f.lines[i].Status = "Retired"
		}
	}
	return nil
}

func (f *budgetFake) BudgetAllocationTypeRules(context.Context) ([]engagemententity.BudgetAllocationTypeRule, error) {
	return f.rules, f.failRules
}

func (f *budgetFake) SaveBudgetAllocationTypeRule(_ context.Context, id int, in engagemententity.BudgetRuleSave) (engagemententity.BudgetAllocationTypeRule, error) {
	r := engagemententity.BudgetAllocationTypeRule{AllocationTypeID: id, RuleKind: in.RuleKind, LineCode: in.LineCode}
	f.rules = append(f.rules, r)
	return r, nil
}

func (f *budgetFake) BudgetRegionRules(context.Context) ([]engagemententity.BudgetRegionRule, error) {
	return f.regionRules, nil
}

func (f *budgetFake) BudgetTeamRules(context.Context) ([]engagemententity.BudgetTeamRule, error) {
	return f.teamRules, nil
}

func (f *budgetFake) BudgetEngagementLine(_ context.Context, id string) (*string, error) {
	if c, ok := f.engLine[id]; ok {
		return &c, nil
	}
	return nil, nil
}

func (f *budgetFake) SetBudgetEngagementLine(_ context.Context, id string, code *string, _ string) (*string, error) {
	if f.engLine == nil {
		f.engLine = map[string]string{}
	}
	if code == nil {
		delete(f.engLine, id)
	} else {
		f.engLine[id] = *code
	}
	return code, nil
}

func (f *budgetFake) BudgetTrip(_ context.Context, job string) (engagemententity.BudgetTrip, error) {
	t, ok := f.trips[job]
	if !ok {
		return t, &apierror.UpstreamError{Service: "e", StatusCode: 404, Msg: "not found"}
	}
	return t, nil
}

func (f *budgetFake) SetBudgetTrip(_ context.Context, job string, id *int, _ string) (engagemententity.BudgetTrip, error) {
	f.setTrip = append(f.setTrip, id)
	t := f.trips[job]
	t.JobNumber, t.BudgetLineID, t.HasRow = job, id, true
	by := "USER"
	t.ChosenBy = &by
	if f.trips == nil {
		f.trips = map[string]engagemententity.BudgetTrip{}
	}
	f.trips[job] = t
	return t, nil
}

func (f *budgetFake) BudgetLedger(context.Context, string, string, bool) (engagemententity.BudgetLedger, error) {
	f.mu.Lock()
	f.ledgerCalls++
	f.mu.Unlock()
	return f.ledger, nil
}

func (f *budgetFake) BudgetNeedsLine(context.Context, int, int, int) (engagemententity.BudgetNeedsLinePage, error) {
	return f.needs, nil
}

func (f *budgetFake) AllocationEngagement(_ context.Context, id string) (engagemententity.AllocationEngagement, error) {
	if f.engagement.ID != id {
		return engagemententity.AllocationEngagement{}, allocNotFound()
	}
	return f.engagement, nil
}

func (f *budgetFake) AllocationTypes(context.Context, engagemententity.AllocationTypeFilter) ([]engagemententity.AllocationType, error) {
	return f.types, nil
}

// budgetRates answers every currency at a fixed rate, or fails when fail is set.
type budgetRates struct {
	rate  float64
	fail  bool
	calls int
	mu    sync.Mutex
}

func (r *budgetRates) Rates(_ context.Context, base, date string) (map[string]float64, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if r.fail {
		return nil, errors.New("down")
	}
	return map[string]float64{"USD": r.rate}, nil
}

var budgetNow = time.Date(2027, 3, 10, 9, 0, 0, 0, time.UTC)

func budgetLine(id int, code, fn, sub string, amount float64) engagemententity.BudgetLine {
	return engagemententity.BudgetLine{ID: id, Year: 2027, Code: code, FunctionName: fn, SubFunction: sub, AmountUSD: amount, Status: "Active", SortOrder: id * 10}
}

// newBudgetFake has year 2027, three lines and the seed-like rules.
func newBudgetFake() *budgetFake {
	return &budgetFake{
		years: []engagemententity.BudgetYear{{Year: 2027, StartDate: "2027-01-01", EndDate: "2027-12-31", IsActive: true}},
		lines: []engagemententity.BudgetLine{
			budgetLine(1, "SALES_EU", "Sales", "EU", 10000),
			budgetLine(2, "SALES_APAC", "Sales", "APAC", 5000),
			budgetLine(3, "GA_BOARD", "General Administration", "Board meeting travel", 0),
		},
		rules: []engagemententity.BudgetAllocationTypeRule{
			{AllocationTypeID: 12, RuleKind: "LINE", LineCode: sp("SALES_APAC")},
			{AllocationTypeID: 13, RuleKind: "REGION"},
			{AllocationTypeID: 14, RuleKind: "ENGAGEMENT"},
		},
		regionRules: []engagemententity.BudgetRegionRule{{Region: "EU", LineCode: sp("SALES_EU")}},
		engagement:  allocTestEngagement(2),
		types: []engagemententity.AllocationType{{ID: 12, AllocationTypeName: "Consulting", EngagementTypeID: 2, EngagementTypeName: "Paid"},
			{ID: 14, AllocationTypeName: "Board", EngagementTypeID: 1, EngagementTypeName: "Internal"}},
	}
}

func newBudgetSvc(f *budgetFake, rates *budgetRates, snd *allocFakeSender, ttl time.Duration) *BudgetService {
	var conv FinanceCurrencyConverter
	if rates != nil {
		conv = rates
	}
	return NewBudgetService(f, nil, snd, NewRateCache(conv, func() time.Time { return budgetNow }),
		BudgetOptions{LineRecipients: []string{"owner@example.com"}, AppBaseURL: "https://one.example", CacheTTL: ttl,
			Now: func() time.Time { return budgetNow }})
}

func budgetPerms(travel ...int) domain.Perms {
	return domain.Perms{Email: "u@wso2.com", Alloc: domain.NewPrivilegeSet(nil), Travel: domain.NewPrivilegeSet(travel)}
}

func budgetAdminPerms() domain.Perms {
	return domain.Perms{Email: "admin@wso2.com", Alloc: domain.NewPrivilegeSet([]int{23}), Travel: domain.NewPrivilegeSet([]int{29}),
		AllocRoles: []string{domain.AllocationsSuperAdminGroup}, TravelRoles: []string{domain.TravelSuperAdminGroup}}
}
