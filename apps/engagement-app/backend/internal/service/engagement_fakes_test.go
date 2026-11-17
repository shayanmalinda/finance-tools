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
	"sync"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// engagementFakeEntity is an in-memory Engagement Entity for the area's
// service tests.
type engagementFakeEntity struct {
	EngagementEntityAPI
	mu          sync.Mutex
	engagements map[string]engagemententity.EngagementRecord
	roster      map[string][]engagemententity.EngagementRosterAllocation
	search      engagemententity.EngagementSearchQuery
	created     engagemententity.EngagementCreateInput
	updates     []engagemententity.EngagementUpdateInput
	cancelled   []engagemententity.EngagementRosterAllocation
	rosterErr   error
}

func engagementP(s string) *string { return &s }

var engagementTrue = true

func engagementTestRef() engagemententity.ReferenceLists {
	return engagemententity.ReferenceLists{
		EngagementTypes: []engagemententity.EngagementType{
			{ID: 1, Name: "Internal Engagement", Type: "Internal", IsActive: true},
			{ID: 2, Name: "Paid", Type: "Customer", IsActive: false},
			{ID: 3, Name: "Non-Paid Pre Sale", Type: "Customer", IsActive: true},
			{ID: 4, Name: "Non-Paid Post Sale", Type: "Customer", IsActive: true},
			{ID: 5, Name: "Marketing Event", Type: "Internal", IsActive: true},
			{ID: 6, Name: "Paid Post Sale", Type: "Customer", IsActive: true},
			{ID: 7, Name: "Paid Pre Sale", Type: "Customer", IsActive: true},
		},
		AllocationTypes: []engagemententity.AllocationType{
			{ID: 10, AllocationTypeName: "Digital Operations", Status: "ACTIVE", EngagementTypeID: 1,
				DefaultEngagementCode: engagementP("DIG-GEN"), EmailToAllocations: engagementP("No")},
			{ID: 12, AllocationTypeName: "Conference", Status: "Active", EngagementTypeID: 5,
				DefaultEngagementCode: engagementP("MKT"), EngagementCodeSuffix: engagementP("CONF")},
			{ID: 30, AllocationTypeName: "PoC", Status: "Active", EngagementTypeID: 3},
			{ID: 40, AllocationTypeName: "Health Check", Status: "Active", EngagementTypeID: 4, EngagementCodeSuffix: engagementP("HC")},
			{ID: 41, AllocationTypeName: "No Suffix", Status: "Active", EngagementTypeID: 4},
			{ID: 60, AllocationTypeName: "Consulting", Status: "Active", EngagementTypeID: 6, EmailToAllocations: engagementP("Yes")},
			{ID: 99, AllocationTypeName: "Old", Status: "Deleted", EngagementTypeID: 1},
		},
		BusinessUnits: []engagemententity.ReferenceItem{{ID: 1, Name: "Digital Operations", Active: true}, {ID: 2, Name: "Marketing", Active: true}},
		ProductUnits:  []engagemententity.ReferenceProductUnit{{ID: 1, Name: "Digital", Active: true}, {ID: 2, Name: "Brand", Active: true}},
		Regions:       []engagemententity.ReferenceItem{{ID: 1, Name: "APAC", Active: true}, {ID: 2, Name: "NA", Active: true}},
		SubRegions: []engagemententity.ReferenceSubRegion{
			{ID: 1, Name: "LK", Active: true, Region: engagementP("APAC")}, {ID: 2, Name: "NA - EAST", Active: true, Region: engagementP("NA")},
		},
		CostTypes: []engagemententity.ReferenceCostType{{ID: 1, Name: "Air Ticket"}},
	}
}

func (f *engagementFakeEntity) ReferenceData(context.Context) (engagemententity.ReferenceLists, error) {
	return engagementTestRef(), nil
}

func (f *engagementFakeEntity) EngagementSearch(_ context.Context, q engagemententity.EngagementSearchQuery) (engagemententity.EngagementSearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.search = q
	var items []engagemententity.EngagementRecord
	for _, e := range f.engagements {
		for _, t := range q.EngagementTypeIDs {
			if e.EngagementTypeID == t && (engagementVal(e.RequestStatus) == "Pending Approval") == q.PendingApproval {
				items = append(items, e)
			}
		}
	}
	return engagemententity.EngagementSearchResult{Items: items, Total: len(items), TypeCounts: map[int]int{1: 2, 3: 4, 5: 1, 6: 3}}, nil
}

func (f *engagementFakeEntity) EngagementFacets(context.Context, []int) (engagemententity.EngagementFacetLists, error) {
	return engagemententity.EngagementFacetLists{Accounts: []string{"Acme"}, AllocationTypes: []string{}, Countries: []string{}}, nil
}

func (f *engagementFakeEntity) EngagementGet(_ context.Context, id string) (engagemententity.EngagementRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.engagements[id]
	if !ok {
		return e, &apierror.UpstreamError{Service: "engagement-entity", StatusCode: 404, Msg: "Engagement " + id + " not found."}
	}
	return e, nil
}

