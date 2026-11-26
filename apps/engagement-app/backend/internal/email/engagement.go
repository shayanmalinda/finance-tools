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
	// RoutingName, ConsultantName and ActorName are the greeting names of
	// those recipients ("Allocations Team", or a first name): "" when unknown
	// or when several people share the To line.
	RoutingName, ConsultantName, ActorName string
	// ApproverName is the first name of the approver when the approvers list
	// holds one person; "" keeps the approvers' team label.
	ApproverName string
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

// EngagementMessage builds the engagement email for ev (subject, title,
// description, button, To and CC), sent by actor. The email goes To whoever
// must act next and everyone else is copied: an internal engagement or
// marketing event waiting for approval goes To its approvers ("Approval
// Needed"), an approved one or a customer engagement To whoever allocates
// ("Ready to Allocate").
func EngagementMessage(ev EngagementEvent, e EngagementEmailData, r EngagementRecipients, actor string) Message {
	internal := engagementIsInternal(e.EngagementTypeID)
	marketing := e.EngagementTypeID == 5
	what := "engagement"
	if marketing {
		what = "marketing event"
	}
	template := "customerEngagement"
	switch {
	case marketing:
		template = "marketingEvent"
	case internal:
		template = "internalEngagement"
	}

	base := strings.TrimRight(r.AppBaseURL, "/") + "/finance/engagement-app/engagements/" + url.PathEscape(e.ID)
	var action, description, label, link, reason string
	var to, cc []string
	var toRoles, ccRoles []string
	var toName string
	allocate := func(done string) {
		action, label, link = "Ready to Allocate", "Allocate Now", base+"/allocate"
		description = done + " Allocate the consultants it needs."
		to, toName = r.Routing, r.RoutingName
		toRoles = []string{"whoever allocates: the allocations team or the requester's lead (per allocation type)"}
		reason = "you allocate consultants for this " + what
	}
	switch ev {
	case EngagementCreated:
		if internal {
			action, label, link = "Approval Needed", "Review and Approve", base+"/details"
			description = "A new " + what + " was created and needs your approval. Review it and approve it."
			to, toName, toRoles = r.InternalApprovers, "Internal Engagement Approvers", []string{"the internal engagement approvers"}
			if marketing {
				to, toName, toRoles = r.MarketingApprovers, "Marketing Approvers", []string{"the marketing event approvers"}
			}
			if r.ApproverName != "" && len(to) == 1 {
				toName = r.ApproverName
			}
			reason = "you approve " + what + "s"
			cc = append(cc, r.Routing...)
			ccRoles = append(ccRoles, "whoever allocates (per allocation type)")
		} else {
			allocate("A new " + what + " was created.")
		}
	case EngagementApproved, EngagementUpdatedAndApproved:
		done := "The " + what + " was approved."
		if ev == EngagementUpdatedAndApproved {
			done = "The " + what + " was updated and approved."
		}
		allocate(done)
		if engagementTravelExpected(e.Nature) && r.TravelGroup != "" {
			cc = append(cc, r.TravelGroup)
			ccRoles = append(ccRoles, "the travel team")
		}
	case EngagementUpdated:
		action, label, link = "Updated", "View Engagement", base+"/allocations"
		if e.RequestStatus == "Pending Approval" {
			link = base + "/details"
		}
		description = "The " + what + " was updated. Review the changes."
		to, toName = r.Routing, r.RoutingName
		toRoles = []string{"whoever allocates: the allocations team or the requester's lead (per allocation type)"}
		reason = "you allocate consultants for this " + what
	case EngagementCancelled:
		action = "Cancelled"
		description = "The " + what + " was cancelled, and so were its allocations."
		to, toName = r.Consultants, r.ConsultantName
		toRoles = []string{"the allocated consultants"}
		reason = "you were allocated to this " + what
		if len(to) == 0 {
			to, toName, toRoles = r.Routing, r.RoutingName, []string{"whoever allocates (per allocation type)"}
			reason = "you allocate consultants for this " + what
		} else {
			cc = append(cc, r.Routing...)
			ccRoles = append(ccRoles, "whoever allocates (per allocation type)")
		}
		if r.TravelAffected && r.TravelGroup != "" {
			cc = append(cc, r.TravelGroup)
			ccRoles = append(ccRoles, "the travel team")
		}
		template += "Cancelled"
	}
	// Nobody to act: the person who made the change gets it.
	if len(to) == 0 {
		to, toName, toRoles = []string{actor}, r.ActorName, []string{"the person who made the change"}
		reason = "you made this change"
	} else {
		cc = append(cc, actor)
		ccRoles = append(ccRoles, "the person who made the change")
	}
	cc = append(cc, e.Requester)
	ccRoles = append(ccRoles, "the engagement requester")

	facts := engFacts{ID: e.ID, Customer: e.CustomerName, Project: e.ProjectName, Product: e.ProductName,
		AllocationType: e.AllocationTypeName, Code: e.EngagementCode, Nature: e.Nature, Country: e.Country,
		Address: e.Address, Start: e.StartDate, End: e.EndDate, Contact: e.CustomerContact, Domain: e.BusinessDomain,
		OpportunityOwner: e.OpportunityOwner, TechnicalOwner: e.TechnicalOwner, Scope: e.EngagementScope,
		Comment: e.Comment, BusinessUnit: e.BusinessUnit, ProductUnit: e.ProductUnit, SubRegion: e.SubRegion,
		ContractsSigned: e.ContractsSigned, Kind: e.EngagementTypeID}
	content := map[string]string{
		"title":         action,
		"description":   html.EscapeString(description),
		"greeting_name": toName,
		"reason":        "You are receiving this because " + reason + ", or because you are copied on it.",
		"action_label":  label,
		"action_link":   link,
		"details_html":  detailsSection(facts.blockTitle(), facts.rows(true)...),
	}
	notification := string(ev)
	if ev == EngagementUpdatedAndApproved {
		notification = string(EngagementApproved)
	}
	return Message{
		App: AppAllocations, To: to, CC: cc, Subject: subjectFor(action, e.ID, e.ProjectName), TemplateID: template, Content: content,
		NotificationType: notification, ReferenceID: e.ID,
		Audience: Audience{To: toRoles, CC: ccRoles},
	}
}
