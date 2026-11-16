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

package filestore

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestCheckImage(t *testing.T) {
	if ct, ext, err := CheckImage(pngHeader); err != nil || ct != "image/png" || ext != "png" {
		t.Errorf("png: %s %s %v", ct, ext, err)
	}
	if _, _, err := CheckImage([]byte("%PDF-1.4")); !errors.Is(err, ErrUnsupportedImage) {
		t.Errorf("pdf: %v", err)
	}
}

func TestLegacyRoundTrip(t *testing.T) {
	blob := EncodeLegacy("image/png", pngHeader)
	if !strings.HasPrefix(string(blob), "data:image/png;base64,") {
		t.Fatalf("blob = %s", blob)
	}
	ct, data, err := DecodeLegacy(blob)
	if err != nil || ct != "image/png" || !bytes.Equal(data, pngHeader) {
		t.Errorf("decode: %s %v", ct, err)
	}
	if ct, data, _ := DecodeLegacy(pngHeader); ct != "image/png" || !bytes.Equal(data, pngHeader) {
		t.Errorf("raw bytes: %s", ct)
	}
}

func TestBlobName(t *testing.T) {
	n := BlobName("jane@wso2.com", "L/K", "png")
	if !strings.HasPrefix(n, "jane@wso2.com_") || !strings.HasSuffix(n, "_LK.png") {
		t.Errorf("name = %s", n)
	}
}

func TestAzure_PutGetSigned(t *testing.T) {
	var mu sync.Mutex
	blobs := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "SharedKey acct:") || r.Header.Get("x-ms-date") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("x-ms-blob-type") != "BlockBlob" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			b, _ := io.ReadAll(r.Body)
			blobs[r.URL.Path] = b
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			b, ok := blobs[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(b)
		}
	}))
	defer srv.Close()
	a, err := NewAzure("acct", base64.StdEncoding.EncodeToString([]byte("secret")), srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) }
	ctx := context.Background()
	if err := a.Put(ctx, "visas/jane@wso2.com_x_LK.png", "", bytes.NewReader([]byte("abc")), 3); err != nil {
		t.Fatal(err)
	}
	rc, _, err := a.Get(ctx, "visas/jane@wso2.com_x_LK.png")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "abc" {
		t.Errorf("got %q", b)
	}
	if _, _, err := a.Get(ctx, "visas/missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := a.Put(ctx, "no-container", "", bytes.NewReader(nil), 0); err == nil {
		t.Error("accepted a key without a container")
	}
}

// The signature follows the documented Shared Key string-to-sign; a fixed
// request gives a fixed signature.
func TestAzure_SignatureIsDeterministic(t *testing.T) {
	a, _ := NewAzure("acct", base64.StdEncoding.EncodeToString([]byte("k")), "https://acct.blob.core.windows.net", nil)
	req, _ := http.NewRequest(http.MethodGet, "https://acct.blob.core.windows.net/visas/a.png", nil)
	req.Header.Set("x-ms-date", "Thu, 05 Nov 2026 00:00:00 GMT")
	req.Header.Set("x-ms-version", azureAPIVersion)
	s1, s2 := a.sign(req), a.sign(req)
	if s1 != s2 || s1 == "" {
		t.Errorf("signature %q vs %q", s1, s2)
	}
}

func TestUnconfigured(t *testing.T) {
	if err := (Unconfigured{}).Put(context.Background(), "a/b", "", nil, 0); !errors.Is(err, ErrNotConfigured) {
		t.Error(err)
	}
}
