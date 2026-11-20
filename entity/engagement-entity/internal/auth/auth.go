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

// DefaultClientIDClaims are where a gateway JWT names the calling
// application, tried in order: client_id, then azp, as the Customer Entity
// (cs-tools-membership-scope entity-service, AUTH_INTERNAL_CLIENT_IDS) reads
// it from the same Choreo gateway, then the WSO2 API Manager's application
// claim. Config.ClientIDClaims (CLIENT_ID_CLAIMS) replaces the list when the
// gateway names the client elsewhere.
var DefaultClientIDClaims = []string{"client_id", "azp", "http://wso2.org/claims/client_id"}

// DefaultAPIContextClaims are where a gateway JWT names the API it was
// issued for (the WSO2 gateway's API context claim, e.g.
// "/wutu/engagement-entity/v1.0"). Config.APIContextClaims
// (API_CONTEXT_CLAIMS) replaces the list.
var DefaultAPIContextClaims = []string{"http://wso2.org/claims/apicontext"}

// Config is what verification needs.
type Config struct {
	// Issuer must equal the JWT's iss claim.
	Issuer string
	// JWKSURL is where the gateway's signing keys are. https only (http on
	// loopback, for tests).
	JWKSURL string
	// AllowedClientIDs are the applications let through.
	AllowedClientIDs []string
	// ClientIDClaims are the claims that name the calling application, tried
	// in order; empty uses DefaultClientIDClaims.
	ClientIDClaims []string
	// APIContexts, when set, binds the assertion to this API: its API context
	// claim must be one of them, so an assertion the gateway issued for
	// another API (Sales or HR entity, Email Service) is refused here. Empty
	// skips the check (until the claim is confirmed on the gateway).
	APIContexts []string
	// APIContextClaims are the claims that name that API, tried in order;
	// empty uses DefaultAPIContextClaims.
	APIContextClaims []string
	// ClockSkew is the leeway on exp, nbf and iat.
	ClockSkew time.Duration
}

// ErrNotAllowed means the JWT is valid but its application is not listed.
var ErrNotAllowed = errors.New("auth: client application not allowed")

// ErrWrongAPI means the JWT is valid but was issued for another API.
var ErrWrongAPI = errors.New("auth: gateway JWT issued for another API")

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
	if len(cfg.ClientIDClaims) == 0 {
		cfg.ClientIDClaims = DefaultClientIDClaims
	}
	if len(cfg.APIContextClaims) == 0 {
		cfg.APIContextClaims = DefaultAPIContextClaims
	}
	return &Verifier{cfg: cfg, keys: newKeySet(cfg.JWKSURL, secureRedirects(httpClient))}, nil
}

// maxJWKSRedirects bounds the redirects a JWKS fetch follows.
const maxJWKSRedirects = 5

// secureRedirects returns a copy of c whose every redirect hop must pass the
// same https (http only on loopback) rule as the configured JWKS URL, so the
// JWKS host can't hand the key fetch to a plaintext hop where the keys could
// be swapped.
func secureRedirects(c *http.Client) *http.Client {
	out := *c
	out.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxJWKSRedirects {
			return errors.New("auth: too many JWKS redirects")
		}
		if !SecureOrLoopback(req.URL) {
			return fmt.Errorf("auth: JWKS redirect to a non-https URL %q", req.URL.Redacted())
		}
		return nil
	}
	return &out
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
	for _, name := range v.cfg.ClientIDClaims {
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
	if len(v.cfg.APIContexts) > 0 {
		api := ""
		for _, name := range v.cfg.APIContextClaims {
			if s, _ := claims[name].(string); strings.TrimSpace(s) != "" {
				api = strings.TrimRight(strings.TrimSpace(s), "/")
				break
			}
		}
		if !slices.ContainsFunc(v.cfg.APIContexts, func(c string) bool { return strings.TrimRight(c, "/") == api }) {
			// Claim names only, as above.
			log.Printf("auth: refused a gateway JWT not issued for this API (API context claims checked: %s)", strings.Join(v.cfg.APIContextClaims, ","))
			return "", ErrWrongAPI
		}
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
				if errors.Is(err, ErrWrongAPI) {
					deny(w, http.StatusForbidden, "assertion not issued for this API")
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
