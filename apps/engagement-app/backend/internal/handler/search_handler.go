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
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

// SearchHandler serves the global search.
type SearchHandler struct{ svc service.SearchService }

// NewSearchHandler constructs a SearchHandler.
func NewSearchHandler(svc service.SearchService) *SearchHandler { return &SearchHandler{svc: svc} }

// Search handles GET /search?q=&limit=. limit is per group; absent means the
// default, above the maximum is clamped.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	p, ok := permsFrom(w, r)
	if !ok {
		return
	}
	limit := 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeServiceError(w, r, &apierror.ValidationError{Msg: "limit must be a whole number of 1 or more."})
			return
		}
		limit = n
	}
	resp, err := h.svc.Search(r.Context(), p, r.URL.Query().Get("q"), limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, resp)
}
