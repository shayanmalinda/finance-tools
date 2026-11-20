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

package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer  = "wso2.org/products/am"
	testBackend = "backend-client-id"
)

// fakeGateway serves a JWKS over TLS and signs JWTs with its key.
type fakeGateway struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey
	kid string
}

func newFakeGateway(t *testing.T) *fakeGateway {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGateway{t: t, key: key, kid: "gw-1"}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": g.kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGateway) verifier() *Verifier {
	g.t.Helper()
	v, err := NewVerifier(Config{Issuer: testIssuer, JWKSURL: g.srv.URL + "/jwks", AllowedClientIDs: []string{"other-app", testBackend}}, g.srv.Client())
	if err != nil {
		g.t.Fatal(err)
	}
	return v
}

func (g *fakeGateway) sign(claims jwt.MapClaims, kid string) string {
	g.t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(g.key)
	if err != nil {
		g.t.Fatal(err)
	}
	return s
}

func claims(extra jwt.MapClaims) jwt.MapClaims {
	c := jwt.MapClaims{"iss": testIssuer, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "azp": testBackend}
	for k, v := range extra {
		if v == nil {
			delete(c, k)
			continue
		}
		c[k] = v
	}
	return c
}

func TestMiddleware(t *testing.T) {
	g := newFakeGateway(t)
	h := Middleware(g.verifier())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	hs256, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims(nil)).SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatal(err)
	}
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims(nil)).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		path   string
		token  string
		want   int
	}{
		{"valid, azp", http.MethodGet, "/engagements", g.sign(claims(nil), g.kid), http.StatusNoContent},
		{"valid, client_id claim", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"azp": nil, "client_id": testBackend}), g.kid), http.StatusNoContent},
		{"client_id wins over azp", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"client_id": testBackend, "azp": "someone-elses-app"}), g.kid), http.StatusNoContent},
		{"client_id not allowed, azp allowed", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"client_id": "someone-elses-app"}), g.kid), http.StatusForbidden},
		{"valid, WSO2 claim", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"azp": nil, "http://wso2.org/claims/client_id": testBackend}), g.kid), http.StatusNoContent},
		{"missing header", http.MethodGet, "/engagements", "", http.StatusUnauthorized},
		{"wrong issuer", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"iss": "https://evil.example"}), g.kid), http.StatusUnauthorized},
		{"expired", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}), g.kid), http.StatusUnauthorized},
		{"no exp", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"exp": nil}), g.kid), http.StatusUnauthorized},
		{"unknown kid", http.MethodGet, "/engagements", g.sign(claims(nil), "rotated-away"), http.StatusUnauthorized},
		{"HS256", http.MethodGet, "/engagements", hs256, http.StatusUnauthorized},
		{"alg none", http.MethodGet, "/engagements", none, http.StatusUnauthorized},
		{"no client claim", http.MethodGet, "/engagements", g.sign(claims(jwt.MapClaims{"azp": nil}), g.kid), http.StatusUnauthorized},
		{"client not allowed", http.MethodPost, "/role-settings/travel/roles/1/privileges", g.sign(claims(jwt.MapClaims{"azp": "someone-elses-app"}), g.kid), http.StatusForbidden},
		{"health stays open", http.MethodGet, "/health", "", http.StatusNoContent},
		{"only GET /health is open", http.MethodPost, "/health", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, c.path, nil)
			if c.token != "" {
				r.Header.Set("x-jwt-assertion", c.token) // case-insensitive header
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Errorf("status = %d, want %d (body %s)", w.Code, c.want, w.Body)
			}
		})
	}
}

func TestOffLetsEverythingThrough(t *testing.T) {
	h := Off(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/engagements", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d", w.Code)
	}
}

func TestNewVerifierFailsClosed(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no issuer":         {JWKSURL: "https://gw.example/jwks", AllowedClientIDs: []string{"a"}},
		"no allowed client": {Issuer: testIssuer, JWKSURL: "https://gw.example/jwks"},
		"http JWKS":         {Issuer: testIssuer, JWKSURL: "http://gw.example/jwks", AllowedClientIDs: []string{"a"}},
	} {
		if _, err := NewVerifier(cfg, nil); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// A gateway that names the client in another claim is configured with
// ClientIDClaims, without a code change; the default list then no longer applies.
func TestVerifier_ConfiguredClientIDClaims(t *testing.T) {
	g := newFakeGateway(t)
	v, err := NewVerifier(Config{Issuer: testIssuer, JWKSURL: g.srv.URL + "/jwks", AllowedClientIDs: []string{testBackend},
		ClientIDClaims: []string{"http://wso2.org/claims/applicationUUId"}}, g.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	tok := g.sign(claims(jwt.MapClaims{"azp": nil, "http://wso2.org/claims/applicationUUId": testBackend}), g.kid)
	if got, err := v.Verify(t.Context(), tok); err != nil || got != testBackend {
		t.Fatalf("configured claim: client %q, err %v", got, err)
	}
	if _, err := v.Verify(t.Context(), g.sign(claims(nil), g.kid)); err == nil {
		t.Fatal("azp was accepted although CLIENT_ID_CLAIMS names only applicationUUId")
	}
}

// Every redirect hop of the JWKS fetch must pass the https (http only on
// loopback) rule, not only the configured URL.
func TestSecureRedirects(t *testing.T) {
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://jwks.example/keys", http.StatusFound)
	}))
	defer jwks.Close()
	c := secureRedirects(jwks.Client())
	// The policy must reject the hop itself: a network error reaching the
	// plaintext host would also fail the Get, so check the policy's message.
	if _, err := c.Get(jwks.URL); err == nil || !strings.Contains(err.Error(), "JWKS redirect to a non-https URL") {
		t.Fatalf("redirect to plaintext http: err = %v, want the redirect-policy rejection", err)
	}
	if err := c.CheckRedirect(httptest.NewRequest(http.MethodGet, "http://jwks.example/keys", nil), nil); err == nil {
		t.Fatal("CheckRedirect allowed a plaintext non-loopback target")
	}
	for _, target := range []string{"https://jwks.example/keys", "http://127.0.0.1:8443/keys"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if err := c.CheckRedirect(req, nil); err != nil {
			t.Errorf("%s: %v", target, err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "https://jwks.example/keys", nil)
	if err := c.CheckRedirect(req, make([]*http.Request, maxJWKSRedirects)); err == nil {
		t.Error("redirect chain longer than the limit was followed")
	}
}
