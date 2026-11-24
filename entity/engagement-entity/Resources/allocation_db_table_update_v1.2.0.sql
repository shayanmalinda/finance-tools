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

-- Email log of the Engagement App's allocation emails: one row per send
-- attempt, Sent or Failed (with the error). Same columns as
-- travelapp.email_auditlog. Safe to run more than once.
CREATE TABLE IF NOT EXISTS allocation_db.email_auditlog (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    subject VARCHAR(255) DEFAULT NULL,
    templateId VARCHAR(255) DEFAULT NULL,
    contentKeyValPairs VARCHAR(3000) DEFAULT NULL,
    recipients VARCHAR(255) DEFAULT NULL,
    cc_recipients VARCHAR(255) DEFAULT NULL,
    datetime DATETIME DEFAULT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'Sent',
    error_message VARCHAR(500) NULL,
    INDEX idx_email_auditlog_search (subject, templateId),
    INDEX idx_email_auditlog_datetime (datetime)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
