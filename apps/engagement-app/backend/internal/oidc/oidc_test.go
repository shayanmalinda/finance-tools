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

package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://api.asgardeo.io/t/wso2/oauth2/token"
	testAudience = "one-wso2-client"
)

// fakeIssuer serves a JWKS over TLS and signs tokens with its current key.
type fakeIssuer struct {
	t   *testing.T
	srv *httptest.Server

	mu   sync.Mutex
	kid  string
	key  *rsa.PrivateKey
	hits int
	// status, when set, is returned instead of the key set; empty serves a
	// set with no keys.
	status int
	empty  bool
}

func (f *fakeIssuer) serve(status int, empty bool) {
	f.mu.Lock()
	f.status, f.empty = status, empty
	f.mu.Unlock()
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{t: t}
	f.rotate("key-1")
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits++
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		if f.empty {
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIssuer) rotate(kid string) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.kid, f.key = kid, key
	f.mu.Unlock()
}

func (f *fakeIssuer) verifier() *Verifier {
	f.t.Helper()
	v, err := NewVerifier(Config{
		Issuer: testIssuer, JWKSURL: f.srv.URL + "/jwks", Audiences: []string{"other-client", testAudience},
	}, f.srv.Client())
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fakeIssuer) fetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}

func validClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub":    "a1b2c3",
		"iss":    testIssuer,
		"aud":    []string{testAudience},
		"exp":    now.Add(time.Hour).Unix(),
		"iat":    now.Unix(),
		"email":  "Jane@wso2.com",
		"groups": []string{"wso2-employees", "app-travel-admin-super"},
	}
}

func (f *fakeIssuer) sign(claims jwt.MapClaims) string {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	s, err := tok.SignedString(f.key)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestVerify_ValidToken(t *testing.T) {
	f := newFakeIssuer(t)
	got, err := f.verifier().Verify(context.Background(), f.sign(validClaims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	want := Claims{Subject: "a1b2c3", Email: "Jane@wso2.com", Groups: []string{"wso2-employees", "app-travel-admin-super"}, GroupsPresent: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestVerify_ClaimVariants(t *testing.T) {
	tests := []struct {
		name       string
		edit       func(jwt.MapClaims)
		wantEmail  string
		wantGroups []string
	}{
		{"WSO2 claim names win", func(c jwt.MapClaims) {
			c["http://wso2.org/claims/emailaddress"] = "jane@wso2.com"
			c["http://wso2.org/claims/groups"] = []string{"app-allocations-admin-super"}
		}, "jane@wso2.com", []string{"app-allocations-admin-super"}},
		{"single group as a string", func(c jwt.MapClaims) { c["groups"] = "wso2-employees" },
			"Jane@wso2.com", []string{"wso2-employees"}},
		{"comma-separated groups", func(c jwt.MapClaims) { c["groups"] = "wso2-employees, app-travel-manager-po" },
			"Jane@wso2.com", []string{"wso2-employees", "app-travel-manager-po"}},
		{"no groups", func(c jwt.MapClaims) { delete(c, "groups") }, "Jane@wso2.com", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeIssuer(t)
			c := validClaims()
			tc.edit(c)
			got, err := f.verifier().Verify(context.Background(), f.sign(c))
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if got.Email != tc.wantEmail || len(got.Groups) != len(tc.wantGroups) {
				t.Fatalf("got %+v", got)
			}
			for i := range tc.wantGroups {
				if got.Groups[i] != tc.wantGroups[i] {
					t.Errorf("groups = %v, want %v", got.Groups, tc.wantGroups)
				}
			}
		})
	}
}

func TestVerify_Rejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(jwt.MapClaims)
	}{
		{"wrong issuer", func(c jwt.MapClaims) { c["iss"] = "https://evil.example/token" }},
		{"wrong audience", func(c jwt.MapClaims) { c["aud"] = []string{"someone-else"} }},
		{"no audience while audiences are configured", func(c jwt.MapClaims) { delete(c, "aud") }},
		{"expired", func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"no expiry", func(c jwt.MapClaims) { delete(c, "exp") }},
		{"issued in the future", func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Hour).Unix() }},
		{"malformed groups", func(c jwt.MapClaims) { c["groups"] = 42 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeIssuer(t)
			c := validClaims()
			tc.edit(c)
			if _, err := f.verifier().Verify(context.Background(), f.sign(c)); err == nil {
				t.Error("Verify accepted the token")
			}
		})
	}
}

