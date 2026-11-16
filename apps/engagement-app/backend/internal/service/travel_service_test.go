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
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
)

// ── Fakes ─────────────────────────────────────────────────────────────────

type travelFakeEntity struct {
	engagemententity.TravelAPI
	mu        sync.Mutex
	trips     map[string]engagemententity.TravelRequestRecord
	visas     map[int]engagemententity.TravelVisaRecord
	docs      map[int]engagemententity.TravelDocumentRecord
	searches  []engagemententity.TravelRequestSearch
	page      engagemententity.TravelRequestPage
	writes    []engagemententity.TravelRequestWrite
	visaWrite *engagemententity.TravelVisaCreate
	docWrite  *engagemententity.TravelDocumentCreate
	visaQuery engagemententity.TravelVisaFilter
	audits    []engagemententity.TravelEmailAudit
	perDiems  []engagemententity.TravelActivePerDiem
}

func travelNotFoundErr() error {
	return &apierror.UpstreamError{Service: "engagement-entity", StatusCode: 404, Msg: "not found"}
}

func (f *travelFakeEntity) SearchTravelRequests(_ context.Context, s engagemententity.TravelRequestSearch) (engagemententity.TravelRequestPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches = append(f.searches, s)
	return f.page, nil
}

func (f *travelFakeEntity) GetTravelRequest(_ context.Context, jn string) (engagemententity.TravelRequestRecord, error) {
	if t, ok := f.trips[jn]; ok {
		return t, nil
	}
	return engagemententity.TravelRequestRecord{}, travelNotFoundErr()
}

func (f *travelFakeEntity) UpdateTravelRequest(_ context.Context, jn string, in engagemententity.TravelRequestWrite) (engagemententity.TravelRequestRecord, error) {
	f.writes = append(f.writes, in)
	t := f.trips[jn]
	if in.Status != nil {
		t.Status = *in.Status
	}
	if in.TravelAgentID != nil {
		t.TravelAgentID = in.TravelAgentID
		for _, a := range travelAgents {
			if a.ID == *in.TravelAgentID {
				name := a.Name
				t.TravelAgentName = &name
			}
		}
	}
	if in.VisaID != nil {
		t.VisaID = in.VisaID
		t.Visa = &engagemententity.TravelVisaRef{ID: *in.VisaID, VisaType: travelP("Business/Visit")}
	}
	f.trips[jn] = t
	return t, nil
}

var travelAgents = []engagemententity.TravelAgentRecord{{ID: 1, Name: "Classic Travels"}, {ID: 2, Name: "Acorn Travels"}}

func (f *travelFakeEntity) TravelAgencies(context.Context, string) ([]engagemententity.TravelAgentRecord, error) {
	return travelAgents, nil
}

func (f *travelFakeEntity) TravelActivePerDiems(context.Context, string) ([]engagemententity.TravelActivePerDiem, error) {
	return f.perDiems, nil
}

func (f *travelFakeEntity) InsertTravelEmailAudit(_ context.Context, in engagemententity.TravelEmailAudit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, in)
	return nil
}

func (f *travelFakeEntity) GetVisa(_ context.Context, id int) (engagemententity.TravelVisaRecord, error) {
	if v, ok := f.visas[id]; ok {
		return v, nil
	}
	return engagemententity.TravelVisaRecord{}, travelNotFoundErr()
}

func (f *travelFakeEntity) SearchVisas(_ context.Context, q engagemententity.TravelVisaFilter) (engagemententity.TravelVisaPage, error) {
	f.visaQuery = q
	var out []engagemententity.TravelVisaRecord
	for _, v := range f.visas {
		out = append(out, v)
	}
	return engagemententity.TravelVisaPage{Items: out, Total: len(out)}, nil
}

func (f *travelFakeEntity) CreateVisa(_ context.Context, in engagemententity.TravelVisaCreate) (engagemententity.TravelVisaRecord, error) {
	f.visaWrite = &in
	return engagemententity.TravelVisaRecord{ID: 900, Email: &in.Email, Country: &in.Country, Status: travelP("Active"),
		EndDate: &in.EndDate, ImageID: &in.ImageID}, nil
}

func (f *travelFakeEntity) UpdateVisa(_ context.Context, id int, in engagemententity.TravelVisaUpdate) (engagemententity.TravelVisaRecord, error) {
	v := f.visas[id]
	v.Status, v.EndDate = &in.Status, &in.EndDate
	return v, nil
}

func (f *travelFakeEntity) GetTravelDocument(_ context.Context, id int) (engagemententity.TravelDocumentRecord, error) {
	if d, ok := f.docs[id]; ok {
		return d, nil
	}
	return engagemententity.TravelDocumentRecord{}, travelNotFoundErr()
}

func (f *travelFakeEntity) SearchTravelDocuments(context.Context, engagemententity.TravelDocumentFilter) (engagemententity.TravelDocumentPage, error) {
	var out []engagemententity.TravelDocumentRecord
	for _, d := range f.docs {
		out = append(out, d)
	}
	return engagemententity.TravelDocumentPage{Items: out, Total: len(out)}, nil
}

