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
	"reflect"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func TestAllocationRules(t *testing.T) {
	name, partner := allocPartnerName("partner-allocations-group+Nimbus_Consulting+Arjun+Rao@wso2.com")
	if *name != "Arjun Rao (Partner)" || *partner != "Nimbus Consulting" {
		t.Errorf("partner name = %s / %s", *name, *partner)
	}
	// Pre/post sale groups alone cannot allocate; 6 alone covers only type 2.
	cases := []struct {
		ids  []int
		typ  int
		want bool
	}{
		{[]int{32}, 6, false}, {[]int{6, 32}, 6, true}, {[]int{6, 32}, 4, true}, {[]int{6}, 4, false},
		{[]int{6, 33}, 7, true}, {[]int{6}, 2, true}, {[]int{16}, 1, true}, {[]int{21}, 5, false}, {[]int{20}, 1, false},
		{[]int{6}, 1, false}, {[]int{6}, 5, false}, {[]int{16}, 5, true},
	}
	for _, c := range cases {
		if got := allocCanAllocate(allocPerms(c.ids...), c.typ); got != c.want {
			t.Errorf("allocate %v type %d = %v", c.ids, c.typ, got)
		}
	}
	if !allocCanChangeStatus(allocPerms(20), 5) || allocCanChangeStatus(allocPerms(10), 3) {
		t.Error("status rule")
	}
	if got := AllocationNextStatuses("Confirmed"); !reflect.DeepEqual(got, []string{"Rejected by Consultant", "Rejected - Visa Issues", "Rejected - Other", "Allocation Cancelled"}) {
		t.Errorf("next = %v", got)
	}
	if got := AllocationNextStatuses("Rejected - Other"); got[0] != "Clearance In Progress" {
		t.Errorf("reopen = %v", got)
	}
	// Hourly windows: 09:00-11:00 Colombo vs 12:00-13:00 Colombo do not intersect.
	h := engagemententity.AllocationRecord{ID: "A1", StartDate: "2026-11-01", EndDate: "2026-11-30", IsRecurring: sp("Yes"),
		StartTime: sp("09:00"), EndTime: sp("11:00"), TimeZone: sp("Asia/Colombo")}
	ex := []engagemententity.AllocationRecord{h}
	if busy := allocAvailability(ex, "", "", "2026-11-10", "2026-11-12", true,
		&domain.AllocationHourly{TimeZone: "Asia/Colombo", StartTime: "12:00", EndTime: "13:00"}); len(busy) != 0 {
		t.Error("disjoint hourly windows should be allowed")
	}
	if busy := allocAvailability(ex, "", "", "2026-11-10", "2026-11-12", true,
		&domain.AllocationHourly{TimeZone: "UTC", StartTime: "04:00", EndTime: "05:00"}); len(busy) != 1 {
		t.Error("09:30 Colombo is 04:00 UTC: should clash")
	}
	if busy := allocAvailability(ex, "", "", "2026-11-10", "2026-11-12", true, nil); len(busy) != 1 {
		t.Error("a daily allocation is blocked by an hourly one")
	}
}

func TestAllocAvailability_BackToBackHourlySlotsDoNotClash(t *testing.T) {
	h := engagemententity.AllocationRecord{ID: "A1", StartDate: "2026-11-01", EndDate: "2026-11-30", IsRecurring: sp("Yes"),
		StartTime: sp("10:00"), EndTime: sp("11:00"), TimeZone: sp("UTC")}
	ex := []engagemententity.AllocationRecord{h}
	for _, w := range [][2]string{{"09:00", "10:00"}, {"11:00", "12:00"}} {
		if busy := allocAvailability(ex, "", "", "2026-11-10", "2026-11-12", true,
			&domain.AllocationHourly{TimeZone: "UTC", StartTime: w[0], EndTime: w[1]}); len(busy) != 0 {
			t.Errorf("%s-%s next to 10:00-11:00 should be allowed", w[0], w[1])
		}
	}
	if busy := allocAvailability(ex, "", "", "2026-11-10", "2026-11-12", true,
		&domain.AllocationHourly{TimeZone: "UTC", StartTime: "09:00", EndTime: "12:00"}); len(busy) != 1 {
		t.Error("09:00-12:00 contains 10:00-11:00: should clash")
	}
}

