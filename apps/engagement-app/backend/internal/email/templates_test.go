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
			for _, want := range []string{"<title>Choreo Labs</title>", "© 2026 Choreo Labs. All Rights Reserved", "Choreo Labs Digital Team", "Body text",
				"choreo-labs-logo-light-400px.png", "choreo-labs-logo-navy-400px.png", `href="https://choreolabs.com/"`, `alt="Choreo Labs Logo"`,
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
	page, err := Render("noContent", map[string]string{"Description": "Pay $1 &amp; go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, "Pay $1 &amp; go") || !strings.Contains(page, "<!-- [TRAVEL_AGENT_NAME] -->") {
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
		{FinancePerDiemsAdded, "per-diems", "Per diems added", "Review Per Diems"},
		{FinanceInvoicesAdded, "invoices", "Travel invoices added", "Review Invoices"},
		{FinanceInvoicesApproved, "invoices", "Travel invoices approved", "View Invoices"},
		{FinanceInvoicesRejected, "invoices", "Travel invoices rejected", "View Invoices"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			m := FinanceTravelMessage(r, c.kind, "TRV-2026-11-0003", "", c.tab, FinanceItems{})
			if m.TemplateID != FinanceTemplate {
				t.Fatalf("template = %q", m.TemplateID)
			}
			page := renderMessage(t, m)
			link := `href="https://one-stg.wso2.com/finance/engagement-app/travel-requests/TRV-2026-11-0003/` + c.tab + `"`
			for _, want := range []string{"TRV-2026-11-0003", "Not recorded", c.change, ">" + c.label + "</a>", link} {
				if !strings.Contains(page, want) {
					t.Errorf("lacks %q", want)
				}
			}
			if strings.Contains(page, "<!-- [") {
				t.Errorf("leftover placeholder in %s", c.kind)
			}
		})
	}
	if m := FinanceTravelMessage(r, FinanceInvoicesAdded, "TRV-1", `"WSO2 - UK"`, "invoices", FinanceItems{}); m.Content["traveller_company"] != "WSO2 - UK" {
		t.Errorf("company = %q", m.Content["traveller_company"])
	}
}

// Every allocation email links the person to their own allocations (Me →
// Allocations) in One WSO2.
func TestAllocationMessage_MyAllocationsButton(t *testing.T) {
	g := allocGroups
	g.AppBaseURL = "https://one-stg.wso2.com/"
	for _, c := range []struct{ clearance, prev string }{
		{allocClearanceInProgress, ""}, {allocConfirmed, allocAcceptedByConsultant}, {allocAllocationCancelled, allocConfirmed},
	} {
		m := AllocationMessage(AllocationEmailData{EngagementID: "E06405", EngagementTypeID: 3, ConsultantEmail: "jane@wso2.com",
			ClearanceStatus: c.clearance, PrevClearanceStatus: c.prev}, g)
		page := renderMessage(t, m)
		if !strings.Contains(page, `href="https://one-stg.wso2.com/me/allocations"`) || !strings.Contains(page, ">View My Allocations</a>") {
			t.Errorf("%s (%s): no View My Allocations button", c.clearance, m.TemplateID)
		}
	}
}

// The finance emails carry the records they are about, every value escaped:
// the per diem, the added invoice (paid amount only when set), and the
// decided invoices with the rejection reason.
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
	for _, want := range []string{">Per Diem<", ">Daily Rate<", ">70.0<", "6 Nov 2026, 13:00", "16 Nov 2026, 18:00", ">11<", "GBP&lt;script&gt; 735.0"} {
		if !strings.Contains(page, want) {
			t.Errorf("per diem: lacks %q", want)
		}
	}

	inv := FinanceInvoiceItem{Number: "INV-<script>1", CostType: "Air Ticket", Month: "November", Currency: "USD", Amount: 1250.5}
	page = render(FinanceInvoicesAdded, FinanceItems{Invoices: []FinanceInvoiceItem{inv}})
	for _, want := range []string{">Invoice<", "INV-&lt;script&gt;1", ">Air Ticket<", ">November<", "USD 1250.5"} {
		if !strings.Contains(page, want) {
			t.Errorf("invoice added: lacks %q", want)
		}
	}
	if strings.Contains(page, "Paid Amount") {
		t.Error("invoice added: paid amount shown without one")
	}
	inv.PaidCurrency, inv.PaidAmount = "LKR", 375000
	if page = render(FinanceInvoicesAdded, FinanceItems{Invoices: []FinanceInvoiceItem{inv}}); !strings.Contains(page, "LKR 375000.0") {
		t.Error("invoice added: paid amount missing")
	}

	two := []FinanceInvoiceItem{{Number: "A-1", CostType: "Hotel", Currency: "USD", Amount: 980}, {Number: "A-2", Currency: "LKR", Amount: 45000}}
	page = render(FinanceInvoicesApproved, FinanceItems{Invoices: two})
	for _, want := range []string{">Invoices<", "class=\"ea-table\"", ">A-1<", ">Hotel<", "USD 980.0", ">A-2<", "LKR 45000.0", ">—<"} {
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
