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
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

// The travel request emails of the legacy Travel app
// (apps/travel/backend/modules/email/email.bal): same templates, subjects,
// recipients and content keys.

// Travel email template ids.
const (
	TravelTemplateAssign    = "assignRequests"
	TravelTemplateNoContent = "noContent"
	TravelTemplateCompleted = "completeTravel"
)

// The legacy APP_NAME and CLOSING_AND_SIGNATURE of the Travel app, written
// into the email_auditlog content as the legacy app did.
const (
	travelAppName   = engagementAppName
	travelSignature = digitalTeamSignature
)

// TravelSettings are the legacy emailConfigurations the travel emails read.
type TravelSettings struct {
	// LogisticsGroup is travelLogisticGroupEmail, the To of every email.
	LogisticsGroup string
	// FinanceGroup is financeGroupEmail.
	FinanceGroup string
	// AppBaseURL is the One WSO2 base URL for action links.
	AppBaseURL string
	// CCTraveller is the legacy isSendEmailToTraveller (default false).
	CCTraveller bool
	// CompanyRecipients maps an HR company to its finance addresses
	// (legacy companyEmailRecipientsMapping).
	CompanyRecipients map[string][]string
}

// TravelTrip is the travel request data the emails show.
type TravelTrip struct {
	JobNumber      string
	TravellerEmail string
	// TravellerName is the traveller's name from HR ("" when unknown).
	TravellerName string
	Customer      string
	Country       string
	City          string
	// StartDate and EndDate are YYYY-MM-DD.
	StartDate string
	EndDate   string
}

// TravelVisa is the visa data the emails show; empty fields print "N/A".
type TravelVisa struct {
	VisaType  string
	Country   string
	EntryType string
	StartDate string
	EndDate   string
}

// TravelPerDiem is one Active per diem of the Completed email. Dates are
// "YYYY-MM-DD HH:MM:SS"; numbers are printed as the legacy app did.
type TravelPerDiem struct {
	StartDate string
	EndDate   string
	NoOfDays  string
	Currency  string
	Rate      string
	Total     string
}

// travelSubject is "[Engagement App] <Action>: <Job> <Traveller> to <Country>".
func travelSubject(action string, t TravelTrip) string {
	return subjectFor(action, t.JobNumber, tripName(t.TravellerName, t.Country))
}

// tripName is the record name of a trip: "Jane Doe to Sri Lanka".
func tripName(traveller, country string) string {
	if emptyValue(traveller) {
		if emptyValue(country) {
			return "Travel request"
		}
		return "Trip to " + strings.TrimSpace(country)
	}
	return joinNonEmpty(" to ", traveller, country)
}

// TravelActionLink is a tab of the travel request page in One WSO2 (info,
// per-diems, invoices).
func TravelActionLink(baseURL, jobNumber string, tab ...string) string {
	t := "info"
	if len(tab) > 0 {
		t = tab[0]
	}
	return strings.TrimRight(baseURL, "/") + "/finance/engagement-app/travel-requests/" +
		url.PathEscape(jobNumber) + "/" + t
}

func travelCC(s TravelSettings, travellerEmail string) []string {
	if s.CCTraveller && travellerEmail != "" {
		return []string{travellerEmail}
	}
	return []string{}
}

// travelCCRole is the CC role label of the traveller copy (travelCC).
const travelCCRole = "the traveller, when traveller copies are on"

// travelDetails is the summary of a trip and its visa (empty rows are left out).
func travelDetails(t TravelTrip, v TravelVisa, agentName string) string {
	return detailsSection("Trip",
		row{"Job Number", t.JobNumber}, personRow("Traveller", joinNonEmpty(" ", t.TravellerName), t.TravellerEmail),
		row{"Dates", dateRange(t.StartDate, t.EndDate)},
		row{"Destination", joinNonEmpty(", ", t.City, t.Country)},
		row{"Customer", t.Customer}, row{"Travel Agent", agentName}) +
		detailsSection("Visa",
			row{"Visa", joinNonEmpty(" ", v.VisaType, v.EntryType)}, row{"Country", v.Country},
			row{"Valid", dateRange(v.StartDate, v.EndDate)})
}

