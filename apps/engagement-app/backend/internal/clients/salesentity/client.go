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

// Package salesentity is the client for the Sales Entity Service
// (digiops-sales/sales-entity-service), which reads Salesforce. The backend
// uses it for account search, opportunities, opportunity line items and
// Marketing Event campaigns.
//
// The service lives in another Choreo project, so calls go through a Choreo
// connection with OAuth2 client credentials (rest.NewOAuthHTTPClient).
// Request and response types carry only the fields the Engagement App uses;
// add fields from the service's openapi.yaml as screens need them.
package salesentity

import (
	"context"
	"net/http"
	"slices"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

const serviceName = "sales-entity"

// Client is what the backend needs from the Sales Entity Service.
type Client interface {
	// SearchCustomers calls POST /customer-search.
	SearchCustomers(ctx context.Context, f CustomerFilter) ([]Customer, error)
	// SearchOpportunities calls POST /opportunities/search for one page.
	SearchOpportunities(ctx context.Context, f OpportunityFilter) ([]Opportunity, error)
	// SearchCampaigns calls POST /campaigns/search.
	SearchCampaigns(ctx context.Context, f CampaignFilter) ([]Campaign, error)
	// Opportunities returns every opportunity of an account (allocation BFF
	// getOpportunities) from POST /opportunities/search, all pages. onlyActive
	// false leaves isOnlyActiveOpportunities unset, as the legacy BFF did.
	Opportunities(ctx context.Context, customerID string, onlyActive bool) ([]SalesOpportunity, error)
	// OpportunityLineItems returns an opportunity's line items with the legacy
	// classifications from POST /opportunity-line-items/search, all pages.
	OpportunityLineItems(ctx context.Context, opportunityID string) ([]SalesLineItem, error)
	// OpportunityLineItem looks one line item up by id (Fetch New Dates); nil
	// when Salesforce has none.
	OpportunityLineItem(ctx context.Context, id string) (*SalesLineItem, error)
}

// CustomerFilter is the subset of the service's CustomersFilter we send.
type CustomerFilter struct {
	IDs     []string `json:"ids,omitempty"`
	Company string   `json:"company,omitempty"`
	// CustomerNameLike matches accounts whose name contains this text.
	CustomerNameLike string `json:"customerNameLike,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	// IsRealTime reads Salesforce live instead of the sync database.
	IsRealTime bool `json:"isRealTime"`
}

// CustomerAddress is a Salesforce Account address.
type CustomerAddress struct {
	BillingStreet     *string `json:"billingStreet"`
	BillingCity       *string `json:"billingCity"`
	BillingState      *string `json:"billingState"`
	BillingPostalCode *string `json:"billingPostalCode"`
	BillingCountry    *string `json:"billingCountry"`
	Country           *string `json:"country"`
}

// Customer is a Salesforce Account.
type Customer struct {
	ID             string           `json:"id"`
	Name           *string          `json:"name"`
	SalesRegion    *string          `json:"salesRegion"`
	SubRegion      *string          `json:"subRegion"`
	BillingCountry *string          `json:"billingCountry"`
	Address        *CustomerAddress `json:"address"`
}

// OpportunityFilter is the subset of the service's OpportunityFilter we send.
type OpportunityFilter struct {
	IDs          []string `json:"ids,omitempty"`
	CustomerID   string   `json:"customerId,omitempty"`
	NameContains string   `json:"nameContains,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	Offset       int      `json:"offset,omitempty"`
}

// Opportunity is a Salesforce Opportunity.
type Opportunity struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	CustomerID   string  `json:"customerId"`
	StageName    *string `json:"stageName"`
	BusinessUnit *string `json:"businessUnit"`
	IsWon        *bool   `json:"isWon"`
}

// CampaignFilter is the service's CampaignFilter.
type CampaignFilter struct {
	// IDs looks campaigns up by id (any status unless IsActive is set).
	IDs []string `json:"ids,omitempty"`
	// NameLike matches campaigns whose name contains this text.
	NameLike string `json:"nameLike,omitempty"`
	// IsActive nil means active only (the service default).
	IsActive *bool `json:"isActive,omitempty"`
	Limit    int   `json:"limit,omitempty"`
	Offset   int   `json:"offset,omitempty"`
}

