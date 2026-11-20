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
	if want := []string{"allocations@wso2.com", "jane@wso2.com", "lead@wso2.com", "mgr@wso2.com", "travel@wso2.com"}; !reflect.DeepEqual(m.To, want) {
		t.Errorf("to = %v", m.To)
	}
	if m.Subject != "Re: [Engagement App] Consultant request for E07201 : Acme - Acme <PoC>" {
		t.Errorf("subject = %s", m.Subject)
	}
	c := m.Content
	if c["title"] != "Allocation Confirmed for Engagement" || c["consultant"] != "jane@wso2.com" ||
		c["project_name"] != "Acme &lt;PoC&gt;" || c["business_unit"] != "APIM" || c["location_address"] != "N/A" ||
		c["travel_readiness_comment"] != "Visa available for the allocation period" ||
		!strings.Contains(c["visa_data"], "Sri Lanka: Business Multiple Entry, Visa Approved from 2026-01-01 to 2026-11-08") ||
		!strings.Contains(c["visa_data"], "expiring within a week") || c["project_name_label"] != "Opportunity" {
		t.Errorf("content = %v", c)
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
	base := AllocationEmailData{EngagementTypeID: 5, EngagementID: "EVT00001", ConsultantEmail: "jane@wso2.com", Actor: "mgr@wso2.com"}

	d := base
	d.ClearanceStatus = "Clearance In Progress"
	m := AllocationMessage(d, allocGroups)
	if m.TemplateID != "marketingEventAllocation" || m.Content["title"] != "Allocation Added for Marketing Event" || m.Content["to"] != "jane@wso2.com" {
		t.Errorf("CIP = %s %v", m.TemplateID, m.Content)
	}

	d = base
	d.EngagementTypeID = 1
	d.ClearanceStatus, d.PrevClearanceStatus, d.TravelRequired = "Rejected by Consultant", "Confirmed", true
	m = AllocationMessage(d, allocGroups)
	if m.Content["title"] != "Allocation Declined for Internal Engagement" || m.To[len(m.To)-1] != "travel@wso2.com" || m.Content["info_title"] != "Engagement Info" {
		t.Errorf("declined = %v %v", m.To, m.Content)
	}

	d = base
	d.EngagementTypeID = 3
	d.ConsultantEmail = "partner-allocations-group+Nimbus+Arjun+Rao@wso2.com"
	d.ClearanceStatus = "Confirmed"
	m = AllocationMessage(d, allocGroups)
	if !reflect.DeepEqual(m.To, []string{"partners@wso2.com", "mgr@wso2.com"}) || m.Content["visa_data"] != "" || m.Content["travel_readiness_comment"] != "" {
		t.Errorf("partner = %v %v", m.To, m.Content)
	}

	d = base
	d.ClearanceStatus = "Confirmed"
	m = AllocationMessage(d, allocGroups)
	if m.Content["travel_readiness_comment"] != "Visa not available" || m.Content["title"] != "Allocation Confirmed for Marketing Event" {
		t.Errorf("no visas = %v", m.Content)
	}
}
