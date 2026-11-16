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

// ReferenceNamedItem is a business unit or region.
type ReferenceNamedItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ReferenceProductUnit is a product unit with its business unit and the
// engagement kinds it may fund.
type ReferenceProductUnit struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Active       bool    `json:"active"`
	BusinessUnit *string `json:"businessUnit"`
	Customer     bool    `json:"customer"`
	Internal     bool    `json:"internal"`
	Marketing    bool    `json:"marketing"`
}

// ReferenceSubRegion is a sales sub region with its region.
type ReferenceSubRegion struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Active bool    `json:"active"`
	Region *string `json:"region"`
}

// ReferenceEngagementType is an engagement type.
type ReferenceEngagementType struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"isActive"`
}

// ReferenceAllocationType is an allocation type with its engagement type.
type ReferenceAllocationType struct {
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

// ReferenceTravelAgency is a travel agency.
type ReferenceTravelAgency struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	NetsuiteVendorID *string `json:"netsuiteVendorId"`
	Active           bool    `json:"active"`
}

// ReferencePerDiemRate is an active per diem rate.
type ReferencePerDiemRate struct {
	Country  string  `json:"country"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// ReferenceData is GET /reference-data: every dropdown list of the forms
// and filters.
type ReferenceData struct {
	EngagementTypes         []ReferenceEngagementType `json:"engagementTypes"`
	AllocationTypes         []ReferenceAllocationType `json:"allocationTypes"`
	BusinessUnits           []ReferenceNamedItem      `json:"businessUnits"`
	ProductUnits            []ReferenceProductUnit    `json:"productUnits"`
	Regions                 []ReferenceNamedItem      `json:"regions"`
	SubRegions              []ReferenceSubRegion      `json:"subRegions"`
	Countries               []string                  `json:"countries"`
	EngagementNatures       []string                  `json:"engagementNatures"`
	EventNatures            []string                  `json:"eventNatures"`
	RequestStatuses         []string                  `json:"requestStatuses"`
	EngagementStatuses      []string                  `json:"engagementStatuses"`
	ClearanceStatuses       []string                  `json:"clearanceStatuses"`
	ClearanceTransitions    map[string][]string       `json:"clearanceTransitions"`
	TravelStatuses          []string                  `json:"travelStatuses"`
	TravelStatusTransitions map[string][]string       `json:"travelStatusTransitions"`
	TravelAgencies          []ReferenceTravelAgency   `json:"travelAgencies"`
	CostTypes               []string                  `json:"costTypes"`
	PerDiemRates            []ReferencePerDiemRate    `json:"perDiemRates"`
	Currencies              []string                  `json:"currencies"`
	InvoiceCurrencies       []string                  `json:"invoiceCurrencies"`
	TravelCardCurrencies    []string                  `json:"travelCardCurrencies"`
	VisaTypes               []string                  `json:"visaTypes"`
	EntryTypes              []string                  `json:"entryTypes"`
	TravelDocumentTypes     []string                  `json:"travelDocumentTypes"`
	TimeZones               []string                  `json:"timeZones"`
}

// ReferenceAccount is a Salesforce account (account search).
type ReferenceAccount struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Country     *string `json:"country"`
	SalesRegion *string `json:"salesRegion"`
	SubRegion   *string `json:"subRegion"`
	Address     *string `json:"address"`
}

// ReferenceLineItem is an opportunity line item.
type ReferenceLineItem struct {
	ID               string  `json:"id"`
	Product          string  `json:"product"`
	EngagementCode   *string `json:"engagementCode"`
	StartDate        *string `json:"startDate"`
	EndDate          *string `json:"endDate"`
	Description      *string `json:"description"`
	BusinessUnit     *string `json:"businessUnit"`
	ProductUnit      *string `json:"productUnit"`
	IsTentativeDates *bool   `json:"isTentativeDates"`
}

// ReferenceOpportunity is a Salesforce opportunity with its line items.
type ReferenceOpportunity struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Owner          *string             `json:"owner"`
	TechnicalOwner *string             `json:"technicalOwner"`
	IsWon          *bool               `json:"isWon"`
	Address        *string             `json:"address"`
	PreSalesCode   *string             `json:"preSalesCode"`
	PostSalesCode  *string             `json:"postSalesCode"`
	LineItems      []ReferenceLineItem `json:"lineItems"`
}

// ReferenceCampaign is a Salesforce campaign.
type ReferenceCampaign struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      *string `json:"type"`
	Status    *string `json:"status"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
}

// ReferenceEmployee is an HR employee (person pickers).
type ReferenceEmployee struct {
	Email        string  `json:"email"`
	Name         string  `json:"name"`
	EmployeeID   *string `json:"employeeId"`
	JobRole      *string `json:"jobRole"`
	JobBand      *string `json:"jobBand"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	Team         *string `json:"team"`
	Company      *string `json:"company"`
	Location     *string `json:"location"`
	ManagerEmail *string `json:"managerEmail"`
}

// ReferenceDepartment is a department with its teams.
type ReferenceDepartment struct {
	Name  string   `json:"name"`
	Teams []string `json:"teams"`
}

// ReferenceBusinessUnit is a business unit with its departments.
type ReferenceBusinessUnit struct {
	Name        string                `json:"name"`
	Departments []ReferenceDepartment `json:"departments"`
}

// ReferenceOrgStructure is GET /org-structure.
type ReferenceOrgStructure struct {
	BusinessUnits []ReferenceBusinessUnit `json:"businessUnits"`
}

// ReferenceItems is the {items: [...]} answer of the lookups.
type ReferenceItems[T any] struct {
	Items []T `json:"items"`
}

// RoleSettingsRole is a role of one legacy app with its privilege ids.
type RoleSettingsRole struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PrivilegeIDs []int  `json:"privilegeIds"`
}

// RoleSettingsPrivilege is an active privilege of one legacy app.
type RoleSettingsPrivilege struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// RoleSettings is GET /role-settings/{app}.
type RoleSettings struct {
	Roles      []RoleSettingsRole      `json:"roles"`
	Privileges []RoleSettingsPrivilege `json:"privileges"`
}

// RoleSettingsUpdate is the body of PUT /role-settings/{app}/roles/{roleId}/privileges.
type RoleSettingsUpdate struct {
	PrivilegeIDs []int `json:"privilegeIds"`
}

// SettingsAllocationTypeInput is the body of the allocation type writes.
type SettingsAllocationTypeInput struct {
	EngagementTypeID      int    `json:"engagementTypeId"`
	AllocationTypeName    string `json:"allocationTypeName"`
	DefaultEngagementCode string `json:"defaultEngagementCode"`
	EngagementCodeSuffix  string `json:"engagementCodeSuffix"`
	Department            string `json:"department"`
	EmailToAllocations    string `json:"emailToAllocations"`
	Color                 string `json:"color"`
}
