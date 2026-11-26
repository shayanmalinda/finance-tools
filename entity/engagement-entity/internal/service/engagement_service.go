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
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// EngagementService reads and writes engagements.
type EngagementService interface {
	Search(ctx context.Context, req domain.EngagementSearchRequest) (domain.EngagementSearchResponse, error)
	Facets(ctx context.Context, engagementTypeIDs []int) (domain.EngagementFacetsResponse, error)
	Get(ctx context.Context, id string) (domain.Engagement, error)
	Allocations(ctx context.Context, id string, activeOnly bool) (domain.EngagementAllocationsResponse, error)
	Create(ctx context.Context, req domain.EngagementCreateRequest) (domain.Engagement, error)
	Update(ctx context.Context, id string, req domain.EngagementUpdateRequest) (domain.EngagementUpdateResponse, error)
}

// EngagementMarketingEventCreator creates the NetSuite marketing event of a
// new Marketing Event (legacy netsuite:createMarketingEvent with eventName
// = the event name and eventID = the new engagement id). It runs inside the
// create transaction: an error rolls the engagement back, as in legacy.
type EngagementMarketingEventCreator interface {
	CreateMarketingEvent(ctx context.Context, eventName, eventID string) error
}

const (
	engagementDefaultLimit = 50
	engagementMaxLimit     = 500
)

var (
	// engagementIDPattern is the legacy validateData(ALPHANUMERIC, id).
	engagementIDPattern   = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	engagementDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// engagementRequestStatuses are the legacy engagement.status values.
	engagementRequestStatuses = []string{
		domain.EngagementRequestPendingApproval, domain.EngagementRequestNew, domain.EngagementRequestUpdated,
		domain.EngagementRequestApproved, domain.EngagementRequestInProgress, domain.EngagementRequestCompleted,
		domain.EngagementRequestCancelled,
	}
	// engagementStatuses are the legacy engagement.engagement_status values.
	engagementStatuses = []string{"Confirmed", "Pending", "In Progress", "Completed", "Cancelled"}
)

type engagementService struct {
	repo     repository.EngagementRepository
	netsuite EngagementMarketingEventCreator
	now      func() time.Time
}

// NewEngagementService constructs an EngagementService. marketing may be nil
// (NetSuite not configured): a new Marketing Event is then saved without a
// NetSuite marketing event, and a warning is logged.
func NewEngagementService(repo repository.EngagementRepository, marketing EngagementMarketingEventCreator) EngagementService {
	return &engagementService{repo: repo, netsuite: marketing, now: time.Now}
}

func engagementValidID(id string) error {
	if !engagementIDPattern.MatchString(id) {
		return &apierror.ValidationError{Msg: "Invalid engagement id format."}
	}
	return nil
}

func engagementCheckDate(name, v string) error {
	if v == "" {
		return nil
	}
	if !engagementDatePattern.MatchString(v) {
		return &apierror.ValidationError{Msg: name + " must be a date (YYYY-MM-DD)"}
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return &apierror.ValidationError{Msg: name + " must be a date (YYYY-MM-DD)"}
	}
	return nil
}

func engagementOneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func (s *engagementService) Search(ctx context.Context, req domain.EngagementSearchRequest) (domain.EngagementSearchResponse, error) {
	if req.Limit == 0 {
		req.Limit = engagementDefaultLimit
	}
	if req.Limit < 1 || req.Limit > engagementMaxLimit {
		return domain.EngagementSearchResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("limit must be between 1 and %d", engagementMaxLimit)}
	}
	if req.Offset < 0 {
		return domain.EngagementSearchResponse{}, &apierror.ValidationError{Msg: "offset must be 0 or more"}
	}
	for name, v := range map[string]string{"startFrom": req.StartFrom, "startTo": req.StartTo, "endTo": req.EndTo, "today": req.Today} {
		if err := engagementCheckDate(name, v); err != nil {
			return domain.EngagementSearchResponse{}, err
		}
	}
	if req.Consultants != "" && req.Consultants != "allocated" && req.Consultants != "none" {
		return domain.EngagementSearchResponse{}, &apierror.ValidationError{Msg: "consultants must be allocated or none"}
	}
	// An empty or unknown sort gives the default order (the repository
	// whitelists the column sorts), so older callers keep working.
	if req.Sort == "" {
		req.Sort = domain.EngagementSortCurrent
	}
	if req.Today == "" {
		req.Today = s.now().UTC().Format("2006-01-02")
	}
	items, total, err := s.repo.Search(ctx, req)
	if err != nil {
		return domain.EngagementSearchResponse{}, err
	}
	if err := s.enrich(ctx, items, req.IncludeConsultants); err != nil {
		return domain.EngagementSearchResponse{}, err
	}
	counts, err := s.repo.TypeCounts(ctx, req.PendingApproval)
	if err != nil {
		return domain.EngagementSearchResponse{}, err
	}
	return domain.EngagementSearchResponse{Items: items, Total: total, TypeCounts: counts}, nil
}

