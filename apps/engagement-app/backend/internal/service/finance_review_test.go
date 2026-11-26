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
	"strings"
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
		{"netsuite off", &apierror.UpstreamError{StatusCode: 503, Msg: "NetSuite push is not enabled"},
			financeNetSuiteOffMsg, financeNetSuiteOffMsg},
		// Any other 503 is the entity being unavailable, not NetSuite off;
		// the push was in flight, so its outcome is unknown.
		{"entity down", &apierror.UpstreamError{StatusCode: 503, Msg: "Service Unavailable"},
			financeOutcomeUnknownMsg, financeEntityDownMsg},
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
	// NetSuite off on the first record: nothing ran, the stop error is the
	// answer.
	e := &stopEntity{finEntity: &finEntity{}, failID: 1,
		err: &apierror.UpstreamError{StatusCode: 503, Msg: "NetSuite push is not enabled"}}
	s := NewFinanceService(e, finHR{}, &finMail{}, email.FinanceRecipients{}, nil, time.Now)
	if _, err := s.PushInvoices(context.Background(), perms("f@wso2.com", 34), []int{1, 2}); !errors.As(err,
		new(*apierror.ServiceUnavailableError)) {
		t.Errorf("first record, NetSuite off: %v", err)
	}
	// Any other 503 on the first record may have come after the push
	// started: the first record is outcome unknown, the rest not started.
	e.err = &apierror.UpstreamError{StatusCode: 503, Msg: "Service Unavailable"}
	res, err := s.PushInvoices(context.Background(), perms("f@wso2.com", 34), []int{1, 2})
	if err != nil || len(res.Results) != 2 || *res.Results[0].Message != financeOutcomeUnknownMsg ||
		*res.Results[1].Message != financeEntityDownMsg {
		t.Errorf("first record, entity down: %+v, %v", res, err)
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
		out[i] = engagemententity.FinanceInvoice{ID: i + 1, InvoiceNumber: v.InvoiceNumber, JobNumber: jn, CostTypeID: v.CostTypeID,
			Month: v.Month, InvoiceCurrency: v.InvoiceCurrency, InvoiceAmount: v.InvoiceAmount}
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
	// Each email shows its own invoice, the cost type named from its id.
	for i, want := range []string{"A1", "A2", "A3"} {
		items := m.sent[i].Content["details_html"]
		if !strings.Contains(items, ">"+want+"<") || !strings.Contains(items, ">Air Ticket<") || !strings.Contains(items, ">Nov<") {
			t.Errorf("email %d items:\n%s", i, items)
		}
	}
	if !strings.Contains(m.sent[2].Content["details_html"], "USD 30.00") {
		t.Error("third email lacks its amount")
	}
}

// A positive amount must be at least one cent; it can't pass as 0.00.
func TestFinanceCentsNeedsOneCent(t *testing.T) {
	for f, want := range map[float64]bool{100.1: true, 0.01: true, 0: true, 0.000000001: false, 0.0099999999: false, 100.004: false} {
		if got := financeCents(f); got != want {
			t.Errorf("financeCents(%v) = %v, want %v", f, got, want)
		}
	}
}

// A batch that stops while the first record's call is in flight reports
// that record as outcome unknown, not a bare batch error; one that stops
// before any call keeps the batch error; earlier answers are kept.
func TestFinanceBatchStop(t *testing.T) {
	empty := domain.BatchResult{Results: []domain.BatchItem{}}
	out, err := financeBatchStop(empty, "A", true, []string{"B"}, context.DeadlineExceeded)
	if err != nil || len(out.Results) != 2 || *out.Results[0].Message != financeOutcomeUnknownMsg ||
		*out.Results[1].Message != financeOutOfTimeMsg {
		t.Errorf("first in flight: %+v, %v", out, err)
	}
	if _, err := financeBatchStop(empty, "A", false, []string{"B"}, context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("before any call: err = %v, want the batch error", err)
	}
	done := domain.BatchResult{Results: []domain.BatchItem{{ID: "A", Message: financeStr("Not pushed: the trip is closed.")}}}
	out, err = financeBatchStop(done, "B", false, []string{"C"}, context.DeadlineExceeded)
	if err != nil || len(out.Results) != 3 || out.Results[0].ID != "A" || *out.Results[1].Message != financeOutOfTimeMsg {
		t.Errorf("after a checked record: %+v, %v", out, err)
	}
}

// slowPayEntity reads a payment slowly and counts pushes.
type slowPayEntity struct {
	*finEntity
	delay  time.Duration
	pushes int
}

func (f *slowPayEntity) FinancePayment(context.Context, string) (engagemententity.FinancePayment, error) {
	time.Sleep(f.delay)
	return f.finEntity.payment, nil
}

func (f *slowPayEntity) FinancePushPayment(context.Context, string, string, engagemententity.FinanceEmployee) (
	engagemententity.FinancePushResult, error) {
	f.pushes++
	return engagemententity.FinancePushResult{OK: true}, nil
}

// When the lookups use up the time reserve, the payment is not pushed: it is
// reported as not started, never as an unknown outcome.
func TestFinancePushPaymentsRechecksTheReserveBeforePushing(t *testing.T) {
	e := &slowPayEntity{finEntity: &finEntity{payment: engagemententity.FinancePayment{PerDiemNumber: job + "-001",
		JobNumber: job, TravelStatus: fsp("In Progress"), TravellerEmail: fsp("t@wso2.com")}}, delay: 300 * time.Millisecond}
	s := NewFinanceService(e, finHR{}, &finMail{}, email.FinanceRecipients{}, nil, time.Now)
	s.batchReserve = 300 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	res, err := s.PushPayments(ctx, perms("f@wso2.com", 40), []string{job + "-001"})
	if err != nil || len(res.Results) != 1 || *res.Results[0].Message != financeOutOfTimeMsg || e.pushes != 0 {
		t.Errorf("res = %+v, err = %v, pushes = %d", res, err, e.pushes)
	}
}

// cancelHR cancels the request while the traveller is looked up.
type cancelHR struct {
	countHR
	cancel context.CancelFunc
}

func (h *cancelHR) Employee(ctx context.Context, e string) (*hrentity.Employee, error) {
	h.cancel()
	return h.countHR.Employee(ctx, e)
}

// A request canceled before the push, with time to spare, is not pushed and
// is reported as not started, never as an unknown outcome.
func TestFinancePushPaymentsStopsWhenCanceledBeforePush(t *testing.T) {
	e := &slowPayEntity{finEntity: &finEntity{payment: engagemententity.FinancePayment{PerDiemNumber: job + "-001",
		JobNumber: job, TravelStatus: fsp("In Progress"), TravellerEmail: fsp("t@wso2.com")}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s := NewFinanceService(e, &cancelHR{cancel: cancel}, &finMail{}, email.FinanceRecipients{}, nil, time.Now)
	_, err := s.PushPayments(ctx, perms("f@wso2.com", 40), []string{job + "-001", job + "-002"})
	if !errors.Is(err, context.Canceled) || e.pushes != 0 {
		t.Errorf("err = %v, pushes = %d", err, e.pushes)
	}
}
