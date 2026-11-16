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
