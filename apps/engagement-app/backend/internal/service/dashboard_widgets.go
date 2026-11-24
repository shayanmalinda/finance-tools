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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Section keys of GET /dashboard?sections=.
const (
	sectionMyWork               = "myWork"
	sectionWaitingOnYou         = "waitingOnYou"
	sectionTravel               = "travel"
	sectionTravelTeam           = "travelTeam"
	sectionEngagementManager    = "engagementManager"
	sectionAccountManager       = "accountManager"
	sectionMarketingEvents      = "marketingEvents"
	sectionEngagements          = "engagements"
	sectionAvailableConsultants = "availableConsultants"
	sectionAllocationsOverTime  = "allocationsOverTime"
)

// DashboardSectionKeys are the keys GET /dashboard accepts in `sections`.
var DashboardSectionKeys = []string{
	sectionMyWork, sectionWaitingOnYou, sectionTravel, sectionTravelTeam, sectionEngagementManager,
	sectionAccountManager, sectionMarketingEvents, sectionEngagements, sectionAvailableConsultants,
	sectionAllocationsOverTime,
}

// widgetWho is who the caller is, for the widget rules: the two Travel and
// Allocations super admin roles, the finance groups and the allocate types.
type widgetWho struct {
	p        domain.Perms
	finance  bool // a travel finance group with own Travel 39 or 41
	tsa      bool // Travel Super Admin
	asa      bool // Allocations Super Admin
	costView bool // finance or Travel Super Admin
}

func newWidgetWho(p domain.Perms) widgetWho {
	w := widgetWho{
		p:       p,
		finance: dashboardHasCardRole(p) && p.TravelOwn.Any(39, 41),
		tsa:     domain.HasRole(p.TravelRoles, domain.TravelSuperAdminGroup),
		asa:     domain.HasRole(p.AllocRoles, domain.AllocationsSuperAdminGroup),
	}
	w.costView = w.finance || w.tsa
	return w
}

// allocatesAny reports whether the set may allocate at least one engagement type.
func allocatesAny(set domain.PrivilegeSet) bool {
	for _, t := range dashboardEngagementTypeIDs {
		if dashboardCanAllocateType(set, t) {
			return true
		}
	}
	return false
}

// dashboardTrendRule is the Allocations Over Time rule on a privilege set.
func dashboardTrendRule(set domain.PrivilegeSet) bool {
	return set.Has(1) && (set.Any(9, 19, 21, 22, 26, 29) ||
		dashboardCanAllocateType(set, 1) || dashboardCanAllocateType(set, 3) || dashboardCanAllocateType(set, 4))
}

// dashboardWidgetDef is one catalogue entry: the section its data comes from
// (blank for the Travel Costs widgets and Allocations Today, which have their
// own endpoints), who may see it and whether it is on by default.
type dashboardWidgetDef struct {
	id        string
	section   string
	visible   func(w widgetWho) bool
	defaultOn func(w widgetWho) bool
}

var (
	always = func(widgetWho) bool { return true }

	// Visibility uses effective privileges and roles (super admins counted);
	// default-on uses the working roles (own privileges), except that the
	// Travel Super Admin counts as the Travel Team's working role.
	visCost    = func(w widgetWho) bool { return w.costView }
	visTravel  = func(w widgetWho) bool { return w.p.Travel.Has(1) || w.tsa }
	defTravel  = func(w widgetWho) bool { return w.p.TravelOwn.Has(1) || w.tsa }
	visAllocT  = func(w widgetWho) bool { return allocatesAny(w.p.Alloc) || w.asa }
	defAllocT  = func(w widgetWho) bool { return allocatesAny(w.p.AllocOwn) }
	visAM      = func(w widgetWho) bool { return w.p.Alloc.Has(2) || w.asa }
	defAM      = func(w widgetWho) bool { return w.p.AllocOwn.Has(2) && !allocatesAny(w.p.AllocOwn) }
	visEvents  = func(w widgetWho) bool { return w.p.Alloc.Any(29, 31) || w.asa }
	defEvents  = func(w widgetWho) bool { return w.p.AllocOwn.Has(31) }
	defFinance = func(w widgetWho) bool { return w.finance }
)

