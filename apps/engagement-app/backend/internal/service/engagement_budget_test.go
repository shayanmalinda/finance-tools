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
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func engagementBudgetService(entity *engagementFakeEntity, hooks BudgetHooks) EngagementService {
	refs := NewReferenceService(entity, engagementFakeSales{}, engagementFakeHR{})
	return NewEngagementService(entity, refs, engagementFakeSales{}, engagementFakeHR{}, &engagementFakeMail{},
		config.EmailConfig{AppBaseURL: "https://one.wso2.com"}, func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) },
		WithEngagementBudgets(hooks))
}

func TestEngagementCreate_BudgetLineCode(t *testing.T) {
	bf := newBudgetFake()
	bs := newBudgetSvc(bf, nil, &allocFakeSender{}, 0)
	entity := engagementTestEntity()
	svc := engagementBudgetService(entity, bs)
	admin := engagementPerms("a@wso2.com", engagementAdmin, nil)
	in := domain.EngagementCreate{EngagementTypeID: 1, AllocationTypeID: 10, Title: "Board", Nature: "On-site", Country: "Sri Lanka",
		Address: "Colombo", BusinessUnit: "Digital Operations", ProductUnit: "Digital", SalesSubRegion: "LK", StartDate: "2026-12-01", EndDate: "2026-12-01",
		BudgetLineCode: sp("GA_BOARD")}
	d, err := svc.Create(context.Background(), admin, in)
	if err != nil {
		t.Fatal(err)
	}
	line, _ := d.BudgetLine.(*domain.EngagementBudgetLine)
	if bf.engLine["E09999"] != "GA_BOARD" || line == nil || line.Code != "GA_BOARD" || line.Name != "G&A: Board meeting travel" {
		t.Errorf("line %q detail %+v", bf.engLine["E09999"], d.BudgetLine)
	}
	// An unknown code is refused before anything is written.
	in.BudgetLineCode = sp("NOPE")
	if _, err := svc.Create(context.Background(), admin, in); !isAllocValidation(err, "Choose a valid travel budget line.") {
		t.Errorf("unknown code: %v", err)
	}
	// Customer engagements ignore the field and show null.
	cust := domain.EngagementCreate{EngagementTypeID: 6, AccountID: "001A", OpportunityID: "006A", OpportunityLineItemID: "00kA",
		AllocationTypeID: 60, Nature: "On-site", Country: "United States of America", Address: "1 Main St", BusinessUnit: "Marketing",
		ProductUnit: "Brand", SalesSubRegion: "LK", CustomerContact: "Dana", StartDate: "2026-01-01", EndDate: "2026-01-02", BudgetLineCode: sp("GA_BOARD")}
	delete(bf.engLine, "E09999")
	d, err = svc.Create(context.Background(), engagementPerms("consultant@wso2.com", engagementCustMgr, nil), cust)
	if err != nil || len(bf.engLine) != 0 || d.BudgetLine == nil {
		t.Errorf("customer: %v %v %+v", err, bf.engLine, d.BudgetLine)
	}
	// Off: the field is ignored and the page has no budgetLine.
	off := engagementBudgetService(engagementTestEntity(), nil)
	in.BudgetLineCode = sp("GA_BOARD")
	if d, err = off.Create(context.Background(), admin, in); err != nil || d.BudgetLine != nil || len(bf.engLine) != 0 {
		t.Errorf("off: %v %+v", err, d.BudgetLine)
	}
}
