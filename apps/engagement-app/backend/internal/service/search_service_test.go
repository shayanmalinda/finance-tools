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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// searchFake implements every search source; each group's behaviour is a field.
type searchFake struct {
	mu       sync.Mutex
	travel   func(ctx context.Context, q domain.TravelListQuery) (domain.TravelRequestPage, error)
	invoices func(ctx context.Context, q domain.InvoiceQuery) (domain.InvoicePage, error)
	payments func(ctx context.Context, q domain.PaymentQuery) (domain.PerDiemPaymentPage, error)
	eng      func(ctx context.Context, f EngagementListFilter) (domain.EngagementPage, error)
	allocs   func(ctx context.Context, f domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error)

	travelQ   domain.TravelListQuery
	invoiceQ  domain.InvoiceQuery
	paymentQ  domain.PaymentQuery
	engFilter []EngagementListFilter
	allocF    domain.AllocationListFilter
}

func (f *searchFake) List(ctx context.Context, _ domain.Perms, q domain.TravelListQuery) (domain.TravelRequestPage, error) {
	f.mu.Lock()
	f.travelQ = q
	f.mu.Unlock()
	return f.travel(ctx, q)
}

func (f *searchFake) Invoices(ctx context.Context, _ domain.Perms, q domain.InvoiceQuery) (domain.InvoicePage, error) {
	f.mu.Lock()
	f.invoiceQ = q
	f.mu.Unlock()
	return f.invoices(ctx, q)
}

func (f *searchFake) Payments(ctx context.Context, _ domain.Perms, q domain.PaymentQuery) (domain.PerDiemPaymentPage, error) {
	f.mu.Lock()
	f.paymentQ = q
	f.mu.Unlock()
	return f.payments(ctx, q)
}

// Engagements and allocations have a List method each; they are separate
// types so the fake can serve both.
type searchEngFake struct{ f *searchFake }

func (e searchEngFake) List(ctx context.Context, _ domain.Perms, fl EngagementListFilter) (domain.EngagementPage, error) {
	e.f.mu.Lock()
	e.f.engFilter = append(e.f.engFilter, fl)
	e.f.mu.Unlock()
	return e.f.eng(ctx, fl)
}

type searchAllocFake struct{ f *searchFake }

func (a searchAllocFake) List(ctx context.Context, _ domain.Perms, fl domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
	a.f.mu.Lock()
	a.f.allocF = fl
	a.f.mu.Unlock()
	return a.f.allocs(ctx, fl)
}

func (f *searchFake) sources() SearchSources {
	return SearchSources{Travel: f, Invoices: f, Payments: f, Engagements: searchEngFake{f}, Allocations: searchAllocFake{f}}
}

// newSearchFake returns a fake where every group has one hit.
func newSearchFake() *searchFake {
	return &searchFake{
		travel: func(context.Context, domain.TravelListQuery) (domain.TravelRequestPage, error) {
			return domain.TravelRequestPage{Total: 12, Items: []domain.TravelRequestSummary{{
				JobNumber: "TRV-2026-11-0003", Status: "In Progress", Traveller: domain.Person{Email: "jane@wso2.com", Name: sp("Jane Doe")},
				Country: sp("India"), StartDate: "2026-11-01", EndDate: "2026-11-12"}}}, nil
		},
		invoices: func(context.Context, domain.InvoiceQuery) (domain.InvoicePage, error) {
			return domain.InvoicePage{Total: 1, Items: []domain.Invoice{{ID: 7, InvoiceNumber: "INV-7", JobNumber: "TRV-2026-11-0003",
				Agency: sp("Acme Travel"), Amount: 1200.5, Currency: "USD", Status: "Pending"}}}, nil
		},
		payments: func(context.Context, domain.PaymentQuery) (domain.PerDiemPaymentPage, error) {
			return domain.PerDiemPaymentPage{Total: 1, Items: []domain.PerDiemPayment{{ID: "PD-1", JobNumber: "TRV-2026-11-0003",
				Traveller: &domain.Person{Email: "jane@wso2.com"}, Amount: 80, Currency: "EUR", Status: "Pending"}}}, nil
		},
		eng: func(_ context.Context, fl EngagementListFilter) (domain.EngagementPage, error) {
			return domain.EngagementPage{Total: 2, Items: []domain.EngagementSummary{{ID: "E-" + fl.Segment, Title: "Engagement " + fl.Segment,
				Account: sp("Acme"), EngagementCode: "AC-1", EngagementTypeName: "Paid", EngagementStatus: "Active"},
				{ID: "E2-" + fl.Segment, Title: "Second"}}}, nil
		},
		allocs: func(context.Context, domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
			return domain.Page[domain.AllocationWithEngagement]{Total: 1, Items: []domain.AllocationWithEngagement{{
				Allocation: domain.Allocation{ID: "A1", Consultant: domain.Person{Email: "c@wso2.com"}, StartDate: "2026-11-28", EndDate: "2026-12-03",
					ClearanceStatus: sp("Confirmed")},
				Engagement: domain.AllocationEngagementSummary{Kind: "customer", Title: "Big Project", Account: sp("Acme")}}}}, nil
		},
	}
}

