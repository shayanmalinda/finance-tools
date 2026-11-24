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
func (s *dashboardService) roleSections(ctx context.Context, g *dashboardGroup, p domain.Perms, want map[string]bool, todayStr string, today time.Time, out *domain.Dashboard) {
	if want[sectionTravelTeam] {
		g.Go(ctx, []string{sectionTravelTeam}, func() error {
			t, err := s.entity.DashboardTravelTeam(ctx, todayStr)
			if err != nil {
				return err
			}
			out.TravelTeam = dashboardTravelTeam(t)
			return nil
		})
	}
	if want[sectionEngagementManager] {
		types := dashboardAllocateTypes(p)
		g.Go(ctx, []string{sectionEngagementManager}, func() error {
			m, err := s.entity.DashboardManager(ctx, todayStr, types)
			if err != nil {
				return err
			}
			out.EngagementManager = dashboardEngagementManager(m)
			return nil
		})
	}
	if want[sectionAccountManager] {
		g.Go(ctx, []string{sectionAccountManager}, func() error {
			a, err := s.entity.DashboardAccountManager(ctx, p.Email, todayStr)
			if err != nil {
				return err
			}
			out.AccountManager = dashboardAccountManager(a, today)
			return nil
		})
	}
	if want[sectionMarketingEvents] {
		g.Go(ctx, []string{sectionMarketingEvents}, func() error {
			e, err := s.entity.DashboardEvents(ctx, todayStr)
			if err != nil {
				return err
			}
			out.MarketingEvents = dashboardMarketingEvents(e, today)
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
		for i := range m.StartingIn14Days.Items {
			out = append(out, &m.StartingIn14Days.Items[i].Consultant)
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

func dashboardTravelTeam(t *engagemententity.DashboardTravelTeam) *domain.DashboardTravelTeam {
	out := &domain.DashboardTravelTeam{FrequentTravellers: []domain.DashboardFrequentTraveller{}}
	out.AwayThisWeek.Travellers, out.AwayThisWeek.Countries = t.AwayTravellers, t.AwayCountries
	out.EndedNotCompleted.Count = t.EndedNotCompleted
	out.PoQueue = domain.DashboardPoQueue{Count: t.POCount, OldestDays: t.POOldestDays}
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
	for _, f := range t.FrequentTravellers {
		out.FrequentTravellers = append(out.FrequentTravellers, domain.DashboardFrequentTraveller{
			Traveller: domain.Person{Email: f.Label}, Trips: f.Count,
		})
	}
	out.AgentWorkload = dashboardCountBuckets(t.AgentWorkload, 5)
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

func dashboardEngagementManager(m *engagemententity.DashboardManager) *domain.DashboardEngagementManager {
	out := &domain.DashboardEngagementManager{
		WithoutConsultants: dashboardEngagementList(m.WithoutConsultants, m.WithoutConsultantsTotal),
		MostAllocated:      []domain.DashboardMostAllocated{},
	}
	out.StartingIn14Days.Total = m.StartingIn14DaysTotal
	out.StartingIn14Days.Items = []domain.DashboardStartingAllocation{}
	for _, a := range m.StartingIn14Days {
		out.StartingIn14Days.Items = append(out.StartingIn14Days.Items, domain.DashboardStartingAllocation{
			AllocationID: a.AllocationID, Consultant: domain.Person{Email: a.Email}, EngagementID: a.EngagementID,
			EngagementTitle: dashboardEngagementTitle(a.EngagementID, a.EngagementTypeID, a.Account, a.Title),
			StartDate:       a.StartDate, EndDate: a.EndDate, TravelRequired: a.TravelRequired,
		})
	}
	for _, c := range m.MostAllocated {
		out.MostAllocated = append(out.MostAllocated, domain.DashboardMostAllocated{Consultant: domain.Person{Email: c.Label}, Days: c.Count})
	}
	out.TopCustomers.By = m.TopBy
	out.TopCustomers.Items = dashboardCountBuckets(m.Top, 8)
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

func dashboardAccountManager(a *engagemententity.DashboardAccountManager, today time.Time) *domain.DashboardAccountManager {
	return &domain.DashboardAccountManager{
		StartedWithoutConsultants: dashboardEngagementList(a.StartedWithoutConsultants, a.StartedWithoutConsultantsTotal),
		ByStatus:                  dashboardCountBuckets(a.ByStatus, 0),
		ByMonth:                   dashboardMonthSeries(a.ByMonth, dashboardMonthsFrom(today, 6), "Engagements"),
	}
}

func dashboardPercent(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return dashboardRound(float64(part) / float64(total) * 100)
}

func dashboardMarketingEvents(e *engagemententity.DashboardEvents, today time.Time) *domain.DashboardMarketingEvents {
	out := &domain.DashboardMarketingEvents{
		ByMonth:               dashboardMonthSeries(e.ByMonth, dashboardMonths(today), "Events"),
		ByRegion:              dashboardCountBuckets(e.ByRegion, 8),
		FundingByBusinessUnit: dashboardCountBuckets(e.FundingByBusinessUnit, 6),
	}
	if !e.CampaignAvailable {
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
