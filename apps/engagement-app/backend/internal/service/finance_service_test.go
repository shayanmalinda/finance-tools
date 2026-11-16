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
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

func fsp(s string) *string { return &s }
func fip(i int) *int       { return &i }

type finEntity struct {
	engagemententity.FinanceAPI
	trip       engagemententity.FinanceTrip
	perDiems   []engagemententity.FinancePerDiem
	basis      engagemententity.FinancePaymentBasis
	invoice    engagemententity.FinanceInvoice
	invoices   []engagemententity.FinanceInvoice
	payment    engagemententity.FinancePayment
	invFilter  engagemententity.FinanceInvoiceFilter
	payFilters []engagemententity.FinancePaymentFilter
	decision   engagemententity.FinanceInvoiceDecision
	decisions  []engagemententity.FinanceDecisionResult
	addedPD    engagemententity.FinancePerDiemCreate
	addedPay   engagemententity.FinancePaymentCreate
	pushErr    error
	pushed     []int
	deleted    bool
	subsidiary string
}

func (f *finEntity) FinanceTrip(context.Context, string) (engagemententity.FinanceTrip, error) {
	return f.trip, nil
}
func (f *finEntity) FinancePerDiems(context.Context, string) ([]engagemententity.FinancePerDiem, error) {
	return f.perDiems, nil
}
func (f *finEntity) FinancePerDiem(_ context.Context, _ string, id int) (engagemententity.FinancePerDiem, error) {
	for _, p := range f.perDiems {
		if p.ID == id {
			return p, nil
		}
	}
	return engagemententity.FinancePerDiem{}, &apierror.UpstreamError{StatusCode: 404, Msg: "Per diem not found."}
}
func (f *finEntity) FinanceAddPerDiem(_ context.Context, jn string, in engagemententity.FinancePerDiemCreate) (engagemententity.FinancePerDiem, error) {
	f.addedPD = in
	return engagemententity.FinancePerDiem{ID: 1, JobNumber: jn, Currency: &in.Currency, PerDiemValue: in.PerDiemValue,
		Total: in.Total, StartDate: &in.StartDate, EndDate: &in.EndDate, NoOfDays: &in.NoOfDays}, nil
}
func (f *finEntity) FinanceDeletePerDiem(context.Context, string, int) error {
	f.deleted = true
	return nil
}
func (f *finEntity) FinancePaymentBasis(context.Context, string) (engagemententity.FinancePaymentBasis, error) {
	return f.basis, nil
}
func (f *finEntity) FinanceInvoices(_ context.Context, fl engagemententity.FinanceInvoiceFilter) ([]engagemententity.FinanceInvoice, int, error) {
	f.invFilter = fl
	return f.invoices, len(f.invoices), nil
}
func (f *finEntity) FinanceInvoiceCounts(context.Context, int) (engagemententity.FinanceInvoiceCounts, error) {
	return engagemententity.FinanceInvoiceCounts{Pending: 1, AcceptedByTravel: 2, AcceptedByFinance: 3, Failed: 4, All: 10}, nil
}
func (f *finEntity) FinanceInvoice(context.Context, int) (engagemententity.FinanceInvoice, error) {
	return f.invoice, nil
}
func (f *finEntity) FinanceInvoiceReference(context.Context, int) (*string, error) {
	return fsp("5501"), nil
}
func (f *finEntity) FinanceDecideInvoices(_ context.Context, d engagemententity.FinanceInvoiceDecision) ([]engagemententity.FinanceDecisionResult, error) {
	f.decision = d
	return f.decisions, nil
}
func (f *finEntity) FinancePushInvoice(_ context.Context, id int, _ string) (engagemententity.FinancePushResult, error) {
	f.pushed = append(f.pushed, id)
	if f.pushErr != nil {
		return engagemententity.FinancePushResult{}, f.pushErr
	}
	return engagemententity.FinancePushResult{OK: true, Outcome: "pushed", Reference: fsp("5501")}, nil
}
func (f *finEntity) FinancePayments(_ context.Context, fl engagemententity.FinancePaymentFilter) ([]engagemententity.FinancePayment, int, error) {
	f.payFilters = append(f.payFilters, fl)
	return []engagemententity.FinancePayment{f.payment}, 1, nil
}
func (f *finEntity) FinancePaymentCounts(context.Context) (engagemententity.FinancePaymentCounts, error) {
	return engagemententity.FinancePaymentCounts{Pending: 2, Failed: 1, All: 9}, nil
}
func (f *finEntity) FinancePayment(context.Context, string) (engagemententity.FinancePayment, error) {
	return f.payment, nil
}
func (f *finEntity) FinanceAddPayment(_ context.Context, jn string, in engagemententity.FinancePaymentCreate) (engagemententity.FinancePayment, error) {
	f.addedPay = in
	return engagemententity.FinancePayment{JobNumber: jn, PerDiemNumber: in.PerDiemNumber, NSStatus: fsp("Pending"),
		PaymentAmount: &in.PaymentAmount, PaymentCurrency: &in.PaymentCurrency}, nil
}
func (f *finEntity) FinancePushPayment(_ context.Context, n, _ string, emp engagemententity.FinanceEmployee) (engagemententity.FinancePushResult, error) {
	if emp.Company == "" {
		return engagemententity.FinancePushResult{}, errors.New("no company")
	}
	return engagemententity.FinancePushResult{OK: true, Reference: fsp("7701")}, nil
}
func (f *finEntity) FinanceSubsidiary(context.Context, string) (engagemententity.FinanceSubsidiary, error) {
	return engagemententity.FinanceSubsidiary{CurrencyCode: f.subsidiary}, nil
}

