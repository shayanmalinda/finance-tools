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

package salesentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
)

func TestSearchCampaigns_SendsNameLike(t *testing.T) {
	var body map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`[{"id":"7015g000000AbCdAAK","name":"WSO2Con 2027"}]`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, nil).SearchCampaigns(context.Background(), CampaignFilter{NameLike: "WSO2Con"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/campaigns/search" {
		t.Errorf("path = %q", path)
	}
	if body["nameLike"] != "WSO2Con" || len(body) != 1 {
		t.Errorf("request body = %v, want only nameLike", body)
	}
	if len(got) != 1 || got[0].Name != "WSO2Con 2027" {
		t.Errorf("got %+v", got)
	}
}

func TestUnconfiguredAnswers503(t *testing.T) {
	_, err := Unconfigured{}.SearchCampaigns(context.Background(), CampaignFilter{})
	var ue *apierror.UpstreamError
	if !errors.As(err, &ue) || ue.ClientError() {
		t.Errorf("err = %v, want a non-client UpstreamError", err)
	}
}

// Opportunities pages 1000 at a time until a short page, sorted by id, and
// leaves isOnlyActiveOpportunities unset for "not only active".
func TestOpportunities_PagesAndActiveFlag(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/opportunities/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		if body["offset"].(float64) == 0 {
			page := make([]map[string]any, searchPageSize)
			for i := range page {
				page[i] = map[string]any{"id": "006A", "name": "PoC", "owner": map[string]any{"name": "Kevin"}, "isWon": true}
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"006B","name":"Renewal","owner":null,"preSalesCode":"B-PR","invoices":[]}]`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, nil).Opportunities(context.Background(), "001A", false)
	if err != nil || len(got) != searchPageSize+1 || *got[0].Owner.Name != "Kevin" || *got[searchPageSize].PreSalesCode != "B-PR" {
		t.Fatalf("got %d err %v", len(got), err)
	}
	if len(bodies) != 2 || bodies[1]["offset"].(float64) != searchPageSize {
		t.Fatalf("bodies = %v", bodies)
	}
	if _, ok := bodies[0]["isOnlyActiveOpportunities"]; ok || bodies[0]["customerId"] != "001A" ||
		bodies[0]["limit"].(float64) != searchPageSize || bodies[0]["sortBy"] != "id" {
		t.Errorf("body = %v", bodies[0])
	}

	bodies = nil
	if _, err := New(srv.URL, nil).Opportunities(context.Background(), "001A", true); err != nil {
		t.Fatal(err)
	}
	if bodies[0]["isOnlyActiveOpportunities"] != true {
		t.Errorf("body = %v", bodies[0])
	}
}

func TestOpportunityLineItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/opportunity-line-items/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch body["id"] {
		case "00kA":
			if body["limit"].(float64) != 1 {
				t.Errorf("body = %v", body)
			}
			_, _ = w.Write([]byte(`[{"id":"00kA","serviceStartDate":"2026-10-19","serviceEndDate":"2026-11-19"}]`))
		case "00kNone":
			_, _ = w.Write([]byte(`[]`))
		default:
			if _, ok := body["classifications"]; ok || body["opportunityId"] != "006A" || body["limit"].(float64) != searchPageSize {
				t.Errorf("body = %v", body)
			}
			_, _ = w.Write([]byte(`[{"id":"00kA","classification":"CO","engagementCodeAuto":"NWB-CO"},{"id":"00kB","classification":"SUB"},{"id":"00kC"}]`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	items, err := c.OpportunityLineItems(context.Background(), "006A")
	if err != nil || len(items) != 1 || *items[0].EngagementCodeAuto != "NWB-CO" {
		t.Fatalf("items %+v err %v", items, err)
	}
	item, err := c.OpportunityLineItem(context.Background(), "00kA")
	if err != nil || *item.ServiceEndDate != "2026-11-19" {
		t.Fatalf("item %+v err %v", item, err)
	}
	if item, err := c.OpportunityLineItem(context.Background(), "00kNone"); err != nil || item != nil {
		t.Fatalf("item %+v err %v, want nil", item, err)
	}
	if _, err := New("", nil).OpportunityLineItem(context.Background(), "x"); err == nil {
		t.Error("no base URL: want an error")
	}
	if _, err := New("", nil).Opportunities(context.Background(), "001A", false); err == nil {
		t.Error("no base URL: want an error")
	}
}

func TestOpportunities_FullLastPageAtTheCapIsAnError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		page := make([]map[string]any, searchPageSize)
		for i := range page {
			page[i] = map[string]any{"id": "006A"}
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	got, err := New(srv.URL, nil).Opportunities(context.Background(), "001A", false)
	if err == nil || got != nil {
		t.Fatalf("got %d rows err %v, want an error", len(got), err)
	}
	if calls != searchMaxPages {
		t.Errorf("calls = %d, want %d", calls, searchMaxPages)
	}
}
