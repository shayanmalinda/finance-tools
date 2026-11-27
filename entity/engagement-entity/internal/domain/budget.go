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

// Travel budgets (travelapp). See Resources/travelapp_table_update_v1.2.0.sql.

// Budget line statuses.
const (
	BudgetLineActive  = "Active"
	BudgetLineRetired = "Retired"
)

// Allocation type rule kinds.
const (
	BudgetRuleLine       = "LINE"
	BudgetRuleRegion     = "REGION"
	BudgetRuleTeam       = "TEAM"
	BudgetRuleEngagement = "ENGAGEMENT"
	BudgetRuleNone       = "NONE"
)

// Suggestion sources and who chose a trip's line.
const (
	BudgetSourceEngagement     = "ENGAGEMENT"
	BudgetSourceAllocationType = "ALLOCATION_TYPE"
	BudgetSourceRegion         = "REGION"
	BudgetSourceTeam           = "TEAM"
	BudgetSourceNone           = "NONE"
	BudgetChosenUser           = "USER"
	BudgetChosenSystem         = "SYSTEM"
)

// BudgetNeedsLineMax caps the page size of GET /budgets/needs-line.
const BudgetNeedsLineMax = 200

// BudgetYear is a budget year (GET /budgets/years).
type BudgetYear struct {
	Year      int    `json:"year"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	IsActive  bool   `json:"isActive"`
}

// BudgetYearsResponse is GET /budgets/years.
type BudgetYearsResponse struct {
	Years []BudgetYear `json:"years"`
}

// BudgetLine is one budget line of a year.
type BudgetLine struct {
	ID           int     `json:"id"`
	Year         int     `json:"year"`
	Code         string  `json:"code"`
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	Status       string  `json:"status"`
	SortOrder    int     `json:"sortOrder"`
	UpdatedBy    *string `json:"updatedBy"`
	UpdatedAt    *string `json:"updatedAt"`
}

// BudgetLinesResponse is GET /budgets/lines.
type BudgetLinesResponse struct {
	Lines []BudgetLine `json:"lines"`
}

// BudgetLineCreateRequest is POST /budgets/lines.
type BudgetLineCreateRequest struct {
	Year         int     `json:"year"`
	Code         string  `json:"code"`
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	SortOrder    int     `json:"sortOrder"`
	CreatedBy    string  `json:"createdBy"`
}

// BudgetLineUpdateRequest is PUT /budgets/lines/{id}. Everything is sent;
// code and year never change.
type BudgetLineUpdateRequest struct {
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	SortOrder    int     `json:"sortOrder"`
	Status       string  `json:"status"`
	UpdatedBy    string  `json:"updatedBy"`
}

// BudgetAllocationTypeRule is the rule of one allocation type.
type BudgetAllocationTypeRule struct {
	AllocationTypeID int     `json:"allocationTypeId"`
	RuleKind         string  `json:"ruleKind"`
	LineCode         *string `json:"lineCode"`
}

// BudgetAllocationTypeRulesResponse is GET /budgets/allocation-type-rules.
type BudgetAllocationTypeRulesResponse struct {
	Rules []BudgetAllocationTypeRule `json:"rules"`
}

// BudgetAllocationTypeRuleRequest is PUT /budgets/allocation-type-rules/{allocationTypeId}.
type BudgetAllocationTypeRuleRequest struct {
	RuleKind  string  `json:"ruleKind"`
	LineCode  *string `json:"lineCode"`
	UpdatedBy string  `json:"updatedBy"`
}

// BudgetRegionRule maps a funding region (and sub region) to a Sales line.
type BudgetRegionRule struct {
	Region    string  `json:"region"`
	SubRegion *string `json:"subRegion"`
	LineCode  *string `json:"lineCode"`
}

// BudgetRegionRulesResponse is GET /budgets/region-rules.
type BudgetRegionRulesResponse struct {
	Rules []BudgetRegionRule `json:"rules"`
}

// BudgetTeamRule maps a traveller's HR team to a line. A nil column matches any.
type BudgetTeamRule struct {
	ID           int     `json:"id"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	Team         *string `json:"team"`
	LineCode     string  `json:"lineCode"`
}

