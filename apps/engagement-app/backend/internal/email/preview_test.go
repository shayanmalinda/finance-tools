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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package email

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

// TestWritePreviews renders one realistic sample of every template, built by
// the real message builders, into $EMAIL_PREVIEW_DIR (plus subjects.json) to
// review the design in a browser. Skipped unless EMAIL_PREVIEW_DIR is set.
// previewSample is one email built by its real builder; staging renders it
// as a not-live email (with the staging banner).
type previewSample struct {
	name    string
	m       Message
	staging bool
}

// previewSamples are one realistic email per template and notification type,
// with the data of the staging test: the marketing event EVT00272, the
// consultant Subavarshana Arumugam and the trip TRV-2026-11-0005 to Sri Lanka.
func previewSamples() (time.Time, []previewSample) {
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	base := "https://one-stg.wso2.com"
	event := EngagementEmailData{
		ID: "EVT00272", EngagementTypeID: 5, RequestStatus: "Pending Approval", CustomerName: "Choreo Labs",
		ProjectName: "[Email test] Marketing event", AllocationTypeName: "Conference", Nature: "Physical",
		Country: "Sri Lanka", Address: "Colombo", StartDate: "2026-12-10", EndDate: "2026-12-11",
		Comment: "Email test.", Requester: "requester@example.com",
	}
	customer := event
	customer.ID, customer.EngagementTypeID, customer.CustomerName = "E06405", 3, "Acme Corp"
	customer.ProjectName, customer.ProductName, customer.AllocationTypeName = "Acme ELA Deal - 2026 - Renewal", "WSO2 API Manager", "Pre-Sale Engagement"
	customer.EngagementCode, customer.Nature, customer.BusinessDomain = "CF3rV-PR", "On-site", "Financial Services"
	customer.CustomerContact, customer.OpportunityOwner, customer.TechnicalOwner = "Jordan Lee / +1 555 0100", "Alex Morgan", "owner@example.com"
	customer.ContractsSigned, customer.EngagementScope, customer.SubRegion, customer.BusinessUnit, customer.ProductUnit = true, "API platform health check.", "NA - EAST", "IAM", "IAM"
	internal := event
	internal.ID, internal.EngagementTypeID, internal.ProjectName, internal.AllocationTypeName = "INT00118", 1, "Bug Bash - Q4", "Bug Busting"
	internal.BusinessUnit, internal.ProductUnit = "IAM", "IAM"
	rec := EngagementRecipients{AppBaseURL: base, InternalApprovers: []string{"internal-approvers@example.com"},
		MarketingApprovers: []string{"marketing-approvers@example.com"}, TravelGroup: "travel@example.com",
		Routing: []string{"allocations@example.com"}, RoutingName: "Allocations Team", ActorName: "Nimal",
		Consultants: []string{"consultant@example.com"}, ConsultantName: "Subavarshana", TravelAffected: true}
	approved := event
	approved.RequestStatus = "Approved"
	actor := "creator@example.com"

	alloc := AllocationEmailData{
		EngagementTypeID: 5, EngagementID: event.ID, CustomerName: event.CustomerName, OpportunityName: event.ProjectName,
		AllocationTypeName: event.AllocationTypeName, EngagementNature: event.Nature, Country: event.Country, Address: event.Address,
		EngagementComment: event.Comment, ConsultantEmail: "consultant@example.com", ConsultantName: "Subavarshana",
		ClearanceStatus: allocClearanceInProgress, PrevClearanceStatus: allocReadyForClearance, TravelRequired: true,
		StartDate: "2026-12-10", EndDate: "2026-12-11", AllocationComment: "Booth lead.", Actor: "allocator@example.com",
		Leads: []string{"lead@example.com"}, Now: now,
	}
	with := func(c, prev string) AllocationEmailData {
		d := alloc
		d.ClearanceStatus, d.PrevClearanceStatus = c, prev
		return d
	}
	custAlloc := with(allocClearanceInProgress, "")
	custAlloc.EngagementTypeID, custAlloc.EngagementID, custAlloc.OpportunityName, custAlloc.CustomerName = 3, customer.ID, customer.ProjectName, customer.CustomerName
	intAlloc := with(allocClearanceInProgress, "")
	intAlloc.EngagementTypeID, intAlloc.EngagementID, intAlloc.OpportunityName = 1, internal.ID, internal.ProjectName
	confirmed := with(allocConfirmed, allocAcceptedByConsultant)
	confirmed.Visas = []AllocationEmailVisa{{Country: "Sri Lanka", VisaType: "Business/Visit", EntryType: "Multiple", Status: "Active", StartDate: "2026-03-01", EndDate: "2026-12-15"}}
	groups := AllocationEmailGroups{AllocationsGroup: "allocations@example.com", PartnerAllocationsGroup: "partner-allocations-group@example.com",
		TravelGroup: "travel@example.com", AppBaseURL: base}

	ts := TravelSettings{LogisticsGroup: "travel-logistics@example.com", FinanceGroup: "finance@example.com", AppBaseURL: base, CCTraveller: true}
	trip := TravelTrip{JobNumber: "TRV-2026-11-0005", TravellerEmail: "consultant@example.com", TravellerName: "Subavarshana Arumugam",
		Customer: "Choreo Labs", Country: "Sri Lanka", City: "Colombo", StartDate: "2026-12-10", EndDate: "2026-12-11"}
	visa := TravelVisa{VisaType: "Business/Visit", Country: "Sri Lanka", EntryType: "Multiple", StartDate: "2026-03-01", EndDate: "2026-12-15"}
	perDiems := []TravelPerDiem{{StartDate: "2026-12-10 09:00:00", EndDate: "2026-12-11 18:00:00", NoOfDays: "2", Currency: "USD", Rate: "50.0", Total: "100.0"}}

	fin := FinanceRecipients{TravelLogisticsGroup: "travel-logistics@example.com", FinanceGroup: "finance@example.com",
		CompanyRecipients: map[string][]string{"WSO2 - LK": {"finance-lk@example.com"}}, AppBaseURL: base}
	items := func(f FinanceItems) FinanceItems {
		f.TravellerName, f.Country, f.TravellerEmail = trip.TravellerName, trip.Country, trip.TravellerEmail
		return f
	}
	inv1 := FinanceInvoiceItem{Number: "EMAILTEST-001", CostType: "Air Ticket", Month: "December", Currency: "USD", Amount: 100}
	inv2 := FinanceInvoiceItem{Number: "EMAILTEST-002", CostType: "Hotel", Currency: "USD", Amount: 250}
	samples := []previewSample{
		{"marketingEvent-approval-needed", EngagementMessage(EngagementCreated, event, rec, actor), true},
		{"marketingEvent-ready-to-allocate", EngagementMessage(EngagementApproved, approved, rec, "approver@example.com"), false},
		{"marketingEvent-updated", EngagementMessage(EngagementUpdated, approved, rec, actor), false},
		{"marketingEventCancelled", EngagementMessage(EngagementCancelled, event, rec, actor), false},
		{"internalEngagement-approval-needed", EngagementMessage(EngagementCreated, internal, rec, actor), false},
		{"internalEngagement-ready-to-allocate", EngagementMessage(EngagementApproved, internal, rec, "approver@example.com"), false},
		{"internalEngagementCancelled", EngagementMessage(EngagementCancelled, internal, rec, actor), false},
		{"customerEngagement-ready-to-allocate", EngagementMessage(EngagementCreated, customer, rec, actor), false},
		{"customerEngagementCancelled", EngagementMessage(EngagementCancelled, customer, rec, actor), false},
		{"marketingEventAllocation-allocated", AllocationMessage(alloc, groups), false},
		{"marketingEventAllocation-declined", AllocationMessage(with(allocRejectedByConsultant, allocClearanceInProgress), groups), false},
		{"marketingEventAllocation-accepted", AllocationMessage(with(allocAcceptedByConsultant, allocClearanceInProgress), groups), false},
		{"marketingEventAllocation-cancelled", AllocationMessage(with(allocAllocationCancelled, allocAcceptedByConsultant), groups), false},
		{"internalEngagementAllocation-allocated", AllocationMessage(intAlloc, groups), false},
		{"customerEngagementAllocation-allocated", AllocationMessage(custAlloc, groups), false},
		{"consultantConfirmed", AllocationMessage(confirmed, groups), false},
		{"assignRequests-assigned", TravelAssigned(ts, trip, visa, "Classic Travels"), false},
		{"assignRequests-visa-updated", TravelVisaChanged(ts, trip, visa, "Classic Travels"), false},
		{"completeTravel", TravelCompleted(ts, trip, visa, "Classic Travels", "WSO2 - LK", perDiems), false},
		{"noContent-on-hold", TravelNotice(ts, trip, "Travel On Hold", "This travel request is on hold.", ""), false},
		{"noContent-cancelled", TravelNotice(ts, trip, "Travel Cancelled", "This travel request was cancelled.", ""), false},
		{"noContent-reassigned", TravelNotice(ts, trip, "Travel Reassigned", "This travel request is no longer assigned to Classic Travels.", "Classic Travels"), false},
		{"travelFinanceUpdate-perdiems-added", FinanceTravelMessage(fin, FinancePerDiemsAdded, trip.JobNumber, "WSO2 - LK", "per-diems", items(FinanceItems{PerDiem: &FinancePerDiemItem{
			Currency: "USD", Rate: 50, Total: 100, From: "2026-12-10 09:00:00", To: "2026-12-11 18:00:00", Days: 2}})), false},
		{"travelFinanceUpdate-invoices-added", FinanceTravelMessage(fin, FinanceInvoicesAdded, trip.JobNumber, "WSO2 - LK", "invoices", items(FinanceItems{Invoices: []FinanceInvoiceItem{inv1}})), true},
		{"travelFinanceUpdate-invoices-approved", FinanceTravelMessage(fin, FinanceInvoicesApproved, trip.JobNumber, "", "invoices", items(FinanceItems{Invoices: []FinanceInvoiceItem{inv1, inv2}})), false},
		{"travelFinanceUpdate-invoices-rejected", FinanceTravelMessage(fin, FinanceInvoicesRejected, trip.JobNumber, "WSO2 - LK", "invoices", items(FinanceItems{
			Reason: "The hotel invoice covers two extra nights.", Invoices: []FinanceInvoiceItem{inv2}})), false},
	}
	return now, samples
}

