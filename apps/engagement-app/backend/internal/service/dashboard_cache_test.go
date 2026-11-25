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
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func callCount(f *fakeDashboardEntity, name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func twoCostLines() []engagemententity.DashboardCostEntry {
	return []engagemententity.DashboardCostEntry{
		entry("INV-1", "invoice", "TRV-1", "2026-10-14", "USD", 100),
		entry("PD-1", "perDiem", "TRV-1", "2026-10-15", "USD", 40),
	}
}

func TestDashboardCache_TTLFromConfig(t *testing.T) {
	f := &fakeDashboardEntity{}
	if s := NewDashboardService(f, nil, nil, dashboardNow).(*dashboardService); s.cacheTTL != config.DefaultDashboardCacheTTL || config.DefaultDashboardCacheTTL != time.Hour {
		t.Errorf("default TTL = %v, want 1h", s.cacheTTL)
	}
	if s := NewDashboardService(f, nil, nil, dashboardNow, WithCacheTTL(30*time.Minute)).(*dashboardService); s.cacheTTL != 30*time.Minute || s.costCache.ttl != 30*time.Minute || s.aggCache.ttl != 30*time.Minute {
		t.Errorf("override TTL = %v", s.cacheTTL)
	}

	// 0 disables: every request reads the entity again.
	off := NewDashboardService(f, nil, nil, dashboardNow, WithCacheTTL(0))
	p := dashboardPerms("tt@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, append([]int{1}, dashboardEmployeeTravel...), append([]int{1}, dashboardEmployeeTravel...))
	for i := 0; i < 2; i++ {
		if _, err := off.Dashboard(context.Background(), p, []string{"travelTeam"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := callCount(f, "travelTeam"); n != 2 {
		t.Errorf("entity calls with TTL 0 = %d, want 2", n)
	}
	cf := &fakeDashboardEntity{costResp: &engagemententity.DashboardCostLines{Items: twoCostLines()}}
	offCost := NewDashboardService(cf, nil, nil, dashboardNow, WithCacheTTL(0))
	mustSummary(t, offCost, tsaPerms(), cq)
	mustSummary(t, offCost, tsaPerms(), cq)
	if cf.costCalls != 4 { // current + previous range, twice
		t.Errorf("cost reads with TTL 0 = %d, want 4", cf.costCalls)
	}
}

// Two callers with different privileges share one cost read and still get
// different answers, because the privilege filter runs after the cache.
func TestDashboardCache_CostSetSharedButFilteredPerCaller(t *testing.T) {
	svc, f := costService(twoCostLines(), nil)
	full := mustSummary(t, svc, tsaPerms(), cq)
	reads := f.costCalls
	// Travel 39 only: invoices, no per diem section (41, 42).
	limited := mustSummary(t, svc, finPerms(39), cq)
	if f.costCalls != reads {
		t.Errorf("second caller read the entity again: %d -> %d", reads, f.costCalls)
	}
	if full.Kpis.Total.Value != 140 {
		t.Errorf("full total = %v, want 140", full.Kpis.Total.Value)
	}
	if limited.Kpis.Total.Value == full.Kpis.Total.Value {
		t.Errorf("limited caller got the full total %v: the cache shared a filtered answer", limited.Kpis.Total.Value)
	}
	// And the full caller is unchanged by the limited caller's read.
	if again := mustSummary(t, svc, tsaPerms(), cq); again.Kpis.Total.Value != 140 {
		t.Errorf("full total after = %v", again.Kpis.Total.Value)
	}
}

// Data As Of is when the cached data was loaded, not when it was asked for.
func TestDashboardCache_DataAsOfIsLoadTime(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	f := &fakeDashboardEntity{costResp: &engagemententity.DashboardCostLines{Items: twoCostLines()}}
	svc := NewDashboardService(f, nil, nil, func() time.Time { return now })
	first := mustSummary(t, svc, tsaPerms(), cq)
	now = now.Add(30 * time.Minute)
	second := mustSummary(t, svc, finPerms(39, 41), cq)
	if first.DataAsOf != "2026-11-05T09:00:00Z" || second.DataAsOf != first.DataAsOf {
		t.Errorf("dataAsOf = %s then %s, want the 09:00 load time both times", first.DataAsOf, second.DataAsOf)
	}
}

// A failed read is not kept; a partial set is kept for a minute at most; a
// complete set for the full TTL.
func TestDashboardCache_FailuresAndPartialSetsAreNotKeptLong(t *testing.T) {
	f := &fakeDashboardEntity{costResp: &engagemententity.DashboardCostLines{Items: twoCostLines()}, fail: errors.New("down")}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	if _, err := svc.TravelCosts(context.Background(), tsaPerms(), cq); err == nil {
		t.Fatal("want an error")
	}
	f.fail = nil
	if s := mustSummary(t, svc, tsaPerms(), cq); s.Kpis.Total.Value != 140 {
		t.Errorf("after recovery total = %v: the failure was kept", s.Kpis.Total.Value)
	}

	clock := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	c := newCostSetCache(4, time.Hour, func() time.Time { return clock })
	ctx := context.Background()
	loads := 0
	load := func(set *costSet) func() (*costSet, error) {
		return func() (*costSet, error) { loads++; return set, nil }
	}
	_, _ = c.get(ctx, "partial", load(&costSet{failed: []string{"card"}}))
	_, _ = c.get(ctx, "truncated", load(&costSet{truncated: true}))
	_, _ = c.get(ctx, "ok", load(&costSet{}))
	clock = clock.Add(2 * time.Minute)
	loads = 0
	_, _ = c.get(ctx, "partial", load(&costSet{}))
	_, _ = c.get(ctx, "truncated", load(&costSet{}))
	_, _ = c.get(ctx, "ok", load(&costSet{}))
	if loads != 2 {
		t.Errorf("reloads after 2 minutes = %d, want 2 (the partial and truncated sets, not the complete one)", loads)
	}
	clock = clock.Add(time.Hour)
	loads = 0
	_, _ = c.get(ctx, "ok", load(&costSet{}))
	if loads != 1 {
		t.Errorf("complete set after the TTL: loads = %d, want 1", loads)
	}
}

// The cache is bounded by entries and by lines.
func TestDashboardCache_MemoryBound(t *testing.T) {
	c := newCostSetCache(3, time.Hour, nil)
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c", "d"} {
		_, _ = c.get(ctx, k, func() (*costSet, error) { return &costSet{}, nil })
	}
	if len(c.items) != 3 || c.items["a"] != nil {
		t.Errorf("entries = %d, want the oldest dropped at 3", len(c.items))
	}
	big := func() (*costSet, error) { return &costSet{lines: make([]costLine, costSetMaxLines/2+1)}, nil }
	_, _ = c.get(ctx, "big1", big)
	_, _ = c.get(ctx, "big2", big) // together over the line budget: big1 goes
	if c.total > costSetMaxLines || c.items["big1"] != nil || c.items["big2"] == nil {
		t.Errorf("total lines = %d, big1 kept = %v", c.total, c.items["big1"] != nil)
	}
	_, _ = c.get(ctx, "huge", func() (*costSet, error) { return &costSet{lines: make([]costLine, costSetMaxLines+1)}, nil })
	if c.items["huge"] != nil {
		t.Error("a set over the whole budget must be served, not kept")
	}
}

// Role sections are shared per query; "mine" sections stay live.
func TestDashboardCache_SharedSectionsCachedMineStaysLive(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	travel := append([]int{1, 32}, dashboardEmployeeTravel...)
	a := dashboardPerms("a@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, travel, travel)
	b := dashboardPerms("b@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, append([]int{1}, dashboardEmployeeTravel...), append([]int{1}, dashboardEmployeeTravel...))
	ga, err := svc.Dashboard(context.Background(), a, []string{"travelTeam"})
	if err != nil {
		t.Fatal(err)
	}
	gb, err := svc.Dashboard(context.Background(), b, []string{"travelTeam"})
	if err != nil {
		t.Fatal(err)
	}
	if n := callCount(f, "travelTeam"); n != 1 {
		t.Errorf("travelTeam entity reads = %d, want 1 shared", n)
	}
	// Same cached data, different widgets: only a (Travel 32) gets the PO age buckets.
	if len(ga.TravelTeam.PoQueue.ByAge) != 4 || gb.TravelTeam.PoQueue.ByAge != nil {
		t.Errorf("byAge a=%v b=%v: privileges must be applied after the cache", ga.TravelTeam.PoQueue.ByAge, gb.TravelTeam.PoQueue.ByAge)
	}

	// Waiting on You and Account Manager are personal: read every time.
	poOwn := append(dashboardRange(1, 17), 25, 26, 27, 28, 30, 32, 41, 42, 45, 46)
	q := dashboardPerms("q@wso2.com", dashboardRange(1, 33), []int{1}, dashboardRange(1, 49), poOwn, "app-travel-admin-super-stg")
	f.queuesResp = &engagemententity.DashboardQueues{NewTravelRequests: 4}
	first, _ := svc.Dashboard(context.Background(), q, nil)
	f.mu.Lock()
	f.queuesResp = &engagemententity.DashboardQueues{NewTravelRequests: 9}
	f.mu.Unlock()
	second, _ := svc.Dashboard(context.Background(), q, nil)
	if first.WaitingOnYou[0].Count != 4 || second.WaitingOnYou[0].Count != 9 {
		t.Errorf("queue counts %d then %d: Waiting on You must stay live", first.WaitingOnYou[0].Count, second.WaitingOnYou[0].Count)
	}
	svc2 := NewDashboardService(f, nil, nil, dashboardNow)
	am := dashboardPerms("am@wso2.com", dashboardRange(1, 33), dashboardRange(1, 33), dashboardEmployeeTravel, dashboardEmployeeTravel)
	for i := 0; i < 2; i++ {
		_, _ = svc2.Dashboard(context.Background(), am, []string{"accountManager"})
	}
	if n := callCount(f, "accountManager"); n != 2 {
		t.Errorf("accountManager reads = %d, want 2: every request live", n)
	}
}

// An agent is cached per agency: another agency never sees it.
func TestDashboardCache_AgentKeyedByAgency(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	agent := func(id int, name string) domain.Perms {
		p := dashboardPerms(name+"@agency.com", nil, nil, []int{18}, []int{18})
		p.Agency = &domain.TravelAgency{ID: id, Name: name}
		return p
	}
	a1, err := svc.Dashboard(context.Background(), agent(1, "Classic"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := svc.Dashboard(context.Background(), agent(2, "Other"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.agencyID != 2 {
		t.Errorf("second agency's read used agency %d, want 2: it was served agency 1's cache", f.agencyID)
	}
	if a1.Agent.Agency != "Classic" || a2.Agent.Agency != "Other" {
		t.Errorf("agencies = %q, %q", a1.Agent.Agency, a2.Agent.Agency)
	}
	if _, err := svc.Dashboard(context.Background(), agent(1, "Classic"), nil); err != nil || f.agencyID != 2 {
		t.Errorf("agency 1 again should be a cache hit (last read agency %d, err %v)", f.agencyID, err)
	}
}

func TestDashboardCache_InvalidateOnWrite(t *testing.T) {
	svc, f := costService(twoCostLines(), nil)
	mustSummary(t, svc, tsaPerms(), cq)
	reads := f.costCalls
	mustSummary(t, svc, tsaPerms(), cq)
	if f.costCalls != reads {
		t.Fatal("expected a cache hit before the write")
	}
	svc.(CacheInvalidator).InvalidateCache()
	mustSummary(t, svc, tsaPerms(), cq)
	if f.costCalls == reads {
		t.Error("InvalidateCache did not drop the cost sets")
	}

	// A load that began before the clear is returned but not kept.
	c := newLoadCache[string](4, time.Hour, 2, nil)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = c.get(context.Background(), "k", func() (string, error) { close(started); <-release; return "old", nil })
		close(done)
	}()
	<-started
	c.clear()
	close(release)
	<-done
	if len(c.items) != 0 {
		t.Error("a load that started before the clear was kept")
	}
}
