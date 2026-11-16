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
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

func travelRe(s string) string { return regexp.QuoteMeta(s) }

func travelNewDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func travelCheck(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

var travelRowColumns = []string{
	"job_number", "status", "traveller_email", "city", "country", "customer_name", "start_date", "end_date",
	"allocation_id", "engagement_id", "engagement_code", "engagement_type", "travel_agent_id",
	"name_of_travel_agent", "visa_id", "visa", "v.email", "v.country", "v.visa_type", "v.entry_type",
	"v.start_date", "v.end_date", "last_updated_by", "last_updated_date",
}

func travelDay(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func travelSampleRow() *sqlmock.Rows {
	return sqlmock.NewRows(travelRowColumns).AddRow(
		"TRV-2026-10-1246", "In Progress", "chanuka@wso2.com", "London", "United Kingdom", "Acme",
		travelDay("2026-10-06"), travelDay("2026-10-16"), "A2609100005", "E07201", "ACME-01", "Paid",
		int64(2), "Acorn Travels", int64(301), nil, "chanuka@wso2.com", "GB", "Business/Visit", "Multiple",
		travelDay("2026-01-10"), travelDay("2028-01-09"), "shivanga@wso2.com", travelDay("2026-10-01"))
}

func TestTravelSearch_FiltersSearchSortAndPaging(t *testing.T) {
	db, mock := travelNewDB(t)
	agency := 2
	f := domain.TravelRequestSearch{
		Statuses: []string{"In Progress", "Extended"}, TravelAgentID: &agency,
		TravellerEmails: []string{"chanuka@wso2.com"}, Countries: []string{"United Kingdom"},
		AgencyNames: []string{"Acorn Travels"}, StartFrom: "2026-10-01", EndTo: "2026-10-31",
		Search: "lon_", SearchEngagements: true, SearchAgency: true, EngagementIDs: []string{"E07201"},
		Today: "2026-10-05", Limit: 10, Offset: 20,
	}
	where := ` WHERE t.status NOT IN ('Deleted', '') AND t.status IN (?, ?) AND t.travel_agent_id = ? AND t.traveller_email IN (?)` +
		` AND t.country IN (?) AND ta.name_of_travel_agent IN (?) AND DATE(t.start_date) >= ? AND DATE(t.end_date) <= ?` +
		` AND (t.job_number LIKE ? OR t.traveller_email LIKE ? OR t.city LIKE ? OR t.country LIKE ?` +
		` OR t.customer_name LIKE ? OR t.engagement_id IN (?) OR ta.name_of_travel_agent LIKE ?)`
	like := `%lon\_%`
	args := []any{"In Progress", "Extended", 2, "chanuka@wso2.com", "United Kingdom", "Acorn Travels",
		"2026-10-01", "2026-10-31", like, like, like, like, like, "E07201", like}
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM travel AS t`) + `.*` + travelRe(where)).
		WithArgs(travelArgs(args)...).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(31))
	all := append(append([]any{}, args...), "2026-10-05", "2026-10-05", 10, 20)
	mock.ExpectQuery(travelRe(where) + `\s+ORDER BY CASE WHEN DATE\(t.end_date\) >= \? THEN 0 ELSE 1 END.*LIMIT \? OFFSET \?`).
		WithArgs(travelArgs(all)...).WillReturnRows(travelSampleRow())

	items, total, err := NewTravelRequestRepository(db).Search(context.Background(), f)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 31 || len(items) != 1 {
		t.Fatalf("total %d items %d", total, len(items))
	}
	got := items[0]
	if got.StartDate != "2026-10-06" || got.EndDate != "2026-10-16" || *got.TravelAgentID != 2 ||
		*got.TravelAgentName != "Acorn Travels" || got.Visa == nil || *got.Visa.VisaType != "Business/Visit" ||
		*got.Visa.EndDate != "2028-01-09" || *got.LastUpdatedDate != "2026-10-01 00:00:00" || got.VisaText != nil {
		t.Errorf("row = %+v", got)
	}
	travelCheck(t, mock)
}

func travelArgs(vals []any) []driver.Value {
	out := make([]driver.Value, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

func TestTravelSearch_NoMatchesSkipsTheRowQuery(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM travel AS t`)).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	items, total, err := NewTravelRequestRepository(db).Search(context.Background(),
		domain.TravelRequestSearch{Sort: domain.TravelSortStartDesc, Limit: 10})
	if err != nil || total != 0 || items == nil || len(items) != 0 {
		t.Fatalf("items %v total %d err %v", items, total, err)
	}
	travelCheck(t, mock)
}

