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
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// roleSections adds the Travel Team, Engagement Manager, Account Manager and
// Marketing Events sections the caller wants, each as its own entity call.
func (s *dashboardService) roleSections(ctx context.Context, g *dashboardGroup, p domain.Perms, want map[string]bool, vis widgetSet, todayStr string, today time.Time, out *domain.Dashboard) {
	if want[sectionTravelTeam] {
		g.Go(ctx, []string{sectionTravelTeam}, func() error {
			t, err := cachedAgg(ctx, s, aggKey("travelTeam", todayStr), func(c context.Context) (*engagemententity.DashboardTravelTeam, error) {
				return s.entity.DashboardTravelTeam(c, todayStr)
			})
			if err != nil {
				return err
			}
			out.TravelTeam = dashboardTravelTeam(t, vis)
			return nil
		})
	}
	if want[sectionEngagementManager] {
		types := dashboardAllocateTypes(p)
		g.Go(ctx, []string{sectionEngagementManager}, func() error {
			m, err := cachedAgg(ctx, s, aggKey("manager", []any{todayStr, types}), func(c context.Context) (*engagemententity.DashboardManager, error) {
				return s.entity.DashboardManager(c, todayStr, types)
			})
			if err != nil {
				return err
			}
			out.EngagementManager = dashboardEngagementManager(m, vis)
			return nil
		})
	}
	// Account Manager is "mine" (the caller's own engagements), so it stays live.
	if want[sectionAccountManager] {
		g.Go(ctx, []string{sectionAccountManager}, func() error {
			a, err := s.entity.DashboardAccountManager(ctx, p.Email, todayStr)
			if err != nil {
				return err
			}
			out.AccountManager = dashboardAccountManager(a, today, vis)
			return nil
		})
	}
	if want[sectionMarketingEvents] {
		g.Go(ctx, []string{sectionMarketingEvents}, func() error {
			e, err := cachedAgg(ctx, s, aggKey("events", todayStr), func(c context.Context) (*engagemententity.DashboardEvents, error) {
				return s.entity.DashboardEvents(c, todayStr)
			})
			if err != nil {
				return err
			}
			out.MarketingEvents = dashboardMarketingEvents(e, today, vis)
			return nil
		})
	}
}

// dashboardAllocateTypes are the engagement types the caller may allocate
// (effective privileges); an Allocations Super Admin gets all seven.
func dashboardAllocateTypes(p domain.Perms) []int {
	if domain.HasRole(p.AllocRoles, domain.AllocationsSuperAdminGroup) {
		return append([]int(nil), dashboardEngagementTypeIDs...)
	}
	var out []int
	for _, t := range dashboardEngagementTypeIDs {
		if dashboardCanAllocateType(p.Alloc, t) {
			out = append(out, t)
		}
	}
	return out
}

// dashboardRolePeople lists the people of the role sections, to fill names.
func dashboardRolePeople(d *domain.Dashboard) []*domain.Person {
	var out []*domain.Person
	if t := d.TravelTeam; t != nil {
		for i := range t.FrequentTravellers {
			out = append(out, &t.FrequentTravellers[i].Traveller)
		}
	}
	if m := d.EngagementManager; m != nil {
		if m.StartingIn14Days != nil {
			for i := range m.StartingIn14Days.Items {
				out = append(out, &m.StartingIn14Days.Items[i].Consultant)
			}
		}
		for i := range m.MostAllocated {
			out = append(out, &m.MostAllocated[i].Consultant)
		}
	}
	if e := d.MarketingEvents; e != nil && e.CampaignCoverage != nil {
		for i := range e.CampaignCoverage.ByCreator {
			out = append(out, &e.CampaignCoverage.ByCreator[i].Creator)
		}
	}
	return out
}

// dashboardPoAgeLabels are the four age buckets of the PO queue.
var dashboardPoAgeLabels = []string{"Under 7 days", "7 to 30 days", "31 to 90 days", "Over 90 days"}

