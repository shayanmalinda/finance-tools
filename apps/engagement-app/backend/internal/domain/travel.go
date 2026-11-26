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

// Travel request statuses (legacy TRAVEL_STATUS), stored as is.
const (
	TravelStatusNew        = "New"
	TravelStatusInProgress = "In Progress"
	TravelStatusOnHold     = "On Hold"
	TravelStatusExtended   = "Extended"
	TravelStatusCompleted  = "Completed"
	TravelStatusCancelled  = "Cancelled"
)

// TravelStatuses lists every status a travel request can be set to.
var TravelStatuses = []string{
	TravelStatusNew, TravelStatusInProgress, TravelStatusOnHold,
	TravelStatusExtended, TravelStatusCompleted, TravelStatusCancelled,
}

// Travel request scopes (who sees what on the travel request page).
const (
	// TravelScopeFull: Travel 1, 2 or 5 (Travel Team, finance).
	TravelScopeFull = "full"
	// TravelScopeAgency: an agent of the request's assigned agency.
	TravelScopeAgency = "agency"
	// TravelScopeOwn: the traveller (Travel 36), read-only.
	TravelScopeOwn = "own"
)

// Travel request list segments.
const (
	TravelSegmentNew        = "new"
	TravelSegmentInProgress = "in-progress"
	TravelSegmentCompleted  = "completed"
	TravelSegmentAll        = "all"
)

// The two legacy visa sentinels: real visa_status rows a request's visa_id
// points at when no visa applies.
const (
	TravelVisaNotApplicable = "Visa Not Applicable"
	TravelNoValidVisa       = "No Valid Visa Available"
)

// TravelRequestActions says what the caller may do on a travel request now
// (legacy privileges and the request's status).
type TravelRequestActions struct {
	Assign            bool `json:"assign"`
	Update            bool `json:"update"`
	EditStatus        bool `json:"editStatus"`
	EditAgency        bool `json:"editAgency"`
	EditVisa          bool `json:"editVisa"`
	AddPerDiem        bool `json:"addPerDiem"`
	DeletePerDiem     bool `json:"deletePerDiem"`
	AddInvoice        bool `json:"addInvoice"`
	AddTravelCard     bool `json:"addTravelCard"`
	AddPayment        bool `json:"addPayment"`
	EditPayment       bool `json:"editPayment"`
	PushPayments      bool `json:"pushPayments"`
	ApproveInvoicesPo bool `json:"approveInvoicesPo"`
	ApproveInvoices   bool `json:"approveInvoices"`
	PushInvoices      bool `json:"pushInvoices"`
}

// TravelRequestTabs says which tabs of the travel request page the caller
// may open (Travel Information is always shown).
type TravelRequestTabs struct {
	PerDiems    bool `json:"perDiems"`
	Invoices    bool `json:"invoices"`
	TravelCards bool `json:"travelCards"`
	Payments    bool `json:"payments"`
	// Expenses is the card spend and expense claims tab (Travel 45, full
	// scope only: never agency staff or the traveller).
	Expenses bool `json:"expenses"`
}

// TravelEngagementRef is the engagement shown with a travel request.
type TravelEngagementRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Account is the customer of a customer engagement, else null.
	Account            *string `json:"account"`
	EngagementTypeID   *int    `json:"engagementTypeId"`
	EngagementTypeName *string `json:"engagementTypeName"`
}

// TravelRequestSummary is a row of the travel request lists.
type TravelRequestSummary struct {
	JobNumber    string  `json:"jobNumber"`
	Status       string  `json:"status"`
	Traveller    Person  `json:"traveller"`
	BaseLocation *string `json:"baseLocation"`
	City         *string `json:"city"`
	Country      *string `json:"country"`
	StartDate    string  `json:"startDate"`
	EndDate      string  `json:"endDate"`
	// Agency is the travel agent's name; null for agents.
	Agency *string `json:"agency"`
	// Visa is "VisaType (EntryType)", a sentinel, or the legacy free text.
	Visa         *string              `json:"visa"`
	VisaValidity *string              `json:"visaValidity"`
	Engagement   *TravelEngagementRef `json:"engagement"`
	Campaign     *CampaignRef         `json:"campaign"`
	Actions      TravelRequestActions `json:"actions"`
}

// TravelRequestPage is GET /travel-requests and GET /me/travel-requests.
type TravelRequestPage struct {
	Items []TravelRequestSummary `json:"items"`
	Total int                    `json:"total"`
	// SegmentCounts (list only) counts the matches of each segment the
	// caller has.
	SegmentCounts map[string]int `json:"segmentCounts,omitempty"`
}

// TravelFundingSource is one funding source of the engagement.
type TravelFundingSource struct {
	Region       string   `json:"region"`
	SubRegion    *string  `json:"subRegion"`
	BusinessUnit string   `json:"businessUnit"`
	ProductUnit  *string  `json:"productUnit"`
	Percentage   *float64 `json:"percentage"`
}

