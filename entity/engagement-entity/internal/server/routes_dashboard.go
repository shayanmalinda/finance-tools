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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/handler"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// registerDashboardRoutes registers the dashboard screen area's routes. The area
// owns this file: its dependency graph (repository → service → handler) and
// its routes live here.
//
// Every route is a read of aggregates (COUNT / SUM ... GROUP BY), one query
// per schema; the backend decides what the caller may see.
func registerDashboardRoutes(mux *http.ServeMux, d Deps) {
	repo := repository.NewDashboardRepository(d.Pools.Allocation, d.Pools.Travel, d.Pools.Expense)
	h := handler.NewDashboardHandler(service.NewDashboardService(repo, nil))

	// The legacy Allocations dashboard (allocation_db), verbatim queries.
	mux.HandleFunc("GET /dashboard/allocations-today", h.AllocationsToday)
	// The person's own allocations (allocation_db) and trips (travelapp).
	mux.HandleFunc("GET /dashboard/my-work", h.MyWork)
	// Waiting on You counts (allocation_db and travelapp).
	mux.HandleFunc("GET /dashboard/queues", h.Queues)
	// Travel and Travel Cost (travelapp, expense_app for card spend).
	mux.HandleFunc("GET /dashboard/travel", h.Travel)
	// Engagements starting soon, Available Consultants, Allocations Over Time.
	mux.HandleFunc("GET /dashboard/engagements", h.Engagements)
	// The travel agent dashboard (travelapp, one agency).
	mux.HandleFunc("GET /dashboard/agency", h.Agency)
	// Release 2 widgets: cost lines (travelapp, allocation_db, expense_app) and
	// the role stats (travelapp, allocation_db).
	mux.HandleFunc("GET /dashboard/cost-lines", h.CostLines)
	mux.HandleFunc("GET /dashboard/travel-team", h.TravelTeam)
	mux.HandleFunc("GET /dashboard/manager", h.Manager)
	mux.HandleFunc("GET /dashboard/account-manager", h.AccountManager)
	mux.HandleFunc("GET /dashboard/events", h.Events)
	mux.HandleFunc("GET /dashboard/my-visas", h.MyVisas)
}
