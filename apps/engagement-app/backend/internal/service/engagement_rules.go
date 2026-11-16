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
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// The engagement record-level rules, ported from the legacy Allocations
// backend (interceptor + handlers) and webapp, as the Release 1 prototype
// lists them (screens-alloc.md §0, alloc-port-spec §2.3). Every number is an
// Allocations privilege unless it says Travel.

// engagementState is what the rules read from an engagement.
type engagementState struct {
	typeID        int
	requestStatus string
	createdBy     string
}

func (e engagementState) kind() string { return domain.EngagementKindOf(e.typeID) }
func (e engagementState) customer() bool {
	return e.kind() == domain.EngagementKindCustomer
}
func (e engagementState) pending() bool {
	return e.requestStatus == domain.EngagementRequestPendingApproval
}
func (e engagementState) cancelled() bool {
	return e.requestStatus == domain.EngagementRequestCancelled
}

// engagementPatchOK is the legacy check on every engagement PATCH
// (Allocations 4, 14, 7, 17 or 5), kept for Release 1.
func engagementPatchOK(p domain.Perms) bool { return p.Alloc.Any(4, 14, 7, 17, 5) }

// engagementRole is engRole: the customer list / allocate privileges for a
// customer engagement (3, 6, 32, 33), the internal ones (13, 16) otherwise.
func engagementRole(p domain.Perms, kind string) bool {
	if kind == domain.EngagementKindCustomer {
		return p.Alloc.Any(3, 6, 32, 33)
	}
	return p.Alloc.Any(13, 16)
}

// engagementSeesTravel is trips.view or trips.manage: Travel 1, 2 or 5.
func engagementSeesTravel(p domain.Perms) bool { return p.Travel.Any(1, 2, 5) }

// engagementCanAllocateType is the legacy rule, both halves: the Allocate
// page needs 6, 16 or 21, and the engagement type needs 32 (post sale 4, 6),
// 33 (pre sale 3, 7), 6 (retired Paid 2) or 16 (internal 1, event 5).
func engagementCanAllocateType(p domain.Perms, typeID int) bool {
	if !p.Alloc.Any(6, 16, 21) {
		return false
	}
	switch typeID {
	case domain.EngagementTypeNonPaidPost, domain.EngagementTypePaidPostSale:
		return p.Alloc.Has(32)
	case domain.EngagementTypeNonPaidPre, domain.EngagementTypePaidPreSale:
		return p.Alloc.Has(33)
	case domain.EngagementTypePaid:
		return p.Alloc.Has(6)
	case domain.EngagementTypeInternal, domain.EngagementTypeMarketing:
		return p.Alloc.Has(16)
	}
	return false
}

// engagementCanEdit is canEditEng plus the legacy PATCH check: 5 (customer)
// or 15 (internal, marketing).
func engagementCanEdit(p domain.Perms, e engagementState) bool {
	if !engagementPatchOK(p) {
		return false
	}
	if e.customer() {
		return p.Alloc.Has(5)
	}
	return p.Alloc.Has(15)
}

// engagementCanEditPending is canEditPending: a Pending Approval internal
// engagement with 27, or event with 30, plus the PATCH check.
func engagementCanEditPending(p domain.Perms, e engagementState) bool {
	if !e.pending() || !engagementPatchOK(p) {
		return false
	}
	switch e.typeID {
	case domain.EngagementTypeMarketing:
		return p.Alloc.Has(30)
	case domain.EngagementTypeInternal:
		return p.Alloc.Has(27)
	}
	return false
}

// engagementCanApprove is canApprove: Pending Approval, type 1 or 5, the
// PATCH check, and 28 on both segments (legacy quirk: the Marketing Event
// Approver's 31 is never checked).
func engagementCanApprove(p domain.Perms, e engagementState) bool {
	return e.pending() && engagementPatchOK(p) && p.Alloc.Has(28) &&
		(e.typeID == domain.EngagementTypeInternal || e.typeID == domain.EngagementTypeMarketing)
}

// engagementCanComplete is Mark as Completed / Reopen: 7 (customer) or 17.
func engagementCanComplete(p domain.Perms, e engagementState) bool {
	if e.customer() {
		return p.Alloc.Has(7)
	}
	return p.Alloc.Has(17)
}

