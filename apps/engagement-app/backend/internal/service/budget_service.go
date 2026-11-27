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
	"log/slog"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// BudgetEntityAPI is what the Travel Budgets service reads and writes in the
// Engagement Entity.
type BudgetEntityAPI interface {
	engagemententity.BudgetAPI
	AllocationEngagement(ctx context.Context, id string) (engagemententity.AllocationEngagement, error)
	AllocationTypes(ctx context.Context, f engagemententity.AllocationTypeFilter) ([]engagemententity.AllocationType, error)
}

// BudgetOptions configure the Travel Budgets service.
type BudgetOptions struct {
	// LineRecipients get the "needs a budget line" email.
	LineRecipients []string
	AppBaseURL     string
	// CacheTTL is how long the summary and reference data are kept; 0 keeps nothing.
	CacheTTL time.Duration
	Now      func() time.Time
}

// BudgetScreen is the Travel Budgets screens and settings (the handler).
type BudgetScreen interface {
	Preview(ctx context.Context, q BudgetPreviewQuery) (domain.BudgetPreview, error)
	Summary(ctx context.Context, p domain.Perms, year int) (domain.BudgetSummary, error)
	Lines(ctx context.Context, p domain.Perms, year int, includeRetired bool) ([]domain.BudgetLine, error)
	CreateLine(ctx context.Context, p domain.Perms, in domain.BudgetLineInput) (domain.BudgetLine, error)
	UpdateLine(ctx context.Context, p domain.Perms, id int, in domain.BudgetLineInput) (domain.BudgetLine, error)
	RetireLine(ctx context.Context, p domain.Perms, id int) error
	AllocationTypeRules(ctx context.Context) ([]domain.BudgetAllocationTypeRule, error)
	SaveAllocationTypeRule(ctx context.Context, p domain.Perms, allocationTypeID int, in domain.BudgetRuleInput) (domain.BudgetAllocationTypeRule, error)
	NeedsLine(ctx context.Context, p domain.Perms, year, limit, offset int) (domain.Page[domain.NeedsLineItem], error)
	SetTripLine(ctx context.Context, p domain.Perms, jobNumber string, lineID *int) (domain.TripBudgetView, error)
}

// BudgetHooks is what the allocation, engagement and travel services call
// when Travel Budgets are on. They hold nil when the feature is off and then
// do nothing budget related.
type BudgetHooks interface {
	// PlanTrip decides the budget line of a trip about to be created.
	PlanTrip(ctx context.Context, in TripPlanInput) (*TripPlan, error)
	// TripCreated runs after the entity created the trip: it drops the caches,
	// asks the budget owner for a line when there is none, and returns the
	// trip's budget for the response (nil when no budget year holds the trip).
	TripCreated(ctx context.Context, p domain.Perms, plan *TripPlan, jobNumber string) *domain.TripBudgetView
	// TripView is the budget of an existing trip for the caller, nil when none.
	TripView(ctx context.Context, p domain.Perms, jobNumber string) *domain.TripBudgetView
	// EngagementLine is the line set on an engagement, nil when none.
	EngagementLine(ctx context.Context, engagementID string) *domain.EngagementBudgetLine
	// CheckEngagementLine rejects a code that is not a line ("" is fine).
	CheckEngagementLine(ctx context.Context, code string) error
	// SetEngagementLine sets the engagement's line; "" clears it.
	SetEngagementLine(ctx context.Context, p domain.Perms, engagementID, code string) error
}

// BudgetService serves the Travel Budgets.
type BudgetService struct {
	entity BudgetEntityAPI
	people *travelPeople
	mail   email.Sender
	rates  *RateCache
	opts   BudgetOptions
	now    func() time.Time

	// ref holds lines, years and rules, summary the per-year figures. Both are
	// dropped by every budget write and by a trip being created.
	ref      *loadCache[any]
	summary  *loadCache[*domain.BudgetSummary]
	typicals *loadCache[*budgetTypical]
	// held serves the last result while its rates are being fetched, so a
	// preview does not rebuild it.
	held struct {
		summary *staleHold[*domain.BudgetSummary]
		typical *staleHold[*budgetTypical]
	}
	// bg tracks the background work (emails, rate warm-up).
	bg sync.WaitGroup
}

// NewBudgetService constructs the service. hr may be nil (no traveller names,
// no team suggestions). rates may be nil (nothing in another currency than
// USD converts).
func NewBudgetService(entity BudgetEntityAPI, hr hrentity.Client, mail email.Sender, rates *RateCache, o BudgetOptions) *BudgetService {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	if rates == nil {
		rates = newDashboardRates(nil, now)
	}
	s := &BudgetService{entity: entity, mail: mail, rates: rates, opts: o, now: now}
	if hr != nil {
		s.people = newTravelPeople(hr)
	}
	s.ref = newLoadCache[any](64, o.CacheTTL, 4, now)
	s.summary = newLoadCache[*domain.BudgetSummary](8, o.CacheTTL, 2, now)
	s.summary.ttlFor = func(v *domain.BudgetSummary) time.Duration {
		if v.RatesPending {
			return 0 // not kept, so the next call keeps warming the rate cache
		}
		return o.CacheTTL
	}
	s.held.summary, s.held.typical = newStaleHold[*domain.BudgetSummary](), newStaleHold[*budgetTypical]()
	s.typicals = newLoadCache[*budgetTypical](2, budgetTypicalTTL, 1, now)
	s.typicals.ttlFor = func(v *budgetTypical) time.Duration {
		if v.pending {
			return 0
		}
		return budgetTypicalTTL
	}
	return s
}

var (
	_ BudgetScreen = (*BudgetService)(nil)
	_ BudgetHooks  = (*BudgetService)(nil)
)

// wait blocks until the background work (emails, rate warm-up) is done.
func (s *BudgetService) wait() { s.bg.Wait() }

// ── Access ──────────────────────────────────────────────────────────────────

// budgetAdmin is the person in both super admin groups who holds Allocations
// 23 and Travel 29: Budget Lines and the allocation type rules.
func budgetAdmin(p domain.Perms) bool {
	return bothSuperAdmins(p) && p.Alloc.Has(23) && p.Travel.Has(29)
}

// budgetView may see the summary and the Needs a Budget Line list. Travel 29
// is included: the owner picks lines from the email link.
func budgetView(p domain.Perms) bool { return budgetAdmin(p) || p.Travel.Any(1, 4, 5, 6, 29, 35) }