// renderSample renders a sample as the sender would: the page and subject.
func renderSample(s previewSample, now time.Time) (page, subject string, err error) {
	content := map[string]string{}
	for k, v := range s.m.Content {
		content[k] = v
	}
	content["APP_NAME"] = appNames[s.m.App]
	content["CLOSING_AND_SIGNATURE"] = signatures[s.m.App]
	content["CURRENT_YEAR"] = strconv.Itoa(now.Year())
	banner, subject := "", subjectLine(s.m.Subject)
	if s.staging {
		banner = stagingBanner(s.m.Audience)
	}
	page, err = Render(s.m.TemplateID, content, banner)
	return page, subject, err
}

// Every email names the Engagement App (subject tag, APP_NAME, footer), never
// the legacy Allocations or Travel apps, and the footer says they merged.
func TestSamples_EngagementAppNaming(t *testing.T) {
	now, samples := previewSamples()
	for _, s := range samples {
		page, subject, err := renderSample(s, now)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if !strings.HasPrefix(subject, "[Engagement App] ") || strings.Contains(subject, "[STAGING]") || strings.HasPrefix(subject, "Re:") {
			t.Errorf("%s: subject %q lacks the Engagement App tag", s.name, subject)
		}
		for _, gone := range []string{"Allocations App", "Travel App", "Allocation App", "[Allocations]", "[Travel App]"} {
			if strings.Contains(page, gone) || strings.Contains(subject, gone) {
				t.Errorf("%s: still says %q", s.name, gone)
			}
		}
		for _, want := range []string{"automated message from the Engagement App", "The Allocations and Travel apps are now the Engagement App."} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: lacks %q", s.name, want)
			}
		}
	}
}

