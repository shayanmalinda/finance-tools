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
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// AllocationService is the allocation screen area: engagement allocations,
// default (team) allocations, their travel rows, partner consultants and the
// Consultant Allocations report. It validates input; privileges and workflow
// are the backend's.
type AllocationService interface {
	Engagement(ctx context.Context, id string) (domain.AllocationEngagement, error)
	Get(ctx context.Context, id string) (domain.AllocationRecord, error)
	Timeline(ctx context.Context, req domain.AllocationTimelineRequest) (domain.AllocationListResponse, error)
	Search(ctx context.Context, req domain.AllocationSearchRequest) (domain.AllocationSearchResponse, error)
	Bulk(ctx context.Context, req domain.AllocationBulkRequest) (domain.AllocationBulkResponse, error)
	Update(ctx context.Context, id string, req domain.AllocationUpdateRequest) (domain.AllocationUpdateResponse, error)
	Delete(ctx context.Context, id string) error
	Visas(ctx context.Context, email, startDate, endDate string) (domain.AllocationVisaResponse, error)
	ConsultantAllocations(ctx context.Context, f domain.ConsultantAllocationFilter) (domain.ConsultantAllocationsResponse, error)
	NonAllocatedEmployees(ctx context.Context, f domain.ConsultantAllocationFilter) (domain.ConsultantNonAllocatedResponse, error)
	TeamDefaults(ctx context.Context) (domain.ConsultantTeamDefaultsResponse, error)
	EngagementCodes(ctx context.Context) (domain.ConsultantEngagementCodesResponse, error)
}

// Limits of one request.
const (
	allocationMaxBulk    = 5000
	allocationMaxEmails  = 5000
	allocationMaxPage    = 500
	allocationMaxComment = 255
)

type allocationService struct {
	repo repository.AllocationRepository
}

// NewAllocationService constructs an AllocationService.
func NewAllocationService(repo repository.AllocationRepository) AllocationService {
	return &allocationService{repo: repo}
}

func allocationValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func allocationValidTime(s string) bool {
	if _, err := time.Parse("15:04", s); err == nil {
		return true
	}
	_, err := time.Parse("15:04:05", s)
	return err == nil
}

func allocationIsClearance(s string) bool {
	for _, c := range domain.AllocationClearanceStatuses {
		if c == s {
			return true
		}
	}
	return false
}

func allocationBad(format string, a ...any) error {
	return &apierror.ValidationError{Msg: fmt.Sprintf(format, a...)}
}

func (s *allocationService) Engagement(ctx context.Context, id string) (domain.AllocationEngagement, error) {
	if strings.TrimSpace(id) == "" {
		return domain.AllocationEngagement{}, allocationBad("engagement id is required")
	}
	e, err := s.repo.Engagement(ctx, id)
	if err != nil {
		return domain.AllocationEngagement{}, err
	}
	if e == nil {
		return domain.AllocationEngagement{}, &apierror.NotFoundError{Msg: "engagement " + id + " not found"}
	}
	return *e, nil
}

func (s *allocationService) Get(ctx context.Context, id string) (domain.AllocationRecord, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.AllocationRecord{}, err
	}
	if a == nil {
		return domain.AllocationRecord{}, &apierror.NotFoundError{Msg: "allocation " + id + " not found"}
	}
	return *a, nil
}

func (s *allocationService) Timeline(ctx context.Context, req domain.AllocationTimelineRequest) (domain.AllocationListResponse, error) {
	if len(req.Emails) > allocationMaxEmails {
		return domain.AllocationListResponse{}, allocationBad("at most %d emails", allocationMaxEmails)
	}
	if !allocationValidDate(req.StartDate) || !allocationValidDate(req.EndDate) {
		return domain.AllocationListResponse{}, allocationBad("startDate and endDate must be dates (YYYY-MM-DD)")
	}
	rows, err := s.repo.Timeline(ctx, req)
	if err != nil {
		return domain.AllocationListResponse{}, err
	}
	return domain.AllocationListResponse{Allocations: rows}, nil
}

