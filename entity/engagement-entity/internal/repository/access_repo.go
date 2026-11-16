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

// Package repository holds the SQL against the legacy schemas. It carries no
// business rules: validation and defaults live in the service layer.
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// AccessRepository reads one legacy app's roles, privileges and
// role_privilege tables. Both schemas use the same table and column names.
type AccessRepository interface {
	ListRoles(ctx context.Context) ([]domain.Role, error)
	// ListPrivileges returns the active privileges, as the legacy Role
	// Settings screens list them.
	ListPrivileges(ctx context.Context) ([]domain.Privilege, error)
	// RolePrivilegeMap returns every role name with the privilege ids it
	// grants, the mapping the legacy backends resolve a caller's groups
	// against.
	RolePrivilegeMap(ctx context.Context) (map[string][]int, error)
}

type accessRepo struct {
	db  *sql.DB
	app domain.App
}

// NewAccessRepository constructs an AccessRepository over app's schema. db
// must be that app's pool.
func NewAccessRepository(db *sql.DB, app domain.App) AccessRepository {
	return &accessRepo{db: db, app: app}
}

func (r *accessRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name FROM roles ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("%s roles.List: %w", r.app, err)
	}
	defer rows.Close()

	var roles []domain.Role
	for rows.Next() {
		var role domain.Role
		if err := rows.Scan(&role.ID, &role.Name); err != nil {
			return nil, fmt.Errorf("%s roles.List scan: %w", r.app, err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *accessRepo) ListPrivileges(ctx context.Context) ([]domain.Privilege, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description FROM privileges WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("%s privileges.List: %w", r.app, err)
	}
	defer rows.Close()

	var privileges []domain.Privilege
	for rows.Next() {
		var p domain.Privilege
		if err := rows.Scan(&p.ID, &p.Name, &p.Description); err != nil {
			return nil, fmt.Errorf("%s privileges.List scan: %w", r.app, err)
		}
		privileges = append(privileges, p)
	}
	return privileges, rows.Err()
}

// rolePrivilegeQuery reproduces each legacy backend's "privileges for these
// groups" query without the group filter, so the caller can resolve groups
// locally and get exactly the legacy answer:
//
//   - Allocations (apps/allocation/backend getUserPrivilegesQuery) joins
//     role_privilege to roles only.
//   - Travel (apps/travel/backend getUserPrivilegesQuery) also inner-joins
//     privileges, so a role_privilege row pointing at a deleted privilege
//     grants nothing.
//
// Neither filters on privileges.status, so neither does this.
func (r *accessRepo) rolePrivilegeQuery() string {
	if r.app == domain.AppTravel {
		return `SELECT r.name, rp.privilege_id
			FROM role_privilege rp
			JOIN roles r ON r.id = rp.role_id
			JOIN privileges p ON p.id = rp.privilege_id
			ORDER BY r.name, rp.privilege_id`
	}
	return `SELECT r.name, rp.privilege_id
		FROM role_privilege rp
		JOIN roles r ON r.id = rp.role_id
		ORDER BY r.name, rp.privilege_id`
}

func (r *accessRepo) RolePrivilegeMap(ctx context.Context) (map[string][]int, error) {
	rows, err := r.db.QueryContext(ctx, r.rolePrivilegeQuery())
	if err != nil {
		return nil, fmt.Errorf("%s rolePrivilege.Map: %w", r.app, err)
	}
	defer rows.Close()

	out := make(map[string][]int)
	for rows.Next() {
		var (
			role string
			id   int
		)
		if err := rows.Scan(&role, &id); err != nil {
			return nil, fmt.Errorf("%s rolePrivilege.Map scan: %w", r.app, err)
		}
		// role_privilege has no unique key in the dumps, so a pair can repeat;
		// rows arrive ordered, so a repeat is always the previous entry.
		if ids := out[role]; len(ids) > 0 && ids[len(ids)-1] == id {
			continue
		}
		out[role] = append(out[role], id)
	}
	return out, rows.Err()
}
