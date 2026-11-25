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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Global search limits: the query length and the per-group item count.
const (
	SearchMinQuery     = 2
	searchMaxQuery     = 200
	SearchDefaultLimit = 5
	SearchMaxLimit     = 20
	// searchTimeout bounds the whole search; a group still running then is
	// reported as failed.
	searchTimeout = 8 * time.Second
)

// The slices of the list services global search reads. The services apply
// their own privilege and record-scope rules, so a caller only finds what the
// matching list screen would show them.
type (
	// SearchTravelSource is the travel requests list.
	SearchTravelSource interface {
		List(ctx context.Context, p domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error)
	}
	// SearchInvoiceSource is the invoices list.
	SearchInvoiceSource interface {
		Invoices(ctx context.Context, p domain.Perms, q domain.InvoiceQuery) (domain.InvoicePage, error)
	}
	// SearchPaymentSource is the per diem payments list.
	SearchPaymentSource interface {
		Payments(ctx context.Context, p domain.Perms, q domain.PaymentQuery) (domain.PerDiemPaymentPage, error)
	}
	// SearchEngagementSource is the engagements list.
	SearchEngagementSource interface {
		List(ctx context.Context, p domain.Perms, f EngagementListFilter) (domain.EngagementPage, error)
	}
	// SearchAllocationSource is the engagement allocations list.
	SearchAllocationSource interface {
		List(ctx context.Context, p domain.Perms, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error)
	}
)

// SearchSources are the list services a SearchService reads. A nil source
// leaves its group out.
type SearchSources struct {
	Travel      SearchTravelSource
	Invoices    SearchInvoiceSource
	Payments    SearchPaymentSource
	Engagements SearchEngagementSource
	Allocations SearchAllocationSource
}

// SearchService is the global search: one query across the list screens the
// caller may open.
type SearchService interface {
	Search(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchResponse, error)
}

type searchService struct {
	src     SearchSources
	timeout time.Duration
}

// NewSearchService constructs a SearchService over src.
func NewSearchService(src SearchSources) SearchService {
	return &searchService{src: src, timeout: searchTimeout}
}

// searchGroupDef is one group: who may search it (the gate of the matching
// list route) and how.
type searchGroupDef struct {
	kind string
	gate domain.Gate
	run  func(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error)
}

func (s *searchService) groups() []searchGroupDef {
	travel := func(ids ...int) domain.Gate { return domain.Gate{domain.AppTravel: ids} }
	alloc := func(ids ...int) domain.Gate { return domain.Gate{domain.AppAllocations: ids} }
	var defs []searchGroupDef
	add := func(ok bool, d searchGroupDef) {
		if ok {
			defs = append(defs, d)
		}
	}
	add(s.src.Travel != nil, searchGroupDef{domain.SearchTravelRequests, travel(1, 2, 18), s.travelRequests})
	add(s.src.Invoices != nil, searchGroupDef{domain.SearchInvoices, travel(39, 32, 33, 34), s.invoices})
	add(s.src.Payments != nil, searchGroupDef{domain.SearchPerDiemPayments, travel(40, 41), s.payments})
	add(s.src.Engagements != nil, searchGroupDef{domain.SearchEngagements, alloc(2, 3, 9, 11, 12, 13, 19), s.engagements})
	add(s.src.Allocations != nil, searchGroupDef{domain.SearchAllocations, alloc(9, 19), s.allocations})
	return defs
}

type searchOutcome struct {
	idx   int
	group domain.SearchGroup
	err   error
}

