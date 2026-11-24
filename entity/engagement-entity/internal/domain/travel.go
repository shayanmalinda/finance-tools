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

// Travel request statuses as stored in travelapp.travel.status (legacy
// TRAVEL_STATUS). "Deleted" exists in data but no code path sets it.
const (
	TravelStatusNew        = "New"
	TravelStatusInProgress = "In Progress"
	TravelStatusOnHold     = "On Hold"
	TravelStatusExtended   = "Extended"
	TravelStatusCompleted  = "Completed"
	TravelStatusCancelled  = "Cancelled"
)

// TravelStatuses are the statuses a travel request may be set to.
var TravelStatuses = []string{
	TravelStatusNew, TravelStatusInProgress, TravelStatusOnHold,
	TravelStatusExtended, TravelStatusCompleted, TravelStatusCancelled,
}

// Visa and travel document statuses (visa_status.status,
// employee_travel_document.status).
const (
	TravelRecordActive   = "Active"
	TravelRecordInactive = "Inactive"
	TravelRecordExpired  = "Expired"
)

// Travel document types (employee_travel_document.document_type).
const (
	TravelDocPassport      = "Passport"
	TravelDocDriverLicense = "Driver License"
	TravelDocOtherID       = "Other ID Document"
)

// Travel request list sort orders.
const (
	// TravelSortCurrentFirst lists trips ending today or later first (start
	// date ascending), then past trips (start date descending).
	TravelSortCurrentFirst = "current"
	// TravelSortStartDesc lists by start date, newest first.
	TravelSortStartDesc = "startDesc"
)

// TravelRequestSearch filters POST /travel-requests/search. Every multi-value
// filter is OR within itself and AND across filters. Rows with status
// Deleted or NULL are never returned (the legacy get_sorted_all_travel_requests
// view excludes them).
type TravelRequestSearch struct {
	Statuses        []string `json:"statuses"`
	TravellerEmails []string `json:"travellerEmails"`
	Countries       []string `json:"countries"`
	// AgencyNames filters by travel agent name (travel_agents.name_of_travel_agent).
	AgencyNames []string `json:"agencyNames"`
	// TravelAgentID scopes the list to one agency (agents see only theirs).
	TravelAgentID *int `json:"travelAgentId"`
	// VisaID returns the requests that use this visa.
	VisaID *int `json:"visaId"`
	// StartFrom (start date on or after) and EndTo (end date on or before),
	// YYYY-MM-DD.
	StartFrom string `json:"startFrom"`
	EndTo     string `json:"endTo"`
	// Search is a case-insensitive substring of the job number, traveller
	// email, city or country; with SearchEngagements also the account and
	// the engagement title; with SearchAgency also the travel agent name.
	Search            string `json:"search"`
	SearchEngagements bool   `json:"searchEngagements"`
	SearchAgency      bool   `json:"searchAgency"`
	// Sort is TravelSortCurrentFirst (default) or TravelSortStartDesc.
	Sort string `json:"sort"`
	// Today (YYYY-MM-DD) splits current from past trips; empty means the
	// entity's UTC date.
	Today  string `json:"today"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	// WithStatusCounts also returns the number of matches per status,
	// ignoring Statuses (the backend builds its segment counts from it).
	WithStatusCounts bool `json:"withStatusCounts"`

	// EngagementIDs is filled by the service from the engagement title
	// search (allocation_db); never read from the request.
	EngagementIDs []string `json:"-"`
}

// TravelVisaRef is the visa a travel request points at (travel.visa_id).
type TravelVisaRef struct {
	ID        int     `json:"id"`
	Email     *string `json:"email"`
	Country   *string `json:"country"`
	VisaType  *string `json:"visaType"`
	EntryType *string `json:"entryType"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// TravelEngagement is the live engagement of a travel request, read from
// allocation_db through the request's allocation (the legacy
// includeEngagements overlay).
type TravelEngagement struct {
	ID             string  `json:"id"`
	Title          *string `json:"title"`
	CustomerName   *string `json:"customerName"`
	EngagementCode *string `json:"engagementCode"`
	// TypeName is engagement_type.name (e.g. "Marketing Event"), Kind is
	// engagement_type.type ("Internal" or "Customer").
	TypeID   *int    `json:"typeId"`
	TypeName *string `json:"typeName"`
	Kind     *string `json:"kind"`
	// CampaignID is the Salesforce campaign of a Marketing Event.
	CampaignID *string `json:"campaignId"`
}

// TravelFundingSource is one engagement_funding_source row with its names.
type TravelFundingSource struct {
	BusinessUnit *string  `json:"businessUnit"`
	ProductUnit  *string  `json:"productUnit"`
	Region       *string  `json:"region"`
	SubRegion    *string  `json:"subRegion"`
	Percentage   *float64 `json:"percentage"`
}

// TravelRequest is a row of travelapp.travel with its agency and visa.
type TravelRequest struct {
	JobNumber      string  `json:"jobNumber"`
	Status         string  `json:"status"`
	TravellerEmail string  `json:"travellerEmail"`
	City           *string `json:"city"`
	Country        string  `json:"country"`
	CustomerName   string  `json:"customerName"`
	StartDate      string  `json:"startDate"`
	EndDate        string  `json:"endDate"`
	AllocationID   *string `json:"allocationId"`
	// EngagementID, EngagementCode and EngagementType are the values copied
	// into travel when the request was created; Engagement has the live ones.
	EngagementID    *string        `json:"engagementId"`
	EngagementCode  string         `json:"engagementCode"`
	EngagementType  string         `json:"engagementType"`
	TravelAgentID   *int           `json:"travelAgentId"`
	TravelAgentName *string        `json:"travelAgentName"`
	VisaID          *int           `json:"visaId"`
	VisaText        *string        `json:"visa"`
	Visa            *TravelVisaRef `json:"visaDetails"`
	LastUpdatedBy   *string        `json:"lastUpdatedBy"`
	LastUpdatedDate *string        `json:"lastUpdatedDate"`
	// Engagement is nil when the allocation or engagement is not found.
	Engagement *TravelEngagement `json:"engagement"`
}

// TravelRequestPage is returned by POST /travel-requests/search.
type TravelRequestPage struct {
	Items []TravelRequest `json:"items"`
	Total int             `json:"total"`
	// StatusCounts maps a stored status to its number of matches, ignoring
	// the Statuses filter. Only with WithStatusCounts.
	StatusCounts map[string]int `json:"statusCounts,omitempty"`
}

// TravelRequestDetail is returned by GET /travel-requests/{jobNumber}.
type TravelRequestDetail struct {
	TravelRequest
	FundingSources []TravelFundingSource `json:"fundingSources"`
}

// UpdateTravelRequestInput is the body of PATCH /travel-requests/{jobNumber}
// (legacy updateTravel). Nil fields are left unchanged; last_updated_by and
// last_updated_date are always set.
type UpdateTravelRequestInput struct {
	Status        *string `json:"status"`
	TravelAgentID *int    `json:"travelAgentId"`
	VisaID        *int    `json:"visaId"`
	UpdatedBy     string  `json:"updatedBy"`
}

// TravelAgent is a row of travelapp.travel_agents.
type TravelAgent struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Status *string `json:"status"`
}

