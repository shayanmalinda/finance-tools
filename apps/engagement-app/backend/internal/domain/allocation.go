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

// Allocation clearance statuses (legacy values, unchanged).
const (
	AllocationClearanceTentative            = "Tentative"
	AllocationClearanceReadyForClearance    = "Ready for Clearance"
	AllocationClearanceInProgress           = "Clearance In Progress"
	AllocationClearanceAcceptedByConsultant = "Accepted by Consultant"
	AllocationClearanceRejectedByConsultant = "Rejected by Consultant"
	AllocationClearanceRejectedVisaIssues   = "Rejected - Visa Issues"
	AllocationClearanceRejectedOther        = "Rejected - Other"
	AllocationClearanceConfirmed            = "Confirmed"
	AllocationClearanceConfirmedVisaPending = "Confirmed - Visa Pending"
	AllocationClearanceAllocationCancelled  = "Allocation Cancelled"
)

// AllocationHourly is the recurring time window of an hourly allocation.
type AllocationHourly struct {
	TimeZone  string `json:"timeZone"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// AllocationActions are what the caller may do on an allocation now.
type AllocationActions struct {
	ChangeStatus bool `json:"changeStatus"`
}

// AllocationTravelRequest is the travel request raised for an allocation.
type AllocationTravelRequest struct {
	JobNumber string `json:"jobNumber"`
	CanOpen   bool   `json:"canOpen"`
}

// Allocation is an engagement allocation (openapi Allocation).
type Allocation struct {
	ID              string                   `json:"id"`
	EngagementID    string                   `json:"engagementId"`
	Consultant      Person                   `json:"consultant"`
	Partner         *string                  `json:"partner"`
	StartDate       string                   `json:"startDate"`
	EndDate         string                   `json:"endDate"`
	Hourly          *AllocationHourly        `json:"hourly"`
	TravelRequired  bool                     `json:"travelRequired"`
	ClearanceStatus *string                  `json:"clearanceStatus"`
	Comment         *string                  `json:"comment"`
	AddedBy         *string                  `json:"addedBy"`
	AddedOn         *string                  `json:"addedOn"`
	TravelRequest   *AllocationTravelRequest `json:"travelRequest"`
	Actions         AllocationActions        `json:"actions"`
}

// AllocationEngagementSummary is the engagement of an allocation list row.
type AllocationEngagementSummary struct {
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	EngagementTypeID int     `json:"engagementTypeId"`
	Title            string  `json:"title"`
	Account          *string `json:"account"`
	AllocationType   string  `json:"allocationType"`
	EngagementCode   string  `json:"engagementCode"`
	Country          *string `json:"country"`
	Address          *string `json:"address"`
	CanOpen          bool    `json:"canOpen"`
}

// AllocationWithEngagement is a row of the Engagement Allocations list.
type AllocationWithEngagement struct {
	Allocation
	Engagement AllocationEngagementSummary `json:"engagement"`
}

// AllocationListFilter is GET /allocations.
type AllocationListFilter struct {
	Search            string
	Kinds             []string
	EngagementTypeIDs []int
	ClearanceStatuses []string
	Consultants       []string
	AllocationTypes   []string
	TravelRequired    *bool
	StartFrom         string
	EndTo             string
	// Sort is one of AllocationSorts; empty or unknown keeps the
	// current-first order.
	Sort   string
	Limit  int
	Offset int
}

// AllocationCreateRequest is POST /engagements/{id}/allocations (Add New
// Allocation).
type AllocationCreateRequest struct {
	ConsultantEmail        *string           `json:"consultantEmail"`
	StartDate              *string           `json:"startDate"`
	EndDate                *string           `json:"endDate"`
	Hourly                 *AllocationHourly `json:"hourly"`
	ClearanceStatus        *string           `json:"clearanceStatus"`
	Tentative              *bool             `json:"tentative"`
	TravelRequired         *bool             `json:"travelRequired"`
	Comment                *string           `json:"comment"`
	PreparationStart       *string           `json:"preparationStart"`
	PostEngagementEnd      *string           `json:"postEngagementEnd"`
	TrimDefaultAllocations *bool             `json:"trimDefaultAllocations"`
	// BudgetLineID is the travel budget line the allocator chose; ignored
	// while travel budgets are off or no trip is created.
	BudgetLineID *int `json:"budgetLineId"`
}

// AllocationCreated is the result of an allocation or a status change.
type AllocationCreated struct {
	Allocation    Allocation `json:"allocation"`
	TravelRequest *string    `json:"travelRequest"`
	EmailsSent    bool       `json:"emailsSent"`
	// Budget is the trip's *TripBudgetView (null when no trip was created).
	// It is left out entirely while travel budgets are off.
	Budget any `json:"budget,omitempty"`
}

// AllocationStatusUpdate is PATCH /allocations/{id}/status.
type AllocationStatusUpdate struct {
	ClearanceStatus *string `json:"clearanceStatus"`
	Comment         *string `json:"comment"`
	// BudgetLineID: see AllocationCreateRequest.
	BudgetLineID *int `json:"budgetLineId"`
}

// AllocationEmployee is a person row of a timeline (openapi Employee).
type AllocationEmployee struct {
	Email        string  `json:"email"`
	Name         *string `json:"name"`
	EmployeeID   *string `json:"employeeId"`
	JobRole      *string `json:"jobRole"`
	JobBand      *string `json:"jobBand"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	Team         *string `json:"team"`
	Company      *string `json:"company"`
	Location     *string `json:"location"`
	ManagerEmail *string `json:"managerEmail"`
}

