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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// Legacy message texts of the Allocate page (AllocationsBy.js).
const (
	allocMsgRequired       = "You need to fill the required fields"
	allocMsgPastEngagement = "You cannot allocate for past engagements"
	allocMsgSelectEmail    = "Please select an email"
	allocMsgInvalidDates   = "Dates / Time cannot be invalid"
	allocMsgOutOfRange     = "Start Date / End Date is not within the range"
	allocMsgEndBeforeStart = "End Date should be higher than Start Date"
	allocMsgEndTime        = "End Time should be higher than Start Time"
	allocMsgPrepOrder      = "Pre-Prep Start Date must be less than the allocation Start Date"
	allocMsgPostOrder      = "Post-Prep End Date must be greater than the allocation End Date"
	allocMsgPrepRange      = "Pre-Prep Start Date is not within the range"
	allocMsgPostRange      = "Post-Prep End Date is not within the range"
	allocMsgRefresh        = " Please refresh the timeline if you do not have the latest / cannot see it in the timeline."
	allocMsgBusy           = "This person is already allocated for the selected period of time."
	allocMsgBusyPrep       = "This person is already allocated for the selected period of preparation overhead."
	allocMsgBusyPost       = "This person is already allocated for the selected period of post overhead."
	allocMsgFullOverlay    = "Operation is not permitted! New allocation is fully overlaying an existing default allocation. Update / delete default allocation from Team Allocations page."
	allocMsgDefaultOverlap = "These operation(s) overlaps with existing default allocation(s) (please refresh the timeline if you do not have the latest / cannot see it in the timeline). Confirming this action will result in updating the existing default allocation(s) for user. Are you sure you want to add this allocation ?"
	allocMsgTypeDenied     = "Access Denied: You do not have sufficient privileges to allocate consultants for this engagement type."
	allocMsgCommentLength  = "The comment can have at most 255 characters."
)

// AllocationDeps are what the allocation services use.
type AllocationDeps struct {
	Entity     engagemententity.Client
	HR         hrentity.Client
	Org        hrentity.OrgClient
	Email      email.Sender
	Recipients config.EmailConfig
	Now        func() time.Time
}

// AllocationService is the Allocate page, the Engagement Allocations list,
// the allocation status popup and the people timelines.
type AllocationService interface {
	Create(ctx context.Context, p domain.Perms, engagementID string, req domain.AllocationCreateRequest) (domain.AllocationCreated, error)
	List(ctx context.Context, p domain.Perms, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error)
	UpdateStatus(ctx context.Context, p domain.Perms, id string, req domain.AllocationStatusUpdate) (domain.AllocationCreated, error)
	// ListMine is GET /me/allocations: the caller's own allocations, any engagement type.
	ListMine(ctx context.Context, p domain.Perms, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error)
	Timeline(ctx context.Context, f domain.AllocationTimelineFilter) (domain.AllocationTimelineResponse, error)
}

type allocationService struct {
	d      AllocationDeps
	people *travelPeople
}

// NewAllocationService constructs an AllocationService.
func NewAllocationService(d AllocationDeps) AllocationService {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &allocationService{d: d}
	if d.HR != nil {
		s.people = newTravelPeople(d.HR)
	}
	return s
}

func allocBad(msg string) error          { return &apierror.ValidationError{Msg: msg} }
func allocBadf(f string, a ...any) error { return &apierror.ValidationError{Msg: fmt.Sprintf(f, a...)} }

// allocUpstreamNotFound reports whether err is the entity's 404.
func allocUpstreamNotFound(err error) bool {
	var up *apierror.UpstreamError
	return errors.As(err, &up) && up.StatusCode == http.StatusNotFound
}

// ── Allocate ────────────────────────────────────────────────────────────────

