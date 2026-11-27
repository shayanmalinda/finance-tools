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
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// TravelOptions configure the travel services.
type TravelOptions struct {
	// VisaNotApplicableID and NoValidVisaID are the visa_status ids of the
	// two legacy sentinels (production 332 and 333, staging 40 and 41).
	VisaNotApplicableID int
	NoValidVisaID       int
	// Email holds the legacy travel email settings.
	Email email.TravelSettings
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Campaigns resolves campaign names; nil leaves them unset.
	Campaigns *CampaignNames
	// Budgets is nil while travel budgets are off: the page then has no
	// budget field at all.
	Budgets BudgetHooks
}

func (o TravelOptions) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// TravelService serves the travel request list, page and updates.
type TravelService interface {
	// List is GET /travel-requests.
	List(ctx context.Context, p domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error)
	// ListMine is GET /me/travel-requests.
	ListMine(ctx context.Context, p domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error)
	// Get is GET /travel-requests/{jobNumber}.
	Get(ctx context.Context, p domain.Perms, jobNumber string) (domain.TravelRequestDetail, error)
	// Update is PATCH /travel-requests/{jobNumber}: assign, or update the
	// status, travel agent and visa.
	Update(ctx context.Context, p domain.Perms, jobNumber string, in domain.TravelRequestUpdate) (domain.TravelRequestDetail, error)
	// VisaOptions is GET /travel-requests/{jobNumber}/visa-options.
	VisaOptions(ctx context.Context, p domain.Perms, jobNumber string) (domain.TravelVisaOptions, error)
}

type travelService struct {
	entity engagemententity.TravelAPI
	people *travelPeople
	mail   email.Sender
	opts   TravelOptions
}

// NewTravelService constructs a TravelService.
func NewTravelService(entity engagemententity.TravelAPI, hr hrentity.Client, mail email.Sender, opts TravelOptions) TravelService {
	return &travelService{entity: entity, people: newTravelPeople(hr), mail: mail, opts: opts}
}

// travelJobNumber is the legacy JOB_NUMBER_FORMAT.
var travelJobNumber = regexp.MustCompile(`^TRV-\d{4}-\d{2}-\d{4}$`)

// travelSegments maps a list segment to its statuses (nil = every status).
var travelSegments = map[string][]string{
	domain.TravelSegmentNew:        {domain.TravelStatusNew},
	domain.TravelSegmentInProgress: {domain.TravelStatusInProgress, domain.TravelStatusExtended, domain.TravelStatusOnHold},
	domain.TravelSegmentCompleted:  {domain.TravelStatusCompleted, domain.TravelStatusCancelled},
	domain.TravelSegmentAll:        nil,
}

func travelForbidden(msg string) error { return &apierror.ForbiddenError{Msg: msg} }

func travelBadRequest(msg string) error { return &apierror.ValidationError{Msg: msg} }

// travelNotFound turns the entity's 404 into a NotFoundError with msg.
func travelNotFound(err error, msg string) error {
	var up *apierror.UpstreamError
	if errors.As(err, &up) && up.StatusCode == 404 {
		return &apierror.NotFoundError{Msg: msg}
	}
	return err
}

func travelStatusesValid(statuses []string) error {
	for _, s := range statuses {
		if !slices.Contains(domain.TravelStatuses, s) {
			return travelBadRequest(fmt.Sprintf("%q is not a travel status.", s))
		}
	}
	return nil
}

func (s *travelService) today() string { return s.opts.now().UTC().Format(time.DateOnly) }

// ── Lists ─────────────────────────────────────────────────────────────────