func (f *travelFakeEntity) CreateTravelDocument(_ context.Context, in engagemententity.TravelDocumentCreate) (engagemententity.TravelDocumentRecord, error) {
	f.docWrite = &in
	return engagemententity.TravelDocumentRecord{ID: 77, Email: &in.Email, DocumentNo: &in.DocumentNo, Status: "Active"}, nil
}

type travelFakeHR struct {
	hrentity.Unconfigured
	fail bool
}

func (h travelFakeHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	if h.fail {
		return nil, errors.New("hr down")
	}
	company := "WSO2 - UK"
	loc := "Colombo"
	return &hrentity.Employee{WorkEmail: e, FirstName: "Chanuka", LastName: "Perera", Location: &loc, Company: &company}, nil
}

func (travelFakeHR) SearchEmployees(context.Context, string, []string, int) ([]hrentity.Employee, error) {
	return nil, nil
}

type travelFakeMail struct {
	mu   sync.Mutex
	sent []email.Message
	fail bool
}

func (m *travelFakeMail) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("email service down")
	}
	m.sent = append(m.sent, msg)
	return nil
}

type travelMemStore struct {
	blobs map[string][]byte
	off   bool
}

func (s *travelMemStore) Put(_ context.Context, key, _ string, r io.Reader, _ int64) error {
	if s.off {
		return filestore.ErrNotConfigured
	}
	b, _ := io.ReadAll(r)
	s.blobs[key] = b
	return nil
}

func (s *travelMemStore) Get(_ context.Context, key string) (io.ReadCloser, filestore.Object, error) {
	if s.off {
		return nil, filestore.Object{}, filestore.ErrNotConfigured
	}
	b, ok := s.blobs[key]
	if !ok {
		return nil, filestore.Object{}, filestore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), filestore.Object{Key: key}, nil
}

func travelP[T any](v T) *T { return &v }

var travelNow = func() time.Time { return time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC) }

func travelPerms(email string, ids ...int) domain.Perms {
	return domain.Perms{Email: email, Travel: domain.NewPrivilegeSet(ids), Alloc: domain.PrivilegeSet{}}
}

// Production role sets (screens-travel-dash §0).
var (
	travelTeam    = travelPerms("shivanga@wso2.com", 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 25, 26, 27, 28, 30, 32, 41, 42, 45, 46)
	travelFinance = travelPerms("udanee@wso2.com", 2, 3, 8, 28, 30, 33, 34, 35, 39, 40, 41, 42, 43, 44, 45)
	travelEmp     = travelPerms("chanuka@wso2.com", 19, 20, 21, 22, 23, 24, 36, 37, 38, 47, 48, 49)
)

func travelAgent() domain.Perms {
	p := travelPerms("agent@acorn.com", 3, 4, 6, 18, 26, 27, 28, 30, 31, 45, 46)
	p.Agency = &domain.TravelAgency{ID: 2, Name: "Acorn Travels"}
	return p
}

func travelTrip(status string, agent *int) engagemententity.TravelRequestRecord {
	t := engagemententity.TravelRequestRecord{
		JobNumber: "TRV-2026-10-1246", Status: status, TravellerEmail: "chanuka@wso2.com", City: travelP("Singapore"),
		Country: "Singapore", CustomerName: "WSO2", StartDate: "2026-10-06", EndDate: "2026-10-16",
		AllocationID: travelP("A2609100005"), EngagementCode: "MKT-01", EngagementType: "Marketing Event",
		TravelAgentID: agent,
		Engagement: &engagemententity.TravelEngagementRecord{ID: "EVT00012", Title: travelP("Integration Summit Singapore 2026"),
			CustomerName: travelP("WSO2"), Kind: travelP("Internal"), TypeName: travelP("Marketing Event"),
			CampaignID: travelP("7015g00000CkR08AAF")},
		FundingSources: []engagemententity.TravelFundingSourceRecord{{Region: travelP("APAC"), BusinessUnit: travelP("Integration"), Percentage: travelP(100.0)}},
	}
	if agent != nil {
		t.TravelAgentName = travelP(map[int]string{1: "Classic Travels", 2: "Acorn Travels"}[*agent])
	}
	return t
}

type travelFixture struct {
	entity *travelFakeEntity
	mail   *travelFakeMail
	files  *travelMemStore
	svc    TravelService
	docs   TravelDocumentsService
}

