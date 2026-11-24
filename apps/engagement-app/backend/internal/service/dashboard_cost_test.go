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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func finPerms(own ...int) domain.Perms {
	t := append(append([]int{}, dashboardEmployeeTravel...), own...)
	return dashboardPerms("fin@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, t, t, "App-Travel-Approver-Finance-stg")
}

func tsaPerms() domain.Perms {
	return dashboardPerms("tsa@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardRange(1, 49), dashboardEmployeeTravel, "app-travel-admin-super-stg")
}

func entry(id, source, job, date, cur string, amt float64) engagemententity.DashboardCostEntry {
	e := engagemententity.DashboardCostEntry{ID: id, Source: source, Date: date, OriginalCurrency: cur, OriginalAmount: amt, Paid: "unknown", CostType: "Other"}
	switch source {
	case "perDiem":
		e.CostType = "Per Diem"
	case "card":
		e.CostType = "Card"
	case "claim":
		e.CostType = "Claim"
	}
	if job != "" {
		e.JobNumber = sptr(job)
	}
	return e
}

// costService serves lines (the same for any range unless fn is set).
func costService(lines []engagemententity.DashboardCostEntry, conv FinanceCurrencyConverter) (DashboardService, *fakeDashboardEntity) {
	f := &fakeDashboardEntity{costResp: &engagemententity.DashboardCostLines{Items: lines}}
	return NewDashboardService(f, nil, conv, dashboardNow), f
}

var cq = CostQuery{From: "2026-01-01", To: "2026-11-05", Compare: true}

func mustSummary(t *testing.T, svc DashboardService, p domain.Perms, q CostQuery) *domain.TravelCostSummary {
	t.Helper()
	s, err := svc.TravelCosts(context.Background(), p, q)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTravelCosts_KpisBucketsAndTotals(t *testing.T) {
	a := entry("INV-1", "invoice", "TRV-1", "2026-10-14", "LKR", 90000)
	a.Country, a.Customer, a.Paid, a.TravelAgentID, a.TravelAgentName, a.TravellerEmail = sptr("India"), sptr("Acme"), "paid", iptr(3), sptr("Classic"), sptr("Jane@Example.com")
	b := entry("PD-1", "perDiem", "trv-1", "2026-10-15", "USD", 200)
	b.Country, b.Paid, b.TravelAgentID, b.TravelAgentName, b.TravellerEmail = sptr("India"), "paid", iptr(3), sptr("Classic"), sptr("jane@example.com")
	c := entry("CC-1", "card", "", "2026-10-16", "EUR", 100) // unlinked, EUR has no rate
	c.TravellerEmail = sptr("bob@example.com")
	d := entry("EC-1", "claim", "TRV-2", "2026-11-02", "USD", 50)
	d.Country, d.Paid = sptr("France"), "nonPaid"
	e := entry("CC-2", "card", "", "2026-10-20", "LKR", 15000)
	e.CCCurrency, e.CCAmount = "USD", 48.2
	svc, _ := costService([]engagemententity.DashboardCostEntry{a, b, c, d, e}, &fakeRates{rates: map[string]float64{"LKR": 300}})
	s := mustSummary(t, svc, finPerms(39, 41), cq)

	// 300 + 200 + 50 + 48.2 USD; the EUR card line has no rate.
	if s.Kpis.Total.Value != 598.2 || s.Kpis.Trips.Value != 2 || s.Kpis.Travellers.Value != 2 || s.Kpis.UnlinkedSpend.Value != 48.2 {
		t.Errorf("kpis = %+v", s.Kpis)
	}
	// Average per trip: lines with a job number (550) over 2 trips.
	if s.Kpis.AveragePerTrip.Value != 275 {
		t.Errorf("average per trip = %v", s.Kpis.AveragePerTrip.Value)
	}
	sum := 0.0
	for _, bk := range s.BySource {
		sum += bk.Value
	}
	if dashboardRound(sum) != s.Kpis.Total.Value || len(s.BySource) != 4 {
		t.Errorf("bySource %+v does not add up to the total", s.BySource)
	}
	if len(s.NotConverted) != 1 || s.NotConverted[0] != (domain.CostNotConverted{Source: "card", Currency: "EUR", Amount: 100, Lines: 1}) {
		t.Errorf("notConverted = %+v", s.NotConverted)
	}
	if s.Compare == nil || s.Compare.From != "2025-01-01" || s.Compare.To != "2025-11-05" || s.Currency != "USD" || s.RateRule != "monthEnd" {
		t.Errorf("compare/currency = %+v %s", s.Compare, s.Currency)
	}
	// Paid 500 / (500 + 50).
	if s.PaidSharePercent == nil || *s.PaidSharePercent != 90.9 {
		t.Errorf("paid share = %v", s.PaidSharePercent)
	}
	if len(s.ByPaid) != 3 || s.ByPaid[0].Key != "paid" || s.ByPaid[2].Key != "unknown" {
		t.Errorf("byPaid = %+v", s.ByPaid)
	}
	// Paid lines only; the per diem has no customer on record.
	if len(s.ByCustomer) != 2 || s.ByCustomer[0].Label != "Acme" || *s.ByCustomer[0].Trips != 1 || s.ByCustomer[1].Label != "Unknown" {
		t.Errorf("byCustomer = %+v", s.ByCustomer)
	}
	var agent *domain.CostBucket
	for i := range s.ByTravelAgent {
		if s.ByTravelAgent[i].Key == "3" {
			agent = &s.ByTravelAgent[i]
		}
	}
	if agent == nil || *agent.Trips != 1 || *agent.AveragePerTrip != 500 {
		t.Errorf("byTravelAgent = %+v", s.ByTravelAgent)
	}
	for _, ct := range s.ByCostType {
		if (ct.Key == "Card" || ct.Key == "Claim") == *ct.Categorised {
			t.Errorf("categorised wrong for %+v", ct)
		}
	}
	if len(s.ByMonth) != 11 || s.ByMonth[9].Month != "2026-10" || s.ByMonth[9].Values["Travel Invoices"] != 300 {
		t.Errorf("byMonth = %+v", s.ByMonth[9])
	}
	if len(s.Facets.Countries) != 3 || s.Facets.Countries[2] != "Unknown" || len(s.Facets.TravelAgents) != 1 {
		t.Errorf("facets = %+v", s.Facets)
	}
}

func TestTravelCosts_ThinRuleTopTenAndOther(t *testing.T) {
	var cur, prev []engagemententity.DashboardCostEntry
	for i := 0; i < 12; i++ {
		e := entry(fmt.Sprintf("INV-%d", i), "invoice", "", "2026-10-01", "USD", float64(100-i))
		e.Country = sptr("C" + strconv.Itoa(i))
		cur = append(cur, e)
	}
	// Last year only C0 (so C0 compares, the rest are thin) and C11 (which falls into Other).
	p0 := entry("INV-p0", "invoice", "", "2025-10-01", "USD", 50)
	p0.Country = sptr("C0")
	p11 := entry("INV-p11", "invoice", "", "2025-10-01", "USD", 10)
	p11.Country = sptr("C11")
	prev = append(prev, p0, p11)
	f := &fakeDashboardEntity{costFn: func(fl engagemententity.DashboardCostLinesFilter) *engagemententity.DashboardCostLines {
		if strings.HasPrefix(fl.From, "2025") {
			return &engagemententity.DashboardCostLines{Items: prev}
		}
		return &engagemententity.DashboardCostLines{Items: cur}
	}}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	s := mustSummary(t, svc, finPerms(39, 41), cq)
	if len(s.ByCountry) != 11 || s.ByCountry[10].Label != "Other" || s.ByCountry[10].Key != "" || s.ByCountry[10].Value != 89+90 {
		t.Fatalf("byCountry = %+v", s.ByCountry)
	}
	if s.ByCountry[10].Previous == nil || *s.ByCountry[10].Previous != 10 {
		t.Errorf("Other previous = %v", s.ByCountry[10].Previous)
	}
	c0 := s.ByCountry[0]
	if c0.Thin || c0.ChangePercent == nil || *c0.ChangePercent != 100 || *c0.Previous != 50 {
		t.Errorf("C0 = %+v, want +100%% on 50", c0)
	}
	if c1 := s.ByCountry[1]; !c1.Thin || c1.ChangePercent != nil {
		t.Errorf("C1 has no previous year: %+v", c1)
	}
	// More than five times last year is thin too.
	if p, c, thin := costChange(501, 100, true); !thin || c != nil || *p != 100 {
		t.Errorf("501 vs 100: thin %v change %v", thin, c)
	}
	if _, c, thin := costChange(500, 100, true); thin || *c != 400 {
		t.Errorf("500 vs 100: thin %v change %v", thin, c)
	}
	if p, c, thin := costChange(5, 0, false); p != nil || c != nil || thin {
		t.Error("no comparison must give nothing")
	}
	// compare=false makes one entity call and no previous figures.
	f.costCalls = 0
	svc = NewDashboardService(f, nil, nil, dashboardNow)
	s = mustSummary(t, svc, finPerms(39, 41), CostQuery{From: cq.From, To: cq.To})
	if f.costCalls != 1 || s.Compare != nil || s.Kpis.Total.Previous != nil {
		t.Errorf("compare=false: %d calls, %+v", f.costCalls, s.Compare)
	}
}

func TestShiftYear(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	for in, want := range map[string]string{"2028-02-29": "2027-02-28", "2026-11-24": "2025-11-24", "2028-03-01": "2027-03-01", "2025-02-28": "2024-02-28"} {
		if got := shiftYear(d(in)).Format("2006-01-02"); got != want {
			t.Errorf("shiftYear(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestTravelCosts_Validation(t *testing.T) {
	svc, _ := costService(nil, nil)
	bad := []CostQuery{
		{From: "2026-01-01"},
		{To: "2026-01-01"},
		{From: "bad", To: "2026-01-01"},
		{From: "2026-02-01", To: "2026-01-01"},
		{From: "2024-01-01", To: "2026-11-05"},
		{From: "2026-01-01", To: "2026-02-01", Filter: CostFilter{Sources: []string{"cash"}}},
		{From: "2026-01-01", To: "2026-02-01", Filter: CostFilter{Paid: []string{"maybe"}}},
		{From: "2026-01-01", To: "2026-02-01", Filter: CostFilter{Employees: make([]string, 51)}},
	}
	for i, q := range bad {
		if _, err := svc.TravelCosts(context.Background(), finPerms(39, 41), q); !dashboardIsValidation(err) {
			t.Errorf("case %d (%+v): err = %v", i, q, err)
		}
	}
	// No dates: this year to date. 731 days exactly is fine.
	s := mustSummary(t, svc, finPerms(39, 41), CostQuery{})
	if s.From != "2026-01-01" || s.To != "2026-11-05" {
		t.Errorf("default range = %s to %s", s.From, s.To)
	}
	mustSummary(t, svc, finPerms(39, 41), CostQuery{From: "2024-11-05", To: "2026-11-05"})
	if _, err := svc.CostTransactions(context.Background(), finPerms(39, 41), cq, CostTransactionsQuery{GroupBy: "colour"}); !dashboardIsValidation(err) {
		t.Errorf("groupBy: %v", err)
	}
	if _, err := svc.CostTransactions(context.Background(), finPerms(39, 41), cq, CostTransactionsQuery{Sort: "size"}); !dashboardIsValidation(err) {
		t.Errorf("sort: %v", err)
	}
}

func TestTravelCosts_Authorisation(t *testing.T) {
	lines := []engagemententity.DashboardCostEntry{
		entry("INV-1", "invoice", "T1", "2026-10-01", "USD", 10), entry("PD-1", "perDiem", "T1", "2026-10-01", "USD", 20),
		entry("CC-1", "card", "T2", "2026-10-01", "USD", 30), entry("EC-1", "claim", "T2", "2026-10-01", "USD", 40),
	}
	lines[2].TravellerEmail = sptr("cardholder@example.com")
	svc, _ := costService(lines, nil)
	ctx := context.Background()

	// A Travel Super Admin who is in no finance group gets all four sources, the employee filter, duplicates and both CSVs.
	tsa := tsaPerms()
	s := mustSummary(t, svc, tsa, cq)
	if s.Kpis.Total.Value != 100 || len(s.BySource) != 4 {
		t.Errorf("tsa total = %v sources %d", s.Kpis.Total.Value, len(s.BySource))
	}
	byEmp := cq
	byEmp.Filter.Employees = []string{"cardholder@example.com"}
	if s := mustSummary(t, svc, tsa, byEmp); s.Kpis.Total.Value != 30 {
		t.Errorf("employee filter total = %v", s.Kpis.Total.Value)
	}
	if _, err := svc.CostDuplicates(ctx, tsa, cq, 25, 0); err != nil {
		t.Errorf("tsa duplicates: %v", err)
	}
	if c, err := svc.TravelCostsCSV(ctx, tsa, cq, ""); err != nil || len(c.Rows) < 10 {
		t.Errorf("tsa summary csv: %v", err)
	}
	if c, err := svc.CostTransactionsCSV(ctx, tsa, cq); err != nil || len(c.Rows) < 10 {
		t.Errorf("tsa transactions csv: %v", err)
	}
	if page, err := svc.CostTransactions(ctx, tsa, cq, CostTransactionsQuery{}); err != nil || page.(domain.CostLinePage).Total != 4 {
		t.Errorf("tsa transactions: %+v %v", page, err)
	}

	// Neither finance nor Travel Super Admin: 403 on all three routes (and the CSVs).
	for name, p := range map[string]domain.Perms{
		"employee":      dashboardPerms("e@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel),
		"travel team":   dashboardPerms("t@wso2.com", nil, nil, []int{1, 39, 41}, []int{1, 39, 41}),
		"finance group": dashboardPerms("f@wso2.com", nil, nil, dashboardEmployeeTravel, dashboardEmployeeTravel, "app-travel-approver-finance"), // group without 39 or 41
		"account mgr":   dashboardPerms("a@wso2.com", []int{1, 2}, []int{1, 2}, nil, nil),
	} {
		if _, err := svc.TravelCosts(ctx, p, cq); !isForbidden(err) {
			t.Errorf("%s: summary err = %v", name, err)
		}
		if _, err := svc.CostTransactions(ctx, p, cq, CostTransactionsQuery{}); !isForbidden(err) {
			t.Errorf("%s: transactions err = %v", name, err)
		}
		if _, err := svc.CostDuplicates(ctx, p, cq, 25, 0); !isForbidden(err) {
			t.Errorf("%s: duplicates err = %v", name, err)
		}
		if _, err := svc.TravelCostsCSV(ctx, p, cq, ""); !isForbidden(err) {
			t.Errorf("%s: csv err = %v", name, err)
		}
	}

	// A finance caller without 39/30 loses invoices, without 41/42 loses per diems; cards and claims stay.
	for _, c := range []struct {
		own  []int
		want float64
		n    int
	}{{[]int{39}, 80, 3}, {[]int{41}, 90, 3}, {[]int{39, 41}, 100, 4}, {[]int{39, 30}, 80, 3}, {[]int{41, 42}, 90, 3}} {
		p := finPerms(c.own...)
		s := mustSummary(t, svc, p, cq)
		if s.Kpis.Total.Value != c.want || len(s.BySource) != c.n {
			t.Errorf("own %v: total %v sources %d, want %v / %d", c.own, s.Kpis.Total.Value, len(s.BySource), c.want, c.n)
		}
		// Asking for a source they may not see is ignored, never an error.
		q := cq
		q.Filter.Sources = []string{"invoice", "perDiem", "card"}
		if s := mustSummary(t, svc, p, q); s.Kpis.Total.Value > c.want-40 {
			t.Errorf("own %v: source filter total %v includes claims or hidden sources", c.own, s.Kpis.Total.Value)
		}
	}
}

func TestTravelCosts_FailedSourcesAndTruncation(t *testing.T) {
	f := &fakeDashboardEntity{costResp: &engagemententity.DashboardCostLines{
		Items:       []engagemententity.DashboardCostEntry{entry("INV-1", "invoice", "T1", "2026-10-01", "USD", 10)},
		Unavailable: []string{"card", "claim"}, Truncated: []string{"invoice"},
	}}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	s := mustSummary(t, svc, finPerms(39, 41), cq)
	if len(s.FailedSources) != 2 || s.FailedSources[0] != "card" || !s.Truncated || s.Kpis.Total.Value != 10 {
		t.Errorf("failedSources %v truncated %v total %v", s.FailedSources, s.Truncated, s.Kpis.Total.Value)
	}
}

func TestCostApply_Filters(t *testing.T) {
	mk := func(id, src, job, country, customer, paid, ct string, agent *int, email string) costLine {
		e := entry(id, src, job, "2026-10-01", "USD", 10)
		e.Paid, e.CostType, e.TravelAgentID = paid, ct, agent
		if country != "" {
			e.Country = sptr(country)
		}
		if customer != "" {
			e.Customer = sptr(customer)
		}
		l := costLine{e: e, converted: true, usd: dashboardUSD{USD: 10}, job: strings.ToUpper(job), month: "2026-10"}
		if email != "" {
			l.traveller = &domain.Person{Email: email}
		}
		return l
	}
	lines := []costLine{
		mk("1", "invoice", "T1", "India", "Acme", "paid", "Hotel", iptr(3), "a@x.com"),
		mk("2", "card", "", "", "", "unknown", "Card", nil, "b@x.com"),
		mk("3", "claim", "t2", "France", "Acme", "nonPaid", "Claim", iptr(4), ""),
	}
	ids := func(f CostFilter) string {
		var out []string
		for _, l := range costApply(lines, f) {
			out = append(out, l.e.ID)
		}
		return strings.Join(out, ",")
	}
	for name, c := range map[string]struct {
		f    CostFilter
		want string
	}{
		"none":        {CostFilter{}, "1,2,3"},
		"source":      {CostFilter{Sources: []string{"card", "claim"}}, "2,3"},
		"employee":    {CostFilter{Employees: []string{"A@x.com"}}, "1"},
		"job":         {CostFilter{JobNumbers: []string{" t1", "T2"}}, "1,3"},
		"country":     {CostFilter{Countries: []string{"india"}}, "1"},
		"unknown":     {CostFilter{Countries: []string{"Unknown"}}, "2"},
		"agent":       {CostFilter{TravelAgents: []int{4}}, "3"},
		"no agent":    {CostFilter{TravelAgents: []int{0}}, "2"},
		"paid":        {CostFilter{Paid: []string{"paid", "unknown"}}, "1,2"},
		"customer":    {CostFilter{Customers: []string{"acme"}}, "1,3"},
		"cost type":   {CostFilter{CostTypes: []string{"claim"}}, "3"},
		"unlinked":    {CostFilter{Unlinked: true}, "2"},
		"two filters": {CostFilter{Customers: []string{"Acme"}, Paid: []string{"paid"}}, "1"},
	} {
		if got := ids(c.f); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

func TestCostDuplicates_Rule(t *testing.T) {
	mk := func(id, src, job, date string, usd float64, converted bool) costLine {
		e := entry(id, src, job, date, "USD", usd)
		return costLine{e: e, converted: converted, usd: dashboardUSD{USD: usd, Rate: 1}, job: strings.ToUpper(strings.TrimSpace(job)), month: date[:7]}
	}
	lines := []costLine{
		mk("A", "invoice", "T1", "2026-10-01", 100, true),
		mk("B", "card", " t1", "2026-10-15", 101, true),     // 14 days apart, within 1%: a duplicate
		mk("C", "claim", "T1", "2026-10-16", 100, true),     // 15 days from A, 1 day from B (1.0%): pairs with B only
		mk("D", "perDiem", "T1", "2026-10-02", 102.5, true), // 2.4% off A: no
		mk("E", "invoice", "T1", "2026-10-03", 100, true),   // same source as A: ignored with A, pairs with B? B is 12 days later
		mk("F", "card", "", "2026-10-01", 100, true),        // no job number
		mk("G", "claim", "T9", "2026-10-01", 100, false),    // not converted
		mk("H", "card", "T9", "2026-10-01", 100, true),
	}
	got := costDuplicates(lines)
	pairs := map[string]string{}
	for _, d := range got {
		pairs[d.A.ID+d.B.ID] = fmt.Sprint(d.DaysApart)
	}
	want := map[string]string{"AB": "14", "BC": "1", "EB": "12", "EC": "13"}
	if len(pairs) != len(want) {
		t.Fatalf("pairs = %v, want %v", pairs, want)
	}
	for k, v := range want {
		if pairs[k] != v {
			t.Errorf("pair %s days = %q, want %s (all: %v)", k, pairs[k], v, pairs)
		}
	}
	if got[0].USDAmount < got[len(got)-1].USDAmount {
		t.Error("not sorted by usdAmount descending")
	}
}

func TestCostGroupsAndTransactions(t *testing.T) {
	a := entry("INV-1", "invoice", "TRV-1", "2026-10-14", "USD", 100)
	a.Country, a.TravellerEmail = sptr("India"), sptr("jane@x.com")
	b := entry("PD-1", "perDiem", "TRV-1", "2026-10-15", "USD", 50)
	b.Country, b.TravellerEmail = sptr("India"), sptr("jane@x.com")
	c := entry("CC-1", "card", "", "2026-09-01", "USD", 10)
	c.TravellerEmail = sptr("bob@x.com")
	svc, _ := costService([]engagemententity.DashboardCostEntry{a, b, c}, nil)
	ctx := context.Background()
	out, err := svc.CostTransactions(ctx, finPerms(39, 41), cq, CostTransactionsQuery{GroupBy: "country", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	gp := out.(domain.CostGroupPage)
	if gp.Total != 2 || gp.Groups[0].Key != "India" || gp.Groups[0].Total != 150 || gp.Groups[0].Lines != 2 || gp.Groups[0].Trips != 1 || gp.Groups[0].Travellers != 1 || gp.Groups[1].Key != "Unknown" {
		t.Errorf("groups = %+v", gp)
	}
	out, _ = svc.CostTransactions(ctx, finPerms(39, 41), cq, CostTransactionsQuery{GroupBy: "month", Limit: 1, Offset: 1, Sort: "date"})
	if gp := out.(domain.CostGroupPage); gp.Total != 2 || len(gp.Groups) != 1 || gp.Groups[0].Key != "2026-10" {
		t.Errorf("month groups page = %+v", gp)
	}
	// Lines: default newest first; paging; totals cover every filtered line.
	out, _ = svc.CostTransactions(ctx, finPerms(39, 41), cq, CostTransactionsQuery{Limit: 2})
	lp := out.(domain.CostLinePage)
	if lp.Total != 3 || lp.TotalUSD != 160 || len(lp.Items) != 2 || lp.Items[0].ID != "PD-1" {
		t.Errorf("lines = %+v", lp)
	}
	out, _ = svc.CostTransactions(ctx, finPerms(39, 41), cq, CostTransactionsQuery{Limit: 5, Sort: "-amount"})
	if lp := out.(domain.CostLinePage); lp.Items[0].ID != "INV-1" || *lp.Items[0].Rate != 1 || *lp.Items[0].RateSource != "recorded" {
		t.Errorf("by amount = %+v", lp.Items[0])
	}
}

// The summary, the transactions and both CSVs read one cached set, so their
// totals agree, and trimming for a caller never changes the cached lines.
func TestCostTotalsAgreeAndCacheIsNotMutated(t *testing.T) {
	lines := []engagemententity.DashboardCostEntry{
		entry("INV-1", "invoice", "T1", "2026-10-01", "LKR", 90000), entry("PD-1", "perDiem", "T1", "2026-10-02", "USD", 20),
		entry("CC-1", "card", "T2", "2026-10-03", "USD", 30), entry("EC-1", "claim", "", "2026-10-04", "USD", 40),
	}
	svc, f := costService(lines, &fakeRates{rates: map[string]float64{"LKR": 300}})
	ctx := context.Background()
	q := cq
	q.Compare = false
	// A finance caller who only sees cards and claims first, then a Travel Super Admin: the cached set is the same.
	fin := finPerms(41)
	s1 := mustSummary(t, svc, fin, q)
	if s1.Kpis.Total.Value != 20+30+40 { // per diem, card, claim
		t.Errorf("finance total = %v", s1.Kpis.Total.Value)
	}
	s2 := mustSummary(t, svc, tsaPerms(), q)
	if s2.Kpis.Total.Value != 300+20+30+40 {
		t.Errorf("tsa total = %v (the cached set was trimmed for the first caller?)", s2.Kpis.Total.Value)
	}
	page, _ := svc.CostTransactions(ctx, tsaPerms(), q, CostTransactionsQuery{})
	if page.(domain.CostLinePage).TotalUSD != s2.Kpis.Total.Value {
		t.Error("transactions total differs from the summary")
	}
	srcSum := 0.0
	for _, b := range s2.BySource {
		srcSum += b.Value
	}
	if dashboardRound(srcSum) != s2.Kpis.Total.Value {
		t.Error("bySource does not add up")
	}
	csvT, _ := svc.CostTransactionsCSV(ctx, tsaPerms(), q)
	var totalRow []string
	for _, r := range csvT.Rows {
		if len(r) == 2 && r[0] == "Total USD" {
			totalRow = r
		}
	}
	if totalRow == nil || totalRow[1] != "390.00" {
		t.Errorf("transactions csv total row = %v", totalRow)
	}
	if f.costCalls != 1 {
		t.Errorf("entity calls = %d, want one for the whole sequence (60 s cached set)", f.costCalls)
	}
	// The cached lines still hold all four sources.
	set, _ := svc.(*dashboardService).costSetFor(ctx, q.From, q.To)
	if len(set.lines) != 4 {
		t.Errorf("cached set has %d lines", len(set.lines))
	}
}

func TestCostCSV_HeaderAndSections(t *testing.T) {
	a := entry("INV-1", "invoice", "T1", "2026-10-01", "USD", 10)
	a.Customer, a.Paid, a.Country = sptr("=cmd|' /C calc'!A0"), "paid", sptr("India")
	svc, _ := costService([]engagemententity.DashboardCostEntry{a}, nil)
	ctx := context.Background()
	q := cq
	q.Filter.Sources = []string{"invoice"}
	c, err := svc.TravelCostsCSV(ctx, finPerms(39, 41), q, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Filename != "Travel_Costs_Summary_2026-01-01_2026-11-05.csv" || !strings.HasPrefix(c.Rows[0][0], "Amounts recorded in USD") {
		t.Errorf("file %s first row %v", c.Filename, c.Rows[0])
	}
	text := ""
	for _, r := range c.Rows {
		text += strings.Join(r, ",") + "\n"
	}
	for _, want := range []string{"Travel Costs Summary", "Date range,2026-01-01 to 2026-11-05", "Compared with,2025-01-01 to 2025-11-05",
		"Filters,Source=Travel Invoices", "Currency,USD", "Rate rule,NetSuite month-end rate", "Data as of,2026-11-05 09:00 UTC",
		"Item,This Period,Last Year,Change %", "Spend by Destination Country", "Month,Travel Invoices,Per Diems,Credit Card Expenses,Expense Claims,Total"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary csv lacks %q:\n%s", want, text)
		}
	}
	one, err := svc.TravelCostsCSV(ctx, finPerms(39, 41), cq, "byCountry")
	if err != nil || one.Filename != "Travel_Costs_ByCountry_2026-01-01_2026-11-05.csv" || strings.Contains(fmt.Sprint(one.Rows), "Spend by Source") {
		t.Errorf("section csv: %v %v", one, err)
	}
	if _, err := svc.TravelCostsCSV(ctx, finPerms(39, 41), cq, "nope"); !dashboardIsValidation(err) {
		t.Errorf("bad section: %v", err)
	}
	tx, _ := svc.CostTransactionsCSV(ctx, finPerms(39, 41), cq)
	last := tx.Rows[len(tx.Rows)-1]
	if len(last) != 21 || last[0] != "INV-1" || last[11] != "=cmd|' /C calc'!A0" || last[20] != "recorded" {
		t.Errorf("transactions row = %v", last)
	}
}
