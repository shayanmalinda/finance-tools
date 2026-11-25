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
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// TravelRequestService serves the travel request screens: travelapp rows
// combined in Go with their allocation_db engagement.
type TravelRequestService interface {
	Search(ctx context.Context, f domain.TravelRequestSearch) (domain.TravelRequestPage, error)
	Get(ctx context.Context, jobNumber string) (domain.TravelRequestDetail, error)
	Update(ctx context.Context, jobNumber string, in domain.UpdateTravelRequestInput) (domain.TravelRequestDetail, error)
	Agents(ctx context.Context, status string) (domain.TravelAgentList, error)
	ActivePerDiems(ctx context.Context, jobNumber string) (domain.TravelActivePerDiemList, error)
	InsertEmailAudit(ctx context.Context, in domain.TravelEmailAuditInput) error
}

// TravelDocumentService serves visas and travel documents.
type TravelDocumentService interface {
	SearchVisas(ctx context.Context, f domain.TravelVisaFilter) (domain.TravelVisaPage, error)
	GetVisa(ctx context.Context, id int) (domain.TravelVisa, error)
	CreateVisa(ctx context.Context, in domain.CreateTravelVisaInput) (domain.TravelVisa, error)
	UpdateVisa(ctx context.Context, id int, in domain.UpdateTravelVisaInput) (domain.TravelVisa, error)
	SearchDocuments(ctx context.Context, f domain.TravelDocumentFilter) (domain.TravelDocumentPage, error)
	GetDocument(ctx context.Context, id int) (domain.TravelDocument, error)
	CreateDocument(ctx context.Context, in domain.CreateTravelDocumentInput) (domain.TravelDocument, error)
	UpdateDocument(ctx context.Context, id int, in domain.UpdateTravelDocumentInput) (domain.TravelDocument, error)
}

// Legacy paging (entity getValidatedLimit): default 100, at most 1000.
const (
	travelDefaultLimit = 100
	travelMaxLimit     = 1000
)

// travelJobNumber is the legacy JOB_NUMBER_FORMAT.
var travelJobNumber = regexp.MustCompile(`^TRV-\d{4}-\d{2}-\d{4}$`)

// TravelJobNumberValid reports whether s is a legacy job number.
func TravelJobNumberValid(s string) bool { return travelJobNumber.MatchString(s) }

func travelPage(limit, offset int) (int, int, error) {
	if offset < 0 {
		return 0, 0, &apierror.ValidationError{Msg: "offset must be 0 or more"}
	}
	switch {
	case limit <= 0:
		limit = travelDefaultLimit
	case limit > travelMaxLimit:
		limit = travelMaxLimit
	}
	return limit, offset, nil
}

func travelCheckDate(field, v string) error {
	if v == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, v); err != nil {
		return &apierror.ValidationError{Msg: field + " must be a date (YYYY-MM-DD)"}
	}
	return nil
}

func travelToday(v string) string {
	if v != "" {
		return v
	}
	return time.Now().UTC().Format(time.DateOnly)
}