func newTravelFixture(trip engagemententity.TravelRequestRecord) *travelFixture {
	f := &travelFixture{
		entity: &travelFakeEntity{
			trips: map[string]engagemententity.TravelRequestRecord{trip.JobNumber: trip},
			visas: map[int]engagemententity.TravelVisaRecord{
				301: {ID: 301, Email: travelP("chanuka@wso2.com"), Country: travelP("SG"), VisaType: travelP("Business/Visit"),
					EntryType: travelP("Multiple"), StartDate: travelP("2026-01-10"), EndDate: travelP("2028-01-09"),
					Status: travelP("Active"), ImageID: travelP("chanuka@wso2.com_u_SG.png")},
				302: {ID: 302, Email: travelP("someone@wso2.com"), Status: travelP("Active")},
			},
			docs: map[int]engagemententity.TravelDocumentRecord{
				201: {ID: 201, Email: travelP("chanuka@wso2.com"), DocumentType: travelP("Passport"), DocumentNo: travelP("N1234913"),
					IssuedCountry: travelP("LK"), ExpiryDate: travelP("2032-05-10"), Status: "Active"},
			},
		},
		mail:  &travelFakeMail{},
		files: &travelMemStore{blobs: map[string][]byte{}},
	}
	opts := TravelOptions{
		VisaNotApplicableID: 332, NoValidVisaID: 333, AuditEmails: true, Now: travelNow,
		Email: email.TravelSettings{LogisticsGroup: "travel-logistics@wso2.com", FinanceGroup: "finance@wso2.com",
			AppBaseURL: "https://one.wso2.com", CompanyRecipients: map[string][]string{"WSO2 - UK": {"uk-entity@wso2.com", "uk-pay@wso2.com"}}},
	}
	f.svc = NewTravelService(f.entity, travelFakeHR{}, f.mail, opts)
	f.docs = NewTravelDocumentsService(f.entity, travelFakeHR{}, f.files, travelNow, 332, 333)
	return f
}

func travelIs[T error](err error) bool {
	var target T
	return errors.As(err, &target)
}

// ── Access ────────────────────────────────────────────────────────────────

func TestTravelScopeTabsAndActions(t *testing.T) {
	acorn, classic := 2, 1
	if s := travelScope(travelTeam, "x@wso2.com", nil); s != domain.TravelScopeFull {
		t.Errorf("team scope %q", s)
	}
	if s := travelScope(travelAgent(), "x@wso2.com", &acorn); s != domain.TravelScopeAgency {
		t.Errorf("agent own agency %q", s)
	}
	if s := travelScope(travelAgent(), "x@wso2.com", &classic); s != "" {
		t.Errorf("agent other agency %q", s)
	}
	if s := travelScope(travelEmp, "Chanuka@wso2.com", nil); s != domain.TravelScopeOwn {
		t.Errorf("traveller %q", s)
	}
	if s := travelScope(travelEmp, "other@wso2.com", nil); s != "" {
		t.Errorf("stranger %q", s)
	}

	// Travel Team on a New request: Assign and Update; agent and visa
	// editable because the status may move to In Progress.
	a := travelActions(travelTeam, domain.TravelScopeFull, domain.TravelStatusNew)
	if !a.Assign || !a.Update || !a.EditStatus || !a.EditAgency || !a.EditVisa || a.AddPerDiem {
		t.Errorf("team new = %+v", a)
	}
	// Agent: no agent change, own invoices tab, no cards or payments.
	a = travelActions(travelAgent(), domain.TravelScopeAgency, domain.TravelStatusInProgress)
	if a.Assign || !a.Update || !a.EditStatus || a.EditAgency || !a.EditVisa || !a.AddPerDiem || !a.AddInvoice ||
		a.AddTravelCard || a.PushPayments {
		t.Errorf("agent = %+v", a)
	}
	if tabs := travelTabs(travelAgent(), domain.TravelScopeAgency); !tabs.PerDiems || !tabs.Invoices || tabs.TravelCards || tabs.Payments {
		t.Errorf("agent tabs = %+v", tabs)
	}
	// Finance approver: finance step (no HR approval), push invoices (L6: 34).
	a = travelActions(travelFinance, domain.TravelScopeFull, domain.TravelStatusExtended)
	if !a.ApproveInvoices || a.ApproveInvoicesPo || !a.PushInvoices || !a.AddPayment || !a.PushPayments || a.Update {
		t.Errorf("finance = %+v", a)
	}
	// Travel Team holds HR approval, so not the finance step.
	if a := travelActions(travelTeam, domain.TravelScopeFull, domain.TravelStatusInProgress); !a.ApproveInvoicesPo || a.ApproveInvoices {
		t.Errorf("team invoices = %+v", a)
	}
	// Add Invoice also on Cancelled; nothing else there.
	agent := travelPerms("a@x.com", 30, 31)
	if a := travelActions(agent, domain.TravelScopeFull, domain.TravelStatusCancelled); !a.AddInvoice || a.ApproveInvoicesPo {
		t.Errorf("cancelled = %+v", a)
	}
	// Own scope is read-only; tabs follow the "View Own" privileges.
	if a := travelActions(travelEmp, domain.TravelScopeOwn, domain.TravelStatusInProgress); a != (domain.TravelRequestActions{}) {
		t.Errorf("own = %+v", a)
	}
	if tabs := travelTabs(travelEmp, domain.TravelScopeOwn); !tabs.PerDiems || !tabs.Invoices || !tabs.TravelCards || !tabs.Payments {
		t.Errorf("own tabs = %+v", tabs)
	}
}

