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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

type fakeCostRepo struct {
	repository.DashboardRepository
	invoices, payments, cards, claims []repository.DashboardCostRow
	cardErr                           error
	trips                             map[string]repository.DashboardTripRef
	engs                              map[string]repository.DashboardCostEngagement
	filter                            domain.DashboardCostLinesFilter
	tripJobs                          []string
}

func (f *fakeCostRepo) InvoiceLines(_ context.Context, fl domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error) {
	f.filter = fl
	return f.invoices, nil
}
func (f *fakeCostRepo) PaymentLines(context.Context, domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error) {
	return f.payments, nil
}
func (f *fakeCostRepo) CardLines(context.Context, domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error) {
	return f.cards, f.cardErr
}
func (f *fakeCostRepo) ClaimLines(context.Context, domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error) {
	return f.claims, nil
}
func (f *fakeCostRepo) TripsByJob(_ context.Context, jobs []string) (map[string]repository.DashboardTripRef, error) {
	f.tripJobs = jobs
	return f.trips, nil
}
func (f *fakeCostRepo) CostEngagements(context.Context, []string) (map[string]repository.DashboardCostEngagement, error) {
	return f.engs, nil
}

func sp(s string) *string { return &s }

func costFilter() domain.DashboardCostLinesFilter {
	return domain.DashboardCostLinesFilter{From: "2026-01-01", To: "2026-11-05"}
}

func TestDashboardPaidOf(t *testing.T) {
	for name, want := range map[string]string{
		"Paid": "paid", "paid pre sale": "paid", " Paid Post Sale ": "paid",
		"Internal Engagement": "nonPaid", "Marketing Event": "nonPaid", "Pre Sale": "nonPaid",
		"": "unknown", "   ": "unknown",
	} {
		if got := dashboardPaidOf(name); got != want {
			t.Errorf("dashboardPaidOf(%q) = %s, want %s", name, got, want)
		}
	}
}

// A card line whose job number matches a trip takes the trip's country, agent
// and customer; one without a trip keeps its own travel_country and has no
// agent; the traveller of a card line stays the cardholder.
func TestDashboardCostLines_CardLineJoinsTheTrip(t *testing.T) {
	repo := &fakeCostRepo{
		invoices: []repository.DashboardCostRow{{
			ID: "INV-1", Source: "invoice", JobNumber: sp("TRV-1"), Currency: "LKR", Amount: 100, CostType: "Hotel",
			Trip: repository.DashboardTripRef{JobNumber: sp("TRV-1"), TravellerEmail: sp("jane@example.com"), Country: sp("India"), AllocationID: sp("A-1"), EngagementType: sp("Internal Engagement")},
		}},
		cards: []repository.DashboardCostRow{
			{ID: "CC-1", Source: "card", JobNumber: sp("trv-1"), Currency: "EUR", Amount: 10, CostType: "Card", EmployeeEmail: sp("cardholder@example.com"), TravelCountry: sp("Nowhere")},
			{ID: "CC-2", Source: "card", JobNumber: sp("TRV-TYPO"), Currency: "EUR", Amount: 20, CostType: "Card", EmployeeEmail: sp("x@example.com"), TravelCountry: sp("France")},
			{ID: "CC-3", Source: "card", Currency: "EUR", Amount: 30, CostType: "Card", EmployeeEmail: sp("y@example.com")},
		},
		trips: map[string]repository.DashboardTripRef{
			"TRV-1": {JobNumber: sp("TRV-1"), TravellerEmail: sp("jane@example.com"), Country: sp("India"), AgentID: ip(3), AgentName: sp("Classic"), AllocationID: sp("A-1"), CustomerName: sp("Trip Customer"), EngagementType: sp("Paid")},
		},
		engs: map[string]repository.DashboardCostEngagement{
			"A-1": {EngagementID: "E-1", CustomerName: sp("Acme"), TypeName: sp("Paid Post Sale"), AllocationType: sp("Onsite")},
		},
	}
	got, err := NewDashboardService(repo, nil).CostLines(context.Background(), costFilter())
	if err != nil || len(got.Items) != 4 {
		t.Fatalf("CostLines = %+v, %v", got, err)
	}
	byID := map[string]domain.DashboardCostEntry{}
	for _, e := range got.Items {
		byID[e.ID] = e
	}
	inv := byID["INV-1"]
	if *inv.TravellerEmail != "jane@example.com" || *inv.Country != "India" || *inv.EngagementID != "E-1" || *inv.Customer != "Acme" || inv.Paid != "paid" || *inv.AllocationType != "Onsite" {
		t.Errorf("invoice = %+v", inv)
	}
	c1 := byID["CC-1"]
	if *c1.TravellerEmail != "cardholder@example.com" || *c1.Country != "India" || *c1.TravelAgentName != "Classic" || *c1.Customer != "Acme" || c1.Paid != "paid" || *c1.EngagementID != "E-1" {
		t.Errorf("card with a trip = %+v", c1)
	}
	c2 := byID["CC-2"]
	if *c2.Country != "France" || c2.TravelAgentID != nil || c2.Paid != "unknown" || c2.EngagementID != nil {
		t.Errorf("card without a trip = %+v", c2)
	}
	if c3 := byID["CC-3"]; c3.JobNumber != nil || c3.Country != nil || c3.Paid != "unknown" {
		t.Errorf("card without a job number = %+v", c3)
	}
	if len(repo.tripJobs) != 2 {
		t.Errorf("trip lookups = %v, want the two distinct job numbers", repo.tripJobs)
	}
}

