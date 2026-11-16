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

// Package domain holds the request and response types shared across handlers,
// services and repositories.
package domain

// App names one of the two legacy apps. Each keeps its own roles, privileges
// and role_privilege tables in its own schema, and a privilege id means
// different things in each: Allocations privilege 1 is the Dashboard page,
// Travel privilege 1 is something else. The two are never merged.
type App string

// The two legacy apps.
const (
	// AppAllocations is the Allocations app (allocation_db).
	AppAllocations App = "allocations"
	// AppTravel is the Travel app (travelapp).
	AppTravel App = "travel"
)

// Apps lists every App, in a fixed order.
var Apps = []App{AppAllocations, AppTravel}

// Valid reports whether a is one of Apps.
func (a App) Valid() bool {
	for _, known := range Apps {
		if a == known {
			return true
		}
	}
	return false
}

// Role is a row of a legacy roles table. Its name is an Asgardeo group name
// (e.g. "app-allocations-admin-super"): membership comes from the caller's
// groups, there is no user-to-role table.
type Role struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Privilege is an active row of a legacy privileges table.
type Privilege struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// ListRolesResponse is returned by GET /access/{app}/roles.
type ListRolesResponse struct {
	Roles []Role `json:"roles"`
}

// ListPrivilegesResponse is returned by GET /access/{app}/privileges.
type ListPrivilegesResponse struct {
	Privileges []Privilege `json:"privileges"`
}

// RolePrivilegeMapResponse is returned by GET /access/{app}/role-privileges.
//
// The whole mapping is returned in one call rather than answering "may this
// group do X?" per request: the Engagement App Backend caches it and
// resolves a caller's groups locally, so an access check never becomes a
// network round trip.
type RolePrivilegeMapResponse struct {
	// RolePrivileges maps a role (group) name to the privilege ids it grants,
	// ascending. A role with no privileges is absent.
	RolePrivileges map[string][]int `json:"rolePrivileges"`
}

// TravelAgency is a row of travelapp.travel_agents.
type TravelAgency struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// TravelAgencyResponse is returned by GET /access/travel/agency. Agency is
// null when the person is not an active travel agent employee.
type TravelAgencyResponse struct {
	Agency *TravelAgency `json:"agency"`
}
