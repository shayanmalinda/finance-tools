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
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // hourly allocations compare times across IANA zones

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Engagement type ids (allocation_db.engagement_type).
const (
	allocTypeInternal      = 1
	allocTypePaid          = 2
	allocTypeNonPaidPre    = 3
	allocTypeNonPaidPost   = 4
	allocTypeMarketing     = 5
	allocTypePaidPostSale  = 6
	allocTypePaidPreSale   = 7
	allocDateLayout        = "2006-01-02"
	allocPastDaysAlloc     = 365 * 5 // ALLOWED_PAST_DAYS_ALLOC
	allocPastDaysDefault   = 30      // ALLOWED_PAST_DAYS_DEF_ALLOC
	allocOverheadDays      = 14      // PRE / POST_ENGAGEMENT_OVERHEAD_DAYS
	allocEOTypeName        = "Engagement Overhead"
	allocDefaultEngagement = "-1"
	allocBulkEngagement    = "-1.0"
	allocPartnerPrefix     = "partner-allocations-group"
	allocStatusActive      = "Active"
	allocStatusInactive    = "Inactive"
	allocRequestInProgress = "In Progress"
	allocRequestPending    = "Pending Approval"
	allocRequestCancelled  = "Cancelled"
)

// allocKind is the engagement kind of an engagement type: customer (2, 3,
// 4, 6, 7), internal (1) or marketing (5).
func allocKind(typeID int) string {
	switch typeID {
	case allocTypeInternal:
		return "internal"
	case allocTypeMarketing:
		return "marketing"
	}
	return "customer"
}

// allocKindTypes are the engagement type ids of each kind.
var allocKindTypes = map[string][]int{
	"customer":  {allocTypePaid, allocTypeNonPaidPre, allocTypeNonPaidPost, allocTypePaidPostSale, allocTypePaidPreSale},
	"internal":  {allocTypeInternal},
	"marketing": {allocTypeMarketing},
}

func allocIsInternalType(typeID int) bool {
	return typeID == allocTypeInternal || typeID == allocTypeMarketing
}

func allocIsPaidType(typeID int) bool {
	return typeID == allocTypePaid || typeID == allocTypePaidPostSale || typeID == allocTypePaidPreSale
}

// allocTypeRuleOK is the legacy isAllocateAllowed: post sale (4, 6) needs
// Allocations 32, pre sale (3, 7) 33, the retired Paid type (2) 6, internal
// (1) and Marketing Event (5) 16.
func allocTypeRuleOK(p domain.Perms, typeID int) bool {
	switch typeID {
	case allocTypeNonPaidPost, allocTypePaidPostSale:
		return p.Alloc.Has(32)
	case allocTypeNonPaidPre, allocTypePaidPreSale:
		return p.Alloc.Has(33)
	case allocTypePaid:
		return p.Alloc.Has(6)
	case allocTypeInternal, allocTypeMarketing:
		return p.Alloc.Has(16)
	}
	return false
}

// allocCanAllocate is canAllocateType: the Allocate page gate (6, 16 or 21)
// and the per-type rule. A pre-sale or post-sale group alone cannot
// allocate (legacy quirk).
func allocCanAllocate(p domain.Perms, typeID int) bool {
	return p.Alloc.Any(6, 16, 21) && allocTypeRuleOK(p, typeID)
}

// allocCanChangeStatus is canEditStatus: customer engagements use the
// allocate check; internal engagements and events need Allocations 20.
// Privilege 10 is never checked (legacy quirk).
func allocCanChangeStatus(p domain.Perms, typeID int) bool {
	if allocIsInternalType(typeID) {
		return p.Alloc.Has(20)
	}
	return allocCanAllocate(p, typeID)
}

func allocSeesTravel(p domain.Perms) bool {
	return p.Travel.Any(1, 2, 5) && !p.IsAgent()
}

// allocListTypes are the engagement types whose engagements the caller may
// open from the allocation lists: canSeeEng (customer: 3 or 6/32/33; internal and events: 13 or 16)
// and seesAllocList (customer: 6/32/33 or travel view; internal and events:
// 16 or travel view). Travel agents see none.
func allocListTypes(p domain.Perms) map[int]bool {
	out := map[int]bool{}
	if p.IsAgent() {
		return out
	}
	travel := allocSeesTravel(p)
	customer := p.Alloc.Any(6, 32, 33)
	if (p.Alloc.Has(3) || customer) && (customer || travel) {
		for _, t := range allocKindTypes["customer"] {
			out[t] = true
		}
	}
	if (p.Alloc.Has(13) || p.Alloc.Has(16)) && (p.Alloc.Has(16) || travel) {
		out[allocTypeInternal], out[allocTypeMarketing] = true, true
	}
	return out
}

