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
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// Consultant Allocations report: a port of the legacy webapp computation
// (apps/allocation/webapp/src/components/reports/ConsultantAllocations.js
// getAllocationsForReport, groupByAllocation, getMappingEngCodeData) over
// the entity's consultantAllocations, nonAllocatedEmployees,
// default-allocation-data-for-teams and engagement-code-data.

const (
	reportNotDefined   = "NOT_DEFINED"
	reportNA           = "N/A"
	reportNotAvailable = "NOT AVAILABLE"
	reportNoValue      = "ALL" // NO_VAL_STR
	reportLeaveCode    = "LEAVE"
	reportLeftCode     = "LEFT"
	reportHoursPerDay  = 8
)

// reportRnDDesignations are engineerDesignationsForRnd: the job roles
// whose unallocated time is reported against their team default.
var reportRnDDesignations = map[string]bool{
	"Senior Architect": true, "Architect": true, "Associate Architect": true, "Senior Technical Lead": true,
	"Technical Lead": true, "Associate Technical Lead": true, "Senior Software Engineer": true,
	"Software Engineer": true, "Associate Software Engineer": true,
}

// ConsultantReportCSVHeader is the 18 legacy CSV columns, in order.
var ConsultantReportCSVHeader = []string{
	"Engagement Type", "Engagement Location", "Country", "Allocation Type", "Consultant E-mail", "Department",
	"Employee Business Unit", "Company", "Engagement Code", "Daily / Hourly", "Calendar Days", "Working Days",
	"Working Hours", "Engagement Business Unit", "Engagement Sales POD", "Engagement Group", "Account Name", "GL Code",
}

// ConsultantReportService computes the report.
type ConsultantReportService interface {
	Report(ctx context.Context, f domain.ConsultantReportFilter) ([]domain.ConsultantReportRow, error)
}

type consultantReportService struct{ d AllocationDeps }

// NewConsultantReportService constructs a ConsultantReportService.
func NewConsultantReportService(d AllocationDeps) ConsultantReportService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &consultantReportService{d: d}
}

type reportDefault struct{ allocType, code, engType string }

// reportRow is a row before grouping; workingHours nil for daily rows.
type reportRow struct {
	domain.ConsultantReportRow
	hourly bool
}

func reportRound(v float64) float64 { return math.Round(v*100) / 100 }

func reportDays(start, end time.Time) int { return int(math.Floor(end.Sub(start).Hours()/24)) + 1 }

func reportWorkdays(start, end time.Time) int {
	n := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return n
}

func reportKey(s *string) string {
	if s == nil || *s == "" {
		return reportNoValue
	}
	return strings.ToUpper(*s)
}

// reportJSSub is JavaScript's String.prototype.substring(a, b).
func reportJSSub(s string, a, b int) string {
	if a > len(s) {
		a = len(s)
	}
	if b > len(s) {
		b = len(s)
	}
	if a > b {
		a, b = b, a
	}
	return s[a:b]
}