// ── Lists ─────────────────────────────────────────────────────────────────

func TestTravelList_AgentIsForcedToItsAgency(t *testing.T) {
	agency := 2
	f := newTravelFixture(travelTrip(domain.TravelStatusInProgress, &agency))
	f.entity.page = engagemententity.TravelRequestPage{
		Items: []engagemententity.TravelRequestRecord{travelTrip(domain.TravelStatusInProgress, &agency)}, Total: 1,
		StatusCounts: map[string]int{"In Progress": 1, "Completed": 2},
	}
	page, err := f.svc.List(context.Background(), travelAgent(), domain.TravelListQuery{
		Search: "acme", Agencies: []string{"Classic Travels"}, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := f.entity.searches[0]
	if s.TravelAgentID == nil || *s.TravelAgentID != 2 || s.SearchEngagements || s.SearchAgency || s.AgencyNames != nil ||
		s.Sort != engagemententity.TravelSortCurrentFirst || s.Today != "2026-10-05" || !s.WithStatusCounts {
		t.Errorf("search = %+v", s)
	}
	row := page.Items[0]
	if row.Agency != nil || row.Engagement != nil || row.Campaign != nil || !row.Actions.Update || row.Actions.EditAgency {
		t.Errorf("agent row = %+v", row)
	}
	if _, has := page.SegmentCounts["new"]; has || page.SegmentCounts["in-progress"] != 1 ||
		page.SegmentCounts["completed"] != 2 || page.SegmentCounts["all"] != 3 {
		t.Errorf("counts = %v", page.SegmentCounts)
	}
	if _, err := f.svc.List(context.Background(), travelAgent(), domain.TravelListQuery{Segment: "new"}); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("agent new segment: %v", err)
	}
	noAgency := travelAgent()
	noAgency.Agency = nil
	if page, err := f.svc.List(context.Background(), noAgency, domain.TravelListQuery{}); err != nil || len(page.Items) != 0 || len(f.entity.searches) != 1 {
		t.Errorf("no agency: %+v %v", page, err)
	}
}

func TestTravelList_TravelTeamSegmentsAndRows(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	visa := 332
	rec := travelTrip(domain.TravelStatusNew, nil)
	rec.VisaID = &visa
	f.entity.page = engagemententity.TravelRequestPage{Items: []engagemententity.TravelRequestRecord{rec}, Total: 1,
		StatusCounts: map[string]int{"New": 4, "In Progress": 2, "": 1}}
	page, err := f.svc.List(context.Background(), travelTeam, domain.TravelListQuery{Segment: "new", Agencies: []string{"Acorn Travels"}})
	if err != nil {
		t.Fatal(err)
	}
	s := f.entity.searches[0]
	if len(s.Statuses) != 1 || s.Statuses[0] != "New" || !s.SearchEngagements || !s.SearchAgency || s.AgencyNames[0] != "Acorn Travels" {
		t.Errorf("search = %+v", s)
	}
	row := page.Items[0]
	if row.Traveller.Name == nil || *row.Traveller.Name != "Chanuka Perera" || *row.BaseLocation != "Colombo" ||
		*row.Visa != "Visa Not Applicable" || row.Engagement == nil || row.Engagement.Account != nil ||
		row.Campaign == nil || row.Campaign.ID == "" || !row.Actions.Assign {
		t.Errorf("row = %+v", row)
	}
	if page.SegmentCounts["new"] != 4 || page.SegmentCounts["all"] != 7 {
		t.Errorf("counts = %v", page.SegmentCounts)
	}

	// Segment and status filter do not overlap: no rows, counts kept.
	page, err = f.svc.List(context.Background(), travelTeam, domain.TravelListQuery{Segment: "completed", Statuses: []string{"New"}})
	if err != nil || len(page.Items) != 0 || page.Total != 0 || page.SegmentCounts["new"] != 4 || page.SegmentCounts["all"] != 4 {
		t.Errorf("empty intersection: %+v %v", page, err)
	}
	for _, q := range []domain.TravelListQuery{{Segment: "later"}, {Statuses: []string{"Approved"}}} {
		if _, err := f.svc.List(context.Background(), travelTeam, q); !travelIs[*apierror.ValidationError](err) {
			t.Errorf("%+v: %v", q, err)
		}
	}
}

func TestTravelListMine(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusInProgress, nil))
	f.entity.page = engagemententity.TravelRequestPage{Items: []engagemententity.TravelRequestRecord{travelTrip(domain.TravelStatusInProgress, nil)}, Total: 1}
	page, err := f.svc.ListMine(context.Background(), travelTeam, domain.TravelListQuery{Search: "summit"})
	if err != nil {
		t.Fatal(err)
	}
	s := f.entity.searches[0]
	if s.TravellerEmails[0] != "shivanga@wso2.com" || s.Sort != engagemententity.TravelSortStartDesc || !s.SearchEngagements {
		t.Errorf("search = %+v", s)
	}
	if page.Items[0].Actions != (domain.TravelRequestActions{}) || page.SegmentCounts != nil {
		t.Errorf("own rows must be read-only: %+v", page)
	}
}

