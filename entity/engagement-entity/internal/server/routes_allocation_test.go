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
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
)

func newAllocationTestRouter(t *testing.T) (http.Handler, sqlmock.Sqlmock) {
	t.Helper()
	alloc, allocMock := mockDB(t)
	travel, _ := mockDB(t)
	return NewRouter(Deps{Pools: &db.Pools{Allocation: alloc, Travel: travel}}), allocMock
}

func allocationServe(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
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

func TestAllocationRoutes(t *testing.T) {
	h, mock := newAllocationTestRouter(t)

	mock.ExpectQuery("FROM engagement AS e").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if rec := allocationServe(h, http.MethodGet, "/allocations/engagements/E00001", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing engagement: %d %s", rec.Code, rec.Body)
	}
	if rec := allocationServe(h, http.MethodPost, "/allocations/bulk", `{"createdBy":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bulk without createdBy: %d", rec.Code)
	}
	if rec := allocationServe(h, http.MethodPost, "/allocations/bulk", `{"unknown":1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d", rec.Code)
	}
	if rec := allocationServe(h, http.MethodGet, "/consultant-allocations?startDate=2026-01-01&endDate=2026-01-31&allocationTypeId=x", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("report type id: %d", rec.Code)
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO allocation").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id FROM allocation").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A0000000001"))
	mock.ExpectCommit()
	rec := allocationServe(h, http.MethodPost, "/allocations/bulk", `{"createdBy":"m@example.com","newAllocations":[
		{"email":"a@example.com","allocationTypeId":1,"status":"Active","startDate":"2026-11-01","endDate":"2026-11-02",
		 "engagementId":"-1","engagementCode":"DIG-GEN","isTravelRequired":"No","isRecurring":"No"}]}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"firstAllocationId":"A0000000001"`) {
		t.Errorf("bulk: %d %s", rec.Code, rec.Body)
	}

	mock.ExpectQuery("WHERE a.id = ?").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if rec := allocationServe(h, http.MethodDelete, "/allocations/A9", ""); rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