func (s *travelService) List(ctx context.Context, p domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error) {
	segment := q.Segment
	if segment == "" {
		segment = domain.TravelSegmentAll
	}
	segStatuses, ok := travelSegments[segment]
	if !ok {
		return domain.TravelRequestPage{}, travelBadRequest("segment must be new, in-progress, completed or all.")
	}
	sort := engagemententity.TravelSortCurrentFirst
	if q.Sort != "" {
		if !slices.Contains(domain.TravelSorts, q.Sort) {
			return domain.TravelRequestPage{}, travelBadRequest("sort must be one of " + strings.Join(domain.TravelSorts, ", ") + ".")
		}
		sort = q.Sort
	}
	hasNew := p.Travel.Has(travelPrivNewRequests)
	if segment == domain.TravelSegmentNew && !hasNew {
		return domain.TravelRequestPage{}, travelForbidden("You do not have permission to view new travel requests.")
	}
	if err := travelStatusesValid(q.Statuses); err != nil {
		return domain.TravelRequestPage{}, err
	}

	f := engagemententity.TravelRequestSearch{
		TravellerEmails: q.Travellers, Countries: q.Countries, StartFrom: q.StartFrom, EndTo: q.EndTo,
		Search: q.Search, Sort: sort, Today: s.today(),
		Limit: q.Limit, Offset: q.Offset, WithStatusCounts: true,
	}
	// Travel 1 or 2 see every request; an agent (18 only) sees its agency's
	// (legacy agent-travel-search), without the engagement or agency search.
	if p.Travel.Any(travelPrivNewRequests, travelPrivAllRequests) {
		f.AgencyNames, f.SearchEngagements, f.SearchAgency = q.Agencies, true, true
	} else {
		if p.Agency == nil || !p.Travel.Has(travelPrivAgentRequests) {
			return travelEmptyPage(hasNew), nil
		}
		id := p.Agency.ID
		f.TravelAgentID = &id
	}

	// The segment and the status filter intersect; an empty intersection
	// still returns the segment counts.
	f.Statuses = segStatuses
	none := false
	if len(q.Statuses) > 0 {
		if segStatuses == nil {
			f.Statuses = q.Statuses
		} else {
			f.Statuses = nil
			for _, st := range segStatuses {
				if slices.Contains(q.Statuses, st) {
					f.Statuses = append(f.Statuses, st)
				}
			}
			if len(f.Statuses) == 0 {
				none, f.Statuses, f.Limit = true, q.Statuses, 1
			}
		}
	}
	page, err := s.entity.SearchTravelRequests(ctx, f)
	if err != nil {
		return domain.TravelRequestPage{}, err
	}
	if none {
		page.Items, page.Total = nil, 0
	}
	out := domain.TravelRequestPage{
		Items: s.summaries(ctx, p, page.Items, false), Total: page.Total,
		SegmentCounts: travelSegmentCounts(page.StatusCounts, q.Statuses, hasNew),
	}
	return out, nil
}

func travelEmptyPage(hasNew bool) domain.TravelRequestPage {
	return domain.TravelRequestPage{Items: []domain.TravelRequestSummary{}, SegmentCounts: travelSegmentCounts(nil, nil, hasNew)}
}

// travelSegmentCounts sums the per-status counts into the segments the
// caller has, keeping only the statuses of the status filter.
func travelSegmentCounts(counts map[string]int, filter []string, hasNew bool) map[string]int {
	allowed := func(st string) bool { return len(filter) == 0 || slices.Contains(filter, st) }
	out := map[string]int{}
	for seg, statuses := range travelSegments {
		if seg == domain.TravelSegmentNew && !hasNew {
			continue
		}
		n := 0
		if statuses == nil {
			for st, c := range counts {
				if allowed(st) {
					n += c
				}
			}
		} else {
			for _, st := range statuses {
				if allowed(st) {
					n += counts[st]
				}
			}
		}
		out[seg] = n
	}
	return out
}

