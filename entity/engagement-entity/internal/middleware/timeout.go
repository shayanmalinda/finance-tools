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
	"time"
)

// Timeout returns middleware that cancels the request context after duration
// d, or after long (with the write deadline moved to match) for requests
// isLong picks (the NetSuite pushes).
func Timeout(d, long time.Duration, isLong func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := d
			if isLong != nil && isLong(r) {
				limit = long
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(long + 10*time.Second))
			}
			ctx, cancel := context.WithTimeout(r.Context(), limit)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
