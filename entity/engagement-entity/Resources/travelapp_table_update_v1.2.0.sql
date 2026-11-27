-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
--
-- WSO2 LLC. licenses this file to you under the Apache License,
-- Version 2.0 (the "License"); you may not use this file except
-- in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied. See the License for the
-- specific language governing permissions and limitations

-- FY27 travel budgets: seven new tables in travelapp. Nothing existing is
-- changed. Every statement is CREATE TABLE IF NOT EXISTS, so a second run
-- changes nothing. Run before travelapp_budget_seed_fy27_v1.2.0.sql.

-- Financial years. A trip belongs to the year containing its start date.
CREATE TABLE IF NOT EXISTS travelapp.budget_year (
  budget_year SMALLINT UNSIGNED NOT NULL PRIMARY KEY,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- One budget line per year. code is stable across years.
CREATE TABLE IF NOT EXISTS travelapp.budget_line (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  budget_year SMALLINT UNSIGNED NOT NULL,
  code VARCHAR(40) NOT NULL,
  function_name VARCHAR(100) NOT NULL,
  sub_function VARCHAR(150) NOT NULL,
  holder_name VARCHAR(255) NULL,
  holder_email VARCHAR(255) NULL,
  amount_usd DECIMAL(14,2) NOT NULL DEFAULT 0,
  status VARCHAR(16) NOT NULL DEFAULT 'Active', -- Active | Retired
  sort_order INT NOT NULL DEFAULT 0,
  created_by VARCHAR(255) NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_by VARCHAR(255) NULL,
  updated_at DATETIME NULL,
  UNIQUE KEY uq_budget_line_year_code (budget_year, code),
  INDEX idx_budget_line_year (budget_year, status)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- The budget line of a trip. job_number matches travel.job_number (latin1);
-- no foreign key, as for the other travelapp side tables.
CREATE TABLE IF NOT EXISTS travelapp.travel_budget (
  job_number VARCHAR(255) CHARACTER SET latin1 NOT NULL PRIMARY KEY,
  budget_year SMALLINT UNSIGNED NULL,
  budget_line_id INT UNSIGNED NULL,
  suggested_line_id INT UNSIGNED NULL,
  suggestion_source VARCHAR(20) NULL,
  chosen_by VARCHAR(8) NULL,
  rate_date DATE NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_by VARCHAR(255) NULL,
  updated_at DATETIME NULL,
  INDEX idx_travel_budget_line (budget_line_id),
  INDEX idx_travel_budget_year (budget_year, budget_line_id)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- Allocation type -> budget line rule (LINE | REGION | TEAM | ENGAGEMENT | NONE).
-- allocation_type_id is an allocation_db id: no cross-schema foreign key.
CREATE TABLE IF NOT EXISTS travelapp.budget_allocation_type_rule (
  allocation_type_id INT NOT NULL PRIMARY KEY,
  rule_kind VARCHAR(12) NOT NULL,
  line_code VARCHAR(40) NULL,
  updated_by VARCHAR(255) NULL,
  updated_at DATETIME NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- Funding region -> Sales line. A trailing * in region is a prefix match,
-- a NULL sub_region matches any, a NULL line means none.
CREATE TABLE IF NOT EXISTS travelapp.budget_region_rule (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  region VARCHAR(100) NOT NULL,
  sub_region VARCHAR(100) NULL,
  line_code VARCHAR(40) NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- Traveller's HR team -> line. NULL in a column matches any.
CREATE TABLE IF NOT EXISTS travelapp.budget_team_rule (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  business_unit VARCHAR(100) NULL,
  department VARCHAR(100) NULL,
  team VARCHAR(150) NULL,
  line_code VARCHAR(40) NOT NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

-- The line set on an engagement (kept here so allocation_db.engagement and
-- its triggers stay untouched). engagement_id is an allocation_db id.
CREATE TABLE IF NOT EXISTS travelapp.engagement_budget_line (
  engagement_id VARCHAR(255) NOT NULL PRIMARY KEY,
  line_code VARCHAR(40) NOT NULL,
  updated_by VARCHAR(255) NULL,
  updated_at DATETIME NULL
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
