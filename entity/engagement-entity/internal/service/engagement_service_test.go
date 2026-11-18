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
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type engagementFakeRepo struct {
	repository.EngagementRepository
	search    domain.EngagementSearchRequest
	created   domain.EngagementCreateRequest
	updated   domain.EngagementUpdateRequest
	engType   int
	createErr error
	updateErr error
}

func (f *engagementFakeRepo) Search(_ context.Context, req domain.EngagementSearchRequest) ([]domain.Engagement, int, error) {
	f.search = req
	return []domain.Engagement{{ID: "E1"}}, 1, nil
}
func (f *engagementFakeRepo) TypeCounts(context.Context, bool) (map[int]int, error) {
	return map[int]int{3: 1}, nil
}
func (f *engagementFakeRepo) FundingSources(_ context.Context, ids []string) (map[string][]domain.EngagementFundingSource, error) {
	return map[string][]domain.EngagementFundingSource{ids[0]: {{BusinessUnit: "BU", Percentage: 100}}}, nil
}
func (f *engagementFakeRepo) Consultants(context.Context, []string) (map[string][]string, error) {
	return map[string][]string{}, nil
}
func (f *engagementFakeRepo) Get(_ context.Context, id string) (domain.Engagement, error) {
	if id == "E404" {
		return domain.Engagement{}, repository.ErrEngagementNotFound
	}
	return domain.Engagement{ID: id}, nil
}
func (f *engagementFakeRepo) Create(ctx context.Context, req domain.EngagementCreateRequest, hook repository.EngagementBeforeCommit) (string, error) {
	f.created = req
	if f.createErr != nil {
		return "", f.createErr
	}
	if err := hook(ctx, "EVT00510", f.engType); err != nil {
		return "", err
	}
	return "EVT00510", nil
}
func (f *engagementFakeRepo) Update(_ context.Context, _ string, req domain.EngagementUpdateRequest) ([]domain.EngagementAllocation, error) {
	f.updated = req
	return []domain.EngagementAllocation{}, f.updateErr
}

type engagementFakeNetSuite struct {
	name, id string
	err      error
}

func (f *engagementFakeNetSuite) CreateMarketingEvent(_ context.Context, name, id string) error {
	f.name, f.id = name, id
	return f.err
}

func engagementIsValidation(t *testing.T, err error) {
	t.Helper()
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestEngagementSearch_DefaultsAndValidation(t *testing.T) {
	repo := &engagementFakeRepo{}
	svc := NewEngagementService(repo, nil).(*engagementService)
	svc.now = func() time.Time { return time.Date(2026, 11, 5, 23, 0, 0, 0, time.UTC) }
	resp, err := svc.Search(context.Background(), domain.EngagementSearchRequest{IncludeConsultants: true})
	if err != nil {
		t.Fatal(err)
	}
	if repo.search.Limit != 50 || repo.search.Sort != "current" || repo.search.Today != "2026-11-05" {
		t.Errorf("search = %+v", repo.search)
	}
	if resp.Total != 1 || len(resp.Items[0].FundingSources) != 1 || resp.Items[0].Consultants == nil || resp.TypeCounts[3] != 1 {
		t.Errorf("resp = %+v", resp)
	}
	for _, bad := range []domain.EngagementSearchRequest{
		{Limit: 501}, {Offset: -1}, {StartFrom: "2026-13-01"}, {Consultants: "some"}, {Sort: "id"},
	} {
		_, err := svc.Search(context.Background(), bad)
		engagementIsValidation(t, err)
	}
}

func TestEngagementGet_IDFormatAndNotFound(t *testing.T) {
	svc := NewEngagementService(&engagementFakeRepo{}, nil)
	_, err := svc.Get(context.Background(), "E1'--")
	engagementIsValidation(t, err)
	_, err = svc.Get(context.Background(), "E404")
	var nf *apierror.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v", err)
	}
}

func engagementValidCreate() domain.EngagementCreateRequest {
	return domain.EngagementCreateRequest{
		AllocationTypeID: 12, UUID: "u", CreatedBy: "me@example.com", RequestStatus: "Pending Approval",
		EngagementStatus: "Pending", StartDate: "2026-11-20", EndDate: "2026-11-22",
		ProjectName: "WSO2Con → Asia 🎉", Address: "Line 1\nLine 2",
	}
}

