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

// registerAllocationRoutes registers the allocation screen area's routes. The area
// owns this file: its dependency graph (repository → service → handler) and
// its routes live here.
//
// Engagement allocations, default (team) allocations, the travel rows they
// create (travelapp) and the Consultant
// Allocations report. Writes take the acting user in the body (createdBy /
// updatedBy).
func registerAllocationRoutes(mux *http.ServeMux, d Deps) {
	pools := d.Pools
	repo := repository.NewAllocationRepository(pools.Allocation, pools.Travel)
	h := handler.NewAllocationHandler(service.NewAllocationService(repo))

	mux.HandleFunc("GET /allocations/engagements/{id}", h.Engagement)
	mux.HandleFunc("GET /allocations/visas", h.Visas)
	mux.HandleFunc("POST /allocations/timeline", h.Timeline)
	mux.HandleFunc("POST /allocations/search", h.Search)
	mux.HandleFunc("POST /allocations/bulk", h.Bulk)
	mux.HandleFunc("GET /allocations/{id}", h.Get)
	mux.HandleFunc("PATCH /allocations/{id}", h.Update)
	mux.HandleFunc("DELETE /allocations/{id}", h.Delete)

	// Allocation emails log (allocation_db.email_auditlog).
	emailAudit := handler.NewEmailAuditHandler(service.NewAllocationEmailAuditService(
		repository.NewAllocationEmailAuditRepository(pools.Allocation)))
	mux.HandleFunc("POST /allocation-email-audit-logs", emailAudit.InsertAllocationEmailAudit)

	mux.HandleFunc("GET /consultant-allocations", h.ConsultantAllocations)
	mux.HandleFunc("GET /consultant-allocations/non-allocated", h.NonAllocatedEmployees)
	mux.HandleFunc("GET /consultant-allocations/team-defaults", h.TeamDefaults)
	mux.HandleFunc("GET /consultant-allocations/engagement-codes", h.EngagementCodes)
}
