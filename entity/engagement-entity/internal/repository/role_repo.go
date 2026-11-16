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
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ErrRoleNotFound is returned when the app has no role with the id.
var ErrRoleNotFound = errors.New("role not found")

// RoleRepository reads and replaces one legacy app's role privileges (Role
// Settings). Both schemas use the same roles / role_privilege tables; the
// privilege audit differs per app.
type RoleRepository interface {
	// Roles returns every role with its privilege ids.
	Roles(ctx context.Context) ([]domain.RoleWithPrivileges, error)
	Role(ctx context.Context, id int) (domain.RoleWithPrivileges, error)
	// ReplacePrivileges replaces the role's privileges and writes the
	// legacy privilege audit.
	ReplacePrivileges(ctx context.Context, roleID int, privilegeIDs []int, updatedBy string) error
}

type roleRepo struct {
	db  *sql.DB
	app domain.App
}

// NewRoleRepository constructs a RoleRepository over app's schema. db must be
// that app's pool.
func NewRoleRepository(db *sql.DB, app domain.App) RoleRepository {
	return &roleRepo{db: db, app: app}
}

// Roles ports getRolesQuery and getPrivilegesByRoleQuery (one query for all
// roles instead of one per role).
func (r *roleRepo) Roles(ctx context.Context) ([]domain.RoleWithPrivileges, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM roles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("%s roles.List: %w", r.app, err)
	}
	out := []domain.RoleWithPrivileges{}
	index := map[int]int{}
	for rows.Next() {
		role := domain.RoleWithPrivileges{PrivilegeIDs: []int{}}
		if err := rows.Scan(&role.ID, &role.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("%s roles.List scan: %w", r.app, err)
		}
		index[role.ID] = len(out)
		out = append(out, role)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("%s roles.List rows: %w", r.app, err)
	}

	rp, err := r.db.QueryContext(ctx, `SELECT DISTINCT role_id, privilege_id FROM role_privilege ORDER BY role_id, privilege_id`)
	if err != nil {
		return nil, fmt.Errorf("%s rolePrivilege.List: %w", r.app, err)
	}
	defer rp.Close()
	for rp.Next() {
		var roleID, privID int
		if err := rp.Scan(&roleID, &privID); err != nil {
			return nil, fmt.Errorf("%s rolePrivilege.List scan: %w", r.app, err)
		}
		if i, ok := index[roleID]; ok {
			out[i].PrivilegeIDs = append(out[i].PrivilegeIDs, privID)
		}
	}
	return out, rp.Err()
}

func (r *roleRepo) Role(ctx context.Context, id int) (domain.RoleWithPrivileges, error) {
	role := domain.RoleWithPrivileges{PrivilegeIDs: []int{}}
	err := r.db.QueryRowContext(ctx, `SELECT id, name FROM roles WHERE id = ?`, id).Scan(&role.ID, &role.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return role, ErrRoleNotFound
	}
	if err != nil {
		return role, fmt.Errorf("%s roles.Get: %w", r.app, err)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT privilege_id FROM role_privilege WHERE role_id = ? ORDER BY privilege_id`, id)
	if err != nil {
		return role, fmt.Errorf("%s rolePrivilege.ByRole: %w", r.app, err)
	}
	defer rows.Close()
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return role, fmt.Errorf("%s rolePrivilege.ByRole scan: %w", r.app, err)
		}
		role.PrivilegeIDs = append(role.PrivilegeIDs, p)
	}
	return role, rows.Err()
}

// ReplacePrivileges ports the two legacy updates:
//
//   - Allocations (db_functions.bal updatePrivilegesForRole): in one
//     transaction delete the role's rows and insert the new set (skipped
//     when empty); after the commit an audit_log row (resource ROLE, action
//     UPDATE) with the legacy description. An audit failure is only logged.
//   - Travel (database.bal updateRolePrivileges): in one transaction delete,
//     insert, and a privilege_auditlog row whose privilege_ids is the list
//     as Ballerina printed it ("[1,2,3]").
func (r *roleRepo) ReplacePrivileges(ctx context.Context, roleID int, privilegeIDs []int, updatedBy string) error {
	err := WithTx(ctx, r.db, func(tx *sql.Tx) error {
		var before []int
		if audit.Enabled() {
			var err error
			if before, err = rolePrivilegeIDs(ctx, tx, roleID); err != nil {
				return fmt.Errorf("%s rolePrivilege.Before: %w", r.app, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM role_privilege WHERE role_id = ?`, roleID); err != nil {
			return fmt.Errorf("%s rolePrivilege.Delete: %w", r.app, err)
		}
		if len(privilegeIDs) > 0 {
			values := strings.TrimSuffix(strings.Repeat("(?, ?), ", len(privilegeIDs)), ", ")
			args := make([]any, 0, 2*len(privilegeIDs))
			for _, p := range privilegeIDs {
				args = append(args, roleID, p)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO role_privilege (role_id, privilege_id) VALUES `+values, args...); err != nil {
				return fmt.Errorf("%s rolePrivilege.Insert: %w", r.app, err)
			}
		}
		if r.app == domain.AppTravel {
			if _, err := tx.ExecContext(ctx, `INSERT INTO privilege_auditlog (role_id, privilege_ids, updated_by, date)
				VALUES (?, ?, ?, NOW())`, roleID, roleIntList(privilegeIDs), updatedBy); err != nil {
				return fmt.Errorf("travel privilegeAuditlog.Insert: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Event{
			Action: audit.Update, EntityType: "role", EntityID: strconv.Itoa(roleID), Actor: updatedBy,
			Changes:  map[string]audit.Change{"privilege_ids": {Before: before, After: privilegeIDs}},
			Metadata: map[string]any{"app": r.app},
		})
	})
	if err != nil || r.app != domain.AppAllocations {
		return err
	}
	desc := fmt.Sprintf("The role with %d as the ID is updated by %s with the following privileges: %s",
		roleID, updatedBy, roleStringList(privilegeIDs))
	if len(privilegeIDs) == 0 {
		desc = fmt.Sprintf("The role with %d as the ID is updated by %s to have no privileges", roleID, updatedBy)
	}
	SettingsAuditLog(ctx, r.db, "ROLE", "UPDATE", desc, updatedBy)
	return nil
}

func rolePrivilegeIDs(ctx context.Context, tx *sql.Tx, roleID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT privilege_id FROM role_privilege WHERE role_id = ? ORDER BY privilege_id`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// roleIntList formats ids as Ballerina's int[].toString(): [1,2,3].
func roleIntList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// roleStringList formats ids as Ballerina's string[].toString() of the
// legacy string ids: ["1","2"].
func roleStringList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = `"` + strconv.Itoa(id) + `"`
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// SettingsAuditLog ports the Allocations BFF addAuditLog: one audit_log
// row; a failure is only logged, never returned.
func SettingsAuditLog(ctx context.Context, db *sql.DB, resource, action, description, user string) {
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_log (resource, action, user, description, timestamp)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP())`, resource, action, user, description); err != nil {
		log.Printf("Error while inserting audit log for %s %s by %s (%s): %v", resource, action, user, description, err) // #nosec G706
	}
}
