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
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// jwtAssertionHeader carries the JWT the Choreo API gateway forwards after it
// has validated the user's access token. One WSO2 sends the access token as
// Authorization: Bearer; behind the gateway it arrives as this header.
const jwtAssertionHeader = "X-Jwt-Assertion"

// Verifier validates a token and returns its claims. *oidc.MultiVerifier
// implements it; tests supply a fake.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (oidc.Claims, error)
}

// UserinfoFetcher reads email and groups from the identity provider's
// userinfo endpoint with the caller's access token. *oidc.UserinfoClient
// implements it.
type UserinfoFetcher interface {
	Fetch(ctx context.Context, accessToken string) (oidc.Claims, error)
}

const unauthorizedMsg = "You are not authorized to perform this action. Please sign in again."

// unavailableMsg answers a request whose access cannot be checked right now
// (the identity provider or the role tables are unreachable): the caller's
// access is unknown, not denied, so it is 503 rather than 401 or 403.
const unavailableMsg = "Unable to verify your access right now. Please try again shortly."

// Auth verifies the token from X-Jwt-Assertion or Authorization: Bearer and
// puts the caller in the context; a Bearer token lacking email or groups is
// completed from userinfo. No bypass: anything else is 401.
func Auth(v Verifier, userinfo UserinfoFetcher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, bearer := tokenFrom(r)
			if raw == "" {
				apierror.WriteJSON(w, http.StatusUnauthorized, unauthorizedMsg)
				return
			}
			claims, err := v.Verify(r.Context(), raw)
			if errors.Is(err, oidc.ErrKeysUnavailable) {
				slog.ErrorContext(r.Context(), "auth: signing keys unavailable", "err", err)
				apierror.WriteJSON(w, http.StatusServiceUnavailable, unavailableMsg)
				return
			}
			if err != nil {
				slog.WarnContext(r.Context(), "auth: token rejected", "err", err)
				apierror.WriteJSON(w, http.StatusUnauthorized, unauthorizedMsg)
				return
			}
			if bearer && userinfo != nil && (claims.Email == "" || !claims.GroupsPresent) {
				extra, err := userinfo.Fetch(r.Context(), raw)
				if err != nil {
					// The missing email or groups are unknown, not empty:
					// answering with no privileges would show "no access".
					slog.ErrorContext(r.Context(), "auth: userinfo lookup failed", "err", err)
					apierror.WriteJSON(w, http.StatusServiceUnavailable, unavailableMsg)
					return
				}
				if claims.Email == "" {
					claims.Email = extra.Email
				}
				if !claims.GroupsPresent {
					claims.Groups = extra.Groups
				}
			}
			if strings.TrimSpace(claims.Email) == "" {
				slog.WarnContext(r.Context(), "auth: rejected", "err", oidc.ErrNoEmail)
				apierror.WriteJSON(w, http.StatusUnauthorized, unauthorizedMsg)
				return
			}
			caller := reqcontext.Caller{
				Email:  strings.ToLower(strings.TrimSpace(claims.Email)),
				Groups: claims.Groups,
			}
			next.ServeHTTP(w, r.WithContext(reqcontext.WithCaller(r.Context(), caller)))
		})
	}
}

// tokenFrom returns the request's token and whether it came from
// Authorization: Bearer (an access token) rather than the gateway header.
func tokenFrom(r *http.Request) (string, bool) {
	if raw := strings.TrimSpace(r.Header.Get(jwtAssertionHeader)); raw != "" {
		return raw, false
	}
	const prefix = "bearer "
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):]), true
	}
	return "", false
}

// PrivilegeChecker answers whether groups hold a privilege in an app.
// service.AccessService implements it.
type PrivilegeChecker interface {
	HasAnyPrivilege(ctx context.Context, app domain.App, groups []string, ids ...int) (bool, error)
}

// RequireAnyPrivilege lets a request through only when the caller holds at
// least one of ids in app, the legacy privilege check every screen endpoint
// repeats on the server (client-side gating is presentation only). It must
// run inside Auth. A failure to read the role tables is 503, not 403: the
// caller's access is unknown, not denied.
func RequireAnyPrivilege(pc PrivilegeChecker, app domain.App, ids ...int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, ok := reqcontext.CallerFrom(r.Context())
			if !ok {
				apierror.WriteJSON(w, http.StatusUnauthorized, unauthorizedMsg)
				return
			}
			allowed, err := pc.HasAnyPrivilege(r.Context(), app, caller.Groups, ids...)
			if err != nil {
				slog.ErrorContext(r.Context(), "privilege check failed", "app", app, "err", err)
				apierror.WriteJSON(w, http.StatusServiceUnavailable, unavailableMsg)
				return
			}
			if !allowed {
				apierror.WriteJSON(w, http.StatusForbidden, forbiddenMsg)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
