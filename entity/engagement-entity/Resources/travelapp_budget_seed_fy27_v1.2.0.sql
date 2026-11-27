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

-- FY27 travel budget seed. Idempotent: lines are upserted (amounts reset to these
-- figures on a re-run, so run it once before Finance edits lines in Settings),
-- rules are skipped when they exist. Needs a user that reads allocation_db and
-- writes travelapp.

INSERT INTO travelapp.budget_year (budget_year, start_date, end_date, is_active)
VALUES (2027, '2027-01-01', '2027-12-31', 1)
ON DUPLICATE KEY UPDATE start_date = VALUES(start_date), end_date = VALUES(end_date), is_active = VALUES(is_active);

INSERT INTO travelapp.budget_line (budget_year, code, function_name, sub_function, holder_name, amount_usd, sort_order) VALUES
  (2027, 'COS_NONREC', 'Non-Recurring Revenue COS', 'Direct delivery travel, non-recurring', NULL, 180000.00, 10),
  (2027, 'COS_REC', 'Recurring Revenue COS', 'Direct delivery travel, recurring', NULL, 520000.00, 20),
  (2027, 'SALES_APAC', 'Sales', 'APAC', 'Genie', 60000.00, 30),
  (2027, 'SALES_EU', 'Sales', 'EU', 'Ricardo', 75000.00, 40),
  (2027, 'SALES_LATAM', 'Sales', 'LATAM', 'Fernando', 40000.00, 50),
  (2027, 'SALES_ME', 'Sales', 'ME', 'Uday', 30500.00, 60),
  (2027, 'SALES_NA', 'Sales', 'NA', 'Brett', 50000.00, 70),
  (2027, 'SALES_AFRICA', 'Sales', 'AFRICA', 'Dinal', 42500.00, 80),
  (2027, 'SALES_CHANNEL', 'Sales', 'Channel Team', 'Kristian', 60000.00, 90),
  (2027, 'SALES_ENG', 'Sales', 'Sales Engineering', 'Vidura', 45000.00, 100),
  (2027, 'SALES_CENTRAL', 'Sales', 'Central Sales Team', 'Shivanga', 45000.00, 110),
  (2027, 'RND_AGENT_API', 'Research and Development', 'Agent & API Management', 'Rania / Derric', 50000.00, 120),
  (2027, 'RND_INTEGRATION', 'Research and Development', 'Integration Product', 'Sharya', 50000.00, 130),
  (2027, 'RND_IAM', 'Research and Development', 'IAM Product', 'Geethika', 50000.00, 140),
  (2027, 'RND_DEV_PLATFORM', 'Research and Development', 'Developer Platform', 'Lakmal', 50000.00, 150),
  (2027, 'RND_ALLIANCES', 'Research and Development', 'Technology Alliances & Solutions', 'Mifan', 25000.00, 160),
  (2027, 'RND_CHOREO', 'Research and Development', 'Choreo Cloud', 'Shankar', 25000.00, 170),
  (2027, 'RND_TRAVEL', 'Research and Development', 'R&D travel', 'Shankar', 100000.00, 180),
  (2027, 'GA_BOARD', 'General Administration', 'Board meeting travel', 'Shivanga', 500000.00, 190),
  (2027, 'GA_SENIOR', 'General Administration', 'Senior leadership meetings', 'Shivanga', 455000.00, 200),
  (2027, 'GA_GENERAL', 'General Administration', 'General corporate team travel', 'Shivanga', 732000.00, 210),
  (2027, 'MKT_TRAVEL', 'Marketing', 'Marketing travel', 'Thibaut', 15000.00, 220)
ON DUPLICATE KEY UPDATE function_name = VALUES(function_name), sub_function = VALUES(sub_function),
  holder_name = VALUES(holder_name), amount_usd = VALUES(amount_usd), sort_order = VALUES(sort_order);

-- Allocation type rules, one per allocation type name (every id with that name).
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_APAC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - APAC';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Admin - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'ENGAGEMENT', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Internal Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'ENGAGEMENT', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'External Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'ENGAGEMENT', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'WSO2Con';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'REGION', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Pre-Sale Engagement';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Admin&HR - Office Rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'REGION', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales Routine Customer Visit';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'REGION', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_SENIOR' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Senior Leadership Travel';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_SENIOR' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Planning Group Meeting';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_EU' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - EU';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_LATAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - LATAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Product Specialist';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture Routine Customer Visit';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'REGION', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Routine Customer Visit';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_EU' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'EU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Integration - Office Rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'QSP';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales - APAC';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Corporate BU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales - US';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Business Operations';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Product Specialist - FoC';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_APAC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'APAC Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_AFRICA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Africa Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Review';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Engineering - Conferences';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_EU' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - UK';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_LATAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'LATAM Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_AGENT_API' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'AI BU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_IAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'IAM - Conferences / Summits';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_REC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Managed Services';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_NA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - US North East';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Engineering - Summit';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Corporate';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'IAM - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CENTRAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - Office Rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Architecture';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_REC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Integration - APIM Support';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'HR - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Training';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Engineering-Bootcamp';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - IAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales - UK';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'SDR - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_REC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Support Related Customer Firefighting';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_IAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'IAM BU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_CHOREO' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Choreo BU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_NA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - US West';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_SENIOR' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Engineering - Leadership Meeting';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Training Related Customer Firefighting';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Engineering-VisitingOtherOffices';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Finance - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_IAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'IAM - Product Feature Development';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting Related Customer Firefighting';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Operational Specialist';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CENTRAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales Bootcamp';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - APAC';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales - LATAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_CHANNEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Channel Sales - CHANNEL SALES';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Security';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'CTO - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Internal Support';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'REGION', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Field Sales';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Dev-Rel';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Onboarding Services';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Product Specialist-POC';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'AI - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'API BU - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_NA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'North America Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_AGENT_API' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'NonRnD Pool - APIM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - Office rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_NA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - US South';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Pre-sale Support';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Product Feature Development (Customer Commitment)';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_AFRICA' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Sales - Africa';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Integration - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - Integration - Lead';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Legal - General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Deployment Configuration Review';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'TEAM', NULL FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Product Management Support';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Technology Area Specialist';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_IAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Innovation-IAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_CHOREO' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Innovation-Choreo';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - US North East';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - LATAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Integration - CS - Migrations';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Corporate - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_IAM' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Documentation-IAM';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - IAM - Office Rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'GA_GENERAL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Finance - Office Rotation';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'AI BU - Marketing General';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - SME';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Consulting - Platform Architect';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_ALLIANCES' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solutions BU Funded Event';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'RND_INTEGRATION' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Integration - Product Feature Development';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - EU';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'COS_NONREC' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Customer Success - WSO2 Digital';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'SALES_ENG' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Solution Architecture - US West';
INSERT IGNORE INTO travelapp.budget_allocation_type_rule (allocation_type_id, rule_kind, line_code) SELECT aT.id, 'LINE', 'MKT_TRAVEL' FROM allocation_db.allocation_type AS aT WHERE aT.allocation_type = 'Corporate BU - Marketing General';

