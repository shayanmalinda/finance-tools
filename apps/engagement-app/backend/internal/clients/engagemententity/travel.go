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

// TravelAPI is what the travel screen area needs from the Engagement Entity.
// The area owns this file: its methods, request and response types, and the
// HTTPClient implementations live here.
type TravelAPI interface {
	// SearchTravelRequests calls POST /travel-requests/search.
	SearchTravelRequests(ctx context.Context, f TravelRequestSearch) (TravelRequestPage, error)
	// GetTravelRequest calls GET /travel-requests/{jobNumber}; an unknown job
	// number is an UpstreamError with StatusCode 404.
	GetTravelRequest(ctx context.Context, jobNumber string) (TravelRequestRecord, error)
	// UpdateTravelRequest calls PATCH /travel-requests/{jobNumber}.
	UpdateTravelRequest(ctx context.Context, jobNumber string, in TravelRequestWrite) (TravelRequestRecord, error)
	// TravelActivePerDiems calls GET /travel-requests/{jobNumber}/active-per-diems.
	TravelActivePerDiems(ctx context.Context, jobNumber string) ([]TravelActivePerDiem, error)
	// TravelAgencies calls GET /travel-agencies; status "" lists all.
	TravelAgencies(ctx context.Context, status string) ([]TravelAgentRecord, error)

	// SearchVisas calls GET /visas.
	SearchVisas(ctx context.Context, f TravelVisaFilter) (TravelVisaPage, error)
	// GetVisa calls GET /visas/{id}.
	GetVisa(ctx context.Context, id int) (TravelVisaRecord, error)
	// CreateVisa calls POST /visas.
	CreateVisa(ctx context.Context, in TravelVisaCreate) (TravelVisaRecord, error)
	// UpdateVisa calls PUT /visas/{id}.
	UpdateVisa(ctx context.Context, id int, in TravelVisaUpdate) (TravelVisaRecord, error)
	// SearchTravelDocuments calls GET /travel-documents.
	SearchTravelDocuments(ctx context.Context, f TravelDocumentFilter) (TravelDocumentPage, error)
	// GetTravelDocument calls GET /travel-documents/{id}.
	GetTravelDocument(ctx context.Context, id int) (TravelDocumentRecord, error)
	// CreateTravelDocument calls POST /travel-documents.
	CreateTravelDocument(ctx context.Context, in TravelDocumentCreate) (TravelDocumentRecord, error)
	// UpdateTravelDocument calls PUT /travel-documents/{id}.
	UpdateTravelDocument(ctx context.Context, id int, in TravelDocumentUpdate) (TravelDocumentRecord, error)
}

// Travel request list sorts the entity accepts.
const (
	TravelSortCurrentFirst = "current"
	TravelSortStartDesc    = "startDesc"
)

