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

// Engagement type kinds, the values of engagement_type.type.
const (
	EngagementKindInternal = "Internal"
	EngagementKindCustomer = "Customer"
)

// EngagementType is a row of allocation_db.engagement_type, e.g.
// {5, "Marketing Event", "Internal", true}.
type EngagementType struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"isActive"`
}

// EngagementTypeFilter narrows GET /engagement-types. Zero values mean "any".
type EngagementTypeFilter struct {
	// Type is "Internal", "Customer" or "" for both.
	Type string
	// IsActive filters on is_active when set.
	IsActive *bool
}

// ListEngagementTypesResponse is returned by GET /engagement-types.
type ListEngagementTypesResponse struct {
	EngagementTypes []EngagementType `json:"engagementTypes"`
}

// AllocationType is a row of allocation_db.allocation_type joined to its
// engagement type. Field names follow the legacy Allocations backend's
// AllocationType record so the screens keep their wording.
type AllocationType struct {
	ID                    int     `json:"id"`
	AllocationTypeName    string  `json:"allocationTypeName"`
	Status                string  `json:"status"`
	Color                 *string `json:"color"`
	Department            *string `json:"department"`
	EmailToAllocations    *string `json:"emailToAllocations"`
	DefaultEngagementCode *string `json:"defaultEngagementCode"`
	EngagementCodeSuffix  *string `json:"engagementCodeSuffix"`
	EngagementTypeID      int     `json:"engagementTypeId"`
	EngagementTypeName    string  `json:"engagementTypeName"`
}

// Allocation type statuses the legacy list shows. Rows in any other status
// never appear.
const (
	AllocationTypeStatusActive  = "Active"
	AllocationTypeStatusDeleted = "Deleted"
)

// AllocationTypeFilter narrows GET /allocation-types. Zero values mean "any".
type AllocationTypeFilter struct {
	EngagementTypeID int
	// Status is "Active", "Deleted" or "" for both.
	Status string
}

// ListAllocationTypesResponse is returned by GET /allocation-types.
type ListAllocationTypesResponse struct {
	AllocationTypes []AllocationType `json:"allocationTypes"`
}
