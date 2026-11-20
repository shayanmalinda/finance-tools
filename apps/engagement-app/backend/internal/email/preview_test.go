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
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestWritePreviews renders one realistic sample of every template, built by
// the real message builders, into $EMAIL_PREVIEW_DIR (plus subjects.json) to
// review the design in a browser. Skipped unless EMAIL_PREVIEW_DIR is set.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("EMAIL_PREVIEW_DIR")
	if dir == "" {
		t.Skip("EMAIL_PREVIEW_DIR not set")
	}
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	base := "https://one-stg.wso2.com"
	eng := EngagementEmailData{
		ID: "E06405", EngagementTypeID: 3, RequestStatus: "New", CustomerName: "Fidelity Investments",
		ProjectName: "Fidelity ELA Deal - 2026 - Renewal [Y3 of 5]", ProductName: "WSO2 API Manager",
		AllocationTypeName: "Pre-Sale Engagement", EngagementCode: "CF3rV-PR", Nature: "On-site", Country: "United States of America",
		Address: "245 Summer Street, Boston, Massachusetts 02210-1133", StartDate: "2027-11-01", EndDate: "2027-11-12",
		CustomerContact: "Jordan Lee / +1 555 0100", BusinessDomain: "Financial Services", OpportunityOwner: "Lochana De Alwis",
		TechnicalOwner: "mohan@wso2.com", ContractsSigned: true, EngagementScope: "API platform health check and upgrade plan.",
		Comment: "Customer prefers the first week on site.", BusinessUnit: "IAM", ProductUnit: "IAM", SubRegion: "NA - EAST",
		Requester: "requester@example.com",
	}
	rec := EngagementRecipients{AppBaseURL: base, InternalApprovers: []string{"approver@example.com"}, MarketingApprovers: []string{"marketing@example.com"}}
	internal := eng
	internal.EngagementTypeID, internal.ProjectName, internal.AllocationTypeName, internal.EngagementCode = 1, "Bug Bash - Q4", "Bug Busting", "RND-GEN"
	internal.CustomerName = "WSO2"
	event := eng
	event.EngagementTypeID, event.ProjectName, event.AllocationTypeName, event.EngagementCode = 5, "Choreo Labs Summit 2027", "Conference", "EVT-SUM"
	event.CustomerName = "Choreo Labs"

	alloc := AllocationEmailData{
		EngagementTypeID: 3, EngagementID: "E06405", CustomerName: eng.CustomerName, OpportunityName: eng.ProjectName,
		ProductName: eng.ProductName, AllocationTypeName: eng.AllocationTypeName, EngagementCode: eng.EngagementCode,
		EngagementNature: eng.Nature, Country: eng.Country, Address: eng.Address, CustomerContact: eng.CustomerContact,
		BusinessDomain: eng.BusinessDomain, OpportunityOwner: eng.OpportunityOwner, TechnicalOwner: eng.TechnicalOwner,
		ContractsSigned: true, EngagementScope: eng.EngagementScope, EngagementComment: eng.Comment,
		FundingSources:  []AllocationEmailFunding{{BusinessUnit: "IAM", ProductUnit: "IAM", SubRegion: "NA - EAST"}},
		ConsultantEmail: "consultant@example.com", ClearanceStatus: allocClearanceInProgress, PrevClearanceStatus: allocReadyForClearance,
		TravelRequired: true, StartDate: "2027-11-01", EndDate: "2027-11-12", AllocationComment: "Lead the API gateway track.",
		Actor: "allocator@example.com", Now: now,
	}
	allocInternal, allocEvent, confirmed := alloc, alloc, alloc
	allocInternal.EngagementTypeID, allocInternal.OpportunityName = 1, internal.ProjectName
	allocEvent.EngagementTypeID, allocEvent.OpportunityName = 5, event.ProjectName
	confirmed.ClearanceStatus, confirmed.PrevClearanceStatus = allocConfirmed, allocClearanceInProgress
	confirmed.Visas = []AllocationEmailVisa{{Country: "United States of America", VisaType: "Business/Visit", EntryType: "Multiple", Status: "Active", StartDate: "2026-03-01", EndDate: "2027-11-15"}}
	groups := AllocationEmailGroups{AllocationsGroup: "allocations@example.com", TravelGroup: "travel@example.com"}

	ts := TravelSettings{LogisticsGroup: "travel-logistics@example.com", FinanceGroup: "finance@example.com", AppBaseURL: base}
	trip := TravelTrip{JobNumber: "TRV-2026-11-0003", TravellerEmail: "traveller@example.com", Customer: "Fidelity Investments",
		Country: "United States of America", City: "Boston", StartDate: "2027-11-01", EndDate: "2027-11-12"}
	visa := TravelVisa{VisaType: "Business/Visit", Country: "United States of America", EntryType: "Multiple", StartDate: "2026-03-01", EndDate: "2027-11-15"}
	perDiems := []TravelPerDiem{
		{StartDate: "2027-11-01 09:00:00", EndDate: "2027-11-06 18:00:00", NoOfDays: "6", Currency: "USD", Rate: "75.00", Total: "450.00"},
		{StartDate: "2027-11-07 09:00:00", EndDate: "2027-11-12 18:00:00", NoOfDays: "6", Currency: "USD", Rate: "75.00", Total: "450.00"},
	}

	samples := []struct {
		name    string
		m       Message
		staging bool
	}{
		{"customerEngagement", EngagementMessage(EngagementCreated, eng, rec, "requester@example.com"), true},
		{"customerEngagementCancelled", EngagementMessage(EngagementCancelled, eng, rec, "requester@example.com"), false},
		{"internalEngagement", EngagementMessage(EngagementCreated, internal, rec, "requester@example.com"), false},
		{"internalEngagementCancelled", EngagementMessage(EngagementCancelled, internal, rec, "requester@example.com"), false},
		{"marketingEvent", EngagementMessage(EngagementCreated, event, rec, "requester@example.com"), false},
		{"marketingEventCancelled", EngagementMessage(EngagementCancelled, event, rec, "requester@example.com"), false},
		{"customerEngagementAllocation", AllocationMessage(alloc, groups), false},
		{"internalEngagementAllocation", AllocationMessage(allocInternal, groups), false},
		{"marketingEventAllocation", AllocationMessage(allocEvent, groups), false},
		{"consultantConfirmed", AllocationMessage(confirmed, groups), false},
		{"assignRequests", TravelAssigned(ts, trip, visa, "Classic Travels"), false},
		{"completeTravel", TravelCompleted(ts, trip, visa, "Classic Travels", "WSO2 - USA", perDiems), false},
		{"noContent", TravelNotice(ts, trip.JobNumber, trip.TravellerEmail, "Classic Travels", "Cancelled", "Please note that this travel request has been cancelled"), false},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	subjects := map[string]string{}
	for _, s := range samples {
		if s.m.TemplateID != s.name {
			t.Errorf("%s: builder produced template %q", s.name, s.m.TemplateID)
			continue
		}
		content := map[string]string{}
		for k, v := range s.m.Content {
			content[k] = v
		}
		content["APP_NAME"] = appNames[s.m.App]
		content["CLOSING_AND_SIGNATURE"] = signatures[s.m.App]
		content["CURRENT_YEAR"] = strconv.Itoa(now.Year())
		banner, subject := "", subjectLine(s.m.Subject)
		if s.staging {
			to, cc := FilterRecipients(s.m.To, s.m.CC, nil)
			banner, subject = stagingBanner(to, cc), stagingSubjectPrefix+subject
		}
		page, err := Render(s.m.TemplateID, content, banner)
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
