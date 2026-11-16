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

import "strings"

// Legacy invoice.status values (Travel app INVOICE_APPROVAL), unchanged.
const (
	InvoiceStatusPending           = "Pending"
	InvoiceStatusAcceptedByTravel  = "Accepted by travel"
	InvoiceStatusAcceptedByFinance = "Accepted by finance"
	InvoiceStatusComplete          = "Complete"
	InvoiceStatusRejectedByTravel  = "Rejected by travel"
	InvoiceStatusRejectedByFinance = "Rejected by finance"
)

// perdiem_payment.ns_status values. Pending and Complete are the legacy
// values. Failed is the Release 1 NS Sync Failures marker (see
// FinancePaymentNSFailed).
const (
	PaymentNSPending  = "Pending"
	PaymentNSComplete = "Complete"
	// PaymentNSFailed marks a payment whose push to NetSuite ended without
	// a vendor bill id: NetSuite may or may not hold it. Only a re-sync
	// (lookup) moves it on: to Complete when NetSuite has it, back to
	// Pending when it does not. Legacy code treats it as "not Pending", so
	// it can be neither edited nor pushed again.
	PaymentNSFailed = "Failed"
	// PaymentNSPushing marks a payment while its push is with NetSuite.
	PaymentNSPushing = "Pushing"
)

// Per diem allocation status values.
const (
	PerDiemStatusActive  = "Active"
	PerDiemStatusDeleted = "Deleted"
)

// The finance rules read TravelStatusInProgress, TravelStatusExtended and
// TravelStatusCancelled (travel.go).

// InvoiceNSFailurePrefix starts invoice.reject_reason on an invoice whose
// push to NetSuite ended without a purchase order id (NS Sync Failures).
// The invoice keeps the legacy status "Accepted by finance"; legacy code
// writes reject_reason only together with a Rejected status, so an
// accepted invoice with this prefix is unambiguous. A re-sync clears it.
const InvoiceNSFailurePrefix = "NetSuite push failed: "

// InvoiceNSPushingMarker is invoice.reject_reason on a Complete invoice
// while its push is with NetSuite.
const InvoiceNSPushingMarker = "NetSuite push in progress"

// InvoiceNSReferencePrefix starts invoice.reject_reason on a Complete
// invoice pushed by this app: the purchase order id follows (there is no
// column for it; a Complete invoice has no reject reason).
const InvoiceNSReferencePrefix = "NetSuite PO: "

// InvoiceCostTypeCurrency is the cost type whose invoices count against the
// per diem balance (legacy CURRENCY_COST_TYPE).
const InvoiceCostTypeCurrency = "Currency"

// InvoiceCostTypeTravelInsuranceID is the Travel Insurance cost type id
// (legacy TRAVEL_INSURANCE_INVOICE_COST_TYPE); its invoices use the "ALL"
// account classification.
const InvoiceCostTypeTravelInsuranceID = 5

// FinanceCampaign is the Salesforce campaign of a marketing event trip
// (engagement.campaign_id; name = engagement.project_name).
type FinanceCampaign struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// FinanceTrip is the travel request facts the finance rules need.
type FinanceTrip struct {
	JobNumber      string           `json:"jobNumber"`
	Status         *string          `json:"status"`
	TravellerEmail string           `json:"travellerEmail"`
	AgencyID       *int             `json:"agencyId"`
	AgencyName     *string          `json:"agencyName"`
	AllocationID   *string          `json:"allocationId"`
	City           *string          `json:"city"`
	Country        *string          `json:"country"`
	StartDate      *string          `json:"startDate"`
	EndDate        *string          `json:"endDate"`
	CustomerName   *string          `json:"customerName"`
	EngagementCode *string          `json:"engagementCode"`
	Campaign       *FinanceCampaign `json:"campaign"`
}

// FinanceEngagement is the engagement of a trip (allocation → engagement in
// allocation_db), live: the travel row's engagement_code is a stale copy.
type FinanceEngagement struct {
	ID             string
	EngagementCode string
	CustomerName   *string
	ProjectName    *string
	CampaignID     *string
}

// FinanceFundingSource is a row of engagement_funding_source with names.
type FinanceFundingSource struct {
	BusinessUnit *string
	ProductUnit  *string
	Region       *string
	SubRegion    *string
	// Percentage is the DECIMAL as stored, e.g. "50.00".
	Percentage string
}

