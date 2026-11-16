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
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

const (
	maxRequestBodySize = 1 << 20 // 1 MiB
	defaultLimit       = 50
	maxLimit           = 500
)

// decodeJSON decodes a JSON request body into dst: unknown fields rejected,
// 1 MiB cap, a single object. It writes the 400 and returns false on failure.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, decodeErrMsg(err))
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.WriteJSON(w, http.StatusBadRequest, "The request body must contain a single JSON object.")
		return false
	}
	return true
}

func decodeErrMsg(err error) string {
	var maxBytes *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &maxBytes):
		return "The request body is too large."
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("The request body is not valid JSON (at position %d).", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		return fmt.Sprintf("Invalid value for %q: expected %s.", typeErr.Field, typeErr.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return "Unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ") + " in the request body."
	}
	return "Invalid request body."
}

// writeJSONStatus writes v as JSON with status.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// permsFrom returns the caller's privileges attached by middleware.Require.
// Every route behind Require has them; a route without is a wiring bug, and
// the request is refused rather than served without a check.
func permsFrom(w http.ResponseWriter, r *http.Request) (domain.Perms, bool) {
	p, ok := reqcontext.PermsFrom(r.Context())
	if !ok {
		apierror.WriteJSON(w, http.StatusForbidden, "You do not have permission to perform this action.")
		return domain.Perms{}, false
	}
	return p, true
}

// queryList returns the repeated (and comma-free) values of a query
// parameter, trimmed, empties dropped.
func queryList(r *http.Request, key string) []string {
	var out []string
	for _, v := range r.URL.Query()[key] {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// queryInts is queryList parsed as integers; a malformed value is a 400.
func queryInts(r *http.Request, key string) ([]int, error) {
	var out []int
	for _, v := range queryList(r, key) {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, &apierror.ValidationError{Msg: fmt.Sprintf("%s must be a whole number.", key)}
		}
		out = append(out, n)
	}
	return out, nil
}

// queryDate parses an optional YYYY-MM-DD parameter.
func queryDate(r *http.Request, key string) (string, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return "", &apierror.ValidationError{Msg: fmt.Sprintf("%s must be a date (YYYY-MM-DD).", key)}
	}
	return v, nil
}

// pagination parses limit (default 50, max 500) and offset (default 0).
func pagination(r *http.Request) (limit, offset int, err error) {
	limit, offset = defaultLimit, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > maxLimit {
			return 0, 0, &apierror.ValidationError{Msg: fmt.Sprintf("limit must be between 1 and %d.", maxLimit)}
		}
		limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return 0, 0, &apierror.ValidationError{Msg: "offset must be 0 or more."}
		}
		offset = n
	}
	return limit, offset, nil
}

// reportCSV reports whether the caller asked for format=csv; any format
// other than json or csv is a 400.
func reportCSV(r *http.Request) (bool, error) {
	switch strings.ToLower(r.URL.Query().Get("format")) {
	case "", "json":
		return false, nil
	case "csv":
		return true, nil
	}
	return false, &apierror.ValidationError{Msg: "format must be json or csv."}
}

// writeCSV writes an RFC 4180 CSV attachment.
func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	cw := csv.NewWriter(w)
	_ = cw.Write(header)
	for _, row := range rows {
		_ = cw.Write(neutraliseFormulas(row))
	}
	cw.Flush()
}

// neutraliseFormulas prefixes a cell that a spreadsheet would run as a
// formula (=, +, -, @ at the start) with a single quote (CSV injection).
// Plain negative numbers are left as they are.
func neutraliseFormulas(row []string) []string {
	out := make([]string, len(row))
	for i, c := range row {
		if c != "" && strings.ContainsRune("=+-@\t\r", rune(c[0])) {
			if _, err := strconv.ParseFloat(c, 64); err != nil {
				c = "'" + c
			}
		}
		out[i] = c
	}
	return out
}
