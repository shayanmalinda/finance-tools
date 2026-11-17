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
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

type allocationMocks struct {
	alloc, travel sqlmock.Sqlmock
	repo          *allocationRepo
}

func newAllocationMocks(t *testing.T) allocationMocks {
	t.Helper()
	a, am, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	tr, tm, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); tr.Close() })
	return allocationMocks{alloc: am, travel: tm, repo: &allocationRepo{alloc: a, travel: tr}}
}

func (m allocationMocks) verify(t *testing.T) {
	t.Helper()
	for name, mk := range map[string]sqlmock.Sqlmock{"alloc": m.alloc, "travel": m.travel} {
		if err := mk.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

var allocationCols = []string{
	"id", "email", "allocation_type", "aT.allocation_type", "color", "startDate", "endDate",
	"isRecurring", "start_time", "end_time", "time_zone", "comment", "clearance_status",
	"isTravelRequired", "status", "added_by", "added_date", "engagement_id", "engagement_code",
	"e.id", "eaT.engagement_type", "eT.name", "eT.type", "eaT.allocation_type", "customer_name", "project_name",
	"e.engagement_code", "country", "location", "e.status", "engagement_status",
	"eStart", "eEnd", "opportunity_line_item_id",
}

// engagementAllocationRow is A1 of E07201 (Paid Post Sale), Confirmed, travel Yes.
func engagementAllocationRow(clearance, travel string) []driver.Value {
	return []driver.Value{
		"A0000000001", "jane@wso2.com", 12, "Consulting", "#d030eb",
		time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		"No", nil, nil, nil, "note", clearance,
		travel, "Active", "mgr@wso2.com", []byte("2026-11-01 10:20:30"), "E07201", "NWB-CO",
		"E07201", 6, "Paid Post Sale", "Customer", "Consulting", "Acme", "Acme - PoC",
		"NWB-CO", "Sri Lanka", "Colombo", "In Progress", "Confirmed",
		"2026-11-01", "2026-11-30", "LI-1",
	}
}

func defaultAllocationRow() []driver.Value {
	return []driver.Value{
		"A0000000002", "jane@wso2.com", 40, "Digital Operations", nil,
		"2026-11-01", "2026-11-30", "No", nil, nil, nil, nil, "Confirmed",
		"No", "Active", "mgr@wso2.com", nil, "-1", "DIG-GEN",
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	}
}

func TestAllocationEngagement_WithFundingSources(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectQuery(re("WHERE e.id != '' AND e.id = ?")).WithArgs("E07201").WillReturnRows(
		sqlmock.NewRows([]string{"id", "et", "etn", "ett", "at", "atn", "eta", "cn", "cc", "pn", "oi", "li", "prod",
			"loc", "country", "bd", "user", "cs", "code", "s", "e", "st", "es", "scope", "nature", "contact", "owner",
			"tech", "comment", "desc"}).
			AddRow("E07201", 6, "Paid Post Sale", "Customer", 12, "Consulting", "Yes", "Acme", "001", "Acme - PoC",
				"OPP", "LI-1", "APIM", "Colombo", "Sri Lanka", nil, "kevin@wso2.com", int64(1), "NWB-CO",
				time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "2026-11-30", "New", "Pending", nil, "On-site", nil,
				"Owner", "Tech", nil, nil))
	m.alloc.ExpectQuery(re("FROM engagement_funding_source AS eng_fs")).WithArgs("E07201").WillReturnRows(
		sqlmock.NewRows([]string{"engagementId", "bu", "pu", "sub", "region", "pct"}).
			AddRow("E07201", "APIM", "APIM-PU", "NA - EAST", "NA", []byte("100.00")))

	e, err := m.repo.Engagement(context.Background(), "E07201")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil || e.EngagementTypeID != 6 || !e.ContractsSigned || *e.StartDate != "2026-11-01" || *e.EndDate != "2026-11-30" {
		t.Fatalf("got %+v", e)
	}
	if len(e.FundingSources) != 1 || *e.FundingSources[0].Percentage != 100 || *e.FundingSources[0].SubRegion != "NA - EAST" {
		t.Errorf("funding = %+v", e.FundingSources)
	}
	m.verify(t)
}

func TestAllocationEngagement_NotFound(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectQuery(re("FROM engagement AS e")).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	e, err := m.repo.Engagement(context.Background(), "E1")
	if err != nil || e != nil {
		t.Fatalf("got %v, %v", e, err)
	}
	m.verify(t)
}

func TestAllocationGet_EngagementAndDefault(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WithArgs("A0000000001").
		WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow("Confirmed", "Yes")...))
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WithArgs("A0000000002").
		WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(defaultAllocationRow()...))
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WithArgs("X").WillReturnRows(sqlmock.NewRows(allocationCols))

	a, err := m.repo.Get(context.Background(), "A0000000001")
	if err != nil {
		t.Fatal(err)
	}
	if a.Engagement == nil || a.Engagement.ID != "E07201" || a.Engagement.EngagementTypeID != 6 ||
		a.StartDate != "2026-11-10" || *a.AddedDate != "2026-11-01T10:20:30Z" || *a.Engagement.ProductID != "LI-1" {
		t.Errorf("engagement allocation = %+v / %+v", a, a.Engagement)
	}
	d, err := m.repo.Get(context.Background(), "A0000000002")
	if err != nil || d.Engagement != nil || *d.EngagementCode != "DIG-GEN" {
		t.Errorf("default allocation = %+v, %v", d, err)
	}
	if none, err := m.repo.Get(context.Background(), "X"); none != nil || err != nil {
		t.Errorf("missing = %v, %v", none, err)
	}
	m.verify(t)
}

