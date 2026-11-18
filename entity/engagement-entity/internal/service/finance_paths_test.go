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
	"errors"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
)

const (
	pathsJob     = "TRV-2026-11-0001"
	pathsPerDiem = "TRV-2026-11-0001-001"
	pathsActor   = "finance@example.com"
)

// wantValidation fails unless err is a ValidationError whose message contains want.
func wantValidation(t *testing.T, err error, want string) {
	t.Helper()
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, want) {
		t.Errorf("err = %v, want validation %q", err, want)
	}
}

func wantErrAs[T error](t *testing.T, err error, want string) {
	t.Helper()
	var target T
	if !errors.As(err, &target) || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want %T %q", err, target, want)
	}
}

func failedPayment() *domain.FinancePayment {
	p := pendingPayment()
	p.NSStatus = fp(domain.PaymentNSFailed)
	return p
}

func TestResyncPayment(t *testing.T) {
	ctx := t.Context()
	t.Run("found settles as found", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: failedPayment()}
		ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "7701"}}}
		res, err := newFinanceSvc(repo, ns).ResyncPayment(ctx, pathsPerDiem, pathsActor)
		if err != nil {
			t.Fatal(err)
		}
		if !res.OK || res.Outcome != domain.NSOutcomeFound || res.Reference == nil || *res.Reference != "7701" ||
			res.JobNumber != pathsJob || strings.Join(repo.calls, ",") != "resolve:found" || ns.created != 0 {
			t.Errorf("res = %+v, calls = %v, created = %d", res, repo.calls, ns.created)
		}
		if len(repo.pushes) != 1 || repo.pushes[0].Metadata["operation"] != "resync" ||
			repo.pushes[0].Metadata["outcome"] != domain.NSOutcomeFound {
			t.Errorf("audit = %+v", repo.pushes)
		}
	})
	t.Run("not found goes back to push", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: failedPayment()}
		res, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).ResyncPayment(ctx, pathsPerDiem, pathsActor)
		if err != nil {
			t.Fatal(err)
		}
		if res.OK || res.Outcome != domain.NSOutcomeNotFound || res.Message == nil ||
			!strings.Contains(*res.Message, "back in Per Diem Payments to Push") ||
			strings.Join(repo.calls, ",") != "resolve:notfound" {
			t.Errorf("res = %+v, calls = %v", res, repo.calls)
		}
	})
	for name, tc := range map[string]struct {
		repo *fakeFinanceRepo
		ns   *fakeNetSuite
		msg  string
	}{
		"not on NS Sync Failures": {&fakeFinanceRepo{payment: pendingPayment()}, &fakeNetSuite{enabled: true},
			"Only payments on NS Sync Failures can be re-synced."},
		"lookup fails": {&fakeFinanceRepo{payment: failedPayment()},
			&fakeNetSuite{enabled: true, lookupErr: errors.New("down")}, "Could not reach NetSuite. Please try again."},
		"changed meanwhile": {&fakeFinanceRepo{payment: failedPayment(), resolveLost: true},
			&fakeNetSuite{enabled: true}, "The payment changed while it was being re-synced."},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := newFinanceSvc(tc.repo, tc.ns).ResyncPayment(ctx, pathsPerDiem, pathsActor)
			if err != nil {
				t.Fatal(err)
			}
			if res.OK || res.Outcome != domain.NSOutcomeSkipped || res.Message == nil || *res.Message != tc.msg {
				t.Errorf("res = %+v", res)
			}
		})
	}
	t.Run("guards", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: failedPayment()}
		_, err := newFinanceSvc(repo, &fakeNetSuite{}).ResyncPayment(ctx, pathsPerDiem, pathsActor)
		wantErrAs[*apierror.ServiceUnavailableError](t, err, "NetSuite push is not enabled")
		_, err = newFinanceSvc(repo, &fakeNetSuite{enabled: true}).ResyncPayment(ctx, pathsPerDiem, "")
		wantValidation(t, err, "the acting user's email is required")
		_, err = newFinanceSvc(repo, &fakeNetSuite{enabled: true}).ResyncPayment(ctx, "TRV-1", pathsActor)
		wantValidation(t, err, "Invalid per diem number format")
		_, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true}).
			ResyncPayment(ctx, pathsPerDiem, pathsActor)
		wantErrAs[*apierror.NotFoundError](t, err, "Per diem payment not found.")
		if len(repo.calls) != 0 {
			t.Errorf("calls = %v, want none", repo.calls)
		}
	})
}

