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

package handler

import (
	"net/http"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// AccessHandler serves the legacy apps' roles and privileges under
// /access/{app}/..., where {app} is allocations or travel.
type AccessHandler struct{ svc service.AccessService }

// NewAccessHandler constructs an AccessHandler.
func NewAccessHandler(svc service.AccessService) *AccessHandler {
	return &AccessHandler{svc: svc}
}

func appFrom(r *http.Request) domain.App { return domain.App(r.PathValue("app")) }

// ListRoles handles GET /access/{app}/roles.
func (h *AccessHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ListRoles(r.Context(), appFrom(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListPrivileges handles GET /access/{app}/privileges.
func (h *AccessHandler) ListPrivileges(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ListPrivileges(r.Context(), appFrom(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// RolePrivilegeMap handles GET /access/{app}/role-privileges. Callers fetch
// the whole map and cache it; this is not a per-request authorisation check.
func (h *AccessHandler) RolePrivilegeMap(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.RolePrivilegeMap(r.Context(), appFrom(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// TravelAgency handles GET /access/travel/agency?email=.
func (h *AccessHandler) TravelAgency(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.TravelAgency(r.Context(), r.URL.Query().Get("email"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