func TestAllocationTimeline_WindowTypesAndActiveOnly(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectQuery(re("WHERE a.email IN (?, ?) AND (")+".*"+re("AND a.status = 'Active' AND a.allocation_type IN (?) ORDER BY a.start_date, a.id")).
		WithArgs("a@wso2.com", "b@wso2.com", "2026-11-01", "2026-11-30", "2026-11-01", "2026-11-30",
			"2026-11-01", "2026-11-30", "2026-11-01", "2026-11-30", 40).
		WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(defaultAllocationRow()...))

	got, err := m.repo.Timeline(context.Background(), domain.AllocationTimelineRequest{
		Emails: []string{"a@wso2.com", "b@wso2.com"}, StartDate: "2026-11-01", EndDate: "2026-11-30", AllocationTypeIDs: []int{40},
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if out, err := m.repo.Timeline(context.Background(), domain.AllocationTimelineRequest{}); err != nil || out == nil || len(out) != 0 {
		t.Errorf("no emails: %v, %v", out, err)
	}
	m.verify(t)
}

func TestAllocationSearch_FiltersCountPageAndTravel(t *testing.T) {
	m := newAllocationMocks(t)
	yes := true
	req := domain.AllocationSearchRequest{
		EngagementTypeIDs: []int{3, 4}, ClearanceStatuses: []string{"Confirmed"}, Consultants: []string{"jane@wso2.com"},
		AllocationTypes: []string{"Consulting"}, TravelRequired: &yes, StartFrom: "2026-01-01", EndTo: "2026-12-31",
		Search: "50%", Today: "2026-11-05", Limit: 10, Offset: 20,
	}
	where := re("WHERE e.status != 'Deleted' AND a.status IN ('Active') AND eaT.engagement_type IN (?, ?)" +
		" AND a.clearance_status IN (?) AND a.email IN (?) AND eaT.allocation_type IN (?)" +
		" AND a.isTravelRequired = 'Yes' AND a.start_date >= ? AND a.end_date <= ?" +
		" AND (a.id LIKE ? OR a.email LIKE ? OR e.id LIKE ? OR e.project_name LIKE ? OR e.customer_name LIKE ? OR e.engagement_code LIKE ?)")
	args := []driver.Value{3, 4, "Confirmed", "jane@wso2.com", "Consulting", "2026-01-01", "2026-12-31 23:59:59",
		`%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`, `%50\%%`}
	m.alloc.ExpectQuery(re("SELECT COUNT(*)") + ".*" + where).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(21))
	m.alloc.ExpectQuery(where + ".*" + re("ORDER BY (a.end_date >= ?) DESC, CASE WHEN a.end_date >= ? THEN a.start_date END ASC, a.start_date DESC, a.id ASC LIMIT ? OFFSET ?")).
		WithArgs(append(args, "2026-11-05", "2026-11-05", 10, 20)...).
		WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow("Confirmed", "Yes")...))
	m.travel.ExpectQuery(re("FROM travel WHERE allocation_id IN (?) ORDER BY job_number")).WithArgs("A0000000001").
		WillReturnRows(sqlmock.NewRows([]string{"job", "aid", "email", "status"}).
			AddRow("TRV-2026-11-0001", "A0000000001", "jane@wso2.com", "New"))

	items, total, err := m.repo.Search(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if total != 21 || len(items) != 1 || len(items[0].TravelRequests) != 1 || items[0].TravelRequests[0].JobNumber != "TRV-2026-11-0001" {
		t.Errorf("got %d %+v", total, items)
	}
	m.verify(t)
}