// TravelAgentList is returned by GET /travel-agencies.
type TravelAgentList struct {
	Items []TravelAgent `json:"items"`
}

// TravelActivePerDiem is an Active perdiem_allocation row, as the legacy
// Completed email lists them. Dates are "YYYY-MM-DD HH:MM:SS".
type TravelActivePerDiem struct {
	StartDate    *string  `json:"startDate"`
	EndDate      *string  `json:"endDate"`
	NoOfDays     *int     `json:"noOfDays"`
	Currency     *string  `json:"currency"`
	PerDiemValue float64  `json:"perDiemValue"`
	Total        *float64 `json:"total"`
}

// TravelActivePerDiemList is returned by GET /travel-requests/{jobNumber}/active-per-diems.
type TravelActivePerDiemList struct {
	Items []TravelActivePerDiem `json:"items"`
}

// TravelEmailAuditInput is one travelapp email_auditlog row (legacy
// insertEmailAuditLog, plus the send status).
type TravelEmailAuditInput = EmailAuditInput

// TravelPassportRef is the passport a visa is tagged with.
type TravelPassportRef struct {
	ID         int     `json:"id"`
	Country    *string `json:"country"`
	DocumentNo *string `json:"documentNo"`
	IssuedDate *string `json:"issuedDate"`
	ExpiryDate *string `json:"expiryDate"`
}

// TravelVisa is a row of travelapp.visa_status. Dates are YYYY-MM-DD.
type TravelVisa struct {
	ID         int                `json:"id"`
	Email      *string            `json:"email"`
	Country    *string            `json:"country"`
	VisaType   *string            `json:"visaType"`
	EntryType  *string            `json:"entryType"`
	StartDate  *string            `json:"startDate"`
	EndDate    *string            `json:"endDate"`
	IssuedDate *string            `json:"issuedDate"`
	Status     *string            `json:"status"`
	ImageID    *string            `json:"imageId"`
	PassportID *int               `json:"passportId"`
	Passport   *TravelPassportRef `json:"passport"`
	AddedBy    *string            `json:"addedBy"`
	UpdatedBy  *string            `json:"updatedBy"`
}

