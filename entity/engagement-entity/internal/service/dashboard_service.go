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
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// DashboardService computes the dashboard aggregates. It has no notion of
// who is asking: the backend asks only for what the caller may see.
type DashboardService interface {
	AllocationsToday(ctx context.Context, f domain.DashboardAllocationsTodayFilter) (domain.DashboardAllocationsToday, error)
	MyWork(ctx context.Context, f domain.DashboardMyWorkFilter) (domain.DashboardMyWork, error)
	Queues(ctx context.Context, f domain.DashboardQueuesFilter) (domain.DashboardQueues, error)
	Travel(ctx context.Context, f domain.DashboardTravelFilter) (domain.DashboardTravel, error)
	Engagements(ctx context.Context, f domain.DashboardEngagementsFilter) (domain.DashboardEngagements, error)
	Agency(ctx context.Context, agencyID int, today string) (domain.DashboardAgency, error)
}

const (
	dashboardDateLayout = "2006-01-02"
	// dashboardUpcomingLimit is the length of the Upcoming Trips lists.
	dashboardUpcomingLimit = 5
	// dashboardStartingSoonLimit is the length of "Starting in the Next 30 Days".
	dashboardStartingSoonLimit = 6
	// dashboardPerDiemLimit is the length of "Per Diems on Open Trips".
	dashboardPerDiemLimit = 5
)

type dashboardService struct {
	repo repository.DashboardRepository
	now  func() time.Time
}

// NewDashboardService constructs a DashboardService. now may be nil (time.Now).
func NewDashboardService(repo repository.DashboardRepository, now func() time.Time) DashboardService {
	if now == nil {
		now = time.Now
	}
	return &dashboardService{repo: repo, now: now}
}

