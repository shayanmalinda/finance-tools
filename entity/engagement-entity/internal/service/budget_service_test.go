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

type fakeBudgetRepo struct {
	repository.BudgetRepository
	lines     map[int]domain.BudgetLine
	trip      domain.BudgetTrip
	tripErr   error
	codes     map[string]bool
	saved     *domain.BudgetAllocationTypeRuleRequest
	tripLine  *int
	tripSaved bool
}

func (f *fakeBudgetRepo) Years(context.Context) ([]domain.BudgetYear, error) {
	return []domain.BudgetYear{{Year: 2027}}, nil
}

func (f *fakeBudgetRepo) Line(_ context.Context, id int) (domain.BudgetLine, error) {
	l, ok := f.lines[id]
	if !ok {
		return l, repository.ErrBudgetNotFound
	}
	return l, nil
}

func (f *fakeBudgetRepo) LineCodeExists(_ context.Context, c string) (bool, error) {
	return f.codes[c], nil
}

func (f *fakeBudgetRepo) Trip(context.Context, string) (domain.BudgetTrip, error) {
	return f.trip, f.tripErr
}

func (f *fakeBudgetRepo) SetTripLine(_ context.Context, _ string, _, line *int, _ string) error {
	f.tripLine, f.tripSaved = line, true
	return nil
}

func (f *fakeBudgetRepo) SaveAllocationTypeRule(_ context.Context, _ int, in domain.BudgetAllocationTypeRuleRequest) error {
	f.saved = &in
	return nil
}

func (f *fakeBudgetRepo) CreateLine(context.Context, domain.BudgetLineCreateRequest) (int, error) {
	return 1, nil
}

func withTables(t *testing.T, present bool) {
	t.Helper()
	was := repository.HasBudgetTables()
	repository.SetBudgetTables(present)
	t.Cleanup(func() { repository.SetBudgetTables(was) })
}

func isBad(err error) bool {
	var ve *apierror.ValidationError
	return errors.As(err, &ve)
}

func TestBudgetService_WithoutTablesEveryCallIs503(t *testing.T) {
	withTables(t, false)
	s := NewBudgetService(&fakeBudgetRepo{})
	ctx := context.Background()
	_, e1 := s.Years(ctx)
	_, e2 := s.Lines(ctx, 2027, false)
	_, e3 := s.Trip(ctx, "TRV-1")
	_, e4 := s.NeedsLine(ctx, 2027, 0, 0)
	_, e5 := s.Ledger(ctx, domain.BudgetLedgerFilter{})
	for i, e := range []error{e1, e2, e3, e4, e5} {
		if !errors.Is(e, repository.ErrBudgetNoTables) {
			t.Errorf("call %d: err = %v, want ErrBudgetNoTables", i, e)
		}
	}
}

