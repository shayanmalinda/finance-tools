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

// Package email sends notifications through the WSO2 email service with the
// legacy recipient rules and testing mode.
package email

import (
	"context"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

// App is the legacy app an email belongs to. Each has its own app id in the
// email service, sender and signature.
type App string

// The two legacy apps.
const (
	AppAllocations App = "allocations"
	AppTravel      App = "travel"
)

// Message is one email.
type Message struct {
	App App
	To  []string
	CC  []string
	// Subject as the legacy app wrote it, e.g. "[Allocations] Consultant
	// request for E07201 : Acme - PoC".
	Subject string
	// TemplateID is the email service template.
	TemplateID string
	// Content fills the template. Values are inserted into HTML by the email
	// service; callers HTML-escape user text (html.EscapeString).
	Content map[string]string
	// NotificationType and ReferenceID are for logs only.
	NotificationType string
	ReferenceID      string
}

// Sender sends email. Send must not be retried by callers on error: a
// retry after a timeout can deliver the same email twice.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Config is the email service and the per-app settings of the legacy
// [app_allocation.email] and [app_travel.email.emailConfigurations] sections.
type Config struct {
	// ServiceURL empty: emails are logged, not sent.
	ServiceURL string
	// AppUUID per app (legacy appUuid / travelAppUUID).
	AppUUID map[App]string
	// From per app (legacy allocationManagerEmail / travelAppEmail).
	From map[App]string
	// TestRecipients, when set (emails not live: IS_LIVE_EMAILS=false),
	// receive every email instead of the real recipients and CC is dropped
	// (legacy isSendTestingEmailsOnlyVal=1 with allocationTestEmail).
	TestRecipients []string
	// Suppressed addresses never receive email (legacy noEmailsTo).
	Suppressed []string
}

// legacy globals added to every template.
var (
	appNames   = map[App]string{AppAllocations: "Allocations App", AppTravel: "Travel App"}
	signatures = map[App]string{AppAllocations: "Cheers,<br> WSO2 Internal Apps Team", AppTravel: "Cheers,<br> Apps & Automations Team"}
)

// HTTPSender sends through the email service.
type HTTPSender struct {
	cfg  Config
	rest *rest.Client
	now  func() time.Time
}

// New returns an HTTPSender. client authenticates to the email service
// (normally rest.NewOAuthHTTPClient with the shared client credentials).
func New(cfg Config, rc *rest.Client) *HTTPSender {
	return &HTTPSender{cfg: cfg, rest: rc, now: time.Now}
}

type payload struct {
	AppUUID            string            `json:"appUuid"`
	To                 []string          `json:"to"`
	CC                 []string          `json:"cc"`
	From               string            `json:"frm"`
	Subject            string            `json:"subject"`
	TemplateID         string            `json:"templateId"`
	ContentKeyValPairs map[string]string `json:"contentKeyValPairs"`
}

// Send applies the recipient rules and posts the email.
func (s *HTTPSender) Send(ctx context.Context, m Message) error {
	to, cc := FilterRecipients(m.To, m.CC, s.cfg.Suppressed)
	if len(to) == 0 {
		slog.WarnContext(ctx, "email skipped: no To recipient", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID)
		return nil
	}
	content := make(map[string]string, len(m.Content)+3)
	for k, v := range m.Content {
		content[k] = v
	}
	content["APP_NAME"] = appNames[m.App]
	content["CLOSING_AND_SIGNATURE"] = signatures[m.App]
	content["CURRENT_YEAR"] = strconv.Itoa(s.now().UTC().Year())

	testing := len(s.cfg.TestRecipients) > 0
	p := payload{
		AppUUID: s.cfg.AppUUID[m.App], To: to, CC: cc, From: s.cfg.From[m.App],
		Subject: subjectLine(m.Subject), TemplateID: m.TemplateID, ContentKeyValPairs: content,
	}
	if testing {
		p.To, p.CC = append([]string(nil), s.cfg.TestRecipients...), []string{}
	}
	if err := s.rest.Post(ctx, "/send-smtp-email", p, nil); err != nil {
		slog.ErrorContext(ctx, "email not sent", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID, "err", err)
		return err
	}
	// Counts, not addresses: type and ref already identify the email.
	slog.InfoContext(ctx, "email sent", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID,
		"toCount", len(to), "ccCount", len(cc), "testingRedirect", testing)
	return nil
}

// subjectLine replaces control characters (CR, LF, tab, ...) in a subject
// with spaces: the subject embeds free text such as project names, and a line
// break in it could add a mail header.
func subjectLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// validAddress reports whether a is one plain email address: no display
// name, list or line break.
func validAddress(a string) bool {
	if strings.ContainsAny(a, "\r\n,") {
		return false
	}
	addr, err := mail.ParseAddress(a)
	return err == nil && addr.Address == a
}

// FilterRecipients ports the legacy filterRecipients: trim each address,
// drop empty, invalid and suppressed ones (compared lower-case), de-duplicate
// To, and drop CC entries already in To or already in CC. Never nil.
func FilterRecipients(toIn, ccIn, suppressed []string) (to, cc []string) {
	sup := make(map[string]bool, len(suppressed))
	for _, a := range suppressed {
		sup[strings.ToLower(strings.TrimSpace(a))] = true
	}
	allowed := func(a string) bool { return a != "" && !sup[strings.ToLower(a)] && validAddress(a) }
	to, cc = []string{}, []string{}
	inTo := map[string]bool{}
	for _, a := range toIn {
		a = strings.TrimSpace(a)
		if allowed(a) && !inTo[a] {
			inTo[a] = true
			to = append(to, a)
		}
	}
	inCC := map[string]bool{}
	for _, a := range ccIn {
		a = strings.TrimSpace(a)
		if allowed(a) && !inTo[a] && !inCC[a] {
			inCC[a] = true
			cc = append(cc, a)
		}
	}
	return to, cc
}

// LogSender is the Sender used when EMAIL_DISABLED is set: it logs that an
// email would have been sent and returns nil, so local runs need no email
// service. Like Send, it leaves the addresses and the subject (which names
// projects and customers) out of the log.
type LogSender struct{}

// Send logs m.
func (LogSender) Send(ctx context.Context, m Message) error {
	to, cc := FilterRecipients(m.To, m.CC, nil)
	slog.InfoContext(ctx, "email disabled; not sent", "type", m.NotificationType, "ref", m.ReferenceID,
		"template", m.TemplateID, "toCount", len(to), "ccCount", len(cc))
	return nil
}