// budgetSetLine may choose the line of an existing trip.
func budgetSetLine(p domain.Perms) bool { return p.Travel.Any(1, 4, 5, 6, 29) }

func budgetBad(msg string) error { return &apierror.ValidationError{Msg: msg} }

// ── Reference data (cached) ─────────────────────────────────────────────────

func budgetRef[T any](ctx context.Context, s *BudgetService, key string, load func() (T, error)) (T, error) {
	v, err := s.ref.get(ctx, key, func() (any, error) { return load() })
	if err != nil {
		var zero T
		return zero, err
	}
	return v.(T), nil
}

// dropCache forgets the lines, rules and summaries after a write.
func (s *BudgetService) dropCache() {
	s.ref.clear()
	s.summary.clear()
	s.held.summary.clear()
}

func (s *BudgetService) years(ctx context.Context) ([]engagemententity.BudgetYear, error) {
	return budgetRef(ctx, s, "years", func() ([]engagemententity.BudgetYear, error) { return s.entity.BudgetYears(ctx) })
}

// lines are every line of the year, retired ones too.
func (s *BudgetService) lines(ctx context.Context, year int) ([]engagemententity.BudgetLine, error) {
	return budgetRef(ctx, s, fmt.Sprint("lines:", year), func() ([]engagemententity.BudgetLine, error) {
		return s.entity.BudgetLines(ctx, year, true)
	})
}

func (s *BudgetService) typeRules(ctx context.Context) (map[int]budgetRule, error) {
	return budgetRef(ctx, s, "type-rules", func() (map[int]budgetRule, error) {
		rs, err := s.entity.BudgetAllocationTypeRules(ctx)
		if err != nil {
			return nil, err
		}
		m := make(map[int]budgetRule, len(rs))
		for _, r := range rs {
			m[r.AllocationTypeID] = budgetRule{Kind: r.RuleKind, LineCode: engagementVal(r.LineCode)}
		}
		return m, nil
	})
}

func (s *BudgetService) regionRules(ctx context.Context) ([]engagemententity.BudgetRegionRule, error) {
	return budgetRef(ctx, s, "region-rules", func() ([]engagemententity.BudgetRegionRule, error) { return s.entity.BudgetRegionRules(ctx) })
}

func (s *BudgetService) teamRules(ctx context.Context) ([]engagemententity.BudgetTeamRule, error) {
	return budgetRef(ctx, s, "team-rules", func() ([]engagemententity.BudgetTeamRule, error) { return s.entity.BudgetTeamRules(ctx) })
}

// budgetYearFor is the active year whose bounds hold date.
func budgetYearFor(years []engagemententity.BudgetYear, date string) (engagemententity.BudgetYear, bool) {
	for _, y := range years {
		if y.IsActive && y.StartDate <= date && date <= y.EndDate {
			return y, true
		}
	}
	return engagemententity.BudgetYear{}, false
}

// budgetDefaultYear is the active year holding today, else the latest year.
func budgetDefaultYear(years []engagemententity.BudgetYear, today string) (engagemententity.BudgetYear, bool) {
	if y, ok := budgetYearFor(years, today); ok {
		return y, true
	}
	var best engagemententity.BudgetYear
	found := false
	for _, y := range years {
		if !found || y.Year > best.Year {
			best, found = y, true
		}
	}
	return best, found
}

// pickYear resolves a requested year (0 is the default year).
func (s *BudgetService) pickYear(ctx context.Context, year int) (engagemententity.BudgetYear, error) {
	years, err := s.years(ctx)
	if err != nil {
		return engagemententity.BudgetYear{}, err
	}
	if year == 0 {
		if y, ok := budgetDefaultYear(years, s.now().UTC().Format(dashboardDateLayout)); ok {
			return y, nil
		}
		return engagemententity.BudgetYear{}, &apierror.NotFoundError{Msg: "No budget year is set up yet."}
	}
	for _, y := range years {
		if y.Year == year {
			return y, nil
		}
	}
	return engagemententity.BudgetYear{}, &apierror.NotFoundError{Msg: "That budget year was not found."}
}

func (s *BudgetService) domainLine(l engagemententity.BudgetLine) domain.BudgetLine {
	return domain.BudgetLine{ID: l.ID, Year: l.Year, Code: l.Code, FunctionName: l.FunctionName, SubFunction: l.SubFunction,
		Name: budgetLineName(l), HolderName: l.HolderName, HolderEmail: l.HolderEmail, AmountUSD: l.AmountUSD,
		Status: l.Status, SortOrder: l.SortOrder, UpdatedBy: l.UpdatedBy, UpdatedAt: l.UpdatedAt}
}

// ── Lines and rules (settings) ──────────────────────────────────────────────

// Lines lists a year's lines for the pickers and the Settings tab. Retired
// lines are for the budget admin only; amounts and holders only for people who
// may view or set budget lines.
func (s *BudgetService) Lines(ctx context.Context, p domain.Perms, year int, includeRetired bool) ([]domain.BudgetLine, error) {
	y, err := s.pickYear(ctx, year)
	if err != nil {
		if year == 0 {
			if _, none := err.(*apierror.NotFoundError); none {
				return []domain.BudgetLine{}, nil
			}
		}
		return nil, err
	}
	ls, err := s.lines(ctx, y.Year)
	if err != nil {
		return nil, err
	}
	includeRetired = includeRetired && budgetAdmin(p)
	full := budgetView(p) || budgetSetLine(p)
	out := make([]domain.BudgetLine, 0, len(ls))
	for _, l := range ls {
		if l.Status != domain.BudgetLineActive && !includeRetired {
			continue
		}
		d := s.domainLine(l)
		if !full {
			// The picker only needs names: no amounts or holders.
			d = domain.BudgetLine{ID: d.ID, Year: d.Year, Code: d.Code, FunctionName: d.FunctionName,
				SubFunction: d.SubFunction, Name: d.Name, Status: d.Status, SortOrder: d.SortOrder}
		}
		out = append(out, d)
	}
	return out, nil
}

var (
	budgetCodeRE    = regexp.MustCompile(`^[A-Z0-9_]{1,40}$`)
	budgetNonCodeRE = regexp.MustCompile(`[^A-Z0-9]+`)
)

