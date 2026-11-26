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
	"slices"
	"strings"
	"time"
)

// Allocation email: a port of the legacy Allocations backend's
// sendEmailForAllocation and isAllocationMailInitiated
// (apps/allocation/backend/modules/email/email.bal, alloc-port-spec §5.4).

// Clearance values the allocation emails react to.
const (
	allocClearanceInProgress   = "Clearance In Progress"
	allocReadyForClearance     = "Ready for Clearance"
	allocRejectedByConsultant  = "Rejected by Consultant"
	allocRejectedOther         = "Rejected - Other"
	allocRejectedVisaIssues    = "Rejected - Visa Issues"
	allocAcceptedByConsultant  = "Accepted by Consultant"
	allocConfirmed             = "Confirmed"
	allocConfirmedVisaPending  = "Confirmed - Visa Pending"
	allocAllocationCancelled   = "Allocation Cancelled"
	allocPartnerEmailPrefix    = "partner-allocations-group"
	allocNotificationPrefix    = "Allocation"
	allocVisaParagraphStyle    = `font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif;font-size: 14px;line-height: 22px;color: #0B1633;text-align: left;margin: 0;font-weight: 400;`
	allocVisaWarningStyle      = `font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif;font-size: 13px;line-height: 20px;color: #C43300;text-align: left;margin: 0 0 8px;font-weight: 500;display: block;`
	allocVisaAvailableComment  = "Visa available for the allocation period"
	allocVisaUnavailableComent = "Visa not available"
)

// AllocationMailInitiated ports isAllocationMailInitiated: whether a change
// to clearance (from prev, "" for a new allocation) sends an email.
func AllocationMailInitiated(clearance, prev string) bool {
	return clearance == allocClearanceInProgress || clearance == allocRejectedByConsultant ||
		clearance == allocRejectedOther || (prev != allocReadyForClearance && clearance == allocAllocationCancelled) ||
		clearance == allocAcceptedByConsultant || clearance == allocRejectedVisaIssues ||
		(prev != allocConfirmedVisaPending && clearance == allocConfirmed) || clearance == allocConfirmedVisaPending
}

// AllocationIsConfirmation reports whether the email for this change is the
// "consultant confirmed" one (which lists the consultant's visas).
func AllocationIsConfirmation(clearance, prev string) bool {
	return (prev != allocConfirmedVisaPending && clearance == allocConfirmed) || clearance == allocConfirmedVisaPending
}

// AllocationIsPartnerEmail reports whether email is a partner pseudo-email.
func AllocationIsPartnerEmail(email string) bool {
	return strings.Contains(email, allocPartnerEmailPrefix)
}

// AllocationEmailFunding is an engagement funding source.
type AllocationEmailFunding struct {
	BusinessUnit string
	ProductUnit  string
	SubRegion    string
}

// AllocationEmailVisa is a visa of the consultant (travel readiness).
type AllocationEmailVisa struct {
	Country   string // the visa's own country; empty falls back to the engagement's
	VisaType  string
	EntryType string
	Status    string
	StartDate string // YYYY-MM-DD
	EndDate   string // YYYY-MM-DD
}

// AllocationEmailData is everything the allocation email shows.
type AllocationEmailData struct {
	EngagementTypeID    int
	EngagementID        string
	CustomerName        string
	OpportunityName     string
	ProductName         string
	AllocationTypeName  string
	EngagementCode      string
	EngagementNature    string
	Country             string
	Address             string
	CustomerContact     string
	BusinessDomain      string
	OpportunityOwner    string
	TechnicalOwner      string
	ContractsSigned     bool
	EngagementScope     string
	EngagementComment   string
	FundingSources      []AllocationEmailFunding
	ConsultantEmail     string
	ClearanceStatus     string
	PrevClearanceStatus string
	TravelRequired      bool
	StartDate           string
	EndDate             string
	AllocationComment   string
	// ConsultantName is the consultant's first name for the greeting ("" when unknown).
	ConsultantName string
	// Actor is the user who made the change; always a recipient.
	Actor string
	// Leads are the consultant's reporting leads (not for partners).
	Leads []string
	// Visas are the consultant's visas for the Confirmed email.
	Visas []AllocationEmailVisa
	Now   time.Time
}

