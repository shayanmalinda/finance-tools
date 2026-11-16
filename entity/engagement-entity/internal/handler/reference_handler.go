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
	"strconv"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/service"
)

// ReferenceHandler serves GET /reference-data, Role Settings and
// the Allocation Types settings writes.
type ReferenceHandler struct {
	refs     service.ReferenceService
	roles    service.RoleService
	settings service.SettingsService
}

// NewReferenceHandler constructs a ReferenceHandler.
func NewReferenceHandler(refs service.ReferenceService, roles service.RoleService,
	settings service.SettingsService) *ReferenceHandler {
	return &ReferenceHandler{refs: refs, roles: roles, settings: settings}
}

// referencePathInt reads a positive integer path value, writing the 400.
func referencePathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	v, err := strconv.Atoi(r.PathValue(name))
	if err != nil || v <= 0 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: name + " must be a positive integer"})
		return 0, false
	}
	return v, true
}

func referenceNoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// ReferenceData handles GET /reference-data.
func (h *ReferenceHandler) ReferenceData(w http.ResponseWriter, r *http.Request) {
	resp, err := h.refs.ReferenceData(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// RoleSettings handles GET /role-settings/{app}.
func (h *ReferenceHandler) RoleSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := h.roles.Settings(r.Context(), domain.App(r.PathValue("app")))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ReplaceRolePrivileges handles PUT /role-settings/{app}/roles/{roleId}/privileges.
func (h *ReferenceHandler) ReplaceRolePrivileges(w http.ResponseWriter, r *http.Request) {
	roleID, ok := referencePathInt(w, r, "roleId")
	if !ok {
		return
	}
	var req domain.RolePrivilegesUpdateRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.roles.ReplacePrivileges(r.Context(), domain.App(r.PathValue("app")), roleID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// CreateAllocationType handles POST /settings/allocation-types.
func (h *ReferenceHandler) CreateAllocationType(w http.ResponseWriter, r *http.Request) {
	var in domain.SettingsAllocationTypeInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.settings.CreateAllocationType(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	engagementWriteCreated(w, resp)
}

// UpdateAllocationType handles PUT /settings/allocation-types/{id}.
func (h *ReferenceHandler) UpdateAllocationType(w http.ResponseWriter, r *http.Request) {
	id, ok := referencePathInt(w, r, "id")
	if !ok {
		return
	}
	var in domain.SettingsAllocationTypeInput
	if !decodeRequest(w, r, &in) {
		return
	}
	resp, err := h.settings.UpdateAllocationType(r.Context(), id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// DeleteAllocationType handles DELETE /settings/allocation-types/{id} with
// body {"actor": "..."}.
func (h *ReferenceHandler) DeleteAllocationType(w http.ResponseWriter, r *http.Request) {
	id, ok := referencePathInt(w, r, "id")
	if !ok {
		return
	}
	var req domain.SettingsAllocationTypeDeleteRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	if err := h.settings.DeleteAllocationType(r.Context(), id, req.Actor); err != nil {
		writeServiceError(w, r, err)
		return
	}
	referenceNoContent(w)
}
