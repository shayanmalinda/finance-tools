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
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

func TestAuditEventBuilders(t *testing.T) {
	r := httptest.NewRequest("GET", "/reports/invoices?from=2026-01-01&to=2026-01-31", nil)
	if e, ok := auditExport("invoice", "invoices")(r); !ok || e.Action != "View" || e.Metadata["report"] != "invoices" {
		t.Errorf("a JSON report must be audited as a view: %+v, %v", e, ok)
	}
	r = httptest.NewRequest("GET", "/reports/invoices?from=2026-01-01&format=CSV", nil)
	if e, ok := auditExport("invoice", "invoices")(r); !ok || e.Action != "Export" || e.Metadata["report"] != "invoices" ||
		e.Metadata["query"] != "from=2026-01-01&format=CSV" {
		t.Errorf("export = %+v, %v", e, ok)
	}

	r = httptest.NewRequest("GET", "/me/passports", nil)
	r = r.WithContext(reqcontext.WithCaller(r.Context(), reqcontext.Caller{Email: "jane@wso2.com"}))
	if e, _ := auditPassports(true)(r); e.EntityType != "travel_document" || e.ParentID != "jane@wso2.com" {
		t.Errorf("own passports = %+v", e)
	}
	if e, _ := auditList("visa", true)(r); e.Action != "View" || e.EntityID != "" || e.ParentID != "jane@wso2.com" {
		t.Errorf("own visas = %+v", e)
	}
}
