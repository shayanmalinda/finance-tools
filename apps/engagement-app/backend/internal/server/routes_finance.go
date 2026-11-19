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
	"context"
	"log/slog"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// financeCurrency is the legacy currency conversion service, or nil when
// it is not configured (conversions answer 503).
func financeCurrency(c config.CurrencyConfig) service.FinanceCurrencyConverter {
	if !c.Configured() {
		slog.Warn("CURRENCY_CONVERT_SERVICE_URL is not set: per diem payment add and edit answer 503")
		return nil
	}
	return service.NewFinanceCurrencyClient(c.URL, rest.NewOAuthHTTPClient(c.Client.TokenURL, c.Client.ClientID, c.Client.ClientSecret, c.Client.Scopes))
}

// registerFinanceRoutes registers the finance screen area's routes. The area
// owns this file: its dependency graph (service → handler) and its routes
// live here, each behind a.handle / a.handleIdem with its legacy gate.
func registerFinanceRoutes(a *area) {
	rc := a.Recipients
	svc := service.NewFinanceService(a.Entity, a.HR, a.Email, email.FinanceRecipients{
		TravelLogisticsGroup: rc.TravelLogisticsGroup, FinanceGroup: rc.FinanceGroup,
		CompanyRecipients: rc.CompanyRecipients, AppBaseURL: rc.AppBaseURL,
	}, financeCurrency(a.Currency), a.Now)
	svc.Campaigns = a.Campaigns
	if a.Recipients.Enabled() {
		svc.WithEmailAudit(func(ctx context.Context, m email.Message) {
			service.TravelAuditEmail(ctx, a.Entity, a.Now(), m)
		})
	}
	h := handler.NewFinanceHandler(svc)
	travel := func(ids ...int) domain.Gate { return domain.Gate{domain.AppTravel: ids} }

	// A travel request's finance tabs (legacy interceptor rules: View any or
	// View own on GET, the Add privilege on POST).
	a.handle("GET /travel-requests/{jobNumber}/per-diems", travel(18, 28, 37), h.PerDiems)
	a.handleIdem("POST /travel-requests/{jobNumber}/per-diems", travel(26), h.AddPerDiem)
	a.handle("DELETE /travel-requests/{jobNumber}/per-diems/{perDiemId}", travel(27), h.DeletePerDiem)
	a.handle("GET /travel-requests/{jobNumber}/invoices", travel(30, 38), h.TripInvoices)
	a.handleIdem("POST /travel-requests/{jobNumber}/invoices", travel(31), h.AddInvoices)
	a.handle("GET /travel-requests/{jobNumber}/travel-cards", travel(45, 48), h.TravelCards)
	a.handleIdem("POST /travel-requests/{jobNumber}/travel-cards", travel(46), h.AddTravelCard)
	a.handle("GET /travel-requests/{jobNumber}/per-diem-payments", travel(42, 47), h.TripPayments)
	a.handleIdem("POST /travel-requests/{jobNumber}/per-diem-payments", travel(43), h.AddPayment)

	// Invoices & Payments. Pushing an invoice needs Travel 34 from the queue
	// and from the trip page alike (LEGACY-ISSUES L6: the legacy by-job push
	// matched the Add Invoices rule, 31).
	a.handle("GET /invoices", travel(39, 32, 33, 34), h.Invoices)
	a.handle("GET /invoices/{invoiceId}", travel(30, 38, 39), h.Invoice)
	a.handle("POST /invoices/approve", travel(32, 33), h.ApproveInvoices)
	a.handle("POST /invoices/reject", travel(32, 33), h.RejectInvoices)
	a.handleIdem("POST /invoices/push", travel(34), h.PushInvoices)
	a.handle("POST /invoices/resync", travel(34), h.ResyncInvoices)
	a.handle("GET /per-diem-payments", travel(40, 41), h.Payments)
	a.handle("GET /per-diem-payments/{paymentId}", travel(41, 42, 47), h.Payment)
	a.handle("PATCH /per-diem-payments/{paymentId}", travel(44), h.UpdatePayment)
	a.handleIdem("POST /per-diem-payments/push", travel(40), h.PushPayments)
	a.handle("POST /per-diem-payments/resync", travel(40), h.ResyncPayments)
	a.handle("GET /netsuite-sync-failures", travel(34, 40), h.SyncFailures)

	// Reports (UI-only privileges 39 and 41 in the legacy app).
	a.handleReport("GET /reports/invoices", travel(39), auditExport("invoice", "invoices"), h.InvoiceReport)
	a.handleReport("GET /reports/per-diem-payments", travel(41), auditExport("per_diem_payment", "per-diem-payments"), h.PaymentReport)
}
