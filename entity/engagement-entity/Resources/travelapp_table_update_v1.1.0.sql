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

-- Send status on travelapp.email_auditlog: rows written before this have
-- only ever been successful sends, so the default 'Sent' is correct for
-- them. The legacy Travel app's INSERT lists its columns, so it keeps
-- working. MySQL has no ADD COLUMN IF NOT EXISTS: run this once. A second
-- run fails with "Duplicate column name", which changes nothing.
ALTER TABLE travelapp.email_auditlog
    ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'Sent',
    ADD COLUMN error_message VARCHAR(500) NULL;