func TestTravelSearch_StartDescOrder(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery(travelRe(`WHERE t.status NOT IN ('Deleted', '') AND t.traveller_email IN (?) ORDER BY t.start_date DESC, t.job_number LIMIT ? OFFSET ?`)).
		WithArgs("me@wso2.com", 50, 0).WillReturnRows(travelSampleRow())
	if _, _, err := NewTravelRequestRepository(db).Search(context.Background(), domain.TravelRequestSearch{
		TravellerEmails: []string{"me@wso2.com"}, Sort: domain.TravelSortStartDesc, Limit: 50,
	}); err != nil {
		t.Fatal(err)
	}
	travelCheck(t, mock)
}

func TestTravelStatusCounts_IgnoresStatuses(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`SELECT COALESCE(t.status, ''), COUNT(*)`) + `.*` +
		travelRe(`WHERE t.status NOT IN ('Deleted', '') AND t.country IN (?) GROUP BY t.status`)).
		WithArgs("Sri Lanka").
		WillReturnRows(sqlmock.NewRows([]string{"status", "n"}).AddRow("New", 3).AddRow("In Progress", 5))
	got, err := NewTravelRequestRepository(db).StatusCounts(context.Background(), domain.TravelRequestSearch{
		Statuses: []string{"New"}, Countries: []string{"Sri Lanka"},
	})
	if err != nil || got["New"] != 3 || got["In Progress"] != 5 {
		t.Fatalf("got %v err %v", got, err)
	}
	travelCheck(t, mock)
}

func TestTravelGet(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`WHERE t.job_number = ? LIMIT 1`)).WithArgs("TRV-2026-10-1246").WillReturnRows(travelSampleRow())
	mock.ExpectQuery(travelRe(`WHERE t.job_number = ? LIMIT 1`)).WithArgs("TRV-2026-10-9999").
		WillReturnRows(sqlmock.NewRows(travelRowColumns))
	repo := NewTravelRequestRepository(db)
	got, err := repo.Get(context.Background(), "TRV-2026-10-1246")
	if err != nil || got == nil || got.JobNumber != "TRV-2026-10-1246" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got, err := repo.Get(context.Background(), "TRV-2026-10-9999"); err != nil || got != nil {
		t.Fatalf("missing: got %+v err %v", got, err)
	}
	travelCheck(t, mock)
}

func TestTravelUpdate_SetsOnlyGivenFields(t *testing.T) {
	db, mock := travelNewDB(t)
	status, agent, visa := "In Progress", 2, 301
	mock.ExpectBegin()
	mock.ExpectExec(travelRe(`UPDATE travel SET last_updated_by = ?, last_updated_date = NOW(), status = ?, travel_agent_id = ?, visa_id = ? WHERE job_number = ?`)).
		WithArgs("shivanga@wso2.com", status, agent, visa, "TRV-2026-10-1246").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(travelRe(`UPDATE travel SET last_updated_by = ?, last_updated_date = NOW(), status = ? WHERE job_number = ?`)).
		WithArgs("shivanga@wso2.com", status, "TRV-2026-10-0000").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	repo := NewTravelRequestRepository(db)
	found, err := repo.Update(context.Background(), "TRV-2026-10-1246", domain.UpdateTravelRequestInput{
		Status: &status, TravelAgentID: &agent, VisaID: &visa, UpdatedBy: "shivanga@wso2.com",
	})
	if err != nil || !found {
		t.Fatalf("found %v err %v", found, err)
	}
	found, err = repo.Update(context.Background(), "TRV-2026-10-0000", domain.UpdateTravelRequestInput{
		Status: &status, UpdatedBy: "shivanga@wso2.com",
	})
	if err != nil || found {
		t.Fatalf("missing: found %v err %v", found, err)
	}
	travelCheck(t, mock)
}

