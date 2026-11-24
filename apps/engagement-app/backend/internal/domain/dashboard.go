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

// Dashboard (GET /dashboard), Allocations Today (GET
// /dashboard/allocations-today) and the Me overview card (GET /me/overview).
// A section the caller may not see is absent (nil, omitted from the JSON).

// DashboardListLink names the list a count card opens and the filter that
// reproduces its count: Path is a backend API path, Query its parameters.
type DashboardListLink struct {
	// Path is the One WSO2 route of the list.
	Path string `json:"path"`
}

// DashboardCountCard is a Waiting on You card (openapi CountCard).
type DashboardCountCard struct {
	Key         string            `json:"key"`
	Label       string            `json:"label"`
	Count       int               `json:"count"`
	Highlighted bool              `json:"highlighted"`
	List        DashboardListLink `json:"list"`
}

// DashboardBucket is a chart bar (openapi Bucket). ID and Color are set where
// the legacy screens colour by them (engagement type id, allocation type
// colour).
type DashboardBucket struct {
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
	Percent float64 `json:"percent"`
	ID      *int    `json:"id,omitempty"`
	Color   *string `json:"color,omitempty"`
}

// DashboardSeriesPoint is one month of a chart (openapi SeriesPoint).
type DashboardSeriesPoint struct {
	Month  string             `json:"month"`
	Values map[string]float64 `json:"values"`
}

// DashboardAllocationSummary is one of the caller's allocations (openapi
// AllocationSummary).
type DashboardAllocationSummary struct {
	ID              string  `json:"id"`
	EngagementID    string  `json:"engagementId"`
	EngagementTitle string  `json:"engagementTitle"`
	StartDate       string  `json:"startDate"`
	EndDate         string  `json:"endDate"`
	ClearanceStatus *string `json:"clearanceStatus"`
	Current         bool    `json:"current"`
	// CanOpen: the person may open the engagement (the same rule as My
	// Allocations' engagement link).
	CanOpen bool `json:"canOpen"`
}

// DashboardTeamAllocation is today's team allocation (openapi TimelineItem,
// kind default).
type DashboardTeamAllocation struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	EngagementID    *string `json:"engagementId"`
	Label           string  `json:"label"`
	AllocationType  string  `json:"allocationType"`
	EngagementCode  string  `json:"engagementCode"`
	Color           *string `json:"color"`
	StartDate       string  `json:"startDate"`
	EndDate         string  `json:"endDate"`
	ClearanceStatus string  `json:"clearanceStatus"`
	Comment         *string `json:"comment"`
}

// DashboardTrip is a travel request row (a subset of openapi
// TravelRequestSummary).
type DashboardTrip struct {
	JobNumber string  `json:"jobNumber"`
	Status    *string `json:"status"`
	Traveller Person  `json:"traveller"`
	City      *string `json:"city"`
	Country   *string `json:"country"`
	StartDate string  `json:"startDate"`
	EndDate   string  `json:"endDate"`
	Agency    *string `json:"agency"`
}

// DashboardAgentTrip is a travel request row for a travel agent: no agency.
type DashboardAgentTrip struct {
	JobNumber string  `json:"jobNumber"`
	Status    *string `json:"status"`
	Traveller Person  `json:"traveller"`
	City      *string `json:"city"`
	Country   *string `json:"country"`
	StartDate string  `json:"startDate"`
	EndDate   string  `json:"endDate"`
}

// DashboardMyWork is the caller's own work.
type DashboardMyWork struct {
	CurrentAllocation   *DashboardAllocationSummary `json:"currentAllocation"`
	TodayTeamAllocation *DashboardTeamAllocation    `json:"todayTeamAllocation"`
	NextAllocation      *DashboardAllocationSummary `json:"nextAllocation"`
	NextTrips           []DashboardTrip             `json:"nextTrips"`
	UpcomingTripCount   int                         `json:"upcomingTripCount"`
}

// DashboardTravel is the Travel section.
type DashboardTravel struct {
	InProgress         int                    `json:"inProgress"`
	StartingIn30Days   int                    `json:"startingIn30Days"`
	CompletedThisMonth int                    `json:"completedThisMonth"`
	Last12Months       int                    `json:"last12Months"`
	Upcoming           []DashboardTrip        `json:"upcoming"`
	UpcomingTotal      int                    `json:"upcomingTotal"`
	TopDestinations    []DashboardBucket      `json:"topDestinations"`
	ByMonth            []DashboardSeriesPoint `json:"byMonth"`
	ByAgency           []DashboardBucket      `json:"byAgency"`
}

