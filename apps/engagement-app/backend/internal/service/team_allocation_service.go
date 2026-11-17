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
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

const (
	teamAllocMsgPast30    = "Dates cannot be less than  30 days from today"
	teamAllocMsgTooOld    = "Default allocations that started more than 30 days ago cannot be changed."
	teamAllocMsgBusy      = "This person is already allocated for the selected period of time."
	teamAllocMaxBulkRows  = 5000
	teamAllocInternalType = allocTypeInternal
)

// TeamAllocationService is Team Allocations: Search by Team, add / edit /
// delete a default allocation, and Bulk Upload.
type TeamAllocationService interface {
	Search(ctx context.Context, f domain.AllocationTimelineFilter) (domain.AllocationTimelineResponse, error)
	Create(ctx context.Context, p domain.Perms, req domain.TeamAllocationCreate) (domain.AllocationTimelineItem, error)
	Update(ctx context.Context, p domain.Perms, id string, req domain.TeamAllocationUpdate) (domain.AllocationTimelineItem, error)
	Delete(ctx context.Context, p domain.Perms, id string) error
	ValidateBulk(ctx context.Context, csv string) (domain.TeamAllocationBulkResult, error)
	UploadBulk(ctx context.Context, p domain.Perms, csv string) (domain.TeamAllocationBulkResult, error)
}

type teamAllocationService struct{ a *allocationService }

// NewTeamAllocationService constructs a TeamAllocationService.
func NewTeamAllocationService(d AllocationDeps) TeamAllocationService {
	return &teamAllocationService{a: NewAllocationService(d).(*allocationService)}
}

// Search is Search by Team: one of business unit, department or team.
func (s *teamAllocationService) Search(ctx context.Context, f domain.AllocationTimelineFilter) (domain.AllocationTimelineResponse, error) {
	if f.BusinessUnit == "" && f.Department == "" && f.Team == "" {
		return domain.AllocationTimelineResponse{People: []domain.AllocationTimelineRow{}},
			allocBad("Select a Business Unit, Department or Team")
	}
	f.Emails = nil
	return s.a.Timeline(ctx, f)
}

// teamTypeFor returns the internal (engagement type 1) Active allocation
// type the Team Allocations popup offers.
func (s *teamAllocationService) teamTypeFor(ctx context.Context, id int) (engagemententity.AllocationType, error) {
	types, err := s.a.d.Entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{EngagementTypeID: teamAllocInternalType})
	if err != nil {
		return engagemententity.AllocationType{}, err
	}
	for _, t := range types {
		if t.ID == id && strings.EqualFold(t.Status, "Active") {
			return t, nil
		}
	}
	return engagemententity.AllocationType{}, allocBad("Select an active internal allocation type.")
}