// Search runs every group the caller may search in parallel under one
// timeout. A group that fails or times out is named in Failed; the others
// still return.
func (s *searchService) Search(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchResponse, error) {
	q = strings.TrimSpace(q)
	if n := utf8.RuneCountInString(q); n < SearchMinQuery {
		return domain.SearchResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("q must be at least %d characters.", SearchMinQuery)}
	} else if n > searchMaxQuery {
		return domain.SearchResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("q must be at most %d characters.", searchMaxQuery)}
	}
	limit = ClampSearchLimit(limit)

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var defs []searchGroupDef
	for _, d := range s.groups() {
		if p.Passes(d.gate) {
			defs = append(defs, d)
		}
	}
	out := make(chan searchOutcome, len(defs))
	for i, d := range defs {
		go func() {
			var o searchOutcome
			o.idx = i
			defer func() {
				if r := recover(); r != nil {
					o.err = fmt.Errorf("panic: %v", r)
				}
				out <- o
			}()
			o.group, o.err = d.run(ctx, p, q, limit)
		}()
	}

	done := make([]*searchOutcome, len(defs))
	for received := 0; received < len(defs); {
		select {
		case o := <-out:
			done[o.idx] = &o
			received++
		case <-ctx.Done():
			received = len(defs)
		}
	}

	resp := domain.SearchResponse{Query: q, Groups: []domain.SearchGroup{}, Failed: []string{}}
	for i, d := range defs {
		o := done[i]
		var forbidden *apierror.ForbiddenError
		switch {
		case o == nil:
			slog.WarnContext(ctx, "global search group timed out", "group", d.kind)
			resp.Failed = append(resp.Failed, d.kind)
		case errors.As(o.err, &forbidden):
			// The list would refuse this caller (record scope): leave it out.
		case o.err != nil:
			slog.WarnContext(ctx, "global search group failed", "group", d.kind, "err", o.err)
			resp.Failed = append(resp.Failed, d.kind)
		case o.group.Total > 0 && len(o.group.Items) > 0:
			resp.Groups = append(resp.Groups, o.group)
		}
	}
	return resp, nil
}

// ClampSearchLimit applies the default (below 1) and the maximum per group.
func ClampSearchLimit(limit int) int {
	switch {
	case limit < 1:
		return SearchDefaultLimit
	case limit > SearchMaxLimit:
		return SearchMaxLimit
	}
	return limit
}

// ── Job numbers ───────────────────────────────────────────────────────────

var (
	// searchJobFull is a whole job number typed without its separators or
	// prefix: "TRV2026110003", "trv 2026 11 0003", "2026110003".
	searchJobFull = regexp.MustCompile(`(?i)^(?:trv[- ]?)?(\d{4})[- ]?(\d{2})[- ]?(\d{4})$`)
	// searchJobPrefix is the start of a job number in any case: "trv-2026-11".
	searchJobPrefix = regexp.MustCompile(`(?i)^trv[- ]`)
)

// searchJobTerm normalises a query for the job number column (a substring
// match): the TRV- prefix is upper-case and present when the whole number is
// typed.
func searchJobTerm(q string) string {
	if m := searchJobFull.FindStringSubmatch(q); m != nil {
		return "TRV-" + m[1] + "-" + m[2] + "-" + m[3]
	}
	if searchJobPrefix.MatchString(q) {
		return "TRV-" + strings.TrimSpace(q[4:])
	}
	return q
}

// ── Groups ────────────────────────────────────────────────────────────────

func (s *searchService) travelRequests(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error) {
	page, err := s.src.Travel.List(ctx, p, domain.TravelListQuery{Search: searchJobTerm(q), Limit: limit})
	if err != nil {
		return domain.SearchGroup{}, err
	}
	g := domain.SearchGroup{Kind: domain.SearchTravelRequests, Total: page.Total, Items: []domain.SearchItem{}}
	for _, r := range page.Items {
		g.Items = append(g.Items, domain.SearchItem{
			ID: r.JobNumber, Title: r.JobNumber, Status: r.Status,
			Subtitle: searchJoin(personLabel(&r.Traveller), deref(r.Country), searchDateRange(r.StartDate, r.EndDate)),
		})
	}
	return g, nil
}

func (s *searchService) invoices(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error) {
	// No tab: the list picks All when the caller has it, else their first own queue.
	page, err := s.src.Invoices.Invoices(ctx, p, domain.InvoiceQuery{Search: searchJobTerm(q), Limit: limit})
	if err != nil {
		return domain.SearchGroup{}, err
	}
	g := domain.SearchGroup{Kind: domain.SearchInvoices, Total: page.Total, Items: []domain.SearchItem{}}
	for _, i := range page.Items {
		g.Items = append(g.Items, domain.SearchItem{
			ID: strconv.Itoa(i.ID), Title: i.InvoiceNumber, Status: i.Status, JobNumber: i.JobNumber,
			Subtitle: searchJoin(i.JobNumber, deref(i.Agency), searchAmount(i.Amount, i.Currency)),
		})
	}
	return g, nil
}

func (s *searchService) payments(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error) {
	page, err := s.src.Payments.Payments(ctx, p, domain.PaymentQuery{Search: searchJobTerm(q), Limit: limit})
	if err != nil {
		return domain.SearchGroup{}, err
	}
	g := domain.SearchGroup{Kind: domain.SearchPerDiemPayments, Total: page.Total, Items: []domain.SearchItem{}}
	for _, i := range page.Items {
		g.Items = append(g.Items, domain.SearchItem{
			ID: i.ID, Title: i.ID, Status: i.Status, JobNumber: i.JobNumber,
			Subtitle: searchJoin(i.JobNumber, personLabel(i.Traveller), searchAmount(i.Amount, i.Currency)),
		})
	}
	return g, nil
}

