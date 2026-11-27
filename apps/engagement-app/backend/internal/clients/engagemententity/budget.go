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
	"net/http"
	"net/url"
	"strconv"
)

// BudgetAPI is what the Travel Budgets area needs from the Engagement Entity
// (DESIGN.md section 2). The entity answers 503 on every /budgets route until
// its budget tables exist.
type BudgetAPI interface {
	BudgetYears(ctx context.Context) ([]BudgetYear, error)
	BudgetLines(ctx context.Context, year int, includeRetired bool) ([]BudgetLine, error)
	CreateBudgetLine(ctx context.Context, in BudgetLineCreate) (BudgetLine, error)
	UpdateBudgetLine(ctx context.Context, id int, in BudgetLineUpdate) (BudgetLine, error)
	// RetireBudgetLine soft-retires a line: trips keep it.
	RetireBudgetLine(ctx context.Context, id int, updatedBy string) error
	BudgetAllocationTypeRules(ctx context.Context) ([]BudgetAllocationTypeRule, error)
	SaveBudgetAllocationTypeRule(ctx context.Context, allocationTypeID int, in BudgetRuleSave) (BudgetAllocationTypeRule, error)
	BudgetRegionRules(ctx context.Context) ([]BudgetRegionRule, error)
	BudgetTeamRules(ctx context.Context) ([]BudgetTeamRule, error)
	// BudgetEngagementLine returns the line code set on the engagement, nil when unset.
	BudgetEngagementLine(ctx context.Context, engagementID string) (*string, error)
	// SetBudgetEngagementLine sets the engagement's line; a nil code clears it.
	SetBudgetEngagementLine(ctx context.Context, engagementID string, code *string, updatedBy string) (*string, error)
	BudgetTrip(ctx context.Context, jobNumber string) (BudgetTrip, error)
	// SetBudgetTrip sets the trip's line (nil clears it) as a user choice.
	SetBudgetTrip(ctx context.Context, jobNumber string, lineID *int, updatedBy string) (BudgetTrip, error)
	BudgetLedger(ctx context.Context, from, to string, unassigned bool) (BudgetLedger, error)
	BudgetNeedsLine(ctx context.Context, year, limit, offset int) (BudgetNeedsLinePage, error)
}

// BudgetYear is a budget year with its date bounds.
type BudgetYear struct {
	Year      int    `json:"year"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	IsActive  bool   `json:"isActive"`
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

// BudgetLineCreate is POST /budgets/lines.
type BudgetLineCreate struct {
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

// BudgetLineUpdate is PUT /budgets/lines/{id}: every field is sent, the code
// and year never change.
type BudgetLineUpdate struct {
	FunctionName string  `json:"functionName"`
	SubFunction  string  `json:"subFunction"`
	HolderName   *string `json:"holderName"`
	HolderEmail  *string `json:"holderEmail"`
	AmountUSD    float64 `json:"amountUsd"`
	SortOrder    int     `json:"sortOrder"`
	Status       string  `json:"status"`
	UpdatedBy    string  `json:"updatedBy"`
}

// BudgetAllocationTypeRule says how an allocation type picks a line.
type BudgetAllocationTypeRule struct {
	AllocationTypeID int     `json:"allocationTypeId"`
	RuleKind         string  `json:"ruleKind"`
	LineCode         *string `json:"lineCode"`
}

// BudgetRuleSave is PUT /budgets/allocation-type-rules/{allocationTypeId}.
type BudgetRuleSave struct {
	RuleKind  string  `json:"ruleKind"`
	LineCode  *string `json:"lineCode"`
	UpdatedBy string  `json:"updatedBy"`
}

// BudgetRegionRule maps a region (a trailing * is a prefix) and optional
// sub-region to a line code; a nil code means no line.
type BudgetRegionRule struct {
	Region    string  `json:"region"`
	SubRegion *string `json:"subRegion"`
	LineCode  *string `json:"lineCode"`
}

// BudgetTeamRule maps the traveller's HR team to a line code; nil columns
// match anything.
type BudgetTeamRule struct {
	ID           int     `json:"id"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	Team         *string `json:"team"`
	LineCode     string  `json:"lineCode"`
}

