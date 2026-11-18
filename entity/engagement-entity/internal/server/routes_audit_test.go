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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
)

func TestAuditEvents(t *testing.T) {
	audit.SetEnabled(true)
	t.Cleanup(func() { audit.SetEnabled(false) })
	h, allocMock, travelMock := newTestRouter(t)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/audit-events", strings.NewReader(body))
		req.Header.Set("X-User-Email", "fin@example.com")
		req.Header.Set("X-Correlation-ID", "req-9")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	travelMock.ExpectExec(`INSERT INTO audit_event`).
		WithArgs("fin@example.com", "user", "Download", "travel_document", "12", "employee", "c@example.com", "req-9",
			"engagement-app", nil, nil).WillReturnResult(sqlmock.NewResult(1, 1))
	if rec := post(`{"action":"Download","entityType":"travel_document","entityId":"12","parentType":"employee","parentId":"c@example.com"}`); rec.Code != http.StatusNoContent {
		t.Errorf("download: %d %s", rec.Code, rec.Body)
	}
	allocMock.ExpectExec(`INSERT INTO audit_event`).
		WithArgs("fin@example.com", "user", "Export", "allocation", nil, nil, nil, "req-9", "engagement-app", nil,
			`{"report":"consultant-allocations"}`).WillReturnResult(sqlmock.NewResult(1, 1))
	if rec := post(`{"action":"Export","entityType":"allocation","metadata":{"report":"consultant-allocations"}}`); rec.Code != http.StatusNoContent {
		t.Errorf("export: %d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{
		`{"action":"Update","entityType":"invoice","entityId":"7"}`,
		`{"action":"View","entityType":"nope"}`,
	} {
		if rec := post(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, rec.Code)
		}
	}
	for name, m := range map[string]sqlmock.Sqlmock{"alloc": allocMock, "travel": travelMock} {
		if err := m.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