func TestAllocationCreate_HourlyTimesAreSavedAsHHMM(t *testing.T) {
	ent := &allocFakeEntity{engagement: allocTestEngagement(1)}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	req := domain.AllocationCreateRequest{ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2026-11-10"),
		EndDate: sp("2026-11-20"), TravelRequired: bp(false),
		Hourly: &domain.AllocationHourly{TimeZone: "UTC", StartTime: "9:00", EndTime: "17:00:00"}}
	if _, err := svc.Create(context.Background(), allocPerms(16), "E07201", req); err != nil {
		t.Fatalf("9:00-17:00 is a valid window (\"9:00\" sorts after \"17:00\" as text): %v", err)
	}
	m := ent.bulk[0].NewAllocations[0]
	if *m.StartTime != "09:00" || *m.EndTime != "17:00" {
		t.Errorf("saved %s-%s, want 09:00-17:00", *m.StartTime, *m.EndTime)
	}
}

func TestAllocationCreate_CustomerReadyForClearanceAndRequestStatus(t *testing.T) {
	ent := &allocFakeEntity{engagement: allocTestEngagement(4)}
	snd := &allocFakeSender{}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), snd))
	req := domain.AllocationCreateRequest{ConsultantEmail: sp("Jane@wso2.com"), StartDate: sp("2026-11-10"),
		EndDate: sp("2026-11-20"), TravelRequired: bp(true), Comment: sp("hi")}

	if _, err := svc.Create(context.Background(), allocPerms(6), "E07201", req); !errorsIsForbidden(err) {
		t.Fatalf("6 without 32 on post sale: %v", err)
	}
	out, err := svc.Create(context.Background(), allocPerms(6, 32), "E07201", req)
	if err != nil {
		t.Fatal(err)
	}
	b := ent.bulk[0]
	m := b.NewAllocations[0]
	if *m.ClearanceStatus != "Ready for Clearance" || m.Email != "jane@wso2.com" || m.IsTravelRequired != "Yes" ||
		m.EngagementCode != "NWB-CO" || m.AllocationTypeID != 12 || b.CreatedBy != "mgr@wso2.com" ||
		b.RequestStatusInProgress == nil || *b.RequestStatusInProgress != "E07201" {
		t.Errorf("bulk = %+v / %+v", b, m)
	}
	if out.Allocation.ID != "A0000000099" || out.EmailsSent || len(snd.sent) != 0 {
		t.Errorf("Ready for Clearance sends no email: %+v %v", out, snd.sent)
	}
}

func errorsIsForbidden(err error) bool {
	var fe *apierror.ForbiddenError
	return errors.As(err, &fe)
}

func TestAllocationCreate_ValidationOrderAndTexts(t *testing.T) {
	ent := &allocFakeEntity{engagement: allocTestEngagement(1)}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	p := allocPerms(16)
	base := func() domain.AllocationCreateRequest {
		return domain.AllocationCreateRequest{ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2026-11-10"),
			EndDate: sp("2026-11-20"), TravelRequired: bp(false)}
	}
	cases := []struct {
		mut func(*domain.AllocationCreateRequest)
		msg string
	}{
		{func(r *domain.AllocationCreateRequest) { r.ConsultantEmail = nil }, "Please select an email"},
		{func(r *domain.AllocationCreateRequest) { r.ConsultantEmail = sp("ghost@wso2.com") }, "Please select an email"},
		{func(r *domain.AllocationCreateRequest) { r.TravelRequired = nil }, "You need to fill the required fields"},
		{func(r *domain.AllocationCreateRequest) { r.StartDate = sp("2026-10-31") }, "Start Date / End Date is not within the range"},
		{func(r *domain.AllocationCreateRequest) { r.EndDate = sp("2026-12-01") }, "Start Date / End Date is not within the range"},
		{func(r *domain.AllocationCreateRequest) { r.StartDate, r.EndDate = sp("2026-11-21"), sp("2026-11-20") }, "End Date should be higher than Start Date"},
		{func(r *domain.AllocationCreateRequest) {
			r.Hourly = &domain.AllocationHourly{TimeZone: "UTC", StartTime: "11:00", EndTime: "09:00"}
		}, "End Time should be higher than Start Time"},
		{func(r *domain.AllocationCreateRequest) {
			r.Hourly = &domain.AllocationHourly{TimeZone: "Mars/Olympus", StartTime: "09:00", EndTime: "11:00"}
		}, allocMsgInvalidDates},
		// Loads only where the host has that zone file: not a stored zone.
		{func(r *domain.AllocationCreateRequest) {
			r.Hourly = &domain.AllocationHourly{TimeZone: "localtime", StartTime: "09:00", EndTime: "11:00"}
		}, allocMsgInvalidDates},
		{func(r *domain.AllocationCreateRequest) {
			r.Hourly = &domain.AllocationHourly{TimeZone: "Local", StartTime: "09:00", EndTime: "11:00"}
		}, allocMsgInvalidDates},
		{func(r *domain.AllocationCreateRequest) { r.ClearanceStatus = sp("Ready for Clearance") }, ""},
		{func(r *domain.AllocationCreateRequest) { r.PreparationStart = sp("2026-11-01") }, "Preparation and post engagement overheads are only for paid engagements."},
		{func(r *domain.AllocationCreateRequest) { r.Comment = sp(strings.Repeat("x", 256)) }, allocMsgCommentLength},
	}
	for i, c := range cases {
		r := base()
		c.mut(&r)
		if _, err := svc.Create(context.Background(), p, "E07201", r); !isAllocValidation(err, c.msg) {
			t.Errorf("case %d: err = %v, want %q", i, err, c.msg)
		}
	}

	ent.engagement.RequestStatus = sp("Pending Approval")
	if _, err := svc.Create(context.Background(), p, "E07201", base()); !isAllocValidation(err, "") {
		t.Errorf("pending: %v", err)
	}
	ent.engagement.RequestStatus = sp("New")
	ent.engagement.StartDate, ent.engagement.EndDate = sp("2020-01-01"), sp("2021-01-01")
	if _, err := svc.Create(context.Background(), p, "E07201", base()); !isAllocValidation(err, allocMsgPastEngagement) {
		t.Errorf("past engagement: %v", err)
	}
	if _, err := svc.Create(context.Background(), p, "E99999", base()); err == nil {
		t.Error("missing engagement: want 404")
	}
	if len(ent.bulk) != 0 {
		t.Error("nothing should be saved")
	}
}