// TravelRequestDetail is GET /travel-requests/{jobNumber}.
type TravelRequestDetail struct {
	TravelRequestSummary
	Scope          string  `json:"scope"`
	AllocationID   *string `json:"allocationId"`
	VisaID         *int    `json:"visaId"`
	EngagementCode *string `json:"engagementCode"`
	EngagementType *string `json:"engagementType"`
	Address        *string `json:"address"`
	// FundingSources is set for Travel 35 in scope full or own, else null.
	FundingSources []TravelFundingSource `json:"fundingSources"`
	Tabs           TravelRequestTabs     `json:"tabs"`
}

// TravelRequestUpdate is the PATCH /travel-requests/{jobNumber} body. Nil
// fields are left unchanged.
type TravelRequestUpdate struct {
	Status *string `json:"status"`
	// Agency is the travel agent's name.
	Agency *string `json:"agency"`
	// Visa is a visa id from visa-options or one of the two sentinels.
	Visa *string `json:"visa"`
}

// TravelVisaOption is one choice of the Visa select.
type TravelVisaOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TravelVisaOptions is GET /travel-requests/{jobNumber}/visa-options.
type TravelVisaOptions struct {
	Items []TravelVisaOption `json:"items"`
}

// TravelSorts are the column sorts of GET /travel-requests (`sort` query
// parameter); a leading "-" means descending. Empty keeps the default order
// (current trips first). Traveller and Visa are not sortable.
var TravelSorts = []string{
	"jobNumber", "-jobNumber", "startDate", "-startDate",
	"destination", "-destination", "agency", "-agency", "status", "-status",
}

// TravelListQuery is the parsed query of the travel request lists.
type TravelListQuery struct {
	Segment     string
	Sort        string
	Search      string
	Travellers  []string
	Countries   []string
	Agencies    []string
	Statuses    []string
	StartFrom   string
	EndTo       string
	Limit       int
	Offset      int
	OwnRequests bool
}

// TravelPassport is the passport a visa is tagged with.
type TravelPassport struct {
	ID           int     `json:"id"`
	Country      *string `json:"country"`
	MaskedNumber string  `json:"maskedNumber"`
	IssuedDate   *string `json:"issuedDate"`
	ExpiryDate   *string `json:"expiryDate"`
}

// Displayed visa and document statuses.
const (
	TravelRecordActive   = "Active"
	TravelRecordInactive = "Inactive"
	TravelRecordExpired  = "Expired"
)

// TravelVisa is a visa as the screens show it.
type TravelVisa struct {
	ID         int             `json:"id"`
	Traveller  Person          `json:"traveller"`
	Country    *string         `json:"country"`
	VisaType   *string         `json:"visaType"`
	EntryType  *string         `json:"entryType"`
	StartDate  *string         `json:"startDate"`
	EndDate    *string         `json:"endDate"`
	IssuedDate *string         `json:"issuedDate"`
	Passport   *TravelPassport `json:"passport"`
	// Status is Active, Inactive or Expired (Active past its end date).
	Status   string `json:"status"`
	HasImage bool   `json:"hasImage"`
}

// TravelVisaPage is a page of visas.
type TravelVisaPage struct {
	Items []TravelVisa `json:"items"`
	Total int          `json:"total"`
}

// TravelDocument is a travel document as the screens show it.
type TravelDocument struct {
	ID           int     `json:"id"`
	Traveller    Person  `json:"traveller"`
	DocumentType *string `json:"documentType"`
	MaskedNumber string  `json:"maskedNumber"`
	// DocumentNumber is the full number, only to holders of Edit Travel
	// Document (13) on GET /travel-documents/{id}, to prefill the edit form.
	DocumentNumber *string `json:"documentNumber,omitempty"`
	Country        *string `json:"country"`
	IssuedDate     *string `json:"issuedDate"`
	ExpiryDate     *string `json:"expiryDate"`
	// Status is Active, Inactive or Expired (Active past its expiry date).
	Status   string `json:"status"`
	HasImage bool   `json:"hasImage"`
}

// TravelDocumentPage is a page of travel documents.
type TravelDocumentPage struct {
	Items []TravelDocument `json:"items"`
	Total int              `json:"total"`
}

// TravelImage is an uploaded visa or document image.
type TravelImage struct {
	Data []byte
	// Present is false when the form had no image part.
	Present bool
}

// TravelVisaForm is the parsed VisaForm (multipart).
type TravelVisaForm struct {
	TravellerEmail string
	Country        string
	VisaType       string
	EntryType      string
	IssuedDate     string
	StartDate      string
	EndDate        string
	PassportID     *int
	Status         string
	Image          TravelImage
}

// TravelDocumentForm is the parsed TravelDocumentForm (multipart).
type TravelDocumentForm struct {
	TravellerEmail string
	DocumentType   string
	DocumentNumber string
	Country        string
	IssuedDate     string
	ExpiryDate     string
	Status         string
	Image          TravelImage
}

// TravelRecordQuery is the parsed query of the visa and document lists.
type TravelRecordQuery struct {
	Search        string
	Travellers    []string
	Countries     []string
	DocumentTypes []string
	// Status is Active or Expired (computed from the end or expiry date).
	Status string
	// Sort is one of VisaSorts or DocumentSorts (by list); empty or unknown
	// keeps the expiry order.
	Sort   string
	Limit  int
	Offset int
}
