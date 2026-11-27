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
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/handler"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/middleware"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// Deps are the router's dependencies.
type Deps struct {
	Pools *db.Pools
	// NetSuite is the NetSuite account; pushing is off unless
	// NetSuite.PushEnabled.
	NetSuite config.NetSuiteConfig
	// Auth admits callers: auth.Middleware with the gateway JWT verifier, or
	// auth.Off for a local run and tests. Required.
	Auth func(http.Handler) http.Handler
}

// NewRouter builds the dependency graph and routes. Every route except
// GET /health needs a gateway JWT from a listed client application (Deps.Auth).
func NewRouter(d Deps) http.Handler {
	if d.Auth == nil {
		panic("server: Deps.Auth is required (auth.Off only for a local run or tests)")
	}
	pools := d.Pools
	// ── Repositories ────────────────────────────────────────────────────────
	agencyRepo := repository.NewAgencyRepository(pools.Travel)
	accessRepos := map[domain.App]repository.AccessRepository{
		domain.AppAllocations: repository.NewAccessRepository(pools.Allocation, domain.AppAllocations),
		domain.AppTravel:      repository.NewAccessRepository(pools.Travel, domain.AppTravel),
	}
	typeRepo := repository.NewTypeRepository(pools.Allocation)

	// ── Services ─────────────────────────────────────────────────────────────
	// Deliberately NOT cached: role settings are edited in-app, and a revoked
	// privilege must not linger here on top of the backend's own short cache.
	accessSvc := service.NewAccessService(accessRepos, agencyRepo)
	typeSvc := service.NewCachedTypeService(service.NewTypeService(typeRepo))

	// ── Handlers ─────────────────────────────────────────────────────────────
	accessH := handler.NewAccessHandler(accessSvc)
	typeH := handler.NewTypeHandler(typeSvc)
	auditH := handler.NewAuditHandler(service.NewAuditService(repository.NewAuditRepository(pools.Allocation, pools.Travel)))

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handler.HealthCheck)

	// Roles and privileges of each legacy app ({app} = allocations | travel).
	// The backend's /user-info resolves the caller's groups against
	// role-privileges.
	mux.HandleFunc("GET /access/{app}/roles", accessH.ListRoles)
	mux.HandleFunc("GET /access/{app}/privileges", accessH.ListPrivileges)
	mux.HandleFunc("GET /access/{app}/role-privileges", accessH.RolePrivilegeMap)
	// The travel agency a person works for (travel_agent_employee); agents
	// see only that agency's travel requests and invoices.
	mux.HandleFunc("GET /access/travel/agency", accessH.TravelAgency)

	// Lookups (allocation_db).
	mux.HandleFunc("GET /engagement-types", typeH.ListEngagementTypes)
	mux.HandleFunc("GET /allocation-types", typeH.ListAllocationTypes)

	// Views, downloads and exports the backend logs (written to the schema
	// of the entity type). Writes record their own events.
	mux.HandleFunc("POST /audit-events", auditH.Record)

	// Screen areas. Each registers its own routes from its own file.
	registerAllocationRoutes(mux, d)
	registerEngagementRoutes(mux, d)
	registerTravelRoutes(mux, d)
	registerFinanceRoutes(mux, d)
	registerDashboardRoutes(mux, d)
	registerBudgetRoutes(mux, d)

	// Logger wraps Recovery so a panicking request still gets its access
	// line, with the 500 Recovery wrote, and wraps Auth so refused requests
	// are logged too. Actor runs after Auth: X-User-Email is taken only from
	// an admitted caller.
	return middleware.CorrelationID(
		middleware.Logger(
			middleware.Recovery(
				d.Auth(middleware.Actor(
					middleware.Timeout(30*time.Second, netsuite.Timeout+30*time.Second, isNetSuiteRoute)(mux),
				)),
			),
		),
	)
}

// isNetSuiteRoute picks the push and re-sync routes, which wait on NetSuite.
func isNetSuiteRoute(r *http.Request) bool {
	p := r.URL.Path
	return r.Method == http.MethodPost && strings.HasPrefix(p, "/finance/") &&
		(strings.HasSuffix(p, "/push") || strings.HasSuffix(p, "/resync"))
}
