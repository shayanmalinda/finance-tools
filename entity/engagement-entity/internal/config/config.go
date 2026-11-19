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

// Package config loads settings from environment variables; the database
// host, user and password and the gateway JWT settings (or AUTH_DISABLED for
// a local run) are required.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/auth"
)

// Legacy schema names. They are the same in every environment, so they are
// constants rather than settings.
const (
	AllocationSchema = "allocation_db"
	TravelSchema     = "travelapp"
	ExpenseSchema    = "expense_app"
)

// Config holds all environment-driven settings for the engagement entity service.
type Config struct {
	// DB is the one MySQL user the entity uses for every legacy schema:
	// allocation_db and travelapp read/write, expense_app read-only (the
	// expense pool is opened read-only whatever the user is granted).
	DB DBConfig

	// NetSuite is used only when NetSuite.PushEnabled is true.
	NetSuite NetSuiteConfig

	// Auth is how callers are admitted (see package auth).
	Auth AuthConfig

	// AuditOptional lets the entity start without the audit_event tables
	// (AUDIT_OPTIONAL, local runs only); otherwise startup fails without them.
	AuditOptional bool

	ServerPort string
}

// DefaultGatewayIssuer is the iss of the JWT the Choreo gateway forwards.
const DefaultGatewayIssuer = "wso2.org/products/am"

// AuthConfig is the gateway JWT check. Disabled is for a local run only: the
// server then listens on loopback (ListenAddr), so a deployment that sets it
// by mistake is unreachable rather than open.
type AuthConfig struct {
	Disabled         bool
	Issuer           string
	JWKSURL          string
	AllowedClientIDs []string
	// ClientIDClaims (CLIENT_ID_CLAIMS) are the claims naming the calling
	// application, in order; empty uses the auth package's default.
	ClientIDClaims []string
}

// ListenAddr is where the server listens: every interface, or only
// loopback when auth is disabled.
func (c *Config) ListenAddr() string {
	if c.Auth.Disabled {
		return net.JoinHostPort("127.0.0.1", c.ServerPort)
	}
	return ":" + c.ServerPort
}

// DBConfig is how the entity reaches MySQL.
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	// TLS is the go-sql-driver tls= value. Empty means verified TLS
	// (tls=true), as in the GRC compliance entity. A local MySQL without TLS
	// sets DB_TLS=false; skip-verify is for non-production only.
	TLS string
	// TLSCA is a PEM CA bundle (DB_TLS_CA, mounted as a file) that signs the
	// MySQL server certificate, for a server whose CA is internal.
	TLSCA string
	// TLSServerName is the name the certificate must carry
	// (DB_TLS_SERVER_NAME), for connecting by IP to a server whose
	// certificate names a host.
	TLSServerName string
}

// CustomTLS reports whether DB_TLS_CA or DB_TLS_SERVER_NAME asks for a
// verified TLS config of our own.
func (d DBConfig) CustomTLS() bool { return d.TLSCA != "" || d.TLSServerName != "" }

// DSN returns the go-sql-driver DSN for schema. The db package normalises it
// further (UTC, TLS default, read-only session).
// The DSN is built by the driver, so a password with "@", "/" or ":" in it
// needs no escaping.
func (d DBConfig) DSN(schema string) string {
	c := mysql.NewConfig()
	c.User = d.User
	c.Passwd = d.Password
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(d.Host, strconv.Itoa(d.Port))
	c.DBName = schema
	c.ParseTime = true
	c.TLSConfig = d.TLS
	return c.FormatDSN()
}