func TestAllocationSearch_NoVisibleTypesRunsNoQuery(t *testing.T) {
	m := newAllocationMocks(t)
	items, total, err := m.repo.Search(context.Background(), domain.AllocationSearchRequest{Limit: 10})
	if err != nil || total != 0 || items == nil {
		t.Errorf("got %v %d %v", items, total, err)
	}
	m.verify(t)
}

func confirmed() *string { s := "Confirmed"; return &s }

func allocStr(s string) *string { return &s }

func TestAllocationBulk_IdenticalActiveAllocationIsRefused(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")+".*"+re("FOR UPDATE")).
		WithArgs("jane@wso2.com", "E07201", "2026-11-10", "2026-11-20", "09:00", "11:00").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	m.alloc.ExpectRollback()

	_, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{{Email: "jane@wso2.com", EngagementID: "E07201",
			StartDate: "2026-11-10", EndDate: "2026-11-20", StartTime: allocStr("09:00"), EndTime: allocStr("11:00"), Status: "Active"}}})
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	m.verify(t)
}

func TestAllocationBulk_DuplicateCandidatesInOnePayloadAreRefused(t *testing.T) {
	m := newAllocationMocks(t)
	a := domain.AllocationInput{Email: "jane@wso2.com", EngagementID: "E07201", StartDate: "2026-11-10", EndDate: "2026-11-20",
		StartTime: allocStr("09:00"), EndTime: allocStr("11:00"), Status: "Active"}
	twin := a
	twin.Email = "Jane@wso2.com"
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectRollback()

	_, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{a, twin}})
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	m.verify(t)
}

func TestAllocationBulk_InactiveAfterAnActiveTwinIsRefused(t *testing.T) {
	m := newAllocationMocks(t)
	a := domain.AllocationInput{Email: "jane@wso2.com", EngagementID: "E07201", StartDate: "2026-11-10",
		EndDate: "2026-11-20", Status: "Active"}
	inactive := a
	inactive.Status = "Inactive"
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectRollback()

	// In separate requests the Inactive one meets the saved Active row.
	_, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{a, inactive}})
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	m.verify(t)
}

func TestAllocationBulk_InactiveTwinsAreLeftToTheSavedRowCheck(t *testing.T) {
	m := newAllocationMocks(t)
	a := domain.AllocationInput{Email: "jane@wso2.com", EngagementID: "E07201", StartDate: "2026-11-10",
		EndDate: "2026-11-20", Status: "Inactive"}
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	// The second twin reaches the saved-row check: the first set no in-payload key.
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	m.alloc.ExpectRollback()

	_, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{a, a}})
	var ce *apierror.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	m.verify(t)
}

func TestAllocationBulk_TravelBindsToTheFirstQualifyingAllocationID(t *testing.T) {
	m := newAllocationMocks(t)
	eng := "E07201"
	plain := domain.AllocationInput{Email: "jane@wso2.com", AllocationTypeID: 12, Status: "Active",
		StartDate: "2026-11-10", EndDate: "2026-11-20", EngagementID: eng, IsTravelRequired: "No", IsRecurring: "No"}
	qualifying := plain
	qualifying.IsTravelRequired, qualifying.ClearanceStatus = "Yes", confirmed()
	qualifying.StartTime, qualifying.EndTime = allocStr("09:00"), allocStr("11:00")

	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*)")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectQuery(re("SELECT COUNT(*)")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A1"))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A2"))
	m.alloc.ExpectQuery(re("SELECT aT.allocation_type")).WithArgs("A2").WillReturnRows(
		sqlmock.NewRows([]string{"t", "c", "l", "cn", "p"}).AddRow("T", "LK", nil, "Acme", nil))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).WithArgs("jane@wso2.com", "T", "LK", nil, "", "Acme", "2026-11-10", "2026-11-20",
		"New", "Pre-Approved", "mgr@wso2.com", "mgr@wso2.com", "unmodified", "Approved", nil, eng, "A2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-1"))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()

	// Distinct times keep the two rows from being duplicates of each other.
	plain.StartTime, plain.EndTime = allocStr("13:00"), allocStr("14:00")
	if _, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{plain, qualifying}}); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}

