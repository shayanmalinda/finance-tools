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

package netsuite

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Timeout is the legacy NetSuite client timeout (300 s).
const Timeout = 300 * time.Second

const (
	restletPath  = "/app/site/hosting/restlet.nl"
	maxErrorText = 2 << 10
	maxBody      = 8 << 20
)

// ErrDisabled is returned by every call when NetSuite is switched off.
var ErrDisabled = errors.New("NetSuite is not enabled (NETSUITE_PUSH_ENABLED is false)")

// ErrEventExists is NetSuite refusing a marketing event whose name it has.
var ErrEventExists = errors.New("Event already exists in NetSuite")

// Record is a transaction a lookup found: its NetSuite internal id and its
// transaction id (the invoice or per diem number).
type Record struct {
	InternalID    string
	TransactionID string
}

// FundingSource is one funding line of a purchase order or vendor bill
// (legacy FundingSourceInInvoice). Region is the engagement's sub region.
type FundingSource struct {
	BusinessUnit *string `json:"businessUnit"`
	ProductUnit  *string `json:"productUnit"`
	Region       *string `json:"region"`
	Percentage   string  `json:"percentage"`
}

// PurchaseOrder is the legacy PurchaseOrderInput, sent as is except
// TransactionDate, which Create rewrites from YYYY-MM-DD to "04 November,
// 2026" (legacy transformClaimDate).
type PurchaseOrder struct {
	TransactionID   string          `json:"transactionId"`
	Department      string          `json:"department"`
	EngagementCode  string          `json:"engagementCode"`
	TransactionDate string          `json:"transactionDate"`
	VendorID        string          `json:"vendorId"`
	TotalAmount     string          `json:"totalAmount"`
	BillCurrency    string          `json:"billCurrency"`
	Memo            string          `json:"memo"`
	Account         string          `json:"account"`
	TaxCode         string          `json:"taxCode"`
	FundingSources  []FundingSource `json:"fundingSources"`
	POType          string          `json:"poType"`
	JobNumber       string          `json:"jobNumber"`
	InvoiceNumber   string          `json:"invoiceNumber"`
	ApproverEmail   string          `json:"approverEmail"`
}

