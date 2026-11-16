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

package netsuite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
)

func testConfig(base string) Config {
	return Config{
		Enabled: true, AccountID: "1234_SB1", ConsumerKey: "ck", ConsumerSecret: "cs~*", TokenID: "tok en",
		TokenSecret: "ts", RestletsBaseURL: base,
		CreatePurchaseOrder: Script{"101", "1"}, GetPurchaseOrder: Script{"102", "1"},
		CreateVendorBill: Script{"103", "2"}, GetVendorBill: Script{"104", "1"},
		CreateMarketingEvent: Script{"105", "1"}, PurchaseOrderTaxCode: "7",
	}
}

func TestFormEncodeIsJavaURLEncoder(t *testing.T) {
	for in, want := range map[string]string{
		"TRV-2026-11-0001": "TRV-2026-11-0001",
		"a b*c~d":          "a+b*c%7Ed",
		"x/y=z&é":          "x%2Fy%3Dz%26%C3%A9",
	} {
		if got := formEncode(in); got != want {
			t.Errorf("formEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

// The base string is spelled out by hand from the legacy algorithm: query
// values as they appear in the URL, merged with the protocol parameters,
// sorted, joined, then the whole string form-encoded.
func TestOAuthHeaderMatchesLegacyAlgorithm(t *testing.T) {
	c := testConfig("https://1234-sb1.restlets.api.netsuite.com")
	u := c.RestletsBaseURL + "/app/site/hosting/restlet.nl?script=102&deploy=1&transactionId=INV-1"
	got := oauthHeader(c, "GET", u, "abcd1234", "1700000000")

	normalized := "deploy=1&oauth_consumer_key=ck&oauth_nonce=abcd1234&oauth_signature_method=HMAC-SHA256" +
		"&oauth_timestamp=1700000000&oauth_token=tok en&oauth_version=1.0&script=102&transactionId=INV-1"
	base := "GET&" + "https%3A%2F%2F1234-sb1.restlets.api.netsuite.com%2Fapp%2Fsite%2Fhosting%2Frestlet.nl" +
		"&" + formEncode(normalized)
	mac := hmac.New(sha256.New, []byte("cs%7E*&ts"))
	mac.Write([]byte(base))
	sig := formEncode(base64.StdEncoding.EncodeToString(mac.Sum(nil)))

	want := `OAuth realm="1234_SB1",oauth_consumer_key="ck",oauth_token="tok+en",oauth_signature="` + sig +
		`",oauth_timestamp="1700000000",oauth_nonce="abcd1234",oauth_signature_method="HMAC-SHA256",oauth_version="1.0"`
	if got != want {
		t.Errorf("header\n got %s\nwant %s", got, want)
	}
}

func TestTransformDate(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-04":          "04 November, 2026",
		"2026-11-5":           "5 November, 2026",
		"2026-01-31 10:00:00": "31 January, 2026",
	} {
		got, err := TransformDate(in)
		if err != nil || got != want {
			t.Errorf("TransformDate(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := TransformDate("2026/11/04"); err == nil {
		t.Error("want an error for a bad date")
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(testConfig(srv.URL), srv.Client())
	c.now = func() time.Time { return time.Unix(1700000000, 0) }
	c.nonce = func() string { return "abcd1234" }
	return c
}

func TestCreatePurchaseOrder(t *testing.T) {
	var gotBody map[string]any
	var gotQuery, gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotAuth = r.URL.RawQuery, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{"id": 5501}`))
	})
	id, err := c.CreatePurchaseOrder(context.Background(), PurchaseOrder{TransactionID: "INV-1", TransactionDate: "2026-11-04", TotalAmount: "155100.00"})
	if err != nil || id != "5501" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if gotQuery != "script=101&deploy=1" {
		t.Errorf("query = %q", gotQuery)
	}
	if !strings.HasPrefix(gotAuth, `OAuth realm="1234_SB1",`) {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody["transactionDate"] != "04 November, 2026" || gotBody["totalAmount"] != "155100.00" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestCreateFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"suitescript error": {200, `{"name":"X","message":"Vendor is inactive","stack":[]}`, "Error: Vendor is inactive"},
		"non-200":           {500, `boom`, "Status code: 500 Details: boom"},
		"no id":             {200, `{"ok":true}`, "Unexpected response"},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.CreateVendorBill(context.Background(), VendorBill{TransactionDate: "2026-11-5"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("transactionId") != "TRV-2026-11-0001-001" || r.URL.Query().Get("script") != "104" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"vendorBills":[{"internalId":77,"transactionId":"TRV-2026-11-0001-001"},{"internalId":"78","transactionId":"OTHER"}]}`))
	})
	got, err := c.LookupVendorBills(context.Background(), "TRV-2026-11-0001-001")
	if err != nil || len(got) != 1 || got[0].InternalID != "77" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestDisabled(t *testing.T) {
	c := New(Config{}, nil)
	if _, err := c.LookupPurchaseOrders(context.Background(), "x"); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v", err)
	}
	if _, err := c.CreatePurchaseOrder(context.Background(), PurchaseOrder{}); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v", err)
	}
}

func TestConfigFrom(t *testing.T) {
	c := ConfigFrom(config.NetSuiteConfig{PushEnabled: true, AccountID: "a", ConsumerKey: "b", ConsumerSecret: "c",
		TokenID: "d", TokenSecret: "e", RestletsBaseURL: "https://x/", CreatePurchaseOrderScriptID: "11",
		GetPurchaseOrderScriptID: "12", PurchaseOrderTaxCode: "7", CreateVendorBillScriptID: "13", GetVendorBillScriptID: "14",
		CreateMarketingEventScriptID: "15"})
	if c.CreatePurchaseOrder != (Script{"11", "1"}) || c.CreateVendorBill != (Script{"13", "1"}) || c.RestletsBaseURL != "https://x" {
		t.Errorf("config = %+v", c)
	}
	if m := c.Missing(); len(m) != 0 {
		t.Errorf("missing = %v", m)
	}
	c.GetVendorBill.ID = ""
	if m := c.Missing(); len(m) != 1 || m[0] != "NETSUITE_GET_VENDOR_BILL_SCRIPT_ID" {
		t.Errorf("missing = %v", m)
	}
}

func TestCreateMarketingEvent(t *testing.T) {
	var gotBody map[string]any
	var gotQuery string
	status := http.StatusOK
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`7712`))
	})
	if err := c.CreateMarketingEvent(context.Background(), "WSO2Con 2027", "EVT00510"); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "script=105&deploy=1" || gotBody["eventName"] != "WSO2Con 2027" || gotBody["eventID"] != "EVT00510" {
		t.Errorf("query %q body %v", gotQuery, gotBody)
	}
	status = http.StatusBadRequest
	if err := c.CreateMarketingEvent(context.Background(), "x", "EVT1"); err == nil || errors.Is(err, ErrEventExists) {
		t.Error("a non-200 must be an error")
	}
	c2 := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"Event already exists in NetSuite","message":"..."}}`))
	})
	if err := c2.CreateMarketingEvent(context.Background(), "x", "EVT1"); !errors.Is(err, ErrEventExists) {
		t.Errorf("exists: %v", err)
	}
	if err := (*Client)(nil).CreateMarketingEvent(context.Background(), "x", "y"); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: %v", err)
	}
}
