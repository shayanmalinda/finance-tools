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
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func ltrip(job string, line *int, status, country, start string) engagemententity.BudgetLedgerTrip {
	rd := "2027-02-01"
	return engagemententity.BudgetLedgerTrip{JobNumber: job, BudgetLineID: line, Status: status, Country: sp(country), StartDate: start, RateDate: &rd}
}

func lcost(job, cur string, amount float64) engagemententity.BudgetLedgerCost {
	return engagemententity.BudgetLedgerCost{JobNumber: job, Source: "invoice", Currency: cur, Amount: amount}
}

// summaryFake: line 1 has a costed trip (USD 1000 plus LKR 300 at 300/USD),
// a cancelled costed trip and an open trip with no cost. Line 2 has nothing.
// One trip has no line. One spend row names no trip.
func summaryFake() *budgetFake {
	f := newBudgetFake()
	one := 1
	f.ledger = engagemententity.BudgetLedger{ExpenseAvailable: true,
		Trips: []engagemententity.BudgetLedgerTrip{
			ltrip("T1", &one, "Completed", "Germany", "2027-02-10"),
			ltrip("T2", &one, "Cancelled", "Germany", "2027-02-11"),
			ltrip("T3", &one, "New", "Germany", "2027-04-01"),
			ltrip("T4", nil, "In Progress", "Germany", "2027-05-01"),
		},
		Costs:      []engagemententity.BudgetLedgerCost{lcost("T1", "USD", 1000), lcost("T1", "LKR", 300), lcost("T2", "USD", 50), lcost("T4", "USD", 200)},
		Unassigned: []engagemententity.BudgetLedgerUnassigned{{Source: "card", Month: "2027-03", Currency: "USD", Amount: 12.5}}}
	f.needs = engagemententity.BudgetNeedsLinePage{Total: 1}
	return f
}

