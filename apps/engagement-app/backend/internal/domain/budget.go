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

import "encoding/json"

// Travel budget suggestion sources, stored with each trip.
const (
	BudgetSourceEngagement     = "ENGAGEMENT"
	BudgetSourceAllocationType = "ALLOCATION_TYPE"
	BudgetSourceRegion         = "REGION"
	BudgetSourceTeam           = "TEAM"
	BudgetSourceNone           = "NONE"
)

// Who chose the line of a trip.
const (
	BudgetChosenByUser   = "USER"
	BudgetChosenBySystem = "SYSTEM"
)

// Allocation type rule kinds.
const (
	BudgetRuleLine       = "LINE"
	BudgetRuleRegion     = "REGION"
	BudgetRuleTeam       = "TEAM"
	BudgetRuleEngagement = "ENGAGEMENT"
	BudgetRuleNone       = "NONE"
)

// Budget line statuses.
const (
	BudgetLineActive  = "Active"
	BudgetLineRetired = "Retired"
)

// BudgetLine is a budget line of a year. Name is "Sales: EU" (budgetLineName).
type BudgetLine struct {
	ID           int     `json:"id"`
	Year         int     `json:"year"`
	Code         string  `json:"code"`
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	Name         string  `json:"name"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	Status       string  `json:"status"`
	SortOrder    int     `json:"sortOrder"`
	UpdatedBy    *string `json:"updatedBy"`
	UpdatedAt    *string `json:"updatedAt"`
}

// BudgetLineInput is the body of POST and PUT /budgets/lines. Year and Code
// are only used on create (a line's code and year never change). Status is
// only used on update.
type BudgetLineInput struct {
	Year         int     `json:"year"`
	Code         string  `json:"code"`
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	SortOrder    int     `json:"sortOrder"`
	Status       string  `json:"status"`
}

// BudgetLines is the list of lines.
type BudgetLines struct {
	Lines []BudgetLine `json:"lines"`
}

// BudgetAllocationTypeRule is an allocation type with the rule that picks
// its line (kind NONE when it has no rule).
type BudgetAllocationTypeRule struct {
	AllocationTypeID   int     `json:"allocationTypeId"`
	AllocationTypeName string  `json:"allocationTypeName"`
	EngagementTypeID   int     `json:"engagementTypeId"`
	EngagementTypeName string  `json:"engagementTypeName"`
	RuleKind           string  `json:"ruleKind"`
	LineCode           *string `json:"lineCode"`
	LineName           *string `json:"lineName"`
}

// BudgetAllocationTypeRules is the list of rules.
type BudgetAllocationTypeRules struct {
	Rules []BudgetAllocationTypeRule `json:"rules"`
}

// BudgetRuleInput is the body of PUT /budgets/allocation-type-rules/{allocationTypeId}.
type BudgetRuleInput struct {
	RuleKind string  `json:"ruleKind"`
	LineCode *string `json:"lineCode"`
}

// BudgetLineRef names a line.
type BudgetLineRef struct {
	LineID int    `json:"lineId"`
	Name   string `json:"name"`
}

// BudgetSuggested is the line suggested for a trip.
type BudgetSuggested struct {
	LineID int    `json:"lineId"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// BudgetFigures are the money figures of one line. Left may be negative.
type BudgetFigures struct {
	LineID       int     `json:"lineId"`
	Name         string  `json:"name"`
	HolderName   *string `json:"holderName"`
	BudgetUSD    float64 `json:"budgetUsd"`
	SpentUSD     float64 `json:"spentUsd"`
	CommittedUSD float64 `json:"committedUsd"`
	LeftUSD      float64 `json:"leftUsd"`
	UsedPercent  float64 `json:"usedPercent"`
}

// BudgetAlternative is a line the allocator may pick instead.
type BudgetAlternative struct {
	LineID  int     `json:"lineId"`
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	LeftUSD float64 `json:"leftUsd"`
}

// BudgetTypicalCost is the typical cost of a trip. Basis is country, global or
// none (then USD is null).
type BudgetTypicalCost struct {
	USD        *float64 `json:"usd"`
	Basis      string   `json:"basis"`
	SampleSize int      `json:"sampleSize"`
}

// BudgetPreview is GET /budgets/preview. When InScope is false only inScope
// is written.
type BudgetPreview struct {
	InScope      bool                `json:"inScope"`
	Year         int                 `json:"year"`
	Country      string              `json:"country"`
	Suggested    *BudgetSuggested    `json:"suggested"`
	Source       string              `json:"source"`
	Line         *BudgetFigures      `json:"line"`
	Alternatives []BudgetAlternative `json:"alternatives"`
	TypicalCost  BudgetTypicalCost   `json:"typicalCost"`
	Warning      bool                `json:"warning"`
	RatesPending bool                `json:"ratesPending"`
}

// MarshalJSON writes {"inScope":false} alone for a trip outside every budget year.
func (p BudgetPreview) MarshalJSON() ([]byte, error) {
	if !p.InScope {
		return []byte(`{"inScope":false}`), nil
	}
	type plain BudgetPreview
	return json.Marshal(plain(p))
}

// BudgetSummaryLine is a line row of the summary.
type BudgetSummaryLine struct {
	LineID               int     `json:"lineId"`
	Code                 string  `json:"code"`
	FunctionName         string  `json:"functionName"`
	Name                 string  `json:"name"`
	HolderName           *string `json:"holderName"`
	Status               string  `json:"status"`
	BudgetUSD            float64 `json:"budgetUsd"`
	SpentUSD             float64 `json:"spentUsd"`
	CommittedUSD         float64 `json:"committedUsd"`
	LeftUSD              float64 `json:"leftUsd"`
	UsedPercent          float64 `json:"usedPercent"`
	Trips                int     `json:"trips"`
	OpenTripsWithoutCost int     `json:"openTripsWithoutCost"`
}

// BudgetTotals are the totals row of the summary.
type BudgetTotals struct {
	BudgetUSD    float64 `json:"budgetUsd"`
	SpentUSD     float64 `json:"spentUsd"`
	CommittedUSD float64 `json:"committedUsd"`
	LeftUSD      float64 `json:"leftUsd"`
	UsedPercent  float64 `json:"usedPercent"`
}

// BudgetUnassigned is the spend that belongs to no line.
type BudgetUnassigned struct {
	SpentUSD         float64 `json:"spentUsd"`
	TripsWithoutLine int     `json:"tripsWithoutLine"`
	NoTripSpendUSD   float64 `json:"noTripSpendUsd"`
}

// BudgetUnconverted counts the costs left out because no rate was found.
type BudgetUnconverted struct {
	Count      int      `json:"count"`
	Currencies []string `json:"currencies"`
}

// BudgetSummary is GET /budgets/summary.
type BudgetSummary struct {
	Year             int                 `json:"year"`
	StartDate        string              `json:"startDate"`
	EndDate          string              `json:"endDate"`
	Years            []int               `json:"years"`
	AsOf             string              `json:"asOf"`
	RatesPending     bool                `json:"ratesPending"`
	ExpenseAvailable bool                `json:"expenseAvailable"`
	Lines            []BudgetSummaryLine `json:"lines"`
	Totals           BudgetTotals        `json:"totals"`
	Unassigned       BudgetUnassigned    `json:"unassigned"`
	NeedsLineCount   int                 `json:"needsLineCount"`
	Unconverted      BudgetUnconverted   `json:"unconverted"`
}

// TripBudgetView is the budget of one trip for the caller.
type TripBudgetView struct {
	JobNumber string         `json:"jobNumber"`
	Year      *int           `json:"year"`
	Line      *BudgetLineRef `json:"line"`
	Suggested *BudgetLineRef `json:"suggested"`
	Source    string         `json:"source"`
	ChosenBy  *string        `json:"chosenBy"`
	NeedsLine bool           `json:"needsLine"`
	CanEdit   bool           `json:"canEdit"`
}

// TripBudgetSet is the body of PUT /travel-requests/{jobNumber}/budget-line.
type TripBudgetSet struct {
	BudgetLineID *int `json:"budgetLineId"`
}

// NeedsLineItem is a trip that has no budget line.
type NeedsLineItem struct {
	JobNumber        string         `json:"jobNumber"`
	TravellerEmail   *string        `json:"travellerEmail"`
	TravellerName    *string        `json:"travellerName"`
	Country          *string        `json:"country"`
	StartDate        *string        `json:"startDate"`
	EndDate          *string        `json:"endDate"`
	Status           *string        `json:"status"`
	EngagementID     *string        `json:"engagementId"`
	EngagementType   *string        `json:"engagementType"`
	CustomerName     *string        `json:"customerName"`
	BudgetYear       *int           `json:"budgetYear"`
	BudgetLineID     *int           `json:"budgetLineId"`
	SuggestedLineID  *int           `json:"suggestedLineId"`
	Suggested        *BudgetLineRef `json:"suggested"`
	SuggestionSource *string        `json:"suggestionSource"`
	ChosenBy         *string        `json:"chosenBy"`
	RateDate         *string        `json:"rateDate"`
	HasRow           bool           `json:"hasRow"`
}

// EngagementBudgetLine is the line set on an engagement.
type EngagementBudgetLine struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