func TestAllocationBulk_InsertsTravelUpdatesLogsAndRequestStatus(t *testing.T) {
	m := newAllocationMocks(t)
	eng := "E07201"
	main := domain.AllocationInput{Email: "jane@wso2.com", AllocationTypeID: 12, Status: "Active", ClearanceStatus: confirmed(),
		StartDate: "2026-11-10", EndDate: "2026-11-20", EngagementID: eng, EngagementCode: "NWB-CO",
		IsTravelRequired: "Yes", IsRecurring: "No"}
	eo := domain.AllocationInput{Email: "jane@wso2.com", AllocationTypeID: 99, Status: "Active", ClearanceStatus: confirmed(),
		StartDate: "2026-11-05", EndDate: "2026-11-09", EngagementID: "-1.0", EngagementCode: "NWB-CO-EO",
		IsTravelRequired: "No", IsRecurring: "No"}

	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*) FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).
		WithArgs("jane@wso2.com", 12, "Active", nil, "Confirmed", "2026-11-10", "2026-11-20", nil, nil, nil,
			"mgr@wso2.com", eng, "NWB-CO", "Yes", "No", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WithArgs("jane@wso2.com", eng, "2026-11-10", "2026-11-20", "Active").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A0000000009"))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT aT.allocation_type, e.country")).WithArgs("A0000000009").
		WillReturnRows(sqlmock.NewRows([]string{"t", "c", "l", "cn", "p"}).
			AddRow("Consulting", "Sri Lanka", "Colombo → HQ", "Acme", "LI-1"))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).
		WithArgs("jane@wso2.com", "Consulting", "Sri Lanka", "Colombo -> HQ", "NWB-CO", "Acme", "2026-11-10", "2026-11-20",
			"New", "Pre-Approved", "mgr@wso2.com", "mgr@wso2.com", "unmodified", "Approved", "LI-1", eng, "A0000000009").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number FROM travel")).WithArgs("A0000000009", "jane@wso2.com").
		WillReturnRows(sqlmock.NewRows([]string{"job"}).AddRow("TRV-2026-11-0042"))
	m.travel.ExpectCommit()
	m.alloc.ExpectExec(re("UPDATE allocation SET start_date = ?, end_date = ?, updated_by = ?")).
		WithArgs("2026-11-21", "2026-11-30", "mgr@wso2.com", "A0000000005").WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectExec(re("UPDATE engagement SET updated_by = ?, status = ? WHERE Id = ?")).
		WithArgs("mgr@wso2.com", "In Progress", eng).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()

	resp, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{
		CreatedBy: "mgr@wso2.com", NewAllocations: []domain.AllocationInput{main, eo},
		UpdateAllocations:       []domain.AllocationDateUpdate{{ID: "A0000000005", StartDate: "2026-11-21", EndDate: "2026-11-30"}},
		RequestStatusInProgress: &eng,
	})
	if err != nil {
		t.Fatal(err)
	}
	if *resp.FirstAllocationID != "A0000000009" || *resp.TravelJobNumber != "TRV-2026-11-0042" || resp.RecordsAdded != 2 || resp.RecordsUpdated != 1 {
		t.Errorf("resp = %+v", resp)
	}
	m.verify(t)
}