// engagements searches the three segments in parallel (the list needs one)
// and merges them, customer first. A segment the caller may not list is
// skipped.
func (s *searchService) engagements(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error) {
	segments := []string{domain.EngagementKindCustomer, domain.EngagementKindInternal, domain.EngagementKindMarketing}
	pages := make([]domain.EngagementPage, len(segments))
	errs := make([]error, len(segments))
	var wg sync.WaitGroup
	for i, seg := range segments {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("panic: %v", r)
				}
			}()
			pages[i], errs[i] = s.src.Engagements.List(ctx, p, EngagementListFilter{Segment: seg, Search: q, Limit: limit})
		}()
	}
	wg.Wait()

	g := domain.SearchGroup{Kind: domain.SearchEngagements, Items: []domain.SearchItem{}}
	allForbidden := true
	for i, seg := range segments {
		var forbidden *apierror.ForbiddenError
		if errors.As(errs[i], &forbidden) {
			continue
		}
		allForbidden = false
		if errs[i] != nil {
			return domain.SearchGroup{}, errs[i]
		}
		g.Total += pages[i].Total
		for _, e := range pages[i].Items {
			status := e.EngagementStatus
			if status == "" {
				status = e.RequestStatus
			}
			g.Items = append(g.Items, domain.SearchItem{
				ID: e.ID, Title: e.Title, Status: status, Segment: seg,
				Subtitle: searchJoin(deref(e.Account), e.EngagementCode, e.EngagementTypeName),
			})
		}
	}
	if allForbidden {
		return domain.SearchGroup{}, &apierror.ForbiddenError{Msg: "no engagement segment"}
	}
	if len(g.Items) > limit {
		g.Items = g.Items[:limit]
	}
	return g, nil
}

func (s *searchService) allocations(ctx context.Context, p domain.Perms, q string, limit int) (domain.SearchGroup, error) {
	page, err := s.src.Allocations.List(ctx, p, domain.AllocationListFilter{Search: q, Limit: limit})
	if err != nil {
		return domain.SearchGroup{}, err
	}
	g := domain.SearchGroup{Kind: domain.SearchAllocations, Total: page.Total, Items: []domain.SearchItem{}}
	for _, a := range page.Items {
		g.Items = append(g.Items, domain.SearchItem{
			ID: a.ID, Title: personLabel(&a.Consultant), Status: deref(a.ClearanceStatus), Segment: a.Engagement.Kind,
			Subtitle: searchJoin(a.Engagement.Title, deref(a.Engagement.Account), searchDateRange(a.StartDate, a.EndDate)),
		})
	}
	return g, nil
}

// ── Formatting ────────────────────────────────────────────────────────────

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func personLabel(p *domain.Person) string {
	if p == nil {
		return ""
	}
	if n := strings.TrimSpace(deref(p.Name)); n != "" {
		return n
	}
	return p.Email
}

// searchJoin joins the non-empty parts with ", ".
func searchJoin(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func searchAmount(amount float64, currency string) string {
	return strings.TrimSpace(strconv.FormatFloat(amount, 'f', 2, 64) + " " + currency)
}

// searchDateRange writes two ISO dates as "1–12 Nov 2026", "28 Oct – 3 Nov
// 2026" or "28 Dec 2026 – 3 Jan 2027". Unparseable dates are shown as is.
func searchDateRange(start, end string) string {
	s, err1 := time.Parse(time.DateOnly, start)
	e, err2 := time.Parse(time.DateOnly, end)
	if err1 != nil || err2 != nil {
		return searchJoinDates(start, end)
	}
	switch {
	case s.Equal(e):
		return s.Format("2 Jan 2006")
	case s.Year() == e.Year() && s.Month() == e.Month():
		return s.Format("2") + "–" + e.Format("2 Jan 2006")
	case s.Year() == e.Year():
		return s.Format("2 Jan") + " – " + e.Format("2 Jan 2006")
	}
	return s.Format("2 Jan 2006") + " – " + e.Format("2 Jan 2006")
}

func searchJoinDates(start, end string) string {
	switch {
	case start != "" && end != "":
		return start + " – " + end
	case start != "":
		return start
	}
	return end
}