func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("EMAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("EMAIL_PREVIEW_DIR not set")
	}
	now, samples := previewSamples()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	subjects := map[string]string{}
	for _, s := range samples {
		if s.name != s.m.TemplateID && !strings.HasPrefix(s.name, s.m.TemplateID+"-") {
			t.Errorf("%s: builder produced template %q", s.name, s.m.TemplateID)
			continue
		}
		page, subject, err := renderSample(s, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.name+".html"), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
		subjects[s.name] = subject
	}
	b, _ := json.MarshalIndent(subjects, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "subjects.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSendSamples sends one sample of every email type to the test recipients
// through the normal sender, so a change can be checked in a real inbox
// without clicking through the app. Dev only: skipped unless
// EMAIL_SEND_SAMPLES=true, and it refuses to run when emails are live.
//
//	EMAIL_SEND_SAMPLES=true IS_LIVE_EMAILS=false EMAIL_TEST_RECIPIENTS=me@wso2.com \
//	EMAIL_SERVICE_URL=... OUTBOUND_TOKEN_URL=... OUTBOUND_CLIENT_ID=... OUTBOUND_CLIENT_SECRET=... \
//	EMAIL_ALLOCATIONS_FROM=... EMAIL_TRAVEL_FROM=... \
//	[HR_ENTITY_URL=... EMAIL_SAMPLE_PHOTO_EMAIL=someone@wso2.com] go test ./internal/email -run TestSendSamples -v
func TestSendSamples(t *testing.T) {
	if os.Getenv("EMAIL_SEND_SAMPLES") != "true" {
		t.Skip("EMAIL_SEND_SAMPLES is not true")
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("IS_LIVE_EMAILS")), "true") {
		t.Fatal("refusing to send samples: IS_LIVE_EMAILS=true")
	}
	var recipients []string
	for _, a := range strings.Split(os.Getenv("EMAIL_TEST_RECIPIENTS"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			recipients = append(recipients, a)
		}
	}
	url := strings.TrimSpace(os.Getenv("EMAIL_SERVICE_URL"))
	if len(recipients) == 0 || url == "" {
		t.Fatal("EMAIL_TEST_RECIPIENTS and EMAIL_SERVICE_URL are required")
	}
	oauth := rest.NewOAuthHTTPClient(os.Getenv("OUTBOUND_TOKEN_URL"), os.Getenv("OUTBOUND_CLIENT_ID"), os.Getenv("OUTBOUND_CLIENT_SECRET"), strings.Fields(strings.ReplaceAll(os.Getenv("OUTBOUND_SCOPES"), ",", " ")))
	sender := New(Config{
		ServiceURL:     url,
		From:           map[App]string{AppAllocations: os.Getenv("EMAIL_ALLOCATIONS_FROM"), AppTravel: os.Getenv("EMAIL_TRAVEL_FROM")},
		TestRecipients: recipients,
	}, rest.New("email", url, oauth))
	// EMAIL_SAMPLE_PHOTO_EMAIL (with HR_ENTITY_URL) shows that person's real
	// photo, read from the HR Entity, for the sample consultant. Nothing is
	// stored or committed: the photo is fetched for each run.
	if who := strings.TrimSpace(os.Getenv("EMAIL_SAMPLE_PHOTO_EMAIL")); who != "" {
		hr := hrentity.New(strings.TrimSpace(os.Getenv("HR_ENTITY_URL")), oauth)
		sender.WithPhotos(NewPhotoFetcher(func(ctx context.Context, addr string) (string, error) {
			if addr == "consultant@example.com" {
				addr = who
			}
			return hr.Thumbnail(ctx, addr)
		}))
	}
	// EMAIL_SAMPLES_ONLY (comma-separated sample names) sends just those.
	only := map[string]bool{}
	for _, n := range strings.Split(os.Getenv("EMAIL_SAMPLES_ONLY"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			only[n] = true
		}
	}
	_, samples := previewSamples()
	for _, s := range samples {
		if len(only) > 0 && !only[s.name] {
			continue
		}
		if err := sender.Send(context.Background(), s.m); err != nil {
			t.Errorf("%s: %v", s.name, err)
			continue
		}
		t.Logf("sent %s: %s", s.name, s.m.Subject)
	}
}