// FinancePerDiem is a per diem allocation (perdiem_allocation).
type FinancePerDiem struct {
	ID             int     `json:"id"`
	EngagementCode string  `json:"engagementCode"`
	JobNumber      string  `json:"jobNumber"`
	Currency       *string `json:"currency"`
	PerDiemValue   float64 `json:"perDiemValue"`
	Total          float64 `json:"total"`
	// StartDate and EndDate are "YYYY-MM-DD HH:MM:SS".
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
	NoOfDays  *int    `json:"noOfDays"`
	Status    string  `json:"status"`
}

// FinancePerDiemCreate adds a per diem allocation.
type FinancePerDiemCreate struct {
	Currency     string  `json:"currency"`
	PerDiemValue float64 `json:"perDiemValue"`
	Total        float64 `json:"total"`
	StartDate    string  `json:"startDate"`
	EndDate      string  `json:"endDate"`
	NoOfDays     int     `json:"noOfDays"`
	CreatedBy    string  `json:"createdBy"`
}

// FinanceAmount is an amount in a currency on a date (YYYY-MM-DD HH:MM:SS),
// as the per diem balance needs it.
type FinanceAmount struct {
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	AddedDate string  `json:"addedDate"`
}

// FinancePerDiemTotal is the sum of a trip's active per diems in one currency.
type FinancePerDiemTotal struct {
	Currency    string  `json:"currency"`
	TotalAmount float64 `json:"totalAmount"`
}

// FinancePaymentBasis is everything the per diem balance is computed from
// (legacy getConvertedPerDiemPaymentSummary inputs).
type FinancePaymentBasis struct {
	// PerDiemTotals are per currency, first added currency first.
	PerDiemTotals []FinancePerDiemTotal `json:"perDiemTotals"`
	// CurrencyInvoices are Complete invoices of cost type Currency, in their
	// converted (paid) currency and amount.
	CurrencyInvoices []FinanceAmount `json:"currencyInvoices"`
	TravelCards      []FinanceAmount `json:"travelCards"`
	// Payments are in their original payment currency and amount.
	Payments     []FinanceAmount `json:"payments"`
	PaymentCount int             `json:"paymentCount"`
}

// FinanceTravelCard is a travel card (travel_card).
type FinanceTravelCard struct {
	ID        int     `json:"id"`
	JobNumber string  `json:"jobNumber"`
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
	AddedBy   string  `json:"addedBy"`
	AddedDate string  `json:"addedDate"`
}

// FinanceTravelCardCreate adds a travel card.
type FinanceTravelCardCreate struct {
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
	CreatedBy string  `json:"createdBy"`
}

// FinanceCostType is an active invoice cost type (inv_cost_type).
type FinanceCostType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// FinanceInvoice is an invoice with its legacy joins (classification, cost
// type, the adding agent's agency) and its trip.
type FinanceInvoice struct {
	ID                      int              `json:"id"`
	InvoiceNumber           string           `json:"invoiceNumber"`
	Month                   string           `json:"month"`
	CostTypeID              int              `json:"costTypeId"`
	CostTypeName            *string          `json:"costTypeName"`
	AccountClassificationID int              `json:"accountClassificationId"`
	AccountInternalID       *string          `json:"accountInternalId"`
	Department              *string          `json:"department"`
	InvoiceCurrency         string           `json:"invoiceCurrency"`
	InvoiceAmount           float64          `json:"invoiceAmount"`
	ConvertedCurrency       string           `json:"convertedCurrency"`
	ConvertedAmount         float64          `json:"convertedAmount"`
	Status                  string           `json:"status"`
	AddedBy                 *string          `json:"addedBy"`
	AddedDate               *string          `json:"addedDate"`
	UpdatedBy               *string          `json:"updatedBy"`
	UpdatedDate             *string          `json:"updatedDate"`
	RejectReason            *string          `json:"rejectReason"`
	JobNumber               string           `json:"jobNumber"`
	HRApprovedBy            *string          `json:"hrApprovedBy"`
	FinanceApprovedBy       *string          `json:"financeApprovedBy"`
	AgentEmail              *string          `json:"agentEmail"`
	VendorID                *int             `json:"vendorId"`
	AgencyID                *int             `json:"agencyId"`
	AgencyName              *string          `json:"agencyName"`
	TravellerEmail          *string          `json:"travellerEmail"`
	TravelStatus            *string          `json:"travelStatus"`
	City                    *string          `json:"city"`
	Country                 *string          `json:"country"`
	TravelStartDate         *string          `json:"travelStartDate"`
	TravelEndDate           *string          `json:"travelEndDate"`
	AllocationID            *string          `json:"-"`
	NetSuiteFailed          bool             `json:"netsuiteFailed"`
	NetSuitePushing         bool             `json:"netsuitePushing"`
	NetSuiteError           *string          `json:"netsuiteError"`
	NetSuiteReference       *string          `json:"netsuiteReference"`
	Campaign                *FinanceCampaign `json:"campaign"`
	InvoiceAmountText       string           `json:"-"`
	ConvertedAmountText     string           `json:"-"`
}

