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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/auth"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/netsuite"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/server"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Not the parser's error: it can quote the offending line, secret
		// value and all.
		log.Fatal("load .env: the file could not be read or parsed; check its syntax")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	switch cfg.DB.TLS {
	case "skip-verify":
		log.Println("WARNING: DB_TLS=skip-verify does not verify the MySQL certificate: non-production only. Use DB_TLS_CA (and DB_TLS_SERVER_NAME) instead")
	case "false":
		log.Println("WARNING: DB_TLS=false connects to MySQL without TLS: local only")
	}

	pools, err := db.New(cfg)
	if err != nil {
		log.Fatalf("connect to databases: %v", err)
	}
	defer pools.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	hasCampaign, err := repository.DetectEngagementCampaignColumn(ctx, pools.Allocation)
	if err != nil {
		cancel()
		log.Fatalf("read allocation_db schema: %v", err)
	}
	allocAudit, err := audit.TableExists(ctx, pools.Allocation)
	if err != nil {
		cancel()
		log.Fatalf("read allocation_db schema: %v", err)
	}
	travelAudit, err := audit.TableExists(ctx, pools.Travel)
	cancel()
	if err != nil {
		log.Fatalf("read travelapp schema: %v", err)
	}
	if err := audit.CheckTables(allocAudit, travelAudit, cfg.AuditOptional); err != nil {
		log.Fatalf("invalid database: %v", err)
	}
	audit.SetEnabled(allocAudit && travelAudit)
	if !audit.Enabled() {
		log.Println("WARNING: AUDIT_OPTIONAL is set and audit_event is missing: audit events are not recorded (local run only)")
	}
	repository.SetEngagementCampaignColumn(hasCampaign)
	if !hasCampaign {
		log.Println("WARNING: allocation_db.engagement.campaign_id is missing (run Resources/allocation_db_table_update_v1.0.0.sql): campaigns read as empty and Marketing Events cannot be saved with a campaign")
	}

	gate := auth.Off
	if cfg.Auth.Disabled {
		log.Println("WARNING: AUTH_DISABLED is set: no gateway JWT is checked and the server listens on loopback only (local run)")
	} else {
		v, err := auth.NewVerifier(auth.Config{
			Issuer:           cfg.Auth.Issuer,
			JWKSURL:          cfg.Auth.JWKSURL,
			AllowedClientIDs: cfg.Auth.AllowedClientIDs,
			ClientIDClaims:   cfg.Auth.ClientIDClaims,
			APIContexts:      cfg.Auth.APIContexts,
			APIContextClaims: cfg.Auth.APIContextClaims,
			ClockSkew:        60 * time.Second,
		}, nil)
		if err != nil {
			log.Fatalf("invalid configuration: %v", err)
		}
		if len(cfg.Auth.APIContexts) == 0 {
			log.Println("WARNING: GATEWAY_JWT_API_CONTEXTS is not set: gateway assertions are not bound to this API")
		}
		gate = auth.Middleware(v)
	}

	addr := cfg.ListenAddr()
	srv := server.New(addr, server.Deps{Pools: pools, NetSuite: cfg.NetSuite, Auth: gate})
	if !cfg.NetSuite.PushEnabled {
		log.Println("NetSuite push is disabled (NETSUITE_PUSH_ENABLED is not true): push and re-sync answer 503")
	}

	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go sweepStalePushes(sweepCtx, pools)

	go func() {
		log.Printf("Engagement Entity REST Service started on port %s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Println("server stopped")
}

const (
	// stalePushAge must stay above the longest a live push can run (the
	// settle context, 2*netsuite.Timeout plus a final write), or the sweep
	// would expire a push that is still going.
	stalePushAge = 3 * netsuite.Timeout
	// stalePushSweepEvery is how often the sweep looks for stale pushes.
	stalePushSweepEvery = time.Minute
)

// sweepStalePushes moves NetSuite pushes stuck for stalePushAge to NS Sync
// Failures.
func sweepStalePushes(ctx context.Context, pools *db.Pools) {
	t := time.NewTicker(stalePushSweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := repository.ExpireStalePushes(ctx, pools.Travel, stalePushAge); err != nil {
				log.Printf("expire stale NetSuite pushes: %v", err)
			}
		}
	}
}