func TestAllocationCreate_OverlapsAndDefaultTrim(t *testing.T) {
	ent := &allocFakeEntity{engagement: allocTestEngagement(6), types: []engagemententity.AllocationType{
		{ID: 77, AllocationTypeName: "Engagement Overhead", EngagementCodeSuffix: sp("EO")}}}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	p := allocPerms(6, 32)
	req := domain.AllocationCreateRequest{ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2026-11-10"),
		EndDate: sp("2026-11-20"), TravelRequired: bp(false)}

	// An active engagement allocation overlapping: 409 with the overlap.
	ent.timeline = []engagemententity.AllocationRecord{{ID: "A1", Email: "jane@wso2.com", StartDate: "2026-11-15",
		EndDate: "2026-11-25", EngagementID: sp("E00001"), ClearanceStatus: sp("Confirmed"),
		Engagement: &engagemententity.AllocationEngagementRef{ID: "E00001", Title: sp("Other")}}}
	oe, ok := isAllocOverlap(must(svc.Create(context.Background(), p, "E07201", req)), allocMsgBusy+allocMsgRefresh)
	if !ok || len(oe.Overlaps) != 1 || oe.Overlaps[0].Label != ": Other" {
		t.Fatalf("overlap = %+v", oe)
	}
	// A cancelled one does not block.
	ent.timeline[0].ClearanceStatus = sp("Allocation Cancelled")

	// Default allocation fully covered: refused.
	ent.timeline = append(ent.timeline, engagemententity.AllocationRecord{ID: "D1", Email: "jane@wso2.com",
		StartDate: "2026-11-12", EndDate: "2026-11-14", EngagementID: sp("-1"), ClearanceStatus: sp("Confirmed")})
	if _, ok := isAllocOverlap(must(svc.Create(context.Background(), p, "E07201", req)), allocMsgFullOverlay); !ok {
		t.Fatal("full overlay should be refused")
	}

	// Default allocation around the new range: confirm, then split.
	ent.timeline[1] = engagemententity.AllocationRecord{ID: "D1", Email: "jane@wso2.com", AllocationTypeID: ip(40),
		StartDate: "2026-11-01", EndDate: "2026-11-30", EngagementID: sp("-1"), EngagementCode: sp("DIG-GEN"), ClearanceStatus: sp("Confirmed")}
	oe, ok = isAllocOverlap(must(svc.Create(context.Background(), p, "E07201", req)), allocMsgDefaultOverlap)
	if !ok || len(oe.DefaultOverlaps) != 1 {
		t.Fatalf("default overlap = %+v", oe)
	}
	req.TrimDefaultAllocations = bp(true)
	req.PreparationStart, req.PostEngagementEnd = sp("2026-11-08"), sp("2026-11-22")
	if _, err := svc.Create(context.Background(), p, "E07201", req); err != nil {
		t.Fatal(err)
	}
	b := ent.bulk[0]
	if len(b.NewAllocations) != 4 {
		t.Fatalf("new = %+v", b.NewAllocations)
	}
	prep, post, split := b.NewAllocations[1], b.NewAllocations[2], b.NewAllocations[3]
	if prep.StartDate != "2026-11-08" || prep.EndDate != "2026-11-09" || prep.EngagementCode != "NWB-CO-EO" ||
		prep.EngagementID != "-1.0" || prep.AllocationTypeID != 77 || *prep.ClearanceStatus != "Confirmed" {
		t.Errorf("prep = %+v", prep)
	}
	if post.StartDate != "2026-11-21" || post.EndDate != "2026-11-22" {
		t.Errorf("post = %+v", post)
	}
	if split.StartDate != "2026-11-23" || split.EndDate != "2026-11-30" || split.EngagementID != "-1" || split.AllocationTypeID != 40 {
		t.Errorf("split = %+v", split)
	}
	if !reflect.DeepEqual(b.UpdateAllocations, []engagemententity.AllocationDateUpdate{{ID: "D1", StartDate: "2026-11-01", EndDate: "2026-11-07"}}) {
		t.Errorf("updates = %+v", b.UpdateAllocations)
	}
}

