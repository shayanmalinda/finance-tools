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
	allocVisaWarningStyle      = `font-family: 'Inter', 'Segoe UI', Helvetica, Arial, sans-serif;font-size: 13px;line-height: 20px;color: #b42318;text-align: left;margin: 0 0 8px;font-weight: 500;display: block;`
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

func allocNA(s string) string {
	if strings.TrimSpace(s) == "" {
		return "N/A"
	}
	return s
}

func allocDateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// AllocationMessage ports sendEmailForAllocation: recipients, subject,
// title, description, template and content keys for one allocation change.
func AllocationMessage(d AllocationEmailData, g AllocationEmailGroups) Message {
	isInternal := d.EngagementTypeID == 1 || d.EngagementTypeID == 5
	isEvent := d.EngagementTypeID == 5
	email := d.ConsultantEmail
	pick := func(event, internal, customer string) string {
		switch {
		case isEvent:
			return event
		case isInternal:
			return internal
		}
		return customer
	}

	var to []string
	if AllocationIsPartnerEmail(email) {
		to = append(to, g.PartnerAllocationsGroup)
	} else {
		to = append(to, g.AllocationsGroup, email)
		to = append(to, d.Leads...)
	}
	to = append(to, d.Actor)

	c, prev := d.ClearanceStatus, d.PrevClearanceStatus
	travelArranged := d.TravelRequired && (prev == allocConfirmed || prev == allocConfirmedVisaPending)
	templateID := pick("marketingEventAllocation", "internalEngagementAllocation", "customerEngagementAllocation")
	var toKey, title, description, consultant, visaData, readiness string

	switch {
	case c == allocClearanceInProgress:
		toKey = email
		title = pick("Allocation Added for Marketing Event", "Allocation Added for Internal Engagement", "Allocation Added for Customer Engagement")
		if isEvent {
			description = "You have been identified as a consultant for the following marketing event. Please discuss with your lead and let us know your availability."
		} else {
			description = "You have been identified as a consultant for the following engagement. Please discuss with your lead and let us know your availability."
		}
	case c == allocRejectedByConsultant || c == allocRejectedOther:
		if travelArranged {
			to = append(to, g.TravelGroup)
		}
		toKey = "All"
		title = pick("Allocation Declined for Marketing Event", "Allocation Declined for Internal Engagement", "Allocation Declined for Customer Engagement")
		description = fmt.Sprintf("Please note that %s has declined the following allocation request", email)
	case prev != allocReadyForClearance && c == allocAllocationCancelled:
		if travelArranged {
			to = append(to, g.TravelGroup)
		}
		toKey = email
		title = pick("Allocation Cancelled for Marketing Event", "Allocation Cancelled for Internal Engagement", "Allocation Cancelled for Customer Engagement")
		description = "Please note that the following allocation has been cancelled."
	case c == allocAcceptedByConsultant:
		toKey = "All"
		title = pick("Allocation Accepted for Marketing Event", "Allocation Accepted for Internal Engagement", "Allocation Accepted for Customer Engagement")
		description = fmt.Sprintf("The following allocation requested has been accepted by %s.", email)
	case c == allocRejectedVisaIssues:
		if travelArranged {
			to = append(to, g.TravelGroup)
		}
		toKey = "All"
		title = pick("Allocation Cancelled for Marketing Event", "Allocation Cancelled for Internal Engagement", "Allocation Cancelled for Customer Engagement")
		description = fmt.Sprintf("The following allocation requested has been cancelled for %s due to visa issues", email)
	case AllocationIsConfirmation(c, prev):
		if d.TravelRequired {
			to = append(to, g.TravelGroup)
		}
		if isEvent {
			title = "Allocation Confirmed for Marketing Event"
			description = "The following consultant has been confirmed for this marketing event. Please make necessary arrangements."
		} else {
			title = "Allocation Confirmed for Engagement"
			description = "The following consultant has been confirmed for this engagement. Please make necessary arrangements."
		}
		consultant = email
		if !AllocationIsPartnerEmail(email) {
			visaData, readiness = allocVisaData(d)
		}
		templateID = "consultantConfirmed"
	}

	bu, pu, pod := "N/A", "N/A", "N/A"
	if len(d.FundingSources) == 1 {
		bu, pu, pod = d.FundingSources[0].BusinessUnit, d.FundingSources[0].ProductUnit, d.FundingSources[0].SubRegion
	}
	contracts := "No"
	if d.ContractsSigned {
		contracts = "Yes"
	}
	content := map[string]string{
		"to":                       toKey,
		"title":                    title,
		"description":              description,
		"info_title":               map[bool]string{true: "Marketing Event Info", false: "Engagement Info"}[isEvent],
		"project_name_label":       pick("Event Name", "Name / Purpose", "Opportunity"),
		"consultant":               consultant,
		"customer_name":            d.CustomerName,
		"project_name":             d.OpportunityName,
		"product_name":             d.ProductName,
		"allocation_type":          d.AllocationTypeName,
		"engagement_code":          d.EngagementCode,
		"engagement_nature":        d.EngagementNature,
		"global_pod":               pod,
		"country":                  d.Country,
		"location_address":         allocNA(d.Address),
		"product_unit":             pu,
		"business_unit":            bu,
		"start_date":               allocDateOnly(d.StartDate),
		"end_date":                 allocDateOnly(d.EndDate),
		"customer_contact":         allocNA(d.CustomerContact),
		"business_domain":          allocNA(d.BusinessDomain),
		"opportunity_owner":        d.OpportunityOwner,
		"technical_owner":          d.TechnicalOwner,
		"contracts_signed":         contracts,
		"engagement_scope":         allocNA(d.EngagementScope),
		"comments":                 allocNA(d.EngagementComment),
		"allocation_comments":      allocNA(d.AllocationComment),
		"consultant_role":          "",
		"travel_readiness_comment": readiness,
		"action_link":              AllocationMyAllocationsLink(g.AppBaseURL),
	}
	for k, v := range content {
		content[k] = html.EscapeString(v)
	}
	content["visa_data"] = visaData // built escaped

	return Message{
		App:              AppAllocations,
		To:               to,
		CC:               []string{},
		Subject:          "Re: [Allocations] Consultant request for " + d.EngagementID + " : " + d.CustomerName + " - " + d.OpportunityName,
		TemplateID:       templateID,
		Content:          content,
		NotificationType: allocNotificationPrefix + ": " + c,
		ReferenceID:      d.EngagementID,
	}
}

// allocVisaData builds the legacy visa paragraphs and readiness comment.
// The "expiring within a week" warning compares the visa end date with now
// plus seven days, as the legacy code does.
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
			fmt.Fprintf(&b, `<span style="%s">(Above visa will be expiring within a week after the allocation end date)</span>`, allocVisaWarningStyle)
		}
	}
	return b.String(), allocVisaAvailableComment
}
