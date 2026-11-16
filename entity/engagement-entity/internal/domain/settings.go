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

// SettingsAllocationTypeInput is the body of POST /settings/allocation-types
// and PUT /settings/allocation-types/{id} (legacy AllocationTypeRecord).
// Empty optional strings are stored as NULL.
type SettingsAllocationTypeInput struct {
	EngagementTypeID      int    `json:"engagementTypeId"`
	AllocationTypeName    string `json:"allocationTypeName"`
	DefaultEngagementCode string `json:"defaultEngagementCode"`
	EngagementCodeSuffix  string `json:"engagementCodeSuffix"`
	Department            string `json:"department"`
	// EmailToAllocations is Yes, No or empty.
	EmailToAllocations string `json:"emailToAllocations"`
	Color              string `json:"color"`
	// Actor is the acting user's email, written to the audit log.
	Actor string `json:"actor"`
}

// SettingsAllocationTypeDeleteRequest is the body of
// DELETE /settings/allocation-types/{id}.
type SettingsAllocationTypeDeleteRequest struct {
	Actor string `json:"actor"`
}
