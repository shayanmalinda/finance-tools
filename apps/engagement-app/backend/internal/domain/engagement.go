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

// Engagement kinds (the list segments): customer = engagement types 2, 3, 4,
// 6, 7; internal = 1 (Internal Engagement); marketing = 5 (Marketing Event).
const (
	EngagementKindCustomer  = "customer"
	EngagementKindInternal  = "internal"
	EngagementKindMarketing = "marketing"
)

// Engagement type ids (allocation_db.engagement_type).
const (
	EngagementTypeInternal      = 1
	EngagementTypePaid          = 2
	EngagementTypeNonPaidPre    = 3
	EngagementTypeNonPaidPost   = 4
	EngagementTypeMarketing     = 5
	EngagementTypePaidPostSale  = 6
	EngagementTypePaidPreSale   = 7
	EngagementScopeFull         = "full"
	EngagementScopeRoster       = "roster"
	EngagementScopeConsultants  = "consultants"
	EngagementApprovalsInternal = "internal"
	EngagementApprovalsEvents   = "events"
)

// Legacy request statuses (engagement.status).
const (
	EngagementRequestPendingApproval = "Pending Approval"
	EngagementRequestNew             = "New"
	EngagementRequestUpdated         = "Updated"
	EngagementRequestApproved        = "Approved"
	EngagementRequestInProgress      = "In Progress"
	EngagementRequestCompleted       = "Completed"
	EngagementRequestCancelled       = "Cancelled"
)

// Legacy engagement statuses (engagement.engagement_status).
const (
	EngagementStatusPending   = "Pending"
	EngagementStatusCancelled = "Cancelled"
)

// EngagementKindOf maps an engagement type id to its list segment.
func EngagementKindOf(engagementTypeID int) string {
	switch engagementTypeID {
	case EngagementTypeInternal:
		return EngagementKindInternal
	case EngagementTypeMarketing:
		return EngagementKindMarketing
	default:
		return EngagementKindCustomer
	}
}

// EngagementFundingSource is one funding source of an engagement.
type EngagementFundingSource struct {
	Region       string  `json:"region"`
	SubRegion    string  `json:"subRegion"`
	BusinessUnit string  `json:"businessUnit"`
	ProductUnit  string  `json:"productUnit"`
	Percentage   float64 `json:"percentage"`
}

// EngagementActions is what the caller may do on an engagement now.
type EngagementActions struct {
	Allocate               bool `json:"allocate"`
	Edit                   bool `json:"edit"`
	EditPending            bool `json:"editPending"`
	Approve                bool `json:"approve"`
	Complete               bool `json:"complete"`
	ChangeEngagementStatus bool `json:"changeEngagementStatus"`
	ChangeAllocationStatus bool `json:"changeAllocationStatus"`
}

// EngagementSummary is a list row (and the head of the engagement page).
type EngagementSummary struct {
	ID                 string                    `json:"id"`
	Kind               string                    `json:"kind"`
	EngagementTypeID   int                       `json:"engagementTypeId"`
	EngagementTypeName string                    `json:"engagementTypeName"`
	Account            *string                   `json:"account"`
	Title              string                    `json:"title"`
	Campaign           *CampaignRef              `json:"campaign"`
	AllocationType     string                    `json:"allocationType"`
	EngagementCode     string                    `json:"engagementCode"`
	Nature             string                    `json:"nature"`
	City               *string                   `json:"city"`
	Country            string                    `json:"country"`
	BusinessUnit       *string                   `json:"businessUnit"`
	FundingSources     []EngagementFundingSource `json:"fundingSources"`
	StartDate          string                    `json:"startDate"`
	EndDate            string                    `json:"endDate"`
	RequestStatus      string                    `json:"requestStatus"`
	EngagementStatus   string                    `json:"engagementStatus"`
	CreatedBy          *Person                   `json:"createdBy"`
	CreatedOn          *string                   `json:"createdOn"`
	// Consultants is null unless the caller holds Allocations 8 (customer)
	// or 18 (internal, marketing).
	Consultants []Person          `json:"consultants"`
	Actions     EngagementActions `json:"actions"`
}

// EngagementPage is a page of engagements with the count per segment.
type EngagementPage struct {
	Items         []EngagementSummary `json:"items"`
	Total         int                 `json:"total"`
	SegmentCounts map[string]int      `json:"segmentCounts"`
}

// EngagementFacets are the distinct filter values of a segment.
type EngagementFacets struct {
	Accounts        []string `json:"accounts"`
	AllocationTypes []string `json:"allocationTypes"`
	Countries       []string `json:"countries"`
}

