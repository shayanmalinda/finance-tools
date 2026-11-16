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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type fakeAllocationRepo struct {
	repository.AllocationRepository
	rec     *domain.AllocationRecord
	updated *domain.AllocationUpdateRequest
	bulk    *domain.AllocationBulkRequest
	deleted string
}

func (f *fakeAllocationRepo) Get(context.Context, string) (*domain.AllocationRecord, error) {
	return f.rec, nil
}

func (f *fakeAllocationRepo) Engagement(context.Context, string) (*domain.AllocationEngagement, error) {
	return nil, nil
}

func (f *fakeAllocationRepo) Update(_ context.Context, pre domain.AllocationRecord, req domain.AllocationUpdateRequest) (domain.AllocationUpdateResponse, error) {
	f.updated = &req
	return domain.AllocationUpdateResponse{Allocation: pre}, nil
}

func (f *fakeAllocationRepo) Bulk(_ context.Context, req domain.AllocationBulkRequest) (domain.AllocationBulkResponse, error) {
	f.bulk = &req
	return domain.AllocationBulkResponse{RecordsAdded: len(req.NewAllocations)}, nil
}

func (f *fakeAllocationRepo) Delete(_ context.Context, id string) error {
	f.deleted = id
	return nil
}

func isValidation(err error) bool {
	var ve *apierror.ValidationError
	return errors.As(err, &ve)
}

func isNotFound(err error) bool {
	var nf *apierror.NotFoundError
	return errors.As(err, &nf)
}

func goodInput() domain.AllocationInput {
	return domain.AllocationInput{Email: "a@wso2.com", AllocationTypeID: 1, Status: "Active", StartDate: "2026-11-01",
		EndDate: "2026-11-02", EngagementID: "-1", IsTravelRequired: "No", IsRecurring: "No"}
}

func TestAllocationBulk_Validation(t *testing.T) {
	repo := &fakeAllocationRepo{}
	svc := NewAllocationService(repo)
	bad := []domain.AllocationBulkRequest{
		{NewAllocations: []domain.AllocationInput{goodInput()}},
		{CreatedBy: "m@wso2.com"},
	}
	mutate := []func(*domain.AllocationInput){
		func(a *domain.AllocationInput) { a.Email = "" },
		func(a *domain.AllocationInput) { a.StartDate = "2026-13-01" },
		func(a *domain.AllocationInput) { a.IsTravelRequired = "maybe" },
		func(a *domain.AllocationInput) { s := "Done"; a.ClearanceStatus = &s },
		func(a *domain.AllocationInput) { s := "25:00"; a.StartTime = &s },
		func(a *domain.AllocationInput) { a.Status = "Deleted" },
	}
	for _, m := range mutate {
		in := goodInput()
		m(&in)
		bad = append(bad, domain.AllocationBulkRequest{CreatedBy: "m@wso2.com", NewAllocations: []domain.AllocationInput{in}})
	}
	for i, req := range bad {
		if _, err := svc.Bulk(context.Background(), req); !isValidation(err) {
			t.Errorf("case %d: err = %v, want validation", i, err)
		}
	}
	if repo.bulk != nil {
		t.Fatal("repository called for invalid input")
	}
	if _, err := svc.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "m@wso2.com",
		NewAllocations: []domain.AllocationInput{goodInput()}}); err != nil || repo.bulk == nil {
		t.Errorf("valid: %v", err)
	}
}

func TestAllocationUpdate_ValidationAndNotFound(t *testing.T) {
	repo := &fakeAllocationRepo{}
	svc := NewAllocationService(repo)
	s := "Confirmed"
	if _, err := svc.Update(context.Background(), "A1", domain.AllocationUpdateRequest{ClearanceStatus: &s}); !isValidation(err) {
		t.Errorf("no updatedBy: %v", err)
	}
	if _, err := svc.Update(context.Background(), "A1", domain.AllocationUpdateRequest{UpdatedBy: "m"}); !isValidation(err) {
		t.Errorf("nothing to update: %v", err)
	}
	bad := "Gone"
	if _, err := svc.Update(context.Background(), "A1", domain.AllocationUpdateRequest{UpdatedBy: "m", Status: &bad}); !isValidation(err) {
		t.Errorf("bad status: %v", err)
	}
	if _, err := svc.Update(context.Background(), "A1", domain.AllocationUpdateRequest{UpdatedBy: "m", ClearanceStatus: &s}); !isNotFound(err) {
		t.Errorf("missing allocation: %v", err)
	}
	repo.rec = &domain.AllocationRecord{ID: "A1"}
	if _, err := svc.Update(context.Background(), "A1", domain.AllocationUpdateRequest{UpdatedBy: "m", ClearanceStatus: &s}); err != nil || repo.updated == nil {
		t.Errorf("valid: %v", err)
	}
	if err := svc.Delete(context.Background(), "A1"); err != nil || repo.deleted != "A1" {
		t.Errorf("delete: %v", err)
	}
}

func TestAllocationReadsValidateInput(t *testing.T) {
	svc := NewAllocationService(&fakeAllocationRepo{})
	ctx := context.Background()
	if _, err := svc.Engagement(ctx, "E1"); !isNotFound(err) {
		t.Errorf("engagement: %v", err)
	}
	if _, err := svc.Timeline(ctx, domain.AllocationTimelineRequest{StartDate: "x", EndDate: "2026-01-01"}); !isValidation(err) {
		t.Errorf("timeline: %v", err)
	}
	if _, err := svc.Search(ctx, domain.AllocationSearchRequest{Limit: 0, Today: "2026-01-01"}); !isValidation(err) {
		t.Errorf("search limit: %v", err)
	}
	if _, err := svc.Search(ctx, domain.AllocationSearchRequest{Limit: 10, Today: "today"}); !isValidation(err) {
		t.Errorf("search today: %v", err)
	}
	if _, err := svc.Visas(ctx, "", "2026-01-01", "2026-01-02"); !isValidation(err) {
		t.Errorf("visas: %v", err)
	}
	if _, err := svc.ConsultantAllocations(ctx, domain.ConsultantAllocationFilter{StartDate: "2026-01-01"}); !isValidation(err) {
		t.Errorf("report: %v", err)
	}
}
