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

package service

import (
	"slices"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Travel privilege ids (travelapp.privileges). They belong to the Travel app
// only: Allocations ids mean other things.
const (
	travelPrivNewRequests     = 1
	travelPrivAllRequests     = 2
	travelPrivEditStatus      = 4
	travelPrivEditAgent       = 5
	travelPrivEditVisa        = 6
	travelPrivAddVisa         = 7
	travelPrivViewVisaList    = 8
	travelPrivViewVisaInfo    = 9
	travelPrivEditVisa2       = 10
	travelPrivViewDocList     = 11
	travelPrivViewDocInfo     = 12
	travelPrivEditDoc         = 13
	travelPrivAddDoc          = 14
	travelPrivAgentRequests   = 18
	travelPrivViewOwnVisa     = 20
	travelPrivAddOwnVisa      = 21
	travelPrivViewOwnDoc      = 23
	travelPrivAddOwnDoc       = 24
	travelPrivAddPerDiems     = 26
	travelPrivDeletePerDiems  = 27
	travelPrivViewPerDiems    = 28
	travelPrivViewInvoices    = 30
	travelPrivAddInvoices     = 31
	travelPrivInvoiceHR       = 32
	travelPrivInvoiceFinance  = 33
	travelPrivPushInvoices    = 34
	travelPrivFundingSources  = 35
	travelPrivViewOwnRequest  = 36
	travelPrivViewOwnPerDiems = 37
	travelPrivViewOwnInvoices = 38
	travelPrivPushPayments    = 40
	travelPrivViewPayments    = 42
	travelPrivAddPayments     = 43
	travelPrivEditPayments    = 44
	travelPrivViewCards       = 45
	travelPrivAddCards        = 46
	travelPrivViewOwnPayments = 47
	travelPrivViewOwnCards    = 48
	travelPrivMyRequests      = 49
)

// travelNextStatuses is the legacy UI transition matrix
// (TravelInformation.js): the statuses a request may move to from each
// status. The legacy server accepted any status; Release 1 enforces this
// matrix on the server too.
var travelNextStatuses = map[string][]string{
	domain.TravelStatusNew:        {domain.TravelStatusNew, domain.TravelStatusInProgress, domain.TravelStatusOnHold, domain.TravelStatusCancelled},
	domain.TravelStatusInProgress: {domain.TravelStatusInProgress, domain.TravelStatusCompleted, domain.TravelStatusOnHold, domain.TravelStatusCancelled, domain.TravelStatusExtended},
	domain.TravelStatusCompleted:  {domain.TravelStatusCompleted, domain.TravelStatusExtended, domain.TravelStatusCancelled},
	domain.TravelStatusOnHold:     {domain.TravelStatusInProgress, domain.TravelStatusOnHold, domain.TravelStatusCancelled, domain.TravelStatusExtended},
	domain.TravelStatusCancelled:  {domain.TravelStatusInProgress, domain.TravelStatusOnHold, domain.TravelStatusCancelled, domain.TravelStatusExtended},
	domain.TravelStatusExtended:   {domain.TravelStatusOnHold, domain.TravelStatusCancelled, domain.TravelStatusExtended, domain.TravelStatusCompleted},
}

// travelAssignStatuses are the statuses the Assign card offers.
var travelAssignStatuses = []string{domain.TravelStatusInProgress, domain.TravelStatusOnHold, domain.TravelStatusCancelled}

// travelOpen reports whether a request is open (In Progress or Extended):
// per diems, cards, payments, invoice actions and the agent and visa can
// change only then.
func travelOpen(status string) bool {
	return status == domain.TravelStatusInProgress || status == domain.TravelStatusExtended
}

// travelNeedsAgentAndVisa: In Progress, Completed and Extended need a travel
// agent and a visa (legacy UI rule, enforced on the server in Release 1).
func travelNeedsAgentAndVisa(status string) bool {
	return status == domain.TravelStatusInProgress || status == domain.TravelStatusCompleted ||
		status == domain.TravelStatusExtended
}

// travelFull reports whether p sees every travel request (Travel 1, 2 or 5).
// A holder of the agent privilege (18) without 1 or 2 never does, even with
// 5 and even when no agency resolved for them: it is scoped to its agency,
// as in List and the finance views, and fails closed without one.
func travelFull(p domain.Perms) bool {
	agentOnly := p.Travel.Has(travelPrivAgentRequests) && !p.Travel.Any(travelPrivNewRequests, travelPrivAllRequests)
	return !agentOnly && p.Travel.Any(travelPrivNewRequests, travelPrivAllRequests, travelPrivEditAgent)
}

// travelScope is the caller's scope on a request, or "" (no access):
//   - full: Travel 1, 2 or 5, unless an agent;
//   - agency: an agent (18) of the request's assigned agency (Release 1
//     scoping; legacy let holders of 3 open any request by number);
//   - own: the traveller with View Own Travel Request Information (36).
func travelScope(p domain.Perms, travellerEmail string, agentID *int) string {
	switch {
	case travelFull(p):
		return domain.TravelScopeFull
	case p.Agency != nil && p.Travel.Has(travelPrivAgentRequests) && agentID != nil && *agentID == p.Agency.ID:
		return domain.TravelScopeAgency
	case strings.EqualFold(strings.TrimSpace(travellerEmail), p.Email) && p.Travel.Has(travelPrivViewOwnRequest):
		return domain.TravelScopeOwn
	}
	return ""
}

// travelSees reports whether a scope sees a part of the request
// (prototype tripShows): full and own see everything; agency sees per
// diems and invoices only (no engagement, funding, cards or payments).
func travelSees(scope, part string) bool {
	switch scope {
	case domain.TravelScopeFull, domain.TravelScopeOwn:
		return true
	case domain.TravelScopeAgency:
		return part == "perdiem" || part == "invoices"
	}
	return false
}

// travelSection is the trip page section gate (prototype secOK): scope
// full or agency needs the "View" privilege, scope own the "View Own" one.
func travelSection(p domain.Perms, scope, part string, view, viewOwn int) bool {
	if !travelSees(scope, part) {
		return false
	}
	if scope == domain.TravelScopeOwn {
		return p.Travel.Has(viewOwn)
	}
	return p.Travel.Has(view)
}

func travelTabs(p domain.Perms, scope string) domain.TravelRequestTabs {
	return domain.TravelRequestTabs{
		PerDiems:    travelSection(p, scope, "perdiem", travelPrivViewPerDiems, travelPrivViewOwnPerDiems),
		Invoices:    travelSection(p, scope, "invoices", travelPrivViewInvoices, travelPrivViewOwnInvoices),
		TravelCards: travelSection(p, scope, "card", travelPrivViewCards, travelPrivViewOwnCards),
		Payments:    travelSection(p, scope, "payments", travelPrivViewPayments, travelPrivViewOwnPayments),
		Expenses:    scope == domain.TravelScopeFull && p.Travel.Has(travelPrivViewCards),
	}
}

// travelActions computes every action flag of a request from the caller's
// privileges, scope and the request's status (screens-travel-dash §1-§2).
// Scope own is read-only. Rules that need other data (no payment yet for
// Add Travel Card, per diems exist for Add Payment, the invoice's own
// status) are checked by the endpoints that do the action.
func travelActions(p domain.Perms, scope, status string) domain.TravelRequestActions {
	var a domain.TravelRequestActions
	if scope != domain.TravelScopeFull && scope != domain.TravelScopeAgency {
		return a
	}
	t := p.Travel
	open := travelOpen(status)
	tabs := travelTabs(p, scope)

	a.Assign = status == domain.TravelStatusNew && t.Has(travelPrivNewRequests)
	a.Update = t.Any(travelPrivEditStatus, travelPrivEditAgent, travelPrivEditVisa)
	a.EditStatus = a.Update && t.Has(travelPrivEditStatus) && travelNextStatuses[status] != nil
	// The agent and visa can change while the (new) status is open, so they
	// are editable on an open request, or when the caller may change the
	// status to an open one.
	canOpen := open || (a.EditStatus && slices.ContainsFunc(travelNextStatuses[status], travelOpen))
	a.EditAgency = a.Update && t.Has(travelPrivEditAgent) && canOpen
	a.EditVisa = a.Update && t.Has(travelPrivEditVisa) && canOpen

	a.AddPerDiem = tabs.PerDiems && open && t.Has(travelPrivAddPerDiems)
	a.DeletePerDiem = tabs.PerDiems && open && t.Has(travelPrivDeletePerDiems)
	a.AddInvoice = tabs.Invoices && t.Has(travelPrivAddInvoices) &&
		(open || status == domain.TravelStatusCancelled)
	a.ApproveInvoicesPo = tabs.Invoices && open && t.Has(travelPrivInvoiceHR)
	// Legacy finStep: the finance step only for users without HR approval.
	a.ApproveInvoices = tabs.Invoices && open && t.Has(travelPrivInvoiceFinance) && !t.Has(travelPrivInvoiceHR)
	// LEGACY-ISSUES L6: the per-trip push needs Push Invoices to Net Suite (34).
	a.PushInvoices = tabs.Invoices && open && t.Has(travelPrivPushInvoices)

	a.AddTravelCard = tabs.TravelCards && open && t.Has(travelPrivAddCards)
	a.AddPayment = tabs.Payments && open && t.Has(travelPrivAddPayments)
	a.EditPayment = tabs.Payments && open && t.Has(travelPrivEditPayments)
	a.PushPayments = tabs.Payments && t.Has(travelPrivPushPayments)
	return a
}
