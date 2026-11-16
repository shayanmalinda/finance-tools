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

// Package audit writes audit_event rows (same table in allocation_db and
// travelapp), each in the transaction of the change it describes. The table
// is append-only: there is no update or delete path.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/middleware"
)

// Action is what happened to the entity.
type Action string

const (
	Create       Action = "Create"
	Update       Action = "Update"
	Delete       Action = "Delete"
	StatusChange Action = "Status Change"
	Push         Action = "Push"
	View         Action = "View"
	Download     Action = "Download"
	Export       Action = "Export"
)

const source = "engagement-app"

// Change is one field before and after a write.
type Change struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

// Event is one audit_event row.
type Event struct {
	Action     Action
	EntityType string
	EntityID   string
	ParentType string
	ParentID   string
	// ParentColumn takes ParentID from this column of a tracked row.
	ParentColumn string
	// Actor is used when the request names no user (X-User-Email).
	Actor    string
	Changes  map[string]Change
	Metadata map[string]any
}

// Querier is a *sql.Tx, or a *sql.DB for an event that is not part of a write.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var enabled atomic.Bool

func init() { enabled.Store(true) }

// SetEnabled switches recording on or off (off until both schemas have the table).
func SetEnabled(on bool) { enabled.Store(on) }

// Enabled reports whether events are recorded.
func Enabled() bool { return enabled.Load() }