// ── Page ──────────────────────────────────────────────────────────────────

func TestTravelGet_Scopes(t *testing.T) {
	acorn := 2
	f := newTravelFixture(travelTrip(domain.TravelStatusInProgress, &acorn))
	ctx := context.Background()

	d, err := f.svc.Get(ctx, travelFinance, "TRV-2026-10-1246")
	if err != nil {
		t.Fatal(err)
	}
	if d.Scope != "full" || len(d.FundingSources) != 1 || *d.EngagementType != "Marketing Event" || *d.AllocationID != "A2609100005" ||
		d.Campaign == nil || d.Campaign.ID != "7015g00000CkR08AAF" || !d.Tabs.Payments {
		t.Errorf("finance = %+v", d)
	}
	d, err = f.svc.Get(ctx, travelAgent(), "TRV-2026-10-1246")
	if err != nil || d.Scope != "agency" || d.Engagement != nil || d.FundingSources != nil || d.AllocationID != nil ||
		d.Agency != nil || d.Tabs.TravelCards {
		t.Errorf("agent = %+v %v", d, err)
	}
	d, err = f.svc.Get(ctx, travelEmp, "TRV-2026-10-1246")
	if err != nil || d.Scope != "own" || d.Engagement == nil || d.FundingSources != nil || d.Actions.Update {
		t.Errorf("own = %+v %v", d, err)
	}
	if _, err := f.svc.Get(ctx, travelPerms("x@wso2.com", 36), "TRV-2026-10-1246"); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("stranger: %v", err)
	}
	if _, err := f.svc.Get(ctx, travelTeam, "TRV-2026-10-0000"); !travelIs[*apierror.NotFoundError](err) {
		t.Errorf("missing: %v", err)
	}
	if _, err := f.svc.Get(ctx, travelTeam, "E07201"); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("bad job number: %v", err)
	}
}

func TestTravelGet_HRDownStillOpens(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	svc := NewTravelService(f.entity, travelFakeHR{fail: true}, f.mail, TravelOptions{Now: travelNow})
	d, err := svc.Get(context.Background(), travelTeam, "TRV-2026-10-1246")
	if err != nil || d.Traveller.Name != nil || d.BaseLocation != nil {
		t.Errorf("detail %+v err %v", d, err)
	}
}

func TestTravelVisaOptions(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	f.entity.visas = map[int]engagemententity.TravelVisaRecord{301: f.entity.visas[301]}
	opts, err := f.svc.VisaOptions(context.Background(), travelTeam, "TRV-2026-10-1246")
	if err != nil {
		t.Fatal(err)
	}
	q := f.entity.visaQuery
	if q.Emails[0] != "chanuka@wso2.com" || q.Status != "Active" || q.CoversFrom != "2026-10-06" || q.CoversTo != "2026-10-16" || q.ExcludeIDs != nil {
		t.Errorf("query = %+v", q)
	}
	if len(opts.Items) != 3 || opts.Items[0].Value != "301" ||
		opts.Items[0].Label != "SG - Business/Visit - Multiple - (2026-01-10 to 2028-01-09)" ||
		opts.Items[1].Value != "No Valid Visa Available" || opts.Items[2].Value != "Visa Not Applicable" {
		t.Errorf("options = %+v", opts.Items)
	}
	if _, err := f.svc.VisaOptions(context.Background(), travelEmp, "TRV-2026-10-1246"); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("traveller: %v", err)
	}
}

// The placeholder visas are left out of both visa lists.
func TestTravelListVisas_HidesPlaceholders(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	for _, own := range []bool{false, true} {
		if _, err := f.docs.ListVisas(context.Background(), travelTeam, domain.TravelRecordQuery{Limit: 10}, own); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f.entity.visaQuery.ExcludeIDs, []int{332, 333}) {
			t.Errorf("own %v: exclude = %v", own, f.entity.visaQuery.ExcludeIDs)
		}
	}
}

// Only the Expired filter asks the entity for the visas marked Expired.
func TestTravelListVisas_ExpiredFilterIncludesMarkedExpired(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	for status, want := range map[string]bool{"": false, "Active": false, "Expired": true} {
		q := domain.TravelRecordQuery{Status: status, Limit: 10}
		if _, err := f.docs.ListVisas(context.Background(), travelTeam, q, false); err != nil {
			t.Fatal(err)
		}
		if got := f.entity.visaQuery.IncludeMarkedExpired; got != want {
			t.Errorf("status %q: includeMarkedExpired = %v, want %v", status, got, want)
		}
	}
}

// ── Update ────────────────────────────────────────────────────────────────

