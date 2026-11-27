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
	"math"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// BudgetService is the travel budget area: lines, the rules that suggest a
// line, the line of an engagement and of a trip, and the cost ledger. Every
// method answers 503 until the budget tables exist.
type BudgetService interface {
	Years(ctx context.Context) (domain.BudgetYearsResponse, error)
	Lines(ctx context.Context, year int, includeRetired bool) (domain.BudgetLinesResponse, error)
	CreateLine(ctx context.Context, in domain.BudgetLineCreateRequest) (domain.BudgetLine, error)
	UpdateLine(ctx context.Context, id int, in domain.BudgetLineUpdateRequest) (domain.BudgetLine, error)
	RetireLine(ctx context.Context, id int, actor string) error
	AllocationTypeRules(ctx context.Context) (domain.BudgetAllocationTypeRulesResponse, error)
	SaveAllocationTypeRule(ctx context.Context, id int, in domain.BudgetAllocationTypeRuleRequest) (domain.BudgetAllocationTypeRule, error)
	RegionRules(ctx context.Context) (domain.BudgetRegionRulesResponse, error)
	TeamRules(ctx context.Context) (domain.BudgetTeamRulesResponse, error)
	EngagementLine(ctx context.Context, engagementID string) (domain.BudgetEngagementLine, error)
	SetEngagementLine(ctx context.Context, engagementID string, in domain.BudgetEngagementLineRequest) (domain.BudgetEngagementLine, error)
	Trip(ctx context.Context, jobNumber string) (domain.BudgetTrip, error)
	SetTripLine(ctx context.Context, jobNumber string, in domain.BudgetTripRequest) (domain.BudgetTrip, error)
	NeedsLine(ctx context.Context, year, limit, offset int) (domain.BudgetNeedsLineResponse, error)
	Ledger(ctx context.Context, f domain.BudgetLedgerFilter) (domain.BudgetLedger, error)
}

type budgetService struct{ repo repository.BudgetRepository }

// NewBudgetService constructs a BudgetService.
func NewBudgetService(repo repository.BudgetRepository) BudgetService {
	return &budgetService{repo: repo}
}

func budgetBad(msg string) error { return &apierror.ValidationError{Msg: msg} }

// ready refuses a call before the DDL has run.
func budgetReady() error {
	if !repository.HasBudgetTables() {
		return repository.ErrBudgetNoTables
	}
	return nil
}

// budgetBlank is nil for a blank string, else the trimmed value.
func budgetBlank(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func budgetMoney(v float64) (float64, error) {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, budgetBad("amountUsd must be 0 or more.")
	}
	return math.Round(v*100) / 100, nil
}

func budgetNotFound(what string) error { return &apierror.NotFoundError{Msg: what + " not found."} }

func (s *budgetService) Years(ctx context.Context) (domain.BudgetYearsResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetYearsResponse{}, err
	}
	y, err := s.repo.Years(ctx)
	return domain.BudgetYearsResponse{Years: y}, err
}

func (s *budgetService) Lines(ctx context.Context, year int, includeRetired bool) (domain.BudgetLinesResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetLinesResponse{}, err
	}
	if year < 0 {
		return domain.BudgetLinesResponse{}, budgetBad("year must be a positive integer")
	}
	l, err := s.repo.Lines(ctx, year, includeRetired)
	return domain.BudgetLinesResponse{Lines: l}, err
}

// budgetLineFields checks the fields a create and an update share.
func budgetLineFields(function, sub *string, amount *float64) error {
	*function, *sub = strings.TrimSpace(*function), strings.TrimSpace(*sub)
	if *function == "" || len(*function) > 100 {
		return budgetBad("functionName is required (100 characters at most).")
	}
	if *sub == "" || len(*sub) > 150 {
		return budgetBad("subFunction is required (150 characters at most).")
	}
	v, err := budgetMoney(*amount)
	*amount = v
	return err
}

func (s *budgetService) CreateLine(ctx context.Context, in domain.BudgetLineCreateRequest) (domain.BudgetLine, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetLine{}, err
	}
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" || len(in.Code) > 40 {
		return domain.BudgetLine{}, budgetBad("code is required (40 characters at most).")
	}
	if err := budgetLineFields(&in.FunctionName, &in.SubFunction, &in.AmountUSD); err != nil {
		return domain.BudgetLine{}, err
	}
	in.HolderName, in.HolderEmail = budgetBlank(in.HolderName), budgetBlank(in.HolderEmail)
	years, err := s.repo.Years(ctx)
	if err != nil {
		return domain.BudgetLine{}, err
	}
	known := false
	for _, y := range years {
		known = known || y.Year == in.Year
	}
	if !known {
		return domain.BudgetLine{}, budgetBad("Unknown budget year.")
	}
	id, err := s.repo.CreateLine(ctx, in)
	if err != nil {
		return domain.BudgetLine{}, err
	}
	return s.repo.Line(ctx, id)
}

