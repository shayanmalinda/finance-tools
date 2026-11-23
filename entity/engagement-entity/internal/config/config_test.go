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
		"AUTH_DISABLED", "GATEWAY_JWT_ISSUER", "GATEWAY_JWT_JWKS_URL", "ALLOWED_CLIENT_IDS", "CLIENT_ID_CLAIMS", "GATEWAY_JWT_API_CONTEXTS", "API_CONTEXT_CLAIMS",
		"DB_TLS_CA", "DB_TLS_SERVER_NAME", "AUDIT_OPTIONAL", "NON_PRODUCTION",
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
	c := &Config{DB: DBConfig{Host: "h", User: "u", Password: "p", Port: 3306}, Auth: AuthConfig{Disabled: true}, NonProduction: true}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	c.NetSuite.PushEnabled = true
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "NETSUITE_ACCOUNT_ID") || !strings.Contains(err.Error(), "NETSUITE_PURCHASE_ORDER_TAX_CODE") {
		t.Errorf("push enabled without credentials: err = %v", err)
	}
}

// Auth fails closed: without AUTH_DISABLED the gateway JWKS URL (https) and
// the allowed client ids are required.
func TestValidateRequiresGatewayAuth(t *testing.T) {
	base := func() *Config { return &Config{DB: DBConfig{Host: "h", User: "u", Password: "p", Port: 3306}} }
	err := base().Validate()
	if err == nil || !strings.Contains(err.Error(), "GATEWAY_JWT_JWKS_URL") || !strings.Contains(err.Error(), "ALLOWED_CLIENT_IDS") {
		t.Errorf("no auth settings: err = %v", err)
	}
	c := base()
	c.Auth = AuthConfig{JWKSURL: "http://gw.example/.wellknown/jwks", AllowedClientIDs: []string{"backend"}}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "must be an https URL") {
		t.Errorf("plain http JWKS: err = %v", err)
	}
	c.Auth.JWKSURL = "https://gw.example/.wellknown/jwks"
	c.Auth.APIContexts = []string{"/wutu/engagement-entity/v1.0"}
	if err := c.Validate(); err != nil {
		t.Errorf("complete auth: err = %v", err)
	}
	c = base()
	c.Auth.Disabled = true
	c.NonProduction = true
	if err := c.Validate(); err != nil {
		t.Errorf("AUTH_DISABLED: err = %v", err)
	}
}

func TestLoadAuth(t *testing.T) {
	clearEnv(t)
	t.Setenv("GATEWAY_JWT_JWKS_URL", " https://gw.example/.wellknown/jwks ")
	t.Setenv("ALLOWED_CLIENT_IDS", " a, ,b ")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Disabled || c.Auth.Issuer != DefaultGatewayIssuer || c.Auth.JWKSURL != "https://gw.example/.wellknown/jwks" ||
		len(c.Auth.AllowedClientIDs) != 2 || c.Auth.AllowedClientIDs[0] != "a" || c.Auth.AllowedClientIDs[1] != "b" {
		t.Errorf("auth = %+v", c.Auth)
	}
	if len(c.Auth.ClientIDClaims) != 0 {
		t.Errorf("CLIENT_ID_CLAIMS unset: claims = %v, want empty (the auth default)", c.Auth.ClientIDClaims)
	}
	t.Setenv("CLIENT_ID_CLAIMS", " http://wso2.org/claims/applicationUUId , ,azp ")
	if c, err = Load(); err != nil || len(c.Auth.ClientIDClaims) != 2 ||
		c.Auth.ClientIDClaims[0] != "http://wso2.org/claims/applicationUUId" || c.Auth.ClientIDClaims[1] != "azp" {
		t.Errorf("CLIENT_ID_CLAIMS: claims = %v, err %v", c.Auth.ClientIDClaims, err)
	}
	if len(c.Auth.APIContexts) != 0 || len(c.Auth.APIContextClaims) != 0 {
		t.Errorf("API context unset: %v / %v", c.Auth.APIContexts, c.Auth.APIContextClaims)
	}
	t.Setenv("GATEWAY_JWT_API_CONTEXTS", " /wutu/engagement-entity/v1.0 , ")
	t.Setenv("API_CONTEXT_CLAIMS", "apicontext")
	if c, err = Load(); err != nil || len(c.Auth.APIContexts) != 1 || c.Auth.APIContexts[0] != "/wutu/engagement-entity/v1.0" ||
		len(c.Auth.APIContextClaims) != 1 || c.Auth.APIContextClaims[0] != "apicontext" {
		t.Errorf("API context: %+v, err %v", c.Auth, err)
	}
	t.Setenv("AUTH_DISABLED", "maybe")
	if _, err := Load(); err == nil {
		t.Error("AUTH_DISABLED=maybe: want error")
	}
}