// busyFor returns the person's other active, non-terminal allocations
// (engagement or default) touching [start, end]: the Team Allocations page
// refuses any overlap.
func (s *teamAllocationService) busyFor(ctx context.Context, email, start, end, exclude string) ([]engagemententity.AllocationRecord, error) {
	rows, err := s.a.d.Entity.AllocationTimeline(ctx, engagemententity.AllocationTimelineRequest{
		Emails: []string{email}, StartDate: start, EndDate: end})
	if err != nil {
		return nil, err
	}
	var out []engagemententity.AllocationRecord
	for _, r := range rows {
		if r.ID == exclude || allocIsTerminal(allocStr(r.ClearanceStatus)) {
			continue
		}
		if !(r.EndDate < start || r.StartDate > end) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Create adds a default allocation (legacy submitAllocation on the Team
// Allocations page: engagementId "-1", Confirmed, the type's default code).
func (s *teamAllocationService) Create(ctx context.Context, p domain.Perms, req domain.TeamAllocationCreate) (domain.AllocationTimelineItem, error) {
	var none domain.AllocationTimelineItem
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.AllocationTypeID <= 0 || req.StartDate == "" || req.EndDate == "" {
		return none, allocBad(allocMsgRequired)
	}
	t, err := s.teamTypeFor(ctx, req.AllocationTypeID)
	if err != nil {
		return none, err
	}
	code := allocStr(t.DefaultEngagementCode)
	if code == "" {
		code = strings.TrimSpace(allocStr(req.EngagementCode))
	}
	if code == "" {
		return none, allocBad(allocMsgRequired)
	}
	if req.Comment != nil && len([]rune(*req.Comment)) > 255 {
		return none, allocBad(allocMsgCommentLength)
	}
	if _, ok := allocParseDate(req.StartDate); !ok {
		return none, allocBad(allocMsgInvalidDates)
	}
	if _, ok := allocParseDate(req.EndDate); !ok {
		return none, allocBad(allocMsgInvalidDates)
	}
	today := allocToday(s.a.d.Now())
	limit := allocAddDays(today, -allocPastDaysDefault)
	if req.StartDate < limit || req.EndDate < limit {
		return none, allocBad(teamAllocMsgPast30)
	}
	if req.StartDate > req.EndDate {
		return none, allocBad(allocMsgEndBeforeStart)
	}
	emp, err := s.a.d.HR.Employee(ctx, email)
	if err != nil {
		return none, err
	}
	if emp == nil {
		return none, allocBad(allocMsgSelectEmail)
	}
	busy, err := s.busyFor(ctx, email, req.StartDate, req.EndDate, "")
	if err != nil {
		return none, err
	}
	if len(busy) > 0 {
		return none, s.a.overlapError(teamAllocMsgBusy, busy, nil, today)
	}
	comment := allocStr(req.Comment)
	res, err := s.a.d.Entity.BulkAllocations(ctx, engagemententity.AllocationBulkRequest{CreatedBy: p.Email,
		NewAllocations: []engagemententity.AllocationInput{{
			Email: email, AllocationTypeID: t.ID, Status: allocStatusActive, AllocationStatus: allocPtr(allocStatusActive),
			ClearanceStatus: allocPtr(domain.AllocationClearanceConfirmed), StartDate: req.StartDate, EndDate: req.EndDate,
			EngagementID: allocDefaultEngagement, EngagementCode: code, IsTravelRequired: "No", IsRecurring: "No", Comment: &comment,
		}}})
	if err != nil {
		return none, err
	}
	rec := engagemententity.AllocationRecord{ID: allocStr(res.FirstAllocationID), Email: email, AllocationTypeID: &t.ID,
		AllocationTypeName: &t.AllocationTypeName, AllocationTypeColor: t.Color, StartDate: req.StartDate, EndDate: req.EndDate,
		ClearanceStatus: allocPtr(domain.AllocationClearanceConfirmed), Comment: &comment, EngagementID: allocPtr(allocDefaultEngagement),
		EngagementCode: &code}
	if res.FirstAllocationID != nil {
		if r, err := s.a.d.Entity.Allocation(ctx, *res.FirstAllocationID); err == nil {
			rec = r
		}
	}
	return allocTimelineItem(rec, today), nil
}

// defaultAllocation reads an active default allocation, or 404.
func (s *teamAllocationService) defaultAllocation(ctx context.Context, id string) (engagemententity.AllocationRecord, error) {
	a, err := s.a.d.Entity.Allocation(ctx, id)
	if allocUpstreamNotFound(err) || (err == nil && (!allocIsDefaultID(a.EngagementID) || allocStr(a.Status) != allocStatusActive)) {
		return a, &apierror.NotFoundError{Msg: "Default allocation " + id + " was not found."}
	}
	return a, err
}

// Update changes a default allocation's dates and comment. A date can only
// change while the stored date is at most 30 days old, and not to a date
// older than that (legacy popup: the date fields are disabled otherwise).
func (s *teamAllocationService) Update(ctx context.Context, p domain.Perms, id string, req domain.TeamAllocationUpdate) (domain.AllocationTimelineItem, error) {
	var none domain.AllocationTimelineItem
	a, err := s.defaultAllocation(ctx, id)
	if err != nil {
		return none, err
	}
	if req.StartDate == nil && req.EndDate == nil && req.Comment == nil {
		return none, allocBad(allocMsgRequired)
	}
	today := allocToday(s.a.d.Now())
	limit := allocAddDays(today, -allocPastDaysDefault)
	start, end := a.StartDate, a.EndDate
	var upd engagemententity.AllocationUpdateRequest
	if req.StartDate != nil && *req.StartDate != a.StartDate {
		if _, ok := allocParseDate(*req.StartDate); !ok {
			return none, allocBad(allocMsgInvalidDates)
		}
		if a.StartDate < limit {
			return none, allocBad(teamAllocMsgTooOld)
		}
		if *req.StartDate < limit {
			return none, allocBad(teamAllocMsgPast30)
		}
		start = *req.StartDate
		upd.StartDate = &start
	}
	if req.EndDate != nil && *req.EndDate != a.EndDate {
		if _, ok := allocParseDate(*req.EndDate); !ok {
			return none, allocBad(allocMsgInvalidDates)
		}
		if a.EndDate < limit {
			return none, allocBad(teamAllocMsgTooOld)
		}
		if *req.EndDate < limit {
			return none, allocBad(teamAllocMsgPast30)
		}
		end = *req.EndDate
		upd.EndDate = &end
	}
	if start > end {
		return none, allocBad(allocMsgEndBeforeStart)
	}
	if req.Comment != nil {
		if len([]rune(*req.Comment)) > 255 {
			return none, allocBad(allocMsgCommentLength)
		}
		upd.Comment = req.Comment
	}
	if upd.StartDate != nil || upd.EndDate != nil {
		busy, err := s.busyFor(ctx, a.Email, start, end, a.ID)
		if err != nil {
			return none, err
		}
		if len(busy) > 0 {
			return none, s.a.overlapError(teamAllocMsgBusy, busy, nil, today)
		}
	}
	if upd.StartDate == nil && upd.EndDate == nil && upd.Comment == nil {
		return allocTimelineItem(a, today), nil
	}
	upd.UpdatedBy = p.Email
	res, err := s.a.d.Entity.UpdateAllocation(ctx, id, upd)
	if err != nil {
		return none, err
	}
	return allocTimelineItem(res.Allocation, today), nil
}

// Delete soft-deletes a default allocation (status Deleted) that started at
// most 30 days ago (legacy isRemovable).
func (s *teamAllocationService) Delete(ctx context.Context, _ domain.Perms, id string) error {
	a, err := s.defaultAllocation(ctx, id)
	if err != nil {
		return err
	}
	if a.StartDate < allocAddDays(allocToday(s.a.d.Now()), -allocPastDaysDefault) {
		return allocBad(teamAllocMsgTooOld)
	}
	return s.a.d.Entity.DeleteAllocation(ctx, id)
}

// ── Bulk Upload ─────────────────────────────────────────────────────────────

type teamBulkRow struct {
	line                       int
	email, team, start, end    string
	typeName                   string
	typeID                     int
	code                       string
	validDates, valid, ordered bool
}

// ValidateBulk ports BulkUpload.js validation: legacy texts, overlaps
// within the CSV and with existing allocations; Release 1 also rejects a row
// whose start is after its end. Nothing is saved.
func (s *teamAllocationService) ValidateBulk(ctx context.Context, data string) (domain.TeamAllocationBulkResult, error) {
	res, _, err := s.validateBulk(ctx, data)
	return res, err
}

// UploadBulk validates again and saves the valid rows (Confirmed, the type's
// default code, engagementId "-1.0"), with allocation_upload_log rows.
func (s *teamAllocationService) UploadBulk(ctx context.Context, p domain.Perms, data string) (domain.TeamAllocationBulkResult, error) {
	res, rows, err := s.validateBulk(ctx, data)
	if err != nil {
		return res, err
	}
	saved := 0
	res.Saved = &saved
	var news []engagemententity.AllocationInput
	var logs []engagemententity.AllocationUploadLog
	for _, r := range rows {
		if !r.valid {
			continue
		}
		news = append(news, engagemententity.AllocationInput{
			Email: r.email, AllocationTypeID: r.typeID, Status: allocStatusActive,
			ClearanceStatus: allocPtr(domain.AllocationClearanceConfirmed), StartDate: r.start, EndDate: r.end,
			EngagementID: allocBulkEngagement, EngagementCode: r.code, IsTravelRequired: "No", IsRecurring: "No",
		})
		logs = append(logs, engagemententity.AllocationUploadLog{Email: r.email, AllocationTypeID: r.typeID,
			StartDate: r.start, EndDate: r.end, EngagementCode: r.code})
	}
	if len(news) == 0 {
		return res, nil
	}
	out, err := s.a.d.Entity.BulkAllocations(ctx, engagemententity.AllocationBulkRequest{
		NewAllocations: news, AllocationLogs: logs, CreatedBy: p.Email})
	if err != nil {
		return res, err
	}
	saved = out.RecordsAdded
	return res, nil
}

func (s *teamAllocationService) validateBulk(ctx context.Context, data string) (domain.TeamAllocationBulkResult, []*teamBulkRow, error) {
	res := domain.TeamAllocationBulkResult{Errors: []string{}, Valid: []domain.TeamAllocationBulkRow{}}
	rows, badLines, err := teamParseCSV(data)
	if err != nil {
		return res, nil, err
	}
	if len(rows) > teamAllocMaxBulkRows {
		return res, nil, allocBadf("The file has more than %d rows.", teamAllocMaxBulkRows)
	}
	if len(badLines) > 0 {
		res.Errors = append(res.Errors, "Incorrect number of columns / empty data in these lines: "+strings.Join(badLines, ","))
	}
	if len(rows) == 0 {
		return res, nil, allocBad("Cannot proceed. Please recheck the csv file for empty data / incorrect number of columns")
	}
	if s.a.d.Org == nil {
		return res, nil, &apierror.UpstreamError{Service: "hr-entity", Msg: "not configured"}
	}
	emps, err := s.a.d.Org.EmployeesByOrg(ctx, hrentity.OrgEmployeeFilter{
		Statuses: []string{"Active", "Marked Leaver"}, ExcludedEmploymentTypes: []string{"Internship"}})
	if err != nil {
		return res, nil, err
	}
	known := map[string]bool{}
	for _, e := range emps {
		known[strings.ToLower(e.WorkEmail)] = true
	}
	// The same types Create offers: Active internal ones.
	types, err := s.a.d.Entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{
		EngagementTypeID: teamAllocInternalType, Status: "Active"})
	if err != nil {
		return res, nil, err
	}
	pastLimit := allocAddDays(allocToday(s.a.d.Now()), -allocPastDaysDefault)

	// Existing allocations of every person over the CSV's date range.
	minStart, maxEnd := "", ""
	var emails []string
	seen := map[string]bool{}
	for _, r := range rows {
		if !r.validDates {
			continue
		}
		if minStart == "" || r.start < minStart {
			minStart = r.start
		}
		if maxEnd == "" || r.end > maxEnd {
			maxEnd = r.end
		}
		if !seen[r.email] {
			seen[r.email] = true
			emails = append(emails, r.email)
		}
	}
	existing := map[string][]engagemententity.AllocationRecord{}
	if len(emails) > 0 {
		recs, err := s.a.d.Entity.AllocationTimeline(ctx, engagemententity.AllocationTimelineRequest{
			Emails: emails, StartDate: allocMinDate(minStart, maxEnd), EndDate: allocMaxDate(minStart, maxEnd)})
		if err != nil {
			return res, nil, err
		}
		for _, a := range recs {
			existing[strings.ToLower(a.Email)] = append(existing[strings.ToLower(a.Email)], a)
		}
	}

	for _, r := range rows {
		ok := true
		if !known[r.email] {
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("The email id is invalid for %s", r.email))
		}
		// The legacy team check (validateTeam) always passes; the team column is ignored.
		if r.validDates && teamCSVOverlap(r, rows) {
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("The allocation is overlapped for %s %s <-> %s in csv", r.email, r.start, r.end))
		}
		r.typeID, r.code = 0, ""
		for _, t := range types {
			if t.AllocationTypeName == r.typeName && t.EngagementTypeID == teamAllocInternalType &&
				strings.EqualFold(t.Status, "Active") {
				r.typeID, r.code = t.ID, strings.TrimSpace(allocStr(t.DefaultEngagementCode))
				break
			}
		}
		switch {
		case r.typeID == 0:
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("The allocation type for allocation of %s is wrong", r.email))
		case r.code == "":
			ok = false
			res.Errors = append(res.Errors,
				fmt.Sprintf("The allocation type for allocation of %s has no default engagement code", r.email))
		}
		switch {
		case !r.validDates:
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: Start Date [%s] / End Date [%s] must be dates (YYYY-MM-DD)", r.email, r.start, r.end))
		case !r.ordered:
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: Start Date [%s] is greater than End Date [%s]", r.email, r.start, r.end))
		case r.start < pastLimit || r.end < pastLimit:
			ok = false
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", r.email, teamAllocMsgPast30))
		default:
			if errs := teamExistingOverlaps(r, existing[r.email]); len(errs) > 0 {
				ok = false
				res.Errors = append(res.Errors, errs...)
			}
		}
		r.valid = ok
		if ok {
			res.Valid = append(res.Valid, domain.TeamAllocationBulkRow{Line: r.line, Email: r.email, Team: r.team,
				StartDate: r.start, EndDate: r.end, AllocationType: r.typeName})
		}
	}
	return res, rows, nil
}

