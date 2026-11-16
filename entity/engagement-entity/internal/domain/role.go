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

// RoleWithPrivileges is a role of one legacy app with the privilege ids its
// role_privilege rows grant, ascending.
type RoleWithPrivileges struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PrivilegeIDs []int  `json:"privilegeIds"`
}

// RoleSettingsResponse is returned by GET /role-settings/{app}.
type RoleSettingsResponse struct {
	Roles      []RoleWithPrivileges `json:"roles"`
	Privileges []Privilege          `json:"privileges"`
}

// RolePrivilegesUpdateRequest is the body of
// PUT /role-settings/{app}/roles/{roleId}/privileges: the new set replaces
// the role's privileges.
type RolePrivilegesUpdateRequest struct {
	PrivilegeIDs []int  `json:"privilegeIds"`
	UpdatedBy    string `json:"updatedBy"`
}
