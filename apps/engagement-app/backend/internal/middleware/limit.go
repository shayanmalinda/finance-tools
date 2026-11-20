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
	"net/http"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
)

// LimitRetryAfter is the Retry-After (seconds) a refused request is given.
const LimitRetryAfter = "10"

// Limit runs at most n requests of the wrapped handler at once on this pod.
// One more is refused at once with 503 and Retry-After, not queued: the
// handlers it guards (unpaged reports) each hold a whole report in memory,
// so a burst must not pile up. n below 1 is treated as 1.
func Limit(n int) func(http.Handler) http.Handler {
	if n < 1 {
		n = 1
	}
	slots := make(chan struct{}, n)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
			default:
				w.Header().Set("Retry-After", LimitRetryAfter)
				apierror.WriteJSON(w, http.StatusServiceUnavailable, "Too many reports are being prepared. Please try again in a few seconds.")
				return
			}
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		})
	}
}
