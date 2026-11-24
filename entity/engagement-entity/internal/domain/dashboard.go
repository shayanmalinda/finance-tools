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

// Dashboard figures. Every figure is an aggregate (COUNT / SUM ... GROUP BY)
// over one schema; the Engagement App Backend decides which of them the
// caller may see and shapes them for the screen.

// ── Allocations Today (the legacy Allocations dashboard) ──────────────────────

// DashboardAllocationsTodayFilter selects the legacy dashboard variant. An
// empty Date runs the "today" queries (curdate()); a date runs the "past
// date" queries. BusinessUnitID 0 means every business unit.
type DashboardAllocationsTodayFilter struct {
	Date           string
	BusinessUnitID int
}

// DashboardEngagementTypeCount is a row of getAllocationsWithEngagementTypeQuery.
type DashboardEngagementTypeCount struct {
	EngagementTypeID int    `json:"engagementTypeId"`
	EngagementType   string `json:"engagementType"`
	Count            int    `json:"count"`
}

// DashboardAllocationTypeCount is a row of the paid / non-paid by allocation
// type queries.
type DashboardAllocationTypeCount struct {
	AllocationType string  `json:"allocationType"`
	Color          *string `json:"color"`
	Count          int     `json:"count"`
}

// DashboardAllocationsToday is the legacy dashboard's three result sets plus
// the distinct consultants under the same predicates.
type DashboardAllocationsToday struct {
	ByEngagementType        []DashboardEngagementTypeCount `json:"byEngagementType"`
	PaidByAllocationType    []DashboardAllocationTypeCount `json:"paidByAllocationType"`
	NonPaidByAllocationType []DashboardAllocationTypeCount `json:"nonPaidByAllocationType"`
	Consultants             int                            `json:"consultants"`
}

// ── My Work / Me overview ───────────────────────────────────────────────────

// DashboardMyWorkFilter names the person and the day.
type DashboardMyWorkFilter struct {
	Email string
	Today string
	// Trips adds the person's upcoming travel requests (travelapp).
	Trips bool
	// TripLimit caps the trips returned (default 3).
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

// DashboardTeamAllocation is a default (team) allocation.
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

// DashboardTrip is a travel request row for the dashboard lists.
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

// DashboardMyWork is the person's own allocations and trips.
type DashboardMyWork struct {
	// Allocations are the active engagement allocations ending today or
	// later, by start date (at most 20).
	Allocations []DashboardOwnAllocation `json:"allocations"`
	// TeamAllocation is today's team allocation other than Engagement
	// Overhead, if any.
	TeamAllocation *DashboardTeamAllocation `json:"teamAllocation"`
	Trips          []DashboardTrip          `json:"trips"`
	TripCount      int                      `json:"tripCount"`
}

// ── Waiting on You ──────────────────────────────────────────────────────────

// DashboardQueuesFilter selects which queues to count.
type DashboardQueuesFilter struct {
	// PendingApprovalTypes are the engagement types (1, 5) whose Pending
	// Approval engagements are counted.
	PendingApprovalTypes []int
	// ClearanceTypes are the engagement types whose Active allocations at
	// Ready for Clearance / Clearance In Progress are counted.
	ClearanceTypes []int
	// TravelRequests counts travel requests with status New.
	TravelRequests bool
	// Invoices counts the invoice queues; Payments the per diem payment ones.
	Invoices bool
	Payments bool
}

// DashboardQueues are the counts. Maps are keyed by engagement type id or
// clearance status; absent keys mean zero.
type DashboardQueues struct {
	PendingApproval map[int]int    `json:"pendingApproval"`
	Clearance       map[string]int `json:"clearance"`
	NewTravel       int            `json:"newTravelRequests"`
	// Invoices by queue: Pending, Accepted by travel, Accepted by finance not
	// failed (to push) and failed pushes.
	InvoicesPending          int `json:"invoicesPending"`
	InvoicesAcceptedByTravel int `json:"invoicesAcceptedByTravel"`
	InvoicesToPush           int `json:"invoicesToPush"`
	InvoicesFailed           int `json:"invoicesFailed"`
	PaymentsToPush           int `json:"paymentsToPush"`
	PaymentsFailed           int `json:"paymentsFailed"`
}

// ── Travel and Travel Cost ──────────────────────────────────────────────────

// DashboardTravelFilter selects the travel parts to compute.
type DashboardTravelFilter struct {
	Today string
	// Trips computes the Travel section.
	Trips bool
	// CostInvoices, CostPayments and CostCards add the Travel Cost lines from
	// invoices, per diem payments and card spend (expense_app).
	CostInvoices bool
	CostPayments bool
	CostCards    bool
	// CostClaims adds the expense claims tagged with a travel (expense_app).
	CostClaims bool
}

// DashboardCount is a label with a count.
type DashboardCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// DashboardTravelTrips is the Travel section.
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
	DashboardCostClaim   = "claim"
)

// DashboardCostLine is a sum of original amounts by source, month (YYYY-MM),
// cost type (invoices only) and original currency (upper-cased, blank when
// the record has none). Card groups also carry CCAmount, the sum of the
// amount the card was billed, and CCCurrency, "USD" when that billing
// currency is USD (blank otherwise); the BFF uses it instead of converting.
type DashboardCostLine struct {
	Source     string  `json:"source"`
	Month      string  `json:"month"`
	CostType   string  `json:"costType"`
	Currency   string  `json:"currency"`
	Amount     float64 `json:"amount"`
	CCAmount   float64 `json:"ccAmount,omitempty"`
	CCCurrency string  `json:"ccCurrency,omitempty"`
}

// DashboardTravel is the Travel and Travel Cost figures.
type DashboardTravel struct {
	// From and To (exclusive) bound the 12-month window, first days of months.
	From  string                `json:"from"`
	To    string                `json:"to"`
	Trips *DashboardTravelTrips `json:"trips,omitempty"`
	Cost  []DashboardCostLine   `json:"cost"`
}

// ── Engagements, Available Consultants, Allocations Over Time ──────────────

// DashboardEngagementsFilter selects the allocation_db parts.
type DashboardEngagementsFilter struct {
	Today string
	// EngagementTypes are the types whose engagements starting in the next
	// 30 days are listed; empty skips the list.
	EngagementTypes []int
	// Bench adds Available Consultants; Trend adds Allocations Over Time.
	Bench bool
	Trend bool
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

// DashboardBenchRow is the consultant pool and the allocated part of it in a
// business unit.
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

// DashboardEngagements is the allocation_db part of the dashboard.
type DashboardEngagements struct {
	StartingSoon      []DashboardEngagement `json:"startingSoon"`
	StartingSoonTotal int                   `json:"startingSoonTotal"`
	Bench             []DashboardBenchRow   `json:"bench,omitempty"`
	Trend             *DashboardTrend       `json:"trend,omitempty"`
}

// ── Travel agent dashboard ──────────────────────────────────────────────────

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

// DashboardAgency is the travel agent dashboard: only the agency's own trips
// (travel.travel_agent_id) and the invoices its people added.
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
