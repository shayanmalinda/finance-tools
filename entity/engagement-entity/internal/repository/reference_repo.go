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
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// ReferenceRepository reads the lookup lists of allocation_db (business and
// product units, regions, sub regions) and travelapp (travel agencies,
// invoice cost types, per diem rates).
type ReferenceRepository interface {
	BusinessUnits(ctx context.Context) ([]domain.ReferenceItem, error)
	ProductUnits(ctx context.Context) ([]domain.ReferenceProductUnit, error)
	Regions(ctx context.Context) ([]domain.ReferenceItem, error)
	SubRegions(ctx context.Context) ([]domain.ReferenceSubRegion, error)
	TravelAgencies(ctx context.Context) ([]domain.ReferenceTravelAgency, error)
	CostTypes(ctx context.Context) ([]domain.ReferenceCostType, error)
	PerDiemRates(ctx context.Context) ([]domain.ReferencePerDiemRate, error)
}

type referenceRepo struct {
	alloc  *sql.DB
	travel *sql.DB
}

// NewReferenceRepository constructs a ReferenceRepository over the
// allocation_db and travelapp pools.
func NewReferenceRepository(alloc, travel *sql.DB) ReferenceRepository {
	return &referenceRepo{alloc: alloc, travel: travel}
}

// BusinessUnits ports getBusinessUnitsListQuery.
func (r *referenceRepo) BusinessUnits(ctx context.Context) ([]domain.ReferenceItem, error) {
	return referenceItems(ctx, r.alloc, `SELECT id, name, is_active FROM business_unit ORDER BY id`, "businessUnit")
}

// Regions ports getRegionsListQuery.
func (r *referenceRepo) Regions(ctx context.Context) ([]domain.ReferenceItem, error) {
	return referenceItems(ctx, r.alloc, `SELECT id, name, is_active FROM region ORDER BY id`, "region")
}

func referenceItems(ctx context.Context, db *sql.DB, q, what string) ([]domain.ReferenceItem, error) {
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("%s.List: %w", what, err)
	}
	defer rows.Close()
	out := []domain.ReferenceItem{}
	for rows.Next() {
		var it domain.ReferenceItem
		var active sql.NullBool
		if err := rows.Scan(&it.ID, &it.Name, &active); err != nil {
			return nil, fmt.Errorf("%s.List scan: %w", what, err)
		}
		it.Active = active.Bool
		out = append(out, it)
	}
	return out, rows.Err()
}

// ProductUnits ports getProductUnitsListQuery, with the parent business
// unit's name instead of its id.
func (r *referenceRepo) ProductUnits(ctx context.Context) ([]domain.ReferenceProductUnit, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT pu.id, pu.name, pu.is_active, bu.name, pu.is_customer, pu.is_internal, pu.is_marketing
		FROM product_unit AS pu LEFT JOIN business_unit AS bu ON pu.business_unit_id = bu.id
		ORDER BY pu.id`)
	if err != nil {
		return nil, fmt.Errorf("productUnit.List: %w", err)
	}
	defer rows.Close()
	out := []domain.ReferenceProductUnit{}
	for rows.Next() {
		var (
			p                                  domain.ReferenceProductUnit
			active, customer, internal, market sql.NullBool
			bu                                 sql.NullString
		)
		if err := rows.Scan(&p.ID, &p.Name, &active, &bu, &customer, &internal, &market); err != nil {
			return nil, fmt.Errorf("productUnit.List scan: %w", err)
		}
		p.Active, p.Customer, p.Internal, p.Marketing = active.Bool, customer.Bool, internal.Bool, market.Bool
		p.BusinessUnit = engagementStr(bu)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SubRegions ports getSubRegionsListQuery, with the region's name.
func (r *referenceRepo) SubRegions(ctx context.Context) ([]domain.ReferenceSubRegion, error) {
	rows, err := r.alloc.QueryContext(ctx, `SELECT sr.id, sr.name, sr.is_active, rg.name
		FROM sub_region AS sr LEFT JOIN region AS rg ON sr.region_id = rg.id
		ORDER BY sr.id`)
	if err != nil {
		return nil, fmt.Errorf("subRegion.List: %w", err)
	}
	defer rows.Close()
	out := []domain.ReferenceSubRegion{}
	for rows.Next() {
		var (
			s      domain.ReferenceSubRegion
			active sql.NullBool
			region sql.NullString
		)
		if err := rows.Scan(&s.ID, &s.Name, &active, &region); err != nil {
			return nil, fmt.Errorf("subRegion.List scan: %w", err)
		}
		s.Active, s.Region = active.Bool, engagementStr(region)
		out = append(out, s)
	}
	return out, rows.Err()
}

// TravelAgencies ports getTravelAgentsQuery with status "all" (every agency,
// by name), adding the NetSuite vendor id.
func (r *referenceRepo) TravelAgencies(ctx context.Context) ([]domain.ReferenceTravelAgency, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT id, name_of_travel_agent, status, vendor_id
		FROM travel_agents ORDER BY name_of_travel_agent`)
	if err != nil {
		return nil, fmt.Errorf("travelAgent.List: %w", err)
	}
	defer rows.Close()
	out := []domain.ReferenceTravelAgency{}
	for rows.Next() {
		var (
			a      domain.ReferenceTravelAgency
			status sql.NullString
			vendor sql.NullInt64
		)
		if err := rows.Scan(&a.ID, &a.Name, &status, &vendor); err != nil {
			return nil, fmt.Errorf("travelAgent.List scan: %w", err)
		}
		// The legacy "active" filter is status = 'Active' under a
		// case-insensitive collation.
		a.Active = strings.EqualFold(status.String, "Active")
		if vendor.Valid {
			v := strconv.FormatInt(vendor.Int64, 10)
			a.NetsuiteVendorID = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CostTypes ports getInvoiceCostTypesQuery.
func (r *referenceRepo) CostTypes(ctx context.Context) ([]domain.ReferenceCostType, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT id, name FROM inv_cost_type WHERE status = 'active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("invCostType.List: %w", err)
	}
	defer rows.Close()
	out := []domain.ReferenceCostType{}
	for rows.Next() {
		var c domain.ReferenceCostType
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, fmt.Errorf("invCostType.List scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PerDiemRates ports getPerDiemMetaDataQuery('Active'). amount is a VARCHAR
// column; a row whose amount is not a number is logged and left out.
func (r *referenceRepo) PerDiemRates(ctx context.Context) ([]domain.ReferencePerDiemRate, error) {
	rows, err := r.travel.QueryContext(ctx, `SELECT id, country, currency, amount FROM perdiem_metadata
		WHERE status = 'Active' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("perDiemMetadata.List: %w", err)
	}
	defer rows.Close()
	out := []domain.ReferencePerDiemRate{}
	for rows.Next() {
		var (
			id                      int
			country, currency, amnt sql.NullString
		)
		if err := rows.Scan(&id, &country, &currency, &amnt); err != nil {
			return nil, fmt.Errorf("perDiemMetadata.List scan: %w", err)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(amnt.String), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			log.Printf("perdiem_metadata id %d: amount %q is not a number; left out", id, amnt.String) // #nosec G706
			continue
		}
		out = append(out, domain.ReferencePerDiemRate{Country: country.String, Currency: currency.String, Amount: v})
	}
	return out, rows.Err()
}