func TestTravelAssign_InProgressSendsAssignedEmail(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	ctx := context.Background()
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{
		Status: travelP("In Progress"), Agency: travelP("Acorn Travels")}); !travelIs[*apierror.ValidationError](err) {
		t.Fatalf("missing visa: %v", err)
	}
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("Completed")}); !travelIs[*apierror.ValidationError](err) {
		t.Fatalf("assign completed: %v", err)
	}
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{
		Status: travelP("In Progress"), Agency: travelP("Acorn Travels"), Visa: travelP("302")}); !travelIs[*apierror.ValidationError](err) {
		t.Fatalf("someone else's visa: %v", err)
	}
	d, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{
		Status: travelP("In Progress"), Agency: travelP("acorn travels"), Visa: travelP("301")})
	if err != nil {
		t.Fatal(err)
	}
	w := f.entity.writes[0]
	if *w.Status != "In Progress" || *w.TravelAgentID != 2 || *w.VisaID != 301 || w.UpdatedBy != "shivanga@wso2.com" {
		t.Errorf("write = %+v", w)
	}
	if d.Status != "In Progress" || *d.Agency != "Acorn Travels" {
		t.Errorf("detail = %+v", d)
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("sent %d emails", len(f.mail.sent))
	}
	m := f.mail.sent[0]
	if m.TemplateID != "assignRequests" || m.Subject != "[Travel App] Travel Request (#TRV-2026-10-1246)" ||
		m.To[0] != "travel-logistics@wso2.com" || len(m.CC) != 0 || m.Content["title"] != "New Travel Request Assigned" ||
		m.Content["travel_agent_name"] != "Acorn Travels" || m.Content["visa_type"] != "Business/Visit" ||
		m.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-requests/TRV-2026-10-1246/info" {
		t.Errorf("email = %+v", m)
	}
	if len(f.entity.audits) != 1 || f.entity.audits[0].Recipients != "travel-logistics@wso2.com" ||
		!strings.Contains(f.entity.audits[0].ContentKeyValPairs, `"APP_NAME":"Travel App"`) {
		t.Errorf("audits = %+v", f.entity.audits)
	}
}

func TestTravelAssign_CancelledAndOnHold(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	if _, err := f.svc.Update(context.Background(), travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("On Hold")}); err != nil {
		t.Fatal(err)
	}
	if len(f.mail.sent) != 0 {
		t.Errorf("legacy assign On Hold sends nothing; sent %+v", f.mail.sent)
	}
	f = newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	if _, err := f.svc.Update(context.Background(), travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("Cancelled")}); err != nil {
		t.Fatal(err)
	}
	if len(f.mail.sent) != 1 || f.mail.sent[0].Content["travel_agent_name"] != "All" || f.mail.sent[0].TemplateID != "noContent" {
		t.Errorf("sent %+v", f.mail.sent)
	}
}

func TestTravelUpdate_RulesAndEmails(t *testing.T) {
	acorn := 2
	ctx := context.Background()
	open := func() *travelFixture {
		trip := travelTrip(domain.TravelStatusInProgress, &acorn)
		trip.VisaID = travelP(301)
		trip.Visa = &engagemententity.TravelVisaRef{ID: 301, VisaType: travelP("Business/Visit")}
		return newTravelFixture(trip)
	}

	f := open()
	// The transition matrix: In Progress cannot go back to New.
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("New")}); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("matrix: %v", err)
	}
	// An agent may not change the agent (no Travel 5).
	if _, err := f.svc.Update(ctx, travelAgent(), "TRV-2026-10-1246", domain.TravelRequestUpdate{Agency: travelP("Classic Travels")}); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("agent changes agent: %v", err)
	}
	// Finance (no 4, 5 or 6) may not change anything.
	if _, err := f.svc.Update(ctx, travelFinance, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("On Hold")}); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("finance: %v", err)
	}
	// The agent can change only while the new status is open.
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{
		Status: travelP("On Hold"), Agency: travelP("Classic Travels")}); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("agent while on hold: %v", err)
	}
	// Nothing changes: no write, no email.
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("In Progress")}); err != nil ||
		len(f.entity.writes) != 0 || len(f.mail.sent) != 0 {
		t.Errorf("no-op: %v writes %d", err, len(f.entity.writes))
	}

	// Agent change: "Cancelled for you" to the old agent, assigned to the new.
	f = open()
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{
		Agency: travelP("Classic Travels"), Visa: travelP("Visa Not Applicable")}); err != nil {
		t.Fatal(err)
	}
	if w := f.entity.writes[0]; w.Status != nil || *w.TravelAgentID != 1 || *w.VisaID != 332 {
		t.Errorf("write = %+v", w)
	}
	if len(f.mail.sent) != 2 || f.mail.sent[0].Content["travel_agent_name"] != "Acorn Travels" ||
		f.mail.sent[0].Content["description"] != "Please note that this travel request has been Cancelled for you." ||
		f.mail.sent[1].Content["title"] != "New Travel Request Assigned" || f.mail.sent[1].Content["travel_agent_name"] != "Classic Travels" {
		t.Errorf("sent = %+v", f.mail.sent)
	}

	// Visa change by the agent: "Visa has been changed".
	f = open()
	if _, err := f.svc.Update(ctx, travelAgent(), "TRV-2026-10-1246", domain.TravelRequestUpdate{Visa: travelP("333")}); err != nil {
		t.Fatal(err)
	}
	if len(f.mail.sent) != 1 || f.mail.sent[0].Content["title"] != "Visa has been changed" {
		t.Errorf("sent = %+v", f.mail.sent)
	}

	// Completed: per diem table and the company's finance recipients.
	f = open()
	f.entity.perDiems = []engagemententity.TravelActivePerDiem{{StartDate: travelP("2026-10-06 09:00:00"),
		EndDate: travelP("2026-10-16 18:00:00"), NoOfDays: travelP(11), Currency: travelP("GBP"), PerDiemValue: 70, Total: travelP(770.0)}}
	if _, err := f.svc.Update(ctx, travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("Completed")}); err != nil {
		t.Fatal(err)
	}
	m := f.mail.sent[0]
	if m.TemplateID != "completeTravel" || strings.Join(m.To, ",") != "travel-logistics@wso2.com,uk-entity@wso2.com" ||
		strings.Join(m.CC, ",") != "uk-pay@wso2.com" || !strings.Contains(m.Content["table_data"], "2026-10-06 09:00") ||
		!strings.Contains(m.Content["table_data"], "770.0") {
		t.Errorf("completed = %+v", m)
	}
}

