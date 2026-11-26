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

// AllocationAPI is what the allocation screen area needs from the Engagement Entity.
// The area owns this file: its methods, request and response types, and the
// HTTPClient implementations live here.
type AllocationAPI interface {
	// AllocationEngagement returns the engagement an allocation is made on
	// (404 when there is none).
	AllocationEngagement(ctx context.Context, id string) (AllocationEngagement, error)
	// Allocation returns one allocation in any status (404 when none).
	Allocation(ctx context.Context, id string) (AllocationRecord, error)
	// AllocationTimeline returns the people's Active allocations touching a window.
	AllocationTimeline(ctx context.Context, req AllocationTimelineRequest) ([]AllocationRecord, error)
	// SearchAllocations returns one page of the Engagement Allocations list.
	SearchAllocations(ctx context.Context, req AllocationSearchRequest) (AllocationSearchResult, error)
	// BulkAllocations inserts and updates allocations in one transaction
	// (409 for an identical active engagement allocation).
	BulkAllocations(ctx context.Context, req AllocationBulkRequest) (AllocationBulkResult, error)
	// UpdateAllocation changes an allocation and its travel row.
	UpdateAllocation(ctx context.Context, id string, req AllocationUpdateRequest) (AllocationUpdateResult, error)
	// DeleteAllocation sets the allocation's status to Deleted.
	DeleteAllocation(ctx context.Context, id string) error
	// AllocationVisas returns the person's visas valid over the dates.
	AllocationVisas(ctx context.Context, email, startDate, endDate string) ([]AllocationVisa, error)
	// ConsultantAllocations is the Consultant Allocations report query.
	ConsultantAllocations(ctx context.Context, f ConsultantAllocationFilter) ([]ConsultantAllocation, error)
	// ConsultantNonAllocated are employees with no allocation in the window.
	ConsultantNonAllocated(ctx context.Context, f ConsultantAllocationFilter) ([]ConsultantNonAllocatedEmployee, error)
	// ConsultantTeamDefaults are the default allocation types per team.
	ConsultantTeamDefaults(ctx context.Context) ([]ConsultantTeamDefault, error)
	// ConsultantEngagementCodes are the engagement code groups and GL codes, in table order.
	ConsultantEngagementCodes(ctx context.Context) ([]ConsultantEngagementCode, error)
}

// AllocationFundingSource is an engagement funding source, by name.
type AllocationFundingSource struct {
	BusinessUnit *string  `json:"businessUnit"`
	ProductUnit  *string  `json:"productUnit"`
	Region       *string  `json:"region"`
	SubRegion    *string  `json:"subRegion"`
	Percentage   *float64 `json:"percentage"`
}

// AllocationEngagement is the engagement an allocation is made on.
type AllocationEngagement struct {
	ID                 string                    `json:"id"`
	EngagementTypeID   int                       `json:"engagementTypeId"`
	EngagementTypeName *string                   `json:"engagementTypeName"`
	EngagementKind     *string                   `json:"engagementKind"`
	AllocationTypeID   *int                      `json:"allocationTypeId"`
	AllocationTypeName *string                   `json:"allocationTypeName"`
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

// AllocationEngagementRef is the engagement side of an engagement allocation.
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
	ProductID          *string `json:"productId"`
}

// AllocationTravelRef is a travel row created for an allocation.
type AllocationTravelRef struct {
	JobNumber      string  `json:"jobNumber"`
	AllocationID   string  `json:"allocationId"`
	TravellerEmail string  `json:"travellerEmail"`
	Status         *string `json:"status"`
}

// AllocationRecord is an engagement allocation (Engagement set) or a
// default (team) allocation (Engagement nil).
type AllocationRecord struct {
	ID                  string                   `json:"id"`
	Email               string                   `json:"email"`
	AllocationTypeID    *int                     `json:"allocationTypeId"`
	AllocationTypeName  *string                  `json:"allocationTypeName"`
	AllocationTypeColor *string                  `json:"allocationTypeColor"`
	StartDate           string                   `json:"startDate"`
	EndDate             string                   `json:"endDate"`
	IsRecurring         *string                  `json:"isRecurring"`
	StartTime           *string                  `json:"startTime"`
	EndTime             *string                  `json:"endTime"`
	TimeZone            *string                  `json:"timeZone"`
	Comment             *string                  `json:"comment"`
	ClearanceStatus     *string                  `json:"clearanceStatus"`
	IsTravelRequired    *string                  `json:"isTravelRequired"`
	Status              *string                  `json:"status"`
	AddedBy             *string                  `json:"addedBy"`
	AddedDate           *string                  `json:"addedDate"`
	EngagementID        *string                  `json:"engagementId"`
	EngagementCode      *string                  `json:"engagementCode"`
	Engagement          *AllocationEngagementRef `json:"engagement"`
	TravelRequests      []AllocationTravelRef    `json:"travelRequests,omitempty"`
}

