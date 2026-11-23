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

// FinanceAPI is what the finance screen area needs from the Engagement Entity.
// The area owns this file: its methods, request and response types, and the
// HTTPClient implementations live here. Paths are the entity's /finance/...
// routes (openapi.finance.yaml of the entity).
type FinanceAPI interface {
	FinanceTrip(ctx context.Context, jobNumber string) (FinanceTrip, error)

	FinancePerDiems(ctx context.Context, jobNumber string) ([]FinancePerDiem, error)
	FinancePerDiem(ctx context.Context, jobNumber string, id int) (FinancePerDiem, error)
	FinanceAddPerDiem(ctx context.Context, jobNumber string, in FinancePerDiemCreate) (FinancePerDiem, error)
	FinanceDeletePerDiem(ctx context.Context, jobNumber string, id int) error
	FinancePaymentBasis(ctx context.Context, jobNumber string) (FinancePaymentBasis, error)

	FinanceTravelCards(ctx context.Context, jobNumber string) ([]FinanceTravelCard, error)
	FinanceAddTravelCard(ctx context.Context, jobNumber string, in FinanceTravelCardCreate) (FinanceTravelCard, error)
	FinanceTripExpenses(ctx context.Context, jobNumber string) (FinanceTripExpenses, error)

	FinanceCostTypes(ctx context.Context) ([]FinanceCostType, error)
	FinanceInvoices(ctx context.Context, f FinanceInvoiceFilter) ([]FinanceInvoice, int, error)
	FinanceInvoice(ctx context.Context, id int) (FinanceInvoice, error)
	FinanceInvoiceCounts(ctx context.Context, agencyID int) (FinanceInvoiceCounts, error)
	FinanceAddInvoices(ctx context.Context, jobNumber string, in FinanceInvoicesCreate) ([]FinanceInvoice, error)
	FinanceDecideInvoices(ctx context.Context, d FinanceInvoiceDecision) ([]FinanceDecisionResult, error)
	FinancePushInvoice(ctx context.Context, id int, actor string) (FinancePushResult, error)
	FinanceResyncInvoice(ctx context.Context, id int, actor string) (FinancePushResult, error)
	FinanceInvoiceReference(ctx context.Context, id int) (*string, error)

	FinancePayments(ctx context.Context, f FinancePaymentFilter) ([]FinancePayment, int, error)
	FinancePayment(ctx context.Context, perDiemNumber string) (FinancePayment, error)
	FinancePaymentCounts(ctx context.Context) (FinancePaymentCounts, error)
	FinanceAddPayment(ctx context.Context, jobNumber string, in FinancePaymentCreate) (FinancePayment, error)
	FinanceUpdatePayment(ctx context.Context, perDiemNumber string, in FinancePaymentUpdate) (FinancePayment, error)
	FinancePushPayment(ctx context.Context, perDiemNumber, actor string, employee FinanceEmployee) (FinancePushResult, error)
	FinanceResyncPayment(ctx context.Context, perDiemNumber, actor string) (FinancePushResult, error)
	FinancePaymentReference(ctx context.Context, perDiemNumber string) (*string, error)

	FinanceSubsidiary(ctx context.Context, company string) (FinanceSubsidiary, error)
}

// FinanceCampaign is a marketing event trip's campaign.
type FinanceCampaign struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// FinanceTrip is a travel request's finance facts. Dates are
// "YYYY-MM-DD HH:MM:SS".
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

// FinancePerDiem is a per diem allocation.
type FinancePerDiem struct {
	ID             int     `json:"id"`
	EngagementCode string  `json:"engagementCode"`
	JobNumber      string  `json:"jobNumber"`
	Currency       *string `json:"currency"`
	PerDiemValue   float64 `json:"perDiemValue"`
	Total          float64 `json:"total"`
	StartDate      *string `json:"startDate"`
	EndDate        *string `json:"endDate"`
	NoOfDays       *int    `json:"noOfDays"`
	Status         string  `json:"status"`
}

