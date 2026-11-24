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

package engagemententity

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// DashboardCostLinesFilter is the query of GET /dashboard/cost-lines. From
// and To are inclusive days. Sources, JobNumbers and Employees are pushed
// down to SQL; the BFF leaves them empty in normal use.
type DashboardCostLinesFilter struct {
	From, To   string
	Sources    []string
	JobNumbers []string
	Employees  []string
	Limit      int
}

// DashboardCostEntry is one cost line in its original amount and currency.
// CCAmount and CCCurrency are set on card lines: what the card was billed,
// and "USD" when it was billed in USD.
type DashboardCostEntry struct {
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Reference        *string `json:"reference"`
	JobNumber        *string `json:"jobNumber"`
	TravellerEmail   *string `json:"travellerEmail"`
	Date             string  `json:"date"`
	OriginalAmount   float64 `json:"originalAmount"`
	OriginalCurrency string  `json:"originalCurrency"`
	CCAmount         float64 `json:"ccAmount"`
	CCCurrency       string  `json:"ccCurrency"`
	CostType         string  `json:"costType"`
	SourceType       *string `json:"sourceType"`
	Country          *string `json:"country"`
	TravelAgentID    *int    `json:"travelAgentId"`
	TravelAgentName  *string `json:"travelAgentName"`
	Customer         *string `json:"customer"`
	Paid             string  `json:"paid"`
	EngagementID     *string `json:"engagementId"`
	AllocationType   *string `json:"allocationType"`
}

// DashboardCostLines is GET /dashboard/cost-lines. Truncated names the
// sources that hit the line cap and Unavailable the ones that could not be
// read.
type DashboardCostLines struct {
	Items       []DashboardCostEntry `json:"items"`
	Truncated   []string             `json:"truncated"`
	Unavailable []string             `json:"unavailable"`
}

// DashboardTravelTeam is GET /dashboard/travel-team.
type DashboardTravelTeam struct {
	AwayTravellers     int              `json:"awayTravellers"`
	AwayCountries      int              `json:"awayCountries"`
	EndedNotCompleted  int              `json:"endedNotCompleted"`
	POCount            int              `json:"poCount"`
	POOldestDays       int              `json:"poOldestDays"`
	POByAge            []int            `json:"poByAge"`
	FrequentTravellers []DashboardCount `json:"frequentTravellers"`
	AgentWorkload      []DashboardCount `json:"agentWorkload"`
}

// DashboardStartingAllocation is an allocation starting soon.
type DashboardStartingAllocation struct {
	AllocationID     string  `json:"allocationId"`
	Email            string  `json:"email"`
	EngagementID     string  `json:"engagementId"`
	EngagementTypeID *int    `json:"engagementTypeId"`
	Account          *string `json:"account"`
	Title            *string `json:"title"`
	StartDate        string  `json:"startDate"`
	EndDate          string  `json:"endDate"`
	TravelRequired   bool    `json:"travelRequired"`
}

// DashboardManager is GET /dashboard/manager.
type DashboardManager struct {
	WithoutConsultants      []DashboardEngagement         `json:"withoutConsultants"`
	WithoutConsultantsTotal int                           `json:"withoutConsultantsTotal"`
	StartingIn14Days        []DashboardStartingAllocation `json:"startingIn14Days"`
	StartingIn14DaysTotal   int                           `json:"startingIn14DaysTotal"`
	MostAllocated           []DashboardCount              `json:"mostAllocated"`
	TopBy                   string                        `json:"topBy"`
	Top                     []DashboardCount              `json:"top"`
}

// DashboardAccountManager is GET /dashboard/account-manager.
type DashboardAccountManager struct {
	StartedWithoutConsultants      []DashboardEngagement `json:"startedWithoutConsultants"`
	StartedWithoutConsultantsTotal int                   `json:"startedWithoutConsultantsTotal"`
	ByStatus                       []DashboardCount      `json:"byStatus"`
	ByMonth                        []DashboardCount      `json:"byMonth"`
}

