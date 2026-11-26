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

func TestTravelEmails_Shape(t *testing.T) {
	s := travelTestSettings()
	s.CCTraveller = true
	trip := TravelTrip{JobNumber: "TRV-2026-10-1246", TravellerEmail: "traveller@wso2.com", TravellerName: "Jane Doe", Customer: "Acme & Sons",
		Country: "United Kingdom", City: "<London>", StartDate: "2026-10-06", EndDate: "2026-10-16"}

	m := TravelAssigned(s, trip, TravelVisa{}, "Acorn Travels")
	if m.App != AppTravel || m.TemplateID != "assignRequests" || m.CC[0] != "traveller@wso2.com" ||
		m.Subject != "[Engagement App] Travel Agent Assigned: TRV-2026-10-1246 Jane Doe to United Kingdom" ||
		m.Content["greeting_name"] != "Acorn Travels" || m.Content["action_label"] != "Review Travel Request" ||
		m.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-requests/TRV-2026-10-1246/info" {
		t.Errorf("assigned = %+v", m)
	}
	page := renderMessage(t, m)
	for _, want := range []string{"Acme &amp; Sons", "&lt;London&gt;", "6&#8211;16 Oct 2026", "Acorn Travels", "Hi Acorn Travels,"} {
		if !strings.Contains(page, want) {
			t.Errorf("assigned page lacks %q", want)
		}
	}
	if strings.Contains(page, "N/A") || strings.Contains(page, ">Visa<") {
		t.Error("empty visa block or N/A shown")
	}
	n := TravelNotice(s, trip, "Travel Cancelled", "x", "")
	if len(n.CC) != 1 || n.TemplateID != "noContent" || n.Subject != "[Engagement App] Travel Cancelled: TRV-2026-10-1246 Jane Doe to United Kingdom" {
		t.Errorf("notice = %+v", n)
	}
	// The agency greeted must act; the email still goes To the logistics team.
	if v := TravelVisaChanged(s, trip, TravelVisa{}, "Acorn Travels"); v.Content["greeting_name"] != "Acorn Travels" || v.To[0] != "logistics@wso2.com" {
		t.Errorf("visa changed = %+v", v)
	}
	if v := TravelAssigned(s, trip, TravelVisa{}, ""); v.Content["greeting_name"] != "Travel Logistics Team" {
		t.Errorf("no agency greeting = %q", v.Content["greeting_name"])
	}
	if r := TravelNotice(s, trip, "Travel Reassigned", "x", "Classic Travels"); r.Content["greeting_name"] != "Classic Travels" || r.To[0] != "logistics@wso2.com" {
		t.Errorf("reassigned = %+v", r)
	}
	if n.Content["greeting_name"] != "Travel Logistics Team" {
		t.Errorf("notice greeting = %q", n.Content["greeting_name"])
	}
	// Completed goes To the company's finance contacts, with logistics and the traveller copied.
	c := TravelCompleted(s, trip, TravelVisa{VisaType: "Work Permit", StartDate: "2026-01-01 00:00:00"}, "Acorn", "WSO2 - UK", nil)
	if c.Content["greeting_name"] != "Finance Team" || c.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-requests/TRV-2026-10-1246/per-diems" ||
		strings.Join(c.To, ",") != "uk1@wso2.com,uk2@wso2.com" ||
		strings.Join(c.CC, ",") != "logistics@wso2.com,ukpay@wso2.com,traveller@wso2.com" {
		t.Errorf("completed = %+v", c)
	}
	c = TravelCompleted(s, trip, TravelVisa{}, "Acorn", "", []TravelPerDiem{{StartDate: "2026-10-06 09:00:00", NoOfDays: "11", Currency: "USD", Rate: "70.0", Total: "770.0"}})
	if d := c.Content["details_html"]; !strings.Contains(d, "6 Oct 2026, 09:00") || !strings.Contains(d, "USD 770.00") {
		t.Errorf("table = %s", d)
	}

	var content map[string]string
	if err := json.Unmarshal([]byte(AuditContent(m, 2026)), &content); err != nil ||
		content["CURRENT_YEAR"] != "2026" || content["APP_NAME"] != "Engagement App" || content["title"] != "Travel Agent Assigned" {
		t.Errorf("audit content = %v %v", content, err)
	}
	if TravelFormatNumber(70) != "70.0" || TravelFormatNumber(12.5) != "12.5" {
		t.Error("number format")
	}
}