// Create ports the Allocate page's Add New Allocation (AllocationsBy.js
// isAllocationFormValid, _validateAndSubmitAllocations,
// createAndSubmitAllAllocations) and the backend's POST allocations/bulk.
func (s *allocationService) Create(ctx context.Context, p domain.Perms, engagementID string, req domain.AllocationCreateRequest) (domain.AllocationCreated, error) {
	var none domain.AllocationCreated
	eng, err := s.d.Entity.AllocationEngagement(ctx, engagementID)
	if allocUpstreamNotFound(err) {
		return none, &apierror.NotFoundError{Msg: "Engagement " + engagementID + " was not found."}
	}
	if err != nil {
		return none, err
	}
	typeID := eng.EngagementTypeID
	if !allocCanAllocate(p, typeID) {
		return none, &apierror.ForbiddenError{Msg: allocMsgTypeDenied}
	}
	switch {
	case allocStr(eng.RequestStatus) == allocRequestPending:
		return none, allocBad("This engagement is waiting for approval. People can be allocated once it is approved.")
	case allocStr(eng.RequestStatus) == allocRequestCancelled || allocStr(eng.EngagementStatus) == allocRequestCancelled:
		return none, allocBad("This engagement is cancelled.")
	}
	engStart, engEnd := allocStr(eng.StartDate), allocStr(eng.EndDate)
	if _, ok := allocParseDate(engStart); !ok {
		return none, allocBad("This engagement has no start and end dates.")
	}
	if _, ok := allocParseDate(engEnd); !ok {
		return none, allocBad("This engagement has no start and end dates.")
	}
	today := allocToday(s.d.Now())
	pastLimit := allocAddDays(today, -allocPastDaysAlloc)
	if engStart < pastLimit && engEnd < pastLimit {
		return none, allocBad(allocMsgPastEngagement)
	}

	consultant, isPartner, err := s.consultantOf(ctx, req)
	if err != nil {
		return none, err
	}
	in, err := s.validateCreate(eng, req, isPartner, today)
	if err != nil {
		return none, err
	}

	// Availability over the person's Active allocations (legacy fetched the
	// timeline window; this reads every allocation touching the engagement,
	// the new dates and the overhead dates).
	from := allocMinDate(engStart, in.start)
	to := allocMaxDate(engEnd, in.end)
	if in.prep != "" {
		from = allocMinDate(from, in.prep)
	}
	if in.post != "" {
		to = allocMaxDate(to, in.post)
	}
	existing, err := s.d.Entity.AllocationTimeline(ctx, engagemententity.AllocationTimelineRequest{
		Emails: []string{consultant}, StartDate: from, EndDate: to})
	if err != nil {
		return none, err
	}
	var engagementAllocs, defaults []engagemententity.AllocationRecord
	for _, a := range existing {
		if allocIsDefaultID(a.EngagementID) {
			defaults = append(defaults, a)
		} else if !allocIsTerminal(allocStr(a.ClearanceStatus)) {
			engagementAllocs = append(engagementAllocs, a)
		}
	}
	if busy := allocAvailability(engagementAllocs, engStart, engEnd, in.start, in.end, true, in.hourly); len(busy) > 0 {
		return none, s.overlapError(allocMsgBusy+allocMsgRefresh, busy, nil, today)
	}
	if in.prep != "" {
		if busy := allocAvailability(engagementAllocs, engStart, engEnd, in.prep, allocAddDays(in.start, -1), false, nil); len(busy) > 0 {
			return none, s.overlapError(allocMsgBusyPrep+allocMsgRefresh, busy, nil, today)
		}
	}
	if in.post != "" {
		if busy := allocAvailability(engagementAllocs, engStart, engEnd, allocAddDays(in.end, 1), in.post, false, nil); len(busy) > 0 {
			return none, s.overlapError(allocMsgBusyPost+allocMsgRefresh, busy, nil, today)
		}
	}

	// Default (team) allocations over [preparation start or start, post end or end].
	span := [2]string{in.start, in.end}
	if in.prep != "" {
		span[0] = in.prep
	}
	if in.post != "" {
		span[1] = in.post
	}
	overlapped := allocDefaultOverlaps(defaults, span[0], span[1])
	var updates []engagemententity.AllocationDateUpdate
	var splits []engagemententity.AllocationInput
	if len(overlapped) > 0 {
		for _, a := range overlapped {
			if span[0] <= a.StartDate && span[1] >= a.EndDate {
				return none, s.overlapError(allocMsgFullOverlay, nil, []engagemententity.AllocationRecord{a}, today)
			}
		}
		if req.TrimDefaultAllocations == nil || !*req.TrimDefaultAllocations {
			return none, s.overlapError(allocMsgDefaultOverlap, nil, overlapped, today)
		}
		updates, splits = allocTrimDefaults(overlapped, span[0], span[1])
	}

	clearance := in.clearance
	travel := "No"
	if *req.TravelRequired {
		travel = "Yes"
	}
	main := engagemententity.AllocationInput{
		Email: consultant, AllocationTypeID: allocIntOr(eng.AllocationTypeID), Status: allocStatusActive,
		AllocationStatus: allocPtr(allocStatusActive), ClearanceStatus: &clearance, StartDate: in.start, EndDate: in.end,
		EngagementID: eng.ID, EngagementCode: allocStr(eng.EngagementCode), IsTravelRequired: travel,
		IsRecurring: "No", Comment: req.Comment,
	}
	if in.hourly != nil {
		main.IsRecurring = "Yes"
		main.StartTime, main.EndTime, main.TimeZone = &in.hourly.StartTime, &in.hourly.EndTime, &in.hourly.TimeZone
	}
	news := []engagemententity.AllocationInput{main}
	if in.prep != "" || in.post != "" {
		eo, err := s.overheadType(ctx)
		if err != nil {
			return none, err
		}
		suffix := allocStr(eo.EngagementCodeSuffix)
		if suffix == "" {
			suffix = "EO"
		}
		code := allocStr(eng.EngagementCode) + "-" + suffix
		mk := func(start, end string) engagemententity.AllocationInput {
			return engagemententity.AllocationInput{Email: consultant, AllocationTypeID: eo.ID, Status: allocStatusActive,
				AllocationStatus: allocPtr(allocStatusActive), ClearanceStatus: allocPtr(domain.AllocationClearanceConfirmed),
				StartDate: start, EndDate: end, EngagementID: allocBulkEngagement, EngagementCode: code,
				IsTravelRequired: "No", IsRecurring: "No", Comment: allocPtr("")}
		}
		if in.prep != "" {
			news = append(news, mk(in.prep, allocAddDays(in.start, -1)))
		}
		if in.post != "" {
			news = append(news, mk(allocAddDays(in.end, 1), in.post))
		}
	}
	news = append(news, splits...)

	bulk := engagemententity.AllocationBulkRequest{NewAllocations: news, UpdateAllocations: updates, CreatedBy: p.Email}
	if rs := allocStr(eng.RequestStatus); rs != "" && rs != allocRequestInProgress {
		bulk.RequestStatusInProgress = &eng.ID // legacy: the page's follow-up PATCH {status: "In Progress"}
	}
	res, err := s.d.Entity.BulkAllocations(ctx, bulk)
	if err != nil {
		return none, err
	}

	out := domain.AllocationCreated{TravelRequest: res.TravelJobNumber}
	saved := engagemententity.AllocationRecord{
		ID: allocStr(res.FirstAllocationID), Email: consultant, StartDate: in.start, EndDate: in.end,
		ClearanceStatus: &clearance, IsTravelRequired: &travel, Comment: req.Comment, AddedBy: &p.Email,
		EngagementID: &eng.ID, IsRecurring: &main.IsRecurring, StartTime: main.StartTime, EndTime: main.EndTime, TimeZone: main.TimeZone,
	}
	if res.FirstAllocationID != nil {
		if rec, err := s.d.Entity.Allocation(ctx, *res.FirstAllocationID); err == nil {
			saved = rec
		} else {
			slog.WarnContext(ctx, "allocation saved but not read back", "allocation", *res.FirstAllocationID, "err", err)
		}
	}
	out.Allocation = s.toAllocation(p, saved, typeID, res.TravelJobNumber)

	if email.AllocationMailInitiated(clearance, "") {
		out.EmailsSent = s.sendAllocationEmail(ctx, eng, saved, clearance, "", p.Email)
	}
	return out, nil
}