// TableExists reports whether db's schema has audit_event.
func TableExists(ctx context.Context, db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'audit_event'`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("detect audit_event: %w", err)
	}
	return n > 0, nil
}

// masked are the sensitive table.column values, stored as their last 4 characters.
var masked = map[string]bool{"employee_travel_document.document_no": true}

// Mask keeps the last 4 characters of s.
func Mask(s string) string {
	r := []rune(s)
	if len(r) <= 4 {
		return "****"
	}
	return "****" + string(r[len(r)-4:])
}

// Record inserts e.
func Record(ctx context.Context, q Querier, e Event) error {
	if !Enabled() {
		return nil
	}
	actor := middleware.ActorFromContext(ctx)
	if actor == "" {
		actor = strings.ToLower(strings.TrimSpace(e.Actor))
	}
	actorType := "user"
	if actor == "" {
		actorType = "system"
	}
	changes, err := jsonOrNil(len(e.Changes) > 0, e.Changes)
	if err != nil {
		return err
	}
	metadata, err := jsonOrNil(len(e.Metadata) > 0, e.Metadata)
	if err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO audit_event
		(actor_email, actor_type, action, entity_type, entity_id, parent_type, parent_id, request_id, source, changes, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullable(actor), actorType, string(e.Action), e.EntityType, nullable(e.EntityID), nullable(e.ParentType),
		nullable(e.ParentID), nullable(cut(middleware.CorrelationIDFromContext(ctx), 64)), source, changes, metadata); err != nil {
		return fmt.Errorf("audit.Record %s %s: %w", e.Action, e.EntityType, err)
	}
	return nil
}

func jsonOrNil(present bool, v any) (any, error) {
	if !present {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: marshal: %w", err)
	}
	return string(b), nil
}

// cut keeps a caller-sent value within its column.
func cut(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type row map[string]any

// Tracker holds rows of one table read before a write, to record one event
// per row with the fields the write changed.
type Tracker struct {
	table, key string
	ids        []string
	before     map[string]row
}

// Track reads the rows of table matching where (locked) before a write. key
// is the table's id column.
func Track(ctx context.Context, q Querier, table, key, where string, args ...any) (*Tracker, error) {
	t := &Tracker{table: table, key: key, before: map[string]row{}}
	if !Enabled() {
		return t, nil
	}
	rows, err := selectRows(ctx, q, `SELECT * FROM `+table+` WHERE `+where+` FOR UPDATE`, args...)
	if err != nil {
		return nil, fmt.Errorf("audit.Track %s: %w", table, err)
	}
	for _, r := range rows {
		id := t.idOf(r)
		t.ids = append(t.ids, id)
		t.before[id] = r
	}
	return t, nil
}

// Inserted tracks rows a write has just inserted.
func Inserted(table, key string, ids ...any) *Tracker {
	t := &Tracker{table: table, key: key, before: map[string]row{}}
	for _, id := range ids {
		t.ids = append(t.ids, fmt.Sprint(id))
	}
	return t
}

// Record reads the tracked rows again and records e for each, with the
// changed fields. An Update that changes a status column is a Status Change.
func (t *Tracker) Record(ctx context.Context, q Querier, e Event) error {
	if !Enabled() || len(t.ids) == 0 {
		return nil
	}
	args := make([]any, len(t.ids))
	for i, id := range t.ids {
		args[i] = id
	}
	rows, err := selectRows(ctx, q, `SELECT * FROM `+t.table+` WHERE `+t.key+` IN (`+
		strings.TrimSuffix(strings.Repeat("?, ", len(args)), ", ")+`)`, args...)
	if err != nil {
		return fmt.Errorf("audit.Record %s: %w", t.table, err)
	}
	after := map[string]row{}
	for _, r := range rows {
		after[t.idOf(r)] = r
	}
	for _, id := range t.ids {
		ev := e
		ev.EntityID = id
		ev.Changes = t.diff(t.before[id], after[id])
		for k, v := range e.Changes {
			ev.Changes[k] = v
		}
		if len(ev.Changes) == 0 && len(ev.Metadata) == 0 && t.before[id] != nil {
			continue
		}
		if ev.Action == Update && statusChanged(ev.Changes) {
			ev.Action = StatusChange
		}
		if ev.ParentID == "" && ev.ParentColumn != "" {
			v := valueOf(after[id], ev.ParentColumn)
			if v == nil {
				v = valueOf(t.before[id], ev.ParentColumn)
			}
			ev.ParentID = str(v)
		}
		if err := Record(ctx, q, ev); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tracker) idOf(r row) string { return str(valueOf(r, t.key)) }

func (t *Tracker) diff(before, after row) map[string]Change {
	out := map[string]Change{}
	seen := map[string]bool{}
	for _, r := range []row{before, after} {
		for col := range r {
			if seen[col] {
				continue
			}
			seen[col] = true
			b, a := before[col], after[col]
			if reflect.DeepEqual(b, a) {
				continue
			}
			if masked[t.table+"."+strings.ToLower(col)] {
				b, a = maskValue(b), maskValue(a)
			}
			out[col] = Change{Before: b, After: a}
		}
	}
	return out
}

// statusChanged is true when the only real changes are status columns;
// bookkeeping columns (who/when) are ignored.
func statusChanged(changes map[string]Change) bool {
	status := false
	for col := range changes {
		c := strings.ToLower(col)
		switch {
		case c == "status" || strings.HasSuffix(c, "_status"):
			status = true
		case bookkeeping[c]:
		default:
			return false
		}
	}
	return status
}

var bookkeeping = map[string]bool{
	"updated_by": true, "updated_date": true, "updated_at": true,
	"last_updated_by": true, "last_updated_date": true, "last_updated_at": true,
}

func maskValue(v any) any {
	if v == nil {
		return nil
	}
	return Mask(str(v))
}

// valueOf reads col of r, ignoring case (legacy queries spell id and Id).
func valueOf(r row, col string) any {
	if v, ok := r[col]; ok {
		return v
	}
	for k, v := range r {
		if strings.EqualFold(k, col) {
			return v
		}
	}
	return nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func selectRows(ctx context.Context, q Querier, query string, args ...any) ([]row, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	var out []row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := row{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				if types[i].DatabaseTypeName() == "BIT" {
					r[c] = bitValue(b)
				} else {
					r[c] = string(b)
				}
			} else {
				r[c] = vals[i]
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func bitValue(b []byte) uint64 {
	var n uint64
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	return n
}
