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
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type travelFakeRequests struct {
	repository.TravelRequestRepository
	rows       []domain.TravelRequest
	lastSearch domain.TravelRequestSearch
	counts     map[string]int
	updated    *domain.UpdateTravelRequestInput
	found      bool
}

func (f *travelFakeRequests) Search(_ context.Context, s domain.TravelRequestSearch) ([]domain.TravelRequest, int, error) {
	f.lastSearch = s
	return append([]domain.TravelRequest{}, f.rows...), len(f.rows), nil
}

func (f *travelFakeRequests) StatusCounts(context.Context, domain.TravelRequestSearch) (map[string]int, error) {
	return f.counts, nil
}

func (f *travelFakeRequests) Get(_ context.Context, jn string) (*domain.TravelRequest, error) {
	for _, r := range f.rows {
		if r.JobNumber == jn {
			return &r, nil
		}
	}
	return nil, nil
}

func (f *travelFakeRequests) Update(_ context.Context, _ string, in domain.UpdateTravelRequestInput) (bool, error) {
	f.updated = &in
	return f.found, nil
}

func (f *travelFakeRequests) Agents(context.Context, string) ([]domain.TravelAgent, error) {
	return []domain.TravelAgent{{ID: 2, Name: "Acorn Travels"}}, nil
}

type travelFakeEngagements struct{ titleSearches int }

func (f *travelFakeEngagements) ByAllocation(_ context.Context, ids []string) (map[string]domain.TravelEngagement, error) {
	out := map[string]domain.TravelEngagement{}
	for _, id := range ids {
		if id == "A1" {
			out[id] = domain.TravelEngagement{ID: "E1"}
		}
	}
	return out, nil
}

func (f *travelFakeEngagements) IDsByTitle(context.Context, string) ([]string, error) {
	f.titleSearches++
	return []string{"E1"}, nil
}

func (f *travelFakeEngagements) FundingSources(context.Context, string) ([]domain.TravelFundingSource, error) {
	return []domain.TravelFundingSource{{}}, nil
}

type travelFakeDocuments struct {
	repository.TravelDocumentRepository
	visas     map[int]domain.TravelVisa
	updateErr error
}

func (f *travelFakeDocuments) GetVisa(_ context.Context, id int) (*domain.TravelVisa, error) {
	if v, ok := f.visas[id]; ok {
		return &v, nil
	}
	return nil, nil
}

func (f *travelFakeDocuments) UpdateVisa(context.Context, int, domain.UpdateTravelVisaInput) error {
	return f.updateErr
}

func (f *travelFakeDocuments) CreateVisa(context.Context, domain.CreateTravelVisaInput) (int, error) {
	return 1, nil
}

func (f *travelFakeDocuments) UpdateDocument(context.Context, int, domain.UpdateTravelDocumentInput) error {
	return f.updateErr
}

func travelStr(s string) *string { return &s }

func newTravelSvc() (*travelFakeRequests, *travelFakeEngagements, *travelFakeDocuments, TravelRequestService) {
	req := &travelFakeRequests{
		rows: []domain.TravelRequest{
			{JobNumber: "TRV-2026-10-0001", AllocationID: travelStr("A1")},
			{JobNumber: "TRV-2026-10-0002", AllocationID: travelStr("A2")},
		},
		counts: map[string]int{"New": 2},
		found:  true,
	}
	eng := &travelFakeEngagements{}
	docs := &travelFakeDocuments{visas: map[int]domain.TravelVisa{301: {ID: 301}}}
	return req, eng, docs, NewTravelRequestService(req, eng, docs)
}

func travelIsValidation(err error) bool {
	var ve *apierror.ValidationError
	return errors.As(err, &ve)
}