// FinancePerDiemCreate adds a per diem (dates "YYYY-MM-DD HH:MM:SS").
type FinancePerDiemCreate struct {
	Currency     string  `json:"currency"`
	PerDiemValue float64 `json:"perDiemValue"`
	Total        float64 `json:"total"`
	StartDate    string  `json:"startDate"`
	EndDate      string  `json:"endDate"`
	NoOfDays     int     `json:"noOfDays"`
	CreatedBy    string  `json:"createdBy"`
}

// FinanceAmount is an amount in a currency on a date.
type FinanceAmount struct {
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	AddedDate string  `json:"addedDate"`
}

// FinancePerDiemTotal is a trip's per diem total in one currency.
type FinancePerDiemTotal struct {
	Currency    string  `json:"currency"`
	TotalAmount float64 `json:"totalAmount"`
}

// FinancePaymentBasis is what the per diem balance is computed from.
type FinancePaymentBasis struct {
	PerDiemTotals    []FinancePerDiemTotal `json:"perDiemTotals"`
	CurrencyInvoices []FinanceAmount       `json:"currencyInvoices"`
	TravelCards      []FinanceAmount       `json:"travelCards"`
	Payments         []FinanceAmount       `json:"payments"`
	PaymentCount     int                   `json:"paymentCount"`
}

// FinanceTravelCard is a travel card.
// FinanceCardExpense is a credit card transaction tagged with a travel.
type FinanceCardExpense struct {
	ID            int      `json:"id"`
	Date          *string  `json:"date"`
	ExpenseType   *string  `json:"expenseType"`
	Reference     *string  `json:"reference"`
	Amount        *float64 `json:"amount"`
	Currency      *string  `json:"currency"`
	EmployeeEmail *string  `json:"employeeEmail"`
	Status        *string  `json:"status"`
}

// FinanceExpenseClaim is an expense claim line tagged with a travel. Status
// is the legacy code: 0 pending approval, 1 rejected, 2 lead approved, 3
// approved.
type FinanceExpenseClaim struct {
	ID                    int      `json:"id"`
	ClaimNumber           *string  `json:"claimNumber"`
	Date                  *string  `json:"date"`
	ExpenseType           *string  `json:"expenseType"`
	Amount                *float64 `json:"amount"`
	Currency              *string  `json:"currency"`
	ReimbursementAmount   *float64 `json:"reimbursementAmount"`
	ReimbursementCurrency *string  `json:"reimbursementCurrency"`
	EmployeeEmail         *string  `json:"employeeEmail"`
	Status                *string  `json:"status"`
}

// FinanceTripExpenses is a travel's card spend and expense claims.
type FinanceTripExpenses struct {
	CardExpenses  []FinanceCardExpense  `json:"cardExpenses"`
	ExpenseClaims []FinanceExpenseClaim `json:"expenseClaims"`
}

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

// FinanceCostType is an active invoice cost type.
type FinanceCostType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// FinanceInvoice is an invoice with its trip.
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
	NetSuiteFailed          bool             `json:"netsuiteFailed"`
	NetSuitePushing         bool             `json:"netsuitePushing"`
	NetSuiteError           *string          `json:"netsuiteError"`
	NetSuiteReference       *string          `json:"netsuiteReference"`
	Campaign                *FinanceCampaign `json:"campaign"`
}

// FinanceInvoiceFilter narrows FinanceInvoices. Zero values mean "any".
type FinanceInvoiceFilter struct {
	JobNumber   string
	Statuses    []string
	Failed      string // "true", "false" or ""
	Search      string
	CostTypes   []string
	Agencies    []string
	Months      []string
	CampaignIDs []string
	AgencyID    int
	AddedFrom   string
	AddedTo     string
	// Limit 0 reads every match (up to the entity's maximum).
	Limit  int
	Offset int
}

// FinanceInvoiceCounts are the invoice queue sizes.
type FinanceInvoiceCounts struct {
	Pending           int `json:"pending"`
	AcceptedByTravel  int `json:"acceptedByTravel"`
	AcceptedByFinance int `json:"acceptedByFinance"`
	Failed            int `json:"failed"`
	All               int `json:"all"`
}

// FinanceInvoiceCreate is one invoice to add.
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

