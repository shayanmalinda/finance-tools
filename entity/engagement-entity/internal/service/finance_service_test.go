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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
)

func fp[T any](v T) *T { return &v }

type fakeFinanceRepo struct {
	repository.FinanceRepository
	invoice     *domain.FinanceInvoice
	payment     *domain.FinancePayment
	claimWon    bool
	calls       []string
	failMsg     string
	paymentN    int
	exists      bool
	classByCode *int
	classBySuf  *int
	inserted    []repository.FinanceInvoiceInsert
	pushes      []audit.Event
}

func (f *fakeFinanceRepo) log(s string) { f.calls = append(f.calls, s) }

func (f *fakeFinanceRepo) AuditPush(_ context.Context, e audit.Event) error {
	f.pushes = append(f.pushes, e)
	return nil
}

func (f *fakeFinanceRepo) Trip(_ context.Context, jn string) (*domain.FinanceTrip, error) {
	return &domain.FinanceTrip{JobNumber: jn, TravellerEmail: "jane@wso2.com", AllocationID: fp("A1"), CustomerName: fp("Travel Customer"),
		EngagementCode: fp("STALE")}, nil
}
func (f *fakeFinanceRepo) Invoice(context.Context, int) (*domain.FinanceInvoice, error) {
	if f.invoice == nil {
		return nil, nil
	}
	c := *f.invoice
	return &c, nil
}
func (f *fakeFinanceRepo) ClaimInvoicePush(context.Context, int, string) (bool, error) {
	f.log("claim")
	return f.claimWon, nil
}
func (f *fakeFinanceRepo) ReleaseInvoicePush(context.Context, int, string) error {
	f.log("release")
	return nil
}
func (f *fakeFinanceRepo) FailInvoicePush(_ context.Context, _ int, _, msg string) error {
	f.log("fail")
	f.failMsg = msg
	return nil
}
func (f *fakeFinanceRepo) ResolveInvoiceFailure(_ context.Context, _ int, _, ref string) (bool, error) {
	f.log("resolve:" + map[bool]string{true: "found " + ref, false: "notfound"}[ref != ""])
	return true, nil
}
func (f *fakeFinanceRepo) CompleteInvoicePush(_ context.Context, _ int, ref string) error {
	f.log("ref " + ref)
	return nil
}
func (f *fakeFinanceRepo) Payment(context.Context, string) (*domain.FinancePayment, error) {
	if f.payment == nil {
		return nil, nil
	}
	c := *f.payment
	return &c, nil
}
func (f *fakeFinanceRepo) ClaimPaymentPush(context.Context, int, string) (bool, error) {
	f.log("claim")
	return f.claimWon, nil
}
func (f *fakeFinanceRepo) FailPaymentPush(context.Context, int, string) error {
	f.log("fail")
	return nil
}
func (f *fakeFinanceRepo) CompletePaymentPush(context.Context, int, string) error {
	f.log("complete")
	return nil
}
func (f *fakeFinanceRepo) CostTypeIDByName(context.Context, string) (*int, error) { return fp(6), nil }
func (f *fakeFinanceRepo) ClassificationID(_ context.Context, code, suffix string, _ *int) (*int, error) {
	if code != "" {
		return f.classByCode, nil
	}
	return f.classBySuf, nil
}
func (f *fakeFinanceRepo) ClassificationData(context.Context, int) (*string, *string, error) {
	return fp("Finance"), fp("6001"), nil
}
func (f *fakeFinanceRepo) CostTypes(context.Context) ([]domain.FinanceCostType, error) {
	return []domain.FinanceCostType{{ID: 1, Name: "Air Ticket"}, {ID: 5, Name: "Travel Insurance"}, {ID: 6, Name: "Currency"}}, nil
}
func (f *fakeFinanceRepo) InvoiceNumberExists(context.Context, string) (bool, error) {
	return f.exists, nil
}
func (f *fakeFinanceRepo) PaymentCount(context.Context, string) (int, error) { return f.paymentN, nil }
func (f *fakeFinanceRepo) InsertInvoices(_ context.Context, _, _ string, rows []repository.FinanceInvoiceInsert) ([]int, error) {
	f.inserted = rows
	return []int{1}, nil
}

