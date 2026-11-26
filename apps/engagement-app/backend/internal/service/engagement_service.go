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
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/cache"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// EngagementListFilter is GET /engagements.
type EngagementListFilter struct {
	Segment            string
	Search             string
	EngagementTypeIDs  []int
	Accounts           []string
	AllocationTypes    []string
	RequestStatuses    []string
	EngagementStatuses []string
	BusinessUnits      []string
	Countries          []string
	Consultants        string
	StartFrom          string
	StartTo            string
	EndTo              string
	// Sort is one of domain.EngagementSorts; empty or unknown keeps the
	// current-first order.
	Sort string
	// IncludePending adds the Pending Approval engagements the caller may
	// view to the list; false keeps the legacy list without them.
	IncludePending bool
	Limit          int
	Offset         int
}

// EngagementService is the engagements and approvals screens.
type EngagementService interface {
	List(ctx context.Context, p domain.Perms, f EngagementListFilter) (domain.EngagementPage, error)
	Facets(ctx context.Context, p domain.Perms, segment string) (domain.EngagementFacets, error)
	Approvals(ctx context.Context, p domain.Perms, segment, search, sort string, limit, offset int) (domain.EngagementPage, error)
	Get(ctx context.Context, p domain.Perms, id string) (domain.EngagementDetail, error)
	Create(ctx context.Context, p domain.Perms, in domain.EngagementCreate) (domain.EngagementDetail, error)
	Update(ctx context.Context, p domain.Perms, id string, in domain.EngagementUpdate) (domain.EngagementDetail, error)
	SetRequestStatus(ctx context.Context, p domain.Perms, id, status string) (domain.EngagementDetail, error)
	SetEngagementStatus(ctx context.Context, p domain.Perms, id string, in domain.EngagementStatusChange) (domain.EngagementDetail, error)
	Approve(ctx context.Context, p domain.Perms, id string) (domain.EngagementDetail, error)
}

// The legacy WSO2 account written on internal engagements and events.
const (
	engagementWSO2Name = "WSO2"
	engagementWSO2Code = "001E0000002SFO2IAO"
)

// engagementNameTTL bounds how long a person's HR name is reused.
const engagementNameTTL = 10 * time.Minute

type engagementService struct {
	entity     EngagementEntityAPI
	refs       ReferenceService
	sales      salesentity.Client
	hr         hrentity.Client
	mail       email.Sender
	recipients config.EmailConfig
	now        func() time.Time
	names      *cache.Cache[string, string]
	campaigns  *CampaignNames
}

// NewEngagementService constructs an EngagementService. refs must be built
// over the same Sales client (it resolves accounts and campaigns).
func NewEngagementService(entity EngagementEntityAPI, refs ReferenceService, sales salesentity.Client, hr hrentity.Client,
	mail email.Sender, recipients config.EmailConfig, now func() time.Time) EngagementService {
	if now == nil {
		now = time.Now
	}
	return &engagementService{entity: entity, refs: refs, sales: sales, hr: hr, mail: mail, recipients: recipients,
		now: now, names: cache.New[string, string](engagementNameTTL), campaigns: NewCampaignNames(sales)}
}

// fillCampaigns resolves the campaign names of a page of engagements.
func (s *engagementService) fillCampaigns(ctx context.Context, items []domain.EngagementSummary) {
	refs := make([]*domain.CampaignRef, 0, len(items))
	for i := range items {
		refs = append(refs, items[i].Campaign)
	}
	s.campaigns.Fill(ctx, refs...)
}

func engagementVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func engagementState0(r engagemententity.EngagementRecord) engagementState {
	return engagementState{typeID: r.EngagementTypeID, requestStatus: engagementVal(r.RequestStatus), createdBy: engagementVal(r.CreatedBy)}
}

func engagementForbidden() error {
	return &apierror.ForbiddenError{Msg: "You do not have permission to perform this action."}
}