// budgetNormCode turns a code the settings form derived from the function
// and name ("Sales APAC", "sales-apac") into the stored form (SALES_APAC).
func budgetNormCode(c string) string {
	c = strings.Trim(budgetNonCodeRE.ReplaceAllString(strings.ToUpper(strings.TrimSpace(c)), "_"), "_")
	if len(c) > 40 {
		c = strings.Trim(c[:40], "_")
	}
	return c
}

// budgetCheckLine validates a line body. Year and code only count on create.
func budgetCheckLine(in domain.BudgetLineInput, create bool) error {
	if create {
		if in.Year <= 0 {
			return budgetBad("Choose the budget year.")
		}
		if !budgetCodeRE.MatchString(in.Code) {
			return budgetBad("The line code can have 1 to 40 capital letters, digits or underscores.")
		}
	} else if in.Status != domain.BudgetLineActive && in.Status != domain.BudgetLineRetired {
		return budgetBad("Status must be Active or Retired.")
	}
	switch {
	case strings.TrimSpace(in.FunctionName) == "":
		return budgetBad("Add the function.")
	case len([]rune(in.FunctionName)) > 100:
		return budgetBad("The function can have at most 100 characters.")
	case strings.TrimSpace(in.SubFunction) == "":
		return budgetBad("Add the budget line.")
	case len([]rune(in.SubFunction)) > 150:
		return budgetBad("The budget line can have at most 150 characters.")
	case in.AmountUSD < 0 || in.AmountUSD > 1e11:
		return budgetBad("The budget must be 0 or more.")
	case in.HolderName != nil && len([]rune(*in.HolderName)) > 255:
		return budgetBad("The budget holder can have at most 255 characters.")
	}
	if e := engagementVal(in.HolderEmail); e != "" {
		if a, err := mail.ParseAddress(e); err != nil || a.Address != e || len(e) > 255 {
			return budgetBad("The holder email must be an email address.")
		}
	}
	return nil
}

func budgetTrimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	if v := strings.TrimSpace(*p); v != "" {
		return &v
	}
	return nil
}

func (s *BudgetService) CreateLine(ctx context.Context, p domain.Perms, in domain.BudgetLineInput) (domain.BudgetLine, error) {
	if !budgetAdmin(p) {
		return domain.BudgetLine{}, engagementForbidden()
	}
	in.FunctionName, in.SubFunction = strings.TrimSpace(in.FunctionName), strings.TrimSpace(in.SubFunction)
	in.HolderName, in.HolderEmail = budgetTrimPtr(in.HolderName), budgetTrimPtr(in.HolderEmail)
	in.Code = budgetNormCode(in.Code)
	if err := budgetCheckLine(in, true); err != nil {
		return domain.BudgetLine{}, err
	}
	l, err := s.entity.CreateBudgetLine(ctx, engagemententity.BudgetLineCreate{Year: in.Year, Code: in.Code,
		FunctionName: in.FunctionName, SubFunction: in.SubFunction, HolderName: in.HolderName, HolderEmail: in.HolderEmail,
		AmountUSD: in.AmountUSD, SortOrder: in.SortOrder, CreatedBy: p.Email})
	if err != nil {
		return domain.BudgetLine{}, err
	}
	s.dropCache()
	return s.domainLine(l), nil
}

func (s *BudgetService) UpdateLine(ctx context.Context, p domain.Perms, id int, in domain.BudgetLineInput) (domain.BudgetLine, error) {
	if !budgetAdmin(p) {
		return domain.BudgetLine{}, engagementForbidden()
	}
	in.FunctionName, in.SubFunction = strings.TrimSpace(in.FunctionName), strings.TrimSpace(in.SubFunction)
	in.HolderName, in.HolderEmail = budgetTrimPtr(in.HolderName), budgetTrimPtr(in.HolderEmail)
	if err := budgetCheckLine(in, false); err != nil {
		return domain.BudgetLine{}, err
	}
	l, err := s.entity.UpdateBudgetLine(ctx, id, engagemententity.BudgetLineUpdate{FunctionName: in.FunctionName,
		SubFunction: in.SubFunction, HolderName: in.HolderName, HolderEmail: in.HolderEmail, AmountUSD: in.AmountUSD,
		SortOrder: in.SortOrder, Status: in.Status, UpdatedBy: p.Email})
	if err != nil {
		return domain.BudgetLine{}, err
	}
	s.dropCache()
	return s.domainLine(l), nil
}

// RetireLine retires a line: trips already on it keep it.
func (s *BudgetService) RetireLine(ctx context.Context, p domain.Perms, id int) error {
	if !budgetAdmin(p) {
		return engagementForbidden()
	}
	if err := s.entity.RetireBudgetLine(ctx, id, p.Email); err != nil {
		return err
	}
	s.dropCache()
	return nil
}

// knownCode reports whether code is the code of a line of any year, and
// returns that line's name (newest year first).
func (s *BudgetService) knownCode(ctx context.Context, code string) (name string, ok bool, err error) {
	years, err := s.years(ctx)
	if err != nil {
		return "", false, err
	}
	sort.Slice(years, func(i, j int) bool { return years[i].Year > years[j].Year })
	for _, y := range years {
		ls, err := s.lines(ctx, y.Year)
		if err != nil {
			return "", false, err
		}
		for _, l := range ls {
			if l.Code == code && l.Status == domain.BudgetLineActive {
				return budgetLineName(l), true, nil
			}
		}
	}
	return "", false, nil
}

// AllocationTypeRules lists every Active allocation type with its rule (NONE
// when it has none), for the Allocation Types settings.
func (s *BudgetService) AllocationTypeRules(ctx context.Context) ([]domain.BudgetAllocationTypeRule, error) {
	types, err := s.entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{Status: "Active"})
	if err != nil {
		return nil, err
	}
	rules, err := s.typeRules(ctx)
	if err != nil {
		return nil, err
	}
	names := s.codeNames(ctx)
	out := make([]domain.BudgetAllocationTypeRule, 0, len(types))
	for _, t := range types {
		out = append(out, budgetRuleView(t.ID, t.AllocationTypeName, t.EngagementTypeID, t.EngagementTypeName, rules[t.ID], names))
	}
	return out, nil
}

func budgetRuleView(id int, name string, engTypeID int, engType string, r budgetRule, names map[string]string) domain.BudgetAllocationTypeRule {
	v := domain.BudgetAllocationTypeRule{AllocationTypeID: id, AllocationTypeName: name, EngagementTypeID: engTypeID,
		EngagementTypeName: engType, RuleKind: r.Kind}
	if v.RuleKind == "" {
		v.RuleKind = domain.BudgetRuleNone
	}
	if r.LineCode != "" {
		code := r.LineCode
		v.LineCode = &code
		n := names[code]
		if n == "" {
			n = code
		}
		v.LineName = &n
	}
	return v
}

