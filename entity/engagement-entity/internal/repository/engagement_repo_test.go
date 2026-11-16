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
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func engagementMocks(t *testing.T) (sqlmock.Sqlmock, sqlmock.Sqlmock, *engagementRepo) {
	t.Helper()
	alloc, allocMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	travel, travelMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { alloc.Close(); travel.Close() })
	return allocMock, travelMock, &engagementRepo{alloc: alloc, travel: travel}
}

var engagementTestCols = []string{"id", "engagement_type", "eT.name", "allocation_type", "aT.allocation_type",
	"customer_code", "customer_name", "project_name", "opportunity_id", "opportunity_line_item_id",
	"product_name", "location", "country", "business_domain", "username", "contracts_signed",
	"engagement_code", "start", "end", "status", "engagement_scope", "engagement_nature", "engagement_status",
	"customer_contact", "owner", "tentative_dates", "technical_owner", "comment", "description", "updated_by",
	"last_updated_date", "campaign_id"}

func engagementTestRow(id string, engType int, status string) []driver.Value {
	return []driver.Value{id, engType, "Marketing Event", 12, "Conference",
		"001E0000002SFO2IAO", "WSO2", "WSO2Con Asia", "", "",
		"", "Colombo\nSri Lanka", "Sri Lanka", nil, "kevin@wso2.com", 0,
		"MKT-GEN", "2026-11-20", "2026-11-22", status, "scope", "Physical", "Pending",
		nil, nil, 0, nil, nil, "desc", nil,
		time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC), "7015g00000ApD26AAF"}
}