// Validity filters, computed from the end (visa) or expiry (document) date
// against Today, as the legacy lists did.
const (
	TravelValidityActive  = "active"
	TravelValidityExpired = "expired"
)

// TravelVisaFilter narrows GET /visas.
type TravelVisaFilter struct {
	Emails    []string
	Countries []string
	// Status is the stored status (e.g. Active).
	Status string
	// Validity is TravelValidityActive (end date today or later) or
	// TravelValidityExpired.
	Validity string
	// IncludeMarkedExpired also returns the visas whose stored status is
	// Expired (marked expired by hand), whatever their end date.
	IncludeMarkedExpired bool
	Today                string
	// CoversFrom and CoversTo keep visas valid for the whole period
	// (start date on or before CoversFrom, end date on or after CoversTo).
	CoversFrom string
	CoversTo   string
	// Search matches email, country, visa type and entry type.
	Search string
	// ExcludeIDs leaves out these visa_status rows (the placeholder visas).
	ExcludeIDs []int
	Limit      int
	Offset     int
}

// TravelVisaPage is returned by GET /visas.
type TravelVisaPage struct {
	Items []TravelVisa `json:"items"`
	Total int          `json:"total"`
}

// CreateTravelVisaInput is the body of POST /visas. An Active visa of the
// same email and country is set Inactive first (legacy override rule).
type CreateTravelVisaInput struct {
	Email      string `json:"email"`
	Country    string `json:"country"`
	VisaType   string `json:"visaType"`
	EntryType  string `json:"entryType"`
	IssuedDate string `json:"issuedDate"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	PassportID *int   `json:"passportId"`
	ImageID    string `json:"imageId"`
	CreatedBy  string `json:"createdBy"`
}

// UpdateTravelVisaInput is the body of PUT /visas/{id}. ImageID nil keeps
// the image.
type UpdateTravelVisaInput struct {
	Country    string  `json:"country"`
	VisaType   string  `json:"visaType"`
	EntryType  string  `json:"entryType"`
	IssuedDate string  `json:"issuedDate"`
	StartDate  string  `json:"startDate"`
	EndDate    string  `json:"endDate"`
	PassportID *int    `json:"passportId"`
	Status     string  `json:"status"`
	ImageID    *string `json:"imageId"`
	UpdatedBy  string  `json:"updatedBy"`
}

// TravelDocument is a row of travelapp.employee_travel_document.
type TravelDocument struct {
	ID            int     `json:"id"`
	Email         *string `json:"email"`
	DocumentNo    *string `json:"documentNo"`
	DocumentType  *string `json:"documentType"`
	IssuedCountry *string `json:"issuedCountry"`
	IssuedDate    *string `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate"`
	Status        string  `json:"status"`
	ImageID       *string `json:"imageId"`
	AddedBy       *string `json:"addedBy"`
	UpdatedBy     *string `json:"updatedBy"`
}

// TravelDocumentFilter narrows GET /travel-documents.
type TravelDocumentFilter struct {
	Emails        []string
	Countries     []string
	DocumentTypes []string
	Status        string
	// Validity: active = no expiry or expiry today or later.
	Validity string
	Today    string
	// Search matches email, document number, type and country.
	Search string
	Limit  int
	Offset int
}

// TravelDocumentPage is returned by GET /travel-documents.
type TravelDocumentPage struct {
	Items []TravelDocument `json:"items"`
	Total int              `json:"total"`
}

// CreateTravelDocumentInput is the body of POST /travel-documents. An Active
// document of the same email, type and country is set Inactive first.
type CreateTravelDocumentInput struct {
	Email         string  `json:"email"`
	DocumentNo    string  `json:"documentNo"`
	DocumentType  string  `json:"documentType"`
	IssuedCountry string  `json:"issuedCountry"`
	IssuedDate    string  `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate"`
	ImageID       string  `json:"imageId"`
	CreatedBy     string  `json:"createdBy"`
}

// UpdateTravelDocumentInput is the body of PUT /travel-documents/{id}.
// DocumentNo and ImageID nil keep the stored value.
type UpdateTravelDocumentInput struct {
	DocumentNo    *string `json:"documentNo"`
	DocumentType  string  `json:"documentType"`
	IssuedCountry string  `json:"issuedCountry"`
	IssuedDate    string  `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate"`
	Status        string  `json:"status"`
	ImageID       *string `json:"imageId"`
	UpdatedBy     string  `json:"updatedBy"`
}