type finHR struct{ hrentity.Client }

func (finHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	return &hrentity.Employee{WorkEmail: e, EmployeeID: "LK1", Company: fsp("WSO2 - UK")}, nil
}

type finMail struct{ sent []email.Message }

func (m *finMail) Send(_ context.Context, msg email.Message) error {
	m.sent = append(m.sent, msg)
	return errors.New("email service down") // must never fail the action
}

type finFX struct{ calls int }

func (f *finFX) Rates(_ context.Context, base, date string) (map[string]float64, error) {
	f.calls++
	return map[string]float64{"LKR": 0.0025, "USD": 0.8}, nil
}

func perms(email string, travel ...int) domain.Perms {
	return domain.Perms{Email: email, Travel: domain.NewPrivilegeSet(travel), TravelOwn: domain.NewPrivilegeSet(travel)}
}

func newFin(e *finEntity, fx FinanceCurrencyConverter) (*FinanceService, *finMail) {
	m := &finMail{}
	s := NewFinanceService(e, finHR{}, m, email.FinanceRecipients{TravelLogisticsGroup: "logistics@wso2.com", FinanceGroup: "fin@wso2.com"},
		fx, func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) })
	return s, m
}

func openTrip() engagemententity.FinanceTrip {
	return engagemententity.FinanceTrip{JobNumber: "TRV-2026-11-0001", Status: fsp("In Progress"), TravellerEmail: "jane@wso2.com", AgencyID: fip(4)}
}

const job = "TRV-2026-11-0001"

func isForbidden(err error) bool { return errors.As(err, new(*apierror.ForbiddenError)) }

func TestFinancePerDiemsScope(t *testing.T) {
	ctx := context.Background()
	s, _ := newFin(&finEntity{trip: openTrip()}, nil)
	if _, err := s.PerDiems(ctx, perms("jane@wso2.com", 37), job); err != nil {
		t.Errorf("traveller with 37: %v", err)
	}
	if _, err := s.PerDiems(ctx, perms("other@wso2.com", 37), job); !isForbidden(err) {
		t.Errorf("other with 37: %v", err)
	}
	if _, err := s.PerDiems(ctx, perms("fin@wso2.com", 28), job); err != nil {
		t.Errorf("28: %v", err)
	}
	agent := perms("a@agency.com", 18, 28)
	agent.Agency = &domain.TravelAgency{ID: 9}
	if _, err := s.PerDiems(ctx, agent, job); !isForbidden(err) {
		t.Errorf("agent of another agency: %v", err)
	}
	agent.Agency.ID = 4
	if _, err := s.PerDiems(ctx, agent, job); err != nil {
		t.Errorf("agent of the trip's agency: %v", err)
	}
	if _, err := s.TravelCards(ctx, agent, job); !isForbidden(err) {
		t.Errorf("agents never see travel cards: %v", err)
	}
	if _, err := s.PerDiems(ctx, perms("x@wso2.com", 28), "TRV-1"); !errors.As(err, new(*apierror.ValidationError)) {
		t.Errorf("bad job number: %v", err)
	}
}

