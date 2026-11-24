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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"reflect"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

const (
	rTSA = "app-travel-admin-super"
	rASA = "app-allocations-admin-super"
)

func widgetIDs(ws []domain.DashboardWidget) string {
	ids := make([]string, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	return strings.Join(ids, ",")
}

func widgetDefaults(ws []domain.DashboardWidget) map[string]bool {
	out := map[string]bool{}
	for _, w := range ws {
		out[w.ID] = w.DefaultOn
	}
	return out
}

func personaPerms(email string, alloc, allocOwn, travel, travelOwn []int, travelRoles, allocRoles []string) domain.Perms {
	p := dashboardPerms(email, alloc, allocOwn, travel, travelOwn, travelRoles...)
	p.AllocRoles = allocRoles
	return p
}

func hasAll(t *testing.T, ws []domain.DashboardWidget, ids ...string) {
	t.Helper()
	got := widgetDefaults(ws)
	for _, id := range ids {
		if _, ok := got[id]; !ok {
			t.Errorf("widget %s missing from %s", id, widgetIDs(ws))
		}
	}
}

func hasNone(t *testing.T, ws []domain.DashboardWidget, ids ...string) {
	t.Helper()
	got := widgetDefaults(ws)
	for _, id := range ids {
		if _, ok := got[id]; ok {
			t.Errorf("widget %s must not be visible: %s", id, widgetIDs(ws))
		}
	}
}

func defaultsOn(ws []domain.DashboardWidget) string {
	var ids []string
	for _, w := range ws {
		if w.DefaultOn {
			ids = append(ids, w.ID)
		}
	}
	return strings.Join(ids, ",")
}

var allTCWidgets = []string{"tc-kpis", "tc-source", "tc-month", "tc-cost-type", "tc-country", "tc-paid", "tc-customers", "tc-agents", "tc-lines"}

func TestDashboardWidgets_Finance(t *testing.T) {
	own := []int{39, 40, 41, 30, 42}
	p := personaPerms("fin@wso2.com", []int{1, 22}, []int{1, 22}, append(append([]int{}, dashboardEmployeeTravel...), own...), append(append([]int{}, dashboardEmployeeTravel...), own...), []string{"app-travel-approver-finance"}, nil)
	ws := dashboardWidgets(p)
	hasAll(t, ws, allTCWidgets...)
	// Default on: every tc-* except tc-agents, plus the Allocations ones its own privileges allow (22).
	want := "my-work,tc-kpis,tc-source,tc-month,tc-cost-type,tc-country,tc-paid,tc-customers,tc-lines,allocations-today"
	// waiting-on-you appears when there is a queue: finance has own 40 (pay).
	want = strings.Replace(want, "my-work,", "my-work,waiting-on-you,", 1)
	if got := defaultsOn(ws); got != want+",allocations-over-time" {
		t.Errorf("finance defaults = %s\nwant %s", got, want+",allocations-over-time")
	}
	hasNone(t, ws, "tt-glance", "em-no-consultants", "am-by-status", "ev-by-month")
}

func TestDashboardWidgets_TravelSuperAdminOnly(t *testing.T) {
	p := personaPerms("tsa@wso2.com", []int{1}, []int{1}, dashboardRange(1, 49), dashboardEmployeeTravel, []string{rTSA}, nil)
	ws := dashboardWidgets(p)
	// Every travel and tc-* widget is visible; no allocation, engagement or event widget.
	hasAll(t, ws, append(allTCWidgets, "travel", "tt-glance", "tt-po-age", "tt-frequent", "tt-agent-workload")...)
	hasNone(t, ws, "em-no-consultants", "am-by-month", "ev-by-month", "engagements", "available-consultants")
	d := widgetDefaults(ws)
	// Default-on follows working roles: tc-lines is off, tc-agents on (a TSA who is not finance).
	if d["tc-lines"] || d["tc-source"] || !d["tc-kpis"] || !d["tc-cost-type"] || !d["tc-agents"] || !d["tt-po-age"] || !d["travel"] {
		t.Errorf("defaults = %v", d)
	}
}

func TestDashboardWidgets_AllocationsSuperAdminOnly(t *testing.T) {
	p := personaPerms("asa@wso2.com", dashboardRange(1, 33), []int{1}, dashboardEmployeeTravel, dashboardEmployeeTravel, nil, []string{rASA})
	ws := dashboardWidgets(p)
	hasAll(t, ws, "em-no-consultants", "em-starting-14", "em-most-allocated", "em-top-customers", "am-started-no-consultants", "am-by-status", "am-by-month",
		"ev-by-month", "ev-by-region", "ev-funding-bu", "ev-campaign", "engagements", "available-consultants", "allocations-today", "allocations-over-time")
	hasNone(t, ws, append(allTCWidgets, "travel", "tt-glance")...)
	d := widgetDefaults(ws)
	for _, id := range []string{"em-no-consultants", "am-by-status", "ev-by-month", "engagements", "available-consultants", "allocations-over-time"} {
		if d[id] {
			t.Errorf("%s is on by default for a pure super admin", id)
		}
	}
}

func TestDashboardWidgets_BothSuperAdminsSeeEveryWidget(t *testing.T) {
	p := personaPerms("both@wso2.com", dashboardRange(1, 33), []int{1}, dashboardRange(1, 49), dashboardEmployeeTravel, []string{rTSA}, []string{rASA})
	ws := dashboardWidgets(p)
	if len(ws) != len(dashboardWidgetCatalogue)-1 { // waiting-on-you needs a queue from own privileges
		t.Errorf("%d widgets, want every one but waiting-on-you: %s", len(ws), widgetIDs(ws))
	}
	hasAll(t, ws, "tc-lines", "tt-po-age", "ev-campaign", "em-top-customers")
}

// A super admin of one app with roles of the other: the union applies.
func TestDashboardWidgets_UnionAcrossApps(t *testing.T) {
	p := personaPerms("mix@wso2.com", []int{1, 2}, []int{1, 2}, dashboardRange(1, 49), dashboardEmployeeTravel, []string{rTSA}, nil)
	ws := dashboardWidgets(p)
	hasAll(t, ws, "tc-kpis", "tt-glance", "am-by-status", "am-by-month")
	d := widgetDefaults(ws)
	if !d["am-by-status"] || d["ev-by-month"] {
		t.Errorf("defaults = %v", d)
	}
}

func TestDashboardWidgets_OtherPersonas(t *testing.T) {
	// Pre/Post-sale engagement manager.
	em := []int{1, 3, 6, 9, 13, 16, 32, 33}
	ws := dashboardWidgets(personaPerms("em@wso2.com", em, em, dashboardEmployeeTravel, dashboardEmployeeTravel, nil, nil))
	hasAll(t, ws, "em-no-consultants", "engagements", "available-consultants", "allocations-today", "allocations-over-time")
	if d := widgetDefaults(ws); !d["em-top-customers"] || !d["engagements"] || d["am-by-status"] {
		t.Errorf("em defaults = %v", d)
	}
	// Account manager: Allocations 2 and no allocate right.
	am := []int{1, 2, 3}
	ws = dashboardWidgets(personaPerms("am@wso2.com", am, am, nil, nil, nil, nil))
	if d := widgetDefaults(ws); !d["am-started-no-consultants"] || !d["am-by-month"] || !d["engagements"] {
		t.Errorf("am defaults = %v", d)
	}
	hasNone(t, ws, "em-no-consultants")
	// Marketing event approver.
	ev := []int{1, 29, 31}
	ws = dashboardWidgets(personaPerms("ev@wso2.com", ev, ev, nil, nil, nil, nil))
	if got := defaultsOn(ws); got != "my-work,ev-by-month,ev-by-region,ev-funding-bu,ev-campaign,allocations-over-time" {
		t.Errorf("ev defaults = %s", got)
	}
	// A travel agent has no widgets.
	agent := dashboardPerms("ag@x.com", nil, nil, []int{18}, []int{18})
	agent.Agency = &domain.TravelAgency{ID: 3, Name: "Birch"}
	if ws := dashboardWidgets(agent); len(ws) != 0 || ws == nil {
		t.Errorf("agent widgets = %v", ws)
	}
	// An employee has My Work, and Allocations Today in Manage Widgets.
	ws = dashboardWidgets(dashboardPerms("e@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel))
	if widgetIDs(ws) != "my-work,allocations-today" {
		t.Errorf("employee widgets = %s", widgetIDs(ws))
	}
}

// A caller who passes the Travel Team section's widest rule (Travel 1) but not
// tt-po-age (no 32, 39 or 30, not a Travel Super Admin) gets no byAge buckets.
func TestDashboard_TravelTeamPerWidgetData(t *testing.T) {
	p := dashboardPerms("tt@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, append([]int{1}, dashboardEmployeeTravel...), append([]int{1}, dashboardEmployeeTravel...))
	f := &fakeDashboardEntity{}
	got, err := NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(), p, []string{"travelTeam"})
	if err != nil || got.TravelTeam == nil {
		t.Fatalf("travelTeam = %+v, %v", got.TravelTeam, err)
	}
	hasNone(t, got.Widgets, "tt-po-age")
	if got.TravelTeam.PoQueue == nil || got.TravelTeam.PoQueue.Count != 10 || got.TravelTeam.PoQueue.ByAge != nil {
		t.Errorf("poQueue = %+v, want the glance count and no byAge", got.TravelTeam.PoQueue)
	}
	if got.TravelTeam.FrequentTravellers == nil || got.TravelTeam.AwayThisWeek == nil {
		t.Errorf("other parts missing: %+v", got.TravelTeam)
	}

	// With Travel 32 the byAge buckets appear: four, always.
	p.Travel = domain.NewPrivilegeSet(append([]int{1, 32}, dashboardEmployeeTravel...))
	got, _ = NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(), p, []string{"travelTeam"})
	if len(got.TravelTeam.PoQueue.ByAge) != 4 || got.TravelTeam.PoQueue.ByAge[3].Label != "Over 90 days" || got.TravelTeam.PoQueue.ByAge[3].Value != 4 {
		t.Errorf("byAge = %+v", got.TravelTeam.PoQueue.ByAge)
	}
}

// The other role sections fill only the parts whose widget is visible.
func TestDashboardRoleSections_PartsFollowWidgets(t *testing.T) {
	only := func(ids ...string) widgetSet {
		v := widgetSet{}
		for _, id := range ids {
			v[id] = true
		}
		return v
	}
	m := dashboardEngagementManager(&engagemententity.DashboardManager{TopBy: "country"}, only("em-top-customers"))
	if m.WithoutConsultants != nil || m.StartingIn14Days != nil || m.MostAllocated != nil || m.TopCustomers == nil || m.TopCustomers.By != "country" {
		t.Errorf("manager = %+v", m)
	}
	a := dashboardAccountManager(&engagemententity.DashboardAccountManager{}, dashboardNow(), only("am-by-month"))
	if a.StartedWithoutConsultants != nil || a.ByStatus != nil || len(a.ByMonth) != 6 || a.ByMonth[0].Month != "2026-11" || a.ByMonth[5].Month != "2027-04" {
		t.Errorf("account manager = %+v", a)
	}
	ev := &engagemententity.DashboardEvents{CampaignAvailable: true, CampaignCreators: []engagemententity.DashboardCampaignCreator{{Email: "a@x.com", Total: 6, WithCampaign: 5}, {Email: "b@x.com", Total: 14, WithCampaign: 10}}}
	e := dashboardMarketingEvents(ev, dashboardNow(), only("ev-campaign"))
	if e.ByMonth != nil || e.ByRegion != nil || e.CampaignCoverage == nil || e.CampaignCoverage.Total != 20 || e.CampaignCoverage.Percent != 75 ||
		e.CampaignCoverage.ByCreator[0].Creator.Email != "b@x.com" || e.CampaignCoverage.ByCreator[1].Percent != 83.33 {
		t.Errorf("events = %+v", e)
	}
	// Without the column there is no coverage.
	if e := dashboardMarketingEvents(&engagemententity.DashboardEvents{}, dashboardNow(), only("ev-campaign", "ev-by-month")); e.CampaignCoverage != nil || len(e.ByMonth) != 12 {
		t.Errorf("events without campaign = %+v", e)
	}
}

func TestDashboard_SectionsParam(t *testing.T) {
	p := dashboardPerms("e@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel)
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	if _, err := svc.Dashboard(context.Background(), p, []string{"myWork", "nope"}); !dashboardIsValidation(err) {
		t.Errorf("unknown section: %v", err)
	}
	if _, err := svc.Dashboard(context.Background(), p, []string{"travelCost"}); !dashboardIsValidation(err) {
		t.Errorf("travelCost is not a section key: %v", err)
	}
	// A section the caller may not see is left out without error and without a call.
	got, err := svc.Dashboard(context.Background(), p, []string{"travelTeam", "engagementManager", "accountManager", "marketingEvents", "myWork"})
	if err != nil || got.TravelTeam != nil || got.EngagementManager != nil || got.AccountManager != nil || got.MarketingEvents != nil {
		t.Errorf("hidden sections returned: %+v %v", got, err)
	}
	if len(f.calls) != 0 || f.myWork == nil {
		t.Errorf("entity calls = %v, myWork %+v", f.calls, f.myWork)
	}
	// An empty list asks for nothing.
	f.myWork = nil
	if got, _ := svc.Dashboard(context.Background(), p, []string{}); f.myWork != nil || got.MyWork != nil {
		t.Error("empty sections still built My Work")
	}
}

// The Engagement Manager types are the effective allocate types; an
// Allocations Super Admin gets all seven.
func TestDashboard_EngagementManagerTypes(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	post := []int{1, 6, 32}
	p := dashboardPerms("em@wso2.com", post, post, nil, nil)
	if _, err := svc.Dashboard(context.Background(), p, []string{"engagementManager"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.managerArgs, []int{2, 4, 6}) {
		t.Errorf("types = %v, want the post sale ones plus the retired Paid", f.managerArgs)
	}
	asa := personaPerms("asa@wso2.com", dashboardRange(1, 33), []int{1}, nil, nil, nil, []string{rASA})
	if _, err := svc.Dashboard(context.Background(), asa, []string{"engagementManager"}); err != nil || len(f.managerArgs) != 7 {
		t.Errorf("super admin types = %v, %v", f.managerArgs, err)
	}
}

func TestMeOverview_TripVisas(t *testing.T) {
	f := &fakeDashboardEntity{
		myWorkResp: &engagemententity.DashboardMyWork{},
		visasResp: &engagemententity.DashboardMyVisas{
			Trips: []engagemententity.DashboardMyVisaTrip{
				{JobNumber: "T1", Country: sptr("India"), StartDate: "2026-12-01", EndDate: "2026-12-10", VisaID: iptr(12), VisaType: sptr("B1/B2"), VisaEnd: sptr("2027-03-01")},
				{JobNumber: "T2", Country: sptr("France"), StartDate: "2026-12-01", EndDate: "2026-12-10", VisaID: iptr(333)},
				{JobNumber: "T3", Country: sptr("Japan"), StartDate: "2026-12-01", EndDate: "2026-12-10", VisaID: iptr(12), VisaType: sptr("Tourist"), VisaEnd: sptr("2026-12-05")},
				{JobNumber: "T4", Country: sptr("Spain"), StartDate: "2026-12-01", EndDate: "2026-12-10"},
				{JobNumber: "T5", Country: sptr("Brazil"), StartDate: "2026-12-01", EndDate: "2026-12-10", VisaID: iptr(332)},
				{JobNumber: "T6", Country: sptr("Peru"), StartDate: "2026-12-01", EndDate: "2026-12-10", VisaID: iptr(333)},
			},
			Visas: []engagemententity.DashboardOwnVisa{
				{Country: " india ", VisaType: "B1/B2", StartDate: "2026-01-01", EndDate: "2027-03-01"},
				{Country: "Peru", VisaType: "Tourist", StartDate: "2026-12-05", EndDate: "2027-01-01"}, // does not cover the whole trip
			},
		},
	}
	got, err := NewDashboardService(f, nil, nil, dashboardNow).MeOverview(context.Background(), dashboardPerms("j@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel))
	if err != nil || len(got.TripVisas) != 6 {
		t.Fatalf("tripVisas = %+v, %v", got.TripVisas, err)
	}
	v := got.TripVisas
	if v[0].MyVisa == nil || v[0].Waiting || *v[0].TripVisa != "B1/B2, ends 2027-03-01" {
		t.Errorf("covered by an own visa: %+v", v[0])
	}
	if v[1].MyVisa != nil || !v[1].Waiting {
		t.Errorf("No Valid Visa Available: %+v", v[1])
	}
	if !v[2].Waiting {
		t.Errorf("visa ends before the trip: %+v", v[2])
	}
	if v[3].Waiting || v[3].TripVisa != nil {
		t.Errorf("no visa picked is not waiting: %+v", v[3])
	}
	if v[4].Waiting {
		t.Errorf("Visa Not Applicable is not waiting: %+v", v[4])
	}
	if !v[5].Waiting {
		t.Errorf("a partial own visa does not cover the trip: %+v", v[5])
	}
	// Without own trips access there is no visa call.
	f.calls = nil
	got, _ = NewDashboardService(f, nil, nil, dashboardNow).MeOverview(context.Background(), dashboardPerms("j@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, nil, nil))
	if got.TripVisas != nil || len(f.calls) != 0 {
		t.Errorf("visas read without trips access: %+v %v", got.TripVisas, f.calls)
	}
}

func sptr(s string) *string { return &s }
func iptr(n int) *int       { return &n }

func dashboardIsValidation(err error) bool {
	var v *apierror.ValidationError
	return errors.As(err, &v)
}

// The visas are an extra on the card: a failed lookup leaves the allocations and trips already read.
func TestMeOverview_VisaFailureKeepsTheRest(t *testing.T) {
	f := &fakeDashboardEntity{
		myWorkResp: &engagemententity.DashboardMyWork{Trips: []engagemententity.DashboardTrip{{JobNumber: "T1", StartDate: "2026-12-01", EndDate: "2026-12-10"}}},
		visasErr:   errors.New("hr database down"),
	}
	got, err := NewDashboardService(f, nil, nil, dashboardNow).MeOverview(context.Background(), dashboardPerms("j@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel))
	if err != nil || len(got.UpcomingTrips) != 1 || got.TripVisas != nil {
		t.Errorf("overview = %+v, %v; want the trip and no visas", got, err)
	}
}
