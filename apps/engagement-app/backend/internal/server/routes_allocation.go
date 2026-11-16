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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// registerAllocationRoutes registers the allocation screen area's routes. The area
// owns this file: its dependency graph (service → handler) and its routes
// live here, each behind a.handle / a.handleIdem with its legacy gate.
func registerAllocationRoutes(a *area) {
	org, _ := a.HR.(hrentity.OrgClient)
	if org == nil {
		org = hrentity.Unconfigured{}
	}
	deps := service.AllocationDeps{Entity: a.Entity, HR: a.HR, Org: org, Email: a.Email, Recipients: a.Recipients, Now: a.Now}
	h := handler.NewAllocationHandler(service.NewAllocationService(deps), service.NewTeamAllocationService(deps),
		service.NewConsultantReportService(deps))
	alloc := func(ids ...int) domain.Gate { return domain.Gate{domain.AppAllocations: ids} }

	// Allocate page: legacy GET/POST allocations, get-allocations,
	// engagement-search/{id}/allocations and POST allocations/bulk accepted
	// 6, 16 or 21; the per-type rule (32 / 33 / 6) is in the service.
	a.handleIdem("POST /engagements/{id}/allocations", alloc(6, 16, 21), h.Create)
	a.handle("GET /allocation-timeline", alloc(6, 16, 21), h.Timeline)

	// Engagement Allocations list (legacy allocation-search: 9, 19) and the
	// status popup (legacy PATCH allocations: 6, 16, 21, 10, 20; Release 1
	// gate per openapi: 6, 16, 20, 21, 32, 33; the record rule is in the
	// service).
	a.handle("GET /allocations", alloc(9, 19), h.List)
	a.handle("GET /me/allocations", domain.Gate{domain.AppAllocations: {1}, domain.AppTravel: {49}}, h.ListMine)
	a.handle("PATCH /allocations/{id}/status", alloc(6, 16, 20, 21, 32, 33), h.UpdateStatus)

	// Team Allocations (21; legacy DELETE allocations accepted 21 only).
	a.handle("GET /team-allocations", alloc(21), h.SearchTeam)
	a.handleIdem("POST /team-allocations", alloc(21), h.CreateTeam)
	a.handle("PATCH /team-allocations/{id}", alloc(21), h.UpdateTeam)
	a.handle("DELETE /team-allocations/{id}", alloc(21), h.DeleteTeam)
	a.handle("POST /team-allocations/bulk/validate", alloc(21), h.ValidateBulk)
	a.handleIdem("POST /team-allocations/bulk", alloc(21), h.UploadBulk)

	// Consultant Allocations report (22).
	a.handleAudited("GET /reports/consultant-allocations", alloc(22), auditExport("allocation", "consultant-allocations"), h.Report)
}
