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
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/db"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/repository"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/server"
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
	audit.SetEnabled(allocAudit && travelAudit)
	if !audit.Enabled() {
		log.Println("WARNING: audit_event is missing in allocation_db or travelapp (run " +
			"allocation_db_table_update_v1.1.0.sql and travelapp_table_update_v1.0.0.sql): " +
			"audit events are not recorded")
	}
	repository.SetEngagementCampaignColumn(hasCampaign)
	if !hasCampaign {
		log.Println("WARNING: allocation_db.engagement.campaign_id is missing (run Resources/allocation_db_table_update_v1.0.0.sql): campaigns read as empty and Marketing Events cannot be saved with a campaign")
	}

	addr := ":" + cfg.ServerPort
	srv := server.New(addr, server.Deps{Pools: pools, NetSuite: cfg.NetSuite})
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

// sweepStalePushes moves NetSuite pushes stuck for 15 minutes (three times
// the NetSuite timeout) to NS Sync Failures.
func sweepStalePushes(ctx context.Context, pools *db.Pools) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := repository.ExpireStalePushes(ctx, pools.Travel, 15*time.Minute); err != nil {
				log.Printf("expire stale NetSuite pushes: %v", err)
			}
		}
	}
}