func TestFinanceAddPerDiem(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{trip: openTrip()}
	s, mail := newFin(e, nil)
	pd, err := s.AddPerDiem(ctx, perms("po@wso2.com", 26), job, domain.PerDiemCreate{
		StartDate: "2026-11-06", StartTime: "13:00", EndDate: "2026-11-16", EndTime: "18:00", Currency: "gbp", Rate: 70})
	if err != nil {
		t.Fatal(err)
	}
	// 11 days, half first day: (0.5 + 1 + 9) × 70 = 735.
	if e.addedPD.NoOfDays != 11 || e.addedPD.Total != 735 || e.addedPD.Currency != "GBP" ||
		e.addedPD.StartDate != "2026-11-06 13:00:00" || pd.StartTime != "13:00" || pd.Days != 11 {
		t.Errorf("added %+v, returned %+v", e.addedPD, pd)
	}
	if len(mail.sent) != 1 || mail.sent[0].Content["title"] != "New per diems have been added" ||
		mail.sent[0].Subject != "[Travel App] Travel Request (#"+job+")" {
		t.Errorf("mail = %+v", mail.sent)
	}

	e.perDiems = []engagemententity.FinancePerDiem{{ID: 1, Currency: fsp("USD")}}
	if _, err := s.AddPerDiem(ctx, perms("po@wso2.com", 26), job, domain.PerDiemCreate{
		StartDate: "2026-11-06", StartTime: "09:00", EndDate: "2026-11-07", EndTime: "09:00", Currency: "GBP", Rate: 70}); err == nil ||
		!strings.Contains(err.Error(), "Currency mismatch") {
		t.Errorf("currency mismatch: %v", err)
	}
	e.trip.Status = fsp("Completed")
	if _, err := s.AddPerDiem(ctx, perms("po@wso2.com", 26), job, domain.PerDiemCreate{}); err == nil {
		t.Error("a closed trip must refuse")
	}
}

func TestFinanceDeletePerDiemProcessed(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{trip: openTrip(),
		perDiems: []engagemententity.FinancePerDiem{{ID: 5, Currency: fsp("GBP"), Total: 300}},
		basis: engagemententity.FinancePaymentBasis{PaymentCount: 1,
			PerDiemTotals: []engagemententity.FinancePerDiemTotal{{Currency: "GBP", TotalAmount: 700}},
			Payments:      []engagemententity.FinanceAmount{{Amount: 450, Currency: "GBP", AddedDate: "2026-11-02 10:00:00"}}},
	}
	s, _ := newFin(e, nil)
	if err := s.DeletePerDiem(ctx, perms("po@wso2.com", 27), job, 5); err == nil || err.Error() != FinanceDeleteProcessedMsg || e.deleted {
		t.Errorf("processed: %v, deleted %v", err, e.deleted)
	}
	e.basis.Payments[0].Amount = 100
	if err := s.DeletePerDiem(ctx, perms("po@wso2.com", 27), job, 5); err != nil || !e.deleted {
		t.Errorf("allowed: %v", err)
	}
	// A conversion is needed and the converter is not configured: 503.
	e.basis.Payments[0].Currency = "USD"
	if err := s.DeletePerDiem(ctx, perms("po@wso2.com", 27), job, 5); !errors.As(err, new(*apierror.ServiceUnavailableError)) {
		t.Errorf("no converter: %v", err)
	}
}