type fakeFinanceAlloc struct {
	repository.FinanceAllocationRepository
}

func (fakeFinanceAlloc) EngagementOfAllocation(context.Context, string) (*domain.FinanceEngagement, error) {
	return &domain.FinanceEngagement{ID: "E1", EngagementCode: "PS-ACME-1", CustomerName: fp("Acme")}, nil
}
func (fakeFinanceAlloc) FundingSources(context.Context, string) ([]domain.FinanceFundingSource, error) {
	return []domain.FinanceFundingSource{{BusinessUnit: fp("BU"), SubRegion: fp("UK"), Region: fp("Europe"), Percentage: "100.00"}}, nil
}
func (fakeFinanceAlloc) Campaigns(context.Context, []string) (map[string]domain.FinanceCampaign, error) {
	return map[string]domain.FinanceCampaign{}, nil
}

type fakeFinanceExpense struct{}

func (fakeFinanceExpense) Subsidiary(_ context.Context, c string) (*domain.FinanceSubsidiary, error) {
	if c != "WSO2 - UK" {
		return nil, nil
	}
	return &domain.FinanceSubsidiary{LegalName: "WSO2 UK Ltd", CurrencyCode: "GBP", TaxCodeInternalID: "12"}, nil
}

type fakeNetSuite struct {
	enabled   bool
	found     []netsuite.Record
	lookupErr error
	createErr error
	po        *netsuite.PurchaseOrder
	vb        *netsuite.VendorBill
	created   int
}

func (f *fakeNetSuite) Enabled() bool                { return f.enabled }
func (f *fakeNetSuite) PurchaseOrderTaxCode() string { return "7" }
func (f *fakeNetSuite) LookupPurchaseOrders(context.Context, string) ([]netsuite.Record, error) {
	return f.found, f.lookupErr
}
func (f *fakeNetSuite) LookupVendorBills(context.Context, string) ([]netsuite.Record, error) {
	return f.found, f.lookupErr
}
func (f *fakeNetSuite) CreatePurchaseOrder(_ context.Context, po netsuite.PurchaseOrder) (string, error) {
	f.created++
	f.po = &po
	return "5501", f.createErr
}
func (f *fakeNetSuite) CreateVendorBill(_ context.Context, vb netsuite.VendorBill) (string, error) {
	f.created++
	f.vb = &vb
	return "7701", f.createErr
}

func approvedInvoice() *domain.FinanceInvoice {
	return &domain.FinanceInvoice{ID: 7, InvoiceNumber: "INV-1", JobNumber: "TRV-2026-11-0001", Status: "Accepted by finance",
		InvoiceCurrency: "USD", InvoiceAmountText: "155100.00", ConvertedCurrency: "USD", ConvertedAmountText: "155100.00",
		VendorID: fp(2339), Department: fp("Sales"), AccountInternalID: fp("6001"), CostTypeName: fp("Currency"),
		FinanceApprovedBy: fp("fin@wso2.com"), AddedDate: fp("2026-11-04 10:00:00")}
}

func newFinanceSvc(repo *fakeFinanceRepo, ns *fakeNetSuite) *financeService {
	s := NewFinanceService(repo, fakeFinanceAlloc{}, fakeFinanceExpense{}, ns).(*financeService)
	s.now = func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) }
	return s
}

