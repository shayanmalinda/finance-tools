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

package audit

import (
	"strings"
	"testing"
)

// Without audit_event in both schemas the entity refuses to start (unless
// AUDIT_OPTIONAL), naming the migration to run.
func TestCheckTables(t *testing.T) {
	if err := CheckTables(true, true, false); err != nil {
		t.Errorf("both present: %v", err)
	}
	err := CheckTables(false, true, false)
	if err == nil || !strings.Contains(err.Error(), AllocationMigration) || strings.Contains(err.Error(), TravelMigration) {
		t.Errorf("allocation missing: %v", err)
	}
	err = CheckTables(true, false, false)
	if err == nil || !strings.Contains(err.Error(), TravelMigration) || strings.Contains(err.Error(), AllocationMigration) {
		t.Errorf("travel missing: %v", err)
	}
	err = CheckTables(false, false, false)
	if err == nil || !strings.Contains(err.Error(), "allocation_db_table_update_v1.1.0.sql") || !strings.Contains(err.Error(), "travelapp_table_update_v1.0.0.sql") {
		t.Errorf("both missing: %v", err)
	}
	if err := CheckTables(false, false, true); err != nil {
		t.Errorf("AUDIT_OPTIONAL: %v", err)
	}
}