// NetSuiteConfig is the NetSuite account the entity pushes invoices
// (Purchase Orders) and per diem payments (Vendor Bills) to, and the legacy
// Travel app's RESTlets. Every field except PushEnabled is read only when
// PushEnabled is true.
type NetSuiteConfig struct {
	// PushEnabled switches pushing to NetSuite (and the lookups that guard
	// each push and re-sync) on. Off by default, so the entity runs without
	// NetSuite credentials; a push request then answers 503.
	PushEnabled bool

	AccountID      string
	ConsumerKey    string
	ConsumerSecret string
	TokenID        string
	TokenSecret    string
	// RestletsBaseURL defaults to https://<account>.restlets.api.netsuite.com.
	RestletsBaseURL string

	// RESTlet script ids and deployment ids (deployment default 1) of the
	// legacy createPurchaseOrder, getPurchaseOrder, insertPerDiemPayments and
	// getVendorBill.
	CreatePurchaseOrderScriptID string
	CreatePurchaseOrderDeployID string
	GetPurchaseOrderScriptID    string
	GetPurchaseOrderDeployID    string
	CreateVendorBillScriptID    string
	CreateVendorBillDeployID    string
	GetVendorBillScriptID       string
	GetVendorBillDeployID       string
	// CreateMarketingEvent* is the legacy createMarketingEvent RESTlet,
	// called when a Marketing Event engagement is created.
	CreateMarketingEventScriptID string
	CreateMarketingEventDeployID string

	// PurchaseOrderTaxCode is sent on every purchase order (legacy
	// [app_travel.operation] taxCode).
	PurchaseOrderTaxCode string
}

// Load reads configuration from environment variables. Malformed values are
// returned as errors rather than silently defaulted.
func Load() (*Config, error) {
	port := 3306
	if v := strings.TrimSpace(os.Getenv("DB_PORT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return nil, errors.New("DB_PORT must be a port number")
		}
		port = n
	}
	push := false
	if v := strings.TrimSpace(os.Getenv("NETSUITE_PUSH_ENABLED")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("NETSUITE_PUSH_ENABLED must be true or false")
		}
		push = b
	}
	authOff := false
	if v := strings.TrimSpace(os.Getenv("AUTH_DISABLED")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("AUTH_DISABLED must be true or false")
		}
		authOff = b
	}
	auditOptional := false
	if v := strings.TrimSpace(os.Getenv("AUDIT_OPTIONAL")); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("AUDIT_OPTIONAL must be true or false")
		}
		auditOptional = b
	}
	var allowed []string
	for _, id := range strings.Split(os.Getenv("ALLOWED_CLIENT_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			allowed = append(allowed, id)
		}
	}
	var claims []string
	for _, c := range strings.Split(os.Getenv("CLIENT_ID_CLAIMS"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			claims = append(claims, c)
		}
	}
	ns := NetSuiteConfig{PushEnabled: push}
	if push {
		ns.AccountID = strings.TrimSpace(os.Getenv("NETSUITE_ACCOUNT_ID"))
		ns.ConsumerKey = os.Getenv("NETSUITE_CONSUMER_KEY")
		ns.ConsumerSecret = os.Getenv("NETSUITE_CONSUMER_SECRET")
		ns.TokenID = os.Getenv("NETSUITE_TOKEN_ID")
		ns.TokenSecret = os.Getenv("NETSUITE_TOKEN_SECRET")
		ns.RestletsBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("NETSUITE_RESTLETS_BASE_URL")), "/")
		if ns.RestletsBaseURL == "" && ns.AccountID != "" {
			ns.RestletsBaseURL = "https://" + strings.ToLower(strings.ReplaceAll(ns.AccountID, "_", "-")) + ".restlets.api.netsuite.com"
		}
		ns.CreatePurchaseOrderScriptID = strings.TrimSpace(os.Getenv("NETSUITE_CREATE_PURCHASE_ORDER_SCRIPT_ID"))
		ns.CreatePurchaseOrderDeployID = getEnvOrDefault("NETSUITE_CREATE_PURCHASE_ORDER_DEPLOY_ID", "1")
		ns.GetPurchaseOrderScriptID = strings.TrimSpace(os.Getenv("NETSUITE_GET_PURCHASE_ORDER_SCRIPT_ID"))
		ns.GetPurchaseOrderDeployID = getEnvOrDefault("NETSUITE_GET_PURCHASE_ORDER_DEPLOY_ID", "1")
		ns.CreateVendorBillScriptID = strings.TrimSpace(os.Getenv("NETSUITE_CREATE_VENDOR_BILL_SCRIPT_ID"))
		ns.CreateVendorBillDeployID = getEnvOrDefault("NETSUITE_CREATE_VENDOR_BILL_DEPLOY_ID", "1")
		ns.GetVendorBillScriptID = strings.TrimSpace(os.Getenv("NETSUITE_GET_VENDOR_BILL_SCRIPT_ID"))
		ns.GetVendorBillDeployID = getEnvOrDefault("NETSUITE_GET_VENDOR_BILL_DEPLOY_ID", "1")
		ns.CreateMarketingEventScriptID = strings.TrimSpace(os.Getenv("NETSUITE_CREATE_MARKETING_EVENT_SCRIPT_ID"))
		ns.CreateMarketingEventDeployID = getEnvOrDefault("NETSUITE_CREATE_MARKETING_EVENT_DEPLOY_ID", "1")
		ns.PurchaseOrderTaxCode = strings.TrimSpace(os.Getenv("NETSUITE_PURCHASE_ORDER_TAX_CODE"))
	}
	return &Config{
		DB: DBConfig{
			Host:     strings.TrimSpace(os.Getenv("DB_HOST")),
			Port:     port,
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			TLS:      strings.TrimSpace(os.Getenv("DB_TLS")),

			TLSCA:         strings.TrimSpace(os.Getenv("DB_TLS_CA")),
			TLSServerName: strings.TrimSpace(os.Getenv("DB_TLS_SERVER_NAME")),
		},
		NetSuite:      ns,
		AuditOptional: auditOptional,
		Auth: AuthConfig{
			Disabled:         authOff,
			Issuer:           getEnvOrDefault("GATEWAY_JWT_ISSUER", DefaultGatewayIssuer),
			JWKSURL:          strings.TrimSpace(os.Getenv("GATEWAY_JWT_JWKS_URL")),
			AllowedClientIDs: allowed,
			ClientIDClaims:   claims,
		},
		ServerPort: getEnvOrDefault("SERVER_PORT", "8290"),
	}, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

