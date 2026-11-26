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

// Legacy engagement.status (request status) values.
const (
	EngagementRequestPendingApproval = "Pending Approval"
	EngagementRequestNew             = "New"
	EngagementRequestUpdated         = "Updated"
	EngagementRequestApproved        = "Approved"
	EngagementRequestInProgress      = "In Progress"
	EngagementRequestCompleted       = "Completed"
	EngagementRequestCancelled       = "Cancelled"
)

// EngagementStatusCancelled is the engagement.engagement_status value that
// cancels an engagement and cascades to its allocations and travel requests.
const EngagementStatusCancelled = "Cancelled"

// Legacy allocation values the engagement cancel cascade reads and writes.
const (
	EngagementAllocationInactive           = "Inactive"
	EngagementClearanceConfirmed           = "Confirmed"
	EngagementClearanceConfirmedVisaPend   = "Confirmed - Visa Pending"
	EngagementClearanceAllocationCancelled = "Allocation Cancelled"
	EngagementTravelRequiredYes            = "Yes"
	EngagementTravelStatusCancelled        = "Cancelled"
)

// EngagementTypeMarketingEvent is engagement_type 5 (Marketing Event).
const EngagementTypeMarketingEvent = 5

// EngagementFundingSource is a row of engagement_funding_source, resolved to
// names as the legacy getFundingSources returns it.
type EngagementFundingSource struct {
	BusinessUnit string  `json:"businessUnit"`
	ProductUnit  string  `json:"productUnit"`
	Region       string  `json:"region"`
	SubRegion    string  `json:"subRegion"`
	Percentage   float64 `json:"percentage"`
}

// Engagement is a row of allocation_db.engagement with its allocation and
// engagement type. Nullable columns are pointers.
type Engagement struct {
	ID                    string                    `json:"id"`
	EngagementTypeID      int                       `json:"engagementTypeId"`
	EngagementTypeName    *string                   `json:"engagementTypeName"`
	AllocationTypeID      int                       `json:"allocationTypeId"`
	AllocationTypeName    *string                   `json:"allocationTypeName"`
	CustomerCode          *string                   `json:"customerCode"`
	CustomerName          *string                   `json:"customerName"`
	ProjectName           *string                   `json:"projectName"`
	OpportunityID         *string                   `json:"opportunityId"`
	OpportunityLineItemID *string                   `json:"opportunityLineItemId"`
	ProductName           *string                   `json:"productName"`
	Address               *string                   `json:"address"`
	Country               *string                   `json:"country"`
	BusinessDomain        *string                   `json:"businessDomain"`
	CreatedBy             *string                   `json:"createdBy"`
	ContractsSigned       *bool                     `json:"contractsSigned"`
	EngagementCode        *string                   `json:"engagementCode"`
	StartDate             *string                   `json:"startDate"`
	EndDate               *string                   `json:"endDate"`
	RequestStatus         *string                   `json:"requestStatus"`
	EngagementScope       *string                   `json:"engagementScope"`
	EngagementNature      *string                   `json:"engagementNature"`
	EngagementStatus      *string                   `json:"engagementStatus"`
	CustomerContact       *string                   `json:"customerContact"`
	OpportunityOwner      *string                   `json:"opportunityOwner"`
	TentativeDates        *bool                     `json:"tentativeDates"`
	TechnicalOwner        *string                   `json:"technicalOwner"`
	Comment               *string                   `json:"comment"`
	Description           *string                   `json:"description"`
	UpdatedBy             *string                   `json:"updatedBy"`
	CreatedOn             *string                   `json:"createdOn"`
	CampaignID            *string                   `json:"campaignId"`
	FundingSources        []EngagementFundingSource `json:"fundingSources"`
	// Consultants are the distinct emails of the engagement's Active
	// allocations; set by the search when asked for.
	Consultants []string `json:"consultants,omitempty"`
}

// Engagement list sort orders.
const (
	// EngagementSortCurrent puts engagements ending today or later first
	// (start ascending), then finished ones (start descending).
	EngagementSortCurrent = "current"
	// EngagementSortCreated is oldest created first (Approvals).
	EngagementSortCreated = "created"
)

// EngagementSearchRequest is the body of POST /engagements/search. Empty
// fields mean "any". Multi-value fields are OR within, AND across.
type EngagementSearchRequest struct {
	// PendingApproval true lists only Pending Approval engagements (the
	// legacy approval search); false lists every other engagement.
	PendingApproval bool `json:"pendingApproval"`
	// IncludePending true adds the Pending Approval engagements to the
	// "every other engagement" list. Absent or false keeps the legacy list
	// (no Pending Approval). Ignored when PendingApproval is true.
	IncludePending bool `json:"includePending"`
	// PendingCreator, with PendingApproval true, keeps only the Pending
	// Approval engagements created by this person (the creator's own).
	PendingCreator    string   `json:"pendingCreator"`
	EngagementTypeIDs []int    `json:"engagementTypeIds"`
	Search            string   `json:"search"`
	Accounts          []string `json:"accounts"`
	AllocationTypes   []string `json:"allocationTypes"`
	RequestStatuses   []string `json:"requestStatuses"`
	// EngagementStatuses match engagement_status.
	EngagementStatuses []string `json:"engagementStatuses"`
	BusinessUnits      []string `json:"businessUnits"`
	Countries          []string `json:"countries"`
	// Consultants is "allocated" (has an Active allocation), "none" or "".
	Consultants string `json:"consultants"`
	StartFrom   string `json:"startFrom"`
	StartTo     string `json:"startTo"`
	EndTo       string `json:"endTo"`
	// Sort is EngagementSortCurrent (default), EngagementSortCreated, or one
	// of the column sorts (engagements: id, type, country, startDate,
	// requestStatus, engagementStatus, created; approvals: id, type, country,
	// startDate, businessUnit, created; a leading "-" means descending).
	// An unknown value gives the default.
	Sort string `json:"sort"`
	// Today (YYYY-MM-DD) splits current from finished rows; required for
	// the current sort.
	Today              string `json:"today"`
	IncludeConsultants bool   `json:"includeConsultants"`
	Limit              int    `json:"limit"`
	Offset             int    `json:"offset"`
}

