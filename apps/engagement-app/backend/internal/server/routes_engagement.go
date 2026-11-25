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

// The production client must serve the engagements area: a missing method
// fails the build, not the start-up (registerEngagementRoutes skips the area
// only for a test double that lacks it).
var _ service.EngagementEntityAPI = (*engagemententity.HTTPClient)(nil)

// registerEngagementRoutes registers the engagements area: engagements,
// approvals, reference data and lookups, Role Settings and
// allocation type settings. Each route's gate is the union of the legacy
// interceptor lists the screen action used; record-level rules are in the
// services.
func registerEngagementRoutes(a *area) {
	entity, ok := a.Entity.(service.EngagementEntityAPI)
	if !ok {
		slog.Error("engagement routes not registered: the Engagement Entity client lacks EngagementAPI")
		return
	}
	refs := service.NewReferenceService(entity, a.Sales, a.HR)
	engagements := service.NewEngagementService(entity, refs, a.Sales, a.HR, a.Email, a.Recipients, a.Now)
	a.engagements = engagements
	settings := service.NewSettingsServiceWithRoleHook(entity, refs, a.Access.InvalidateRoles)

	refH := handler.NewReferenceHandler(refs)
	engH := handler.NewEngagementHandler(engagements)
	setH := handler.NewSettingsHandler(settings)

	alloc := func(ids ...int) domain.Gate { return domain.Gate{domain.AppAllocations: ids} }

	// Reference data: any signed-in user (Auth only).
	a.mux.HandleFunc("GET /reference-data", refH.ReferenceData)
	a.handle("GET /accounts", alloc(2, 5), refH.Accounts)
	a.handle("GET /accounts/{accountId}/opportunities", alloc(2, 5), refH.Opportunities)
	// Campaigns: the event forms, and the Campaign filter on the invoice,
	// per diem and travel request lists.
	a.handle("GET /campaigns", domain.Gate{
		domain.AppAllocations: {12, 15, 30},
		domain.AppTravel:      {1, 2, 32, 33, 34, 39, 40, 41},
	}, refH.Campaigns)
	a.handle("GET /employees", domain.Gate{
		domain.AppAllocations: {6, 16, 21, 22, 32, 33},
		domain.AppTravel:      {1, 2, 7, 8, 11, 14},
	}, refH.Employees)
	a.handle("GET /org-structure", alloc(6, 16, 21, 22, 32, 33), refH.OrgStructure)

	// Engagements and approvals.
	listGate := alloc(2, 3, 9, 11, 12, 13, 19)
	a.handle("GET /engagements", listGate, engH.List)
	a.handle("GET /engagements/facets", listGate, engH.Facets)
	a.handleIdem("POST /engagements", alloc(2, 11, 12), engH.Create)
	a.handle("GET /engagements/{id}", alloc(3, 6, 9, 13, 16, 19, 26, 29, 32, 33), engH.Get)
	a.handle("PATCH /engagements/{id}", alloc(4, 5, 7, 14, 15, 17, 27, 30), engH.Update)
	a.handle("POST /engagements/{id}/request-status", alloc(7, 17), engH.SetRequestStatus)
	a.handle("POST /engagements/{id}/engagement-status", alloc(4, 5, 7, 14, 17), engH.SetEngagementStatus)
	a.handle("POST /engagements/{id}/approve", alloc(28), engH.Approve)
	a.handle("GET /approvals", alloc(26, 29), engH.Approvals)

	// Settings.
	roleGate := domain.Gate{domain.AppAllocations: {23}, domain.AppTravel: {29}}
	a.handle("GET /role-settings/{app}", roleGate, setH.RoleSettings)
	a.handle("PUT /role-settings/{app}/roles/{roleId}/privileges", roleGate, setH.UpdateRolePrivileges)
	a.handle("GET /settings/allocation-types", alloc(25), setH.AllocationTypes)
	a.handle("POST /settings/allocation-types", alloc(25), setH.CreateAllocationType)
	a.handle("PUT /settings/allocation-types/{allocationTypeId}", alloc(25), setH.UpdateAllocationType)
	a.handle("DELETE /settings/allocation-types/{allocationTypeId}", alloc(25), setH.DeleteAllocationType)
}
