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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

// decoded is the rendered HTML a test server received.
func decoded(t *testing.T, p payload) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(p.Template)
	if err != nil {
		t.Fatalf("template is not base64: %v", err)
	}
	return string(b)
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
	cfg := Config{ServiceURL: srv.URL, From: map[App]string{AppTravel: "travel@wso2.com"}}
	s := New(cfg, rest.New("email", srv.URL, srv.Client()))
	s.now = func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) }
	msg := Message{App: AppTravel, To: []string{"t@wso2.com"}, CC: []string{"t@wso2.com", "f@wso2.com"},
		Subject: "[Travel App] Travel Request (#TRV-1)", TemplateID: "noContent",
		Content: map[string]string{"description": "Trip <b>assigned</b>", "travel_agent_name": "Acorn"}}
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	// The Email Service gets the finished HTML, base64-encoded, at /send-email.
	want, err := Render("noContent", map[string]string{"description": "Trip <b>assigned</b>", "travel_agent_name": "Acorn",
		"APP_NAME": "Travel App", "CLOSING_AND_SIGNATURE": "Cheers,<br> Choreo Labs Digital Team", "CURRENT_YEAR": "2026"}, "")
	if err != nil {
		t.Fatal(err)
	}
	page := decoded(t, got)
	if path != "/send-email" || got.From != "travel@wso2.com" || !reflect.DeepEqual(got.To, []string{"t@wso2.com"}) ||
		!reflect.DeepEqual(got.CC, []string{"f@wso2.com"}) || page != want {
		t.Errorf("payload = to %v cc %v from %q (path %s), html matches render: %v", got.To, got.CC, got.From, path, page == want)
	}
	for _, s := range []string{"Choreo Labs Digital Team", "© 2026 Choreo Labs. All Rights Reserved", "Trip <b>assigned</b>", "Acorn"} {
		if !strings.Contains(page, s) {
			t.Errorf("rendered email lacks %q", s)
		}
	}
	// Live: no staging marks.
	if got.Subject != "[Travel App] Travel Request (#TRV-1)" || strings.Contains(page, "Staging test email") {
		t.Errorf("live email carries staging marks: subject %q", got.Subject)
	}

	// Not live (staging): only the test recipients, no CC, marked as staging
	// with the real recipients (escaped) in a notice above the body.
	cfg.TestRecipients = []string{"qa@wso2.com", "lead@wso2.com"}
	s = New(cfg, rest.New("email", srv.URL, srv.Client()))
	msg.To, msg.CC = []string{"t@wso2.com"}, []string{"f&o@wso2.com"}
	if err := s.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.To, []string{"qa@wso2.com", "lead@wso2.com"}) || len(got.CC) != 0 {
		t.Errorf("testing mode: to %v cc %v", got.To, got.CC)
	}
	page = decoded(t, got)
	if got.Subject != "[STAGING] [Travel App] Travel Request (#TRV-1)" ||
		!strings.Contains(page, "Staging test email. In production it would go to: <b>To:</b> t@wso2.com; <b>CC:</b> f&amp;o@wso2.com.") {
		t.Errorf("staging marks: subject %q", got.Subject)
	}
	// travel_agent_name appears only in the noContent body (the base also shows the description).
	if strings.Index(page, "Staging test email") > strings.Index(page, "Acorn") {
		t.Error("the staging notice is not above the body")
	}
}

// An unknown template id is an error, and nothing is posted: never a blank email.
func TestHTTPSender_UnknownTemplate(t *testing.T) {
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { posted = true }))
	defer srv.Close()
	s := New(Config{ServiceURL: srv.URL}, rest.New("email", srv.URL, srv.Client()))
	err := s.Send(context.Background(), Message{App: AppTravel, To: []string{"t@wso2.com"}, TemplateID: "nope"})
	if !errors.Is(err, ErrUnknownTemplate) || posted {
		t.Errorf("err = %v, posted = %v", err, posted)
	}
}

func TestHTTPSender_NoRecipientsIsNotAnError(t *testing.T) {
	s := New(Config{}, rest.New("email", "http://127.0.0.1:1", nil))
	if err := s.Send(context.Background(), Message{To: []string{" "}}); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestFilterRecipients_DropsInvalidAddresses(t *testing.T) {
	to, cc := FilterRecipients(
		[]string{"a@wso2.com", "x@wso2.com\r\nBcc: evil@x.com", "b@wso2.com,c@wso2.com", "Jane <j@wso2.com>", "nope"},
		[]string{"d@wso2.com", "e@wso2.com\nBcc: evil@x.com"}, nil)
	if !reflect.DeepEqual(to, []string{"a@wso2.com"}) || !reflect.DeepEqual(cc, []string{"d@wso2.com"}) {
		t.Errorf("to %v cc %v", to, cc)
	}
}

func TestHTTPSender_SubjectHasNoLineBreaks(t *testing.T) {
	var got payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	s := New(Config{ServiceURL: srv.URL}, rest.New("email", srv.URL, srv.Client()))
	if err := s.Send(context.Background(), Message{App: AppTravel, To: []string{"t@wso2.com"}, TemplateID: "noContent",
		Subject: "[Allocations] x\r\nBcc: evil@x.com\tend"}); err != nil {
		t.Fatal(err)
	}
	if got.Subject != "[Allocations] x  Bcc: evil@x.com end" {
		t.Errorf("subject = %q", got.Subject)
	}
}

// Neither sender writes addresses or the subject (which names projects and
// customers) to the log; counts and the reference are enough.
func TestSenders_LogNoAddressesOrSubject(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	m := Message{App: AppAllocations, To: []string{"jane@wso2.com"}, CC: []string{"lead@wso2.com"},
		Subject: "Consultant request for E07201 : Acme - PoC", ReferenceID: "E07201", NotificationType: "allocation", TemplateID: "noContent"}
	s := New(Config{ServiceURL: srv.URL}, rest.New("email", srv.URL, srv.Client()))
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if err := (LogSender{}).Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "@") || strings.Contains(out, "Acme") {
		t.Errorf("log carries addresses or subject: %s", out)
	}
	if strings.Count(out, "ref=E07201") != 2 || strings.Count(out, "toCount=1 ccCount=1") != 2 {
		t.Errorf("log = %s", out)
	}
}