func engagementOneOf(v string, list []string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// engagementPartner returns the display name and partner of a partner
// consultant's pseudo-email partner-allocations-group+PARTNER+FIRST+LAST@wso2.com.
func engagementPartner(email string) (name, partner string, ok bool) {
	local, _, found := strings.Cut(strings.ToLower(email), "@")
	if !found || !strings.HasPrefix(local, "partner-allocations-group+") {
		return "", "", false
	}
	orig := email[:len(local)]
	parts := strings.Split(orig, "+")
	clean := func(s string) string { return strings.ReplaceAll(s, "_", " ") }
	switch len(parts) {
	case 4:
		return clean(parts[2]) + " " + clean(parts[3]) + " (Partner)", clean(parts[1]), true
	case 3:
		return clean(parts[2]) + " (Partner)", clean(parts[1]), true
	}
	return "", "", false
}

func engagementPerson(email string, name *string) domain.Person {
	if n, _, ok := engagementPartner(email); ok {
		return domain.Person{Email: email, Name: &n}
	}
	return domain.Person{Email: email, Name: name}
}

// engagementDisplayStatus is the Engagement Status shown: Cancelled when the
// request is Cancelled, else the stored value or Pending.
func engagementDisplayStatus(r engagemententity.EngagementRecord) string {
	if engagementVal(r.RequestStatus) == domain.EngagementRequestCancelled {
		return domain.EngagementStatusCancelled
	}
	if s := engagementVal(r.EngagementStatus); s != "" {
		return s
	}
	return domain.EngagementStatusPending
}

func engagementFunding(rows []engagemententity.EngagementFundingRow) []domain.EngagementFundingSource {
	out := make([]domain.EngagementFundingSource, len(rows))
	for i, f := range rows {
		out[i] = domain.EngagementFundingSource{Region: f.Region, SubRegion: f.SubRegion, BusinessUnit: f.BusinessUnit,
			ProductUnit: f.ProductUnit, Percentage: f.Percentage}
	}
	return out
}

func (s *engagementService) summary(p domain.Perms, r engagemententity.EngagementRecord, withConsultants bool) domain.EngagementSummary {
	st := engagementState0(r)
	sum := domain.EngagementSummary{
		ID: r.ID, Kind: st.kind(), EngagementTypeID: r.EngagementTypeID, EngagementTypeName: engagementVal(r.EngagementTypeName),
		Account: r.CustomerName, Title: engagementVal(r.ProjectName), AllocationType: engagementVal(r.AllocationTypeName),
		EngagementCode: engagementVal(r.EngagementCode), Nature: engagementVal(r.EngagementNature), Country: engagementVal(r.Country),
		FundingSources: engagementFunding(r.FundingSources), StartDate: engagementVal(r.StartDate), EndDate: engagementVal(r.EndDate),
		RequestStatus: engagementVal(r.RequestStatus), EngagementStatus: engagementDisplayStatus(r), CreatedOn: r.CreatedOn,
		Actions: engagementActionsFor(p, st),
	}
	if r.CampaignID != nil && *r.CampaignID != "" {
		sum.Campaign = &domain.CampaignRef{ID: *r.CampaignID} // name: s.campaigns.Fill
	}
	if r.CreatedBy != nil && *r.CreatedBy != "" {
		sum.CreatedBy = &domain.Person{Email: *r.CreatedBy}
	}
	if st.kind() != domain.EngagementKindMarketing && len(r.FundingSources) > 0 && r.FundingSources[0].BusinessUnit != "" {
		bu := r.FundingSources[0].BusinessUnit
		sum.BusinessUnit = &bu
	}
	if withConsultants && engagementSeesConsultants(p, st.kind()) {
		sum.Consultants = make([]domain.Person, 0, len(r.Consultants))
		for _, c := range r.Consultants {
			sum.Consultants = append(sum.Consultants, engagementPerson(c, nil))
		}
	}
	return sum
}

// ── Lists ────────────────────────────────────────────────────────────────────

func engagementCheckKind(segment string) error {
	switch segment {
	case domain.EngagementKindCustomer, domain.EngagementKindInternal, domain.EngagementKindMarketing:
		return nil
	}
	return &apierror.ValidationError{Msg: "segment must be customer, internal or marketing."}
}

func engagementSegmentCounts(p domain.Perms, typeCounts map[int]int) map[string]int {
	out := map[string]int{}
	for _, kind := range []string{domain.EngagementKindCustomer, domain.EngagementKindInternal, domain.EngagementKindMarketing} {
		if !engagementSegmentGate(p, kind) {
			continue
		}
		n := 0
		if engagementRole(p, kind) {
			for _, t := range engagementSegmentTypes(kind) {
				n += typeCounts[t]
			}
		}
		out[kind] = n
	}
	return out
}

// List is the Engagements list: one segment, only engagements the caller's
// engRole covers. Pending Approval is left out (it is on Approvals) unless
// IncludePending asks for the ones the caller may view: all of them with
// engRole, else (internal and marketing only) all of a type the caller
// approves (Allocations 26 / 29) or the caller's own.
func (s *engagementService) List(ctx context.Context, p domain.Perms, f EngagementListFilter) (domain.EngagementPage, error) {
	if err := engagementCheckKind(f.Segment); err != nil {
		return domain.EngagementPage{}, err
	}
	if p.IsAgent() || !engagementSegmentGate(p, f.Segment) {
		return domain.EngagementPage{}, engagementForbidden()
	}
	for _, v := range f.RequestStatuses {
		if !engagementOneOf(v, domain.ReferenceRequestStatuses) {
			return domain.EngagementPage{}, &apierror.ValidationError{Msg: fmt.Sprintf("Unknown request status %q.", v)}
		}
	}
	for _, v := range f.EngagementStatuses {
		if !engagementOneOf(v, domain.ReferenceEngagementStatuses) {
			return domain.EngagementPage{}, &apierror.ValidationError{Msg: fmt.Sprintf("Unknown engagement status %q.", v)}
		}
	}
	if f.Consultants != "" && f.Consultants != "allocated" && f.Consultants != "none" {
		return domain.EngagementPage{}, &apierror.ValidationError{Msg: "consultants must be allocated or none."}
	}
	types := engagementSegmentTypes(f.Segment)
	if len(f.EngagementTypeIDs) > 0 {
		var keep []int
		for _, t := range f.EngagementTypeIDs {
			if engagementOneOfInt(t, types) {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			keep = []int{0} // matches nothing
		}
		types = keep
	}
	allowedTypes := types
	if !engagementRole(p, f.Segment) {
		// The segment is shown (for Create) but its rows need engRole.
		types = []int{0}
	}
	q := engagemententity.EngagementSearchQuery{
		EngagementTypeIDs: types, Search: f.Search, Accounts: f.Accounts, AllocationTypes: f.AllocationTypes,
		RequestStatuses: f.RequestStatuses, EngagementStatuses: f.EngagementStatuses, BusinessUnits: f.BusinessUnits,
		Countries: f.Countries, Consultants: f.Consultants, StartFrom: f.StartFrom, StartTo: f.StartTo, EndTo: f.EndTo,
		Sort: engagementSort(f.Sort, domain.EngagementSorts, "current"), Today: s.now().UTC().Format("2006-01-02"),
		IncludeConsultants: engagementSeesConsultants(p, f.Segment), Limit: f.Limit, Offset: f.Offset,
	}
	pendingOnly := false
	if f.IncludePending && f.Segment != domain.EngagementKindCustomer {
		if engagementRole(p, f.Segment) {
			q.IncludePending = true
		} else if approver, creator := engagementPendingViewer(p, f.Segment); approver || creator {
			// No engRole, so no regular rows: only the Pending Approval ones
			// the caller may view (a creator sees only their own).
			pendingOnly = true
			q.PendingApproval, q.IncludeConsultants, q.EngagementTypeIDs = true, false, allowedTypes
			if !approver {
				q.PendingCreator = p.Email
			}
		}
	}
	res, err := s.entity.EngagementSearch(ctx, q)
	if err != nil {
		return domain.EngagementPage{}, err
	}
	page := domain.EngagementPage{Items: make([]domain.EngagementSummary, 0, len(res.Items)), Total: res.Total,
		SegmentCounts: engagementSegmentCounts(p, res.TypeCounts)}
	if pendingOnly {
		// The counts are of the pending rows only: report just this segment.
		n := 0
		for _, t := range engagementSegmentTypes(f.Segment) {
			n += res.TypeCounts[t]
		}
		page.SegmentCounts = map[string]int{f.Segment: n}
	}
	for _, r := range res.Items {
		page.Items = append(page.Items, s.summary(p, r, true))
	}
	s.fillCampaigns(ctx, page.Items)
	return page, nil
}

// engagementPendingViewer says how a caller without engRole may view Pending
// Approval engagements of an internal or marketing segment: approver (all of
// the type) or creator (their own, the engagement rules' creator scope).
func engagementPendingViewer(p domain.Perms, segment string) (approver, creator bool) {
	if p.IsAgent() || p.Email == "" {
		return false, false
	}
	switch segment {
	case domain.EngagementKindMarketing:
		approver = p.Alloc.Has(29)
	case domain.EngagementKindInternal:
		approver = p.Alloc.Has(26)
	default:
		return false, false
	}
	return approver, true
}

// engagementSort keeps sort when it is a known column sort, else fallback.
func engagementSort(sort string, keys []string, fallback string) string {
	if s := domain.KnownSort(sort, keys); s != "" {
		return s
	}
	return fallback
}

func engagementOneOfInt(v int, list []int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Facets are the distinct accounts, allocation types and countries of a
// segment's engagements.
func (s *engagementService) Facets(ctx context.Context, p domain.Perms, segment string) (domain.EngagementFacets, error) {
	if err := engagementCheckKind(segment); err != nil {
		return domain.EngagementFacets{}, err
	}
	if p.IsAgent() || !engagementSegmentGate(p, segment) {
		return domain.EngagementFacets{}, engagementForbidden()
	}
	if !engagementRole(p, segment) {
		return domain.EngagementFacets{Accounts: []string{}, AllocationTypes: []string{}, Countries: []string{}}, nil
	}
	f, err := s.entity.EngagementFacets(ctx, engagementSegmentTypes(segment))
	if err != nil {
		return domain.EngagementFacets{}, err
	}
	return domain.EngagementFacets(f), nil
}

// Approvals lists the Pending Approval internal engagements (Allocations 26)
// or marketing events (29), oldest first. Legacy did not scope the list to
// the approver.
func (s *engagementService) Approvals(ctx context.Context, p domain.Perms, segment, search, sort string, limit, offset int) (domain.EngagementPage, error) {
	var typeID, priv int
	switch segment {
	case domain.EngagementApprovalsInternal:
		typeID, priv = domain.EngagementTypeInternal, 26
	case domain.EngagementApprovalsEvents, domain.EngagementKindMarketing:
		typeID, priv, segment = domain.EngagementTypeMarketing, 29, domain.EngagementApprovalsEvents
	default:
		return domain.EngagementPage{}, &apierror.ValidationError{Msg: "segment must be internal or events."}
	}
	if !p.Alloc.Has(priv) {
		return domain.EngagementPage{}, engagementForbidden()
	}
	res, err := s.entity.EngagementSearch(ctx, engagemententity.EngagementSearchQuery{
		PendingApproval: true, EngagementTypeIDs: []int{typeID}, Search: search,
		Sort: engagementSort(sort, domain.ApprovalSorts, "created"), Limit: limit, Offset: offset,
	})
	if err != nil {
		return domain.EngagementPage{}, err
	}
	page := domain.EngagementPage{Items: make([]domain.EngagementSummary, 0, len(res.Items)), Total: res.Total, SegmentCounts: map[string]int{}}
	if p.Alloc.Has(26) {
		page.SegmentCounts[domain.EngagementApprovalsInternal] = res.TypeCounts[domain.EngagementTypeInternal]
	}
	if p.Alloc.Has(29) {
		page.SegmentCounts[domain.EngagementApprovalsEvents] = res.TypeCounts[domain.EngagementTypeMarketing]
	}
	for _, r := range res.Items {
		page.Items = append(page.Items, s.summary(p, r, false))
	}
	s.fillCampaigns(ctx, page.Items)
	return page, nil
}

// ── Engagement page ──────────────────────────────────────────────────────────

// lookupNames resolves HR display names, eight at a time. A failed lookup is
// logged and leaves the name empty; a person HR does not know is cached as ""
// so each page view does not ask again.
func (s *engagementService) lookupNames(ctx context.Context, emails []string) map[string]string {
	out := map[string]string{}
	var (
		todo []string
		mu   sync.Mutex
		wg   sync.WaitGroup
		sem  = make(chan struct{}, 8)
	)
	seen := map[string]bool{}
	for _, e := range emails {
		k := strings.ToLower(e)
		if e == "" || seen[k] {
			continue
		}
		seen[k] = true
		if _, _, partner := engagementPartner(e); partner {
			continue
		}
		if n, ok := s.names.Get(k); ok {
			if n != "" {
				out[k] = n
			}
			continue
		}
		todo = append(todo, e)
	}
	for _, e := range todo {
		wg.Add(1)
		go func(e string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			emp, err := s.hr.Employee(ctx, e)
			if err != nil {
				slog.WarnContext(ctx, "engagement: HR name lookup failed", "err", err)
				return
			}
			n := ""
			if emp != nil {
				n = strings.TrimSpace(emp.FirstName + " " + emp.LastName)
			}
			s.names.Set(strings.ToLower(e), n)
			if n == "" {
				return
			}
			mu.Lock()
			out[strings.ToLower(e)] = n
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return out
}

func engagementNamePtr(names map[string]string, email string) *string {
	if n, ok := names[strings.ToLower(email)]; ok {
		return &n
	}
	return nil
}

// engagementTravelRef picks the travel request shown for an allocation: the
// latest one that is not Cancelled, else the latest.
func engagementTravelRef(refs []engagemententity.EngagementTravelRef) *engagemententity.EngagementTravelRef {
	var pick *engagemententity.EngagementTravelRef
	for i := range refs {
		r := &refs[i]
		if engagementVal(r.Status) != "Cancelled" {
			pick = r
		}
	}
	if pick == nil && len(refs) > 0 {
		pick = &refs[len(refs)-1]
	}
	return pick
}

func engagementTerminal(clearance string) bool {
	return strings.Contains(clearance, "Rejected") || strings.Contains(clearance, "Cancelled")
}

// detailScope is the caller's view scope. Without requireScope (the answer to
// a write) a caller with no view scope gets the minimal Consultants scope,
// never the roster.
func detailScope(p domain.Perms, st engagementState, requireScope bool) (string, error) {
	scope := engagementScopeFor(p, st)
	if scope == "" {
		if requireScope {
			return "", engagementForbidden()
		}
		scope = domain.EngagementScopeConsultants
	}
	return scope, nil
}

// detailBase is the engagement page without its allocations.
func (s *engagementService) detailBase(p domain.Perms, r engagemententity.EngagementRecord, st engagementState,
	scope string) domain.EngagementDetail {
	d := domain.EngagementDetail{
		EngagementSummary: s.summary(p, r, false), Scope: scope,
		AccountID: r.CustomerCode, OpportunityID: r.OpportunityID, OpportunityLineItemID: r.OpportunityLineItemID,
		Product: r.ProductName, Address: r.Address, Description: r.Description, CustomerContact: r.CustomerContact,
		BusinessDomain: r.BusinessDomain, OpportunityOwner: r.OpportunityOwner, TechnicalOwner: r.TechnicalOwner,
		ContractsSigned: r.ContractsSigned, EngagementScope: r.EngagementScope, Comments: r.Comment,
		TentativeDates: r.TentativeDates != nil && *r.TentativeDates, Allocations: []domain.EngagementAllocation{},
	}
	if st.kind() != domain.EngagementKindMarketing && len(r.FundingSources) > 0 {
		d.ProductUnit = referencePtr(r.FundingSources[0].ProductUnit)
		d.SalesSubRegion = referencePtr(r.FundingSources[0].SubRegion)
	}
	return d
}

// written answers a write that has already been saved. A failed read here
// must not turn the saved write into an error the client retries (the retry
// would repeat the write and its email), so it logs a warning and answers
// with the page without its allocations.
func (s *engagementService) written(ctx context.Context, p domain.Perms,
	r engagemententity.EngagementRecord) domain.EngagementDetail {
	d, err := s.detail(ctx, p, r, false)
	if err == nil {
		return d
	}
	slog.WarnContext(ctx, "engagement saved but its page could not be read, answering without allocations",
		"engagement", r.ID, "err", err)
	st := engagementState0(r)
	scope, _ := detailScope(p, st, false)
	d = s.detailBase(p, r, st, scope)
	s.campaigns.Fill(ctx, d.Campaign)
	return d
}

// detail assembles the engagement page for the caller.
func (s *engagementService) detail(ctx context.Context, p domain.Perms, r engagemententity.EngagementRecord,
	requireScope bool) (domain.EngagementDetail, error) {
	st := engagementState0(r)
	scope, err := detailScope(p, st, requireScope)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	d := s.detailBase(p, r, st, scope)
	roster, err := s.entity.EngagementRoster(ctx, r.ID, true)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	emails := []string{engagementVal(r.CreatedBy)}
	for _, a := range roster {
		emails = append(emails, a.Email)
	}
	names := s.lookupNames(ctx, emails)
	if d.CreatedBy != nil {
		d.CreatedBy.Name = engagementNamePtr(names, d.CreatedBy.Email)
	}
	preview := domain.EngagementCascadePreview{}
	changeStatus := engagementCanEditAllocationStatus(p, st)
	for _, a := range roster {
		clearance := engagementVal(a.ClearanceStatus)
		travelYes := engagementVal(a.IsTravelRequired) == "Yes"
		preview.ActiveAllocations++
		if (clearance == "Confirmed" || clearance == "Confirmed - Visa Pending") && travelYes {
			for _, t := range a.TravelRequests {
				if engagementVal(t.Status) != "Cancelled" {
					preview.OpenTravelRequests++
				}
			}
		}
		person := engagementPerson(a.Email, engagementNamePtr(names, a.Email))
		if scope == domain.EngagementScopeConsultants {
			if engagementTerminal(clearance) {
				continue
			}
			d.Allocations = append(d.Allocations, domain.EngagementAllocation{
				Consultant: person, StartDate: engagementVal(a.StartDate), EndDate: engagementVal(a.EndDate),
			})
			continue
		}
		view := domain.EngagementAllocation{
			ID: a.ID, EngagementID: r.ID, Consultant: person, StartDate: engagementVal(a.StartDate), EndDate: engagementVal(a.EndDate),
			TravelRequired: &travelYes, ClearanceStatus: a.ClearanceStatus, Comment: a.Comment, AddedBy: a.AddedBy,
			AddedOn: a.AddedDate, Actions: &domain.EngagementAllocationActions{ChangeStatus: changeStatus},
		}
		if _, partner, ok := engagementPartner(a.Email); ok {
			view.Partner = &partner
		}
		if engagementVal(a.IsRecurring) == "Yes" && engagementVal(a.StartTime) != "" && engagementVal(a.EndTime) != "" {
			view.Hourly = &domain.EngagementHourly{TimeZone: engagementVal(a.TimeZone),
				StartTime: engagementHHMM(engagementVal(a.StartTime)), EndTime: engagementHHMM(engagementVal(a.EndTime))}
		}
		if scope == domain.EngagementScopeFull && travelYes {
			if t := engagementTravelRef(a.TravelRequests); t != nil {
				canOpen := p.Travel.Any(1, 2, 5) || (p.Travel.Has(36) && strings.EqualFold(a.Email, p.Email))
				view.TravelRequest = &domain.EngagementTravelLink{JobNumber: t.JobNumber, CanOpen: canOpen}
			}
		}
		d.Allocations = append(d.Allocations, view)
	}
	if engagementCanSetStatus(p, st) {
		d.CascadePreview = &preview
	}
	s.campaigns.Fill(ctx, d.Campaign)
	return d, nil
}

// engagementDay keeps the YYYY-MM-DD of a Salesforce date or date-time.
func engagementDay(v string) string {
	if len(v) > 10 {
		return v[:10]
	}
	return v
}

func engagementHHMM(t string) string {
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

func (s *engagementService) Get(ctx context.Context, p domain.Perms, id string) (domain.EngagementDetail, error) {
	r, err := s.entity.EngagementGet(ctx, id)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	return s.detail(ctx, p, r, true)
}

// ── Create ───────────────────────────────────────────────────────────────────

func engagementValidation(msg string) error { return &apierror.ValidationError{Msg: msg} }

func engagementUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func engagementCheckDates(start, end string) error {
	if start == "" || end == "" {
		return engagementValidation("Add the start and end dates.")
	}
	s, err1 := time.Parse("2006-01-02", start)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return engagementValidation("Dates must be YYYY-MM-DD.")
	}
	if e.Before(s) {
		return engagementValidation("The end date is before the start date.")
	}
	return nil
}

func engagementNatures(kind string) []string {
	if kind == domain.EngagementKindMarketing {
		return domain.ReferenceEventNatures
	}
	return domain.ReferenceEngagementNatures
}

// engagementAllocationType finds an Active allocation type of the
// engagement type.
func engagementAllocationType(ref domain.ReferenceData, id, typeID int) *domain.ReferenceAllocationType {
	for i := range ref.AllocationTypes {
		at := &ref.AllocationTypes[i]
		if at.ID == id && at.EngagementTypeID == typeID && strings.EqualFold(at.Status, "Active") {
			return at
		}
	}
	return nil
}

func engagementAllocationTypeByID(ref domain.ReferenceData, id int) *domain.ReferenceAllocationType {
	for i := range ref.AllocationTypes {
		if ref.AllocationTypes[i].ID == id {
			return &ref.AllocationTypes[i]
		}
	}
	return nil
}

// engagementDefaultCode is the internal / event code: the type's default
// code plus "-" and its suffix when it has one.
func engagementDefaultCode(at *domain.ReferenceAllocationType) string {
	code := engagementVal(at.DefaultEngagementCode)
	if suf := engagementVal(at.EngagementCodeSuffix); suf != "" {
		code += "-" + suf
	}
	return code
}

// engagementFundingRow builds the single 100% funding source the customer
// and internal forms save from Business Unit, Product Unit and Sales Sub
// Region (the region is the sub region's).
func engagementFundingRow(ref domain.ReferenceData, bu, pu, subRegion string) (engagemententity.EngagementFundingRow, error) {
	row := engagemententity.EngagementFundingRow{BusinessUnit: bu, ProductUnit: pu, SubRegion: subRegion, Percentage: 100}
	if bu == "" || !engagementHasName(ref.BusinessUnits, bu) {
		return row, engagementValidation("Select the business unit.")
	}
	if pu == "" || !engagementHasPU(ref, pu) {
		return row, engagementValidation("Select the product unit.")
	}
	region, ok := engagementRegionOf(ref, subRegion)
	if subRegion == "" || !ok {
		return row, engagementValidation("Select the sales sub region.")
	}
	row.Region = region
	return row, nil
}

func engagementHasName(items []domain.ReferenceNamedItem, name string) bool {
	for _, it := range items {
		if it.Name == name {
			return true
		}
	}
	return false
}

func engagementHasPU(ref domain.ReferenceData, name string) bool {
	for _, it := range ref.ProductUnits {
		if it.Name == name {
			return true
		}
	}
	return false
}

func engagementRegionOf(ref domain.ReferenceData, subRegion string) (string, bool) {
	for _, it := range ref.SubRegions {
		if it.Name == subRegion {
			return engagementVal(it.Region), true
		}
	}
	return "", false
}

// engagementMarketingFunding validates the funding grid (legacy messages).
func engagementMarketingFunding(ref domain.ReferenceData, rows []domain.EngagementFundingSource) ([]engagemententity.EngagementFundingRow, error) {
	if len(rows) == 0 {
		return nil, engagementValidation("Every funding source needs a region and a business unit.")
	}
	var total float64
	out := make([]engagemententity.EngagementFundingRow, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		r.Region, r.SubRegion = strings.TrimSpace(r.Region), strings.TrimSpace(r.SubRegion)
		r.BusinessUnit, r.ProductUnit = strings.TrimSpace(r.BusinessUnit), strings.TrimSpace(r.ProductUnit)
		if r.Region == "" || r.BusinessUnit == "" {
			return nil, engagementValidation("Every funding source needs a region and a business unit.")
		}
		if r.SubRegion == "" || r.ProductUnit == "" {
			return nil, engagementValidation("Every funding source needs a sub region and a product unit.")
		}
		if !engagementHasName(ref.Regions, r.Region) || !engagementHasName(ref.BusinessUnits, r.BusinessUnit) ||
			!engagementHasPU(ref, r.ProductUnit) {
			return nil, engagementValidation("A funding source names an unknown region, business unit or product unit.")
		}
		if reg, ok := engagementRegionOf(ref, r.SubRegion); !ok || reg != r.Region {
			return nil, engagementValidation("A funding source's sub region is not in its region.")
		}
		if r.Percentage <= 0 {
			return nil, engagementValidation("A funding source cannot be 0%.")
		}
		if r.Percentage > 100 {
			return nil, engagementValidation("A funding source cannot be more than 100%.")
		}
		key := r.Region + "|" + r.SubRegion + "|" + r.BusinessUnit + "|" + r.ProductUnit
		if seen[key] {
			return nil, engagementValidation("The same funding source is added twice.")
		}
		seen[key] = true
		total += r.Percentage
		out = append(out, engagemententity.EngagementFundingRow{BusinessUnit: r.BusinessUnit, ProductUnit: r.ProductUnit,
			Region: r.Region, SubRegion: r.SubRegion, Percentage: r.Percentage})
	}
	if total = math.Round(total*100) / 100; total != 100 {
		return nil, engagementValidation(fmt.Sprintf("Funding sources add up to %s%%. They must total 100%%.", strconv.FormatFloat(total, 'f', -1, 64)))
	}
	return out, nil
}

func engagementPaid(typeID int) bool {
	return typeID == domain.EngagementTypePaid || typeID == domain.EngagementTypePaidPostSale || typeID == domain.EngagementTypePaidPreSale
}

// Create ports POST /engagements with the code, Salesforce and campaign
// values resolved on the server (the legacy webapp computed them).
func (s *engagementService) Create(ctx context.Context, p domain.Perms, in domain.EngagementCreate) (domain.EngagementDetail, error) {
	ref, err := s.refs.ReferenceData(ctx)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	var engType *domain.ReferenceEngagementType
	for i := range ref.EngagementTypes {
		if ref.EngagementTypes[i].ID == in.EngagementTypeID && ref.EngagementTypes[i].IsActive {
			engType = &ref.EngagementTypes[i]
		}
	}
	if engType == nil {
		return domain.EngagementDetail{}, engagementValidation("Select the engagement type.")
	}
	if !p.Alloc.Has(engagementCreatePrivilege(in.EngagementTypeID)) {
		return domain.EngagementDetail{}, engagementForbidden()
	}
	kind := domain.EngagementKindOf(in.EngagementTypeID)
	at := engagementAllocationType(ref, in.AllocationTypeID, in.EngagementTypeID)
	if at == nil {
		return domain.EngagementDetail{}, engagementValidation("Select the allocation type.")
	}
	if !engagementOneOf(in.Nature, engagementNatures(kind)) {
		return domain.EngagementDetail{}, engagementValidation("Select the location.")
	}
	if !engagementOneOf(in.Country, domain.ReferenceCountries) {
		return domain.EngagementDetail{}, engagementValidation("Select the country.")
	}
	if strings.TrimSpace(in.Address) == "" {
		return domain.EngagementDetail{}, engagementValidation("Add the address.")
	}
	if len([]rune(in.Address)) > 300 {
		return domain.EngagementDetail{}, engagementValidation("The address can have at most 300 characters.")
	}

	c := engagemententity.EngagementCreateInput{
		AllocationTypeID: at.ID, Address: in.Address, StartDate: in.StartDate, EndDate: in.EndDate,
		EngagementScope: in.EngagementScope, EngagementStatus: domain.EngagementStatusPending, EngagementNature: in.Nature,
		Comment: in.Comments, Country: in.Country, UUID: engagementUUID(), CreatedBy: p.Email,
		CustomerName: engagementWSO2Name, CustomerCode: engagementWSO2Code,
		RequestStatus: domain.EngagementRequestPendingApproval, FundingSources: []engagemententity.EngagementFundingRow{},
	}
	switch kind {
	case domain.EngagementKindCustomer:
		if err := s.createCustomer(ctx, ref, at, in, &c); err != nil {
			return domain.EngagementDetail{}, err
		}
	case domain.EngagementKindInternal:
		title := strings.TrimSpace(in.Title)
		if title == "" {
			return domain.EngagementDetail{}, engagementValidation("Add the name or purpose.")
		}
		if len([]rune(title)) > 100 {
			return domain.EngagementDetail{}, engagementValidation("The name or purpose can have at most 100 characters.")
		}
		row, err := engagementFundingRow(ref, in.BusinessUnit, in.ProductUnit, in.SalesSubRegion)
		if err != nil {
			return domain.EngagementDetail{}, err
		}
		c.ProjectName, c.Description, c.EngagementCode = title, in.Description, engagementDefaultCode(at)
		c.FundingSources = []engagemententity.EngagementFundingRow{row}
	case domain.EngagementKindMarketing:
		// Event Name is free text (sent to NetSuite); the campaign is separate.
		title := strings.TrimSpace(in.Title)
		if title == "" {
			return domain.EngagementDetail{}, engagementValidation("Add the event name.")
		}
		if len([]rune(title)) > 100 {
			return domain.EngagementDetail{}, engagementValidation("The event name can have at most 100 characters.")
		}
		camp, err := s.refsCampaign(ctx, in.CampaignID, in.CampaignName)
		if err != nil {
			return domain.EngagementDetail{}, err
		}
		rows, err := engagementMarketingFunding(ref, in.FundingSources)
		if err != nil {
			return domain.EngagementDetail{}, err
		}
		id := camp.ID
		c.ProjectName, c.CampaignID, c.Description, c.EngagementCode = title, &id, in.Description, engagementDefaultCode(at)
		c.FundingSources = rows
	}
	if err := engagementCheckDates(c.StartDate, c.EndDate); err != nil {
		return domain.EngagementDetail{}, err
	}

	rec, err := s.entity.EngagementCreate(ctx, c)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	s.sendEmail(ctx, p.Email, email.EngagementCreated, rec, at, nil)
	return s.written(ctx, p, rec), nil
}

func (s *engagementService) refsCampaign(ctx context.Context, id, name string) (*domain.ReferenceCampaign, error) {
	camp, err := s.refs.Campaign(ctx, id, name)
	if err != nil {
		return nil, err
	}
	if camp == nil {
		return nil, engagementValidation("Select the campaign.")
	}
	return camp, nil
}

// createCustomer resolves the account, opportunity and line item and the
// engagement code (legacy EngagementForm codeFor rules).
func (s *engagementService) createCustomer(ctx context.Context, ref domain.ReferenceData, at *domain.ReferenceAllocationType,
	in domain.EngagementCreate, c *engagemententity.EngagementCreateInput) error {
	if strings.TrimSpace(in.AccountID) == "" {
		return engagementValidation("Select an account from Salesforce.")
	}
	acct, err := s.refs.Account(ctx, in.AccountID)
	if err != nil {
		return err
	}
	if acct == nil || referenceStr(acct.Name) == "" {
		return engagementValidation("Select an account from Salesforce.")
	}
	if strings.TrimSpace(in.OpportunityID) == "" {
		return engagementValidation("Select the opportunity.")
	}
	opps, err := s.sales.Opportunities(ctx, in.AccountID, in.EngagementTypeID != domain.EngagementTypeNonPaidPost)
	if err != nil {
		return err
	}
	var opp *salesentity.SalesOpportunity
	for i := range opps {
		if opps[i].ID == in.OpportunityID {
			opp = &opps[i]
		}
	}
	if opp == nil {
		return engagementValidation("Select the opportunity.")
	}
	var item *salesentity.SalesLineItem
	needItem := in.EngagementTypeID == domain.EngagementTypePaidPostSale || in.EngagementTypeID == domain.EngagementTypePaidPreSale ||
		in.EngagementTypeID == domain.EngagementTypePaid
	if in.EngagementTypeID != domain.EngagementTypeNonPaidPre && strings.TrimSpace(in.OpportunityLineItemID) != "" {
		items, err := s.sales.OpportunityLineItems(ctx, opp.ID)
		if err != nil {
			return err
		}
		for i := range items {
			if referenceStr(items[i].ID) == in.OpportunityLineItemID {
				item = &items[i]
			}
		}
		if item == nil {
			return engagementValidation("Select the opportunity line item.")
		}
	}
	if needItem && item == nil {
		return engagementValidation("Select the opportunity line item.")
	}
	if strings.TrimSpace(in.CustomerContact) == "" {
		return engagementValidation("Add the customer contact.")
	}

	switch in.EngagementTypeID {
	case domain.EngagementTypeNonPaidPre:
		c.EngagementCode = strings.TrimSpace(strings.Split(referenceStr(opp.PreSalesCode), "(")[0])
	case domain.EngagementTypeNonPaidPost:
		suffix := strings.TrimSpace(engagementVal(at.EngagementCodeSuffix))
		if suffix == "" {
			return engagementValidation("Engagement Code cannot be set. Engagement Code Suffix is not available for the selected Allocation Type.")
		}
		if base := strings.TrimSpace(strings.Split(referenceStr(opp.PostSalesEngagementCode), "-")[0]); base != "" {
			c.EngagementCode = base + "-" + suffix
		}
	default:
		// Only types 2, 6 and 7 require a line item; another customer type
		// without one has no code to take.
		if item == nil {
			return engagementValidation("Select the opportunity line item.")
		}
		c.EngagementCode = strings.TrimSpace(referenceStr(item.EngagementCodeAuto))
	}
	if c.EngagementCode == "" {
		return engagementValidation("Engagement Code cannot be set. The selected opportunity has no engagement code.")
	}
	bu, pu := in.BusinessUnit, in.ProductUnit
	if engagementPaid(in.EngagementTypeID) && item != nil {
		// Paid: dates, business and product unit come from the line item.
		if v := referenceStr(item.ServiceStartDate); v != "" {
			c.StartDate = engagementDay(v)
		}
		if v := referenceStr(item.ServiceEndDate); v != "" {
			c.EndDate = engagementDay(v)
		}
		if v := referenceStr(item.BusinessUnit); v != "" {
			bu = v
		}
		if v := referenceStr(item.ProductUnit); v != "" {
			pu = v
		}
		c.Description = referenceStr(item.Description)
	}
	subRegion := in.SalesSubRegion
	if v := referenceStr(acct.SubRegion); v != "" {
		subRegion = v
	}
	row, err := engagementFundingRow(ref, bu, pu, subRegion)
	if err != nil {
		return err
	}
	c.FundingSources = []engagemententity.EngagementFundingRow{row}
	c.CustomerName, c.CustomerCode = referenceStr(acct.Name), acct.ID
	c.ProjectName, c.OpportunityID = opp.Name, opp.ID
	if opp.Owner != nil {
		c.OpportunityOwner = referenceStr(opp.Owner.Name)
	}
	c.TechnicalOwner = referenceStr(opp.TechnicalOwner)
	c.ContractsSigned = opp.IsWon != nil && *opp.IsWon
	if item != nil {
		c.OpportunityLineItemID, c.ProductName = referenceStr(item.ID), referenceStr(item.Name)
	}
	c.BusinessDomain, c.CustomerContact, c.TentativeDates = in.BusinessDomain, in.CustomerContact, in.TentativeDates
	// Paid engagements take Tentative Dates from the line item, as legacy did.
	if engagementPaid(in.EngagementTypeID) && item != nil {
		c.TentativeDates = item.IsTentativeDates != nil && *item.IsTentativeDates
	}
	c.RequestStatus = domain.EngagementRequestNew
	return nil
}

// ── Emails ───────────────────────────────────────────────────────────────────

// routing ports the legacy engagement email routing from the allocation
// type's Notify To Allocations: "No" → the requester's reporting lead(s);
// "Yes", or any customer engagement → the Allocations group; else nobody.
func (s *engagementService) routing(ctx context.Context, r engagemententity.EngagementRecord, at *domain.ReferenceAllocationType) []string {
	notify := ""
	if at != nil {
		notify = engagementVal(at.EmailToAllocations)
	}
	internal := r.EngagementTypeID == domain.EngagementTypeInternal || r.EngagementTypeID == domain.EngagementTypeMarketing
	switch {
	case notify == "No":
		leads, err := s.hr.ReportsTo(ctx, engagementVal(r.CreatedBy))
		if err != nil {
			slog.WarnContext(ctx, "Could not resolve the reporting lead, notifying the remaining recipients",
				"employee", engagementVal(r.CreatedBy), "err", err)
			return nil
		}
		return leads
	case notify == "Yes" || !internal:
		if s.recipients.AllocationsGroup != "" {
			return []string{s.recipients.AllocationsGroup}
		}
	}
	return nil
}

// sendEmail sends an engagement email; a failure is logged, never returned.
func (s *engagementService) sendEmail(ctx context.Context, actor string, ev email.EngagementEvent, r engagemententity.EngagementRecord,
	at *domain.ReferenceAllocationType, cancelled []engagemententity.EngagementRosterAllocation) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if at == nil {
		if ref, err := s.refs.ReferenceData(ctx); err == nil {
			at = engagementAllocationTypeByID(ref, r.AllocationTypeID)
		}
	}
	data := email.EngagementEmailData{
		ID: r.ID, EngagementTypeID: r.EngagementTypeID, RequestStatus: engagementVal(r.RequestStatus),
		CustomerName: engagementVal(r.CustomerName), ProjectName: engagementVal(r.ProjectName), ProductName: engagementVal(r.ProductName),
		EngagementCode: engagementVal(r.EngagementCode), Nature: engagementVal(r.EngagementNature), Country: engagementVal(r.Country),
		Address: engagementVal(r.Address), StartDate: engagementVal(r.StartDate), EndDate: engagementVal(r.EndDate),
		CustomerContact: engagementVal(r.CustomerContact), BusinessDomain: engagementVal(r.BusinessDomain),
		OpportunityOwner: engagementVal(r.OpportunityOwner), TechnicalOwner: engagementVal(r.TechnicalOwner),
		ContractsSigned: r.ContractsSigned != nil && *r.ContractsSigned, EngagementScope: engagementVal(r.EngagementScope),
		Comment: engagementVal(r.Comment), Requester: engagementVal(r.CreatedBy),
	}
	if at != nil {
		data.AllocationTypeName = at.AllocationTypeName
	} else {
		data.AllocationTypeName = engagementVal(r.AllocationTypeName)
	}
	if len(r.FundingSources) > 0 {
		f := r.FundingSources[0]
		data.BusinessUnit, data.ProductUnit, data.SubRegion = f.BusinessUnit, f.ProductUnit, f.SubRegion
	}
	rcpt := email.EngagementRecipients{
		AppBaseURL: s.recipients.AppBaseURL, InternalApprovers: s.recipients.InternalApprovers,
		MarketingApprovers: s.recipients.MarketingApprovers, TravelGroup: s.recipients.TravelGroup,
		Routing: s.routing(ctx, r, at),
	}
	for _, a := range cancelled {
		rcpt.Consultants = append(rcpt.Consultants, a.Email)
		cl := engagementVal(a.ClearanceStatus)
		if engagementVal(a.IsTravelRequired) == "Yes" && (cl == "Confirmed" || cl == "Confirmed - Visa Pending") {
			rcpt.TravelAffected = true
		}
	}
	msg := email.EngagementMessage(ev, data, rcpt, actor)
	if err := s.mail.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "engagement email not sent", "type", string(ev), "engagement", r.ID, "err", err)
	}
}

// ── Update, status changes, approve ──────────────────────────────────────────

func engagementConflict(msg string) error { return &apierror.ConflictError{Msg: msg} }

func engagementStr(v string) *string { return &v }

// Update ports the Edit / Edit Pending / Confirm and Approve saves.
func (s *engagementService) Update(ctx context.Context, p domain.Perms, id string, in domain.EngagementUpdate) (domain.EngagementDetail, error) {
	r, err := s.entity.EngagementGet(ctx, id)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	st := engagementState0(r)
	kind := st.kind()
	approve := in.Approve != nil && *in.Approve
	switch {
	case st.pending():
		if !engagementCanEditPending(p, st) {
			return domain.EngagementDetail{}, engagementForbidden()
		}
	case st.cancelled():
		return domain.EngagementDetail{}, engagementConflict("This engagement is cancelled.")
	default:
		if !engagementCanEdit(p, st) {
			return domain.EngagementDetail{}, engagementForbidden()
		}
	}
	if approve && !engagementCanApprove(p, st) {
		if !st.pending() {
			return domain.EngagementDetail{}, engagementConflict("Only engagements waiting for approval can be approved.")
		}
		return domain.EngagementDetail{}, engagementForbidden()
	}
	ref, err := s.refs.ReferenceData(ctx)
	if err != nil {
		return domain.EngagementDetail{}, err
	}

	u := engagemententity.EngagementUpdateInput{UpdatedBy: p.Email}
	changed := false
	locked := func(field string, sent *string, current string, editable bool) (*string, error) {
		if sent == nil {
			return nil, nil
		}
		if !editable {
			if *sent != current {
				return nil, engagementValidation(field + " cannot be changed.")
			}
			return nil, nil
		}
		changed = true
		return sent, nil
	}
	var cur engagemententity.EngagementFundingRow
	if len(r.FundingSources) > 0 {
		cur = r.FundingSources[0]
	}
	paid := engagementPaid(r.EngagementTypeID)

	if in.Nature != nil {
		if !engagementOneOf(*in.Nature, engagementNatures(kind)) {
			return domain.EngagementDetail{}, engagementValidation("Select the location.")
		}
		u.EngagementNature, changed = in.Nature, true
	}
	if in.Address != nil {
		if strings.TrimSpace(*in.Address) == "" {
			return domain.EngagementDetail{}, engagementValidation("Add the address.")
		}
		if len([]rune(*in.Address)) > 300 {
			return domain.EngagementDetail{}, engagementValidation("The address can have at most 300 characters.")
		}
		u.Address, changed = in.Address, true
	}
	if in.CustomerContact != nil {
		if kind == domain.EngagementKindCustomer && strings.TrimSpace(*in.CustomerContact) == "" {
			return domain.EngagementDetail{}, engagementValidation("Add the customer contact.")
		}
		u.CustomerContact, changed = in.CustomerContact, true
	}
	if in.EngagementScope != nil {
		u.EngagementScope, changed = in.EngagementScope, true
	}
	if in.TentativeDates != nil && kind == domain.EngagementKindCustomer {
		u.TentativeDates, changed = in.TentativeDates, true
	}
	if in.Comments != nil {
		u.Comment, changed = in.Comments, true
	}
	if u.Description, err = locked("Description", in.Description, engagementVal(r.Description), kind != domain.EngagementKindCustomer); err != nil {
		return domain.EngagementDetail{}, err
	}
	if u.Country, err = locked("Country", in.Country, engagementVal(r.Country), st.pending()); err != nil {
		return domain.EngagementDetail{}, err
	}
	if u.Country != nil && !engagementOneOf(*u.Country, domain.ReferenceCountries) {
		return domain.EngagementDetail{}, engagementValidation("Select the country.")
	}
	if u.ProjectName, err = locked("Name / Purpose", in.Title, engagementVal(r.ProjectName),
		st.pending() && r.EngagementTypeID == domain.EngagementTypeInternal); err != nil {
		return domain.EngagementDetail{}, err
	}
	if u.ProjectName != nil {
		t := strings.TrimSpace(*u.ProjectName)
		if t == "" {
			return domain.EngagementDetail{}, engagementValidation("Add the name or purpose.")
		}
		if len([]rune(t)) > 100 {
			return domain.EngagementDetail{}, engagementValidation("The name or purpose can have at most 100 characters.")
		}
		u.ProjectName = &t
	}
	if in.CampaignID != nil {
		// Changeable while pending; an event without one may add it later.
		editable := r.EngagementTypeID == domain.EngagementTypeMarketing && (st.pending() || engagementVal(r.CampaignID) == "")
		if !editable {
			if *in.CampaignID != engagementVal(r.CampaignID) {
				return domain.EngagementDetail{}, engagementValidation("Campaign cannot be changed.")
			}
		} else if *in.CampaignID != engagementVal(r.CampaignID) {
			camp, err := s.refsCampaign(ctx, *in.CampaignID, engagementVal(in.CampaignName))
			if err != nil {
				return domain.EngagementDetail{}, err
			}
			id := camp.ID
			u.CampaignID, changed = &id, true
		}
	}

	// Dates: paid engagements take them from Salesforce (Fetch New Dates).
	start, end := in.StartDate, in.EndDate
	if in.RefreshDatesFromSalesforce != nil && *in.RefreshDatesFromSalesforce {
		if !paid || engagementVal(r.OpportunityLineItemID) == "" {
			return domain.EngagementDetail{}, engagementValidation("Fetch New Dates applies to paid engagements only.")
		}
		item, err := s.sales.OpportunityLineItem(ctx, engagementVal(r.OpportunityLineItemID))
		if err != nil {
			return domain.EngagementDetail{}, err
		}
		if item == nil || referenceStr(item.ServiceStartDate) == "" || referenceStr(item.ServiceEndDate) == "" {
			return domain.EngagementDetail{}, engagementValidation("Salesforce has no dates for the opportunity line item.")
		}
		start, end = engagementStr(engagementDay(referenceStr(item.ServiceStartDate))), engagementStr(engagementDay(referenceStr(item.ServiceEndDate)))
	} else if paid {
		if (start != nil && *start != engagementVal(r.StartDate)) || (end != nil && *end != engagementVal(r.EndDate)) {
			return domain.EngagementDetail{}, engagementValidation("The dates of a paid engagement come from Salesforce. Use Fetch New Dates.")
		}
		start, end = nil, nil
	}
	if start != nil || end != nil {
		ns, ne := engagementVal(r.StartDate), engagementVal(r.EndDate)
		if start != nil {
			ns = *start
		}
		if end != nil {
			ne = *end
		}
		if err := engagementCheckDates(ns, ne); err != nil {
			return domain.EngagementDetail{}, err
		}
		u.StartDate, u.EndDate, changed = start, end, true
	}

	// Allocation type (pending only), with the code recomputed.
	var at *domain.ReferenceAllocationType
	if in.AllocationTypeID != nil && *in.AllocationTypeID != r.AllocationTypeID {
		if !st.pending() {
			return domain.EngagementDetail{}, engagementValidation("Allocation Type cannot be changed.")
		}
		at = engagementAllocationType(ref, *in.AllocationTypeID, r.EngagementTypeID)
		if at == nil {
			return domain.EngagementDetail{}, engagementValidation("Select the allocation type.")
		}
		atID := at.ID
		u.AllocationTypeID, u.EngagementCode, changed = &atID, engagementStr(engagementDefaultCode(at)), true
	}

	// Funding: the grid for events, Business Unit / Product Unit / Sales Sub
	// Region (one 100% row) for the others.
	if kind == domain.EngagementKindMarketing {
		if in.BusinessUnit != nil || in.ProductUnit != nil || in.SalesSubRegion != nil {
			return domain.EngagementDetail{}, engagementValidation("A marketing event is funded by its funding sources.")
		}
		if in.FundingSources != nil {
			rows, err := engagementMarketingFunding(ref, in.FundingSources)
			if err != nil {
				return domain.EngagementDetail{}, err
			}
			u.FundingSources, changed = rows, true
		}
	} else {
		if in.FundingSources != nil {
			return domain.EngagementDetail{}, engagementValidation("Funding sources apply to marketing events only.")
		}
		bu, pu, sub := cur.BusinessUnit, cur.ProductUnit, cur.SubRegion
		touched := false
		if in.BusinessUnit != nil && *in.BusinessUnit != bu {
			bu, touched = *in.BusinessUnit, true
		}
		if in.ProductUnit != nil && *in.ProductUnit != pu {
			pu, touched = *in.ProductUnit, true
		}
		if in.SalesSubRegion != nil && *in.SalesSubRegion != sub {
			if !st.pending() {
				return domain.EngagementDetail{}, engagementValidation("Sales Sub Region cannot be changed.")
			}
			sub, touched = *in.SalesSubRegion, true
		}
		if touched {
			row, err := engagementFundingRow(ref, bu, pu, sub)
			if err != nil {
				return domain.EngagementDetail{}, err
			}
			u.FundingSources, changed = []engagemententity.EngagementFundingRow{row}, true
		}
	}

	if !changed && !approve {
		return s.detail(ctx, p, r, false)
	}
	switch {
	case approve:
		u.RequestStatus = engagementStr(domain.EngagementRequestApproved)
	case changed && !st.pending() && st.requestStatus != domain.EngagementRequestCompleted && !st.cancelled():
		u.RequestStatus = engagementStr(domain.EngagementRequestUpdated)
	}
	res, err := s.entity.EngagementUpdate(ctx, id, u)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	switch {
	case approve && changed:
		s.sendEmail(ctx, p.Email, email.EngagementUpdatedAndApproved, res.Engagement, nil, nil)
	case approve:
		s.sendEmail(ctx, p.Email, email.EngagementApproved, res.Engagement, nil, nil)
	case changed:
		s.sendEmail(ctx, p.Email, email.EngagementUpdated, res.Engagement, nil, nil)
	}
	return s.written(ctx, p, res.Engagement), nil
}

// SetRequestStatus is Mark as Completed / Reopen (no email, as legacy).
func (s *engagementService) SetRequestStatus(ctx context.Context, p domain.Perms, id, status string) (domain.EngagementDetail, error) {
	if status != domain.EngagementRequestCompleted && status != domain.EngagementRequestInProgress {
		return domain.EngagementDetail{}, engagementValidation("status must be Completed or In Progress.")
	}
	r, err := s.entity.EngagementGet(ctx, id)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	st := engagementState0(r)
	if !engagementCanComplete(p, st) {
		return domain.EngagementDetail{}, engagementForbidden()
	}
	if st.pending() {
		return domain.EngagementDetail{}, engagementConflict("This engagement is waiting for approval.")
	}
	if st.cancelled() {
		return domain.EngagementDetail{}, engagementConflict("This engagement is cancelled.")
	}
	res, err := s.entity.EngagementUpdate(ctx, id, engagemententity.EngagementUpdateInput{RequestStatus: &status, UpdatedBy: p.Email})
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	return s.written(ctx, p, res.Engagement), nil
}

// SetEngagementStatus is Change Engagement Status. Cancelled also sets the
// request status Cancelled and cascades (entity), and sends the legacy
// cancellation email; other values send nothing.
func (s *engagementService) SetEngagementStatus(ctx context.Context, p domain.Perms, id string, in domain.EngagementStatusChange) (domain.EngagementDetail, error) {
	if !engagementOneOf(in.Status, domain.ReferenceEngagementStatuses) {
		return domain.EngagementDetail{}, engagementValidation("Select the new status")
	}
	r, err := s.entity.EngagementGet(ctx, id)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	st := engagementState0(r)
	if !engagementCanSetStatus(p, st) {
		return domain.EngagementDetail{}, engagementForbidden()
	}
	if st.pending() {
		return domain.EngagementDetail{}, engagementConflict("This engagement is waiting for approval.")
	}
	if st.cancelled() {
		return domain.EngagementDetail{}, engagementConflict("This engagement is cancelled.")
	}
	u := engagemententity.EngagementUpdateInput{EngagementStatus: &in.Status, UpdatedBy: p.Email}
	cancel := in.Status == domain.EngagementStatusCancelled
	if cancel {
		u.RequestStatus = engagementStr(domain.EngagementRequestCancelled)
	}
	res, err := s.entity.EngagementUpdate(ctx, id, u)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	if cancel {
		s.sendEmail(ctx, p.Email, email.EngagementCancelled, res.Engagement, nil, res.CancelledAllocations)
	}
	return s.written(ctx, p, res.Engagement), nil
}

// Approve sets request status Approved (Allocations 28 plus the legacy
// PATCH check) and sends the "Approved" email. There is no reject.
func (s *engagementService) Approve(ctx context.Context, p domain.Perms, id string) (domain.EngagementDetail, error) {
	r, err := s.entity.EngagementGet(ctx, id)
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	st := engagementState0(r)
	if !st.pending() || (r.EngagementTypeID != domain.EngagementTypeInternal && r.EngagementTypeID != domain.EngagementTypeMarketing) {
		return domain.EngagementDetail{}, engagementConflict("Only engagements waiting for approval can be approved.")
	}
	if !engagementCanApprove(p, st) {
		return domain.EngagementDetail{}, engagementForbidden()
	}
	res, err := s.entity.EngagementUpdate(ctx, id, engagemententity.EngagementUpdateInput{
		RequestStatus: engagementStr(domain.EngagementRequestApproved), UpdatedBy: p.Email,
	})
	if err != nil {
		return domain.EngagementDetail{}, err
	}
	s.sendEmail(ctx, p.Email, email.EngagementApproved, res.Engagement, nil, nil)
	return s.written(ctx, p, res.Engagement), nil
}