func ip(n int) *int { return &n }

// The trip's own engagement type is the fallback when the allocation has no engagement.
func TestDashboardCostLines_PaidFallsBackToTheTripType(t *testing.T) {
	repo := &fakeCostRepo{payments: []repository.DashboardCostRow{
		{ID: "PD-1", Source: "perDiem", Trip: repository.DashboardTripRef{JobNumber: sp("TRV-1"), EngagementType: sp("Paid Pre Sale")}},
		{ID: "PD-2", Source: "perDiem", Trip: repository.DashboardTripRef{JobNumber: sp("TRV-2"), EngagementType: sp("Marketing Event")}},
		{ID: "PD-3", Source: "perDiem", Trip: repository.DashboardTripRef{JobNumber: sp("TRV-3")}},
	}}
	got, err := NewDashboardService(repo, nil).CostLines(context.Background(), domain.DashboardCostLinesFilter{From: "2026-01-01", To: "2026-11-05", Sources: []string{"perDiem"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"paid", "nonPaid", "unknown"}
	for i, e := range got.Items {
		if e.Paid != want[i] {
			t.Errorf("%s paid = %s, want %s", e.ID, e.Paid, want[i])
		}
	}
}

func TestDashboardCostLines_TruncationAndUnavailable(t *testing.T) {
	rows := make([]repository.DashboardCostRow, 4)
	for i := range rows {
		rows[i] = repository.DashboardCostRow{ID: "INV-" + string(rune('1'+i)), Source: "invoice"}
	}
	repo := &fakeCostRepo{invoices: rows, cardErr: repository.ErrDashboardNoExpenseDB, claims: []repository.DashboardCostRow{{ID: "EC-1", Source: "claim"}}}
	f := costFilter()
	f.Limit = 3 // the repository returned 4: one more than the cap
	got, err := NewDashboardService(repo, nil).CostLines(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 4 || len(got.Truncated) != 1 || got.Truncated[0] != "invoice" {
		t.Errorf("items %d truncated %v, want 3 invoices + 1 claim and invoice truncated", len(got.Items), got.Truncated)
	}
	if len(got.Unavailable) != 1 || got.Unavailable[0] != "card" {
		t.Errorf("unavailable = %v, want [card]", got.Unavailable)
	}
}

func TestDashboardCostLines_Validation(t *testing.T) {
	svc := NewDashboardService(&fakeCostRepo{}, nil)
	bad := []domain.DashboardCostLinesFilter{
		{From: "x", To: "2026-11-05"},
		{From: "2026-01-01", To: "2026/11/05"},
		{From: "2026-11-05", To: "2026-01-01"},
		{From: "2024-01-01", To: "2026-11-05"}, // more than 731 days
		{From: "2026-01-01", To: "2026-11-05", Sources: []string{"cash"}},
	}
	for _, f := range bad {
		if _, err := svc.CostLines(context.Background(), f); !dashboardIsValidation(err) {
			t.Errorf("%+v: err = %v, want a validation error", f, err)
		}
	}
	// Exactly 731 days is allowed.
	if _, err := svc.CostLines(context.Background(), domain.DashboardCostLinesFilter{From: "2024-11-05", To: "2026-11-05"}); err != nil {
		t.Errorf("731 days: %v", err)
	}
}

func TestDashboardCostLines_InvoiceFailureFailsTheCall(t *testing.T) {
	boom := errors.New("travelapp down")
	svc := NewDashboardService(&failingInvoiceRepo{err: boom}, nil)
	if _, err := svc.CostLines(context.Background(), costFilter()); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

type failingInvoiceRepo struct {
	fakeCostRepo
	err error
}

func (f *failingInvoiceRepo) InvoiceLines(context.Context, domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error) {
	return nil, f.err
}

func TestDashboardWidgetsServiceValidation(t *testing.T) {
	svc := NewDashboardService(&fakeCostRepo{}, nil)
	if _, err := svc.AccountManager(context.Background(), "", "2026-11-05"); !dashboardIsValidation(err) {
		t.Errorf("AccountManager without email: %v", err)
	}
	if _, err := svc.MyVisas(context.Background(), "nobody", "2026-11-05"); !dashboardIsValidation(err) {
		t.Errorf("MyVisas without a valid email: %v", err)
	}
	if _, err := svc.TravelTeam(context.Background(), "yesterday"); !dashboardIsValidation(err) {
		t.Errorf("TravelTeam bad date: %v", err)
	}
	// No allocate types: an empty manager answer without any query.
	got, err := svc.Manager(context.Background(), domain.DashboardManagerFilter{Today: "2026-11-05"})
	if err != nil || got.TopBy != "customer" || len(got.WithoutConsultants) != 0 {
		t.Errorf("Manager without types = %+v, %v", got, err)
	}
}
