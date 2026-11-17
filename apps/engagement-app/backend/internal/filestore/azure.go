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
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const azureAPIVersion = "2021-08-06"

// Azure is a Store over Azure Blob Storage with Shared Key authorisation
// (the account name and access key the legacy Travel backend uses:
// [app_travel.file.fileServer] accountName and FILE_STORAGE_ACCESS_KEY).
type Azure struct {
	account  string
	key      []byte
	endpoint string // https://<account>.blob.core.windows.net
	http     *http.Client
	now      func() time.Time
}

// NewAzure returns an Azure store. accessKey is the base64 account key.
// endpoint empty means the public Azure endpoint of account.
func NewAzure(account, accessKey, endpoint string, httpClient *http.Client) (*Azure, error) {
	k, err := base64.StdEncoding.DecodeString(accessKey)
	if err != nil {
		return nil, fmt.Errorf("filestore: FILE_STORAGE_ACCESS_KEY is not base64: %w", err)
	}
	if endpoint == "" {
		endpoint = "https://" + account + ".blob.core.windows.net"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Azure{account: account, key: k, endpoint: strings.TrimRight(endpoint, "/"), http: httpClient, now: time.Now}, nil
}

func (a *Azure) blobURL(key string) (*url.URL, error) {
	container, blob, ok := strings.Cut(key, "/")
	if !ok || container == "" || blob == "" {
		return nil, fmt.Errorf("filestore: key %q must be container/blob", key)
	}
	return url.Parse(a.endpoint + "/" + url.PathEscape(container) + "/" + url.PathEscape(blob))
}

// Put uploads a block blob.
func (a *Azure) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	u, err := a.blobURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), r)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("filestore: put %s: status %d", key, resp.StatusCode)
	}
	return nil
}

// Get downloads a blob.
func (a *Azure) Get(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	u, err := a.blobURL(key)
	if err != nil {
		return nil, Object{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, Object{}, err
	}
	resp, err := a.do(req)
	if err != nil {
		return nil, Object{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, Object{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, Object{}, fmt.Errorf("filestore: get %s: status %d", key, resp.StatusCode)
	}
	return resp.Body, Object{Key: key, ContentType: resp.Header.Get("Content-Type"), Size: resp.ContentLength}, nil
}

// Delete removes a blob; a missing blob is not an error.
func (a *Azure) Delete(ctx context.Context, key string) error {
	u, err := a.blobURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := a.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("filestore: delete %s: status %d", key, resp.StatusCode)
	}
	return nil
}

func (a *Azure) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("x-ms-date", a.now().UTC().Format(http.TimeFormat))
	req.Header.Set("x-ms-version", azureAPIVersion)
	req.Header.Set("Authorization", "SharedKey "+a.account+":"+a.sign(req))
	return a.http.Do(req)
}

// sign computes the Shared Key signature (Blob service, version 2015-02-21
// and later: an empty Content-Length for zero bytes).
func (a *Azure) sign(req *http.Request) string {
	length := ""
	if req.ContentLength > 0 {
		length = strconv.FormatInt(req.ContentLength, 10)
	}
	h := req.Header
	var ms []string
	for k := range h {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "x-ms-") {
			ms = append(ms, lk)
		}
	}
	sort.Strings(ms)
	var canonHeaders strings.Builder
	for _, k := range ms {
		canonHeaders.WriteString(k + ":" + strings.TrimSpace(h.Get(k)) + "\n")
	}
	resource := "/" + a.account + req.URL.EscapedPath()
	q := req.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		resource += "\n" + strings.ToLower(k) + ":" + strings.Join(vals, ",")
	}
	toSign := strings.Join([]string{
		req.Method,
		h.Get("Content-Encoding"), h.Get("Content-Language"), length, h.Get("Content-MD5"), h.Get("Content-Type"),
		"", // Date (x-ms-date is used)
		h.Get("If-Modified-Since"), h.Get("If-Match"), h.Get("If-None-Match"), h.Get("If-Unmodified-Since"), h.Get("Range"),
	}, "\n") + "\n" + canonHeaders.String() + resource
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(toSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