func must[T any](_ T, err error) error { return err }

func TestAllocationCreate_InternalConfirmedEmailsWithVisasAndPartner(t *testing.T) {
	ent := &allocFakeEntity{engagement: allocTestEngagement(1),
		visas: []engagemententity.AllocationVisa{{VisaType: sp("Business"), EntryType: sp("Single"), Status: sp("Approved"),
			StartDate: sp("2026-01-01"), EndDate: sp("2027-01-01")}}}
	ent.allocations = map[string]engagemententity.AllocationRecord{"A0000000099": {ID: "A0000000099", Email: "jane@wso2.com",
		StartDate: "2026-11-10", EndDate: "2026-11-20", ClearanceStatus: sp("Confirmed"), IsTravelRequired: sp("Yes"),
		EngagementID: sp("E07201"), Engagement: &engagemententity.AllocationEngagementRef{ID: "E07201", EngagementTypeID: 1}}}
	snd := &allocFakeSender{}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), snd))
	out, err := svc.Create(context.Background(), allocPerms(16, 20), "E07201", domain.AllocationCreateRequest{
		ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2026-11-10"), EndDate: sp("2026-11-20"),
		TravelRequired: bp(true), ClearanceStatus: sp("Confirmed")})
	if err != nil {
		t.Fatal(err)
	}
	if out.TravelRequest == nil || *out.TravelRequest != "TRV-2026-11-0001" || !out.EmailsSent || !out.Allocation.Actions.ChangeStatus {
		t.Errorf("out = %+v", out)
	}
	m := snd.sent[0]
	if m.TemplateID != "consultantConfirmed" || !strings.Contains(m.Content["visa_data"], "Business Single Entry") ||
		!reflect.DeepEqual(m.To, []string{"allocations@wso2.com", "jane@wso2.com", "lead1@wso2.com", "lead2@wso2.com", "mgr@wso2.com", "travel@wso2.com"}) {
		t.Errorf("email = %+v", m)
	}

	// A visa lookup failure drops the email (legacy), not the allocation.
	ent.visaErr = errors.New("down")
	out, err = svc.Create(context.Background(), allocPerms(16), "E07201", domain.AllocationCreateRequest{
		ConsultantEmail: sp("jane@wso2.com"), StartDate: sp("2026-11-10"), EndDate: sp("2026-11-20"),
		TravelRequired: bp(true), ClearanceStatus: sp("Confirmed")})
	if err != nil || out.EmailsSent {
		t.Errorf("visa failure: %+v %v", out, err)
	}

	// Partners are Confirmed, named by pseudo-email, emailed to the partner group.
}

