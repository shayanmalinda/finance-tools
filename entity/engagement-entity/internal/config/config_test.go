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

package config

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// clearEnv blanks every variable Load reads, so the shell can't leak in.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SERVER_PORT", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_TLS",
		"NETSUITE_PUSH_ENABLED", "NETSUITE_ACCOUNT_ID", "NETSUITE_RESTLETS_BASE_URL",
		"NETSUITE_CONSUMER_KEY", "NETSUITE_CONSUMER_SECRET", "NETSUITE_TOKEN_ID", "NETSUITE_TOKEN_SECRET",
		"NETSUITE_PURCHASE_ORDER_TAX_CODE",
		"NETSUITE_CREATE_MARKETING_EVENT_SCRIPT_ID", "NETSUITE_CREATE_MARKETING_EVENT_DEPLOY_ID",
		"NETSUITE_CREATE_PURCHASE_ORDER_SCRIPT_ID", "NETSUITE_CREATE_PURCHASE_ORDER_DEPLOY_ID",
		"NETSUITE_CREATE_VENDOR_BILL_SCRIPT_ID", "NETSUITE_CREATE_VENDOR_BILL_DEPLOY_ID",
		"NETSUITE_GET_PURCHASE_ORDER_SCRIPT_ID", "NETSUITE_GET_PURCHASE_ORDER_DEPLOY_ID",
		"NETSUITE_GET_VENDOR_BILL_SCRIPT_ID", "NETSUITE_GET_VENDOR_BILL_DEPLOY_ID",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerPort != "8290" || c.DB.Port != 3306 || c.NetSuite.PushEnabled {
		t.Errorf("defaults = port %q, db port %d, push %v", c.ServerPort, c.DB.Port, c.NetSuite.PushEnabled)
	}
}

func TestLoadRejectsMalformed(t *testing.T) {
	clearEnv(t)
	t.Setenv("DB_PORT", "abc")
	if _, err := Load(); err == nil {
		t.Error("DB_PORT=abc: want error")
	}
	t.Setenv("DB_PORT", "")
	t.Setenv("NETSUITE_PUSH_ENABLED", "maybe")
	if _, err := Load(); err == nil {
		t.Error("NETSUITE_PUSH_ENABLED=maybe: want error")
	}
}

func TestValidateNamesEveryMissingSetting(t *testing.T) {
	err := (&Config{}).Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	for _, key := range []string{"DB_HOST", "DB_USER", "DB_PASSWORD"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
	if strings.Contains(err.Error(), "NETSUITE") {
		t.Errorf("NetSuite settings required while push is off: %v", err)
	}
}

func TestValidateNetSuiteOnlyWhenPushEnabled(t *testing.T) {
	c := &Config{DB: DBConfig{Host: "h", User: "u", Password: "p", Port: 3306}}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	c.NetSuite.PushEnabled = true
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "NETSUITE_ACCOUNT_ID") || !strings.Contains(err.Error(), "NETSUITE_PURCHASE_ORDER_TAX_CODE") {
		t.Errorf("push enabled without credentials: err = %v", err)
	}
}

func TestDSN(t *testing.T) {
	d := DBConfig{Host: "db.example", Port: 3306, User: "u", Password: "p@s/s:1"}
	cfg, err := mysql.ParseDSN(d.DSN(AllocationSchema))
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if cfg.User != "u" || cfg.Passwd != "p@s/s:1" || cfg.Addr != "db.example:3306" || cfg.DBName != "allocation_db" || !cfg.ParseTime || cfg.TLSConfig != "" {
		t.Errorf("parsed = %+v", cfg)
	}
	d.TLS = "false"
	cfg, err = mysql.ParseDSN(d.DSN(TravelSchema))
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if cfg.TLSConfig != "false" || cfg.DBName != "travelapp" {
		t.Errorf("parsed with TLS = %+v", cfg)
	}
}

func TestDefaultRestletsURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("NETSUITE_PUSH_ENABLED", "true")
	t.Setenv("NETSUITE_ACCOUNT_ID", "1234567_SB1")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.NetSuite.RestletsBaseURL; got != "https://1234567-sb1.restlets.api.netsuite.com" {
		t.Errorf("RestletsBaseURL = %q", got)
	}
}