// codeNames maps line codes to names, from the default year's lines. A
// failure leaves the names empty (the code is shown instead).
func (s *BudgetService) codeNames(ctx context.Context) map[string]string {
	out := map[string]string{}
	y, err := s.pickYear(ctx, 0)
	if err != nil {
		return out
	}
	ls, err := s.lines(ctx, y.Year)
	if err != nil {
		slog.WarnContext(ctx, "budget line names not read", "err", err)
		return out
	}
	for _, l := range ls {
		out[l.Code] = budgetLineName(l)
	}
	return out
}

func (s *BudgetService) SaveAllocationTypeRule(ctx context.Context, p domain.Perms, allocationTypeID int, in domain.BudgetRuleInput) (domain.BudgetAllocationTypeRule, error) {
	var none domain.BudgetAllocationTypeRule
	if !budgetAdmin(p) {
		return none, engagementForbidden()
	}
	code := ""
	if in.LineCode != nil {
		code = strings.TrimSpace(*in.LineCode)
	}
	switch in.RuleKind {
	case domain.BudgetRuleLine:
		if code == "" {
			return none, budgetBad("Choose the budget line.")
		}
	case domain.BudgetRuleRegion, domain.BudgetRuleTeam, domain.BudgetRuleEngagement, domain.BudgetRuleNone:
		if code != "" {
			return none, budgetBad("Only the rule for one budget line has a line.")
		}
	default:
		return none, budgetBad("The rule must be LINE, REGION, TEAM, ENGAGEMENT or NONE.")
	}
	var save *string
	if code != "" {
		_, ok, err := s.knownCode(ctx, code)
		if err != nil {
			return none, err
		}
		if !ok {
			return none, budgetBad("Choose a valid budget line.")
		}
		save = &code
	}
	types, err := s.entity.AllocationTypes(ctx, engagemententity.AllocationTypeFilter{})
	if err != nil {
		return none, err
	}
	var typ *engagemententity.AllocationType
	for i := range types {
		if types[i].ID == allocationTypeID {
			typ = &types[i]
		}
	}
	if typ == nil {
		return none, &apierror.NotFoundError{Msg: "That allocation type was not found."}
	}
	r, err := s.entity.SaveBudgetAllocationTypeRule(ctx, allocationTypeID, engagemententity.BudgetRuleSave{RuleKind: in.RuleKind, LineCode: save, UpdatedBy: p.Email})
	if err != nil {
		return none, err
	}
	s.dropCache()
	return budgetRuleView(typ.ID, typ.AllocationTypeName, typ.EngagementTypeID, typ.EngagementTypeName,
		budgetRule{Kind: r.RuleKind, LineCode: engagementVal(r.LineCode)}, s.codeNames(ctx)), nil
}

// ── Suggestion ──────────────────────────────────────────────────────────────

// budgetSuggestion is the line chosen by the rules (nil line: none).
type budgetSuggestion struct {
	line   *engagemententity.BudgetLine
	source string
}

// suggest reads what the rules need (the engagement's line, the allocation
// type's rule, the funding region or the traveller's HR team) and applies
// budgetSuggest. An error means the inputs could not be read.
func (s *BudgetService) suggest(ctx context.Context, eng engagemententity.AllocationEngagement, allocTypeID int,
	travellerEmail string, lines []engagemententity.BudgetLine) (budgetSuggestion, error) {
	in := budgetSuggestIn{ActiveCodes: map[string]bool{}}
	byCode := map[string]*engagemententity.BudgetLine{}
	for i := range lines {
		if lines[i].Status == domain.BudgetLineActive {
			in.ActiveCodes[lines[i].Code] = true
			byCode[lines[i].Code] = &lines[i]
		}
	}
	if eng.EngagementTypeID == domain.EngagementTypeInternal || eng.EngagementTypeID == domain.EngagementTypeMarketing {
		code, err := s.entity.BudgetEngagementLine(ctx, eng.ID)
		if err != nil {
			return budgetSuggestion{}, err
		}
		in.EngagementLine = engagementVal(code)
	}
	rules, err := s.typeRules(ctx)
	if err != nil {
		return budgetSuggestion{}, err
	}
	in.Rule = rules[allocTypeID]
	if in.EngagementLine == "" {
		switch in.Rule.Kind {
		case domain.BudgetRuleRegion:
			for _, f := range eng.FundingSources {
				pct := 0.0
				if f.Percentage != nil {
					pct = *f.Percentage
				}
				in.Funding = append(in.Funding, budgetFunding{Region: allocStr(f.Region), SubRegion: allocStr(f.SubRegion), Percentage: pct})
			}
			if in.RegionRules, err = s.regionRules(ctx); err != nil {
				return budgetSuggestion{}, err
			}
		case domain.BudgetRuleTeam:
			if in.TeamRules, err = s.teamRules(ctx); err != nil {
				return budgetSuggestion{}, err
			}
			in.HR = s.travellerTeam(ctx, travellerEmail)
		}
	}
	code, source := budgetSuggest(in)
	return budgetSuggestion{line: byCode[code], source: source}, nil
}

// travellerTeam reads the traveller's team from HR; nil when HR fails or does
// not know the person (the rule then finds no line).
func (s *BudgetService) travellerTeam(ctx context.Context, emailAddr string) *budgetHR {
	if s.people == nil {
		return nil
	}
	e, err := s.people.hr.Employee(ctx, strings.ToLower(strings.TrimSpace(emailAddr)))
	if err != nil {
		slog.WarnContext(ctx, "budget suggestion: HR lookup failed", "err", err)
		return nil
	}
	if e == nil {
		return nil
	}
	return &budgetHR{BusinessUnit: engagementVal(e.BusinessUnit), Department: engagementVal(e.Department), Team: engagementVal(e.Team)}
}

// ── Figures ─────────────────────────────────────────────────────────────────

// budgetRateDate is the date whose NetSuite rate converts a trip's costs: the
// day the trip was allocated, never later than today. To use the month-end
// rule of Travel Costs instead, return
// dashboardRateDate(rateDate[:7], today) here and nowhere else.
func budgetRateDate(rateDate string, today time.Time) string {
	d := today.UTC().Format(dashboardDateLayout)
	if len(rateDate) != len(dashboardDateLayout) || rateDate > d {
		return d
	}
	return rateDate
}

