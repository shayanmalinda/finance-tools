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

package domain

import "strings"

// The column sorts each list accepts in its optional `sort` query parameter.
// A leading "-" means descending; the entity service adds an id tiebreak.
var (
	EngagementSorts = []string{"id", "type", "country", "startDate", "requestStatus", "engagementStatus", "created"}
	ApprovalSorts   = []string{"id", "type", "country", "startDate", "businessUnit", "created"}
	AllocationSorts = []string{"id", "consultant", "engagement", "type", "startDate", "status", "created"}
	InvoiceSorts    = []string{"invoiceNumber", "jobNumber", "costType", "addedDate", "status"}
	PaymentSorts    = []string{"number", "jobNumber", "addedDate", "status"}
	VisaSorts       = []string{"traveller", "country", "visaType", "validFrom", "expiry"}
	DocumentSorts   = []string{"traveller", "type", "country", "issued", "expiry"}
)

// KnownSort returns sort when it is one of keys (with an optional leading
// "-"), and "" otherwise. An empty result makes the entity service keep the
// list's default order, so an unknown sort is ignored rather than rejected
// and the previous web app, which sends an empty one, sees no change.
func KnownSort(sort string, keys []string) string {
	key := strings.TrimPrefix(sort, "-")
	for _, k := range keys {
		if k == key {
			return sort
		}
	}
	return ""
}
