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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// registerDashboardRoutes registers the dashboard screen area's routes. The area
// owns this file: its dependency graph (service → handler) and its routes
// live here, each behind a.handle / a.handleIdem with its legacy gate.
func registerDashboardRoutes(a *area) {
	h := handler.NewDashboardHandler(service.NewDashboardService(a.Entity, a.HR, a.Converter, a.Now))

	// Dashboard: Allocations "Dashboard Page" (1, every employee) or a travel
	// agent (Travel 18). Each section is then gated on the caller's own
	// privileges in the service.
	a.handle("GET /dashboard", domain.Gate{domain.AppAllocations: {1}, domain.AppTravel: {18}}, h.Get)
	// The legacy Allocations dashboard: Dashboard Page (1).
	a.handle("GET /dashboard/allocations-today", domain.Gate{domain.AppAllocations: {1}}, h.AllocationsToday)
	// Me overview card: any employee (Allocations 1 or My Travel Requests 49).
	a.handle("GET /me/overview", domain.Gate{domain.AppAllocations: {1}, domain.AppTravel: {49}}, h.MeOverview)
}
