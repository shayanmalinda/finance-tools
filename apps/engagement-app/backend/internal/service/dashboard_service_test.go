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
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// fakeDashboardEntity records what the service asked for.
type fakeDashboardEntity struct {
	mu         sync.Mutex
	myWork     *engagemententity.DashboardMyWorkFilter
	queues     *engagemententity.DashboardQueuesFilter
	travel     *engagemententity.DashboardTravelFilter
	engs       *engagemententity.DashboardEngagementsFilter
	agencyID   int
	todayArgs  []string
	allocDate  *string
	fail       error
	travelFail error

	myWorkResp *engagemententity.DashboardMyWork
	queuesResp *engagemententity.DashboardQueues
	travelResp *engagemententity.DashboardTravel
	engsResp   *engagemententity.DashboardEngagements

	costResp    *engagemententity.DashboardCostLines
	costCalls   int
	costRanges  []string
	costFn      func(engagemententity.DashboardCostLinesFilter) *engagemententity.DashboardCostLines
	teamResp    *engagemententity.DashboardTravelTeam
	managerResp *engagemententity.DashboardManager
	managerArgs []int
	amResp      *engagemententity.DashboardAccountManager
	eventsResp  *engagemententity.DashboardEvents
	visasResp   *engagemententity.DashboardMyVisas
	visasErr    error
	trips       map[string]engagemententity.TravelRequestRecord
	calls       []string
}

func (f *fakeDashboardEntity) call(name string) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
}

func (f *fakeDashboardEntity) GetTravelRequest(_ context.Context, jn string) (engagemententity.TravelRequestRecord, error) {
	if t, ok := f.trips[jn]; ok {
		return t, nil
	}
	return engagemententity.TravelRequestRecord{}, &apierror.UpstreamError{Service: "engagement-entity", StatusCode: 404, Msg: "not found"}
}

func (f *fakeDashboardEntity) DashboardCostLines(_ context.Context, fl engagemententity.DashboardCostLinesFilter) (*engagemententity.DashboardCostLines, error) {
	f.mu.Lock()
	f.costCalls++
	f.costRanges = append(f.costRanges, fl.From+"|"+fl.To)
	fn := f.costFn
	f.mu.Unlock()
	if fn != nil {
		return fn(fl), f.fail
	}
	if f.costResp != nil {
		return f.costResp, f.fail
	}
	return &engagemententity.DashboardCostLines{}, f.fail
}
func (f *fakeDashboardEntity) DashboardTravelTeam(context.Context, string) (*engagemententity.DashboardTravelTeam, error) {
	f.call("travelTeam")
	if f.teamResp != nil {
		return f.teamResp, f.fail
	}
	return &engagemententity.DashboardTravelTeam{POByAge: []int{1, 2, 3, 4}, POCount: 10, POOldestDays: 99}, f.fail
}
func (f *fakeDashboardEntity) DashboardManager(_ context.Context, _ string, types []int) (*engagemententity.DashboardManager, error) {
	f.call("manager")
	f.mu.Lock()
	f.managerArgs = types
	f.mu.Unlock()
	if f.managerResp != nil {
		return f.managerResp, f.fail
	}
	return &engagemententity.DashboardManager{TopBy: "customer"}, f.fail
}
func (f *fakeDashboardEntity) DashboardAccountManager(context.Context, string, string) (*engagemententity.DashboardAccountManager, error) {
	f.call("accountManager")
	if f.amResp != nil {
		return f.amResp, f.fail
	}
	return &engagemententity.DashboardAccountManager{}, f.fail
}
func (f *fakeDashboardEntity) DashboardEvents(context.Context, string) (*engagemententity.DashboardEvents, error) {
	f.call("events")
	if f.eventsResp != nil {
		return f.eventsResp, f.fail
	}
	return &engagemententity.DashboardEvents{}, f.fail
}
func (f *fakeDashboardEntity) DashboardMyVisas(context.Context, string, string) (*engagemententity.DashboardMyVisas, error) {
	f.call("myVisas")
	if f.visasErr != nil {
		return nil, f.visasErr
	}
	if f.visasResp != nil {
		return f.visasResp, f.fail
	}
	return &engagemententity.DashboardMyVisas{}, f.fail
}