func TestBudgetSummary(t *testing.T) {
	f := summaryFake()
	svc := newBudgetSvc(f, &budgetRates{rate: 300}, &allocFakeSender{}, time.Minute)
	sum, err := svc.Summary(context.Background(), budgetPerms(35), 0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Year != 2027 || sum.RatesPending || !sum.ExpenseAvailable || sum.NeedsLineCount != 1 || len(sum.Lines) != 3 {
		t.Fatalf("summary = %+v", sum)
	}
	eu := sum.Lines[0]
	// Spent: 1000 + 300/300 + the cancelled trip's 50. Committed: the open trip
	// at the global median of the sample (the fake serves the same ledger to
	// the typical window: trips of 1001 and 200 USD, median 600.50).
	if eu.SpentUSD != 1051 || eu.CommittedUSD != 600.5 || eu.LeftUSD != 8348.5 || eu.UsedPercent != 16.5 || eu.Trips != 2 || eu.OpenTripsWithoutCost != 1 {
		t.Errorf("line = %+v", eu)
	}
	if sum.Unassigned.SpentUSD != 212.5 || sum.Unassigned.TripsWithoutLine != 1 || sum.Unassigned.NoTripSpendUSD != 12.5 {
		t.Errorf("unassigned = %+v", sum.Unassigned)
	}
	if sum.Totals.BudgetUSD != 15000 || sum.Totals.SpentUSD != 1263.5 || sum.Totals.CommittedUSD != 600.5 || sum.Totals.LeftUSD != 13136 {
		t.Errorf("totals = %+v", sum.Totals)
	}
	// A second call is served from the cache: the ledger is read once for the
	// year and once for the typical costs.
	calls := f.ledgerCalls
	if _, err := svc.Summary(context.Background(), budgetPerms(35), 2027); err != nil || f.ledgerCalls != calls {
		t.Errorf("not cached: %v %d %d", err, f.ledgerCalls, calls)
	}
	// A write drops it.
	if _, err := svc.UpdateLine(context.Background(), budgetAdminPerms(), 1, domain.BudgetLineInput{FunctionName: "Sales", SubFunction: "EU", AmountUSD: 20000, Status: "Active"}); err != nil {
		t.Fatal(err)
	}
	sum, _ = svc.Summary(context.Background(), budgetPerms(35), 2027)
	if sum.Lines[0].BudgetUSD != 20000 || f.ledgerCalls == calls {
		t.Errorf("cache kept after a write: %+v", sum.Lines[0])
	}
}

func TestBudgetSummary_CommittedUsesTheTypicalCost(t *testing.T) {
	f := summaryFake()
	// Five past trips to Germany in the typical window cost 100, 200, 300, 400 and 500 USD.
	led := f.ledger
	for i, c := range []float64{100, 200, 300, 400, 500} {
		job := "P" + string(rune('A'+i))
		led.Trips = append(led.Trips, ltrip(job, nil, "Completed", "Germany", "2026-06-10"))
		led.Costs = append(led.Costs, lcost(job, "USD", c))
	}
	f.ledger = led
	svc := newBudgetSvc(f, &budgetRates{rate: 300}, &allocFakeSender{}, time.Minute)
	sum, err := svc.Summary(context.Background(), budgetPerms(1), 2027)
	if err != nil {
		t.Fatal(err)
	}
	// The sample is every costed non-cancelled trip in the ledger (the fake
	// returns the same rows for both calls): the median is the 8-trip middle.
	eu := sum.Lines[0]
	if eu.CommittedUSD <= 0 || eu.LeftUSD != 10000-eu.SpentUSD-eu.CommittedUSD {
		t.Errorf("line = %+v", eu)
	}
}

func TestBudgetSummary_UnresolvedRatesArePendingAndNotCached(t *testing.T) {
	f := summaryFake()
	rates := &budgetRates{fail: true}
	svc := newBudgetSvc(f, rates, &allocFakeSender{}, time.Minute)
	sum, err := svc.Summary(context.Background(), budgetPerms(1), 2027)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.RatesPending || sum.Unconverted.Count != 1 || len(sum.Unconverted.Currencies) != 1 || sum.Unconverted.Currencies[0] != "LKR" || sum.Lines[0].SpentUSD != 1050 {
		t.Errorf("summary = %+v", sum)
	}
	svc.wait()
	calls := f.ledgerCalls
	if _, err := svc.Summary(context.Background(), budgetPerms(1), 2027); err != nil || f.ledgerCalls == calls {
		t.Error("a pending summary must not be cached")
	}
	// Without any converter nothing can be fetched: the LKR cost is reported
	// as unconverted, not pending.
	svc = newBudgetSvc(summaryFake(), nil, &allocFakeSender{}, time.Minute)
	if sum, _ = svc.Summary(context.Background(), budgetPerms(1), 2027); sum.RatesPending || sum.Unconverted.Count != 1 {
		t.Errorf("no converter: %+v", sum)
	}
}

// A blank currency, or one NetSuite has no rate for, never arrives: the
// summary reports it as unconverted and is cached as usual.
func TestBudgetSummary_UnconvertibleRowsDoNotMarkPending(t *testing.T) {
	f := summaryFake()
	f.ledger.Costs = []engagemententity.BudgetLedgerCost{lcost("T1", "USD", 1000), lcost("T1", "", 40), lcost("T1", "XYZ", 5)}
	rates := &budgetRates{rate: 0} // answers, but with no usable USD rate
	svc := newBudgetSvc(f, rates, &allocFakeSender{}, time.Minute)
	sum, err := svc.Summary(context.Background(), budgetPerms(1), 2027)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RatesPending || sum.Unconverted.Count != 2 || sum.Lines[0].SpentUSD != 1000 {
		t.Fatalf("summary = %+v", sum)
	}
	svc.wait()
	calls := f.ledgerCalls
	if _, err := svc.Summary(context.Background(), budgetPerms(1), 2027); err != nil || f.ledgerCalls != calls {
		t.Errorf("an unconvertible summary should be cached: %v %d %d", err, f.ledgerCalls, calls)
	}
	typ, err := svc.typical(context.Background())
	if err != nil || typ.pending {
		t.Errorf("typical pending = %+v %v", typ, err)
	}
}

// While rates are being fetched the last summary is served, not rebuilt.
func TestBudgetSummary_ServesTheLastSummaryWhileRatesWarm(t *testing.T) {
	f := summaryFake()
	rates := &budgetRates{fail: true}
	svc := newBudgetSvc(f, rates, &allocFakeSender{}, time.Minute)
	svc.held.summary.set("2027", &domain.BudgetSummary{Year: 2027, RatesPending: true})
	calls := f.ledgerCalls
	sum, err := svc.Summary(context.Background(), budgetPerms(1), 2027)
	if err != nil || !sum.RatesPending || f.ledgerCalls != calls {
		t.Errorf("held summary not served: %+v %v %d %d", sum, err, f.ledgerCalls, calls)
	}
	// A write drops it.
	svc.dropCache()
	if sum, _ = svc.Summary(context.Background(), budgetPerms(1), 2027); len(sum.Lines) == 0 || f.ledgerCalls == calls {
		t.Errorf("held summary kept after a write: %+v", sum)
	}
	svc.wait()
	// After the warm-up the hold is gone, so a pending build sets it once and
	// the next call finds the rates cached.
	if _, ok := svc.held.summary.get("2027"); ok {
		t.Error("hold should end with the warm-up")
	}
}

// Card and claim rows may spell the job number in another case.
func TestBudgetSummary_CostsMatchTheTripJobNumberIgnoringCase(t *testing.T) {
	f := summaryFake()
	f.ledger.Costs = []engagemententity.BudgetLedgerCost{lcost(" t1 ", "USD", 700)}
	svc := newBudgetSvc(f, &budgetRates{rate: 300}, &allocFakeSender{}, time.Minute)
	sum, err := svc.Summary(context.Background(), budgetPerms(1), 2027)
	if err != nil || sum.Lines[0].SpentUSD != 700 {
		t.Errorf("summary = %+v %v", sum.Lines[0], err)
	}
}

func TestBudgetLines_NamesOnlyWithoutBudgetAccess(t *testing.T) {
	f := summaryFake()
	f.lines[0].HolderName = sp("Ricardo")
	svc := newBudgetSvc(f, nil, &allocFakeSender{}, 0)
	picker := domain.Perms{Email: "e@wso2.com", Alloc: domain.NewPrivilegeSet([]int{12}), Travel: domain.NewPrivilegeSet(nil)}
	ls, err := svc.Lines(context.Background(), picker, 2027, false)
	if err != nil || len(ls) == 0 {
		t.Fatalf("lines = %v %v", ls, err)
	}
	for _, l := range ls {
		if l.AmountUSD != 0 || l.HolderName != nil || l.HolderEmail != nil || l.UpdatedBy != nil || l.Name == "" || l.Code == "" {
			t.Errorf("picker line = %+v", l)
		}
	}
	if ls, _ = svc.Lines(context.Background(), budgetPerms(35), 2027, false); ls[0].AmountUSD == 0 || ls[0].HolderName == nil {
		t.Errorf("viewer line = %+v", ls[0])
	}
}

func TestBudgetAccessMatrix(t *testing.T) {
	both := func(a, tr []int, allocRole, travelRole bool) domain.Perms {
		p := domain.Perms{Alloc: domain.NewPrivilegeSet(a), Travel: domain.NewPrivilegeSet(tr)}
		if allocRole {
			p.AllocRoles = []string{domain.AllocationsSuperAdminGroup}
		}
		if travelRole {
			p.TravelRoles = []string{domain.TravelSuperAdminGroup + "-stg"}
		}
		return p
	}
	admin := both([]int{23}, []int{29}, true, true)
	cases := []struct {
		name                string
		p                   domain.Perms
		admin, view, setter bool
	}{
		{"admin", admin, true, true, true},
		{"only the allocations group", both([]int{23}, []int{29}, true, false), false, true, true},
		{"only the travel group", both([]int{23}, []int{29}, false, true), false, true, true},
		{"groups without 23", both(nil, []int{29}, true, true), false, true, true},
		{"groups without 29", both([]int{23}, nil, true, true), false, false, false},
		{"travel 35 views only", both(nil, []int{35}, false, false), false, true, false},
		{"travel 6 views and sets", both(nil, []int{6}, false, false), false, true, true},
		{"travel 29 views and sets", both(nil, []int{29}, false, false), false, true, true},
		{"allocations 23 alone", both([]int{23}, nil, false, false), false, false, false},
		{"nothing", both(nil, nil, false, false), false, false, false},
	}
	for _, c := range cases {
		if got := budgetAdmin(c.p); got != c.admin {
			t.Errorf("%s: admin = %v", c.name, got)
		}
		if got := budgetView(c.p); got != c.view {
			t.Errorf("%s: view = %v", c.name, got)
		}
		if got := budgetSetLine(c.p); got != c.setter {
			t.Errorf("%s: set line = %v", c.name, got)
		}
	}
	svc := newBudgetSvc(summaryFake(), nil, &allocFakeSender{}, 0)
	if _, err := svc.Summary(context.Background(), budgetPerms(29), 0); err != nil {
		t.Errorf("travel 29 summary: %v", err)
	}
	if _, err := svc.CreateLine(context.Background(), budgetPerms(29), domain.BudgetLineInput{}); !errorsIsForbidden(err) {
		t.Errorf("create without admin: %v", err)
	}
	if _, err := svc.SetTripLine(context.Background(), budgetPerms(35), "TRV-1", nil); !errorsIsForbidden(err) {
		t.Errorf("travel 35 set line: %v", err)
	}
	if _, err := svc.SetTripLine(context.Background(), budgetPerms(29), "TRV-1", ip(2)); err != nil {
		t.Errorf("travel 29 set line: %v", err)
	}
	// Retired lines are listed for the admin only.
	f := summaryFake()
	f.lines[2].Status = "Retired"
	svc = newBudgetSvc(f, nil, &allocFakeSender{}, 0)
	if ls, _ := svc.Lines(context.Background(), budgetPerms(1), 2027, true); len(ls) != 2 {
		t.Errorf("non-admin lines = %d", len(ls))
	}
	if ls, _ := svc.Lines(context.Background(), admin, 2027, true); len(ls) != 3 {
		t.Errorf("admin lines = %d", len(ls))
	}
}

func TestBudgetLineValidation(t *testing.T) {
	svc := newBudgetSvc(newBudgetFake(), nil, &allocFakeSender{}, 0)
	ok := domain.BudgetLineInput{Year: 2027, Code: "NEW_LINE", FunctionName: "Sales", SubFunction: "X", AmountUSD: 5}
	l, err := svc.CreateLine(context.Background(), budgetAdminPerms(), ok)
	if err != nil || l.Name != "Sales: X" {
		t.Fatalf("create = %+v %v", l, err)
	}
	for _, mut := range []func(*domain.BudgetLineInput){
		func(i *domain.BudgetLineInput) { i.Code = "!!!" }, func(i *domain.BudgetLineInput) { i.Year = 0 },
		func(i *domain.BudgetLineInput) { i.AmountUSD = -1 }, func(i *domain.BudgetLineInput) { i.FunctionName = " " },
		func(i *domain.BudgetLineInput) { i.HolderEmail = sp("not an email") },
	} {
		in := ok
		mut(&in)
		if _, err := svc.CreateLine(context.Background(), budgetAdminPerms(), in); !isAllocValidation(err, "") {
			t.Errorf("%+v accepted: %v", in, err)
		}
	}
	if _, err := svc.UpdateLine(context.Background(), budgetAdminPerms(), 1, domain.BudgetLineInput{FunctionName: "a", SubFunction: "b", Status: "Gone"}); !isAllocValidation(err, "Status must be Active or Retired.") {
		t.Errorf("status: %v", err)
	}
	// Rules: LINE needs a code, others none.
	for _, in := range []domain.BudgetRuleInput{{RuleKind: "LINE"}, {RuleKind: "TEAM", LineCode: sp("SALES_EU")}, {RuleKind: "X"}, {RuleKind: "LINE", LineCode: sp("NOPE")}} {
		if _, err := svc.SaveAllocationTypeRule(context.Background(), budgetAdminPerms(), 12, in); !isAllocValidation(err, "") {
			t.Errorf("rule %+v accepted: %v", in, err)
		}
	}
	r, err := svc.SaveAllocationTypeRule(context.Background(), budgetAdminPerms(), 12, domain.BudgetRuleInput{RuleKind: "LINE", LineCode: sp("SALES_EU")})
	if err != nil || r.LineName == nil || *r.LineName != "Sales: EU" {
		t.Errorf("rule = %+v %v", r, err)
	}
	rs, err := svc.AllocationTypeRules(context.Background())
	if err != nil || len(rs) != 2 || rs[1].RuleKind != "ENGAGEMENT" {
		t.Errorf("rules = %+v %v", rs, err)
	}
}

func TestBudgetPreview(t *testing.T) {
	f := summaryFake()
	// Five Germany trips make a country median in the typical window.
	for i, c := range []float64{3000, 3000, 3000, 3000, 3000} {
		job := "P" + string(rune('A'+i))
		f.ledger.Trips = append(f.ledger.Trips, ltrip(job, nil, "Completed", "Germany", "2026-06-10"))
		f.ledger.Costs = append(f.ledger.Costs, lcost(job, "USD", c))
	}
	f.engagement.FundingSources = []engagemententity.AllocationFundingSource{{Region: sp("EU"), Percentage: fp(100)}}
	f.engagement.AllocationTypeID = ip(13) // REGION
	svc := newBudgetSvc(f, &budgetRates{rate: 300}, &allocFakeSender{}, time.Minute)
	q := BudgetPreviewQuery{EngagementID: "E07201", TravellerEmail: "jane@wso2.com", StartDate: "2027-02-01", EndDate: "2027-02-05"}
	pv, err := svc.Preview(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !pv.InScope || pv.Year != 2027 || pv.Source != "REGION" || pv.Suggested == nil || pv.Suggested.Code != "SALES_EU" || pv.Suggested.Name != "Sales: EU" ||
		pv.Country != "Sri Lanka" || len(pv.Alternatives) != 3 || pv.Line == nil || pv.Line.LineID != 1 {
		t.Fatalf("preview = %+v", pv)
	}
	// The line's left is under the typical cost of a trip to Sri Lanka only when it is that low.
	pv2, _ := svc.Preview(context.Background(), BudgetPreviewQuery{EngagementID: q.EngagementID, TravellerEmail: q.TravellerEmail,
		StartDate: q.StartDate, EndDate: q.EndDate, Country: "Germany"})
	if pv2.TypicalCost.Basis != "country" || pv2.TypicalCost.USD == nil || *pv2.TypicalCost.USD != 3000 || pv2.TypicalCost.SampleSize < 5 {
		t.Errorf("typical = %+v", pv2.TypicalCost)
	}
	if pv.TypicalCost.Basis != "global" {
		t.Errorf("Sri Lanka has no trips of its own: %+v", pv.TypicalCost)
	}
	// Warning: the typical cost is over what is left.
	f.lines[0].AmountUSD = 1500
	svc.dropCache()
	if pv3, _ := svc.Preview(context.Background(), BudgetPreviewQuery{EngagementID: q.EngagementID, TravellerEmail: q.TravellerEmail,
		StartDate: q.StartDate, EndDate: q.EndDate, Country: "Germany"}); !pv3.Warning {
		t.Errorf("no warning: %+v", pv3)
	}
	// Outside every budget year only inScope is written.
	q.StartDate, q.EndDate = "2026-02-01", "2026-02-05"
	out, _ := svc.Preview(context.Background(), q)
	if b, _ := out.MarshalJSON(); string(b) != `{"inScope":false}` {
		t.Errorf("out of scope = %s", b)
	}
	if _, err := svc.Preview(context.Background(), BudgetPreviewQuery{EngagementID: "E1"}); !isAllocValidation(err, "") {
		t.Errorf("missing params: %v", err)
	}
}

func fp(v float64) *float64 { return &v }

func TestBudgetRateDate(t *testing.T) {
	today := time.Date(2027, 3, 10, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{"2027-02-01": "2027-02-01", "2027-09-01": "2027-03-10", "": "2027-03-10"} {
		if got := budgetRateDate(in, today); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestRateSetUsdOn(t *testing.T) {
	rs := rateSet{{"LKR", "2027-02-01"}: 300}
	if v, ok := rs.usdOn("lkr", 600, "2027-02-01"); !ok || v != 2 {
		t.Errorf("lkr = %v %v", v, ok)
	}
	if v, ok := rs.usdOn("USD", 10.456, "x"); !ok || v != 10.46 {
		t.Errorf("usd = %v %v", v, ok)
	}
	if _, ok := rs.usdOn("EUR", 1, "2027-02-01"); ok {
		t.Error("no rate should fail")
	}
}