// enrich adds the funding sources (and consultants) of a page of rows: one
// query each for the whole page.
func (s *engagementService) enrich(ctx context.Context, items []domain.Engagement, consultants bool) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	fs, err := s.repo.FundingSources(ctx, ids)
	if err != nil {
		return err
	}
	var cons map[string][]string
	if consultants {
		if cons, err = s.repo.Consultants(ctx, ids); err != nil {
			return err
		}
	}
	for i := range items {
		if f, ok := fs[items[i].ID]; ok {
			items[i].FundingSources = f
		}
		if consultants {
			items[i].Consultants = cons[items[i].ID]
			if items[i].Consultants == nil {
				items[i].Consultants = []string{}
			}
		}
	}
	return nil
}

func (s *engagementService) Facets(ctx context.Context, engagementTypeIDs []int) (domain.EngagementFacetsResponse, error) {
	return s.repo.Facets(ctx, engagementTypeIDs)
}

func engagementNotFound(id string) error {
	return &apierror.NotFoundError{Msg: fmt.Sprintf("Engagement %s not found.", id)}
}

func (s *engagementService) Get(ctx context.Context, id string) (domain.Engagement, error) {
	if err := engagementValidID(id); err != nil {
		return domain.Engagement{}, err
	}
	e, err := s.repo.Get(ctx, id)
	if errors.Is(err, repository.ErrEngagementNotFound) {
		return domain.Engagement{}, engagementNotFound(id)
	}
	if err != nil {
		return domain.Engagement{}, err
	}
	items := []domain.Engagement{e}
	if err := s.enrich(ctx, items, false); err != nil {
		return domain.Engagement{}, err
	}
	return items[0], nil
}

func (s *engagementService) Allocations(ctx context.Context, id string, activeOnly bool) (domain.EngagementAllocationsResponse, error) {
	if err := engagementValidID(id); err != nil {
		return domain.EngagementAllocationsResponse{}, err
	}
	allocs, err := s.repo.Allocations(ctx, id, activeOnly)
	if err != nil {
		return domain.EngagementAllocationsResponse{}, err
	}
	return domain.EngagementAllocationsResponse{Allocations: allocs}, nil
}

func engagementNormFunding(fs []domain.EngagementFundingSource) []domain.EngagementFundingSource {
	out := make([]domain.EngagementFundingSource, len(fs))
	for i, f := range fs {
		out[i] = domain.EngagementFundingSource{
			BusinessUnit: strings.TrimSpace(f.BusinessUnit), ProductUnit: strings.TrimSpace(f.ProductUnit),
			Region: strings.TrimSpace(f.Region), SubRegion: strings.TrimSpace(f.SubRegion), Percentage: f.Percentage,
		}
	}
	return out
}

