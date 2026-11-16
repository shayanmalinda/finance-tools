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

package service

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

func TestConsultantReport_LegacyComputation(t *testing.T) {
	jane := func(a engagemententity.ConsultantAllocation) engagemententity.ConsultantAllocation {
		a.Email, a.BusinessUnit, a.Department, a.Team, a.Company, a.Location = "jane@wso2.com", sp("Eng"), sp("Dev"), sp("T1"), sp("WSO2 UK"), sp("UK")
		return a
	}
	ent := &allocFakeEntity{
		consultant: []engagemententity.ConsultantAllocation{
			jane(engagemententity.ConsultantAllocation{ID: "A1", StartDate: "2026-10-30", EndDate: "2026-11-04", EngagementID: sp("E1"),
				InitialEngagementCode: sp("NWB-CO"), AllocationTypeName: sp("Consulting"), EngagementTypeName: sp("Paid Post Sale"),
				EngagementNature: sp("On-site"), Country: sp("Sri Lanka"),
				FundingSources: []engagemententity.AllocationFundingSource{{BusinessUnit: sp("APIM"), SubRegion: sp("NA - EAST")}}}),
			jane(engagemententity.ConsultantAllocation{ID: "A2", StartDate: "2026-11-05", EndDate: "2026-11-05", EngagementID: sp("-1"),
				InitialEngagementCode: sp("LEAVE"), AllocationTypeName: sp("Leave"), EngagementTypeName: sp("Internal Engagement")}),
			jane(engagemententity.ConsultantAllocation{ID: "A3", StartDate: "2026-11-06", EndDate: "2026-11-06", EngagementID: sp("-1"),
				InitialEngagementCode: sp("LEFT"), AllocationTypeName: sp("Left"), EngagementTypeName: sp("Internal Engagement")}),
			{ID: "A4", Email: "bob@wso2.com", StartDate: "2026-11-01", EndDate: "2026-11-30", EngagementID: sp("E2"), IsRecurring: sp("Yes"),
				StartTime: sp("09:00"), EndTime: sp("11:00"), EngagementCode: sp("SAL01-PRX"), AllocationTypeName: sp("Support"),
				EngagementTypeName: sp("Non-Paid Post Sale"), EngagementNature: sp("Off-site"), Country: sp("India")},
		},
		nonAlloc: []engagemententity.ConsultantNonAllocatedEmployee{
			{WorkEmail: sp("carl@wso2.com"), JobRole: sp("Software Engineer"), BusinessUnit: sp("eng"), Department: sp("dev"), Team: sp("t1")},
			{WorkEmail: sp("dave@wso2.com"), JobRole: sp("HR Manager")},
		},
		defaults: []engagemententity.ConsultantTeamDefault{
			{BusinessUnit: sp("Eng"), Department: sp("Dev"), Team: sp("T1"), DefaultAllocTypeName: sp("Digital"), EngagementTypeName: sp("Internal Engagement"), DefaultEngagementCode: sp("DIG-GEN")},
		},
		codes: []engagemententity.ConsultantEngagementCode{
			{EngagementCode: sp("NWB-CO"), EngGroup: sp("Delivery"), AccountName: sp("Revenue"), GLCode: sp("4000")},
			{EngagementCode: sp("SAL99-PRY"), EngGroup: sp("Sales"), AccountName: nil, GLCode: sp("5000")},
		},
	}
	svc := NewConsultantReportService(allocTestDeps(ent, allocTestHR(), &allocFakeSender{}))
	rows, err := svc.Report(context.Background(), domain.ConsultantReportFilter{From: "2026-11-02", To: "2026-11-08"})
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		typ, email, code   string
		cal, work          float64
		hours              *float64
		glGroup, accountGL string
		location, country  string
	}
	ten := 10.0
	wants := []want{
		{"Paid Post Sale", "jane@wso2.com", "NWB-CO", 3, 3, nil, "Delivery", "Revenue/4000", "On-site", "Sri Lanka"},
		{"NOT_DEFINED", "bob@wso2.com", "NOT_DEFINED", 5.25, 3.75, nil, "NOT AVAILABLE", "NOT AVAILABLE/NOT AVAILABLE", "N/A", "N/A"},
		{"Non-Paid Post Sale", "bob@wso2.com", "SAL01-PRX", 1.75, 1.25, &ten, "Sales", "NOT AVAILABLE/5000", "Off-site", "India"},
		{"Internal Engagement", "jane@wso2.com", "DIG-GEN", 3, 1, nil, "NOT AVAILABLE", "NOT AVAILABLE/NOT AVAILABLE", "N/A", "United Kingdom"},
		{"Internal Engagement", "carl@wso2.com", "DIG-GEN", 7, 5, nil, "NOT AVAILABLE", "NOT AVAILABLE/NOT AVAILABLE", "N/A", "N/A"},
	}
	if len(rows) != len(wants) {
		t.Fatalf("rows = %+v", rows)
	}
	for i, w := range wants {
		r := rows[i]
		hoursOK := (w.hours == nil) == (r.WorkingHours == nil) && (w.hours == nil || *w.hours == *r.WorkingHours)
		if r.EngagementType != w.typ || r.ConsultantEmail != w.email || r.EngagementCode != w.code || r.CalendarDays != w.cal ||
			r.WorkingDays != w.work || !hoursOK || r.EngagementGroup != w.glGroup || r.AccountName+"/"+r.GLCode != w.accountGL ||
			r.EngagementLocation != w.location || r.Country != w.country {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
	if rows[0].EngagementBusinessUnit != "APIM" || rows[0].EngagementSalesPod != "NA - EAST" || rows[2].DailyOrHourly != "Hourly" {
		t.Errorf("funding / hourly = %+v / %+v", rows[0], rows[2])
	}
	csv := ConsultantReportCSVRows(rows)
	if len(csv[0]) != 18 || len(ConsultantReportCSVHeader) != 18 || csv[2][12] != "10" || csv[2][10] != "1.75" || csv[0][12] != "" {
		t.Errorf("csv = %v", csv)
	}

	// One consultant, no allocations: up to the final day goes to the team
	// default; none without a final day (legacy).
	ent.consultant = nil
	hr := allocTestHR()
	hr.records["gone@wso2.com"] = &hrentity.EmployeeRecord{BusinessUnit: sp("Eng"), Department: sp("Dev"), Team: sp("T1"), FinalDayOfEmployment: sp("2026-11-04")}
	svc = NewConsultantReportService(allocTestDeps(ent, hr, &allocFakeSender{}))
	rows, err = svc.Report(context.Background(), domain.ConsultantReportFilter{From: "2026-11-02", To: "2026-11-08", Consultant: "gone@wso2.com"})
	if err != nil || len(rows) != 1 || rows[0].CalendarDays != 3 || rows[0].WorkingDays != 3 || rows[0].AllocationType != "Digital" {
		t.Errorf("leaver = %+v %v", rows, err)
	}
	rows, err = svc.Report(context.Background(), domain.ConsultantReportFilter{From: "2026-11-02", To: "2026-11-08", Consultant: "jane@wso2.com"})
	if err != nil || len(rows) != 0 {
		t.Errorf("no final day = %+v %v", rows, err)
	}
	// A type filter skips the gap and the unallocated people.
	rows, err = svc.Report(context.Background(), domain.ConsultantReportFilter{From: "2026-11-02", To: "2026-11-08", AllocationTypeIDs: []int{1}})
	if err != nil || len(rows) != 0 {
		t.Errorf("type filter = %+v %v", rows, err)
	}
	if _, err := svc.Report(context.Background(), domain.ConsultantReportFilter{From: "2026-11-09", To: "2026-11-08"}); !isAllocValidation(err, "End Date should be greater than Start Date") {
		t.Errorf("order: %v", err)
	}
}