// day parses an optional YYYY-MM-DD; empty means today (UTC, the
// sessions' time zone).
func (s *dashboardService) day(v, name string) (time.Time, error) {
	if v == "" {
		y, m, d := s.now().UTC().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse(dashboardDateLayout, v)
	if err != nil {
		return time.Time{}, &apierror.ValidationError{Msg: name + " must be a date (YYYY-MM-DD)"}
	}
	return t, nil
}

// DashboardWindowFor returns the day boundaries for today.
func DashboardWindowFor(today time.Time) repository.DashboardWindow {
	f := func(t time.Time) string { return t.Format(dashboardDateLayout) }
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	return repository.DashboardWindow{
		Today:       f(today),
		Tomorrow:    f(today.AddDate(0, 0, 1)),
		In7Days:     f(today.AddDate(0, 0, 8)),
		In30Days:    f(today.AddDate(0, 0, 31)),
		MonthStart:  f(month),
		NextMonth:   f(month.AddDate(0, 1, 0)),
		YearStart:   f(month.AddDate(0, -11, 0)),
		BenchEndDay: f(today.AddDate(0, 0, 13)),
	}
}

func (s *dashboardService) AllocationsToday(ctx context.Context, f domain.DashboardAllocationsTodayFilter) (domain.DashboardAllocationsToday, error) {
	if f.Date != "" {
		if _, err := time.Parse(dashboardDateLayout, f.Date); err != nil {
			return domain.DashboardAllocationsToday{}, &apierror.ValidationError{Msg: "date must be a date (YYYY-MM-DD)"}
		}
	}
	if f.BusinessUnitID < 0 {
		return domain.DashboardAllocationsToday{}, &apierror.ValidationError{Msg: "businessUnitId must be a positive integer"}
	}
	return s.repo.AllocationsToday(ctx, f)
}

func (s *dashboardService) MyWork(ctx context.Context, f domain.DashboardMyWorkFilter) (domain.DashboardMyWork, error) {
	email := strings.TrimSpace(f.Email)
	if email == "" || !strings.Contains(email, "@") {
		return domain.DashboardMyWork{}, &apierror.ValidationError{Msg: "email is required"}
	}
	today, err := s.day(f.Today, "today")
	if err != nil {
		return domain.DashboardMyWork{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardMyWork{Trips: []domain.DashboardTrip{}}
	if out.Allocations, err = s.repo.OwnAllocations(ctx, email, w.Today); err != nil {
		return domain.DashboardMyWork{}, err
	}
	if out.TeamAllocation, err = s.repo.TeamAllocationOn(ctx, email, w.Today, w.Tomorrow); err != nil {
		return domain.DashboardMyWork{}, err
	}
	if f.Trips {
		limit := f.TripLimit
		if limit <= 0 || limit > 20 {
			limit = 3
		}
		if out.Trips, out.TripCount, err = s.repo.UpcomingTrips(ctx, repository.DashboardTripFilter{
			Today: w.Today, Email: email, Limit: limit,
		}); err != nil {
			return domain.DashboardMyWork{}, err
		}
	}
	return out, nil
}

func (s *dashboardService) Queues(ctx context.Context, f domain.DashboardQueuesFilter) (domain.DashboardQueues, error) {
	out := domain.DashboardQueues{PendingApproval: map[int]int{}, Clearance: map[string]int{}}
	var err error
	if len(f.PendingApprovalTypes) > 0 {
		if out.PendingApproval, err = s.repo.PendingApprovalCounts(ctx, f.PendingApprovalTypes); err != nil {
			return domain.DashboardQueues{}, err
		}
	}
	if len(f.ClearanceTypes) > 0 {
		if out.Clearance, err = s.repo.ClearanceCounts(ctx, f.ClearanceTypes); err != nil {
			return domain.DashboardQueues{}, err
		}
	}
	if f.TravelRequests {
		if out.NewTravel, err = s.repo.NewTravelRequestCount(ctx); err != nil {
			return domain.DashboardQueues{}, err
		}
	}
	if f.Invoices {
		c, err := s.repo.InvoiceQueueCounts(ctx)
		if err != nil {
			return domain.DashboardQueues{}, err
		}
		out.InvoicesPending, out.InvoicesAcceptedByTravel, out.InvoicesToPush, out.InvoicesFailed =
			c.Pending, c.AcceptedByTravel, c.ToPush, c.Failed
	}
	if f.Payments {
		if out.PaymentsToPush, out.PaymentsFailed, err = s.repo.PaymentQueueCounts(ctx); err != nil {
			return domain.DashboardQueues{}, err
		}
	}
	return out, nil
}

func (s *dashboardService) Travel(ctx context.Context, f domain.DashboardTravelFilter) (domain.DashboardTravel, error) {
	today, err := s.day(f.Today, "today")
	if err != nil {
		return domain.DashboardTravel{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardTravel{From: w.YearStart, To: w.NextMonth, Cost: []domain.DashboardCostLine{}}
	if f.Trips {
		k, err := s.repo.TravelKPIs(ctx, w)
		if err != nil {
			return domain.DashboardTravel{}, err
		}
		upcoming, _, err := s.repo.UpcomingTrips(ctx, repository.DashboardTripFilter{Today: w.Today, Limit: dashboardUpcomingLimit})
		if err != nil {
			return domain.DashboardTravel{}, err
		}
		cells, err := s.repo.TravelCells(ctx, w)
		if err != nil {
			return domain.DashboardTravel{}, err
		}
		trips := &domain.DashboardTravelTrips{
			InProgress: k.InProgress, StartingIn30Days: k.StartingIn30Days,
			CompletedThisMonth: k.CompletedThisMonth, Last12Months: k.Last12Months,
			UpcomingTotal: k.Upcoming, Upcoming: upcoming,
		}
		byMonth, byCountry, byAgency := map[string]int{}, map[string]int{}, map[string]int{}
		for _, c := range cells {
			byMonth[c.Month] += c.Count
			byCountry[c.Country] += c.Count
			if c.Agency != nil && *c.Agency != "" {
				byAgency[*c.Agency] += c.Count
			}
		}
		trips.ByMonth = dashboardSortedCounts(byMonth, false)
		trips.ByCountry = dashboardSortedCounts(byCountry, true)
		trips.ByAgency = dashboardSortedCounts(byAgency, true)
		out.Trips = trips
	}
	for _, part := range []struct {
		on bool
		fn func(context.Context, repository.DashboardWindow) ([]domain.DashboardCostLine, error)
	}{
		{f.CostInvoices, s.repo.InvoiceCost},
		{f.CostPayments, s.repo.PaymentCost},
		{f.CostCards, s.repo.CardCost},
		{f.CostClaims, s.repo.ClaimCost},
	} {
		if !part.on {
			continue
		}
		lines, err := part.fn(ctx, w)
		if errors.Is(err, repository.ErrDashboardNoExpenseDB) {
			return domain.DashboardTravel{}, &apierror.ServiceUnavailableError{Msg: err.Error()}
		}
		if err != nil {
			return domain.DashboardTravel{}, err
		}
		out.Cost = append(out.Cost, lines...)
	}
	return out, nil
}

func (s *dashboardService) Engagements(ctx context.Context, f domain.DashboardEngagementsFilter) (domain.DashboardEngagements, error) {
	today, err := s.day(f.Today, "today")
	if err != nil {
		return domain.DashboardEngagements{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardEngagements{StartingSoon: []domain.DashboardEngagement{}}
	if len(f.EngagementTypes) > 0 {
		if out.StartingSoon, out.StartingSoonTotal, err = s.repo.EngagementsStarting(ctx, f.EngagementTypes,
			w.Today, w.In30Days, dashboardStartingSoonLimit); err != nil {
			return domain.DashboardEngagements{}, err
		}
	}
	if f.Bench {
		if out.Bench, err = s.repo.Bench(ctx, w.Today, w.BenchEndDay); err != nil {
			return domain.DashboardEngagements{}, err
		}
	}
	if f.Trend {
		byMonth, err := s.repo.AllocationsByStartMonth(ctx, w.YearStart, w.Tomorrow)
		if err != nil {
			return domain.DashboardEngagements{}, err
		}
		byBU, err := s.repo.AllocationsByBusinessUnit(ctx, w.YearStart, w.Tomorrow)
		if err != nil {
			return domain.DashboardEngagements{}, err
		}
		out.Trend = &domain.DashboardTrend{ByMonth: byMonth, ByBusinessUnit: byBU}
	}
	return out, nil
}

func (s *dashboardService) Agency(ctx context.Context, agencyID int, todayStr string) (domain.DashboardAgency, error) {
	if agencyID <= 0 {
		return domain.DashboardAgency{}, &apierror.ValidationError{Msg: "agencyId must be a positive integer"}
	}
	today, err := s.day(todayStr, "today")
	if err != nil {
		return domain.DashboardAgency{}, err
	}
	w := DashboardWindowFor(today)
	k, err := s.repo.AgencyKPIs(ctx, agencyID, w)
	if err != nil {
		return domain.DashboardAgency{}, err
	}
	out := domain.DashboardAgency{
		InProgress: k.InProgress, StartingIn7Days: k.StartingIn7Days, StartingIn30Days: k.StartingIn30Days,
		CompletedWithoutInvoice: k.CompletedWithoutInvoice, UpcomingTotal: k.Upcoming,
	}
	if out.Upcoming, _, err = s.repo.UpcomingTrips(ctx, repository.DashboardTripFilter{
		Today: w.Today, AgencyID: agencyID, Limit: dashboardUpcomingLimit,
	}); err != nil {
		return domain.DashboardAgency{}, err
	}
	if out.InvoicesByStatus, err = s.repo.AgencyInvoicesByStatus(ctx, agencyID); err != nil {
		return domain.DashboardAgency{}, err
	}
	if out.TripsByMonth, err = s.repo.AgencyTripsByMonth(ctx, agencyID, w); err != nil {
		return domain.DashboardAgency{}, err
	}
	if out.PerDiemsOnOpenTrips, err = s.repo.AgencyPerDiems(ctx, agencyID, dashboardPerDiemLimit); err != nil {
		return domain.DashboardAgency{}, err
	}
	return out, nil
}

// dashboardSortedCounts turns a label → count map into a list, by count
// descending (byCount) or by label ascending.
func dashboardSortedCounts(m map[string]int, byCount bool) []domain.DashboardCount {
	out := make([]domain.DashboardCount, 0, len(m))
	for k, v := range m {
		out = append(out, domain.DashboardCount{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if byCount && out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}
