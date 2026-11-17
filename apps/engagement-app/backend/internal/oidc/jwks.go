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
	"errors"
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

// failureBackoff is how long a failed fetch is reused before the next try;
// it rate limits a failing endpoint without holding a good key set back.
const failureBackoff = 5 * time.Second

// maxKeyAge re-fetches the key set when it is older than this, so a key the
// identity provider rotated out or revoked stops being trusted.
const maxKeyAge = 30 * time.Minute

// fetchTimeout bounds one JWKS fetch. The fetch is detached from the request
// that triggered it, so one cancelled request cannot fail it for everyone.
const fetchTimeout = 10 * time.Second

// ErrKeysUnavailable reports that the signing keys could not be fetched: the
// token's validity is unknown, not refused.
var ErrKeysUnavailable = errors.New("oidc: signing keys unavailable")

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

	minGap  time.Duration
	backoff time.Duration
	maxAge  time.Duration

	// refreshMu serialises refreshes; mu guards only the key map and the
	// fetch state, so a slow JWKS fetch never blocks cached-key lookups.
	refreshMu   sync.Mutex
	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastRefresh time.Time // last successful fetch
	lastFailure time.Time
	lastErr     error
	fetches     int
}

func newKeySet(url string, httpClient *http.Client) *keySet {
	return &keySet{url: url, httpClient: httpClient, minGap: minRefreshGap, backoff: failureBackoff, maxAge: maxKeyAge}
}

func (k *keySet) lookup(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, fmt.Errorf("oidc: token has no kid header")
	}
	k.mu.RLock()
	key, ok := k.keys[kid]
	stale := !k.lastRefresh.IsZero() && time.Since(k.lastRefresh) > k.maxAge
	k.mu.RUnlock()
	if ok && !stale {
		return key, nil
	}
	if err := k.refreshIfDue(ctx); err != nil && !ok {
		return nil, err
	}
	// A failed max-age refresh keeps the cached set: the key was good at the
	// last successful fetch.
	k.mu.RLock()
	key, ok = k.keys[kid]
	k.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("oidc: no signing key for kid %q", kid)
	}
	return key, nil
}

func (k *keySet) refreshIfDue(ctx context.Context) error {
	k.refreshMu.Lock()
	defer k.refreshMu.Unlock()
	k.mu.Lock()
	if !k.lastRefresh.IsZero() && time.Since(k.lastRefresh) < k.minGap {
		k.mu.Unlock()
		return nil
	}
	// A failing endpoint is rate limited too: its error is reused for a while.
	if k.lastErr != nil && time.Since(k.lastFailure) < k.backoff {
		err := k.lastErr
		k.mu.Unlock()
		return err
	}
	k.fetches++
	k.mu.Unlock()

	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
	defer cancel()
	keys, err := k.fetch(fctx)

	k.mu.Lock()
	defer k.mu.Unlock()
	if err != nil {
		k.lastErr = fmt.Errorf("%w: %w", ErrKeysUnavailable, err)
		k.lastFailure = time.Now()
		return k.lastErr
	}
	k.keys, k.lastRefresh, k.lastErr = keys, time.Now(), nil
	return nil
}

// fetch reads the JWKS. A set with no usable RSA key is an error, so the
// previous good set is kept.
func (k *keySet) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return nil, fmt.Errorf("building JWKS request: %w", err)
	}
	resp, err := k.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding JWKS: %w", err)
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
	if len(keys) == 0 {
		return nil, errors.New("JWKS has no usable RSA key")
	}
	return keys, nil
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
