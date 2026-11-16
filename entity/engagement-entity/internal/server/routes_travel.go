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

// registerTravelRoutes registers the travel screen area's routes. The area
// owns this file: its dependency graph (repository → service → handler) and
// its routes live here.
//
// Travel requests live in travelapp and their engagements in allocation_db;
// the service reads each schema through its own pool and combines them in
// Go (no cross-schema SQL).
func registerTravelRoutes(mux *http.ServeMux, d Deps) {
	travelRepo := repository.NewTravelRequestRepository(d.Pools.Travel)
	engagementRepo := repository.NewTravelEngagementRepository(d.Pools.Allocation)
	documentRepo := repository.NewTravelDocumentRepository(d.Pools.Travel)

	requests := service.NewTravelRequestService(travelRepo, engagementRepo, documentRepo)
	documents := service.NewTravelDocumentService(documentRepo)
	h := handler.NewTravelHandler(requests, documents)

	// Travel requests.
	mux.HandleFunc("POST /travel-requests/search", h.SearchTravelRequests)
	mux.HandleFunc("GET /travel-requests/{jobNumber}", h.GetTravelRequest)
	mux.HandleFunc("PATCH /travel-requests/{jobNumber}", h.UpdateTravelRequest)
	mux.HandleFunc("GET /travel-requests/{jobNumber}/active-per-diems", h.ActivePerDiems)
	mux.HandleFunc("GET /travel-agencies", h.ListTravelAgencies)
	mux.HandleFunc("POST /travel-email-audit-logs", h.InsertEmailAudit)

	// Visas and travel documents.
	mux.HandleFunc("GET /visas", h.ListVisas)
	mux.HandleFunc("POST /visas", h.CreateVisa)
	mux.HandleFunc("GET /visas/{id}", h.GetVisa)
	mux.HandleFunc("PUT /visas/{id}", h.UpdateVisa)
	mux.HandleFunc("GET /travel-documents", h.ListTravelDocuments)
	mux.HandleFunc("POST /travel-documents", h.CreateTravelDocument)
	mux.HandleFunc("GET /travel-documents/{id}", h.GetTravelDocument)
	mux.HandleFunc("PUT /travel-documents/{id}", h.UpdateTravelDocument)
}
