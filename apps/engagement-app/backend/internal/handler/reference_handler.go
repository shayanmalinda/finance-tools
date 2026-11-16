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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// ReferenceHandler serves the lookups: reference data, accounts,
// opportunities, campaigns, employees and the org structure.
type ReferenceHandler struct{ svc service.ReferenceService }

// NewReferenceHandler constructs a ReferenceHandler.
func NewReferenceHandler(svc service.ReferenceService) *ReferenceHandler {
	return &ReferenceHandler{svc: svc}
}

func referenceWriteItems[T any](w http.ResponseWriter, r *http.Request, items []T, err error) {
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	writeJSON(w, domain.ReferenceItems[T]{Items: items})
}

// ReferenceData handles GET /reference-data.
func (h *ReferenceHandler) ReferenceData(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.ReferenceData(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// Accounts handles GET /accounts?search=&limit=.
func (h *ReferenceHandler) Accounts(w http.ResponseWriter, r *http.Request) {
	limit, err := engagementQueryInt(r, "limit")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, err := h.svc.Accounts(r.Context(), r.URL.Query().Get("search"), limit)
	referenceWriteItems(w, r, items, err)
}

// Opportunities handles GET /accounts/{accountId}/opportunities?engagementTypeId=.
func (h *ReferenceHandler) Opportunities(w http.ResponseWriter, r *http.Request) {
	typeID, err := engagementQueryInt(r, "engagementTypeId")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, err := h.svc.Opportunities(r.Context(), r.PathValue("accountId"), typeID)
	referenceWriteItems(w, r, items, err)
}

// Campaigns handles GET /campaigns?search=&limit=.
func (h *ReferenceHandler) Campaigns(w http.ResponseWriter, r *http.Request) {
	limit, err := engagementQueryInt(r, "limit")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, err := h.svc.Campaigns(r.Context(), r.URL.Query().Get("search"), limit)
	referenceWriteItems(w, r, items, err)
}

// Employees handles GET /employees?search=&limit=.
func (h *ReferenceHandler) Employees(w http.ResponseWriter, r *http.Request) {
	limit, err := engagementQueryInt(r, "limit")
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	items, err := h.svc.Employees(r.Context(), r.URL.Query().Get("search"), limit)
	referenceWriteItems(w, r, items, err)
}

// OrgStructure handles GET /org-structure.
func (h *ReferenceHandler) OrgStructure(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.OrgStructure(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