func searchPerms(travel, alloc []int) domain.Perms {
	return domain.Perms{Email: "jane@wso2.com", Travel: domain.NewPrivilegeSet(travel), Alloc: domain.NewPrivilegeSet(alloc)}
}

func kinds(r domain.SearchResponse) []string {
	out := []string{}
	for _, g := range r.Groups {
		out = append(out, g.Kind)
	}
	return out
}

func TestSearchGroupsOrderAndPrivilegeOmission(t *testing.T) {
	all := []string{"travelRequests", "invoices", "perDiemPayments", "engagements", "allocations"}
	for _, tc := range []struct {
		name          string
		travel, alloc []int
		want          []string
	}{
		{"everything, stable order", []int{1, 39, 41}, []int{3, 9}, all},
		{"travel only", []int{1, 39, 41}, nil, all[:3]},
		{"allocations only", nil, []int{3, 9}, all[3:]},
		{"engagements without allocations list", nil, []int{3}, []string{"engagements"}},
		{"allocations list only", nil, []int{9}, []string{"engagements", "allocations"}},
		{"agent privilege alone", []int{18}, nil, []string{"travelRequests"}},
		{"invoice approver only", []int{32}, nil, []string{"invoices"}},
		{"per diem push only", []int{40}, nil, []string{"perDiemPayments"}},
		{"no privileges", nil, nil, []string{}},
		{"unrelated privileges", []int{49}, []int{1}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := NewSearchService(newSearchFake().sources()).Search(context.Background(), searchPerms(tc.travel, tc.alloc), "acme", 5)
			if err != nil {
				t.Fatal(err)
			}
			if got := kinds(resp); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("groups = %v, want %v", got, tc.want)
			}
			if resp.Failed == nil || len(resp.Failed) != 0 {
				t.Errorf("failed = %#v, want empty", resp.Failed)
			}
		})
	}
}

func TestSearchQueryValidation(t *testing.T) {
	for _, tc := range []struct {
		q    string
		ok   bool
		want string
	}{
		{"", false, ""}, {"a", false, ""}, {"  a ", false, ""}, {"é", false, ""},
		{"ab", true, "ab"}, {"  ab  ", true, "ab"}, {"éé", true, "éé"},
		{strings.Repeat("x", 200), true, strings.Repeat("x", 200)}, {strings.Repeat("x", 201), false, ""},
	} {
		resp, err := NewSearchService(newSearchFake().sources()).Search(context.Background(), searchPerms([]int{1}, nil), tc.q, 5)
		var ve *apierror.ValidationError
		if tc.ok && err != nil || !tc.ok && !errors.As(err, &ve) {
			t.Errorf("q=%q: err = %v, want ok=%v", tc.q, err, tc.ok)
		}
		if tc.ok && resp.Query != tc.want {
			t.Errorf("q=%q: query = %q, want %q", tc.q, resp.Query, tc.want)
		}
	}
}

func TestSearchLimitClamp(t *testing.T) {
	for limit, want := range map[int]int{-3: 5, 0: 5, 1: 1, 5: 5, 20: 20, 21: 20, 500: 20} {
		f := newSearchFake()
		_, err := NewSearchService(f.sources()).Search(context.Background(), searchPerms([]int{1, 39, 41}, []int{3, 9}), "acme", limit)
		if err != nil {
			t.Fatal(err)
		}
		got := []int{f.travelQ.Limit, f.invoiceQ.Limit, f.paymentQ.Limit, f.allocF.Limit}
		for _, e := range f.engFilter {
			got = append(got, e.Limit)
		}
		for _, g := range got {
			if g != want {
				t.Errorf("limit %d: source limits %v, want all %d", limit, got, want)
				break
			}
		}
	}
}