func (f *fakeDashboardEntity) DashboardAllocationsToday(_ context.Context, date string, _ int) (*engagemententity.DashboardAllocationsToday, error) {
	f.allocDate = &date
	c := "#abc"
	return &engagemententity.DashboardAllocationsToday{
		ByEngagementType:        []engagemententity.DashboardEngagementTypeCount{{EngagementTypeID: 1, EngagementType: "Internal Engagement", Count: 6}, {EngagementTypeID: 6, EngagementType: "Paid Post Sale", Count: 2}},
		PaidByAllocationType:    []engagemententity.DashboardAllocationTypeCount{{AllocationType: "Implementation", Color: &c, Count: 2}},
		NonPaidByAllocationType: []engagemententity.DashboardAllocationTypeCount{{AllocationType: "R&D", Count: 6}},
		Consultants:             7,
	}, f.fail
}
func (f *fakeDashboardEntity) DashboardMyWork(_ context.Context, fl engagemententity.DashboardMyWorkFilter) (*engagemententity.DashboardMyWork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.myWork = &fl
	if f.myWorkResp != nil {
		return f.myWorkResp, f.fail
	}
	return &engagemententity.DashboardMyWork{}, f.fail
}
func (f *fakeDashboardEntity) DashboardQueues(_ context.Context, fl engagemententity.DashboardQueuesFilter) (*engagemententity.DashboardQueues, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queues = &fl
	if f.queuesResp != nil {
		return f.queuesResp, f.fail
	}
	return &engagemententity.DashboardQueues{}, f.fail
}
func (f *fakeDashboardEntity) DashboardTravel(_ context.Context, fl engagemententity.DashboardTravelFilter) (*engagemententity.DashboardTravel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.travel = &fl
	if f.travelFail != nil {
		return nil, f.travelFail
	}
	if f.travelResp != nil {
		return f.travelResp, f.fail
	}
	return &engagemententity.DashboardTravel{Trips: &engagemententity.DashboardTravelTrips{}}, f.fail
}
func (f *fakeDashboardEntity) DashboardEngagements(_ context.Context, fl engagemententity.DashboardEngagementsFilter) (*engagemententity.DashboardEngagements, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.engs = &fl
	if f.engsResp != nil {
		return f.engsResp, f.fail
	}
	return &engagemententity.DashboardEngagements{Trend: &engagemententity.DashboardTrend{}}, f.fail
}
func (f *fakeDashboardEntity) DashboardAgency(_ context.Context, id int, _ string) (*engagemententity.DashboardAgency, error) {
	f.agencyID = id
	return &engagemententity.DashboardAgency{
		InvoicesByStatus: []engagemententity.DashboardCount{{Label: "Pending", Count: 2}, {Label: "Rejected by travel", Count: 1}, {Label: "Rejected by finance", Count: 1}},
		Upcoming:         []engagemententity.DashboardTrip{{JobNumber: "TRV-1", TravellerEmail: "a@wso2.com"}},
	}, f.fail
}

var dashboardNow = func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) }

func dashboardPerms(email string, alloc, allocOwn, travel, travelOwn []int, travelRoles ...string) domain.Perms {
	return domain.Perms{
		Email: email,
		Alloc: domain.NewPrivilegeSet(alloc), AllocOwn: domain.NewPrivilegeSet(allocOwn),
		Travel: domain.NewPrivilegeSet(travel), TravelOwn: domain.NewPrivilegeSet(travelOwn),
		TravelRoles: travelRoles,
	}
}