func budgetPercent(spent, committed, budget float64) float64 {
	if budget <= 0 {
		return 0
	}
	return float64(int((spent+committed)/budget*1000+0.5)) / 10
}

// budgetAcc adds up one line.
type budgetAcc struct {
	spent, committed float64
	trips, open      int
}

// Summary is the year's lines with spent, committed and left.
func (s *BudgetService) Summary(ctx context.Context, p domain.Perms, year int) (domain.BudgetSummary, error) {
	if !budgetView(p) {
		return domain.BudgetSummary{}, engagementForbidden()
	}
	v, err := s.yearSummary(ctx, year)
	if err != nil {
		return domain.BudgetSummary{}, err
	}
	return *v, nil
}

// yearSummary is the cached summary of a year (0 is the default year). The
// value is shared and read-only.
func (s *BudgetService) yearSummary(ctx context.Context, year int) (*domain.BudgetSummary, error) {
	y, err := s.pickYear(ctx, year)
	if err != nil {
		return nil, err
	}
	if v, ok := s.held.summary.get(fmt.Sprint(y.Year)); ok {
		return v, nil
	}
	return s.summary.get(ctx, fmt.Sprint(y.Year), func() (*domain.BudgetSummary, error) { return s.buildSummary(ctx, y) })
}

func (s *BudgetService) buildSummary(ctx context.Context, y engagemententity.BudgetYear) (*domain.BudgetSummary, error) {
	var (
		wg                             sync.WaitGroup
		lines                          []engagemententity.BudgetLine
		ledger                         engagemententity.BudgetLedger
		typ                            *budgetTypical
		needs                          engagemententity.BudgetNeedsLinePage
		lineErr, ledgErr, typErr, nErr error
	)
	goSafe(ctx, &wg, &lineErr, func() { lines, lineErr = s.lines(ctx, y.Year) })
	goSafe(ctx, &wg, &ledgErr, func() { ledger, ledgErr = s.entity.BudgetLedger(ctx, y.StartDate, y.EndDate, true) })
	goSafe(ctx, &wg, &typErr, func() { typ, typErr = s.typical(ctx) })
	goSafe(ctx, &wg, &nErr, func() { needs, nErr = s.entity.BudgetNeedsLine(ctx, y.Year, 1, 0) })
	var years []engagemententity.BudgetYear
	var yearErr error
	goSafe(ctx, &wg, &yearErr, func() { years, yearErr = s.years(ctx) })
	wg.Wait()
	for _, err := range []error{yearErr, lineErr, ledgErr, typErr, nErr} {
		if err != nil {
			return nil, err
		}
	}
	if ledger.Truncated {
		slog.WarnContext(ctx, "budget summary: the ledger was truncated", "year", y.Year)
	}

	today := s.now().UTC()
	tripByJob := make(map[string]engagemententity.BudgetLedgerTrip, len(ledger.Trips))
	rateDate := make(map[string]string, len(ledger.Trips))
	for _, t := range ledger.Trips {
		tripByJob[budgetJobKey(t.JobNumber)] = t
		rd := t.StartDate
		if t.RateDate != nil && *t.RateDate != "" {
			rd = *t.RateDate
		}
		rateDate[budgetJobKey(t.JobNumber)] = budgetRateDate(rd, today)
	}
	var pairs []ratePair
	for _, c := range ledger.Costs {
		if job := budgetJobKey(c.JobNumber); tripByJob[job].JobNumber != "" {
			pairs = append(pairs, ratePair{budgetCurrency(c.Currency), rateDate[job]})
		}
	}
	for _, u := range ledger.Unassigned {
		pairs = append(pairs, ratePair{budgetCurrency(u.Currency), dashboardRateDate(u.Month, today)})
	}
	rs := s.rates.resolve(ctx, pairs)

	out := &domain.BudgetSummary{Year: y.Year, StartDate: y.StartDate, EndDate: y.EndDate, AsOf: today.Format(time.RFC3339),
		ExpenseAvailable: ledger.ExpenseAvailable, Lines: []domain.BudgetSummaryLine{}, NeedsLineCount: needs.Total,
		Unconverted: domain.BudgetUnconverted{Currencies: []string{}}, Years: []int{}}
	for _, yr := range years {
		out.Years = append(out.Years, yr.Year)
	}
	sort.Ints(out.Years)
	unconverted := map[string]bool{}
	skip := func(currency string) {
		out.Unconverted.Count++
		unconverted[budgetCurrency(currency)] = true
	}
	tripCost := map[string]float64{}
	hasCost := map[string]bool{}
	for _, c := range ledger.Costs {
		job := budgetJobKey(c.JobNumber)
		if tripByJob[job].JobNumber == "" {
			continue
		}
		hasCost[job] = true
		usd, ok := rs.usdOn(c.Currency, c.Amount, rateDate[job])
		if !ok {
			skip(c.Currency)
			continue
		}
		tripCost[job] += usd
	}
	noTrip := 0.0
	for _, u := range ledger.Unassigned {
		usd, ok := rs.usdOn(u.Currency, u.Amount, dashboardRateDate(u.Month, today))
		if !ok {
			skip(u.Currency)
			continue
		}
		noTrip += usd
	}

	lineByID := make(map[int]engagemententity.BudgetLine, len(lines))
	acc := make(map[int]*budgetAcc, len(lines))
	for _, l := range lines {
		lineByID[l.ID] = l
		acc[l.ID] = &budgetAcc{}
	}
	unassigned, withoutLine := 0.0, 0
	for _, t := range ledger.Trips {
		job := budgetJobKey(t.JobNumber)
		cost := tripCost[job]
		open := t.Status != domain.TravelStatusCancelled
		var a *budgetAcc
		if t.BudgetLineID != nil {
			a = acc[*t.BudgetLineID]
		}
		if a == nil {
			unassigned += cost
			if open {
				withoutLine++
			}
			continue
		}
		a.spent += cost
		if !open {
			continue
		}
		a.trips++
		if t.Status != domain.TravelStatusCompleted && !hasCost[job] {
			a.open++
			usd, _, _ := typ.forCountry(budgetCountry(t.Country))
			a.committed += usd
		}
	}

	var tot budgetAcc
	totalBudget := 0.0
	for _, l := range lines {
		a := acc[l.ID]
		if l.Status != domain.BudgetLineActive && a.trips == 0 && a.spent == 0 {
			continue
		}
		spent, committed := dashboardRound(a.spent), dashboardRound(a.committed)
		out.Lines = append(out.Lines, domain.BudgetSummaryLine{LineID: l.ID, Code: l.Code, FunctionName: l.FunctionName,
			Name: l.SubFunction, HolderName: l.HolderName, Status: l.Status, BudgetUSD: l.AmountUSD, SpentUSD: spent,
			CommittedUSD: committed, LeftUSD: dashboardRound(l.AmountUSD - spent - committed),
			UsedPercent: budgetPercent(spent, committed, l.AmountUSD), Trips: a.trips, OpenTripsWithoutCost: a.open})
		totalBudget += l.AmountUSD
		tot.spent += spent
		tot.committed += committed
	}
	noTrip = dashboardRound(noTrip)
	out.Unassigned = domain.BudgetUnassigned{SpentUSD: dashboardRound(unassigned + noTrip), TripsWithoutLine: withoutLine, NoTripSpendUSD: noTrip}
	spent, committed := dashboardRound(tot.spent+out.Unassigned.SpentUSD), dashboardRound(tot.committed)
	out.Totals = domain.BudgetTotals{BudgetUSD: dashboardRound(totalBudget), SpentUSD: spent, CommittedUSD: committed,
		LeftUSD: dashboardRound(totalBudget - spent - committed), UsedPercent: budgetPercent(spent, committed, totalBudget)}
	for c := range unconverted {
		out.Unconverted.Currencies = append(out.Unconverted.Currencies, c)
	}
	sort.Strings(out.Unconverted.Currencies)
	// Pending only while a missing rate can still be fetched. A blank currency
	// or a rate NetSuite does not have stays in Unconverted and the summary is
	// cached as usual.
	missing := missingPairs(pairs, rs)
	out.RatesPending = out.Unconverted.Count > 0 && s.rates.fetchable(missing)
	if out.RatesPending {
		key := fmt.Sprint(y.Year)
		s.held.summary.set(key, out)
		s.warmRates(ctx, missing, func() { s.held.summary.del(key) })
	}
	return out, nil
}