func TestPushInvoice_PushesAndBuildsTheLegacyPayload(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice(), claimWon: true}
	ns := &fakeNetSuite{enabled: true}
	res, err := newFinanceSvc(repo, ns).PushInvoice(context.Background(), 7, "p@wso2.com")
	if err != nil || !res.OK || res.Outcome != domain.NSOutcomePushed || *res.Reference != "5501" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	po := ns.po
	if po.Memo != "Currency - jane@wso2.com - (USD 155100.00) - Acme - TRV-2026-11-0001" || po.TransactionDate != "2026-11-04" ||
		po.VendorID != "2339" || po.TaxCode != "7" || po.POType != "Travel-PO" || po.EngagementCode != "PS-ACME-1" ||
		po.TotalAmount != "155100.00" || *po.FundingSources[0].Region != "UK" {
		t.Errorf("po = %+v", po)
	}
	if strings.Join(repo.calls, ",") != "claim,ref 5501" {
		t.Errorf("calls = %v", repo.calls)
	}
	if len(repo.pushes) != 1 || repo.pushes[0].Action != audit.Push || repo.pushes[0].EntityID != "7" ||
		repo.pushes[0].Metadata["outcome"] != domain.NSOutcomePushed || repo.pushes[0].Metadata["reference"] != "5501" {
		t.Errorf("pushes = %+v", repo.pushes)
	}
}

func TestPushInvoice_LookupFoundDoesNotPushAgain(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice(), claimWon: true}
	ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "4400"}}}
	res, _ := newFinanceSvc(repo, ns).PushInvoice(context.Background(), 7, "p@wso2.com")
	if !res.OK || res.Outcome != domain.NSOutcomeFound || *res.Reference != "4400" || ns.created != 0 ||
		strings.Join(repo.calls, ",") != "claim,ref 4400" {
		t.Errorf("res = %+v, created = %d, calls = %v", res, ns.created, repo.calls)
	}
}

func TestPushInvoice_OnePushInFlight(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice(), claimWon: false}
	ns := &fakeNetSuite{enabled: true}
	res, _ := newFinanceSvc(repo, ns).PushInvoice(context.Background(), 7, "p@wso2.com")
	if res.OK || res.Outcome != domain.NSOutcomeSkipped || ns.created != 0 || len(repo.pushes) != 0 {
		t.Errorf("res = %+v, pushes = %+v", res, repo.pushes)
	}
}

func TestPushInvoice_FailureIsMarked(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice(), claimWon: true}
	ns := &fakeNetSuite{enabled: true, createErr: errors.New("No status returned from NetSuite")}
	res, _ := newFinanceSvc(repo, ns).PushInvoice(context.Background(), 7, "p@wso2.com")
	if res.OK || res.Outcome != domain.NSOutcomeFailed || strings.Join(repo.calls, ",") != "claim,fail" || repo.failMsg == "" {
		t.Errorf("res = %+v, calls = %v", res, repo.calls)
	}
	if len(repo.pushes) != 1 || repo.pushes[0].Metadata["outcome"] != domain.NSOutcomeFailed {
		t.Errorf("pushes = %+v", repo.pushes)
	}
}

func TestPushInvoice_LookupErrorReleases(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice(), claimWon: true}
	ns := &fakeNetSuite{enabled: true, lookupErr: errors.New("down")}
	res, _ := newFinanceSvc(repo, ns).PushInvoice(context.Background(), 7, "p@wso2.com")
	if res.OK || strings.Join(repo.calls, ",") != "claim,release" || ns.created != 0 {
		t.Errorf("res = %+v, calls = %v", res, repo.calls)
	}
}

func TestPushInvoice_Guards(t *testing.T) {
	ctx := context.Background()
	if _, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).PushInvoice(ctx, 7, "p@wso2.com"); !errors.As(err, new(*apierror.ServiceUnavailableError)) {
		t.Errorf("disabled: err = %v", err)
	}
	lkr := approvedInvoice()
	lkr.ConvertedCurrency = "LKR"
	failed := approvedInvoice()
	failed.NetSuiteFailed = true
	noVendor := approvedInvoice()
	noVendor.VendorID = nil
	for name, inv := range map[string]*domain.FinanceInvoice{"lkr": lkr, "failed": failed, "no vendor": noVendor} {
		repo := &fakeFinanceRepo{invoice: inv, claimWon: true}
		res, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).PushInvoice(ctx, 7, "p@wso2.com")
		if err != nil || res.OK || len(repo.calls) != 0 {
			t.Errorf("%s: res = %+v, calls = %v, err = %v", name, res, repo.calls, err)
		}
	}
}