func dashboardRange(from, to int) []int {
	var out []int
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

var (
	dashboardEmployeeAlloc  = []int{1}
	dashboardEmployeeTravel = []int{19, 20, 21, 22, 23, 24, 36, 37, 38, 47, 48, 49}
)

// Employee: nothing from this endpoint (Allocations Today has its own; own
// allocations and trips are on the Me overview).
func TestDashboard_Employee(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("jane@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.myWork != nil || f.queues != nil || f.travel != nil || f.engs != nil {
		t.Errorf("unexpected calls: queues %+v travel %+v engs %+v", f.queues, f.travel, f.engs)
	}
	b, _ := json.Marshal(got)
	if string(b) != `{"date":"2026-11-05","failedSections":[],"widgets":[{"id":"allocations-today","defaultOn":false}]}` {
		t.Errorf("nothing to show must be only the date, failedSections and widgets, got %s", b)
	}
}

// Travel Team: super admin of both apps plus manager-po. Queues come only
// from own (non super admin) privileges, except Ready for Clearance; no
// finance, push or approval-of-engagements cards and no Travel Cost.
func TestDashboard_TravelTeamQueuesIgnoreSuperAdmin(t *testing.T) {
	f := &fakeDashboardEntity{queuesResp: &engagemententity.DashboardQueues{NewTravelRequests: 4, InvoicesPending: 2, InvoicesToPush: 9}}
	poOwn := append(dashboardRange(1, 17), 25, 26, 27, 28, 30, 32, 41, 42, 45, 46)
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("employee@wso2.com", dashboardRange(1, 33), []int{1}, dashboardRange(1, 49), poOwn, "app-travel-admin-super-stg", "app-travel-manager-po-stg"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range got.WaitingOnYou {
		keys = append(keys, c.Key)
	}
	if strings.Join(keys, ",") != "newTravelRequests,approve-po" {
		t.Errorf("cards = %v", keys)
	}
	if f.queues.Invoices != true || f.queues.Payments || len(f.queues.PendingApprovalTypes) != 0 ||
		len(f.queues.ClearanceTypes) != 0 {
		t.Errorf("queues filter = %+v", f.queues)
	}
	if got.WaitingOnYou[0].Count != 4 || got.WaitingOnYou[1].Count != 2 || got.WaitingOnYou[1].List.Path != "/finance/engagement-app/invoices/po" {
		t.Errorf("cards = %+v", got.WaitingOnYou)
	}
	// The Travel Super Admin sees Travel and (deprecated) Travel Cost with all four sources.
	if f.travel == nil || !f.travel.Trips || !f.travel.CostInvoices || !f.travel.CostPayments || !f.travel.CostCards || !f.travel.CostClaims || got.TravelCost == nil {
		t.Errorf("travel filter = %+v cost %+v", f.travel, got.TravelCost)
	}
	// Allocation sections are not on by default (own privileges are only Dashboard Page) ...
	if f.engs != nil {
		t.Errorf("engagements read by default: %+v", f.engs)
	}
	// ... but are served when asked for, since the effective privileges allow them.
	f.engs = nil
	got, err = svc.Dashboard(context.Background(), dashboardPerms("employee@wso2.com", dashboardRange(1, 33), []int{1}, dashboardRange(1, 49), poOwn, "app-travel-admin-super-stg", "app-travel-manager-po-stg"),
		[]string{"engagements", "availableConsultants", "allocationsOverTime"})
	if err != nil || f.engs == nil || len(f.engs.EngagementTypes) != 7 || !f.engs.Bench || !f.engs.Trend || got.Engagements == nil {
		t.Errorf("engagements filter = %+v, %v", f.engs, err)
	}

	// Super admin alone (no Travel Team group): Travel, but no travel queues.
	f = &fakeDashboardEntity{}
	got, err = NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(), dashboardPerms("s@wso2.com",
		dashboardRange(1, 25), []int{1}, dashboardRange(1, 49), dashboardEmployeeTravel, "app-travel-admin-super-stg", "wso2-employees"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !f.travel.Trips || !f.travel.CostInvoices || got.TravelCost == nil || f.queues != nil {
		t.Errorf("super admin: travel %+v queues %+v", f.travel, f.queues)
	}
}

// Finance approver: finance queues, Travel Cost with card spend; the finance
// step only without HR approval (32).
func TestDashboard_FinanceQueuesAndCardSpend(t *testing.T) {
	approverFinance := []int{2, 3, 8, 28, 30, 33, 34, 35, 39, 40, 41, 42, 43, 44, 45}
	f := &fakeDashboardEntity{queuesResp: &engagemententity.DashboardQueues{
		InvoicesAcceptedByTravel: 3, InvoicesToPush: 2, InvoicesFailed: 1, PaymentsToPush: 5, PaymentsFailed: 2,
	}}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	p := dashboardPerms("analyst@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, approverFinance, approverFinance, "App-Travel-Approver-Finance-stg")
	got, err := svc.Dashboard(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"approve": 3, "push": 2, "pay": 5, "failed": 3}
	cards := map[string]int{}
	for _, c := range got.WaitingOnYou {
		if _, dup := cards[c.Key]; dup {
			t.Errorf("card %s appears twice", c.Key)
		}
		cards[c.Key] = c.Count
	}
	if !reflect.DeepEqual(cards, want) {
		t.Errorf("cards = %v, want %v", cards, want)
	}
	// Travel Cost with card spend, but no Travel section.
	if !f.travel.CostCards || !f.travel.CostInvoices || f.travel.Trips {
		t.Errorf("travel filter = %+v", f.travel)
	}

	// With own HR approval (32), the finance step is not theirs.
	p.TravelOwn = domain.NewPrivilegeSet(append(approverFinance, 32))
	got, _ = svc.Dashboard(context.Background(), p, nil)
	for _, c := range got.WaitingOnYou {
		if c.Key == "approve" {
			t.Error("approve card shown to a holder of 32")
		}
	}
}

// Approvers: Pending Approval (own 28 / 31, always highlighted) and Ready
// for Clearance over the types they may set the status of.
func TestDashboard_ApprovalAndClearanceQueues(t *testing.T) {
	internalMgr := append(append([]int{1, 11}, dashboardRange(13, 21)...), 26, 27, 28)
	f := &fakeDashboardEntity{queuesResp: &engagemententity.DashboardQueues{
		PendingApproval: map[int]int{1: 2, 5: 9}, Clearance: map[string]int{"Ready for Clearance": 4},
	}}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("m@wso2.com", internalMgr, internalMgr, nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.queues.PendingApprovalTypes, []int{1}) || !reflect.DeepEqual(f.queues.ClearanceTypes, []int{1, 5}) {
		t.Errorf("queues filter = %+v", f.queues)
	}
	if len(got.WaitingOnYou) != 2 {
		t.Fatalf("cards = %+v", got.WaitingOnYou)
	}
	pa, ready := got.WaitingOnYou[0], got.WaitingOnYou[1]
	if pa.Key != "pendingApproval" || pa.Count != 2 || !pa.Highlighted || pa.List.Path != "/finance/engagement-app/approvals/internal" {
		t.Errorf("pending approval = %+v", pa)
	}
	if ready.Count != 4 || !ready.Highlighted || ready.List.Path != "/finance/engagement-app/allocations/engagement?clearanceStatus=Ready%20for%20Clearance&engagementTypeId=1&engagementTypeId=5" {
		t.Errorf("ready = %+v", ready)
	}
	// Engagements only of internal kinds; trend for allocation-list holders.
	if !reflect.DeepEqual(f.engs.EngagementTypes, []int{1, 5}) || !f.engs.Bench || !f.engs.Trend {
		t.Errorf("engagements filter = %+v", f.engs)
	}

	// An approver without an engagement update privilege cannot approve: no tile.
	approver := []int{1, 26, 27, 28, 29, 30, 31}
	if got, _ := svc.Dashboard(context.Background(), dashboardPerms("ap@wso2.com", approver, approver, nil, nil), nil); len(got.WaitingOnYou) != 0 {
		t.Errorf("approver-only cards = %+v", got.WaitingOnYou)
	}

	// A manager without 28 has no Pending Approval.
	mgr := internalMgr[:len(internalMgr)-1]
	got, _ = svc.Dashboard(context.Background(), dashboardPerms("m@wso2.com", mgr, mgr, nil, nil), nil)
	if len(got.WaitingOnYou) != 1 || got.WaitingOnYou[0].Key != "readyForClearance" {
		t.Errorf("manager cards = %+v", got.WaitingOnYou)
	}

	// A pre-sale group alone cannot allocate (legacy quirk): no clearance queue.
	if types := dashboardClearanceTypes(domain.NewPrivilegeSet([]int{33})); len(types) != 0 {
		t.Errorf("pre-sale only: %v", types)
	}
	if types := dashboardClearanceTypes(domain.NewPrivilegeSet([]int{1, 2, 3, 6, 9, 10, 33})); !reflect.DeepEqual(types, []int{3, 7}) {
		t.Errorf("customer manager + pre sale: %v", types)
	}
}

// Ready for Clearance counts on own privileges only: a super admin who holds
// the privileges through the super admin group alone gets no tile.
func TestDashboard_ClearanceIgnoresSuperAdmin(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	all := dashboardRange(1, 40)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("sa@wso2.com", all, nil, nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got.WaitingOnYou {
		if c.Key == "readyForClearance" {
			t.Errorf("super admin got a clearance tile: %+v", got.WaitingOnYou)
		}
	}
	if f.queues != nil && len(f.queues.ClearanceTypes) != 0 {
		t.Errorf("clearance types for a super admin: %v", f.queues.ClearanceTypes)
	}

	own := append(append([]int{1, 11}, dashboardRange(13, 21)...), 26, 27, 28)
	got, _ = svc.Dashboard(context.Background(), dashboardPerms("m@wso2.com", all, own, nil, nil), nil)
	if f.queues == nil || len(f.queues.ClearanceTypes) == 0 {
		t.Errorf("own-privilege manager lost the clearance queue: %+v", got.WaitingOnYou)
	}
}

// Available Consultants is gated on 9, 19 or 21 alone, not on 3 or 13.
func TestDashboard_AvailableConsultantsWithoutEngagements(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("rm@wso2.com", []int{21}, []int{21}, nil, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.engs == nil || !f.engs.Bench || f.engs.Trend || len(f.engs.EngagementTypes) != 0 {
		t.Fatalf("engagements filter = %+v", f.engs)
	}
	if got.AvailableConsultants == nil || got.Engagements != nil {
		t.Errorf("available = %+v engagements = %+v", got.AvailableConsultants, got.Engagements)
	}
}

func TestDashboard_TravelAgentGetsOnlyTheAgentDashboard(t *testing.T) {
	f := &fakeDashboardEntity{}
	p := dashboardPerms("agent@acorn.com", nil, nil, []int{3, 4, 6, 18, 26, 27, 28, 30, 31}, []int{3, 4, 6, 18, 26, 27, 28, 30, 31})
	p.Agency = &domain.TravelAgency{ID: 3, Name: "Acorn Travels"}
	got, err := NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent == nil || got.Travel != nil || got.WaitingOnYou != nil || f.agencyID != 3 {
		t.Fatalf("got %+v (agency %d)", got, f.agencyID)
	}
	a := got.Agent
	if a.Agency != "Acorn Travels" || a.InvoicesRejected != 2 || len(a.TripsByMonth) != 12 || len(a.Upcoming) != 1 {
		t.Errorf("agent = %+v", a)
	}
	b, _ := json.Marshal(a.Upcoming[0])
	if strings.Contains(string(b), "agency") {
		t.Errorf("agents get no agency field: %s", b)
	}
}

func TestDashboard_EntityErrorFailsTheRequest(t *testing.T) {
	f := &fakeDashboardEntity{fail: &apierror.UpstreamError{Service: "engagement-entity", Msg: "down"}}
	_, err := NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(), dashboardPerms("a@wso2.com", []int{1, 2, 3}, []int{1, 2, 3}, nil, nil), nil)
	var up *apierror.UpstreamError
	if !errors.As(err, &up) {
		t.Errorf("err = %v", err)
	}
}

// One failing section is reported; the others are still returned.
func TestDashboard_FailedSectionKeepsTheOthers(t *testing.T) {
	f := &fakeDashboardEntity{travelFail: &apierror.UpstreamError{Service: "engagement-entity", Msg: "down"}}
	got, err := NewDashboardService(f, nil, nil, dashboardNow).Dashboard(context.Background(),
		dashboardPerms("a@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, []int{1, 2, 49}, []int{1, 2, 49}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.FailedSections, []string{"travel"}) || got.Travel != nil || f.myWork != nil {
		t.Errorf("failed %v travel %v", got.FailedSections, got.Travel)
	}
}

// The Me overview reads the entity's my-work endpoint (the dashboard no longer does).
func TestMeOverview_AllocationsAndTrips(t *testing.T) {
	six, one := 6, 1
	acme, title, purpose := "Acme Corp", "API rollout", "Platform upgrade"
	ready := "Ready for Clearance"
	typ, code := "R&D", "RND-GEN"
	f := &fakeDashboardEntity{myWorkResp: &engagemententity.DashboardMyWork{
		Allocations: []engagemententity.DashboardOwnAllocation{
			{ID: "A1", EngagementID: "E1", EngagementTypeID: &six, Account: &acme, Title: &title, StartDate: "2026-11-01", EndDate: "2026-11-20"},
			{ID: "A2", EngagementID: "E2", EngagementTypeID: &one, Account: &acme, Title: &purpose, StartDate: "2026-11-03", EndDate: "2026-11-09"},
			{ID: "A3", EngagementID: "E3", EngagementTypeID: &one, Title: &purpose, StartDate: "2026-12-01", EndDate: "2026-12-09", ClearanceStatus: &ready},
		},
		TeamAllocation: &engagemententity.DashboardTeamAllocation{ID: "A9", AllocationType: &typ, EngagementCode: &code, StartDate: "2026-11-01", EndDate: "2026-11-30"},
		Trips:          []engagemententity.DashboardTrip{{JobNumber: "TRV-2026-11-0001", TravellerEmail: "jane@wso2.com"}},
		TripCount:      2,
	}}
	svc := NewDashboardService(f, nil, nil, dashboardNow)

	// Me overview: current first, then the future ones; at most three.
	ov, err := svc.MeOverview(context.Background(), dashboardPerms("jane@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, dashboardEmployeeTravel, dashboardEmployeeTravel))
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.UpcomingAllocations) != 2 || ov.UpcomingAllocations[0].ID != "A1" || ov.UpcomingAllocations[1].ID != "A3" || len(ov.UpcomingTrips) != 1 {
		t.Errorf("overview = %+v", ov)
	}
	if f.myWork.TripLimit != 3 {
		t.Errorf("trip limit = %d", f.myWork.TripLimit)
	}
	// Without My Travel Requests no trips are read.
	if _, err := svc.MeOverview(context.Background(), dashboardPerms("jane@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, nil, nil)); err != nil || f.myWork.Trips {
		t.Errorf("trips read without 49/36: %+v, %v", f.myWork, err)
	}
}

func TestDashboard_TravelCostUnconvertedWithoutConverter(t *testing.T) {
	cost := []engagemententity.DashboardCostLine{
		{Source: "invoice", Month: "2026-11", CostType: "Air Ticket", Currency: "USD", Amount: 1000},
		{Source: "invoice", Month: "2026-10", CostType: "Hotel", Currency: "USD", Amount: 500.555},
		{Source: "invoice", Month: "2026-10", CostType: "Hotel", Currency: "LKR", Amount: 90000},
		{Source: "invoice", Month: "2025-11", CostType: "Hotel", Currency: "USD", Amount: 77}, // outside the window
		{Source: "perDiem", Month: "2026-10", Currency: "USD", Amount: 400},
		{Source: "perDiem", Month: "2026-09", Currency: "GBP", Amount: 70},
		{Source: "card", Month: "2026-10", Currency: "USD", Amount: 100},
		{Source: "claim", Month: "2026-10", Currency: "USD", Amount: 50},
	}
	tr := &engagemententity.DashboardTravel{From: "2025-12-01", To: "2026-12-01", Cost: cost}
	today := dashboardNow()

	c := dashboardTravelCost(context.Background(), tr, today, true, newDashboardRates(nil, dashboardNow))
	if c.Total != 2050.56 || c.TravelInvoices != 1500.56 || c.PerDiems != 400 || c.CreditCardExpenses == nil || *c.CreditCardExpenses != 100 ||
		c.ExpenseClaims == nil || *c.ExpenseClaims != 50 {
		t.Errorf("totals = %+v", c)
	}
	if c.LastMonth != 1050.56 || c.CurrentMonth != 1000 || len(c.ByMonth) != 12 || c.ByMonth[11].Month != "2026-11" {
		t.Errorf("months = last %v current %v %+v", c.LastMonth, c.CurrentMonth, c.ByMonth)
	}
	if c.ByMonth[10].Values["Credit Card Expenses"] != 100 || c.ByMonth[10].Values["Expense Claims"] != 50 || c.ByMonth[0].Values["Per Diems"] != 0 {
		t.Errorf("series = %+v", c.ByMonth[10])
	}
	if len(c.ByType) != 5 || c.ByType[0].Label != "Air Ticket" {
		t.Errorf("byType = %+v", c.ByType)
	}
	if len(c.NotConverted) != 2 || c.NotConverted[0] != (domain.DashboardUnconverted{Source: "invoice", Currency: "LKR", Amount: 90000}) {
		t.Errorf("notConverted = %+v", c.NotConverted)
	}

	// Without a card role, card lines are ignored and the series has no card key.
	c = dashboardTravelCost(context.Background(), tr, today, false, newDashboardRates(nil, dashboardNow))
	if c.CreditCardExpenses != nil || c.ExpenseClaims != nil || c.Total != 1900.56 {
		t.Errorf("no card: %+v", c)
	}
	if _, ok := c.ByMonth[10].Values["Credit Card Expenses"]; ok {
		t.Error("card series without a card role")
	}
}

// fakeRates answers with fixed USD rates and counts calls.
type fakeRates struct {
	mu    sync.Mutex
	rates map[string]float64 // base currency -> units per USD
	err   error
	calls []string
	delay time.Duration
}

func (f *fakeRates) Rates(ctx context.Context, base, date string) (map[string]float64, error) {
	f.mu.Lock()
	f.calls = append(f.calls, base+"|"+date)
	delay, fail := f.delay, f.err
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail != nil {
		return nil, fail
	}
	return map[string]float64{"USD": f.rates[base]}, nil
}

func (f *fakeRates) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func dashboardCostFixture() *engagemententity.DashboardTravel {
	return &engagemententity.DashboardTravel{From: "2025-12-01", To: "2026-12-01", Cost: []engagemententity.DashboardCostLine{
		{Source: "invoice", Month: "2026-10", CostType: "Hotel", Currency: "LKR", Amount: 90000},
		{Source: "invoice", Month: "2026-11", CostType: "Hotel", Currency: "LKR", Amount: 30000},
		{Source: "perDiem", Month: "2026-10", Currency: "USD", Amount: 400},
		{Source: "card", Month: "2026-10", Currency: "LKR", Amount: 15000, CCCurrency: "USD", CCAmount: 48.2},
		{Source: "claim", Month: "2026-10", Currency: "EUR", Amount: 100},
	}}
}

func TestDashboard_TravelCostConvertsAtMonthEndRate(t *testing.T) {
	f := &fakeRates{rates: map[string]float64{"LKR": 300, "EUR": 0.9}}
	c := dashboardTravelCost(context.Background(), dashboardCostFixture(), dashboardNow(), true, newDashboardRates(f, dashboardNow))
	// 90000/300 = 300 (October), 30000/300 = 100 (November, today's rate), 400 USD,
	// the card billed in USD counts 48.20 without a rate, 100 EUR / 0.9 = 111.11.
	if c.TravelInvoices != 400 || c.PerDiems != 400 || *c.CreditCardExpenses != 48.2 || *c.ExpenseClaims != 111.11 || c.Total != 959.31 {
		t.Errorf("totals = %+v", c)
	}
	if c.CurrentMonth != 100 || c.LastMonth != 859.31 {
		t.Errorf("months: last %v current %v", c.LastMonth, c.CurrentMonth)
	}
	if c.RateRule != "monthEnd" || len(c.NotConverted) != 0 {
		t.Errorf("notConverted = %+v", c.NotConverted)
	}
	got := map[string]bool{}
	for _, call := range f.calls {
		got[call] = true
	}
	// LKR October (month end), LKR November (today), EUR October; the USD line
	// and the card billed in USD make no call.
	if len(f.calls) != 3 || !got["LKR|2026-10-31"] || !got["LKR|2026-11-05"] || !got["EUR|2026-10-31"] {
		t.Errorf("rate calls = %v", f.calls)
	}
}

func TestDashboard_TravelCostNotConverted(t *testing.T) {
	cases := map[string]FinanceCurrencyConverter{
		"nil converter":  nil,
		"converter down": &fakeRates{err: errors.New("boom")},
		"rate zero":      &fakeRates{rates: map[string]float64{"LKR": 0, "EUR": 0}},
	}
	for name, conv := range cases {
		t.Run(name, func(t *testing.T) {
			c := dashboardTravelCost(context.Background(), dashboardCostFixture(), dashboardNow(), true, newDashboardRates(conv, dashboardNow))
			// Only the USD per diem and the USD-billed card line are totalled.
			if c.Total != 448.2 || c.PerDiems != 400 || c.TravelInvoices != 0 || *c.ExpenseClaims != 0 {
				t.Errorf("totals = %+v", c)
			}
			want := []domain.DashboardUnconverted{
				{Source: "claim", Currency: "EUR", Amount: 100},
				{Source: "invoice", Currency: "LKR", Amount: 120000},
			}
			if len(c.NotConverted) != 2 || c.NotConverted[0] != want[0] || c.NotConverted[1] != want[1] {
				t.Errorf("notConverted = %+v", c.NotConverted)
			}
		})
	}
}

func TestDashboard_TravelCostSectionSurvivesRateFailure(t *testing.T) {
	f := &fakeDashboardEntity{travelResp: dashboardCostFixture()}
	svc := NewDashboardService(f, nil, &fakeRates{err: errors.New("down")}, dashboardNow)
	got, err := svc.Dashboard(context.Background(), dashboardPerms("fin@wso2.com", nil, nil, []int{39, 41}, []int{39, 41},
		"App-Travel-Approver-Finance-stg"), nil)
	if err != nil || got.TravelCost == nil || len(got.FailedSections) != 0 || len(got.TravelCost.NotConverted) != 2 {
		t.Fatalf("got %+v, %v", got.TravelCost, err)
	}
}

func TestDashboardBuckets_TopNWithOther(t *testing.T) {
	rows := []dashboardRow{{label: "a", value: 5}, {label: "b", value: 3}, {label: "c", value: 1}, {label: "d", value: 1}}
	got := dashboardBuckets(rows, 3)
	if len(got) != 3 || got[2].Label != "Other" || got[2].Value != 2 || got[0].Percent != 50 {
		t.Errorf("buckets = %+v", got)
	}
}

func TestDashboardAllocationsTodayService(t *testing.T) {
	f := &fakeDashboardEntity{}
	svc := NewDashboardService(f, nil, nil, dashboardNow)
	p := dashboardPerms("a@wso2.com", dashboardEmployeeAlloc, dashboardEmployeeAlloc, nil, nil)

	got, err := svc.AllocationsToday(context.Background(), p, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if *f.allocDate != "2026-11-05" || got.Date != "2026-11-05" || got.BusinessUnitID != nil {
		t.Errorf("today: entity date %q, %+v", *f.allocDate, got)
	}
	if got.Allocations != 8 || got.Paid != 2 || got.NonPaid != 6 || got.Consultants != 7 {
		t.Errorf("kpis = %+v", got)
	}
	if got.ByEngagementType[0].Label != "Internal Engagement" || *got.ByEngagementType[0].ID != 1 || got.ByEngagementType[0].Percent != 75 {
		t.Errorf("by engagement type = %+v", got.ByEngagementType)
	}
	if *got.PaidByAllocationType[0].Color != "#abc" {
		t.Errorf("paid = %+v", got.PaidByAllocationType)
	}

	// No date and today's date are the same request to the entity (one rule, one clock); a past date is passed on.
	if _, err := svc.AllocationsToday(context.Background(), p, "2026-11-05", 4); err != nil || *f.allocDate != "2026-11-05" {
		t.Errorf("explicit today: %q, %v", *f.allocDate, err)
	}
	if got, err := svc.AllocationsToday(context.Background(), p, "2026-10-01", 4); err != nil || *f.allocDate != "2026-10-01" || *got.BusinessUnitID != 4 {
		t.Errorf("past: %q, %v", *f.allocDate, err)
	}
	var ve *apierror.ValidationError
	if _, err := svc.AllocationsToday(context.Background(), p, "2026-11-06", 0); !errors.As(err, &ve) {
		t.Errorf("future date: %v", err)
	}
}

func TestDashboardSplitAllocationsCanOpen(t *testing.T) {
	paid, marketing := domain.EngagementTypePaid, domain.EngagementTypeMarketing
	list := []engagemententity.DashboardOwnAllocation{
		{ID: "1", EngagementID: "E1", EngagementTypeID: &paid, StartDate: "2026-11-01", EndDate: "2026-11-30"},
		{ID: "2", EngagementID: "E2", EngagementTypeID: &marketing, StartDate: "2026-12-01", EndDate: "2026-12-05"},
		{ID: "3", EngagementID: "E3", StartDate: "2026-12-10", EndDate: "2026-12-12"},
	}
	current, next := dashboardSplitAllocations(list, "2026-11-24", map[int]bool{paid: true})
	if current == nil || !current.CanOpen {
		t.Fatalf("current = %+v, want E1 openable", current)
	}
	if len(next) != 2 || next[0].CanOpen || next[1].CanOpen {
		t.Fatalf("next = %+v, want E2 and E3 not openable", next)
	}
}
