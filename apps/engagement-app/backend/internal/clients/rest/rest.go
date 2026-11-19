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

// Package rest is the small JSON-over-HTTP client every outbound dependency
// client is built on. It forwards the request's correlation ID, caps response
// sizes, and turns every failure into *apierror.UpstreamError so handlers map
// dependency failures the same way everywhere.
package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/redact"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
)

const (
	// DefaultTimeout bounds one outbound call end to end.
	DefaultTimeout = 30 * time.Second

	correlationIDHeader = "X-Correlation-ID"
	maxErrorBody        = 8 << 10
)

// maxResponseBody is high for the unpaged reports (a 10-year Consultant
// Allocations report is over 16 MiB).
var maxResponseBody int64 = 128 << 20

// Client calls one dependency at baseURL.
type Client struct {
	service string
	baseURL string
	http    *http.Client
}

// New returns a Client for service (a name used in errors and logs) at
// baseURL. httpClient nil uses a plain client with DefaultTimeout.
func New(service, baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	return &Client{service: service, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

// NewOAuthHTTPClient returns an http.Client that fetches, caches and renews
// an OAuth2 client-credentials token, for dependencies in other Choreo
// projects reached through a Choreo connection. scopes, when given, are
// requested in the token; empty sends no scope parameter.
func NewOAuthHTTPClient(tokenURL, clientID, clientSecret string, scopes []string) *http.Client {
	cc := clientcredentials.Config{ClientID: clientID, ClientSecret: clientSecret, TokenURL: tokenURL, Scopes: scopes}
	// The token source fetches with the HTTP client on the construction
	// context, so give it one with a timeout; otherwise a stalled token
	// endpoint blocks every request behind the token mutex.
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: DefaultTimeout})
	c := cc.Client(tokenCtx)
	c.Timeout = DefaultTimeout
	return c
}

// Get performs GET path and decodes a 2xx JSON body into out (out may be nil).
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodGet, path, nil, out)
}

// Post performs POST path with a JSON body, decoding a 2xx body into out.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.Do(ctx, http.MethodPost, path, body, out)
}

// Do performs one request. path is appended to the base URL as is; callers
// escape any user-supplied segment (url.PathEscape) or query value
// (url.Values).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: marshal request: %w", c.service, err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", c.service, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if id := reqcontext.CorrelationID(ctx); id != "" {
		req.Header.Set(correlationIDHeader, id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Transport error text can carry URLs (a redirect's Location, the
		// token endpoint) and a token endpoint error can quote its response
		// body, so neither is logged: a token failure logs its status, any
		// other failure a fixed category.
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			status := 0
			if re.Response != nil {
				status = re.Response.StatusCode
			}
			slog.ErrorContext(ctx, "dependency token request failed", "service", c.service, "method", method,
				"path", redact.Path(path), "tokenStatus", status)
		} else {
			slog.ErrorContext(ctx, "dependency request failed", "service", c.service, "method", method,
				"path", redact.Path(path), "cause", transportCause(err))
		}
		return &apierror.UpstreamError{Service: c.service, Msg: "unreachable"}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := http.StatusText(resp.StatusCode)
		// Entity services answer {"code":..,"message":".."} or {"message":".."}.
		var e struct {
			Message string `json:"message"`
		}
		if raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody)); len(raw) > 0 {
			if json.Unmarshal(raw, &e) == nil && e.Message != "" {
				msg = e.Message
			}
		}
		slog.WarnContext(ctx, "dependency returned an error", "service", c.service, "method", method,
			"path", redact.Path(path), "status", resp.StatusCode)
		return &apierror.UpstreamError{Service: c.service, StatusCode: resp.StatusCode, Msg: msg}
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))
		return nil
	}
	lr := &io.LimitedReader{R: resp.Body, N: maxResponseBody}
	if err := json.NewDecoder(lr).Decode(out); err != nil {
		if lr.N == 0 {
			return &apierror.ValidationError{Msg: "The result is too large to return. Narrow the filters, for example a shorter date range."}
		}
		if ctx.Err() != nil {
			// The deadline ended while the body was read: a stop, like one
			// during the request, not a malformed answer.
			return ctx.Err()
		}
		return &apierror.UpstreamError{Service: c.service, StatusCode: resp.StatusCode, Msg: "malformed response: " + err.Error()}
	}
	return nil
}

// transportCause names the kind of a transport failure without its text.
func transportCause(err error) string {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var certErr *tls.CertificateVerificationError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.As(err, &certErr):
		return "tls"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.As(err, &opErr):
		return "connection"
	default:
		return "transport"
	}
}