func TestResyncInvoice(t *testing.T) {
	inv := approvedInvoice()
	inv.NetSuiteFailed = true
	repo := &fakeFinanceRepo{invoice: inv}
	ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "4400"}}}
	res, _ := newFinanceSvc(repo, ns).ResyncInvoice(context.Background(), 7, "p@wso2.com")
	if !res.OK || *res.Reference != "4400" || repo.calls[0] != "resolve:found 4400" || ns.created != 0 {
		t.Errorf("found: res = %+v, calls = %v", res, repo.calls)
	}
	repo = &fakeFinanceRepo{invoice: inv}
	res, _ = newFinanceSvc(repo, &fakeNetSuite{enabled: true}).ResyncInvoice(context.Background(), 7, "p@wso2.com")
	if res.OK || res.Outcome != domain.NSOutcomeNotFound || repo.calls[0] != "resolve:notfound" {
		t.Errorf("not found: res = %+v, calls = %v", res, repo.calls)
	}
	repo = &fakeFinanceRepo{invoice: approvedInvoice()}
	res, _ = newFinanceSvc(repo, &fakeNetSuite{enabled: true}).ResyncInvoice(context.Background(), 7, "p@wso2.com")
	if res.OK || len(repo.calls) != 0 {
		t.Errorf("not failed: res = %+v", res)
	}
}

func pendingPayment() *domain.FinancePayment {
	return &domain.FinancePayment{ID: 3, JobNumber: "TRV-2026-11-0001", PerDiemNumber: "TRV-2026-11-0001-001",
		PaymentCurrency: fp("GBP"), PaymentAmount: fp(400.0), NSStatus: fp("Pending"),
		ConvertedCurrency: fp("LKR"), ConvertedAmount: fp(152000.5), ConvertedAmountText: fp("152000.50")}
}

func TestPushPayment_VendorBillPayload(t *testing.T) {
	repo := &fakeFinanceRepo{payment: pendingPayment(), claimWon: true, classBySuf: fp(9)}
	ns := &fakeNetSuite{enabled: true}
	emp := &domain.FinanceEmployee{EmployeeID: "LK100", Email: "jane@wso2.com", FirstName: "Jane", LastName: "Doe",
		JobRole: "Engineer", Department: "Eng", Company: "WSO2 - UK"}
	res, err := newFinanceSvc(repo, ns).PushPayment(context.Background(), "TRV-2026-11-0001-001",
		domain.FinancePushRequest{UpdatedBy: "p@wso2.com", Employee: emp})
	if err != nil || !res.OK || *res.Reference != "7701" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	vb := ns.vb
	wantMemo := "Per Diem for jane@wso2.com - GBP 400.0 - TRV-2026-11-0001 - \n    Travel Customer - PS-ACME-1"
	if vb.Memo != wantMemo || vb.TransactionDate != "2026-11-5" || vb.TotalAmount != "152000.50" || vb.BillCurrency != "LKR" ||
		vb.TaxCode != "12" || vb.Employee.Subsidiary != "WSO2 UK Ltd" || vb.Account != "6001" {
		t.Errorf("vb = %+v", vb)
	}
}

func TestPushPayment_Guards(t *testing.T) {
	ctx := context.Background()
	emp := &domain.FinanceEmployee{Company: "Unknown Co"}
	repo := &fakeFinanceRepo{payment: pendingPayment(), claimWon: true, classByCode: fp(9)}
	res, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).PushPayment(ctx, "TRV-2026-11-0001-001",
		domain.FinancePushRequest{UpdatedBy: "p@wso2.com", Employee: emp})
	if err != nil || res.OK || !strings.Contains(*res.Message, "No Subsidiary") || len(repo.calls) != 0 {
		t.Errorf("no subsidiary: res = %+v, err = %v", res, err)
	}
	failed := pendingPayment()
	failed.NSStatus = fp("Failed")
	repo = &fakeFinanceRepo{payment: failed, claimWon: true}
	res, _ = newFinanceSvc(repo, &fakeNetSuite{enabled: true}).PushPayment(ctx, "TRV-2026-11-0001-001",
		domain.FinancePushRequest{UpdatedBy: "p@wso2.com", Employee: emp})
	if res.OK || len(repo.calls) != 0 {
		t.Errorf("failed payment pushed: %+v", res)
	}
	if _, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).PushPayment(ctx, "TRV-2026-11-0001-001",
		domain.FinancePushRequest{UpdatedBy: "p@wso2.com"}); !errors.As(err, new(*apierror.ValidationError)) {
		t.Errorf("no employee: err = %v", err)
	}
}