// travelClean trims every value and drops the empty ones.
func travelClean(vals []string) []string {
	var out []string
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

type travelRequestService struct {
	travel      repository.TravelRequestRepository
	engagements repository.TravelEngagementRepository
	documents   repository.TravelDocumentRepository
}

// NewTravelRequestService constructs a TravelRequestService.
func NewTravelRequestService(travel repository.TravelRequestRepository, engagements repository.TravelEngagementRepository,
	documents repository.TravelDocumentRepository) TravelRequestService {
	return &travelRequestService{travel: travel, engagements: engagements, documents: documents}
}

func (s *travelRequestService) Search(ctx context.Context, f domain.TravelRequestSearch) (domain.TravelRequestPage, error) {
	var err error
	if f.Limit, f.Offset, err = travelPage(f.Limit, f.Offset); err != nil {
		return domain.TravelRequestPage{}, err
	}
	f.Statuses, f.TravellerEmails = travelClean(f.Statuses), travelClean(f.TravellerEmails)
	f.Countries, f.AgencyNames = travelClean(f.Countries), travelClean(f.AgencyNames)
	for _, st := range f.Statuses {
		if !slices.Contains(domain.TravelStatuses, st) {
			return domain.TravelRequestPage{}, &apierror.ValidationError{Msg: fmt.Sprintf("invalid travel status %q", st)}
		}
	}
	for field, v := range map[string]string{"startFrom": f.StartFrom, "endTo": f.EndTo, "today": f.Today} {
		if err := travelCheckDate(field, v); err != nil {
			return domain.TravelRequestPage{}, err
		}
	}
	switch f.Sort {
	case "":
		f.Sort = domain.TravelSortCurrentFirst
	default:
		if !slices.Contains(domain.TravelSorts, f.Sort) {
			return domain.TravelRequestPage{}, &apierror.ValidationError{
				Msg: "sort must be one of " + strings.Join(domain.TravelSorts, ", ")}
		}
	}
	f.Today = travelToday(f.Today)
	f.Search = strings.TrimSpace(f.Search)
	if f.Search != "" && f.SearchEngagements {
		if f.EngagementIDs, err = s.engagements.IDsByTitle(ctx, f.Search); err != nil {
			return domain.TravelRequestPage{}, err
		}
	}

	items, total, err := s.travel.Search(ctx, f)
	if err != nil {
		return domain.TravelRequestPage{}, err
	}
	if err := s.attachEngagements(ctx, items); err != nil {
		return domain.TravelRequestPage{}, err
	}
	page := domain.TravelRequestPage{Items: items, Total: total}
	if f.WithStatusCounts {
		if page.StatusCounts, err = s.travel.StatusCounts(ctx, f); err != nil {
			return domain.TravelRequestPage{}, err
		}
	}
	return page, nil
}

// attachEngagements overlays the live engagement of each request (one
// allocation_db query for the page).
func (s *travelRequestService) attachEngagements(ctx context.Context, items []domain.TravelRequest) error {
	var ids []string
	seen := map[string]bool{}
	for _, t := range items {
		if t.AllocationID != nil && *t.AllocationID != "" && !seen[*t.AllocationID] {
			seen[*t.AllocationID] = true
			ids = append(ids, *t.AllocationID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	byAllocation, err := s.engagements.ByAllocation(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		if a := items[i].AllocationID; a != nil {
			if e, ok := byAllocation[*a]; ok {
				items[i].Engagement = &e
			}
		}
	}
	return nil
}

func (s *travelRequestService) Get(ctx context.Context, jobNumber string) (domain.TravelRequestDetail, error) {
	if !TravelJobNumberValid(jobNumber) {
		return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: "Invalid job number format"}
	}
	t, err := s.travel.Get(ctx, jobNumber)
	if err != nil {
		return domain.TravelRequestDetail{}, err
	}
	if t == nil {
		return domain.TravelRequestDetail{}, &apierror.NotFoundError{Msg: fmt.Sprintf("travel request %s not found", jobNumber)}
	}
	items := []domain.TravelRequest{*t}
	if err := s.attachEngagements(ctx, items); err != nil {
		return domain.TravelRequestDetail{}, err
	}
	d := domain.TravelRequestDetail{TravelRequest: items[0], FundingSources: []domain.TravelFundingSource{}}
	if d.Engagement != nil {
		if d.FundingSources, err = s.engagements.FundingSources(ctx, d.Engagement.ID); err != nil {
			return domain.TravelRequestDetail{}, err
		}
	}
	return d, nil
}

func (s *travelRequestService) Update(ctx context.Context, jobNumber string, in domain.UpdateTravelRequestInput) (domain.TravelRequestDetail, error) {
	if !TravelJobNumberValid(jobNumber) {
		return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: "Invalid job number format"}
	}
	in.UpdatedBy = strings.TrimSpace(in.UpdatedBy)
	if in.UpdatedBy == "" {
		return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if in.Status == nil && in.TravelAgentID == nil && in.VisaID == nil {
		return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: "nothing to update: give status, travelAgentId or visaId"}
	}
	if in.Status != nil && !slices.Contains(domain.TravelStatuses, *in.Status) {
		return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: "Invalid travel status"}
	}
	if in.TravelAgentID != nil {
		agents, err := s.travel.Agents(ctx, "")
		if err != nil {
			return domain.TravelRequestDetail{}, err
		}
		if !slices.ContainsFunc(agents, func(a domain.TravelAgent) bool { return a.ID == *in.TravelAgentID }) {
			return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: fmt.Sprintf("unknown travel agent %d", *in.TravelAgentID)}
		}
	}
	if in.VisaID != nil {
		v, err := s.documents.GetVisa(ctx, *in.VisaID)
		if err != nil {
			return domain.TravelRequestDetail{}, err
		}
		if v == nil {
			return domain.TravelRequestDetail{}, &apierror.ValidationError{Msg: fmt.Sprintf("unknown visa %d", *in.VisaID)}
		}
	}
	found, err := s.travel.Update(ctx, jobNumber, in)
	if err != nil {
		return domain.TravelRequestDetail{}, err
	}
	if !found {
		return domain.TravelRequestDetail{}, &apierror.NotFoundError{Msg: fmt.Sprintf("travel request %s not found", jobNumber)}
	}
	return s.Get(ctx, jobNumber)
}

