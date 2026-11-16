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

// Package engagemententity is the client for the Engagement Entity
// (entity/engagement-entity), the only service that touches the legacy
// allocation_db and travelapp databases and NetSuite. The entity is visible
// to this Choreo project only and needs no credentials.
package engagemententity

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Client is what the backend needs from the Engagement Entity. Services
// depend on these interfaces so tests can use a fake. Each screen area
// declares its own methods in its own file (allocation.go, travel.go,
// finance.go, dashboard.go); HTTPClient implements them all.
type Client interface {
	AccessAPI
	EngagementAPI
	AllocationAPI
	TravelAPI
	FinanceAPI
	DashboardAPI
	AuditAPI
}

// AccessAPI is the roles, privileges and lookups shared by every area.
type AccessAPI interface {
	// RolePrivileges returns app's role (group) name to privilege ids map.
	RolePrivileges(ctx context.Context, app domain.App) (map[string][]int, error)
	Roles(ctx context.Context, app domain.App) ([]Role, error)
	Privileges(ctx context.Context, app domain.App) ([]Privilege, error)
	// TravelAgency returns the travel agency email works for, or nil.
	TravelAgency(ctx context.Context, email string) (*domain.TravelAgency, error)
	EngagementTypes(ctx context.Context, f EngagementTypeFilter) ([]EngagementType, error)
	AllocationTypes(ctx context.Context, f AllocationTypeFilter) ([]AllocationType, error)
}

// Role is a legacy role; its name is an Asgardeo group name.
type Role struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Privilege is an active legacy privilege.
type Privilege struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// EngagementType is a row of engagement_type.
type EngagementType struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"isActive"`
}

// EngagementTypeFilter narrows EngagementTypes. Zero values mean "any".
type EngagementTypeFilter struct {
	Type     string // Internal | Customer
	IsActive *bool
}

// AllocationType is a row of allocation_type with its engagement type.
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

// AllocationTypeFilter narrows AllocationTypes. Zero values mean "any".
type AllocationTypeFilter struct {
	EngagementTypeID int
	Status           string // Active | Deleted
}

// HTTPClient implements Client over HTTP.
type HTTPClient struct{ rest *rest.Client }

// PushTimeout bounds a NetSuite push or re-sync call (the entity allows
// NetSuite 300 s). Other calls end with the 30 s request context.
const PushTimeout = 330 * time.Second

// New returns an HTTPClient for the entity at baseURL. httpClient nil uses
// a plain client with PushTimeout.
func New(baseURL string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: PushTimeout}
	}
	return &HTTPClient{rest: rest.New("engagement-entity", baseURL, withUser(httpClient))}
}

var _ Client = (*HTTPClient)(nil)

func accessPath(app domain.App, resource string) string {
	return "/access/" + url.PathEscape(string(app)) + "/" + resource
}

// RolePrivileges calls GET /access/{app}/role-privileges.
func (c *HTTPClient) RolePrivileges(ctx context.Context, app domain.App) (map[string][]int, error) {
	var resp struct {
		RolePrivileges map[string][]int `json:"rolePrivileges"`
	}
	if err := c.rest.Get(ctx, accessPath(app, "role-privileges"), &resp); err != nil {
		return nil, err
	}
	return resp.RolePrivileges, nil
}

// Roles calls GET /access/{app}/roles.
func (c *HTTPClient) Roles(ctx context.Context, app domain.App) ([]Role, error) {
	var resp struct {
		Roles []Role `json:"roles"`
	}
	if err := c.rest.Get(ctx, accessPath(app, "roles"), &resp); err != nil {
		return nil, err
	}
	return resp.Roles, nil
}

// Privileges calls GET /access/{app}/privileges.
func (c *HTTPClient) Privileges(ctx context.Context, app domain.App) ([]Privilege, error) {
	var resp struct {
		Privileges []Privilege `json:"privileges"`
	}
	if err := c.rest.Get(ctx, accessPath(app, "privileges"), &resp); err != nil {
		return nil, err
	}
	return resp.Privileges, nil
}

// TravelAgency calls GET /access/travel/agency.
func (c *HTTPClient) TravelAgency(ctx context.Context, email string) (*domain.TravelAgency, error) {
	var resp struct {
		Agency *domain.TravelAgency `json:"agency"`
	}
	if err := c.rest.Get(ctx, withQuery("/access/travel/agency", url.Values{"email": {email}}), &resp); err != nil {
		return nil, err
	}
	return resp.Agency, nil
}

// EngagementTypes calls GET /engagement-types.
func (c *HTTPClient) EngagementTypes(ctx context.Context, f EngagementTypeFilter) ([]EngagementType, error) {
	q := url.Values{}
	if f.Type != "" {
		q.Set("type", f.Type)
	}
	if f.IsActive != nil {
		q.Set("isActive", strconv.FormatBool(*f.IsActive))
	}
	var resp struct {
		EngagementTypes []EngagementType `json:"engagementTypes"`
	}
	if err := c.rest.Get(ctx, withQuery("/engagement-types", q), &resp); err != nil {
		return nil, err
	}
	return resp.EngagementTypes, nil
}

// AllocationTypes calls GET /allocation-types.
func (c *HTTPClient) AllocationTypes(ctx context.Context, f AllocationTypeFilter) ([]AllocationType, error) {
	q := url.Values{}
	if f.EngagementTypeID > 0 {
		q.Set("engagementTypeId", strconv.Itoa(f.EngagementTypeID))
	}
	if f.Status != "" {
		q.Set("status", f.Status)
	}
	var resp struct {
		AllocationTypes []AllocationType `json:"allocationTypes"`
	}
	if err := c.rest.Get(ctx, withQuery("/allocation-types", q), &resp); err != nil {
		return nil, err
	}
	return resp.AllocationTypes, nil
}

func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}
