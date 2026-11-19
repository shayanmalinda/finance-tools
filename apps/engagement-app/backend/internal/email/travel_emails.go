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
	travelAppName   = "Travel App"
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
	Customer       string
	Country        string
	City           string
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

func travelSubject(jobNumber string) string {
	return "[Travel App] Travel Request (#" + jobNumber + ")"
}

// TravelActionLink is the travel request page in One WSO2.
func TravelActionLink(baseURL, jobNumber string) string {
	return strings.TrimRight(baseURL, "/") + "/finance/engagement-app/travel-requests/" +
		url.PathEscape(jobNumber) + "/info"
}

func travelCC(s TravelSettings, travellerEmail string) []string {
	if s.CCTraveller && travellerEmail != "" {
		return []string{travellerEmail}
	}
	return []string{}
}

func travelNA(v string) string {
	if strings.TrimSpace(v) == "" {
		return "N/A"
	}
	return html.EscapeString(v)
}

func travelDate10(v string) string {
	if len(v) >= 10 {
		return v[:10]
	}
	return v
}

func travelDate16(v string) string {
	if len(v) >= 16 {
		return v[:16]
	}
	return v
}

// travelTripContent is the content of the assignRequests and completeTravel
// templates (legacy sendAssignRequestEmail).
func travelTripContent(s TravelSettings, title, description, agentName string, t TravelTrip, v TravelVisa) map[string]string {
	visaStart, visaEnd := "N/A", "N/A"
	if strings.TrimSpace(v.StartDate) != "" {
		visaStart = html.EscapeString(travelDate10(v.StartDate))
	}
	if strings.TrimSpace(v.EndDate) != "" {
		visaEnd = html.EscapeString(travelDate10(v.EndDate))
	}
	return map[string]string{
		"title":             title,
		"description":       description,
		"travel_agent_name": html.EscapeString(agentName),
		"job_number":        html.EscapeString(t.JobNumber),
		"traveller_email":   html.EscapeString(t.TravellerEmail),
		"customer":          html.EscapeString(t.Customer),
		"country":           html.EscapeString(t.Country),
		"travel_location":   html.EscapeString(t.City),
		"start_date":        html.EscapeString(travelDate10(t.StartDate)),
		"end_date":          html.EscapeString(travelDate10(t.EndDate)),
		"visa_type":         travelNA(v.VisaType),
		"visa_country":      travelNA(v.Country),
		"visa_entry_type":   travelNA(v.EntryType),
		"visa_start_date":   visaStart,
		"visa_end_date":     visaEnd,
		"action_link":       TravelActionLink(s.AppBaseURL, t.JobNumber),
	}
}

// TravelAssigned is "New Travel Request Assigned" (legacy
// sendAssignRequestEmail): on assigning In Progress with an agent and a
// visa, and to the new agent when the agent changes.
func TravelAssigned(s TravelSettings, t TravelTrip, v TravelVisa, agentName string) Message {
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, t.TravellerEmail),
		Subject: travelSubject(t.JobNumber), TemplateID: TravelTemplateAssign,
		Content: travelTripContent(s, "New Travel Request Assigned",
			"Please take necessary actions for the following Travel Request", agentName, t, v),
		NotificationType: "travel-assigned", ReferenceID: t.JobNumber,
	}
}

// TravelVisaChanged is "Visa has been changed" (legacy sendVisaChangeEmail).
func TravelVisaChanged(s TravelSettings, t TravelTrip, v TravelVisa, agentName string) Message {
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, t.TravellerEmail),
		Subject: travelSubject(t.JobNumber), TemplateID: TravelTemplateAssign,
		Content: travelTripContent(s, "Visa has been changed",
			"Visa has been changed. Please take necessary actions for the following Travel Request", agentName, t, v),
		NotificationType: "travel-visa-changed", ReferenceID: t.JobNumber,
	}
}

// TravelNotice is the noContent email (legacy sendNoContentTravelEmail
// without the finance group): On Hold, Cancelled, In Progress, and the
// "Cancelled for you" notice to a replaced agent.
func TravelNotice(s TravelSettings, jobNumber, travellerEmail, addressing, title, description string) Message {
	return Message{
		App: AppTravel, To: []string{s.LogisticsGroup}, CC: travelCC(s, travellerEmail),
		Subject: travelSubject(jobNumber), TemplateID: TravelTemplateNoContent,
		Content: map[string]string{
			"title":             title,
			"description":       description,
			"travel_agent_name": html.EscapeString(addressing),
			"action_link":       TravelActionLink(s.AppBaseURL, jobNumber),
		},
		NotificationType: "travel-notice", ReferenceID: jobNumber,
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
// logistics plus the company's finance recipients, with the Active per
// diems as a table or "No Perdiems".
func TravelCompleted(s TravelSettings, t TravelTrip, v TravelVisa, agentName, company string, perDiems []TravelPerDiem) Message {
	finTo, finCC := TravelFinanceRecipients(s, company)
	to := append([]string{s.LogisticsGroup}, finTo...)
	cc := append(append([]string{}, finCC...), travelCC(s, t.TravellerEmail)...)

	table := "No Perdiems"
	if len(perDiems) > 0 {
		var b strings.Builder
		b.WriteString(`<tr>
								<th style="padding: 5px 5px;">Start Date/Time</th>
								<th style="padding: 5px 5px;">End Date/Time</th>
								<th style="padding: 5px 5px;">No. of Days	</th>
								<th style="padding: 5px 5px;">Currency</th>
								<th style="padding: 5px 5px;">Per Diem Amount</th>
								<th style="padding: 5px 5px;">Total Amount</th>
							</tr>
                            `)
		for _, p := range perDiems {
			fmt.Fprintf(&b, `<tr>
                                        <td style="padding: 5px 5px;">%s</td>
                                        <td style="padding: 5px 5px;">%s</td>
                                        <td style="padding: 5px 5px;">%s</td>
                                        <td style="padding: 5px 5px;">%s</td>
                                        <td style="padding: 5px 5px;">%s</td>
                                        <td style="padding: 5px 5px;">%s</td>
                                      </tr>
                                    `,
				html.EscapeString(travelDate16(p.StartDate)), html.EscapeString(travelDate16(p.EndDate)),
				html.EscapeString(p.NoOfDays), html.EscapeString(p.Currency), html.EscapeString(p.Rate),
				html.EscapeString(p.Total))
		}
		table = b.String()
	}
	content := travelTripContent(s, "Completed", "Please note that this travel request has been completed", agentName, t, v)
	content["table_data"] = table
	return Message{
		App: AppTravel, To: to, CC: cc, Subject: travelSubject(t.JobNumber), TemplateID: TravelTemplateCompleted,
		Content: content, NotificationType: "travel-completed", ReferenceID: t.JobNumber,
	}
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

// TravelAuditContent is the email_auditlog contentKeyValPairs: the final
// content map as JSON, with the three keys the sender adds.
func TravelAuditContent(m Message, year int) string {
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
