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

package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/cache"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

const (
	campaignNameTTL   = 10 * time.Minute
	campaignNameBatch = 100
)

// CampaignNames resolves Salesforce campaign names by id (a Marketing
// Event's campaign_id), in batches and cached briefly. "" is cached for an
// id Salesforce does not know.
type CampaignNames struct {
	sales salesentity.Client
	names *cache.Cache[string, string]
}

// NewCampaignNames returns a resolver over the Sales Entity.
func NewCampaignNames(sales salesentity.Client) *CampaignNames {
	return &CampaignNames{sales: sales, names: cache.New[string, string](campaignNameTTL)}
}

// Fill sets each ref's Name from Salesforce (nil when unknown or the Sales
// Entity is unreachable). A nil resolver leaves the refs as they are.
func (n *CampaignNames) Fill(ctx context.Context, refs ...*domain.CampaignRef) {
	if n == nil || n.sales == nil {
		return
	}
	var missing []string
	seen := map[string]bool{}
	for _, r := range refs {
		if r == nil || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if _, ok := n.names.Get(r.ID); !ok {
			missing = append(missing, r.ID)
		}
	}
	for start := 0; start < len(missing); start += campaignNameBatch {
		ids := missing[start:min(start+campaignNameBatch, len(missing))]
		found, err := n.sales.SearchCampaigns(ctx, salesentity.CampaignFilter{IDs: ids, Limit: len(ids)})
		if err != nil {
			slog.WarnContext(ctx, "campaign names not resolved", "err", err)
			break
		}
		got := map[string]string{}
		for _, c := range found {
			got[c.ID] = c.Name
		}
		for _, id := range ids {
			n.names.Set(id, got[id])
		}
	}
	for _, r := range refs {
		if r == nil {
			continue
		}
		r.Name = nil
		if name, _ := n.names.Get(r.ID); name != "" {
			r.Name = &name
		}
	}
}
