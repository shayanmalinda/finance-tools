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
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

// dashboardMaxRange is the longest cost line range, in days (two years).
const dashboardMaxRange = 731

// dashboardPaidTypes are the engagement type names that are paid work.
var dashboardPaidTypes = map[string]bool{"paid": true, "paid pre sale": true, "paid post sale": true}

// dashboardPaidOf maps an engagement type name to paid, nonPaid or unknown.
func dashboardPaidOf(typeName string) string {
	name := strings.ToLower(strings.TrimSpace(typeName))
	switch {
	case name == "":
		return domain.DashboardUnknown
	case dashboardPaidTypes[name]:
		return domain.DashboardPaid
	}
	return domain.DashboardNonPaid
}

var dashboardCostSources = []string{domain.DashboardCostInvoice, domain.DashboardCostPerDiem, domain.DashboardCostCard, domain.DashboardCostClaim}

// CostLines reads the cost lines of every asked source in the range (any date
// when jobNumbers are given without a range), joins
// the trips and engagements in Go (the schemas cannot be joined in SQL) and
// returns them in their original amounts. A source that cannot be read is in
// Unavailable (card and claim when expense_app is down); invoices and per
// diem payments fail the call.
func (s *dashboardService) CostLines(ctx context.Context, f domain.DashboardCostLinesFilter) (domain.DashboardCostEntries, error) {
	// The range is required, except when jobs are asked for by number: then
	// from and to may both be left out to read every date of those jobs.
	var err error
	if f.From != "" || f.To != "" || len(f.JobNumbers) == 0 {
		var from, to time.Time
		if from, err = time.Parse(dashboardDateLayout, f.From); err != nil {
			return domain.DashboardCostEntries{}, &apierror.ValidationError{Msg: "from must be a date (YYYY-MM-DD)"}
		}
		if to, err = time.Parse(dashboardDateLayout, f.To); err != nil {
			return domain.DashboardCostEntries{}, &apierror.ValidationError{Msg: "to must be a date (YYYY-MM-DD)"}
		}
		if to.Before(from) || to.Sub(from) > dashboardMaxRange*24*time.Hour {
			return domain.DashboardCostEntries{}, &apierror.ValidationError{Msg: "from must not be after to, and the range is at most 731 days"}
		}
	}
	want := map[string]bool{}
	for _, src := range f.Sources {
		ok := false
		for _, k := range dashboardCostSources {
			ok = ok || k == src
		}
		if !ok {
			return domain.DashboardCostEntries{}, &apierror.ValidationError{Msg: "source must be invoice, perDiem, card or claim"}
		}
		want[src] = true
	}
	if len(want) == 0 {
		for _, k := range dashboardCostSources {
			want[k] = true
		}
	}
	limit := f.Limit
	if limit <= 0 || limit > domain.DashboardCostLinesMax {
		limit = domain.DashboardCostLinesMax
	}
	f.Limit = limit

	type result struct {
		rows []repository.DashboardCostRow
		err  error
	}
	res := map[string]*result{}
	var wg sync.WaitGroup
	run := func(src string, fn func(context.Context, domain.DashboardCostLinesFilter) ([]repository.DashboardCostRow, error)) {
		if !want[src] {
			return
		}
		r := &result{}
		res[src] = r
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.rows, r.err = fn(ctx, f)
		}()
	}
	// Invoices and per diem payments (travelapp) run in parallel; card then
	// claim (the expense_app pool has 5 connections) one after the other.
	run(domain.DashboardCostInvoice, s.repo.InvoiceLines)
	run(domain.DashboardCostPerDiem, s.repo.PaymentLines)
	if want[domain.DashboardCostCard] || want[domain.DashboardCostClaim] {
		rc, rl := &result{}, &result{}
		res[domain.DashboardCostCard], res[domain.DashboardCostClaim] = rc, rl
		wg.Add(1)
		go func() {
			defer wg.Done()
			if want[domain.DashboardCostCard] {
				rc.rows, rc.err = s.repo.CardLines(ctx, f)
			}
			if want[domain.DashboardCostClaim] {
				rl.rows, rl.err = s.repo.ClaimLines(ctx, f)
			}
		}()
	}
	wg.Wait()

	out := domain.DashboardCostEntries{Items: []domain.DashboardCostEntry{}, Truncated: []string{}, Unavailable: []string{}}
	var rows []repository.DashboardCostRow
	for _, src := range dashboardCostSources {
		r := res[src]
		if r == nil {
			continue
		}
		if r.err != nil {
			if src == domain.DashboardCostCard || src == domain.DashboardCostClaim {
				slog.WarnContext(ctx, "dashboard cost lines: source unavailable", "source", src, "err", r.err)
				out.Unavailable = append(out.Unavailable, src)
				continue
			}
			return domain.DashboardCostEntries{}, r.err
		}
		if len(r.rows) > limit {
			r.rows = r.rows[:limit]
			out.Truncated = append(out.Truncated, src)
		}
		rows = append(rows, r.rows...)
	}

	// Trips of card and claim lines, matched on the upper-cased job number.
	trips := map[string]repository.DashboardTripRef{}
	var jobs []string
	seen := map[string]bool{}
	for _, r := range rows {
		if r.Source != domain.DashboardCostCard && r.Source != domain.DashboardCostClaim || r.JobNumber == nil {
			continue
		}
		if k := strings.ToUpper(*r.JobNumber); !seen[k] {
			seen[k] = true
			jobs = append(jobs, *r.JobNumber)
		}
	}
	if len(jobs) > 0 {
		if trips, err = s.repo.TripsByJob(ctx, jobs); err != nil {
			return domain.DashboardCostEntries{}, err
		}
	}
	tripOf := func(r repository.DashboardCostRow) repository.DashboardTripRef {
		if r.Source == domain.DashboardCostInvoice || r.Source == domain.DashboardCostPerDiem {
			return r.Trip
		}
		if r.JobNumber != nil {
			return trips[strings.ToUpper(*r.JobNumber)]
		}
		return repository.DashboardTripRef{}
	}

	// Engagements of the trips' allocations.
	var allocs []string
	seen = map[string]bool{}
	for _, r := range rows {
		if a := tripOf(r).AllocationID; a != nil && !seen[*a] {
			seen[*a] = true
			allocs = append(allocs, *a)
		}
	}
	engs := map[string]repository.DashboardCostEngagement{}
	if len(allocs) > 0 {
		if engs, err = s.repo.CostEngagements(ctx, allocs); err != nil {
			return domain.DashboardCostEntries{}, err
		}
	}

	unmatched := 0
	for _, r := range rows {
		t := tripOf(r)
		e := domain.DashboardCostEntry{
			ID: r.ID, Source: r.Source, Reference: r.Reference, JobNumber: r.JobNumber, Date: r.Date,
			OriginalAmount: r.Amount, OriginalCurrency: r.Currency, CCAmount: r.CCAmount, CCCurrency: r.CCCurrency,
			CostType: r.CostType, SourceType: r.SourceType, TravelAgentID: t.AgentID, TravelAgentName: t.AgentName,
		}
		isExpense := r.Source == domain.DashboardCostCard || r.Source == domain.DashboardCostClaim
		if isExpense {
			e.TravellerEmail = r.EmployeeEmail
			if t.JobNumber == nil && r.JobNumber != nil {
				unmatched++
			}
		} else {
			e.TravellerEmail = t.TravellerEmail
		}
		e.Country = t.Country
		if e.Country == nil && isExpense {
			e.Country = r.TravelCountry
		}
		e.Customer, e.Paid = t.CustomerName, dashboardPaidOf("")
		typeName := ""
		if t.EngagementType != nil {
			typeName = *t.EngagementType
		}
		if t.AllocationID != nil {
			if eng, ok := engs[*t.AllocationID]; ok {
				id := eng.EngagementID
				e.EngagementID, e.AllocationType = &id, eng.AllocationType
				if eng.CustomerName != nil && strings.TrimSpace(*eng.CustomerName) != "" {
					e.Customer = eng.CustomerName
				}
				if eng.TypeName != nil && strings.TrimSpace(*eng.TypeName) != "" {
					typeName = *eng.TypeName
				}
			}
		}
		e.Paid = dashboardPaidOf(typeName)
		out.Items = append(out.Items, e)
	}
	if unmatched > 0 {
		slog.DebugContext(ctx, "dashboard cost lines: card and claim lines with a job number that matched no trip", "count", unmatched)
	}
	return out, nil
}