func TestFinanceAddPayment(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{trip: openTrip(), subsidiary: "LKR",
		basis: engagemententity.FinancePaymentBasis{PaymentCount: 2,
			PerDiemTotals: []engagemententity.FinancePerDiemTotal{{Currency: "GBP", TotalAmount: 700}},
			TravelCards:   []engagemententity.FinanceAmount{{Amount: 100, Currency: "GBP"}},
			Payments:      []engagemententity.FinanceAmount{{Amount: 200, Currency: "GBP"}, {Amount: 100, Currency: "GBP"}}},
	}
	if _, err := func() (domain.PerDiemPayment, error) {
		s, _ := newFin(e, nil)
		return s.AddPayment(ctx, perms("f@wso2.com", 43), job, 10)
	}(); !errors.As(err, new(*apierror.ServiceUnavailableError)) {
		t.Errorf("no converter: %v", err)
	}
	fx := &finFX{}
	s, _ := newFin(e, fx)
	if _, err := s.AddPayment(ctx, perms("f@wso2.com", 43), job, 301); err == nil || err.Error() != FinanceInsufficientMsg {
		t.Errorf("over balance: %v", err)
	}
	pay, err := s.AddPayment(ctx, perms("f@wso2.com", 43), job, 300)
	if err != nil {
		t.Fatal(err)
	}
	// GBP → LKR: amount / rate (the legacy divides).
	if e.addedPay.PerDiemNumber != job+"-003" || e.addedPay.ConvertedCurrency != "LKR" ||
		math.Abs(e.addedPay.ConvertedAmount-120000) > 1e-6 || pay.ID != job+"-003" {
		t.Errorf("added %+v", e.addedPay)
	}
	e.basis.PerDiemTotals = append(e.basis.PerDiemTotals, engagemententity.FinancePerDiemTotal{Currency: "USD", TotalAmount: 1})
	if _, err := s.AddPayment(ctx, perms("f@wso2.com", 43), job, 1); err == nil || err.Error() != FinanceMultiCurrencyMsg {
		t.Errorf("multi currency: %v", err)
	}
}

// A pushed invoice carries its stored purchase order id.
func TestFinanceInvoicePushedReference(t *testing.T) {
	s, _ := newFin(&finEntity{}, nil)
	ref := "5501"
	inv := s.invoiceOf(perms("f@wso2.com", 34), engagemententity.FinanceInvoice{ID: 1, Status: "Complete", NetSuiteReference: &ref})
	if inv.NetSuite.Status != domain.NetSuitePushed || inv.NetSuite.Reference == nil || *inv.NetSuite.Reference != "5501" {
		t.Errorf("netsuite = %+v", inv.NetSuite)
	}
}