func TestEngagementCreate_NormalisesTextAndCreatesNetSuiteEvent(t *testing.T) {
	repo := &engagementFakeRepo{engType: 5}
	ns := &engagementFakeNetSuite{}
	e, err := NewEngagementService(repo, ns).Create(context.Background(), engagementValidCreate())
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "EVT00510" || repo.created.ProjectName != "WSO2Con -> Asia" || repo.created.Address != "Line 1\nLine 2" {
		t.Errorf("created = %+v, e = %+v", repo.created, e)
	}
	if ns.name != "WSO2Con -> Asia" || ns.id != "EVT00510" {
		t.Errorf("netsuite = %+v", ns)
	}
}

func TestEngagementCreate_NetSuiteFailureIs503AndNilClientSkips(t *testing.T) {
	_, err := NewEngagementService(&engagementFakeRepo{engType: 5}, &engagementFakeNetSuite{err: errors.New("down")}).
		Create(context.Background(), engagementValidCreate())
	var su *apierror.ServiceUnavailableError
	if !errors.As(err, &su) {
		t.Fatalf("err = %v", err)
	}
	var ce *apierror.ConflictError
	if _, err := NewEngagementService(&engagementFakeRepo{engType: 5}, &engagementFakeNetSuite{err: netsuite.ErrEventExists}).
		Create(context.Background(), engagementValidCreate()); !errors.As(err, &ce) {
		t.Fatalf("event exists: %v", err)
	}
	if _, err := NewEngagementService(&engagementFakeRepo{engType: 5}, nil).Create(context.Background(), engagementValidCreate()); err != nil {
		t.Fatalf("nil NetSuite: %v", err)
	}
	// Not a marketing event: NetSuite is not called.
	ns := &engagementFakeNetSuite{err: errors.New("must not be called")}
	if _, err := NewEngagementService(&engagementFakeRepo{engType: 1}, ns).Create(context.Background(), engagementValidCreate()); err != nil || ns.id != "" {
		t.Fatalf("internal: err %v, ns %+v", err, ns)
	}
}

func TestEngagementCreate_Validation(t *testing.T) {
	svc := NewEngagementService(&engagementFakeRepo{}, nil)
	for _, mut := range []func(*domain.EngagementCreateRequest){
		func(r *domain.EngagementCreateRequest) { r.AllocationTypeID = 0 },
		func(r *domain.EngagementCreateRequest) { r.UUID = "" },
		func(r *domain.EngagementCreateRequest) { r.CreatedBy = "" },
		func(r *domain.EngagementCreateRequest) { r.RequestStatus = "Open" },
		func(r *domain.EngagementCreateRequest) { r.EngagementStatus = "Open" },
		func(r *domain.EngagementCreateRequest) { r.StartDate = "" },
		func(r *domain.EngagementCreateRequest) { r.EndDate = "20-11-2026" },
	} {
		req := engagementValidCreate()
		mut(&req)
		_, err := svc.Create(context.Background(), req)
		engagementIsValidation(t, err)
	}
	_, err := NewEngagementService(&engagementFakeRepo{createErr: repository.ErrEngagementAllocationTypeNotFound}, nil).
		Create(context.Background(), engagementValidCreate())
	engagementIsValidation(t, err)
}

func TestEngagementUpdate(t *testing.T) {
	repo := &engagementFakeRepo{}
	svc := NewEngagementService(repo, nil)
	comment := "naïve → ok"
	resp, err := svc.Update(context.Background(), "E1", domain.EngagementUpdateRequest{UpdatedBy: "me", Comment: &comment})
	if err != nil || resp.Engagement.ID != "E1" || resp.CancelledAllocations == nil {
		t.Fatalf("resp %+v err %v", resp, err)
	}
	if *repo.updated.Comment != "naïve -> ok" {
		t.Errorf("comment = %q", *repo.updated.Comment)
	}
	bad := "Done"
	for _, req := range []domain.EngagementUpdateRequest{
		{},
		{UpdatedBy: "me", RequestStatus: &bad},
		{UpdatedBy: "me", EngagementStatus: &bad},
		{UpdatedBy: "me", StartDate: new(string)},
	} {
		_, err := svc.Update(context.Background(), "E1", req)
		engagementIsValidation(t, err)
	}
	_, err = NewEngagementService(&engagementFakeRepo{updateErr: repository.ErrEngagementNotFound}, nil).
		Update(context.Background(), "E1", domain.EngagementUpdateRequest{UpdatedBy: "me"})
	var nf *apierror.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err = %v", err)
	}
}
