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
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
)

// The travel finance notifications (legacy Travel app
// sendNoContentTravelEmail with isSendToFinanceGroup = true).
const (
	FinancePerDiemsAdded    = "perdiems-added"
	FinanceInvoicesAdded    = "invoices-added"
	FinanceInvoicesApproved = "invoices-approved"
	FinanceInvoicesRejected = "invoices-rejected"
)

// FinanceTemplate is the template of the finance travel emails.
const FinanceTemplate = "travelFinanceUpdate"

// financeTexts are the title (also the subject's action), what happened and
// what to do, and the button label per kind.
var financeTexts = map[string][3]string{
	FinancePerDiemsAdded:    {"Per Diem Added", "A per diem was added to this trip. Review it.", "Review Per Diems"},
	FinanceInvoicesAdded:    {"Invoice Added", "A travel invoice was added to this trip. Review and approve it.", "Review Invoices"},
	FinanceInvoicesApproved: {"Invoice Approved", "Travel invoices for this trip were approved. Check them before they go to NetSuite.", "View Invoices"},
	FinanceInvoicesRejected: {"Invoice Rejected", "Travel invoices for this trip were rejected. Fix them and add them again.", "View Invoices"},
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

// FinanceTravelMessage builds a travel finance email for jobNumber: template
// travelFinanceUpdate, To the traveller company's finance recipients (the
// finance group when the company is unknown) who review it, with travel
// logistics and the rest of finance copied. A rejection goes To travel
// logistics to fix it, with finance copied. tab is the trip page tab the
// button opens (per-diems, invoices); items are the records shown below.
func FinanceTravelMessage(r FinanceRecipients, kind, jobNumber, travellerCompany, tab string, items FinanceItems) Message {
	texts := financeTexts[kind]
	finTo, finCC := []string{r.FinanceGroup}, []string(nil)
	if travellerCompany != "" {
		finTo, finCC = financeEntityRecipients(r, travellerCompany)
	}
	to, cc, name := finTo, append([]string{r.TravelLogisticsGroup}, finCC...), "Finance Team"
	toRole, reason := "the finance contacts for the traveller's company (the finance team if none)", "you handle travel finance"
	ccRoles := []string{"the travel logistics team"}
	if len(finCC) > 0 {
		ccRoles = append(ccRoles, "the company's finance contacts or finance team")
	}
	if kind == FinanceInvoicesRejected {
		// The agency fixes and re-adds its invoice: greet it, still To the logistics team.
		to, cc, name = []string{r.TravelLogisticsGroup}, append(append([]string{}, finTo...), finCC...), travelGreeting(rejectedAgency(items))
		toRole, reason = "the travel logistics team", "you arrange travel"
		ccRoles = []string{"the finance contacts for the traveller's company (the finance team if none)"}
	}
	link := strings.TrimRight(r.AppBaseURL, "/") + "/finance/engagement-app/travel-requests/" + url.PathEscape(jobNumber) + "/" + tab
	details := detailsSection("Trip", row{"Job Number", jobNumber}, personRow("Traveller", items.TravellerName, items.TravellerEmail),
		row{"Destination", items.Country}, row{"Traveller Company", strings.TrimSpace(strings.Trim(strings.TrimSpace(travellerCompany), `"`))}) + financeItemsHTML(items)
	return Message{
		App:        AppTravel,
		To:         to,
		CC:         cc,
		Subject:    subjectFor(texts[0], jobNumber, tripName(items.TravellerName, items.Country)),
		TemplateID: FinanceTemplate,
		Content: map[string]string{
			"title":         texts[0],
			"description":   texts[1],
			"greeting_name": name,
			"reason":        "You are receiving this because " + reason + ", or because you are copied on it.",
			"action_label":  texts[2],
			"action_link":   link,
			"details_html":  details,
		},
		NotificationType: kind,
		ReferenceID:      jobNumber,
		Audience:         Audience{To: []string{toRole}, CC: ccRoles},
	}
}

// rejectedAgency is the agency of the rejected invoices when they share one,
// the trip's agency when they name none, else "" (several agencies).
func rejectedAgency(items FinanceItems) string {
	agency := ""
	for _, inv := range items.Invoices {
		a := strings.TrimSpace(inv.Agency)
		switch {
		case a == "":
		case agency == "":
			agency = a
		case agency != a:
			return ""
		}
	}
	if agency == "" {
		return strings.TrimSpace(items.AgencyName)
	}
	return agency
}

// FinanceItems are the records a travel finance email is about: the per
// diem that was added, the invoice that was added, or the invoices that were
// approved or rejected (with the rejection reason). Empty renders nothing.
type FinanceItems struct {
	PerDiem  *FinancePerDiemItem
	Invoices []FinanceInvoiceItem
	// Reason is the rejection reason (invoices rejected).
	Reason string
	// TravellerName and Country name the trip in the subject ("Jane Doe to Sri Lanka").
	TravellerName, Country string
	// TravellerEmail is shown under the traveller's name in the details.
	TravellerEmail string
	// AgencyName is the trip's travel agency, the greeting of a rejection
	// when the rejected invoices name no agency of their own.
	AgencyName string
}

// FinancePerDiemItem is a per diem. From and To are "YYYY-MM-DD HH:MM:SS".
type FinancePerDiemItem struct {
	Currency string
	Rate     float64
	Total    float64
	From, To string
	Days     int
}

// FinanceInvoiceItem is a travel invoice. Paid is shown only when PaidAmount > 0.
type FinanceInvoiceItem struct {
	Number   string
	CostType string
	Month    string
	Currency string
	Amount   float64
	// Agency is the invoice's travel agency ("" when none).
	Agency       string
	PaidCurrency string
	PaidAmount   float64
}

// financeItemsHTML renders the items below the trip; every value is
// HTML-escaped.
func financeItemsHTML(items FinanceItems) string {
	var b strings.Builder
	if pd := items.PerDiem; pd != nil {
		b.WriteString(detailsSection("Per Diem",
			row{"Dates", financeDateTime(pd.From) + " to " + financeDateTime(pd.To)},
			row{"Days", fmt.Sprint(pd.Days)},
			row{"Daily Rate", money(pd.Currency, pd.Rate)},
			row{"Total", money(pd.Currency, pd.Total)}))
	}
	if len(items.Invoices) == 1 && items.Reason == "" {
		inv := items.Invoices[0]
		rows := []row{
			{"Invoice Number", inv.Number},
			{"Cost Type", inv.CostType},
			{"Month", inv.Month},
			{"Invoice Amount", money(inv.Currency, inv.Amount)},
		}
		if inv.PaidAmount > 0 {
			rows = append(rows, row{"Paid Amount", money(inv.PaidCurrency, inv.PaidAmount)})
		}
		b.WriteString(detailsSection("Invoice", rows...))
		return b.String()
	}
	if items.Reason != "" {
		b.WriteString(detailsSection("Rejection", row{"Reason", items.Reason}))
	}
	if len(items.Invoices) > 0 {
		b.WriteString(sectionTitle("Invoices"))
		fmt.Fprintf(&b, `<div class="ea-scroll" style="overflow-x: auto; -webkit-overflow-scrolling: touch;">`+"\n"+
			`<table role="presentation" class="ea-table" width="100%%" cellspacing="0" cellpadding="0" border="0" style="%s">`+"\n"+
			"\t<tr><th style=\"%s\">Invoice Number</th><th style=\"%s\">Cost Type</th><th style=\"%s\">Amount</th></tr>\n",
			layTable, layTh, layTh, layTh)
		for _, inv := range items.Invoices {
			fmt.Fprintf(&b, "\t<tr><td style=\"%s\">%s</td><td style=\"%s\">%s</td><td style=\"%s\">%s</td></tr>\n",
				layTd, html.EscapeString(inv.Number), layTd, html.EscapeString(financeOrDash(inv.CostType)),
				layTd, html.EscapeString(money(inv.Currency, inv.Amount)))
		}
		b.WriteString("</table>\n</div>\n")
	}
	return b.String()
}

// financeDateTime renders "YYYY-MM-DD HH:MM:SS" as "6 Nov 2026, 13:00";
// anything else is shown as given.
func financeDateTime(v string) string {
	t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(v))
	if err != nil {
		return financeOrDash(v)
	}
	return t.Format("2 Jan 2006, 15:04")
}

func financeOrDash(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return "—"
}
