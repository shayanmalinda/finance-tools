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

package hrentity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
)

func TestEmployee(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Variables["email"] != "jane@wso2.com" {
			t.Errorf("variables = %v", req.Variables)
		}
		_, _ = w.Write([]byte(`{"data":{"employee":{"employeeId":"LK100","workEmail":"jane@wso2.com","firstName":"Jane","lastName":"Doe","managerEmail":"lead@wso2.com"}}}`))
	}))
	defer srv.Close()

	e, err := New(srv.URL, nil).Employee(context.Background(), " jane@wso2.com ")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil || e.FirstName != "Jane" || e.ManagerEmail == nil || *e.ManagerEmail != "lead@wso2.com" {
		t.Errorf("got %+v", e)
	}
}

func TestGraphQLErrorsAreUpstreamErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	_, err := New(srv.URL, nil).SearchEmployees(context.Background(), "jane", []string{"Active"}, 10)
	var ue *apierror.UpstreamError
	if !errors.As(err, &ue) || ue.Msg != "boom" || ue.ClientError() {
		t.Errorf("err = %v, want a non-client UpstreamError carrying the GraphQL message", err)
	}
}

func TestReportsToAndOrgData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Variables["email"] != nil {
			_, _ = w.Write([]byte(`{"data":{"employee":{"reportsToEmail":"a@wso2.com, b@wso2.com"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"orgData":{"orgStructure":[{"name":"Digital Operations","children":[{"name":"WSO2 Digital","children":[{"name":"Finance Apps"}]}]}]}}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, nil)
	leads, err := c.ReportsTo(context.Background(), "x@wso2.com")
	if err != nil || len(leads) != 2 || leads[1] != "b@wso2.com" {
		t.Fatalf("leads %v err %v", leads, err)
	}
	org, err := c.OrgData(context.Background(), []string{"Active"})
	if err != nil || len(org) != 1 || org[0].Children[0].Children[0].Name != "Finance Apps" {
		t.Fatalf("org %+v err %v", org, err)
	}
}

func TestEmployeesByOrg_ReadsUntilAnEmptyPage(t *testing.T) {
	// HR caps the page at 2, below orgEmployeesPageSize: a short page is not the last.
	all := []string{"a@wso2.com", "b@wso2.com", "c@wso2.com"}
	var offsets []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		off := req.Variables["offset"].(float64)
		offsets = append(offsets, off)
		var page []map[string]string
		for i := int(off); i < len(all) && i < int(off)+2; i++ {
			page = append(page, map[string]string{"workEmail": all[i]})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"employees": page}})
	}))
	defer srv.Close()
	got, err := New(srv.URL, nil).EmployeesByOrg(context.Background(), OrgEmployeeFilter{})
	if err != nil || len(got) != 3 || got[2].WorkEmail != "c@wso2.com" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if len(offsets) != 3 || offsets[1] != 2 || offsets[2] != 3 {
		t.Errorf("offsets = %v", offsets)
	}
}

func TestEmployeesByOrg_RepeatedPageIsAnError(t *testing.T) {
	// HR ignores the offset and keeps sending the same page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"employees":[{"workEmail":"a@wso2.com"}]}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, nil).EmployeesByOrg(context.Background(), OrgEmployeeFilter{})
	var ue *apierror.UpstreamError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UpstreamError", err)
	}
}

// A listing longer than 100 pages completes when HR caps each page at 100
// rows: the bound follows rows, not the requested page size.
func TestEmployeesByOrg_SmallPagesStillComplete(t *testing.T) {
	const total, pageCap = 10_050, 100
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		off := int(req.Variables["offset"].(float64))
		page := []map[string]string{}
		for i := off; i < total && i < off+pageCap; i++ {
			page = append(page, map[string]string{"workEmail": "e" + strconv.Itoa(i) + "@wso2.com"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"employees": page}})
	}))
	defer srv.Close()
	got, err := New(srv.URL, nil).EmployeesByOrg(context.Background(), OrgEmployeeFilter{})
	if err != nil || len(got) != total {
		t.Fatalf("got %d rows, err %v", len(got), err)
	}
}

// A listing that fills exactly the last permitted page still ends: one more
// call finds the empty page.
func TestEmployeesByOrg_EndsAtThePageLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req graphqlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		off := int(req.Variables["offset"].(float64))
		page := []map[string]string{}
		if off < orgEmployeesMaxPages { // one row per page
			page = append(page, map[string]string{"workEmail": "e" + strconv.Itoa(off) + "@wso2.com"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"employees": page}})
	}))
	defer srv.Close()
	got, err := New(srv.URL, nil).EmployeesByOrg(context.Background(), OrgEmployeeFilter{})
	if err != nil || len(got) != orgEmployeesMaxPages || int(calls.Load()) != orgEmployeesMaxPages+1 {
		t.Fatalf("got %d rows in %d calls, err %v", len(got), calls.Load(), err)
	}
}