func dashboardTravelTeam(t *engagemententity.DashboardTravelTeam, vis widgetSet) *domain.DashboardTravelTeam {
	out := &domain.DashboardTravelTeam{}
	if vis.has("tt-glance") {
		out.AwayThisWeek = &domain.DashboardAway{Travellers: t.AwayTravellers, Countries: t.AwayCountries}
		out.EndedNotCompleted = &domain.DashboardCount{Count: t.EndedNotCompleted}
		out.PoQueue = &domain.DashboardPoQueue{Count: t.POCount, OldestDays: t.POOldestDays}
	}
	if vis.has("tt-po-age") {
		if out.PoQueue == nil {
			out.PoQueue = &domain.DashboardPoQueue{Count: t.POCount, OldestDays: t.POOldestDays}
		}
		total := 0
		for _, n := range t.POByAge {
			total += n
		}
		for i, label := range dashboardPoAgeLabels {
			n := 0
			if i < len(t.POByAge) {
				n = t.POByAge[i]
			}
			b := domain.DashboardBucket{Label: label, Value: float64(n)}
			if total > 0 {
				b.Percent = dashboardRound(float64(n) / float64(total) * 100)
			}
			out.PoQueue.ByAge = append(out.PoQueue.ByAge, b)
		}
	}
	if vis.has("tt-frequent") {
		out.FrequentTravellers = []domain.DashboardFrequentTraveller{}
		for _, f := range t.FrequentTravellers {
			out.FrequentTravellers = append(out.FrequentTravellers, domain.DashboardFrequentTraveller{
				Traveller: domain.Person{Email: f.Label}, Trips: f.Count,
			})
		}
	}
	if vis.has("tt-agent-workload") {
		out.AgentWorkload = dashboardCountBuckets(t.AgentWorkload, 5)
	}
	return out
}

// dashboardEngagementItem maps an entity engagement to the API one.
func dashboardEngagementItem(x engagemententity.DashboardEngagement) domain.DashboardEngagement {
	typeID := 0
	if x.EngagementTypeID != nil {
		typeID = *x.EngagementTypeID
	}
	kind := dashboardKind(typeID)
	var account *string
	if kind == "customer" {
		account = x.Account
	}
	return domain.DashboardEngagement{
		ID: x.ID, Kind: kind, EngagementTypeID: typeID, EngagementTypeName: x.EngagementTypeName,
		Account: account, Title: dashboardStr(x.Title), AllocationType: x.AllocationType,
		EngagementCode: x.EngagementCode, City: x.City, Country: x.Country, StartDate: x.StartDate,
		EndDate: x.EndDate, RequestStatus: x.RequestStatus, EngagementStatus: x.EngagementStatus,
		Consultants: x.Consultants,
	}
}

func dashboardEngagementList(items []engagemententity.DashboardEngagement, total int) domain.DashboardEngagementList {
	out := domain.DashboardEngagementList{Total: total, Items: []domain.DashboardEngagement{}}
	for _, x := range items {
		out.Items = append(out.Items, dashboardEngagementItem(x))
	}
	return out
}

func dashboardEngagementManager(m *engagemententity.DashboardManager, vis widgetSet) *domain.DashboardEngagementManager {
	out := &domain.DashboardEngagementManager{}
	if vis.has("em-no-consultants") {
		l := dashboardEngagementList(m.WithoutConsultants, m.WithoutConsultantsTotal)
		out.WithoutConsultants = &l
	}
	if vis.has("em-starting-14") {
		out.StartingIn14Days = &domain.DashboardStartingList{Total: m.StartingIn14DaysTotal, Items: []domain.DashboardStartingAllocation{}}
		for _, a := range m.StartingIn14Days {
			out.StartingIn14Days.Items = append(out.StartingIn14Days.Items, domain.DashboardStartingAllocation{
				AllocationID: a.AllocationID, Consultant: domain.Person{Email: a.Email}, EngagementID: a.EngagementID,
				EngagementTitle: dashboardEngagementTitle(a.EngagementID, a.EngagementTypeID, a.Account, a.Title),
				StartDate:       a.StartDate, EndDate: a.EndDate, TravelRequired: a.TravelRequired,
			})
		}
	}
	if vis.has("em-most-allocated") {
		out.MostAllocated = []domain.DashboardMostAllocated{}
		for _, c := range m.MostAllocated {
			out.MostAllocated = append(out.MostAllocated, domain.DashboardMostAllocated{Consultant: domain.Person{Email: c.Label}, Days: c.Count})
		}
	}
	if vis.has("em-top-customers") {
		out.TopCustomers = &domain.DashboardTopCustomers{By: m.TopBy, Items: dashboardCountBuckets(m.Top, 8)}
	}
	return out
}

// dashboardMonthsFrom returns n months from today's month, as YYYY-MM.
func dashboardMonthsFrom(today time.Time, n int) []string {
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	out := make([]string, n)
	for i := range out {
		out[i] = first.AddDate(0, i, 0).Format("2006-01")
	}
	return out
}

