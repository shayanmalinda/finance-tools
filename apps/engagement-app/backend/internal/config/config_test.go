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
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

var allKeys = []string{
	"SERVER_PORT", "JWT_ISSUER", "JWT_JWKS_URL", "JWT_USERINFO_URL", "GATEWAY_JWT_ISSUER", "GATEWAY_JWT_JWKS_URL",
	"GATEWAY_JWT_AUDIENCES", "EMAIL_DISABLED", "IS_LIVE_EMAILS", "EMAIL_TEST_RECIPIENTS",
	"JWT_AUDIENCES", "JWT_CLOCK_SKEW_SECONDS", "ENGAGEMENT_ENTITY_BASE_URL", "SALES_ENTITY_BASE_URL", "HR_ENTITY_URL",
	"OUTBOUND_TOKEN_URL", "OUTBOUND_CLIENT_ID", "OUTBOUND_CLIENT_SECRET", "EMAIL_SERVICE_URL", "APP_BASE_URL",
	"FILE_STORAGE_ACCOUNT", "FILE_STORAGE_ACCESS_KEY", "CORS_ALLOWED_ORIGINS", "DASHBOARD_CACHE_TTL",
	"EMAIL_COMPANY_RECIPIENTS", "EMAIL_INTERNAL_APPROVERS",
	"CURRENCY_CONVERT_SERVICE_URL", "CURRENCY_CONVERT_CLIENT_ID", "CURRENCY_CONVERT_CLIENT_SECRET",
	"REPORT_MAX_CONCURRENT", "OUTBOUND_SCOPES", "ACCESS_REQUEST_EMAIL",
	"BUDGETS_ENABLED", "BUDGET_LINE_RECIPIENTS", "BUDGETS_CACHE_TTL",
}

func setMinimalEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
	t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", "http://localhost:8290/")
	t.Setenv("JWT_AUDIENCES", "client-a")
	t.Setenv("EMAIL_DISABLED", "true")
}

// The only required settings are the entity URL, the audience and either
// the email service or the explicit email opt-out; everything else defaults.
func TestLoadMinimal(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.EngagementEntityBaseURL != "http://localhost:8290" {
		t.Errorf("EngagementEntityBaseURL = %q, want trailing slash trimmed", c.EngagementEntityBaseURL)
	}
	if c.JWT.Issuer != DefaultIssuer || c.JWT.JWKSURL != DefaultJWKSURL || c.JWT.UserinfoURL != DefaultUserinfoURL {
		t.Errorf("JWT defaults = %+v", c.JWT)
	}
	if c.Client.TokenURL != DefaultTokenURL || c.ServerPort != "8190" || c.JWT.ClockSkew != 60*time.Second {
		t.Errorf("defaults: token %q port %q skew %v", c.Client.TokenURL, c.ServerPort, c.JWT.ClockSkew)
	}
	if c.Email.Enabled() || c.Files.Enabled() || len(c.CORSAllowedOrigins) != 0 {
		t.Error("email, files and CORS must be off by default")
	}
}

// A missing EMAIL_SERVICE_URL stops startup unless EMAIL_DISABLED says the
// emails are meant to be off.
func TestValidateRequiresEmailOrOptOut(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("EMAIL_DISABLED", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "EMAIL_SERVICE_URL") {
		t.Errorf("Validate() = %v, want EMAIL_SERVICE_URL required", err)
	}

	t.Setenv("EMAIL_DISABLED", "maybe")
	if _, err := Load(); err == nil {
		t.Error("Load accepted EMAIL_DISABLED=maybe")
	}

	// Set together, the opt-out wins: emails stay off.
	t.Setenv("EMAIL_DISABLED", "true")
	t.Setenv("EMAIL_SERVICE_URL", "https://email.example")
	t.Setenv("OUTBOUND_CLIENT_ID", "id")
	t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.Email.Enabled() {
		t.Error("EMAIL_DISABLED=true left email enabled")
	}
}

// With EMAIL_DISABLED a leftover EMAIL_SERVICE_URL is never used, so it
// needs neither https nor the client pair.
func TestValidateIgnoresTheEmailURLWhenDisabled(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("EMAIL_SERVICE_URL", "http://email.example")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}

func TestLoadGatewayAudiences(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.JWT.GatewayAudiences, []string{"client-a"}) {
		t.Errorf("GatewayAudiences = %v, want JWT_AUDIENCES by default", c.JWT.GatewayAudiences)
	}
	t.Setenv("GATEWAY_JWT_AUDIENCES", "gw-1, gw-2")
	if c, err = Load(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.JWT.GatewayAudiences, []string{"gw-1", "gw-2"}) || !slices.Equal(c.JWT.Audiences, []string{"client-a"}) {
		t.Errorf("audiences = %v, gateway %v", c.JWT.Audiences, c.JWT.GatewayAudiences)
	}
}