// AllocationTimelineRequest asks for the people's allocations over a window.
type AllocationTimelineRequest struct {
	Emails            []string `json:"emails"`
	StartDate         string   `json:"startDate"`
	EndDate           string   `json:"endDate"`
	AllocationTypeIDs []int    `json:"allocationTypeIds"`
}

// AllocationSearchRequest is the Engagement Allocations list query.
type AllocationSearchRequest struct {
	EngagementTypeIDs []int    `json:"engagementTypeIds"`
	ClearanceStatuses []string `json:"clearanceStatuses"`
	Consultants       []string `json:"consultants"`
	AllocationTypes   []string `json:"allocationTypes"`
	TravelRequired    *bool    `json:"travelRequired"`
	StartFrom         string   `json:"startFrom"`
	EndTo             string   `json:"endTo"`
	Search            string   `json:"search"`
	Today             string   `json:"today"`
	Sort              string   `json:"sort,omitempty"`
	Limit             int      `json:"limit"`
	Offset            int      `json:"offset"`
}

// AllocationSearchResult is one page and the total.
type AllocationSearchResult struct {
	Items []AllocationRecord `json:"items"`
	Total int                `json:"total"`
}

// AllocationInput is one allocation to insert.
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

// AllocationUploadLog is an allocation_upload_log row.
type AllocationUploadLog struct {
	Email            string `json:"email"`
	AllocationTypeID int    `json:"allocationTypeId"`
	StartDate        string `json:"startDate"`
	EndDate          string `json:"endDate"`
	EngagementCode   string `json:"engagementCode"`
}

// AllocationBulkRequest is the legacy bulkUploadAllocations payload.
type AllocationBulkRequest struct {
	NewAllocations          []AllocationInput      `json:"newAllocations"`
	UpdateAllocations       []AllocationDateUpdate `json:"updateAllocations"`
	AllocationLogs          []AllocationUploadLog  `json:"allocationLogs"`
	RequestStatusInProgress *string                `json:"requestStatusInProgress"`
	CreatedBy               string                 `json:"createdBy"`
}

// AllocationBulkResult is the outcome of a bulk write.
type AllocationBulkResult struct {
	RecordsAdded      int     `json:"recordsAdded"`
	RecordsUpdated    int     `json:"recordsUpdated"`
	FirstAllocationID *string `json:"firstAllocationId"`
	TravelJobNumber   *string `json:"travelJobNumber"`
}

// AllocationUpdateRequest changes an allocation; only fields sent change.
type AllocationUpdateRequest struct {
	ClearanceStatus *string `json:"clearanceStatus,omitempty"`
	Status          *string `json:"status,omitempty"`
	Comment         *string `json:"comment,omitempty"`
	StartDate       *string `json:"startDate,omitempty"`
	EndDate         *string `json:"endDate,omitempty"`
	UpdatedBy       string  `json:"updatedBy"`
}

// AllocationUpdateResult is the updated allocation and its travel outcome.
type AllocationUpdateResult struct {
	Allocation              AllocationRecord `json:"allocation"`
	PreviousClearanceStatus *string          `json:"previousClearanceStatus"`
	TravelJobNumber         *string          `json:"travelJobNumber"`
	TravelCancelled         bool             `json:"travelCancelled"`
}

// AllocationVisa is a visa of the consultant.
type AllocationVisa struct {
	Country   *string `json:"country"`
	VisaType  *string `json:"visaType"`
	EntryType *string `json:"entryType"`
	Status    *string `json:"status"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// ConsultantAllocationFilter narrows the report queries.
type ConsultantAllocationFilter struct {
	StartDate         string
	EndDate           string
	Email             string
	AllocationTypeIDs []int
	BusinessUnit      string
	Department        string
	Team              string
}

// ConsultantAllocation is one row of the legacy report query.
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

// ConsultantNonAllocatedEmployee is an employee without allocations.
type ConsultantNonAllocatedEmployee struct {
	WorkEmail    *string `json:"workEmail"`
	Team         *string `json:"team"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	JobRole      *string `json:"jobRole"`
	Company      *string `json:"company"`
}

// ConsultantTeamDefault is a team's default allocation type and code.
type ConsultantTeamDefault struct {
	BusinessUnit          *string `json:"businessUnit"`
	Department            *string `json:"department"`
	Team                  *string `json:"team"`
	DefaultAllocTypeName  *string `json:"defaultAllocTypeName"`
	EngagementTypeName    *string `json:"engagementTypeName"`
	DefaultEngagementCode *string `json:"defaultEngagementCode"`
}

// ConsultantEngagementCode is an engcode_grouping_gl_codes row.
type ConsultantEngagementCode struct {
	EngagementCode *string `json:"engagementCode"`
	EngGroup       *string `json:"engGroup"`
	AccountName    *string `json:"accountName"`
	GLCode         *string `json:"glCode"`
}

// AllocationEngagement calls GET /allocations/engagements/{id}.
func (c *HTTPClient) AllocationEngagement(ctx context.Context, id string) (AllocationEngagement, error) {
	var out AllocationEngagement
	err := c.rest.Get(ctx, "/allocations/engagements/"+pathSegment(id), &out)
	return out, err
}

