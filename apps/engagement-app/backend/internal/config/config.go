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

// Package config loads settings from environment variables; only secrets and
// environment-specific values must be set.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
)

// WSO2 Asgardeo organisation endpoints. Public, the same in every
// environment, so they are defaults rather than required settings.
const (
	DefaultIssuer      = "https://api.asgardeo.io/t/wso2/oauth2/token"
	DefaultJWKSURL     = "https://api.asgardeo.io/t/wso2/oauth2/jwks"
	DefaultUserinfoURL = "https://api.asgardeo.io/t/wso2/oauth2/userinfo"
	DefaultTokenURL    = "https://api.asgardeo.io/t/wso2/oauth2/token"
)

// Config holds all environment-driven settings for the Engagement App Backend.
type Config struct {
	ServerPort string

	JWT JWTConfig

	// EngagementEntityBaseURL is the Engagement Entity's project-internal URL.
	// The entity has no auth of its own (Project visibility), so no
	// credentials are needed.
	EngagementEntityBaseURL string

	// Choreo connections; unset, calls to that service answer 503.
	SalesEntityBaseURL string
	HREntityURL        string

	// Client is the one OAuth2 client-credentials pair the backend uses for
	// every outbound WSO2 service (Sales Entity, HR Entity, email), as the
	// legacy Allocations backend does with app_allocation.entity.clientAuthConfig.
	Client OAuthClient

	Email  EmailConfig
	Files  FileConfig
	Travel TravelConfig
	// Currency is the legacy currency conversion service (per diem payments).
	Currency CurrencyConfig

	// CORSAllowedOrigins lists the webapp origins allowed to call this backend
	// from a browser. Empty means none (never a wildcard). Behind the Choreo
	// gateway CORS is the gateway's; locally set http://localhost:3000.
	CORSAllowedOrigins []string
}

// JWTConfig is how the caller's token is verified. There is no switch that
// turns verification off.
type JWTConfig struct {
	// Issuer and JWKSURL verify the Asgardeo access token One WSO2 sends as
	// Authorization: Bearer.
	Issuer  string
	JWKSURL string
	// UserinfoURL is asked for the email and groups an access token omits.
	UserinfoURL string
	// GatewayIssuer and GatewayJWKSURL additionally trust the Choreo
	// gateway's JWT (X-Jwt-Assertion) when it is signed by another issuer.
	// Both or neither.
	GatewayIssuer  string
	GatewayJWKSURL string
	// Audiences, when set, must contain one of the token's aud values.
	Audiences []string
	// ClockSkew is the leeway allowed on exp, nbf and iat.
	ClockSkew time.Duration
}

// OAuthClient is an OAuth2 client-credentials pair.
type OAuthClient struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
}

// Configured reports whether the pair is complete.
func (c OAuthClient) Configured() bool { return c.ClientID != "" && c.ClientSecret != "" }

// EmailConfig is the email service and the legacy recipient settings; all optional.
type EmailConfig struct {
	// ServiceURL unset: emails are logged and not sent.
	ServiceURL string
	// AppBaseURL is the One WSO2 link written into emails.
	AppBaseURL string

	AllocationsAppUUID string
	TravelAppUUID      string
	AllocationsFrom    string
	TravelFrom         string
	// TestRecipient set: every email goes to it alone (testing mode).
	TestRecipient string
	// Suppressed addresses never receive email (noEmailsTo).
	Suppressed []string
	// CCTraveller copies the traveller on travel request emails
	// (isSendEmailToTraveller).
	CCTraveller bool

	AllocationsGroup        string
	PartnerAllocationsGroup string
	// TravelGroup is the Allocations app's travelGroupEmail.
	TravelGroup          string
	InternalApprovers    []string
	MarketingApprovers   []string
	TravelLogisticsGroup string
	FinanceGroup         string
	// CompanyRecipients maps an HR company name to the finance addresses of
	// that subsidiary (companyEmailRecipientsMapping).
	CompanyRecipients map[string][]string
}

// Enabled reports whether emails are sent.
func (e EmailConfig) Enabled() bool { return e.ServiceURL != "" }

// CurrencyConfig is the legacy currency conversion service
// ([app_travel.utils] currencyConvertServiceEndpoint, POST /convert-currency).
// Unset URL: adding or editing a per diem payment answers 503.
type CurrencyConfig struct {
	URL string
	// Client defaults to the shared client-credentials pair; the legacy
	// Travel backend used its own [app_travel.utils.clientAuthConfig], which
	// can be set here when the shared pair is not subscribed to the service.
	Client OAuthClient
}

// Configured reports whether conversions can be made.
func (c CurrencyConfig) Configured() bool { return c.URL != "" && c.Client.Configured() }