// BudgetTrip is a trip with its budget row (TripBudget).
type BudgetTrip struct {
	JobNumber        string  `json:"jobNumber"`
	TravellerEmail   *string `json:"travellerEmail"`
	Country          *string `json:"country"`
	StartDate        *string `json:"startDate"`
	EndDate          *string `json:"endDate"`
	Status           *string `json:"status"`
	EngagementID     *string `json:"engagementId"`
	EngagementType   *string `json:"engagementType"`
	CustomerName     *string `json:"customerName"`
	BudgetYear       *int    `json:"budgetYear"`
	BudgetLineID     *int    `json:"budgetLineId"`
	SuggestedLineID  *int    `json:"suggestedLineId"`
	SuggestionSource *string `json:"suggestionSource"`
	ChosenBy         *string `json:"chosenBy"`
	RateDate         *string `json:"rateDate"`
	HasRow           bool    `json:"hasRow"`
}

// BudgetLedgerTrip is a trip of the ledger range with its line.
type BudgetLedgerTrip struct {
	JobNumber    string  `json:"jobNumber"`
	BudgetLineID *int    `json:"budgetLineId"`
	Status       string  `json:"status"`
	Country      *string `json:"country"`
	StartDate    string  `json:"startDate"`
	EndDate      *string `json:"endDate"`
	RateDate     *string `json:"rateDate"`
}

// BudgetLedgerCost is the sum of one trip's cost per source and currency.
type BudgetLedgerCost struct {
	JobNumber string  `json:"jobNumber"`
	Source    string  `json:"source"`
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
}