func TestValidateRequiresEntityURL(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", "")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ENGAGEMENT_ENTITY_BASE_URL") {
		t.Errorf("Validate() = %v", err)
	}
}

func TestValidateRequiresAudiences(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JWT_AUDIENCES", "")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "JWT_AUDIENCES") {
		t.Errorf("Validate() = %v", err)
	}
}

func TestValidateRejectsInsecureJWKS(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JWT_JWKS_URL", "http://issuer.example/jwks")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "JWT_JWKS_URL must be an https URL") {
		t.Errorf("Validate() = %v", err)
	}
}

// URLs that carry the client secret or a bearer token must be https; http
// is allowed only on localhost for local runs.
func TestValidateRejectsInsecureOutboundURLs(t *testing.T) {
	for _, key := range []string{"OUTBOUND_TOKEN_URL", "SALES_ENTITY_BASE_URL", "HR_ENTITY_URL", "EMAIL_SERVICE_URL",
		"CURRENCY_CONVERT_SERVICE_URL"} {
		t.Run(key, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv("EMAIL_DISABLED", "") // email on, so its URL is checked
			t.Setenv("EMAIL_SERVICE_URL", "https://email.example")
			t.Setenv("EMAIL_TEST_RECIPIENTS", "qa@wso2.com")
			t.Setenv("OUTBOUND_CLIENT_ID", "id")
			t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
			t.Setenv(key, "http://svc.example/x")
			c, _ := Load()
			if err := c.Validate(); err == nil || !strings.Contains(err.Error(), key+" must be an https URL") {
				t.Errorf("Validate() = %v", err)
			}
			t.Setenv(key, "http://localhost:9090/x")
			c, _ = Load()
			if err := c.Validate(); err != nil {
				t.Errorf("localhost: Validate() = %v", err)
			}
		})
	}
}

func TestValidateGatewayPair(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("GATEWAY_JWT_ISSUER", "https://gw.example/token")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "together") {
		t.Errorf("Validate() = %v", err)
	}
	t.Setenv("GATEWAY_JWT_JWKS_URL", "https://gw.example/jwks")
	c, _ = Load()
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}

func TestValidateOutboundServicesNeedClient(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("SALES_ENTITY_BASE_URL", "https://sales.example")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_CLIENT_ID") {
		t.Errorf("Validate() = %v", err)
	}
	t.Setenv("OUTBOUND_CLIENT_ID", "id")
	t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
	c, _ = Load()
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}

// The Engagement Entity is in another Choreo project: a deployed one is
// reached over https with the subscribed client pair; only a local one is
// called without a token.
func TestValidateEntityNeedsClientUnlessLocal(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", "https://entity.example/engagement")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ENGAGEMENT_ENTITY_BASE_URL") {
		t.Errorf("remote entity without client: Validate() = %v", err)
	}
	t.Setenv("OUTBOUND_CLIENT_ID", "id")
	t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
	c, _ = Load()
	if err := c.Validate(); err != nil {
		t.Errorf("remote entity with client: Validate() = %v", err)
	}

	t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", "http://entity.example/engagement")
	c, _ = Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ENGAGEMENT_ENTITY_BASE_URL must be an https URL") {
		t.Errorf("plain http entity: Validate() = %v", err)
	}

	for _, local := range []string{"http://localhost:8290", "http://127.0.0.1:8290", "http://[::1]:8290"} {
		setMinimalEnv(t)
		t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", local)
		c, _ = Load()
		if err := c.Validate(); err != nil {
			t.Errorf("local entity %s: Validate() = %v", local, err)
		}
	}
}

// One outbound pair serves every service, the currency service included;
// the old per-service CURRENCY_CONVERT_CLIENT_* pair is no longer read.
func TestCurrencyUsesTheOutboundPair(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("CURRENCY_CONVERT_SERVICE_URL", "https://currency.example")
	t.Setenv("CURRENCY_CONVERT_CLIENT_ID", "old-id")
	t.Setenv("CURRENCY_CONVERT_CLIENT_SECRET", "old-secret")
	c, _ := Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OUTBOUND_CLIENT_ID and OUTBOUND_CLIENT_SECRET are required when CURRENCY_CONVERT_SERVICE_URL is set") {
		t.Errorf("without the outbound pair: Validate() = %v", err)
	}
	t.Setenv("OUTBOUND_CLIENT_ID", "id")
	t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
	c, _ = Load()
	if err := c.Validate(); err != nil {
		t.Errorf("with the outbound pair: Validate() = %v", err)
	}
	if !reflect.DeepEqual(c.Currency.Client, c.Client) || c.Currency.Client.ClientID != "id" {
		t.Errorf("currency client = %+v, outbound = %+v", c.Currency.Client, c.Client)
	}
}

