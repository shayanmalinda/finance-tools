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
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
)

func newTestRouter(t *testing.T) (http.Handler, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	alloc, allocMock := mockDB(t)
	travel, travelMock := mockDB(t)
	return NewRouter(Deps{Pools: &db.Pools{Allocation: alloc, Travel: travel}}), allocMock, travelMock
}

func serve(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealth(t *testing.T) {
	h, _, _ := newTestRouter(t)
	rec := serve(h, http.MethodGet, "/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Correlation-ID") == "" {
		t.Error("no X-Correlation-ID on the response")
	}
}

// Each app's request reaches that app's schema only.
func TestRolePrivileges_RoutedToTheAppsOwnSchema(t *testing.T) {
	h, allocMock, travelMock := newTestRouter(t)
	travelMock.ExpectQuery("FROM role_privilege").WillReturnRows(
		sqlmock.NewRows([]string{"name", "privilege_id"}).AddRow("app-travel-admin-super", 7))

	rec := serve(h, http.MethodGet, "/access/travel/role-privileges")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct {
		RolePrivileges map[string][]int `json:"rolePrivileges"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if got := body.RolePrivileges["app-travel-admin-super"]; len(got) != 1 || got[0] != 7 {
		t.Errorf("rolePrivileges = %v", body.RolePrivileges)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestErrorsUseTheSharedShape(t *testing.T) {
	h, _, _ := newTestRouter(t)
	tests := []struct {
		target string
		want   int
	}{
		{"/access/finance/roles", http.StatusNotFound},
		{"/engagement-types?isActive=maybe", http.StatusBadRequest},
		{"/engagement-types?type=Partner", http.StatusBadRequest},
		{"/allocation-types?engagementTypeId=abc", http.StatusBadRequest},
		{"/allocation-types?status=Archived", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			rec := serve(h, http.MethodGet, tc.target)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body.Code != tc.want || body.Message == "" {
				t.Errorf("body = %+v, err = %v; want {code, message}", body, err)
			}
		})
	}
}