func TestTravelAgents(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`SELECT id, name_of_travel_agent, status FROM travel_agents WHERE status = ? ORDER BY name_of_travel_agent`)).
		WithArgs("Active").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status"}).AddRow(1, "Acorn Travels", "Active"))
	mock.ExpectQuery(travelRe(`FROM travel_agents ORDER BY name_of_travel_agent`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status"}))
	repo := NewTravelRequestRepository(db)
	got, err := repo.Agents(context.Background(), "Active")
	if err != nil || len(got) != 1 || got[0].Name != "Acorn Travels" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got, err := repo.Agents(context.Background(), ""); err != nil || got == nil {
		t.Fatalf("all: got %v err %v", got, err)
	}
	travelCheck(t, mock)
}

func TestTravelActivePerDiems(t *testing.T) {
	db, mock := travelNewDB(t)
	start := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	mock.ExpectQuery(travelRe(`FROM perdiem_allocation
		WHERE job_number = ? AND status = 'Active'`)).WithArgs("TRV-2026-10-1246").
		WillReturnRows(sqlmock.NewRows([]string{"start_date", "end_date", "no_of_days", "currency", "perdiem_value", "total"}).
			AddRow(start, nil, 11, "GBP", 70.0, 770.0))
	got, err := NewTravelRequestRepository(db).ActivePerDiems(context.Background(), "TRV-2026-10-1246")
	if err != nil || len(got) != 1 || *got[0].StartDate != "2026-10-06 09:00:00" || got[0].EndDate != nil ||
		*got[0].NoOfDays != 11 || *got[0].Total != 770 {
		t.Fatalf("got %+v err %v", got, err)
	}
	travelCheck(t, mock)
}

func TestTravelInsertEmailAudit_CutsToColumnSizes(t *testing.T) {
	db, mock := travelNewDB(t)
	long := strings.Repeat("a", 300)
	mock.ExpectExec(travelRe(`INSERT INTO email_auditlog`)).
		WithArgs("subj", "noContent", "{}", strings.Repeat("a", 255), "").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewTravelRequestRepository(db).InsertEmailAudit(context.Background(), domain.TravelEmailAuditInput{
		Subject: "subj", TemplateID: "noContent", ContentKeyValPairs: "{}", Recipients: long,
	}); err != nil {
		t.Fatal(err)
	}
	travelCheck(t, mock)
}

func TestTravelEngagements(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`WHERE e.status != 'Deleted' AND a.id IN (?, ?)`)).WithArgs("A1", "A2").
		WillReturnRows(sqlmock.NewRows([]string{"a.id", "e.id", "project_name", "customer_name", "engagement_code",
			"campaign_id", "eT.id", "eT.name", "eT.type"}).
			AddRow("A1", "EVT00012", "Integration Summit Singapore 2026", "WSO2", "MKT-01", "7015g00000CkR08AAF",
				5, "Marketing Event", "Internal"))
	mock.ExpectQuery(travelRe(`SELECT id FROM engagement
		WHERE status != 'Deleted' AND project_name LIKE ?`)).WithArgs("%Summit%", 1000).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("EVT00012"))
	mock.ExpectQuery(travelRe(`WHERE eng_fs.engagement_id = ?`)).WithArgs("EVT00012").
		WillReturnRows(sqlmock.NewRows([]string{"bu", "pu", "region", "sub", "pct"}).
			AddRow("Integration", "APIM", "APAC", "SEA", 60.0).AddRow("Integration", nil, "EU", "UK", nil))
	repo := NewTravelEngagementRepository(db)
	ctx := context.Background()
	got, err := repo.ByAllocation(ctx, []string{"A1", "A2"})
	if err != nil || len(got) != 1 || *got["A1"].CampaignID != "7015g00000CkR08AAF" || *got["A1"].TypeName != "Marketing Event" || *got["A1"].TypeID != 5 {
		t.Fatalf("got %+v err %v", got, err)
	}
	ids, err := repo.IDsByTitle(ctx, "Summit")
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids %v err %v", ids, err)
	}
	fs, err := repo.FundingSources(ctx, "EVT00012")
	if err != nil || len(fs) != 2 || *fs[0].Percentage != 60 || fs[1].Percentage != nil || fs[1].ProductUnit != nil {
		t.Fatalf("funding %+v err %v", fs, err)
	}
	travelCheck(t, mock)
}

func TestTravelEngagements_ChunksLongIDLists(t *testing.T) {
	db, mock := travelNewDB(t)
	ids := make([]string, travelInChunk+1)
	for i := range ids {
		ids[i] = "A" + strings.Repeat("1", i%5+1)
	}
	cols := []string{"a.id", "e.id", "project_name", "customer_name", "engagement_code", "campaign_id", "eT.name", "eT.type"}
	mock.ExpectQuery(`a.id IN \(` + strings.Repeat(`\?, `, travelInChunk-1) + `\?\)`).WillReturnRows(sqlmock.NewRows(cols))
	mock.ExpectQuery(travelRe(`a.id IN (?)`)).WillReturnRows(sqlmock.NewRows(cols))
	if _, err := NewTravelEngagementRepository(db).ByAllocation(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	travelCheck(t, mock)
}
