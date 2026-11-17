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
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// A holder of the agent page (18) with no agency row and no Travel 1 or 2
// is not an agent, but must not get the full per diem view either.
func TestFinancePerDiemsAgentPageWithoutAgency(t *testing.T) {
	ctx := context.Background()
	s, _ := newFin(&finEntity{trip: openTrip()}, nil)
	if _, err := s.PerDiems(ctx, perms("x@wso2.com", 18), job); !isForbidden(err) {
		t.Errorf("18 without an agency: %v", err)
	}
	if _, err := s.PerDiems(ctx, perms("jane@wso2.com", 18, 37), job); err != nil {
		t.Errorf("18 without an agency, own trip with 37: %v", err)
	}
	if _, err := s.PerDiems(ctx, perms("x@wso2.com", 18, 2), job); err != nil {
		t.Errorf("18 with Travel 2: %v", err)
	}
}

// 0.6 - (0.1 + 0.2) is 0.29999999999999993 in float64: a 0.3 per diem is
// exactly the remaining balance and may be deleted.
func TestFinanceDeletePerDiemRoundsTheBalance(t *testing.T) {
	e := &finEntity{trip: openTrip(),
		perDiems: []engagemententity.FinancePerDiem{{ID: 5, Currency: fsp("GBP"), Total: 0.3}},
		basis: engagemententity.FinancePaymentBasis{PaymentCount: 2,
			PerDiemTotals: []engagemententity.FinancePerDiemTotal{{Currency: "GBP", TotalAmount: 0.6}},
			Payments: []engagemententity.FinanceAmount{{Amount: 0.1, Currency: "GBP"},
				{Amount: 0.2, Currency: "GBP"}}},
	}
	s, _ := newFin(e, nil)
	if err := s.DeletePerDiem(context.Background(), perms("po@wso2.com", 27), job, 5); err != nil || !e.deleted {
		t.Errorf("delete = %v, deleted %v", err, e.deleted)
	}
}

func TestFinancePaymentAmountsAreWholeCents(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{trip: openTrip(), subsidiary: "GBP",
		basis: engagemententity.FinancePaymentBasis{
			PerDiemTotals: []engagemententity.FinancePerDiemTotal{{Currency: "GBP", TotalAmount: 700}}},
		payment: engagemententity.FinancePayment{PerDiemNumber: job + "-001", JobNumber: job, NSStatus: fsp("Pending"),
			TravelStatus: fsp("In Progress")},
	}
	s, _ := newFin(e, &finFX{})
	if _, err := s.AddPayment(ctx, perms("f@wso2.com", 43), job, 100.004); err == nil ||
		err.Error() != FinancePaymentCentsMsg {
		t.Errorf("add 100.004: %v", err)
	}
	if _, err := s.AddPayment(ctx, perms("f@wso2.com", 43), job, 100.1); err != nil {
		t.Errorf("add 100.1: %v", err)
	}
	if _, err := s.UpdatePayment(ctx, perms("f@wso2.com", 44), job+"-001", 1.005); err == nil ||
		err.Error() != FinancePaymentCentsMsg {
		t.Errorf("update 1.005: %v", err)
	}
}

// All with every status sends no filter, so payments with a NULL or
// unexpected ns_status are listed, as counts.All includes them.
func TestFinancePaymentsAllSendsNoStatusFilter(t *testing.T) {
	e := &finEntity{payment: engagemententity.FinancePayment{PerDiemNumber: job + "-001", JobNumber: job}}
	s, _ := newFin(e, nil)
	if _, err := s.Payments(context.Background(), perms("f@wso2.com", 41), domain.PaymentQuery{Tab: "all"}); err != nil {
		t.Fatal(err)
	}
	if len(e.payFilters) != 1 || e.payFilters[0].NSStatuses != nil {
		t.Errorf("filters = %+v, want no status filter", e.payFilters)
	}
}

// stopEntity fails the push of one invoice with err.
type stopEntity struct {
	*finEntity
	failID int
	err    error
}

