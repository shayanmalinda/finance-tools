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

// Package oidc verifies the JWT the Choreo API gateway forwards in the
// x-jwt-assertion header: RS256 signature against the issuer's JWKS, issuer,
// audience, and expiry. Everything is driven by Config, so it is testable
// against a fake issuer and carries no provider-specific code.
//
// The approach follows the invoice-app backend (internal/oidc: JWKS cached,
// refreshed on an unknown key id at most once per minRefreshGap) and the GRC
// platform backend (reads only n/e from each JWK, fails closed on an empty
// audience list).
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config is everything token verification needs.
type Config struct {
	// Issuer must equal the token's iss claim exactly.
	Issuer string
	// JWKSURL is where the signing keys are fetched from. Must be https.
	JWKSURL string
	// Audiences: the token's aud must contain at least one. Required.
	Audiences []string
	// ClockSkew is the leeway on exp, nbf and iat.
	ClockSkew time.Duration
}

// Claims are the identity claims the backend uses.
type Claims struct {
	Subject string
	// Email is the user's email as the token carries it (not lower-cased).
	Email string
	// Groups are the user's Asgardeo groups. Empty when the token has none.
	Groups []string
	// GroupsPresent reports whether the token carried a groups claim at all.
	// An Asgardeo access token often omits it; the caller then asks the
	// userinfo endpoint (UserinfoClient), as the invoice-app backend does.
	GroupsPresent bool
}

// ErrNoEmail means neither the token nor the userinfo endpoint named the
// caller's email: the legacy data is keyed on email, so such a caller cannot
// be served.
var ErrNoEmail = errors.New("oidc: no email for the caller")

// Verifier validates tokens against Config. Safe for concurrent use.
type Verifier struct {
	cfg  Config
	keys *keySet
}

// NewVerifier builds a Verifier. httpClient is used to fetch the JWKS; nil
// uses a client with a 10 second timeout. It fails closed: an empty issuer,
// an empty audience list or a non-https JWKS URL is an error, never a
// verifier that skips the check.
func NewVerifier(cfg Config, httpClient *http.Client) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("oidc: issuer is required")
	}
	if len(cfg.Audiences) == 0 {
		return nil, errors.New("oidc: at least one audience is required")
	}
	u, err := url.Parse(cfg.JWKSURL)
	if err != nil || !SecureOrLoopback(u) {
		return nil, fmt.Errorf("oidc: JWKS URL must be https (http only on localhost), got %q", cfg.JWKSURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
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
			return errors.New("oidc: too many JWKS redirects")
		}
		if !SecureOrLoopback(req.URL) {
			return fmt.Errorf("oidc: JWKS redirect to a non-https URL %q", req.URL.Redacted())
		}
		return nil
	}
	return &out
}

// tokenClaims decodes the claims this package reads. The WSO2-namespaced
// claim names are what the gateway's backend JWT uses; the plain OIDC names
// are what an Asgardeo token uses. The legacy Travel backend reads both.
type tokenClaims struct {
	Email      string          `json:"email"`
	WSO2Email  string          `json:"http://wso2.org/claims/emailaddress"`
	Groups     json.RawMessage `json:"groups"`
	WSO2Groups json.RawMessage `json:"http://wso2.org/claims/groups"`
	jwt.RegisteredClaims
}

// Verify validates rawToken and returns its claims. A token without an email
// claim is valid here; the caller decides (see ErrNoEmail).
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	var c tokenClaims
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.cfg.ClockSkew),
		jwt.WithAudience(v.cfg.Audiences...), // any of
	}
	token, err := jwt.ParseWithClaims(rawToken, &c,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			return v.keys.lookup(ctx, kid)
		},
		opts...,
	)
	if err != nil {
		return Claims{}, fmt.Errorf("oidc: validating token: %w", err)
	}
	if !token.Valid {
		return Claims{}, errors.New("oidc: token failed validation")
	}

	email := firstNonEmpty(c.WSO2Email, c.Email)
	groups, err := parseGroups(c.WSO2Groups)
	if err == nil && len(groups) == 0 {
		groups, err = parseGroups(c.Groups)
	}
	if err != nil {
		return Claims{}, fmt.Errorf("oidc: groups claim: %w", err)
	}
	present := claimPresent(c.WSO2Groups) || claimPresent(c.Groups)
	return Claims{Subject: c.Subject, Email: email, Groups: groups, GroupsPresent: present}, nil
}

func claimPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// parseGroups accepts the groups claim as a JSON array of strings, or as a
// single string (WSO2 identity servers send one group as a plain string, and
// several as a comma-separated string when the claim is not multi-valued).
func parseGroups(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return cleanGroups(list), nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, errors.New("must be a string or an array of strings")
	}
	return cleanGroups(strings.Split(single, ",")), nil
}

func cleanGroups(in []string) []string {
	out := make([]string, 0, len(in))
	for _, g := range in {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// SecureOrLoopback reports whether u is https, or http to localhost for local testing.
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