// budgetJobKey is a job number as the ledger maps it: trimmed and upper-cased.
func budgetJobKey(j string) string { return strings.ToUpper(strings.TrimSpace(j)) }

// warmRates resolves the rates a response had to leave out, after it was sent,
// so the next call finds them in the cache.
// done runs after, so the caller stops serving its stale copy.
func (s *BudgetService) warmRates(ctx context.Context, missing []ratePair, done func()) {
	if len(missing) == 0 {
		done()
		return
	}
	goSafe(ctx, &s.bg, nil, func() {
		defer done()
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		s.rates.resolve(wctx, missing)
	})
}

// ── Preview ─────────────────────────────────────────────────────────────────

// BudgetPreviewQuery is GET /budgets/preview.
type BudgetPreviewQuery struct {
	EngagementID   string
	TravellerEmail string
	StartDate      string
	EndDate        string
	// Country and AllocationTypeID default to the engagement's.
	Country          string
	AllocationTypeID int
}

// Preview suggests the line for a trip that is about to be allocated and shows
// its figures and the typical cost of the trip.
func (s *BudgetService) Preview(ctx context.Context, q BudgetPreviewQuery) (domain.BudgetPreview, error) {
	none := domain.BudgetPreview{}
	if strings.TrimSpace(q.EngagementID) == "" || strings.TrimSpace(q.TravellerEmail) == "" {
		return none, budgetBad("engagementId and travellerEmail are required.")
	}
	start, okS := allocParseDate(q.StartDate)
	end, okE := allocParseDate(q.EndDate)
	if !okS || !okE || end.Before(start) {
		return none, budgetBad("startDate and endDate must be dates (YYYY-MM-DD), startDate on or before endDate.")
	}
	years, err := s.years(ctx)
	if err != nil {
		return none, err
	}
	y, ok := budgetYearFor(years, q.StartDate)
	if !ok {
		return domain.BudgetPreview{InScope: false}, nil
	}
	eng, err := s.entity.AllocationEngagement(ctx, q.EngagementID)
	if allocUpstreamNotFound(err) {
		return none, &apierror.NotFoundError{Msg: "Engagement " + q.EngagementID + " was not found."}
	}
	if err != nil {
		return none, err
	}
	typeID, country := q.AllocationTypeID, strings.TrimSpace(q.Country)
	if typeID == 0 && eng.AllocationTypeID != nil {
		typeID = *eng.AllocationTypeID
	}
	if country == "" {
		country = allocStr(eng.Country)
	}
	lines, err := s.lines(ctx, y.Year)
	if err != nil {
		return none, err
	}
	sg, err := s.suggest(ctx, eng, typeID, q.TravellerEmail, lines)
	if err != nil {
		return none, err
	}
	sum, err := s.yearSummary(ctx, y.Year)
	if err != nil {
		return none, err
	}
	typ, err := s.typical(ctx)
	if err != nil {
		return none, err
	}
	figures := make(map[int]domain.BudgetSummaryLine, len(sum.Lines))
	for _, l := range sum.Lines {
		figures[l.LineID] = l
	}
	out := domain.BudgetPreview{InScope: true, Year: y.Year, Country: country, Source: sg.source,
		Alternatives: []domain.BudgetAlternative{}, RatesPending: sum.RatesPending || typ.pending}
	for _, l := range lines {
		if l.Status != domain.BudgetLineActive {
			continue
		}
		left := l.AmountUSD
		if f, ok := figures[l.ID]; ok {
			left = f.LeftUSD
		}
		out.Alternatives = append(out.Alternatives, domain.BudgetAlternative{LineID: l.ID, Code: l.Code, Name: budgetLineName(l), LeftUSD: left})
	}
	if sg.line != nil {
		l := *sg.line
		out.Suggested = &domain.BudgetSuggested{LineID: l.ID, Code: l.Code, Name: budgetLineName(l), Source: sg.source}
		f := figures[l.ID]
		if _, ok := figures[l.ID]; !ok {
			f = domain.BudgetSummaryLine{BudgetUSD: l.AmountUSD, LeftUSD: l.AmountUSD}
		}
		out.Line = &domain.BudgetFigures{LineID: l.ID, Name: budgetLineName(l), HolderName: l.HolderName, BudgetUSD: l.AmountUSD,
			SpentUSD: f.SpentUSD, CommittedUSD: f.CommittedUSD, LeftUSD: f.LeftUSD, UsedPercent: f.UsedPercent}
	}
	usd, basis, n := typ.forCountry(budgetCountry(&country))
	out.TypicalCost = domain.BudgetTypicalCost{Basis: basis, SampleSize: n}
	if basis != budgetBasisNone {
		out.TypicalCost.USD = &usd
		out.Warning = out.Line != nil && usd > out.Line.LeftUSD
	}
	return out, nil
}