// travelContent is the content every travel email shares.
func travelContent(s TravelSettings, t TravelTrip, action, description, name, label, tab, reason string) map[string]string {
	return map[string]string{
		"title":         action,
		"description":   html.EscapeString(description),
		"greeting_name": name,
		"reason":        "You are receiving this because " + reason + ", or because you are copied on it.",
		"action_label":  label,
		"action_link":   TravelActionLink(s.AppBaseURL, t.JobNumber, tab),
	}
}

// TravelAssigned is "Travel Agent Assigned" (legacy sendAssignRequestEmail):
// on assigning In Progress with an agent and a visa, and to the new agent
// when the agent changes.
func TravelAssigned(s TravelSettings, t TravelTrip, v TravelVisa, agentName string) Message {
	assigned := "This travel request was assigned."
	if !emptyValue(agentName) {
		assigned = "This travel request was assigned to " + agentName + "."
	}
	c := travelContent(s, t, "Travel Agent Assigned", assigned+" Review it and arrange the trip.", travelGreeting(agentName), "Review Travel Request", "info", "you arrange travel")
	c["details_html"] = travelDetails(t, v, agentName)
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, t.TravellerEmail),
		Subject: travelSubject("Travel Agent Assigned", t), TemplateID: TravelTemplateAssign, Content: c,
		NotificationType: "travel-assigned", ReferenceID: t.JobNumber,
		Audience: Audience{To: []string{"the travel logistics team"}, CC: []string{travelCCRole}},
	}
}

// TravelVisaChanged is "Visa Updated" (legacy sendVisaChangeEmail).
func TravelVisaChanged(s TravelSettings, t TravelTrip, v TravelVisa, agentName string) Message {
	c := travelContent(s, t, "Visa Updated", "The visa for this trip was changed. Check the new visa and update the travel plans.",
		travelGreeting(agentName), "Review Visa Details", "info", "you arrange travel")
	c["details_html"] = travelDetails(t, v, agentName)
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, t.TravellerEmail),
		Subject: travelSubject("Visa Updated", t), TemplateID: TravelTemplateAssign, Content: c,
		NotificationType: "travel-visa-changed", ReferenceID: t.JobNumber,
		Audience: Audience{To: []string{"the travel logistics team"}, CC: []string{travelCCRole}},
	}
}

// travelGreeting is the greeting name of a travel email: the agency that must
// act when one is named, else the travel logistics team. The email still goes
// To the logistics team either way.
func travelGreeting(agency string) string {
	if a := strings.TrimSpace(agency); a != "" {
		return a
	}
	return "Travel Logistics Team"
}

// TravelNotice is the noContent email (legacy sendNoContentTravelEmail
// without the finance group): On Hold, Cancelled, In Progress, and the
// notice to a replaced agent. title is also the subject's action. agency is
// the agency the notice asks to act (the replaced agent); empty greets the
// travel logistics team.
func TravelNotice(s TravelSettings, t TravelTrip, title, description, agency string) Message {
	c := travelContent(s, t, title, description, travelGreeting(agency), "View Travel Request", "info", "you arrange travel")
	c["details_html"] = travelDetails(t, TravelVisa{}, "")
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, t.TravellerEmail),
		Subject: travelSubject(title, t), TemplateID: TravelTemplateNoContent, Content: c,
		NotificationType: "travel-notice", ReferenceID: t.JobNumber,
		Audience: Audience{To: []string{"the travel logistics team"}, CC: []string{travelCCRole}},
	}
}

// TravelFinanceRecipients ports getEmailRecipientsForEntity: the traveller's
// HR company picks its finance addresses, all but the last in To and the
// last in CC (one address: CC the finance group). No match, no addresses or
// no company: To the finance group.
func TravelFinanceRecipients(s TravelSettings, company string) (to, cc []string) {
	company = strings.TrimSpace(company)
	if company != "" {
		var matches [][]string
		for key, list := range s.CompanyRecipients {
			k := strings.TrimSpace(key)
			if strings.EqualFold(k, company) || strings.EqualFold(k, `"`+company+`"`) {
				matches = append(matches, list)
			}
		}
		if len(matches) == 1 {
			var list []string
			for _, a := range matches[0] {
				if a = strings.TrimSpace(a); a != "" {
					list = append(list, a)
				}
			}
			switch len(list) {
			case 0:
			case 1:
				return list, []string{s.FinanceGroup}
			default:
				return list[:len(list)-1], []string{list[len(list)-1]}
			}
		}
	}
	return []string{s.FinanceGroup}, []string{}
}

