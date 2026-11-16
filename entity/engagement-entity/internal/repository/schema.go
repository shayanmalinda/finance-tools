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
	"sync/atomic"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
)

// engagementCampaignColumn: whether engagement.campaign_id exists (set at
// startup); without it campaigns read as NULL.
var engagementCampaignColumn atomic.Bool

func init() { engagementCampaignColumn.Store(true) }

// SetEngagementCampaignColumn records whether the column exists.
func SetEngagementCampaignColumn(present bool) { engagementCampaignColumn.Store(present) }

// HasEngagementCampaignColumn reports whether the column exists.
func HasEngagementCampaignColumn() bool { return engagementCampaignColumn.Load() }

// campaignCol is "<alias>.campaign_id", or NULL without the column.
func campaignCol(alias string) string {
	if HasEngagementCampaignColumn() {
		return alias + ".campaign_id"
	}
	return "NULL"
}

// DetectEngagementCampaignColumn reads information_schema for the column.
func DetectEngagementCampaignColumn(ctx context.Context, db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'engagement' AND column_name = 'campaign_id'`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("detect engagement.campaign_id: %w", err)
	}
	return n > 0, nil
}

// errEngagementNoCampaignColumn refuses to save a campaign before the
// migration has added the column.
var errEngagementNoCampaignColumn = &apierror.ServiceUnavailableError{
	Msg: "engagement.campaign_id does not exist yet: run Resources/allocation_db_table_update_v1.0.0.sql",
}