func TestEngagementSearch_FiltersSortAndPage(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	req := domain.EngagementSearchRequest{
		EngagementTypeIDs: []int{5}, Search: "con_", Accounts: []string{"WSO2"}, RequestStatuses: []string{"New", "Updated"},
		BusinessUnits: []string{"Marketing"}, Consultants: "none", StartFrom: "2026-01-01", EndTo: "2026-12-31",
		Today: "2026-11-05", Limit: 10, Offset: 20,
	}
	where := `WHERE e.id != '' AND e.status != 'Pending Approval' AND aT.engagement_type IN (?) AND e.customer_name IN (?)` +
		` AND e.status IN (?, ?)`
	mock.ExpectQuery(re(`SELECT COUNT(*) FROM engagement AS e`)).
		WithArgs(5, "WSO2", "New", "Updated", "Marketing", "2026-01-01", "2026-12-31",
			`%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(21))
	mock.ExpectQuery(re(where)).
		WithArgs(5, "WSO2", "New", "Updated", "Marketing", "2026-01-01", "2026-12-31",
			`%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`, `%con\_%`, "2026-11-05", "2026-11-05", 10, 20).
		WillReturnRows(sqlmock.NewRows(engagementTestCols).AddRow(engagementTestRow("EVT00509", 5, "Approved")...))

	items, total, err := repo.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 21 || len(items) != 1 {
		t.Fatalf("total %d, items %d", total, len(items))
	}
	e := items[0]
	if e.ID != "EVT00509" || e.EngagementTypeID != 5 || *e.Address != "Colombo\nSri Lanka" || e.CampaignID == nil ||
		*e.CreatedOn != "2026-10-01T08:30:00Z" || e.ContractsSigned == nil || *e.ContractsSigned || e.BusinessDomain != nil {
		t.Errorf("got %+v", e)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementSearch_ApprovalsBaseSortAndCreatorSearch(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`WHERE e.id != '' AND e.status = 'Pending Approval' AND (e.id LIKE ? OR e.customer_name LIKE ?` +
		` OR e.project_name LIKE ? OR e.engagement_code LIKE ? OR e.country LIKE ? OR aT.allocation_type LIKE ? OR e.username LIKE ?)`)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(re(`ORDER BY e.last_updated_date ASC, e.id ASC LIMIT ? OFFSET ?`)).
		WillReturnRows(sqlmock.NewRows(engagementTestCols))
	items, _, err := repo.Search(context.Background(), domain.EngagementSearchRequest{
		PendingApproval: true, Search: "kevin", Sort: domain.EngagementSortCreated, Limit: 50,
	})
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("items %v, err %v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementTypeCountsAndFacets(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`WHERE e.id != '' AND e.status != 'Pending Approval' GROUP BY aT.engagement_type`)).
		WillReturnRows(sqlmock.NewRows([]string{"t", "n"}).AddRow(1, 4).AddRow(5, 2).AddRow(nil, 9))
	counts, err := repo.TypeCounts(context.Background(), false)
	if err != nil || counts[1] != 4 || counts[5] != 2 || len(counts) != 2 {
		t.Fatalf("counts %v, err %v", counts, err)
	}
	for _, col := range []string{"e.customer_name", "aT.allocation_type", "e.country"} {
		mock.ExpectQuery(re(`SELECT DISTINCT `+col)).WithArgs(3, 4).
			WillReturnRows(sqlmock.NewRows([]string{"v"}).AddRow("x"))
	}
	f, err := repo.Facets(context.Background(), []int{3, 4})
	if err != nil || len(f.Accounts) != 1 || len(f.AllocationTypes) != 1 || len(f.Countries) != 1 {
		t.Fatalf("facets %+v, err %v", f, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementGet_NotFound(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`WHERE e.id = ?`)).WithArgs("E1").WillReturnRows(sqlmock.NewRows(engagementTestCols))
	if _, err := repo.Get(context.Background(), "E1"); !errors.Is(err, ErrEngagementNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestEngagementFundingSourcesAndConsultants(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`FROM engagement_funding_source AS eng_fs`)).WithArgs("E1", "E2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "bu", "pu", "sub", "rg", "pct"}).
			AddRow("E1", "Marketing", "Brand", "LK", "APAC", 60.0).AddRow("E1", "R&D", "APIM", "LK", "APAC", 40.0))
	fs, err := repo.FundingSources(context.Background(), []string{"E1", "E2"})
	if err != nil || len(fs["E1"]) != 2 || fs["E1"][1].Percentage != 40 || fs["E1"][0].Region != "APAC" {
		t.Fatalf("fs %+v err %v", fs, err)
	}
	mock.ExpectQuery(re(`SELECT DISTINCT engagement_id, email FROM allocation WHERE status = 'Active' AND engagement_id IN (?)`)).
		WithArgs("E1").WillReturnRows(sqlmock.NewRows([]string{"e", "m"}).AddRow("E1", "a@wso2.com"))
	cons, err := repo.Consultants(context.Background(), []string{"E1"})
	if err != nil || len(cons["E1"]) != 1 {
		t.Fatalf("cons %v err %v", cons, err)
	}
	// Empty id lists read nothing (legacy read the whole table).
	if fs, _ := repo.FundingSources(context.Background(), nil); len(fs) != 0 {
		t.Error("funding sources for no ids")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementCreate_InsertRereadFundingAndHook(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`SELECT engagement_type FROM allocation_type WHERE id = ?`)).WithArgs(12).
		WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow(5))
	mock.ExpectBegin()
	mock.ExpectExec(re(`INSERT INTO engagement (`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re(`SELECT id FROM engagement WHERE uuid = ?`)).WithArgs("u-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("EVT00510"))
	mock.ExpectExec(re(`INSERT INTO engagement_funding_source`)).
		WithArgs("EVT00510", 100.0, "Marketing", "Brand", "APAC", "LK").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	var hookID string
	var hookType int
	id, err := repo.Create(context.Background(), domain.EngagementCreateRequest{
		AllocationTypeID: 12, UUID: "u-1", ProjectName: "WSO2Con",
		FundingSources: []domain.EngagementFundingSource{{BusinessUnit: "Marketing", ProductUnit: "Brand", Region: "APAC", SubRegion: "LK", Percentage: 100}},
	}, func(_ context.Context, id string, t int) error { hookID, hookType = id, t; return nil })
	if err != nil || id != "EVT00510" || hookID != "EVT00510" || hookType != 5 {
		t.Fatalf("id %q hook %q/%d err %v", id, hookID, hookType, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A NetSuite failure inside the transaction rolls the insert back.
func TestEngagementCreate_HookFailureRollsBack(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`SELECT engagement_type FROM allocation_type`)).WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow(5))
	mock.ExpectBegin()
	mock.ExpectExec(re(`INSERT INTO engagement (`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re(`SELECT id FROM engagement WHERE uuid = ?`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("EVT1"))
	mock.ExpectRollback()
	boom := errors.New("netsuite down")
	_, err := repo.Create(context.Background(), domain.EngagementCreateRequest{AllocationTypeID: 1, UUID: "u"},
		func(context.Context, string, int) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementCreate_UnknownAllocationType(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`SELECT engagement_type FROM allocation_type`)).WillReturnRows(sqlmock.NewRows([]string{"t"}))
	if _, err := repo.Create(context.Background(), domain.EngagementCreateRequest{AllocationTypeID: 9}, nil); !errors.Is(err, ErrEngagementAllocationTypeNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func engagementPtr[T any](v T) *T { return &v }

func TestEngagementUpdate_DynamicSetAndFundingReplace(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectBegin()
	mock.ExpectExec(re(`UPDATE engagement SET updated_by = ?, location = ?, start_date = ?, status = ?, project_name = ? WHERE id = ?`)).
		WithArgs("me@wso2.com", "addr", "2026-12-01", "Updated", "New name", "E1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re(`DELETE FROM engagement_funding_source WHERE engagement_id = ?`)).WithArgs("E1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(re(`INSERT INTO engagement_funding_source`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	before, err := repo.Update(context.Background(), "E1", domain.EngagementUpdateRequest{
		UpdatedBy: "me@wso2.com", Address: engagementPtr("addr"), StartDate: engagementPtr("2026-12-01"),
		RequestStatus: engagementPtr("Updated"), ProjectName: engagementPtr("New name"),
		FundingSources: []domain.EngagementFundingSource{{BusinessUnit: "a", ProductUnit: "b", Region: "c", SubRegion: "d", Percentage: 100}},
	})
	if err != nil || len(before) != 0 || before == nil {
		t.Fatalf("before %v, err %v", before, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementUpdate_NoRowIsNotFound(t *testing.T) {
	mock, _, repo := engagementMocks(t)
	mock.ExpectBegin()
	mock.ExpectExec(re(`UPDATE engagement SET updated_by = ? WHERE id = ?`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := repo.Update(context.Background(), "E9", domain.EngagementUpdateRequest{UpdatedBy: "x"}); !errors.Is(err, ErrEngagementNotFound) {
		t.Fatalf("err = %v", err)
	}
}

var engagementAllocCols = []string{"id", "email", "allocation_type", "start", "end", "comment", "clearance", "travel",
	"added_by", "added_date", "recurring", "start_time", "end_time", "tz", "status"}

// Cancel: pre-read Active allocations, cancel every allocation row, then
// cancel the travel of Confirmed / Confirmed - Visa Pending allocations with
// travel required, before the allocation_db commit.
func TestEngagementUpdate_CancelCascade(t *testing.T) {
	allocMock, travelMock, repo := engagementMocks(t)
	allocMock.ExpectQuery(re(`WHERE a.engagement_id = ? AND a.status = 'Active' ORDER BY a.id`)).WithArgs("E1").
		WillReturnRows(sqlmock.NewRows(engagementAllocCols).
			AddRow("A1", "a@wso2.com", 3, "2026-11-01", "2026-11-05", nil, "Confirmed", "Yes", "x", nil, "No", nil, nil, nil, "Active").
			AddRow("A2", "b@wso2.com", 3, "2026-11-01", "2026-11-05", nil, "Tentative", "Yes", "x", nil, "No", nil, nil, nil, "Active").
			AddRow("A3", "c@wso2.com", 3, "2026-11-01", "2026-11-05", nil, "Confirmed - Visa Pending", nil, "x", nil, nil, nil, nil, nil, "Active").
			AddRow("A4", "d@wso2.com", 3, "2026-11-01", "2026-11-05", nil, "Confirmed - Visa Pending", "Yes", "x", nil, nil, nil, nil, nil, "Active"))
	allocMock.ExpectBegin()
	allocMock.ExpectExec(re(`UPDATE engagement SET updated_by = ?, engagement_status = ?, status = ? WHERE id = ?`)).
		WithArgs("me@wso2.com", "Cancelled", "Cancelled", "E1").WillReturnResult(sqlmock.NewResult(0, 1))
	allocMock.ExpectExec(re(`UPDATE allocation SET updated_by = ?, clearance_status = ?, status = ? WHERE engagement_id = ?`)).
		WithArgs("me@wso2.com", "Allocation Cancelled", "Inactive", "E1").WillReturnResult(sqlmock.NewResult(0, 6))
	travelMock.ExpectBegin()
	for _, a := range [][2]string{{"A1", "a@wso2.com"}, {"A4", "d@wso2.com"}} {
		travelMock.ExpectExec(re(`UPDATE travel SET status = ?, last_updated_by = ?, last_updated_date = NOW() WHERE allocation_id = ? AND traveller_email = ?`)).
			WithArgs("Cancelled", "me@wso2.com", a[0], a[1]).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	travelMock.ExpectCommit()
	allocMock.ExpectCommit()

	before, err := repo.Update(context.Background(), "E1", domain.EngagementUpdateRequest{
		UpdatedBy: "me@wso2.com", EngagementStatus: engagementPtr("Cancelled"), RequestStatus: engagementPtr("Cancelled"),
	})
	if err != nil || len(before) != 4 {
		t.Fatalf("before %d, err %v", len(before), err)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A travel failure rolls back the allocation_db changes.
func TestEngagementUpdate_CancelTravelFailureRollsBack(t *testing.T) {
	allocMock, travelMock, repo := engagementMocks(t)
	allocMock.ExpectQuery(re(`a.status = 'Active'`)).WillReturnRows(sqlmock.NewRows(engagementAllocCols).
		AddRow("A1", "a@wso2.com", 3, "2026-11-01", "2026-11-05", nil, "Confirmed", "Yes", "x", nil, "No", nil, nil, nil, "Active"))
	allocMock.ExpectBegin()
	allocMock.ExpectExec(re(`UPDATE engagement SET`)).WillReturnResult(sqlmock.NewResult(0, 1))
	allocMock.ExpectExec(re(`UPDATE allocation SET`)).WillReturnResult(sqlmock.NewResult(0, 1))
	travelMock.ExpectBegin()
	travelMock.ExpectExec(re(`UPDATE travel SET`)).WillReturnError(errors.New("travel down"))
	travelMock.ExpectRollback()
	allocMock.ExpectRollback()
	if _, err := repo.Update(context.Background(), "E1", domain.EngagementUpdateRequest{
		UpdatedBy: "x", EngagementStatus: engagementPtr("Cancelled"),
	}); err == nil {
		t.Fatal("want error")
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestEngagementAllocations_JoinsTravelByAllocationAndEmail(t *testing.T) {
	allocMock, travelMock, repo := engagementMocks(t)
	added := time.Date(2026, 10, 4, 16, 20, 0, 0, time.UTC)
	allocMock.ExpectQuery(re(`FROM allocation AS a WHERE a.engagement_id = ? ORDER BY a.id`)).WithArgs("E1").
		WillReturnRows(sqlmock.NewRows(engagementAllocCols).
			AddRow("A1", "A@wso2.com", 3, "2026-11-01", "2026-11-05", "c", "Confirmed", "Yes", "x", added, "Yes", "09:00:00", "11:00:00", "Asia/Colombo", "Active").
			AddRow("A2", "b@wso2.com", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	travelMock.ExpectQuery(re(`FROM travel WHERE allocation_id IN (?, ?) ORDER BY job_number`)).WithArgs("A1", "A2").
		WillReturnRows(sqlmock.NewRows([]string{"job", "alloc", "email", "status"}).
			AddRow("TRV-2026-10-1246", "A1", "a@wso2.com", "New").
			AddRow("TRV-2026-10-1300", "A2", "someone@wso2.com", "New"))
	allocs, err := repo.Allocations(context.Background(), "E1", false)
	if err != nil || len(allocs) != 2 {
		t.Fatalf("allocs %v err %v", allocs, err)
	}
	if len(allocs[0].TravelRequests) != 1 || allocs[0].TravelRequests[0].JobNumber != "TRV-2026-10-1246" ||
		*allocs[0].AddedDate != "2026-10-04T16:20:00Z" || *allocs[0].AllocationTypeID != 3 {
		t.Errorf("A1 = %+v", allocs[0])
	}
	if len(allocs[1].TravelRequests) != 0 || allocs[1].TravelRequests == nil || allocs[1].AllocationTypeID != nil {
		t.Errorf("A2 = %+v", allocs[1])
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Before the campaign_id migration runs: reads select NULL, inserts leave the
// column out, and saving a campaign is refused.
func TestEngagement_WithoutCampaignColumn(t *testing.T) {
	SetEngagementCampaignColumn(false)
	t.Cleanup(func() { SetEngagementCampaignColumn(true) })
	if !strings.Contains(engagementSelect(), "e.last_updated_date, NULL") {
		t.Errorf("select = %s", engagementSelect())
	}
	mock, _, repo := engagementMocks(t)
	mock.ExpectQuery(re(`SELECT engagement_type FROM allocation_type`)).WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO engagement \(.*description\)`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(re(`SELECT id FROM engagement WHERE uuid = ?`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("EL1"))
	mock.ExpectCommit()
	if _, err := repo.Create(context.Background(), domain.EngagementCreateRequest{AllocationTypeID: 1, UUID: "u"}, nil); err != nil {
		t.Fatal(err)
	}
	campaign := "7015g00000ApD26AAF"
	mock.ExpectQuery(re(`SELECT engagement_type FROM allocation_type`)).WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow(5))
	if _, err := repo.Create(context.Background(), domain.EngagementCreateRequest{AllocationTypeID: 5, UUID: "v", CampaignID: &campaign}, nil); err == nil {
		t.Error("saved a campaign without the column")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// tentative_dates is BIT(1): read as a 0/1 number, not the raw bit.
func TestEngagementSelectReadsTentativeDatesAsNumber(t *testing.T) {
	if !strings.Contains(engagementSelect(), "COALESCE(e.tentative_dates = 1, 0) AS tentative_dates") {
		t.Errorf("select = %s", engagementSelect())
	}
}

// Legacy addresses encode line breaks as "BR()"; reads decode them.
func TestAddressReadsDecodeLegacyLineBreaks(t *testing.T) {
	const decode = "REPLACE(e.location, 'BR()', CHAR(10))"
	if !strings.Contains(engagementSelect(), decode) || !strings.Contains(allocationSelect, decode) ||
		!strings.Contains(travelRequestColumns, "REPLACE(t.city, 'BR()', CHAR(10))") {
		t.Error("an address read does not decode BR()")
	}
}
