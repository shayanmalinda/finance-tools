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

package main

import (
	"reflect"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/config"
)

// Live emails go to their real recipients (no redirect); otherwise every email
// goes to the test recipients.
func TestTestRecipients(t *testing.T) {
	r := []string{"qa@wso2.com"}
	if got := testRecipients(config.EmailConfig{Live: true, TestRecipients: r}); got != nil {
		t.Errorf("live: %v, want nil", got)
	}
	if got := testRecipients(config.EmailConfig{TestRecipients: r}); !reflect.DeepEqual(got, r) {
		t.Errorf("not live: %v", got)
	}
}