// TravelCompleted is the "Completed" email (legacy sendTravelCompletedEmail):
// To the traveller company's finance recipients (the finance group when
// unknown) to pay the per diems, with travel logistics and the traveller
// copied, and the Active per diems as a table.
func TravelCompleted(s TravelSettings, t TravelTrip, v TravelVisa, agentName, company string, perDiems []TravelPerDiem) Message {
	finTo, finCC := TravelFinanceRecipients(s, company)
	cc := append(append([]string{s.LogisticsGroup}, finCC...), travelCC(s, t.TravellerEmail)...)
	toRole := "the finance contacts for the traveller's company (the finance team if none)"
	if len(finTo) == 1 && strings.TrimSpace(finTo[0]) == "" && strings.TrimSpace(s.LogisticsGroup) != "" {
		// No finance address at all: travel logistics gets it, not nobody.
		finTo, cc, toRole = []string{s.LogisticsGroup}, cc[1:], "the travel logistics team (no finance address is set)"
	}

	details := travelDetails(t, v, agentName)
	if len(perDiems) > 0 {
		var b strings.Builder
		b.WriteString(sectionTitle("Per Diems"))
		fmt.Fprintf(&b, `<div class="ea-scroll" style="overflow-x: auto; -webkit-overflow-scrolling: touch;">`+"\n"+
			`<table role="presentation" class="ea-table" width="100%%" cellspacing="0" cellpadding="0" border="0" style="%s">`+"\n"+
			"\t<tr><th style=\"%s\">From</th><th style=\"%s\">To</th><th style=\"%s\">Days</th><th style=\"%s\">Rate</th><th style=\"%s\">Total</th></tr>\n",
			layTable, layTh, layTh, layTh, layTh, layTh)
		for _, p := range perDiems {
			fmt.Fprintf(&b, "\t<tr><td style=\"%s\">%s</td><td style=\"%s\">%s</td><td style=\"%s\">%s</td><td style=\"%s\">%s</td><td style=\"%s\">%s</td></tr>\n",
				layTd, html.EscapeString(financeDateTime(p.StartDate)), layTd, html.EscapeString(financeDateTime(p.EndDate)),
				layTd, html.EscapeString(p.NoOfDays), layTd, html.EscapeString(money(p.Currency, travelAmount(p.Rate))),
				layTd, html.EscapeString(money(p.Currency, travelAmount(p.Total))))
		}
		b.WriteString("</table>\n</div>\n")
		details += b.String()
	}
	c := travelContent(s, t, "Completed", "This trip is completed. Review its per diems and pay them.", "Finance Team",
		"Review Per Diems", "per-diems", "you handle travel payments")
	c["details_html"] = details
	return Message{
		App: AppTravel, To: finTo, CC: cc, Subject: travelSubject("Completed", t), TemplateID: TravelTemplateCompleted,
		Content: c, NotificationType: "travel-completed", ReferenceID: t.JobNumber,
		Audience: Audience{
			To: []string{toRole},
			CC: []string{"the travel logistics team", "the company's finance contacts or finance team", travelCCRole},
		},
	}
}

// travelAmount reads a per diem number printed by TravelFormatNumber.
func travelAmount(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}

// TravelFormatNumber prints a per diem number as Ballerina's toString did
// for a float (e.g. 70.0, 770.5).
func TravelFormatNumber(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// AuditContent is the email_auditlog contentKeyValPairs (both apps): the final
// content map as JSON, with the three keys the sender adds.
func AuditContent(m Message, year int) string {
	content := make(map[string]string, len(m.Content)+3)
	for k, v := range m.Content {
		content[k] = v
	}
	content["APP_NAME"] = travelAppName
	content["CLOSING_AND_SIGNATURE"] = travelSignature
	content["CURRENT_YEAR"] = strconv.Itoa(year)
	b, err := json.Marshal(content)
	if err != nil {
		return "{}"
	}
	return string(b)
}