// BudgetLedgerUnassigned is travel spend that names no trip, per month.
type BudgetLedgerUnassigned struct {
	Source   string  `json:"source"`
	Month    string  `json:"month"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// BudgetLedger is the trips and costs of a date range.
type BudgetLedger struct {
	From             string                   `json:"from"`
	To               string                   `json:"to"`
	ExpenseAvailable bool                     `json:"expenseAvailable"`
	Truncated        bool                     `json:"truncated"`
	Trips            []BudgetLedgerTrip       `json:"trips"`
	Costs            []BudgetLedgerCost       `json:"costs"`
	Unassigned       []BudgetLedgerUnassigned `json:"unassigned"`
}

// BudgetNeedsLinePage is one page of trips that have no line.
type BudgetNeedsLinePage struct {
	Items []BudgetTrip `json:"items"`
	Total int          `json:"total"`
}

// AllocationTravelBudget is the optional travelBudget of an allocation write:
// the entity stores it with the trip it creates. The ids may be null.
type AllocationTravelBudget struct {
	BudgetLineID     *int    `json:"budgetLineId"`
	SuggestedLineID  *int    `json:"suggestedLineId"`
	SuggestionSource string  `json:"suggestionSource"`
	ChosenBy         *string `json:"chosenBy"`
}

// BudgetYears calls GET /budgets/years.
func (c *HTTPClient) BudgetYears(ctx context.Context) ([]BudgetYear, error) {
	var out struct {
		Years []BudgetYear `json:"years"`
	}
	if err := c.rest.Get(ctx, "/budgets/years", &out); err != nil {
		return nil, err
	}
	return out.Years, nil
}

// BudgetLines calls GET /budgets/lines.
func (c *HTTPClient) BudgetLines(ctx context.Context, year int, includeRetired bool) ([]BudgetLine, error) {
	q := url.Values{"year": {strconv.Itoa(year)}}
	if includeRetired {
		q.Set("includeRetired", "true")
	}
	var out struct {
		Lines []BudgetLine `json:"lines"`
	}
	if err := c.rest.Get(ctx, withQuery("/budgets/lines", q), &out); err != nil {
		return nil, err
	}
	return out.Lines, nil
}

// CreateBudgetLine calls POST /budgets/lines.
func (c *HTTPClient) CreateBudgetLine(ctx context.Context, in BudgetLineCreate) (BudgetLine, error) {
	var out BudgetLine
	err := c.rest.Post(ctx, "/budgets/lines", in, &out)
	return out, err
}

// UpdateBudgetLine calls PUT /budgets/lines/{id}.
func (c *HTTPClient) UpdateBudgetLine(ctx context.Context, id int, in BudgetLineUpdate) (BudgetLine, error) {
	var out BudgetLine
	err := c.rest.Do(ctx, http.MethodPut, "/budgets/lines/"+strconv.Itoa(id), in, &out)
	return out, err
}

// RetireBudgetLine calls DELETE /budgets/lines/{id}.
func (c *HTTPClient) RetireBudgetLine(ctx context.Context, id int, updatedBy string) error {
	path := withQuery("/budgets/lines/"+strconv.Itoa(id), url.Values{"updatedBy": {updatedBy}})
	return c.rest.Do(ctx, http.MethodDelete, path, nil, nil)
}

// BudgetAllocationTypeRules calls GET /budgets/allocation-type-rules.
func (c *HTTPClient) BudgetAllocationTypeRules(ctx context.Context) ([]BudgetAllocationTypeRule, error) {
	var out struct {
		Rules []BudgetAllocationTypeRule `json:"rules"`
	}
	if err := c.rest.Get(ctx, "/budgets/allocation-type-rules", &out); err != nil {
		return nil, err
	}
	return out.Rules, nil
}

// SaveBudgetAllocationTypeRule calls PUT /budgets/allocation-type-rules/{allocationTypeId}.
func (c *HTTPClient) SaveBudgetAllocationTypeRule(ctx context.Context, allocationTypeID int, in BudgetRuleSave) (BudgetAllocationTypeRule, error) {
	var out BudgetAllocationTypeRule
	err := c.rest.Do(ctx, http.MethodPut, "/budgets/allocation-type-rules/"+strconv.Itoa(allocationTypeID), in, &out)
	return out, err
}

// BudgetRegionRules calls GET /budgets/region-rules.
func (c *HTTPClient) BudgetRegionRules(ctx context.Context) ([]BudgetRegionRule, error) {
	var out struct {
		Rules []BudgetRegionRule `json:"rules"`
	}
	if err := c.rest.Get(ctx, "/budgets/region-rules", &out); err != nil {
		return nil, err
	}
	return out.Rules, nil
}

// BudgetTeamRules calls GET /budgets/team-rules.
func (c *HTTPClient) BudgetTeamRules(ctx context.Context) ([]BudgetTeamRule, error) {
	var out struct {
		Rules []BudgetTeamRule `json:"rules"`
	}
	if err := c.rest.Get(ctx, "/budgets/team-rules", &out); err != nil {
		return nil, err
	}
	return out.Rules, nil
}

type budgetEngagementLine struct {
	EngagementID string  `json:"engagementId"`
	LineCode     *string `json:"lineCode"`
}

// BudgetEngagementLine calls GET /budgets/engagements/{engagementId}/line.
func (c *HTTPClient) BudgetEngagementLine(ctx context.Context, engagementID string) (*string, error) {
	var out budgetEngagementLine
	if err := c.rest.Get(ctx, "/budgets/engagements/"+pathSegment(engagementID)+"/line", &out); err != nil {
		return nil, err
	}
	return out.LineCode, nil
}

// SetBudgetEngagementLine calls PUT /budgets/engagements/{engagementId}/line.
func (c *HTTPClient) SetBudgetEngagementLine(ctx context.Context, engagementID string, code *string, updatedBy string) (*string, error) {
	body := struct {
		LineCode  *string `json:"lineCode"`
		UpdatedBy string  `json:"updatedBy"`
	}{code, updatedBy}
	var out budgetEngagementLine
	if err := c.rest.Do(ctx, http.MethodPut, "/budgets/engagements/"+pathSegment(engagementID)+"/line", body, &out); err != nil {
		return nil, err
	}
	return out.LineCode, nil
}

// BudgetTrip calls GET /budgets/trips/{jobNumber}.
func (c *HTTPClient) BudgetTrip(ctx context.Context, jobNumber string) (BudgetTrip, error) {
	var out BudgetTrip
	err := c.rest.Get(ctx, "/budgets/trips/"+pathSegment(jobNumber), &out)
	return out, err
}

// SetBudgetTrip calls PUT /budgets/trips/{jobNumber}.
func (c *HTTPClient) SetBudgetTrip(ctx context.Context, jobNumber string, lineID *int, updatedBy string) (BudgetTrip, error) {
	body := struct {
		BudgetLineID *int   `json:"budgetLineId"`
		UpdatedBy    string `json:"updatedBy"`
	}{lineID, updatedBy}
	var out BudgetTrip
	err := c.rest.Do(ctx, http.MethodPut, "/budgets/trips/"+pathSegment(jobNumber), body, &out)
	return out, err
}

// BudgetLedger calls GET /budgets/ledger.
func (c *HTTPClient) BudgetLedger(ctx context.Context, from, to string, unassigned bool) (BudgetLedger, error) {
	q := url.Values{"from": {from}, "to": {to}}
	if unassigned {
		q.Set("unassigned", "true")
	}
	var out BudgetLedger
	err := c.rest.Get(ctx, withQuery("/budgets/ledger", q), &out)
	return out, err
}

// BudgetNeedsLine calls GET /budgets/needs-line.
func (c *HTTPClient) BudgetNeedsLine(ctx context.Context, year, limit, offset int) (BudgetNeedsLinePage, error) {
	q := url.Values{"year": {strconv.Itoa(year)}, "limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var out BudgetNeedsLinePage
	err := c.rest.Get(ctx, withQuery("/budgets/needs-line", q), &out)
	return out, err
}