// A token without email or groups is valid; the middleware fills them from
// userinfo.
func TestVerify_NoEmailOrGroups(t *testing.T) {
	f := newFakeIssuer(t)
	c := validClaims()
	delete(c, "email")
	delete(c, "groups")
	got, err := f.verifier().Verify(context.Background(), f.sign(c))
	if err != nil || got.Email != "" || got.GroupsPresent {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestMultiVerifier(t *testing.T) {
	a, b := newFakeIssuer(t), newFakeIssuer(t)
	va := a.verifier()
	vb, err := NewVerifier(Config{
		Issuer: "https://gateway.example/token", JWKSURL: b.srv.URL + "/jwks", Audiences: []string{testAudience},
	}, b.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMultiVerifier(va, vb)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(context.Background(), a.sign(validClaims())); err != nil {
		t.Errorf("issuer A: %v", err)
	}
	c := validClaims()
	c["iss"] = "https://gateway.example/token"
	if _, err := m.Verify(context.Background(), b.sign(c)); err != nil {
		t.Errorf("issuer B: %v", err)
	}
	// Signed by A's key but claiming B's issuer: B's keys reject it.
	if _, err := m.Verify(context.Background(), a.sign(c)); err == nil {
		t.Error("accepted a token whose iss names another issuer")
	}
	c["iss"] = "https://evil.example"
	if _, err := m.Verify(context.Background(), a.sign(c)); err == nil {
		t.Error("accepted an untrusted issuer")
	}
	if _, err := NewMultiVerifier(va, va); err == nil {
		t.Error("accepted a duplicate issuer")
	}
}

func TestUserinfoClient(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "s", "email": "jane@wso2.com", "groups": []string{"g1", "g2"}})
	}))
	defer srv.Close()
	u := NewUserinfoClient(srv.URL, srv.Client())
	for i := 0; i < 2; i++ {
		c, err := u.Fetch(context.Background(), "tok")
		if err != nil || c.Email != "jane@wso2.com" || len(c.Groups) != 2 || !c.GroupsPresent {
			t.Fatalf("got %+v, %v", c, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("userinfo called %d times, want 1 (cached)", n)
	}
	if _, err := u.Fetch(context.Background(), "bad"); err == nil {
		t.Error("want an error for a rejected token")
	}
}

// A failing endpoint is not called again for a few seconds: requests fail
// fast instead of each waiting for the timeout.
func TestUserinfoClient_DownEndpointFailsFast(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	u := NewUserinfoClient(srv.URL, srv.Client())
	for _, tok := range []string{"a", "b"} {
		if _, err := u.Fetch(context.Background(), tok); err == nil {
			t.Fatalf("token %s: want an error", tok)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("userinfo called %d times, want 1", n)
	}
}

func TestUserinfoClient_OneCallersFailureDoesNotBlockOthers(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") == "Bearer limited" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"email":"jane@wso2.com","groups":["g"]}`))
	}))
	defer srv.Close()
	u := NewUserinfoClient(srv.URL, srv.Client())
	if _, err := u.Fetch(context.Background(), "limited"); err == nil {
		t.Fatal("429: want an error")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := u.Fetch(cancelled, "aborted"); err == nil {
		t.Fatal("cancelled call: want an error")
	}
	if c, err := u.Fetch(context.Background(), "other"); err != nil || c.Email != "jane@wso2.com" {
		t.Errorf("another caller after a 429 and a cancel: %+v, %v", c, err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("userinfo called %d times, want 2 (the cancelled call never reaches it)", n)
	}
}

func TestVerify_RejectsForgedSignatureAndOtherAlgorithms(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()

	// Same kid, different key.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims())
	forged.Header["kid"] = f.kid
	s, _ := forged.SignedString(other)
	if _, err := v.Verify(context.Background(), s); err == nil {
		t.Error("accepted a token signed by another key")
	}

	// HS256 with the public modulus as the secret (algorithm confusion).
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	hs.Header["kid"] = f.kid
	s, _ = hs.SignedString(f.key.N.Bytes())
	if _, err := v.Verify(context.Background(), s); err == nil {
		t.Error("accepted an HS256 token")
	}

	// alg "none", and a valid RS256 token with its signature cut off.
	none := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
	none.Header["kid"] = f.kid
	s, _ = none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := v.Verify(context.Background(), s); err == nil {
		t.Error("accepted an alg none token")
	}
	signed := f.sign(validClaims())
	unsigned := signed[:strings.LastIndex(signed, ".")+1]
	if _, err := v.Verify(context.Background(), unsigned); err == nil {
		t.Error("accepted a token with an empty signature")
	}

	if _, err := v.Verify(context.Background(), "not-a-jwt"); err == nil {
		t.Error("accepted garbage")
	}
}

// Keys are cached, and a rotated key is picked up by refetching once.
func TestVerify_CachesKeysAndRefetchesOnRotation(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	v.keys.minGap = 0

	for i := 0; i < 3; i++ {
		if _, err := v.Verify(context.Background(), f.sign(validClaims())); err != nil {
			t.Fatalf("Verify: %v", err)
		}
	}
	if n := f.fetches(); n != 1 {
		t.Fatalf("JWKS fetched %d times for one key, want 1", n)
	}

	f.rotate("key-2")
	if _, err := v.Verify(context.Background(), f.sign(validClaims())); err != nil {
		t.Fatalf("Verify after rotation: %v", err)
	}
	if n := f.fetches(); n != 2 {
		t.Errorf("JWKS fetched %d times after rotation, want 2", n)
	}
}

// Many requests verifying at once, across a key rotation, share the key
// cache safely and all pass. Run with -race.
func TestVerify_ConcurrentAcrossRotation(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	v.keys.minGap = 0
	if _, err := v.Verify(context.Background(), f.sign(validClaims())); err != nil {
		t.Fatal(err)
	}
	f.rotate("key-2")
	token := f.sign(validClaims())
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := v.Verify(context.Background(), token); err != nil {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := failed.Load(); n != 0 {
		t.Errorf("%d of 50 concurrent verifies failed", n)
	}
}

// Concurrent lookups of one token share the userinfo cache safely. Run with
// -race.
func TestUserinfoClient_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "s", "email": "jane@wso2.com", "groups": []string{"g1"}})
	}))
	defer srv.Close()
	u := NewUserinfoClient(srv.URL, srv.Client())
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, err := u.Fetch(context.Background(), "tok"); err != nil || c.Email != "jane@wso2.com" {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := failed.Load(); n != 0 {
		t.Errorf("%d of 50 concurrent fetches failed", n)
	}
}