// allocCreateInput is the validated form.
type allocCreateInput struct {
	start, end, prep, post, clearance string
	// hourly is the request's window with its times as HH:MM; nil for a
	// daily allocation.
	hourly *domain.AllocationHourly
}

// validateCreate ports isAllocationFormValid (legacy order and texts).
func (s *allocationService) validateCreate(eng engagemententity.AllocationEngagement, req domain.AllocationCreateRequest, isPartner bool, today string) (allocCreateInput, error) {
	var in allocCreateInput
	typeID := eng.EngagementTypeID
	if req.StartDate == nil || req.EndDate == nil || req.TravelRequired == nil ||
		*req.StartDate == "" || *req.EndDate == "" {
		return in, allocBad(allocMsgRequired)
	}
	if req.Hourly != nil && (req.Hourly.TimeZone == "" || req.Hourly.StartTime == "" || req.Hourly.EndTime == "") {
		return in, allocBad(allocMsgRequired)
	}
	if (req.PreparationStart != nil || req.PostEngagementEnd != nil) && (!allocIsPaidType(typeID) || isPartner) {
		return in, allocBad("Preparation and post engagement overheads are only for paid engagements.")
	}
	if req.PreparationStart != nil && *req.PreparationStart == "" || req.PostEngagementEnd != nil && *req.PostEngagementEnd == "" {
		return in, allocBad(allocMsgRequired)
	}
	switch {
	case isPartner:
		in.clearance = domain.AllocationClearanceConfirmed
	case allocIsInternalType(typeID):
		in.clearance = domain.AllocationClearanceTentative
		if req.ClearanceStatus != nil {
			in.clearance = *req.ClearanceStatus
		}
		switch in.clearance {
		case domain.AllocationClearanceTentative, domain.AllocationClearanceInProgress,
			domain.AllocationClearanceConfirmed, domain.AllocationClearanceConfirmedVisaPending:
		default:
			return in, allocBad("Clearance Status must be Tentative, Clearance In Progress, Confirmed or Confirmed - Visa Pending.")
		}
	case req.Tentative != nil && *req.Tentative:
		in.clearance = domain.AllocationClearanceTentative
	default:
		in.clearance = domain.AllocationClearanceReadyForClearance
	}
	if req.Comment != nil && len([]rune(*req.Comment)) > 255 {
		return in, allocBad(allocMsgCommentLength)
	}

	in.start, in.end = *req.StartDate, *req.EndDate
	in.prep, in.post = allocStr(req.PreparationStart), allocStr(req.PostEngagementEnd)
	for _, d := range []string{in.start, in.end, in.prep, in.post} {
		if _, ok := allocParseDate(d); d != "" && !ok {
			return in, allocBad(allocMsgInvalidDates)
		}
	}
	if req.Hourly != nil {
		startTime, okStart := allocNormTime(req.Hourly.StartTime)
		endTime, okEnd := allocNormTime(req.Hourly.EndTime)
		// Only the zones the Allocate page offers (and UTC) are stored: any
		// other name, such as "Local" or "localtime", could load from one
		// replica's own zone files and not from another's.
		_, err := time.LoadLocation(req.Hourly.TimeZone)
		if !okStart || !okEnd || err != nil || !allocKnownZone(req.Hourly.TimeZone) {
			return in, allocBad(allocMsgInvalidDates)
		}
		in.hourly = &domain.AllocationHourly{TimeZone: req.Hourly.TimeZone, StartTime: startTime, EndTime: endTime}
	}
	pastLimit := allocAddDays(today, -allocPastDaysAlloc)
	if in.start < pastLimit || in.end < pastLimit || (in.prep != "" && in.prep < pastLimit) || (in.post != "" && in.post < pastLimit) {
		return in, allocBadf("Dates cannot be less than  %d days from today", allocPastDaysAlloc)
	}
	engStart, engEnd := allocStr(eng.StartDate), allocStr(eng.EndDate)
	minDate := allocMaxDate(engStart, pastLimit)
	if in.start < minDate || in.start > engEnd || in.end < minDate || in.end > engEnd {
		return in, allocBad(allocMsgOutOfRange)
	}
	if in.start > in.end {
		return in, allocBad(allocMsgEndBeforeStart)
	}
	if in.hourly != nil && in.hourly.StartTime >= in.hourly.EndTime {
		return in, allocBad(allocMsgEndTime)
	}
	if in.prep != "" && in.prep >= in.start {
		return in, allocBad(allocMsgPrepOrder)
	}
	if in.post != "" && in.post <= in.end {
		return in, allocBad(allocMsgPostOrder)
	}
	if in.prep != "" && in.prep < allocMaxDate(allocAddDays(engStart, -allocOverheadDays), pastLimit) {
		return in, allocBad(allocMsgPrepRange)
	}
	if in.post != "" && in.post > allocAddDays(engEnd, allocOverheadDays) {
		return in, allocBad(allocMsgPostRange)
	}
	return in, nil
}

