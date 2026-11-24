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

// EmailAuditHandler serves the allocation_db email log endpoint.
type EmailAuditHandler struct {
	svc service.AllocationEmailAuditService
}

// NewEmailAuditHandler constructs an EmailAuditHandler.
func NewEmailAuditHandler(svc service.AllocationEmailAuditService) *EmailAuditHandler {
	return &EmailAuditHandler{svc: svc}
}

// InsertAllocationEmailAudit handles POST /allocation-email-audit-logs.
func (h *EmailAuditHandler) InsertAllocationEmailAudit(w http.ResponseWriter, r *http.Request) {
	var in domain.EmailAuditInput
	if !decodeRequest(w, r, &in) {
		return
	}
	if err := h.svc.InsertEmailAudit(r.Context(), in); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