func (s *travelRequestService) Agents(ctx context.Context, status string) (domain.TravelAgentList, error) {
	status = strings.TrimSpace(status)
	if status != "" && !strings.EqualFold(status, domain.TravelRecordActive) {
		return domain.TravelAgentList{}, &apierror.ValidationError{Msg: "status must be Active or empty"}
	}
	if status != "" {
		status = domain.TravelRecordActive
	}
	agents, err := s.travel.Agents(ctx, status)
	if err != nil {
		return domain.TravelAgentList{}, err
	}
	if agents == nil {
		agents = []domain.TravelAgent{}
	}
	return domain.TravelAgentList{Items: agents}, nil
}

func (s *travelRequestService) ActivePerDiems(ctx context.Context, jobNumber string) (domain.TravelActivePerDiemList, error) {
	if !TravelJobNumberValid(jobNumber) {
		return domain.TravelActivePerDiemList{}, &apierror.ValidationError{Msg: "Invalid job number format"}
	}
	items, err := s.travel.ActivePerDiems(ctx, jobNumber)
	if err != nil {
		return domain.TravelActivePerDiemList{}, err
	}
	if items == nil {
		items = []domain.TravelActivePerDiem{}
	}
	return domain.TravelActivePerDiemList{Items: items}, nil
}

func (s *travelRequestService) InsertEmailAudit(ctx context.Context, in domain.TravelEmailAuditInput) error {
	in, err := normalizeEmailAudit(in)
	if err != nil {
		return err
	}
	return s.travel.InsertEmailAudit(ctx, in)
}

// ── Visas and travel documents ────────────────────────────────────────────

type travelDocumentService struct {
	repo repository.TravelDocumentRepository
}

// NewTravelDocumentService constructs a TravelDocumentService.
func NewTravelDocumentService(repo repository.TravelDocumentRepository) TravelDocumentService {
	return &travelDocumentService{repo: repo}
}

func travelValidity(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case domain.TravelValidityActive:
		return domain.TravelValidityActive, nil
	case domain.TravelValidityExpired:
		return domain.TravelValidityExpired, nil
	}
	return "", &apierror.ValidationError{Msg: "validity must be active or expired"}
}

// travelRecordStatus returns the canonical stored status among allowed.
func travelRecordStatus(v string, allowed ...string) (string, error) {
	if c, ok := canonical(strings.TrimSpace(v), allowed...); ok {
		return c, nil
	}
	return "", &apierror.ValidationError{Msg: "status must be one of " + strings.Join(allowed, ", ")}
}

func (s *travelDocumentService) SearchVisas(ctx context.Context, f domain.TravelVisaFilter) (domain.TravelVisaPage, error) {
	var err error
	if f.Limit, f.Offset, err = travelPage(f.Limit, f.Offset); err != nil {
		return domain.TravelVisaPage{}, err
	}
	if f.Status != "" {
		if f.Status, err = travelRecordStatus(f.Status, domain.TravelRecordActive, domain.TravelRecordInactive,
			domain.TravelRecordExpired); err != nil {
			return domain.TravelVisaPage{}, err
		}
	}
	if f.Validity, err = travelValidity(f.Validity); err != nil {
		return domain.TravelVisaPage{}, err
	}
	for field, v := range map[string]string{"today": f.Today, "coversFrom": f.CoversFrom, "coversTo": f.CoversTo} {
		if err := travelCheckDate(field, v); err != nil {
			return domain.TravelVisaPage{}, err
		}
	}
	f.Today = travelToday(f.Today)
	f.Emails, f.Countries = travelClean(f.Emails), travelClean(f.Countries)
	items, total, err := s.repo.SearchVisas(ctx, f)
	if err != nil {
		return domain.TravelVisaPage{}, err
	}
	return domain.TravelVisaPage{Items: items, Total: total}, nil
}

