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

package netsuite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"sort"
	"strings"
)

const signatureMethod = "HMAC-SHA256"

// formEncode is Ballerina's url:encode(v, "UTF-8"), i.e. Java's
// URLEncoder.encode: letters, digits and ".-*_" are kept, a space becomes
// "+", every other byte of the UTF-8 encoding becomes %XX (upper case).
// It differs from Go's url.QueryEscape on "*" (kept) and "~" (%7E).
func formEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '.', c == '-', c == '*', c == '_':
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// oauthHeader ports the legacy buildOAuthValue for one request to rawURL
// (the full URL with its already-encoded query string). The query
// parameters are merged with the protocol parameters as they appear in the
// URL (not decoded), sorted by key, joined as k=v with "&", and encoded as a
// whole into the signature base string, exactly as the legacy code does.
func oauthHeader(c Config, method, rawURL, nonce, timestamp string) string {
	params := map[string]string{
		"oauth_consumer_key":     c.ConsumerKey,
		"oauth_token":            c.TokenID,
		"oauth_timestamp":        timestamp,
		"oauth_nonce":            nonce,
		"oauth_signature_method": signatureMethod,
		"oauth_version":          "1.0",
	}
	base := rawURL
	if i := strings.Index(rawURL, "?"); i >= 0 {
		base = rawURL[:i]
		for _, part := range strings.Split(rawURL[i+1:], "&") {
			k, v, _ := strings.Cut(part, "=")
			if _, isProtocol := params[k]; !isProtocol {
				params[k] = v
			}
		}
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + params[k]
	}
	baseString := method + "&" + formEncode(base) + "&" + formEncode(strings.Join(pairs, "&"))

	mac := hmac.New(sha256.New, []byte(formEncode(c.ConsumerSecret)+"&"+formEncode(c.TokenSecret)))
	mac.Write([]byte(baseString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return `OAuth realm="` + c.AccountID + `",` +
		`oauth_consumer_key="` + c.ConsumerKey + `",` +
		`oauth_token="` + formEncode(c.TokenID) + `",` +
		`oauth_signature="` + formEncode(signature) + `",` +
		`oauth_timestamp="` + timestamp + `",` +
		`oauth_nonce="` + nonce + `",` +
		`oauth_signature_method="` + signatureMethod + `",` +
		`oauth_version="1.0"`
}
