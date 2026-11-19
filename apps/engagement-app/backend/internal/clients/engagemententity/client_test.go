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

package engagemententity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func TestHTTPClient_PathsAndDecoding(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch r.URL.Path {
		case "/access/travel/role-privileges":
			_, _ = w.Write([]byte(`{"rolePrivileges":{"app-travel-admin-super":[1,2]}}`))
		case "/allocation-types":
			_, _ = w.Write([]byte(`{"allocationTypes":[{"id":3,"allocationTypeName":"Pre-sale Support","status":"ACTIVE","engagementTypeId":1,"engagementTypeName":"Internal Engagement"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, nil)

	m, err := c.RolePrivileges(context.Background(), domain.AppTravel)
	if err != nil || len(m["app-travel-admin-super"]) != 2 {
		t.Fatalf("RolePrivileges = %v, %v", m, err)
	}
	types, err := c.AllocationTypes(context.Background(), AllocationTypeFilter{EngagementTypeID: 1, Status: "Active"})
	if err != nil || len(types) != 1 || types[0].AllocationTypeName != "Pre-sale Support" {
		t.Fatalf("AllocationTypes = %+v, %v", types, err)
	}
	want := []string{"/access/travel/role-privileges", "/allocation-types?engagementTypeId=1&status=Active"}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, paths[i], want[i])
		}
	}
}