func TestTravelUpdate_RequiresAgentAndVisa(t *testing.T) {
	trip := travelTrip(domain.TravelStatusOnHold, nil)
	f := newTravelFixture(trip)
	_, err := f.svc.Update(context.Background(), travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("In Progress")})
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || ve.Msg != "Please select a Travel Agent and a Visa" {
		t.Errorf("err = %v", err)
	}
}

func TestTravelUpdate_EmailFailureDoesNotFail(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	f.mail.fail = true
	if _, err := f.svc.Update(context.Background(), travelTeam, "TRV-2026-10-1246", domain.TravelRequestUpdate{Status: travelP("Cancelled")}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(f.entity.writes) != 1 || len(f.entity.audits) != 0 {
		t.Errorf("writes %d audits %d", len(f.entity.writes), len(f.entity.audits))
	}
}

// ── Visas and documents ───────────────────────────────────────────────────

var travelPNG = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)

func TestTravelAddVisa_StoresLegacyBlobAndInserts(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	ctx := context.Background()
	form := domain.TravelVisaForm{TravellerEmail: "Chanuka@wso2.com", Country: "SG", VisaType: "Business/Visit", EntryType: "Single",
		IssuedDate: "2026-01-01", StartDate: "2026-01-02", EndDate: "2026-12-31", PassportID: travelP(201),
		Image: domain.TravelImage{Data: travelPNG, Present: true}}
	v, err := f.docs.AddVisa(ctx, travelTeam, form, false)
	if err != nil {
		t.Fatal(err)
	}
	w := f.entity.visaWrite
	if w.Email != "chanuka@wso2.com" || w.CreatedBy != "shivanga@wso2.com" || !strings.HasPrefix(w.ImageID, "chanuka@wso2.com_") ||
		!strings.HasSuffix(w.ImageID, "_SG.png") {
		t.Errorf("write = %+v", w)
	}
	blob := f.files.blobs["visas/"+w.ImageID]
	if !strings.HasPrefix(string(blob), "data:image/png;base64,") {
		t.Errorf("blob = %.40s", blob)
	}
	if v.ID != 900 || !v.HasImage || v.Status != "Active" {
		t.Errorf("visa = %+v", v)
	}

	// Own add: owner is the caller whatever the form says.
	if _, err := f.docs.AddVisa(ctx, travelEmp, form, true); err != nil || f.entity.visaWrite.Email != "chanuka@wso2.com" {
		t.Errorf("own: %v", err)
	}
	bad := form
	bad.PassportID = nil
	if _, err := f.docs.AddVisa(ctx, travelTeam, bad, false); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("no passport: %v", err)
	}
	bad = form
	bad.Image = domain.TravelImage{Data: []byte("%PDF-1.4 hello"), Present: true}
	if _, err := f.docs.AddVisa(ctx, travelTeam, bad, false); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("pdf: %v", err)
	}
	bad = form
	bad.StartDate = "2027-01-01"
	if _, err := f.docs.AddVisa(ctx, travelTeam, bad, false); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("dates: %v", err)
	}
	f.files.off = true
	if _, err := f.docs.AddVisa(ctx, travelTeam, form, false); !travelIs[*apierror.ServiceUnavailableError](err) {
		t.Errorf("no file storage: %v", err)
	}
}