func (s *budgetService) line(ctx context.Context, id int) (domain.BudgetLine, error) {
	l, err := s.repo.Line(ctx, id)
	if errors.Is(err, repository.ErrBudgetNotFound) {
		return l, budgetNotFound("Budget line")
	}
	return l, err
}

func (s *budgetService) UpdateLine(ctx context.Context, id int, in domain.BudgetLineUpdateRequest) (domain.BudgetLine, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetLine{}, err
	}
	if err := budgetLineFields(&in.FunctionName, &in.SubFunction, &in.AmountUSD); err != nil {
		return domain.BudgetLine{}, err
	}
	st, ok := canonical(strings.TrimSpace(in.Status), domain.BudgetLineActive, domain.BudgetLineRetired)
	if !ok {
		return domain.BudgetLine{}, budgetBad("status must be Active or Retired.")
	}
	in.Status = st
	in.HolderName, in.HolderEmail = budgetBlank(in.HolderName), budgetBlank(in.HolderEmail)
	if _, err := s.line(ctx, id); err != nil {
		return domain.BudgetLine{}, err
	}
	if err := s.repo.UpdateLine(ctx, id, in); err != nil {
		return domain.BudgetLine{}, err
	}
	return s.line(ctx, id)
}

func (s *budgetService) RetireLine(ctx context.Context, id int, actor string) error {
	if err := budgetReady(); err != nil {
		return err
	}
	if _, err := s.line(ctx, id); err != nil {
		return err
	}
	return s.repo.RetireLine(ctx, id, actor)
}

func (s *budgetService) AllocationTypeRules(ctx context.Context) (domain.BudgetAllocationTypeRulesResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetAllocationTypeRulesResponse{}, err
	}
	r, err := s.repo.AllocationTypeRules(ctx)
	return domain.BudgetAllocationTypeRulesResponse{Rules: r}, err
}

// lineCode checks that a code names a budget line (of any year).
func (s *budgetService) lineCode(ctx context.Context, code *string) (*string, error) {
	code = budgetBlank(code)
	if code == nil {
		return nil, nil
	}
	ok, err := s.repo.LineCodeExists(ctx, *code)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, budgetBad("Unknown budget line code.")
	}
	return code, nil
}

func (s *budgetService) SaveAllocationTypeRule(ctx context.Context, id int, in domain.BudgetAllocationTypeRuleRequest) (domain.BudgetAllocationTypeRule, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetAllocationTypeRule{}, err
	}
	kind, ok := canonical(strings.TrimSpace(in.RuleKind), domain.BudgetRuleLine, domain.BudgetRuleRegion,
		domain.BudgetRuleTeam, domain.BudgetRuleEngagement, domain.BudgetRuleNone)
	if !ok {
		return domain.BudgetAllocationTypeRule{}, budgetBad("ruleKind must be LINE, REGION, TEAM, ENGAGEMENT or NONE.")
	}
	code, err := s.lineCode(ctx, in.LineCode)
	if err != nil {
		return domain.BudgetAllocationTypeRule{}, err
	}
	if kind == domain.BudgetRuleLine && code == nil {
		return domain.BudgetAllocationTypeRule{}, budgetBad("A LINE rule needs a lineCode.")
	}
	if kind != domain.BudgetRuleLine && code != nil {
		return domain.BudgetAllocationTypeRule{}, budgetBad("Only a LINE rule takes a lineCode.")
	}
	in.RuleKind, in.LineCode = kind, code
	if err := s.repo.SaveAllocationTypeRule(ctx, id, in); err != nil {
		return domain.BudgetAllocationTypeRule{}, err
	}
	return domain.BudgetAllocationTypeRule{AllocationTypeID: id, RuleKind: kind, LineCode: code}, nil
}

func (s *budgetService) RegionRules(ctx context.Context) (domain.BudgetRegionRulesResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetRegionRulesResponse{}, err
	}
	r, err := s.repo.RegionRules(ctx)
	return domain.BudgetRegionRulesResponse{Rules: r}, err
}

