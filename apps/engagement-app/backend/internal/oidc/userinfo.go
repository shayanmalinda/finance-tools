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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	userinfoTTL      = 5 * time.Minute
	userinfoCapacity = 1000
	// userinfoDownFor is how long an unreachable or failing endpoint (a
	// transport error or 5xx) is not called again, so requests fail fast
	// instead of each waiting for the client timeout. A 429 or a request
	// the caller cancelled says nothing about the endpoint for other users,
	// so neither trips it.
	userinfoDownFor = 5 * time.Second
)

// UserinfoClient reads email and groups missing from an access token from the
// userinfo endpoint, cached per token hash, as the invoice-app backend does.
type UserinfoClient struct {
	url  string
	http *http.Client

	mu        sync.Mutex
	cache     map[string]userinfoEntry
	downUntil time.Time
	downErr   error
}

type userinfoEntry struct {
	claims  Claims
	expires time.Time
}

// NewUserinfoClient returns a client for the userinfo endpoint at url.
// httpClient nil uses a client with a 10 second timeout.
func NewUserinfoClient(url string, httpClient *http.Client) *UserinfoClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &UserinfoClient{url: url, http: httpClient, cache: map[string]userinfoEntry{}}
}

// ErrUserinfoRejected reports that the userinfo endpoint refused the access
// token (401): the token is not valid there, so the caller must sign in
// again rather than retry.
var ErrUserinfoRejected = errors.New("oidc: userinfo rejected the token")

// Fetch returns the email and groups userinfo reports for accessToken.
func (u *UserinfoClient) Fetch(ctx context.Context, accessToken string) (Claims, error) {
	sum := sha256.Sum256([]byte(accessToken))
	key := hex.EncodeToString(sum[:])
	now := time.Now()
	u.mu.Lock()
	if e, ok := u.cache[key]; ok && now.Before(e.expires) {
		u.mu.Unlock()
		return e.claims, nil
	}
	if now.Before(u.downUntil) {
		err := u.downErr
		u.mu.Unlock()
		return Claims{}, err
	}
	u.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return Claims{}, fmt.Errorf("oidc: building userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := u.http.Do(req)
	if err != nil {
		err = fmt.Errorf("oidc: calling userinfo: %w", err)
		if ctx.Err() != nil {
			return Claims{}, err
		}
		return Claims{}, u.down(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return Claims{}, u.down(fmt.Errorf("oidc: userinfo returned %d", resp.StatusCode))
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return Claims{}, ErrUserinfoRejected
	}
	if resp.StatusCode != http.StatusOK {
		return Claims{}, fmt.Errorf("oidc: userinfo returned %d", resp.StatusCode)
	}
	var body tokenClaims
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Claims{}, fmt.Errorf("oidc: decoding userinfo: %w", err)
	}
	groups, err := parseGroups(body.WSO2Groups)
	if err == nil && len(groups) == 0 {
		groups, err = parseGroups(body.Groups)
	}
	if err != nil {
		return Claims{}, fmt.Errorf("oidc: userinfo groups: %w", err)
	}
	c := Claims{
		Subject:       body.Subject,
		Email:         firstNonEmpty(body.WSO2Email, body.Email),
		Groups:        groups,
		GroupsPresent: claimPresent(body.WSO2Groups) || claimPresent(body.Groups),
	}

	u.mu.Lock()
	if len(u.cache) >= userinfoCapacity {
		for k, e := range u.cache {
			if now.After(e.expires) || len(u.cache) >= userinfoCapacity {
				delete(u.cache, k)
			}
		}
	}
	u.cache[key] = userinfoEntry{claims: c, expires: now.Add(userinfoTTL)}
	u.mu.Unlock()
	return c, nil
}

// down records that the endpoint itself failed, so calls for the next
// userinfoDownFor return err without waiting on it.
func (u *UserinfoClient) down(err error) error {
	u.mu.Lock()
	u.downUntil, u.downErr = time.Now().Add(userinfoDownFor), err
	u.mu.Unlock()
	return err
}
