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

// Package domain holds the request and response types of the backend's API.
package domain

// App names one of the two legacy apps whose privileges the backend enforces.
// Each app's privilege ids come from its own role tables and mean different
// things: Allocations privilege 1 is the Dashboard page, Travel privilege 1
// is not. They are never merged or compared across apps.
type App string

// The two legacy apps.
const (
	AppAllocations App = "allocations"
	AppTravel      App = "travel"
)

// Apps lists every App, in a fixed order.
var Apps = []App{AppAllocations, AppTravel}

// AppAccess is what the caller may do in one legacy app.
type AppAccess struct {
	// Privileges are the legacy privilege ids the caller's groups grant in
	// this app, ascending and without repeats. Empty, never null.
	Privileges []int `json:"privileges"`
	// OwnPrivileges leave out the super admin group's grants; work queues use
	// them.
	OwnPrivileges []int `json:"ownPrivileges"`
	// Roles are the caller's groups that are roles of this app.
	Roles []string `json:"roles"`
}

// TravelAccess is AppAccess plus the caller's travel agency.
type TravelAccess struct {
	AppAccess
	// Agency is the travel agency of an agent (holder of Travel 18 listed in
	// travel_agent_employee); null for everyone else.
	Agency *TravelAgency `json:"agency"`
}

// WorkQueues are the Invoices & Payments tabs the caller sees, decided on
// the server with the legacy rules, so the webapp shows exactly these.
type WorkQueues struct {
	// Invoices are the invoice tabs in order: approve-po (own Travel 32),
	// approve (own 33 and no 32 held at all), push (own 34), all (39).
	Invoices []string `json:"invoices"`
	// PerDiemPayments are the payment tabs in order: push (own 40), all (41).
	PerDiemPayments []string `json:"perDiemPayments"`
	// NetSuiteSyncFailures: the NS Sync Failures tab (own 34 or own 40).
	NetSuiteSyncFailures bool `json:"netsuiteSyncFailures"`
}

// UserInfoResponse is returned by GET /user-info. One WSO2 gates every
// Finance and Me screen on it: each app's privileges are read from that
// app's field only.
type UserInfoResponse struct {
	Email       string       `json:"email"`
	Allocations AppAccess    `json:"allocations"`
	Travel      TravelAccess `json:"travel"`
	WorkQueues  WorkQueues   `json:"workQueues"`
	// AccessRequestEmail is where to ask for access (ACCESS_REQUEST_EMAIL);
	// omitted when not configured.
	AccessRequestEmail string `json:"accessRequestEmail,omitempty"`
}