// SplitInvoiceRejectReason turns the stored reject_reason into the reject
// reason shown to people and the NS Sync Failures marker: on an invoice
// accepted by finance a reason with InvoiceNSFailurePrefix is the push
// error, never a reject reason.
func (i *FinanceInvoice) SplitInvoiceRejectReason() {
	if i.RejectReason == nil {
		return
	}
	if strings.EqualFold(i.Status, InvoiceStatusAcceptedByFinance) &&
		len(*i.RejectReason) >= len(InvoiceNSFailurePrefix) &&
		strings.EqualFold((*i.RejectReason)[:len(InvoiceNSFailurePrefix)], InvoiceNSFailurePrefix) {
		msg := strings.TrimSpace((*i.RejectReason)[len(InvoiceNSFailurePrefix):])
		i.NetSuiteFailed = true
		i.NetSuiteError = &msg
		i.RejectReason = nil
		return
	}
	if strings.EqualFold(i.Status, InvoiceStatusComplete) && *i.RejectReason == InvoiceNSPushingMarker {
		i.NetSuitePushing = true
		i.RejectReason = nil
		return
	}
	if strings.EqualFold(i.Status, InvoiceStatusComplete) && strings.HasPrefix(*i.RejectReason, InvoiceNSReferencePrefix) {
		ref := strings.TrimSpace(strings.TrimPrefix(*i.RejectReason, InvoiceNSReferencePrefix))
		i.NetSuiteReference = &ref
		i.RejectReason = nil
	}
}

// FinanceInvoiceFilter narrows the invoice list. Empty fields mean "any".
type FinanceInvoiceFilter struct {
	JobNumber string
	// Statuses are legacy status values (OR).
	Statuses []string
	// Failed: "true" only NS Sync Failures, "false" none of them, "" both.
	Failed      string
	Search      string
	CostTypes   []string
	Agencies    []string
	Months      []string
	CampaignIDs []string
	// AgencyID scopes to the invoices added by that agency's agents.
	AgencyID int
	// AddedFrom and AddedTo (YYYY-MM-DD, inclusive) bound added_date.
	AddedFrom string
	AddedTo   string
	// AllocationIDs is resolved from CampaignIDs by the service.
	AllocationIDs []string
	Limit         int
	Offset        int
}

// FinanceInvoiceCounts are the invoice work queue sizes.
type FinanceInvoiceCounts struct {
	Pending           int `json:"pending"`
	AcceptedByTravel  int `json:"acceptedByTravel"`
	AcceptedByFinance int `json:"acceptedByFinance"`
	Failed            int `json:"failed"`
	All               int `json:"all"`
}

// FinanceInvoiceCreate is one invoice to add; CostTypeID is an active
// inv_cost_type id.
type FinanceInvoiceCreate struct {
	InvoiceNumber     string  `json:"invoiceNumber"`
	Month             string  `json:"month"`
	CostTypeID        int     `json:"costTypeId"`
	InvoiceCurrency   string  `json:"invoiceCurrency"`
	InvoiceAmount     float64 `json:"invoiceAmount"`
	ConvertedCurrency string  `json:"convertedCurrency"`
	ConvertedAmount   float64 `json:"convertedAmount"`
}

// FinanceInvoicesCreate adds invoices to a trip.
type FinanceInvoicesCreate struct {
	Invoices  []FinanceInvoiceCreate `json:"invoices"`
	CreatedBy string                 `json:"createdBy"`
}

// Invoice decisions and steps.
const (
	FinanceDecisionApprove = "approve"
	FinanceDecisionReject  = "reject"
	FinanceStepTravel      = "travel"
	FinanceStepFinance     = "finance"
)

// FinanceInvoiceDecision approves or rejects invoices at one step.
type FinanceInvoiceDecision struct {
	InvoiceIDs []int  `json:"invoiceIds"`
	Decision   string `json:"decision"`
	Step       string `json:"step"`
	Reason     string `json:"reason"`
	UpdatedBy  string `json:"updatedBy"`
}

// FinanceDecisionResult is one invoice's outcome.
type FinanceDecisionResult struct {
	ID        int     `json:"id"`
	OK        bool    `json:"ok"`
	JobNumber *string `json:"jobNumber"`
	Message   *string `json:"message"`
}

