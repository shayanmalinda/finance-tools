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

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

// finRouteEntity grants each test group a set of Travel privileges and
// serves a small finance data set.
type finRouteEntity struct {
	engagemententity.Client
}

func (finRouteEntity) RecordAuditEvent(context.Context, domain.AuditEvent) error { return nil }

func (finRouteEntity) RolePrivileges(_ context.Context, app domain.App) (map[string][]int, error) {
	if app == domain.AppAllocations {
		return map[string][]int{}, nil
	}
	return map[string][]int{
		"app-travel-approver-finance": {2, 3, 8, 28, 30, 33, 34, 35, 39, 40, 41, 42, 43, 44, 45},
		"wso2-employees":              {19, 36, 37, 38, 47, 48, 49},
	}, nil
}

func (finRouteEntity) TravelAgency(context.Context, string) (*domain.TravelAgency, error) {
	return nil, nil
}

func (finRouteEntity) FinanceInvoices(context.Context, engagemententity.FinanceInvoiceFilter) ([]engagemententity.FinanceInvoice, int, error) {
	camp := "Integration Summit, Singapore"
	return []engagemententity.FinanceInvoice{{ID: 1, InvoiceNumber: "=INV-1", JobNumber: "TRV-2026-11-0001", Status: "Complete",
		InvoiceCurrency: "USD", InvoiceAmount: 1850, ConvertedCurrency: "USD", ConvertedAmount: 1850, Month: "October",
		Campaign: &engagemententity.FinanceCampaign{ID: "7015g00000ApD26AAF", Name: &camp}}}, 1, nil
}

func (finRouteEntity) FinanceInvoiceCounts(context.Context, int) (engagemententity.FinanceInvoiceCounts, error) {
	return engagemententity.FinanceInvoiceCounts{All: 1}, nil
}

func finRouter(groups ...string) http.Handler {
	return NewRouter(Deps{
		Verifier: fakeVerifier{claims: oidc.Claims{Email: "udanee@wso2.com", Groups: groups, GroupsPresent: true}},
		Entity:   finRouteEntity{},
		Sales:    finRouteSales{},
	})
}

// finRouteSales names the one campaign (by id, as the Sales Entity does).
type finRouteSales struct{ salesentity.Unconfigured }

func (finRouteSales) SearchCampaigns(_ context.Context, f salesentity.CampaignFilter) ([]salesentity.Campaign, error) {
	if len(f.IDs) == 1 && f.IDs[0] == "7015g00000ApD26AAF" {
		return []salesentity.Campaign{{ID: "7015g00000ApD26AAF", Name: "Integration Summit, Singapore"}}, nil
	}
	return nil, nil
}

func finDo(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Jwt-Assertion", "valid")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFinanceRoutesGate(t *testing.T) {
	employee := finRouter("wso2-employees")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/invoices", ""},
		{http.MethodPost, "/invoices/approve", `{"invoiceIds":[1]}`},
		{http.MethodPost, "/invoices/push", `{"invoiceIds":[1]}`},
		{http.MethodPost, "/per-diem-payments/push", `{"paymentIds":["TRV-2026-11-0001-001"]}`},
		{http.MethodGet, "/netsuite-sync-failures", ""},
		{http.MethodGet, "/reports/invoices?from=2026-01-01&to=2026-11-05", ""},
		{http.MethodPost, "/travel-requests/TRV-2026-11-0001/invoices", `{"invoices":[]}`},
		{http.MethodPost, "/travel-requests/TRV-2026-11-0001/per-diems", `{}`},
		{http.MethodDelete, "/travel-requests/TRV-2026-11-0001/per-diems/1", ""},
		{http.MethodPatch, "/per-diem-payments/TRV-2026-11-0001-001", `{"amount":1}`},
	} {
		if rec := finDo(employee, tc.method, tc.path, tc.body); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
}

func TestFinanceRoutesValidation(t *testing.T) {
	fin := finRouter("app-travel-approver-finance")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/reports/invoices?from=2026-11-05&to=2026-01-01", ""},
		{http.MethodGet, "/reports/invoices?from=bad&to=2026-01-01", ""},
		{http.MethodGet, "/reports/invoices?from=2026-01-01&to=2026-11-05&format=pdf", ""},
		{http.MethodGet, "/reports/per-diem-payments?from=2026-01-01&to=2026-11-05&format=xml", ""},
		{http.MethodGet, "/invoices?tab=nope", ""},
		{http.MethodGet, "/invoices/abc", ""},
		{http.MethodPost, "/invoices/push", `{"invoiceIds":[]}`},
		{http.MethodPost, "/invoices/reject", `{"invoiceIds":[1],"reason":""}`},
		{http.MethodPost, "/invoices/approve", `{"ids":[1]}`},
		{http.MethodGet, "/travel-requests/BAD/per-diems", ""},
	} {
		if rec := finDo(fin, tc.method, tc.path, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status = %d, want 400 (%s)", tc.method, tc.path, rec.Code, rec.Body)
		}
	}
	// A queue the caller does not own: their own counts, no rows.
	if rec := finDo(fin, http.MethodGet, "/invoices?tab=approve-po", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("approve-po: status = %d %s", rec.Code, rec.Body)
	}
}

func TestFinanceRoutesInvoiceListAndCSV(t *testing.T) {
	fin := finRouter("app-travel-approver-finance")
	rec := finDo(fin, http.MethodGet, "/invoices?tab=all", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"netsuite":{"status":"Pushed"`) ||
		!strings.Contains(rec.Body.String(), `"tabCounts":{"all":1,"approve":0,"push":0}`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	rec = finDo(fin, http.MethodGet, "/reports/invoices?from=2026-01-01&to=2026-11-05&format=csv", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "2026-01-01_to_2026-11-05_invoices.csv") ||
		!strings.HasPrefix(body, "Invoice Number,Traveller WSO2 Email,Job Number,Invoice Status,Invoice Currency,Invoice Amount,Paid Currency,Paid Amount,Month,Cost Type,Campaign ID,Campaign Name,Reject Reason,Added Date\n") ||
		!strings.Contains(body, `'=INV-1,,TRV-2026-11-0001,Complete,USD,1850.00,USD,1850.00,October,,7015g00000ApD26AAF,"Integration Summit, Singapore",,`) {
		t.Errorf("csv: %d %s", rec.Code, body)
	}
}

// Without the currency conversion service a payment cannot be added: 503.
func TestFinanceRoutesPaymentAddWithoutConverter(t *testing.T) {
	fin := finRouter("app-travel-approver-finance")
	if rec := finDo(fin, http.MethodPost, "/travel-requests/TRV-2026-11-0001/per-diem-payments", `{"amount":10}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}
