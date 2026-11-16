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
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// fakeDashboardRepo records the calls and returns canned data.
type fakeDashboardRepo struct {
	repository.DashboardRepository
	calls   []string
	window  repository.DashboardWindow
	trips   repository.DashboardTripFilter
	cardErr error
}

func (f *fakeDashboardRepo) call(name string) { f.calls = append(f.calls, name) }

func (f *fakeDashboardRepo) OwnAllocations(_ context.Context, email, today string) ([]domain.DashboardOwnAllocation, error) {
	f.call("OwnAllocations " + email + " " + today)
	return []domain.DashboardOwnAllocation{}, nil
}
func (f *fakeDashboardRepo) TeamAllocationOn(_ context.Context, _, day, next string) (*domain.DashboardTeamAllocation, error) {
	f.call("TeamAllocationOn " + day + " " + next)
	return nil, nil
}
func (f *fakeDashboardRepo) UpcomingTrips(_ context.Context, tf repository.DashboardTripFilter) ([]domain.DashboardTrip, int, error) {
	f.call("UpcomingTrips")
	f.trips = tf
	return []domain.DashboardTrip{}, 0, nil
}
func (f *fakeDashboardRepo) PendingApprovalCounts(context.Context, []int) (map[int]int, error) {
	f.call("PendingApprovalCounts")
	return map[int]int{1: 2}, nil
}
func (f *fakeDashboardRepo) ClearanceCounts(context.Context, []int) (map[string]int, error) {
	f.call("ClearanceCounts")
	return map[string]int{}, nil
}
func (f *fakeDashboardRepo) NewTravelRequestCount(context.Context) (int, error) {
	f.call("NewTravelRequestCount")
	return 3, nil
}
func (f *fakeDashboardRepo) InvoiceQueueCounts(context.Context) (repository.DashboardInvoiceQueues, error) {
	f.call("InvoiceQueueCounts")
	return repository.DashboardInvoiceQueues{Pending: 1, AcceptedByTravel: 2, ToPush: 3, Failed: 4}, nil
}
func (f *fakeDashboardRepo) PaymentQueueCounts(context.Context) (int, int, error) {
	f.call("PaymentQueueCounts")
	return 5, 6, nil
}
func (f *fakeDashboardRepo) TravelKPIs(_ context.Context, w repository.DashboardWindow) (repository.DashboardTravelKPIs, error) {
	f.call("TravelKPIs")
	f.window = w
	return repository.DashboardTravelKPIs{Upcoming: 9}, nil
}
func (f *fakeDashboardRepo) TravelCells(context.Context, repository.DashboardWindow) ([]repository.DashboardTravelCell, error) {
	f.call("TravelCells")
	classic, empty := "Classic Travels", ""
	return []repository.DashboardTravelCell{
		{Month: "2026-10", Country: "France", Agency: &classic, Count: 2},
		{Month: "2026-11", Country: "France", Agency: &empty, Count: 1},
		{Month: "2026-11", Country: "India", Count: 4},
	}, nil
}
func (f *fakeDashboardRepo) InvoiceCost(context.Context, repository.DashboardWindow) ([]domain.DashboardCostLine, error) {
	f.call("InvoiceCost")
	return []domain.DashboardCostLine{{Source: domain.DashboardCostInvoice, Amount: 1}}, nil
}
func (f *fakeDashboardRepo) PaymentCost(context.Context, repository.DashboardWindow) ([]domain.DashboardCostLine, error) {
	f.call("PaymentCost")
	return nil, nil
}
func (f *fakeDashboardRepo) CardCost(context.Context, repository.DashboardWindow) ([]domain.DashboardCostLine, error) {
	f.call("CardCost")
	return nil, f.cardErr
}

var dashboardTestNow = func() time.Time { return time.Date(2026, 11, 5, 22, 30, 0, 0, time.UTC) }

