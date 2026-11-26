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
	"strings"
	"time"
)

// The shared look of every email. Colours are the Choreo Labs palette only:
// Signal (button), Burn (orange text on light), Midnight Ink (text), Slate
// (muted text), Paper (summary block) and White (card).
const (
	colSignal = "#FF4600"
	colBurn   = "#C43300"
	colInk    = "#0B1633"
	colSlate  = "#5A6072"
	colPaper  = "#F4F6FA"
	colWhite  = "#FFFFFF"

	layFont    = `font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif;`
	layText    = layFont + ` font-size: 15px; line-height: 24px; color: ` + colInk + `;`
	laySection = `margin: 28px 0 12px; ` + layFont + ` font-size: 13px; line-height: 16px; font-weight: 700; letter-spacing: 1.2px; text-transform: uppercase; color: ` + colBurn + `;`
	layDetails = `width: 100%; border-collapse: separate; border-radius: 6px; background-color: ` + colPaper + `;`
	layLabel   = `padding: 11px 16px; ` + layFont + ` font-size: 14px; line-height: 20px; color: ` + colSlate + `; vertical-align: top;`
	layValue   = `padding: 11px 16px; ` + layFont + ` font-size: 15px; line-height: 20px; font-weight: 500; color: ` + colInk + `; vertical-align: top; word-break: break-word;`
	layRule    = ` border-bottom: 2px solid ` + colWhite + `;`
	layTable   = `width: 100%; border-collapse: collapse; ` + layFont + ` font-size: 14px; line-height: 20px; color: ` + colInk + `;`
	layTh      = `padding: 9px 10px; background-color: ` + colPaper + `; font-size: 13px; font-weight: 600; color: ` + colSlate + `; text-align: left; border-bottom: 2px solid ` + colWhite + `;`
	layTd      = `padding: 9px 10px; text-align: left; vertical-align: top; border-bottom: 1px solid ` + colPaper + `;`
)

// row is one label and value of a details block. value is plain text (a
// string, always HTML-escaped) or a person (see personRow); only a person
// gets an avatar, so no text can ever add markup or an image.
type row struct {
	label string
	value any
}

// person is a person shown with an avatar, their name and their address.
type person struct{ name, addr string }

// personRow is a row naming a person: avatar on the left, the full name and
// the address below it on the right. Either may be unknown.
func personRow(label, name, addr string) row {
	name, addr = strings.TrimSpace(name), strings.TrimSpace(addr)
	if emptyValue(name) || strings.EqualFold(name, addr) {
		name = ""
	}
	return row{label, person{name, addr}}
}

// empty reports a row a block leaves out.
func (r row) empty() bool {
	switch v := r.value.(type) {
	case string:
		return emptyValue(v)
	case person:
		return v.name == "" && emptyValue(v.addr)
	}
	return true
}

// valueHTML is the content of a row's value cell, every text HTML-escaped.
func (r row) valueHTML() string {
	switch v := r.value.(type) {
	case string:
		return html.EscapeString(strings.TrimSpace(v))
	case person:
		return v.html()
	}
	return ""
}

// html is the person block: a small table (no flex) with the avatar on the
// left and the name over the address on the right. The avatar is a marker
// that inlinePhotos turns into the photo or the initials circle.
func (p person) html() string {
	avatar := initialsCircle(initialsOf(p.name, p.addr))
	if !emptyValue(p.addr) {
		avatar = personMarker(p.addr, initialsOf(p.name, p.addr))
	}
	main, sub := p.name, ""
	if main == "" {
		main = p.addr
	} else if !emptyValue(p.addr) {
		sub = `<br><span style="font-size: 13px; line-height: 18px; font-weight: 400; color: ` + colSlate + `;">` + html.EscapeString(p.addr) + `</span>`
	}
	return `<table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr>` +
		fmt.Sprintf(`<td width="%d" valign="middle" style="width: %dpx; padding: 0 10px 0 0;">%s</td>`, photoSize+10, photoSize+10, avatar) +
		`<td valign="middle" style="` + layValue + ` padding: 0;">` + html.EscapeString(main) + sub + `</td></tr></table>`
}

// emptyValue reports a value a block leaves out instead of showing "N/A".
func emptyValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.EqualFold(v, "N/A")
}

// sectionTitle is the small orange heading above a block.
func sectionTitle(title string) string {
	return fmt.Sprintf(`<p style="%s">%s</p>`+"\n", laySection, html.EscapeString(title))
}

