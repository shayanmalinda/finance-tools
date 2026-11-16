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

import (
	"sort"
	"strings"
)

// Person is a user named by email, with a display name when known.
type Person struct {
	Email string  `json:"email"`
	Name  *string `json:"name"`
}

// CampaignRef is the Salesforce campaign of a Marketing Event (Release 1).
type CampaignRef struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// Page is a list response: one page of items and the total number of matches.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Gate is the legacy privilege check of an endpoint: the caller passes when
// they hold at least one listed id in at least one listed app ("any of", as
// every legacy interceptor did). Apps are never mixed: Allocations 1 and
// Travel 1 are different privileges.
type Gate map[App][]int

// PrivilegeSet is a set of one app's privilege ids.
type PrivilegeSet map[int]bool

// NewPrivilegeSet builds a set from ids.
func NewPrivilegeSet(ids []int) PrivilegeSet {
	s := make(PrivilegeSet, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// Has reports whether the set holds id.
func (s PrivilegeSet) Has(id int) bool { return s[id] }

// Any reports whether the set holds at least one of ids.
func (s PrivilegeSet) Any(ids ...int) bool {
	for _, id := range ids {
		if s[id] {
			return true
		}
	}
	return false
}

// IDs returns the ids ascending, never nil.
func (s PrivilegeSet) IDs() []int {
	out := make([]int, 0, len(s))
	for id := range s {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// TravelAgency is a travel agency (travelapp.travel_agents).
type TravelAgency struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Perms is everything the caller may do, resolved once per request from their
// Asgardeo groups and the legacy role tables. Services decide record-level
// rules (scope, ownership, state) from it.
type Perms struct {
	// Email is the caller's work email, lower-cased.
	Email string
	// Alloc and Travel are the privileges the caller's groups grant in each
	// legacy app.
	Alloc  PrivilegeSet
	Travel PrivilegeSet
	// AllocOwn and TravelOwn leave out the super admin groups' grants
	// (app-allocations-admin-super, app-travel-admin-super). Work queues and
	// "Waiting on You" counts use them, as the prototype does.
	AllocOwn  PrivilegeSet
	TravelOwn PrivilegeSet
	// AllocRoles and TravelRoles are the caller's groups that are roles of
	// each app, as named in the role tables.
	AllocRoles  []string
	TravelRoles []string
	// Agency is the travel agency the caller works for (travel_agent_employee),
	// resolved only for holders of Travel 18 (Agent Travel Requests Page).
	Agency *TravelAgency
}

// IsAgent reports whether the caller acts as a travel agent: they hold Travel
// 18 and work for an agency, and do not see every travel request (Travel 1
// or 2).
func (p Perms) IsAgent() bool {
	return p.Agency != nil && p.Travel.Has(18) && !p.Travel.Any(1, 2)
}

// Passes reports whether the caller passes g.
func (p Perms) Passes(g Gate) bool {
	for app, ids := range g {
		switch app {
		case AppAllocations:
			if p.Alloc.Any(ids...) {
				return true
			}
		case AppTravel:
			if p.Travel.Any(ids...) {
				return true
			}
		}
	}
	return false
}

// Super admin groups. Their grants never create a work queue.
const (
	AllocationsSuperAdminGroup = "app-allocations-admin-super"
	TravelSuperAdminGroup      = "app-travel-admin-super"
)

// IsGroup reports whether role is group, ignoring case and an environment
// suffix (staging roles are named "<group>-stg").
func IsGroup(role, group string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	for _, sfx := range []string{"-stg", "-dev", "-uat"} {
		r = strings.TrimSuffix(r, sfx)
	}
	return group != "" && r == strings.ToLower(group)
}

// HasRole reports whether any of roles is group.
func HasRole(roles []string, group string) bool {
	for _, r := range roles {
		if IsGroup(r, group) {
			return true
		}
	}
	return false
}
