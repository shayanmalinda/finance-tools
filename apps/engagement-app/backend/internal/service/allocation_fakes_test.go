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
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// allocFakeEntity is an in-memory Engagement Entity for the allocation area.
type allocFakeEntity struct {
	engagemententity.Client
	mu          sync.Mutex
	engagement  engagemententity.AllocationEngagement
	allocations map[string]engagemententity.AllocationRecord
	timeline    []engagemententity.AllocationRecord
	types       []engagemententity.AllocationType
	visas       []engagemententity.AllocationVisa
	visaErr     error
	search      engagemententity.AllocationSearchResult
	consultant  []engagemententity.ConsultantAllocation
	nonAlloc    []engagemententity.ConsultantNonAllocatedEmployee
	defaults    []engagemententity.ConsultantTeamDefault
	codes       []engagemententity.ConsultantEngagementCode

	bulk     []engagemententity.AllocationBulkRequest
	updates  []engagemententity.AllocationUpdateRequest
	deleted  []string
	searched []engagemententity.AllocationSearchRequest
	timeReqs []engagemententity.AllocationTimelineRequest
}

func allocNotFound() error {
	return &apierror.UpstreamError{Service: "engagement-entity", StatusCode: http.StatusNotFound, Msg: "not found"}
}

func (f *allocFakeEntity) AllocationEngagement(_ context.Context, id string) (engagemententity.AllocationEngagement, error) {
	if f.engagement.ID != id {
		return engagemententity.AllocationEngagement{}, allocNotFound()
	}
	return f.engagement, nil
}

func (f *allocFakeEntity) Allocation(_ context.Context, id string) (engagemententity.AllocationRecord, error) {
	a, ok := f.allocations[id]
	if !ok {
		return a, allocNotFound()
	}
	return a, nil
}

func (f *allocFakeEntity) AllocationTimeline(_ context.Context, req engagemententity.AllocationTimelineRequest) ([]engagemententity.AllocationRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeReqs = append(f.timeReqs, req)
	return f.timeline, nil
}

func (f *allocFakeEntity) SearchAllocations(_ context.Context, req engagemententity.AllocationSearchRequest) (engagemententity.AllocationSearchResult, error) {
	f.searched = append(f.searched, req)
	return f.search, nil
}

func (f *allocFakeEntity) BulkAllocations(_ context.Context, req engagemententity.AllocationBulkRequest) (engagemententity.AllocationBulkResult, error) {
	f.bulk = append(f.bulk, req)
	id := "A0000000099"
	res := engagemententity.AllocationBulkResult{RecordsAdded: len(req.NewAllocations), FirstAllocationID: &id}
	m := req.NewAllocations[0]
	if m.IsTravelRequired == "Yes" && m.ClearanceStatus != nil && (*m.ClearanceStatus == "Confirmed") && !allocIsPartner(m.Email) {
		job := "TRV-2026-11-0001"
		res.TravelJobNumber = &job
	}
	return res, nil
}

func (f *allocFakeEntity) UpdateAllocation(_ context.Context, id string, req engagemententity.AllocationUpdateRequest) (engagemententity.AllocationUpdateResult, error) {
	f.updates = append(f.updates, req)
	a := f.allocations[id]
	prev := a.ClearanceStatus
	if req.ClearanceStatus != nil {
		a.ClearanceStatus = req.ClearanceStatus
	}
	if req.StartDate != nil {
		a.StartDate = *req.StartDate
	}
	return engagemententity.AllocationUpdateResult{Allocation: a, PreviousClearanceStatus: prev}, nil
}

func (f *allocFakeEntity) DeleteAllocation(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *allocFakeEntity) AllocationVisas(context.Context, string, string, string) ([]engagemententity.AllocationVisa, error) {
	return f.visas, f.visaErr
}