func TestInvoiceReference(t *testing.T) {
	ctx := t.Context()
	t.Run("stored reference needs no lookup", func(t *testing.T) {
		inv := approvedInvoice()
		inv.NetSuiteReference = fp("5501")
		ns := &fakeNetSuite{enabled: true}
		ref, err := newFinanceSvc(&fakeFinanceRepo{invoice: inv}, ns).InvoiceReference(ctx, 7)
		if err != nil {
			t.Fatal(err)
		}
		if ref.Reference == nil || *ref.Reference != "5501" || ns.lookups != 0 {
			t.Errorf("ref = %+v, lookups = %d", ref, ns.lookups)
		}
	})
	t.Run("no stored reference looks it up", func(t *testing.T) {
		ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "4400"}}}
		ref, err := newFinanceSvc(&fakeFinanceRepo{invoice: approvedInvoice()}, ns).InvoiceReference(ctx, 7)
		if err != nil {
			t.Fatal(err)
		}
		if ref.Reference == nil || *ref.Reference != "4400" || ns.lookups != 1 {
			t.Errorf("ref = %+v, lookups = %d", ref, ns.lookups)
		}
	})
	t.Run("nothing in NetSuite is a null reference", func(t *testing.T) {
		ref, err := newFinanceSvc(&fakeFinanceRepo{invoice: approvedInvoice()}, &fakeNetSuite{enabled: true}).
			InvoiceReference(ctx, 7)
		if err != nil || ref.Reference != nil {
			t.Errorf("ref = %+v, err = %v", ref, err)
		}
	})
	t.Run("errors", func(t *testing.T) {
		_, err := newFinanceSvc(&fakeFinanceRepo{invoice: approvedInvoice()}, &fakeNetSuite{}).InvoiceReference(ctx, 7)
		wantErrAs[*apierror.ServiceUnavailableError](t, err, "NetSuite push is not enabled")
		_, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true}).InvoiceReference(ctx, 7)
		wantErrAs[*apierror.NotFoundError](t, err, "Invoice not found.")
		_, err = newFinanceSvc(&fakeFinanceRepo{invoice: approvedInvoice()},
			&fakeNetSuite{enabled: true, lookupErr: errors.New("restlet down")}).InvoiceReference(ctx, 7)
		wantErrAs[*apierror.ServiceUnavailableError](t, err, "restlet down")
	})
}

func TestPaymentReference(t *testing.T) {
	ctx := t.Context()
	ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "7701"}}}
	ref, err := newFinanceSvc(&fakeFinanceRepo{}, ns).PaymentReference(ctx, pathsPerDiem)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Reference == nil || *ref.Reference != "7701" {
		t.Errorf("ref = %+v", ref)
	}
	ref, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true}).PaymentReference(ctx, pathsPerDiem)
	if err != nil || ref.Reference != nil {
		t.Errorf("not found: ref = %+v, err = %v", ref, err)
	}
	_, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).PaymentReference(ctx, pathsPerDiem)
	wantErrAs[*apierror.ServiceUnavailableError](t, err, "NetSuite push is not enabled")
	_, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true}).PaymentReference(ctx, pathsJob)
	wantValidation(t, err, "Invalid per diem number format")
	_, err = newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{enabled: true, lookupErr: errors.New("restlet down")}).
		PaymentReference(ctx, pathsPerDiem)
	wantErrAs[*apierror.ServiceUnavailableError](t, err, "restlet down")
}

func goodPerDiem() domain.FinancePerDiemCreate {
	return domain.FinancePerDiemCreate{Currency: "GBP", PerDiemValue: 50, Total: 250, NoOfDays: 5,
		StartDate: "2026-11-10 00:00:00", EndDate: "2026-11-15 00:00:00", CreatedBy: pathsActor}
}

