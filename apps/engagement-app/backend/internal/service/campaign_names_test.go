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
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

type campaignNamesSales struct {
	salesentity.Unconfigured
	calls [][]string
}

func (f *campaignNamesSales) SearchCampaigns(_ context.Context, q salesentity.CampaignFilter) ([]salesentity.Campaign, error) {
	f.calls = append(f.calls, q.IDs)
	return []salesentity.Campaign{{ID: "701A", Name: "WSO2Con"}}, nil
}

// Names are looked up once per id (batched, cached); an unknown id has none.
func TestCampaignNamesFill(t *testing.T) {
	sales := &campaignNamesSales{}
	n := NewCampaignNames(sales)
	a, b, a2 := &domain.CampaignRef{ID: "701A"}, &domain.CampaignRef{ID: "701B"}, &domain.CampaignRef{ID: "701A"}
	n.Fill(context.Background(), a, b, nil, a2)
	if a.Name == nil || *a.Name != "WSO2Con" || *a2.Name != "WSO2Con" || b.Name != nil || len(sales.calls) != 1 || len(sales.calls[0]) != 2 {
		t.Fatalf("a %v b %v calls %v", a.Name, b.Name, sales.calls)
	}
	n.Fill(context.Background(), &domain.CampaignRef{ID: "701B"})
	if len(sales.calls) != 1 {
		t.Errorf("cached ids looked up again: %v", sales.calls)
	}
	(*CampaignNames)(nil).Fill(context.Background(), a) // nil resolver: no-op
}