func (s *engagementService) Create(ctx context.Context, req domain.EngagementCreateRequest) (domain.Engagement, error) {
	switch {
	case req.AllocationTypeID <= 0:
		return domain.Engagement{}, &apierror.ValidationError{Msg: "allocationTypeId is required"}
	case strings.TrimSpace(req.UUID) == "":
		return domain.Engagement{}, &apierror.ValidationError{Msg: "uuid is required"}
	case strings.TrimSpace(req.CreatedBy) == "":
		return domain.Engagement{}, &apierror.ValidationError{Msg: "createdBy is required"}
	case !engagementOneOf(req.RequestStatus, engagementRequestStatuses):
		return domain.Engagement{}, &apierror.ValidationError{Msg: "requestStatus is not a legacy request status"}
	case !engagementOneOf(req.EngagementStatus, engagementStatuses):
		return domain.Engagement{}, &apierror.ValidationError{Msg: "engagementStatus is not a legacy engagement status"}
	}
	for name, v := range map[string]string{"startDate": req.StartDate, "endDate": req.EndDate} {
		if v == "" {
			return domain.Engagement{}, &apierror.ValidationError{Msg: name + " is required"}
		}
		if err := engagementCheckDate(name, v); err != nil {
			return domain.Engagement{}, err
		}
	}
	// Free text written to the latin1 columns.
	for _, p := range []*string{&req.CustomerName, &req.Address, &req.BusinessDomain, &req.EngagementCode,
		&req.ProjectName, &req.OpportunityOwner, &req.ProductName, &req.EngagementScope, &req.EngagementNature,
		&req.CustomerContact, &req.TechnicalOwner, &req.Comment, &req.Country, &req.Description} {
		*p = textnorm.Latin1(*p)
	}
	req.Country = strings.TrimSpace(req.Country)
	req.FundingSources = engagementNormFunding(req.FundingSources)

	eventName := req.ProjectName
	hook := func(ctx context.Context, id string, engType int) error {
		if engType != domain.EngagementTypeMarketingEvent {
			return nil
		}
		if s.netsuite == nil {
			log.Printf("Marketing event %s saved without a NetSuite marketing event: NetSuite is not configured", id) // #nosec G706
			return nil
		}
		if err := s.netsuite.CreateMarketingEvent(ctx, eventName, id); err != nil {
			if errors.Is(err, netsuite.ErrEventExists) {
				return &apierror.ConflictError{Msg: fmt.Sprintf("A marketing event named %q already exists in NetSuite. Use a different Event Name.", eventName)}
			}
			return &apierror.ServiceUnavailableError{Msg: "Failed to create the marketing event in NetSuite: " + err.Error()}
		}
		return nil
	}
	id, err := s.repo.Create(ctx, req, hook)
	if errors.Is(err, repository.ErrEngagementAllocationTypeNotFound) {
		return domain.Engagement{}, &apierror.ValidationError{Msg: fmt.Sprintf("Invalid allocation type id %d", req.AllocationTypeID)}
	}
	if err != nil {
		return domain.Engagement{}, err
	}
	return s.Get(ctx, id)
}

func (s *engagementService) Update(ctx context.Context, id string, req domain.EngagementUpdateRequest) (domain.EngagementUpdateResponse, error) {
	if err := engagementValidID(id); err != nil {
		return domain.EngagementUpdateResponse{}, err
	}
	if strings.TrimSpace(req.UpdatedBy) == "" {
		return domain.EngagementUpdateResponse{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	for name, v := range map[string]*string{"startDate": req.StartDate, "endDate": req.EndDate} {
		if v != nil {
			if *v == "" {
				return domain.EngagementUpdateResponse{}, &apierror.ValidationError{Msg: name + " cannot be empty"}
			}
			if err := engagementCheckDate(name, *v); err != nil {
				return domain.EngagementUpdateResponse{}, err
			}
		}
	}
	if req.RequestStatus != nil && !engagementOneOf(*req.RequestStatus, engagementRequestStatuses) {
		return domain.EngagementUpdateResponse{}, &apierror.ValidationError{Msg: "requestStatus is not a legacy request status"}
	}
	if req.EngagementStatus != nil && !engagementOneOf(*req.EngagementStatus, engagementStatuses) {
		return domain.EngagementUpdateResponse{}, &apierror.ValidationError{Msg: "engagementStatus is not a legacy engagement status"}
	}
	if req.AllocationTypeID != nil && *req.AllocationTypeID <= 0 {
		return domain.EngagementUpdateResponse{}, &apierror.ValidationError{Msg: "allocationTypeId must be a positive integer"}
	}
	for _, p := range []*string{req.EngagementCode, req.EngagementScope, req.EngagementNature, req.Comment,
		req.Country, req.CustomerContact, req.Address, req.Description, req.ProjectName} {
		if p != nil {
			*p = textnorm.Latin1(*p)
		}
	}
	if req.Country != nil {
		*req.Country = strings.TrimSpace(*req.Country)
	}
	req.FundingSources = engagementNormFunding(req.FundingSources)

	cancelled, err := s.repo.Update(ctx, id, req)
	if errors.Is(err, repository.ErrEngagementNotFound) {
		return domain.EngagementUpdateResponse{}, engagementNotFound(id)
	}
	if err != nil {
		return domain.EngagementUpdateResponse{}, err
	}
	// The legacy update re-read the engagement after the commit; a failed
	// re-read is reported although the update was saved.
	e, err := s.Get(ctx, id)
	if err != nil {
		return domain.EngagementUpdateResponse{}, err
	}
	return domain.EngagementUpdateResponse{Engagement: e, CancelledAllocations: cancelled}, nil
}