func (s *travelDocumentService) GetVisa(ctx context.Context, id int) (domain.TravelVisa, error) {
	v, err := s.repo.GetVisa(ctx, id)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	if v == nil {
		return domain.TravelVisa{}, &apierror.NotFoundError{Msg: fmt.Sprintf("visa %d not found", id)}
	}
	return *v, nil
}

// travelVisaFields validates what both visa writes carry.
func travelVisaFields(country, visaType, entryType, issued, start, end string, passportID *int) error {
	if strings.TrimSpace(country) == "" || strings.TrimSpace(visaType) == "" || strings.TrimSpace(entryType) == "" ||
		issued == "" || start == "" || end == "" {
		return &apierror.ValidationError{Msg: "country, visaType, entryType, issuedDate, startDate and endDate are required"}
	}
	for field, v := range map[string]string{"issuedDate": issued, "startDate": start, "endDate": end} {
		if err := travelCheckDate(field, v); err != nil {
			return err
		}
	}
	if start > end {
		return &apierror.ValidationError{Msg: "startDate must not be after endDate"}
	}
	if passportID != nil && *passportID <= 0 {
		return &apierror.ValidationError{Msg: "passportId must be a positive integer"}
	}
	return nil
}

func (s *travelDocumentService) CreateVisa(ctx context.Context, in domain.CreateTravelVisaInput) (domain.TravelVisa, error) {
	in.Email, in.CreatedBy = strings.TrimSpace(in.Email), strings.TrimSpace(in.CreatedBy)
	if in.Email == "" || in.CreatedBy == "" || strings.TrimSpace(in.ImageID) == "" {
		return domain.TravelVisa{}, &apierror.ValidationError{Msg: "email, imageId and createdBy are required"}
	}
	if err := travelVisaFields(in.Country, in.VisaType, in.EntryType, in.IssuedDate, in.StartDate, in.EndDate, in.PassportID); err != nil {
		return domain.TravelVisa{}, err
	}
	id, err := s.repo.CreateVisa(ctx, in)
	if err != nil {
		return domain.TravelVisa{}, err
	}
	return s.GetVisa(ctx, id)
}

func (s *travelDocumentService) UpdateVisa(ctx context.Context, id int, in domain.UpdateTravelVisaInput) (domain.TravelVisa, error) {
	in.UpdatedBy = strings.TrimSpace(in.UpdatedBy)
	if in.UpdatedBy == "" {
		return domain.TravelVisa{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if err := travelVisaFields(in.Country, in.VisaType, in.EntryType, in.IssuedDate, in.StartDate, in.EndDate, in.PassportID); err != nil {
		return domain.TravelVisa{}, err
	}
	var err error
	if in.Status, err = travelRecordStatus(in.Status, domain.TravelRecordActive, domain.TravelRecordInactive,
		domain.TravelRecordExpired); err != nil {
		return domain.TravelVisa{}, err
	}
	if err := travelRepoError(s.repo.UpdateVisa(ctx, id, in), "visa", id,
		"There's an already existing visa with the same email and country."); err != nil {
		return domain.TravelVisa{}, err
	}
	return s.GetVisa(ctx, id)
}

// travelRepoError maps the repository's sentinel errors to API errors.
func travelRepoError(err error, kind string, id int, duplicateMsg string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrTravelRecordNotFound):
		return &apierror.NotFoundError{Msg: fmt.Sprintf("%s %d not found", kind, id)}
	case errors.Is(err, repository.ErrTravelDuplicateActive):
		return &apierror.ConflictError{Msg: duplicateMsg}
	}
	return err
}

var travelDocumentTypes = []string{domain.TravelDocPassport, domain.TravelDocDriverLicense, domain.TravelDocOtherID}

func (s *travelDocumentService) SearchDocuments(ctx context.Context, f domain.TravelDocumentFilter) (domain.TravelDocumentPage, error) {
	var err error
	if f.Limit, f.Offset, err = travelPage(f.Limit, f.Offset); err != nil {
		return domain.TravelDocumentPage{}, err
	}
	if f.Status != "" {
		if f.Status, err = travelRecordStatus(f.Status, domain.TravelRecordActive, domain.TravelRecordInactive); err != nil {
			return domain.TravelDocumentPage{}, err
		}
	}
	if f.Validity, err = travelValidity(f.Validity); err != nil {
		return domain.TravelDocumentPage{}, err
	}
	if err := travelCheckDate("today", f.Today); err != nil {
		return domain.TravelDocumentPage{}, err
	}
	f.Today = travelToday(f.Today)
	f.Emails, f.Countries, f.DocumentTypes = travelClean(f.Emails), travelClean(f.Countries), travelClean(f.DocumentTypes)
	items, total, err := s.repo.SearchDocuments(ctx, f)
	if err != nil {
		return domain.TravelDocumentPage{}, err
	}
	return domain.TravelDocumentPage{Items: items, Total: total}, nil
}

