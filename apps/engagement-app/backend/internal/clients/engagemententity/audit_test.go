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

package engagemententity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

func TestRecordAuditEvent_NamesTheUserAndRequest(t *testing.T) {
	var got struct {
		user, requestID, path string
		body                  domain.AuditEvent
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.user, got.requestID, got.path = r.Header.Get("X-User-Email"), r.Header.Get("X-Correlation-ID"), r.Method+" "+r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	ctx := reqcontext.WithCorrelationID(reqcontext.WithCaller(context.Background(), reqcontext.Caller{Email: "jane@wso2.com"}), "req-1")
	err := New(srv.URL, nil).RecordAuditEvent(ctx, domain.AuditEvent{Action: domain.AuditDownload, EntityType: "visa", EntityID: "5"})
	if err != nil {
		t.Fatal(err)
	}
	if got.user != "jane@wso2.com" || got.requestID != "req-1" || got.path != "POST /audit-events" ||
		got.body.Action != "Download" || got.body.EntityID != "5" {
		t.Errorf("got %+v", got)
	}
}