func TestSearchJobNumberNormalisation(t *testing.T) {
	for q, want := range map[string]string{
		"TRV-2026-11-0003": "TRV-2026-11-0003",
		"2026-11-0003":     "TRV-2026-11-0003",
		"TRV2026110003":    "TRV-2026-11-0003",
		"trv2026110003":    "TRV-2026-11-0003",
		"trv 2026 11 0003": "TRV-2026-11-0003",
		"2026110003":       "TRV-2026-11-0003",
		"trv-2026-11":      "TRV-2026-11",
		"Trv-2026":         "TRV-2026",
		"2026-11":          "2026-11",
		"jane@wso2.com":    "jane@wso2.com",
		"trvl":             "trvl",
		"London":           "London",
	} {
		if got := searchJobTerm(q); got != want {
			t.Errorf("searchJobTerm(%q) = %q, want %q", q, got, want)
		}
	}

	// Job-number sources get the normalised term; engagements and allocations the raw text.
	f := newSearchFake()
	_, err := NewSearchService(f.sources()).Search(context.Background(), searchPerms([]int{1, 39, 41}, []int{3, 9}), "trv2026110003", 5)
	if err != nil {
		t.Fatal(err)
	}
	if f.travelQ.Search != "TRV-2026-11-0003" || f.invoiceQ.Search != "TRV-2026-11-0003" || f.paymentQ.Search != "TRV-2026-11-0003" {
		t.Errorf("job term not normalised: %q %q %q", f.travelQ.Search, f.invoiceQ.Search, f.paymentQ.Search)
	}
	if f.allocF.Search != "trv2026110003" || f.engFilter[0].Search != "trv2026110003" {
		t.Errorf("raw text expected: %q %q", f.allocF.Search, f.engFilter[0].Search)
	}
}

func TestSearchFailedGroups(t *testing.T) {
	boom := &apierror.UpstreamError{Service: "engagement-entity", StatusCode: 502, Msg: "bad gateway"}
	forbidden := &apierror.ForbiddenError{Msg: "no"}
	for _, tc := range []struct {
		name       string
		mutate     func(f *searchFake)
		wantGroups []string
		wantFailed []string
	}{
		{"one source fails, rest return", func(f *searchFake) {
			f.invoices = func(context.Context, domain.InvoiceQuery) (domain.InvoicePage, error) {
				return domain.InvoicePage{}, boom
			}
		}, []string{"travelRequests", "perDiemPayments", "engagements", "allocations"}, []string{"invoices"}},
		{"several fail, failed keeps group order", func(f *searchFake) {
			f.allocs = func(context.Context, domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
				return domain.Page[domain.AllocationWithEngagement]{}, boom
			}
			f.travel = func(context.Context, domain.TravelListQuery) (domain.TravelRequestPage, error) {
				return domain.TravelRequestPage{}, boom
			}
		}, []string{"invoices", "perDiemPayments", "engagements"}, []string{"travelRequests", "allocations"}},
		{"one failing engagement segment fails the group", func(f *searchFake) {
			f.eng = func(_ context.Context, fl EngagementListFilter) (domain.EngagementPage, error) {
				if fl.Segment == "internal" {
					return domain.EngagementPage{}, boom
				}
				return domain.EngagementPage{Total: 1, Items: []domain.EngagementSummary{{ID: "E"}}}, nil
			}
		}, []string{"travelRequests", "invoices", "perDiemPayments", "allocations"}, []string{"engagements"}},
		{"a source that refuses the caller is omitted, not failed", func(f *searchFake) {
			f.invoices = func(context.Context, domain.InvoiceQuery) (domain.InvoicePage, error) {
				return domain.InvoicePage{}, forbidden
			}
		}, []string{"travelRequests", "perDiemPayments", "engagements", "allocations"}, []string{}},
		{"every engagement segment refused is omitted", func(f *searchFake) {
			f.eng = func(context.Context, EngagementListFilter) (domain.EngagementPage, error) {
				return domain.EngagementPage{}, forbidden
			}
		}, []string{"travelRequests", "invoices", "perDiemPayments", "allocations"}, []string{}},
		{"empty groups are omitted", func(f *searchFake) {
			f.travel = func(context.Context, domain.TravelListQuery) (domain.TravelRequestPage, error) {
				return domain.TravelRequestPage{Items: []domain.TravelRequestSummary{}}, nil
			}
		}, []string{"invoices", "perDiemPayments", "engagements", "allocations"}, []string{}},
		{"a panicking source is failed", func(f *searchFake) {
			f.payments = func(context.Context, domain.PaymentQuery) (domain.PerDiemPaymentPage, error) { panic("nil entity") }
		}, []string{"travelRequests", "invoices", "engagements", "allocations"}, []string{"perDiemPayments"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSearchFake()
			tc.mutate(f)
			resp, err := NewSearchService(f.sources()).Search(context.Background(), searchPerms([]int{1, 39, 41}, []int{3, 9}), "acme", 5)
			if err != nil {
				t.Fatal(err)
			}
			if got := kinds(resp); !reflect.DeepEqual(got, tc.wantGroups) {
				t.Errorf("groups = %v, want %v", got, tc.wantGroups)
			}
			if !reflect.DeepEqual(resp.Failed, tc.wantFailed) {
				t.Errorf("failed = %v, want %v", resp.Failed, tc.wantFailed)
			}
		})
	}
}

