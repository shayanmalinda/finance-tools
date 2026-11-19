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

// Package redact strips personal data from values written to logs.
package redact

import "strings"

// EmailSegment replaces a path segment that holds an email address.
const EmailSegment = "{email}"

// Path is path without its query (which can carry emails and names) and
// with every segment that holds an email address, plain or percent-encoded,
// replaced by EmailSegment: GET /travellers/{email}/passports names the
// employee in its path.
func Path(path string) string {
	p, _, _ := strings.Cut(path, "?")
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if strings.Contains(s, "@") || strings.Contains(strings.ToLower(s), "%40") {
			segs[i] = EmailSegment
		}
	}
	return strings.Join(segs, "/")
}
