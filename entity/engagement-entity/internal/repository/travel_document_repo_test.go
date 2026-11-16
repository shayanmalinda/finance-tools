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
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

var travelVisaColumns = []string{"id", "email", "country", "visa_type", "entry_type", "start_date", "end_date",
	"issued_date", "status", "image_id", "passport_id", "p.issued_country", "p.document_no", "p.issued_date",
	"p.expiry_date", "added_by", "updated_by"}

func travelVisaRow() *sqlmock.Rows {
	return sqlmock.NewRows(travelVisaColumns).AddRow(301, "chanuka@wso2.com", "GB", "Business/Visit", "Multiple",
		travelDay("2026-01-10"), travelDay("2028-01-09"), travelDay("2026-01-05"), "Active", "chanuka@wso2.com_x_GB.png",
		201, "LK", "N1234913", travelDay("2022-05-11"), travelDay("2032-05-10"), "chanuka@wso2.com", nil)
}

var travelDocColumns = []string{"id", "email", "document_no", "document_type", "issued_country", "issued_date",
	"expiry_date", "status", "image_id", "added_by", "updated_by"}

func TestTravelSearchVisas_FiltersAndOrder(t *testing.T) {
	db, mock := travelNewDB(t)
	where := ` WHERE v.email IN (?) AND v.country IN (?, ?) AND v.id NOT IN (?, ?) AND v.status = ? AND DATE(v.end_date) >= ?` +
		` AND DATE(v.start_date) <= ? AND DATE(v.end_date) >= ?` +
		` AND (v.email LIKE ? OR v.country LIKE ? OR v.visa_type LIKE ? OR v.entry_type LIKE ?)`
	args := []any{"chanuka@wso2.com", "GB", "LK", 332, 333, "Active", "2026-10-05", "2026-10-06", "2026-10-16",
		"%bus%", "%bus%", "%bus%", "%bus%"}
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM visa_status AS v` + where)).WithArgs(travelArgs(args)...).
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery(travelRe(`LEFT JOIN employee_travel_document AS p ON p.id = v.passport_id` + where +
		` ORDER BY v.end_date ASC, v.id LIMIT ? OFFSET ?`)).
		WithArgs(travelArgs(append(args, 10, 0))...).WillReturnRows(travelVisaRow())
	items, total, err := NewTravelDocumentRepository(db).SearchVisas(context.Background(), domain.TravelVisaFilter{
		Emails: []string{"chanuka@wso2.com"}, Countries: []string{"GB", "LK"}, Status: "Active",
		Validity: domain.TravelValidityActive, Today: "2026-10-05", CoversFrom: "2026-10-06", CoversTo: "2026-10-16",
		Search: "bus", ExcludeIDs: []int{332, 333}, Limit: 10,
	})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("items %d total %d err %v", len(items), total, err)
	}
	v := items[0]
	if *v.StartDate != "2026-01-10" || v.Passport == nil || *v.Passport.DocumentNo != "N1234913" ||
		*v.Passport.ExpiryDate != "2032-05-10" || v.UpdatedBy != nil {
		t.Errorf("visa = %+v", v)
	}
	travelCheck(t, mock)
}

func TestTravelSearchVisas_ExpiredAndNoFilter(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM visa_status AS v WHERE DATE(v.end_date) < ?`)).
		WithArgs("2026-10-05").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM visa_status AS v`)).WithoutArgs().
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(0))
	repo := NewTravelDocumentRepository(db)
	if _, _, err := repo.SearchVisas(context.Background(), domain.TravelVisaFilter{
		Validity: domain.TravelValidityExpired, Today: "2026-10-05", Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if items, _, err := repo.SearchVisas(context.Background(), domain.TravelVisaFilter{Limit: 10}); err != nil || items == nil {
		t.Fatalf("items %v err %v", items, err)
	}
	travelCheck(t, mock)
}

func TestTravelCreateVisa_InactivatesThenInsertsInOneTx(t *testing.T) {
	db, mock := travelNewDB(t)
	passport := 201
	mock.ExpectBegin()
	mock.ExpectExec(travelRe(`UPDATE visa_status SET status = 'Inactive', updated_by = ?, updated_date = NOW()
			WHERE email = ? AND country = ? AND status = 'Active'`)).
		WithArgs("shivanga@wso2.com", "chanuka@wso2.com", "GB").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(travelRe(`INSERT INTO visa_status`)).
		WithArgs("chanuka@wso2.com", "GB", "Business/Visit", "2026-01-10", "2028-01-09", "shivanga@wso2.com",
			"Multiple", "2026-01-05", "img.png", &passport).
		WillReturnResult(sqlmock.NewResult(302, 1))
	mock.ExpectCommit()
	id, err := NewTravelDocumentRepository(db).CreateVisa(context.Background(), domain.CreateTravelVisaInput{
		Email: "chanuka@wso2.com", Country: "GB", VisaType: "Business/Visit", EntryType: "Multiple",
		IssuedDate: "2026-01-05", StartDate: "2026-01-10", EndDate: "2028-01-09", PassportID: &passport,
		ImageID: "img.png", CreatedBy: "shivanga@wso2.com",
	})
	if err != nil || id != 302 {
		t.Fatalf("id %d err %v", id, err)
	}
	travelCheck(t, mock)
}

func TestTravelCreateVisa_RollsBackOnInsertFailure(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE visa_status`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO visa_status`).WillReturnError(errors.New("boom"))
	mock.ExpectRollback()
	if _, err := NewTravelDocumentRepository(db).CreateVisa(context.Background(), domain.CreateTravelVisaInput{}); err == nil {
		t.Fatal("want error")
	}
	travelCheck(t, mock)
}

func TestTravelUpdateVisa(t *testing.T) {
	in := domain.UpdateTravelVisaInput{Country: "GB", VisaType: "Work Permit", EntryType: "Single",
		IssuedDate: "2026-01-05", StartDate: "2026-01-10", EndDate: "2027-01-09", Status: "Active", UpdatedBy: "admin@wso2.com"}

	t.Run("not found", func(t *testing.T) {
		db, mock := travelNewDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(travelRe(`SELECT email FROM visa_status WHERE id = ?`)).WithArgs(9).
			WillReturnRows(sqlmock.NewRows([]string{"email"}))
		mock.ExpectRollback()
		if err := NewTravelDocumentRepository(db).UpdateVisa(context.Background(), 9, in); !errors.Is(err, ErrTravelRecordNotFound) {
			t.Fatalf("err = %v", err)
		}
		travelCheck(t, mock)
	})
	t.Run("another active visa", func(t *testing.T) {
		db, mock := travelNewDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT email FROM visa_status`).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("c@wso2.com"))
		mock.ExpectQuery(travelRe(`WHERE email = ? AND country = ? AND status = 'Active' AND id <> ? LIMIT 1`)).
			WithArgs("c@wso2.com", "GB", 301).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(300))
		mock.ExpectRollback()
		if err := NewTravelDocumentRepository(db).UpdateVisa(context.Background(), 301, in); !errors.Is(err, ErrTravelDuplicateActive) {
			t.Fatalf("err = %v", err)
		}
		travelCheck(t, mock)
	})
	t.Run("updates with image and passport", func(t *testing.T) {
		db, mock := travelNewDB(t)
		img, passport := "new.png", 201
		in := in
		in.ImageID, in.PassportID = &img, &passport
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT email FROM visa_status`).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("c@wso2.com"))
		mock.ExpectQuery(`AND id <> \?`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(travelRe(`UPDATE visa_status SET updated_by = ?, updated_date = NOW(), country = ?, visa_type = ?,
			start_date = ?, end_date = ?, status = ?, entry_type = ?, issued_date = ?, image_id = ?, passport_id = ? WHERE id = ?`)).
			WithArgs("admin@wso2.com", "GB", "Work Permit", "2026-01-10", "2027-01-09", "Active", "Single", "2026-01-05",
				"new.png", 201, 301).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := NewTravelDocumentRepository(db).UpdateVisa(context.Background(), 301, in); err != nil {
			t.Fatal(err)
		}
		travelCheck(t, mock)
	})
}

func TestTravelGetVisa(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectQuery(travelRe(`WHERE v.id = ?`)).WithArgs(301).WillReturnRows(travelVisaRow())
	mock.ExpectQuery(travelRe(`WHERE v.id = ?`)).WithArgs(9).WillReturnRows(sqlmock.NewRows(travelVisaColumns))
	repo := NewTravelDocumentRepository(db)
	if v, err := repo.GetVisa(context.Background(), 301); err != nil || v == nil || v.ID != 301 {
		t.Fatalf("v %+v err %v", v, err)
	}
	if v, err := repo.GetVisa(context.Background(), 9); err != nil || v != nil {
		t.Fatalf("missing: v %+v err %v", v, err)
	}
	travelCheck(t, mock)
}

func TestTravelSearchDocuments(t *testing.T) {
	db, mock := travelNewDB(t)
	where := ` WHERE d.email IN (?) AND d.document_type IN (?) AND d.status = ?` +
		` AND (d.expiry_date IS NULL OR DATE(d.expiry_date) >= ?)` +
		` AND (d.email LIKE ? OR d.document_no LIKE ? OR d.document_type LIKE ? OR d.issued_country LIKE ?)`
	args := []any{"c@wso2.com", "Passport", "Active", "2026-10-05", "%N1%", "%N1%", "%N1%", "%N1%"}
	mock.ExpectQuery(travelRe(`SELECT COUNT(*) FROM employee_travel_document AS d` + where)).
		WithArgs(travelArgs(args)...).WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(1))
	mock.ExpectQuery(travelRe(where + ` ORDER BY d.expiry_date IS NULL, d.expiry_date ASC, d.id LIMIT ? OFFSET ?`)).
		WithArgs(travelArgs(append(args, 5, 5))...).
		WillReturnRows(sqlmock.NewRows(travelDocColumns).AddRow(201, "c@wso2.com", "N1234913", "Passport", "LK",
			travelDay("2022-05-11"), nil, "Active", nil, "c@wso2.com", nil))
	items, total, err := NewTravelDocumentRepository(db).SearchDocuments(context.Background(), domain.TravelDocumentFilter{
		Emails: []string{"c@wso2.com"}, DocumentTypes: []string{"Passport"}, Status: "Active",
		Validity: domain.TravelValidityActive, Today: "2026-10-05", Search: "N1", Limit: 5, Offset: 5,
	})
	if err != nil || total != 1 || len(items) != 1 || items[0].ExpiryDate != nil || *items[0].IssuedDate != "2022-05-11" {
		t.Fatalf("items %+v total %d err %v", items, total, err)
	}
	travelCheck(t, mock)
}

func TestTravelCreateAndUpdateDocument(t *testing.T) {
	db, mock := travelNewDB(t)
	expiry := "2032-05-10"
	mock.ExpectBegin()
	mock.ExpectExec(travelRe(`WHERE email = ? AND document_type = ? AND issued_country = ? AND status = 'Active'`)).
		WithArgs("admin@wso2.com", "c@wso2.com", "Passport", "LK").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(travelRe(`INSERT INTO employee_travel_document`)).
		WithArgs("c@wso2.com", "N1234913", "Passport", "LK", "2022-05-11", &expiry, "admin@wso2.com", "p.png").
		WillReturnResult(sqlmock.NewResult(202, 1))
	mock.ExpectCommit()

	number := "N7654321"
	mock.ExpectBegin()
	mock.ExpectQuery(travelRe(`SELECT email FROM employee_travel_document WHERE id = ?`)).WithArgs(202).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("c@wso2.com"))
	mock.ExpectQuery(travelRe(`WHERE email = ? AND document_type = ? AND issued_country = ? AND status = 'Active' AND id <> ? LIMIT 1`)).
		WithArgs("c@wso2.com", "Passport", "LK", 202).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(travelRe(`UPDATE employee_travel_document SET updated_by = ?, updated_date = NOW(), document_type = ?,
			issued_country = ?, issued_date = ?, status = ?, document_no = ?, expiry_date = ? WHERE id = ?`)).
		WithArgs("admin@wso2.com", "Passport", "LK", "2022-05-11", "Inactive", "N7654321", "2032-05-10", 202).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewTravelDocumentRepository(db)
	id, err := repo.CreateDocument(context.Background(), domain.CreateTravelDocumentInput{
		Email: "c@wso2.com", DocumentNo: "N1234913", DocumentType: "Passport", IssuedCountry: "LK",
		IssuedDate: "2022-05-11", ExpiryDate: &expiry, ImageID: "p.png", CreatedBy: "admin@wso2.com",
	})
	if err != nil || id != 202 {
		t.Fatalf("id %d err %v", id, err)
	}
	if err := repo.UpdateDocument(context.Background(), 202, domain.UpdateTravelDocumentInput{
		DocumentNo: &number, DocumentType: "Passport", IssuedCountry: "LK", IssuedDate: "2022-05-11",
		ExpiryDate: &expiry, Status: "Inactive", UpdatedBy: "admin@wso2.com",
	}); err != nil {
		t.Fatal(err)
	}
	travelCheck(t, mock)
}

func TestTravelUpdateDocument_Duplicate(t *testing.T) {
	db, mock := travelNewDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT email FROM employee_travel_document`).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("c@wso2.com"))
	mock.ExpectQuery(`AND id <> \?`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectRollback()
	err := NewTravelDocumentRepository(db).UpdateDocument(context.Background(), 202, domain.UpdateTravelDocumentInput{
		DocumentType: "Passport", IssuedCountry: "LK", IssuedDate: "2022-05-11", Status: "Active", UpdatedBy: "a@wso2.com"})
	if !errors.Is(err, ErrTravelDuplicateActive) {
		t.Fatalf("err = %v", err)
	}
	travelCheck(t, mock)
}
