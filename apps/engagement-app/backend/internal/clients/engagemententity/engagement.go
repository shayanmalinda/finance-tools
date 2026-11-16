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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// EngagementAPI is what the engagements area (engagements, approvals,
// reference data, Role Settings, partners, allocation type settings) needs
// from the Engagement Entity. The area owns this file.
type EngagementAPI interface {
	// ReferenceData calls GET /reference-data.
	ReferenceData(ctx context.Context) (ReferenceLists, error)
	// EngagementSearch calls POST /engagements/search.
	EngagementSearch(ctx context.Context, q EngagementSearchQuery) (EngagementSearchResult, error)
	// EngagementFacets calls GET /engagements/facets.
	EngagementFacets(ctx context.Context, engagementTypeIDs []int) (EngagementFacetLists, error)
	// EngagementGet calls GET /engagements/{id}.
	EngagementGet(ctx context.Context, id string) (EngagementRecord, error)
	// EngagementRoster calls GET /engagements/{id}/roster.
	EngagementRoster(ctx context.Context, id string, activeOnly bool) ([]EngagementRosterAllocation, error)
	// EngagementCreate calls POST /engagements.
	EngagementCreate(ctx context.Context, in EngagementCreateInput) (EngagementRecord, error)
	// EngagementUpdate calls PATCH /engagements/{id}.
	EngagementUpdate(ctx context.Context, id string, in EngagementUpdateInput) (EngagementUpdateResult, error)
	// RoleSettings calls GET /role-settings/{app}.
	RoleSettings(ctx context.Context, app domain.App) (RoleSettingsLists, error)
	// RoleReplacePrivileges calls PUT /role-settings/{app}/roles/{roleId}/privileges.
	RoleReplacePrivileges(ctx context.Context, app domain.App, roleID int, privilegeIDs []int, updatedBy string) (RoleWithPrivilegeIDs, error)
	// SettingsCreateAllocationType calls POST /settings/allocation-types.
	SettingsCreateAllocationType(ctx context.Context, in SettingsAllocationTypeFields) (AllocationType, error)
	// SettingsUpdateAllocationType calls PUT /settings/allocation-types/{id}.
	SettingsUpdateAllocationType(ctx context.Context, id int, in SettingsAllocationTypeFields) (AllocationType, error)
	// SettingsDeleteAllocationType calls DELETE /settings/allocation-types/{id}.
	SettingsDeleteAllocationType(ctx context.Context, id int, actor string) error
}

var _ EngagementAPI = (*HTTPClient)(nil)

// ── Reference data ───────────────────────────────────────────────────────────

// ReferenceItem is a business unit or region.
type ReferenceItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ReferenceProductUnit is a product unit with its business unit's name.
type ReferenceProductUnit struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Active       bool    `json:"active"`
	BusinessUnit *string `json:"businessUnit"`
	Customer     bool    `json:"customer"`
	Internal     bool    `json:"internal"`
	Marketing    bool    `json:"marketing"`
}

// ReferenceSubRegion is a sub region with its region's name.
type ReferenceSubRegion struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Active bool    `json:"active"`
	Region *string `json:"region"`
}

// ReferenceTravelAgency is a travel agency.
type ReferenceTravelAgency struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	NetsuiteVendorID *string `json:"netsuiteVendorId"`
	Active           bool    `json:"active"`
}