// ── Trips ───────────────────────────────────────────────────────────────────

// viewOf builds the budget of a trip from its entity row. It is nil when no
// budget year holds the trip.
func (s *BudgetService) viewOf(ctx context.Context, p domain.Perms, tb engagemententity.BudgetTrip) *domain.TripBudgetView {
	year := tb.BudgetYear
	if year == nil && tb.StartDate != nil {
		years, err := s.years(ctx)
		if err != nil {
			slog.WarnContext(ctx, "trip budget: years not read", "err", err)
			return nil
		}
		if y, ok := budgetYearFor(years, *tb.StartDate); ok {
			year = &y.Year
		}
	}
	if year == nil {
		return nil
	}
	names := map[int]string{}
	lines, err := s.lines(ctx, *year)
	if err != nil {
		slog.WarnContext(ctx, "trip budget: lines not read", "year", *year, "err", err)
	}
	for _, l := range lines {
		names[l.ID] = budgetLineName(l)
	}
	ref := func(id *int) *domain.BudgetLineRef {
		if id == nil {
			return nil
		}
		return &domain.BudgetLineRef{LineID: *id, Name: names[*id]}
	}
	source := allocStr(tb.SuggestionSource)
	if source == "" {
		source = domain.BudgetSourceNone
	}
	return &domain.TripBudgetView{JobNumber: tb.JobNumber, Year: year, Line: ref(tb.BudgetLineID), Suggested: ref(tb.SuggestedLineID),
		Source: source, ChosenBy: tb.ChosenBy, NeedsLine: tb.BudgetLineID == nil && allocStr(tb.Status) != domain.TravelStatusCancelled,
		CanEdit: budgetSetLine(p)}
}

// TripView is the budget of a trip for people who may see or set budget lines.
func (s *BudgetService) TripView(ctx context.Context, p domain.Perms, jobNumber string) *domain.TripBudgetView {
	if !budgetView(p) && !budgetSetLine(p) {
		return nil
	}
	tb, err := s.entity.BudgetTrip(ctx, jobNumber)
	if err != nil {
		if !allocUpstreamNotFound(err) {
			slog.WarnContext(ctx, "trip budget not read", "err", err)
		}
		return nil
	}
	return s.viewOf(ctx, p, tb)
}

// SetTripLine sets (or clears) the line of a trip.
func (s *BudgetService) SetTripLine(ctx context.Context, p domain.Perms, jobNumber string, lineID *int) (domain.TripBudgetView, error) {
	if !budgetSetLine(p) {
		return domain.TripBudgetView{}, engagementForbidden()
	}
	tb, err := s.entity.SetBudgetTrip(ctx, jobNumber, lineID, p.Email)
	if err != nil {
		return domain.TripBudgetView{}, err
	}
	s.dropCache()
	if v := s.viewOf(ctx, p, tb); v != nil {
		return *v, nil
	}
	return domain.TripBudgetView{JobNumber: tb.JobNumber, Source: domain.BudgetSourceNone, CanEdit: true}, nil
}

