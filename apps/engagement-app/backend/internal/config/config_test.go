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
	"time"
)

var allKeys = []string{
	"SERVER_PORT", "JWT_ISSUER", "JWT_JWKS_URL", "JWT_USERINFO_URL", "GATEWAY_JWT_ISSUER", "GATEWAY_JWT_JWKS_URL",
	"JWT_AUDIENCES", "JWT_CLOCK_SKEW_SECONDS", "ENGAGEMENT_ENTITY_BASE_URL", "SALES_ENTITY_BASE_URL", "HR_ENTITY_URL",
	"ENTITY_TOKEN_URL", "ENTITY_CLIENT_ID", "ENTITY_CLIENT_SECRET", "EMAIL_SERVICE_URL", "APP_BASE_URL",
	"FILE_STORAGE_ACCOUNT", "FILE_STORAGE_ACCESS_KEY", "CORS_ALLOWED_ORIGINS",
	"EMAIL_COMPANY_RECIPIENTS", "EMAIL_INTERNAL_APPROVERS",
	"CURRENCY_CONVERT_SERVICE_URL",
}

func setMinimalEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
	t.Setenv("ENGAGEMENT_ENTITY_BASE_URL", "http://engagement-entity:8290/")
	t.Setenv("JWT_AUDIENCES", "client-a")
}

// The only required settings are the entity URL and the audience; everything
// else defaults.
func TestLoadMinimal(t *testing.T) {
	setMinimalEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.EngagementEntityBaseURL != "http://engagement-entity:8290" {
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
	for _, key := range []string{"ENTITY_TOKEN_URL", "SALES_ENTITY_BASE_URL", "HR_ENTITY_URL", "EMAIL_SERVICE_URL",
		"CURRENCY_CONVERT_SERVICE_URL"} {
		t.Run(key, func(t *testing.T) {
			setMinimalEnv(t)
			t.Setenv("ENTITY_CLIENT_ID", "id")
			t.Setenv("ENTITY_CLIENT_SECRET", "secret")
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
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "ENTITY_CLIENT_ID") {
		t.Errorf("Validate() = %v", err)
	}
	t.Setenv("ENTITY_CLIENT_ID", "id")
	t.Setenv("ENTITY_CLIENT_SECRET", "secret")
	c, _ = Load()
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
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
