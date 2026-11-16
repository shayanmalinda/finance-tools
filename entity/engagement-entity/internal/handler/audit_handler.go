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

// AuditHandler serves POST /audit-events.
type AuditHandler struct{ svc service.AuditService }

// NewAuditHandler constructs an AuditHandler.
func NewAuditHandler(svc service.AuditService) *AuditHandler { return &AuditHandler{svc: svc} }

// Record handles POST /audit-events.
func (h *AuditHandler) Record(w http.ResponseWriter, r *http.Request) {
	var in domain.AuditReadEvent
	if !decodeRequest(w, r, &in) {
		return
	}
	if err := h.svc.RecordRead(r.Context(), in); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