-- Region rules (trailing * = prefix match). A NULL line means no budget line.
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'APAC', NULL, 'SALES_APAC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'APAC' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'ANZ', NULL, 'SALES_APAC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'ANZ' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'EU', NULL, 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'EU' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'UK', NULL, 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'UK' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'ROE', NULL, 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'ROE' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'UK + Ireland', NULL, 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'UK + Ireland' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'Benelux + France', NULL, 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'Benelux + France' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'NA', NULL, 'SALES_NA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'NA' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'NA - *', NULL, 'SALES_NA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'NA - *' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'LATAM', NULL, 'SALES_LATAM' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'LATAM' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'MEA', 'AFRICA', 'SALES_AFRICA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'MEA' AND sub_region <=> 'AFRICA');
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'MEA', 'ME', 'SALES_ME' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'MEA' AND sub_region <=> 'ME');
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'AFRICA', NULL, 'SALES_AFRICA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'AFRICA' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'ME', NULL, 'SALES_ME' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'ME' AND sub_region <=> NULL);
INSERT INTO travelapp.budget_region_rule (region, sub_region, line_code) SELECT 'NO POD', NULL, NULL FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_region_rule WHERE region = 'NO POD' AND sub_region <=> NULL);

-- Team rules (business unit, department, team; NULL = any).
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'Agent Platform BU', 'ENGINEERING', NULL, 'RND_AGENT_API' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'Agent Platform BU' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'API Platform BU', 'ENGINEERING', NULL, 'RND_AGENT_API' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'API Platform BU' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'Choreo', 'ENGINEERING', NULL, 'RND_CHOREO' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'Choreo' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'IAM', 'ENGINEERING', NULL, 'RND_IAM' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'IAM' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'INTEGRATION', 'ENGINEERING', NULL, 'RND_INTEGRATION' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'INTEGRATION' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'SOLUTION', 'ENGINEERING', NULL, 'RND_ALLIANCES' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'SOLUTION' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT 'CORPORATE', 'ENGINEERING', NULL, 'RND_TRAVEL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> 'CORPORATE' AND department <=> 'ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'MARKETING', NULL, 'MKT_TRAVEL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'MARKETING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'APAC', 'SALES_APAC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'APAC');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'EU', 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'EU');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'UK', 'SALES_EU' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'UK');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'LATAM', 'SALES_LATAM' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'LATAM');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'ME', 'SALES_ME' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'ME');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'NA', 'SALES_NA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'NA');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', 'AFRICA', 'SALES_AFRICA' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> 'AFRICA');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES', NULL, 'SALES_CENTRAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'CHANNEL SALES', NULL, 'SALES_CHANNEL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'CHANNEL SALES' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'SALES ENGINEERING', NULL, 'SALES_ENG' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'SALES ENGINEERING' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'CUSTOMER SUCCESS', 'CUSTOMER RELIABILITY ENGINEERING', 'COS_REC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'CUSTOMER SUCCESS' AND team <=> 'CUSTOMER RELIABILITY ENGINEERING');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'CUSTOMER SUCCESS', 'SITE RELIABILITY ENGINEERING', 'COS_REC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'CUSTOMER SUCCESS' AND team <=> 'SITE RELIABILITY ENGINEERING');
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'CUSTOMER SUCCESS', NULL, 'COS_NONREC' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'CUSTOMER SUCCESS' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'ADMINISTRATION', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'ADMINISTRATION' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'BUSINESS OPERATIONS', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'BUSINESS OPERATIONS' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'DIGITAL TRANSFORMATION', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'DIGITAL TRANSFORMATION' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'FINANCE', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'FINANCE' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'LEGAL', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'LEGAL' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'PEOPLE OPERATIONS', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'PEOPLE OPERATIONS' AND team <=> NULL);
INSERT INTO travelapp.budget_team_rule (business_unit, department, team, line_code) SELECT NULL, 'OTHER', NULL, 'GA_GENERAL' FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM travelapp.budget_team_rule WHERE business_unit <=> NULL AND department <=> 'OTHER' AND team <=> NULL);