func (s *travelService) ListMine(ctx context.Context, p domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error) {
	if err := travelStatusesValid(q.Statuses); err != nil {
		return domain.TravelRequestPage{}, err
	}
	page, err := s.entity.SearchTravelRequests(ctx, engagemententity.TravelRequestSearch{
		TravellerEmails: []string{p.Email}, Statuses: q.Statuses, Countries: q.Countries,
		StartFrom: q.StartFrom, EndTo: q.EndTo, Search: q.Search, SearchEngagements: true, SearchAgency: true,
		Sort: engagemententity.TravelSortStartDesc, Today: s.today(), Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		return domain.TravelRequestPage{}, err
	}
	return domain.TravelRequestPage{Items: s.summaries(ctx, p, page.Items, true), Total: page.Total}, nil
}

// summaries builds the list rows; own rows are read-only (no actions).
func (s *travelService) summaries(ctx context.Context, p domain.Perms, recs []engagemententity.TravelRequestRecord, own bool) []domain.TravelRequestSummary {
	emails := make([]string, 0, len(recs))
	for _, r := range recs {
		emails = append(emails, r.TravellerEmail)
	}
	people := s.people.many(ctx, emails)
	out := make([]domain.TravelRequestSummary, 0, len(recs))
	for _, r := range recs {
		scope := domain.TravelScopeOwn
		if !own {
			scope = travelScope(p, r.TravellerEmail, r.TravelAgentID)
		}
		out = append(out, s.summary(p, r, scope, s.people.of(people, r.TravellerEmail)))
	}
	refs := make([]*domain.CampaignRef, 0, len(out))
	for i := range out {
		refs = append(refs, out[i].Campaign)
	}
	s.opts.Campaigns.Fill(ctx, refs...)
	return out
}

func travelOptional(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func travelDeref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// visaLabel is the list's Visa column: a sentinel, "VisaType (EntryType)"
// with its validity, or the legacy free text.
func (s *travelService) visaLabel(r engagemententity.TravelRequestRecord) (label, validity *string) {
	if r.VisaID != nil {
		switch *r.VisaID {
		case s.opts.VisaNotApplicableID:
			v := domain.TravelVisaNotApplicable
			return &v, nil
		case s.opts.NoValidVisaID:
			v := domain.TravelNoValidVisa
			return &v, nil
		}
	}
	if v := r.Visa; v != nil {
		l := travelDeref(v.VisaType)
		if e := travelDeref(v.EntryType); e != "" {
			l = strings.TrimSpace(l + " (" + e + ")")
		}
		if v.StartDate != nil || v.EndDate != nil {
			val := travelDeref(v.StartDate) + " to " + travelDeref(v.EndDate)
			validity = &val
		}
		return travelOptional(l), validity
	}
	if r.VisaText != nil {
		return travelOptional(*r.VisaText), nil
	}
	return nil, nil
}

func (s *travelService) summary(p domain.Perms, r engagemententity.TravelRequestRecord, scope string, person travelPerson) domain.TravelRequestSummary {
	sum := domain.TravelRequestSummary{
		JobNumber: r.JobNumber, Status: r.Status,
		Traveller:    domain.Person{Email: r.TravellerEmail, Name: person.Name},
		BaseLocation: person.Location, City: r.City, Country: travelOptional(r.Country),
		StartDate: r.StartDate, EndDate: r.EndDate,
	}
	if scope != domain.TravelScopeAgency {
		sum.Agency = r.TravelAgentName
	}
	sum.Visa, sum.VisaValidity = s.visaLabel(r)
	if e := r.Engagement; e != nil && travelSees(scope, "engagement") {
		ref := &domain.TravelEngagementRef{ID: e.ID, Title: travelDeref(e.Title), EngagementTypeID: e.TypeID, EngagementTypeName: e.TypeName}
		if travelDeref(e.Kind) == "Customer" {
			ref.Account = e.CustomerName
		}
		sum.Engagement = ref
		if c := travelDeref(e.CampaignID); c != "" {
			sum.Campaign = &domain.CampaignRef{ID: c} // name: Campaigns.Fill
		}
	}
	sum.Actions = travelActions(p, scope, r.Status)
	return sum
}

// ── Travel request page ───────────────────────────────────────────────────

func (s *travelService) load(ctx context.Context, jobNumber string) (engagemententity.TravelRequestRecord, error) {
	if !travelJobNumber.MatchString(jobNumber) {
		return engagemententity.TravelRequestRecord{}, travelBadRequest("Invalid job number format")
	}
	r, err := s.entity.GetTravelRequest(ctx, jobNumber)
	if err != nil {
		return engagemententity.TravelRequestRecord{}, travelNotFound(err, "Travel request "+jobNumber+" was not found.")
	}
	return r, nil
}

const travelNoAccess = "You don't have access to this travel request."

func (s *travelService) Get(ctx context.Context, p domain.Perms, jobNumber string) (domain.TravelRequestDetail, error) {
	r, err := s.load(ctx, jobNumber)
	if err != nil {
		return domain.TravelRequestDetail{}, err
	}
	scope := travelScope(p, r.TravellerEmail, r.TravelAgentID)
	if scope == "" {
		return domain.TravelRequestDetail{}, travelForbidden(travelNoAccess)
	}
	return s.detail(ctx, p, r, scope), nil
}

func (s *travelService) detail(ctx context.Context, p domain.Perms, r engagemententity.TravelRequestRecord, scope string) domain.TravelRequestDetail {
	d := domain.TravelRequestDetail{
		TravelRequestSummary: s.summary(p, r, scope, travelPerson{}),
		Scope:                scope, VisaID: r.VisaID, Address: r.City, Tabs: travelTabs(p, scope),
	}
	// The HR lookup, campaign name and tab counts do not depend on each other.
	var (
		wg     sync.WaitGroup
		person travelPerson
		budget *domain.TripBudgetView
	)
	goSafe(ctx, &wg, nil, func() { person = s.people.get(ctx, r.TravellerEmail) })
	goSafe(ctx, &wg, nil, func() { s.opts.Campaigns.Fill(ctx, d.Campaign) })
	goSafe(ctx, &wg, nil, func() { d.Counts = s.counts(ctx, p, r.JobNumber, d.Tabs) })
	if s.opts.Budgets != nil {
		goSafe(ctx, &wg, nil, func() { budget = s.opts.Budgets.TripView(ctx, p, r.JobNumber) })
	}
	wg.Wait()
	if s.opts.Budgets != nil {
		d.Budget = budget // a typed nil is written as null
	}
	d.Traveller.Name, d.BaseLocation = person.Name, person.Location
	if travelSees(scope, "allocation") {
		d.AllocationID = r.AllocationID
	}
	if travelSees(scope, "engagement") {
		// The live engagement values, as the legacy overlay; the values
		// copied into travel when the engagement is not found.
		d.EngagementCode, d.EngagementType = travelOptional(r.EngagementCode), travelOptional(r.EngagementType)
		if e := r.Engagement; e != nil {
			if e.EngagementCode != nil {
				d.EngagementCode = e.EngagementCode
			}
			if e.TypeName != nil {
				d.EngagementType = e.TypeName
			}
		}
	}
	if p.Travel.Has(travelPrivFundingSources) && travelSees(scope, "funding") {
		d.FundingSources = []domain.TravelFundingSource{}
		for _, f := range r.FundingSources {
			d.FundingSources = append(d.FundingSources, domain.TravelFundingSource{
				Region: travelDeref(f.Region), SubRegion: f.SubRegion, BusinessUnit: travelDeref(f.BusinessUnit),
				ProductUnit: f.ProductUnit, Percentage: f.Percentage,
			})
		}
	}
	return d
}

// counts reads the row counts of the visible tabs; nil when none is visible or
// the entity call fails (the page opens without badges).
func (s *travelService) counts(ctx context.Context, p domain.Perms, jobNumber string, t domain.TravelRequestTabs) *domain.TravelRequestCounts {
	var include []string
	for _, c := range []struct {
		name string
		on   bool
	}{{"perDiems", t.PerDiems}, {"invoices", t.Invoices}, {"payments", t.Payments},
		{"cardExpenses", t.Expenses}, {"expenseClaims", t.Expenses}} {
		if c.on {
			include = append(include, c.name)
		}
	}
	if len(include) == 0 {
		return nil
	}
	c, err := s.entity.TravelCounts(ctx, jobNumber, financeAgencyID(p), include)
	if err != nil {
		slog.WarnContext(ctx, "travel: tab counts failed", "err", err)
		return nil
	}
	if c.Invoices != nil {
		n := min(*c.Invoices, financeTripInvoiceLimit)
		c.Invoices = &n
	}
	out := domain.TravelRequestCounts(c)
	return &out
}

// ── Visa options ──────────────────────────────────────────────────────────

func (s *travelService) VisaOptions(ctx context.Context, p domain.Perms, jobNumber string) (domain.TravelVisaOptions, error) {
	r, err := s.load(ctx, jobNumber)
	if err != nil {
		return domain.TravelVisaOptions{}, err
	}
	if scope := travelScope(p, r.TravellerEmail, r.TravelAgentID); scope != domain.TravelScopeFull && scope != domain.TravelScopeAgency {
		return domain.TravelVisaOptions{}, travelForbidden(travelNoAccess)
	}
	// The traveller's Active visas valid for the whole trip (legacy
	// getVisaOptions), then the two sentinels.
	visas, err := s.entity.SearchVisas(ctx, engagemententity.TravelVisaFilter{
		Emails: []string{r.TravellerEmail}, Status: domain.TravelRecordActive,
		CoversFrom: r.StartDate, CoversTo: r.EndDate, Limit: 1000,
	})
	if err != nil {
		return domain.TravelVisaOptions{}, err
	}
	out := domain.TravelVisaOptions{Items: []domain.TravelVisaOption{}}
	for _, v := range visas.Items {
		if v.ID == s.opts.VisaNotApplicableID || v.ID == s.opts.NoValidVisaID {
			continue
		}
		out.Items = append(out.Items, domain.TravelVisaOption{Value: strconv.Itoa(v.ID), Label: travelVisaOptionLabel(v)})
	}
	out.Items = append(out.Items,
		domain.TravelVisaOption{Value: domain.TravelNoValidVisa, Label: domain.TravelNoValidVisa},
		domain.TravelVisaOption{Value: domain.TravelVisaNotApplicable, Label: domain.TravelVisaNotApplicable})
	return out, nil
}

// travelVisaOptionLabel is the legacy "Country - VisaType - EntryType -
// (start to end)".
func travelVisaOptionLabel(v engagemententity.TravelVisaRecord) string {
	return fmt.Sprintf("%s - %s - %s - (%s to %s)", travelDeref(v.Country), travelDeref(v.VisaType),
		travelDeref(v.EntryType), travelDeref(v.StartDate), travelDeref(v.EndDate))
}

// ── Assign and Update Travel Details ──────────────────────────────────────

// Update ports the legacy assign-travel (a New request, Travel 1) and the
// update-travel-status / -agent / -visa calls (Travel 4 / 5 / 6), in one
// write. Release 1 enforces the UI rules on the server: the transition
// matrix, the agent and visa only while the (new) status is open, and an
// agent and a visa for In Progress, Completed and Extended. The legacy
// server accepted any status. Email failures are logged and never fail the
// update (legacy answered 500 after the write).
func (s *travelService) Update(ctx context.Context, p domain.Perms, jobNumber string, in domain.TravelRequestUpdate) (domain.TravelRequestDetail, error) {
	before, err := s.load(ctx, jobNumber)
	if err != nil {
		return domain.TravelRequestDetail{}, err
	}
	scope := travelScope(p, before.TravellerEmail, before.TravelAgentID)
	if scope != domain.TravelScopeFull && scope != domain.TravelScopeAgency {
		return domain.TravelRequestDetail{}, travelForbidden(travelNoAccess)
	}

	newStatus := before.Status
	if in.Status != nil {
		newStatus = strings.TrimSpace(*in.Status)
		if err := travelStatusesValid([]string{newStatus}); err != nil {
			return domain.TravelRequestDetail{}, err
		}
	}
	agentID, agentName := before.TravelAgentID, travelDeref(before.TravelAgentName)
	if in.Agency != nil {
		id, name, err := s.resolveAgency(ctx, *in.Agency)
		if err != nil {
			return domain.TravelRequestDetail{}, err
		}
		agentID, agentName = &id, name
	}
	visaID := before.VisaID
	if in.Visa != nil {
		id, err := s.resolveVisa(ctx, before, *in.Visa)
		if err != nil {
			return domain.TravelRequestDetail{}, err
		}
		visaID = &id
	}
	statusChanged := newStatus != before.Status
	agentChanged := !travelSameID(agentID, before.TravelAgentID)
	visaChanged := !travelSameID(visaID, before.VisaID)

	assign := before.Status == domain.TravelStatusNew && p.Travel.Has(travelPrivNewRequests)
	if assign {
		if in.Status == nil || (newStatus == domain.TravelStatusInProgress && (agentID == nil || visaID == nil)) {
			return domain.TravelRequestDetail{}, travelBadRequest("Required fields can not be empty. Please check the form again.")
		}
		if !slices.Contains(travelAssignStatuses, newStatus) {
			return domain.TravelRequestDetail{}, travelBadRequest("A new travel request can be set to In Progress, On Hold or Cancelled.")
		}
	} else {
		if err := travelCheckUpdate(p, before.Status, newStatus, statusChanged, agentChanged, visaChanged); err != nil {
			return domain.TravelRequestDetail{}, err
		}
		if !statusChanged && !agentChanged && !visaChanged {
			return s.detail(ctx, p, before, scope), nil
		}
		if travelNeedsAgentAndVisa(newStatus) {
			switch {
			case agentID == nil && visaID == nil:
				return domain.TravelRequestDetail{}, travelBadRequest("Please select a Travel Agent and a Visa")
			case agentID == nil:
				return domain.TravelRequestDetail{}, travelBadRequest("Please select a Travel Agent")
			case visaID == nil:
				return domain.TravelRequestDetail{}, travelBadRequest("Please select a Visa")
			}
		}
	}

	write := engagemententity.TravelRequestWrite{UpdatedBy: p.Email}
	if statusChanged || assign {
		write.Status = &newStatus
	}
	if agentChanged {
		write.TravelAgentID = agentID
	}
	if visaChanged {
		write.VisaID = visaID
	}
	after, err := s.entity.UpdateTravelRequest(ctx, jobNumber, write)
	if err != nil {
		return domain.TravelRequestDetail{}, travelNotFound(err, "Travel request "+jobNumber+" was not found.")
	}
	if agentName == "" {
		agentName = travelDeref(after.TravelAgentName)
	}

	mailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	if assign {
		s.assignEmails(mailCtx, after, newStatus, agentName)
	} else {
		s.updateEmails(mailCtx, before, after, statusChanged, agentChanged, visaChanged, agentName)
	}

	scope = travelScope(p, after.TravellerEmail, after.TravelAgentID)
	if scope == "" {
		// An agent who moved the request away keeps the answer it acted on.
		scope = domain.TravelScopeAgency
	}
	return s.detail(ctx, p, after, scope), nil
}

// travelCheckUpdate checks the Update Travel Details rules field by field.
func travelCheckUpdate(p domain.Perms, from, to string, statusChanged, agentChanged, visaChanged bool) error {
	if statusChanged {
		if !p.Travel.Has(travelPrivEditStatus) {
			return travelForbidden("You do not have permission to change the travel status.")
		}
		if !slices.Contains(travelNextStatuses[from], to) {
			return travelBadRequest(fmt.Sprintf("A travel request that is %s cannot be set to %s.", travelOr(from, "without a status"), to))
		}
	}
	if agentChanged && !p.Travel.Has(travelPrivEditAgent) {
		return travelForbidden("You do not have permission to change the travel agent.")
	}
	if visaChanged && !p.Travel.Has(travelPrivEditVisa) {
		return travelForbidden("You do not have permission to change the visa.")
	}
	if (agentChanged || visaChanged) && !travelOpen(to) {
		return travelBadRequest("The travel agent and visa can be changed while the request is In Progress or Extended.")
	}
	return nil
}

func travelOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func travelSameID(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// resolveAgency finds an Active travel agent by name.
func (s *travelService) resolveAgency(ctx context.Context, name string) (int, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, "", travelBadRequest("Please select a Travel Agent")
	}
	agents, err := s.entity.TravelAgencies(ctx, domain.TravelRecordActive)
	if err != nil {
		return 0, "", err
	}
	for _, a := range agents {
		if strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return a.ID, a.Name, nil
		}
	}
	return 0, "", travelBadRequest(fmt.Sprintf("%q is not an active travel agent.", name))
}

// resolveVisa accepts a sentinel (by label or id) or one of the traveller's
// visas.
func (s *travelService) resolveVisa(ctx context.Context, r engagemententity.TravelRequestRecord, v string) (int, error) {
	v = strings.TrimSpace(v)
	switch {
	case strings.EqualFold(v, domain.TravelVisaNotApplicable):
		return s.opts.VisaNotApplicableID, nil
	case strings.EqualFold(v, domain.TravelNoValidVisa):
		return s.opts.NoValidVisaID, nil
	}
	id, err := strconv.Atoi(v)
	if err != nil || id <= 0 {
		return 0, travelBadRequest("Please select a Visa")
	}
	if id == s.opts.VisaNotApplicableID || id == s.opts.NoValidVisaID {
		return id, nil
	}
	visa, err := s.entity.GetVisa(ctx, id)
	if err != nil {
		var up *apierror.UpstreamError
		if errors.As(err, &up) && up.StatusCode == 404 {
			return 0, travelBadRequest("The selected visa was not found.")
		}
		return 0, err
	}
	if !strings.EqualFold(strings.TrimSpace(travelDeref(visa.Email)), strings.TrimSpace(r.TravellerEmail)) {
		return 0, travelBadRequest("The selected visa is not one of the traveller's visas.")
	}
	// A new choice must be one VisaOptions offers: Active and covering the
	// whole trip. The visa already on the request is kept as it is.
	if !travelSameID(&id, r.VisaID) && !travelVisaCovers(visa, r.StartDate, r.EndDate) {
		return 0, travelBadRequest("Please select an Active visa that covers the whole trip.")
	}
	return id, nil
}

// travelVisaCovers reports whether v is Active and valid from start to end
// (dates compared by their YYYY-MM-DD part, as the entity's coversFrom and
// coversTo filters do).
func travelVisaCovers(v engagemententity.TravelVisaRecord, start, end string) bool {
	day := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) > 10 {
			s = s[:10]
		}
		return s
	}
	vs, ve := day(travelDeref(v.StartDate)), day(travelDeref(v.EndDate))
	return strings.EqualFold(travelDeref(v.Status), domain.TravelRecordActive) && vs != "" && ve != "" &&
		vs <= day(start) && ve >= day(end)
}

