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

package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

func TestFilterRecipients(t *testing.T) {
	to, cc := FilterRecipients(
		[]string{" a@wso2.com", "a@wso2.com", "", "Gone@wso2.com", "b@wso2.com"},
		[]string{"b@wso2.com", "c@wso2.com", "c@wso2.com ", "gone@WSO2.com"},
		[]string{"gone@wso2.com"})
	if !reflect.DeepEqual(to, []string{"a@wso2.com", "b@wso2.com"}) || !reflect.DeepEqual(cc, []string{"c@wso2.com"}) {
		t.Errorf("to = %v, cc = %v", to, cc)
	}
}

func TestHTTPSender(t *testing.T) {
	var got payload
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := Config{ServiceURL: srv.URL, AppUUID: map[App]string{AppTravel: "uuid-t"}, From: map[App]string{AppTravel: "travel@wso2.com"}}
	s := New(cfg, rest.New("email", srv.URL, srv.Client()))
	s.now = func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) }
	err := s.Send(context.Background(), Message{App: AppTravel, To: []string{"t@wso2.com"}, CC: []string{"t@wso2.com", "f@wso2.com"},
		Subject: "[Travel App] Travel Request (#TRV-1)", TemplateID: "noContent", Content: map[string]string{"title": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/send-smtp-email" || got.AppUUID != "uuid-t" || got.From != "travel@wso2.com" ||
		!reflect.DeepEqual(got.CC, []string{"f@wso2.com"}) || got.ContentKeyValPairs["APP_NAME"] != "Travel App" ||
		got.ContentKeyValPairs["CURRENT_YEAR"] != "2026" || got.ContentKeyValPairs["title"] != "x" {
		t.Errorf("payload = %+v (path %s)", got, path)
	}

	// Testing mode: one recipient, no CC.
	cfg.TestRecipient = "qa@wso2.com"
	s = New(cfg, rest.New("email", srv.URL, srv.Client()))
	_ = s.Send(context.Background(), Message{App: AppTravel, To: []string{"t@wso2.com"}, CC: []string{"f@wso2.com"}})
	if !reflect.DeepEqual(got.To, []string{"qa@wso2.com"}) || len(got.CC) != 0 {
		t.Errorf("testing mode: to %v cc %v", got.To, got.CC)
	}
}

func TestHTTPSender_NoRecipientsIsNotAnError(t *testing.T) {
	s := New(Config{}, rest.New("email", "http://127.0.0.1:1", nil))
	if err := s.Send(context.Background(), Message{To: []string{" "}}); err != nil {
		t.Errorf("err = %v", err)
	}
}
