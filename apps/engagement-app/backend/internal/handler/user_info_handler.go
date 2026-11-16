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

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// UserInfoHandler serves GET /user-info.
type UserInfoHandler struct{ svc service.AccessService }

// NewUserInfoHandler constructs a UserInfoHandler.
func NewUserInfoHandler(svc service.AccessService) *UserInfoHandler {
	return &UserInfoHandler{svc: svc}
}

// Get handles GET /user-info: the caller's email and, separately for each
// legacy app, the privilege ids their groups grant. Any signed-in user may
// call it; a person with no privileges gets empty lists, not 403, so the
// webapp can tell "denied" from "failed".
func (h *UserInfoHandler) Get(w http.ResponseWriter, r *http.Request) {
	caller, ok := reqcontext.CallerFrom(r.Context())
	if !ok {
		writeServiceError(w, r, &apierror.UnauthorizedError{Msg: "no signed-in user"})
		return
	}
	resp, err := h.svc.UserInfo(r.Context(), caller)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
