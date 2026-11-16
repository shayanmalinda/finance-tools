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
	"html"
	"net/url"
	"strings"
)

// The travel finance notifications (legacy Travel app
// sendNoContentTravelEmail with isSendToFinanceGroup = true).
const (
	FinancePerDiemsAdded    = "perdiems-added"
	FinanceInvoicesAdded    = "invoices-added"
	FinanceInvoicesApproved = "invoices-approved"
	FinanceInvoicesRejected = "invoices-rejected"
)

var financeTexts = map[string][2]string{
	FinancePerDiemsAdded:    {"New per diems have been added", "Please note that there are newly added per diems."},
	FinanceInvoicesAdded:    {"New travel invoices have been added", "Please note that there are newly added travel invoices."},
	FinanceInvoicesApproved: {"Travel invoices have been approved", "Please note that there are newly approved travel invoices."},
	FinanceInvoicesRejected: {"Travel invoices have been rejected", "Please note that there are newly rejected travel invoices."},
}

// FinanceRecipients are the addresses the travel finance emails use (from
// config.EmailConfig).
type FinanceRecipients struct {
	TravelLogisticsGroup string
	FinanceGroup         string
	// CompanyRecipients maps an HR company to its finance addresses
	// (companyEmailRecipientsMapping).
	CompanyRecipients map[string][]string
	// AppBaseURL is the One WSO2 base URL for the action link.
	AppBaseURL string
}

// financeEntityRecipients ports getEmailRecipientsForEntity: with exactly one
// company entry matching (trimmed, case-insensitive), every address but the
// last goes to To and the last to CC; a single address goes to To with the
// finance group in CC (the legacy dropped that single address, a bug); no
// match or an empty entry sends to the finance group.
func financeEntityRecipients(r FinanceRecipients, company string) (to, cc []string) {
	company = strings.TrimSpace(strings.Trim(strings.TrimSpace(company), `"`))
	var match []string
	n := 0
	for k, v := range r.CompanyRecipients {
		if strings.EqualFold(strings.TrimSpace(strings.Trim(k, `"`)), company) {
			n++
			match = v
		}
	}
	if n != 1 || len(match) == 0 {
		return []string{r.FinanceGroup}, nil
	}
	if len(match) == 1 {
		return []string{match[0]}, []string{r.FinanceGroup}
	}
	return append([]string{}, match[:len(match)-1]...), []string{match[len(match)-1]}
}

// FinanceTravelMessage builds a travel finance email for jobNumber:
// subject "[Travel App] Travel Request (#job)", template noContent, To the
// travel logistics group plus the traveller company's finance recipients
// (the finance group when the company is unknown). tab is the trip page tab
// the action link opens (per-diems, invoices).
func FinanceTravelMessage(r FinanceRecipients, kind, jobNumber, travellerCompany, tab string) Message {
	texts := financeTexts[kind]
	to := []string{r.TravelLogisticsGroup}
	var cc []string
	if travellerCompany == "" {
		to = append(to, r.FinanceGroup)
	} else {
		t, c := financeEntityRecipients(r, travellerCompany)
		to = append(to, t...)
		cc = append(cc, c...)
	}
	link := strings.TrimRight(r.AppBaseURL, "/") + "/finance/engagement-app/travel-requests/" + url.PathEscape(jobNumber) + "/" + tab
	return Message{
		App:        AppTravel,
		To:         to,
		CC:         cc,
		Subject:    "[Travel App] Travel Request (#" + jobNumber + ")",
		TemplateID: "noContent",
		Content: map[string]string{
			"title":             texts[0],
			"description":       texts[1],
			"travel_agent_name": "All",
			"action_link":       html.EscapeString(link),
		},
		NotificationType: kind,
		ReferenceID:      jobNumber,
	}
}
