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
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/handler"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// travelOptions builds the travel services' options from the shared deps.
func travelOptions(a *area) service.TravelOptions {
	r := a.Recipients
	return service.TravelOptions{
		VisaNotApplicableID: a.Travel.VisaNotApplicableID,
		NoValidVisaID:       a.Travel.NoValidVisaID,
		Email: email.TravelSettings{
			LogisticsGroup: r.TravelLogisticsGroup, FinanceGroup: r.FinanceGroup, AppBaseURL: r.AppBaseURL,
			CCTraveller: r.CCTraveller, CompanyRecipients: r.CompanyRecipients,
		},
		Campaigns: a.Campaigns,
		Now:       a.Now,
		Budgets:   budgetHooks(a.budgets),
	}
}

// registerTravelRoutes registers the travel screen area's routes. The area
// owns this file: its dependency graph (service → handler) and its routes
// live here, each behind a.handle / a.handleIdem with its legacy gate.
func registerTravelRoutes(a *area) {
	opts := travelOptions(a)
	a.travel = service.NewTravelService(a.Entity, a.HR, a.Email, opts)
	h := handler.NewTravelHandler(
		a.travel,
		service.NewTravelDocumentsService(a.Entity, a.HR, a.Files, a.Now, opts.VisaNotApplicableID, opts.NoValidVisaID),
	)
	travel := func(ids ...int) domain.Gate { return domain.Gate{domain.AppTravel: ids} }

	// Travel requests. Record-level scope (full / agency / own) is in the
	// service.
	a.handle("GET /travel-requests", travel(1, 2, 18), h.ListTravelRequests)
	a.handle("GET /travel-requests/{jobNumber}", travel(1, 2, 5, 18, 36), h.GetTravelRequest)
	a.handle("PATCH /travel-requests/{jobNumber}", travel(1, 4, 5, 6), h.UpdateTravelRequest)
	a.handle("GET /travel-requests/{jobNumber}/visa-options", travel(1, 6), h.VisaOptions)
	a.handle("GET /me/travel-requests", travel(49, 36), h.ListMyTravelRequests)

	// Visas.
	a.handleAudited("GET /visas", travel(8), auditList("visa", false), h.ListVisas)
	a.handleIdem("POST /visas", travel(7), h.AddVisa)
	a.handleAudited("GET /visas/{visaId}", travel(9, 20), auditOne(domain.AuditView, "visa", "visaId"), h.GetVisa)
	a.handle("PUT /visas/{visaId}", travel(10), h.UpdateVisa)
	a.handleAudited("GET /visas/{visaId}/image", travel(9, 20, 18), auditOne(domain.AuditDownload, "visa", "visaId"), h.VisaImage)
	a.handleAudited("GET /me/visas", travel(20), auditList("visa", true), h.ListMyVisas)
	a.handleIdem("POST /me/visas", travel(21), h.AddMyVisa)

	// Travel documents.
	a.handleAudited("GET /travel-documents", travel(11), auditList("travel_document", false), h.ListTravelDocuments)
	a.handleIdem("POST /travel-documents", travel(14), h.AddTravelDocument)
	a.handleAudited("GET /travel-documents/{documentId}", travel(12, 23),
		auditOne(domain.AuditView, "travel_document", "documentId"), h.GetTravelDocument)
	a.handle("PUT /travel-documents/{documentId}", travel(13), h.UpdateTravelDocument)
	a.handleAudited("GET /travel-documents/{documentId}/image", travel(12, 23),
		auditOne(domain.AuditDownload, "travel_document", "documentId"), h.TravelDocumentImage)
	a.handleAudited("GET /me/travel-documents", travel(23), auditList("travel_document", true), h.ListMyTravelDocuments)
	a.handleIdem("POST /me/travel-documents", travel(24), h.AddMyTravelDocument)

	// Passports for the visa form's Passport field.
	a.handleAudited("GET /travellers/{email}/passports", travel(7, 10), auditPassports(false), h.TravellerPassports)
	a.handleAudited("GET /me/passports", travel(21), auditPassports(true), h.MyPassports)
}
