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

package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/handler"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// registerFinanceRoutes registers the finance screen area's routes. The area
// owns this file: its dependency graph (repository → service → handler) and
// its routes live here.
func registerFinanceRoutes(mux *http.ServeMux, d Deps) {
	nsCfg := netsuite.ConfigFrom(d.NetSuite)
	if missing := nsCfg.Missing(); len(missing) > 0 {
		slog.Error("NetSuite push is enabled but not fully configured; pushes and lookups answer 503",
			"missing", strings.Join(missing, ","))
		nsCfg.Enabled = false
	}
	if !nsCfg.Enabled {
		slog.Warn("NetSuite is disabled: invoice and per diem pushes answer 503, entry duplicate checks skip NetSuite")
	}

	svc := service.NewFinanceService(
		repository.NewFinanceRepository(d.Pools.Travel),
		repository.NewFinanceAllocationRepository(d.Pools.Allocation),
		repository.NewFinanceExpenseRepository(d.Pools.Expense),
		netsuite.New(nsCfg, nil),
	)
	h := handler.NewFinanceHandler(svc)

	// A travel request's finance tabs.
	mux.HandleFunc("GET /finance/travel-requests/{jobNumber}", h.Trip)
	mux.HandleFunc("GET /finance/travel-requests/{jobNumber}/per-diems", h.PerDiems)
	mux.HandleFunc("POST /finance/travel-requests/{jobNumber}/per-diems", h.AddPerDiem)
	mux.HandleFunc("GET /finance/travel-requests/{jobNumber}/per-diems/{id}", h.PerDiem)
	mux.HandleFunc("DELETE /finance/travel-requests/{jobNumber}/per-diems/{id}", h.DeletePerDiem)
	mux.HandleFunc("GET /finance/travel-requests/{jobNumber}/payment-basis", h.PaymentBasis)
	mux.HandleFunc("GET /finance/travel-requests/{jobNumber}/travel-cards", h.TravelCards)
	mux.HandleFunc("POST /finance/travel-requests/{jobNumber}/travel-cards", h.AddTravelCard)
	mux.HandleFunc("POST /finance/travel-requests/{jobNumber}/invoices", h.AddInvoices)
	mux.HandleFunc("POST /finance/travel-requests/{jobNumber}/per-diem-payments", h.AddPayment)

	// Invoices.
	mux.HandleFunc("GET /finance/invoice-cost-types", h.CostTypes)
	mux.HandleFunc("GET /finance/invoices", h.Invoices)
	mux.HandleFunc("GET /finance/invoices/counts", h.InvoiceCounts)
	mux.HandleFunc("POST /finance/invoices/decisions", h.DecideInvoices)
	mux.HandleFunc("GET /finance/invoices/{id}", h.Invoice)
	mux.HandleFunc("POST /finance/invoices/{id}/push", h.PushInvoice)
	mux.HandleFunc("POST /finance/invoices/{id}/resync", h.ResyncInvoice)
	mux.HandleFunc("GET /finance/invoices/{id}/netsuite-reference", h.InvoiceReference)

	// Per diem payments.
	mux.HandleFunc("GET /finance/per-diem-payments", h.Payments)
	mux.HandleFunc("GET /finance/per-diem-payments/counts", h.PaymentCounts)
	mux.HandleFunc("GET /finance/per-diem-payments/{perDiemNumber}", h.Payment)
	mux.HandleFunc("PATCH /finance/per-diem-payments/{perDiemNumber}", h.UpdatePayment)
	mux.HandleFunc("POST /finance/per-diem-payments/{perDiemNumber}/push", h.PushPayment)
	mux.HandleFunc("POST /finance/per-diem-payments/{perDiemNumber}/resync", h.ResyncPayment)
	mux.HandleFunc("GET /finance/per-diem-payments/{perDiemNumber}/netsuite-reference", h.PaymentReference)

	// expense_app subsidiaries (the traveller's currency and tax code).
	mux.HandleFunc("GET /finance/subsidiaries/{company}", h.Subsidiary)
}