// Report computes the Consultant Allocations report rows.
func (s *consultantReportService) Report(ctx context.Context, f domain.ConsultantReportFilter) ([]domain.ConsultantReportRow, error) {
	start, ok1 := allocParseDate(f.From)
	end, ok2 := allocParseDate(f.To)
	if !ok1 || !ok2 {
		return nil, allocBad("Start/End dates should be valid dates")
	}
	if start.After(end) {
		return nil, allocBad("End Date should be greater than Start Date")
	}
	filter := engagemententity.ConsultantAllocationFilter{StartDate: f.From, EndDate: f.To, Email: f.Consultant,
		AllocationTypeIDs: f.AllocationTypeIDs, BusinessUnit: f.BusinessUnit, Department: f.Department, Team: f.Team}
	allocs, err := s.d.Entity.ConsultantAllocations(ctx, filter)
	if err != nil {
		return nil, err
	}
	defaultRows, err := s.d.Entity.ConsultantTeamDefaults(ctx)
	if err != nil {
		return nil, err
	}
	codeRows, err := s.d.Entity.ConsultantEngagementCodes(ctx)
	if err != nil {
		return nil, err
	}
	defaults := map[string]reportDefault{}
	for _, d := range defaultRows {
		k := reportKey(d.BusinessUnit) + "\x00" + reportKey(d.Department) + "\x00" + reportKey(d.Team)
		if _, dup := defaults[k]; !dup {
			defaults[k] = reportDefault{allocStr(d.DefaultAllocTypeName), allocStr(d.DefaultEngagementCode), allocStr(d.EngagementTypeName)}
		}
	}
	defaultFor := func(bu, dept, team *string) reportDefault {
		d, ok := defaults[reportKey(bu)+"\x00"+reportKey(dept)+"\x00"+reportKey(team)]
		if !ok {
			return reportDefault{reportNotDefined, reportNotDefined, reportNotDefined}
		}
		if d.allocType == "" {
			d.allocType = reportNotDefined
		}
		if d.code == "" {
			d.code = reportNotDefined
		}
		if d.engType == "" {
			d.engType = reportNotDefined
		}
		return d
	}
	defaultRow := func(d reportDefault, email string, dept, bu, company *string, cal, work float64) reportRow {
		return reportRow{ConsultantReportRow: domain.ConsultantReportRow{
			EngagementType: d.engType, EngagementLocation: reportNA, Country: reportNA, AllocationType: d.allocType,
			ConsultantEmail: email, Department: allocStr(dept), EmployeeBusinessUnit: allocStr(bu), Company: allocStr(company),
			EngagementCode: d.code, DailyOrHourly: "Daily", CalendarDays: cal, WorkingDays: work,
			EngagementBusinessUnit: reportNA, EngagementSalesPod: reportNA}}
	}

	period := reportDays(start, end)
	periodWork := reportWorkdays(start, end)
	noTypeFilter := len(f.AllocationTypeIDs) == 0

	// Group by email in first-seen order.
	var order []string
	byEmail := map[string][]engagemententity.ConsultantAllocation{}
	for _, a := range allocs {
		if _, ok := byEmail[a.Email]; !ok {
			order = append(order, a.Email)
		}
		byEmail[a.Email] = append(byEmail[a.Email], a)
	}

	var rows []reportRow
	for _, email := range order {
		list := byEmail[email]
		first := list[0]
		totalDays, totalWork := 0.0, 0.0
		for _, a := range list {
			isDefault := allocIsDefaultID(a.EngagementID)
			orNA := func(v *string) string {
				if v != nil {
					return *v
				}
				if isDefault {
					return reportNA
				}
				return reportNotDefined
			}
			// Engagement BU / Sales POD only with exactly one funding source.
			engBU, engPOD := "", ""
			if len(a.FundingSources) == 1 {
				engBU, engPOD = orNA(a.FundingSources[0].BusinessUnit), orNA(a.FundingSources[0].SubRegion)
			}
			country := allocStr(a.Country)
			if a.Country == nil {
				switch loc := allocStr(a.Location); loc {
				case "UK":
					country = "United Kingdom"
				case "US":
					country = "United States of America"
				default:
					country = loc
				}
			}
			code := allocStr(a.InitialEngagementCode)
			if code == "" {
				code = allocStr(a.EngagementCode)
			}
			if code == "" {
				code = allocStr(a.DefaultEngagementCodeForAllocType)
			}
			typeName, engType := allocStr(a.AllocationTypeName), allocStr(a.EngagementTypeName)
			if code == reportLeaveCode {
				d := defaultFor(a.BusinessUnit, a.Department, a.Team)
				code, typeName, engType = d.code, d.allocType, d.engType
			}
			aStart, _ := allocParseDate(a.StartDate)
			aEnd, _ := allocParseDate(a.EndDate)
			if aStart.Before(start) {
				aStart = start
			}
			if aEnd.After(end) {
				aEnd = end
			}
			days, work := reportDays(aStart, aEnd), reportWorkdays(aStart, aEnd)
			hourly := allocStr(a.IsRecurring) == "Yes"
			row := reportRow{hourly: hourly, ConsultantReportRow: domain.ConsultantReportRow{
				EngagementType: engType, EngagementLocation: orNA(a.EngagementNature), Country: country,
				AllocationType: typeName, ConsultantEmail: a.Email, Department: allocStr(a.Department),
				EmployeeBusinessUnit: allocStr(a.BusinessUnit), Company: allocStr(a.Company), EngagementCode: code,
				DailyOrHourly: "Daily", CalendarDays: float64(days), WorkingDays: float64(work),
				EngagementBusinessUnit: engBU, EngagementSalesPod: engPOD}}
			if hourly {
				perDay := 0.0
				if a.StartTime != nil && a.EndTime != nil {
					st, e1 := time.Parse("15:04", allocTrimSeconds(*a.StartTime))
					et, e2 := time.Parse("15:04", allocTrimSeconds(*a.EndTime))
					if e1 == nil && e2 == nil {
						perDay = et.Sub(st).Hours()
					}
				}
				hours := float64(work) * perDay
				row.DailyOrHourly = "Hourly"
				row.CalendarDays = reportRound(float64(days) * perDay / reportHoursPerDay)
				row.WorkingDays = reportRound(hours / reportHoursPerDay)
				row.WorkingHours = &hours
			}
			totalDays += row.CalendarDays
			totalWork += row.WorkingDays
			if code != reportLeftCode {
				rows = append(rows, row)
			}
		}
		// The rest of the window goes to the team default.
		if noTypeFilter && totalDays < float64(period) {
			last := list[len(list)-1]
			d := defaultFor(last.BusinessUnit, last.Department, last.Team)
			rows = append(rows, defaultRow(d, email, first.Department, first.BusinessUnit, first.Company,
				reportRound(float64(period)-totalDays), reportRound(float64(periodWork)-totalWork)))
		}
	}

	switch {
	case f.Consultant != "" && len(allocs) == 0 && noTypeFilter:
		// No allocations for the one consultant: the employee's window up to
		// the final day of employment goes to the team default. Legacy: an
		// employee with no final day gets no row.
		if s.d.Org == nil {
			break
		}
		rec, err := s.d.Org.EmployeeRecord(ctx, f.Consultant)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			if final, ok := allocParseDate(allocDatePrefix(allocStr(rec.FinalDayOfEmployment))); ok && final.After(start) {
				days, work := period, periodWork
				if final.Before(end) {
					days, work = reportDays(start, final), reportWorkdays(start, final)
				}
				d := defaultFor(rec.BusinessUnit, rec.Department, rec.Team)
				rows = append(rows, defaultRow(d, f.Consultant, rec.Department, rec.BusinessUnit, rec.Company, float64(days), float64(work)))
			}
		}
	case f.Consultant == "" && noTypeFilter:
		emps, err := s.d.Entity.ConsultantNonAllocated(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, e := range emps {
			if !reportRnDDesignations[allocStr(e.JobRole)] {
				continue
			}
			d := defaultFor(e.BusinessUnit, e.Department, e.Team)
			rows = append(rows, defaultRow(d, allocStr(e.WorkEmail), e.Department, e.BusinessUnit, e.Company, float64(period), float64(periodWork)))
		}
	}
	return reportGroup(rows, codeRows), nil
}