// ── Role stats ──────────────────────────────────────────────────────────────

func (s *dashboardService) TravelTeam(ctx context.Context, todayStr string) (domain.DashboardTravelTeam, error) {
	today, err := s.day(todayStr, "today")
	if err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardTravelTeam{}
	if out.AwayTravellers, out.AwayCountries, err = s.repo.TravelTeamAway(ctx, w); err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	if out.EndedNotCompleted, err = s.repo.TravelEndedNotCompleted(ctx, w); err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	byAge, total, oldest, err := s.repo.POQueueByAge(ctx, w.Today)
	if err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	out.POByAge, out.POCount, out.POOldestDays = byAge[:], total, oldest
	if out.FrequentTravellers, err = s.repo.FrequentTravellers(ctx, w); err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	if out.AgentWorkload, err = s.repo.AgentWorkload(ctx, w); err != nil {
		return domain.DashboardTravelTeam{}, err
	}
	return out, nil
}

// The lengths of the Engagement Manager and Account Manager lists and tops.
const (
	dashboardManagerListLimit = 6
	dashboardManagerTopLimit  = 8
)

func (s *dashboardService) Manager(ctx context.Context, f domain.DashboardManagerFilter) (domain.DashboardManager, error) {
	today, err := s.day(f.Today, "today")
	if err != nil {
		return domain.DashboardManager{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardManager{
		WithoutConsultants: []domain.DashboardEngagement{}, StartingIn14Days: []domain.DashboardStartingAllocation{},
		MostAllocated: []domain.DashboardCount{}, Top: []domain.DashboardCount{}, TopBy: "customer",
	}
	if len(f.Types) == 0 {
		return out, nil
	}
	if out.WithoutConsultants, out.WithoutConsultantsTotal, err = s.repo.EngagementsStartingWithout(ctx, f.Types,
		w.Today, w.In30Days, dashboardManagerListLimit); err != nil {
		return domain.DashboardManager{}, err
	}
	if out.StartingIn14Days, out.StartingIn14DaysTotal, err = s.repo.AllocationsStarting(ctx, f.Types,
		w.Tomorrow, w.In14Days, dashboardManagerListLimit); err != nil {
		return domain.DashboardManager{}, err
	}
	if out.MostAllocated, err = s.repo.MostAllocated(ctx, f.Types, w.Minus90, w.Tomorrow, dashboardManagerTopLimit); err != nil {
		return domain.DashboardManager{}, err
	}
	if out.TopBy, out.Top, err = s.repo.TopCustomers(ctx, f.Types, w.YearStart, w.Tomorrow, dashboardManagerTopLimit); err != nil {
		return domain.DashboardManager{}, err
	}
	return out, nil
}

func (s *dashboardService) AccountManager(ctx context.Context, email, todayStr string) (domain.DashboardAccountManager, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return domain.DashboardAccountManager{}, &apierror.ValidationError{Msg: "email is required"}
	}
	today, err := s.day(todayStr, "today")
	if err != nil {
		return domain.DashboardAccountManager{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardAccountManager{}
	if out.StartedWithoutConsultants, out.StartedWithoutConsultantsTotal, err = s.repo.OwnEngagementsRunningWithout(ctx, email, w.Today, dashboardManagerListLimit); err != nil {
		return domain.DashboardAccountManager{}, err
	}
	if out.ByStatus, err = s.repo.OwnEngagementsByStatus(ctx, email, w.YearStart); err != nil {
		return domain.DashboardAccountManager{}, err
	}
	if out.ByMonth, err = s.repo.OwnEngagementsByMonth(ctx, email, w.MonthStart, w.MonthPlus6); err != nil {
		return domain.DashboardAccountManager{}, err
	}
	return out, nil
}

func (s *dashboardService) Events(ctx context.Context, todayStr string) (domain.DashboardEvents, error) {
	today, err := s.day(todayStr, "today")
	if err != nil {
		return domain.DashboardEvents{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardEvents{}
	if out.ByMonth, err = s.repo.EventsByMonth(ctx, w.YearStart, w.NextMonth); err != nil {
		return domain.DashboardEvents{}, err
	}
	if out.ByRegion, err = s.repo.EventsByRegion(ctx, w.YearStart, w.NextMonth, 8); err != nil {
		return domain.DashboardEvents{}, err
	}
	if out.FundingByBusinessUnit, err = s.repo.EventsByBusinessUnit(ctx, w.YearStart, w.NextMonth, 6); err != nil {
		return domain.DashboardEvents{}, err
	}
	since := today.AddDate(0, 0, -90).Format(dashboardDateLayout)
	if out.CampaignCreators, out.CampaignAvailable, err = s.repo.EventCampaignCoverage(ctx, since); err != nil {
		return domain.DashboardEvents{}, err
	}
	if out.CampaignCreators == nil {
		out.CampaignCreators = []domain.DashboardCampaignCreator{}
	}
	return out, nil
}

// dashboardMyVisaTripLimit is the most trips the Me card lists.
const dashboardMyVisaTripLimit = 20

func (s *dashboardService) MyVisas(ctx context.Context, email, todayStr string) (domain.DashboardMyVisas, error) {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return domain.DashboardMyVisas{}, &apierror.ValidationError{Msg: "email is required"}
	}
	today, err := s.day(todayStr, "today")
	if err != nil {
		return domain.DashboardMyVisas{}, err
	}
	w := DashboardWindowFor(today)
	out := domain.DashboardMyVisas{}
	if out.Trips, err = s.repo.OwnTripVisas(ctx, email, w.Today, dashboardMyVisaTripLimit); err != nil {
		return domain.DashboardMyVisas{}, err
	}
	if len(out.Trips) == 0 {
		out.Visas = []domain.DashboardOwnVisa{}
		return out, nil
	}
	if out.Visas, err = s.repo.OwnActiveVisas(ctx, email, w.Today); err != nil {
		return domain.DashboardMyVisas{}, err
	}
	return out, nil
}