func (f *engagementFakeEntity) EngagementRoster(_ context.Context, id string, _ bool) ([]engagemententity.EngagementRosterAllocation, error) {
	if f.rosterErr != nil {
		return nil, f.rosterErr
	}
	return f.roster[id], nil
}

func (f *engagementFakeEntity) EngagementCreate(_ context.Context, in engagemententity.EngagementCreateInput) (engagemententity.EngagementRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = in
	typeID := map[int]int{10: 1, 12: 5, 30: 3, 40: 4, 41: 4, 60: 6}[in.AllocationTypeID]
	return engagemententity.EngagementRecord{ID: "E09999", EngagementTypeID: typeID, AllocationTypeID: in.AllocationTypeID,
		ProjectName: &in.ProjectName, CustomerName: &in.CustomerName, RequestStatus: &in.RequestStatus,
		CreatedBy: &in.CreatedBy, EngagementNature: &in.EngagementNature, CampaignID: in.CampaignID}, nil
}

func (f *engagementFakeEntity) EngagementUpdate(_ context.Context, id string, in engagemententity.EngagementUpdateInput) (engagemententity.EngagementUpdateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, in)
	e := f.engagements[id]
	if in.RequestStatus != nil {
		e.RequestStatus = in.RequestStatus
	}
	if in.EngagementStatus != nil {
		e.EngagementStatus = in.EngagementStatus
	}
	f.engagements[id] = e
	res := engagemententity.EngagementUpdateResult{Engagement: e, CancelledAllocations: []engagemententity.EngagementRosterAllocation{}}
	if in.EngagementStatus != nil && *in.EngagementStatus == "Cancelled" {
		res.CancelledAllocations = f.roster[id]
	}
	return res, nil
}

func (f *engagementFakeEntity) lastUpdate() engagemententity.EngagementUpdateInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) == 0 {
		return engagemententity.EngagementUpdateInput{}
	}
	return f.updates[len(f.updates)-1]
}

// engagementFakeSales is a Sales Entity with one account, opportunity and
// line item, and one campaign.
type engagementFakeSales struct{ salesentity.Unconfigured }

func (engagementFakeSales) SearchCustomers(_ context.Context, f salesentity.CustomerFilter) ([]salesentity.Customer, error) {
	// Only the live path filters by name (legacy isRealTime: true).
	if !f.IsRealTime {
		return nil, nil
	}
	return []salesentity.Customer{{ID: "001A", Name: engagementP("Acme"), SubRegion: engagementP("NA - EAST"),
		Address: &salesentity.CustomerAddress{BillingStreet: engagementP("1 Main St"), BillingCountry: engagementP("United States of America")}}}, nil
}

func (engagementFakeSales) Opportunities(context.Context, string, bool) ([]salesentity.SalesOpportunity, error) {
	return []salesentity.SalesOpportunity{{ID: "006A", Name: "Acme - APIM PoC", Owner: &salesentity.SalesUser{Name: engagementP("Kevin")},
		PreSalesCode: engagementP("ACME-PR (Open Banking)"), PostSalesEngagementCode: engagementP("ACME-PS-01"), IsWon: new(bool)}}, nil
}

func (engagementFakeSales) OpportunityLineItems(context.Context, string) ([]salesentity.SalesLineItem, error) {
	return []salesentity.SalesLineItem{{ID: engagementP("00kA"), Name: engagementP("APIM Consulting"), EngagementCodeAuto: engagementP("ACME-APIM-CO"),
		ServiceStartDate: engagementP("2026-12-01"), ServiceEndDate: engagementP("2026-12-20T00:00:00Z"),
		BusinessUnit: engagementP("Digital Operations"), ProductUnit: engagementP("Digital"), IsTentativeDates: &engagementTrue}}, nil
}

func (engagementFakeSales) OpportunityLineItem(context.Context, string) (*salesentity.SalesLineItem, error) {
	return &salesentity.SalesLineItem{ServiceStartDate: engagementP("2027-01-05"), ServiceEndDate: engagementP("2027-01-30")}, nil
}

func (engagementFakeSales) SearchCampaigns(_ context.Context, f salesentity.CampaignFilter) ([]salesentity.Campaign, error) {
	return []salesentity.Campaign{{ID: "7015g00000ApD26AAF", Name: "WSO2Con Asia 2027", IsActive: true}}, nil
}

type engagementFakeHR struct{ hrentity.Unconfigured }

func (engagementFakeHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	return &hrentity.Employee{WorkEmail: e, FirstName: "Jane", LastName: "Doe"}, nil
}

func (engagementFakeHR) ReportsTo(context.Context, string) ([]string, error) {
	return []string{"lead@wso2.com"}, nil
}

type engagementFakeMail struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *engagementFakeMail) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func engagementPerms(email string, alloc []int, travel []int) domain.Perms {
	return domain.Perms{Email: email, Alloc: domain.NewPrivilegeSet(alloc), Travel: domain.NewPrivilegeSet(travel)}
}