func TestAllocationBulk_UploadLogsAndTravelFailureRollsBack(t *testing.T) {
	m := newAllocationMocks(t)
	logs := domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{{Email: "a@wso2.com", AllocationTypeID: 40, Status: "Active",
			StartDate: "2026-11-01", EndDate: "2026-11-05", EngagementID: "-1.0", EngagementCode: "DIG-GEN",
			IsTravelRequired: "No", IsRecurring: "No"}},
		AllocationLogs: []domain.AllocationUploadLog{{Email: "a@wso2.com", AllocationTypeID: 40, StartDate: "2026-11-01",
			EndDate: "2026-11-05", EngagementCode: "DIG-GEN"}}}
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A1"))
	m.alloc.ExpectExec(re("INSERT INTO allocation_upload_log (")).
		WithArgs("a@wso2.com", "2026-11-01", "2026-11-05", 40, "DIG-GEN", "mgr@wso2.com").WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()
	if _, err := m.repo.Bulk(context.Background(), logs); err != nil {
		t.Fatal(err)
	}

	// A travel insert failure rolls the allocation back (legacy order).
	m.alloc.ExpectBegin()
	m.alloc.ExpectQuery(re("SELECT COUNT(*)")).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	m.alloc.ExpectExec(re("INSERT INTO allocation (")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectQuery(re("SELECT id FROM allocation")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("A2"))
	m.alloc.ExpectQuery(re("SELECT aT.allocation_type")).WithArgs("A2").WillReturnRows(
		sqlmock.NewRows([]string{"t", "c", "l", "cn", "p"}).AddRow("T", "LK", nil, "Acme", nil))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).WillReturnError(errors.New("boom"))
	m.travel.ExpectRollback()
	m.alloc.ExpectRollback()
	_, err := m.repo.Bulk(context.Background(), domain.AllocationBulkRequest{CreatedBy: "mgr@wso2.com",
		NewAllocations: []domain.AllocationInput{{Email: "b@wso2.com", AllocationTypeID: 1, Status: "Active", ClearanceStatus: confirmed(),
			StartDate: "2026-11-01", EndDate: "2026-11-02", EngagementID: "E1", IsTravelRequired: "Yes", IsRecurring: "No"}}})
	if err == nil {
		t.Error("travel failure: want error")
	}
	m.verify(t)
}

func preRecord(clearance, travel, email string) domain.AllocationRecord {
	c, tr, by := clearance, travel, "adder@wso2.com"
	code, country, addr, cust, typ, li := "NWB-CO", "Sri Lanka", "Colombo", "Acme", "Consulting", "LI-1"
	return domain.AllocationRecord{ID: "A0000000001", Email: email, StartDate: "2026-11-10", EndDate: "2026-11-20",
		ClearanceStatus: &c, IsTravelRequired: &tr, AddedBy: &by,
		Engagement: &domain.AllocationEngagementRef{ID: "E07201", EngagementCode: &code, Country: &country, Address: &addr,
			CustomerName: &cust, AllocationTypeName: &typ, ProductID: &li}}
}

func TestAllocationUpdate_ConfirmCreatesTravelAddedByTheAllocator(t *testing.T) {
	m := newAllocationMocks(t)
	next, status, comment := "Confirmed", "Active", "ok"
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET updated_by = ?, clearance_status = ?, status = ?, comment = ? WHERE Id = ?")).
		WithArgs("mgr@wso2.com", "Confirmed", "Active", "ok", "A0000000001").WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).
		WithArgs("jane@wso2.com", "Consulting", "Sri Lanka", "Colombo", "NWB-CO", "Acme", "2026-11-10", "2026-11-20",
			"New", "Pre-Approved", "adder@wso2.com", "adder@wso2.com", "unmodified", "Approved", "LI-1", "E07201", "A0000000001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-1"))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow("Confirmed", "Yes")...))

	resp, err := m.repo.Update(context.Background(), preRecord("Ready for Clearance", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &next, Status: &status, Comment: &comment, UpdatedBy: "mgr@wso2.com"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.TravelJobNumber == nil || *resp.TravelJobNumber != "TRV-1" || *resp.PreviousClearanceStatus != "Ready for Clearance" {
		t.Errorf("resp = %+v", resp)
	}
	m.verify(t)
}

func TestAllocationUpdate_UnchangedStatusAddsNoTravel_UpdatedDatesGoToTravel(t *testing.T) {
	m := newAllocationMocks(t)
	same := "Confirmed"
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow(same, "Yes")...))
	if _, err := m.repo.Update(context.Background(), preRecord("Confirmed", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &same, UpdatedBy: "mgr@wso2.com"}); err != nil {
		t.Fatal(err)
	}

	start, end := "2026-12-01", "2026-12-05"
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("INSERT INTO travel (")).
		WithArgs("jane@wso2.com", "Consulting", "Sri Lanka", "Colombo", "NWB-CO", "Acme", start, end,
			"New", "Pre-Approved", "adder@wso2.com", "adder@wso2.com", "unmodified", "Approved", "LI-1", "E07201", "A0000000001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectQuery(re("SELECT job_number")).WillReturnRows(sqlmock.NewRows([]string{"j"}).AddRow("TRV-2"))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow(same, "Yes")...))
	if _, err := m.repo.Update(context.Background(), preRecord("Ready for Clearance", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &same, StartDate: &start, EndDate: &end, UpdatedBy: "mgr@wso2.com"}); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}

