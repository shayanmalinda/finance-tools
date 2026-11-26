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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package email

import (
	"errors"
	"strings"
	"testing"
)

// Every template id the backend sends renders a complete Choreo Labs email.
func TestRender_EveryTemplate(t *testing.T) {
	ids := []string{
		"customerEngagement", "internalEngagement", "marketingEvent",
		"customerEngagementCancelled", "internalEngagementCancelled", "marketingEventCancelled",
		"customerEngagementAllocation", "internalEngagementAllocation", "marketingEventAllocation",
		"consultantConfirmed", TravelTemplateAssign, TravelTemplateCompleted, TravelTemplateNoContent,
	}
	content := map[string]string{"CURRENT_YEAR": "2026", "CLOSING_AND_SIGNATURE": digitalTeamSignature, "description": "Body text"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			page, err := Render(id, content, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"<title>Choreo Labs</title>", "&#169; 2026 Choreo Labs. All Rights Reserved", "Choreo Labs Digital Team", "Body text",
				`src="cid:choreo-labs-logo-light"`, `src="cid:choreo-labs-logo-navy"`, `href="https://choreolabs.com/"`, `alt="Choreo Labs"`,
				// Responsive: a viewport, a 600px card that goes full width on small screens.
				`<meta name="viewport" content="width=device-width, initial-scale=1">`, "max-width: 600px;", "@media only screen and (max-width: 620px)"} {
				if !strings.Contains(page, want) {
					t.Errorf("lacks %q", want)
				}
			}
			for _, gone := range []string{bodyMarker, "WSO2, Inc. All Rights Reserved", "<title>WSO2", "WSO2-Logo", `alt="WSO2 Logo"`, `href="https://wso2.com/"`, "twitter-dark", "Roboto"} {
				if strings.Contains(page, gone) {
					t.Errorf("still has %q", gone)
				}
			}
		})
	}
}