func TestFinanceInvoiceTabs(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{invoices: []engagemententity.FinanceInvoice{{ID: 1, Status: "Accepted by travel"}}}
	s, _ := newFin(e, nil)
	fin := perms("f@wso2.com", 33, 34)
	page, err := s.Invoices(ctx, fin, domain.InvoiceQuery{Tab: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.invFilter.Statuses) != 1 || e.invFilter.Statuses[0] != "Accepted by travel" || !page.Items[0].Actions.Approve ||
		page.TabCounts["approve"] != 2 || page.TabCounts["push"] != 3 {
		t.Errorf("page = %+v, filter %+v", page, e.invFilter)
	}
	// A tab the caller does not have: no rows, their own counts only.
	e.invFilter = engagemententity.FinanceInvoiceFilter{}
	page, err = s.Invoices(ctx, fin, domain.InvoiceQuery{Tab: "approve-po"})
	if err != nil || len(page.Items) != 0 || e.invFilter.Statuses != nil || page.TabCounts["approve"] != 2 {
		t.Errorf("approve-po without 32: %+v %v", page, err)
	}
	if _, has := page.TabCounts["approve-po"]; has {
		t.Errorf("count of a tab the caller does not have: %v", page.TabCounts)
	}
	// The super admin's grants make no queue.
	super := perms("s@wso2.com", 32, 33, 34, 39)
	super.TravelOwn = domain.NewPrivilegeSet(nil)
	if page, err := s.Invoices(ctx, super, domain.InvoiceQuery{Tab: "push"}); err != nil || len(page.Items) != 0 || page.TabCounts["push"] != 0 {
		t.Errorf("push queue from super admin grants: %+v %v", page, err)
	}
	if _, err := s.Invoices(ctx, super, domain.InvoiceQuery{Tab: "all", PoApproval: []string{"Rejected"}, NetSuiteStatus: []string{"Pending"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(e.invFilter.Statuses, ",") != "Rejected by travel" {
		t.Errorf("status filter = %v", e.invFilter.Statuses)
	}
	if _, err := s.Invoices(ctx, fin, domain.InvoiceQuery{Tab: "push"}); err != nil || e.invFilter.Failed != "false" {
		t.Errorf("push tab must leave out NS Sync Failures: %v %+v", err, e.invFilter)
	}
}

func TestFinanceDecideHRWinsAndOneEmail(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{trip: openTrip(), decisions: []engagemententity.FinanceDecisionResult{
		{ID: 1, OK: true, JobNumber: fsp("TRV-2026-11-0001")}, {ID: 2, OK: true, JobNumber: fsp("TRV-2026-11-0002")},
		{ID: 3, Message: fsp("Problem in Travel Invoice Status")}}}
	s, mail := newFin(e, nil)
	res, err := s.ApproveInvoices(ctx, perms("s@wso2.com", 32, 33), []int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if e.decision.Step != "travel" || len(res.Results) != 3 || res.Results[2].OK {
		t.Errorf("decision %+v, res %+v", e.decision, res)
	}
	if len(mail.sent) != 1 || mail.sent[0].ReferenceID != "TRV-2026-11-0002" || mail.sent[0].Content["title"] != "Travel invoices have been approved" {
		t.Errorf("mail = %+v", mail.sent)
	}
	if _, err := s.RejectInvoices(ctx, perms("f@wso2.com", 33), []int{1}, " "); err == nil {
		t.Error("reject needs a reason")
	}
	if _, err := s.RejectInvoices(ctx, perms("f@wso2.com", 33), []int{1}, "Wrong amount"); err != nil || e.decision.Step != "finance" || e.decision.Reason != "Wrong amount" {
		t.Errorf("finance reject: %v %+v", err, e.decision)
	}
}

func TestFinancePushInvoices(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{}
	s, _ := newFin(e, nil)
	res, err := s.PushInvoices(ctx, perms("f@wso2.com", 34), []int{1, 1, 2})
	if err != nil || len(res.Results) != 2 || !res.Results[0].OK || *res.Results[0].NetSuiteReference != "5501" {
		t.Errorf("res = %+v, %v", res, err)
	}
	e.pushErr = &apierror.UpstreamError{StatusCode: 503, Msg: "service temporarily unavailable"}
	if _, err := s.PushInvoices(ctx, perms("f@wso2.com", 34), []int{1}); !errors.As(err, new(*apierror.ServiceUnavailableError)) {
		t.Errorf("netsuite off: %v", err)
	}
	e.pushErr, e.pushed = nil, nil
	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	res, _ = s.PushInvoices(short, perms("f@wso2.com", 34), []int{1, 2})
	if len(e.pushed) != 0 || res.Results[0].OK || *res.Results[0].Message != financeOutOfTimeMsg {
		t.Errorf("out of time: pushed %v, res %+v", e.pushed, res)
	}
}

func TestFinancePaymentsAndFailures(t *testing.T) {
	ctx := context.Background()
	e := &finEntity{payment: engagemententity.FinancePayment{PerDiemNumber: job + "-001", JobNumber: job,
		NSStatus: fsp("Failed"), TravelStatus: fsp("In Progress"), TravellerEmail: fsp("jane@wso2.com"), PaymentAmount: new(float64)}}
	s, _ := newFin(e, nil)
	page, err := s.Payments(ctx, perms("f@wso2.com", 40, 41), domain.PaymentQuery{Tab: "all", NetSuiteStatus: []string{"Pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(e.payFilters[0].NSStatuses, ",") != "Pending,Failed" || !page.Items[0].Actions.Resync ||
		page.Items[0].NetSuite.Status != "Failed" || page.Items[0].Status != "Pending" || page.TabCounts["push"] != 2 {
		t.Errorf("page = %+v, filter = %+v", page, e.payFilters)
	}
	sf, err := s.SyncFailures(ctx, perms("f@wso2.com", 40), "")
	if err != nil || sf.Total != 1 || sf.Items[0].Record != "payment" {
		t.Errorf("sync failures = %+v, %v", sf, err)
	}
	if _, err := s.Payment(ctx, perms("other@wso2.com", 47), job+"-001"); !isForbidden(err) {
		t.Errorf("own payment of another traveller: %v", err)
	}
	res, err := s.PushPayments(ctx, perms("f@wso2.com", 40), []string{job + "-001"})
	if err != nil || !res.Results[0].OK {
		t.Errorf("push = %+v, %v", res, err)
	}
	e.payment.TravelStatus = fsp("Completed")
	res, _ = s.PushPayments(ctx, perms("f@wso2.com", 40), []string{job + "-001"})
	if res.Results[0].OK || *res.Results[0].Message != financeTripNotOpenMsg {
		t.Errorf("closed trip pushed: %+v", res)
	}
}

func TestFinanceRecipients(t *testing.T) {
	r := email.FinanceRecipients{TravelLogisticsGroup: "log@wso2.com", FinanceGroup: "fin@wso2.com", AppBaseURL: "https://one.wso2.com/",
		CompanyRecipients: map[string][]string{"WSO2 - UK": {"uk1@wso2.com", "ukpay@wso2.com"}, "WSO2 Inc": {"us@wso2.com"}}}
	m := email.FinanceTravelMessage(r, email.FinanceInvoicesAdded, job, " wso2 - uk ", "invoices")
	if strings.Join(m.To, ",") != "log@wso2.com,uk1@wso2.com" || strings.Join(m.CC, ",") != "ukpay@wso2.com" ||
		m.Content["action_link"] != "https://one.wso2.com/finance/engagement-app/travel-requests/"+job+"/invoices" || m.TemplateID != "noContent" {
		t.Errorf("uk = %+v", m)
	}
	m = email.FinanceTravelMessage(r, email.FinanceInvoicesAdded, job, "WSO2 Inc", "invoices")
	if strings.Join(m.To, ",") != "log@wso2.com,us@wso2.com" || strings.Join(m.CC, ",") != "fin@wso2.com" {
		t.Errorf("single address = %+v", m)
	}
	m = email.FinanceTravelMessage(r, email.FinanceInvoicesAdded, job, "", "invoices")
	if strings.Join(m.To, ",") != "log@wso2.com,fin@wso2.com" || len(m.CC) != 0 {
		t.Errorf("no company = %+v", m)
	}
}

// Every sent finance email writes its email_auditlog row when an audit hook
// is set (the Travel app did for every email).
func TestFinanceEmailAudit(t *testing.T) {
	e := &finEntity{trip: openTrip()}
	s := NewFinanceService(e, finHR{}, email.LogSender{}, email.FinanceRecipients{TravelLogisticsGroup: "logistics@wso2.com"}, nil, nil)
	var audited []email.Message
	s.WithEmailAudit(func(_ context.Context, m email.Message) { audited = append(audited, m) })
	if _, err := s.AddPerDiem(context.Background(), perms("po@wso2.com", 26), job, domain.PerDiemCreate{
		StartDate: "2026-11-06", StartTime: "09:00", EndDate: "2026-11-07", EndTime: "18:00", Currency: "GBP", Rate: 70}); err != nil {
		t.Fatal(err)
	}
	if len(audited) != 1 || audited[0].Subject != "[Travel App] Travel Request (#"+job+")" {
		t.Errorf("audited = %+v", audited)
	}
}

// A failed send writes no audit row.
func TestFinanceEmailAuditSkippedWhenSendFails(t *testing.T) {
	s, _ := newFin(&finEntity{trip: openTrip()}, nil)
	called := false
	s.WithEmailAudit(func(context.Context, email.Message) { called = true })
	_, _ = s.AddPerDiem(context.Background(), perms("po@wso2.com", 26), job, domain.PerDiemCreate{
		StartDate: "2026-11-06", StartTime: "09:00", EndDate: "2026-11-07", EndTime: "18:00", Currency: "GBP", Rate: 70})
	if called {
		t.Error("audited an email that was not sent")
	}
}

func TestFinancePushingState(t *testing.T) {
	s, _ := newFin(&finEntity{}, nil)
	p := perms("f@wso2.com", 34, 40)
	inv := s.invoiceOf(p, engagemententity.FinanceInvoice{Status: "Complete", NetSuitePushing: true})
	if inv.NetSuite.Status != domain.NetSuitePushing || inv.Actions.Push || inv.Actions.Resync {
		t.Errorf("invoice = %+v", inv)
	}
	pay := s.paymentOf(p, engagemententity.FinancePayment{NSStatus: fsp("Pushing"), TravelStatus: fsp("In Progress")})
	if pay.NetSuite.Status != domain.NetSuitePushing || pay.Status != "Complete" || pay.Actions.Push || pay.Actions.Edit {
		t.Errorf("payment = %+v", pay)
	}
}
