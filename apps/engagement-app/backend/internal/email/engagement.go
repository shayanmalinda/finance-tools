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

// EngagementEvent is the engagement lifecycle event an email is for.
type EngagementEvent string

// The legacy engagement emails (apps/allocation/backend/modules/email,
// alloc-port-spec §5.2).
const (
	EngagementCreated            EngagementEvent = "ENGAGEMENT_CREATED"
	EngagementUpdated            EngagementEvent = "ENGAGEMENT_UPDATED"
	EngagementApproved           EngagementEvent = "ENGAGEMENT_APPROVED"
	EngagementUpdatedAndApproved EngagementEvent = "ENGAGEMENT_UPDATED_AND_APPROVED"
	EngagementCancelled          EngagementEvent = "ENGAGEMENT_CANCELLED"
)

// EngagementEmailData is the engagement as the legacy templates read it.
type EngagementEmailData struct {
	ID                 string
	EngagementTypeID   int
	RequestStatus      string
	CustomerName       string
	ProjectName        string
	ProductName        string
	AllocationTypeName string
	EngagementCode     string
	Nature             string
	Country            string
	Address            string
	StartDate          string
	EndDate            string
	CustomerContact    string
	BusinessDomain     string
	OpportunityOwner   string
	TechnicalOwner     string
	ContractsSigned    bool
	EngagementScope    string
	Comment            string
	// First funding source (the templates show one); empty for none.
	BusinessUnit string
	ProductUnit  string
	SubRegion    string
	// Requester is engagement.username, copied on every email.
	Requester string
}

// EngagementRecipients are the addresses an engagement email adds beyond
// the actor and the requester.
type EngagementRecipients struct {
	// AppBaseURL is the One WSO2 origin for the link.
	AppBaseURL string
	// Approvers are the legacy approveInternalEmail / approveMarketingEmail
	// lists (created emails of types 1 and 5).
	InternalApprovers  []string
	MarketingApprovers []string
	TravelGroup        string
	// Routing is the allocation type's routing: the requester's lead(s)
	// (Notify To Allocations "No"), the Allocations group ("Yes", or any
	// customer engagement), or nothing.
	Routing []string
	// Consultants are the cancelled allocations' consultants.
	Consultants []string
	// TravelAffected copies the Travel group on a cancellation: a cancelled
	// allocation was Confirmed / Confirmed - Visa Pending with travel.
	TravelAffected bool
}

func engagementIsInternal(t int) bool { return t == 1 || t == 5 }

// engagementTravelExpected is the legacy isTravelExpected.
func engagementTravelExpected(nature string) bool {
	switch strings.ToLower(strings.TrimSpace(nature)) {
	case "physical", "hybrid", "on-site":
		return true
	}
	return false
}

func engagementOrNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

func engagementDateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// EngagementMessage builds the legacy engagement email for ev (subject,
// title, description, template id, content keys, To and CC), sent by actor.
func EngagementMessage(ev EngagementEvent, e EngagementEmailData, r EngagementRecipients, actor string) Message {
	internal := engagementIsInternal(e.EngagementTypeID)
	marketing := e.EngagementTypeID == 5
	ref := e.ID + " : " + e.CustomerName + " - " + e.ProjectName

	var subject, title, description, template string
	switch {
	case internal && marketing:
		template = "marketingEvent"
	case internal:
		template = "internalEngagement"
	default:
		template = "customerEngagement"
	}
	what := "engagement"
	if marketing {
		what = "marketing event"
	}
	to := []string{}
	cc := []string{}
	var toRoles, ccRoles []string
	switch ev {
	case EngagementCreated:
		literal := "Consultant"
		if internal {
			literal = "Approval"
		}
		subject = "[Engagement App] " + literal + " request for " + ref
		switch {
		case marketing:
			title = "New Marketing Event created - Please approve"
		case internal:
			title = "New Internal Engagement created - Please approve"
		default:
			title = "New Customer Engagement Created"
		}
		description = "A new " + what + " has been created. Please review and approve."
		to = append(to, actor)
		toRoles = append(toRoles, "the engagement creator")
		if internal {
			if marketing {
				to = append(to, r.MarketingApprovers...)
				toRoles = append(toRoles, "marketing event approvers")
			} else {
				to = append(to, r.InternalApprovers...)
				toRoles = append(toRoles, "internal engagement approvers")
			}
		}
	case EngagementUpdated:
		literal := "Consultant"
		if e.RequestStatus == "Pending Approval" {
			literal = "Approval"
		}
		subject = "Re: [Engagement App] " + literal + " request for " + ref
		switch {
		case marketing:
			title = "Marketing Event Updated"
		case internal:
			title = "Internal Engagement Updated"
		default:
			title = "Customer Engagement Updated"
		}
		description = "Please note that the following " + what + " has been updated."
	case EngagementApproved, EngagementUpdatedAndApproved:
		status := "Approved"
		if ev == EngagementUpdatedAndApproved {
			status = "Updated and Approved"
		}
		subject = "Re: [Engagement App] Consultant request for " + ref
		if marketing {
			title = "Marketing Event " + status
			template = "marketingEvent"
		} else {
			title = "Internal Engagement " + status
			template = "internalEngagement"
		}
		description = "Please note that the following " + what + " has been " + strings.ToLower(status) + "."
	case EngagementCancelled:
		subject = "Re: [Engagement App] Consultant request for " + ref
		switch {
		case marketing:
			title, template = "Marketing Event Cancelled", "marketingEventCancelled"
		case internal:
			title, template = "Internal Engagement Cancelled", "internalEngagementCancelled"
		default:
			title, template = "Customer Engagement Cancelled", "customerEngagementCancelled"
		}
		description = "Please note that the following " + what + " has been cancelled."
		to = append(to, r.Consultants...)
		toRoles = append(toRoles, "the allocated consultants")
		if r.TravelAffected && r.TravelGroup != "" {
			cc = append(cc, r.TravelGroup)
			ccRoles = append(ccRoles, "the travel team")
		}
	}
	cc = append(cc, e.Requester)
	ccRoles = append(ccRoles, "the engagement requester")
	to = append(to, r.Routing...)
	if len(r.Routing) > 0 {
		toRoles = append(toRoles, "the requester's lead or the allocations team (per allocation type)")
	}
	to = append(to, actor)
	toRoles = append(toRoles, "the person who made the change")
	if (ev == EngagementApproved || ev == EngagementUpdatedAndApproved) && engagementTravelExpected(e.Nature) && r.TravelGroup != "" {
		cc = append(cc, r.TravelGroup)
		ccRoles = append(ccRoles, "the travel team")
	}

	base := strings.TrimRight(r.AppBaseURL, "/") + "/finance/engagement-app"
	link := base + "/engagements/" + url.PathEscape(e.ID) + "/allocations"
	if strings.Contains(title, "Please approve") {
		seg := "internal"
		if marketing {
			seg = "marketing"
		}
		link = base + "/approvals/" + seg
	}
	contracts := "No"
	if e.ContractsSigned {
		contracts = "Yes"
	}
	esc := html.EscapeString
	content := map[string]string{
		"title":             title,
		"description":       description,
		"allocate_now_link": link,
		"customer_name":     esc(e.CustomerName),
		"project_name":      esc(e.ProjectName),
		"product_name":      esc(e.ProductName),
		"allocation_type":   esc(e.AllocationTypeName),
		"engagement_code":   esc(e.EngagementCode),
		"engagement_nature": esc(e.Nature),
		"global_pod":        esc(engagementOrNA(e.SubRegion)),
		"country":           esc(e.Country),
		"location_address":  esc(engagementOrNA(e.Address)),
		"product_unit":      esc(engagementOrNA(e.ProductUnit)),
		"business_unit":     esc(engagementOrNA(e.BusinessUnit)),
		"start_date":        engagementDateOnly(e.StartDate),
		"end_date":          engagementDateOnly(e.EndDate),
		"customer_contact":  esc(engagementOrNA(e.CustomerContact)),
		"business_domain":   esc(engagementOrNA(e.BusinessDomain)),
		"opportunity_owner": esc(e.OpportunityOwner),
		"technical_owner":   esc(e.TechnicalOwner),
		"contracts_signed":  contracts,
		"engagement_scope":  esc(engagementOrNA(e.EngagementScope)),
		"comments":          esc(engagementOrNA(e.Comment)),
	}
	notification := string(ev)
	if ev == EngagementUpdatedAndApproved {
		notification = string(EngagementApproved)
	}
	return Message{
		App: AppAllocations, To: to, CC: cc, Subject: subject, TemplateID: template, Content: content,
		NotificationType: notification, ReferenceID: e.ID,
		Audience: Audience{To: toRoles, CC: ccRoles},
	}
}
