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
	"database/sql"
	"errors"
	"fmt"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
)

// DashboardRepository reads the dashboard aggregates. Every method runs
// against one schema (allocation_db, travelapp or expense_app); nothing joins
// across schemas. Dates are YYYY-MM-DD strings and ranges are half-open
// ([from, to)), so DATE and DATETIME columns compare the same way.
type DashboardRepository interface {
	// allocation_db
	AllocationsToday(ctx context.Context, f domain.DashboardAllocationsTodayFilter) (domain.DashboardAllocationsToday, error)
	OwnAllocations(ctx context.Context, email, today string) ([]domain.DashboardOwnAllocation, error)
	TeamAllocationOn(ctx context.Context, email, day, nextDay string) (*domain.DashboardTeamAllocation, error)
	PendingApprovalCounts(ctx context.Context, engagementTypes []int) (map[int]int, error)
	ClearanceCounts(ctx context.Context, engagementTypes []int) (map[string]int, error)
	EngagementsStarting(ctx context.Context, engagementTypes []int, from, to string, limit int) ([]domain.DashboardEngagement, int, error)
	Bench(ctx context.Context, from, to string) ([]domain.DashboardBenchRow, error)
	AllocationsByStartMonth(ctx context.Context, from, to string) ([]domain.DashboardCount, error)
	AllocationsByBusinessUnit(ctx context.Context, from, to string) ([]domain.DashboardCount, error)

	// travelapp
	UpcomingTrips(ctx context.Context, f DashboardTripFilter) ([]domain.DashboardTrip, int, error)
	NewTravelRequestCount(ctx context.Context) (int, error)
	InvoiceQueueCounts(ctx context.Context) (DashboardInvoiceQueues, error)
	PaymentQueueCounts(ctx context.Context) (toPush, failed int, err error)
	TravelKPIs(ctx context.Context, w DashboardWindow) (DashboardTravelKPIs, error)
	TravelCells(ctx context.Context, w DashboardWindow) ([]DashboardTravelCell, error)
	InvoiceCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error)
	PaymentCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error)
	AgencyKPIs(ctx context.Context, agencyID int, w DashboardWindow) (DashboardAgencyKPIs, error)
	AgencyInvoicesByStatus(ctx context.Context, agencyID int) ([]domain.DashboardCount, error)
	AgencyTripsByMonth(ctx context.Context, agencyID int, w DashboardWindow) ([]domain.DashboardCount, error)
	AgencyPerDiems(ctx context.Context, agencyID, limit int) ([]domain.DashboardAgencyPerDiem, error)

	// expense_app
	CardCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error)
	ClaimCost(ctx context.Context, w DashboardWindow) ([]domain.DashboardCostLine, error)
}

// DashboardWindow holds the day boundaries a dashboard query needs.
type DashboardWindow struct {
	Today       string // today
	Tomorrow    string // today + 1
	In7Days     string // today + 8 (exclusive bound for "start <= today + 7")
	In30Days    string // today + 31 (exclusive bound for "start <= today + 30")
	MonthStart  string // first day of the current month
	NextMonth   string // first day of the next month
	YearStart   string // first day of the month 11 months back (12-month window)
	BenchEndDay string // today + 13 (Available Consultants window end, inclusive)
}

// DashboardTripFilter narrows UpcomingTrips. At most one of Email and AgencyID.
type DashboardTripFilter struct {
	Today    string
	Email    string // the traveller
	AgencyID int    // the assigned travel agency
	Limit    int
}

// DashboardInvoiceQueues are the invoice work-queue counts.
type DashboardInvoiceQueues struct {
	Pending, AcceptedByTravel, ToPush, Failed int
}

// DashboardTravelKPIs are the Travel section tiles.
type DashboardTravelKPIs struct {
	InProgress, StartingIn30Days, CompletedThisMonth, Last12Months, Upcoming int
}

// DashboardTravelCell is a count of trips by start month, country and agency.
type DashboardTravelCell struct {
	Month   string
	Country string
	Agency  *string
	Count   int
}

// DashboardAgencyKPIs are the travel agent's At a Glance tiles from travel.
type DashboardAgencyKPIs struct {
	InProgress, StartingIn7Days, StartingIn30Days, CompletedWithoutInvoice, Upcoming int
}

// ErrDashboardNoExpenseDB is returned when card spend is asked for without an
// expense_app pool.
var ErrDashboardNoExpenseDB = errors.New("expense_app pool is not configured")

type dashboardRepo struct {
	alloc, travel, expense *sql.DB
}

// NewDashboardRepository constructs a DashboardRepository over the
// allocation_db, travelapp and expense_app (read-only) pools.
func NewDashboardRepository(alloc, travel, expense *sql.DB) DashboardRepository {
	return &dashboardRepo{alloc: alloc, travel: travel, expense: expense}
}

// Shared predicates.
const (
	// dashboardActiveAllocation is an allocation that is live and not
	// rejected or cancelled: the legacy lists read status 'Active' only, and
	// the clearance test is the prototype's ACTIVE (no "Rejected" or
	// "Cancelled" in the clearance status; old rows with none count).
	dashboardActiveAllocation = `a.status = 'Active'
		AND COALESCE(a.clearance_status, '') NOT LIKE '%Rejected%'
		AND COALESCE(a.clearance_status, '') NOT LIKE '%Cancelled%'`

	// dashboardEngagementAllocation excludes default (team) allocations.
	dashboardEngagementAllocation = `a.engagement_id NOT IN ('-1', '-1.0')`

	// dashboardInvoiceFailed marks a finance-approved invoice whose NetSuite
	// push failed: the finance area keeps the legacy status 'Accepted by
	// finance' and writes domain.InvoiceNSFailurePrefix into reject_reason
	// (legacy writes reject_reason only with a Rejected status, so the marker
	// is unambiguous). Must match the finance area's NS Sync Failures list.
	dashboardInvoiceFailed = `(COALESCE(i.reject_reason, '') LIKE 'NetSuite push failed:%')`

	// dashboardPaymentFailed marks a per diem payment whose push failed: the
	// finance area sets ns_status 'Failed' (domain.PaymentNSFailed).
	dashboardPaymentFailed = `pp.ns_status = 'Failed'`
)

// countRows scans (label, count) rows.
func dashboardCountRows(rows *sql.Rows, what string) ([]domain.DashboardCount, error) {
	defer rows.Close()
	out := []domain.DashboardCount{}
	for rows.Next() {
		var label sql.NullString
		var c domain.DashboardCount
		if err := rows.Scan(&label, &c.Count); err != nil {
			return nil, fmt.Errorf("%s scan: %w", what, err)
		}
		c.Label = label.String
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", what, err)
	}
	return out, nil
}

// dashboardCostRows scans (month, costType, currency, amount) rows.
func dashboardCostRows(rows *sql.Rows, source, what string) ([]domain.DashboardCostLine, error) {
	defer rows.Close()
	out := []domain.DashboardCostLine{}
	for rows.Next() {
		var month, costType, currency sql.NullString
		var amount sql.NullFloat64
		if err := rows.Scan(&month, &costType, &currency, &amount); err != nil {
			return nil, fmt.Errorf("%s scan: %w", what, err)
		}
		out = append(out, domain.DashboardCostLine{
			Source: source, Month: month.String, CostType: costType.String,
			Currency: currency.String, Amount: amount.Float64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", what, err)
	}
	return out, nil
}
