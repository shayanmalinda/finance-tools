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
	"encoding/json"
	"strings"
	"testing"
)

func travelTestSettings() TravelSettings {
	return TravelSettings{
		LogisticsGroup: "logistics@wso2.com", FinanceGroup: "finance@wso2.com", AppBaseURL: "https://one.wso2.com/",
		CompanyRecipients: map[string][]string{
			"WSO2 - UK":    {"uk1@wso2.com", "uk2@wso2.com", "ukpay@wso2.com"},
			`"WSO2 - SG"`:  {"sg@wso2.com"},
			"WSO2 - SPAIN": {},
			"WSO2- INDIA ": {"in@wso2.com", "inpay@wso2.com"},
		},
	}
}

func TestTravelFinanceRecipients(t *testing.T) {
	s := travelTestSettings()
	tests := []struct{ company, to, cc string }{
		{"WSO2 - UK", "uk1@wso2.com,uk2@wso2.com", "ukpay@wso2.com"},
		{"wso2 - sg", "sg@wso2.com", "finance@wso2.com"}, // quoted key, one address
		{"WSO2 - SPAIN", "finance@wso2.com", ""},         // no addresses
		{"WSO2- INDIA", "in@wso2.com", "inpay@wso2.com"}, // trimmed key
		{"WSO2 Inc", "finance@wso2.com", ""},             // no match
		{"", "finance@wso2.com", ""},                     // unknown company
	}
	for _, tc := range tests {
		to, cc := TravelFinanceRecipients(s, tc.company)
		if strings.Join(to, ",") != tc.to || strings.Join(cc, ",") != tc.cc {
			t.Errorf("%q: to %v cc %v", tc.company, to, cc)
		}
	}
}

func TestTravelEmails_LegacyShape(t *testing.T) {
	s := travelTestSettings()
	s.CCTraveller = true
	trip := TravelTrip{JobNumber: "TRV-2026-10-1246", TravellerEmail: "traveller@wso2.com", Customer: "Acme & Sons",
		Country: "United Kingdom", City: "<London>", StartDate: "2026-10-06", EndDate: "2026-10-16"}

	m := TravelAssigned(s, trip, TravelVisa{}, "Acorn Travels")
	if m.App != AppTravel || m.TemplateID != "assignRequests" || m.CC[0] != "traveller@wso2.com" ||
		m.Content["customer"] != "Acme &amp; Sons" || m.Content["travel_location"] != "&lt;London&gt;" ||
		m.Content["visa_type"] != "N/A" || m.Content["visa_start_date"] != "N/A" ||
		m.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-requests/TRV-2026-10-1246/info" {
		t.Errorf("assigned = %+v", m)
	}
	n := TravelNotice(s, trip.JobNumber, "", "All", "Travel Request is Cancelled", "x")
	if len(n.Content) != 4 || len(n.CC) != 0 || n.TemplateID != "noContent" {
		t.Errorf("notice = %+v", n)
	}
	c := TravelCompleted(s, trip, TravelVisa{VisaType: "Work Permit", StartDate: "2026-01-01 00:00:00"}, "Acorn", "WSO2 - UK", nil)
	if c.Content["table_data"] != "No Perdiems" || c.Content["visa_start_date"] != "2026-01-01" ||
		strings.Join(c.To, ",") != "logistics@wso2.com,uk1@wso2.com,uk2@wso2.com" ||
		strings.Join(c.CC, ",") != "ukpay@wso2.com,traveller@wso2.com" {
		t.Errorf("completed = %+v", c)
	}
	c = TravelCompleted(s, trip, TravelVisa{}, "Acorn", "", []TravelPerDiem{{StartDate: "2026-10-06 09:00:00", NoOfDays: "11", Total: "770.0"}})
	if !strings.Contains(c.Content["table_data"], "<td style=\"padding: 5px 5px;\">2026-10-06 09:00</td>") {
		t.Errorf("table = %s", c.Content["table_data"])
	}

	var content map[string]string
	if err := json.Unmarshal([]byte(TravelAuditContent(m, 2026)), &content); err != nil ||
		content["CURRENT_YEAR"] != "2026" || content["APP_NAME"] != "Travel App" || content["title"] != "New Travel Request Assigned" {
		t.Errorf("audit content = %v %v", content, err)
	}
	if TravelFormatNumber(70) != "70.0" || TravelFormatNumber(12.5) != "12.5" {
		t.Error("number format")
	}
}