// DashboardCampaignCreator is the campaign coverage of one creator.
type DashboardCampaignCreator struct {
	Email        string `json:"email"`
	Total        int    `json:"total"`
	WithCampaign int    `json:"withCampaign"`
}

// DashboardEvents is GET /dashboard/events.
type DashboardEvents struct {
	ByMonth               []DashboardCount           `json:"byMonth"`
	ByRegion              []DashboardCount           `json:"byRegion"`
	FundingByBusinessUnit []DashboardCount           `json:"fundingByBusinessUnit"`
	CampaignAvailable     bool                       `json:"campaignAvailable"`
	CampaignCreators      []DashboardCampaignCreator `json:"campaignCreators"`
}

// DashboardMyVisaTrip is an own upcoming trip with the visa picked on it.
type DashboardMyVisaTrip struct {
	JobNumber string  `json:"jobNumber"`
	Country   *string `json:"country"`
	City      *string `json:"city"`
	StartDate string  `json:"startDate"`
	EndDate   string  `json:"endDate"`
	VisaID    *int    `json:"visaId"`
	VisaType  *string `json:"visaType"`
	VisaEnd   *string `json:"visaEnd"`
}

// DashboardOwnVisa is one of the person's Active visas.
type DashboardOwnVisa struct {
	Country   string `json:"country"`
	VisaType  string `json:"visaType"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// DashboardMyVisas is GET /dashboard/my-visas.
type DashboardMyVisas struct {
	Trips []DashboardMyVisaTrip `json:"trips"`
	Visas []DashboardOwnVisa    `json:"visas"`
}

func dashboardAddStrings(q url.Values, key string, vals []string) {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			q.Add(key, v)
		}
	}
}

// DashboardCostLines calls GET /dashboard/cost-lines.
func (c *HTTPClient) DashboardCostLines(ctx context.Context, f DashboardCostLinesFilter) (*DashboardCostLines, error) {
	q := url.Values{"from": {f.From}, "to": {f.To}}
	dashboardAddStrings(q, "source", f.Sources)
	dashboardAddStrings(q, "jobNumber", f.JobNumbers)
	dashboardAddStrings(q, "employee", f.Employees)
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	var out DashboardCostLines
	if err := c.rest.Get(ctx, withQuery("/dashboard/cost-lines", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func dashboardTodayQuery(today string) url.Values {
	q := url.Values{}
	if today != "" {
		q.Set("today", today)
	}
	return q
}

// DashboardTravelTeam calls GET /dashboard/travel-team.
func (c *HTTPClient) DashboardTravelTeam(ctx context.Context, today string) (*DashboardTravelTeam, error) {
	var out DashboardTravelTeam
	if err := c.rest.Get(ctx, withQuery("/dashboard/travel-team", dashboardTodayQuery(today)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardManager calls GET /dashboard/manager.
func (c *HTTPClient) DashboardManager(ctx context.Context, today string, engagementTypes []int) (*DashboardManager, error) {
	q := dashboardTodayQuery(today)
	dashboardAddInts(q, "engagementType", engagementTypes)
	var out DashboardManager
	if err := c.rest.Get(ctx, withQuery("/dashboard/manager", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardAccountManager calls GET /dashboard/account-manager.
func (c *HTTPClient) DashboardAccountManager(ctx context.Context, email, today string) (*DashboardAccountManager, error) {
	q := dashboardTodayQuery(today)
	q.Set("email", email)
	var out DashboardAccountManager
	if err := c.rest.Get(ctx, withQuery("/dashboard/account-manager", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardEvents calls GET /dashboard/events.
func (c *HTTPClient) DashboardEvents(ctx context.Context, today string) (*DashboardEvents, error) {
	var out DashboardEvents
	if err := c.rest.Get(ctx, withQuery("/dashboard/events", dashboardTodayQuery(today)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardMyVisas calls GET /dashboard/my-visas.
func (c *HTTPClient) DashboardMyVisas(ctx context.Context, email, today string) (*DashboardMyVisas, error) {
	q := dashboardTodayQuery(today)
	q.Set("email", email)
	var out DashboardMyVisas
	if err := c.rest.Get(ctx, withQuery("/dashboard/my-visas", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
