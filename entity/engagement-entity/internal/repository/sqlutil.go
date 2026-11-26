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

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
)

// WithTx runs fn in a transaction on db and commits when fn returns nil;
// any error (or panic) rolls back. The legacy Ballerina code ran several
// multi-statement writes without a transaction; the port wraps them so a
// failure half way leaves nothing behind.
func WithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Placeholders returns "?, ?, ?" for n bind parameters (n > 0). Callers must
// never build an IN () with n == 0: the legacy queries failed with a syntax
// error there; the port skips the condition or returns no rows instead.
func Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// Args converts a typed slice to []any for a query's bind parameters.
func Args[T any](vals []T) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

// LikeContains escapes s for a LIKE pattern and wraps it in %...%.
func LikeContains(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// SortColumns maps each whitelisted sort key (without the leading "-") to the
// fixed column expressions it orders by. The expressions are written in the
// code, never taken from a request, so a sort can never reach the SQL text.
type SortColumns map[string][]string

// Order returns the ORDER BY clause (with its leading space) for sort, a key
// with an optional leading "-" for descending: every column of the key gets
// DESC when descending, and tiebreak is appended last so pages stay stable
// (left out when the key already orders by that column).
// It reports false for an empty or unknown sort, and the caller then uses
// its own default order.
func (c SortColumns) Order(sort, tiebreak string) (string, bool) {
	key, desc := strings.CutPrefix(sort, "-")
	cols, ok := c[key]
	if !ok {
		return "", false
	}
	parts := make([]string, 0, len(cols)+1)
	for _, col := range cols {
		if desc {
			col += " DESC"
		}
		parts = append(parts, col)
	}
	if !slices.Contains(cols, tiebreak) {
		parts = append(parts, tiebreak)
	}
	return " ORDER BY " + strings.Join(parts, ", "), true
}
