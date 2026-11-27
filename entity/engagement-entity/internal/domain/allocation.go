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

// Allocation status values (allocation.status) and the other legacy values
// the allocation area writes or reacts to.
const (
	AllocationStatusActive   = "Active"
	AllocationStatusInactive = "Inactive"

	AllocationClearanceConfirmed            = "Confirmed"
	AllocationClearanceConfirmedVisaPending = "Confirmed - Visa Pending"

	// AllocationPartnerEmailPrefix starts every partner consultant's
	// pseudo-email (partner-allocations-group+PARTNER+FIRST+LAST@wso2.com).
	AllocationPartnerEmailPrefix = "partner-allocations-group"
)

// AllocationClearanceStatuses are the legacy clearance values
// (entity types/allocation.bal AllocationClearanceStatus).
var AllocationClearanceStatuses = []string{
	"Tentative", "Ready for Clearance", "Clearance In Progress", "Accepted by Consultant",
	"Rejected by Consultant", "Rejected - Visa Issues", "Rejected - Other", "Confirmed",
	"Confirmed - Visa Pending", "Allocation Cancelled",
}

// AllocationIsDefault reports whether an allocation's engagement_id marks a
// default (team) allocation: "-1" or "-1.0" (legacy isEngagementAllocation,
// negated). An empty id counts as default too.
func AllocationIsDefault(engagementID string) bool {
	return engagementID == "" || engagementID == "-1" || engagementID == "-1.0"
}

// AllocationFundingSource is one engagement_funding_source row resolved to
// names (legacy getFundingSources).
type AllocationFundingSource struct {
	EngagementID string   `json:"-"`
	BusinessUnit *string  `json:"businessUnit"`
	ProductUnit  *string  `json:"productUnit"`
	Region       *string  `json:"region"`
	SubRegion    *string  `json:"subRegion"`
	Percentage   *float64 `json:"percentage"`
}

// AllocationEngagement is what the allocation area reads of an engagement
// (legacy getEngagement, with the allocation type's notification setting and
// the funding sources).
type AllocationEngagement struct {
	ID                 string  `json:"id"`
	EngagementTypeID   int     `json:"engagementTypeId"`
	EngagementTypeName *string `json:"engagementTypeName"`
	// EngagementKind is engagement_type.type: Internal or Customer.
	EngagementKind     *string `json:"engagementKind"`
	AllocationTypeID   *int    `json:"allocationTypeId"`
	AllocationTypeName *string `json:"allocationTypeName"`
	// EmailToAllocations is the allocation type's Notify To Allocations.
	EmailToAllocations *string                   `json:"emailToAllocations"`
	CustomerName       *string                   `json:"customerName"`
	CustomerCode       *string                   `json:"customerCode"`
	Title              *string                   `json:"title"`
	OpportunityID      *string                   `json:"opportunityId"`
	ProductID          *string                   `json:"productId"`
	ProductName        *string                   `json:"productName"`
	Address            *string                   `json:"address"`
	Country            *string                   `json:"country"`
	BusinessDomain     *string                   `json:"businessDomain"`
	CreatedBy          *string                   `json:"createdBy"`
	ContractsSigned    bool                      `json:"contractsSigned"`
	EngagementCode     *string                   `json:"engagementCode"`
	StartDate          *string                   `json:"startDate"`
	EndDate            *string                   `json:"endDate"`
	RequestStatus      *string                   `json:"requestStatus"`
	EngagementStatus   *string                   `json:"engagementStatus"`
	EngagementScope    *string                   `json:"engagementScope"`
	EngagementNature   *string                   `json:"engagementNature"`
	CustomerContact    *string                   `json:"customerContact"`
	OpportunityOwner   *string                   `json:"opportunityOwner"`
	TechnicalOwner     *string                   `json:"technicalOwner"`
	Comment            *string                   `json:"comment"`
	Description        *string                   `json:"description"`
	FundingSources     []AllocationFundingSource `json:"fundingSources"`
}