func allocDatePrefix(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// reportGroup ports groupByAllocation (sum by engagement type, allocation
// type, email, code and daily/hourly, first-seen order), the GL lookup, and
// the final sort by engagement type, descending.
func reportGroup(rows []reportRow, codeRows []engagemententity.ConsultantEngagementCode) []domain.ConsultantReportRow {
	codes := map[string]engagemententity.ConsultantEngagementCode{}
	var codeOrder []string
	for _, c := range codeRows {
		k := allocStr(c.EngagementCode)
		if k == "" {
			continue
		}
		if _, ok := codes[k]; !ok {
			codeOrder = append(codeOrder, k)
		}
		codes[k] = c
	}
	lookup := func(code string) (engagemententity.ConsultantEngagementCode, bool) {
		if c, ok := codes[code]; ok {
			return c, true
		}
		for _, k := range codeOrder {
			if len(k) > 5 && k[5] == '-' && strings.Index(k[5:], "-") == 0 &&
				reportJSSub(k, 6, 8) == reportJSSub(code, 6, 8) {
				return codes[k], true
			}
		}
		return engagemententity.ConsultantEngagementCode{}, false
	}
	orNotAvailable := func(v *string) string {
		if v == nil || *v == "" {
			return reportNotAvailable
		}
		return *v
	}

	out := []domain.ConsultantReportRow{}
	index := map[string]int{}
	for _, r := range rows {
		mode := "daily"
		if r.hourly {
			mode = "hourly"
		}
		k := strings.Join([]string{r.EngagementType, r.AllocationType, r.ConsultantEmail, r.EngagementCode, mode}, "~")
		if i, ok := index[k]; ok {
			g := &out[i]
			g.CalendarDays = reportRound(g.CalendarDays + r.CalendarDays)
			g.WorkingDays = reportRound(g.WorkingDays + r.WorkingDays)
			if g.WorkingHours != nil && r.WorkingHours != nil {
				sum := *g.WorkingHours + *r.WorkingHours
				g.WorkingHours = &sum
			}
			continue
		}
		g := r.ConsultantReportRow
		g.EngagementGroup, g.AccountName, g.GLCode = reportNotAvailable, reportNotAvailable, reportNotAvailable
		if g.EngagementCode != reportNotDefined {
			if c, ok := lookup(g.EngagementCode); ok {
				g.EngagementGroup, g.AccountName, g.GLCode = orNotAvailable(c.EngGroup), orNotAvailable(c.AccountName), orNotAvailable(c.GLCode)
			}
		}
		index[k] = len(out)
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].EngagementType) > strings.ToLower(out[j].EngagementType)
	})
	return out
}

// ConsultantReportCSVRows renders rows as the 18 legacy CSV columns.
func ConsultantReportCSVRows(rows []domain.ConsultantReportRow) [][]string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		hours := ""
		if r.WorkingHours != nil {
			hours = num(*r.WorkingHours)
		}
		out = append(out, []string{r.EngagementType, r.EngagementLocation, r.Country, r.AllocationType, r.ConsultantEmail,
			r.Department, r.EmployeeBusinessUnit, r.Company, r.EngagementCode, r.DailyOrHourly, num(r.CalendarDays),
			num(r.WorkingDays), hours, r.EngagementBusinessUnit, r.EngagementSalesPod, r.EngagementGroup, r.AccountName, r.GLCode})
	}
	return out
}
