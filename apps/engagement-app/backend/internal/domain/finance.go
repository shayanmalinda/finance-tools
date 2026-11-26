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

// Travel privileges of the finance area (legacy Travel app constants).
const (
	TravelPrivAgentTravelRequests = 18
	TravelPrivAddPerDiems         = 26
	TravelPrivDeletePerDiems      = 27
	TravelPrivViewPerDiems        = 28
	TravelPrivViewInvoices        = 30
	TravelPrivAddInvoices         = 31
	TravelPrivInvoiceHRApproval   = 32
	TravelPrivInvoiceFinance      = 33
	TravelPrivPushInvoices        = 34
	TravelPrivViewOwnPerDiems     = 37
	TravelPrivViewOwnInvoices     = 38
	TravelPrivViewAllInvoicesPage = 39
	TravelPrivPushPerDiemPayments = 40
	TravelPrivViewAllPaymentsPage = 41
	TravelPrivViewPerDiemPayments = 42
	TravelPrivAddPerDiemPayments  = 43
	TravelPrivEditPerDiemPayment  = 44
	TravelPrivViewTravelCards     = 45
	TravelPrivAddTravelCards      = 46
	TravelPrivViewOwnPayments     = 47
	TravelPrivViewOwnTravelCards  = 48
)

// NetSuiteState is a record's NetSuite status as the screens show it.
type NetSuiteState struct {
	// Status is Pending, Pushing, Pushed or Failed.
	Status    string  `json:"status"`
	Reference *string `json:"reference"`
	Error     *string `json:"error"`
}

// NetSuite statuses.
const (
	NetSuitePending = "Pending"
	NetSuitePushing = "Pushing"
	NetSuitePushed  = "Pushed"
	NetSuiteFailed  = "Failed"
)

// Approval states of the invoice PO and finance chips.
const (
	ApprovalPending  = "Pending"
	ApprovalApproved = "Approved"
	ApprovalRejected = "Rejected"
)

// PerDiem is a per diem allocation of a travel request.
type PerDiem struct {
	ID        int     `json:"id"`
	JobNumber string  `json:"jobNumber"`
	StartDate string  `json:"startDate"`
	StartTime string  `json:"startTime"`
	EndDate   string  `json:"endDate"`
	EndTime   string  `json:"endTime"`
	Days      float64 `json:"days"`
	Currency  string  `json:"currency"`
	Rate      float64 `json:"rate"`
	Total     float64 `json:"total"`
	// AddedBy is not stored on the per diem (only in perdiem_auditlog).
	AddedBy   *string `json:"addedBy"`
	CanDelete bool    `json:"canDelete"`
}

// PerDiemCreate is the Add Per Diem dialog.
type PerDiemCreate struct {
	StartDate string  `json:"startDate"`
	StartTime string  `json:"startTime"`
	EndDate   string  `json:"endDate"`
	EndTime   string  `json:"endTime"`
	Currency  string  `json:"currency"`
	Rate      float64 `json:"rate"`
}

// TravelCard is a travel card of a travel request.
type TravelCard struct {
	ID        int     `json:"id"`
	JobNumber string  `json:"jobNumber"`
	Currency  string  `json:"currency"`
	Amount    float64 `json:"amount"`
	AddedBy   string  `json:"addedBy"`
	AddedOn   string  `json:"addedOn"`
}

// CardExpense is a credit card transaction tagged with a travel. Status is
// expense_app's (new, pending_lead, pending_finance, submitted).
type CardExpense struct {
	ID            int      `json:"id"`
	Date          *string  `json:"date"`
	ExpenseType   *string  `json:"expenseType"`
	Reference     *string  `json:"reference"`
	Amount        *float64 `json:"amount"`
	Currency      *string  `json:"currency"`
	EmployeeEmail *string  `json:"employeeEmail"`
	Status        *string  `json:"status"`
}

