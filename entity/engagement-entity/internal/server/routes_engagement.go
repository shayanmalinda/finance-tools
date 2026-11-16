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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/handler"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// registerEngagementRoutes registers the engagements area's routes:
// engagements, reference data, Role Settings and the Allocation
// Types settings writes. The area owns this file.
//
// The NetSuite marketing event of a new Marketing Event is created through
// engagementMarketingEvents, nil until a NetSuite client exists (the
// engagement is then saved without one and a warning is logged).
func registerEngagementRoutes(mux *http.ServeMux, d Deps) {
	pools := d.Pools

	// ── Repositories ────────────────────────────────────────────────────────
	engagementRepo := repository.NewEngagementRepository(pools.Allocation, pools.Travel)
	referenceRepo := repository.NewReferenceRepository(pools.Allocation, pools.Travel)
	typeRepo := repository.NewTypeRepository(pools.Allocation)
	settingsRepo := repository.NewSettingsRepository(pools.Allocation)
	roleRepos := map[domain.App]repository.RoleRepository{
		domain.AppAllocations: repository.NewRoleRepository(pools.Allocation, domain.AppAllocations),
		domain.AppTravel:      repository.NewRoleRepository(pools.Travel, domain.AppTravel),
	}
	accessRepos := map[domain.App]repository.AccessRepository{
		domain.AppAllocations: repository.NewAccessRepository(pools.Allocation, domain.AppAllocations),
		domain.AppTravel:      repository.NewAccessRepository(pools.Travel, domain.AppTravel),
	}

	// ── Services ─────────────────────────────────────────────────────────────
	engagementSvc := service.NewEngagementService(engagementRepo, engagementMarketingEvents(d))
	referenceSvc := service.NewReferenceService(typeRepo, referenceRepo)
	roleSvc := service.NewRoleService(roleRepos, accessRepos)
	settingsSvc := service.NewSettingsService(settingsRepo)

	// ── Handlers ─────────────────────────────────────────────────────────────
	engagementH := handler.NewEngagementHandler(engagementSvc)
	referenceH := handler.NewReferenceHandler(referenceSvc, roleSvc, settingsSvc)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /reference-data", referenceH.ReferenceData)

	mux.HandleFunc("POST /engagements/search", engagementH.Search)
	mux.HandleFunc("GET /engagements/facets", engagementH.Facets)
	mux.HandleFunc("POST /engagements", engagementH.Create)
	mux.HandleFunc("GET /engagements/{id}", engagementH.Get)
	mux.HandleFunc("PATCH /engagements/{id}", engagementH.Update)
	mux.HandleFunc("GET /engagements/{id}/roster", engagementH.Allocations)

	mux.HandleFunc("GET /role-settings/{app}", referenceH.RoleSettings)
	mux.HandleFunc("PUT /role-settings/{app}/roles/{roleId}/privileges", referenceH.ReplaceRolePrivileges)

	mux.HandleFunc("POST /settings/allocation-types", referenceH.CreateAllocationType)
	mux.HandleFunc("PUT /settings/allocation-types/{id}", referenceH.UpdateAllocationType)
	mux.HandleFunc("DELETE /settings/allocation-types/{id}", referenceH.DeleteAllocationType)
}

// engagementMarketingEvents returns the NetSuite marketing event creator,
// or nil when NetSuite is switched off (NETSUITE_PUSH_ENABLED=false): a new
// Marketing Event is then saved without a NetSuite event, and a warning is
// logged. With NetSuite on, a failed call rolls the create back, as legacy.
func engagementMarketingEvents(d Deps) service.EngagementMarketingEventCreator {
	cfg := netsuite.ConfigFrom(d.NetSuite)
	if !cfg.Enabled || len(cfg.Missing()) > 0 {
		return nil
	}
	return netsuite.New(cfg, nil)
}