func (s *allocationService) Search(ctx context.Context, req domain.AllocationSearchRequest) (domain.AllocationSearchResponse, error) {
	if req.Limit < 1 || req.Limit > allocationMaxPage || req.Offset < 0 {
		return domain.AllocationSearchResponse{}, allocationBad("limit must be 1-%d and offset 0 or more", allocationMaxPage)
	}
	if !allocationValidDate(req.Today) {
		return domain.AllocationSearchResponse{}, allocationBad("today must be a date (YYYY-MM-DD)")
	}
	for _, d := range []string{req.StartFrom, req.EndTo} {
		if d != "" && !allocationValidDate(d) {
			return domain.AllocationSearchResponse{}, allocationBad("startFrom and endTo must be dates (YYYY-MM-DD)")
		}
	}
	req.Search = strings.TrimSpace(req.Search)
	items, total, err := s.repo.Search(ctx, req)
	if err != nil {
		return domain.AllocationSearchResponse{}, err
	}
	return domain.AllocationSearchResponse{Items: items, Total: total}, nil
}

func validateAllocationInput(i int, a domain.AllocationInput) error {
	switch {
	case strings.TrimSpace(a.Email) == "":
		return allocationBad("newAllocations[%d]: email is required", i)
	case a.AllocationTypeID <= 0:
		return allocationBad("newAllocations[%d]: allocationTypeId is required", i)
	case a.Status != domain.AllocationStatusActive && a.Status != domain.AllocationStatusInactive:
		return allocationBad("newAllocations[%d]: status must be Active or Inactive", i)
	case !allocationValidDate(a.StartDate) || !allocationValidDate(a.EndDate):
		return allocationBad("newAllocations[%d]: startDate and endDate must be dates (YYYY-MM-DD)", i)
	case a.EngagementID == "":
		return allocationBad("newAllocations[%d]: engagementId is required (-1 for a default allocation)", i)
	case a.IsTravelRequired != "Yes" && a.IsTravelRequired != "No":
		return allocationBad("newAllocations[%d]: isTravelRequired must be Yes or No", i)
	case a.IsRecurring != "Yes" && a.IsRecurring != "No":
		return allocationBad("newAllocations[%d]: isRecurring must be Yes or No", i)
	case a.ClearanceStatus != nil && !allocationIsClearance(*a.ClearanceStatus):
		return allocationBad("newAllocations[%d]: unknown clearanceStatus %q", i, *a.ClearanceStatus)
	case a.Comment != nil && len([]rune(*a.Comment)) > allocationMaxComment:
		return allocationBad("newAllocations[%d]: comment must be %d characters or fewer", i, allocationMaxComment)
	}
	for _, t := range []*string{a.StartTime, a.EndTime} {
		if t != nil && !allocationValidTime(*t) {
			return allocationBad("newAllocations[%d]: startTime and endTime must be times (HH:MM)", i)
		}
	}
	return nil
}

func (s *allocationService) Bulk(ctx context.Context, req domain.AllocationBulkRequest) (domain.AllocationBulkResponse, error) {
	if strings.TrimSpace(req.CreatedBy) == "" {
		return domain.AllocationBulkResponse{}, allocationBad("createdBy is required")
	}
	if len(req.NewAllocations) == 0 {
		return domain.AllocationBulkResponse{}, allocationBad("newAllocations must not be empty")
	}
	if len(req.NewAllocations)+len(req.UpdateAllocations)+len(req.AllocationLogs) > 3*allocationMaxBulk ||
		len(req.NewAllocations) > allocationMaxBulk {
		return domain.AllocationBulkResponse{}, allocationBad("at most %d allocations per request", allocationMaxBulk)
	}
	for i, a := range req.NewAllocations {
		if err := validateAllocationInput(i, a); err != nil {
			return domain.AllocationBulkResponse{}, err
		}
	}
	for i, u := range req.UpdateAllocations {
		if u.ID == "" || !allocationValidDate(u.StartDate) || !allocationValidDate(u.EndDate) {
			return domain.AllocationBulkResponse{}, allocationBad("updateAllocations[%d]: id, startDate and endDate are required", i)
		}
	}
	for i, l := range req.AllocationLogs {
		if l.Email == "" || l.AllocationTypeID <= 0 || !allocationValidDate(l.StartDate) || !allocationValidDate(l.EndDate) {
			return domain.AllocationBulkResponse{}, allocationBad("allocationLogs[%d]: email, allocationTypeId and dates are required", i)
		}
	}
	if req.RequestStatusInProgress != nil && strings.TrimSpace(*req.RequestStatusInProgress) == "" {
		return domain.AllocationBulkResponse{}, allocationBad("requestStatusInProgress must be an engagement id")
	}
	return s.repo.Bulk(ctx, req)
}