// allocTransitions is the Update Allocation Status popup's NEXT table
// (screens-alloc.md §3a, legacy EngagementAllocations.js getStatusList).
var allocTransitions = map[string][]string{
	domain.AllocationClearanceTentative: {domain.AllocationClearanceReadyForClearance, domain.AllocationClearanceInProgress,
		domain.AllocationClearanceConfirmed, domain.AllocationClearanceConfirmedVisaPending, domain.AllocationClearanceAllocationCancelled},
	domain.AllocationClearanceReadyForClearance: {domain.AllocationClearanceInProgress, domain.AllocationClearanceConfirmed,
		domain.AllocationClearanceConfirmedVisaPending, domain.AllocationClearanceAllocationCancelled},
	domain.AllocationClearanceInProgress: {domain.AllocationClearanceAcceptedByConsultant, domain.AllocationClearanceRejectedByConsultant,
		domain.AllocationClearanceAllocationCancelled},
	domain.AllocationClearanceAcceptedByConsultant: {domain.AllocationClearanceConfirmed, domain.AllocationClearanceConfirmedVisaPending,
		domain.AllocationClearanceRejectedByConsultant, domain.AllocationClearanceAllocationCancelled},
	domain.AllocationClearanceConfirmed: {domain.AllocationClearanceRejectedByConsultant, domain.AllocationClearanceRejectedVisaIssues,
		domain.AllocationClearanceRejectedOther, domain.AllocationClearanceAllocationCancelled},
	domain.AllocationClearanceConfirmedVisaPending: {domain.AllocationClearanceConfirmed, domain.AllocationClearanceRejectedVisaIssues,
		domain.AllocationClearanceRejectedByConsultant, domain.AllocationClearanceAllocationCancelled},
}

// allocReopen is the default row: terminal (rejected, cancelled) or empty.
var allocReopen = []string{domain.AllocationClearanceInProgress, domain.AllocationClearanceConfirmed,
	domain.AllocationClearanceConfirmedVisaPending, domain.AllocationClearanceAllocationCancelled}

// AllocationNextStatuses returns the clearance statuses offered after current.
func AllocationNextStatuses(current string) []string {
	if next, ok := allocTransitions[current]; ok {
		return next
	}
	return allocReopen
}

// allocIsTerminal: Allocation Cancelled and every Rejected status (legacy
// TERMINAL_CLEARANCE_STATUSES); they save the allocation as Inactive.
func allocIsTerminal(c string) bool {
	switch c {
	case domain.AllocationClearanceAllocationCancelled, domain.AllocationClearanceRejectedByConsultant,
		domain.AllocationClearanceRejectedVisaIssues, domain.AllocationClearanceRejectedOther:
		return true
	}
	return false
}

func allocIsDefaultID(engagementID *string) bool {
	return engagementID == nil || *engagementID == "" || *engagementID == "-1" || *engagementID == "-1.0"
}

func allocIsPartner(email string) bool { return strings.Contains(email, allocPartnerPrefix) }

// allocPartnerName reads "First Last (Partner)" and the partner name back
// from a pseudo-email.
func allocPartnerName(email string) (name, partner *string) {
	if !allocIsPartner(email) {
		return nil, nil
	}
	local := strings.SplitN(email, "@", 2)[0]
	parts := strings.Split(local, "+")[1:]
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], "_", " ")
	}
	if len(parts) == 0 {
		return nil, nil
	}
	p := parts[0]
	n := strings.TrimSpace(strings.Join(parts[1:], " ")) + " (Partner)"
	return &n, &p
}

func allocStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func allocPtr(s string) *string { return &s }

func allocParseDate(s string) (time.Time, bool) {
	t, err := time.Parse(allocDateLayout, s)
	return t, err == nil
}

func allocAddDays(s string, n int) string {
	t, ok := allocParseDate(s)
	if !ok {
		return s
	}
	return t.AddDate(0, 0, n).Format(allocDateLayout)
}

func allocMaxDate(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func allocMinDate(a, b string) string {
	if a < b {
		return a
	}
	return b
}

// allocToday is the current date (YYYY-MM-DD) of now.
func allocToday(now time.Time) string { return now.Format(allocDateLayout) }

// allocNormTime returns a time of day (H:MM, HH:MM or HH:MM:SS) as HH:MM,
// so that times compare as strings; ok is false when s is not a time.
func allocNormTime(s string) (string, bool) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("15:04"), true
		}
	}
	return "", false
}