func TestSearchTimeoutReportsSlowGroupsAsFailed(t *testing.T) {
	f := newSearchFake()
	// Ignores its context, as a hung connection would.
	release := make(chan struct{})
	defer close(release)
	f.allocs = func(context.Context, domain.AllocationListFilter) (domain.Page[domain.AllocationWithEngagement], error) {
		<-release
		return domain.Page[domain.AllocationWithEngagement]{}, nil
	}
	f.invoices = func(ctx context.Context, _ domain.InvoiceQuery) (domain.InvoicePage, error) {
		<-ctx.Done()
		return domain.InvoicePage{}, ctx.Err()
	}
	svc := NewSearchService(f.sources()).(*searchService)
	svc.timeout = 50 * time.Millisecond

	start := time.Now()
	resp, err := svc.Search(context.Background(), searchPerms([]int{1, 39, 41}, []int{3, 9}), "acme", 5)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("search waited %v for a hung source", time.Since(start))
	}
	if want := []string{"travelRequests", "perDiemPayments", "engagements"}; !reflect.DeepEqual(kinds(resp), want) {
		t.Errorf("groups = %v, want %v", kinds(resp), want)
	}
	if want := []string{"invoices", "allocations"}; !reflect.DeepEqual(resp.Failed, want) {
		t.Errorf("failed = %v, want %v", resp.Failed, want)
	}
}

func TestSearchItems(t *testing.T) {
	f := newSearchFake()
	resp, err := NewSearchService(f.sources()).Search(context.Background(), searchPerms([]int{1, 39, 41}, []int{3, 9}), "acme", 3)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(resp)
	for _, want := range []string{
		`{"id":"TRV-2026-11-0003","title":"TRV-2026-11-0003","subtitle":"Jane Doe, India, 1–12 Nov 2026","status":"In Progress"}`,
		`"total":12`,
		`{"id":"7","title":"INV-7","subtitle":"TRV-2026-11-0003, Acme Travel, 1200.50 USD","status":"Pending","jobNumber":"TRV-2026-11-0003"}`,
		`{"id":"PD-1","title":"PD-1","subtitle":"TRV-2026-11-0003, jane@wso2.com, 80.00 EUR","status":"Pending","jobNumber":"TRV-2026-11-0003"}`,
		`{"id":"E-customer","title":"Engagement customer","subtitle":"Acme, AC-1, Paid","status":"Active","segment":"customer"}`,
		`{"id":"A1","title":"c@wso2.com","subtitle":"Big Project, Acme, 28 Nov – 3 Dec 2026","status":"Confirmed","segment":"customer"}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("response lacks %s\n%s", want, b)
		}
	}
	// Engagements: three segments of two hits merge in segment order, cut to the limit, total summed.
	for _, g := range resp.Groups {
		if g.Kind != "engagements" {
			continue
		}
		var ids []string
		for _, it := range g.Items {
			ids = append(ids, it.ID)
		}
		if g.Total != 6 || !reflect.DeepEqual(ids, []string{"E-customer", "E2-customer", "E-internal"}) {
			t.Errorf("engagements total %d items %v", g.Total, ids)
		}
	}
	if len(f.engFilter) != 3 {
		t.Errorf("engagement segments searched = %d, want 3", len(f.engFilter))
	}
}

func TestSearchDateRange(t *testing.T) {
	for _, tc := range []struct{ s, e, want string }{
		{"2026-11-01", "2026-11-12", "1–12 Nov 2026"},
		{"2026-10-28", "2026-11-03", "28 Oct – 3 Nov 2026"},
		{"2026-12-28", "2027-01-03", "28 Dec 2026 – 3 Jan 2027"},
		{"2026-11-05", "2026-11-05", "5 Nov 2026"},
		{"", "", ""},
		{"soon", "later", "soon – later"},
	} {
		if got := searchDateRange(tc.s, tc.e); got != tc.want {
			t.Errorf("searchDateRange(%q, %q) = %q, want %q", tc.s, tc.e, got, tc.want)
		}
	}
}
