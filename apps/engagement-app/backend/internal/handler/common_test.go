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

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPagination(t *testing.T) {
	for q, ok := range map[string]bool{"": true, "limit=10&offset=20": true, "limit=0": false, "limit=501": false, "offset=-1": false, "limit=x": false} {
		r := httptest.NewRequest(http.MethodGet, "/x?"+q, nil)
		if _, _, err := pagination(r); (err == nil) != ok {
			t.Errorf("%q: err = %v", q, err)
		}
	}
}

func TestWriteCSVNeutralisesFormulas(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCSV(rec, "x.csv", []string{"a", "b"}, [][]string{{"=HYPERLINK(1)", "-12.5"}, {"a,b", `say "hi"`}})
	got := rec.Body.String()
	want := "a,b\n'=HYPERLINK(1),-12.5\n\"a,b\",\"say \"\"hi\"\"\"\n"
	if got != want {
		t.Errorf("csv = %q, want %q", got, want)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), `"x.csv"`) {
		t.Error("no filename")
	}
}

func TestQueryHelpers(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?s=a&s=+b+&s=&n=1&n=2&d=2026-11-05&bad=5", nil)
	if got := queryList(r, "s"); len(got) != 2 || got[1] != "b" {
		t.Errorf("queryList = %v", got)
	}
	if got, err := queryInts(r, "n"); err != nil || len(got) != 2 {
		t.Errorf("queryInts = %v %v", got, err)
	}
	if _, err := queryInts(httptest.NewRequest(http.MethodGet, "/x?n=a", nil), "n"); err == nil {
		t.Error("queryInts accepted a")
	}
	if d, err := queryDate(r, "d"); err != nil || d != "2026-11-05" {
		t.Errorf("queryDate = %q %v", d, err)
	}
	if _, err := queryDate(httptest.NewRequest(http.MethodGet, "/x?d=05/11/2026", nil), "d"); err == nil {
		t.Error("queryDate accepted 05/11/2026")
	}
}