func (f *allocFakeEntity) AllocationTypes(_ context.Context, flt engagemententity.AllocationTypeFilter) ([]engagemententity.AllocationType, error) {
	var out []engagemententity.AllocationType
	for _, t := range f.types {
		if flt.EngagementTypeID == 0 || t.EngagementTypeID == flt.EngagementTypeID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *allocFakeEntity) ConsultantAllocations(context.Context, engagemententity.ConsultantAllocationFilter) ([]engagemententity.ConsultantAllocation, error) {
	return f.consultant, nil
}

func (f *allocFakeEntity) ConsultantNonAllocated(context.Context, engagemententity.ConsultantAllocationFilter) ([]engagemententity.ConsultantNonAllocatedEmployee, error) {
	return f.nonAlloc, nil
}

func (f *allocFakeEntity) ConsultantTeamDefaults(context.Context) ([]engagemententity.ConsultantTeamDefault, error) {
	return f.defaults, nil
}

func (f *allocFakeEntity) ConsultantEngagementCodes(context.Context) ([]engagemententity.ConsultantEngagementCode, error) {
	return f.codes, nil
}

// allocFakeHR knows employees by email and an org listing.
type allocFakeHR struct {
	hrentity.Unconfigured
	employees map[string]*hrentity.Employee
	org       []hrentity.OrgEmployee
	records   map[string]*hrentity.EmployeeRecord
	orgFilter *hrentity.OrgEmployeeFilter
}

func (h *allocFakeHR) Employee(_ context.Context, e string) (*hrentity.Employee, error) {
	return h.employees[e], nil
}

func (h *allocFakeHR) SearchEmployees(context.Context, string, []string, int) ([]hrentity.Employee, error) {
	return nil, errors.New("unused")
}

func (h *allocFakeHR) EmployeesByOrg(_ context.Context, f hrentity.OrgEmployeeFilter) ([]hrentity.OrgEmployee, error) {
	h.orgFilter = &f
	return h.org, nil
}

func (h *allocFakeHR) EmployeeRecord(_ context.Context, e string) (*hrentity.EmployeeRecord, error) {
	return h.records[e], nil
}

type allocFakeSender struct{ sent []email.Message }

func (s *allocFakeSender) Send(_ context.Context, m email.Message) error {
	s.sent = append(s.sent, m)
	return nil
}

var allocNow = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

func allocTestDeps(ent *allocFakeEntity, hr *allocFakeHR, snd *allocFakeSender) AllocationDeps {
	return AllocationDeps{Entity: ent, HR: hr, Org: hr, Email: snd, Now: func() time.Time { return allocNow },
		Recipients: config.EmailConfig{AllocationsGroup: "allocations@wso2.com", PartnerAllocationsGroup: "partners@wso2.com", TravelGroup: "travel@wso2.com"}}
}

func allocPerms(ids ...int) domain.Perms {
	return domain.Perms{Email: "mgr@wso2.com", Alloc: domain.NewPrivilegeSet(ids), Travel: domain.NewPrivilegeSet(nil)}
}

func sp(s string) *string { return &s }
func ip(i int) *int       { return &i }
func bp(b bool) *bool     { return &b }

func allocTestEngagement(typeID int) engagemententity.AllocationEngagement {
	return engagemententity.AllocationEngagement{ID: "E07201", EngagementTypeID: typeID, AllocationTypeID: ip(12),
		AllocationTypeName: sp("Consulting"), CustomerName: sp("Acme"), Title: sp("Acme - PoC"), EngagementCode: sp("NWB-CO"),
		StartDate: sp("2026-11-01"), EndDate: sp("2026-11-30"), RequestStatus: sp("New"), EngagementStatus: sp("Pending"),
		Country: sp("Sri Lanka")}
}

func allocTestHR() *allocFakeHR {
	return &allocFakeHR{employees: map[string]*hrentity.Employee{"jane@wso2.com": {WorkEmail: "jane@wso2.com", FirstName: "Jane", LastName: "Doe"}},
		records: map[string]*hrentity.EmployeeRecord{"jane@wso2.com": {ReportsToEmail: sp("lead1@wso2.com,lead2@wso2.com")}}}
}

func isAllocValidation(err error, msg string) bool {
	var ve *apierror.ValidationError
	return errors.As(err, &ve) && (msg == "" || ve.Msg == msg)
}

func isAllocOverlap(err error, msg string) (*domain.AllocationOverlapError, bool) {
	var oe *domain.AllocationOverlapError
	ok := errors.As(err, &oe) && (msg == "" || oe.Message == msg)
	return oe, ok
}
