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

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

type auditFake struct {
	travel, alloc []engagemententity.EmailAudit
	fail          bool
}

func (f *auditFake) InsertTravelEmailAudit(_ context.Context, in engagemententity.TravelEmailAudit) error {
	f.travel = append(f.travel, in)
	if f.fail {
		return errors.New("Unknown column 'status'")
	}
	return nil
}

func (f *auditFake) InsertAllocationEmailAudit(_ context.Context, in engagemententity.EmailAudit) error {
	f.alloc = append(f.alloc, in)
	if f.fail {
		return errors.New("Table 'allocation_db.email_auditlog' doesn't exist")
	}
	return nil
}

func auditMsg(app email.App) email.Message {
	return email.Message{App: app, To: []string{"a@wso2.com", " "}, CC: []string{"b@wso2.com"}, Subject: "subj",
		TemplateID: "noContent", Content: map[string]string{"k": "v"}}
}

func TestEmailAuditor_SentGoesToTheAppsOwnLog(t *testing.T) {
	f := &auditFake{}
	now := func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) }
	audit := EmailAuditor(f, now)
	audit(context.Background(), auditMsg(email.AppTravel), nil)
	audit(context.Background(), auditMsg(email.AppAllocations), nil)
	if len(f.travel) != 1 || len(f.alloc) != 1 {
		t.Fatalf("travel %d alloc %d rows", len(f.travel), len(f.alloc))
	}
	r := f.travel[0]
	if r.Status != "Sent" || r.ErrorMessage != "" || r.Recipients != "a@wso2.com" || r.CCRecipients != "b@wso2.com" ||
		r.Subject != "subj" || !strings.Contains(r.ContentKeyValPairs, `"CURRENT_YEAR":"2026"`) {
		t.Errorf("row = %+v", r)
	}
}

func TestEmailAuditor_FailedKeepsTheErrorWithoutSecrets(t *testing.T) {
	f := &auditFake{}
	audit := EmailAuditor(f, nil)
	audit(context.Background(), auditMsg(email.AppAllocations),
		errors.New("post https://email.example.com/send-email?key=abc: Bearer eyJhbGciOi.xyz token=s3cret failed\n"+strings.Repeat("x", 800)))
	r := f.alloc[0]
	if r.Status != "Failed" {
		t.Errorf("status = %q", r.Status)
	}
	for _, leak := range []string{"https://", "eyJhbGciOi", "s3cret", "\n"} {
		if strings.Contains(r.ErrorMessage, leak) {
			t.Errorf("error message leaks %q: %s", leak, r.ErrorMessage)
		}
	}
	if len(r.ErrorMessage) != 500 || !strings.HasPrefix(r.ErrorMessage, "post [redacted]") {
		t.Errorf("error message = %d chars: %.60s", len(r.ErrorMessage), r.ErrorMessage)
	}
}

// A failed log write (a table or column not migrated yet) is logged only.
func TestEmailAuditor_WriteFailureIsSwallowed(t *testing.T) {
	f := &auditFake{fail: true}
	audit := EmailAuditor(f, nil)
	audit(context.Background(), auditMsg(email.AppTravel), nil)
	audit(context.Background(), auditMsg(email.AppAllocations), errors.New("down"))
	if len(f.travel) != 1 || len(f.alloc) != 1 {
		t.Errorf("both writes should have been tried: %d %d", len(f.travel), len(f.alloc))
	}
}

// End to end: the email service is down and the log is down too; the user's
// action (a travel update that emails) still succeeds.
func TestEmailAudit_SendAndLogFailuresDoNotFailTheAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer srv.Close()
	log := &auditFake{fail: true}
	sender := email.New(email.Config{ServiceURL: srv.URL}, rest.New("email", srv.URL, srv.Client())).
		WithAudit(EmailAuditor(log, nil))
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	svc := NewTravelService(f.entity, travelFakeHR{}, sender, TravelOptions{VisaNotApplicableID: 332, NoValidVisaID: 333, Now: travelNow,
		Email: email.TravelSettings{LogisticsGroup: "travel-logistics@wso2.com", AppBaseURL: "https://one.wso2.com"}})
	if _, err := svc.Update(context.Background(), travelTeam, "TRV-2026-10-1246",
		domain.TravelRequestUpdate{Status: travelP("Cancelled")}); err != nil {
		t.Fatalf("the action failed: %v", err)
	}
	if len(log.travel) != 1 || log.travel[0].Status != "Failed" || !strings.Contains(log.travel[0].ErrorMessage, "502") {
		t.Errorf("rows = %+v", log.travel)
	}
}
