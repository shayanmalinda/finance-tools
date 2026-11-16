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

package engagemententity

import (
	"context"
	"net/http"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

// AuditAPI records the views, downloads and exports the backend serves.
type AuditAPI interface {
	// RecordAuditEvent calls POST /audit-events.
	RecordAuditEvent(ctx context.Context, e domain.AuditEvent) error
}

// RecordAuditEvent calls POST /audit-events.
func (c *HTTPClient) RecordAuditEvent(ctx context.Context, e domain.AuditEvent) error {
	return c.rest.Post(ctx, "/audit-events", e, nil)
}

const userEmailHeader = "X-User-Email"

// userTransport names the signed-in user on every entity call, for the
// entity's audit trail.
type userTransport struct{ next http.RoundTripper }

func (t userTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c, ok := reqcontext.CallerFrom(req.Context()); ok && c.Email != "" {
		req = req.Clone(req.Context())
		req.Header.Set(userEmailHeader, c.Email)
	}
	return t.next.RoundTrip(req)
}

func withUser(c *http.Client) *http.Client {
	out := *c
	next := out.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	out.Transport = userTransport{next: next}
	return &out
}