func (f *stopEntity) FinancePushInvoice(ctx context.Context, id int, actor string) (engagemententity.FinancePushResult, error) {
	if id == f.failID {
		return engagemententity.FinancePushResult{}, f.err
	}
	return f.finEntity.FinancePushInvoice(ctx, id, actor)
}

// A batch stopped after records were pushed keeps their results.
func TestFinancePushInvoicesKeepsDoneResultsOnStop(t *testing.T) {
	for _, c := range []struct {
		name      string
		err       error
		cur, rest string
	}{
		{"netsuite off", &apierror.UpstreamError{StatusCode: 503, Msg: "off"}, financeNetSuiteOffMsg, financeNetSuiteOffMsg},
		{"deadline", context.DeadlineExceeded, financeOutcomeUnknownMsg, financeOutOfTimeMsg},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &stopEntity{finEntity: &finEntity{}, failID: 2, err: c.err}
			s := NewFinanceService(e, finHR{}, &finMail{}, email.FinanceRecipients{}, nil,
				func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) })
			res, err := s.PushInvoices(context.Background(), perms("f@wso2.com", 34), []int{1, 2, 3})
			if err != nil || len(res.Results) != 3 {
				t.Fatalf("res = %+v, err = %v", res, err)
			}
			if !res.Results[0].OK || *res.Results[1].Message != c.cur || *res.Results[2].Message != c.rest ||
				len(e.pushed) != 1 {
				t.Errorf("res = %+v, pushed %v", res, e.pushed)
			}
		})
	}
	// Nothing pushed yet: the stop error is the answer.
	e := &stopEntity{finEntity: &finEntity{}, failID: 1, err: &apierror.UpstreamError{StatusCode: 503, Msg: "off"}}
	s := NewFinanceService(e, finHR{}, &finMail{}, email.FinanceRecipients{}, nil, time.Now)
	if _, err := s.PushInvoices(context.Background(), perms("f@wso2.com", 34), []int{1, 2}); !errors.As(err,
		new(*apierror.ServiceUnavailableError)) {
		t.Errorf("first record: %v", err)
	}
}

// countHR counts HR employee lookups.
type countHR struct {
	hrentity.Client
	n int
}

func (h *countHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	h.n++
	return &hrentity.Employee{WorkEmail: e, Company: fsp("WSO2 - UK")}, nil
}

// addEntity accepts invoices.
type addEntity struct{ *finEntity }

func (addEntity) FinanceCostTypes(context.Context) ([]engagemententity.FinanceCostType, error) {
	return []engagemententity.FinanceCostType{{ID: 3, Name: "Air Ticket"}}, nil
}
func (addEntity) FinanceAddInvoices(_ context.Context, jn string, in engagemententity.FinanceInvoicesCreate) ([]engagemententity.FinanceInvoice, error) {
	out := make([]engagemententity.FinanceInvoice, len(in.Invoices))
	for i, v := range in.Invoices {
		out[i] = engagemententity.FinanceInvoice{ID: i + 1, InvoiceNumber: v.InvoiceNumber, JobNumber: jn}
	}
	return out, nil
}

// One email per invoice (legacy), but the traveller's company is read once.
func TestFinanceAddInvoicesReadsTheCompanyOnce(t *testing.T) {
	hr, m := &countHR{}, &finMail{}
	s := NewFinanceService(addEntity{&finEntity{trip: openTrip()}}, hr, m, email.FinanceRecipients{FinanceGroup: "fin@wso2.com"},
		nil, time.Now)
	rows := []domain.InvoiceCreate{
		{InvoiceNumber: "A1", Month: "Nov", CostType: "Air Ticket", Amount: 10, Currency: "USD"},
		{InvoiceNumber: "A2", Month: "Nov", CostType: "Air Ticket", Amount: 20, Currency: "USD"},
		{InvoiceNumber: "A3", Month: "Nov", CostType: "Air Ticket", Amount: 30, Currency: "USD"},
	}
	if _, err := s.AddInvoices(context.Background(), perms("a@wso2.com", 31), job, rows); err != nil {
		t.Fatal(err)
	}
	if len(m.sent) != 3 || hr.n != 1 {
		t.Errorf("emails %d, HR lookups %d; want 3 and 1", len(m.sent), hr.n)
	}
}