// Validate returns an error naming every required setting that is missing.
func (c *Config) Validate() error {
	var errs []error
	if c.DB.Host == "" {
		errs = append(errs, errors.New("DB_HOST is required"))
	}
	if c.DB.User == "" {
		errs = append(errs, errors.New("DB_USER is required"))
	}
	if c.DB.Password == "" {
		errs = append(errs, errors.New("DB_PASSWORD is required"))
	}
	if c.DB.CustomTLS() && c.DB.TLS != "" && c.DB.TLS != "true" {
		errs = append(errs, errors.New("DB_TLS must be empty or true when DB_TLS_CA or DB_TLS_SERVER_NAME is set"))
	}
	if !c.Auth.Disabled {
		if c.Auth.JWKSURL == "" {
			errs = append(errs, errors.New("GATEWAY_JWT_JWKS_URL is required (or AUTH_DISABLED=true for a local run)"))
		} else if u, err := url.Parse(c.Auth.JWKSURL); err != nil || !auth.SecureOrLoopback(u) {
			errs = append(errs, errors.New("GATEWAY_JWT_JWKS_URL must be an https URL (http only on localhost)"))
		}
		if len(c.Auth.AllowedClientIDs) == 0 {
			errs = append(errs, errors.New("ALLOWED_CLIENT_IDS is required (the Engagement App Backend's client id)"))
		}
	}
	if n := c.NetSuite; n.PushEnabled {
		for _, kv := range []struct{ key, val string }{
			{"NETSUITE_ACCOUNT_ID", n.AccountID},
			{"NETSUITE_CONSUMER_KEY", n.ConsumerKey},
			{"NETSUITE_CONSUMER_SECRET", n.ConsumerSecret},
			{"NETSUITE_TOKEN_ID", n.TokenID},
			{"NETSUITE_TOKEN_SECRET", n.TokenSecret},
			{"NETSUITE_CREATE_PURCHASE_ORDER_SCRIPT_ID", n.CreatePurchaseOrderScriptID},
			{"NETSUITE_GET_PURCHASE_ORDER_SCRIPT_ID", n.GetPurchaseOrderScriptID},
			{"NETSUITE_CREATE_VENDOR_BILL_SCRIPT_ID", n.CreateVendorBillScriptID},
			{"NETSUITE_GET_VENDOR_BILL_SCRIPT_ID", n.GetVendorBillScriptID},
			{"NETSUITE_CREATE_MARKETING_EVENT_SCRIPT_ID", n.CreateMarketingEventScriptID},
			{"NETSUITE_PURCHASE_ORDER_TAX_CODE", n.PurchaseOrderTaxCode},
		} {
			if kv.val == "" {
				errs = append(errs, fmt.Errorf("%s is required when NETSUITE_PUSH_ENABLED=true", kv.key))
			}
		}
	}
	return errors.Join(errs...)
}