// teamParseCSV reads the headerless 5-column CSV (username, team, start,
// end, allocation type name); the username gets "@wso2.com" (lower-cased).
func teamParseCSV(data string) ([]*teamBulkRow, []string, error) {
	r := csv.NewReader(strings.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows []*teamBulkRow
	var bad []string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, allocBad("The file could not be read as CSV.")
		}
		line, _ := r.FieldPos(0)
		for i := range rec {
			rec[i] = strings.TrimSpace(rec[i])
		}
		if len(rec) != 5 || rec[0] == "" {
			bad = append(bad, fmt.Sprint(line))
			continue
		}
		row := &teamBulkRow{line: line, email: strings.ToLower(rec[0]) + "@wso2.com", team: rec[1],
			start: rec[2], end: rec[3], typeName: rec[4]}
		_, okS := allocParseDate(row.start)
		_, okE := allocParseDate(row.end)
		row.validDates = okS && okE
		row.ordered = row.start <= row.end
		rows = append(rows, row)
	}
	return rows, bad, nil
}

// teamCSVOverlap is validateOverlappingAllocSameEmailInCSV.
func teamCSVOverlap(r *teamBulkRow, rows []*teamBulkRow) bool {
	for _, o := range rows {
		if o == r || o.email != r.email || !o.validDates {
			continue
		}
		if (r.start < o.start && r.end >= o.start) || (r.start >= o.start && r.start <= o.end) {
			return true
		}
	}
	return false
}

// teamExistingOverlaps is validateAllocationWithExisting's messages.
func teamExistingOverlaps(r *teamBulkRow, existing []engagemententity.AllocationRecord) []string {
	var out []string
	for _, a := range existing {
		end := fmt.Sprintf("with existing %s allocation.", allocStr(a.AllocationTypeName))
		switch {
		case a.StartDate <= r.start && a.EndDate >= r.end:
			out = append(out, fmt.Sprintf("%s: New Allocation fully overlaps %s", r.email, end))
		case a.StartDate <= r.start && a.EndDate >= r.start:
			out = append(out, fmt.Sprintf("%s: New Allocation overlaps at start [%s] %s", r.email, r.start, end))
		case a.StartDate <= r.end && a.EndDate >= r.end:
			out = append(out, fmt.Sprintf("%s: New Allocation overlaps at end [%s] %s", r.email, r.end, end))
		case a.StartDate >= r.start && a.EndDate <= r.end:
			out = append(out, fmt.Sprintf("%s: New Allocation overlap at middle [%s and %s] %s", r.email, a.StartDate, a.EndDate, end))
		}
	}
	return out
}