// FinanceInvoiceDecision approves or rejects invoices at one step
// (step "travel" = PO approval, "finance" = finance approval).
type FinanceInvoiceDecision struct {
	InvoiceIDs []int  `json:"invoiceIds"`
	Decision   string `json:"decision"`
	Step       string `json:"step"`
	Reason     string `json:"reason,omitempty"`
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
	Campaign          *FinanceCampaign `json:"campaign"`
}

// FinancePaymentFilter narrows FinancePayments.
type FinancePaymentFilter struct {
	JobNumber         string
	NSStatuses        []string
	Search            string
	Travellers        []string
	ConvertedCurrency []string
	CampaignIDs       []string
	AddedFrom         string
	AddedTo           string
	Limit             int
	Offset            int
}

// FinancePaymentCounts are the payment queue sizes.
type FinancePaymentCounts struct {
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
	All     int `json:"all"`
}

// FinancePaymentCreate adds a converted payment.
type FinancePaymentCreate struct {
	PerDiemNumber     string  `json:"perDiemNumber"`
	PaymentCurrency   string  `json:"paymentCurrency"`
	PaymentAmount     float64 `json:"paymentAmount"`
	ConvertedCurrency string  `json:"convertedCurrency"`
	ConvertedAmount   float64 `json:"convertedAmount"`
	CreatedBy         string  `json:"createdBy"`
}

// FinancePaymentUpdate edits a pending payment.
type FinancePaymentUpdate struct {
	PaymentAmount     float64 `json:"paymentAmount"`
	ConvertedCurrency string  `json:"convertedCurrency"`
	ConvertedAmount   float64 `json:"convertedAmount"`
	UpdatedBy         string  `json:"updatedBy"`
}