func TestAddPerDiem(t *testing.T) {
	ctx := t.Context()
	t.Run("uses the trip's live engagement code", func(t *testing.T) {
		saved := &domain.FinancePerDiem{ID: 11, JobNumber: pathsJob, EngagementCode: "PS-ACME-1"}
		repo := &fakeFinanceRepo{perDiem: saved}
		got, err := newFinanceSvc(repo, &fakeNetSuite{}).AddPerDiem(ctx, pathsJob, goodPerDiem())
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != 11 || repo.perDiemCode != "PS-ACME-1" || repo.perDiemIn == nil || repo.perDiemIn.Total != 250 {
			t.Errorf("got = %+v, code = %q, in = %+v", got, repo.perDiemCode, repo.perDiemIn)
		}
	})
	for name, tc := range map[string]struct {
		edit func(*domain.FinancePerDiemCreate)
		want string
	}{
		"currency": {func(in *domain.FinancePerDiemCreate) { in.Currency = "gbp" },
			"currency must be a three-letter"},
		"value": {func(in *domain.FinancePerDiemCreate) { in.PerDiemValue = 0 },
			"perDiemValue must be more than 0"},
		"total": {func(in *domain.FinancePerDiemCreate) { in.Total = -1 },
			"total must not be negative"},
		"days": {func(in *domain.FinancePerDiemCreate) { in.NoOfDays = 0 },
			"noOfDays must be 1 or more"},
		"date shape": {func(in *domain.FinancePerDiemCreate) { in.StartDate = "2026-11-10" },
			"startDate and endDate must be"},
		"end first": {func(in *domain.FinancePerDiemCreate) { in.EndDate = in.StartDate },
			"Please check the dates again"},
		"actor": {func(in *domain.FinancePerDiemCreate) { in.CreatedBy = "finance" },
			"the acting user's email is required"},
	} {
		t.Run(name, func(t *testing.T) {
			in := goodPerDiem()
			tc.edit(&in)
			repo := &fakeFinanceRepo{}
			_, err := newFinanceSvc(repo, &fakeNetSuite{}).AddPerDiem(ctx, pathsJob, in)
			wantValidation(t, err, tc.want)
			if len(repo.calls) != 0 {
				t.Errorf("calls = %v, want none", repo.calls)
			}
		})
	}
	t.Run("trip guards", func(t *testing.T) {
		_, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).AddPerDiem(ctx, "TRV-1", goodPerDiem())
		wantValidation(t, err, "Invalid job number format")
		_, err = newFinanceSvc(&fakeFinanceRepo{noTrip: true}, &fakeNetSuite{}).AddPerDiem(ctx, pathsJob, goodPerDiem())
		wantErrAs[*apierror.NotFoundError](t, err, "Travel request not found.")
		noCode := &domain.FinanceTrip{JobNumber: pathsJob, TravellerEmail: "traveller@example.com"}
		repo := &fakeFinanceRepo{trip: noCode}
		_, err = newFinanceSvc(repo, &fakeNetSuite{}).AddPerDiem(ctx, pathsJob, goodPerDiem())
		wantValidation(t, err, "The travel request has no engagement code.")
		if len(repo.calls) != 0 {
			t.Errorf("calls = %v, want none", repo.calls)
		}
	})
}

func TestDeletePerDiem(t *testing.T) {
	ctx := t.Context()
	repo := &fakeFinanceRepo{deleteOK: true}
	if err := newFinanceSvc(repo, &fakeNetSuite{}).DeletePerDiem(ctx, pathsJob, 11); err != nil {
		t.Fatal(err)
	}
	if strings.Join(repo.calls, ",") != "delete per diem" {
		t.Errorf("calls = %v", repo.calls)
	}
	err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).DeletePerDiem(ctx, pathsJob, 11)
	wantErrAs[*apierror.NotFoundError](t, err, "Per diem not found.")
	repo = &fakeFinanceRepo{deleteOK: true}
	err = newFinanceSvc(repo, &fakeNetSuite{}).DeletePerDiem(ctx, "TRV-1", 11)
	wantValidation(t, err, "Invalid job number format")
	if len(repo.calls) != 0 {
		t.Errorf("calls = %v, want none", repo.calls)
	}
}

