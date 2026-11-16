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
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// SettingsHandler serves Role Settings, Partners and Allocation Types.
type SettingsHandler struct{ svc service.SettingsService }

// NewSettingsHandler constructs a SettingsHandler.
func NewSettingsHandler(svc service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

func settingsNoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// RoleSettings handles GET /role-settings/{app}.
func (h *SettingsHandler) RoleSettings(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.RoleSettings(r.Context(), p, domain.App(r.PathValue("app")))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// UpdateRolePrivileges handles PUT /role-settings/{app}/roles/{roleId}/privileges.
func (h *SettingsHandler) UpdateRolePrivileges(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	roleID, ok := engagementPathInt(w, r, "roleId")
	if !ok {
		return
	}
	var in domain.RoleSettingsUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.UpdateRolePrivileges(r.Context(), p, domain.App(r.PathValue("app")), roleID, in.PrivilegeIDs)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// AllocationTypes handles GET /settings/allocation-types?engagementTypeId=&search=.
func (h *SettingsHandler) AllocationTypes(w http.ResponseWriter, r *http.Request) {
	typeID, err := engagementQueryInt(r, "engagementTypeId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > 200 {
		writeServiceError(w, r, &apierror.ValidationError{Msg: "search can have at most 200 characters."})
		return
	}
	items, err := h.svc.AllocationTypes(r.Context(), typeID, search)
	referenceWriteItems(w, r, items, err)
}

// CreateAllocationType handles POST /settings/allocation-types.
func (h *SettingsHandler) CreateAllocationType(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	var in domain.SettingsAllocationTypeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.CreateAllocationType(r.Context(), p, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// UpdateAllocationType handles PUT /settings/allocation-types/{allocationTypeId}.
func (h *SettingsHandler) UpdateAllocationType(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := engagementPathInt(w, r, "allocationTypeId")
	if !ok {
		return
	}
	var in domain.SettingsAllocationTypeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	resp, err := h.svc.UpdateAllocationType(r.Context(), p, id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// DeleteAllocationType handles DELETE /settings/allocation-types/{allocationTypeId}.
func (h *SettingsHandler) DeleteAllocationType(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	id, ok := engagementPathInt(w, r, "allocationTypeId")
	if !ok {
		return
	}
	if err := h.svc.DeleteAllocationType(r.Context(), p, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	settingsNoContent(w)
}