// consultantOf resolves the consultant's email: an employee known to HR.
func (s *allocationService) consultantOf(ctx context.Context, req domain.AllocationCreateRequest) (string, bool, error) {
	hasEmail := req.ConsultantEmail != nil && strings.TrimSpace(*req.ConsultantEmail) != ""
	switch {
	case hasEmail:
		e := strings.ToLower(strings.TrimSpace(*req.ConsultantEmail))
		if allocIsPartner(e) {
			return "", false, allocBad(allocMsgSelectEmail)
		}
		emp, err := s.d.HR.Employee(ctx, e)
		if err != nil {
			return "", false, err
		}
		if emp == nil {
			return "", false, allocBad(allocMsgSelectEmail)
		}
		return e, false, nil
	}
	return "", false, allocBad(allocMsgSelectEmail)
}

// overheadType finds the "Engagement Overhead" allocation type by name, as
// the legacy page did.
func (s *allocationService) overheadType(ctx context.Context) (engagemententity.AllocationType, error) {
	types, err := s.d.Entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{})
	if err != nil {
		return engagemententity.AllocationType{}, err
	}
	for _, t := range types {
		if t.AllocationTypeName == allocEOTypeName {
			return t, nil
		}
	}
	return engagemententity.AllocationType{}, errors.New("the Engagement Overhead allocation type is missing")
}

// allocTrimDefaults ports getAllocationObjectsForOverlapping (after the
// full-overlay check): default allocations around the new range are split,
// those starting inside it move their start after it, those starting
// before it end the day before it.
func allocTrimDefaults(overlapped []engagemententity.AllocationRecord, start, end string) ([]engagemententity.AllocationDateUpdate, []engagemententity.AllocationInput) {
	var updates []engagemententity.AllocationDateUpdate
	var news []engagemententity.AllocationInput
	for _, a := range overlapped {
		switch {
		case a.StartDate < start && end < a.EndDate:
			updates = append(updates, engagemententity.AllocationDateUpdate{ID: a.ID, StartDate: a.StartDate, EndDate: allocAddDays(start, -1)})
			clearance := allocStr(a.ClearanceStatus)
			if clearance == "" {
				clearance = domain.AllocationClearanceConfirmed
			}
			travel := "No"
			if allocStr(a.IsTravelRequired) == "Yes" {
				travel = "Yes"
			}
			comment := allocStr(a.Comment)
			news = append(news, engagemententity.AllocationInput{
				Email: a.Email, AllocationTypeID: allocIntOr(a.AllocationTypeID), Status: allocStatusActive,
				AllocationStatus: allocPtr(allocStatusActive), ClearanceStatus: &clearance,
				StartDate: allocAddDays(end, 1), EndDate: a.EndDate, EngagementID: allocStr(a.EngagementID),
				EngagementCode: allocStr(a.EngagementCode), IsTravelRequired: travel, IsRecurring: "No", Comment: &comment,
			})
		case a.StartDate >= start:
			updates = append(updates, engagemententity.AllocationDateUpdate{ID: a.ID, StartDate: allocAddDays(end, 1), EndDate: a.EndDate})
		default:
			updates = append(updates, engagemententity.AllocationDateUpdate{ID: a.ID, StartDate: a.StartDate, EndDate: allocAddDays(start, -1)})
		}
	}
	return updates, news
}

func allocIntOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func (s *allocationService) overlapError(msg string, busy, defaults []engagemententity.AllocationRecord, today string) error {
	return &domain.AllocationOverlapError{Code: http.StatusConflict, Message: msg,
		Overlaps: allocNilIfEmpty(allocTimelineItems(busy, today)), DefaultOverlaps: allocNilIfEmpty(allocTimelineItems(defaults, today))}
}

func allocNilIfEmpty(items []domain.AllocationTimelineItem) []domain.AllocationTimelineItem {
	if len(items) == 0 {
		return nil
	}
	return items
}

// ── Emails ──────────────────────────────────────────────────────────────────

// sendAllocationEmail sends the legacy allocation email for a change to
// clearance (from prev). Failures are logged; it reports whether an email
// went out.
func (s *allocationService) sendAllocationEmail(ctx context.Context, eng engagemententity.AllocationEngagement,
	a engagemententity.AllocationRecord, clearance, prev, actor string) bool {
	d := email.AllocationEmailData{
		EngagementTypeID: eng.EngagementTypeID, EngagementID: eng.ID, CustomerName: allocStr(eng.CustomerName),
		OpportunityName: allocStr(eng.Title), ProductName: allocStr(eng.ProductName),
		AllocationTypeName: allocStr(eng.AllocationTypeName), EngagementCode: allocStr(eng.EngagementCode),
		EngagementNature: allocStr(eng.EngagementNature), Country: allocStr(eng.Country), Address: allocStr(eng.Address),
		CustomerContact: allocStr(eng.CustomerContact), BusinessDomain: allocStr(eng.BusinessDomain),
		OpportunityOwner: allocStr(eng.OpportunityOwner), TechnicalOwner: allocStr(eng.TechnicalOwner),
		ContractsSigned: eng.ContractsSigned, EngagementScope: allocStr(eng.EngagementScope),
		EngagementComment: allocStr(eng.Comment), ConsultantEmail: a.Email, ClearanceStatus: clearance,
		PrevClearanceStatus: prev, TravelRequired: allocStr(a.IsTravelRequired) == "Yes",
		StartDate: a.StartDate, EndDate: a.EndDate, AllocationComment: allocStr(a.Comment), Actor: actor, Now: s.d.Now(),
	}
	for _, f := range eng.FundingSources {
		d.FundingSources = append(d.FundingSources, email.AllocationEmailFunding{
			BusinessUnit: allocStr(f.BusinessUnit), ProductUnit: allocStr(f.ProductUnit), SubRegion: allocStr(f.SubRegion)})
	}
	if !email.AllocationIsPartnerEmail(a.Email) {
		d.Leads = s.leadsOf(ctx, a.Email)
		if email.AllocationIsConfirmation(clearance, prev) {
			visas, err := s.d.Entity.AllocationVisas(ctx, a.Email, a.StartDate, a.EndDate)
			if err != nil {
				// Legacy: a visa lookup error aborts the email.
				slog.ErrorContext(ctx, "allocation email not sent: visa lookup failed", "allocation", a.ID, "err", err)
				return false
			}
			for _, v := range visas {
				d.Visas = append(d.Visas, email.AllocationEmailVisa{Country: allocStr(v.Country),
					VisaType: allocStr(v.VisaType), EntryType: allocStr(v.EntryType), Status: allocStr(v.Status),
					StartDate: allocStr(v.StartDate), EndDate: allocStr(v.EndDate)})
			}
		}
	}
	msg := email.AllocationMessage(d, email.AllocationEmailGroups{
		AllocationsGroup: s.d.Recipients.AllocationsGroup, PartnerAllocationsGroup: s.d.Recipients.PartnerAllocationsGroup,
		TravelGroup: s.d.Recipients.TravelGroup, AppBaseURL: s.d.Recipients.AppBaseURL})
	if err := s.d.Email.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "allocation email not sent", "allocation", a.ID, "err", err)
		return false
	}
	return true
}

