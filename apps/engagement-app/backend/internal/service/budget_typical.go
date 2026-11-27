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
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
)

// Typical cost of a trip, the estimate behind "committed" and the preview's
// warning: the median USD cost of the trips of the last 24 months.
const (
	budgetTypicalMonths = 24
	budgetTypicalTTL    = 6 * time.Hour
	// budgetTypicalMinTrips is how many past trips a country needs to have a
	// typical cost of its own.
	budgetTypicalMinTrips = 5
)

// Bases of a typical cost.
const (
	budgetBasisCountry = "country"
	budgetBasisGlobal  = "global"
	budgetBasisNone    = "none"
)

// budgetTypical holds the medians. pending is set when some cost could not be
// converted, so the result is not kept.
type budgetTypical struct {
	country map[string]budgetStat
	global  budgetStat
	pending bool
}

// budgetStat is a median over n trips.
type budgetStat struct {
	median float64
	n      int
}

// forCountry is the typical cost of a trip to country: its own median with 5
// or more past trips, else the median of all trips, else none.
func (t *budgetTypical) forCountry(country string) (usd float64, basis string, n int) {
	if t == nil {
		return 0, budgetBasisNone, 0
	}
	if st, ok := t.country[country]; ok && country != "" && st.n >= budgetTypicalMinTrips {
		return st.median, budgetBasisCountry, st.n
	}
	if t.global.n > 0 {
		return t.global.median, budgetBasisGlobal, t.global.n
	}
	return 0, budgetBasisNone, 0
}

// typical returns the typical costs, kept for 6 hours unless some rate was
// missing.
func (s *BudgetService) typical(ctx context.Context) (*budgetTypical, error) {
	return s.typicals.get(ctx, "typical", func() (*budgetTypical, error) { return s.buildTypical(ctx) })
}

func (s *BudgetService) buildTypical(ctx context.Context) (*budgetTypical, error) {
	now := s.now().UTC()
	from := time.Date(now.Year(), now.Month()-budgetTypicalMonths, 1, 0, 0, 0, 0, time.UTC)
	ledger, err := s.entity.BudgetLedger(ctx, from.Format(dashboardDateLayout), now.Format(dashboardDateLayout), false)
	if err != nil {
		return nil, err
	}
	if ledger.Truncated {
		slog.WarnContext(ctx, "budget typical cost: the ledger was truncated")
	}
	// Past costs use the month-end rule of the trip's month, which keeps the
	// rate lookups to about one per currency and month.
	date := func(start string) string {
		if len(start) < 7 {
			return now.Format(dashboardDateLayout)
		}
		return dashboardRateDate(start[:7], now)
	}
	trips := make(map[string]engagemententity.BudgetLedgerTrip, len(ledger.Trips))
	for _, t := range ledger.Trips {
		trips[t.JobNumber] = t
	}
	var pairs []ratePair
	for _, c := range ledger.Costs {
		if t, ok := trips[c.JobNumber]; ok {
			pairs = append(pairs, ratePair{budgetCurrency(c.Currency), date(t.StartDate)})
		}
	}
	rs := s.rates.resolve(ctx, pairs)

	total := map[string]float64{}
	broken := map[string]bool{}
	for _, c := range ledger.Costs {
		t, ok := trips[c.JobNumber]
		if !ok {
			continue
		}
		usd, ok := rs.usdOn(c.Currency, c.Amount, date(t.StartDate))
		if !ok {
			broken[c.JobNumber] = true
			continue
		}
		total[c.JobNumber] += usd
	}
	out := &budgetTypical{country: map[string]budgetStat{}}
	byCountry := map[string][]float64{}
	var all []float64
	for job, v := range total {
		t := trips[job]
		if t.Status == "Cancelled" || broken[job] || v <= 0 {
			continue
		}
		byCountry[budgetCountry(t.Country)] = append(byCountry[budgetCountry(t.Country)], v)
		all = append(all, v)
	}
	// A trip with one unconverted cost is left out whole: a partial total
	// would pull the median down.
	out.pending = len(broken) > 0
	for c, v := range byCountry {
		out.country[c] = budgetStat{median: budgetMedian(v), n: len(v)}
	}
	out.global = budgetStat{median: budgetMedian(all), n: len(all)}
	if out.pending {
		s.warmRates(ctx, missingPairs(pairs, rs))
	}
	return out, nil
}

// budgetCurrency is a currency code as the rate cache keys it.
func budgetCurrency(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// missingPairs are the non-USD pairs rs has no rate for, once each.
func missingPairs(pairs []ratePair, rs rateSet) []ratePair {
	var out []ratePair
	seen := map[ratePair]bool{}
	for _, p := range pairs {
		if p.Currency == "" || p.Currency == "USD" || seen[p] {
			continue
		}
		seen[p] = true
		if _, ok := rs[p]; !ok {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Currency != out[j].Currency {
			return out[i].Currency < out[j].Currency
		}
		return out[i].Date < out[j].Date
	})
	return out
}