// AllocationEngagementRef is the engagement side of an engagement
// allocation row.
type AllocationEngagementRef struct {
	ID                 string  `json:"id"`
	EngagementTypeID   int     `json:"engagementTypeId"`
	EngagementTypeName *string `json:"engagementTypeName"`
	EngagementKind     *string `json:"engagementKind"`
	AllocationTypeName *string `json:"allocationTypeName"`
	CustomerName       *string `json:"customerName"`
	Title              *string `json:"title"`
	EngagementCode     *string `json:"engagementCode"`
	Country            *string `json:"country"`
	Address            *string `json:"address"`
	RequestStatus      *string `json:"requestStatus"`
	EngagementStatus   *string `json:"engagementStatus"`
	StartDate          *string `json:"startDate"`
	EndDate            *string `json:"endDate"`
	// ProductID is the opportunity line item id (travel rows copy it).
	ProductID *string `json:"productId"`
}

// AllocationTravelRef is a travelapp.travel row created for an allocation.
type AllocationTravelRef struct {
	JobNumber      string  `json:"jobNumber"`
	AllocationID   string  `json:"allocationId"`
	TravellerEmail string  `json:"travellerEmail"`
	Status         *string `json:"status"`
}

// AllocationRecord is one allocation row: an engagement allocation (with
// Engagement) or a default (team) allocation (Engagement nil).
type AllocationRecord struct {
	ID                  string  `json:"id"`
	Email               string  `json:"email"`
	AllocationTypeID    *int    `json:"allocationTypeId"`
	AllocationTypeName  *string `json:"allocationTypeName"`
	AllocationTypeColor *string `json:"allocationTypeColor"`
	StartDate           string  `json:"startDate"`
	EndDate             string  `json:"endDate"`
	IsRecurring         *string `json:"isRecurring"`
	StartTime           *string `json:"startTime"`
	EndTime             *string `json:"endTime"`
	TimeZone            *string `json:"timeZone"`
	Comment             *string `json:"comment"`
	ClearanceStatus     *string `json:"clearanceStatus"`
	IsTravelRequired    *string `json:"isTravelRequired"`
	Status              *string `json:"status"`
	AddedBy             *string `json:"addedBy"`
	AddedDate           *string `json:"addedDate"`
	// EngagementID is allocation.engagement_id ("-1" / "-1.0" for default
	// allocations).
	EngagementID *string `json:"engagementId"`
	// EngagementCode is allocation.engagement_code (the code the allocation
	// was made with; the type's default code for default allocations).
	EngagementCode *string                  `json:"engagementCode"`
	Engagement     *AllocationEngagementRef `json:"engagement"`
	// TravelRequests are the travel rows of the allocation (search only).
	TravelRequests []AllocationTravelRef `json:"travelRequests,omitempty"`
}

// AllocationTimelineRequest is POST /allocations/timeline: the active
// allocations of people over a window (legacy getAllocationsByEmail).
type AllocationTimelineRequest struct {
	Emails            []string `json:"emails"`
	StartDate         string   `json:"startDate"`
	EndDate           string   `json:"endDate"`
	AllocationTypeIDs []int    `json:"allocationTypeIds"`
}

// AllocationListResponse wraps a list of allocation rows.
type AllocationListResponse struct {
	Allocations []AllocationRecord `json:"allocations"`
}