// TravelConfig holds legacy Travel data ids that differ per environment.
type TravelConfig struct {
	// VisaNotApplicableID and NoValidVisaID are the visa_status rows the
	// legacy app uses as the "Visa Not Applicable" and "No Valid Visa
	// Available" choices (production 332 and 333; staging 40 and 41).
	VisaNotApplicableID int
	NoValidVisaID       int
}

// FileConfig is the Azure Blob Storage account the legacy Travel app keeps
// visa and travel document images in.
type FileConfig struct {
	// Account unset: uploads and downloads answer 503.
	Account   string
	AccessKey string
}

// Enabled reports whether file storage is configured.
func (f FileConfig) Enabled() bool { return f.Account != "" }

// Load reads configuration from environment variables. Malformed numbers are
// errors, not silently defaulted.
func Load() (*Config, error) {
	skew, err := getSeconds("JWT_CLOCK_SKEW_SECONDS", 60)
	if err != nil {
		return nil, err
	}
	visaNA, err := getPositiveInt("TRAVEL_VISA_NOT_APPLICABLE_ID", 332)
	if err != nil {
		return nil, err
	}
	noVisa, err := getPositiveInt("TRAVEL_NO_VALID_VISA_ID", 333)
	if err != nil {
		return nil, err
	}
	ccTraveller := false
	if v := strings.TrimSpace(os.Getenv("EMAIL_CC_TRAVELLER")); v != "" {
		if ccTraveller, err = strconv.ParseBool(v); err != nil {
			return nil, errors.New("EMAIL_CC_TRAVELLER must be true or false")
		}
	}
	shared := OAuthClient{
		TokenURL:     getEnvOrDefault("ENTITY_TOKEN_URL", DefaultTokenURL),
		ClientID:     strings.TrimSpace(os.Getenv("ENTITY_CLIENT_ID")),
		ClientSecret: os.Getenv("ENTITY_CLIENT_SECRET"),
	}
	currency := CurrencyConfig{URL: trimURL(os.Getenv("CURRENCY_CONVERT_SERVICE_URL")), Client: shared}
	if id := strings.TrimSpace(os.Getenv("CURRENCY_CONVERT_CLIENT_ID")); id != "" {
		currency.Client = OAuthClient{
			TokenURL:     getEnvOrDefault("CURRENCY_CONVERT_TOKEN_URL", shared.TokenURL),
			ClientID:     id,
			ClientSecret: os.Getenv("CURRENCY_CONVERT_CLIENT_SECRET"),
		}
	}
	companies := map[string][]string{}
	if raw := strings.TrimSpace(os.Getenv("EMAIL_COMPANY_RECIPIENTS")); raw != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, errors.New(`EMAIL_COMPANY_RECIPIENTS must be a JSON object such as {"WSO2 - UK": "a@wso2.com,b@wso2.com"}`)
		}
		for company, list := range m {
			companies[company] = splitList(list)
		}
	}
	return &Config{
		ServerPort: getEnvOrDefault("SERVER_PORT", "8190"),
		JWT: JWTConfig{
			Issuer:         getEnvOrDefault("JWT_ISSUER", DefaultIssuer),
			JWKSURL:        getEnvOrDefault("JWT_JWKS_URL", DefaultJWKSURL),
			UserinfoURL:    getEnvOrDefault("JWT_USERINFO_URL", DefaultUserinfoURL),
			GatewayIssuer:  strings.TrimSpace(os.Getenv("GATEWAY_JWT_ISSUER")),
			GatewayJWKSURL: strings.TrimSpace(os.Getenv("GATEWAY_JWT_JWKS_URL")),
			Audiences:      getList("JWT_AUDIENCES"),
			ClockSkew:      skew,
		},
		EngagementEntityBaseURL: trimURL(os.Getenv("ENGAGEMENT_ENTITY_BASE_URL")),
		SalesEntityBaseURL:      trimURL(os.Getenv("SALES_ENTITY_BASE_URL")),
		HREntityURL:             strings.TrimSpace(os.Getenv("HR_ENTITY_URL")),
		Client:                  shared,
		Email: EmailConfig{
			ServiceURL:              trimURL(os.Getenv("EMAIL_SERVICE_URL")),
			AppBaseURL:              trimURL(getEnvOrDefault("APP_BASE_URL", "https://one.wso2.com")),
			AllocationsAppUUID:      strings.TrimSpace(os.Getenv("EMAIL_ALLOCATIONS_APP_UUID")),
			TravelAppUUID:           strings.TrimSpace(os.Getenv("EMAIL_TRAVEL_APP_UUID")),
			AllocationsFrom:         strings.TrimSpace(os.Getenv("EMAIL_ALLOCATIONS_FROM")),
			TravelFrom:              strings.TrimSpace(os.Getenv("EMAIL_TRAVEL_FROM")),
			TestRecipient:           strings.TrimSpace(os.Getenv("EMAIL_TEST_RECIPIENT")),
			Suppressed:              getList("EMAIL_SUPPRESSED"),
			CCTraveller:             ccTraveller,
			AllocationsGroup:        strings.TrimSpace(os.Getenv("EMAIL_ALLOCATIONS_GROUP")),
			PartnerAllocationsGroup: strings.TrimSpace(os.Getenv("EMAIL_PARTNER_ALLOCATIONS_GROUP")),
			TravelGroup:             strings.TrimSpace(os.Getenv("EMAIL_TRAVEL_GROUP")),
			InternalApprovers:       getList("EMAIL_INTERNAL_APPROVERS"),
			MarketingApprovers:      getList("EMAIL_MARKETING_APPROVERS"),
			TravelLogisticsGroup:    strings.TrimSpace(os.Getenv("EMAIL_TRAVEL_LOGISTICS_GROUP")),
			FinanceGroup:            strings.TrimSpace(os.Getenv("EMAIL_FINANCE_GROUP")),
			CompanyRecipients:       companies,
		},
		Files: FileConfig{
			Account:   strings.TrimSpace(os.Getenv("FILE_STORAGE_ACCOUNT")),
			AccessKey: strings.TrimSpace(os.Getenv("FILE_STORAGE_ACCESS_KEY")),
		},
		Travel:             TravelConfig{VisaNotApplicableID: visaNA, NoValidVisaID: noVisa},
		Currency:           currency,
		CORSAllowedOrigins: getList("CORS_ALLOWED_ORIGINS"),
	}, nil
}