// FinanceEmployee is the traveller's HR record sent with a payment push.
type FinanceEmployee struct {
	EmployeeID string `json:"employeeId"`
	Email      string `json:"email"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	JobRole    string `json:"jobRole"`
	Department string `json:"department"`
	Company    string `json:"company"`
}

// NetSuite outcomes of a push or re-sync (entity values).
const (
	FinanceOutcomePushed   = "pushed"
	FinanceOutcomeFound    = "found"
	FinanceOutcomeNotFound = "not_found"
	FinanceOutcomeFailed   = "failed"
	FinanceOutcomeSkipped  = "skipped"
)

// FinancePushResult is one record's push or re-sync outcome.
type FinancePushResult struct {
	ID        string  `json:"id"`
	Number    string  `json:"number"`
	JobNumber string  `json:"jobNumber"`
	OK        bool    `json:"ok"`
	Outcome   string  `json:"outcome"`
	Reference *string `json:"reference"`
	Message   *string `json:"message"`
}

// FinanceSubsidiary is an expense_app subsidiary.
type FinanceSubsidiary struct {
	ID                int    `json:"id"`
	LegalName         string `json:"legalName"`
	CurrencyCode      string `json:"currencyCode"`
	Country           string `json:"country"`
	TaxCodeInternalID string `json:"taxCodeInternalId"`
}

func financeTripPath(jobNumber string, rest ...string) string {
	p := "/finance/travel-requests/" + pathSegment(jobNumber)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// FinanceTrip calls GET /finance/travel-requests/{jobNumber}.
func (c *HTTPClient) FinanceTrip(ctx context.Context, jobNumber string) (FinanceTrip, error) {
	var out FinanceTrip
	err := c.rest.Get(ctx, financeTripPath(jobNumber), &out)
	return out, err
}

// FinancePerDiems calls GET /finance/travel-requests/{jobNumber}/per-diems.
func (c *HTTPClient) FinancePerDiems(ctx context.Context, jobNumber string) ([]FinancePerDiem, error) {
	var out struct {
		PerDiems []FinancePerDiem `json:"perDiems"`
	}
	err := c.rest.Get(ctx, financeTripPath(jobNumber, "per-diems"), &out)
	return out.PerDiems, err
}

// FinancePerDiem calls GET /finance/travel-requests/{jobNumber}/per-diems/{id}.
func (c *HTTPClient) FinancePerDiem(ctx context.Context, jobNumber string, id int) (FinancePerDiem, error) {
	var out FinancePerDiem
	err := c.rest.Get(ctx, financeTripPath(jobNumber, "per-diems", strconv.Itoa(id)), &out)
	return out, err
}

// FinanceAddPerDiem calls POST /finance/travel-requests/{jobNumber}/per-diems.
func (c *HTTPClient) FinanceAddPerDiem(ctx context.Context, jobNumber string, in FinancePerDiemCreate) (FinancePerDiem, error) {
	var out FinancePerDiem
	err := c.rest.Post(ctx, financeTripPath(jobNumber, "per-diems"), in, &out)
	return out, err
}

// FinanceDeletePerDiem calls DELETE /finance/travel-requests/{jobNumber}/per-diems/{id}.
func (c *HTTPClient) FinanceDeletePerDiem(ctx context.Context, jobNumber string, id int) error {
	return c.rest.Do(ctx, http.MethodDelete, financeTripPath(jobNumber, "per-diems", strconv.Itoa(id)), nil, nil)
}

// FinancePaymentBasis calls GET /finance/travel-requests/{jobNumber}/payment-basis.
func (c *HTTPClient) FinancePaymentBasis(ctx context.Context, jobNumber string) (FinancePaymentBasis, error) {
	var out FinancePaymentBasis
	err := c.rest.Get(ctx, financeTripPath(jobNumber, "payment-basis"), &out)
	return out, err
}

// FinanceTripExpenses calls GET /finance/travel-requests/{jobNumber}/expenses.
func (c *HTTPClient) FinanceTripExpenses(ctx context.Context, jobNumber string) (FinanceTripExpenses, error) {
	var out FinanceTripExpenses
	err := c.rest.Get(ctx, financeTripPath(jobNumber, "expenses"), &out)
	return out, err
}

// FinanceTravelCards calls GET /finance/travel-requests/{jobNumber}/travel-cards.
func (c *HTTPClient) FinanceTravelCards(ctx context.Context, jobNumber string) ([]FinanceTravelCard, error) {
	var out struct {
		TravelCards []FinanceTravelCard `json:"travelCards"`
	}
	err := c.rest.Get(ctx, financeTripPath(jobNumber, "travel-cards"), &out)
	return out.TravelCards, err
}

// FinanceAddTravelCard calls POST /finance/travel-requests/{jobNumber}/travel-cards.
func (c *HTTPClient) FinanceAddTravelCard(ctx context.Context, jobNumber string, in FinanceTravelCardCreate) (FinanceTravelCard, error) {
	var out FinanceTravelCard
	err := c.rest.Post(ctx, financeTripPath(jobNumber, "travel-cards"), in, &out)
	return out, err
}

// FinanceCostTypes calls GET /finance/invoice-cost-types.
func (c *HTTPClient) FinanceCostTypes(ctx context.Context) ([]FinanceCostType, error) {
	var out struct {
		CostTypes []FinanceCostType `json:"costTypes"`
	}
	err := c.rest.Get(ctx, "/finance/invoice-cost-types", &out)
	return out.CostTypes, err
}

func financeAdd(q url.Values, key string, vals []string) {
	for _, v := range vals {
		q.Add(key, v)
	}
}

func financePage(q url.Values, limit, offset int) {
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
}

// FinanceInvoices calls GET /finance/invoices.
func (c *HTTPClient) FinanceInvoices(ctx context.Context, f FinanceInvoiceFilter) ([]FinanceInvoice, int, error) {
	q := url.Values{}
	if f.JobNumber != "" {
		q.Set("jobNumber", f.JobNumber)
	}
	financeAdd(q, "status", f.Statuses)
	if f.Failed != "" {
		q.Set("failed", f.Failed)
	}
	if f.Search != "" {
		q.Set("search", f.Search)
	}
	financeAdd(q, "costType", f.CostTypes)
	financeAdd(q, "agency", f.Agencies)
	financeAdd(q, "month", f.Months)
	financeAdd(q, "campaignId", f.CampaignIDs)
	if f.AgencyID > 0 {
		q.Set("agencyId", strconv.Itoa(f.AgencyID))
	}
	if f.AddedFrom != "" {
		q.Set("addedFrom", f.AddedFrom)
	}
	if f.AddedTo != "" {
		q.Set("addedTo", f.AddedTo)
	}
	financePage(q, f.Limit, f.Offset)
	var out struct {
		Invoices []FinanceInvoice `json:"invoices"`
		Total    int              `json:"total"`
	}
	err := c.rest.Get(ctx, withQuery("/finance/invoices", q), &out)
	return out.Invoices, out.Total, err
}

// FinanceInvoice calls GET /finance/invoices/{id}.
func (c *HTTPClient) FinanceInvoice(ctx context.Context, id int) (FinanceInvoice, error) {
	var out FinanceInvoice
	err := c.rest.Get(ctx, "/finance/invoices/"+strconv.Itoa(id), &out)
	return out, err
}

// FinanceInvoiceCounts calls GET /finance/invoices/counts.
func (c *HTTPClient) FinanceInvoiceCounts(ctx context.Context, agencyID int) (FinanceInvoiceCounts, error) {
	q := url.Values{}
	if agencyID > 0 {
		q.Set("agencyId", strconv.Itoa(agencyID))
	}
	var out FinanceInvoiceCounts
	err := c.rest.Get(ctx, withQuery("/finance/invoices/counts", q), &out)
	return out, err
}

// FinanceAddInvoices calls POST /finance/travel-requests/{jobNumber}/invoices.
func (c *HTTPClient) FinanceAddInvoices(ctx context.Context, jobNumber string, in FinanceInvoicesCreate) ([]FinanceInvoice, error) {
	var out struct {
		Invoices []FinanceInvoice `json:"invoices"`
	}
	err := c.rest.Post(ctx, financeTripPath(jobNumber, "invoices"), in, &out)
	return out.Invoices, err
}

// FinanceDecideInvoices calls POST /finance/invoices/decisions.
func (c *HTTPClient) FinanceDecideInvoices(ctx context.Context, d FinanceInvoiceDecision) ([]FinanceDecisionResult, error) {
	var out struct {
		Results []FinanceDecisionResult `json:"results"`
	}
	err := c.rest.Post(ctx, "/finance/invoices/decisions", d, &out)
	return out.Results, err
}

type financeActor struct {
	UpdatedBy string           `json:"updatedBy"`
	Employee  *FinanceEmployee `json:"employee,omitempty"`
}

// FinancePushInvoice calls POST /finance/invoices/{id}/push.
func (c *HTTPClient) FinancePushInvoice(ctx context.Context, id int, actor string) (FinancePushResult, error) {
	var out FinancePushResult
	err := c.rest.Post(ctx, "/finance/invoices/"+strconv.Itoa(id)+"/push", financeActor{UpdatedBy: actor}, &out)
	return out, err
}

// FinanceResyncInvoice calls POST /finance/invoices/{id}/resync.
func (c *HTTPClient) FinanceResyncInvoice(ctx context.Context, id int, actor string) (FinancePushResult, error) {
	var out FinancePushResult
	err := c.rest.Post(ctx, "/finance/invoices/"+strconv.Itoa(id)+"/resync", financeActor{UpdatedBy: actor}, &out)
	return out, err
}

// FinanceInvoiceReference calls GET /finance/invoices/{id}/netsuite-reference.
func (c *HTTPClient) FinanceInvoiceReference(ctx context.Context, id int) (*string, error) {
	var out struct {
		Reference *string `json:"reference"`
	}
	err := c.rest.Get(ctx, "/finance/invoices/"+strconv.Itoa(id)+"/netsuite-reference", &out)
	return out.Reference, err
}

// FinancePayments calls GET /finance/per-diem-payments.
func (c *HTTPClient) FinancePayments(ctx context.Context, f FinancePaymentFilter) ([]FinancePayment, int, error) {
	q := url.Values{}
	if f.JobNumber != "" {
		q.Set("jobNumber", f.JobNumber)
	}
	financeAdd(q, "nsStatus", f.NSStatuses)
	if f.Search != "" {
		q.Set("search", f.Search)
	}
	financeAdd(q, "traveller", f.Travellers)
	financeAdd(q, "convertedCurrency", f.ConvertedCurrency)
	financeAdd(q, "campaignId", f.CampaignIDs)
	if f.AddedFrom != "" {
		q.Set("addedFrom", f.AddedFrom)
	}
	if f.AddedTo != "" {
		q.Set("addedTo", f.AddedTo)
	}
	financePage(q, f.Limit, f.Offset)
	var out struct {
		Payments []FinancePayment `json:"payments"`
		Total    int              `json:"total"`
	}
	err := c.rest.Get(ctx, withQuery("/finance/per-diem-payments", q), &out)
	return out.Payments, out.Total, err
}

func financePaymentPath(perDiemNumber string, rest ...string) string {
	p := "/finance/per-diem-payments/" + pathSegment(perDiemNumber)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

// FinancePayment calls GET /finance/per-diem-payments/{perDiemNumber}.
func (c *HTTPClient) FinancePayment(ctx context.Context, perDiemNumber string) (FinancePayment, error) {
	var out FinancePayment
	err := c.rest.Get(ctx, financePaymentPath(perDiemNumber), &out)
	return out, err
}

// FinancePaymentCounts calls GET /finance/per-diem-payments/counts.
func (c *HTTPClient) FinancePaymentCounts(ctx context.Context) (FinancePaymentCounts, error) {
	var out FinancePaymentCounts
	err := c.rest.Get(ctx, "/finance/per-diem-payments/counts", &out)
	return out, err
}

// FinanceAddPayment calls POST /finance/travel-requests/{jobNumber}/per-diem-payments.
func (c *HTTPClient) FinanceAddPayment(ctx context.Context, jobNumber string, in FinancePaymentCreate) (FinancePayment, error) {
	var out FinancePayment
	err := c.rest.Post(ctx, financeTripPath(jobNumber, "per-diem-payments"), in, &out)
	return out, err
}

// FinanceUpdatePayment calls PATCH /finance/per-diem-payments/{perDiemNumber}.
func (c *HTTPClient) FinanceUpdatePayment(ctx context.Context, perDiemNumber string, in FinancePaymentUpdate) (FinancePayment, error) {
	var out FinancePayment
	err := c.rest.Do(ctx, http.MethodPatch, financePaymentPath(perDiemNumber), in, &out)
	return out, err
}

// FinancePushPayment calls POST /finance/per-diem-payments/{perDiemNumber}/push.
func (c *HTTPClient) FinancePushPayment(ctx context.Context, perDiemNumber, actor string, employee FinanceEmployee) (FinancePushResult, error) {
	var out FinancePushResult
	err := c.rest.Post(ctx, financePaymentPath(perDiemNumber, "push"), financeActor{UpdatedBy: actor, Employee: &employee}, &out)
	return out, err
}

// FinanceResyncPayment calls POST /finance/per-diem-payments/{perDiemNumber}/resync.
func (c *HTTPClient) FinanceResyncPayment(ctx context.Context, perDiemNumber, actor string) (FinancePushResult, error) {
	var out FinancePushResult
	err := c.rest.Post(ctx, financePaymentPath(perDiemNumber, "resync"), financeActor{UpdatedBy: actor}, &out)
	return out, err
}

// FinancePaymentReference calls GET /finance/per-diem-payments/{perDiemNumber}/netsuite-reference.
func (c *HTTPClient) FinancePaymentReference(ctx context.Context, perDiemNumber string) (*string, error) {
	var out struct {
		Reference *string `json:"reference"`
	}
	err := c.rest.Get(ctx, financePaymentPath(perDiemNumber, "netsuite-reference"), &out)
	return out.Reference, err
}

// FinanceSubsidiary calls GET /finance/subsidiaries/{company}.
func (c *HTTPClient) FinanceSubsidiary(ctx context.Context, company string) (FinanceSubsidiary, error) {
	var out FinanceSubsidiary
	err := c.rest.Get(ctx, "/finance/subsidiaries/"+pathSegment(company), &out)
	return out, err
}
