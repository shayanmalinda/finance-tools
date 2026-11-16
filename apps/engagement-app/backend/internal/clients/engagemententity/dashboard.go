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
)

// DashboardAPI is what the dashboard screen area needs from the Engagement Entity.
// The area owns this file: its methods, request and response types, and the
// HTTPClient implementations live here.
//
// Every call returns aggregates; the backend asks only for what the caller
// may see.
type DashboardAPI interface {
	// DashboardAllocationsToday runs the legacy Allocations dashboard queries.
	// date "" is today (the legacy curdate() variant); businessUnitID 0 is all.
	DashboardAllocationsToday(ctx context.Context, date string, businessUnitID int) (*DashboardAllocationsToday, error)
	// DashboardMyWork returns the person's own allocations and (with trips)
	// upcoming trips.
	DashboardMyWork(ctx context.Context, f DashboardMyWorkFilter) (*DashboardMyWork, error)
	// DashboardQueues counts the Waiting on You queues that f asks for.
	DashboardQueues(ctx context.Context, f DashboardQueuesFilter) (*DashboardQueues, error)
	// DashboardTravel returns the Travel and Travel Cost figures f asks for.
	DashboardTravel(ctx context.Context, f DashboardTravelFilter) (*DashboardTravel, error)
	// DashboardEngagements returns engagements starting soon, Available
	// Consultants and Allocations Over Time, as f asks.
	DashboardEngagements(ctx context.Context, f DashboardEngagementsFilter) (*DashboardEngagements, error)
	// DashboardAgency returns the travel agent dashboard of one agency.
	DashboardAgency(ctx context.Context, agencyID int, today string) (*DashboardAgency, error)
}

// DashboardEngagementTypeCount is a legacy "allocations by engagement type" row.
type DashboardEngagementTypeCount struct {
	EngagementTypeID int    `json:"engagementTypeId"`
	EngagementType   string `json:"engagementType"`
	Count            int    `json:"count"`
}

// DashboardAllocationTypeCount is a legacy "allocations by allocation type" row.
type DashboardAllocationTypeCount struct {
	AllocationType string  `json:"allocationType"`
	Color          *string `json:"color"`
	Count          int     `json:"count"`
}

// DashboardAllocationsToday is GET /dashboard/allocations-today.
type DashboardAllocationsToday struct {
	ByEngagementType        []DashboardEngagementTypeCount `json:"byEngagementType"`
	PaidByAllocationType    []DashboardAllocationTypeCount `json:"paidByAllocationType"`
	NonPaidByAllocationType []DashboardAllocationTypeCount `json:"nonPaidByAllocationType"`
	Consultants             int                            `json:"consultants"`
}

// DashboardMyWorkFilter is the query of GET /dashboard/my-work.
type DashboardMyWorkFilter struct {
	Email     string
	Today     string
	Trips     bool
	TripLimit int
}

// DashboardOwnAllocation is one of the person's active engagement allocations.
type DashboardOwnAllocation struct {
	ID               string  `json:"id"`
	EngagementID     string  `json:"engagementId"`
	EngagementTypeID *int    `json:"engagementTypeId"`
	Account          *string `json:"account"`
	Title            *string `json:"title"`
	StartDate        string  `json:"startDate"`
	EndDate          string  `json:"endDate"`
	ClearanceStatus  *string `json:"clearanceStatus"`
}

// DashboardTeamAllocation is a team (default) allocation.
type DashboardTeamAllocation struct {
	ID              string  `json:"id"`
	AllocationType  *string `json:"allocationType"`
	Color           *string `json:"color"`
	EngagementCode  *string `json:"engagementCode"`
	StartDate       string  `json:"startDate"`
	EndDate         string  `json:"endDate"`
	ClearanceStatus *string `json:"clearanceStatus"`
	Comment         *string `json:"comment"`
}

// DashboardTrip is a travel request row.
type DashboardTrip struct {
	JobNumber      string  `json:"jobNumber"`
	Status         *string `json:"status"`
	TravellerEmail string  `json:"travellerEmail"`
	City           *string `json:"city"`
	Country        *string `json:"country"`
	StartDate      string  `json:"startDate"`
	EndDate        string  `json:"endDate"`
	Agency         *string `json:"agency"`
}

// DashboardMyWork is GET /dashboard/my-work.
type DashboardMyWork struct {
	Allocations    []DashboardOwnAllocation `json:"allocations"`
	TeamAllocation *DashboardTeamAllocation `json:"teamAllocation"`
	Trips          []DashboardTrip          `json:"trips"`
	TripCount      int                      `json:"tripCount"`
}

