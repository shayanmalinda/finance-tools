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

package domain

// ── Cost lines (GET /dashboard/cost-lines) ──────────────────────────────────

// DashboardCostLinesMax is the most lines read per source; the queries ask
// for one more to detect truncation.
const DashboardCostLinesMax = 20000

// Paid values of a cost line.
const (
	DashboardPaid    = "paid"
	DashboardNonPaid = "nonPaid"
	DashboardUnknown = "unknown"
)

// DashboardCostLinesFilter selects cost lines. From and To are inclusive days;
// both may be empty only when JobNumbers is set, which reads every date.
type DashboardCostLinesFilter struct {
	From, To   string
	Sources    []string // invoice, perDiem, card, claim; empty means all four
	JobNumbers []string
	Employees  []string
	Limit      int // per source; 0 means DashboardCostLinesMax
}

// DashboardCostEntry is one cost line in its original amount and currency
// (no conversion), with the trip and engagement facts joined in Go.
// CCAmount and CCCurrency are the card's billed amount ("USD" when billed in
// USD) and are set on card lines only.
type DashboardCostEntry struct {
	ID               string  `json:"id"`
	Source           string  `json:"source"`
	Reference        *string `json:"reference"`
	JobNumber        *string `json:"jobNumber"`
	TravellerEmail   *string `json:"travellerEmail"`
	Date             string  `json:"date"`
	OriginalAmount   float64 `json:"originalAmount"`
	OriginalCurrency string  `json:"originalCurrency"`
	CCAmount         float64 `json:"ccAmount,omitempty"`
	CCCurrency       string  `json:"ccCurrency,omitempty"`
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

// DashboardCostEntries is the answer of GET /dashboard/cost-lines. Truncated
// names the sources that hit the cap; Unavailable the ones that could not be
// read (card and claim when expense_app is down).
type DashboardCostEntries struct {
	Items       []DashboardCostEntry `json:"items"`
	Truncated   []string             `json:"truncated"`
	Unavailable []string             `json:"unavailable"`
}

// ── Travel Team ─────────────────────────────────────────────────────────────

// DashboardTravelTeam is the Travel Team section's figures.
type DashboardTravelTeam struct {
	AwayTravellers     int              `json:"awayTravellers"`
	AwayCountries      int              `json:"awayCountries"`
	EndedNotCompleted  int              `json:"endedNotCompleted"`
	POCount            int              `json:"poCount"`
	POOldestDays       int              `json:"poOldestDays"`
	POByAge            []int            `json:"poByAge"` // under 7, 7 to 30, 31 to 90, over 90 days
	FrequentTravellers []DashboardCount `json:"frequentTravellers"`
	AgentWorkload      []DashboardCount `json:"agentWorkload"`
}

// ── Engagement Manager ──────────────────────────────────────────────────────

// DashboardManagerFilter selects the Engagement Manager figures. Types are
// the engagement types the caller allocates.
type DashboardManagerFilter struct {
	Today string
	Types []int
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

// DashboardManager is the Engagement Manager section's figures.
type DashboardManager struct {
	WithoutConsultants      []DashboardEngagement         `json:"withoutConsultants"`
	WithoutConsultantsTotal int                           `json:"withoutConsultantsTotal"`
	StartingIn14Days        []DashboardStartingAllocation `json:"startingIn14Days"`
	StartingIn14DaysTotal   int                           `json:"startingIn14DaysTotal"`
	MostAllocated           []DashboardCount              `json:"mostAllocated"` // label = consultant email, count = days
	TopBy                   string                        `json:"topBy"`         // customer or country
	Top                     []DashboardCount              `json:"top"`           // count = days
}

// ── Account Manager ─────────────────────────────────────────────────────────

// DashboardAccountManager is the Account Manager section's figures, over the
// engagements the person created.
type DashboardAccountManager struct {
	StartedWithoutConsultants      []DashboardEngagement `json:"startedWithoutConsultants"`
	StartedWithoutConsultantsTotal int                   `json:"startedWithoutConsultantsTotal"`
	ByStatus                       []DashboardCount      `json:"byStatus"`
	ByMonth                        []DashboardCount      `json:"byMonth"` // label = YYYY-MM
}

// ── Marketing Events ────────────────────────────────────────────────────────

// DashboardCampaignCreator is the campaign coverage of one creator.
type DashboardCampaignCreator struct {
	Email        string `json:"email"`
	Total        int    `json:"total"`
	WithCampaign int    `json:"withCampaign"`
}

// DashboardEvents is the Marketing Events section's figures.
type DashboardEvents struct {
	ByMonth               []DashboardCount `json:"byMonth"`
	ByRegion              []DashboardCount `json:"byRegion"`
	FundingByBusinessUnit []DashboardCount `json:"fundingByBusinessUnit"`
	// CampaignAvailable is false when engagement.campaign_id does not exist.
	CampaignAvailable bool                       `json:"campaignAvailable"`
	CampaignCreators  []DashboardCampaignCreator `json:"campaignCreators"`
}

// ── My visas ────────────────────────────────────────────────────────────────

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

// DashboardMyVisas is the answer of GET /dashboard/my-visas.
type DashboardMyVisas struct {
	Trips []DashboardMyVisaTrip `json:"trips"`
	Visas []DashboardOwnVisa    `json:"visas"`
}
