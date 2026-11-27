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

package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/filestore"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/middleware"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/oidc"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/server"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/service"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	verifier, err := newVerifier(cfg.JWT)
	if err != nil {
		log.Fatalf("JWT verifier: %v", err)
	}
	if cfg.JWT.GatewayIssuer == "" {
		slog.Info("GATEWAY_JWT_ISSUER is not set: only tokens issued by JWT_ISSUER are accepted (also in X-Jwt-Assertion)")
	}

	// One client-credentials pair for every outbound WSO2 service, as the
	// legacy Allocations backend does.
	var oauth *http.Client
	if cfg.Client.Configured() {
		oauth = rest.NewOAuthHTTPClient(cfg.Client.TokenURL, cfg.Client.ClientID, cfg.Client.ClientSecret, cfg.Client.Scopes)
	}

	var sales salesentity.Client = salesentity.Unconfigured{}
	if cfg.SalesEntityBaseURL != "" {
		sales = salesentity.New(cfg.SalesEntityBaseURL, oauth)
	} else {
		slog.Warn("SALES_ENTITY_BASE_URL is not set: accounts, opportunities and campaigns answer 503")
	}
	var hr hrentity.Client = hrentity.Unconfigured{}
	if cfg.HREntityURL != "" {
		hr = hrentity.New(cfg.HREntityURL, oauth)
	} else {
		slog.Warn("HR_ENTITY_URL is not set: employee and organisation lookups answer 503")
	}

	var files filestore.Store = filestore.Unconfigured{}
	if f := cfg.Files; f.Enabled() {
		az, err := filestore.NewAzure(f.Account, f.AccessKey, "", nil)
		if err != nil {
			log.Fatalf("file storage: %v", err)
		}
		files = az
	} else {
		slog.Warn("FILE_STORAGE_ACCOUNT is not set: passport and visa image uploads and downloads answer 503")
	}

	// The entity is in another Choreo project, behind its Organization
	// endpoint, so it takes the subscribed client pair. A NetSuite push can
	// run for minutes, hence the longer timeout.
	var entityHTTP *http.Client
	if oauth != nil {
		c := *oauth
		c.Timeout = engagemententity.PushTimeout
		entityHTTP = &c
	}
	entity := engagemententity.New(cfg.EngagementEntityBaseURL, entityHTTP)

	// Every send attempt is logged (Sent or Failed) in the entity's
	// email_auditlog. With emails disabled the LogSender sends nothing, so
	// nothing is logged.
	var sender email.Sender = email.LogSender{}
	if e := cfg.Email; e.Enabled() {
		sender = email.New(email.Config{
			ServiceURL:     e.ServiceURL,
			From:           map[email.App]string{email.AppAllocations: e.AllocationsFrom, email.AppTravel: e.TravelFrom},
			TestRecipients: testRecipients(e),
			Suppressed:     e.Suppressed,
		}, rest.New("email", e.ServiceURL, oauth)).WithAudit(service.EmailAuditor(entity, nil))
	} else {
		slog.Warn("EMAIL_DISABLED is set: emails are logged, not sent")
	}

	handler := server.NewRouter(server.Deps{
		Verifier:            verifier,
		Userinfo:            oidc.NewUserinfoClient(cfg.JWT.UserinfoURL, nil),
		Entity:              entity,
		Sales:               sales,
		HR:                  hr,
		Email:               sender,
		Files:               files,
		Recipients:          cfg.Email,
		Travel:              cfg.Travel,
		Currency:            cfg.Currency,
		CORSAllowedOrigins:  cfg.CORSAllowedOrigins,
		ReportMaxConcurrent: cfg.ReportMaxConcurrent,
		DashboardCacheTTL:   cfg.DashboardCacheTTL,
		Budgets:             cfg.Budgets,
		AccessRequestEmail:  cfg.AccessRequestEmail,
	})
	srv := server.New(":"+cfg.ServerPort, handler)

	go func() {
		log.Printf("Engagement App Backend started on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Wait as long as the longest route may run, so a NetSuite push in
	// progress finishes rather than stopping mid-batch. The platform's own
	// termination grace period still caps this.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), server.PushTimeout+10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	if err := middleware.WaitAudits(shutdownCtx); err != nil {
		log.Printf("audit writes still pending at shutdown: %v", err)
	}
	log.Println("server stopped")
}

// newVerifier trusts the Asgardeo issuer (Authorization: Bearer access
// tokens) and, when configured, the Choreo gateway's issuer (X-Jwt-Assertion).
func newVerifier(c config.JWTConfig) (*oidc.MultiVerifier, error) {
	asgardeo, err := oidc.NewVerifier(oidc.Config{
		Issuer: c.Issuer, JWKSURL: c.JWKSURL, Audiences: c.Audiences, ClockSkew: c.ClockSkew,
	}, nil)
	if err != nil {
		return nil, err
	}
	vs := []*oidc.Verifier{asgardeo}
	if c.GatewayIssuer != "" {
		gw, err := oidc.NewVerifier(oidc.Config{
			Issuer: c.GatewayIssuer, JWKSURL: c.GatewayJWKSURL, Audiences: c.GatewayAudiences, ClockSkew: c.ClockSkew,
		}, nil)
		if err != nil {
			return nil, err
		}
		vs = append(vs, gw)
	}
	return oidc.NewMultiVerifier(vs...)
}

// testRecipients are where every email goes when emails are not live (the
// default outside production); live, nil sends to the real recipients.
func testRecipients(e config.EmailConfig) []string {
	if e.Live {
		return nil
	}
	return e.TestRecipients
}