// Disabling auth also binds the server to loopback, so a deployment that
// sets it by mistake is unreachable, not open.
func TestListenAddr(t *testing.T) {
	c := &Config{ServerPort: "8290"}
	if got := c.ListenAddr(); got != ":8290" {
		t.Errorf("auth on: %q", got)
	}
	c.Auth.Disabled = true
	if got := c.ListenAddr(); got != "127.0.0.1:8290" {
		t.Errorf("auth off: %q", got)
	}
}

func TestLoadDBTLSAndAudit(t *testing.T) {
	clearEnv(t)
	t.Setenv("DB_TLS_CA", " /etc/mysql-ca/ca.pem ")
	t.Setenv("DB_TLS_SERVER_NAME", "mysql.internal.wso2.com")
	t.Setenv("AUDIT_OPTIONAL", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DB.TLSCA != "/etc/mysql-ca/ca.pem" || c.DB.TLSServerName != "mysql.internal.wso2.com" || !c.DB.CustomTLS() || !c.AuditOptional {
		t.Errorf("db = %+v, audit optional %v", c.DB, c.AuditOptional)
	}
	t.Setenv("AUDIT_OPTIONAL", "maybe")
	if _, err := Load(); err == nil {
		t.Error("AUDIT_OPTIONAL=maybe: want error")
	}
}

// A CA or server name means verified TLS: it can't be combined with an
// unverified or disabled DB_TLS.
func TestValidateCustomTLSNeedsVerifiedTLS(t *testing.T) {
	for _, mode := range []string{"skip-verify", "false", "preferred"} {
		c := &Config{DB: DBConfig{Host: "h", User: "u", Password: "p", TLS: mode, TLSCA: "/ca.pem"}, Auth: AuthConfig{Disabled: true}, NonProduction: true}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "DB_TLS must be empty or true") {
			t.Errorf("DB_TLS=%s with a CA: err = %v", mode, err)
		}
	}
	for _, mode := range []string{"", "true"} {
		c := &Config{DB: DBConfig{Host: "h", User: "u", Password: "p", TLS: mode, TLSCA: "/ca.pem"}, Auth: AuthConfig{Disabled: true}, NonProduction: true}
		if err := c.Validate(); err != nil {
			t.Errorf("DB_TLS=%q with a CA: err = %v", mode, err)
		}
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

// Production fails closed: the weaker settings a staging or local run needs
// stop startup unless NON_PRODUCTION=true.
func TestValidateWeakSettingsNeedNonProduction(t *testing.T) {
	prod := func() *Config {
		return &Config{
			DB: DBConfig{Host: "h", User: "u", Password: "p", Port: 3306},
			Auth: AuthConfig{JWKSURL: "https://gw.example/.wellknown/jwks", AllowedClientIDs: []string{"backend"},
				APIContexts: []string{"/wutu/engagement-entity/v1.0"}},
		}
	}
	if err := prod().Validate(); err != nil {
		t.Fatalf("production settings: err = %v", err)
	}
	for name, weaken := range map[string]func(*Config){
		"DB_TLS=skip-verify":       func(c *Config) { c.DB.TLS = "skip-verify" },
		"DB_TLS=false":             func(c *Config) { c.DB.TLS = "false" },
		"DB_TLS=preferred":         func(c *Config) { c.DB.TLS = "preferred" },
		"AUDIT_OPTIONAL":           func(c *Config) { c.AuditOptional = true },
		"AUTH_DISABLED":            func(c *Config) { c.Auth.Disabled = true },
		"GATEWAY_JWT_API_CONTEXTS": func(c *Config) { c.Auth.APIContexts = nil },
	} {
		c := prod()
		weaken(c)
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "NON_PRODUCTION") {
			t.Errorf("%s in production: err = %v, want a NON_PRODUCTION error", name, err)
		}
		c.NonProduction = true
		if err := c.Validate(); err != nil {
			t.Errorf("%s with NON_PRODUCTION: err = %v", name, err)
		}
	}
	c := prod()
	c.DB.TLS = "true"
	if err := c.Validate(); err != nil {
		t.Errorf("DB_TLS=true: err = %v", err)
	}
}

func TestLoadNonProduction(t *testing.T) {
	clearEnv(t)
	if c, err := Load(); err != nil || c.NonProduction {
		t.Errorf("unset: NonProduction %v, err %v; want false", c != nil && c.NonProduction, err)
	}
	t.Setenv("NON_PRODUCTION", "true")
	if c, err := Load(); err != nil || !c.NonProduction {
		t.Errorf("true: err %v", err)
	}
	t.Setenv("NON_PRODUCTION", "maybe")
	if _, err := Load(); err == nil {
		t.Error("NON_PRODUCTION=maybe: want error")
	}
}
