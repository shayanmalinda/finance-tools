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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// The production client must serve the budgets area: a missing method fails
// the build, not the start-up.
var _ service.BudgetEntityAPI = (*engagemententity.HTTPClient)(nil)

// newBudgets builds the Travel Budgets service, or nil when the feature is off
// (BUDGETS_ENABLED=false) or the entity client cannot serve it. Every other
// area treats nil as "no budgets", so off changes nothing.
func newBudgets(a *area) *service.BudgetService {
	if !a.Budgets.Enabled {
		return nil
	}
	entity, ok := a.Entity.(service.BudgetEntityAPI)
	if !ok {
		slog.Error("travel budgets not enabled: the Engagement Entity client lacks BudgetAPI")
		return nil
	}
	return service.NewBudgetService(entity, a.HR, a.Email, a.Rates, service.BudgetOptions{
		LineRecipients: a.Budgets.LineRecipients, AppBaseURL: a.Recipients.AppBaseURL, CacheTTL: a.Budgets.CacheTTL, Now: a.Now})
}

// budgetHooks is the budgets service as the other areas see it: a nil
// interface (not a nil pointer in one) when the feature is off.
func budgetHooks(b *service.BudgetService) service.BudgetHooks {
	if b == nil {
		return nil
	}
	return b
}

// registerBudgetRoutes registers the Travel Budgets routes. Off, none is
// registered, so every one answers 404.
func registerBudgetRoutes(a *area) {
	if a.budgets == nil {
		return
	}
	h := handler.NewBudgetHandler(a.budgets)
	alloc := func(ids ...int) domain.Gate { return domain.Gate{domain.AppAllocations: ids} }
	viewGate := domain.Gate{domain.AppAllocations: {23}, domain.AppTravel: {1, 4, 5, 6, 29, 35}}
	adminGate := domain.Gate{domain.AppAllocations: {23}, domain.AppTravel: {29}}

	// The preview is for whoever allocates; the service applies no other rule.
	a.handle("GET /budgets/preview", alloc(6, 16, 20, 21, 32, 33), h.Preview)
	a.handle("GET /budgets/summary", viewGate, h.Summary)
	// Lines feed the pickers (every allocator, engagement editor and Travel
	// Team member) and Settings (retired lines for the budget admin only).
	a.handle("GET /budgets/lines", domain.Gate{
		domain.AppAllocations: {6, 11, 12, 15, 16, 20, 21, 23, 25, 27, 30, 32, 33},
		domain.AppTravel:      {1, 4, 5, 6, 29, 35},
	}, h.Lines)
	a.handleIdem("POST /budgets/lines", adminGate, h.CreateLine)
	a.handle("PUT /budgets/lines/{id}", adminGate, h.UpdateLine)
	a.handle("DELETE /budgets/lines/{id}", adminGate, h.RetireLine)
	a.handle("GET /budgets/allocation-type-rules", domain.Gate{domain.AppAllocations: {23, 25}, domain.AppTravel: {29}}, h.AllocationTypeRules)
	a.handle("PUT /budgets/allocation-type-rules/{allocationTypeId}", adminGate, h.SaveAllocationTypeRule)
	a.handle("GET /budgets/needs-line", viewGate, h.NeedsLine)
	a.handle("PUT /travel-requests/{jobNumber}/budget-line", domain.Gate{domain.AppTravel: {1, 4, 5, 6, 29}}, h.SetTripLine)
}