// Keys are case-insensitive and inserted literally ("$" is not special);
// a placeholder with no value stays a comment; the banner shows only when set.
func TestRender_Placeholders(t *testing.T) {
	page, err := Render("noContent", map[string]string{"Description": "Pay $1 &amp; go", "Details_HTML": "<i>Row</i>", "Greeting_Name": "Priya"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "Pay $1 &amp; go") || !strings.Contains(page, "Hi Priya,") || !strings.Contains(page, "<i>Row</i>") ||
		!strings.Contains(page, "<!-- [APP_NAME] -->") || strings.Contains(page, "<!-- [GREETING") {
		t.Error("placeholder replacement is wrong")
	}
	if strings.Contains(page, "border-left: 4px solid #FF4600") {
		t.Error("banner shown without a banner")
	}
	if page, _ = Render("noContent", nil, "Staging notice"); !strings.Contains(page, "Staging notice") || !strings.Contains(page, "border-left: 4px solid #FF4600") {
		t.Error("banner missing")
	}
}

func TestRender_UnknownTemplate(t *testing.T) {
	for _, id := range []string{"", "nope", "base", "../base", "noContent.html"} {
		if _, err := Render(id, nil, ""); !errors.Is(err, ErrUnknownTemplate) {
			t.Errorf("%q: err = %v", id, err)
		}
	}
}

// renderMessage fills the globals Send adds and renders m like production.
func renderMessage(t *testing.T, m Message) string {
	t.Helper()
	content := map[string]string{"APP_NAME": appNames[m.App], "CLOSING_AND_SIGNATURE": digitalTeamSignature, "CURRENT_YEAR": "2026"}
	for k, v := range m.Content {
		content[k] = v
	}
	page, err := Render(m.TemplateID, content, "")
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// Every finance travel email uses its own template with the job, the
// company, what changed and a button labelled for the kind.
func TestFinanceTravelMessage_Template(t *testing.T) {
	r := FinanceRecipients{TravelLogisticsGroup: "travel@wso2.com", FinanceGroup: "finance@wso2.com", AppBaseURL: "https://one-stg.wso2.com/"}
	cases := []struct{ kind, tab, change, label string }{
		{FinancePerDiemsAdded, "per-diems", "A per diem was added", "Review Per Diems"},
		{FinanceInvoicesAdded, "invoices", "A travel invoice was added", "Review Invoices"},
		{FinanceInvoicesApproved, "invoices", "were approved", "View Invoices"},
		{FinanceInvoicesRejected, "invoices", "were rejected", "View Invoices"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			m := FinanceTravelMessage(r, c.kind, "TRV-2026-11-0003", "", c.tab, FinanceItems{})
			if m.TemplateID != FinanceTemplate {
				t.Fatalf("template = %q", m.TemplateID)
			}
			page := renderMessage(t, m)
			link := `href="https://one-stg.wso2.com/finance/engagement-app/travel-requests/TRV-2026-11-0003/` + c.tab + `"`
			for _, want := range []string{"TRV-2026-11-0003", c.change, ">" + c.label + "</a>", link} {
				if !strings.Contains(page, want) {
					t.Errorf("lacks %q", want)
				}
			}
			if strings.Contains(page, "<!-- [") {
				t.Errorf("leftover placeholder in %s", c.kind)
			}
		})
	}
	if m := FinanceTravelMessage(r, FinanceInvoicesAdded, "TRV-1", `"WSO2 - UK"`, "invoices", FinanceItems{}); !strings.Contains(m.Content["details_html"], ">WSO2 - UK<") {
		t.Errorf("company missing: %q", m.Content["details_html"])
	}
}

// An allocation email's button opens the screen for the next step: the
// consultant's own allocations, or the engagement's allocate screen.
func TestAllocationMessage_Buttons(t *testing.T) {
	g := allocGroups
	g.AppBaseURL = "https://one-stg.wso2.com/"
	for _, c := range []struct{ clearance, prev, label, href string }{
		{allocClearanceInProgress, "", "Respond to Allocation", "/me/allocations"},
		{allocRejectedByConsultant, "", "Reallocate", "/finance/engagement-app/engagements/E06405/allocate"},
		{allocAllocationCancelled, allocConfirmed, "View My Allocations", "/me/allocations"},
	} {
		m := AllocationMessage(AllocationEmailData{EngagementID: "E06405", EngagementTypeID: 3, ConsultantEmail: "jane@wso2.com",
			ClearanceStatus: c.clearance, PrevClearanceStatus: c.prev}, g)
		page := renderMessage(t, m)
		if !strings.Contains(page, `href="https://one-stg.wso2.com`+c.href+`"`) || !strings.Contains(page, ">"+c.label+"</a>") {
			t.Errorf("%s (%s): button is not %q to %s", c.clearance, m.TemplateID, c.label, c.href)
		}
	}
}

// The finance emails carry the records they are about, every value escaped:
// the per diem, the added invoice (paid amount only when set), and the
// decided invoices with the rejection reason.
// A rejection asks the invoice's agency to fix it: greet that agency (the
// trip's when the invoices name none, the team when several), To logistics.
func TestFinanceTravelMessage_RejectedGreeting(t *testing.T) {
	r := FinanceRecipients{TravelLogisticsGroup: "l@wso2.com", FinanceGroup: "f@wso2.com", AppBaseURL: "https://one.example"}
	greet := func(items FinanceItems) string {
		m := FinanceTravelMessage(r, FinanceInvoicesRejected, "TRV-1", "", "invoices", items)
		if m.To[0] != "l@wso2.com" {
			t.Errorf("To = %v", m.To)
		}
		return m.Content["greeting_name"]
	}
	if g := greet(FinanceItems{Invoices: []FinanceInvoiceItem{{Agency: "Classic Travels"}, {Agency: "Classic Travels"}}}); g != "Classic Travels" {
		t.Errorf("one agency = %q", g)
	}
	if g := greet(FinanceItems{AgencyName: "Acorn Travels", Invoices: []FinanceInvoiceItem{{}}}); g != "Acorn Travels" {
		t.Errorf("trip agency = %q", g)
	}
	if g := greet(FinanceItems{Invoices: []FinanceInvoiceItem{{Agency: "A"}, {Agency: "B"}}}); g != "Travel Logistics Team" {
		t.Errorf("several agencies = %q", g)
	}
}

func TestFinanceTravelMessage_Items(t *testing.T) {
	r := FinanceRecipients{TravelLogisticsGroup: "l@wso2.com", FinanceGroup: "f@wso2.com", AppBaseURL: "https://one.example"}
	render := func(kind string, items FinanceItems) string {
		t.Helper()
		m := FinanceTravelMessage(r, kind, "TRV-1", "", "invoices", items)
		page, err := Render(m.TemplateID, m.Content, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(page, "<script>") {
			t.Errorf("%s: unescaped value", kind)
		}
		if strings.Contains(page, "<!-- [ITEMS_HTML] -->") {
			t.Errorf("%s: placeholder left", kind)
		}
		return page
	}
	page := render(FinancePerDiemsAdded, FinanceItems{PerDiem: &FinancePerDiemItem{
		Currency: "GBP<script>", Rate: 70, Total: 735, From: "2026-11-06 13:00:00", To: "2026-11-16 18:00:00", Days: 11}})
	for _, want := range []string{">Per Diem<", ">Daily Rate<", "GBP&lt;script&gt; 70.00", "6 Nov 2026, 13:00", "16 Nov 2026, 18:00", ">11<", "GBP&lt;script&gt; 735.00"} {
		if !strings.Contains(page, want) {
			t.Errorf("per diem: lacks %q", want)
		}
	}

	inv := FinanceInvoiceItem{Number: "INV-<script>1", CostType: "Air Ticket", Month: "November", Currency: "USD", Amount: 1250.5}
	page = render(FinanceInvoicesAdded, FinanceItems{Invoices: []FinanceInvoiceItem{inv}})
	for _, want := range []string{">Invoice<", "INV-&lt;script&gt;1", ">Air Ticket<", ">November<", "USD 1250.50"} {
		if !strings.Contains(page, want) {
			t.Errorf("invoice added: lacks %q", want)
		}
	}
	if strings.Contains(page, "Paid Amount") {
		t.Error("invoice added: paid amount shown without one")
	}
	inv.PaidCurrency, inv.PaidAmount = "LKR", 375000
	if page = render(FinanceInvoicesAdded, FinanceItems{Invoices: []FinanceInvoiceItem{inv}}); !strings.Contains(page, "LKR 375000.00") {
		t.Error("invoice added: paid amount missing")
	}

	two := []FinanceInvoiceItem{{Number: "A-1", CostType: "Hotel", Currency: "USD", Amount: 980}, {Number: "A-2", Currency: "LKR", Amount: 45000}}
	page = render(FinanceInvoicesApproved, FinanceItems{Invoices: two})
	for _, want := range []string{">Invoices<", "class=\"ea-table\"", ">A-1<", ">Hotel<", "USD 980.00", ">A-2<", "LKR 45000.00", ">&#8212;<"} {
		if !strings.Contains(page, want) {
			t.Errorf("approved: lacks %q", want)
		}
	}
	page = render(FinanceInvoicesRejected, FinanceItems{Reason: "Wrong <script> amount", Invoices: two[:1]})
	for _, want := range []string{">Rejection<", "Wrong &lt;script&gt; amount", ">A-1<"} {
		if !strings.Contains(page, want) {
			t.Errorf("rejected: lacks %q", want)
		}
	}
	if page = render(FinanceInvoicesApproved, FinanceItems{}); strings.Contains(page, ">Invoices<") || strings.Contains(page, ">Invoice<") {
		t.Error("no items: a section was rendered")
	}
}

func TestAsciiHTML_EncodesNonASCII(t *testing.T) {
	if got := asciiHTML("10–11 Dec, José"); got != "10&#8211;11 Dec, Jos&#233;" {
		t.Fatalf("asciiHTML = %q", got)
	}
}

func TestLogoAttachments_InlinePNGs(t *testing.T) {
	got := logoAttachments()
	if len(got) != 2 || got[0].ContentID != logoLightCID || got[1].ContentID != logoNavyCID {
		t.Fatalf("logo attachments = %+v", got)
	}
	for _, a := range got {
		if a.ContentType != "image/png" || len(a.Attachment) < 8 || a.Attachment[1] != 'P' || a.Attachment[2] != 'N' || a.Attachment[3] != 'G' {
			t.Fatalf("%s is not an inline PNG", a.ContentID)
		}
	}
}