// OUTBOUND_SCOPES: commas or spaces, trimmed, de-duplicated; empty is none.
func TestLoadOutboundScopes(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil || c.Client.Scopes != nil {
		t.Fatalf("unset: %v, %v", c.Client.Scopes, err)
	}
	t.Setenv("OUTBOUND_SCOPES", " engagement:read, engagement:write  engagement:read,,\tengagement:finance ")
	if c, err = Load(); err != nil {
		t.Fatal(err)
	}
	want := []string{"engagement:read", "engagement:write", "engagement:finance"}
	if !reflect.DeepEqual(c.Client.Scopes, want) || !reflect.DeepEqual(c.Currency.Client.Scopes, want) {
		t.Errorf("scopes = %v, currency %v", c.Client.Scopes, c.Currency.Client.Scopes)
	}
}

func TestLoadReportMaxConcurrent(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil || c.ReportMaxConcurrent != 2 {
		t.Fatalf("default: %v, %v", c, err)
	}
	t.Setenv("REPORT_MAX_CONCURRENT", "4")
	if c, err = Load(); err != nil || c.ReportMaxConcurrent != 4 {
		t.Errorf("4: %v, %v", c.ReportMaxConcurrent, err)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		t.Setenv("REPORT_MAX_CONCURRENT", bad)
		if _, err := Load(); err == nil {
			t.Errorf("REPORT_MAX_CONCURRENT=%s: want error", bad)
		}
	}
}

func TestLoadParsesLists(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JWT_AUDIENCES", " client-a , ,client-b ")
	c, _ := Load()
	if got := strings.Join(c.JWT.Audiences, "|"); got != "client-a|client-b" {
		t.Errorf("Audiences = %q", got)
	}
}

func TestLoadRejectsMalformedSkew(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JWT_CLOCK_SKEW_SECONDS", "1m")
	if _, err := Load(); err == nil {
		t.Error("Load accepted JWT_CLOCK_SKEW_SECONDS=1m")
	}
}

func TestLoadEmailAndFiles(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("EMAIL_COMPANY_RECIPIENTS", `{"WSO2 - UK": "a@wso2.com, b@wso2.com"}`)
	t.Setenv("EMAIL_INTERNAL_APPROVERS", "x@wso2.com,y@wso2.com")
	t.Setenv("FILE_STORAGE_ACCOUNT", "acct")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Email.CompanyRecipients["WSO2 - UK"]; len(got) != 2 || got[1] != "b@wso2.com" {
		t.Errorf("company recipients = %v", got)
	}
	if len(c.Email.InternalApprovers) != 2 {
		t.Errorf("approvers = %v", c.Email.InternalApprovers)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "FILE_STORAGE_ACCESS_KEY") {
		t.Errorf("Validate() = %v", err)
	}
	t.Setenv("EMAIL_COMPANY_RECIPIENTS", "not json")
	if _, err := Load(); err == nil {
		t.Error("accepted malformed EMAIL_COMPANY_RECIPIENTS")
	}
}

// Emails are not live by default: with email on, staging must name its test
// recipients or the backend won't start, so it can never mail real people.
// IS_LIVE_EMAILS=true (production) sends to the real recipients.
func TestValidateEmailsLiveOrTestRecipients(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("EMAIL_DISABLED", "")
	t.Setenv("EMAIL_SERVICE_URL", "https://email.example")
	t.Setenv("OUTBOUND_CLIENT_ID", "id")
	t.Setenv("OUTBOUND_CLIENT_SECRET", "secret")
	c, _ := Load()
	if c.Email.Live {
		t.Fatal("IS_LIVE_EMAILS unset: want not live")
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "EMAIL_TEST_RECIPIENTS is required") {
		t.Errorf("not live, no test recipients: Validate() = %v", err)
	}
	t.Setenv("EMAIL_TEST_RECIPIENTS", " qa@wso2.com, ,lead@wso2.com ")
	if c, _ = Load(); c.Validate() != nil || !reflect.DeepEqual(c.Email.TestRecipients, []string{"qa@wso2.com", "lead@wso2.com"}) {
		t.Errorf("test recipients = %v, Validate() = %v", c.Email.TestRecipients, c.Validate())
	}
	t.Setenv("EMAIL_TEST_RECIPIENTS", "qa@wso2.com,not an address")
	if c, _ = Load(); c.Validate() == nil {
		t.Error("an invalid test recipient was accepted")
	}
	t.Setenv("EMAIL_TEST_RECIPIENTS", "")
	t.Setenv("IS_LIVE_EMAILS", "true")
	if c, _ = Load(); !c.Email.Live || c.Validate() != nil {
		t.Errorf("live: Live=%v Validate()=%v", c.Email.Live, c.Validate())
	}
	t.Setenv("IS_LIVE_EMAILS", "yes please")
	if _, err := Load(); err == nil {
		t.Error("IS_LIVE_EMAILS=yes please: want error")
	}
}

