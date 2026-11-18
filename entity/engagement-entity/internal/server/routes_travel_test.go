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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func travelServe(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	h.ServeHTTP(rec, req)
	return rec
}

// The detail combines travelapp and allocation_db, each through its own pool.
func TestTravelRequestDetail_CombinesBothSchemas(t *testing.T) {
	h, allocMock, travelMock := newTestRouter(t)
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	travelMock.ExpectQuery(`WHERE t.job_number = \? LIMIT 1`).WithArgs("TRV-2026-10-1246").
		WillReturnRows(sqlmock.NewRows(make([]string, 24)).AddRow(
			"TRV-2026-10-1246", "In Progress", "traveller@example.com", "Singapore", "Singapore", "WSO2",
			day("2026-10-06"), day("2026-10-16"), "A2609100005", "EVT00012", "MKT-01", "Marketing Event",
			nil, nil, nil, "Visa Not Applicable", nil, nil, nil, nil, nil, nil, nil, nil))
	allocMock.ExpectQuery(`a.id IN \(\?\)`).WithArgs("A2609100005").
		WillReturnRows(sqlmock.NewRows(make([]string, 9)).AddRow("A2609100005", "EVT00012",
			"Integration Summit Singapore 2026", "WSO2", "MKT-01", "701000000000002AAA", 5, "Marketing Event", "Internal"))
	allocMock.ExpectQuery(`FROM engagement_funding_source`).WithArgs("EVT00012").
		WillReturnRows(sqlmock.NewRows(make([]string, 5)).AddRow("Integration", "APIM", "APAC", "SEA", 100.0))

	rec := travelServe(h, http.MethodGet, "/travel-requests/TRV-2026-10-1246", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	var body struct {
		JobNumber  string `json:"jobNumber"`
		Visa       string `json:"visa"`
		Engagement struct {
			Title      string `json:"title"`
			CampaignID string `json:"campaignId"`
		} `json:"engagement"`
		FundingSources []struct {
			Percentage float64 `json:"percentage"`
		} `json:"fundingSources"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Engagement.CampaignID != "701000000000002AAA" || body.Visa != "Visa Not Applicable" ||
		len(body.FundingSources) != 1 || body.FundingSources[0].Percentage != 100 {
		t.Errorf("body = %+v", body)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTravelRoutes_Validation(t *testing.T) {
	h, _, _ := newTestRouter(t)
	tests := []struct {
		method, target, body string
		want                 int
	}{
		{http.MethodPost, "/travel-requests/search", `{"statuses":["Approved"]}`, http.StatusBadRequest},
		{http.MethodPost, "/travel-requests/search", `{"bogus":1}`, http.StatusBadRequest},
		{http.MethodGet, "/travel-requests/E07201", "", http.StatusBadRequest},
		{http.MethodPatch, "/travel-requests/TRV-2026-10-1246", `{"status":"In Progress"}`, http.StatusBadRequest},
		{http.MethodGet, "/visas/abc", "", http.StatusBadRequest},
		{http.MethodGet, "/visas?validity=soon", "", http.StatusBadRequest},
		{http.MethodGet, "/visas?excludeId=x", "", http.StatusBadRequest},
		{http.MethodPut, "/visas/1", `{"status":"Active"}`, http.StatusBadRequest},
		{http.MethodPost, "/travel-documents", `{"documentType":"Visa"}`, http.StatusBadRequest},
		{http.MethodGet, "/travel-documents?limit=x", "", http.StatusBadRequest},
		{http.MethodGet, "/travel-agencies?status=Deleted", "", http.StatusBadRequest},
		{http.MethodPost, "/travel-email-audit-logs", `{}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			if rec := travelServe(h, tc.method, tc.target, tc.body); rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestTravelEmailAudit_NoContent(t *testing.T) {
	h, _, travelMock := newTestRouter(t)
	travelMock.ExpectExec(`INSERT INTO email_auditlog`).WillReturnResult(sqlmock.NewResult(1, 1))
	rec := travelServe(h, http.MethodPost, "/travel-email-audit-logs",
		`{"subject":"[Travel App] Travel Request (#TRV-2026-10-1246)","templateId":"noContent","contentKeyValPairs":"{}","recipients":"a@example.com","ccRecipients":""}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTravelVisaCreate_Created(t *testing.T) {
	h, _, travelMock := newTestRouter(t)
	travelMock.ExpectBegin()
	travelMock.ExpectExec(`UPDATE visa_status SET status = 'Inactive'`).WillReturnResult(sqlmock.NewResult(0, 0))
	travelMock.ExpectExec(`INSERT INTO visa_status`).WillReturnResult(sqlmock.NewResult(5, 1))
	travelMock.ExpectCommit()
	travelMock.ExpectQuery(`WHERE v.id = \?`).WithArgs(5).WillReturnRows(sqlmock.NewRows(make([]string, 17)).
		AddRow(5, "c@example.com", "GB", "Business/Visit", "Single", nil, nil, nil, "Active", "x.png", nil, nil, nil, nil, nil, "c@example.com", nil))
	rec := travelServe(h, http.MethodPost, "/visas", `{"email":"c@example.com","country":"GB","visaType":"Business/Visit",
		"entryType":"Single","issuedDate":"2026-01-01","startDate":"2026-01-02","endDate":"2026-12-31","imageId":"x.png","createdBy":"c@example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
