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

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// travelInChunk bounds the bind parameters of one IN list.
const travelInChunk = 500

// travelEngagementMatchLimit bounds the engagement ids a title search adds
// to a travel search.
const travelEngagementMatchLimit = 1000

// TravelEngagementRepository reads, from allocation_db, the engagement data
// the travel request screens show. It never touches travelapp: the service
// combines the two in Go.
type TravelEngagementRepository interface {
	// ByAllocation returns the engagement of each allocation id found.
	ByAllocation(ctx context.Context, allocationIDs []string) (map[string]domain.TravelEngagement, error)
	// IDsByTitle returns the ids of engagements whose title contains search.
	IDsByTitle(ctx context.Context, search string) ([]string, error)
	// FundingSources returns the funding sources of an engagement.
	FundingSources(ctx context.Context, engagementID string) ([]domain.TravelFundingSource, error)
}

type travelEngagementRepo struct{ db *sql.DB }

// NewTravelEngagementRepository constructs a TravelEngagementRepository. db
// must be the allocation_db pool.
func NewTravelEngagementRepository(db *sql.DB) TravelEngagementRepository {
	return &travelEngagementRepo{db: db}
}

// ByAllocation ports the engagement overlay of the legacy getTravelDetails
// (getEngagementAllocations({allocationIds})): allocation → engagement →
// allocation type → engagement type, engagements not Deleted. campaign_id is
// the Release 1 Marketing Event campaign.
func (r *travelEngagementRepo) ByAllocation(ctx context.Context, allocationIDs []string) (map[string]domain.TravelEngagement, error) {
	out := map[string]domain.TravelEngagement{}
	for start := 0; start < len(allocationIDs); start += travelInChunk {
		end := min(start+travelInChunk, len(allocationIDs))
		ids := allocationIDs[start:end]
		rows, err := r.db.QueryContext(ctx, `SELECT a.id, e.id, e.project_name, e.customer_name, e.engagement_code,
				`+campaignCol("e")+`, eT.id, eT.name, eT.type
			FROM allocation AS a
			JOIN engagement AS e ON a.engagement_id = e.id
			JOIN allocation_type AS aT ON e.allocation_type = aT.id
			JOIN engagement_type AS eT ON aT.engagement_type = eT.id
			WHERE e.status != 'Deleted' AND a.id IN (`+Placeholders(len(ids))+`)`, Args(ids)...)
		if err != nil {
			return nil, fmt.Errorf("travelEngagement.ByAllocation: %w", err)
		}
		for rows.Next() {
			var (
				allocationID string
				e            domain.TravelEngagement
			)
			if err := rows.Scan(&allocationID, &e.ID, &e.Title, &e.CustomerName, &e.EngagementCode,
				&e.CampaignID, &e.TypeID, &e.TypeName, &e.Kind); err != nil {
				rows.Close()
				return nil, fmt.Errorf("travelEngagement.ByAllocation scan: %w", err)
			}
			out[allocationID] = e
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("travelEngagement.ByAllocation rows: %w", err)
		}
	}
	return out, nil
}

func (r *travelEngagementRepo) IDsByTitle(ctx context.Context, search string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM engagement
		WHERE status != 'Deleted' AND project_name LIKE ?
		ORDER BY id LIMIT ?`, LikeContains(search), travelEngagementMatchLimit)
	if err != nil {
		return nil, fmt.Errorf("travelEngagement.IDsByTitle: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("travelEngagement.IDsByTitle scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// FundingSources ports the legacy getFundingSources for one engagement
// (the Travel app's GET allocations/{id}/funding-sources).
func (r *travelEngagementRepo) FundingSources(ctx context.Context, engagementID string) ([]domain.TravelFundingSource, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT business_unit.name, product_unit.name, region.name,
			sub_region.name, eng_fs.percentage
		FROM engagement_funding_source AS eng_fs
			LEFT JOIN business_unit ON eng_fs.business_unit_id = business_unit.id
			LEFT JOIN product_unit ON eng_fs.product_unit_id = product_unit.id
			LEFT JOIN region ON eng_fs.region_id = region.id
			LEFT JOIN sub_region ON eng_fs.sub_region_id = sub_region.id
		WHERE eng_fs.engagement_id = ?`, engagementID)
	if err != nil {
		return nil, fmt.Errorf("travelEngagement.FundingSources: %w", err)
	}
	defer rows.Close()
	out := []domain.TravelFundingSource{}
	for rows.Next() {
		var (
			f   domain.TravelFundingSource
			pct sql.NullFloat64
		)
		if err := rows.Scan(&f.BusinessUnit, &f.ProductUnit, &f.Region, &f.SubRegion, &pct); err != nil {
			return nil, fmt.Errorf("travelEngagement.FundingSources scan: %w", err)
		}
		if pct.Valid {
			v := pct.Float64
			f.Percentage = &v
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