func TestDashboardWindowFor(t *testing.T) {
	w := DashboardWindowFor(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	want := repository.DashboardWindow{
		Today: "2026-01-31", Tomorrow: "2026-02-01", In7Days: "2026-02-08", In30Days: "2026-03-03",
		MonthStart: "2026-01-01", NextMonth: "2026-02-01", YearStart: "2025-02-01", BenchEndDay: "2026-02-13",
	}
	if w != want {
		t.Errorf("window = %+v\nwant     %+v", w, want)
	}
}

func TestDashboardMyWork(t *testing.T) {
	repo := &fakeDashboardRepo{}
	svc := NewDashboardService(repo, dashboardTestNow)

	if _, err := svc.MyWork(context.Background(), domain.DashboardMyWorkFilter{}); !dashboardIsValidation(err) {
		t.Errorf("no email: err = %v", err)
	}
	if _, err := svc.MyWork(context.Background(), domain.DashboardMyWorkFilter{Email: "a@wso2.com", Today: "05/11/2026"}); !dashboardIsValidation(err) {
		t.Errorf("bad date: err = %v", err)
	}

	got, err := svc.MyWork(context.Background(), domain.DashboardMyWorkFilter{Email: "jane@wso2.com"})
	if err != nil {
		t.Fatal(err)
	}
	// Without trips, travelapp is not read; today defaults to the UTC date.
	if strings.Join(repo.calls, "|") != "OwnAllocations jane@wso2.com 2026-11-05|TeamAllocationOn 2026-11-05 2026-11-06" {
		t.Errorf("calls = %v", repo.calls)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"trips":[]`) {
		t.Errorf("trips must be [] : %s", b)
	}

	repo.calls = nil
	if _, err := svc.MyWork(context.Background(), domain.DashboardMyWorkFilter{Email: "jane@wso2.com", Trips: true}); err != nil {
		t.Fatal(err)
	}
	if repo.trips.Email != "jane@wso2.com" || repo.trips.Limit != 3 {
		t.Errorf("trip filter = %+v", repo.trips)
	}
}

func TestDashboardQueues_OnlyWhatIsAskedFor(t *testing.T) {
	repo := &fakeDashboardRepo{}
	svc := NewDashboardService(repo, dashboardTestNow)

	got, err := svc.Queues(context.Background(), domain.DashboardQueuesFilter{PendingApprovalTypes: []int{1}, Invoices: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(repo.calls, "|") != "PendingApprovalCounts|InvoiceQueueCounts" {
		t.Errorf("calls = %v", repo.calls)
	}
	if got.PendingApproval[1] != 2 || got.InvoicesFailed != 4 || got.InvoicesToPush != 3 || got.PaymentsToPush != 0 {
		t.Errorf("queues = %+v", got)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"clearance":{}`) {
		t.Errorf("empty map must be {} : %s", b)
	}
}

func TestDashboardTravel(t *testing.T) {
	repo := &fakeDashboardRepo{}
	svc := NewDashboardService(repo, dashboardTestNow)

	got, err := svc.Travel(context.Background(), domain.DashboardTravelFilter{Trips: true, CostInvoices: true, CostPayments: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(repo.calls, "|"), "CardCost") {
		t.Error("card spend read without costCards")
	}
	if got.From != "2025-12-01" || got.To != "2026-12-01" || repo.window.Today != "2026-11-05" {
		t.Errorf("window = %s..%s, %+v", got.From, got.To, repo.window)
	}
	tr := got.Trips
	if tr == nil || tr.UpcomingTotal != 9 {
		t.Fatalf("trips = %+v", tr)
	}
	if len(tr.ByMonth) != 2 || tr.ByMonth[0].Label != "2026-10" || tr.ByMonth[1].Count != 5 {
		t.Errorf("byMonth = %+v", tr.ByMonth)
	}
	if len(tr.ByCountry) != 2 || tr.ByCountry[0].Label != "India" || tr.ByCountry[1].Count != 3 {
		t.Errorf("byCountry = %+v", tr.ByCountry)
	}
	// Trips without an agency are left out of Trips by Travel Agent.
	if len(tr.ByAgency) != 1 || tr.ByAgency[0].Count != 2 {
		t.Errorf("byAgency = %+v", tr.ByAgency)
	}
	if len(got.Cost) != 1 {
		t.Errorf("cost = %+v", got.Cost)
	}

	// Card spend without the expense pool is a 503, not a 500.
	repo = &fakeDashboardRepo{cardErr: repository.ErrDashboardNoExpenseDB}
	_, err = NewDashboardService(repo, dashboardTestNow).Travel(context.Background(), domain.DashboardTravelFilter{CostCards: true})
	var sue *apierror.ServiceUnavailableError
	if !errors.As(err, &sue) {
		t.Errorf("err = %v, want ServiceUnavailableError", err)
	}
}

func TestDashboardValidation(t *testing.T) {
	svc := NewDashboardService(&fakeDashboardRepo{}, dashboardTestNow)
	ctx := context.Background()
	if _, err := svc.AllocationsToday(ctx, domain.DashboardAllocationsTodayFilter{Date: "2026-13-01"}); !dashboardIsValidation(err) {
		t.Errorf("AllocationsToday bad date: %v", err)
	}
	if _, err := svc.Agency(ctx, 0, ""); !dashboardIsValidation(err) {
		t.Errorf("Agency no id: %v", err)
	}
	if _, err := svc.Engagements(ctx, domain.DashboardEngagementsFilter{Today: "x"}); !dashboardIsValidation(err) {
		t.Errorf("Engagements bad date: %v", err)
	}
}

func dashboardIsValidation(err error) bool {
	var ve *apierror.ValidationError
	return errors.As(err, &ve)
}
