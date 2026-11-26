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

import "testing"

func TestKnownSort(t *testing.T) {
	lists := map[string][]string{
		"engagements": EngagementSorts, "approvals": ApprovalSorts, "allocations": AllocationSorts,
		"invoices": InvoiceSorts, "payments": PaymentSorts, "visas": VisaSorts, "documents": DocumentSorts,
	}
	for name, keys := range lists {
		for _, k := range keys {
			for _, s := range []string{k, "-" + k} {
				if got := KnownSort(s, keys); got != s {
					t.Errorf("%s: KnownSort(%q) = %q, want it kept", name, s, got)
				}
			}
		}
		for _, bad := range []string{"", "-", "--" + keys[0], "unknown", "amount", "current", keys[0] + " DESC", keys[0] + ";DROP TABLE x", "ID"} {
			if got := KnownSort(bad, keys); got != "" {
				t.Errorf("%s: KnownSort(%q) = %q, want empty", name, bad, got)
			}
		}
	}
	// A key of another list is unknown here.
	if got := KnownSort("businessUnit", EngagementSorts); got != "" {
		t.Errorf("businessUnit on engagements = %q", got)
	}
}
