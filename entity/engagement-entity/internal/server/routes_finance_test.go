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
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func financeServe(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec
}

func TestFinanceRoutes_Validation(t *testing.T) {
	h, _, _ := newTestRouter(t)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/finance/travel-requests/BAD-1", ""},
		{http.MethodGet, "/finance/invoices/abc", ""},
		{http.MethodGet, "/finance/invoices?failed=maybe", ""},
		{http.MethodGet, "/finance/invoices?addedFrom=2026-13-01", ""},
		{http.MethodGet, "/finance/per-diem-payments/TRV-1", ""},
		{http.MethodGet, "/finance/travel-requests/TRV-2026-11-0001/counts?agencyId=-1", ""},
		{http.MethodGet, "/finance/travel-requests/TRV-2026-11-0001/counts?agencyId=x", ""},
		{http.MethodPost, "/finance/invoices/decisions", `{"invoiceIds":[1],"decision":"approve","step":"hr","updatedBy":"a@example.com"}`},
		{http.MethodPost, "/finance/invoices/decisions", `{"unknown":1}`},
	} {
		if rec := financeServe(h, tc.method, tc.path, tc.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: status = %d, want 400 (%s)", tc.method, tc.path, rec.Code, rec.Body)
		}
	}
}

// NetSuite is off by default: pushes, re-syncs and lookups answer 503.
func TestFinanceRoutes_NetSuiteDisabledIs503(t *testing.T) {
	h, _, _ := newTestRouter(t)
	for _, path := range []string{"/finance/invoices/7/push", "/finance/invoices/7/resync",
		"/finance/per-diem-payments/TRV-2026-11-0001-001/push", "/finance/per-diem-payments/TRV-2026-11-0001-001/resync"} {
		rec := financeServe(h, http.MethodPost, path, `{"updatedBy":"p@example.com"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", path, rec.Code)
		}
	}
	if rec := financeServe(h, http.MethodGet, "/finance/invoices/7/netsuite-reference", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("reference: status = %d", rec.Code)
	}
}

func TestFinanceRoutes_CostTypesAndCounts(t *testing.T) {
	h, _, travel := newTestRouter(t)
	travel.ExpectQuery(regexp.QuoteMeta("SELECT id, name FROM inv_cost_type WHERE status='active' ORDER BY id")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Air Ticket"))
	rec := financeServe(h, http.MethodGet, "/finance/invoice-cost-types", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"costTypes":[{"id":1,"name":"Air Ticket"}]`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	travel.ExpectQuery(regexp.QuoteMeta("COALESCE(SUM(i.status = 'Pending'), 0)")).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).AddRow(1, 2, 3, 0, 9))
	rec = financeServe(h, http.MethodGet, "/finance/invoices/counts", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"acceptedByFinance":3`) {
		t.Errorf("counts: status = %d, body %s", rec.Code, rec.Body)
	}
	if err := travel.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestFinanceRoutes_TripNotFound(t *testing.T) {
	h, _, travel := newTestRouter(t)
	travel.ExpectQuery(regexp.QuoteMeta("WHERE t.job_number = ?")).WillReturnRows(sqlmock.NewRows([]string{"j"}))
	if rec := financeServe(h, http.MethodGet, "/finance/travel-requests/TRV-2026-11-0001", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}
