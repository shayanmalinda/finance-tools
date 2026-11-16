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

// Package reqcontext holds per-request values that travel from the HTTP layer
// down to services and outbound clients without either importing the other:
// the verified caller and the correlation ID. It stores only plain values.
package reqcontext

import (
	"context"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Caller is the signed-in user, as the verified gateway JWT names them.
type Caller struct {
	// Email is the user's work email, lower-cased.
	Email string
	// Groups are the user's Asgardeo groups, as the token carries them. The
	// legacy apps' role tables are keyed on these names.
	Groups []string
}

type contextKey string

const (
	callerKey        contextKey = "engagement-app-caller"
	correlationIDKey contextKey = "engagement-app-correlation-id"
)

// WithCaller returns a copy of ctx carrying c.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey, c)
}

// CallerFrom returns the Caller attached by the auth middleware, and whether
// one was present. It is absent only on routes outside the auth middleware
// (GET /health).
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey).(Caller)
	return c, ok
}

// WithCorrelationID returns a copy of ctx carrying id.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationID returns the correlation ID attached to ctx, or "".
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey).(string)
	return id
}

const permsKey contextKey = "engagement-app-perms"

// WithPerms returns a copy of ctx carrying the caller's resolved privileges.
func WithPerms(ctx context.Context, p domain.Perms) context.Context {
	return context.WithValue(ctx, permsKey, p)
}

// PermsFrom returns the privileges attached by middleware.Require, and
// whether they were present.
func PermsFrom(ctx context.Context) (domain.Perms, bool) {
	p, ok := ctx.Value(permsKey).(domain.Perms)
	return p, ok
}