// allocBaseMinutes is a time of day in minutes after UTC midnight: legacy
// getBaseTime subtracts the zone's UTC offset. The offset is the zone's on
// the reference date.
func allocBaseMinutes(hhmm, zone, date string) int {
	t, err := time.Parse("15:04", allocTrimSeconds(hhmm))
	if err != nil {
		return 0
	}
	mins := t.Hour()*60 + t.Minute()
	if loc, err := time.LoadLocation(zone); err == nil && zone != "" {
		ref, ok := allocParseDate(date)
		if !ok {
			ref = time.Now()
		}
		_, off := time.Date(ref.Year(), ref.Month(), ref.Day(), 12, 0, 0, 0, loc).Zone()
		mins -= off / 60
	}
	return mins
}

func allocTrimSeconds(s string) string {
	if len(s) >= 5 && s[2] == ':' {
		return s[:5]
	}
	return s
}

// allocHourlyOf returns the hourly window of a stored allocation.
func allocHourlyOf(a engagemententity.AllocationRecord) *domain.AllocationHourly {
	if allocStr(a.IsRecurring) != "Yes" || a.StartTime == nil || a.EndTime == nil {
		return nil
	}
	return &domain.AllocationHourly{TimeZone: allocStr(a.TimeZone), StartTime: allocTrimSeconds(*a.StartTime),
		EndTime: allocTrimSeconds(*a.EndTime)}
}

// allocAvailability ports isPersonAvailable over existing allocations and
// returns the ones that block the new dates (the legacy stopped at the
// first). An existing allocation blocks when it covers the whole engagement
// span, or overlaps [start, end]; when hourlyAllowed, an existing hourly
// allocation only blocks a new hourly one whose window intersects it (times
// compared in UTC), and always blocks a new daily one.
func allocAvailability(existing []engagemententity.AllocationRecord, engStart, engEnd, start, end string,
	hourlyAllowed bool, newHourly *domain.AllocationHourly) []engagemententity.AllocationRecord {
	var out []engagemententity.AllocationRecord
	for _, a := range existing {
		covers := engStart != "" && a.StartDate <= engStart && engEnd <= a.EndDate
		overlaps := (a.StartDate <= start && start <= a.EndDate) || (a.StartDate >= start && a.StartDate <= end)
		if !covers && !overlaps {
			continue
		}
		if h := allocHourlyOf(a); h != nil && hourlyAllowed {
			if newHourly == nil {
				out = append(out, a)
				continue
			}
			aS := allocBaseMinutes(h.StartTime, h.TimeZone, start)
			aE := allocBaseMinutes(h.EndTime, h.TimeZone, start)
			nS := allocBaseMinutes(newHourly.StartTime, newHourly.TimeZone, start)
			nE := allocBaseMinutes(newHourly.EndTime, newHourly.TimeZone, start)
			// Half-open windows: back-to-back slots do not intersect.
			if aS < nE && nS < aE {
				out = append(out, a)
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// allocDefaultOverlaps ports getOverlaps: default allocations touching
// [start, end].
func allocDefaultOverlaps(defaults []engagemententity.AllocationRecord, start, end string) []engagemententity.AllocationRecord {
	var out []engagemententity.AllocationRecord
	for _, a := range defaults {
		if !(start < a.StartDate && end < a.StartDate) && !(start > a.EndDate) {
			out = append(out, a)
		}
	}
	return out
}

// allocTimelineItem builds a timeline bar from a stored allocation.
func allocTimelineItem(a engagemententity.AllocationRecord, today string) domain.AllocationTimelineItem {
	it := domain.AllocationTimelineItem{
		ID: a.ID, Kind: "default", AllocationType: allocStr(a.AllocationTypeName), Color: a.AllocationTypeColor,
		StartDate: a.StartDate, EndDate: a.EndDate, Hourly: allocHourlyOf(a),
		ClearanceStatus: allocStr(a.ClearanceStatus), Comment: a.Comment,
	}
	code := allocStr(a.EngagementCode)
	if a.Engagement != nil && !allocIsDefaultID(a.EngagementID) {
		it.Kind = "engagement"
		it.EngagementID = &a.Engagement.ID
		if code == "" {
			code = allocStr(a.Engagement.EngagementCode)
		}
		it.Label = code + ": " + allocStr(a.Engagement.Title)
	} else {
		it.Label = code + ": " + it.AllocationType
		it.Editable = a.StartDate >= allocAddDays(today, -allocPastDaysDefault)
	}
	it.EngagementCode = code
	return it
}

func allocTimelineItems(rows []engagemententity.AllocationRecord, today string) []domain.AllocationTimelineItem {
	out := make([]domain.AllocationTimelineItem, 0, len(rows))
	for _, a := range rows {
		out = append(out, allocTimelineItem(a, today))
	}
	return out
}

func allocSortedInts(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