// Employee is the legacy NetSuitePerDiemApprover: the traveller (not the
// person pushing) with their subsidiary's legal name.
type Employee struct {
	Email      string `json:"email"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	JobRole    string `json:"jobRole"`
	EmployeeID string `json:"employeeId"`
	Department string `json:"department"`
	Subsidiary string `json:"subsidiary"`
}

// VendorBill is the legacy PerDiemAllocationVendorBillInput; TransactionDate
// is rewritten as for PurchaseOrder.
type VendorBill struct {
	TransactionID   string          `json:"transactionId"`
	Department      string          `json:"department"`
	EngagementCode  string          `json:"engagementCode"`
	TransactionDate string          `json:"transactionDate"`
	TotalAmount     string          `json:"totalAmount"`
	BillCurrency    string          `json:"billCurrency"`
	Memo            string          `json:"memo"`
	Employee        Employee        `json:"employee"`
	Account         string          `json:"account"`
	TaxCode         string          `json:"taxCode"`
	FundingSources  []FundingSource `json:"fundingSources"`
}

// Client calls the RESTlets.
type Client struct {
	cfg   Config
	http  *http.Client
	now   func() time.Time
	nonce func() string
}

// New returns a Client. httpClient nil uses the legacy transport: HTTP/1.1,
// no keep-alive, 300 s timeout.
func New(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DisableKeepAlives = true
		tr.ForceAttemptHTTP2 = false
		// Offer only HTTP/1.1 in the TLS handshake; the cloned default still advertises h2.
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.NextProtos = []string{"http/1.1"}
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		httpClient = &http.Client{Timeout: Timeout, Transport: tr}
	}
	return &Client{cfg: cfg, http: httpClient, now: time.Now, nonce: newNonce}
}

// Enabled reports whether NetSuite calls are switched on.
func (c *Client) Enabled() bool { return c != nil && c.cfg.Enabled }

// PurchaseOrderTaxCode is the configured purchase order tax code.
func (c *Client) PurchaseOrderTaxCode() string { return c.cfg.PurchaseOrderTaxCode }

// newNonce is the legacy uuid:createType4AsString().substring(0, 8): eight
// lower-case hex digits.
func newNonce() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// url builds {base}/app/site/hosting/restlet.nl?script=..&deploy=..[&k=v],
// each value form-encoded, in this order (legacy getPathAndQueryParams).
func (c *Client) url(s Script, extra ...[2]string) string {
	q := "?script=" + formEncode(s.ID) + "&deploy=" + formEncode(s.Deploy)
	for _, kv := range extra {
		q += "&" + kv[0] + "=" + formEncode(kv[1])
	}
	return c.cfg.RestletsBaseURL + restletPath + q
}

func (c *Client) do(ctx context.Context, method, rawURL string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("netsuite: marshal: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return 0, nil, fmt.Errorf("netsuite: build request: %w", err)
	}
	ts := strconv.FormatInt(c.now().Unix(), 10)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", oauthHeader(c.cfg, method, rawURL, c.nonce(), ts))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

// LookupPurchaseOrders ports getPurchaseOrders: GET getPurchaseOrder with
// transactionId = the invoice number.
func (c *Client) LookupPurchaseOrders(ctx context.Context, transactionID string) ([]Record, error) {
	return c.lookup(ctx, c.cfg.GetPurchaseOrder, transactionID, "purchaseOrders", "purchaseOrder", false)
}

// LookupVendorBills ports getVendorBills: GET getVendorBill with
// transactionId = the per diem number.
func (c *Client) LookupVendorBills(ctx context.Context, transactionID string) ([]Record, error) {
	return c.lookup(ctx, c.cfg.GetVendorBill, transactionID, "vendorBills", "vendorBill", true)
}

func (c *Client) lookup(ctx context.Context, s Script, transactionID, field, what string, matchID bool) ([]Record, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	status, raw, err := c.do(ctx, http.MethodGet, c.url(s, [2]string{"transactionId", transactionID}), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to fetch %s from NetSuite. %w", what, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Failed to fetch %s from NetSuite. Status code: %d Details: %s", what, status, clip(raw))
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("Failed to fetch %s from NetSuite. Unexpected response: %s", what, clip(raw))
	}
	var items []map[string]json.RawMessage
	if list, ok := payload[field]; ok && string(list) != "null" {
		if err := json.Unmarshal(list, &items); err != nil {
			return nil, fmt.Errorf("Failed to fetch %s from NetSuite. Unexpected response: %s", what, clip(raw))
		}
	} else {
		return nil, fmt.Errorf("Failed to fetch %s from NetSuite. Unexpected response: %s", what, clip(raw))
	}
	out := make([]Record, 0, len(items))
	for _, it := range items {
		r := Record{InternalID: scalar(it["internalId"]), TransactionID: scalar(it["transactionId"])}
		// The PO script matches on Vendor # and returns the PO's own number,
		// so only vendor bills are checked against the transaction id.
		if matchID && r.TransactionID != "" && !strings.EqualFold(r.TransactionID, transactionID) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// CreatePurchaseOrder ports createPurchaseOrder and returns the new
// purchase order's internal id.
func (c *Client) CreatePurchaseOrder(ctx context.Context, po PurchaseOrder) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	d, err := TransformDate(po.TransactionDate)
	if err != nil {
		return "", err
	}
	po.TransactionDate = d
	return c.create(ctx, c.cfg.CreatePurchaseOrder, po, "Purchase Order")
}

// CreateVendorBill ports createPerDiemAllocationVendorBill and returns the
// new vendor bill's internal id.
func (c *Client) CreateVendorBill(ctx context.Context, vb VendorBill) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	d, err := TransformDate(vb.TransactionDate)
	if err != nil {
		return "", err
	}
	vb.TransactionDate = d
	return c.create(ctx, c.cfg.CreateVendorBill, vb, "Vendor Bill")
}

// CreateMarketingEvent ports createMarketingEvent: a new Marketing Event
// engagement becomes a NetSuite marketing event ({eventName, eventID}). The
// RESTlet answers 200 with the new internal id as the bare body; any other
// status is an error (the legacy create then rolled back).
func (c *Client) CreateMarketingEvent(ctx context.Context, eventName, eventID string) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	body := struct {
		EventName string `json:"eventName"`
		EventID   string `json:"eventID"`
	}{eventName, eventID}
	status, raw, err := c.do(ctx, http.MethodPost, c.url(c.cfg.CreateMarketingEvent), body)
	if err != nil {
		return fmt.Errorf("No status returned from NetSuite for the marketing event: %w", err)
	}
	if status != http.StatusOK {
		if bytes.Contains(raw, []byte(ErrEventExists.Error())) {
			return ErrEventExists
		}
		return fmt.Errorf("Failed to create the marketing event in NetSuite. Status code: %d Details: %s", status, clip(raw))
	}
	return nil
}

// create posts body to s. A 200 carrying an "id" is success. A 200 carrying
// a SuiteScript error ("message") or anything else, a non-200 status, or no
// answer at all is an error: the caller cannot tell whether NetSuite created
// the record, so it records a failure that a lookup can settle later.
func (c *Client) create(ctx context.Context, s Script, body any, what string) (string, error) {
	status, raw, err := c.do(ctx, http.MethodPost, c.url(s), body)
	if err != nil {
		return "", fmt.Errorf("No status returned from NetSuite for the %s: %w", what, err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("Failed pushing %s to NetSuite. Status code: %d Details: %s", what, status, clip(raw))
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("Failed to push %s to NetSuite. Unexpected response: %s", what, clip(raw))
	}
	if msg, ok := payload["message"]; ok {
		return "", fmt.Errorf("Failed to push %s to NetSuite. Error: %s", what, scalar(msg))
	}
	if id := scalar(payload["id"]); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("Failed to push %s to NetSuite. Unexpected response: %s", what, clip(raw))
}

// scalar renders a JSON string or number as text ("" for anything else).
func scalar(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&n) == nil {
		return n.String()
	}
	return ""
}

func clip(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > maxErrorText {
		s = s[:maxErrorText]
	}
	return s
}

var months = [...]string{"January", "February", "March", "April", "May", "June", "July",
	"August", "September", "October", "November", "December"}

// TransformDate ports the legacy transformClaimDate: "2026-11-04" (or
// "2026-11-04 10:00:00", or the unpadded "2026-11-5") becomes
// "04 November, 2026" ("5 November, 2026"); the day is kept as given.
func TransformDate(d string) (string, error) {
	parts := strings.Split(d, "-")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid transaction date %q", d)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 1 || m > 12 {
		return "", fmt.Errorf("invalid transaction date %q", d)
	}
	day, _, _ := strings.Cut(parts[2], " ")
	return day + " " + months[m-1] + ", " + parts[0], nil
}
