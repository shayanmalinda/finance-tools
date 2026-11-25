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

// Global search group kinds, in the order GET /search returns them.
const (
	SearchTravelRequests  = "travelRequests"
	SearchInvoices        = "invoices"
	SearchPerDiemPayments = "perDiemPayments"
	SearchEngagements     = "engagements"
	SearchAllocations     = "allocations"
)

// SearchItem is one hit of a global search group.
type SearchItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Status   string `json:"status"`
	// Segment is the engagement segment (customer, internal or marketing) of
	// an engagement or allocation hit, for building the link.
	Segment string `json:"segment,omitempty"`
	// JobNumber is the travel request of an invoice or per diem payment hit.
	JobNumber string `json:"jobNumber,omitempty"`
}

// SearchGroup is the hits of one kind: the first items and the total number
// of matches.
type SearchGroup struct {
	Kind  string       `json:"kind"`
	Total int          `json:"total"`
	Items []SearchItem `json:"items"`
}

// SearchResponse is GET /search. Groups the caller may not search or that
// have no match are left out; Failed names the groups whose source failed
// or timed out.
type SearchResponse struct {
	Query  string        `json:"query"`
	Groups []SearchGroup `json:"groups"`
	Failed []string      `json:"failed"`
}
