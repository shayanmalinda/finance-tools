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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// AgencyRepository reads which travel agency a person works for.
type AgencyRepository interface {
	// AgencyOf returns the active agency of email, or nil when the person is
	// not an active travel agent employee.
	AgencyOf(ctx context.Context, email string) (*domain.TravelAgency, error)
}

type agencyRepo struct{ db *sql.DB }

// NewAgencyRepository constructs an AgencyRepository. db must be the
// travelapp pool.
func NewAgencyRepository(db *sql.DB) AgencyRepository { return &agencyRepo{db: db} }

// AgencyOf ports the legacy getTravelAgentIdByEmailQuery
// (wso2_email = ? AND status = 'Active', first row) joined to the agency's
// name. The legacy query takes the first row; wso2_email is the primary key,
// so there is at most one.
func (r *agencyRepo) AgencyOf(ctx context.Context, email string) (*domain.TravelAgency, error) {
	var a domain.TravelAgency
	err := r.db.QueryRowContext(ctx, `SELECT ta.id, ta.name_of_travel_agent
		FROM travel_agent_employee tae
		JOIN travel_agents ta ON ta.id = tae.travel_agent_id
		WHERE tae.wso2_email = ? AND tae.status = 'Active'
		LIMIT 1`, email).Scan(&a.ID, &a.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("travelAgentEmployee.AgencyOf: %w", err)
	}
	return &a, nil
}