// Every email uses only the Choreo Labs palette, and every sample has a
// greeting, one button, a first sentence and a subject in "Action: ID name" form.
func TestSamples_PaletteAndStructure(t *testing.T) {
	allowed := map[string]bool{"#FF4600": true, "#C43300": true, "#0B1633": true, "#050A1C": true, "#1F4FFF": true,
		"#5A6072": true, "#F4F6FA": true, "#FFFFFF": true}
	hex := regexp.MustCompile(`#[0-9A-Fa-f]{6}\b`)
	subjectRe := regexp.MustCompile(`^\[Engagement App\] [A-Z][A-Za-z ]+: \S+ .+`)
	now, samples := previewSamples()
	for _, s := range samples {
		page, subject, err := renderSample(s, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range hex.FindAllString(page, -1) {
			if !allowed[strings.ToUpper(c)] {
				t.Errorf("%s: colour %s is not in the palette", s.name, c)
			}
		}
		if !subjectRe.MatchString(subject) {
			t.Errorf("%s: subject %q", s.name, subject)
		}
		if !strings.Contains(page, ">Hi") || strings.Contains(page, "Hi @") || strings.Contains(page, "<!-- [GREETING") || strings.Count(page, `class="ea-btn"`) > 1 {
			t.Errorf("%s: greeting or button is wrong", s.name)
		}
		if len(s.m.Content["title"]) > 32 {
			t.Errorf("%s: title %q is too long for one line", s.name, s.m.Content["title"])
		}
	}
}