// DashboardQueuesFilter is the query of GET /dashboard/queues.
type DashboardQueuesFilter struct {
	PendingApprovalTypes []int
	ClearanceTypes       []int
	TravelRequests       bool
	Invoices             bool
	Payments             bool
}

// DashboardQueues is GET /dashboard/queues.
type DashboardQueues struct {
	PendingApproval          map[int]int    `json:"pendingApproval"`
	Clearance                map[string]int `json:"clearance"`
	NewTravelRequests        int            `json:"newTravelRequests"`
	InvoicesPending          int            `json:"invoicesPending"`
	InvoicesAcceptedByTravel int            `json:"invoicesAcceptedByTravel"`
	InvoicesToPush           int            `json:"invoicesToPush"`
	InvoicesFailed           int            `json:"invoicesFailed"`
	PaymentsToPush           int            `json:"paymentsToPush"`
	PaymentsFailed           int            `json:"paymentsFailed"`
}

// DashboardTravelFilter is the query of GET /dashboard/travel.
type DashboardTravelFilter struct {
	Today        string
	Trips        bool
	CostInvoices bool
	CostPayments bool
	CostCards    bool
}

// DashboardCount is a label with a count.
type DashboardCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// DashboardTravelTrips is the Travel section's figures.
type DashboardTravelTrips struct {
	InProgress         int              `json:"inProgress"`
	StartingIn30Days   int              `json:"startingIn30Days"`
	CompletedThisMonth int              `json:"completedThisMonth"`
	Last12Months       int              `json:"last12Months"`
	UpcomingTotal      int              `json:"upcomingTotal"`
	Upcoming           []DashboardTrip  `json:"upcoming"`
	ByMonth            []DashboardCount `json:"byMonth"`
	ByCountry          []DashboardCount `json:"byCountry"`
	ByAgency           []DashboardCount `json:"byAgency"`
}

// Travel cost line sources.
const (
	DashboardCostInvoice = "invoice"
	DashboardCostPerDiem = "perDiem"
	DashboardCostCard    = "card"
)