func TestAddInvoices_Validation(t *testing.T) {
	ctx := context.Background()
	row := domain.FinanceInvoiceCreate{InvoiceNumber: "INV-1", Month: "October", CostTypeID: 1, InvoiceCurrency: "USD",
		InvoiceAmount: 10, ConvertedCurrency: "USD", ConvertedAmount: 10}
	in := func(rows ...domain.FinanceInvoiceCreate) domain.FinanceInvoicesCreate {
		return domain.FinanceInvoicesCreate{Invoices: rows, CreatedBy: "agent@wso2.com"}
	}
	bad := row
	bad.InvoiceNumber = "INV 1"
	if _, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).AddInvoices(ctx, "TRV-2026-11-0001", in(bad)); err == nil ||
		!strings.Contains(err.Error(), "is an invalid invoice number") {
		t.Errorf("bad number: %v", err)
	}
	if _, err := newFinanceSvc(&fakeFinanceRepo{exists: true}, &fakeNetSuite{}).AddInvoices(ctx, "TRV-2026-11-0001", in(row)); !errors.As(err, new(*apierror.ConflictError)) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true, found: []netsuite.Record{{}}}).AddInvoices(ctx, "TRV-2026-11-0001", in(row)); err == nil ||
		!strings.Contains(err.Error(), "NetSuite") {
		t.Errorf("in netsuite: %v", err)
	}
	cur := row
	cur.CostTypeID = 6
	if _, err := newFinanceSvc(&fakeFinanceRepo{paymentN: 1}, &fakeNetSuite{}).AddInvoices(ctx, "TRV-2026-11-0001", in(cur)); err == nil ||
		err.Error() != FinanceCurrencyInvoiceBlockedMsg {
		t.Errorf("currency: %v", err)
	}
	// No classification for the engagement code or its suffix.
	if _, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).AddInvoices(ctx, "TRV-2026-11-0001", in(row)); err == nil ||
		!strings.Contains(err.Error(), "No valid invoice classification") {
		t.Errorf("classification: %v", err)
	}
	repo := &fakeFinanceRepo{classBySuf: fp(4), invoice: approvedInvoice()}
	got, err := newFinanceSvc(repo, &fakeNetSuite{}).AddInvoices(ctx, "TRV-2026-11-0001", in(row))
	if err != nil || len(got) != 1 || repo.inserted[0].ClassificationID != 4 {
		t.Errorf("insert: %v, %+v", err, repo.inserted)
	}
}

func TestDecideInvoices(t *testing.T) {
	repo := &fakeFinanceRepo{invoice: approvedInvoice()}
	res, err := newFinanceSvc(repo, &fakeNetSuite{}).DecideInvoices(context.Background(), domain.FinanceInvoiceDecision{
		InvoiceIDs: []int{7}, Decision: "approve", Step: "travel", UpdatedBy: "po@wso2.com"})
	if err != nil || len(res) != 1 || res[0].OK || *res[0].Message != FinanceInvoiceStatusMsg {
		t.Errorf("res = %+v, err = %v", res, err)
	}
	if _, err := newFinanceSvc(repo, &fakeNetSuite{}).DecideInvoices(context.Background(), domain.FinanceInvoiceDecision{
		InvoiceIDs: []int{7}, Decision: "reject", Step: "finance", UpdatedBy: "f@wso2.com"}); err == nil {
		t.Error("a reject without a reason must fail")
	}
}