func TestAllocationUpdate_RejectCancelsTravel_PartnerAndCVPToConfirmedDoNot(t *testing.T) {
	m := newAllocationMocks(t)
	rejected, confirmedS, status := "Rejected by Consultant", "Confirmed", "Inactive"
	// Confirmed -> Rejected: cancel.
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectBegin()
	m.travel.ExpectExec(re("UPDATE travel SET status = ? WHERE traveller_email = ? AND allocation_id = ?")).
		WithArgs("Cancelled", "jane@wso2.com", "A0000000001").WillReturnResult(sqlmock.NewResult(0, 1))
	m.travel.ExpectCommit()
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow(rejected, "Yes")...))
	resp, err := m.repo.Update(context.Background(), preRecord("Confirmed", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &rejected, Status: &status, UpdatedBy: "mgr@wso2.com"})
	if err != nil || !resp.TravelCancelled {
		t.Fatalf("reject: %+v, %v", resp, err)
	}

	// Partner: no travel side.
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow(confirmedS, "Yes")...))
	if _, err := m.repo.Update(context.Background(), preRecord("Ready for Clearance", "Yes", "partner-allocations-group+X+A+B@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &confirmedS, UpdatedBy: "mgr@wso2.com"}); err != nil {
		t.Fatal(err)
	}

	// Confirmed - Visa Pending -> Confirmed: neither creates nor cancels.
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()
	m.alloc.ExpectQuery(re("WHERE a.id = ?")).WillReturnRows(sqlmock.NewRows(allocationCols).AddRow(engagementAllocationRow(confirmedS, "Yes")...))
	if _, err := m.repo.Update(context.Background(), preRecord("Confirmed - Visa Pending", "Yes", "jane@wso2.com"),
		domain.AllocationUpdateRequest{ClearanceStatus: &confirmedS, UpdatedBy: "mgr@wso2.com"}); err != nil {
		t.Fatal(err)
	}
	m.verify(t)
}

func TestAllocationDeleteAndVisas(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectBegin()
	m.alloc.ExpectExec(re("UPDATE allocation SET status = 'Deleted' WHERE id = ?")).WithArgs("A1").WillReturnResult(sqlmock.NewResult(0, 1))
	m.alloc.ExpectCommit()
	if err := m.repo.Delete(context.Background(), "A1"); err != nil {
		t.Fatal(err)
	}
	m.travel.ExpectQuery(re("WHERE email = ? AND end_date >= ? AND start_date <= ? LIMIT 100 OFFSET 0")).
		WithArgs("jane@wso2.com", "2026-11-20", "2026-11-10").
		WillReturnRows(sqlmock.NewRows([]string{"c", "t", "e", "s", "sd", "ed"}).
			AddRow("India", "Business", "Multiple", "Approved", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "2027-01-01 00:00:00"))
	v, err := m.repo.Visas(context.Background(), "jane@wso2.com", "2026-11-10", "2026-11-20")
	if err != nil || len(v) != 1 || *v[0].EndDate != "2027-01-01" {
		t.Errorf("visas = %+v, %v", v, err)
	}
	m.verify(t)
}

