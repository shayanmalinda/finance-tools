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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

const jobCostTrip = "TRV-2026-10-0001"

func jobCostFixture(lines []engagemententity.DashboardCostEntry) (DashboardService, *fakeDashboardEntity) {
	svc, f := costService(lines, &fakeRates{rates: map[string]float64{"LKR": 300}})
	agency := 7
	f.trips = map[string]engagemententity.TravelRequestRecord{
		jobCostTrip: {JobNumber: jobCostTrip, TravellerEmail: "jane@wso2.com", StartDate: "2026-10-10", EndDate: "2026-10-20", TravelAgentID: &agency},
	}
	return svc, f
}

// jobCostFullPerms see every request and all sections: View Per Diems (28),
// View Invoices (30) and View Cards (45).
func jobCostFullPerms() domain.Perms {
	return dashboardPerms("ops@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, []int{2, 28, 30, 45}, []int{2, 28, 30, 45})
}

func jobCostLines() []engagemententity.DashboardCostEntry {
	return []engagemententity.DashboardCostEntry{
		// 100 LKR at 300 is 0.33 USD a line: three lines are 0.99, not 1.00.
		entry("INV-1", "invoice", jobCostTrip, "2026-10-14", "LKR", 100),
		entry("INV-2", "invoice", jobCostTrip, "2026-10-15", "LKR", 100),
		entry("INV-3", "invoice", jobCostTrip, "2026-10-16", "LKR", 100),
		entry("PD-1", "perDiem", jobCostTrip, "2026-10-17", "USD", 200),
		entry("CC-1", "card", jobCostTrip, "2026-10-18", "EUR", 80),          // no rate
		entry("EC-1", "claim", "TRV-2026-10-0002", "2026-10-19", "USD", 999), // another job
	}
}

func TestJobCostSummary_TotalIsTheSumOfRoundedLines(t *testing.T) {
	svc, _ := jobCostFixture(jobCostLines())
	p := jobCostFullPerms()
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalUSD != 200.99 {
		t.Errorf("total = %v, want 200.99 (3 x 0.33 + 200)", got.TotalUSD)
	}
	if len(got.BySource) != 4 || got.BySource[0].USD != 0.99 || got.BySource[0].Lines != 3 || got.BySource[1].USD != 200 || got.BySource[3].Lines != 0 {
		t.Errorf("bySource = %+v", got.BySource)
	}
	if len(got.NotConverted) != 1 || got.NotConverted[0] != (domain.CostNotConverted{Source: "card", Currency: "EUR", Amount: 80, Lines: 1}) {
		t.Errorf("notConverted = %+v", got.NotConverted)
	}
	if got.Incomplete || got.DataAsOf == "" {
		t.Errorf("incomplete = %v, dataAsOf = %q", got.Incomplete, got.DataAsOf)
	}
}

// The number is the one Travel Costs shows for the job over a range that
// holds all its lines.
func TestJobCostSummary_EqualsTravelCostsFilteredToTheJob(t *testing.T) {
	svc, _ := jobCostFixture(jobCostLines())
	p := tsaPerms()
	job, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	q := CostQuery{From: "2026-01-01", To: "2026-11-05", Filter: CostFilter{JobNumbers: []string{jobCostTrip}}}
	costs := mustSummary(t, svc, p, q)
	if job.TotalUSD != costs.Kpis.Total.Value {
		t.Errorf("job total %v != Travel Costs total %v", job.TotalUSD, costs.Kpis.Total.Value)
	}
	for i, b := range costs.BySource {
		if job.BySource[i].Key != b.Key || job.BySource[i].USD != b.Value {
			t.Errorf("source %s: %+v != %+v", b.Key, job.BySource[i], b)
		}
	}
	if len(costs.NotConverted) != len(job.NotConverted) || job.NotConverted[0] != costs.NotConverted[0] {
		t.Errorf("notConverted: %+v != %+v", job.NotConverted, costs.NotConverted)
	}
}

func TestJobCostSummary_AsksForTheJobAndWindowsOfAtMost731Days(t *testing.T) {
	svc, f := jobCostFixture(jobCostLines())
	p := jobCostFullPerms()
	if _, err := svc.JobCostSummary(context.Background(), p, jobCostTrip); err != nil {
		t.Fatal(err)
	}
	if f.costCalls != 1 || f.costRanges[0] != "2026-07-12|2026-11-05" {
		t.Errorf("calls = %d, ranges = %v", f.costCalls, f.costRanges)
	}
	// A trip from long ago is read in several windows.
	f.trips[jobCostTrip] = engagemententity.TravelRequestRecord{JobNumber: jobCostTrip, TravellerEmail: "jane@wso2.com", StartDate: "2023-01-10", EndDate: "2023-01-20"}
	f.costCalls, f.costRanges = 0, nil
	if _, err := svc.JobCostSummary(context.Background(), p, jobCostTrip); err != nil {
		t.Fatal(err)
	}
	if f.costCalls < 2 {
		t.Errorf("calls = %d, want several windows", f.costCalls)
	}
}

func TestJobCostSummary_AgentGetsNoSourceSplit(t *testing.T) {
	svc, _ := jobCostFixture(jobCostLines())
	p := dashboardPerms("agent@x.com", nil, nil, []int{18}, []int{18})
	p.Agency = &domain.TravelAgency{ID: 7, Name: "Classic"}
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalUSD != 200.99 || got.BySource != nil || len(got.NotConverted) != 0 {
		t.Errorf("agent summary = %+v", got)
	}
}

func TestJobCostSummary_AccessFollowsTheTravelRequest(t *testing.T) {
	svc, _ := jobCostFixture(jobCostLines())
	ctx := context.Background()
	other := dashboardPerms("agent@y.com", nil, nil, []int{18}, []int{18})
	other.Agency = &domain.TravelAgency{ID: 9, Name: "Other"}
	var fe *apierror.ForbiddenError
	if _, err := svc.JobCostSummary(ctx, other, jobCostTrip); !errors.As(err, &fe) {
		t.Errorf("another agency's agent: err = %v, want 403", err)
	}
	stranger := dashboardPerms("sam@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel)
	if _, err := svc.JobCostSummary(ctx, stranger, jobCostTrip); !errors.As(err, &fe) {
		t.Errorf("another employee: err = %v, want 403", err)
	}
	traveller := dashboardPerms("Jane@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel)
	traveller.Email = "jane@wso2.com"
	got, err := svc.JobCostSummary(ctx, traveller, jobCostTrip)
	if err != nil || got.BySource == nil {
		t.Errorf("the traveller: %+v, %v", got, err)
	}
	var nf *apierror.NotFoundError
	if _, err := svc.JobCostSummary(ctx, traveller, "TRV-2026-10-0099"); !errors.As(err, &nf) {
		t.Errorf("unknown job: err = %v, want 404", err)
	}
	var bad *apierror.ValidationError
	if _, err := svc.JobCostSummary(ctx, traveller, "nope"); !errors.As(err, &bad) {
		t.Errorf("bad job number: err = %v, want 400", err)
	}
}

func TestJobCostSummary_FailedSourceIsIncomplete(t *testing.T) {
	svc, f := jobCostFixture(jobCostLines())
	f.costResp.Unavailable = []string{"card"}
	p := jobCostFullPerms()
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil || !got.Incomplete || len(got.FailedSources) != 1 || got.FailedSources[0] != "card" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestJobCostSummary_SplitFollowsTheCallersSectionRights(t *testing.T) {
	svc, _ := jobCostFixture(append(jobCostLines(), entry("INV-9", "invoice", jobCostTrip, "2026-10-19", "GBP", 10)))
	// View Invoices only: the invoice split, and nothing about the card line
	// that could not be converted.
	p := dashboardPerms("inv@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, []int{2, 30}, []int{2, 30})
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalUSD != 200.99 {
		t.Errorf("the total is for everyone: %v", got.TotalUSD)
	}
	if len(got.BySource) != 1 || got.BySource[0].Key != "invoice" || got.BySource[0].Lines != 4 {
		t.Errorf("bySource = %+v, want the invoice split only", got.BySource)
	}
	if len(got.NotConverted) != 1 || got.NotConverted[0].Source != "invoice" || got.NotConverted[0].Currency != "GBP" {
		t.Errorf("notConverted = %+v, want the invoice GBP line only", got.NotConverted)
	}
	if !got.HasUnconvertedHidden {
		t.Error("hasUnconvertedHidden = false, the card line is hidden and unconverted")
	}
}

func TestJobCostSummary_AgentSeesNoHiddenSourceDetail(t *testing.T) {
	svc, f := jobCostFixture(jobCostLines())
	f.costResp.Unavailable = []string{"card", "claim"}
	p := dashboardPerms("agent@x.com", nil, nil, []int{18}, []int{18})
	p.Agency = &domain.TravelAgency{ID: 7, Name: "Classic"}
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalUSD != 200.99 || got.BySource != nil {
		t.Errorf("agent summary = %+v", got)
	}
	if len(got.NotConverted) != 0 || !got.HasUnconvertedHidden {
		t.Errorf("notConverted = %+v, hidden = %v", got.NotConverted, got.HasUnconvertedHidden)
	}
	if len(got.FailedSources) != 0 || !got.Incomplete {
		t.Errorf("failed = %v, incomplete = %v, want no names and incomplete", got.FailedSources, got.Incomplete)
	}
}

// The traveller on their own trip (View Own Per Diems 37 and Invoices 38)
// gets those sections, not the Expenses ones.
func TestJobCostSummary_TravellerGetsWhatTheirRightsAllow(t *testing.T) {
	svc, _ := jobCostFixture(jobCostLines())
	p := dashboardPerms("jane@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel)
	got, err := svc.JobCostSummary(context.Background(), p, jobCostTrip)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, b := range got.BySource {
		keys = append(keys, b.Key)
	}
	if len(keys) != 2 || keys[0] != "invoice" || keys[1] != "perDiem" {
		t.Errorf("sources = %v, want invoice and perDiem", keys)
	}
	if len(got.NotConverted) != 0 || !got.HasUnconvertedHidden {
		t.Errorf("the unconverted card line must be hidden: %+v, %v", got.NotConverted, got.HasUnconvertedHidden)
	}
}