// EngagementSearchResponse is returned by POST /engagements/search.
type EngagementSearchResponse struct {
	Items []Engagement `json:"items"`
	// Total counts every match.
	Total int `json:"total"`
	// TypeCounts counts the base set (PendingApproval only, no other filter)
	// per engagement type id, for the segment counts.
	TypeCounts map[int]int `json:"typeCounts"`
}

// EngagementFacetsResponse is returned by GET /engagements/facets.
type EngagementFacetsResponse struct {
	Accounts        []string `json:"accounts"`
	AllocationTypes []string `json:"allocationTypes"`
	Countries       []string `json:"countries"`
}

// EngagementCreateRequest is the body of POST /engagements. The backend has
// resolved every Salesforce value; the entity stores what it receives, as
// the legacy addEngagement did.
type EngagementCreateRequest struct {
	AllocationTypeID      int                       `json:"allocationTypeId"`
	CustomerName          string                    `json:"customerName"`
	CustomerCode          string                    `json:"customerCode"`
	Address               string                    `json:"address"`
	BusinessDomain        string                    `json:"businessDomain"`
	ContractsSigned       bool                      `json:"contractsSigned"`
	EngagementCode        string                    `json:"engagementCode"`
	StartDate             string                    `json:"startDate"`
	EndDate               string                    `json:"endDate"`
	RequestStatus         string                    `json:"requestStatus"`
	ProjectName           string                    `json:"projectName"`
	OpportunityID         string                    `json:"opportunityId"`
	OpportunityLineItemID string                    `json:"opportunityLineItemId"`
	OpportunityOwner      string                    `json:"opportunityOwner"`
	ProductName           string                    `json:"productName"`
	EngagementScope       string                    `json:"engagementScope"`
	EngagementStatus      string                    `json:"engagementStatus"`
	TentativeDates        bool                      `json:"tentativeDates"`
	EngagementNature      string                    `json:"engagementNature"`
	CustomerContact       string                    `json:"customerContact"`
	TechnicalOwner        string                    `json:"technicalOwner"`
	Comment               string                    `json:"comment"`
	Country               string                    `json:"country"`
	Description           string                    `json:"description"`
	CampaignID            *string                   `json:"campaignId"`
	FundingSources        []EngagementFundingSource `json:"fundingSources"`
	// UUID is generated by the backend; the new row is re-read by it (the
	// id comes from a database trigger).
	UUID      string `json:"uuid"`
	CreatedBy string `json:"createdBy"`
}

// EngagementUpdateRequest is the body of PATCH /engagements/{id}. Only the
// fields sent are changed. FundingSources replace the existing rows when
// non-empty.
type EngagementUpdateRequest struct {
	AllocationTypeID *int                      `json:"allocationTypeId"`
	EngagementCode   *string                   `json:"engagementCode"`
	EngagementScope  *string                   `json:"engagementScope"`
	EngagementNature *string                   `json:"engagementNature"`
	Comment          *string                   `json:"comment"`
	StartDate        *string                   `json:"startDate"`
	EndDate          *string                   `json:"endDate"`
	EngagementStatus *string                   `json:"engagementStatus"`
	RequestStatus    *string                   `json:"requestStatus"`
	Country          *string                   `json:"country"`
	CustomerContact  *string                   `json:"customerContact"`
	Address          *string                   `json:"address"`
	Description      *string                   `json:"description"`
	ProjectName      *string                   `json:"projectName"`
	CampaignID       *string                   `json:"campaignId"`
	TentativeDates   *bool                     `json:"tentativeDates"`
	FundingSources   []EngagementFundingSource `json:"fundingSources"`
	UpdatedBy        string                    `json:"updatedBy"`
}

// EngagementUpdateResponse is returned by PATCH /engagements/{id}.
type EngagementUpdateResponse struct {
	Engagement Engagement `json:"engagement"`
	// CancelledAllocations are the Active allocations read just before a
	// cancellation cascaded to them (for the cancellation email); empty
	// otherwise.
	CancelledAllocations []EngagementAllocation `json:"cancelledAllocations"`
}

// EngagementTravelRef is a travelapp.travel row raised for an allocation.
type EngagementTravelRef struct {
	JobNumber string  `json:"jobNumber"`
	Status    *string `json:"status"`
}

// EngagementAllocation is an allocation of an engagement with the travel
// requests raised for it.
type EngagementAllocation struct {
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

// EngagementAllocationsResponse is returned by GET /engagements/{id}/roster.
type EngagementAllocationsResponse struct {
	Allocations []EngagementAllocation `json:"allocations"`
}