func (s *allocationService) Update(ctx context.Context, id string, req domain.AllocationUpdateRequest) (domain.AllocationUpdateResponse, error) {
	if strings.TrimSpace(req.UpdatedBy) == "" {
		return domain.AllocationUpdateResponse{}, allocationBad("updatedBy is required")
	}
	if req.ClearanceStatus == nil && req.Status == nil && req.Comment == nil && req.StartDate == nil && req.EndDate == nil {
		return domain.AllocationUpdateResponse{}, allocationBad("nothing to update")
	}
	if req.ClearanceStatus != nil && !allocationIsClearance(*req.ClearanceStatus) {
		return domain.AllocationUpdateResponse{}, allocationBad("unknown clearanceStatus %q", *req.ClearanceStatus)
	}
	if req.Status != nil && *req.Status != domain.AllocationStatusActive && *req.Status != domain.AllocationStatusInactive {
		return domain.AllocationUpdateResponse{}, allocationBad("status must be Active or Inactive")
	}
	for _, d := range []*string{req.StartDate, req.EndDate} {
		if d != nil && !allocationValidDate(*d) {
			return domain.AllocationUpdateResponse{}, allocationBad("startDate and endDate must be dates (YYYY-MM-DD)")
		}
	}
	if req.Comment != nil && len([]rune(*req.Comment)) > allocationMaxComment {
		return domain.AllocationUpdateResponse{}, allocationBad("comment must be %d characters or fewer", allocationMaxComment)
	}
	pre, err := s.Get(ctx, id)
	if err != nil {
		return domain.AllocationUpdateResponse{}, err
	}
	return s.repo.Update(ctx, pre, req)
}

func (s *allocationService) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *allocationService) Visas(ctx context.Context, email, startDate, endDate string) (domain.AllocationVisaResponse, error) {
	if strings.TrimSpace(email) == "" || !allocationValidDate(startDate) || !allocationValidDate(endDate) {
		return domain.AllocationVisaResponse{}, allocationBad("email, startDate and endDate are required")
	}
	v, err := s.repo.Visas(ctx, email, startDate, endDate)
	if err != nil {
		return domain.AllocationVisaResponse{}, err
	}
	return domain.AllocationVisaResponse{Visas: v}, nil
}

func validateConsultantFilter(f domain.ConsultantAllocationFilter) error {
	if !allocationValidDate(f.StartDate) || !allocationValidDate(f.EndDate) {
		return allocationBad("startDate and endDate must be dates (YYYY-MM-DD)")
	}
	return nil
}

func (s *allocationService) ConsultantAllocations(ctx context.Context, f domain.ConsultantAllocationFilter) (domain.ConsultantAllocationsResponse, error) {
	if err := validateConsultantFilter(f); err != nil {
		return domain.ConsultantAllocationsResponse{}, err
	}
	rows, err := s.repo.ConsultantAllocations(ctx, f)
	if err != nil {
		return domain.ConsultantAllocationsResponse{}, err
	}
	return domain.ConsultantAllocationsResponse{Allocations: rows}, nil
}

func (s *allocationService) NonAllocatedEmployees(ctx context.Context, f domain.ConsultantAllocationFilter) (domain.ConsultantNonAllocatedResponse, error) {
	if err := validateConsultantFilter(f); err != nil {
		return domain.ConsultantNonAllocatedResponse{}, err
	}
	rows, err := s.repo.NonAllocatedEmployees(ctx, f)
	if err != nil {
		return domain.ConsultantNonAllocatedResponse{}, err
	}
	return domain.ConsultantNonAllocatedResponse{Employees: rows}, nil
}

func (s *allocationService) TeamDefaults(ctx context.Context) (domain.ConsultantTeamDefaultsResponse, error) {
	rows, err := s.repo.TeamDefaults(ctx)
	if err != nil {
		return domain.ConsultantTeamDefaultsResponse{}, err
	}
	return domain.ConsultantTeamDefaultsResponse{Defaults: rows}, nil
}

func (s *allocationService) EngagementCodes(ctx context.Context) (domain.ConsultantEngagementCodesResponse, error) {
	rows, err := s.repo.EngagementCodes(ctx)
	if err != nil {
		return domain.ConsultantEngagementCodesResponse{}, err
	}
	return domain.ConsultantEngagementCodesResponse{Codes: rows}, nil
}