// AllocationTimelineItem is a bar of a timeline (openapi TimelineItem).
type AllocationTimelineItem struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	EngagementID    *string           `json:"engagementId"`
	Label           string            `json:"label"`
	AllocationType  string            `json:"allocationType"`
	EngagementCode  string            `json:"engagementCode"`
	Color           *string           `json:"color"`
	StartDate       string            `json:"startDate"`
	EndDate         string            `json:"endDate"`
	Hourly          *AllocationHourly `json:"hourly"`
	ClearanceStatus string            `json:"clearanceStatus"`
	Comment         *string           `json:"comment"`
	Editable        bool              `json:"editable"`
}

// AllocationTimelineRow is one person and their bars (openapi TimelineRow).
type AllocationTimelineRow struct {
	Person AllocationEmployee       `json:"person"`
	Items  []AllocationTimelineItem `json:"items"`
}

// AllocationTimelineResponse is GET /allocation-timeline and GET
// /team-allocations.
type AllocationTimelineResponse struct {
	People []AllocationTimelineRow `json:"people"`
}

// AllocationTimelineFilter selects the people of a timeline: emails, or an
// org unit (business unit / department / team, from the HR Entity).
type AllocationTimelineFilter struct {
	Emails            []string
	BusinessUnit      string
	Department        string
	Team              string
	From              string
	To                string
	AllocationTypeIDs []int
}

// AllocationOverlapError is the 409 body of an overlap (openapi
// OverlapError).
type AllocationOverlapError struct {
	Code            int                      `json:"code"`
	Message         string                   `json:"message"`
	Overlaps        []AllocationTimelineItem `json:"overlaps,omitempty"`
	DefaultOverlaps []AllocationTimelineItem `json:"defaultOverlaps,omitempty"`
}

func (e *AllocationOverlapError) Error() string { return e.Message }

// TeamAllocationCreate is POST /team-allocations.
type TeamAllocationCreate struct {
	Email            string  `json:"email"`
	AllocationTypeID int     `json:"allocationTypeId"`
	EngagementCode   *string `json:"engagementCode"`
	StartDate        string  `json:"startDate"`
	EndDate          string  `json:"endDate"`
	Comment          *string `json:"comment"`
	// TrimDefaultAllocations is accepted and ignored: the Team Allocations
	// page refuses any overlap (legacy), it never trims.
	TrimDefaultAllocations *bool `json:"trimDefaultAllocations"`
}

// TeamAllocationUpdate is PATCH /team-allocations/{id}.
type TeamAllocationUpdate struct {
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
	Comment   *string `json:"comment"`
}

// TeamAllocationBulkRequest is the bulk upload CSV.
type TeamAllocationBulkRequest struct {
	CSV string `json:"csv"`
}

// TeamAllocationBulkRow is a CSV row ready to upload.
type TeamAllocationBulkRow struct {
	Line           int    `json:"line"`
	Email          string `json:"email"`
	Team           string `json:"team"`
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
	AllocationType string `json:"allocationType"`
}

// TeamAllocationBulkResult is the validation (and upload) outcome
// (openapi BulkValidation).
type TeamAllocationBulkResult struct {
	Errors []string                `json:"errors"`
	Valid  []TeamAllocationBulkRow `json:"valid"`
	Saved  *int                    `json:"saved,omitempty"`
}

// ConsultantReportFilter is GET /reports/consultant-allocations.
type ConsultantReportFilter struct {
	From              string
	To                string
	Consultant        string
	AllocationTypeIDs []int
	BusinessUnit      string
	Department        string
	Team              string
}

// ConsultantReportRow is one row of the Consultant Allocations report
// (openapi ConsultantAllocationRow; the 18 legacy CSV columns).
type ConsultantReportRow struct {
	EngagementType         string   `json:"engagementType"`
	EngagementLocation     string   `json:"engagementLocation"`
	Country                string   `json:"country"`
	AllocationType         string   `json:"allocationType"`
	ConsultantEmail        string   `json:"consultantEmail"`
	Department             string   `json:"department"`
	EmployeeBusinessUnit   string   `json:"employeeBusinessUnit"`
	Company                string   `json:"company"`
	EngagementCode         string   `json:"engagementCode"`
	DailyOrHourly          string   `json:"dailyOrHourly"`
	CalendarDays           float64  `json:"calendarDays"`
	WorkingDays            float64  `json:"workingDays"`
	WorkingHours           *float64 `json:"workingHours"`
	EngagementBusinessUnit string   `json:"engagementBusinessUnit"`
	EngagementSalesPod     string   `json:"engagementSalesPod"`
	EngagementGroup        string   `json:"engagementGroup"`
	AccountName            string   `json:"accountName"`
	GLCode                 string   `json:"glCode"`
}

// ConsultantReport is the report response.
type ConsultantReport struct {
	Items []ConsultantReportRow `json:"items"`
}
