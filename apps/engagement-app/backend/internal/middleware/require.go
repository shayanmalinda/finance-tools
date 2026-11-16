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

package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// PermsResolver resolves everything a caller may do. service.AccessService
// implements it.
type PermsResolver interface {
	Perms(ctx context.Context, caller reqcontext.Caller) (domain.Perms, error)
}

// Require passes callers holding any privilege in gate and puts their Perms in
// the context; 503 when the role tables cannot be read.
func Require(pr PermsResolver, gate domain.Gate) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, ok := reqcontext.CallerFrom(r.Context())
			if !ok {
				apierror.WriteJSON(w, http.StatusUnauthorized, unauthorizedMsg)
				return
			}
			p, err := pr.Perms(r.Context(), caller)
			if err != nil {
				slog.ErrorContext(r.Context(), "privilege check failed", "err", err)
				apierror.WriteJSON(w, http.StatusServiceUnavailable, "Unable to verify your access right now. Please try again shortly.")
				return
			}
			if !p.Passes(gate) {
				apierror.WriteJSON(w, http.StatusForbidden, forbiddenMsg)
				return
			}
			next.ServeHTTP(w, r.WithContext(reqcontext.WithPerms(r.Context(), p)))
		})
	}
}

const forbiddenMsg = "You do not have permission to perform this action."
