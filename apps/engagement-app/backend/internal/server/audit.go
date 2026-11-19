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

package server

import (
	"net/http"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/middleware"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

type auditEventFunc = func(r *http.Request) (domain.AuditEvent, bool)

// handleAudited is handle plus an audit event for a successful response
// (views and downloads of passports, visas and travel documents, CSV
// exports). One list page is one event.
func (a *area) handleAudited(pattern string, gate domain.Gate, event auditEventFunc, h http.HandlerFunc) {
	a.mux.Handle(pattern, middleware.Require(a.Access, gate)(middleware.Audit(a.Entity, event)(h)))
}

// handleReport is handleAudited for an unpaged report: after the privilege
// check, at most Deps.ReportMaxConcurrent run at once on this pod (503 with
// Retry-After when full), since each holds the whole report in memory.
func (a *area) handleReport(pattern string, gate domain.Gate, event auditEventFunc, h http.HandlerFunc) {
	a.mux.Handle(pattern, middleware.Require(a.Access, gate)(a.Reports(middleware.Audit(a.Entity, event)(h))))
}

// auditOne is a view or download of the record named by path value param.
func auditOne(action, entityType, param string) auditEventFunc {
	return func(r *http.Request) (domain.AuditEvent, bool) {
		return domain.AuditEvent{Action: action, EntityType: entityType, EntityID: r.PathValue(param)}, true
	}
}

// auditList is a list page; own lists name the caller as the parent.
func auditList(entityType string, own bool) auditEventFunc {
	return func(r *http.Request) (domain.AuditEvent, bool) {
		e := domain.AuditEvent{Action: domain.AuditView, EntityType: entityType, Metadata: auditQuery(r)}
		if own {
			e.ParentType, e.ParentID = "employee", auditCaller(r)
		}
		return e, true
	}
}

// auditPassports is a traveller's passport list (own: the caller's).
func auditPassports(own bool) auditEventFunc {
	return func(r *http.Request) (domain.AuditEvent, bool) {
		email := strings.ToLower(strings.TrimSpace(r.PathValue("email")))
		if own {
			email = auditCaller(r)
		}
		return domain.AuditEvent{Action: domain.AuditView, EntityType: "travel_document", ParentType: "employee",
			ParentID: email, Metadata: map[string]any{"documentType": "Passport"}}, true
	}
}

// auditExport is a report read: an export when downloaded as CSV
// (format=csv), a view otherwise, since the JSON carries the same rows.
func auditExport(entityType, report string) auditEventFunc {
	return func(r *http.Request) (domain.AuditEvent, bool) {
		action := domain.AuditView
		if strings.EqualFold(r.URL.Query().Get("format"), "csv") {
			action = domain.AuditExport
		}
		md := auditQuery(r)
		md["report"] = report
		return domain.AuditEvent{Action: action, EntityType: entityType, Metadata: md}, true
	}
}

func auditQuery(r *http.Request) map[string]any {
	md := map[string]any{}
	if q := r.URL.RawQuery; q != "" {
		md["query"] = q
	}
	return md
}

func auditCaller(r *http.Request) string {
	c, _ := reqcontext.CallerFrom(r.Context())
	return c.Email
}
