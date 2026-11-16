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

// ReferenceItem is a row of business_unit or region.
type ReferenceItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ReferenceProductUnit is a row of product_unit with its business unit name.
type ReferenceProductUnit struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Active       bool    `json:"active"`
	BusinessUnit *string `json:"businessUnit"`
	Customer     bool    `json:"customer"`
	Internal     bool    `json:"internal"`
	Marketing    bool    `json:"marketing"`
}

// ReferenceSubRegion is a row of sub_region with its region name.
type ReferenceSubRegion struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Active bool    `json:"active"`
	Region *string `json:"region"`
}

// ReferenceTravelAgency is a row of travelapp.travel_agents.
type ReferenceTravelAgency struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	NetsuiteVendorID *string `json:"netsuiteVendorId"`
	Active           bool    `json:"active"`
}

// ReferenceCostType is an active row of travelapp.inv_cost_type.
type ReferenceCostType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ReferencePerDiemRate is an Active row of travelapp.perdiem_metadata.
type ReferencePerDiemRate struct {
	Country  string  `json:"country"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// ReferenceDataResponse is returned by GET /reference-data: every lookup
// list the forms and filters read from allocation_db and travelapp.
type ReferenceDataResponse struct {
	EngagementTypes []EngagementType        `json:"engagementTypes"`
	AllocationTypes []AllocationType        `json:"allocationTypes"`
	BusinessUnits   []ReferenceItem         `json:"businessUnits"`
	ProductUnits    []ReferenceProductUnit  `json:"productUnits"`
	Regions         []ReferenceItem         `json:"regions"`
	SubRegions      []ReferenceSubRegion    `json:"subRegions"`
	TravelAgencies  []ReferenceTravelAgency `json:"travelAgencies"`
	CostTypes       []ReferenceCostType     `json:"costTypes"`
	PerDiemRates    []ReferencePerDiemRate  `json:"perDiemRates"`
}