func TestTravelVisaImageAndAccess(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	ctx := context.Background()
	f.files.blobs["visas/chanuka@wso2.com_u_SG.png"] = filestore.EncodeLegacy("image/png", travelPNG)

	ct, data, err := f.docs.VisaImage(ctx, travelEmp, 301)
	if err != nil || ct != "image/png" || !bytes.Equal(data, travelPNG) {
		t.Fatalf("owner image: %s %v", ct, err)
	}
	if _, _, err := f.docs.VisaImage(ctx, travelFinance, 301); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("finance (no 9): %v", err)
	}
	// The agent sees the image only when its agency has a request using it.
	f.entity.page = engagemententity.TravelRequestPage{Total: 1}
	if _, _, err := f.docs.VisaImage(ctx, travelAgent(), 301); err != nil {
		t.Errorf("agent with request: %v", err)
	}
	s := f.entity.searches[len(f.entity.searches)-1]
	if *s.VisaID != 301 || *s.TravelAgentID != 2 {
		t.Errorf("agent check = %+v", s)
	}
	f.entity.page = engagemententity.TravelRequestPage{}
	if _, _, err := f.docs.VisaImage(ctx, travelAgent(), 301); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("agent without request: %v", err)
	}
	if _, err := f.docs.GetVisa(ctx, travelEmp, 302); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("someone else's visa: %v", err)
	}
	if _, err := f.docs.GetVisa(ctx, travelTeam, 999); !travelIs[*apierror.NotFoundError](err) {
		t.Errorf("missing: %v", err)
	}
	if _, _, err := f.docs.VisaImage(ctx, travelTeam, 302); !travelIs[*apierror.NotFoundError](err) {
		t.Errorf("no image: %v", err)
	}
}

func TestTravelUpdateVisa_ExpiredEndsToday(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	f.entity.visas[301] = func() engagemententity.TravelVisaRecord {
		v := f.entity.visas[301]
		v.PassportID, v.IssuedDate = travelP(201), travelP("2026-01-05")
		return v
	}()
	v, err := f.docs.UpdateVisa(context.Background(), travelTeam, 301, domain.TravelVisaForm{Status: "expired"})
	if err != nil {
		t.Fatal(err)
	}
	if *v.EndDate != "2026-10-05" || v.Status != "Expired" {
		t.Errorf("visa = %+v", v)
	}
}

func TestTravelDocuments(t *testing.T) {
	f := newTravelFixture(travelTrip(domain.TravelStatusNew, nil))
	ctx := context.Background()
	d, err := f.docs.GetDocument(ctx, travelEmp, 201)
	if err != nil || d.MaskedNumber != "N•••••913" || d.DocumentNumber != nil {
		t.Errorf("own document %+v %v", d, err)
	}
	if d, err := f.docs.GetDocument(ctx, travelTeam, 201); err != nil || *d.DocumentNumber != "N1234913" {
		t.Errorf("editor %+v %v", d, err)
	}
	if _, err := f.docs.GetDocument(ctx, travelPerms("x@wso2.com", 23), 201); !travelIs[*apierror.ForbiddenError](err) {
		t.Errorf("stranger: %v", err)
	}
	form := domain.TravelDocumentForm{TravellerEmail: "chanuka@wso2.com", DocumentType: "other id document",
		DocumentNumber: "ID-1", Country: "LK", IssuedDate: "2020-01-01", Image: domain.TravelImage{Data: travelPNG, Present: true}}
	if _, err := f.docs.AddDocument(ctx, travelTeam, form, false); err != nil {
		t.Fatal(err)
	}
	if w := f.entity.docWrite; w.DocumentType != "Other ID Document" || w.ExpiryDate != nil || w.CreatedBy != "shivanga@wso2.com" {
		t.Errorf("write = %+v", w)
	}
	form.DocumentType = "Passport"
	if _, err := f.docs.AddDocument(ctx, travelTeam, form, false); !travelIs[*apierror.ValidationError](err) {
		t.Errorf("passport without expiry: %v", err)
	}
	page, err := f.docs.Passports(ctx, "chanuka@wso2.com")
	if err != nil || len(page.Items) != 1 || page.Items[0].Status != "Active" {
		t.Errorf("passports %+v %v", page, err)
	}
}

func TestTravelMask(t *testing.T) {
	for in, want := range map[string]string{"N1234913": "N•••••913", "AB12": "•••••2", "": "", "Ñ9876543": "Ñ•••••543"} {
		if got := travelMask(&in); got != want {
			t.Errorf("mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTravelPeopleFillNames(t *testing.T) {
	kept := "Kept Name"
	a, b, c := &domain.Person{Email: "a@wso2.com"}, &domain.Person{Email: "A@wso2.com"}, &domain.Person{Email: "c@wso2.com", Name: &kept}
	newTravelPeople(travelFakeHR{}).fillNames(context.Background(), a, b, c, nil)
	if a.Name == nil || *a.Name != "Chanuka Perera" || b.Name == nil || *c.Name != kept {
		t.Fatalf("names = %v %v %v", a.Name, b.Name, *c.Name)
	}
	d := &domain.Person{Email: "d@wso2.com"}
	newTravelPeople(travelFakeHR{fail: true}).fillNames(context.Background(), d)
	var none *travelPeople
	none.fillNames(context.Background(), d)
	if d.Name != nil || d.Email != "d@wso2.com" {
		t.Fatalf("failed lookup: %+v", d)
	}
}