// AllocationEmailGroups are the legacy group addresses.
type AllocationEmailGroups struct {
	AllocationsGroup        string
	PartnerAllocationsGroup string
	TravelGroup             string
	// AppBaseURL is the One WSO2 origin for the "View My Allocations" link.
	AppBaseURL string
}

// AllocationMyAllocationsLink is the person's own allocations page in One WSO2
// (Me → Allocations), the button in every allocation email.
func AllocationMyAllocationsLink(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/me/allocations"
}

// AllocationMessage builds the email for one allocation change. It goes To
// whoever must act next (the consultant to respond, the allocations team to
// reallocate or confirm, the travel team to arrange travel) and everyone else
// is copied.
func AllocationMessage(d AllocationEmailData, g AllocationEmailGroups) Message {
	isInternal := d.EngagementTypeID == 1 || d.EngagementTypeID == 5
	isEvent := d.EngagementTypeID == 5
	email := d.ConsultantEmail
	partner := AllocationIsPartnerEmail(email)
	what := "engagement"
	if isEvent {
		what = "marketing event"
	}
	templateID := "customerEngagementAllocation"
	switch {
	case isEvent:
		templateID = "marketingEventAllocation"
	case isInternal:
		templateID = "internalEngagementAllocation"
	}
	who := d.ConsultantName
	if who == "" {
		who = email
	}

	// The allocations team (or the partner allocations team) and, for a
	// consultant, the consultant and their leads.
	team, teamName, teamRole := g.AllocationsGroup, "Allocations Team", "the allocations team"
	if partner {
		team, teamName, teamRole = g.PartnerAllocationsGroup, "Partner Allocations Team", "the partner allocations team"
	}
	engagementBase := strings.TrimRight(g.AppBaseURL, "/") + "/finance/engagement-app/engagements/" + url.PathEscape(d.EngagementID)
	myAllocations, myLabel := AllocationMyAllocationsLink(g.AppBaseURL), "View My Allocations"

	c, prev := d.ClearanceStatus, d.PrevClearanceStatus
	travelArranged := d.TravelRequired && (prev == allocConfirmed || prev == allocConfirmedVisaPending)
	confirmed := false
	var action, description, label, link, toName string
	var to []string
	var toRoles []string
	forConsultant := func() {
		if partner {
			to, toName, toRoles = []string{team}, teamName, []string{teamRole}
			label, link = "Review Allocations", engagementBase+"/allocations"
			return
		}
		to, toName, toRoles = []string{email}, d.ConsultantName, []string{"the allocated consultant"}
		label, link = myLabel, myAllocations
	}
	forTeam := func(lbl, path string) {
		to, toName, toRoles = []string{team}, teamName, []string{teamRole}
		label, link = lbl, engagementBase+path
	}
	var visaData, readiness string
	switch {
	case c == allocClearanceInProgress:
		action = "Allocated"
		description = "You have been allocated to the following " + what + ". Discuss it with your lead and let us know your availability."
		if partner {
			description = email + " has been allocated to the following " + what + ". Confirm their availability."
		}
		forConsultant()
		if !partner {
			label = "Respond to Allocation"
		}
	case c == allocRejectedByConsultant || c == allocRejectedOther:
		action = "Allocation Declined"
		description = who + " declined the following allocation. Allocate someone else."
		forTeam("Reallocate", "/allocate")
	case prev != allocReadyForClearance && c == allocAllocationCancelled:
		action = "Allocation Cancelled"
		description = "The following allocation was cancelled."
		forConsultant()
	case c == allocAcceptedByConsultant:
		action = "Allocation Accepted"
		description = who + " accepted the following allocation. Review it and confirm."
		forTeam("Review Allocation", "/allocations")
	case c == allocRejectedVisaIssues:
		action = "Allocation Cancelled"
		description = "The following allocation was cancelled for " + who + " because of visa issues."
		forConsultant()
	case AllocationIsConfirmation(c, prev):
		action = "Allocation Confirmed"
		if d.TravelRequired {
			description = who + " is confirmed for this " + what + ". Arrange the travel."
			to, toName, toRoles = []string{g.TravelGroup}, "Travel Team", []string{"the travel team"}
			label, link = "Review Travel Requests", strings.TrimRight(g.AppBaseURL, "/")+"/finance/engagement-app/travel-requests/new"
		} else {
			description = "You are confirmed for this " + what + "."
			if partner {
				description = email + " is confirmed for this " + what + "."
			}
			forConsultant()
		}
		if !partner {
			visaData, readiness = allocVisaData(d)
		}
		templateID = "consultantConfirmed"
		confirmed = true
	}

	// Copies: everyone else who receives this email today.
	var cc []string
	var ccRoles []string
	add := func(role string, addrs ...string) {
		added := false
		for _, a := range addrs {
			if a != "" && !slices.Contains(to, a) {
				cc = append(cc, a)
				added = true
			}
		}
		if added {
			ccRoles = append(ccRoles, role)
		}
	}
	if !partner {
		add("the allocated consultant", email)
		if len(d.Leads) > 0 {
			add("the consultant's leads", d.Leads...)
		}
	}
	add(teamRole, team)
	add("the person who made the change", d.Actor)
	if (confirmed && d.TravelRequired) || (!confirmed && travelArranged && c != allocClearanceInProgress && c != allocAcceptedByConsultant) {
		add("the travel team", g.TravelGroup)
	}

	facts := engFacts{ID: d.EngagementID, Customer: d.CustomerName, Project: d.OpportunityName, Product: d.ProductName,
		AllocationType: d.AllocationTypeName, Code: d.EngagementCode, Nature: d.EngagementNature, Country: d.Country,
		Address: d.Address, Contact: d.CustomerContact, Domain: d.BusinessDomain, OpportunityOwner: d.OpportunityOwner,
		TechnicalOwner: d.TechnicalOwner, Scope: d.EngagementScope, Comment: d.EngagementComment,
		ContractsSigned: d.ContractsSigned, Kind: d.EngagementTypeID}
	if len(d.FundingSources) == 1 {
		f := d.FundingSources[0]
		facts.BusinessUnit, facts.ProductUnit, facts.SubRegion = f.BusinessUnit, f.ProductUnit, f.SubRegion
	}
	details := detailsSection("Allocation",
		row{"Consultant", who}, row{"Dates", dateRange(d.StartDate, d.EndDate)},
		row{"Allocation Remarks", d.AllocationComment}, row{"Travel Readiness", readiness})
	if visaData != "" {
		details += sectionTitle("Visa") + visaData
	}
	details += detailsSection(facts.blockTitle(), facts.rows(false)...)
	content := map[string]string{
		"title":         action,
		"description":   html.EscapeString(description),
		"greeting_name": toName,
		"reason":        "You are receiving this because you are involved in this allocation, or because you are copied on it.",
		"action_label":  label,
		"action_link":   link,
		"details_html":  details,
	}

	return Message{
		App:              AppAllocations,
		To:               to,
		CC:               cc,
		Audience:         Audience{To: toRoles, CC: ccRoles},
		Subject:          subjectFor(action, d.EngagementID, d.OpportunityName),
		TemplateID:       templateID,
		Content:          content,
		NotificationType: allocNotificationPrefix + ": " + c,
		ReferenceID:      d.EngagementID,
	}
}