// Campaign is a Salesforce Campaign, the Marketing Event's campaign_id.
type Campaign struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      *string `json:"type"`
	Status    *string `json:"status"`
	IsActive  bool    `json:"isActive"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// SalesUser is a Salesforce user.
type SalesUser struct {
	Name *string `json:"name"`
}

// SalesOpportunity is the subset of the service's OpportunityData that the
// allocation screens use (the legacy GraphQL Opportunity fields).
type SalesOpportunity struct {
	ID                      string     `json:"id"`
	Name                    string     `json:"name"`
	Owner                   *SalesUser `json:"owner"`
	PreSalesCode            *string    `json:"preSalesCode"`
	PostSalesEngagementCode *string    `json:"postSalesEngagementCode"`
	CustomerAddress         *string    `json:"customerAddress"`
	TechnicalOwner          *string    `json:"technicalOwner"`
	IsWon                   *bool      `json:"isWon"`
}

// SalesLineItem is the subset of the service's OpportunityLineItem that the
// allocation screens use (the legacy GraphQL subscription line item fields).
type SalesLineItem struct {
	ID                 *string `json:"id"`
	Name               *string `json:"name"`
	Classification     *string `json:"classification"`
	Description        *string `json:"description"`
	EngagementCodeAuto *string `json:"engagementCodeAuto"`
	IsTentativeDates   *bool   `json:"isTentativeDates"`
	ServiceStartDate   *string `json:"serviceStartDate"`
	ServiceEndDate     *string `json:"serviceEndDate"`
	ProductUnit        *string `json:"productUnit"`
	BusinessUnit       *string `json:"businessUnit"`
}

// searchPageSize is the service's largest page (and the legacy DEFAULT_LIMIT);
// pages are read until a short one, with searchMaxPages as a safety stop.
const (
	searchPageSize = 1000
	searchMaxPages = 50
)

// LineItemClassifications are the line item classifications the legacy
// create form asked for.
var LineItemClassifications = []string{"CO", "QS", "PS", "TR", "TAM", "CL"}

// HTTPClient implements Client over the Go Sales Entity Service's REST API. A
// nil rest client means SALES_ENTITY_BASE_URL is unset: calls answer 503.
type HTTPClient struct {
	rest *rest.Client
}

// New returns an HTTPClient for the service at baseURL (empty means not
// configured), authenticating with httpClient (normally
// rest.NewOAuthHTTPClient).
func New(baseURL string, httpClient *http.Client) *HTTPClient {
	c := &HTTPClient{}
	if baseURL != "" {
		c.rest = rest.New(serviceName, baseURL, httpClient)
	}
	return c
}

var _ Client = (*HTTPClient)(nil)

// SearchCustomers implements Client.
func (c *HTTPClient) SearchCustomers(ctx context.Context, f CustomerFilter) ([]Customer, error) {
	var out []Customer
	if c.rest == nil {
		return nil, notConfigured()
	}
	return out, c.rest.Post(ctx, "/customer-search", f, &out)
}

// SearchOpportunities implements Client.
func (c *HTTPClient) SearchOpportunities(ctx context.Context, f OpportunityFilter) ([]Opportunity, error) {
	var out []Opportunity
	if c.rest == nil {
		return nil, notConfigured()
	}
	return out, c.rest.Post(ctx, "/opportunities/search", f, &out)
}

// SearchCampaigns implements Client.
func (c *HTTPClient) SearchCampaigns(ctx context.Context, f CampaignFilter) ([]Campaign, error) {
	var out []Campaign
	if c.rest == nil {
		return nil, notConfigured()
	}
	return out, c.rest.Post(ctx, "/campaigns/search", f, &out)
}

// opportunitySearch is the service's OpportunityFilter as Opportunities sends
// it. sortBy id keeps offset paging stable.
type opportunitySearch struct {
	CustomerID                string `json:"customerId"`
	IsOnlyActiveOpportunities *bool  `json:"isOnlyActiveOpportunities,omitempty"`
	Limit                     int    `json:"limit"`
	Offset                    int    `json:"offset"`
	SortBy                    string `json:"sortBy"`
}

// lineItemSearch is the service's OpportunityLineItemFilter. It has no
// classification filter or sort, so classifications are filtered here.
type lineItemSearch struct {
	ID            string `json:"id,omitempty"`
	OpportunityID string `json:"opportunityId,omitempty"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
}