func TestAddTravelCard(t *testing.T) {
	ctx := t.Context()
	in := domain.FinanceTravelCardCreate{Currency: "GBP", Amount: 120, CreatedBy: pathsActor}
	t.Run("returns the saved card", func(t *testing.T) {
		saved := domain.FinanceTravelCard{ID: 21, JobNumber: pathsJob, Currency: "GBP", Amount: 120,
			AddedBy: pathsActor, AddedDate: "2026-11-05 09:00:00"}
		repo := &fakeFinanceRepo{cards: []domain.FinanceTravelCard{{ID: 20}, saved}}
		got, err := newFinanceSvc(repo, &fakeNetSuite{}).AddTravelCard(ctx, pathsJob, in)
		if err != nil {
			t.Fatal(err)
		}
		if got != saved || repo.cardIn == nil || *repo.cardIn != in {
			t.Errorf("got = %+v, inserted = %+v", got, repo.cardIn)
		}
	})
	t.Run("falls back to the input when the card is not listed", func(t *testing.T) {
		got, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).AddTravelCard(ctx, pathsJob, in)
		if err != nil {
			t.Fatal(err)
		}
		want := domain.FinanceTravelCard{ID: 21, JobNumber: pathsJob, Currency: "GBP", Amount: 120, AddedBy: pathsActor}
		if got != want {
			t.Errorf("got = %+v, want %+v", got, want)
		}
	})
	t.Run("refused once a payment exists", func(t *testing.T) {
		repo := &fakeFinanceRepo{paymentN: 1}
		_, err := newFinanceSvc(repo, &fakeNetSuite{}).AddTravelCard(ctx, pathsJob, in)
		wantValidation(t, err, FinanceCardsBlockedMsg)
		if len(repo.calls) != 0 {
			t.Errorf("calls = %v, want none", repo.calls)
		}
	})
	for name, tc := range map[string]struct {
		in   domain.FinanceTravelCardCreate
		want string
	}{
		"currency": {domain.FinanceTravelCardCreate{Currency: "£", Amount: 1, CreatedBy: pathsActor},
			"currency must be a three-letter"},
		"amount": {domain.FinanceTravelCardCreate{Currency: "GBP", CreatedBy: pathsActor},
			"amount must be more than 0"},
		"actor": {domain.FinanceTravelCardCreate{Currency: "GBP", Amount: 1}, "the acting user's email is required"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).AddTravelCard(ctx, pathsJob, tc.in)
			wantValidation(t, err, tc.want)
		})
	}
	_, err := newFinanceSvc(&fakeFinanceRepo{noTrip: true}, &fakeNetSuite{}).AddTravelCard(ctx, pathsJob, in)
	wantErrAs[*apierror.NotFoundError](t, err, "Travel request not found.")
}

func goodPayment() domain.FinancePaymentCreate {
	return domain.FinancePaymentCreate{PerDiemNumber: pathsPerDiem, PaymentCurrency: "GBP", PaymentAmount: 400,
		ConvertedCurrency: "LKR", ConvertedAmount: 152000.5, CreatedBy: pathsActor}
}

func TestAddPayment(t *testing.T) {
	ctx := t.Context()
	t.Run("checks NetSuite then saves", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: pendingPayment()}
		ns := &fakeNetSuite{enabled: true}
		got, err := newFinanceSvc(repo, ns).AddPayment(ctx, pathsJob, goodPayment())
		if err != nil {
			t.Fatal(err)
		}
		if got.PerDiemNumber != pathsPerDiem || ns.lookups != 1 || strings.Join(repo.calls, ",") != "insert payment" ||
			repo.paymentIn == nil || *repo.paymentIn != goodPayment() {
			t.Errorf("got = %+v, lookups = %d, calls = %v", got, ns.lookups, repo.calls)
		}
	})
	t.Run("NetSuite off skips the lookup", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: pendingPayment()}
		ns := &fakeNetSuite{}
		if _, err := newFinanceSvc(repo, ns).AddPayment(ctx, pathsJob, goodPayment()); err != nil {
			t.Fatal(err)
		}
		if ns.lookups != 0 || strings.Join(repo.calls, ",") != "insert payment" {
			t.Errorf("lookups = %d, calls = %v", ns.lookups, repo.calls)
		}
	})
	t.Run("duplicates and lookup failure are refused", func(t *testing.T) {
		repo := &fakeFinanceRepo{payNumberExists: true}
		_, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).AddPayment(ctx, pathsJob, goodPayment())
		wantErrAs[*apierror.ConflictError](t, err, "Per Diem Number already exists in Travel DB")
		repo = &fakeFinanceRepo{}
		ns := &fakeNetSuite{enabled: true, found: []netsuite.Record{{InternalID: "7701"}}}
		_, err = newFinanceSvc(repo, ns).AddPayment(ctx, pathsJob, goodPayment())
		wantErrAs[*apierror.ConflictError](t, err, "Per Diem Number already exists in Net Suite")
		ns = &fakeNetSuite{enabled: true, lookupErr: errors.New("down")}
		_, err = newFinanceSvc(repo, ns).AddPayment(ctx, pathsJob, goodPayment())
		wantErrAs[*apierror.ServiceUnavailableError](t, err, "Error while validating per diem payments")
		if len(repo.calls) != 0 {
			t.Errorf("calls = %v, want none", repo.calls)
		}
	})
	for name, tc := range map[string]struct {
		edit func(*domain.FinancePaymentCreate)
		want string
	}{
		"other trip's number": {func(in *domain.FinancePaymentCreate) { in.PerDiemNumber = "TRV-2026-11-0002-001" },
			"perDiemNumber must be TRV-2026-11-0001-NNN"},
		"number shape": {func(in *domain.FinancePaymentCreate) { in.PerDiemNumber = pathsJob + "-1" },
			"perDiemNumber must be TRV-2026-11-0001-NNN"},
		"currency": {func(in *domain.FinancePaymentCreate) { in.ConvertedCurrency = "lkr" },
			"paymentCurrency and convertedCurrency must be"},
		"amount": {func(in *domain.FinancePaymentCreate) { in.PaymentAmount = 0 },
			"paymentAmount must be more than 0"},
		"converted": {func(in *domain.FinancePaymentCreate) { in.ConvertedAmount = -1 },
			"paymentAmount must be more than 0"},
		"actor": {func(in *domain.FinancePaymentCreate) { in.CreatedBy = "" },
			"the acting user's email is required"},
	} {
		t.Run(name, func(t *testing.T) {
			in := goodPayment()
			tc.edit(&in)
			repo := &fakeFinanceRepo{}
			_, err := newFinanceSvc(repo, &fakeNetSuite{enabled: true}).AddPayment(ctx, pathsJob, in)
			wantValidation(t, err, tc.want)
			if len(repo.calls) != 0 {
				t.Errorf("calls = %v, want none", repo.calls)
			}
		})
	}
	_, err := newFinanceSvc(&fakeFinanceRepo{noTrip: true}, &fakeNetSuite{enabled: true}).
		AddPayment(ctx, pathsJob, goodPayment())
	wantErrAs[*apierror.NotFoundError](t, err, "Travel request not found.")
}