// allocVisaData builds the legacy visa paragraphs and readiness comment.
// The "expiring within a week" warning compares the visa end date with now
// plus seven days, as the legacy code does.
func allocDateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

func allocVisaData(d AllocationEmailData) (string, string) {
	if len(d.Visas) == 0 {
		return "", allocVisaUnavailableComent
	}
	var b strings.Builder
	weekAhead := d.Now.Add(7 * 24 * time.Hour)
	for _, v := range d.Visas {
		country := v.Country
		if country == "" {
			country = d.Country
		}
		fmt.Fprintf(&b, `<p><span style="%s">%s: %s %s Entry, Visa %s from %s to %s</span></p>`, allocVisaParagraphStyle,
			html.EscapeString(country), html.EscapeString(v.VisaType), html.EscapeString(v.EntryType),
			html.EscapeString(v.Status), html.EscapeString(allocDateOnly(v.StartDate)), html.EscapeString(allocDateOnly(v.EndDate)))
		if end, err := time.Parse("2006-01-02", allocDateOnly(v.EndDate)); err == nil && weekAhead.Sub(end) > 0 {
			fmt.Fprintf(&b, `<span style="%s">The visa expires within a week of the allocation end date</span>`, allocVisaWarningStyle)
		}
	}
	return b.String(), allocVisaAvailableComment
}