// An unknown kid cannot force a JWKS fetch on every request.
func TestVerify_UnknownKidIsRateLimited(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	c := validClaims()
	for i := 0; i < 5; i++ {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		tok.Header["kid"] = "unknown"
		s, _ := tok.SignedString(f.key)
		if _, err := v.Verify(context.Background(), s); err == nil {
			t.Fatal("accepted a token with an unknown kid")
		}
	}
	if n := f.fetches(); n != 1 {
		t.Errorf("JWKS fetched %d times, want 1", n)
	}
}

// A JWKS that cannot be fetched makes the token's validity unknown
// (ErrKeysUnavailable, a 503 in Auth), and the failure is reused for a while
// instead of being retried on every request.
func TestVerify_KeyFetchFailureIsUnavailable(t *testing.T) {
	f := newFakeIssuer(t)
	f.serve(http.StatusBadGateway, false)
	v := f.verifier()
	for i := 0; i < 3; i++ {
		if _, err := v.Verify(context.Background(), f.sign(validClaims())); !errors.Is(err, ErrKeysUnavailable) {
			t.Fatalf("err = %v, want ErrKeysUnavailable", err)
		}
	}
	if n := f.fetches(); n != 1 {
		t.Errorf("JWKS fetched %d times, want 1 (backoff)", n)
	}

	// Once the endpoint recovers and the backoff passes, the key is fetched.
	f.serve(0, false)
	v.keys.backoff = 0
	if _, err := v.Verify(context.Background(), f.sign(validClaims())); err != nil {
		t.Fatalf("Verify after recovery: %v", err)
	}
}

