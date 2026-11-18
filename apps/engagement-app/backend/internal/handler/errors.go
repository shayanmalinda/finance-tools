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

// Package handler contains the backend's HTTP handlers.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/redact"
)

// statusClientClosedRequest is the non-standard 499 for a request the client
// abandoned.
const statusClientClosedRequest = 499

// writeJSON writes v as a 200 JSON response.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeServiceError maps a service-layer error to the HTTP response. Every
// error body is {"code": <status>, "message": "..."}; One WSO2 shows the
// message as is, so messages are written for people.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		ve  *apierror.ValidationError
		ue  *apierror.UnauthorizedError
		fe  *apierror.ForbiddenError
		nfe *apierror.NotFoundError
		ce  *apierror.ConflictError
		sue *apierror.ServiceUnavailableError
		up  *apierror.UpstreamError
	)
	ctx := r.Context()
	switch {
	case errors.As(err, &ve):
		apierror.WriteJSON(w, http.StatusBadRequest, ve.Msg)
	case errors.As(err, &ue):
		apierror.WriteJSON(w, http.StatusUnauthorized, ue.Msg)
	case errors.As(err, &fe):
		apierror.WriteJSON(w, http.StatusForbidden, fe.Msg)
	case errors.As(err, &nfe):
		apierror.WriteJSON(w, http.StatusNotFound, nfe.Msg)
	case errors.As(err, &ce):
		apierror.WriteJSON(w, http.StatusConflict, ce.Msg)
	case errors.As(err, &up) && up.ClientError():
		// The dependency rejected what we sent; its message names the problem.
		slog.WarnContext(ctx, "dependency rejected request", "method", r.Method, "path", redact.Path(r.URL.Path), "err", up)
		if up.StatusCode == http.StatusUnauthorized || up.StatusCode == http.StatusForbidden {
			// A service refusing our credentials is not the user's permission problem.
			apierror.WriteJSON(w, http.StatusServiceUnavailable, "A connected service refused the request. Please try again later.")
			return
		}
		apierror.WriteJSON(w, up.StatusCode, up.Msg)
	case errors.As(err, &up), errors.As(err, &sue):
		slog.ErrorContext(ctx, "dependency unavailable", "method", r.Method, "path", redact.Path(r.URL.Path), "err", err)
		apierror.WriteJSON(w, http.StatusServiceUnavailable, "Service temporarily unavailable. Please try again shortly.")
	case errors.Is(err, context.DeadlineExceeded):
		slog.WarnContext(ctx, "request timed out", "method", r.Method, "path", redact.Path(r.URL.Path))
		apierror.WriteJSON(w, http.StatusGatewayTimeout, "The request timed out. Please try again.")
	case errors.Is(err, context.Canceled):
		// No one reads this, but a non-2xx status keeps Idempotency from
		// replaying it and Audit from recording it as a success.
		slog.InfoContext(ctx, "request canceled by the client", "method", r.Method, "path", redact.Path(r.URL.Path))
		apierror.WriteJSON(w, statusClientClosedRequest, "The request was canceled.")
	default:
		slog.ErrorContext(ctx, "internal error", "method", r.Method, "path", redact.Path(r.URL.Path), "err", err)
		apierror.WriteJSON(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
	}
}