func (s *budgetService) TeamRules(ctx context.Context) (domain.BudgetTeamRulesResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetTeamRulesResponse{}, err
	}
	r, err := s.repo.TeamRules(ctx)
	return domain.BudgetTeamRulesResponse{Rules: r}, err
}

func (s *budgetService) EngagementLine(ctx context.Context, engagementID string) (domain.BudgetEngagementLine, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetEngagementLine{}, err
	}
	code, err := s.repo.EngagementLine(ctx, engagementID)
	return domain.BudgetEngagementLine{EngagementID: engagementID, LineCode: code}, err
}

func (s *budgetService) SetEngagementLine(ctx context.Context, engagementID string, in domain.BudgetEngagementLineRequest) (domain.BudgetEngagementLine, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetEngagementLine{}, err
	}
	code, err := s.lineCode(ctx, in.LineCode)
	if err != nil {
		return domain.BudgetEngagementLine{}, err
	}
	if err := s.repo.SetEngagementLine(ctx, engagementID, code, in.UpdatedBy); err != nil {
		return domain.BudgetEngagementLine{}, err
	}
	return domain.BudgetEngagementLine{EngagementID: engagementID, LineCode: code}, nil
}

func (s *budgetService) Trip(ctx context.Context, jobNumber string) (domain.BudgetTrip, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetTrip{}, err
	}
	t, err := s.repo.Trip(ctx, jobNumber)
	if errors.Is(err, repository.ErrBudgetNotFound) {
		return t, budgetNotFound("Travel request")
	}
	return t, err
}

// SetTripLine sets the line of a trip: it must be an Active line of the year
// holding the trip's start date. nil clears it.
func (s *budgetService) SetTripLine(ctx context.Context, jobNumber string, in domain.BudgetTripRequest) (domain.BudgetTrip, error) {
	trip, err := s.Trip(ctx, jobNumber)
	if err != nil {
		return trip, err
	}
	if in.BudgetLineID != nil {
		l, err := s.repo.Line(ctx, *in.BudgetLineID)
		if errors.Is(err, repository.ErrBudgetNotFound) {
			return trip, budgetBad("Unknown budget line.")
		}
		if err != nil {
			return trip, err
		}
		if l.Status != domain.BudgetLineActive {
			return trip, budgetBad("This budget line is retired.")
		}
		if trip.BudgetYear == nil || *trip.BudgetYear != l.Year {
			return trip, budgetBad("Choose a budget line of the trip's year.")
		}
	}
	if err := s.repo.SetTripLine(ctx, jobNumber, trip.BudgetYear, in.BudgetLineID, in.UpdatedBy); err != nil {
		return trip, err
	}
	return s.repo.Trip(ctx, jobNumber)
}

func (s *budgetService) NeedsLine(ctx context.Context, year, limit, offset int) (domain.BudgetNeedsLineResponse, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetNeedsLineResponse{}, err
	}
	if year <= 0 {
		return domain.BudgetNeedsLineResponse{}, budgetBad("year is required")
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, domain.BudgetNeedsLineMax)
	if offset < 0 {
		return domain.BudgetNeedsLineResponse{}, budgetBad("offset must be 0 or more")
	}
	items, total, err := s.repo.NeedsLine(ctx, year, limit, offset)
	return domain.BudgetNeedsLineResponse{Items: items, Total: total}, err
}

// budgetLedgerMaxDays bounds a ledger range (two years).
const budgetLedgerMaxDays = 731

func (s *budgetService) Ledger(ctx context.Context, f domain.BudgetLedgerFilter) (domain.BudgetLedger, error) {
	if err := budgetReady(); err != nil {
		return domain.BudgetLedger{}, err
	}
	from, err := time.Parse(time.DateOnly, f.From)
	if err != nil {
		return domain.BudgetLedger{}, budgetBad("from must be a date (YYYY-MM-DD)")
	}
	to, err := time.Parse(time.DateOnly, f.To)
	if err != nil {
		return domain.BudgetLedger{}, budgetBad("to must be a date (YYYY-MM-DD)")
	}
	if to.Before(from) {
		return domain.BudgetLedger{}, budgetBad("to must not be before from")
	}
	if to.Sub(from) > budgetLedgerMaxDays*24*time.Hour {
		return domain.BudgetLedger{}, budgetBad("the range is at most 731 days")
	}
	return s.repo.Ledger(ctx, f)
}