// Allocation calls GET /allocations/{id}.
func (c *HTTPClient) Allocation(ctx context.Context, id string) (AllocationRecord, error) {
	var out AllocationRecord
	err := c.rest.Get(ctx, "/allocations/"+pathSegment(id), &out)
	return out, err
}

// AllocationTimeline calls POST /allocations/timeline.
func (c *HTTPClient) AllocationTimeline(ctx context.Context, req AllocationTimelineRequest) ([]AllocationRecord, error) {
	var out struct {
		Allocations []AllocationRecord `json:"allocations"`
	}
	if err := c.rest.Post(ctx, "/allocations/timeline", req, &out); err != nil {
		return nil, err
	}
	return out.Allocations, nil
}

// SearchAllocations calls POST /allocations/search.
func (c *HTTPClient) SearchAllocations(ctx context.Context, req AllocationSearchRequest) (AllocationSearchResult, error) {
	var out AllocationSearchResult
	err := c.rest.Post(ctx, "/allocations/search", req, &out)
	return out, err
}

// BulkAllocations calls POST /allocations/bulk.
func (c *HTTPClient) BulkAllocations(ctx context.Context, req AllocationBulkRequest) (AllocationBulkResult, error) {
	var out AllocationBulkResult
	err := c.rest.Post(ctx, "/allocations/bulk", req, &out)
	return out, err
}

// UpdateAllocation calls PATCH /allocations/{id}.
func (c *HTTPClient) UpdateAllocation(ctx context.Context, id string, req AllocationUpdateRequest) (AllocationUpdateResult, error) {
	var out AllocationUpdateResult
	err := c.rest.Do(ctx, http.MethodPatch, "/allocations/"+pathSegment(id), req, &out)
	return out, err
}

// DeleteAllocation calls DELETE /allocations/{id}.
func (c *HTTPClient) DeleteAllocation(ctx context.Context, id string) error {
	return c.rest.Do(ctx, http.MethodDelete, "/allocations/"+pathSegment(id), nil, nil)
}

// AllocationVisas calls GET /allocations/visas.
func (c *HTTPClient) AllocationVisas(ctx context.Context, email, startDate, endDate string) ([]AllocationVisa, error) {
	var out struct {
		Visas []AllocationVisa `json:"visas"`
	}
	q := url.Values{"email": {email}, "startDate": {startDate}, "endDate": {endDate}}
	if err := c.rest.Get(ctx, withQuery("/allocations/visas", q), &out); err != nil {
		return nil, err
	}
	return out.Visas, nil
}

func consultantQuery(f ConsultantAllocationFilter) url.Values {
	q := url.Values{"startDate": {f.StartDate}, "endDate": {f.EndDate}}
	for k, v := range map[string]string{"email": f.Email, "businessUnit": f.BusinessUnit, "department": f.Department, "team": f.Team} {
		if v != "" {
			q.Set(k, v)
		}
	}
	for _, id := range f.AllocationTypeIDs {
		q.Add("allocationTypeId", strconv.Itoa(id))
	}
	return q
}

// ConsultantAllocations calls GET /consultant-allocations.
func (c *HTTPClient) ConsultantAllocations(ctx context.Context, f ConsultantAllocationFilter) ([]ConsultantAllocation, error) {
	var out struct {
		Allocations []ConsultantAllocation `json:"allocations"`
	}
	if err := c.rest.Get(ctx, withQuery("/consultant-allocations", consultantQuery(f)), &out); err != nil {
		return nil, err
	}
	return out.Allocations, nil
}

// ConsultantNonAllocated calls GET /consultant-allocations/non-allocated.
func (c *HTTPClient) ConsultantNonAllocated(ctx context.Context, f ConsultantAllocationFilter) ([]ConsultantNonAllocatedEmployee, error) {
	var out struct {
		Employees []ConsultantNonAllocatedEmployee `json:"employees"`
	}
	if err := c.rest.Get(ctx, withQuery("/consultant-allocations/non-allocated", consultantQuery(f)), &out); err != nil {
		return nil, err
	}
	return out.Employees, nil
}

// ConsultantTeamDefaults calls GET /consultant-allocations/team-defaults.
func (c *HTTPClient) ConsultantTeamDefaults(ctx context.Context) ([]ConsultantTeamDefault, error) {
	var out struct {
		Defaults []ConsultantTeamDefault `json:"defaults"`
	}
	if err := c.rest.Get(ctx, "/consultant-allocations/team-defaults", &out); err != nil {
		return nil, err
	}
	return out.Defaults, nil
}

// ConsultantEngagementCodes calls GET /consultant-allocations/engagement-codes.
func (c *HTTPClient) ConsultantEngagementCodes(ctx context.Context) ([]ConsultantEngagementCode, error) {
	var out struct {
		Codes []ConsultantEngagementCode `json:"codes"`
	}
	if err := c.rest.Get(ctx, "/consultant-allocations/engagement-codes", &out); err != nil {
		return nil, err
	}
	return out.Codes, nil
}