func TestUpdatePayment(t *testing.T) {
	ctx := t.Context()
	upd := domain.FinancePaymentUpdate{PaymentAmount: 450, ConvertedCurrency: "LKR", ConvertedAmount: 171000,
		UpdatedBy: pathsActor}
	t.Run("pending payment is updated", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: pendingPayment(), updateOK: true}
		got, err := newFinanceSvc(repo, &fakeNetSuite{}).UpdatePayment(ctx, pathsPerDiem, upd)
		if err != nil {
			t.Fatal(err)
		}
		if got.PerDiemNumber != pathsPerDiem || strings.Join(repo.calls, ",") != "update payment" ||
			repo.paymentUpdate == nil || *repo.paymentUpdate != upd {
			t.Errorf("got = %+v, calls = %v, update = %+v", got, repo.calls, repo.paymentUpdate)
		}
	})
	t.Run("not pending is a conflict", func(t *testing.T) {
		repo := &fakeFinanceRepo{payment: failedPayment()}
		_, err := newFinanceSvc(repo, &fakeNetSuite{}).UpdatePayment(ctx, pathsPerDiem, upd)
		wantErrAs[*apierror.ConflictError](t, err, "Only a pending payment can be edited.")
	})
	t.Run("missing payment", func(t *testing.T) {
		repo := &fakeFinanceRepo{updateOK: true}
		_, err := newFinanceSvc(repo, &fakeNetSuite{}).UpdatePayment(ctx, pathsPerDiem, upd)
		wantErrAs[*apierror.NotFoundError](t, err, "Per diem payment not found.")
		if len(repo.calls) != 0 {
			t.Errorf("calls = %v, want none", repo.calls)
		}
	})
	for name, tc := range map[string]struct {
		upd  domain.FinancePaymentUpdate
		want string
	}{
		"amount": {domain.FinancePaymentUpdate{ConvertedCurrency: "LKR", UpdatedBy: pathsActor},
			"paymentAmount must be more than 0"},
		"converted": {domain.FinancePaymentUpdate{PaymentAmount: 1, ConvertedCurrency: "LKR", ConvertedAmount: -1,
			UpdatedBy: pathsActor}, "paymentAmount must be more than 0"},
		"currency": {domain.FinancePaymentUpdate{PaymentAmount: 1, ConvertedCurrency: "Rs", UpdatedBy: pathsActor},
			"convertedCurrency must be a three-letter"},
		"actor": {domain.FinancePaymentUpdate{PaymentAmount: 1, ConvertedCurrency: "LKR"},
			"the acting user's email is required"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeFinanceRepo{payment: pendingPayment(), updateOK: true}
			_, err := newFinanceSvc(repo, &fakeNetSuite{}).UpdatePayment(ctx, pathsPerDiem, tc.upd)
			wantValidation(t, err, tc.want)
			if len(repo.calls) != 0 {
				t.Errorf("calls = %v, want none", repo.calls)
			}
		})
	}
	_, err := newFinanceSvc(&fakeFinanceRepo{}, &fakeNetSuite{}).UpdatePayment(ctx, pathsJob, upd)
	wantValidation(t, err, "Invalid per diem number format")
}