// TravelRequestSearch is the entity's travel request filter. Rows with
// status Deleted or NULL are never returned.
type TravelRequestSearch struct {
	Statuses          []string `json:"statuses,omitempty"`
	TravellerEmails   []string `json:"travellerEmails,omitempty"`
	Countries         []string `json:"countries,omitempty"`
	AgencyNames       []string `json:"agencyNames,omitempty"`
	TravelAgentID     *int     `json:"travelAgentId,omitempty"`
	VisaID            *int     `json:"visaId,omitempty"`
	StartFrom         string   `json:"startFrom,omitempty"`
	EndTo             string   `json:"endTo,omitempty"`
	Search            string   `json:"search,omitempty"`
	SearchEngagements bool     `json:"searchEngagements,omitempty"`
	SearchAgency      bool     `json:"searchAgency,omitempty"`
	Sort              string   `json:"sort,omitempty"`
	Today             string   `json:"today,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	Offset            int      `json:"offset,omitempty"`
	WithStatusCounts  bool     `json:"withStatusCounts,omitempty"`
}

// TravelVisaRef is the visa a travel request points at.
type TravelVisaRef struct {
	ID        int     `json:"id"`
	Email     *string `json:"email"`
	Country   *string `json:"country"`
	VisaType  *string `json:"visaType"`
	EntryType *string `json:"entryType"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// TravelEngagementRecord is the live allocation_db engagement of a request.
type TravelEngagementRecord struct {
	ID             string  `json:"id"`
	Title          *string `json:"title"`
	CustomerName   *string `json:"customerName"`
	EngagementCode *string `json:"engagementCode"`
	TypeID         *int    `json:"typeId"`
	TypeName       *string `json:"typeName"`
	Kind           *string `json:"kind"`
	CampaignID     *string `json:"campaignId"`
}

// TravelFundingSourceRecord is one funding source of the engagement.
type TravelFundingSourceRecord struct {
	BusinessUnit *string  `json:"businessUnit"`
	ProductUnit  *string  `json:"productUnit"`
	Region       *string  `json:"region"`
	SubRegion    *string  `json:"subRegion"`
	Percentage   *float64 `json:"percentage"`
}

// TravelRequestRecord is a travelapp.travel row with its agency, visa and
// engagement. FundingSources is set by GetTravelRequest only.
type TravelRequestRecord struct {
	JobNumber       string                      `json:"jobNumber"`
	Status          string                      `json:"status"`
	TravellerEmail  string                      `json:"travellerEmail"`
	City            *string                     `json:"city"`
	Country         string                      `json:"country"`
	CustomerName    string                      `json:"customerName"`
	StartDate       string                      `json:"startDate"`
	EndDate         string                      `json:"endDate"`
	AllocationID    *string                     `json:"allocationId"`
	EngagementID    *string                     `json:"engagementId"`
	EngagementCode  string                      `json:"engagementCode"`
	EngagementType  string                      `json:"engagementType"`
	TravelAgentID   *int                        `json:"travelAgentId"`
	TravelAgentName *string                     `json:"travelAgentName"`
	VisaID          *int                        `json:"visaId"`
	VisaText        *string                     `json:"visa"`
	Visa            *TravelVisaRef              `json:"visaDetails"`
	LastUpdatedBy   *string                     `json:"lastUpdatedBy"`
	LastUpdatedDate *string                     `json:"lastUpdatedDate"`
	Engagement      *TravelEngagementRecord     `json:"engagement"`
	FundingSources  []TravelFundingSourceRecord `json:"fundingSources,omitempty"`
}

// TravelRequestPage is a page of travel requests.
type TravelRequestPage struct {
	Items        []TravelRequestRecord `json:"items"`
	Total        int                   `json:"total"`
	StatusCounts map[string]int        `json:"statusCounts"`
}

// TravelRequestWrite is the body of PATCH /travel-requests/{jobNumber}.
type TravelRequestWrite struct {
	Status        *string `json:"status,omitempty"`
	TravelAgentID *int    `json:"travelAgentId,omitempty"`
	VisaID        *int    `json:"visaId,omitempty"`
	UpdatedBy     string  `json:"updatedBy"`
}

// TravelAgentRecord is a row of travel_agents.
type TravelAgentRecord struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Status *string `json:"status"`
}

// TravelActivePerDiem is an Active per diem as the Completed email lists it.
// Dates are "YYYY-MM-DD HH:MM:SS".
type TravelActivePerDiem struct {
	StartDate    *string  `json:"startDate"`
	EndDate      *string  `json:"endDate"`
	NoOfDays     *int     `json:"noOfDays"`
	Currency     *string  `json:"currency"`
	PerDiemValue float64  `json:"perDiemValue"`
	Total        *float64 `json:"total"`
}

// Email audit statuses.
const (
	EmailAuditSent   = "Sent"
	EmailAuditFailed = "Failed"
)