// ACCESS_REQUEST_EMAIL defaults to the team address, is trimmed, and must be a plain address.
func TestAccessRequestEmail(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil || c.Validate() != nil || c.AccessRequestEmail != DefaultAccessRequestEmail {
		t.Fatalf("unset: %q, %v, %v", c.AccessRequestEmail, err, c.Validate())
	}
	t.Setenv("ACCESS_REQUEST_EMAIL", " wso2-digital-team@wso2.com ")
	c, _ = Load()
	if c.Validate() != nil || c.AccessRequestEmail != "wso2-digital-team@wso2.com" {
		t.Errorf("valid: %q, %v", c.AccessRequestEmail, c.Validate())
	}
	for _, bad := range []string{"not-an-email", "Team <team@wso2.com>", "a@b.com, c@d.com"} {
		t.Setenv("ACCESS_REQUEST_EMAIL", bad)
		c, _ = Load()
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ACCESS_REQUEST_EMAIL") {
			t.Errorf("%q: Validate() = %v, want an ACCESS_REQUEST_EMAIL error", bad, err)
		}
	}
}

// DASHBOARD_CACHE_TTL defaults to one hour, takes a Go duration, 0 turns the
// cache off, and a negative or unparsable value stops startup.
func TestLoadDashboardCacheTTL(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil || c.DashboardCacheTTL != time.Hour || DefaultDashboardCacheTTL != time.Hour {
		t.Fatalf("default = %v, %v; want 1h", c.DashboardCacheTTL, err)
	}
	for in, want := range map[string]time.Duration{"30m": 30 * time.Minute, "6h": 6 * time.Hour, "90s": 90 * time.Second, "0": 0, "0s": 0} {
		t.Setenv("DASHBOARD_CACHE_TTL", in)
		c, err := Load()
		if err != nil || c.DashboardCacheTTL != want {
			t.Errorf("DASHBOARD_CACHE_TTL=%q: got %v, %v; want %v", in, c.DashboardCacheTTL, err, want)
		}
	}
	for _, bad := range []string{"-1h", "soon", "5", "1d"} {
		t.Setenv("DASHBOARD_CACHE_TTL", bad)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DASHBOARD_CACHE_TTL") {
			t.Errorf("DASHBOARD_CACHE_TTL=%q: err = %v, want a startup error", bad, err)
		}
	}
}

// Travel budgets are off by default, with no recipients and a 10 minute cache.
func TestLoadBudgetDefaults(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Budgets.Enabled || len(c.Budgets.LineRecipients) != 0 || c.Budgets.CacheTTL != DefaultBudgetsCacheTTL {
		t.Errorf("Budgets = %+v, want off, no recipients, %v", c.Budgets, DefaultBudgetsCacheTTL)
	}
}

func TestLoadBudgetSettings(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("BUDGETS_ENABLED", "true")
	t.Setenv("BUDGET_LINE_RECIPIENTS", "a@example.com, b@example.com")
	t.Setenv("BUDGETS_CACHE_TTL", "0")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !c.Budgets.Enabled || !slices.Equal(c.Budgets.LineRecipients, []string{"a@example.com", "b@example.com"}) || c.Budgets.CacheTTL != 0 {
		t.Errorf("Budgets = %+v", c.Budgets)
	}

	t.Setenv("BUDGET_LINE_RECIPIENTS", "not-an-address")
	c, _ = Load()
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "BUDGET_LINE_RECIPIENTS") {
		t.Errorf("Validate() = %v, want a BUDGET_LINE_RECIPIENTS error", err)
	}
	t.Setenv("BUDGETS_ENABLED", "perhaps")
	if _, err := Load(); err == nil {
		t.Error("Load accepted BUDGETS_ENABLED=perhaps")
	}
	t.Setenv("BUDGETS_ENABLED", "")
	t.Setenv("BUDGETS_CACHE_TTL", "-1m")
	if _, err := Load(); err == nil {
		t.Error("Load accepted a negative BUDGETS_CACHE_TTL")
	}
}