func TestConsultantAllocations_LegacyFiltersAndFundingSources(t *testing.T) {
	m := newAllocationMocks(t)
	f := domain.ConsultantAllocationFilter{StartDate: "2026-11-01", EndDate: "2026-11-30", Email: "jane@wso2.com",
		AllocationTypeIDs: []int{12}, BusinessUnit: "PARTNER", Department: "D", Team: "T"}
	m.alloc.ExpectQuery(re("WHERE allocation.status = 'Active'")+".*"+re("AND allocation.clearance_status = 'Confirmed'")+".*"+
		re("AND employee.BusinessUnit = ? AND employee.Department = ? AND employee.Team = ? AND allocation.email IN (?) AND allocation.allocation_type IN (?) ORDER BY id")).
		WithArgs("2026-11-30", "2026-11-01", "2026-11-01", "2026-11-30", "2026-11-01", "2026-11-30", "2026-11-01", "2026-11-30",
			"PARTNER", "D", "T", "jane@wso2.com", 12).
		WillReturnRows(sqlmock.NewRows(make([]string, 22)).
			AddRow("A1", "jane@wso2.com", "Consulting", "2026-10-01 00:00:00", "2026-11-10 00:00:00", "E07201", "No", nil, nil, nil,
				"NWB-CO", "On-site", "Sri Lanka", "NWB-CO", "CS-GEN", "Paid Post Sale", 40, "D", "T", "BU", "LK", "WSO2 Lanka").
			AddRow("A2", "jane@wso2.com", "Digital", "2026-11-11 00:00:00", "2026-11-30 00:00:00", "-1", "Yes", "09:00:00", "11:00:00", "Asia/Colombo",
				nil, nil, nil, "DIG-GEN", "DIG-GEN", "Internal Engagement", 19, "D", "T", "BU", "LK", "WSO2 Lanka"))
	m.alloc.ExpectQuery(re("WHERE eng_fs.engagement_id IN (?)")).WithArgs("E07201").
		WillReturnRows(sqlmock.NewRows([]string{"e", "b", "p", "s", "r", "pct"}).AddRow("E07201", "APIM", "PU", "SR", "R", 100.0))

	rows, err := m.repo.ConsultantAllocations(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[0].FundingSources) != 1 || len(rows[1].FundingSources) != 0 ||
		rows[0].StartDate != "2026-10-01" || *rows[1].StartTime != "09:00" {
		t.Errorf("rows = %+v", rows)
	}
	m.verify(t)
}

func TestNonAllocatedEmployees_SubtractsAllocated(t *testing.T) {
	m := newAllocationMocks(t)
	f := domain.ConsultantAllocationFilter{StartDate: "2026-11-01", EndDate: "2026-11-30", Department: "PARTNER"}
	m.alloc.ExpectQuery(re("SELECT DISTINCT employee.Work_Email AS workEmail") + ".*" + re("AND employee.Department = ?")).
		WillReturnRows(sqlmock.NewRows([]string{"w"}).AddRow("a@wso2.com").AddRow(nil))
	m.alloc.ExpectQuery(re("FROM employee")+".*"+re("WHERE department != 'PARTNER'")+".*"+re("AND employee.Work_Email != '' ORDER BY employee.Work_Email")).
		WithArgs("2026-11-30", "2026-11-01").
		WillReturnRows(sqlmock.NewRows([]string{"w", "t", "b", "d", "j", "c"}).
			AddRow("a@wso2.com", "T", "B", "D", "Software Engineer", "C").
			AddRow("b@wso2.com", "T", "B", "D", "Software Engineer", "C"))
	got, err := m.repo.NonAllocatedEmployees(context.Background(), f)
	if err != nil || len(got) != 1 || *got[0].WorkEmail != "b@wso2.com" {
		t.Errorf("got %+v, %v", got, err)
	}
	m.verify(t)
}

func TestTeamDefaultsAndEngagementCodes(t *testing.T) {
	m := newAllocationMocks(t)
	m.alloc.ExpectQuery(re("FROM default_allocation_for_team AS alft") + ".*" + re("ORDER BY businessUnit, department, team")).
		WillReturnRows(sqlmock.NewRows(make([]string, 6)).AddRow("BU", "D", "T", "Digital", "Internal Engagement", "DIG-GEN"))
	d, err := m.repo.TeamDefaults(context.Background())
	if err != nil || len(d) != 1 || *d[0].DefaultEngagementCode != "DIG-GEN" {
		t.Errorf("defaults = %+v, %v", d, err)
	}
	m.alloc.ExpectQuery(re("FROM engcode_grouping_gl_codes")).
		WillReturnRows(sqlmock.NewRows(make([]string, 4)).AddRow("DIG-GEN", "G", "Acc", "6000"))
	c, err := m.repo.EngagementCodes(context.Background())
	if err != nil || len(c) != 1 || *c[0].GLCode != "6000" {
		t.Errorf("codes = %+v, %v", c, err)
	}
	m.verify(t)
}