// DashboardCostLine is a sum by source, month, cost type and currency; the
// currency is USD when the record carried a USD amount.
type DashboardCostLine struct {
	Source   string  `json:"source"`
	Month    string  `json:"month"`
	CostType string  `json:"costType"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// DashboardTravel is GET /dashboard/travel.
type DashboardTravel struct {
	From  string                `json:"from"`
	To    string                `json:"to"`
	Trips *DashboardTravelTrips `json:"trips"`
	Cost  []DashboardCostLine   `json:"cost"`
}

// DashboardEngagementsFilter is the query of GET /dashboard/engagements.
type DashboardEngagementsFilter struct {
	Today           string
	EngagementTypes []int
	Bench           bool
	Trend           bool
}

// DashboardEngagement is an engagement starting soon.
type DashboardEngagement struct {
	ID                 string  `json:"id"`
	EngagementTypeID   *int    `json:"engagementTypeId"`
	EngagementTypeName *string `json:"engagementTypeName"`
	Account            *string `json:"account"`
	Title              *string `json:"title"`
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

// DashboardBenchRow is a business unit's consultant pool and the allocated part.
type DashboardBenchRow struct {
	BusinessUnit string `json:"businessUnit"`
	Consultants  int    `json:"consultants"`
	Allocated    int    `json:"allocated"`
}

// DashboardTrend is Allocations Over Time.
type DashboardTrend struct {
	ByMonth        []DashboardCount `json:"byMonth"`
	ByBusinessUnit []DashboardCount `json:"byBusinessUnit"`
}

// DashboardEngagements is GET /dashboard/engagements.
type DashboardEngagements struct {
	StartingSoon      []DashboardEngagement `json:"startingSoon"`
	StartingSoonTotal int                   `json:"startingSoonTotal"`
	Bench             []DashboardBenchRow   `json:"bench"`
	Trend             *DashboardTrend       `json:"trend"`
}

// DashboardAgencyPerDiem is a per diem row on an open trip of the agency.
type DashboardAgencyPerDiem struct {
	JobNumber      string   `json:"jobNumber"`
	TravellerEmail string   `json:"travellerEmail"`
	StartDate      *string  `json:"startDate"`
	EndDate        *string  `json:"endDate"`
	Days           *int     `json:"days"`
	Currency       *string  `json:"currency"`
	Total          *float64 `json:"total"`
}

// DashboardAgency is GET /dashboard/agency.
type DashboardAgency struct {
	InProgress              int                      `json:"inProgress"`
	StartingIn7Days         int                      `json:"startingIn7Days"`
	StartingIn30Days        int                      `json:"startingIn30Days"`
	CompletedWithoutInvoice int                      `json:"completedWithoutInvoice"`
	UpcomingTotal           int                      `json:"upcomingTotal"`
	Upcoming                []DashboardTrip          `json:"upcoming"`
	InvoicesByStatus        []DashboardCount         `json:"invoicesByStatus"`
	TripsByMonth            []DashboardCount         `json:"tripsByMonth"`
	PerDiemsOnOpenTrips     []DashboardAgencyPerDiem `json:"perDiemsOnOpenTrips"`
}

func dashboardSetBool(q url.Values, key string, v bool) {
	if v {
		q.Set(key, "true")
	}
}

func dashboardAddInts(q url.Values, key string, vals []int) {
	for _, v := range vals {
		q.Add(key, strconv.Itoa(v))
	}
}

// DashboardAllocationsToday calls GET /dashboard/allocations-today.
func (c *HTTPClient) DashboardAllocationsToday(ctx context.Context, date string, businessUnitID int) (*DashboardAllocationsToday, error) {
	q := url.Values{}
	if date != "" {
		q.Set("date", date)
	}
	if businessUnitID > 0 {
		q.Set("businessUnitId", strconv.Itoa(businessUnitID))
	}
	var out DashboardAllocationsToday
	if err := c.rest.Get(ctx, withQuery("/dashboard/allocations-today", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardMyWork calls GET /dashboard/my-work.
func (c *HTTPClient) DashboardMyWork(ctx context.Context, f DashboardMyWorkFilter) (*DashboardMyWork, error) {
	q := url.Values{"email": {f.Email}}
	if f.Today != "" {
		q.Set("today", f.Today)
	}
	dashboardSetBool(q, "trips", f.Trips)
	if f.TripLimit > 0 {
		q.Set("tripLimit", strconv.Itoa(f.TripLimit))
	}
	var out DashboardMyWork
	if err := c.rest.Get(ctx, withQuery("/dashboard/my-work", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardQueues calls GET /dashboard/queues.
func (c *HTTPClient) DashboardQueues(ctx context.Context, f DashboardQueuesFilter) (*DashboardQueues, error) {
	q := url.Values{}
	dashboardAddInts(q, "pendingApprovalType", f.PendingApprovalTypes)
	dashboardAddInts(q, "clearanceType", f.ClearanceTypes)
	dashboardSetBool(q, "travelRequests", f.TravelRequests)
	dashboardSetBool(q, "invoices", f.Invoices)
	dashboardSetBool(q, "payments", f.Payments)
	var out DashboardQueues
	if err := c.rest.Get(ctx, withQuery("/dashboard/queues", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardTravel calls GET /dashboard/travel.
func (c *HTTPClient) DashboardTravel(ctx context.Context, f DashboardTravelFilter) (*DashboardTravel, error) {
	q := url.Values{}
	if f.Today != "" {
		q.Set("today", f.Today)
	}
	dashboardSetBool(q, "trips", f.Trips)
	dashboardSetBool(q, "costInvoices", f.CostInvoices)
	dashboardSetBool(q, "costPayments", f.CostPayments)
	dashboardSetBool(q, "costCards", f.CostCards)
	var out DashboardTravel
	if err := c.rest.Get(ctx, withQuery("/dashboard/travel", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardEngagements calls GET /dashboard/engagements.
func (c *HTTPClient) DashboardEngagements(ctx context.Context, f DashboardEngagementsFilter) (*DashboardEngagements, error) {
	q := url.Values{}
	if f.Today != "" {
		q.Set("today", f.Today)
	}
	dashboardAddInts(q, "engagementType", f.EngagementTypes)
	dashboardSetBool(q, "bench", f.Bench)
	dashboardSetBool(q, "trend", f.Trend)
	var out DashboardEngagements
	if err := c.rest.Get(ctx, withQuery("/dashboard/engagements", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DashboardAgency calls GET /dashboard/agency.
func (c *HTTPClient) DashboardAgency(ctx context.Context, agencyID int, today string) (*DashboardAgency, error) {
	q := url.Values{"agencyId": {strconv.Itoa(agencyID)}}
	if today != "" {
		q.Set("today", today)
	}
	var out DashboardAgency
	if err := c.rest.Get(ctx, withQuery("/dashboard/agency", q), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
