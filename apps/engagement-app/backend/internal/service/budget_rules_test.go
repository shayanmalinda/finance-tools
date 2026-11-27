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
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
)

func rr(region string, sub *string, code *string) engagemententity.BudgetRegionRule {
	return engagemententity.BudgetRegionRule{Region: region, SubRegion: sub, LineCode: code}
}

var testRegionRules = []engagemententity.BudgetRegionRule{
	rr("APAC", nil, sp("SALES_APAC")), rr("EU", nil, sp("SALES_EU")), rr("UK", nil, sp("SALES_EU")),
	rr("NA", nil, sp("SALES_NA")), rr("NA - *", nil, sp("SALES_NA")), rr("LATAM", nil, sp("SALES_LATAM")),
	rr("MEA", sp("AFRICA"), sp("SALES_AFRICA")), rr("MEA", sp("ME"), sp("SALES_ME")),
	rr("AFRICA", nil, sp("SALES_AFRICA")), rr("ME", nil, sp("SALES_ME")), rr("NO POD", nil, nil),
}

var testTeamRules = []engagemententity.BudgetTeamRule{
	{ID: 1, Department: sp("ENGINEERING"), LineCode: "RND_CHOREO", BusinessUnit: sp("Choreo")},
	{ID: 2, Department: sp("SALES"), LineCode: "SALES_CENTRAL"},
	{ID: 3, Department: sp("SALES"), Team: sp("APAC"), LineCode: "SALES_APAC"},
	{ID: 4, Department: sp("MARKETING"), LineCode: "MKT_TRAVEL"},
}

func TestBudgetSuggest(t *testing.T) {
	active := map[string]bool{}
	for _, c := range []string{"SALES_APAC", "SALES_EU", "SALES_NA", "SALES_LATAM", "SALES_AFRICA", "SALES_ME", "GA_BOARD",
		"COS_NONREC", "RND_CHOREO", "SALES_CENTRAL", "MKT_TRAVEL"} {
		active[c] = true
	}
	fund := func(region, sub string, pct float64) budgetFunding { return budgetFunding{region, sub, pct} }
	tests := []struct {
		name       string
		in         budgetSuggestIn
		code, from string
	}{
		{"engagement line wins", budgetSuggestIn{EngagementLine: "GA_BOARD", Rule: budgetRule{"LINE", "COS_NONREC"}}, "GA_BOARD", "ENGAGEMENT"},
		{"fixed line", budgetSuggestIn{Rule: budgetRule{"LINE", "COS_NONREC"}}, "COS_NONREC", "ALLOCATION_TYPE"},
		{"no rule", budgetSuggestIn{}, "", "NONE"},
		{"engagement kind without a line", budgetSuggestIn{Rule: budgetRule{Kind: "ENGAGEMENT"}}, "", "NONE"},
		{"code not active in the year", budgetSuggestIn{Rule: budgetRule{"LINE", "GONE"}}, "", "NONE"},
		{"engagement line not active", budgetSuggestIn{EngagementLine: "GONE"}, "", "NONE"},
		{"region APAC", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("APAC", "", 100)}}, "SALES_APAC", "REGION"},
		{"region UK", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("UK", "", 100)}}, "SALES_EU", "REGION"},
		{"region NA - East by prefix", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("NA - East", "", 100)}}, "SALES_NA", "REGION"},
		{"sub-region holds the name", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("Other", "NA - EAST", 100)}}, "SALES_NA", "REGION"},
		{"MEA Africa", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("MEA", "AFRICA", 100)}}, "SALES_AFRICA", "REGION"},
		{"MEA ME, trimmed and any case", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund(" mea ", " me ", 100)}}, "SALES_ME", "REGION"},
		{"NO POD has no line", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("NO POD", "", 100)}}, "", "NONE"},
		{"no funding", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}}, "", "NONE"},
		{"highest percentage", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("APAC", "", 30), fund("EU", "", 70)}}, "SALES_EU", "REGION"},
		{"first on a tie", budgetSuggestIn{Rule: budgetRule{Kind: "REGION"}, Funding: []budgetFunding{fund("LATAM", "", 50), fund("EU", "", 50)}}, "SALES_LATAM", "REGION"},
		{"team most specific", budgetSuggestIn{Rule: budgetRule{Kind: "TEAM"}, HR: &budgetHR{"x", "Sales", "apac"}}, "SALES_APAC", "TEAM"},
		{"team less specific", budgetSuggestIn{Rule: budgetRule{Kind: "TEAM"}, HR: &budgetHR{"x", "SALES", "Other"}}, "SALES_CENTRAL", "TEAM"},
		{"team needs every set column", budgetSuggestIn{Rule: budgetRule{Kind: "TEAM"}, HR: &budgetHR{"IAM", "ENGINEERING", ""}}, "", "NONE"},
		{"team business unit", budgetSuggestIn{Rule: budgetRule{Kind: "TEAM"}, HR: &budgetHR{"choreo", "engineering", ""}}, "RND_CHOREO", "TEAM"},
		{"HR failure", budgetSuggestIn{Rule: budgetRule{Kind: "TEAM"}}, "", "NONE"},
	}
	for _, tc := range tests {
		tc.in.ActiveCodes = active
		tc.in.RegionRules, tc.in.TeamRules = testRegionRules, testTeamRules
		if code, from := budgetSuggest(tc.in); code != tc.code || from != tc.from {
			t.Errorf("%s: got %q %q, want %q %q", tc.name, code, from, tc.code, tc.from)
		}
	}
	// Equal specificity: the lowest id wins.
	rules := []engagemententity.BudgetTeamRule{{ID: 9, Department: sp("A"), LineCode: "SALES_EU"}, {ID: 2, Department: sp("A"), LineCode: "SALES_NA"}}
	if got := budgetTeamLine(&budgetHR{Department: "a"}, rules); got != "SALES_NA" {
		t.Errorf("tie = %q", got)
	}
}

func TestBudgetLineName(t *testing.T) {
	for _, c := range []struct{ fn, sub, want string }{
		{"Sales", "EU", "Sales: EU"}, {"Research and Development", "IAM Product", "R&D: IAM Product"},
		{"General Administration", "Board meeting travel", "G&A: Board meeting travel"},
		{"Marketing", "Marketing travel", "Marketing travel"}, {"Non-Recurring Revenue COS", "Direct delivery travel, non-recurring", "Direct delivery travel, non-recurring"},
	} {
		if got := budgetLineName(engagemententity.BudgetLine{FunctionName: c.fn, SubFunction: c.sub}); got != c.want {
			t.Errorf("%s/%s = %q", c.fn, c.sub, got)
		}
	}
}

func TestBudgetMedian(t *testing.T) {
	if budgetMedian(nil) != 0 || budgetMedian([]float64{3, 1, 2}) != 2 || budgetMedian([]float64{4, 1, 3, 2}) != 2.5 {
		t.Error("median")
	}
}
