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

package domain

// Email audit statuses: every send attempt is logged as one of them.
const (
	EmailAuditSent   = "Sent"
	EmailAuditFailed = "Failed"
)

// EmailAuditInput is one email_auditlog row, in travelapp
// (POST /travel-email-audit-logs) and in allocation_db
// (POST /allocation-email-audit-logs).
type EmailAuditInput struct {
	Subject            string `json:"subject"`
	TemplateID         string `json:"templateId"`
	ContentKeyValPairs string `json:"contentKeyValPairs"`
	Recipients         string `json:"recipients"`
	CCRecipients       string `json:"ccRecipients"`
	// Status is "Sent" or "Failed"; empty means "Sent" (a caller that
	// predates the status logs only sends that went out).
	Status string `json:"status"`
	// ErrorMessage is why a send failed; empty for Sent.
	ErrorMessage string `json:"errorMessage"`
}