func TestAllocationUpdateStatus(t *testing.T) {
	alloc := engagemententity.AllocationRecord{ID: "A1", Email: "jane@wso2.com", StartDate: "2026-11-10", EndDate: "2026-11-20",
		ClearanceStatus: sp("Ready for Clearance"), IsTravelRequired: sp("No"), EngagementID: sp("E07201"),
		Engagement: &engagemententity.AllocationEngagementRef{ID: "E07201", EngagementTypeID: 1, StartDate: sp("2026-11-01"), EndDate: sp("2026-11-30")}}
	ent := &allocFakeEntity{engagement: allocTestEngagement(1), allocations: map[string]engagemententity.AllocationRecord{"A1": alloc,
		"D1": {ID: "D1", EngagementID: sp("-1")}}}
	snd := &allocFakeSender{}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), snd))
	ctx := context.Background()

	if _, err := svc.UpdateStatus(ctx, allocPerms(16), "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed")}); !errorsIsForbidden(err) {
		t.Errorf("internal needs 20: %v", err)
	}
	p := allocPerms(20)
	if _, err := svc.UpdateStatus(ctx, p, "A1", domain.AllocationStatusUpdate{}); !isAllocValidation(err, allocMsgRequired) {
		t.Errorf("required: %v", err)
	}
	if _, err := svc.UpdateStatus(ctx, p, "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Accepted by Consultant")}); !isAllocValidation(err, "") {
		t.Errorf("transition: %v", err)
	}
	if _, err := svc.UpdateStatus(ctx, p, "D1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed")}); err == nil {
		t.Error("default allocation: want 404")
	}

	// Another active allocation overlapping blocks a confirmation...
	ent.timeline = []engagemententity.AllocationRecord{alloc, {ID: "A2", Email: "jane@wso2.com", StartDate: "2026-11-18",
		EndDate: "2026-11-25", EngagementID: sp("E1"), ClearanceStatus: sp("Ready for Clearance")}}
	if _, ok := isAllocOverlap(must(svc.UpdateStatus(ctx, p, "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Confirmed")})), ""); !ok {
		t.Error("overlap should block a confirmation")
	}
	// ...but never a cancellation (L5), which saves Inactive and emails.
	out, err := svc.UpdateStatus(ctx, p, "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Allocation Cancelled"), Comment: sp("dup")})
	if err != nil {
		t.Fatal(err)
	}
	u := ent.updates[0]
	if *u.Status != "Inactive" || *u.ClearanceStatus != "Allocation Cancelled" || *u.Comment != "dup" || u.UpdatedBy != "mgr@wso2.com" {
		t.Errorf("update = %+v", u)
	}
	if out.EmailsSent {
		t.Error("cancelling from Ready for Clearance sends no email (legacy gate)")
	}
	ent.timeline = nil
	if _, err := svc.UpdateStatus(ctx, p, "A1", domain.AllocationStatusUpdate{ClearanceStatus: sp("Clearance In Progress")}); err != nil {
		t.Fatal(err)
	}
	if *ent.updates[1].Status != "Active" || len(snd.sent) != 1 || snd.sent[0].Content["title"] != "Allocation Added for Internal Engagement" {
		t.Errorf("CIP: %+v %+v", ent.updates[1], snd.sent)
	}
}

