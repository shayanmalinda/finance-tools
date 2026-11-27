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

package engagemententity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// budgetServer answers every call with body and records the last request.
func budgetServer(t *testing.T, body string) (*HTTPClient, *struct{ method, uri, body string }) {
	t.Helper()
	got := &struct{ method, uri, body string }{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.uri, got.body = r.Method, r.URL.RequestURI(), string(b)
		w.Header().Set("Content-Type", "application/json")
		if body == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, srv.Client()), got
}

func TestBudgetClient(t *testing.T) {
	ctx := context.Background()
	c, got := budgetServer(t, `{"years":[{"year":2027,"startDate":"2027-01-01","endDate":"2027-12-31","isActive":true}]}`)
	if ys, err := c.BudgetYears(ctx); err != nil || len(ys) != 1 || ys[0].Year != 2027 || !ys[0].IsActive || got.uri != "/budgets/years" {
		t.Errorf("years %+v %v %s", ys, err, got.uri)
	}
	c, got = budgetServer(t, `{"lines":[{"id":7,"year":2027,"code":"SALES_EU","functionName":"Sales","subFunction":"EU","holderName":null,"amountUsd":75000.5,"status":"Active","sortOrder":30}]}`)
	ls, err := c.BudgetLines(ctx, 2027, true)
	if err != nil || len(ls) != 1 || ls[0].AmountUSD != 75000.5 || ls[0].HolderName != nil || got.uri != "/budgets/lines?includeRetired=true&year=2027" {
		t.Errorf("lines %+v %v %s", ls, err, got.uri)
	}

	c, got = budgetServer(t, `{"id":8,"year":2027,"code":"X","functionName":"a","subFunction":"b","amountUsd":1,"status":"Active","sortOrder":0}`)
	if _, err := c.CreateBudgetLine(ctx, BudgetLineCreate{Year: 2027, Code: "X", FunctionName: "a", SubFunction: "b", AmountUSD: 1, CreatedBy: "a@wso2.com"}); err != nil ||
		got.method != "POST" || got.uri != "/budgets/lines" {
		t.Errorf("create %v %s %s", err, got.method, got.uri)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(got.body), &sent)
	if sent["createdBy"] != "a@wso2.com" || sent["amountUsd"] != 1.0 || sent["code"] != "X" {
		t.Errorf("body = %s", got.body)
	}
	if _, err := c.UpdateBudgetLine(ctx, 8, BudgetLineUpdate{Status: "Active", UpdatedBy: "a@wso2.com"}); err != nil || got.method != "PUT" || got.uri != "/budgets/lines/8" {
		t.Errorf("update %v %s %s", err, got.method, got.uri)
	}

	c, got = budgetServer(t, "")
	if err := c.RetireBudgetLine(ctx, 8, "a@wso2.com"); err != nil || got.method != "DELETE" || got.uri != "/budgets/lines/8?updatedBy=a%40wso2.com" {
		t.Errorf("retire %v %s %s", err, got.method, got.uri)
	}

	c, got = budgetServer(t, `{"engagementId":"E1","lineCode":"GA_BOARD"}`)
	if code, err := c.BudgetEngagementLine(ctx, "E1"); err != nil || code == nil || *code != "GA_BOARD" || got.uri != "/budgets/engagements/E1/line" {
		t.Errorf("engagement line %v %v %s", code, err, got.uri)
	}
	if _, err := c.SetBudgetEngagementLine(ctx, "E1", nil, "a@wso2.com"); err != nil || got.method != "PUT" || got.body != `{"lineCode":null,"updatedBy":"a@wso2.com"}` {
		t.Errorf("clear line %v %s", err, got.body)
	}

	c, got = budgetServer(t, `{"jobNumber":"TRV-1","budgetLineId":null,"suggestionSource":null,"hasRow":false}`)
	if tb, err := c.BudgetTrip(ctx, "TRV-1"); err != nil || tb.HasRow || tb.BudgetLineID != nil || tb.SuggestionSource != nil {
		t.Errorf("trip %+v %v", tb, err)
	}
	if _, err := c.SetBudgetTrip(ctx, "TRV-1", nil, "a@wso2.com"); err != nil || got.method != "PUT" || got.body != `{"budgetLineId":null,"updatedBy":"a@wso2.com"}` {
		t.Errorf("set trip %v %s", err, got.body)
	}

	c, got = budgetServer(t, `{"from":"2027-01-01","to":"2027-12-31","expenseAvailable":true,"truncated":false,"trips":[{"jobNumber":"T","budgetLineId":3,"status":"New","country":"Germany","startDate":"2027-01-18","endDate":"2027-01-22","rateDate":"2026-12-10"}],`+
		`"costs":[{"jobNumber":"T","source":"invoice","currency":"LKR","amount":412000}],"unassigned":[{"source":"card","month":"2027-03","currency":"USD","amount":812.4}]}`)
	l, err := c.BudgetLedger(ctx, "2027-01-01", "2027-12-31", true)
	if err != nil || len(l.Trips) != 1 || *l.Trips[0].BudgetLineID != 3 || l.Costs[0].Amount != 412000 || l.Unassigned[0].Month != "2027-03" ||
		got.uri != "/budgets/ledger?from=2027-01-01&to=2027-12-31&unassigned=true" {
		t.Errorf("ledger %+v %v %s", l, err, got.uri)
	}
	c, got = budgetServer(t, `{"items":[{"jobNumber":"T"}],"total":9}`)
	if p, err := c.BudgetNeedsLine(ctx, 2027, 50, 0); err != nil || p.Total != 9 || got.uri != "/budgets/needs-line?limit=50&offset=0&year=2027" {
		t.Errorf("needs %+v %v %s", p, err, got.uri)
	}
	c, got = budgetServer(t, `{"rules":[{"allocationTypeId":12,"ruleKind":"LINE","lineCode":"SALES_APAC"}]}`)
	if rs, err := c.BudgetAllocationTypeRules(ctx); err != nil || *rs[0].LineCode != "SALES_APAC" {
		t.Errorf("rules %+v %v", rs, err)
	}
	if _, err := c.SaveBudgetAllocationTypeRule(ctx, 12, BudgetRuleSave{RuleKind: "NONE", UpdatedBy: "a"}); err != nil || got.uri != "/budgets/allocation-type-rules/12" {
		t.Errorf("save rule %v %s", err, got.uri)
	}
}

// The travelBudget of an allocation write is left out unless set, so the
// payload of an app with budgets off is what it always was.
func TestAllocationWritesOmitTravelBudgetWhenNil(t *testing.T) {
	for _, v := range []any{AllocationBulkRequest{}, AllocationUpdateRequest{}} {
		b, _ := json.Marshal(v)
		if m := map[string]any{}; json.Unmarshal(b, &m) != nil || m["travelBudget"] != nil {
			t.Errorf("%T = %s", v, b)
		}
		if _, has := map[string]any{}["travelBudget"]; has {
			t.Fail()
		}
	}
	line := 7
	b, _ := json.Marshal(AllocationUpdateRequest{TravelBudget: &AllocationTravelBudget{BudgetLineID: &line, SuggestionSource: "NONE"}})
	if string(b) != `{"updatedBy":"","travelBudget":{"budgetLineId":7,"suggestedLineId":null,"suggestionSource":"NONE","chosenBy":null}}` {
		t.Errorf("update = %s", b)
	}
}
