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
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAllocationMailInitiated_LegacyGate(t *testing.T) {
	cases := []struct {
		c, prev string
		want    bool
	}{
		{"Clearance In Progress", "", true},
		{"Ready for Clearance", "", false},
		{"Tentative", "", false},
		{"Confirmed", "", true},
		{"Confirmed", "Confirmed - Visa Pending", false},
		{"Confirmed - Visa Pending", "Confirmed", true},
		{"Allocation Cancelled", "Ready for Clearance", false},
		{"Allocation Cancelled", "Confirmed", true},
		{"Accepted by Consultant", "Clearance In Progress", true},
		{"Rejected - Visa Issues", "Confirmed", true},
		{"Rejected - Other", "Confirmed", true},
	}
	for _, tc := range cases {
		if got := AllocationMailInitiated(tc.c, tc.prev); got != tc.want {
			t.Errorf("%q from %q = %v, want %v", tc.c, tc.prev, got, tc.want)
		}
	}
}

var allocGroups = AllocationEmailGroups{AllocationsGroup: "allocations@wso2.com", PartnerAllocationsGroup: "partners@wso2.com", TravelGroup: "travel@wso2.com"}

func TestAllocationMessage_ConfirmedListsVisasAndCopiesTravel(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	m := AllocationMessage(AllocationEmailData{
		EngagementTypeID: 6, EngagementID: "E07201", CustomerName: "Acme", OpportunityName: "Acme <PoC>",
		ConsultantEmail: "jane@wso2.com", ClearanceStatus: "Confirmed", TravelRequired: true, Actor: "mgr@wso2.com",
		Leads: []string{"lead@wso2.com"}, Country: "Sri Lanka", StartDate: "2026-11-10", EndDate: "2026-11-20",
		FundingSources: []AllocationEmailFunding{{BusinessUnit: "APIM", ProductUnit: "PU", SubRegion: "NA"}},
		Visas:          []AllocationEmailVisa{{VisaType: "Business", EntryType: "Multiple", Status: "Approved", StartDate: "2026-01-01", EndDate: "2026-11-08"}},
		Now:            now,
	}, allocGroups)
	if m.TemplateID != "consultantConfirmed" || m.App != AppAllocations {
		t.Errorf("template = %s", m.TemplateID)
	}
	// Travel must arrange the trip, so it is To; everyone else is copied.
	if !reflect.DeepEqual(m.To, []string{"travel@wso2.com"}) ||
		!reflect.DeepEqual(m.CC, []string{"jane@wso2.com", "lead@wso2.com", "allocations@wso2.com", "mgr@wso2.com"}) {
		t.Errorf("to = %v cc = %v", m.To, m.CC)
	}
	if m.Subject != "[Engagement App] Allocation Confirmed: E07201 Acme <PoC>" {
		t.Errorf("subject = %s", m.Subject)
	}
	c := m.Content
	if c["title"] != "Allocation Confirmed" || c["greeting_name"] != "Travel Team" || c["action_label"] != "Review Travel Requests" ||
		!strings.HasSuffix(c["action_link"], "/finance/engagement-app/travel-requests/new") {
		t.Errorf("content = %v", c)
	}
	for _, want := range []string{"Acme &lt;PoC&gt;", "APIM", "Visa available for the allocation period", "10–20 Nov 2026",
		"Sri Lanka: Business Multiple Entry, Visa Approved from 2026-01-01 to 2026-11-08", "expires within a week"} {
		if !strings.Contains(c["details_html"], want) {
			t.Errorf("details lack %q", want)
		}
	}
	if strings.Contains(c["details_html"], "N/A") {
		t.Error("details show N/A")
	}
}

func TestAllocVisaData_LabelsEachVisaWithItsOwnCountry(t *testing.T) {
	data, _ := allocVisaData(AllocationEmailData{Country: "Sri Lanka", Now: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Visas: []AllocationEmailVisa{
			{Country: "India", VisaType: "Business", EntryType: "Single", Status: "Approved", StartDate: "2026-01-01", EndDate: "2027-01-01"},
			{VisaType: "Work", EntryType: "Multiple", Status: "Approved", StartDate: "2026-01-01", EndDate: "2027-01-01"},
		}})
	if !strings.Contains(data, "India: Business Single Entry") || !strings.Contains(data, "Sri Lanka: Work Multiple Entry") ||
		strings.Contains(data, "Sri Lanka: Business") {
		t.Errorf("visa_data = %s", data)
	}
}