func TestAllocationList_VisibilityAndRows(t *testing.T) {
	ent := &allocFakeEntity{search: engagemententity.AllocationSearchResult{Total: 1, Items: []engagemententity.AllocationRecord{{
		ID: "A1", Email: "partner-allocations-group+Nimbus+Arjun+Rao@wso2.com", StartDate: "2026-11-10", EndDate: "2026-11-20",
		IsTravelRequired: sp("Yes"), ClearanceStatus: sp("Confirmed"), EngagementID: sp("E07201"),
		Engagement:     &engagemententity.AllocationEngagementRef{ID: "E07201", EngagementTypeID: 4, Title: sp("PoC"), CustomerName: sp("Acme")},
		TravelRequests: []engagemententity.AllocationTravelRef{{JobNumber: "TRV-1", Status: sp("Cancelled")}, {JobNumber: "TRV-2", Status: sp("New")}, {JobNumber: "TRV-3", Status: sp("Cancelled")}},
	}}}}
	svc := NewAllocationService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	ctx := context.Background()

	// Reports viewer (9, 19, 3, 13): every type, read-only.
	page, err := svc.List(ctx, allocPerms(9, 19, 3, 13), domain.AllocationListFilter{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Allocation.Actions.ChangeStatus ||
		!reflect.DeepEqual(ent.searched[0].EngagementTypeIDs, []int{1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("viewer: %+v %v", page, err)
	}
	p := allocPerms(9, 3, 6, 32)
	p.Travel = domain.NewPrivilegeSet([]int{2})
	page, err = svc.List(ctx, p, domain.AllocationListFilter{Limit: 10, Kinds: []string{"customer", "internal"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := ent.searched[1].EngagementTypeIDs; !reflect.DeepEqual(got, []int{1, 2, 3, 4, 6, 7}) {
		t.Errorf("types = %v", got)
	}
	row := page.Items[0]
	if page.Total != 1 || row.Engagement.Kind != "customer" || *row.Consultant.Name != "Arjun Rao (Partner)" ||
		row.TravelRequest.JobNumber != "TRV-2" || !row.TravelRequest.CanOpen || !row.Actions.ChangeStatus {
		t.Errorf("row = %+v", row)
	}
	if _, err := svc.List(ctx, p, domain.AllocationListFilter{Limit: 10, Kinds: []string{"other"}}); !isAllocValidation(err, "") {
		t.Errorf("bad kind: %v", err)
	}
}

func TestAllocationTimeline_EmailsAndOrg(t *testing.T) {
	hr := allocTestHR()
	hr.org = []hrentity.OrgEmployee{{WorkEmail: "jane@wso2.com", FirstName: "Jane", LastName: "Doe", JobRole: sp("Software Engineer")}}
	ent := &allocFakeEntity{timeline: []engagemententity.AllocationRecord{
		{ID: "D1", Email: "jane@wso2.com", StartDate: "2026-10-20", EndDate: "2026-11-30", EngagementID: sp("-1"),
			EngagementCode: sp("DIG-GEN"), AllocationTypeName: sp("Digital"), ClearanceStatus: sp("Confirmed")},
		{ID: "D2", Email: "jane@wso2.com", StartDate: "2026-09-01", EndDate: "2026-09-30", EngagementID: sp("-1.0"),
			EngagementCode: sp("LEAVE"), AllocationTypeName: sp("Leave")},
	}}
	svc := NewAllocationService(allocTestDeps(ent, hr, &allocFakeSender{}))
	ctx := context.Background()
	resp, err := svc.Timeline(ctx, domain.AllocationTimelineFilter{Team: "Finance Apps", From: "2026-09-01", To: "2026-12-31", AllocationTypeIDs: []int{40}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.People) != 1 || *resp.People[0].Person.Name != "Jane Doe" || len(resp.People[0].Items) != 2 {
		t.Fatalf("resp = %+v", resp)
	}
	it := resp.People[0].Items
	if it[0].Label != "DIG-GEN: Digital" || !it[0].Editable || it[1].Editable || it[0].Kind != "default" {
		t.Errorf("items = %+v", it)
	}
	if hr.orgFilter.Team != "Finance Apps" || hr.orgFilter.ExcludedEmploymentTypes[0] != "Internship" || ent.timeReqs[0].AllocationTypeIDs[0] != 40 {
		t.Errorf("filters = %+v %+v", hr.orgFilter, ent.timeReqs)
	}
	resp, err = svc.Timeline(ctx, domain.AllocationTimelineFilter{Emails: []string{"jane@wso2.com", "partner-allocations-group+P+A+B@wso2.com"}, From: "2026-09-01", To: "2026-12-31"})
	if err != nil || len(resp.People) != 2 || *resp.People[1].Person.Name != "A B (Partner)" || len(resp.People[1].Items) != 0 {
		t.Errorf("emails = %+v %v", resp, err)
	}
	if _, err := svc.Timeline(ctx, domain.AllocationTimelineFilter{From: "2026-09-01", To: "2026-12-31"}); !isAllocValidation(err, "") {
		t.Errorf("no people: %v", err)
	}
	if _, err := svc.Timeline(ctx, domain.AllocationTimelineFilter{Emails: []string{"a"}, From: "2026-12-01", To: "2026-01-01"}); !isAllocValidation(err, "") {
		t.Errorf("from after to: %v", err)
	}
}

func TestAllocationListMine(t *testing.T) {
	ent := &allocFakeEntity{}
	svc := NewAllocationService(allocTestDeps(ent, &allocFakeHR{}, &allocFakeSender{}))
	p := allocPerms(1)
	if _, err := svc.ListMine(context.Background(), p, domain.AllocationListFilter{Consultants: []string{"other@wso2.com"}, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	req := ent.searched[len(ent.searched)-1]
	if len(req.Consultants) != 1 || req.Consultants[0] != "mgr@wso2.com" || len(req.EngagementTypeIDs) != 7 {
		t.Errorf("search = %+v", req)
	}
}
