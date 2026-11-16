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

package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/middleware"
)

const insertSQL = `INSERT INTO audit_event`

// requestCtx runs fn with the context the middleware builds for a request
// with the given headers.
func requestCtx(t *testing.T, user, requestID string, fn func(ctx context.Context)) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if user != "" {
		r.Header.Set("X-User-Email", user)
	}
	if requestID != "" {
		r.Header.Set("X-Correlation-ID", requestID)
	}
	middleware.CorrelationID(middleware.Actor(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		fn(r.Context())
	}))).ServeHTTP(httptest.NewRecorder(), r)
}

func TestRecord_ActorAndRequestFromTheRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
		WithArgs("jane@wso2.com", "user", "View", "visa", "5", nil, nil, "req-1", "engagement-app", nil, `{"own":true}`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
		WithArgs(nil, "system", "Update", "invoice", "7", nil, nil, sqlmock.AnyArg(), "engagement-app", nil, nil).
		WillReturnResult(sqlmock.NewResult(2, 1))
	requestCtx(t, " Jane@WSO2.com ", "req-1", func(ctx context.Context) {
		if err := Record(ctx, db, Event{Action: View, EntityType: "visa", EntityID: "5", Actor: "body@wso2.com",
			Metadata: map[string]any{"own": true}}); err != nil {
			t.Error(err)
		}
	})
	requestCtx(t, "", "", func(ctx context.Context) {
		if err := Record(ctx, db, Event{Action: Update, EntityType: "invoice", EntityID: "7"}); err != nil {
			t.Error(err)
		}
	})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTracker_ChangedFieldsMaskedAndStatusChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"id", "email", "document_no", "status"}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM employee_travel_document WHERE id = ? FOR UPDATE`)).WithArgs(9).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(9, "c@wso2.com", []byte("N1234567"), "Active"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM employee_travel_document WHERE id IN (?)`)).WithArgs("9").
		WillReturnRows(sqlmock.NewRows(cols).AddRow(9, "c@wso2.com", []byte("N7654321"), "Inactive"))
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).
		WithArgs("c@wso2.com", "user", "Update", "travel_document", "9", "employee", "c@wso2.com", nil, "engagement-app",
			`{"document_no":{"before":"****4567","after":"****4321"},"status":{"before":"Active","after":"Inactive"}}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))
	ctx := context.Background()
	tr, err := Track(ctx, db, "employee_travel_document", "id", `id = ?`, 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Record(ctx, db, Event{Action: Update, EntityType: "travel_document", ParentType: "employee",
		ParentColumn: "email", Actor: "c@wso2.com"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDisabled_RecordsNothing(t *testing.T) {
	SetEnabled(false)
	defer SetEnabled(true)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tr, err := Track(ctx, db, "invoice", "id", `id = ?`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Record(ctx, db, Event{Action: Update, EntityType: "invoice"}); err != nil {
		t.Fatal(err)
	}
	if err := Inserted("invoice", "id", 1).Record(ctx, db, Event{Action: Create, EntityType: "invoice"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMask(t *testing.T) {
	for in, want := range map[string]string{"N1234567": "****4567", "123": "****", "": "****"} {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusChangedIgnoresBookkeepingOnly(t *testing.T) {
	cases := []struct {
		cols []string
		want bool
	}{
		{[]string{"status", "updated_by"}, true},
		{[]string{"engagement_status", "last_updated_date"}, true},
		{[]string{"status", "comment"}, false},
		{[]string{"updated_by"}, false},
	}
	for _, c := range cases {
		m := map[string]Change{}
		for _, col := range c.cols {
			m[col] = Change{}
		}
		if got := statusChanged(m); got != c.want {
			t.Errorf("statusChanged(%v) = %v, want %v", c.cols, got, c.want)
		}
	}
}

func TestBitValue(t *testing.T) {
	if bitValue([]byte{0}) != 0 || bitValue([]byte{1}) != 1 {
		t.Error("bitValue should map BIT bytes to 0/1")
	}
}