// leadsOf ports getReportsToEmails: the employee's reporting lead(s) from HR
// (reportsToEmail, comma-separated); none when unknown.
func (s *allocationService) leadsOf(ctx context.Context, employee string) []string {
	if s.d.Org == nil {
		return nil
	}
	rec, err := s.d.Org.EmployeeRecord(ctx, employee)
	if err != nil || rec == nil || strings.TrimSpace(allocStr(rec.ReportsToEmail)) == "" {
		slog.WarnContext(ctx, "could not resolve the reporting lead, notifying the remaining recipients", "employee", employee, "err", err)
		return nil
	}
	return strings.Split(*rec.ReportsToEmail, ",")
}

// ── Allocations list ────────────────────────────────────────────────────────

// List is the Engagement Allocations list (legacy allocation-search with the
// Release 1 filters): every engagement type, as legacy (row actions still
// follow the caller's privileges).
func (s *allocationService) List(ctx context.Context, p domain.Perms, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
	return s.list(ctx, p, f, allocAllTypes())
}

func (s *allocationService) ListMine(ctx context.Context, p domain.Perms, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
	f.Consultants = []string{p.Email}
	return s.list(ctx, p, f, allocAllTypes())
}

func allocAllTypes() map[int]bool {
	all := map[int]bool{}
	for _, types := range allocKindTypes {
		for _, t := range types {
			all[t] = true
		}
	}
	return all
}

