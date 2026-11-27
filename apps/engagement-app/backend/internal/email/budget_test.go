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
	"strings"
	"testing"
)

func TestBudgetLineNeededMessage(t *testing.T) {
	s := BudgetSettings{Recipients: []string{"owner@example.com"}, AppBaseURL: "https://one.wso2.com/"}
	trip := BudgetLineTrip{JobNumber: "TRV-2027-01-0042", TravellerName: "Jane <Doe>", TravellerEmail: "jane@example.com",
		Country: "Sri Lanka", StartDate: "2027-01-18", EndDate: "2027-01-22", Engagement: "E06405 Acme & Sons", AllocationType: "Pre-Sale Engagement"}
	trip.Year = 2027
	m := BudgetLineNeededMessage(s, trip)
	if m.App != AppTravel || m.TemplateID != FinanceTemplate || m.NotificationType != "travelBudgetLineNeeded" ||
		m.ReferenceID != trip.JobNumber || len(m.To) != 1 || m.To[0] != "owner@example.com" || len(m.CC) != 0 ||
		m.Subject != "[Engagement App] Budget Line Needed: TRV-2027-01-0042 Jane <Doe> to Sri Lanka" ||
		m.Content["title"] != "Budget Line Needed" || m.Content["greeting_name"] != "Travel Budget Owner" ||
		m.Content["action_label"] != "Choose Budget Line" ||
		m.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-budgets/needs-a-line?job=TRV-2027-01-0042&year=2027" ||
		len(m.Audience.To) != 1 || m.Audience.To[0] != "the travel budget owner" {
		t.Errorf("message = %+v", m)
	}
	for k, v := range m.Content {
		if k != "details_html" && k != "action_link" && strings.Contains(v, ";") {
			t.Errorf("%s has a semicolon: %q", k, v)
		}
	}
	page := renderMessage(t, m)
	for _, want := range []string{"Jane &lt;Doe&gt;", "Acme &amp; Sons", "Pre-Sale Engagement", "18&#8211;22 Jan 2027", "Hi Travel Budget Owner,", "Choose Budget Line"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// The link is escaped for a job number that is not a plain id.
	trip.JobNumber = "A B&C"
	if got := BudgetLineNeededMessage(s, trip).Content["action_link"]; !strings.HasSuffix(got, "?job=A+B%26C&year=2027") {
		t.Errorf("action_link = %q", got)
	}
}
