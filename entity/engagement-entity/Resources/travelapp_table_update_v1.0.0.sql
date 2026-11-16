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
-- under the License.

CREATE TABLE IF NOT EXISTS travelapp.audit_event (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    occurred_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    actor_email VARCHAR(255) NULL,
    actor_type ENUM('user', 'system') NOT NULL,
    action VARCHAR(20) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_id VARCHAR(64) NULL,
    parent_type VARCHAR(64) NULL,
    parent_id VARCHAR(255) NULL,
    request_id VARCHAR(64) NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'engagement-app',
    changes JSON NULL,
    metadata JSON NULL,
    INDEX idx_audit_event_entity (entity_type, entity_id, occurred_at),
    INDEX idx_audit_event_actor (actor_email, occurred_at),
    INDEX idx_audit_event_occurred (occurred_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