// FinancePayment is a per diem payment with its trip.
type FinancePayment struct {
	ID                int              `json:"id"`
	JobNumber         string           `json:"jobNumber"`
	PerDiemNumber     string           `json:"perDiemNumber"`
	PaymentCurrency   *string          `json:"paymentCurrency"`
	PaymentAmount     *float64         `json:"paymentAmount"`
	NSStatus          *string          `json:"nsStatus"`
	ConvertedCurrency *string          `json:"convertedCurrency"`
	ConvertedAmount   *float64         `json:"convertedAmount"`
	AddedBy           *string          `json:"addedBy"`
	AddedDate         *string          `json:"addedDate"`
	LastUpdatedBy     *string          `json:"lastUpdatedBy"`
	LastUpdatedDate   *string          `json:"lastUpdatedDate"`
	TravellerEmail    *string          `json:"travellerEmail"`
	TravelStatus      *string          `json:"travelStatus"`
	AllocationID      *string          `json:"-"`
	Campaign          *FinanceCampaign `json:"campaign"`
	// ConvertedAmountText is the DECIMAL as stored, for NetSuite.
	ConvertedAmountText *string `json:"-"`
}

// FinancePaymentFilter narrows the payment list.
type FinancePaymentFilter struct {
	JobNumber          string
	NSStatuses         []string
	Search             string
	Travellers         []string
	ConvertedCurrency  []string
	CampaignIDs        []string
	AddedFrom, AddedTo string
	AllocationIDs      []string
	Limit, Offset      int
}

// FinancePaymentCounts are the payment work queue sizes.
type FinancePaymentCounts struct {
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
	All     int `json:"all"`
}

// FinancePaymentCreate adds a per diem payment (already converted by the
// backend).
type FinancePaymentCreate struct {
	PerDiemNumber     string  `json:"perDiemNumber"`
	PaymentCurrency   string  `json:"paymentCurrency"`
	PaymentAmount     float64 `json:"paymentAmount"`
	ConvertedCurrency string  `json:"convertedCurrency"`
	ConvertedAmount   float64 `json:"convertedAmount"`
	CreatedBy         string  `json:"createdBy"`
}

// FinancePaymentUpdate edits a pending payment's amount (and its
// conversion at today's rate).
type FinancePaymentUpdate struct {
	PaymentAmount     float64 `json:"paymentAmount"`
	ConvertedCurrency string  `json:"convertedCurrency"`
	ConvertedAmount   float64 `json:"convertedAmount"`
	UpdatedBy         string  `json:"updatedBy"`
}

// FinanceSubsidiary is an active expense_app subsidiary.
type FinanceSubsidiary struct {
	ID                int    `json:"id"`
	LegalName         string `json:"legalName"`
	CurrencyCode      string `json:"currencyCode"`
	Country           string `json:"country"`
	TaxCodeInternalID string `json:"taxCodeInternalId"`
}

// FinanceEmployee is the traveller's HR record, which the backend passes
// for a vendor bill (the legacy NetSuitePerDiemApprover).
type FinanceEmployee struct {
	EmployeeID string `json:"employeeId"`
	Email      string `json:"email"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	JobRole    string `json:"jobRole"`
	Department string `json:"department"`
	Company    string `json:"company"`
}

// FinancePushRequest pushes or re-syncs one record.
type FinancePushRequest struct {
	UpdatedBy string `json:"updatedBy"`
	// Employee is the traveller, for a payment push only.
	Employee *FinanceEmployee `json:"employee,omitempty"`
}

// NetSuite outcomes of a push or re-sync.
const (
	// NSOutcomePushed: the record was created in NetSuite now.
	NSOutcomePushed = "pushed"
	// NSOutcomeFound: NetSuite already had it; marked pushed, nothing sent.
	NSOutcomeFound = "found"
	// NSOutcomeNotFound: a re-sync found nothing; the record is back in
	// the push queue.
	NSOutcomeNotFound = "not_found"
	// NSOutcomeFailed: the push did not return an id; the record is on NS
	// Sync Failures.
	NSOutcomeFailed = "failed"
	// NSOutcomeSkipped: nothing was done (wrong state, missing data, or a
	// lookup error).
	NSOutcomeSkipped = "skipped"
)

// FinancePushResult is the outcome for one record.
type FinancePushResult struct {
	ID        string  `json:"id"`
	Number    string  `json:"number"`
	JobNumber string  `json:"jobNumber"`
	OK        bool    `json:"ok"`
	Outcome   string  `json:"outcome"`
	Reference *string `json:"reference"`
	Message   *string `json:"message"`
}

// FinanceReference is a live NetSuite lookup answer.
type FinanceReference struct {
	Reference *string `json:"reference"`
}
