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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MultiVerifier verifies a token with the Verifier of its issuer (Asgardeo,
// and the Choreo gateway when configured).
type MultiVerifier struct {
	byIssuer map[string]*Verifier
}

// NewMultiVerifier builds a MultiVerifier from verifiers with distinct issuers.
func NewMultiVerifier(vs ...*Verifier) (*MultiVerifier, error) {
	if len(vs) == 0 {
		return nil, errors.New("oidc: at least one issuer is required")
	}
	m := &MultiVerifier{byIssuer: make(map[string]*Verifier, len(vs))}
	for _, v := range vs {
		if _, dup := m.byIssuer[v.cfg.Issuer]; dup {
			return nil, fmt.Errorf("oidc: issuer %q configured twice", v.cfg.Issuer)
		}
		m.byIssuer[v.cfg.Issuer] = v
	}
	return m, nil
}

// Verify validates rawToken with the Verifier of its issuer.
func (m *MultiVerifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	iss, err := unverifiedIssuer(rawToken)
	if err != nil {
		return Claims{}, err
	}
	v, ok := m.byIssuer[iss]
	if !ok {
		return Claims{}, fmt.Errorf("oidc: untrusted issuer %q", iss)
	}
	return v.Verify(ctx, rawToken)
}

func unverifiedIssuer(raw string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("oidc: token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("oidc: token payload is not base64url")
	}
	var c struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &c); err != nil || c.Iss == "" {
		return "", errors.New("oidc: token has no issuer")
	}
	return c.Iss, nil
}
