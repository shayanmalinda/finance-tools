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
	"net/url"
	"strings"
)

// BudgetLineNeeded is the notification type of the "needs a budget line" email.
const BudgetLineNeeded = "travelBudgetLineNeeded"

// BudgetSettings are the addresses and links the travel budget email uses.
type BudgetSettings struct {
	// Recipients (BUDGET_LINE_RECIPIENTS) are the To; no CC.
	Recipients []string
	// AppBaseURL is the One WSO2 base URL for the action link.
	AppBaseURL string
}

// BudgetLineTrip is the trip the email names.
type BudgetLineTrip struct {
	JobNumber      string
	TravellerName  string
	TravellerEmail string
	Country        string
	// StartDate and EndDate are YYYY-MM-DD.
	StartDate      string
	EndDate        string
	Engagement     string
	AllocationType string
}

// BudgetLineNeededMessage asks the travel budget owner to choose a line for a
// trip the system could not place, using the generic finance template.
func BudgetLineNeededMessage(s BudgetSettings, t BudgetLineTrip) Message {
	link := strings.TrimRight(s.AppBaseURL, "/") + "/finance/engagement-app/travel-budgets/needs-a-line?job=" + url.QueryEscape(t.JobNumber)
	details := detailsSection("Trip",
		row{"Job Number", t.JobNumber},
		personRow("Traveller", t.TravellerName, t.TravellerEmail),
		row{"Destination", t.Country},
		row{"Dates", dateRange(t.StartDate, t.EndDate)},
		row{"Engagement", t.Engagement},
		row{"Allocation Type", t.AllocationType})
	return Message{
		App:        AppTravel,
		To:         append([]string{}, s.Recipients...),
		Subject:    subjectFor("Budget Line Needed", t.JobNumber, tripName(t.TravellerName, t.Country)),
		TemplateID: FinanceTemplate,
		Content: map[string]string{
			"title":         "Budget Line Needed",
			"description":   "This travel request could not be matched to a travel budget line. Please choose one so its cost counts against the right budget.",
			"greeting_name": "Travel Budget Owner",
			"reason":        "You are receiving this because you choose budget lines for travel requests the system cannot place.",
			"action_label":  "Choose Budget Line",
			"action_link":   link,
			"details_html":  details,
		},
		NotificationType: BudgetLineNeeded,
		ReferenceID:      t.JobNumber,
		Audience:         Audience{To: []string{"the travel budget owner"}},
	}
}
