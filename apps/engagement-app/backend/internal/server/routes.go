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
	"net/http"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/middleware"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// Deps are the router's external dependencies, built in main (or faked in
// tests). Nil Email, Files or Userinfo fall back to the logged no-op, the
// unconfigured store (503) and "no userinfo lookup".
type Deps struct {
	Verifier middleware.Verifier
	Userinfo middleware.UserinfoFetcher
	Entity   engagemententity.Client
	Sales    salesentity.Client
	HR       hrentity.Client
	Email    email.Sender
	Files    filestore.Store
	// Recipients are the legacy email group addresses and settings.
	Recipients config.EmailConfig
	// Currency is the legacy currency conversion service; unconfigured,
	// per diem payment add and edit answer 503.
	Currency config.CurrencyConfig
	// Travel holds the legacy visa sentinel ids. Zero values fall back to
	// the production ids.
	Travel config.TravelConfig
	// Budgets is the FY27 Travel Budgets feature (off by default).
	Budgets config.BudgetConfig
	// CORSAllowedOrigins are the webapp origins allowed from a browser.
	CORSAllowedOrigins []string
	// ReportMaxConcurrent caps unpaged report downloads per pod; 0 means 2.
	ReportMaxConcurrent int
	// DashboardCacheTTL is how long this pod keeps the shared dashboard data;
	// 0 keeps nothing (main sets it from DASHBOARD_CACHE_TTL).
	DashboardCacheTTL time.Duration
	// AccessRequestEmail is shown to people with no Finance-side access;
	// empty means none.
	AccessRequestEmail string
	// Now is the clock (tests pin it); nil means time.Now.
	Now func() time.Time
}

// area is what each screen area's register function gets: the shared
// services and helpers to build its own graph and routes.
type area struct {
	Deps
	Access    service.AccessService
	Idem      *middleware.Idempotency
	Campaigns *service.CampaignNames
	// Converter is the currency conversion service, built once for every route that needs it; nil when unconfigured.
	Converter service.FinanceCurrencyConverter
	// Reports is the per-pod cap on unpaged report downloads.
	Reports func(http.Handler) http.Handler
	// Rates is the NetSuite rate cache the dashboard and the budgets share.
	Rates *service.RateCache
	// budgets is the Travel Budgets service, nil when the feature is off.
	budgets *service.BudgetService
	// Travel, Finance, Engagements and Allocations are the list services each
	// area builds, kept for global search (nil when the area is not registered).
	travel      service.TravelService
	finance     *service.FinanceService
	engagements service.EngagementService
	allocations service.AllocationService
	mux         *http.ServeMux
	// invalidateDashboard clears this pod's dashboard cache; set when the
	// dashboard routes are registered.
	invalidateDashboard func()
}

// handle registers pattern behind the legacy privilege gate.
func (a *area) handle(pattern string, gate domain.Gate, h http.HandlerFunc) {
	a.mux.Handle(pattern, middleware.Require(a.Access, gate)(h))
}

// handleIdem is handle plus double-submit protection (Idempotency-Key).
func (a *area) handleIdem(pattern string, gate domain.Gate, h http.HandlerFunc) {
	a.mux.Handle(pattern, middleware.Require(a.Access, gate)(a.Idem.Wrap(h)))
}

