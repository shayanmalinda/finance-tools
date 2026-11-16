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

package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestReferenceLists(t *testing.T) {
	allocMock, travelMock, er := engagementMocks(t)
	repo := &referenceRepo{alloc: er.alloc, travel: er.travel}
	ctx := context.Background()

	allocMock.ExpectQuery(re(`SELECT id, name, is_active FROM business_unit ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "a"}).AddRow(1, "Marketing", 1).AddRow(2, "Old", 0))
	bus, err := repo.BusinessUnits(ctx)
	if err != nil || len(bus) != 2 || !bus[0].Active || bus[1].Active {
		t.Fatalf("bus %+v err %v", bus, err)
	}
	allocMock.ExpectQuery(re(`SELECT id, name, is_active FROM region`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "a"}).AddRow(1, "APAC", 1))
	if rg, err := repo.Regions(ctx); err != nil || len(rg) != 1 {
		t.Fatalf("regions %+v err %v", rg, err)
	}
	allocMock.ExpectQuery(re(`FROM product_unit AS pu LEFT JOIN business_unit AS bu ON pu.business_unit_id = bu.id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "a", "bu", "c", "i", "m"}).AddRow(4, "APIM", 1, "APIM-Software", 1, 0, 1))
	pus, err := repo.ProductUnits(ctx)
	if err != nil || len(pus) != 1 || *pus[0].BusinessUnit != "APIM-Software" || !pus[0].Customer || pus[0].Internal || !pus[0].Marketing {
		t.Fatalf("pus %+v err %v", pus, err)
	}
	allocMock.ExpectQuery(re(`FROM sub_region AS sr LEFT JOIN region AS rg ON sr.region_id = rg.id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "a", "rg"}).AddRow(7, "NA - EAST", 1, "NA"))
	subs, err := repo.SubRegions(ctx)
	if err != nil || len(subs) != 1 || *subs[0].Region != "NA" {
		t.Fatalf("subs %+v err %v", subs, err)
	}

	travelMock.ExpectQuery(re(`SELECT id, name_of_travel_agent, status, vendor_id FROM travel_agents ORDER BY name_of_travel_agent`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "n", "s", "v"}).AddRow(1, "Classic Travels", "Active", 2339).AddRow(2, "Gone", "Deleted", nil))
	ags, err := repo.TravelAgencies(ctx)
	if err != nil || len(ags) != 2 || !ags[0].Active || *ags[0].NetsuiteVendorID != "2339" || ags[1].Active || ags[1].NetsuiteVendorID != nil {
		t.Fatalf("agencies %+v err %v", ags, err)
	}
	travelMock.ExpectQuery(re(`SELECT id, name FROM inv_cost_type WHERE status = 'active' ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "n"}).AddRow(1, "Air Ticket"))
	if ct, err := repo.CostTypes(ctx); err != nil || len(ct) != 1 || ct[0].Name != "Air Ticket" {
		t.Fatalf("cost types %+v err %v", ct, err)
	}
	// amount is a VARCHAR; a non-numeric row is left out.
	travelMock.ExpectQuery(re(`FROM perdiem_metadata WHERE status = 'Active' ORDER BY id`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "c", "cur", "amt"}).
			AddRow(1, "United Kingdom", "GBP", "70").AddRow(2, "Bad", "USD", "n/a").AddRow(3, "Nan", "USD", "NaN").AddRow(4, "Inf", "USD", "+Inf"))
	rates, err := repo.PerDiemRates(ctx)
	if err != nil || len(rates) != 1 || rates[0].Amount != 70 || rates[0].Currency != "GBP" {
		t.Fatalf("rates %+v err %v", rates, err)
	}
	if err := allocMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
	if err := travelMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
