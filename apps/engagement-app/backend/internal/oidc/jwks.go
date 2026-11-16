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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// minRefreshGap bounds JWKS fetches: a token naming an unknown key id
// triggers at most one fetch per gap, so a flood of bogus tokens cannot turn
// into a flood of requests to the identity provider.
const minRefreshGap = 30 * time.Second

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// keySet fetches and caches an issuer's RSA signing keys, refreshing when a
// token names a key id it does not hold (key rotation). It reads only the n/e
// parameters and ignores x5c/x5t, which avoids Go's strict x509 parsing of
// Asgardeo's certificates (see the GRC platform backend's jwksCache).
type keySet struct {
	url        string
	httpClient *http.Client

	minGap time.Duration

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastRefresh time.Time
	fetches     int
}

func newKeySet(url string, httpClient *http.Client) *keySet {
	return &keySet{url: url, httpClient: httpClient, minGap: minRefreshGap}
}

func (k *keySet) lookup(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, fmt.Errorf("oidc: token has no kid header")
	}
	k.mu.RLock()
	key, ok := k.keys[kid]
	k.mu.RUnlock()
	if ok {
		return key, nil
	}
	if err := k.refreshIfDue(ctx); err != nil {
		return nil, err
	}
	k.mu.RLock()
	key, ok = k.keys[kid]
	k.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("oidc: no signing key for kid %q", kid)
	}
	return key, nil
}

func (k *keySet) refreshIfDue(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.lastRefresh.IsZero() && time.Since(k.lastRefresh) < k.minGap {
		return nil
	}
	// Counted before the request, so a failing endpoint is also rate limited.
	k.lastRefresh = time.Now()
	k.fetches++

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return fmt.Errorf("oidc: building JWKS request: %w", err)
	}
	resp, err := k.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("oidc: fetching JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: JWKS endpoint returned %d", resp.StatusCode)
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return fmt.Errorf("oidc: decoding JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, key := range doc.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(key.N, key.E)
		if err != nil {
			continue
		}
		keys[key.Kid] = pub
	}
	k.keys = keys
	return nil
}

func rsaPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e < 3 || e%2 == 0 {
		return nil, fmt.Errorf("invalid RSA exponent %d", e)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}