// AllocationSearchRequest is POST /allocations/search (the Engagement
// Allocations list, legacy allocation-search).
type AllocationSearchRequest struct {
	// EngagementTypeIDs are the engagement types the caller may list; at
	// least one.
	EngagementTypeIDs []int    `json:"engagementTypeIds"`
	ClearanceStatuses []string `json:"clearanceStatuses"`
	Consultants       []string `json:"consultants"`
	// AllocationTypes are allocation type names.
	AllocationTypes []string `json:"allocationTypes"`
	TravelRequired  *bool    `json:"travelRequired"`
	// StartFrom keeps allocations starting on or after it; EndTo those
	// ending on or before it.
	StartFrom string `json:"startFrom"`
	EndTo     string `json:"endTo"`
	Search    string `json:"search"`
	// Today orders current allocations (ending today or later) first.
	Today string `json:"today"`
	// Sort is one column sort (id, consultant, engagement, type, startDate,
	// status, created; a leading "-" means descending). Empty or unknown
	// keeps the current-first order.
	Sort   string `json:"sort"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// AllocationSearchResponse is one page of the list and the total.
type AllocationSearchResponse struct {
	Items []AllocationRecord `json:"items"`
	Total int                `json:"total"`
}

// AllocationInput is one allocation to insert (legacy AllocationInput of
// bulkUploadAllocations).
type AllocationInput struct {
	Email            string  `json:"email"`
	AllocationTypeID int     `json:"allocationTypeId"`
	Status           string  `json:"status"`
	AllocationStatus *string `json:"allocationStatus"`
	ClearanceStatus  *string `json:"clearanceStatus"`
	StartDate        string  `json:"startDate"`
	EndDate          string  `json:"endDate"`
	StartTime        *string `json:"startTime"`
	EndTime          *string `json:"endTime"`
	TimeZone         *string `json:"timeZone"`
	// EngagementID is the engagement id, or "-1" / "-1.0" for a default
	// allocation.
	EngagementID     string  `json:"engagementId"`
	EngagementCode   string  `json:"engagementCode"`
	IsTravelRequired string  `json:"isTravelRequired"`
	IsRecurring      string  `json:"isRecurring"`
	Comment          *string `json:"comment"`
}

// AllocationDateUpdate trims or moves an overlapped default allocation.
type AllocationDateUpdate struct {
	ID        string `json:"id"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// AllocationUploadLog is one allocation_upload_log row (bulk upload).
type AllocationUploadLog struct {
	Email            string `json:"email"`
	AllocationTypeID int    `json:"allocationTypeId"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	EngagementCode   string `json:"engagementCode"`
}

// AllocationBulkRequest is POST /allocations/bulk, the port of
// bulkUploadAllocations: new allocations, date updates of overlapped default
// allocations and upload log rows, in one transaction.
type AllocationBulkRequest struct {
	NewAllocations    []AllocationInput      `json:"newAllocations"`
	UpdateAllocations []AllocationDateUpdate `json:"updateAllocations"`
	AllocationLogs    []AllocationUploadLog  `json:"allocationLogs"`
	// RequestStatusInProgress, when set, is the engagement whose request
	// status becomes "In Progress" in the same transaction (the Allocate
	// page's follow-up PATCH in legacy).
	RequestStatusInProgress *string `json:"requestStatusInProgress"`
	CreatedBy               string  `json:"createdBy"`
	// TravelBudget, when set, is the budget line of the trip this request
	// creates. Absent: no budget row, as before.
	TravelBudget *AllocationTravelBudget `json:"travelBudget"`
}

// AllocationBulkResponse is the outcome of POST /allocations/bulk.
type AllocationBulkResponse struct {
	RecordsAdded   int `json:"recordsAdded"`
	RecordsUpdated int `json:"recordsUpdated"`
	// FirstAllocationID is the id the database gave newAllocations[0].
	FirstAllocationID *string `json:"firstAllocationId"`
	// TravelJobNumber is the travel request created for a confirmed
	// engagement allocation with travel required, if any.
	TravelJobNumber *string `json:"travelJobNumber"`
}

// AllocationUpdateRequest is PATCH /allocations/{id} (legacy
// updateAllocation). Only the fields sent are changed.
type AllocationUpdateRequest struct {
	ClearanceStatus *string `json:"clearanceStatus"`
	Status          *string `json:"status"`
	Comment         *string `json:"comment"`
	StartDate       *string `json:"startDate"`
	EndDate         *string `json:"endDate"`
	UpdatedBy       string  `json:"updatedBy"`
	// TravelBudget is the budget line of the trip this update creates.
	TravelBudget *AllocationTravelBudget `json:"travelBudget"`
}

// AllocationUpdateResponse is the updated allocation and what changed on
// the travel side.
type AllocationUpdateResponse struct {
	Allocation              AllocationRecord `json:"allocation"`
	PreviousClearanceStatus *string          `json:"previousClearanceStatus"`
	TravelJobNumber         *string          `json:"travelJobNumber"`
	TravelCancelled         bool             `json:"travelCancelled"`
}

// AllocationVisa is a visa_status row (Confirmed email travel readiness).
type AllocationVisa struct {
	Country   *string `json:"country"`
	VisaType  *string `json:"visaType"`
	EntryType *string `json:"entryType"`
	Status    *string `json:"status"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// AllocationVisaResponse wraps the visas.
type AllocationVisaResponse struct {
	Visas []AllocationVisa `json:"visas"`
}

// ConsultantAllocationFilter narrows GET /consultant-allocations (legacy
// ConsultantAllocationFilter).
type ConsultantAllocationFilter struct {
	StartDate         string
	EndDate           string
	Email             string
	AllocationTypeIDs []int
	BusinessUnit      string
	Department        string
	Team              string
}

// ConsultantAllocation is one row of the legacy consultantAllocations query.
type ConsultantAllocation struct {
	ID                                string                    `json:"id"`
	Email                             string                    `json:"email"`
	AllocationTypeName                *string                   `json:"allocationTypeName"`
	StartDate                         string                    `json:"startDate"`
	EndDate                           string                    `json:"endDate"`
	EngagementID                      *string                   `json:"engagementId"`
	IsRecurring                       *string                   `json:"isRecurring"`
	StartTime                         *string                   `json:"startTime"`
	EndTime                           *string                   `json:"endTime"`
	TimeZone                          *string                   `json:"timeZone"`
	EngagementCode                    *string                   `json:"engagementCode"`
	EngagementNature                  *string                   `json:"engagementNature"`
	Country                           *string                   `json:"country"`
	InitialEngagementCode             *string                   `json:"initialEngagementCode"`
	DefaultEngagementCodeForAllocType *string                   `json:"defaultEngagementCodeForAllocType"`
	EngagementTypeName                *string                   `json:"engagementTypeName"`
	NumOfDays                         *int                      `json:"numOfDays"`
	Department                        *string                   `json:"department"`
	Team                              *string                   `json:"team"`
	BusinessUnit                      *string                   `json:"businessUnit"`
	Location                          *string                   `json:"location"`
	Company                           *string                   `json:"company"`
	FundingSources                    []AllocationFundingSource `json:"fundingSources"`
}

// ConsultantAllocationsResponse wraps the report rows.
type ConsultantAllocationsResponse struct {
	Allocations []ConsultantAllocation `json:"allocations"`
}

// ConsultantNonAllocatedEmployee is an employee with no allocation in the
// window (legacy nonAllocatedEmployees).
type ConsultantNonAllocatedEmployee struct {
	WorkEmail    *string `json:"workEmail"`
	Team         *string `json:"team"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	JobRole      *string `json:"jobRole"`
	Company      *string `json:"company"`
}

// ConsultantNonAllocatedResponse wraps the employees.
type ConsultantNonAllocatedResponse struct {
	Employees []ConsultantNonAllocatedEmployee `json:"employees"`
}

// ConsultantTeamDefault is one default_allocation_for_team row with its
// allocation and engagement type.
type ConsultantTeamDefault struct {
	BusinessUnit          *string `json:"businessUnit"`
	Department            *string `json:"department"`
	Team                  *string `json:"team"`
	DefaultAllocTypeName  *string `json:"defaultAllocTypeName"`
	EngagementTypeName    *string `json:"engagementTypeName"`
	DefaultEngagementCode *string `json:"defaultEngagementCode"`
}

// ConsultantTeamDefaultsResponse wraps the defaults.
type ConsultantTeamDefaultsResponse struct {
	Defaults []ConsultantTeamDefault `json:"defaults"`
}

// ConsultantEngagementCode is one engcode_grouping_gl_codes row.
type ConsultantEngagementCode struct {
	EngagementCode *string `json:"engagementCode"`
	EngGroup       *string `json:"engGroup"`
	AccountName    *string `json:"accountName"`
	GLCode         *string `json:"glCode"`
}

// ConsultantEngagementCodesResponse wraps the GL code rows, in table order.
type ConsultantEngagementCodesResponse struct {
	Codes []ConsultantEngagementCode `json:"codes"`
}