// EmailAudit is one email_auditlog row (travelapp or allocation_db), one per
// send attempt.
type EmailAudit struct {
	Subject            string `json:"subject"`
	TemplateID         string `json:"templateId"`
	ContentKeyValPairs string `json:"contentKeyValPairs"`
	Recipients         string `json:"recipients"`
	CCRecipients       string `json:"ccRecipients"`
	// Status is EmailAuditSent or EmailAuditFailed.
	Status string `json:"status"`
	// ErrorMessage is why the send failed; empty for Sent.
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// TravelEmailAudit is one travelapp email_auditlog row.
type TravelEmailAudit = EmailAudit

// TravelPassportRef is the passport a visa is tagged with.
type TravelPassportRef struct {
	ID         int     `json:"id"`
	Country    *string `json:"country"`
	DocumentNo *string `json:"documentNo"`
	IssuedDate *string `json:"issuedDate"`
	ExpiryDate *string `json:"expiryDate"`
}

// TravelVisaRecord is a visa_status row.
type TravelVisaRecord struct {
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
}

// Visa and document validity filters.
const (
	TravelValidityActive  = "active"
	TravelValidityExpired = "expired"
)

// TravelVisaFilter narrows SearchVisas.
type TravelVisaFilter struct {
	Emails     []string
	Countries  []string
	Status     string
	Validity   string
	Today      string
	CoversFrom string
	CoversTo   string
	Search     string
	ExcludeIDs []int
	Limit      int
	Offset     int
	// IncludeMarkedExpired also returns visas stored as Expired.
	IncludeMarkedExpired bool
}

// TravelVisaPage is a page of visas.
type TravelVisaPage struct {
	Items []TravelVisaRecord `json:"items"`
	Total int                `json:"total"`
}

// TravelVisaCreate is the body of POST /visas.
type TravelVisaCreate struct {
	Email      string `json:"email"`
	Country    string `json:"country"`
	VisaType   string `json:"visaType"`
	EntryType  string `json:"entryType"`
	IssuedDate string `json:"issuedDate"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	PassportID *int   `json:"passportId,omitempty"`
	ImageID    string `json:"imageId"`
	CreatedBy  string `json:"createdBy"`
}

// TravelVisaUpdate is the body of PUT /visas/{id}.
type TravelVisaUpdate struct {
	Country    string  `json:"country"`
	VisaType   string  `json:"visaType"`
	EntryType  string  `json:"entryType"`
	IssuedDate string  `json:"issuedDate"`
	StartDate  string  `json:"startDate"`
	EndDate    string  `json:"endDate"`
	PassportID *int    `json:"passportId,omitempty"`
	Status     string  `json:"status"`
	ImageID    *string `json:"imageId,omitempty"`
	UpdatedBy  string  `json:"updatedBy"`
}

// TravelDocumentRecord is an employee_travel_document row.
type TravelDocumentRecord struct {
	ID            int     `json:"id"`
	Email         *string `json:"email"`
	DocumentNo    *string `json:"documentNo"`
	DocumentType  *string `json:"documentType"`
	IssuedCountry *string `json:"issuedCountry"`
	IssuedDate    *string `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate"`
	Status        string  `json:"status"`
	ImageID       *string `json:"imageId"`
}

// TravelDocumentFilter narrows SearchTravelDocuments.
type TravelDocumentFilter struct {
	Emails        []string
	Countries     []string
	DocumentTypes []string
	Status        string
	Validity      string
	Today         string
	Search        string
	Limit         int
	Offset        int
}

// TravelDocumentPage is a page of travel documents.
type TravelDocumentPage struct {
	Items []TravelDocumentRecord `json:"items"`
	Total int                    `json:"total"`
}

// TravelDocumentCreate is the body of POST /travel-documents.
type TravelDocumentCreate struct {
	Email         string  `json:"email"`
	DocumentNo    string  `json:"documentNo"`
	DocumentType  string  `json:"documentType"`
	IssuedCountry string  `json:"issuedCountry"`
	IssuedDate    string  `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate,omitempty"`
	ImageID       string  `json:"imageId"`
	CreatedBy     string  `json:"createdBy"`
}

// TravelDocumentUpdate is the body of PUT /travel-documents/{id}.
type TravelDocumentUpdate struct {
	DocumentNo    *string `json:"documentNo,omitempty"`
	DocumentType  string  `json:"documentType"`
	IssuedCountry string  `json:"issuedCountry"`
	IssuedDate    string  `json:"issuedDate"`
	ExpiryDate    *string `json:"expiryDate,omitempty"`
	Status        string  `json:"status"`
	ImageID       *string `json:"imageId,omitempty"`
	UpdatedBy     string  `json:"updatedBy"`
}

func travelRequestPath(jobNumber string) string {
	return "/travel-requests/" + pathSegment(jobNumber)
}

// SearchTravelRequests calls POST /travel-requests/search.
func (c *HTTPClient) SearchTravelRequests(ctx context.Context, f TravelRequestSearch) (TravelRequestPage, error) {
	var resp TravelRequestPage
	if err := c.rest.Post(ctx, "/travel-requests/search", f, &resp); err != nil {
		return TravelRequestPage{}, err
	}
	return resp, nil
}

// GetTravelRequest calls GET /travel-requests/{jobNumber}.
func (c *HTTPClient) GetTravelRequest(ctx context.Context, jobNumber string) (TravelRequestRecord, error) {
	var resp TravelRequestRecord
	if err := c.rest.Get(ctx, travelRequestPath(jobNumber), &resp); err != nil {
		return TravelRequestRecord{}, err
	}
	return resp, nil
}

// UpdateTravelRequest calls PATCH /travel-requests/{jobNumber}.
func (c *HTTPClient) UpdateTravelRequest(ctx context.Context, jobNumber string, in TravelRequestWrite) (TravelRequestRecord, error) {
	var resp TravelRequestRecord
	if err := c.rest.Do(ctx, http.MethodPatch, travelRequestPath(jobNumber), in, &resp); err != nil {
		return TravelRequestRecord{}, err
	}
	return resp, nil
}

// TravelActivePerDiems calls GET /travel-requests/{jobNumber}/active-per-diems.
func (c *HTTPClient) TravelActivePerDiems(ctx context.Context, jobNumber string) ([]TravelActivePerDiem, error) {
	var resp struct {
		Items []TravelActivePerDiem `json:"items"`
	}
	if err := c.rest.Get(ctx, travelRequestPath(jobNumber)+"/active-per-diems", &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// TravelAgencies calls GET /travel-agencies.
func (c *HTTPClient) TravelAgencies(ctx context.Context, status string) ([]TravelAgentRecord, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	var resp struct {
		Items []TravelAgentRecord `json:"items"`
	}
	if err := c.rest.Get(ctx, withQuery("/travel-agencies", q), &resp); err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// InsertTravelEmailAudit calls POST /travel-email-audit-logs.
func (c *HTTPClient) InsertTravelEmailAudit(ctx context.Context, in TravelEmailAudit) error {
	return c.rest.Post(ctx, "/travel-email-audit-logs", in, nil)
}

// InsertAllocationEmailAudit calls POST /allocation-email-audit-logs.
func (c *HTTPClient) InsertAllocationEmailAudit(ctx context.Context, in EmailAudit) error {
	return c.rest.Post(ctx, "/allocation-email-audit-logs", in, nil)
}

func travelAddList(q url.Values, key string, vals []string) {
	for _, v := range vals {
		q.Add(key, v)
	}
}

func travelAddString(q url.Values, key, v string) {
	if v != "" {
		q.Set(key, v)
	}
}

func travelAddPage(q url.Values, limit, offset int) {
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
}

// SearchVisas calls GET /visas.
func (c *HTTPClient) SearchVisas(ctx context.Context, f TravelVisaFilter) (TravelVisaPage, error) {
	q := url.Values{}
	travelAddList(q, "email", f.Emails)
	travelAddList(q, "country", f.Countries)
	travelAddString(q, "status", f.Status)
	travelAddString(q, "validity", f.Validity)
	travelAddString(q, "today", f.Today)
	travelAddString(q, "coversFrom", f.CoversFrom)
	travelAddString(q, "coversTo", f.CoversTo)
	travelAddString(q, "search", f.Search)
	if f.IncludeMarkedExpired {
		q.Set("includeMarkedExpired", "true")
	}
	for _, id := range f.ExcludeIDs {
		q.Add("excludeId", strconv.Itoa(id))
	}
	travelAddPage(q, f.Limit, f.Offset)
	var resp TravelVisaPage
	if err := c.rest.Get(ctx, withQuery("/visas", q), &resp); err != nil {
		return TravelVisaPage{}, err
	}
	return resp, nil
}

// GetVisa calls GET /visas/{id}.
func (c *HTTPClient) GetVisa(ctx context.Context, id int) (TravelVisaRecord, error) {
	var resp TravelVisaRecord
	if err := c.rest.Get(ctx, "/visas/"+strconv.Itoa(id), &resp); err != nil {
		return TravelVisaRecord{}, err
	}
	return resp, nil
}

// CreateVisa calls POST /visas.
func (c *HTTPClient) CreateVisa(ctx context.Context, in TravelVisaCreate) (TravelVisaRecord, error) {
	var resp TravelVisaRecord
	if err := c.rest.Post(ctx, "/visas", in, &resp); err != nil {
		return TravelVisaRecord{}, err
	}
	return resp, nil
}

// UpdateVisa calls PUT /visas/{id}.
func (c *HTTPClient) UpdateVisa(ctx context.Context, id int, in TravelVisaUpdate) (TravelVisaRecord, error) {
	var resp TravelVisaRecord
	if err := c.rest.Do(ctx, http.MethodPut, "/visas/"+strconv.Itoa(id), in, &resp); err != nil {
		return TravelVisaRecord{}, err
	}
	return resp, nil
}

// SearchTravelDocuments calls GET /travel-documents.
func (c *HTTPClient) SearchTravelDocuments(ctx context.Context, f TravelDocumentFilter) (TravelDocumentPage, error) {
	q := url.Values{}
	travelAddList(q, "email", f.Emails)
	travelAddList(q, "country", f.Countries)
	travelAddList(q, "documentType", f.DocumentTypes)
	travelAddString(q, "status", f.Status)
	travelAddString(q, "validity", f.Validity)
	travelAddString(q, "today", f.Today)
	travelAddString(q, "search", f.Search)
	travelAddPage(q, f.Limit, f.Offset)
	var resp TravelDocumentPage
	if err := c.rest.Get(ctx, withQuery("/travel-documents", q), &resp); err != nil {
		return TravelDocumentPage{}, err
	}
	return resp, nil
}

// GetTravelDocument calls GET /travel-documents/{id}.
func (c *HTTPClient) GetTravelDocument(ctx context.Context, id int) (TravelDocumentRecord, error) {
	var resp TravelDocumentRecord
	if err := c.rest.Get(ctx, "/travel-documents/"+strconv.Itoa(id), &resp); err != nil {
		return TravelDocumentRecord{}, err
	}
	return resp, nil
}

// CreateTravelDocument calls POST /travel-documents.
func (c *HTTPClient) CreateTravelDocument(ctx context.Context, in TravelDocumentCreate) (TravelDocumentRecord, error) {
	var resp TravelDocumentRecord
	if err := c.rest.Post(ctx, "/travel-documents", in, &resp); err != nil {
		return TravelDocumentRecord{}, err
	}
	return resp, nil
}

// UpdateTravelDocument calls PUT /travel-documents/{id}.
func (c *HTTPClient) UpdateTravelDocument(ctx context.Context, id int, in TravelDocumentUpdate) (TravelDocumentRecord, error) {
	var resp TravelDocumentRecord
	if err := c.rest.Do(ctx, http.MethodPut, "/travel-documents/"+strconv.Itoa(id), in, &resp); err != nil {
		return TravelDocumentRecord{}, err
	}
	return resp, nil
}