// BudgetTeamRulesResponse is GET /budgets/team-rules.
type BudgetTeamRulesResponse struct {
	Rules []BudgetTeamRule `json:"rules"`
}

// BudgetEngagementLine is the line set on an engagement (nil when unset).
type BudgetEngagementLine struct {
	EngagementID string  `json:"engagementId"`
	LineCode     *string `json:"lineCode"`
}

// BudgetEngagementLineRequest is PUT /budgets/engagements/{engagementId}/line.
type BudgetEngagementLineRequest struct {
	LineCode  *string `json:"lineCode"`
	UpdatedBy string  `json:"updatedBy"`
}

// BudgetTrip is a trip with its budget line (TripBudget).
type BudgetTrip struct {
	JobNumber        string  `json:"jobNumber"`
	TravellerEmail   *string `json:"travellerEmail"`
	Country          *string `json:"country"`
	StartDate        string  `json:"startDate"`
	EndDate          string  `json:"endDate"`
	Status           *string `json:"status"`
	EngagementID     *string `json:"engagementId"`
	EngagementType   *string `json:"engagementType"`
	CustomerName     *string `json:"customerName"`
	BudgetYear       *int    `json:"budgetYear"`
	BudgetLineID     *int    `json:"budgetLineId"`
	SuggestedLineID  *int    `json:"suggestedLineId"`
	SuggestionSource *string `json:"suggestionSource"`
	ChosenBy         *string `json:"chosenBy"`
	// RateDate is the day the trip row was created, or its start date when
	// it has no budget row.
	RateDate string `json:"rateDate"`
	HasRow   bool   `json:"hasRow"`
}

// BudgetTripRequest is PUT /budgets/trips/{jobNumber}.
type BudgetTripRequest struct {
	BudgetLineID *int   `json:"budgetLineId"`
	UpdatedBy    string `json:"updatedBy"`
}

// BudgetNeedsLineResponse is GET /budgets/needs-line.
type BudgetNeedsLineResponse struct {
	Items []BudgetTrip `json:"items"`
	Total int          `json:"total"`
}

// BudgetLedgerFilter selects the ledger: trips whose start date is in From..To.
type BudgetLedgerFilter struct {
	From, To   string
	Unassigned bool
}

// BudgetLedgerTrip is a trip of the ledger range.
type BudgetLedgerTrip struct {
	JobNumber    string  `json:"jobNumber"`
	BudgetLineID *int    `json:"budgetLineId"`
	Status       *string `json:"status"`
	Country      *string `json:"country"`
	StartDate    string  `json:"startDate"`
	EndDate      string  `json:"endDate"`
	RateDate     string  `json:"rateDate"`
}

// BudgetLedgerCost is the sum of one trip's cost in one source and currency.
type BudgetLedgerCost struct {
	JobNumber string  `json:"jobNumber"`
	Source    string  `json:"source"`
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
}

// BudgetLedgerUnassigned is travel spend of one month with no job number.
type BudgetLedgerUnassigned struct {
	Source   string  `json:"source"`
	Month    string  `json:"month"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// BudgetLedger is GET /budgets/ledger. Unassigned is omitted unless asked for.
type BudgetLedger struct {
	From             string                    `json:"from"`
	To               string                    `json:"to"`
	ExpenseAvailable bool                      `json:"expenseAvailable"`
	Truncated        bool                      `json:"truncated"`
	Trips            []BudgetLedgerTrip        `json:"trips"`
	Costs            []BudgetLedgerCost        `json:"costs"`
	Unassigned       *[]BudgetLedgerUnassigned `json:"unassigned,omitempty"`
}

// AllocationTravelBudget is the optional travelBudget of an allocation
// request: the line the new trip is born with.
type AllocationTravelBudget struct {
	BudgetLineID     *int    `json:"budgetLineId"`
	SuggestedLineID  *int    `json:"suggestedLineId"`
	SuggestionSource *string `json:"suggestionSource"`
	ChosenBy         *string `json:"chosenBy"`
}
