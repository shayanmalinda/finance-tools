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
	"net/http"
	"strings"
)

const userEmailHeader = "X-User-Email"

type actorKey struct{}

// Actor stores the signed-in user the Engagement App Backend names in
// X-User-Email, for the audit trail.
func Actor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(strings.TrimSpace(r.Header.Get(userEmailHeader)))
		if email == "" {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, email)))
	})
}

// ActorFromContext returns the user stored by Actor, or "".
func ActorFromContext(ctx context.Context) string {
	v, _ := ctx.Value(actorKey{}).(string)
	return v
}