// ── Emails ────────────────────────────────────────────────────────────────

// peopleBounded is people.get with its own short deadline, so a slow HR cannot
// use up the time the email send needs; a timeout gives an empty person.
func (s *travelService) peopleBounded(ctx context.Context, addr string) travelPerson {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.people.get(ctx, addr)
}

func (s *travelService) travelEmailTrip(ctx context.Context, r engagemententity.TravelRequestRecord) email.TravelTrip {
	return travelEmailTripOf(r, s.peopleBounded(ctx, r.TravellerEmail))
}

// travelEmailTripOf is the trip of r with its traveller already looked up, so
// the notices of one update share a single HR lookup.
func travelEmailTripOf(r engagemententity.TravelRequestRecord, who travelPerson) email.TravelTrip {
	return email.TravelTrip{
		JobNumber: r.JobNumber, TravellerEmail: r.TravellerEmail, TravellerName: travelDeref(who.Name),
		Customer: r.CustomerName,
		Country:  r.Country, City: travelDeref(r.City), StartDate: r.StartDate, EndDate: r.EndDate,
	}
}

func travelEmailVisa(r engagemententity.TravelRequestRecord) email.TravelVisa {
	if r.Visa == nil {
		return email.TravelVisa{}
	}
	v := r.Visa
	return email.TravelVisa{VisaType: travelDeref(v.VisaType), Country: travelDeref(v.Country),
		EntryType: travelDeref(v.EntryType), StartDate: travelDeref(v.StartDate), EndDate: travelDeref(v.EndDate)}
}