func TestBudgetService_RuleValidation(t *testing.T) {
	withTables(t, true)
	f := &fakeBudgetRepo{codes: map[string]bool{"SALES_EU": true}}
	s := NewBudgetService(f)
	ctx := context.Background()
	eu, bogus := "SALES_EU", "NOPE"
	for name, in := range map[string]domain.BudgetAllocationTypeRuleRequest{
		"unknown kind":      {RuleKind: "MAYBE"},
		"LINE without code": {RuleKind: "LINE"},
		"code with REGION":  {RuleKind: "REGION", LineCode: &eu},
		"unknown code":      {RuleKind: "LINE", LineCode: &bogus},
	} {
		if _, err := s.SaveAllocationTypeRule(ctx, 12, in); !isBad(err) {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}
	if f.saved != nil {
		t.Error("an invalid rule was saved")
	}
	r, err := s.SaveAllocationTypeRule(ctx, 12, domain.BudgetAllocationTypeRuleRequest{RuleKind: "line", LineCode: &eu})
	if err != nil || r.RuleKind != "LINE" || f.saved == nil {
		t.Fatalf("valid LINE: %+v, %v", r, err)
	}
	if _, err := s.SaveAllocationTypeRule(ctx, 12, domain.BudgetAllocationTypeRuleRequest{RuleKind: "TEAM"}); err != nil {
		t.Errorf("TEAM without code: %v", err)
	}
}

func TestBudgetService_SetTripLineChecks(t *testing.T) {
	withTables(t, true)
	y27 := 2027
	f := &fakeBudgetRepo{
		trip: domain.BudgetTrip{JobNumber: "TRV-1", BudgetYear: &y27},
		lines: map[int]domain.BudgetLine{
			7: {ID: 7, Year: 2027, Status: "Active"},
			8: {ID: 8, Year: 2027, Status: "Retired"},
			9: {ID: 9, Year: 2028, Status: "Active"},
		},
	}
	s := NewBudgetService(f)
	ctx := context.Background()
	for name, id := range map[string]int{"retired": 8, "other year": 9, "unknown": 99} {
		line := id
		if _, err := s.SetTripLine(ctx, "TRV-1", domain.BudgetTripRequest{BudgetLineID: &line}); !isBad(err) {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}
	if f.tripSaved {
		t.Error("a bad line was saved")
	}
	ok := 7
	if _, err := s.SetTripLine(ctx, "TRV-1", domain.BudgetTripRequest{BudgetLineID: &ok}); err != nil || f.tripLine == nil || *f.tripLine != 7 {
		t.Fatalf("valid line: %v", err)
	}
	if _, err := s.SetTripLine(ctx, "TRV-1", domain.BudgetTripRequest{}); err != nil || f.tripLine != nil {
		t.Fatalf("clearing the line: %v", err)
	}
	// A trip outside every year cannot take a line.
	f.trip.BudgetYear = nil
	if _, err := s.SetTripLine(ctx, "TRV-1", domain.BudgetTripRequest{BudgetLineID: &ok}); !isBad(err) {
		t.Errorf("trip without a year: err = %v, want 400", err)
	}
	f.tripErr = repository.ErrBudgetNotFound
	var nf *apierror.NotFoundError
	if _, err := s.SetTripLine(ctx, "TRV-9", domain.BudgetTripRequest{}); !errors.As(err, &nf) {
		t.Errorf("missing trip: err = %v, want 404", err)
	}
}

func TestBudgetService_CreateLineValidation(t *testing.T) {
	withTables(t, true)
	s := NewBudgetService(&fakeBudgetRepo{lines: map[int]domain.BudgetLine{1: {ID: 1}}})
	ok := domain.BudgetLineCreateRequest{Year: 2027, Code: "X", FunctionName: "Sales", SubFunction: "X", AmountUSD: 10}
	if _, err := s.CreateLine(context.Background(), ok); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for name, mut := range map[string]func(*domain.BudgetLineCreateRequest){
		"unknown year":    func(r *domain.BudgetLineCreateRequest) { r.Year = 1999 },
		"negative amount": func(r *domain.BudgetLineCreateRequest) { r.AmountUSD = -1 },
		"blank code":      func(r *domain.BudgetLineCreateRequest) { r.Code = " " },
		"blank function":  func(r *domain.BudgetLineCreateRequest) { r.FunctionName = "" },
	} {
		in := ok
		mut(&in)
		if _, err := s.CreateLine(context.Background(), in); !isBad(err) {
			t.Errorf("%s: err = %v, want 400", name, err)
		}
	}
}

func TestBudgetService_LedgerAndNeedsLineValidation(t *testing.T) {
	withTables(t, true)
	s := NewBudgetService(&fakeBudgetRepo{})
	ctx := context.Background()
	for _, f := range []domain.BudgetLedgerFilter{
		{}, {From: "2027-01-01"}, {From: "2027-02-01", To: "2027-01-01"}, {From: "2020-01-01", To: "2027-01-01"},
	} {
		if _, err := s.Ledger(ctx, f); !isBad(err) {
			t.Errorf("%+v: err = %v, want 400", f, err)
		}
	}
	if _, err := s.NeedsLine(ctx, 0, 0, 0); !isBad(err) {
		t.Errorf("year 0: err = %v, want 400", err)
	}
	if _, err := s.NeedsLine(ctx, 2027, 10, -1); !isBad(err) {
		t.Errorf("negative offset: err = %v, want 400", err)
	}
}