func dashboardAccountManager(a *engagemententity.DashboardAccountManager, today time.Time, vis widgetSet) *domain.DashboardAccountManager {
	out := &domain.DashboardAccountManager{}
	if vis.has("am-started-no-consultants") {
		l := dashboardEngagementList(a.StartedWithoutConsultants, a.StartedWithoutConsultantsTotal)
		out.StartedWithoutConsultants = &l
	}
	if vis.has("am-by-status") {
		out.ByStatus = dashboardCountBuckets(a.ByStatus, 0)
	}
	if vis.has("am-by-month") {
		out.ByMonth = dashboardMonthSeries(a.ByMonth, dashboardMonthsFrom(today, 6), "Engagements")
	}
	return out
}

func dashboardPercent(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return dashboardRound(float64(part) / float64(total) * 100)
}

func dashboardMarketingEvents(e *engagemententity.DashboardEvents, today time.Time, vis widgetSet) *domain.DashboardMarketingEvents {
	out := &domain.DashboardMarketingEvents{}
	if vis.has("ev-by-month") {
		out.ByMonth = dashboardMonthSeries(e.ByMonth, dashboardMonths(today), "Events")
	}
	if vis.has("ev-by-region") {
		out.ByRegion = dashboardCountBuckets(e.ByRegion, 8)
	}
	if vis.has("ev-funding-bu") {
		out.FundingByBusinessUnit = dashboardCountBuckets(e.FundingByBusinessUnit, 6)
	}
	if !vis.has("ev-campaign") || !e.CampaignAvailable {
		return out
	}
	cov := &domain.DashboardCampaignCoverage{ByCreator: []domain.DashboardCampaignCreator{}}
	creators := append([]engagemententity.DashboardCampaignCreator(nil), e.CampaignCreators...)
	for _, c := range creators {
		cov.Total += c.Total
		cov.WithCampaign += c.WithCampaign
	}
	cov.Percent = dashboardPercent(cov.WithCampaign, cov.Total)
	sort.SliceStable(creators, func(i, j int) bool {
		if creators[i].Total != creators[j].Total {
			return creators[i].Total > creators[j].Total
		}
		return strings.ToLower(creators[i].Email) < strings.ToLower(creators[j].Email)
	})
	for i, c := range creators {
		if i == 5 {
			break
		}
		cov.ByCreator = append(cov.ByCreator, domain.DashboardCampaignCreator{
			Creator: domain.Person{Email: c.Email}, Total: c.Total, WithCampaign: c.WithCampaign,
			Percent: dashboardPercent(c.WithCampaign, c.Total),
		})
	}
	out.CampaignCoverage = cov
	return out
}

// tripVisas pairs each upcoming own trip with the visa picked on it and the
// person's own Active visa that covers it. A trip is waiting for a visa when
// no own visa covers it and the trip's visa is "No Valid Visa Available" or
// ends before the trip does. A trip with no visa picked, or "Visa Not
// Applicable", is not waiting.
func (s *dashboardService) tripVisas(ctx context.Context, email, today string) ([]domain.TripVisa, error) {
	v, err := s.entity.DashboardMyVisas(ctx, email, today)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TripVisa, 0, len(v.Trips))
	for _, t := range v.Trips {
		tv := domain.TripVisa{
			JobNumber: t.JobNumber, Country: dashboardStr(t.Country), City: t.City,
			StartDate: t.StartDate, EndDate: t.EndDate,
		}
		if t.VisaID != nil {
			text := "Visa"
			if t.VisaType != nil && strings.TrimSpace(*t.VisaType) != "" {
				text = strings.TrimSpace(*t.VisaType)
			}
			if t.VisaEnd != nil && *t.VisaEnd != "" {
				text += ", ends " + *t.VisaEnd
			}
			tv.TripVisa = &text
		}
		country := strings.ToLower(strings.TrimSpace(tv.Country))
		for _, own := range v.Visas {
			if country != "" && strings.ToLower(strings.TrimSpace(own.Country)) == country &&
				own.StartDate <= t.StartDate && own.EndDate >= t.EndDate {
				tv.MyVisa = &domain.TripMyVisa{Type: own.VisaType, EndDate: own.EndDate}
				break
			}
		}
		if tv.MyVisa == nil && t.VisaID != nil {
			endsEarly := t.VisaEnd != nil && *t.VisaEnd != "" && *t.VisaEnd < t.EndDate
			tv.Waiting = *t.VisaID == s.noValidVisaID || endsEarly
		}
		out = append(out, tv)
	}
	return out, nil
}