// Validate returns an error naming every setting that is missing or invalid.
func (c *Config) Validate() error {
	var errs []error
	if c.JWT.Issuer == "" {
		errs = append(errs, errors.New("JWT_ISSUER must not be empty"))
	}
	if err := requireHTTPS("JWT_JWKS_URL", c.JWT.JWKSURL); err != nil {
		errs = append(errs, err)
	}
	if err := requireHTTPS("JWT_USERINFO_URL", c.JWT.UserinfoURL); err != nil {
		errs = append(errs, err)
	}
	if (c.JWT.GatewayIssuer == "") != (c.JWT.GatewayJWKSURL == "") {
		errs = append(errs, errors.New("GATEWAY_JWT_ISSUER and GATEWAY_JWT_JWKS_URL are set together"))
	} else if c.JWT.GatewayJWKSURL != "" {
		if err := requireHTTPS("GATEWAY_JWT_JWKS_URL", c.JWT.GatewayJWKSURL); err != nil {
			errs = append(errs, err)
		}
		if c.JWT.GatewayIssuer == c.JWT.Issuer {
			errs = append(errs, errors.New("GATEWAY_JWT_ISSUER must differ from JWT_ISSUER"))
		}
	}
	if c.EngagementEntityBaseURL == "" {
		errs = append(errs, errors.New("ENGAGEMENT_ENTITY_BASE_URL is required"))
	}
	needsClient := map[string]string{
		"SALES_ENTITY_BASE_URL": c.SalesEntityBaseURL,
		"HR_ENTITY_URL":         c.HREntityURL,
		"EMAIL_SERVICE_URL":     c.Email.ServiceURL,
	}
	for _, key := range []string{"SALES_ENTITY_BASE_URL", "HR_ENTITY_URL", "EMAIL_SERVICE_URL"} {
		if needsClient[key] != "" && !c.Client.Configured() {
			errs = append(errs, fmt.Errorf("ENTITY_CLIENT_ID and ENTITY_CLIENT_SECRET are required when %s is set", key))
			break
		}
	}
	if c.Currency.URL != "" && !c.Currency.Client.Configured() {
		errs = append(errs, errors.New("CURRENCY_CONVERT_SERVICE_URL needs ENTITY_CLIENT_ID/SECRET (or CURRENCY_CONVERT_CLIENT_ID/SECRET)"))
	}
	if c.Files.Enabled() && c.Files.AccessKey == "" {
		errs = append(errs, errors.New("FILE_STORAGE_ACCESS_KEY is required when FILE_STORAGE_ACCOUNT is set"))
	}
	return errors.Join(errs...)
}

func requireHTTPS(key, raw string) error {
	if raw == "" {
		return fmt.Errorf("%s must not be empty", key)
	}
	u, err := url.Parse(raw)
	if err != nil || !oidc.SecureOrLoopback(u) {
		return fmt.Errorf("%s must be an https URL (http only on localhost)", key)
	}
	return nil
}

func trimURL(v string) string { return strings.TrimRight(strings.TrimSpace(v), "/") }

func getEnvOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

// getList splits a comma-separated variable, dropping empty entries.
func getList(key string) []string { return splitList(os.Getenv(key)) }

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getSeconds(key string, def int) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(def) * time.Second, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative whole number of seconds", key)
	}
	return time.Duration(n) * time.Second, nil
}

func getPositiveInt(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive whole number", key)
	}
	return n, nil
}
