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

// Package email renders the Engagement App's emails from its own templates
// and sends the finished HTML through the Email Service (POST /send-email),
// with the legacy recipient rules and testing mode.
package email

import (
	"context"
	"encoding/base64"
	"html"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

// App is the legacy app an email belongs to. Each has its own sender and
// APP_NAME.
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
	// Subject, e.g. "[Engagement App] Consultant request for E07201 : Acme -
	// PoC" (the legacy app's subject with the Engagement App tag).
	Subject string
	// TemplateID is the template in templates/ (Render).
	TemplateID string
	// Content fills the template. Values are inserted into HTML as they
	// are; callers HTML-escape user text (html.EscapeString).
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

// Config is the Email Service and the per-app settings of the legacy
// [app_allocation.email] and [app_travel.email.emailConfigurations] sections.
type Config struct {
	// ServiceURL empty: emails are logged, not sent.
	ServiceURL string
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
	// Both apps' emails now come from the Engagement App (APP_NAME).
	appNames = map[App]string{AppAllocations: engagementAppName, AppTravel: engagementAppName}
	// Choreo Labs rebrand (as the Certification Portal): every internal app
	// signs as the Choreo Labs Digital Team.
	signatures = map[App]string{AppAllocations: digitalTeamSignature, AppTravel: digitalTeamSignature}
)

// HTTPSender sends through the Email Service.
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

// payload is the Email Service's POST /send-email body: Template is the
// whole rendered HTML, base64-encoded (the service decodes it).
type payload struct {
	To       []string `json:"to"`
	CC       []string `json:"cc"`
	From     string   `json:"from"`
	Subject  string   `json:"subject"`
	Template string   `json:"template"`
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
	p := payload{To: to, CC: cc, From: s.cfg.From[m.App], Subject: subjectLine(m.Subject)}
	banner := ""
	if testing {
		// Not live (staging): mark the email and show who it would have gone
		// to, so testers can check the routing. Never set when live.
		p.Subject = stagingSubjectPrefix + p.Subject
		banner = stagingBanner(to, cc)
		p.To, p.CC = append([]string(nil), s.cfg.TestRecipients...), []string{}
	}
	page, err := Render(m.TemplateID, content, banner)
	if err != nil {
		slog.ErrorContext(ctx, "email not rendered", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID, "err", err)
		return err
	}
	p.Template = base64.StdEncoding.EncodeToString([]byte(page))
	if err := s.rest.Post(ctx, "/send-email", p, nil); err != nil {
		slog.ErrorContext(ctx, "email not sent", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID, "err", err)
		return err
	}
	// Counts, not addresses: type and ref already identify the email.
	slog.InfoContext(ctx, "email sent", "type", m.NotificationType, "ref", m.ReferenceID, "template", m.TemplateID,
		"toCount", len(to), "ccCount", len(cc), "testingRedirect", testing)
	return nil
}

// engagementAppName is the app every email names: the Allocations and Travel
// apps are now the Engagement App.
const engagementAppName = "Engagement App"

// digitalTeamSignature closes every email (CLOSING_AND_SIGNATURE).
const digitalTeamSignature = "Cheers,<br> Choreo Labs Digital Team"

// stagingSubjectPrefix marks every email sent while emails are not live.
const stagingSubjectPrefix = "[STAGING] "

// stagingBanner is the staging notice Render shows above the body: the real
// recipients, HTML-escaped.
func stagingBanner(to, cc []string) string {
	esc := func(list []string) string {
		if len(list) == 0 {
			return "none"
		}
		out := make([]string, len(list))
		for i, a := range list {
			out[i] = html.EscapeString(a)
		}
		return strings.Join(out, ", ")
	}
	return "Staging test email. In production it would go to: <b>To:</b> " + esc(to) + "; <b>CC:</b> " + esc(cc) + "."
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