// EngagementHourly is the recurring time window of an hourly allocation.
type EngagementHourly struct {
	TimeZone  string `json:"timeZone"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// EngagementTravelLink is the travel request raised for an allocation.
type EngagementTravelLink struct {
	JobNumber string `json:"jobNumber"`
	CanOpen   bool   `json:"canOpen"`
}

// EngagementAllocationActions is what the caller may do on an allocation.
type EngagementAllocationActions struct {
	ChangeStatus bool `json:"changeStatus"`
}

// EngagementAllocation is an allocation on the engagement page. In the
// consultants scope only the consultant and dates are set.
type EngagementAllocation struct {
	ID              string                       `json:"id,omitempty"`
	EngagementID    string                       `json:"engagementId,omitempty"`
	Consultant      Person                       `json:"consultant"`
	Partner         *string                      `json:"partner,omitempty"`
	StartDate       string                       `json:"startDate"`
	EndDate         string                       `json:"endDate"`
	Hourly          *EngagementHourly            `json:"hourly,omitempty"`
	TravelRequired  *bool                        `json:"travelRequired,omitempty"`
	ClearanceStatus *string                      `json:"clearanceStatus,omitempty"`
	Comment         *string                      `json:"comment,omitempty"`
	AddedBy         *string                      `json:"addedBy,omitempty"`
	AddedOn         *string                      `json:"addedOn,omitempty"`
	TravelRequest   *EngagementTravelLink        `json:"travelRequest,omitempty"`
	Actions         *EngagementAllocationActions `json:"actions,omitempty"`
}

// EngagementCascadePreview is what cancelling the engagement would change.
type EngagementCascadePreview struct {
	ActiveAllocations  int `json:"activeAllocations"`
	OpenTravelRequests int `json:"openTravelRequests"`
}

// EngagementDetail is the engagement page.
type EngagementDetail struct {
	EngagementSummary
	Scope                 string                    `json:"scope"`
	AccountID             *string                   `json:"accountId"`
	OpportunityID         *string                   `json:"opportunityId"`
	OpportunityLineItemID *string                   `json:"opportunityLineItemId"`
	Product               *string                   `json:"product"`
	ProductUnit           *string                   `json:"productUnit"`
	SalesSubRegion        *string                   `json:"salesSubRegion"`
	Address               *string                   `json:"address"`
	Description           *string                   `json:"description"`
	CustomerContact       *string                   `json:"customerContact"`
	BusinessDomain        *string                   `json:"businessDomain"`
	OpportunityOwner      *string                   `json:"opportunityOwner"`
	TechnicalOwner        *string                   `json:"technicalOwner"`
	ContractsSigned       *bool                     `json:"contractsSigned"`
	EngagementScope       *string                   `json:"engagementScope"`
	Comments              *string                   `json:"comments"`
	Allocations           []EngagementAllocation    `json:"allocations"`
	CascadePreview        *EngagementCascadePreview `json:"cascadePreview,omitempty"`
	TentativeDates        bool                      `json:"tentativeDates"`
	// BudgetLine is the *EngagementBudgetLine set on the engagement (null
	// when none). It is left out entirely while travel budgets are off.
	BudgetLine any `json:"budgetLine,omitempty"`
}

// EngagementCreate is the body of POST /engagements.
type EngagementCreate struct {
	EngagementTypeID      int                       `json:"engagementTypeId"`
	AccountID             string                    `json:"accountId"`
	OpportunityID         string                    `json:"opportunityId"`
	OpportunityLineItemID string                    `json:"opportunityLineItemId"`
	CampaignID            string                    `json:"campaignId"`
	CampaignName          string                    `json:"campaignName"`
	Title                 string                    `json:"title"`
	Description           string                    `json:"description"`
	AllocationTypeID      int                       `json:"allocationTypeId"`
	Nature                string                    `json:"nature"`
	Country               string                    `json:"country"`
	Address               string                    `json:"address"`
	BusinessUnit          string                    `json:"businessUnit"`
	ProductUnit           string                    `json:"productUnit"`
	FundingSources        []EngagementFundingSource `json:"fundingSources"`
	SalesSubRegion        string                    `json:"salesSubRegion"`
	StartDate             string                    `json:"startDate"`
	EndDate               string                    `json:"endDate"`
	CustomerContact       string                    `json:"customerContact"`
	BusinessDomain        string                    `json:"businessDomain"`
	EngagementScope       string                    `json:"engagementScope"`
	Comments              string                    `json:"comments"`
	// TentativeDates: customer engagements only (legacy checkbox).
	TentativeDates bool `json:"tentativeDates"`
	// BudgetLineCode is the travel budget line of every trip on the
	// engagement (internal engagements and events only; "" sets none).
	// Ignored while travel budgets are off.
	BudgetLineCode *string `json:"budgetLineCode"`
}

// EngagementUpdate is the body of PATCH /engagements/{id}: only the fields
// sent are changed.
type EngagementUpdate struct {
	Nature                     *string                   `json:"nature"`
	Address                    *string                   `json:"address"`
	BusinessUnit               *string                   `json:"businessUnit"`
	ProductUnit                *string                   `json:"productUnit"`
	CustomerContact            *string                   `json:"customerContact"`
	EngagementScope            *string                   `json:"engagementScope"`
	Comments                   *string                   `json:"comments"`
	StartDate                  *string                   `json:"startDate"`
	EndDate                    *string                   `json:"endDate"`
	Description                *string                   `json:"description"`
	FundingSources             []EngagementFundingSource `json:"fundingSources"`
	AllocationTypeID           *int                      `json:"allocationTypeId"`
	Country                    *string                   `json:"country"`
	Title                      *string                   `json:"title"`
	CampaignID                 *string                   `json:"campaignId"`
	CampaignName               *string                   `json:"campaignName"`
	SalesSubRegion             *string                   `json:"salesSubRegion"`
	RefreshDatesFromSalesforce *bool                     `json:"refreshDatesFromSalesforce"`
	Approve                    *bool                     `json:"approve"`
	TentativeDates             *bool                     `json:"tentativeDates"`
	// BudgetLineCode: see EngagementCreate; absent means no change.
	BudgetLineCode *string `json:"budgetLineCode"`
}

// EngagementRequestStatusChange is the body of POST /engagements/{id}/request-status.
type EngagementRequestStatusChange struct {
	Status string `json:"status"`
}

// EngagementStatusChange is the body of POST /engagements/{id}/engagement-status.
type EngagementStatusChange struct {
	Status string `json:"status"`
	// Comment is accepted and not stored: the legacy cancellation has no
	// reason field.
	Comment *string `json:"comment"`
}