// ExpenseClaim is an expense claim line tagged with a travel. Status is the
// legacy code: 0 pending approval, 1 rejected, 2 lead approved, 3 approved.
type ExpenseClaim struct {
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

// TripExpenses is a travel's card spend and expense claims.
type TripExpenses struct {
	CardExpenses  []CardExpense  `json:"cardExpenses"`
	ExpenseClaims []ExpenseClaim `json:"expenseClaims"`
}

// TravelCardCreate is the Add Travel Card dialog.
type TravelCardCreate struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// InvoiceCreate is one row of the Add Invoice dialog.
type InvoiceCreate struct {
	InvoiceNumber string  `json:"invoiceNumber"`
	Month         string  `json:"month"`
	CostType      string  `json:"costType"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	// PaidAmount and PaidCurrency are optional, as in legacy.
	PaidAmount   *float64 `json:"paidAmount"`
	PaidCurrency *string  `json:"paidCurrency"`
}

// InvoiceActions are what the caller may do with an invoice.
type InvoiceActions struct {
	ApprovePo bool `json:"approvePo"`
	Approve   bool `json:"approve"`
	Reject    bool `json:"reject"`
	Push      bool `json:"push"`
	Resync    bool `json:"resync"`
}

// Invoice is an agency invoice.
type Invoice struct {
	ID                int            `json:"id"`
	InvoiceNumber     string         `json:"invoiceNumber"`
	JobNumber         string         `json:"jobNumber"`
	Traveller         *Person        `json:"traveller"`
	Agency            *string        `json:"agency"`
	Month             string         `json:"month"`
	CostType          string         `json:"costType"`
	Amount            float64        `json:"amount"`
	Currency          string         `json:"currency"`
	PaidAmount        float64        `json:"paidAmount"`
	PaidCurrency      string         `json:"paidCurrency"`
	Status            string         `json:"status"`
	PoApproval        string         `json:"poApproval"`
	FinanceApproval   string         `json:"financeApproval"`
	PoApprovedBy      *string        `json:"poApprovedBy"`
	FinanceApprovedBy *string        `json:"financeApprovedBy"`
	RejectReason      *string        `json:"rejectReason"`
	NetSuite          NetSuiteState  `json:"netsuite"`
	Campaign          *CampaignRef   `json:"campaign"`
	Destination       *string        `json:"destination"`
	TravelStartDate   *string        `json:"travelStartDate"`
	TravelEndDate     *string        `json:"travelEndDate"`
	AddedOn           *string        `json:"addedOn"`
	AddedBy           *string        `json:"addedBy"`
	Actions           InvoiceActions `json:"actions"`
	// AgentEmail is the legacy "Traveller WSO2 Email" CSV value (the agent
	// who added the invoice); not in JSON.
	AgentEmail *string `json:"-"`
}

// InvoicePage is a page of invoices with the size of each tab.
type InvoicePage struct {
	Items     []Invoice      `json:"items"`
	Total     int            `json:"total"`
	TabCounts map[string]int `json:"tabCounts,omitempty"`
}

// PaymentActions are what the caller may do with a payment.
type PaymentActions struct {
	Edit   bool `json:"edit"`
	Push   bool `json:"push"`
	Resync bool `json:"resync"`
}

// PerDiemPayment is a per diem payment; its id is the per diem number.
type PerDiemPayment struct {
	ID                string         `json:"id"`
	JobNumber         string         `json:"jobNumber"`
	Traveller         *Person        `json:"traveller"`
	Amount            float64        `json:"amount"`
	Currency          string         `json:"currency"`
	ConvertedAmount   *float64       `json:"convertedAmount"`
	ConvertedCurrency *string        `json:"convertedCurrency"`
	Status            string         `json:"status"`
	NetSuite          NetSuiteState  `json:"netsuite"`
	Campaign          *CampaignRef   `json:"campaign"`
	AddedOn           *string        `json:"addedOn"`
	AddedBy           *string        `json:"addedBy"`
	Actions           PaymentActions `json:"actions"`
}

// PerDiemPaymentPage is a page of payments with the size of each tab.
type PerDiemPaymentPage struct {
	Items     []PerDiemPayment `json:"items"`
	Total     int              `json:"total"`
	TabCounts map[string]int   `json:"tabCounts,omitempty"`
}

// PaymentSummary is the Payment Summary of a travel request.
type PaymentSummary struct {
	Currency              string  `json:"currency"`
	TotalPerDiemAmount    float64 `json:"totalPerDiemAmount"`
	InitialCurrencyPaid   float64 `json:"initialCurrencyPaid"`
	TotalTravelCardAmount float64 `json:"totalTravelCardAmount"`
	TotalPaymentAmount    float64 `json:"totalPaymentAmount"`
	RemainingBalance      float64 `json:"remainingBalance"`
	// ConvertedCurrency is the traveller's subsidiary currency, the
	// currency payments are converted to (null when unknown).
	ConvertedCurrency *string `json:"convertedCurrency"`
}

// TravelRequestPayments is the Per Diem Payments tab.
type TravelRequestPayments struct {
	Items   []PerDiemPayment `json:"items"`
	Summary *PaymentSummary  `json:"summary"`
}

// SyncFailure is a row of NS Sync Failures.
type SyncFailure struct {
	Record    string        `json:"record"`
	ID        string        `json:"id"`
	Number    string        `json:"number"`
	Agency    *string       `json:"agency"`
	JobNumber string        `json:"jobNumber"`
	Traveller *Person       `json:"traveller"`
	Campaign  *CampaignRef  `json:"campaign"`
	Amount    float64       `json:"amount"`
	Currency  string        `json:"currency"`
	NetSuite  NetSuiteState `json:"netsuite"`
}

// SyncFailurePage is NS Sync Failures.
type SyncFailurePage struct {
	Items []SyncFailure `json:"items"`
	Total int           `json:"total"`
}

// BatchItem is one record's outcome of a bulk action.
type BatchItem struct {
	ID                string  `json:"id"`
	OK                bool    `json:"ok"`
	Message           *string `json:"message"`
	NetSuiteReference *string `json:"netsuiteReference"`
}

// BatchResult is the per-record outcome of a bulk action.
type BatchResult struct {
	Results []BatchItem `json:"results"`
}

// InvoiceQuery is GET /invoices.
type InvoiceQuery struct {
	Tab             string
	Search          string
	PoApproval      []string
	FinanceApproval []string
	NetSuiteStatus  []string
	CostTypes       []string
	Agencies        []string
	Months          []string
	CampaignIDs     []string
	// Sort is one of InvoiceSorts; empty or unknown keeps newest first.
	Sort          string
	Limit, Offset int
}

// PaymentQuery is GET /per-diem-payments.
type PaymentQuery struct {
	Tab               string
	Search            string
	NetSuiteStatus    []string
	Travellers        []string
	ConvertedCurrency []string
	CampaignIDs       []string
	// Sort is one of PaymentSorts; empty or unknown keeps newest first.
	Sort          string
	Limit, Offset int
}
