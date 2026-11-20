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

// financeDetails are the "What changed" line and the button label per kind.
var financeDetails = map[string][2]string{
	FinancePerDiemsAdded:    {"Per diems added", "Review Per Diems"},
	FinanceInvoicesAdded:    {"Travel invoices added", "Review Invoices"},
	FinanceInvoicesApproved: {"Travel invoices approved", "View Invoices"},
	FinanceInvoicesRejected: {"Travel invoices rejected", "View Invoices"},
}

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
// subject "[Engagement App] Travel Request (#job)", template travelFinanceUpdate,
// To the travel logistics group plus the traveller company's finance
// recipients (the finance group when the company is unknown). tab is the trip
// page tab the action link opens (per-diems, invoices); items are the records
// shown below "What Changed".
func FinanceTravelMessage(r FinanceRecipients, kind, jobNumber, travellerCompany, tab string, items FinanceItems) Message {
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
		Subject:    "[Engagement App] Travel Request (#" + jobNumber + ")",
		TemplateID: FinanceTemplate,
		Content: map[string]string{
			"title":             texts[0],
			"description":       texts[1],
			"travel_agent_name": "All",
			"job_number":        html.EscapeString(jobNumber),
			"traveller_company": html.EscapeString(financeCompanyOrNA(travellerCompany)),
			"change":            financeDetails[kind][0],
			"action_label":      financeDetails[kind][1],
			"action_link":       html.EscapeString(link),
			"items_html":        financeItemsHTML(items),
		},
		NotificationType: kind,
		ReferenceID:      jobNumber,
	}
}

// FinanceItems are the records a travel finance email is about: the per
// diem that was added, the invoice that was added, or the invoices that were
// approved or rejected (with the rejection reason). Empty renders nothing.
type FinanceItems struct {
	PerDiem  *FinancePerDiemItem
	Invoices []FinanceInvoiceItem
	// Reason is the rejection reason (invoices rejected).
	Reason string
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
	Number       string
	CostType     string
	Month        string
	Currency     string
	Amount       float64
	PaidCurrency string
	PaidAmount   float64
}

// Inline styles of the details, matching the template's detail list and
// the completed email's table (brand: Midnight Ink text, Slate labels,
// hairline #E3E7EF, Paper header row).
const (
	finFont       = `font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif;`
	finSection    = `margin: 28px 0 12px; ` + finFont + ` font-size: 12px; line-height: 16px; font-weight: 700; letter-spacing: 1.2px; text-transform: uppercase; color: #C43300;`
	finDetails    = `width: 100%; border-collapse: separate; border: 1px solid #E3E7EF; border-radius: 6px; background-color: #F4F6FA;`
	finLabelStyle = `padding: 11px 16px; ` + finFont + ` font-size: 13px; line-height: 20px; color: #5A6072; vertical-align: top;`
	finValueStyle = `padding: 11px 16px; ` + finFont + ` font-size: 14px; line-height: 20px; font-weight: 500; color: #0B1633; vertical-align: top; word-break: break-word;`
	finRule       = ` border-bottom: 1px solid #E3E7EF;`
	finTable      = `width: 100%; border-collapse: collapse; border: 1px solid #E3E7EF; ` + finFont + ` font-size: 13px; line-height: 18px; color: #0B1633;`
	finTh         = `padding: 9px 10px; background-color: #F4F6FA; font-size: 12px; font-weight: 600; color: #5A6072; text-align: left; border-bottom: 1px solid #E3E7EF;`
	finTd         = `padding: 9px 10px; text-align: left; vertical-align: top; border-bottom: 1px solid #E3E7EF;`
)

// financeItemsHTML renders the items for the ITEMS_HTML placeholder; every
// value is HTML-escaped.
func financeItemsHTML(items FinanceItems) string {
	var b strings.Builder
	type row struct{ label, value string }
	detail := func(title string, rows []row) {
		fmt.Fprintf(&b, `<p style="%s">%s</p>`+"\n", finSection, html.EscapeString(title))
		fmt.Fprintf(&b, `<table role="presentation" class="ea-details" width="100%%" cellspacing="0" cellpadding="0" border="0" style="%s">`+"\n", finDetails)
		for i, r := range rows {
			rule := finRule
			if i == len(rows)-1 {
				rule = ""
			}
			fmt.Fprintf(&b, "\t<tr class=\"ea-row\">\n\t\t<td class=\"ea-label\" width=\"38%%\" style=\"%s%s\">%s</td>\n\t\t<td class=\"ea-value\" style=\"%s%s\">%s</td>\n\t</tr>\n",
				finLabelStyle, rule, html.EscapeString(r.label), finValueStyle, rule, html.EscapeString(r.value))
		}
		b.WriteString("</table>\n")
	}
	if pd := items.PerDiem; pd != nil {
		detail("Per Diem", []row{
			{"Currency", pd.Currency},
			{"Daily Rate", TravelFormatNumber(pd.Rate)},
			{"From", financeDateTime(pd.From)},
			{"To", financeDateTime(pd.To)},
			{"Days", fmt.Sprint(pd.Days)},
			{"Total", strings.TrimSpace(pd.Currency + " " + TravelFormatNumber(pd.Total))},
		})
	}
	if len(items.Invoices) == 1 && items.Reason == "" {
		inv := items.Invoices[0]
		rows := []row{
			{"Invoice Number", inv.Number},
			{"Cost Type", financeOrDash(inv.CostType)},
			{"Month", financeOrDash(inv.Month)},
			{"Invoice Amount", financeMoney(inv.Currency, inv.Amount)},
		}
		if inv.PaidAmount > 0 {
			rows = append(rows, row{"Paid Amount", financeMoney(inv.PaidCurrency, inv.PaidAmount)})
		}
		detail("Invoice", rows)
		return b.String()
	}
	if items.Reason != "" {
		detail("Rejection", []row{{"Reason", items.Reason}})
	}
	if len(items.Invoices) > 0 {
		fmt.Fprintf(&b, `<p style="%s">Invoices</p>`+"\n", finSection)
		fmt.Fprintf(&b, `<div class="ea-scroll" style="overflow-x: auto; -webkit-overflow-scrolling: touch;">`+"\n"+
			`<table role="presentation" class="ea-table" width="100%%" cellspacing="0" cellpadding="0" border="0" style="%s">`+"\n"+
			"\t<tr><th style=\"%s\">Invoice Number</th><th style=\"%s\">Cost Type</th><th style=\"%s\">Amount</th></tr>\n",
			finTable, finTh, finTh, finTh)
		for _, inv := range items.Invoices {
			fmt.Fprintf(&b, "\t<tr><td style=\"%s\">%s</td><td style=\"%s\">%s</td><td style=\"%s\">%s</td></tr>\n",
				finTd, html.EscapeString(inv.Number), finTd, html.EscapeString(financeOrDash(inv.CostType)),
				finTd, html.EscapeString(financeMoney(inv.Currency, inv.Amount)))
		}
		b.WriteString("</table>\n</div>\n")
	}
	return b.String()
}

// financeMoney is "USD 1250.5" (the completed email's number format).
func financeMoney(currency string, amount float64) string {
	return strings.TrimSpace(strings.TrimSpace(currency) + " " + TravelFormatNumber(amount))
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

// financeCompanyOrNA is the traveller's company, or "Not recorded" when the
// HR entity has none.
func financeCompanyOrNA(company string) string {
	if c := strings.TrimSpace(strings.Trim(strings.TrimSpace(company), `"`)); c != "" {
		return c
	}
	return "Not recorded"
}