// NewRouter wires every route behind Auth (except /health) and the shared
// middleware chain.
func NewRouter(d Deps) http.Handler {
	if d.Email == nil {
		d.Email = email.LogSender{}
	}
	if d.Files == nil {
		d.Files = filestore.Unconfigured{}
	}
	if d.Sales == nil {
		d.Sales = salesentity.Unconfigured{}
	}
	if d.HR == nil {
		d.HR = hrentity.Unconfigured{}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.ReportMaxConcurrent == 0 {
		d.ReportMaxConcurrent = 2
	}
	if d.Travel.VisaNotApplicableID == 0 {
		d.Travel.VisaNotApplicableID = 332
	}
	if d.Travel.NoValidVisaID == 0 {
		d.Travel.NoValidVisaID = 333
	}

	// ── Shared services ──────────────────────────────────────────────────────
	accessSvc := service.NewAccessService(d.Entity)
	userInfoH := handler.NewUserInfoHandler(accessSvc, d.AccessRequestEmail)

	// ── Authenticated routes ─────────────────────────────────────────────────
	api := http.NewServeMux()
	api.HandleFunc("GET /user-info", userInfoH.Get)

	a := &area{Deps: d, Access: accessSvc, Idem: middleware.NewIdempotency(), Campaigns: service.NewCampaignNames(d.Sales),
		Reports: middleware.Limit(d.ReportMaxConcurrent), Converter: financeCurrency(d.Currency), mux: api}
	a.Rates = service.NewRateCache(a.Converter, d.Now)
	a.budgets = newBudgets(a)
	registerEngagementRoutes(a)
	registerAllocationRoutes(a)
	registerTravelRoutes(a)
	registerFinanceRoutes(a)
	registerDashboardRoutes(a)
	registerSearchRoutes(a)
	registerBudgetRoutes(a)

	// ── Root ─────────────────────────────────────────────────────────────────
	root := http.NewServeMux()
	root.HandleFunc("GET /health", handler.HealthCheck)
	root.Handle("/", middleware.Auth(d.Verifier, d.Userinfo)(invalidateOnWrite(jsonMuxErrors(api), func() {
		if a.invalidateDashboard != nil {
			a.invalidateDashboard()
		}
	})))

	return middleware.CorrelationID(
		middleware.Recovery(
			middleware.Logger(
				middleware.CORS(d.CORSAllowedOrigins)(
					middleware.Timeout(RequestTimeout, PushTimeout, isPushRoute)(root),
				),
			),
		),
	)
}

// jsonMuxErrors answers a path or method mux matches no route for with the
// {code, message} JSON body every other error has, instead of ServeMux's
// plain-text 404 and 405 (the Allow header is kept).
func jsonMuxErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern == "" {
			w = &muxErrorWriter{ResponseWriter: w}
		}
		mux.ServeHTTP(w, r)
	})
}

// muxErrorWriter rewrites ServeMux's own 404 or 405 as JSON and drops the
// plain-text body that follows.
type muxErrorWriter struct {
	http.ResponseWriter
	replaced bool
}

func (m *muxErrorWriter) WriteHeader(code int) {
	switch code {
	case http.StatusNotFound:
		m.replaced = true
		apierror.WriteJSON(m.ResponseWriter, code, "The requested resource was not found.")
	case http.StatusMethodNotAllowed:
		m.replaced = true
		apierror.WriteJSON(m.ResponseWriter, code, "This method is not allowed on this resource.")
	default:
		m.ResponseWriter.WriteHeader(code)
	}
}

func (m *muxErrorWriter) Write(b []byte) (int, error) {
	if m.replaced {
		return len(b), nil
	}
	return m.ResponseWriter.Write(b)
}

// RequestTimeout bounds an ordinary request; PushTimeout bounds the NetSuite
// push and re-sync routes (isPushRoute), the longest a request can run.
const (
	RequestTimeout = 30 * time.Second
	PushTimeout    = 10 * time.Minute
)

// isPushRoute picks the NetSuite push and re-sync routes: a bulk push may
// wait on NetSuite (up to 300 s per record in the entity) several times.
func isPushRoute(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "POST /invoices/push", "POST /invoices/resync", "POST /per-diem-payments/push", "POST /per-diem-payments/resync":
		return true
	}
	return false
}

// invalidateOnWrite clears the dashboard cache after every request that may
// change data (anything but GET, HEAD and OPTIONS), whatever its outcome: a
// write that failed part way may still have changed numbers. It clears this
// pod only; other replicas keep their data until the TTL ends.
func invalidateOnWrite(next http.Handler, invalidate func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				invalidate()
			}
		}()
		next.ServeHTTP(w, r)
	})
}