func TestAllocationMessage_BranchesAndPartner(t *testing.T) {
	base := AllocationEmailData{EngagementTypeID: 5, EngagementID: "EVT00001", OpportunityName: "Summit", ConsultantEmail: "jane@wso2.com",
		ConsultantName: "Jane", Actor: "mgr@wso2.com", Leads: []string{"lead@wso2.com"}}

	// Allocated: the consultant acts, so To is the consultant by first name.
	d := base
	d.ClearanceStatus = "Clearance In Progress"
	m := AllocationMessage(d, allocGroups)
	if m.TemplateID != "marketingEventAllocation" || m.Subject != "[Engagement App] Allocated: EVT00001 Summit" ||
		m.Content["greeting_name"] != "Jane" || !reflect.DeepEqual(m.To, []string{"jane@wso2.com"}) ||
		!reflect.DeepEqual(m.CC, []string{"lead@wso2.com", "allocations@wso2.com", "mgr@wso2.com"}) || m.Content["action_link"] != "/me/allocations" {
		t.Errorf("allocated = %+v", m)
	}

	// Declined: the allocations team reallocates; travel is copied when arranged.
	d = base
	d.EngagementTypeID = 1
	d.ClearanceStatus, d.PrevClearanceStatus, d.TravelRequired = "Rejected by Consultant", "Confirmed", true
	m = AllocationMessage(d, allocGroups)
	if m.Content["title"] != "Allocation Declined" || m.Content["greeting_name"] != "Allocations Team" || m.Content["action_label"] != "Reallocate" ||
		m.Content["action_link"] != "/finance/engagement-app/engagements/EVT00001/allocate" ||
		!reflect.DeepEqual(m.To, []string{"allocations@wso2.com"}) || m.CC[len(m.CC)-1] != "travel@wso2.com" {
		t.Errorf("declined = %+v", m)
	}

	// Accepted: the allocations team reviews it.
	d = base
	d.ClearanceStatus, d.PrevClearanceStatus = "Accepted by Consultant", "Clearance In Progress"
	if m = AllocationMessage(d, allocGroups); m.Content["title"] != "Allocation Accepted" || !reflect.DeepEqual(m.To, []string{"allocations@wso2.com"}) {
		t.Errorf("accepted = %+v", m)
	}

	// A partner's emails go To the partner allocations team, with no consultant or leads.
	d = base
	d.EngagementTypeID = 3
	d.ConsultantEmail = "partner-allocations-group+Nimbus+Arjun+Rao@wso2.com"
	d.ClearanceStatus = "Confirmed"
	m = AllocationMessage(d, allocGroups)
	if !reflect.DeepEqual(m.To, []string{"partners@wso2.com"}) || !reflect.DeepEqual(m.CC, []string{"mgr@wso2.com"}) ||
		m.Content["greeting_name"] != "Partner Allocations Team" || strings.Contains(m.Content["details_html"], "Visa") {
		t.Errorf("partner = %+v", m)
	}

	// Confirmed without travel goes To the consultant; no visas is stated.
	d = base
	d.ClearanceStatus = "Confirmed"
	m = AllocationMessage(d, allocGroups)
	if !reflect.DeepEqual(m.To, []string{"jane@wso2.com"}) || !strings.Contains(m.Content["details_html"], "Visa not available") {
		t.Errorf("confirmed = %+v", m)
	}
}

// With no travel team address a travel-required confirmation goes To the
// allocations team instead of being dropped for want of a recipient.
func TestAllocationMessage_ConfirmedWithoutTravelGroup(t *testing.T) {
	g := allocGroups
	g.TravelGroup = ""
	m := AllocationMessage(AllocationEmailData{
		EngagementTypeID: 6, EngagementID: "E07201", CustomerName: "Acme", OpportunityName: "Acme",
		ConsultantEmail: "jane@wso2.com", ClearanceStatus: "Confirmed", TravelRequired: true, Actor: "mgr@wso2.com",
		StartDate: "2026-11-10", EndDate: "2026-11-20", Now: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
	}, g)
	if !reflect.DeepEqual(m.To, []string{"allocations@wso2.com"}) || m.Content["greeting_name"] != "Allocations Team" {
		t.Errorf("to = %v, greeting = %q", m.To, m.Content["greeting_name"])
	}
}