// searchAll posts page after page of a search to path until a short page.
func searchAll[T any](ctx context.Context, c *rest.Client, path string, page func(offset int) any) ([]T, error) {
	if c == nil {
		return nil, notConfigured()
	}
	out := []T{}
	for i := 0; i < searchMaxPages; i++ {
		var batch []T
		if err := c.Post(ctx, path, page(len(out)), &batch); err != nil {
			return nil, err
		}
		out = append(out, batch...)
		if len(batch) < searchPageSize {
			break
		}
	}
	return out, nil
}

// Opportunities implements Client.
func (c *HTTPClient) Opportunities(ctx context.Context, customerID string, onlyActive bool) ([]SalesOpportunity, error) {
	var active *bool
	if onlyActive {
		active = &onlyActive
	}
	return searchAll[SalesOpportunity](ctx, c.rest, "/opportunities/search", func(offset int) any {
		return opportunitySearch{CustomerID: customerID, IsOnlyActiveOpportunities: active,
			Limit: searchPageSize, Offset: offset, SortBy: "id"}
	})
}

// OpportunityLineItems implements Client.
func (c *HTTPClient) OpportunityLineItems(ctx context.Context, opportunityID string) ([]SalesLineItem, error) {
	all, err := searchAll[SalesLineItem](ctx, c.rest, "/opportunity-line-items/search", func(offset int) any {
		return lineItemSearch{OpportunityID: opportunityID, Limit: searchPageSize, Offset: offset}
	})
	if err != nil {
		return nil, err
	}
	out := []SalesLineItem{}
	for _, li := range all {
		if li.Classification != nil && slices.Contains(LineItemClassifications, *li.Classification) {
			out = append(out, li)
		}
	}
	return out, nil
}

// OpportunityLineItem implements Client.
func (c *HTTPClient) OpportunityLineItem(ctx context.Context, id string) (*SalesLineItem, error) {
	if c.rest == nil {
		return nil, notConfigured()
	}
	var items []SalesLineItem
	if err := c.rest.Post(ctx, "/opportunity-line-items/search", lineItemSearch{ID: id, Limit: 1}, &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

// Unconfigured is the Client used when SALES_ENTITY_BASE_URL is unset: the
// backend starts, and every call answers 503.
type Unconfigured struct{}

var _ Client = Unconfigured{}

func notConfigured() error {
	return &apierror.UpstreamError{Service: serviceName, Msg: "not configured (SALES_ENTITY_BASE_URL is unset)"}
}

// SearchCustomers implements Client.
func (Unconfigured) SearchCustomers(context.Context, CustomerFilter) ([]Customer, error) {
	return nil, notConfigured()
}

// SearchOpportunities implements Client.
func (Unconfigured) SearchOpportunities(context.Context, OpportunityFilter) ([]Opportunity, error) {
	return nil, notConfigured()
}

// SearchCampaigns implements Client.
func (Unconfigured) SearchCampaigns(context.Context, CampaignFilter) ([]Campaign, error) {
	return nil, notConfigured()
}

// Opportunities implements Client.
func (Unconfigured) Opportunities(context.Context, string, bool) ([]SalesOpportunity, error) {
	return nil, notConfigured()
}

// OpportunityLineItems implements Client.
func (Unconfigured) OpportunityLineItems(context.Context, string) ([]SalesLineItem, error) {
	return nil, notConfigured()
}

// OpportunityLineItem implements Client.
func (Unconfigured) OpportunityLineItem(context.Context, string) (*SalesLineItem, error) {
	return nil, notConfigured()
}
