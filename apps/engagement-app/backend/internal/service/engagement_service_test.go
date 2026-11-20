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
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Production role grants (screens-alloc.md §0).
var (
	engagementAdmin       = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}
	engagementAccountMgr  = []int{1, 2, 3, 5, 8}
	engagementCustMgr     = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 22}
	engagementInternalMgr = []int{11, 13, 14, 15, 16, 17, 18, 19, 20, 21, 26, 27}
	engagementMktApprover = []int{11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 29, 30, 31}
	engagementApproverInt = []int{26, 27, 28}
	engagementViewer      = []int{1, 3, 8, 9, 13, 18, 19, 22}
)

func engagementStateOf(t int, status string) engagementState {
	return engagementState{typeID: t, requestStatus: status, createdBy: "consultant@wso2.com"}
}

func TestEngagementRules_Actions(t *testing.T) {
	tests := []struct {
		name  string
		perms domain.Perms
		state engagementState
		want  domain.EngagementActions
	}{
		{"admin on a new customer engagement", engagementPerms("a@wso2.com", engagementAdmin, nil), engagementStateOf(6, "New"),
			domain.EngagementActions{Allocate: true, Edit: true, Complete: true, ChangeEngagementStatus: true, ChangeAllocationStatus: true}},
		{"account manager: edit, no allocate, creator may change status", engagementPerms("consultant@wso2.com", engagementAccountMgr, nil),
			engagementStateOf(3, "New"), domain.EngagementActions{Edit: true, ChangeEngagementStatus: true}},
		{"customer manager holds 6 but not 33: cannot allocate pre sale", engagementPerms("m@wso2.com", engagementCustMgr, nil),
			engagementStateOf(3, "In Progress"), domain.EngagementActions{Edit: true, Complete: true, ChangeEngagementStatus: true}},
		{"admin approves a pending event", engagementPerms("a@wso2.com", engagementAdmin, nil), engagementStateOf(5, "Pending Approval"),
			domain.EngagementActions{EditPending: true, Approve: true, ChangeAllocationStatus: true}},
		{"marketing approver cannot approve (needs 28)", engagementPerms("r@wso2.com", engagementMktApprover, nil),
			engagementStateOf(5, "Pending Approval"), domain.EngagementActions{EditPending: true, ChangeAllocationStatus: true}},
		{"approver-only role fails the PATCH check", engagementPerms("x@wso2.com", engagementApproverInt, nil),
			engagementStateOf(1, "Pending Approval"), domain.EngagementActions{}},
		{"cancelled: nothing but allocation status", engagementPerms("a@wso2.com", engagementAdmin, nil), engagementStateOf(1, "Cancelled"),
			domain.EngagementActions{ChangeAllocationStatus: true}},
		{"internal manager on an approved internal engagement", engagementPerms("i@wso2.com", engagementInternalMgr, nil),
			engagementStateOf(1, "Approved"), domain.EngagementActions{Allocate: true, Edit: true, Complete: true, ChangeEngagementStatus: true, ChangeAllocationStatus: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engagementActionsFor(tc.perms, tc.state); got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestEngagementRules_Scope(t *testing.T) {
	agent := engagementPerms("c@agency.com", engagementAdmin, []int{18})
	agent.Agency = &domain.TravelAgency{ID: 1, Name: "Classic"}
	tests := []struct {
		name  string
		perms domain.Perms
		state engagementState
		want  string
	}{
		{"admin with travel view is full", engagementPerms("a@wso2.com", engagementAdmin, []int{2}), engagementStateOf(6, "New"), "full"},
		{"admin without travel is roster", engagementPerms("a@wso2.com", engagementAdmin, nil), engagementStateOf(6, "New"), "roster"},
		{"viewer sees consultants only", engagementPerms("v@wso2.com", engagementViewer, nil), engagementStateOf(6, "New"), "consultants"},
		{"viewer on internal is roster", engagementPerms("v@wso2.com", engagementViewer, nil), engagementStateOf(1, "Approved"), "roster"},
		{"internal approver sees a pending internal engagement", engagementPerms("x@wso2.com", engagementApproverInt, nil),
			engagementStateOf(1, "Pending Approval"), "roster"},
		{"internal approver does not see an approved one", engagementPerms("x@wso2.com", engagementApproverInt, nil),
			engagementStateOf(1, "Approved"), ""},
		{"travel agent never", agent, engagementStateOf(6, "New"), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engagementScopeFor(tc.perms, tc.state); got != tc.want {
				t.Errorf("scope = %q, want %q", got, tc.want)
			}
		})
	}
}

func engagementTestService(entity *engagementFakeEntity, mail *engagementFakeMail) EngagementService {
	refs := NewReferenceService(entity, engagementFakeSales{}, engagementFakeHR{})
	return NewEngagementService(entity, refs, engagementFakeSales{}, engagementFakeHR{}, mail,
		config.EmailConfig{AppBaseURL: "https://one.wso2.com", AllocationsGroup: "allocations@wso2.com", TravelGroup: "travel@wso2.com",
			InternalApprovers: []string{"approve-internal@wso2.com"}, MarketingApprovers: []string{"approve-marketing@wso2.com"}},
		func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) })
}

func engagementTestEntity() *engagementFakeEntity {
	return &engagementFakeEntity{
		engagements: map[string]engagemententity.EngagementRecord{
			"E1": {ID: "E1", EngagementTypeID: 6, AllocationTypeID: 60, RequestStatus: engagementP("New"), CreatedBy: engagementP("consultant@wso2.com"),
				CustomerName: engagementP("Acme"), ProjectName: engagementP("Acme - PoC"), StartDate: engagementP("2026-12-01"),
				EndDate: engagementP("2026-12-20"), OpportunityLineItemID: engagementP("00kA"), EngagementNature: engagementP("On-site"),
				FundingSources: []engagemententity.EngagementFundingRow{{BusinessUnit: "Digital Operations", ProductUnit: "Digital", Region: "NA", SubRegion: "NA - EAST", Percentage: 100}}},
			"EL1": {ID: "EL1", EngagementTypeID: 1, AllocationTypeID: 10, RequestStatus: engagementP("Pending Approval"), CreatedBy: engagementP("consultant@wso2.com"),
				ProjectName: engagementP("Team offsite"), StartDate: engagementP("2026-12-01"), EndDate: engagementP("2026-12-02"),
				EngagementNature: engagementP("On-site"), Country: engagementP("Sri Lanka")},
		},
		roster: map[string][]engagemententity.EngagementRosterAllocation{
			"E1": {
				{ID: "A1", Email: "jane@wso2.com", StartDate: engagementP("2026-12-01"), EndDate: engagementP("2026-12-05"),
					ClearanceStatus: engagementP("Confirmed"), IsTravelRequired: engagementP("Yes"), IsRecurring: engagementP("Yes"),
					StartTime: engagementP("09:00:00"), EndTime: engagementP("11:00:00"), TimeZone: engagementP("Asia/Colombo"),
					TravelRequests: []engagemententity.EngagementTravelRef{{JobNumber: "TRV-1", Status: engagementP("Cancelled")}, {JobNumber: "TRV-2", Status: engagementP("New")}}},
				{ID: "A2", Email: "partner-allocations-group+Nimbus_Consulting+Arjun+Rao@wso2.com", StartDate: engagementP("2026-12-01"),
					EndDate: engagementP("2026-12-05"), ClearanceStatus: engagementP("Rejected by Consultant"), IsTravelRequired: engagementP("No")},
			},
		},
	}
}

func TestEngagementList_SegmentGateRoleAndCounts(t *testing.T) {
	entity := engagementTestEntity()
	svc := engagementTestService(entity, &engagementFakeMail{})
	ctx := context.Background()

	// Account manager (1,2,3,5,8): customer rows with consultants; internal
	// segment not offered.
	page, err := svc.List(ctx, engagementPerms("consultant@wso2.com", engagementAccountMgr, nil), EngagementListFilter{Segment: "customer", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Consultants == nil || !reflect.DeepEqual(page.SegmentCounts, map[string]int{"customer": 7}) {
		t.Errorf("page = %+v", page)
	}
	if !entity.search.IncludeConsultants || entity.search.Sort != "current" || entity.search.Today != "2026-11-05" {
		t.Errorf("search = %+v", entity.search)
	}
	var fe *apierror.ForbiddenError
	if _, err := svc.List(ctx, engagementPerms("consultant@wso2.com", engagementAccountMgr, nil), EngagementListFilter{Segment: "internal"}); !errors.As(err, &fe) {
		t.Fatalf("internal segment: err %v", err)
	}
	// Create-only (2): the segment shows, the rows need engRole.
	if _, err := svc.List(ctx, engagementPerms("c@wso2.com", []int{2}, nil), EngagementListFilter{Segment: "customer"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entity.search.EngagementTypeIDs, []int{0}) {
		t.Errorf("types = %v", entity.search.EngagementTypeIDs)
	}
	var ve *apierror.ValidationError
	for _, f := range []EngagementListFilter{{Segment: "all"}, {Segment: "customer", RequestStatuses: []string{"Open"}}, {Segment: "customer", Consultants: "x"}} {
		if _, err := svc.List(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), f); !errors.As(err, &ve) {
			t.Errorf("filter %+v: err %v", f, err)
		}
	}
	// A type filter outside the segment matches nothing.
	_, _ = svc.List(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), EngagementListFilter{Segment: "customer", EngagementTypeIDs: []int{1, 6}})
	if !reflect.DeepEqual(entity.search.EngagementTypeIDs, []int{6}) {
		t.Errorf("types = %v", entity.search.EngagementTypeIDs)
	}
}

func TestEngagementApprovals(t *testing.T) {
	entity := engagementTestEntity()
	svc := engagementTestService(entity, &engagementFakeMail{})
	page, err := svc.Approvals(context.Background(), engagementPerms("x@wso2.com", engagementApproverInt, nil), "internal", "", 50, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "EL1" || !reflect.DeepEqual(page.SegmentCounts, map[string]int{"internal": 2}) {
		t.Fatalf("page %+v err %v", page, err)
	}
	if !entity.search.PendingApproval || entity.search.Sort != "created" {
		t.Errorf("search = %+v", entity.search)
	}
	var fe *apierror.ForbiddenError
	if _, err := svc.Approvals(context.Background(), engagementPerms("x@wso2.com", engagementApproverInt, nil), "events", "", 50, 0); !errors.As(err, &fe) {
		t.Fatalf("events: %v", err)
	}
}

func TestEngagementGet_ScopesAndAllocations(t *testing.T) {
	svc := engagementTestService(engagementTestEntity(), &engagementFakeMail{})
	ctx := context.Background()
	d, err := svc.Get(ctx, engagementPerms("a@wso2.com", engagementAdmin, []int{2}), "E1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Scope != "full" || len(d.Allocations) != 2 || d.CascadePreview == nil || d.CascadePreview.ActiveAllocations != 2 ||
		d.CascadePreview.OpenTravelRequests != 1 || d.SalesSubRegion == nil || *d.SalesSubRegion != "NA - EAST" {
		t.Fatalf("detail = %+v", d)
	}
	a1, a2 := d.Allocations[0], d.Allocations[1]
	if a1.TravelRequest == nil || a1.TravelRequest.JobNumber != "TRV-2" || !a1.TravelRequest.CanOpen || a1.Hourly == nil ||
		a1.Hourly.StartTime != "09:00" || *a1.Consultant.Name != "Jane Doe" {
		t.Errorf("A1 = %+v", a1)
	}
	if a2.Partner == nil || *a2.Partner != "Nimbus Consulting" || *a2.Consultant.Name != "Arjun Rao (Partner)" {
		t.Errorf("A2 = %+v", a2)
	}
	// Consultants scope: only active, non-terminal allocations, consultant
	// and dates only.
	d, err = svc.Get(ctx, engagementPerms("v@wso2.com", engagementViewer, nil), "E1")
	if err != nil || d.Scope != "consultants" || len(d.Allocations) != 1 || d.Allocations[0].ID != "" || d.CascadePreview != nil {
		t.Fatalf("consultants detail %+v err %v", d, err)
	}
	var fe *apierror.ForbiddenError
	if _, err := svc.Get(ctx, engagementPerms("x@wso2.com", []int{9}, nil), "E1"); !errors.As(err, &fe) {
		t.Fatalf("no scope: %v", err)
	}
}

func TestEngagementCreate_Customer(t *testing.T) {
	entity := engagementTestEntity()
	mail := &engagementFakeMail{}
	svc := engagementTestService(entity, mail)
	in := domain.EngagementCreate{EngagementTypeID: 6, AccountID: "001A", OpportunityID: "006A", OpportunityLineItemID: "00kA",
		AllocationTypeID: 60, Nature: "On-site", Country: "United States of America", Address: "1 Main St\nBoston",
		BusinessUnit: "Marketing", ProductUnit: "Brand", SalesSubRegion: "LK", CustomerContact: "Dana", StartDate: "2026-01-01", EndDate: "2026-01-02"}
	d, err := svc.Create(context.Background(), engagementPerms("consultant@wso2.com", engagementCustMgr, nil), in)
	if err != nil {
		t.Fatal(err)
	}
	c := entity.created
	if c.EngagementCode != "ACME-APIM-CO" || c.StartDate != "2026-12-01" || c.EndDate != "2026-12-20" || c.CustomerName != "Acme" ||
		c.CustomerCode != "001A" || c.ProjectName != "Acme - APIM PoC" || c.OpportunityOwner != "Alex" || c.RequestStatus != "New" ||
		c.EngagementStatus != "Pending" || c.ProductName != "APIM Consulting" || c.UUID == "" || c.CreatedBy != "consultant@wso2.com" ||
		!c.TentativeDates {
		t.Errorf("created = %+v", c)
	}
	// Paid: BU/PU from the line item, sub region from the account.
	if !reflect.DeepEqual(c.FundingSources, []engagemententity.EngagementFundingRow{{BusinessUnit: "Digital Operations", ProductUnit: "Digital", Region: "NA", SubRegion: "NA - EAST", Percentage: 100}}) {
		t.Errorf("funding = %+v", c.FundingSources)
	}
	if d.ID != "E09999" || len(mail.sent) != 1 {
		t.Fatalf("d %+v mails %d", d, len(mail.sent))
	}
	m := mail.sent[0]
	if m.Subject != "[Engagement App] Consultant request for E09999 : Acme - Acme - APIM PoC" || m.TemplateID != "customerEngagement" ||
		!reflect.DeepEqual(m.To, []string{"consultant@wso2.com", "allocations@wso2.com", "consultant@wso2.com"}) {
		t.Errorf("mail = %+v", m)
	}
}

func TestEngagementCreate_CodesAndValidation(t *testing.T) {
	entity := engagementTestEntity()
	svc := engagementTestService(entity, &engagementFakeMail{})
	admin := engagementPerms("a@wso2.com", engagementAdmin, nil)
	base := domain.EngagementCreate{AccountID: "001A", OpportunityID: "006A", Nature: "Off-site", Country: "Sri Lanka", Address: "x",
		BusinessUnit: "Digital Operations", ProductUnit: "Digital", SalesSubRegion: "LK", CustomerContact: "c", StartDate: "2026-12-01", EndDate: "2026-12-02"}

	pre := base
	pre.EngagementTypeID, pre.AllocationTypeID = 3, 30
	if _, err := svc.Create(context.Background(), admin, pre); err != nil {
		t.Fatal(err)
	}
	if entity.created.EngagementCode != "ACME-PR" {
		t.Errorf("pre sale code = %q", entity.created.EngagementCode)
	}
	post := base
	post.EngagementTypeID, post.AllocationTypeID = 4, 40
	if _, err := svc.Create(context.Background(), admin, post); err != nil || entity.created.EngagementCode != "ACME-HC" {
		t.Fatalf("post sale code %q err %v", entity.created.EngagementCode, err)
	}
	internal := domain.EngagementCreate{EngagementTypeID: 1, AllocationTypeID: 10, Title: "Offsite", Nature: "On-site", Country: "Sri Lanka",
		Address: "Colombo", BusinessUnit: "Digital Operations", ProductUnit: "Digital", SalesSubRegion: "LK", StartDate: "2026-12-01", EndDate: "2026-12-01"}
	if _, err := svc.Create(context.Background(), admin, internal); err != nil || entity.created.EngagementCode != "DIG-GEN" ||
		entity.created.RequestStatus != "Pending Approval" || entity.created.CustomerName != "WSO2" {
		t.Fatalf("internal %+v err %v", entity.created, err)
	}
	event := domain.EngagementCreate{EngagementTypeID: 5, AllocationTypeID: 12, CampaignID: "7015g00000ApD26AAF", Title: "WSO2Con Asia Booth",
		Nature: "Physical", Country: "Sri Lanka", Address: "Colombo", StartDate: "2026-12-01", EndDate: "2026-12-03",
		FundingSources: []domain.EngagementFundingSource{
			{Region: "APAC", SubRegion: "LK", BusinessUnit: "Marketing", ProductUnit: "Brand", Percentage: 60},
			{Region: "NA", SubRegion: "NA - EAST", BusinessUnit: "Marketing", ProductUnit: "Brand", Percentage: 40},
		}}
	if _, err := svc.Create(context.Background(), admin, event); err != nil || entity.created.ProjectName != "WSO2Con Asia Booth" ||
		*entity.created.CampaignID != "7015g00000ApD26AAF" || entity.created.EngagementCode != "MKT-CONF" {
		t.Fatalf("event %+v err %v", entity.created, err)
	}

	tests := []struct {
		name string
		mut  func(*domain.EngagementCreate)
		want string
	}{
		{"inactive type", func(c *domain.EngagementCreate) { c.EngagementTypeID = 2 }, "Select the engagement type."},
		{"type mismatch", func(c *domain.EngagementCreate) { c.AllocationTypeID = 10 }, "Select the allocation type."},
		{"no suffix", func(c *domain.EngagementCreate) { c.AllocationTypeID = 41 },
			"Engagement Code cannot be set. Engagement Code Suffix is not available for the selected Allocation Type."},
		{"no account", func(c *domain.EngagementCreate) { c.AccountID = "" }, "Select an account from Salesforce."},
		{"unknown opportunity", func(c *domain.EngagementCreate) { c.OpportunityID = "006Z" }, "Select the opportunity."},
		{"bad nature", func(c *domain.EngagementCreate) { c.Nature = "Physical" }, "Select the location."},
		{"no contact", func(c *domain.EngagementCreate) { c.CustomerContact = "" }, "Add the customer contact."},
		{"dates order", func(c *domain.EngagementCreate) { c.EndDate = "2026-11-01" }, "The end date is before the start date."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := post
			tc.mut(&in)
			_, err := svc.Create(context.Background(), admin, in)
			var ve *apierror.ValidationError
			if !errors.As(err, &ve) || ve.Msg != tc.want {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
	bad := event
	bad.FundingSources = []domain.EngagementFundingSource{{Region: "APAC", SubRegion: "LK", BusinessUnit: "Marketing", ProductUnit: "Brand", Percentage: 60}}
	_, err := svc.Create(context.Background(), admin, bad)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || ve.Msg != "Funding sources add up to 60%. They must total 100%." {
		t.Errorf("funding err = %v", err)
	}
	bad = event
	bad.CampaignID = "701unknown"
	if _, err := svc.Create(context.Background(), admin, bad); !errors.As(err, &ve) || ve.Msg != "Select the campaign." {
		t.Errorf("campaign err = %v", err)
	}
	bad = event
	bad.Title = " "
	if _, err := svc.Create(context.Background(), admin, bad); !errors.As(err, &ve) || ve.Msg != "Add the event name." {
		t.Errorf("event name err = %v", err)
	}
	// Type-specific create privilege: 11 alone cannot create an event.
	var fe *apierror.ForbiddenError
	if _, err := svc.Create(context.Background(), engagementPerms("i@wso2.com", []int{11}, nil), event); !errors.As(err, &fe) {
		t.Errorf("privilege err = %v", err)
	}
}

func TestEngagementUpdate_LocksStatusAndEmail(t *testing.T) {
	entity := engagementTestEntity()
	mail := &engagementFakeMail{}
	svc := engagementTestService(entity, mail)
	admin := engagementPerms("a@wso2.com", engagementAdmin, nil)
	ctx := context.Background()

	scope := "New scope"
	if _, err := svc.Update(ctx, admin, "E1", domain.EngagementUpdate{EngagementScope: &scope}); err != nil {
		t.Fatal(err)
	}
	u := entity.lastUpdate()
	if *u.EngagementScope != scope || *u.RequestStatus != "Updated" || u.UpdatedBy != "a@wso2.com" {
		t.Errorf("update = %+v", u)
	}
	if len(mail.sent) != 1 || mail.sent[0].Content["title"] != "Customer Engagement Updated" {
		t.Errorf("mails = %+v", mail.sent)
	}
	var ve *apierror.ValidationError
	country := "India"
	date := "2026-12-03"
	for _, in := range []domain.EngagementUpdate{{Country: &country}, {StartDate: &date}} {
		if _, err := svc.Update(ctx, admin, "E1", in); !errors.As(err, &ve) {
			t.Errorf("locked %+v: err %v", in, err)
		}
	}
	// Fetch New Dates (paid).
	yes := true
	if _, err := svc.Update(ctx, admin, "E1", domain.EngagementUpdate{RefreshDatesFromSalesforce: &yes}); err != nil {
		t.Fatal(err)
	}
	if u := entity.lastUpdate(); *u.StartDate != "2027-01-05" || *u.EndDate != "2027-01-30" {
		t.Errorf("dates = %+v", u)
	}
	// Account manager (5, no 15) on a customer engagement: allowed; viewer: 403.
	var fe *apierror.ForbiddenError
	if _, err := svc.Update(ctx, engagementPerms("v@wso2.com", engagementViewer, nil), "E1", domain.EngagementUpdate{EngagementScope: &scope}); !errors.As(err, &fe) {
		t.Errorf("viewer: %v", err)
	}
	// Pending edit + approve: Updated and Approved, the code recomputed.
	mail.sent = nil
	at := 10
	title := "New offsite"
	if _, err := svc.Update(ctx, admin, "EL1", domain.EngagementUpdate{Title: &title, AllocationTypeID: &at, Approve: &yes}); err != nil {
		t.Fatal(err)
	}
	if u := entity.lastUpdate(); *u.RequestStatus != "Approved" || *u.ProjectName != "New offsite" {
		t.Errorf("approve update = %+v", u)
	}
	// AllocationTypeID equal to the current one is no change; the title is.
	if len(mail.sent) != 1 || mail.sent[0].Content["title"] != "Internal Engagement Updated and Approved" ||
		!reflect.DeepEqual(mail.sent[0].To, []string{"lead@wso2.com", "a@wso2.com"}) ||
		!reflect.DeepEqual(mail.sent[0].CC, []string{"consultant@wso2.com", "travel@wso2.com"}) {
		t.Errorf("mail = %+v", mail.sent)
	}
}

func TestEngagementStatusChanges(t *testing.T) {
	entity := engagementTestEntity()
	mail := &engagementFakeMail{}
	svc := engagementTestService(entity, mail)
	ctx := context.Background()

	// The creator with only 5 (account manager) may change the status.
	if _, err := svc.SetEngagementStatus(ctx, engagementPerms("consultant@wso2.com", engagementAccountMgr, nil), "E1",
		domain.EngagementStatusChange{Status: "Confirmed"}); err != nil {
		t.Fatal(err)
	}
	if u := entity.lastUpdate(); *u.EngagementStatus != "Confirmed" || u.RequestStatus != nil || len(mail.sent) != 0 {
		t.Errorf("update %+v mails %d", u, len(mail.sent))
	}
	var fe *apierror.ForbiddenError
	if _, err := svc.SetEngagementStatus(ctx, engagementPerms("other@wso2.com", engagementAccountMgr, nil), "E1",
		domain.EngagementStatusChange{Status: "Confirmed"}); !errors.As(err, &fe) {
		t.Errorf("non-creator: %v", err)
	}
	// Cancel: request status Cancelled, consultants and the Travel group emailed.
	if _, err := svc.SetEngagementStatus(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), "E1",
		domain.EngagementStatusChange{Status: "Cancelled"}); err != nil {
		t.Fatal(err)
	}
	if u := entity.lastUpdate(); *u.RequestStatus != "Cancelled" {
		t.Errorf("cancel update = %+v", u)
	}
	m := mail.sent[0]
	if m.TemplateID != "customerEngagementCancelled" || m.To[0] != "jane@wso2.com" ||
		!reflect.DeepEqual(m.CC, []string{"travel@wso2.com", "consultant@wso2.com"}) {
		t.Errorf("cancel mail = %+v", m)
	}
	// Cancelled now: no further status changes.
	var ce *apierror.ConflictError
	if _, err := svc.SetRequestStatus(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), "E1", "Completed"); !errors.As(err, &ce) {
		t.Errorf("complete cancelled: %v", err)
	}
	var ve *apierror.ValidationError
	if _, err := svc.SetRequestStatus(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), "E1", "Approved"); !errors.As(err, &ve) {
		t.Errorf("bad status: %v", err)
	}
}

func TestEngagementApprove(t *testing.T) {
	entity := engagementTestEntity()
	mail := &engagementFakeMail{}
	svc := engagementTestService(entity, mail)
	ctx := context.Background()
	var fe *apierror.ForbiddenError
	// The internal approver role (26, 27, 28) fails the legacy PATCH check.
	if _, err := svc.Approve(ctx, engagementPerms("x@wso2.com", engagementApproverInt, nil), "EL1"); !errors.As(err, &fe) {
		t.Fatalf("approver-only: %v", err)
	}
	if _, err := svc.Approve(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), "EL1"); err != nil {
		t.Fatal(err)
	}
	if u := entity.lastUpdate(); *u.RequestStatus != "Approved" || mail.sent[0].Content["title"] != "Internal Engagement Approved" {
		t.Errorf("update %+v mail %+v", u, mail.sent)
	}
	var ce *apierror.ConflictError
	if _, err := svc.Approve(ctx, engagementPerms("a@wso2.com", engagementAdmin, nil), "E1"); !errors.As(err, &ce) {
		t.Errorf("customer: %v", err)
	}
}

func TestEngagementCreate_RosterFailureAfterTheWriteStillAnswers(t *testing.T) {
	entity := engagementTestEntity()
	entity.rosterErr = &apierror.ServiceUnavailableError{Msg: "down"}
	mail := &engagementFakeMail{}
	svc := engagementTestService(entity, mail)
	internal := domain.EngagementCreate{EngagementTypeID: 1, AllocationTypeID: 10, Title: "Offsite", Nature: "On-site",
		Country: "Sri Lanka", Address: "Colombo", BusinessUnit: "Digital Operations", ProductUnit: "Digital",
		SalesSubRegion: "LK", StartDate: "2026-12-01", EndDate: "2026-12-01"}
	d, err := svc.Create(context.Background(), engagementPerms("a@wso2.com", engagementAdmin, nil), internal)
	if err != nil || d.ID != "E09999" || len(d.Allocations) != 0 || len(mail.sent) != 1 {
		t.Fatalf("detail %+v err %v mails %d", d, err, len(mail.sent))
	}
	// A read with no write still fails.
	if _, err := svc.Get(context.Background(), engagementPerms("a@wso2.com", engagementAdmin, nil), "E1"); err == nil {
		t.Error("Get: want the roster error")
	}
}

func TestEngagementUpdate_NoScopeGetsConsultantsNotRoster(t *testing.T) {
	svc := engagementTestService(engagementTestEntity(), &engagementFakeMail{})
	// Edit (5) alone: may PATCH, has no view scope.
	d, err := svc.Update(context.Background(), engagementPerms("e@wso2.com", []int{5}, nil), "E1", domain.EngagementUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Scope != domain.EngagementScopeConsultants || len(d.Allocations) != 1 || d.Allocations[0].ID != "" ||
		d.Allocations[0].ClearanceStatus != nil {
		t.Fatalf("detail %+v", d)
	}
}

// engagementBlankSales has an opportunity with no engagement codes.
type engagementBlankSales struct{ engagementFakeSales }

func (engagementBlankSales) Opportunities(context.Context, string, bool) ([]salesentity.SalesOpportunity, error) {
	return []salesentity.SalesOpportunity{{ID: "006A", Name: "Acme - APIM PoC", PreSalesCode: engagementP(" (Open Banking)"),
		PostSalesEngagementCode: engagementP("  "), IsWon: new(bool)}}, nil
}

// engagementExtraTypeEntity adds an active customer type 8 with an
// allocation type, which needs no line item.
type engagementExtraTypeEntity struct{ *engagementFakeEntity }

func (engagementExtraTypeEntity) ReferenceData(context.Context) (engagemententity.ReferenceLists, error) {
	ref := engagementTestRef()
	ref.EngagementTypes = append(ref.EngagementTypes, engagemententity.EngagementType{ID: 8, Name: "Support", Type: "Customer",
		IsActive: true})
	ref.AllocationTypes = append(ref.AllocationTypes, engagemententity.AllocationType{ID: 80, AllocationTypeName: "Support",
		Status: "Active", EngagementTypeID: 8})
	return ref, nil
}

func TestEngagementCreate_EmptyCodeAndUnknownTypeAreRefused(t *testing.T) {
	admin := engagementPerms("a@wso2.com", engagementAdmin, nil)
	base := domain.EngagementCreate{AccountID: "001A", OpportunityID: "006A", Nature: "Off-site", Country: "Sri Lanka",
		Address: "x", BusinessUnit: "Digital Operations", ProductUnit: "Digital", SalesSubRegion: "LK", CustomerContact: "c",
		StartDate: "2026-12-01", EndDate: "2026-12-02"}
	mailCfg := config.EmailConfig{AllocationsGroup: "allocations@wso2.com"}
	now := func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) }

	entity := engagementTestEntity()
	sales := engagementBlankSales{}
	svc := NewEngagementService(entity, NewReferenceService(entity, sales, engagementFakeHR{}), sales, engagementFakeHR{},
		&engagementFakeMail{}, mailCfg, now)
	for _, tc := range []struct{ typeID, allocTypeID int }{{3, 30}, {4, 40}} {
		in := base
		in.EngagementTypeID, in.AllocationTypeID = tc.typeID, tc.allocTypeID
		_, err := svc.Create(context.Background(), admin, in)
		var ve *apierror.ValidationError
		if !errors.As(err, &ve) || ve.Msg != "Engagement Code cannot be set. The selected opportunity has no engagement code." {
			t.Errorf("type %d: err = %v", tc.typeID, err)
		}
	}

	extra := engagementExtraTypeEntity{engagementTestEntity()}
	svc = NewEngagementService(extra, NewReferenceService(extra, engagementFakeSales{}, engagementFakeHR{}),
		engagementFakeSales{}, engagementFakeHR{}, &engagementFakeMail{}, mailCfg, now)
	in := base
	in.EngagementTypeID, in.AllocationTypeID = 8, 80
	_, err := svc.Create(context.Background(), admin, in)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || ve.Msg != "Select the opportunity line item." {
		t.Errorf("type 8 without a line item: err = %v", err)
	}
}

// engagementCountingHR knows nobody and counts its lookups.
type engagementCountingHR struct {
	engagementFakeHR
	calls *atomic.Int32
}

func (h engagementCountingHR) Employee(context.Context, string) (*hrentity.Employee, error) {
	h.calls.Add(1)
	return nil, nil
}

func TestEngagementGet_UnknownPeopleAreNotLookedUpAgain(t *testing.T) {
	entity := engagementTestEntity()
	hr := engagementCountingHR{calls: &atomic.Int32{}}
	svc := NewEngagementService(entity, NewReferenceService(entity, engagementFakeSales{}, hr), engagementFakeSales{}, hr,
		&engagementFakeMail{}, config.EmailConfig{}, nil)
	admin := engagementPerms("a@wso2.com", engagementAdmin, nil)
	if _, err := svc.Get(context.Background(), admin, "E1"); err != nil {
		t.Fatal(err)
	}
	first := hr.calls.Load()
	if first == 0 {
		t.Fatal("no HR lookups")
	}
	if _, err := svc.Get(context.Background(), admin, "E1"); err != nil {
		t.Fatal(err)
	}
	if hr.calls.Load() != first {
		t.Errorf("HR lookups = %d after the second view, want %d", hr.calls.Load(), first)
	}
}

// engagementFailingLineItems has ten opportunities whose line items fail.
type engagementFailingLineItems struct {
	engagementFakeSales
	calls *atomic.Int32
}

func (engagementFailingLineItems) Opportunities(context.Context, string, bool) ([]salesentity.SalesOpportunity, error) {
	out := make([]salesentity.SalesOpportunity, 10)
	for i := range out {
		out[i] = salesentity.SalesOpportunity{ID: fmt.Sprintf("006%d", i)}
	}
	return out, nil
}

func (s engagementFailingLineItems) OpportunityLineItems(ctx context.Context, _ string) ([]salesentity.SalesLineItem, error) {
	s.calls.Add(1)
	return nil, &apierror.ServiceUnavailableError{Msg: "down"}
}

func TestReferenceOpportunities_FirstLineItemFailureStopsTheRest(t *testing.T) {
	sales := engagementFailingLineItems{calls: &atomic.Int32{}}
	refs := NewReferenceService(engagementTestEntity(), sales, engagementFakeHR{})
	if _, err := refs.Opportunities(context.Background(), "001A", domain.EngagementTypePaidPostSale); err == nil {
		t.Fatal("want the line item error")
	}
	// At most the four already running when the first failed.
	if n := sales.calls.Load(); n > 4 {
		t.Errorf("line item calls = %d, want at most 4", n)
	}
}
