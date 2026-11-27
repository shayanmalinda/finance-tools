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

// registerBudgetRoutes registers the travel budget area's routes (travelapp,
// expense_app for the ledger). Until the budget tables exist (see
// repository.DetectBudgetTables) every route answers 503.
func registerBudgetRoutes(mux *http.ServeMux, d Deps) {
	h := handler.NewBudgetHandler(service.NewBudgetService(repository.NewBudgetRepository(d.Pools.Travel, d.Pools.Expense)))

	mux.HandleFunc("GET /budgets/years", h.Years)
	mux.HandleFunc("GET /budgets/lines", h.Lines)
	mux.HandleFunc("POST /budgets/lines", h.CreateLine)
	mux.HandleFunc("PUT /budgets/lines/{id}", h.UpdateLine)
	mux.HandleFunc("DELETE /budgets/lines/{id}", h.RetireLine)

	mux.HandleFunc("GET /budgets/allocation-type-rules", h.AllocationTypeRules)
	mux.HandleFunc("PUT /budgets/allocation-type-rules/{allocationTypeId}", h.SaveAllocationTypeRule)
	mux.HandleFunc("GET /budgets/region-rules", h.RegionRules)
	mux.HandleFunc("GET /budgets/team-rules", h.TeamRules)

	mux.HandleFunc("GET /budgets/engagements/{engagementId}/line", h.EngagementLine)
	mux.HandleFunc("PUT /budgets/engagements/{engagementId}/line", h.SetEngagementLine)

	mux.HandleFunc("GET /budgets/trips/{jobNumber}", h.Trip)
	mux.HandleFunc("PUT /budgets/trips/{jobNumber}", h.SetTripLine)
	mux.HandleFunc("GET /budgets/needs-line", h.NeedsLine)
	mux.HandleFunc("GET /budgets/ledger", h.Ledger)
}