// The fetch is detached from the request that triggered it: a cancelled
// request does not fail the fetch for everyone else.
func TestVerify_CancelledRequestStillFetchesKeys(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = v.Verify(ctx, f.sign(validClaims()))
	if _, err := v.Verify(context.Background(), f.sign(validClaims())); err != nil {
		t.Fatalf("Verify after a cancelled first request: %v", err)
	}
	if n := f.fetches(); n != 1 {
		t.Errorf("JWKS fetched %d times, want 1", n)
	}
}

// A fetch with no usable key keeps the previous good set.
func TestVerify_EmptyKeySetKeepsTheOldKeys(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	tok := f.sign(validClaims())
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	f.serve(0, true)
	v.keys.minGap, v.keys.maxAge = 0, 0 // the next lookup re-fetches
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify after an empty JWKS: %v", err)
	}
	if n := f.fetches(); n != 2 {
		t.Errorf("JWKS fetched %d times, want 2", n)
	}
}

// Keys older than maxAge are re-fetched, so a key the issuer dropped stops
// being trusted without a restart.
func TestVerify_MaxAgeDropsARemovedKey(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	old := f.sign(validClaims())
	if _, err := v.Verify(context.Background(), old); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	f.rotate("key-2")
	v.keys.minGap, v.keys.maxAge = 0, 0
	if _, err := v.Verify(context.Background(), old); err == nil {
		t.Error("accepted a token signed with a key the issuer no longer publishes")
	}
}

func TestNewVerifier_FailsClosed(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"no issuer", Config{JWKSURL: "https://x/jwks", Audiences: []string{"a"}}},
		{"http JWKS", Config{Issuer: testIssuer, JWKSURL: "http://x/jwks", Audiences: []string{"a"}}},
		{"no audience", Config{Issuer: testIssuer, JWKSURL: "https://x/jwks"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewVerifier(tc.cfg, nil); err == nil {
				t.Error("NewVerifier accepted an incomplete config")
			}
		})
	}
}

// A 401 from userinfo is ErrUserinfoRejected (Auth answers 401) and does
// not mark the endpoint down for other callers.
func TestUserinfoClient_RejectedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer revoked" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"email":"jane@wso2.com","groups":["g"]}`))
	}))
	defer srv.Close()
	u := NewUserinfoClient(srv.URL, srv.Client())
	if _, err := u.Fetch(context.Background(), "revoked"); !errors.Is(err, ErrUserinfoRejected) {
		t.Fatalf("err = %v, want ErrUserinfoRejected", err)
	}
	if _, err := u.Fetch(context.Background(), "good"); err != nil {
		t.Errorf("another token after a 401: %v", err)
	}
}

// While refreshes fail, a cached key is trusted for maxKeyGrace past
// maxAge, and not after it.
func TestVerify_CachedKeyHasAHardExpiry(t *testing.T) {
	f := newFakeIssuer(t)
	v := f.verifier()
	tok := f.sign(validClaims())
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	f.serve(http.StatusBadGateway, false)
	v.keys.minGap, v.keys.backoff, v.keys.maxAge = 0, 0, 0
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("within the grace period: %v", err)
	}
	v.keys.mu.Lock()
	v.keys.lastRefresh = time.Now().Add(-maxKeyGrace - time.Minute)
	v.keys.mu.Unlock()
	if _, err := v.Verify(context.Background(), tok); !errors.Is(err, ErrKeysUnavailable) {
		t.Errorf("past the grace period: err = %v, want ErrKeysUnavailable", err)
	}
	// A successful refresh trusts the key again.
	f.serve(0, false)
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("after recovery: %v", err)
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
		if err := c.CheckRedirect(httptest.NewRequest(http.MethodGet, target, nil), nil); err != nil {
			t.Errorf("%s: %v", target, err)
		}
	}
	if err := c.CheckRedirect(httptest.NewRequest(http.MethodGet, "https://jwks.example/keys", nil), make([]*http.Request, maxJWKSRedirects)); err == nil {
		t.Error("redirect chain longer than the limit was followed")
	}
}