func (s *allocationService) list(ctx context.Context, p domain.Perms, f domain.AllocationListFilter, allowed map[int]bool) (domain.Page[domain.AllocationWithEngagement], error) {
	page := domain.Page[domain.AllocationWithEngagement]{Items: []domain.AllocationWithEngagement{}}
	visible := allocListTypes(p)
	if len(f.Kinds) > 0 {
		kinds := map[int]bool{}
		for _, k := range f.Kinds {
			types, ok := allocKindTypes[k]
			if !ok {
				return page, allocBadf("kind must be customer, internal or marketing.")
			}
			for _, t := range types {
				kinds[t] = true
			}
		}
		for t := range allowed {
			if !kinds[t] {
				delete(allowed, t)
			}
		}
	}
	if len(f.EngagementTypeIDs) > 0 {
		want := map[int]bool{}
		for _, t := range f.EngagementTypeIDs {
			want[t] = true
		}
		for t := range allowed {
			if !want[t] {
				delete(allowed, t)
			}
		}
	}
	for _, c := range f.ClearanceStatuses {
		if !allocValidClearance(c) {
			return page, allocBadf("Unknown clearance status %q.", c)
		}
	}
	if len(allowed) == 0 {
		return page, nil
	}
	res, err := s.d.Entity.SearchAllocations(ctx, engagemententity.AllocationSearchRequest{
		EngagementTypeIDs: allocSortedInts(allowed), ClearanceStatuses: f.ClearanceStatuses, Consultants: f.Consultants,
		AllocationTypes: f.AllocationTypes, TravelRequired: f.TravelRequired, StartFrom: f.StartFrom, EndTo: f.EndTo,
		Search: f.Search, Today: allocToday(s.d.Now()), Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return page, err
	}
	page.Total = res.Total
	for _, a := range res.Items {
		if a.Engagement == nil {
			continue
		}
		e := a.Engagement
		row := domain.AllocationWithEngagement{
			Allocation: s.toAllocation(p, a, e.EngagementTypeID, nil),
			Engagement: domain.AllocationEngagementSummary{
				ID: e.ID, Kind: allocKind(e.EngagementTypeID), EngagementTypeID: e.EngagementTypeID, Title: allocStr(e.Title),
				Account: e.CustomerName, AllocationType: allocStr(e.AllocationTypeName), EngagementCode: allocStr(e.EngagementCode),
				Country: e.Country, Address: e.Address, CanOpen: visible[e.EngagementTypeID],
			},
		}
		page.Items = append(page.Items, row)
	}
	people := make([]*domain.Person, 0, len(page.Items))
	for i := range page.Items {
		people = append(people, &page.Items[i].Consultant)
	}
	s.people.fillNames(ctx, people...)
	return page, nil
}

func allocValidClearance(c string) bool {
	if c == domain.AllocationClearanceTentative {
		return true
	}
	for _, next := range allocTransitions {
		for _, n := range next {
			if n == c {
				return true
			}
		}
	}
	return false
}

// toAllocation maps a stored engagement allocation to the API shape.
func (s *allocationService) toAllocation(p domain.Perms, a engagemententity.AllocationRecord, typeID int, newJob *string) domain.Allocation {
	out := domain.Allocation{
		ID: a.ID, Consultant: domain.Person{Email: a.Email}, StartDate: a.StartDate, EndDate: a.EndDate,
		Hourly: allocHourlyOf(a), TravelRequired: allocStr(a.IsTravelRequired) == "Yes",
		ClearanceStatus: a.ClearanceStatus, Comment: a.Comment, AddedBy: a.AddedBy, AddedOn: a.AddedDate,
		Actions: domain.AllocationActions{ChangeStatus: allocCanChangeStatus(p, typeID)},
	}
	if a.Engagement != nil {
		out.EngagementID = a.Engagement.ID
	} else if a.EngagementID != nil {
		out.EngagementID = *a.EngagementID
	}
	out.Consultant.Name, out.Partner = allocPartnerName(a.Email)
	// The trip shown is the one just created, else the latest open one,
	// else the latest (rows come ordered by job number).
	job, latest := "", ""
	for _, t := range a.TravelRequests {
		latest = t.JobNumber
		if allocStr(t.Status) != "Cancelled" {
			job = t.JobNumber
		}
	}
	if job == "" {
		job = latest
	}
	if newJob != nil {
		job = *newJob
	}
	if job != "" {
		out.TravelRequest = &domain.AllocationTravelRequest{JobNumber: job, CanOpen: allocSeesTravel(p)}
	}
	return out
}

// ── Status ──────────────────────────────────────────────────────────────────

// UpdateStatus is the Update Allocation Status popup (legacy PATCH
// allocations/{id} with clearanceStatus, comment and status Inactive for a
// terminal status).
func (s *allocationService) UpdateStatus(ctx context.Context, p domain.Perms, id string, req domain.AllocationStatusUpdate) (domain.AllocationCreated, error) {
	var none domain.AllocationCreated
	a, err := s.d.Entity.Allocation(ctx, id)
	if allocUpstreamNotFound(err) || (err == nil && (a.Engagement == nil || allocIsDefaultID(a.EngagementID))) {
		return none, &apierror.NotFoundError{Msg: "Allocation " + id + " was not found."}
	}
	if err != nil {
		return none, err
	}
	typeID := a.Engagement.EngagementTypeID
	if !allocCanChangeStatus(p, typeID) {
		return none, &apierror.ForbiddenError{Msg: "You do not have permission to perform this action."}
	}
	if req.ClearanceStatus == nil || *req.ClearanceStatus == "" {
		return none, allocBad(allocMsgRequired)
	}
	next, current := *req.ClearanceStatus, allocStr(a.ClearanceStatus)
	allowedNext := false
	for _, n := range AllocationNextStatuses(current) {
		allowedNext = allowedNext || n == next
	}
	if !allowedNext {
		return none, allocBadf("An allocation in %q cannot move to %q.", current, next)
	}
	if req.Comment != nil && len([]rune(*req.Comment)) > 255 {
		return none, allocBad(allocMsgCommentLength)
	}
	status := allocStatusActive
	if allocIsTerminal(next) {
		status = allocStatusInactive
	} else {
		// Cancelling or rejecting never runs the availability check
		// (LEGACY-ISSUES L5); other statuses check the person's other active
		// engagement allocations as the Allocate page did.
		if err := s.checkStatusAvailability(ctx, a); err != nil {
			return none, err
		}
	}
	res, err := s.d.Entity.UpdateAllocation(ctx, id, engagemententity.AllocationUpdateRequest{
		ClearanceStatus: &next, Status: &status, Comment: req.Comment, UpdatedBy: p.Email})
	if err != nil {
		return none, err
	}
	out := domain.AllocationCreated{Allocation: s.toAllocation(p, res.Allocation, typeID, res.TravelJobNumber), TravelRequest: res.TravelJobNumber}
	prev := allocStr(res.PreviousClearanceStatus)
	if prev == "" {
		// The entity did not echo the previous status; the one read for this
		// request is what it was.
		prev = current
	}
	if email.AllocationMailInitiated(next, prev) {
		eng, err := s.d.Entity.AllocationEngagement(ctx, a.Engagement.ID)
		if err != nil {
			slog.ErrorContext(ctx, "allocation email not sent: engagement not read", "allocation", id, "err", err)
		} else {
			out.EmailsSent = s.sendAllocationEmail(ctx, eng, res.Allocation, next, prev, p.Email)
		}
	}
	return out, nil
}

func (s *allocationService) checkStatusAvailability(ctx context.Context, a engagemententity.AllocationRecord) error {
	engStart, engEnd := allocStr(a.Engagement.StartDate), allocStr(a.Engagement.EndDate)
	from, to := a.StartDate, a.EndDate
	if engStart != "" && engEnd != "" {
		from, to = allocMinDate(from, engStart), allocMaxDate(to, engEnd)
	}
	existing, err := s.d.Entity.AllocationTimeline(ctx, engagemententity.AllocationTimelineRequest{
		Emails: []string{a.Email}, StartDate: from, EndDate: to})
	if err != nil {
		return err
	}
	var others []engagemententity.AllocationRecord
	for _, o := range existing {
		if o.ID != a.ID && !allocIsDefaultID(o.EngagementID) && !allocIsTerminal(allocStr(o.ClearanceStatus)) {
			others = append(others, o)
		}
	}
	if busy := allocAvailability(others, engStart, engEnd, a.StartDate, a.EndDate, true, allocHourlyOf(a)); len(busy) > 0 {
		return s.overlapError(allocMsgBusy+allocMsgRefresh, busy, nil, allocToday(s.d.Now()))
	}
	return nil
}

// ── Timelines ───────────────────────────────────────────────────────────────

// Timeline returns people and their allocations over a window: the people
// named by email, or the employees of an org unit (Active and Marked Leaver,
// no interns, as the legacy GET employees).
func (s *allocationService) Timeline(ctx context.Context, f domain.AllocationTimelineFilter) (domain.AllocationTimelineResponse, error) {
	resp := domain.AllocationTimelineResponse{People: []domain.AllocationTimelineRow{}}
	if _, ok := allocParseDate(f.From); !ok {
		return resp, allocBad("from and to must be dates (YYYY-MM-DD).")
	}
	if _, ok := allocParseDate(f.To); !ok || f.To < f.From {
		return resp, allocBad("from and to must be dates (YYYY-MM-DD), from on or before to.")
	}
	org := f.BusinessUnit != "" || f.Department != "" || f.Team != ""
	var people []domain.AllocationEmployee
	switch {
	case org:
		if s.d.Org == nil {
			return resp, &apierror.UpstreamError{Service: "hr-entity", Msg: "not configured"}
		}
		emps, err := s.d.Org.EmployeesByOrg(ctx, hrentity.OrgEmployeeFilter{
			Statuses: []string{"Active", "Marked Leaver"}, ExcludedEmploymentTypes: []string{"Internship"},
			BusinessUnit: f.BusinessUnit, Department: f.Department, Team: f.Team})
		if err != nil {
			return resp, err
		}
		for _, e := range emps {
			people = append(people, allocOrgPerson(e))
		}
	case len(f.Emails) > 0:
		people = s.peopleOf(ctx, f.Emails)
	default:
		return resp, allocBad("Select a Business Unit, Department or Team")
	}
	if len(people) == 0 {
		return resp, nil
	}
	emails := make([]string, len(people))
	for i, p := range people {
		emails[i] = p.Email
	}
	rows, err := s.d.Entity.AllocationTimeline(ctx, engagemententity.AllocationTimelineRequest{
		Emails: emails, StartDate: f.From, EndDate: f.To, AllocationTypeIDs: f.AllocationTypeIDs})
	if err != nil {
		return resp, err
	}
	today := allocToday(s.d.Now())
	byEmail := map[string][]domain.AllocationTimelineItem{}
	for _, a := range rows {
		k := strings.ToLower(a.Email)
		byEmail[k] = append(byEmail[k], allocTimelineItem(a, today))
	}
	for _, p := range people {
		items := byEmail[strings.ToLower(p.Email)]
		if items == nil {
			items = []domain.AllocationTimelineItem{}
		}
		resp.People = append(resp.People, domain.AllocationTimelineRow{Person: p, Items: items})
	}
	return resp, nil
}

func allocOrgPerson(e hrentity.OrgEmployee) domain.AllocationEmployee {
	name := strings.TrimSpace(e.FirstName + " " + e.LastName)
	return domain.AllocationEmployee{Email: e.WorkEmail, Name: &name, JobRole: e.JobRole, JobBand: e.JobRole,
		BusinessUnit: e.BusinessUnit, Department: e.Department, Team: e.Team, Company: e.Company, Location: e.Location}
}

// peopleOf reads the named people from HR (at most 8 lookups in flight); a
// partner pseudo-email or a failed lookup keeps the email alone.
func (s *allocationService) peopleOf(ctx context.Context, emails []string) []domain.AllocationEmployee {
	out := make([]domain.AllocationEmployee, len(emails))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, raw := range emails {
		e := strings.TrimSpace(raw)
		out[i] = domain.AllocationEmployee{Email: e}
		if name, _ := allocPartnerName(e); name != nil {
			out[i].Name = name
			continue
		}
		wg.Add(1)
		go func(i int, e string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			emp, err := s.d.HR.Employee(ctx, e)
			if err != nil || emp == nil {
				if err != nil {
					slog.WarnContext(ctx, "timeline person not read from HR", "email", e, "err", err)
				}
				return
			}
			name := strings.TrimSpace(emp.FirstName + " " + emp.LastName)
			out[i] = domain.AllocationEmployee{Email: e, Name: &name, EmployeeID: &emp.EmployeeID, JobRole: emp.JobRole,
				JobBand: emp.JobRole, Department: emp.Department, Company: emp.Company, Location: emp.Location,
				ManagerEmail: emp.ManagerEmail}
		}(i, e)
	}
	wg.Wait()
	return out
}
