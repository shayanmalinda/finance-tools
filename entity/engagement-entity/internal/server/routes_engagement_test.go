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

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
)

func engagementTestMux(t *testing.T) (http.Handler, sqlmock.Sqlmock) {
	t.Helper()
	alloc, allocMock := mockDB(t)
	travel, _ := mockDB(t)
	mux := http.NewServeMux()
	registerEngagementRoutes(mux, Deps{Pools: &db.Pools{Allocation: alloc, Travel: travel}})
	return mux, allocMock
}

func engagementServe(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func TestEngagementRoutes_Validation(t *testing.T) {
	h, _ := engagementTestMux(t)
	tests := []struct {
		method, target, body string
		want                 int
	}{
		{http.MethodPost, "/engagements/search", `{"limit": "ten"}`, http.StatusBadRequest},
		{http.MethodPost, "/engagements/search", `{"unknown": 1}`, http.StatusBadRequest},
		{http.MethodPost, "/engagements/search", `{"consultants": "some"}`, http.StatusBadRequest},
		{http.MethodGet, "/engagements/facets?engagementTypeId=x", "", http.StatusBadRequest},
		{http.MethodGet, "/engagements/E1;DROP", "", http.StatusBadRequest},
		{http.MethodGet, "/engagements/E1/roster?activeOnly=maybe", "", http.StatusBadRequest},
		{http.MethodPost, "/engagements", `{"allocationTypeId": 0}`, http.StatusBadRequest},
		{http.MethodPatch, "/engagements/E1", `{}`, http.StatusBadRequest},
		{http.MethodGet, "/role-settings/finance", "", http.StatusNotFound},
		{http.MethodPut, "/role-settings/travel/roles/x/privileges", `{}`, http.StatusBadRequest},
		{http.MethodPost, "/settings/allocation-types", `{}`, http.StatusBadRequest},
		{http.MethodDelete, "/settings/allocation-types/3", `{}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			rec := engagementServe(h, tc.method, tc.target, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestEngagementRoutes_GetEngagement(t *testing.T) {
	h, mock := engagementTestMux(t)
	cols := []string{"id", "engagement_type", "eT.name", "allocation_type", "aT.allocation_type",
		"customer_code", "customer_name", "project_name", "opportunity_id", "opportunity_line_item_id",
		"product_name", "location", "country", "business_domain", "username", "contracts_signed",
		"engagement_code", "start", "end", "status", "engagement_scope", "engagement_nature", "engagement_status",
		"customer_contact", "owner", "tentative_dates", "technical_owner", "comment", "description", "updated_by",
		"last_updated_date", "campaign_id"}
	mock.ExpectQuery(`WHERE e.id = \?`).WithArgs("E07201").WillReturnRows(sqlmock.NewRows(cols).AddRow(
		"E07201", 6, "Paid Post Sale", 40, "Consulting", "001A", "Northwind Bank", "Open Banking PoC", "006A", "00kA",
		"APIM PoC", "Boston", "United States of America", "Retail", "owner@example.com", 1,
		"NWB-APIM-0712-CO", "2026-10-19", "2026-11-19", "New", nil, "On-site", "Pending",
		nil, "Owner", 0, "Tech", nil, nil, nil, nil, nil))
	mock.ExpectQuery(`FROM engagement_funding_source`).WithArgs("E07201").
		WillReturnRows(sqlmock.NewRows([]string{"id", "bu", "pu", "sub", "rg", "pct"}).AddRow("E07201", "APIM-Software", "APIM", "NA - EAST", "NA", 100))
	rec := engagementServe(h, http.MethodGet, "/engagements/E07201", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "E07201" || body["engagementTypeId"].(float64) != 6 || len(body["fundingSources"].([]any)) != 1 ||
		body["campaignId"] != nil || body["contractsSigned"] != true {
		t.Errorf("body = %v", body)
	}
}
