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

package rest

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/reqcontext"
	"golang.org/x/oauth2/clientcredentials"
)

func TestDo_ForwardsCorrelationIDAndDecodes(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Correlation-ID")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	ctx := reqcontext.WithCorrelationID(context.Background(), "abc-123")
	var out struct{ Status string }
	if err := New("test", srv.URL+"/", nil).Get(ctx, "/health", &out); err != nil {
		t.Fatal(err)
	}
	if gotID != "abc-123" || out.Status != "ok" {
		t.Errorf("correlation id %q, body %+v", gotID, out)
	}
}

func TestDo_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantMsg    string
		clientErr  bool
	}{
		{"entity error shape", 404, `{"code":404,"message":"engagement E1 not found"}`, 404, "engagement E1 not found", true},
		{"sales error shape", 400, `{"message":"invalid request body"}`, 400, "invalid request body", true},
		{"no body", 503, ``, 503, "Service Unavailable", false},
		{"html body", 502, `<html>bad gateway</html>`, 502, "Bad Gateway", false},
		{"malformed 2xx body", 200, `{not json`, 200, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			var out map[string]any
			err := New("engagement-entity", srv.URL, nil).Get(context.Background(), "/x", &out)
			var ue *apierror.UpstreamError
			if !errors.As(err, &ue) {
				t.Fatalf("err = %v, want UpstreamError", err)
			}
			if ue.StatusCode != tc.wantStatus || (tc.wantMsg != "" && ue.Msg != tc.wantMsg) || ue.ClientError() != tc.clientErr {
				t.Errorf("got %+v (client error %v)", ue, ue.ClientError())
			}
		})
	}
}

// A body over the cap is a clear 400, not a dependency failure.
func TestDo_ResponseTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":["` + strings.Repeat("x", 100) + `"]}`))
	}))
	defer srv.Close()
	defer func(n int64) { maxResponseBody = n }(maxResponseBody)
	maxResponseBody = 20
	var out map[string]any
	var ve *apierror.ValidationError
	if err := New("entity", srv.URL, nil).Get(context.Background(), "/x", &out); !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
}

func TestDo_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	err := New("hr-entity", url, nil).Get(context.Background(), "/x", nil)
	var ue *apierror.UpstreamError
	if !errors.As(err, &ue) || ue.StatusCode != 0 || ue.ClientError() {
		t.Errorf("err = %v, want an UpstreamError with no status", err)
	}
}

// Logged paths drop the query (it can carry emails or search text), also for
// an unreachable dependency, whose *url.Error repeats the full URL.
func TestDo_LogsNoQuery(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	_ = New("hr-entity", srv.URL, nil).Get(context.Background(), "/visas?email=jane%40wso2.com", nil)
	srv.Close()
	_ = New("hr-entity", srv.URL, nil).Get(context.Background(), "/visas?email=jane%40wso2.com", nil)

	out := buf.String()
	if strings.Count(out, "path=/visas") != 2 || strings.Contains(out, "jane") {
		t.Errorf("logs = %s", out)
	}
}

// A failed token fetch nests the token endpoint's *url.Error inside the
// resource one: neither URL reaches the log.
func TestDo_LogsNoTokenURL(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	down := httptest.NewServer(http.NotFoundHandler())
	tokenURL := down.URL + "/oauth2/token?tenant=secret-tenant"
	down.Close()
	cfg := clientcredentials.Config{ClientID: "id", ClientSecret: "secret", TokenURL: tokenURL}
	_ = New("hr-entity", "https://hr.example", cfg.Client(context.Background())).Get(context.Background(), "/x", nil)

	if out := buf.String(); strings.Contains(out, "secret-tenant") || strings.Contains(out, "/oauth2/token") ||
		!strings.Contains(out, "dependency request failed") || !strings.Contains(out, "cause=connection") {
		t.Errorf("logs = %s", out)
	}
}

// A redirect with a malformed Location puts that URL in Go's error text: the
// log names only the failure's kind.
func TestDo_LogsNoRedirectTarget(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://[::1%zz]/next?token=leaked-value")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	if err := New("hr-entity", srv.URL, srv.Client()).Get(context.Background(), "/x", nil); err == nil {
		t.Fatal("want an error")
	}
	if out := buf.String(); strings.Contains(out, "leaked-value") || !strings.Contains(out, "cause=transport") {
		t.Errorf("logs = %s", out)
	}
}

// The token request carries scope= when scopes are set and no scope
// parameter when they are not.
func TestNewOAuthHTTPClient_Scopes(t *testing.T) {
	for name, tc := range map[string]struct {
		scopes []string
		want   string
		sent   bool
	}{
		"set":   {[]string{"engagement:read", "engagement:finance"}, "engagement:read engagement:finance", true},
		"empty": {nil, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			var got string
			var sent bool
			tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				got, sent = r.PostForm.Get("scope"), r.PostForm.Has("scope")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokens.Close()
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			defer api.Close()

			c := NewOAuthHTTPClient(tokens.URL, "id", "secret", tc.scopes)
			resp, err := c.Get(api.URL)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if sent != tc.sent || got != tc.want {
				t.Errorf("scope sent %v = %q, want %v %q", sent, got, tc.sent, tc.want)
			}
		})
	}
}