// NeedsLine lists the trips of a year that have no line.
func (s *BudgetService) NeedsLine(ctx context.Context, p domain.Perms, year, limit, offset int) (domain.Page[domain.NeedsLineItem], error) {
	none := domain.Page[domain.NeedsLineItem]{Items: []domain.NeedsLineItem{}}
	if !budgetView(p) {
		return none, engagementForbidden()
	}
	y, err := s.pickYear(ctx, year)
	if err != nil {
		return none, err
	}
	page, err := s.entity.BudgetNeedsLine(ctx, y.Year, limit, offset)
	if err != nil {
		return none, err
	}
	lines, err := s.lines(ctx, y.Year)
	if err != nil {
		return none, err
	}
	names := map[int]string{}
	for _, l := range lines {
		names[l.ID] = budgetLineName(l)
	}
	var emails []string
	for _, t := range page.Items {
		emails = append(emails, allocStr(t.TravellerEmail))
	}
	var people map[string]travelPerson
	if s.people != nil {
		people = s.people.many(ctx, emails)
	}
	out := domain.Page[domain.NeedsLineItem]{Items: make([]domain.NeedsLineItem, 0, len(page.Items)), Total: page.Total}
	for _, t := range page.Items {
		it := domain.NeedsLineItem{JobNumber: t.JobNumber, TravellerEmail: t.TravellerEmail, Country: t.Country, StartDate: t.StartDate,
			EndDate: t.EndDate, Status: t.Status, EngagementID: t.EngagementID, EngagementType: t.EngagementType,
			CustomerName: t.CustomerName, BudgetYear: t.BudgetYear, BudgetLineID: t.BudgetLineID, SuggestedLineID: t.SuggestedLineID,
			SuggestionSource: t.SuggestionSource, ChosenBy: t.ChosenBy, RateDate: t.RateDate, HasRow: t.HasRow}
		if t.SuggestedLineID != nil {
			it.Suggested = &domain.BudgetLineRef{LineID: *t.SuggestedLineID, Name: names[*t.SuggestedLineID]}
		}
		it.TravellerName = people[strings.ToLower(strings.TrimSpace(allocStr(t.TravellerEmail)))].Name
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// ── Allocation flow ─────────────────────────────────────────────────────────

// TripPlanInput is the trip about to be created.
type TripPlanInput struct {
	Engagement      engagemententity.AllocationEngagement
	ConsultantEmail string
	StartDate       string
	EndDate         string
	// RequestedLineID is the line the allocator chose, nil when none.
	RequestedLineID *int
}

// TripPlan is the decided budget of a trip about to be created.
type TripPlan struct {
	// InScope is false when no budget year holds the trip's start date.
	InScope bool
	Year    int
	// Payload is what the entity stores with the trip.
	Payload *engagemententity.AllocationTravelBudget
	names   map[int]string
	trip    email.BudgetLineTrip
}

// PlanTrip decides the line of a trip: the allocator's choice when it is an
// Active line of the trip's year (else 400), otherwise the suggestion. Any
// other failure falls back to no line, because an allocation never fails over
// budgets.
func (s *BudgetService) PlanTrip(ctx context.Context, in TripPlanInput) (*TripPlan, error) {
	e := in.Engagement
	plan := &TripPlan{Payload: &engagemententity.AllocationTravelBudget{SuggestionSource: domain.BudgetSourceNone},
		names: map[int]string{}, trip: email.BudgetLineTrip{TravellerEmail: in.ConsultantEmail, Country: allocStr(e.Country),
			StartDate: in.StartDate, EndDate: in.EndDate, AllocationType: allocStr(e.AllocationTypeName),
			Engagement: strings.TrimSpace(e.ID + " " + allocStr(e.Title))}}
	years, err := s.years(ctx)
	if err != nil {
		slog.WarnContext(ctx, "budget line not decided: years not read", "err", err)
		return plan, nil
	}
	y, ok := budgetYearFor(years, in.StartDate)
	if !ok {
		return plan, nil
	}
	plan.InScope, plan.Year, plan.trip.Year = true, y.Year, y.Year
	lines, err := s.lines(ctx, y.Year)
	if err != nil {
		slog.WarnContext(ctx, "budget line not decided: lines not read", "year", y.Year, "err", err)
		return plan, nil
	}
	active := map[int]bool{}
	for _, l := range lines {
		plan.names[l.ID] = budgetLineName(l)
		active[l.ID] = l.Status == domain.BudgetLineActive
	}
	if in.RequestedLineID != nil && !active[*in.RequestedLineID] {
		return nil, budgetBad("Choose a budget line of the trip's year.")
	}
	allocTypeID := 0
	if e.AllocationTypeID != nil {
		allocTypeID = *e.AllocationTypeID
	}
	sg, err := s.suggest(ctx, e, allocTypeID, in.ConsultantEmail, lines)
	if err != nil {
		slog.WarnContext(ctx, "budget suggestion failed", "engagement", e.ID, "err", err)
		sg = budgetSuggestion{source: domain.BudgetSourceNone}
		if in.RequestedLineID == nil {
			return plan, nil
		}
	}
	pl := plan.Payload
	pl.SuggestionSource = sg.source
	if sg.line != nil {
		id := sg.line.ID
		pl.SuggestedLineID = &id
	}
	switch {
	case in.RequestedLineID != nil:
		id, by := *in.RequestedLineID, domain.BudgetChosenByUser
		pl.BudgetLineID, pl.ChosenBy = &id, &by
	case pl.SuggestedLineID != nil:
		id, by := *pl.SuggestedLineID, domain.BudgetChosenBySystem
		pl.BudgetLineID, pl.ChosenBy = &id, &by
	}
	return plan, nil
}

// TripCreated finishes the budget side of an allocation that created a trip.
func (s *BudgetService) TripCreated(ctx context.Context, p domain.Perms, plan *TripPlan, jobNumber string) *domain.TripBudgetView {
	if plan == nil || jobNumber == "" {
		return nil
	}
	s.dropCache()
	if !plan.InScope {
		return nil
	}
	if plan.Payload.BudgetLineID == nil {
		s.notifyLineNeeded(ctx, plan.trip, jobNumber)
	}
	ref := func(id *int) *domain.BudgetLineRef {
		if id == nil {
			return nil
		}
		return &domain.BudgetLineRef{LineID: *id, Name: plan.names[*id]}
	}
	year := plan.Year
	return &domain.TripBudgetView{JobNumber: jobNumber, Year: &year, Line: ref(plan.Payload.BudgetLineID),
		Suggested: ref(plan.Payload.SuggestedLineID), Source: plan.Payload.SuggestionSource, ChosenBy: plan.Payload.ChosenBy,
		NeedsLine: plan.Payload.BudgetLineID == nil, CanEdit: budgetSetLine(p)}
}

// notifyLineNeeded emails the budget owner, in the background so the
// allocation answers at once.
func (s *BudgetService) notifyLineNeeded(ctx context.Context, trip email.BudgetLineTrip, jobNumber string) {
	if len(s.opts.LineRecipients) == 0 {
		slog.InfoContext(ctx, "budget line email skipped: BUDGET_LINE_RECIPIENTS is empty", "job", jobNumber)
		return
	}
	trip.JobNumber = jobNumber
	goSafe(ctx, &s.bg, nil, func() {
		mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if s.people != nil {
			trip.TravellerName = engagementVal(s.people.get(mctx, trip.TravellerEmail).Name)
		}
		m := email.BudgetLineNeededMessage(email.BudgetSettings{Recipients: s.opts.LineRecipients, AppBaseURL: s.opts.AppBaseURL}, trip)
		if err := s.mail.Send(mctx, m); err != nil {
			slog.ErrorContext(mctx, "budget line email not sent", "job", jobNumber, "err", err)
		}
	})
}

// ── Engagement line ─────────────────────────────────────────────────────────

// EngagementLine is the line set on an engagement, nil when none or unreadable.
func (s *BudgetService) EngagementLine(ctx context.Context, engagementID string) *domain.EngagementBudgetLine {
	code, err := s.entity.BudgetEngagementLine(ctx, engagementID)
	if err != nil {
		slog.WarnContext(ctx, "engagement budget line not read", "engagement", engagementID, "err", err)
		return nil
	}
	if code == nil || *code == "" {
		return nil
	}
	name := s.codeNames(ctx)[*code]
	if name == "" {
		name = *code
	}
	return &domain.EngagementBudgetLine{Code: *code, Name: name}
}

func (s *BudgetService) CheckEngagementLine(ctx context.Context, code string) error {
	if code == "" {
		return nil
	}
	_, ok, err := s.knownCode(ctx, code)
	if err != nil {
		return err
	}
	if !ok {
		return budgetBad("Choose a valid travel budget line.")
	}
	return nil
}

func (s *BudgetService) SetEngagementLine(ctx context.Context, p domain.Perms, engagementID, code string) error {
	var set *string
	if code != "" {
		set = &code
	}
	if _, err := s.entity.SetBudgetEngagementLine(ctx, engagementID, set, p.Email); err != nil {
		return err
	}
	s.dropCache()
	return nil
}
