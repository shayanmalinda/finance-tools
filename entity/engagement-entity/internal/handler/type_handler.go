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

// TypeHandler serves the engagement and allocation type lookups.
type TypeHandler struct{ svc service.TypeService }

// NewTypeHandler constructs a TypeHandler.
func NewTypeHandler(svc service.TypeService) *TypeHandler {
	return &TypeHandler{svc: svc}
}

// ListEngagementTypes handles GET /engagement-types?type=Internal|Customer&isActive=true|false.
func (h *TypeHandler) ListEngagementTypes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.EngagementTypeFilter{Type: q.Get("type")}
	if s := q.Get("isActive"); s != "" {
		v, err := strconv.ParseBool(s)
		if err != nil {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "isActive must be true or false"})
			return
		}
		f.IsActive = &v
	}
	resp, err := h.svc.ListEngagementTypes(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}

// ListAllocationTypes handles GET /allocation-types?engagementTypeId=&status=Active|Deleted.
func (h *TypeHandler) ListAllocationTypes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.AllocationTypeFilter{Status: q.Get("status")}
	if s := q.Get("engagementTypeId"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "engagementTypeId must be a positive integer"})
			return
		}
		f.EngagementTypeID = v
	}
	resp, err := h.svc.ListAllocationTypes(r.Context(), f)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
