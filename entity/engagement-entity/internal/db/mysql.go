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

// Package db manages the MySQL connection pools for the engagement entity service.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/config"
)

const (
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
	pingTimeout     = 10 * time.Second
)

// poolSize bounds one pool. allocation_db and travelapp carry every screen's
// traffic; expense_app (cost dashboard) is small.
type poolSize struct{ maxOpen, maxIdle int }

var (
	readWritePool = poolSize{maxOpen: 20, maxIdle: 5}
	smallPool     = poolSize{maxOpen: 5, maxIdle: 2}
)

// Pools holds one connection pool per legacy schema. Repositories take the
// pool of the schema they read; none of them crosses schemas.
type Pools struct {
	// Allocation is allocation_db (read/write).
	Allocation *sql.DB
	// Travel is travelapp (read/write).
	Travel *sql.DB
	// Expense is expense_app, opened read-only.
	Expense *sql.DB
}

// Close closes every open pool.
func (p *Pools) Close() {
	for _, d := range []*sql.DB{p.Allocation, p.Travel, p.Expense} {
		if d != nil {
			_ = d.Close()
		}
	}
}

// normalizeDSN pins sessions to UTC, defaults to verified TLS (as the GRC
// compliance entity) and makes read-only pools read-only.
func normalizeDSN(dsn string, readOnly bool) string {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return dsn
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	// RowsAffected counts matched rows, as the legacy Ballerina client did:
	// re-saving an unchanged row is not "not found".
	cfg.ClientFoundRows = true
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	if readOnly {
		cfg.Params["transaction_read_only"] = "1"
	}
	if cfg.TLSConfig == "" {
		cfg.TLSConfig = "true"
	}
	// The legacy employee view mixes collations; the driver's default
	// (utf8mb4_general_ci) makes prepared queries on it fail.
	if !strings.Contains(dsn, "collation=") {
		cfg.Collation = "utf8mb4_0900_ai_ci"
	}
	return cfg.FormatDSN()
}

// New opens one pool per schema with the one database user and pings each. On any failure the pools
// already opened are closed. The caller is responsible for calling Close.
func New(cfg *config.Config) (*Pools, error) {
	p := &Pools{}
	var err error
	if p.Allocation, err = open(config.AllocationSchema, cfg.DB.DSN(config.AllocationSchema), false, readWritePool); err != nil {
		return nil, err
	}
	if p.Travel, err = open(config.TravelSchema, cfg.DB.DSN(config.TravelSchema), false, readWritePool); err != nil {
		p.Close()
		return nil, err
	}
	if p.Expense, err = open(config.ExpenseSchema, cfg.DB.DSN(config.ExpenseSchema), true, smallPool); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func open(name, dsn string, readOnly bool, size poolSize) (*sql.DB, error) {
	d, err := sql.Open("mysql", normalizeDSN(dsn, readOnly))
	if err != nil {
		return nil, fmt.Errorf("%s: open mysql: %w", name, err)
	}
	d.SetMaxOpenConns(size.maxOpen)
	d.SetMaxIdleConns(size.maxIdle)
	d.SetConnMaxLifetime(connMaxLifetime)
	d.SetConnMaxIdleTime(connMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("%s: ping mysql: %w", name, err)
	}
	return d, nil
}
