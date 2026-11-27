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
	"sort"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// The pure logic of Travel Budgets: line names and the suggestion of a line
// for a trip. No I/O here, so every rule is table tested.

// budgetLineName is a line's name everywhere: "Sales: EU", "R&D: IAM Product"
// and "G&A: Board meeting travel", the bare sub-function for the rest.
func budgetLineName(l engagemententity.BudgetLine) string {
	switch strings.TrimSpace(l.FunctionName) {
	case "Sales":
		return "Sales: " + l.SubFunction
	case "Research and Development":
		return "R&D: " + l.SubFunction
	case "General Administration":
		return "G&A: " + l.SubFunction
	}
	return l.SubFunction
}

// budgetNorm trims and lower-cases a rule value for matching.
func budgetNorm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// budgetRule is the rule of an allocation type (kind NONE when it has none).
type budgetRule struct {
	Kind     string
	LineCode string
}

// budgetFunding is one funding source of the engagement.
type budgetFunding struct {
	Region, SubRegion string
	Percentage        float64
}

// budgetHR is the traveller's place in the organisation.
type budgetHR struct{ BusinessUnit, Department, Team string }

// budgetSuggestIn is everything suggest reads.
type budgetSuggestIn struct {
	// EngagementLine is the line code set on the engagement ("" when none).
	EngagementLine string
	Rule           budgetRule
	Funding        []budgetFunding
	// HR is nil when the traveller is unknown or HR failed.
	HR          *budgetHR
	RegionRules []engagemententity.BudgetRegionRule
	TeamRules   []engagemententity.BudgetTeamRule
	// ActiveCodes are the codes that have an Active line in the trip's year.
	ActiveCodes map[string]bool
}

// budgetSuggest picks the line code for a trip and says where it came from.
// Order: the engagement's own line, then the allocation type's rule (a fixed
// line, the engagement's region, or the traveller's team). A code with no
// Active line in the trip's year, or nothing that matches, is NONE.
func budgetSuggest(in budgetSuggestIn) (code, source string) {
	switch {
	case in.EngagementLine != "":
		code, source = in.EngagementLine, domain.BudgetSourceEngagement
	case in.Rule.Kind == domain.BudgetRuleLine:
		code, source = in.Rule.LineCode, domain.BudgetSourceAllocationType
	case in.Rule.Kind == domain.BudgetRuleRegion:
		code, source = budgetRegionLine(in.Funding, in.RegionRules), domain.BudgetSourceRegion
	case in.Rule.Kind == domain.BudgetRuleTeam:
		code, source = budgetTeamLine(in.HR, in.TeamRules), domain.BudgetSourceTeam
	}
	if code == "" || !in.ActiveCodes[code] {
		return "", domain.BudgetSourceNone
	}
	return code, source
}

// budgetRegionLine reads the region rules for the funding source with the
// highest percentage (the first on a tie).
func budgetRegionLine(funding []budgetFunding, rules []engagemententity.BudgetRegionRule) string {
	if len(funding) == 0 {
		return ""
	}
	top := funding[0]
	for _, f := range funding[1:] {
		if f.Percentage > top.Percentage {
			top = f
		}
	}
	region, sub := budgetNorm(top.Region), budgetNorm(top.SubRegion)
	if r, ok := budgetRegionMatch(rules, region, sub); ok {
		return r
	}
	// The sub-region names are also region rows, so a match works whichever
	// column holds the name.
	if sub != "" {
		if r, ok := budgetRegionMatch(rules, sub, ""); ok {
			return r
		}
	}
	return ""
}

// budgetRegionMatch finds the first exact match on region and sub-region,
// then the first on region alone (a rule with no sub-region), then the first
// prefix rule (a region ending in *). ok is true on a match, even one that
// maps to no line.
func budgetRegionMatch(rules []engagemententity.BudgetRegionRule, region, sub string) (code string, ok bool) {
	line := func(r engagemententity.BudgetRegionRule) string {
		if r.LineCode == nil {
			return ""
		}
		return *r.LineCode
	}
	if region == "" {
		return "", false
	}
	if sub != "" {
		for _, r := range rules {
			if r.SubRegion != nil && budgetNorm(*r.SubRegion) == sub && budgetNorm(r.Region) == region {
				return line(r), true
			}
		}
	}
	for _, r := range rules {
		if r.SubRegion == nil && budgetNorm(r.Region) == region {
			return line(r), true
		}
	}
	for _, r := range rules {
		prefix, star := strings.CutSuffix(budgetNorm(r.Region), "*")
		if !star || prefix == "" || !strings.HasPrefix(region, prefix) {
			continue
		}
		if r.SubRegion == nil || (sub != "" && budgetNorm(*r.SubRegion) == sub) {
			return line(r), true
		}
	}
	return "", false
}

// budgetTeamLine picks the most specific team rule that matches the
// traveller: the one with the most columns set, then the lowest id.
func budgetTeamLine(hr *budgetHR, rules []engagemententity.BudgetTeamRule) string {
	if hr == nil {
		return ""
	}
	var best *engagemententity.BudgetTeamRule
	bestSet := -1
	for i := range rules {
		r := &rules[i]
		set, ok := 0, true
		for _, c := range []struct {
			rule *string
			have string
		}{{r.BusinessUnit, hr.BusinessUnit}, {r.Department, hr.Department}, {r.Team, hr.Team}} {
			if c.rule == nil {
				continue
			}
			set++
			ok = ok && budgetNorm(*c.rule) == budgetNorm(c.have)
		}
		if ok && (set > bestSet || (set == bestSet && r.ID < best.ID)) {
			best, bestSet = r, set
		}
	}
	if best == nil {
		return ""
	}
	return best.LineCode
}

// budgetCountry is a country as a grouping key; "" for a blank one.
func budgetCountry(c *string) string {
	if c == nil {
		return ""
	}
	return budgetNorm(*c)
}

// budgetMedian is the median of v (v is sorted in place), rounded to cents.
func budgetMedian(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	mid := len(v) / 2
	if len(v)%2 == 1 {
		return dashboardRound(v[mid])
	}
	return dashboardRound((v[mid-1] + v[mid]) / 2)
}