// detailsSection renders a titled block of facts (every value HTML-escaped).
// Rows without a value are left out, and so is the block when none is left.
func detailsSection(title string, rows ...row) string {
	var keep []row
	for _, r := range rows {
		if !r.empty() {
			keep = append(keep, r)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(sectionTitle(title))
	fmt.Fprintf(&b, `<table role="presentation" class="ea-details" width="100%%" cellspacing="0" cellpadding="0" border="0" style="%s">`+"\n", layDetails)
	for i, r := range keep {
		rule := layRule
		if i == len(keep)-1 {
			rule = ""
		}
		fmt.Fprintf(&b, "\t<tr class=\"ea-row\">\n\t\t<td class=\"ea-label\" width=\"38%%\" style=\"%s%s\">%s</td>\n\t\t<td class=\"ea-value\" style=\"%s%s\">%s</td>\n\t</tr>\n",
			layLabel, rule, html.EscapeString(r.label), layValue, rule, r.valueHTML())
	}
	b.WriteString("</table>\n")
	return b.String()
}

// greetingHTML is "Hi Name," ("Hi," when the name is unknown).
func greetingHTML(name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		name = " " + html.EscapeString(name)
	}
	return fmt.Sprintf(`<p style="margin: 0 0 16px; %s">Hi%s,</p>`+"\n", layText, name)
}

// buttonHTML is the one primary button: Signal fill, Midnight Ink text, a
// tap target over 44px tall (full width on phones).
func buttonHTML(label, link string) string {
	return fmt.Sprintf(`<table role="presentation" cellspacing="0" cellpadding="0" border="0" class="ea-btn" style="margin: 24px 0 4px;"><tr>`+
		`<td align="center" bgcolor="%s" style="border-radius: 6px; background-color: %s;">`+
		`<a href="%s" target="_blank" style="display: inline-block; padding: 13px 28px; %s font-size: 15px; line-height: 20px; font-weight: 600; color: %s; text-decoration: none; border-radius: 6px;">%s</a>`+
		"</td></tr></table>\n", colSignal, colSignal, html.EscapeString(link), layFont, colInk, html.EscapeString(label))
}

// parseDay reads the date part of "YYYY-MM-DD" or "YYYY-MM-DD HH:MM:SS".
func parseDay(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) > 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// dateRange is "10–11 Dec 2026", "30 Nov – 2 Dec 2026", "30 Dec 2026 – 2 Jan 2027"
// or one date; text that is not a date is shown as given.
func dateRange(start, end string) string {
	s, okS := parseDay(start)
	e, okE := parseDay(end)
	switch {
	case !okS && !okE:
		return strings.TrimSpace(start + " " + end)
	case !okE || (okS && s.Equal(e)):
		return s.Format("2 Jan 2006")
	case !okS:
		return e.Format("2 Jan 2006")
	case s.Year() != e.Year():
		return s.Format("2 Jan 2006") + " – " + e.Format("2 Jan 2006")
	case s.Month() != e.Month():
		return s.Format("2 Jan") + " – " + e.Format("2 Jan 2006")
	}
	return fmt.Sprintf("%d–%s", s.Day(), e.Format("2 Jan 2006"))
}

// money is "USD 100.00".
func money(currency string, amount float64) string {
	return strings.TrimSpace(strings.TrimSpace(currency) + " " + fmt.Sprintf("%.2f", amount))
}

// joinNonEmpty joins the parts that have a value.
func joinNonEmpty(sep string, parts ...string) string {
	var keep []string
	for _, p := range parts {
		if !emptyValue(p) {
			keep = append(keep, strings.TrimSpace(p))
		}
	}
	return strings.Join(keep, sep)
}

// subjectFor is "[Engagement App] <Action>: <Record ID> <Record name>".
func subjectFor(action, id, name string) string {
	return "[Engagement App] " + action + ": " + joinNonEmpty(" ", id, name)
}

// engFacts are the engagement facts the engagement and allocation emails show.
type engFacts struct {
	ID, Customer, Project, Product, AllocationType, Code, Nature, Country, Address string
	Start, End, Contact, Domain, OpportunityOwner, TechnicalOwner, Scope, Comment  string
	BusinessUnit, ProductUnit, SubRegion, Requester                                string
	ContractsSigned                                                                bool
	// Kind: 1 internal engagement, 5 marketing event, else customer.
	Kind int
}

// rows lists the facts, key facts first; customer-only facts and the
// engagement code (customer engagements only) are left out elsewhere.
func (f engFacts) rows(withDates bool) []row {
	customer := f.Kind != 1 && f.Kind != 5
	name := map[bool]string{true: "Opportunity", false: "Name / Purpose"}[customer]
	if f.Kind == 5 {
		name = "Event Name"
	}
	rows := []row{{"ID", f.ID}, {name, f.Project}}
	if customer {
		rows = append(rows, row{"Client", f.Customer})
	}
	if withDates {
		rows = append(rows, row{"Dates", dateRange(f.Start, f.End)})
	}
	rows = append(rows,
		row{map[bool]string{true: "Event Type", false: "Allocation Type"}[f.Kind == 5], f.AllocationType},
		row{map[bool]string{true: "Event Nature", false: "On-site / Off-site"}[f.Kind == 5], f.Nature},
		row{"Location", joinNonEmpty(", ", f.Address, f.Country)})
	if customer {
		rows = append(rows, row{"Engagement Code", f.Code}, row{"Product", f.Product}, row{"Sales Sub Region", f.SubRegion})
	}
	if f.Kind != 5 {
		rows = append(rows, row{"Business Unit", f.BusinessUnit}, row{"Product Unit", f.ProductUnit})
	}
	if customer {
		rows = append(rows, row{"Customer Contact", f.Contact}, row{"Business Domain", f.Domain},
			row{"Opportunity Owner", f.OpportunityOwner}, row{"Technical Owner", f.TechnicalOwner},
			row{"Contracts Signed", map[bool]string{true: "Yes", false: "No"}[f.ContractsSigned]})
	}
	return append(rows, personRow("Requested By", "", f.Requester), row{"Scope", f.Scope}, row{"Comments", f.Comment})
}

// title of the block: "Marketing Event" or "Engagement".
func (f engFacts) blockTitle() string {
	if f.Kind == 5 {
		return "Marketing Event"
	}
	return "Engagement"
}