// assignEmails are the legacy assign-travel emails: In Progress → "New
// Travel Request Assigned"; Cancelled → "Travel Request is Cancelled"
// addressed to All; On Hold → none.
func (s *travelService) assignEmails(ctx context.Context, r engagemententity.TravelRequestRecord, status, agentName string) {
	set := s.opts.Email
	switch status {
	case domain.TravelStatusInProgress:
		if r.TravelAgentID != nil && r.VisaID != nil {
			s.send(ctx, email.TravelAssigned(set, s.travelEmailTrip(ctx, r), travelEmailVisa(r), agentName))
		}
	case domain.TravelStatusCancelled:
		s.send(ctx, email.TravelNotice(set, s.travelEmailTrip(ctx, r), "Travel Cancelled", "This travel request was cancelled.", ""))
	}
}

// updateEmails are the legacy update-travel-status, -agent and -visa
// emails. A visa change together with an agent change is carried by the new
// agent's "New Travel Request Assigned" email.
func (s *travelService) updateEmails(ctx context.Context, before, after engagemententity.TravelRequestRecord,
	statusChanged, agentChanged, visaChanged bool, agentName string) {
	set := s.opts.Email
	// The traveller is looked up at most once for all the notices of this
	// update (the HR lookup is bounded, and a failure is not cached).
	person := sync.OnceValue(func() travelPerson { return s.peopleBounded(ctx, after.TravellerEmail) })
	trip := func() email.TravelTrip { return travelEmailTripOf(after, person()) }
	if statusChanged {
		switch after.Status {
		case domain.TravelStatusOnHold:
			s.send(ctx, email.TravelNotice(set, trip(), "Travel On Hold", "This travel request is on hold.", ""))
		case domain.TravelStatusCancelled:
			s.send(ctx, email.TravelNotice(set, trip(), "Travel Cancelled", "This travel request was cancelled.", ""))
		case domain.TravelStatusInProgress:
			s.send(ctx, email.TravelNotice(set, trip(), "Travel In Progress", "This travel request is now in progress.", ""))
		case domain.TravelStatusCompleted:
			s.completedEmail(ctx, after, person(), agentName)
		}
	}
	if agentChanged {
		if before.TravelAgentID != nil {
			s.send(ctx, email.TravelNotice(set, trip(), "Travel Reassigned",
				"This travel request is no longer assigned to "+travelDeref(before.TravelAgentName)+".",
				travelDeref(before.TravelAgentName)))
		}
		s.send(ctx, email.TravelAssigned(set, trip(), travelEmailVisa(after), agentName))
	} else if visaChanged && after.TravelAgentID != nil {
		s.send(ctx, email.TravelVisaChanged(set, trip(), travelEmailVisa(after), agentName))
	}
}