// dashboardWidgetCatalogue is the registry, in screen order (spec section 4).
var dashboardWidgetCatalogue = []dashboardWidgetDef{
	{"my-work", sectionMyWork,
		func(w widgetWho) bool { return w.p.Alloc.Has(1) || w.p.Travel.Any(49, 36) }, always},
	{"waiting-on-you", sectionWaitingOnYou,
		func(w widgetWho) bool { return dashboardPlanQueues(w.p).any() }, always},
	{"tc-kpis", "", visCost, func(w widgetWho) bool { return w.costView }},
	{"tc-source", "", visCost, defFinance},
	{"tc-month", "", visCost, defFinance},
	{"tc-cost-type", "", visCost, func(w widgetWho) bool { return w.costView }},
	{"tc-country", "", visCost, defFinance},
	{"tc-paid", "", visCost, defFinance},
	{"tc-customers", "", visCost, defFinance},
	{"tc-agents", "", visCost, func(w widgetWho) bool { return w.tsa && !w.finance }},
	{"tc-lines", "", visCost, defFinance},
	{"tc-duplicates", "", visCost, defFinance},
	{"travel", sectionTravel, visTravel, defTravel},
	{"tt-glance", sectionTravelTeam, visTravel, defTravel},
	{"tt-po-age", sectionTravelTeam,
		func(w widgetWho) bool { return (w.p.Travel.Has(1) && w.p.Travel.Any(32, 39, 30)) || w.tsa }, defTravel},
	{"tt-frequent", sectionTravelTeam, visTravel, defTravel},
	{"tt-agent-workload", sectionTravelTeam, visTravel, defTravel},
	{"em-no-consultants", sectionEngagementManager, visAllocT, defAllocT},
	{"em-starting-14", sectionEngagementManager, visAllocT, defAllocT},
	{"em-most-allocated", sectionEngagementManager, visAllocT, defAllocT},
	{"em-top-customers", sectionEngagementManager, visAllocT, defAllocT},
	{"am-started-no-consultants", sectionAccountManager, visAM, defAM},
	{"am-by-status", sectionAccountManager, visAM, defAM},
	{"am-by-month", sectionAccountManager, visAM, defAM},
	{"ev-by-month", sectionMarketingEvents, visEvents, defEvents},
	{"ev-by-region", sectionMarketingEvents, visEvents, defEvents},
	{"ev-funding-bu", sectionMarketingEvents, visEvents, defEvents},
	{"ev-campaign", sectionMarketingEvents, visEvents, defEvents},
	{"engagements", sectionEngagements,
		func(w widgetWho) bool { return w.p.Alloc.Any(3, 13) || w.asa },
		func(w widgetWho) bool { return w.p.AllocOwn.Any(3, 13) }},
	{"available-consultants", sectionAvailableConsultants,
		func(w widgetWho) bool { return w.p.Alloc.Any(9, 19, 21) || w.asa },
		func(w widgetWho) bool { return w.p.AllocOwn.Any(9, 19, 21) }},
	{"allocations-today", "",
		func(w widgetWho) bool { return w.p.Alloc.Has(1) && !w.p.IsAgent() },
		func(w widgetWho) bool { return w.p.AllocOwn.Any(9, 19, 21, 22) }},
	{"allocations-over-time", sectionAllocationsOverTime,
		func(w widgetWho) bool { return dashboardTrendRule(w.p.Alloc) || w.asa },
		func(w widgetWho) bool { return dashboardTrendRule(w.p.AllocOwn) }},
}

// dashboardWidgets is the single place that decides which widgets the caller
// may see and which are on by default, in catalogue order. A travel agent has
// the agent dashboard instead: no widgets.
func dashboardWidgets(p domain.Perms) []domain.DashboardWidget {
	out := []domain.DashboardWidget{}
	if p.IsAgent() {
		return out
	}
	w := newWidgetWho(p)
	for _, d := range dashboardWidgetCatalogue {
		if d.visible(w) {
			out = append(out, domain.DashboardWidget{ID: d.id, DefaultOn: d.defaultOn(w)})
		}
	}
	return out
}

// dashboardWidgetSections maps each widget id to its section ("" for none).
func dashboardWidgetSection(id string) string {
	for _, d := range dashboardWidgetCatalogue {
		if d.id == id {
			return d.section
		}
	}
	return ""
}

// dashboardSectionSet returns the sections of the widgets the caller may see
// (all=false: only the ones on by default).
func dashboardSectionSet(widgets []domain.DashboardWidget, defaultsOnly bool) map[string]bool {
	out := map[string]bool{}
	for _, w := range widgets {
		if defaultsOnly && !w.DefaultOn {
			continue
		}
		if sec := dashboardWidgetSection(w.ID); sec != "" {
			out[sec] = true
		}
	}
	return out
}

// widgetSet is the ids of the widgets the caller may see. Section builders
// fill a part of a section only when its widget is in the set, so the API
// never returns data for a widget the caller may not see.
type widgetSet map[string]bool

func (v widgetSet) has(id string) bool { return v[id] }

func newWidgetSet(widgets []domain.DashboardWidget) widgetSet {
	out := widgetSet{}
	for _, w := range widgets {
		out[w.ID] = true
	}
	return out
}