// engagementCanSetStatus is canSetStatus (4 customer / 14 internal, or the
// creator) plus the legacy PATCH check.
func engagementCanSetStatus(p domain.Perms, e engagementState) bool {
	if !engagementPatchOK(p) {
		return false
	}
	if e.createdBy != "" && strings.EqualFold(e.createdBy, p.Email) {
		return true
	}
	if e.customer() {
		return p.Alloc.Has(4)
	}
	return p.Alloc.Has(14)
}

// engagementCanEditAllocationStatus is canEditStatus: the allocate rule for
// a customer engagement, 20 for internal and marketing (10 is never
// checked, legacy quirk).
func engagementCanEditAllocationStatus(p domain.Perms, e engagementState) bool {
	if e.customer() {
		return engagementCanAllocateType(p, e.typeID)
	}
	return p.Alloc.Has(20)
}

// engagementActionsFor is engActions (plus Approve and Edit Pending).
func engagementActionsFor(p domain.Perms, e engagementState) domain.EngagementActions {
	open := !e.pending() && !e.cancelled()
	return domain.EngagementActions{
		Allocate:               open && engagementCanAllocateType(p, e.typeID),
		Edit:                   open && engagementCanEdit(p, e),
		EditPending:            engagementCanEditPending(p, e),
		Approve:                engagementCanApprove(p, e),
		Complete:               open && engagementCanComplete(p, e),
		ChangeEngagementStatus: open && engagementCanSetStatus(p, e),
		ChangeAllocationStatus: engagementCanEditAllocationStatus(p, e),
	}
}

// engagementScopeFor is engScope: "" (no access), full, roster or
// consultants. Travel agents never see engagements.
func engagementScopeFor(p domain.Perms, e engagementState) string {
	if p.IsAgent() {
		return ""
	}
	if engagementRole(p, e.kind()) {
		seesTravel := engagementSeesTravel(p)
		switch {
		case seesTravel:
			return domain.EngagementScopeFull
		case e.customer() && !p.Alloc.Any(6, 32, 33):
			return domain.EngagementScopeConsultants
		default:
			return domain.EngagementScopeRoster
		}
	}
	if e.pending() && ((e.typeID == domain.EngagementTypeMarketing && p.Alloc.Has(29)) ||
		(e.typeID == domain.EngagementTypeInternal && p.Alloc.Has(26))) {
		return domain.EngagementScopeRoster
	}
	return ""
}

// engagementSeesConsultants is the Consultants column: 8 (customer) or 18.
func engagementSeesConsultants(p domain.Perms, kind string) bool {
	if kind == domain.EngagementKindCustomer {
		return p.Alloc.Has(8)
	}
	return p.Alloc.Has(18)
}

// engagementSegmentGate is the list segment gate: customer 2, 3 or 9;
// internal and marketing 11, 12, 13 or 19.
func engagementSegmentGate(p domain.Perms, kind string) bool {
	if kind == domain.EngagementKindCustomer {
		return p.Alloc.Any(2, 3, 9)
	}
	return p.Alloc.Any(11, 12, 13, 19)
}

// engagementSegmentTypes are the engagement type ids of a list segment.
func engagementSegmentTypes(kind string) []int {
	switch kind {
	case domain.EngagementKindInternal:
		return []int{domain.EngagementTypeInternal}
	case domain.EngagementKindMarketing:
		return []int{domain.EngagementTypeMarketing}
	}
	return []int{domain.EngagementTypePaid, domain.EngagementTypeNonPaidPre, domain.EngagementTypeNonPaidPost,
		domain.EngagementTypePaidPostSale, domain.EngagementTypePaidPreSale}
}

// engagementCreatePrivilege is the create privilege per type: 2 for the
// customer types, 11 for an internal engagement, 12 for a marketing event.
func engagementCreatePrivilege(typeID int) int {
	switch typeID {
	case domain.EngagementTypeInternal:
		return 11
	case domain.EngagementTypeMarketing:
		return 12
	}
	return 2
}
