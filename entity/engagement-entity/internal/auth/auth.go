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

// Package auth admits only listed client applications. The entity is
// reached through its Choreo Organization endpoint; the gateway there checks
// the caller's OAuth token and forwards a JWT it signs in X-JWT-Assertion.
// The entity verifies that JWT (RS256 against the gateway's JWKS, issuer,
// expiry) and requires the calling application's client id to be listed, so
// another application subscribed to the endpoint is refused.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AssertionHeader is where the Choreo gateway forwards its JWT.
const AssertionHeader = "X-JWT-Assertion"

// clientIDClaims are where a gateway JWT names the calling application,
// tried in order: client_id, then azp, as the Customer Entity
// (cs-tools-membership-scope entity-service, AUTH_INTERNAL_CLIENT_IDS) reads
// it, then the WSO2 API Manager's application claim.
var clientIDClaims = []string{"client_id", "azp", "http://wso2.org/claims/client_id"}

// Config is what verification needs.
type Config struct {
	// Issuer must equal the JWT's iss claim.
	Issuer string
	// JWKSURL is where the gateway's signing keys are. https only (http on
	// loopback, for tests).
	JWKSURL string
	// AllowedClientIDs are the applications let through.
	AllowedClientIDs []string
	// ClockSkew is the leeway on exp, nbf and iat.
	ClockSkew time.Duration
}

// ErrNotAllowed means the JWT is valid but its application is not listed.
var ErrNotAllowed = errors.New("auth: client application not allowed")

// Verifier checks gateway JWTs. Safe for concurrent use.
type Verifier struct {
	cfg  Config
	keys *keySet
}

// NewVerifier fails closed: no issuer, no allowed client or a non-https JWKS
// URL is an error, never a verifier that skips the check.
func NewVerifier(cfg Config, httpClient *http.Client) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("auth: issuer is required")
	}
	if len(cfg.AllowedClientIDs) == 0 {
		return nil, errors.New("auth: at least one allowed client id is required")
	}
	u, err := url.Parse(cfg.JWKSURL)
	if err != nil || !SecureOrLoopback(u) {
		return nil, fmt.Errorf("auth: JWKS URL must be https (http only on localhost), got %q", cfg.JWKSURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Verifier{cfg: cfg, keys: newKeySet(cfg.JWKSURL, httpClient)}, nil
}

// Verify checks rawToken and returns the calling application's client id.
// ErrNotAllowed means a valid JWT from an application that is not listed.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (string, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(rawToken, claims,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			return v.keys.lookup(ctx, kid)
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.cfg.ClockSkew),
	)
	if err != nil {
		return "", fmt.Errorf("auth: validating gateway JWT: %w", err)
	}
	var present []string
	client := ""
	for _, name := range clientIDClaims {
		s, _ := claims[name].(string)
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		present = append(present, name)
		if client == "" {
			client = s
		}
	}
	if client == "" {
		return "", errors.New("auth: gateway JWT names no client application")
	}
	if !slices.Contains(v.cfg.AllowedClientIDs, client) {
		// Claim names only, never values: enough to see which claim the
		// gateway uses without logging identifiers.
		log.Printf("auth: refused a client application not in ALLOWED_CLIENT_IDS (client id claims present: %s)", strings.Join(present, ","))
		return "", ErrNotAllowed
	}
	return client, nil
}

// Middleware admits a request only with a valid gateway JWT from a listed
// application: 401 without one or with an invalid one, 403 for a valid one
// from another application. GET /health stays open for the platform's probes.
func Middleware(v *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}
			raw := strings.TrimSpace(r.Header.Get(AssertionHeader))
			if raw == "" {
				deny(w, http.StatusUnauthorized, "missing gateway assertion")
				return
			}
			if _, err := v.Verify(r.Context(), raw); err != nil {
				if errors.Is(err, ErrNotAllowed) {
					deny(w, http.StatusForbidden, "client application not allowed")
					return
				}
				log.Printf("auth: rejected gateway assertion: %v", err)
				deny(w, http.StatusUnauthorized, "invalid gateway assertion")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Off lets every request through. Only for a local run (AUTH_DISABLED=true,
// which also binds the server to loopback) and for tests.
func Off(next http.Handler) http.Handler { return next }

func deny(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"message":%q}`, msg)
}

// SecureOrLoopback reports whether u is https, or http to localhost.
func SecureOrLoopback(u *url.URL) bool {
	if u == nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	h := u.Hostname()
	return u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1")
}