// ReferenceCostType is an invoice cost type.
type ReferenceCostType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ReferencePerDiemRate is an active per diem rate.
type ReferencePerDiemRate struct {
	Country  string  `json:"country"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// ReferenceLists is GET /reference-data.
type ReferenceLists struct {
	EngagementTypes []EngagementType        `json:"engagementTypes"`
	AllocationTypes []AllocationType        `json:"allocationTypes"`
	BusinessUnits   []ReferenceItem         `json:"businessUnits"`
	ProductUnits    []ReferenceProductUnit  `json:"productUnits"`
	Regions         []ReferenceItem         `json:"regions"`
	SubRegions      []ReferenceSubRegion    `json:"subRegions"`
	TravelAgencies  []ReferenceTravelAgency `json:"travelAgencies"`
	CostTypes       []ReferenceCostType     `json:"costTypes"`
	PerDiemRates    []ReferencePerDiemRate  `json:"perDiemRates"`
}

// ── Engagements ──────────────────────────────────────────────────────────────

// EngagementFundingRow is an engagement funding source, by names.
type EngagementFundingRow struct {
	BusinessUnit string  `json:"businessUnit"`
	ProductUnit  string  `json:"productUnit"`
	Region       string  `json:"region"`
	SubRegion    string  `json:"subRegion"`
	Percentage   float64 `json:"percentage"`
}

// EngagementRecord is an engagement row as the entity returns it.
type EngagementRecord struct {
	ID                    string                 `json:"id"`
	EngagementTypeID      int                    `json:"engagementTypeId"`
	EngagementTypeName    *string                `json:"engagementTypeName"`
	AllocationTypeID      int                    `json:"allocationTypeId"`
	AllocationTypeName    *string                `json:"allocationTypeName"`
	CustomerCode          *string                `json:"customerCode"`
	CustomerName          *string                `json:"customerName"`
	ProjectName           *string                `json:"projectName"`
	OpportunityID         *string                `json:"opportunityId"`
	OpportunityLineItemID *string                `json:"opportunityLineItemId"`
	ProductName           *string                `json:"productName"`
	Address               *string                `json:"address"`
	Country               *string                `json:"country"`
	BusinessDomain        *string                `json:"businessDomain"`
	CreatedBy             *string                `json:"createdBy"`
	ContractsSigned       *bool                  `json:"contractsSigned"`
	EngagementCode        *string                `json:"engagementCode"`
	StartDate             *string                `json:"startDate"`
	EndDate               *string                `json:"endDate"`
	RequestStatus         *string                `json:"requestStatus"`
	EngagementScope       *string                `json:"engagementScope"`
	EngagementNature      *string                `json:"engagementNature"`
	EngagementStatus      *string                `json:"engagementStatus"`
	CustomerContact       *string                `json:"customerContact"`
	OpportunityOwner      *string                `json:"opportunityOwner"`
	TentativeDates        *bool                  `json:"tentativeDates"`
	TechnicalOwner        *string                `json:"technicalOwner"`
	Comment               *string                `json:"comment"`
	Description           *string                `json:"description"`
	UpdatedBy             *string                `json:"updatedBy"`
	CreatedOn             *string                `json:"createdOn"`
	CampaignID            *string                `json:"campaignId"`
	FundingSources        []EngagementFundingRow `json:"fundingSources"`
	Consultants           []string               `json:"consultants,omitempty"`
}

// EngagementSearchQuery is the body of POST /engagements/search.
type EngagementSearchQuery struct {
	PendingApproval    bool     `json:"pendingApproval"`
	EngagementTypeIDs  []int    `json:"engagementTypeIds,omitempty"`
	Search             string   `json:"search,omitempty"`
	Accounts           []string `json:"accounts,omitempty"`
	AllocationTypes    []string `json:"allocationTypes,omitempty"`
	RequestStatuses    []string `json:"requestStatuses,omitempty"`
	EngagementStatuses []string `json:"engagementStatuses,omitempty"`
	BusinessUnits      []string `json:"businessUnits,omitempty"`
	Countries          []string `json:"countries,omitempty"`
	Consultants        string   `json:"consultants,omitempty"`
	StartFrom          string   `json:"startFrom,omitempty"`
	StartTo            string   `json:"startTo,omitempty"`
	EndTo              string   `json:"endTo,omitempty"`
	Sort               string   `json:"sort,omitempty"`
	Today              string   `json:"today,omitempty"`
	IncludeConsultants bool     `json:"includeConsultants"`
	Limit              int      `json:"limit"`
	Offset             int      `json:"offset"`
}

// EngagementSearchResult is POST /engagements/search.
type EngagementSearchResult struct {
	Items      []EngagementRecord `json:"items"`
	Total      int                `json:"total"`
	TypeCounts map[int]int        `json:"typeCounts"`
}

// EngagementFacetLists is GET /engagements/facets.
type EngagementFacetLists struct {
	Accounts        []string `json:"accounts"`
	AllocationTypes []string `json:"allocationTypes"`
	Countries       []string `json:"countries"`
}

// EngagementTravelRef is a travel request raised for an allocation.
type EngagementTravelRef struct {
	JobNumber string  `json:"jobNumber"`
	Status    *string `json:"status"`
}

// EngagementRosterAllocation is an allocation of an engagement.
type EngagementRosterAllocation struct {
	ID               string                `json:"id"`
	Email            string                `json:"email"`
	AllocationTypeID *int                  `json:"allocationTypeId"`
	StartDate        *string               `json:"startDate"`
	EndDate          *string               `json:"endDate"`
	Comment          *string               `json:"comment"`
	ClearanceStatus  *string               `json:"clearanceStatus"`
	IsTravelRequired *string               `json:"isTravelRequired"`
	AddedBy          *string               `json:"addedBy"`
	AddedDate        *string               `json:"addedDate"`
	IsRecurring      *string               `json:"isRecurring"`
	StartTime        *string               `json:"startTime"`
	EndTime          *string               `json:"endTime"`
	TimeZone         *string               `json:"timeZone"`
	Status           *string               `json:"status"`
	TravelRequests   []EngagementTravelRef `json:"travelRequests"`
}

// EngagementCreateInput is the body of POST /engagements.
type EngagementCreateInput struct {
	AllocationTypeID      int                    `json:"allocationTypeId"`
	CustomerName          string                 `json:"customerName"`
	CustomerCode          string                 `json:"customerCode"`
	Address               string                 `json:"address"`
	BusinessDomain        string                 `json:"businessDomain"`
	ContractsSigned       bool                   `json:"contractsSigned"`
	EngagementCode        string                 `json:"engagementCode"`
	StartDate             string                 `json:"startDate"`
	EndDate               string                 `json:"endDate"`
	RequestStatus         string                 `json:"requestStatus"`
	ProjectName           string                 `json:"projectName"`
	OpportunityID         string                 `json:"opportunityId"`
	OpportunityLineItemID string                 `json:"opportunityLineItemId"`
	OpportunityOwner      string                 `json:"opportunityOwner"`
	ProductName           string                 `json:"productName"`
	EngagementScope       string                 `json:"engagementScope"`
	EngagementStatus      string                 `json:"engagementStatus"`
	TentativeDates        bool                   `json:"tentativeDates"`
	EngagementNature      string                 `json:"engagementNature"`
	CustomerContact       string                 `json:"customerContact"`
	TechnicalOwner        string                 `json:"technicalOwner"`
	Comment               string                 `json:"comment"`
	Country               string                 `json:"country"`
	Description           string                 `json:"description"`
	CampaignID            *string                `json:"campaignId"`
	FundingSources        []EngagementFundingRow `json:"fundingSources"`
	UUID                  string                 `json:"uuid"`
	CreatedBy             string                 `json:"createdBy"`
}

// EngagementUpdateInput is the body of PATCH /engagements/{id}: only the
// fields set are changed.
type EngagementUpdateInput struct {
	AllocationTypeID *int                   `json:"allocationTypeId,omitempty"`
	EngagementCode   *string                `json:"engagementCode,omitempty"`
	EngagementScope  *string                `json:"engagementScope,omitempty"`
	EngagementNature *string                `json:"engagementNature,omitempty"`
	Comment          *string                `json:"comment,omitempty"`
	StartDate        *string                `json:"startDate,omitempty"`
	EndDate          *string                `json:"endDate,omitempty"`
	EngagementStatus *string                `json:"engagementStatus,omitempty"`
	RequestStatus    *string                `json:"requestStatus,omitempty"`
	Country          *string                `json:"country,omitempty"`
	CustomerContact  *string                `json:"customerContact,omitempty"`
	Address          *string                `json:"address,omitempty"`
	Description      *string                `json:"description,omitempty"`
	ProjectName      *string                `json:"projectName,omitempty"`
	CampaignID       *string                `json:"campaignId,omitempty"`
	TentativeDates   *bool                  `json:"tentativeDates,omitempty"`
	FundingSources   []EngagementFundingRow `json:"fundingSources,omitempty"`
	UpdatedBy        string                 `json:"updatedBy"`
}

// EngagementUpdateResult is PATCH /engagements/{id}.
type EngagementUpdateResult struct {
	Engagement           EngagementRecord             `json:"engagement"`
	CancelledAllocations []EngagementRosterAllocation `json:"cancelledAllocations"`
}

// ── Role Settings ────────────────────────────────────────────────────────────

// RoleWithPrivilegeIDs is a role with the privilege ids it grants.
type RoleWithPrivilegeIDs struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PrivilegeIDs []int  `json:"privilegeIds"`
}

// RoleSettingsLists is GET /role-settings/{app}.
type RoleSettingsLists struct {
	Roles      []RoleWithPrivilegeIDs `json:"roles"`
	Privileges []Privilege            `json:"privileges"`
}

// ── Partners ─────────────────────────────────────────────────────────────────

// ── Allocation type settings ─────────────────────────────────────────────────

// SettingsAllocationTypeFields is the body of the allocation type writes.
type SettingsAllocationTypeFields struct {
	EngagementTypeID      int    `json:"engagementTypeId"`
	AllocationTypeName    string `json:"allocationTypeName"`
	DefaultEngagementCode string `json:"defaultEngagementCode"`
	EngagementCodeSuffix  string `json:"engagementCodeSuffix"`
	Department            string `json:"department"`
	EmailToAllocations    string `json:"emailToAllocations"`
	Color                 string `json:"color"`
	Actor                 string `json:"actor"`
}

type engagementActorBody struct {
	Actor string `json:"actor"`
}

// ── HTTP implementations ─────────────────────────────────────────────────────

// ReferenceData implements EngagementAPI.
func (c *HTTPClient) ReferenceData(ctx context.Context) (ReferenceLists, error) {
	var out ReferenceLists
	return out, c.rest.Get(ctx, "/reference-data", &out)
}

// EngagementSearch implements EngagementAPI.
func (c *HTTPClient) EngagementSearch(ctx context.Context, q EngagementSearchQuery) (EngagementSearchResult, error) {
	var out EngagementSearchResult
	return out, c.rest.Post(ctx, "/engagements/search", q, &out)
}

// EngagementFacets implements EngagementAPI.
func (c *HTTPClient) EngagementFacets(ctx context.Context, engagementTypeIDs []int) (EngagementFacetLists, error) {
	q := url.Values{}
	for _, id := range engagementTypeIDs {
		q.Add("engagementTypeId", strconv.Itoa(id))
	}
	var out EngagementFacetLists
	return out, c.rest.Get(ctx, withQuery("/engagements/facets", q), &out)
}

func engagementPath(id string) string { return "/engagements/" + url.PathEscape(id) }

// EngagementGet implements EngagementAPI.
func (c *HTTPClient) EngagementGet(ctx context.Context, id string) (EngagementRecord, error) {
	var out EngagementRecord
	return out, c.rest.Get(ctx, engagementPath(id), &out)
}

// EngagementRoster implements EngagementAPI.
func (c *HTTPClient) EngagementRoster(ctx context.Context, id string, activeOnly bool) ([]EngagementRosterAllocation, error) {
	var out struct {
		Allocations []EngagementRosterAllocation `json:"allocations"`
	}
	path := withQuery(engagementPath(id)+"/roster", url.Values{"activeOnly": {strconv.FormatBool(activeOnly)}})
	return out.Allocations, c.rest.Get(ctx, path, &out)
}

// EngagementCreate implements EngagementAPI.
func (c *HTTPClient) EngagementCreate(ctx context.Context, in EngagementCreateInput) (EngagementRecord, error) {
	var out EngagementRecord
	return out, c.rest.Post(ctx, "/engagements", in, &out)
}

// EngagementUpdate implements EngagementAPI.
func (c *HTTPClient) EngagementUpdate(ctx context.Context, id string, in EngagementUpdateInput) (EngagementUpdateResult, error) {
	var out EngagementUpdateResult
	return out, c.rest.Do(ctx, http.MethodPatch, engagementPath(id), in, &out)
}

func roleSettingsPath(app domain.App) string { return "/role-settings/" + url.PathEscape(string(app)) }

// RoleSettings implements EngagementAPI.
func (c *HTTPClient) RoleSettings(ctx context.Context, app domain.App) (RoleSettingsLists, error) {
	var out RoleSettingsLists
	return out, c.rest.Get(ctx, roleSettingsPath(app), &out)
}

// RoleReplacePrivileges implements EngagementAPI.
func (c *HTTPClient) RoleReplacePrivileges(ctx context.Context, app domain.App, roleID int, privilegeIDs []int, updatedBy string) (RoleWithPrivilegeIDs, error) {
	body := struct {
		PrivilegeIDs []int  `json:"privilegeIds"`
		UpdatedBy    string `json:"updatedBy"`
	}{PrivilegeIDs: privilegeIDs, UpdatedBy: updatedBy}
	if body.PrivilegeIDs == nil {
		body.PrivilegeIDs = []int{}
	}
	var out RoleWithPrivilegeIDs
	path := roleSettingsPath(app) + "/roles/" + strconv.Itoa(roleID) + "/privileges"
	return out, c.rest.Do(ctx, http.MethodPut, path, body, &out)
}

// SettingsCreateAllocationType implements EngagementAPI.
func (c *HTTPClient) SettingsCreateAllocationType(ctx context.Context, in SettingsAllocationTypeFields) (AllocationType, error) {
	var out AllocationType
	return out, c.rest.Post(ctx, "/settings/allocation-types", in, &out)
}

// SettingsUpdateAllocationType implements EngagementAPI.
func (c *HTTPClient) SettingsUpdateAllocationType(ctx context.Context, id int, in SettingsAllocationTypeFields) (AllocationType, error) {
	var out AllocationType
	return out, c.rest.Do(ctx, http.MethodPut, "/settings/allocation-types/"+strconv.Itoa(id), in, &out)
}

// SettingsDeleteAllocationType implements EngagementAPI.
func (c *HTTPClient) SettingsDeleteAllocationType(ctx context.Context, id int, actor string) error {
	return c.rest.Do(ctx, http.MethodDelete, "/settings/allocation-types/"+strconv.Itoa(id), engagementActorBody{Actor: actor}, nil)
}