func (s *travelDocumentService) GetDocument(ctx context.Context, id int) (domain.TravelDocument, error) {
	d, err := s.repo.GetDocument(ctx, id)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	if d == nil {
		return domain.TravelDocument{}, &apierror.NotFoundError{Msg: fmt.Sprintf("travel document %d not found", id)}
	}
	return *d, nil
}

// travelDocumentFields validates what both document writes carry. The
// expiry date is required except for Other ID Document.
func travelDocumentFields(docType, country, issued string, expiry *string) (string, error) {
	t, ok := canonical(strings.TrimSpace(docType), travelDocumentTypes...)
	if !ok {
		return "", &apierror.ValidationError{Msg: "documentType must be Passport, Driver License or Other ID Document"}
	}
	if strings.TrimSpace(country) == "" || issued == "" {
		return "", &apierror.ValidationError{Msg: "issuedCountry and issuedDate are required"}
	}
	if err := travelCheckDate("issuedDate", issued); err != nil {
		return "", err
	}
	if expiry != nil && *expiry == "" {
		expiry = nil
	}
	if expiry == nil {
		if t != domain.TravelDocOtherID {
			return "", &apierror.ValidationError{Msg: "expiryDate is required"}
		}
		return t, nil
	}
	if err := travelCheckDate("expiryDate", *expiry); err != nil {
		return "", err
	}
	if issued > *expiry {
		return "", &apierror.ValidationError{Msg: "issuedDate must not be after expiryDate"}
	}
	return t, nil
}

func (s *travelDocumentService) CreateDocument(ctx context.Context, in domain.CreateTravelDocumentInput) (domain.TravelDocument, error) {
	in.Email, in.CreatedBy, in.DocumentNo = strings.TrimSpace(in.Email), strings.TrimSpace(in.CreatedBy), strings.TrimSpace(in.DocumentNo)
	if in.Email == "" || in.CreatedBy == "" || in.DocumentNo == "" || strings.TrimSpace(in.ImageID) == "" {
		return domain.TravelDocument{}, &apierror.ValidationError{Msg: "email, documentNo, imageId and createdBy are required"}
	}
	var err error
	if in.DocumentType, err = travelDocumentFields(in.DocumentType, in.IssuedCountry, in.IssuedDate, in.ExpiryDate); err != nil {
		return domain.TravelDocument{}, err
	}
	if in.ExpiryDate != nil && *in.ExpiryDate == "" {
		in.ExpiryDate = nil
	}
	id, err := s.repo.CreateDocument(ctx, in)
	if err != nil {
		return domain.TravelDocument{}, err
	}
	return s.GetDocument(ctx, id)
}

func (s *travelDocumentService) UpdateDocument(ctx context.Context, id int, in domain.UpdateTravelDocumentInput) (domain.TravelDocument, error) {
	in.UpdatedBy = strings.TrimSpace(in.UpdatedBy)
	if in.UpdatedBy == "" {
		return domain.TravelDocument{}, &apierror.ValidationError{Msg: "updatedBy is required"}
	}
	if in.DocumentNo != nil {
		v := strings.TrimSpace(*in.DocumentNo)
		if v == "" {
			return domain.TravelDocument{}, &apierror.ValidationError{Msg: "documentNo must not be empty"}
		}
		in.DocumentNo = &v
	}
	var err error
	if in.DocumentType, err = travelDocumentFields(in.DocumentType, in.IssuedCountry, in.IssuedDate, in.ExpiryDate); err != nil {
		return domain.TravelDocument{}, err
	}
	if in.ExpiryDate != nil && *in.ExpiryDate == "" {
		in.ExpiryDate = nil
	}
	if in.Status, err = travelRecordStatus(in.Status, domain.TravelRecordActive, domain.TravelRecordInactive); err != nil {
		return domain.TravelDocument{}, err
	}
	if err := travelRepoError(s.repo.UpdateDocument(ctx, id, in), "travel document", id,
		"There's an already existing document for the same user with same country and document type!"); err != nil {
		return domain.TravelDocument{}, err
	}
	return s.GetDocument(ctx, id)
}