// DashboardUnconverted is an amount left out of the USD totals because the
// record carries no USD amount.
type DashboardUnconverted struct {
	Source   string  `json:"source"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// DashboardTravelCost is the Travel Cost section, in USD.
type DashboardTravelCost struct {
	Currency           string                 `json:"currency"`
	RateRule           string                 `json:"rateRule"`
	From               string                 `json:"from"`
	To                 string                 `json:"to"`
	Total              float64                `json:"total"`
	TravelInvoices     float64                `json:"travelInvoices"`
	PerDiems           float64                `json:"perDiems"`
	CreditCardExpenses *float64               `json:"creditCardExpenses"`
	ExpenseClaims      *float64               `json:"expenseClaims"`
	LastMonth          float64                `json:"lastMonth"`
	CurrentMonth       float64                `json:"currentMonth"`
	ByMonth            []DashboardSeriesPoint `json:"byMonth"`
	ByType             []DashboardBucket      `json:"byType"`
	NotConverted       []DashboardUnconverted `json:"notConverted"`
}

// DashboardEngagement is an engagement starting soon (openapi
// EngagementSummary fields plus the consultant count).
type DashboardEngagement struct {
	ID                 string  `json:"id"`
	Kind               string  `json:"kind"`
	EngagementTypeID   int     `json:"engagementTypeId"`
	EngagementTypeName *string `json:"engagementTypeName"`
	Account            *string `json:"account"`
	Title              string  `json:"title"`
	AllocationType     *string `json:"allocationType"`
	EngagementCode     *string `json:"engagementCode"`
	City               *string `json:"city"`
	Country            *string `json:"country"`
	StartDate          string  `json:"startDate"`
	EndDate            string  `json:"endDate"`
	RequestStatus      *string `json:"requestStatus"`
	EngagementStatus   *string `json:"engagementStatus"`
	Consultants        int     `json:"consultants"`
}

// DashboardEngagements is the Engagements section.
type DashboardEngagements struct {
	StartingIn30Days      []DashboardEngagement `json:"startingIn30Days"`
	StartingIn30DaysTotal int                   `json:"startingIn30DaysTotal"`
}

// DashboardAvailableConsultants is Available Consultants.
type DashboardAvailableConsultants struct {
	From           string            `json:"from"`
	To             string            `json:"to"`
	Consultants    int               `json:"consultants"`
	Available      int               `json:"available"`
	Allocated      int               `json:"allocated"`
	ByBusinessUnit []DashboardBucket `json:"byBusinessUnit"`
}

// DashboardAllocationsOverTime is Allocations Over Time.
type DashboardAllocationsOverTime struct {
	ByMonth        []DashboardSeriesPoint `json:"byMonth"`
	BusiestMonth   *DashboardSeriesPoint  `json:"busiestMonth"`
	ByBusinessUnit []DashboardBucket      `json:"byBusinessUnit"`
}

// DashboardAgentPerDiem is a per diem on an open trip of the agency.
type DashboardAgentPerDiem struct {
	JobNumber string   `json:"jobNumber"`
	Traveller Person   `json:"traveller"`
	StartDate *string  `json:"startDate"`
	EndDate   *string  `json:"endDate"`
	Days      *int     `json:"days"`
	Currency  *string  `json:"currency"`
	Total     *float64 `json:"total"`
}

// DashboardAgent is the travel agent dashboard.
type DashboardAgent struct {
	Agency                  string                  `json:"agency"`
	InProgress              int                     `json:"inProgress"`
	StartingIn7Days         int                     `json:"startingIn7Days"`
	CompletedWithoutInvoice int                     `json:"completedWithoutInvoice"`
	InvoicesRejected        int                     `json:"invoicesRejected"`
	Upcoming                []DashboardAgentTrip    `json:"upcoming"`
	UpcomingTotal           int                     `json:"upcomingTotal"`
	InvoicesByStatus        []DashboardBucket       `json:"invoicesByStatus"`
	TripsByMonth            []DashboardSeriesPoint  `json:"tripsByMonth"`
	PerDiemsOnOpenTrips     []DashboardAgentPerDiem `json:"perDiemsOnOpenTrips"`
}

// Dashboard is GET /dashboard. Absent sections are not shown.
type Dashboard struct {
	// FailedSections are the sections that could not be loaded (show an error there).
	FailedSections       []string                       `json:"failedSections"`
	MyWork               *DashboardMyWork               `json:"myWork,omitempty"`
	WaitingOnYou         []DashboardCountCard           `json:"waitingOnYou,omitempty"`
	TravelCost           *DashboardTravelCost           `json:"travelCost,omitempty"`
	Travel               *DashboardTravel               `json:"travel,omitempty"`
	Engagements          *DashboardEngagements          `json:"engagements,omitempty"`
	AvailableConsultants *DashboardAvailableConsultants `json:"availableConsultants,omitempty"`
	AllocationsOverTime  *DashboardAllocationsOverTime  `json:"allocationsOverTime,omitempty"`
	Agent                *DashboardAgent                `json:"agent,omitempty"`
}

// DashboardAllocationsToday is GET /dashboard/allocations-today.
type DashboardAllocationsToday struct {
	Date                    string            `json:"date"`
	BusinessUnitID          *int              `json:"businessUnitId"`
	Allocations             int               `json:"allocations"`
	Consultants             int               `json:"consultants"`
	Paid                    int               `json:"paid"`
	NonPaid                 int               `json:"nonPaid"`
	ByEngagementType        []DashboardBucket `json:"byEngagementType"`
	PaidByAllocationType    []DashboardBucket `json:"paidByAllocationType"`
	NonPaidByAllocationType []DashboardBucket `json:"nonPaidByAllocationType"`
}

// MeOverview is GET /me/overview (openapi MyOverview).
type MeOverview struct {
	UpcomingAllocations []DashboardAllocationSummary `json:"upcomingAllocations"`
	UpcomingTrips       []DashboardTrip              `json:"upcomingTrips"`
}