func TestTravelService_SearchDefaultsAndOverlay(t *testing.T) {
	req, eng, _, svc := newTravelSvc()
	page, err := svc.Search(context.Background(), domain.TravelRequestSearch{
		Search: " acme ", SearchEngagements: true, WithStatusCounts: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.lastSearch.Limit != 100 || req.lastSearch.Sort != domain.TravelSortCurrentFirst || req.lastSearch.Today == "" ||
		req.lastSearch.Search != "acme" || len(req.lastSearch.EngagementIDs) != 1 || eng.titleSearches != 1 {
		t.Errorf("search = %+v", req.lastSearch)
	}
	if page.Items[0].Engagement == nil || page.Items[0].Engagement.ID != "E1" || page.Items[1].Engagement != nil {
		t.Errorf("overlay = %+v", page.Items)
	}
	if page.StatusCounts["New"] != 2 || page.Total != 2 {
		t.Errorf("page = %+v", page)
	}
}

func TestTravelService_SearchValidation(t *testing.T) {
	_, _, _, svc := newTravelSvc()
	for name, tc := range map[string]struct {
		f    domain.TravelRequestSearch
		want string
	}{
		"status":  {domain.TravelRequestSearch{Statuses: []string{"Approved"}}, `invalid travel status "Approved"`},
		"date":    {domain.TravelRequestSearch{StartFrom: "05/10/2026"}, "startFrom must be a date"},
		"sort":    {domain.TravelRequestSearch{Sort: "job"}, "sort must be current or startDesc"},
		"offset":  {domain.TravelRequestSearch{Offset: -1}, "offset must be 0 or more"},
		"endTo":   {domain.TravelRequestSearch{EndTo: "2026-13-01"}, "endTo must be a date"},
		"deleted": {domain.TravelRequestSearch{Statuses: []string{"Deleted"}}, `invalid travel status "Deleted"`},
	} {
		if _, err := svc.Search(context.Background(), tc.f); !travelIsValidation(err) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want validation %q", name, err, tc.want)
		}
	}
}

func TestTravelService_GetAndUpdate(t *testing.T) {
	req, _, _, svc := newTravelSvc()
	ctx := context.Background()
	d, err := svc.Get(ctx, "TRV-2026-10-0001")
	if err != nil || d.Engagement == nil || len(d.FundingSources) != 1 {
		t.Fatalf("detail %+v err %v", d, err)
	}
	if d, err := svc.Get(ctx, "TRV-2026-10-0002"); err != nil || d.FundingSources == nil || len(d.FundingSources) != 0 {
		t.Fatalf("no engagement: %+v err %v", d, err)
	}
	var nf *apierror.NotFoundError
	if _, err := svc.Get(ctx, "TRV-2026-10-9999"); !errors.As(err, &nf) {
		t.Errorf("missing: err = %v", err)
	}
	if _, err := svc.Get(ctx, "E07201"); !travelIsValidation(err) {
		t.Errorf("bad job number: err = %v", err)
	}

	status, agent, visa, badAgent, badVisa := "In Progress", 2, 301, 9, 9
	if _, err := svc.Update(ctx, "TRV-2026-10-0001", domain.UpdateTravelRequestInput{
		Status: &status, TravelAgentID: &agent, VisaID: &visa, UpdatedBy: " jane@example.com "}); err != nil {
		t.Fatal(err)
	}
	if req.updated.UpdatedBy != "jane@example.com" {
		t.Errorf("updatedBy = %q", req.updated.UpdatedBy)
	}
	for name, in := range map[string]domain.UpdateTravelRequestInput{
		"no actor":  {Status: &status},
		"nothing":   {UpdatedBy: "j@example.com"},
		"bad state": {Status: travelStr("Approved"), UpdatedBy: "j@example.com"},
		"agent":     {TravelAgentID: &badAgent, UpdatedBy: "j@example.com"},
		"visa":      {VisaID: &badVisa, UpdatedBy: "j@example.com"},
	} {
		if _, err := svc.Update(ctx, "TRV-2026-10-0001", in); !travelIsValidation(err) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	req.found = false
	if _, err := svc.Update(ctx, "TRV-2026-10-0001", domain.UpdateTravelRequestInput{Status: &status, UpdatedBy: "j@example.com"}); !errors.As(err, &nf) {
		t.Errorf("not found: err = %v", err)
	}
}

func TestTravelService_AgentsAndAudit(t *testing.T) {
	_, _, _, svc := newTravelSvc()
	if _, err := svc.Agents(context.Background(), "deleted"); !travelIsValidation(err) {
		t.Errorf("err = %v", err)
	}
	if l, err := svc.Agents(context.Background(), "active"); err != nil || len(l.Items) != 1 {
		t.Errorf("list %+v err %v", l, err)
	}
	if err := svc.InsertEmailAudit(context.Background(), domain.TravelEmailAuditInput{}); !travelIsValidation(err) {
		t.Errorf("err = %v", err)
	}
}

func TestTravelDocumentService_Validation(t *testing.T) {
	docs := &travelFakeDocuments{visas: map[int]domain.TravelVisa{1: {ID: 1}}}
	svc := NewTravelDocumentService(docs)
	ctx := context.Background()
	good := domain.CreateTravelVisaInput{Email: "c@example.com", Country: "GB", VisaType: "Business/Visit", EntryType: "Single",
		IssuedDate: "2026-01-01", StartDate: "2026-01-02", EndDate: "2026-12-31", ImageID: "x.png", CreatedBy: "c@example.com"}
	if v, err := svc.CreateVisa(ctx, good); err != nil || v.ID != 1 {
		t.Fatalf("create: %+v %v", v, err)
	}
	bad := good
	bad.StartDate = "2027-01-01"
	if _, err := svc.CreateVisa(ctx, bad); !travelIsValidation(err) {
		t.Errorf("dates: %v", err)
	}
	bad = good
	bad.ImageID = ""
	if _, err := svc.CreateVisa(ctx, bad); !travelIsValidation(err) {
		t.Errorf("image: %v", err)
	}

	upd := domain.UpdateTravelVisaInput{Country: "GB", VisaType: "B", EntryType: "S", IssuedDate: "2026-01-01",
		StartDate: "2026-01-02", EndDate: "2026-12-31", Status: "inactive", UpdatedBy: "a@example.com"}
	if _, err := svc.UpdateVisa(ctx, 1, upd); err != nil {
		t.Fatalf("update: %v", err)
	}
	upd.Status = "Deleted"
	if _, err := svc.UpdateVisa(ctx, 1, upd); !travelIsValidation(err) {
		t.Errorf("status: %v", err)
	}
	upd.Status = "Active"
	docs.updateErr = repository.ErrTravelDuplicateActive
	var ce *apierror.ConflictError
	if _, err := svc.UpdateVisa(ctx, 1, upd); !errors.As(err, &ce) {
		t.Errorf("duplicate: %v", err)
	}
	docs.updateErr = repository.ErrTravelRecordNotFound
	var nf *apierror.NotFoundError
	if _, err := svc.UpdateVisa(ctx, 1, upd); !errors.As(err, &nf) {
		t.Errorf("missing: %v", err)
	}

	if _, err := svc.SearchVisas(ctx, domain.TravelVisaFilter{Validity: "soon"}); !travelIsValidation(err) {
		t.Errorf("validity: %v", err)
	}

	doc := func(typ, no, issued string, expiry *string) domain.CreateTravelDocumentInput {
		return domain.CreateTravelDocumentInput{Email: "c@example.com", DocumentNo: no, DocumentType: typ,
			IssuedCountry: "LK", IssuedDate: issued, ExpiryDate: expiry, ImageID: "i", CreatedBy: "c"}
	}
	for name, tc := range map[string]struct {
		in   domain.CreateTravelDocumentInput
		want string
	}{
		"type":        {doc("Visa", "1", "2020-01-01", nil), "documentType must be"},
		"expiry":      {doc("Passport", "1", "2020-01-01", nil), "expiryDate is required"},
		"issued>exp":  {doc("Passport", "1", "2030-01-01", travelStr("2029-01-01")), "issuedDate must not be after expiryDate"},
		"no document": {doc("Passport", "", "2020-01-01", nil), "documentNo"},
	} {
		if _, err := svc.CreateDocument(ctx, tc.in); !travelIsValidation(err) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want validation %q", name, err, tc.want)
		}
	}
	docs.updateErr = repository.ErrTravelDuplicateActive
	if _, err := svc.UpdateDocument(ctx, 1, domain.UpdateTravelDocumentInput{DocumentType: "Other ID Document",
		IssuedCountry: "LK", IssuedDate: "2020-01-01", Status: "Active", UpdatedBy: "a"}); !errors.As(err, &ce) {
		t.Errorf("doc duplicate: %v", err)
	}
}

func TestNormalizeEmailAudit(t *testing.T) {
	ok := domain.EmailAuditInput{Subject: "s", TemplateID: "t"}
	got, err := normalizeEmailAudit(ok)
	if err != nil || got.Status != domain.EmailAuditSent {
		t.Errorf("empty status = %q err %v, want Sent", got.Status, err)
	}
	got, err = normalizeEmailAudit(domain.EmailAuditInput{Subject: "s", TemplateID: "t", Status: "Sent", ErrorMessage: "x"})
	if err != nil || got.ErrorMessage != "" {
		t.Errorf("a Sent row keeps no error: %+v err %v", got, err)
	}
	got, err = normalizeEmailAudit(domain.EmailAuditInput{Subject: "s", TemplateID: "t", Status: "Failed", ErrorMessage: "x"})
	if err != nil || got.Status != "Failed" || got.ErrorMessage != "x" {
		t.Errorf("Failed row: %+v err %v", got, err)
	}
	for _, bad := range []domain.EmailAuditInput{
		{Subject: "s", TemplateID: "t", Status: "sent"},
		{Subject: "s", TemplateID: "t", Status: "Pending"},
		{Subject: " ", TemplateID: "t"},
	} {
		if _, err := normalizeEmailAudit(bad); !travelIsValidation(err) {
			t.Errorf("%+v: err = %v, want validation", bad, err)
		}
	}
}