// completedEmail is the legacy sendTravelCompletedEmail: the Active per
// diems and the traveller's company (HR) for the finance recipients.
func (s *travelService) completedEmail(ctx context.Context, r engagemententity.TravelRequestRecord, who travelPerson, agentName string) {
	perDiems, err := s.entity.TravelActivePerDiems(ctx, r.JobNumber)
	if err != nil {
		slog.ErrorContext(ctx, "travel: Completed email not sent: per diems not read", "job", r.JobNumber, "err", err)
		return
	}
	rows := make([]email.TravelPerDiem, 0, len(perDiems))
	for _, pd := range perDiems {
		row := email.TravelPerDiem{StartDate: travelDeref(pd.StartDate), EndDate: travelDeref(pd.EndDate),
			Currency: travelDeref(pd.Currency), Rate: email.TravelFormatNumber(pd.PerDiemValue)}
		if pd.NoOfDays != nil {
			row.NoOfDays = strconv.Itoa(*pd.NoOfDays)
		}
		if pd.Total != nil {
			row.Total = email.TravelFormatNumber(*pd.Total)
		}
		rows = append(rows, row)
	}
	s.send(ctx, email.TravelCompleted(s.opts.Email, travelEmailTripOf(r, who), travelEmailVisa(r), agentName, travelDeref(who.Company), rows))
}

// send sends m; a failure is logged only. The sender writes the
// email_auditlog row (email.HTTPSender.WithAudit).
func (s *travelService) send(ctx context.Context, m email.Message) {
	if err := s.mail.Send(ctx, m); err != nil {
		slog.ErrorContext(ctx, "travel: email not sent", "type", m.NotificationType, "ref", m.ReferenceID, "err", err)
	}
}
