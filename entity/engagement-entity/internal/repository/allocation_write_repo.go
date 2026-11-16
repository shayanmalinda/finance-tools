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
	"strings"

	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/audit"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/domain"
	"github.com/wso2-open-operations/finance-tools/entity/engagement-entity/internal/textnorm"
)

// Legacy travel values the allocation area writes (entity
// modules/database/constants.bal).
const (
	allocationTravelRequiredYes     = "Yes"
	allocationTravelStatusNew       = "New"
	allocationTravelStatusCancelled = "Cancelled"
	allocationTravelPreApproved     = "Pre-Approved"
	allocationTravelUnmodified      = "unmodified"
	allocationTravelApproved        = "Approved"
	allocationRequestInProgress     = "In Progress"
)

// AllocationDuplicateMsg is the refusal of an identical active allocation
// (LEGACY-ISSUES L5: a double click created two).
const AllocationDuplicateMsg = "This person already has an active allocation on this engagement for the same dates."

// allocationIsPartner reports whether email is a partner pseudo-email.
func allocationIsPartner(email string) bool {
	return strings.Contains(email, domain.AllocationPartnerEmailPrefix)
}

func allocationCountry(s *string) *string {
	if s == nil {
		return nil
	}
	v := textnorm.Country(*s)
	return &v
}

func allocationNorm(s *string) *string {
	if s == nil {
		return nil
	}
	v := textnorm.Latin1(*s)
	return &v
}

// allocationTravelInput is a travel row to insert (legacy InsertTravelInput).
type allocationTravelInput struct {
	travellerEmail string
	startDate      string
	endDate        string
	engagementType *string
	engagementCode *string
	country        *string
	city           *string
	customerName   *string
	addedBy        string
	lastUpdatedBy  string
	productID      *string
	engagementID   string
	allocationID   string
}

// insertTravel ports the legacy insertTravel in its own travelapp
// transaction and returns the job number the travelapp trigger gave the row
// (TRV-YYYY-MM-NNNN). The legacy wrote the travel row before committing the
// allocation_db transaction, so a travel failure rolls the allocation back;
// callers keep that order.
func (r *allocationRepo) insertTravel(ctx context.Context, in allocationTravelInput) (string, error) {
	var job string
	err := WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO travel (
				traveller_email, engagement_type, country, city, engagement_code, customer_name,
				start_date, end_date, status, approver, added_by, last_updated_by, last_updated_date,
				allocation_status, approval_status, opportunity_line_item_id, engagement_id, allocation_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURDATE(), ?, ?, ?, ?, ?)`,
			in.travellerEmail, allocationNorm(in.engagementType), allocationCountry(in.country), allocationNorm(in.city),
			allocationNorm(in.engagementCode), allocationNorm(in.customerName), in.startDate, in.endDate,
			allocationTravelStatusNew, allocationTravelPreApproved, in.addedBy, in.lastUpdatedBy,
			allocationTravelUnmodified, allocationTravelApproved, in.productID, in.engagementID, in.allocationID,
		); err != nil {
			return fmt.Errorf("allocation.insertTravel: %w", err)
		}
		err := tx.QueryRowContext(ctx, `SELECT job_number FROM travel
			WHERE allocation_id = ? AND traveller_email = ?
			ORDER BY job_number DESC LIMIT 1`, in.allocationID, in.travellerEmail).Scan(&job)
		if err != nil {
			return fmt.Errorf("allocation.insertTravel job number: %w", err)
		}
		return audit.Inserted("travel", "job_number", job).Record(ctx, tx, travelAuditEvent(audit.Create, in.lastUpdatedBy))
	})
	return job, err
}

func travelAuditEvent(action audit.Action, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: "travel_request", ParentType: "allocation", ParentColumn: "allocation_id", Actor: actor}
}

func allocationAuditEvent(action audit.Action, actor string) audit.Event {
	return audit.Event{Action: action, EntityType: "allocation", ParentType: "engagement", ParentColumn: "engagement_id", Actor: actor}
}

// cancelTravel ports the legacy cancelTravel (status only, matched on the
// traveller and allocation) and reports whether a row changed.
func (r *allocationRepo) cancelTravel(ctx context.Context, travellerEmail, allocationID, actor string) (bool, error) {
	var n int64
	err := WithTx(ctx, r.travel, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "travel", "job_number", `traveller_email = ? AND allocation_id = ?`, travellerEmail, allocationID)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE travel SET status = ?
			WHERE traveller_email = ? AND allocation_id = ?`,
			allocationTravelStatusCancelled, travellerEmail, allocationID)
		if err != nil {
			return fmt.Errorf("allocation.cancelTravel: %w", err)
		}
		n, _ = res.RowsAffected()
		return t.Record(ctx, tx, travelAuditEvent(audit.StatusChange, actor))
	})
	return n > 0, err
}

// Bulk ports bulkUploadAllocations in one allocation_db transaction:
//
//  1. Refuse an identical active engagement allocation (same consultant,
//     engagement, dates and times; LEGACY-ISSUES L5).
//  2. Insert newAllocations in order; read back the id the trigger gave
//     newAllocations[0].
//  3. For the first new engagement allocation with travel required,
//     clearance Confirmed or Confirmed - Visa Pending and not a partner,
//     insert its travel row (added by the acting user).
//  4. Apply updateAllocations (start, end, updated_by).
//  5. Insert allocationLogs into allocation_upload_log.
//  6. Set the engagement's request status to In Progress when asked.
func (r *allocationRepo) Bulk(ctx context.Context, req domain.AllocationBulkRequest) (domain.AllocationBulkResponse, error) {
	var resp domain.AllocationBulkResponse
	err := WithTx(ctx, r.alloc, func(tx *sql.Tx) error {
		seen := map[string]bool{}
		for _, a := range req.NewAllocations {
			if domain.AllocationIsDefault(a.EngagementID) {
				continue
			}
			// The same candidate twice in one payload is a duplicate as well.
			k := allocationDuplicateKey(a)
			if seen[k] {
				return &apierror.ConflictError{Msg: AllocationDuplicateMsg}
			}
			seen[k] = true
			var n int
			// Same times too: hourly allocations whose windows differ are allowed.
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM allocation
				WHERE email = ? AND engagement_id = ? AND start_date = ? AND end_date = ?
				AND start_time <=> ? AND end_time <=> ? AND status = 'Active'
				FOR UPDATE`, a.Email, a.EngagementID, a.StartDate, a.EndDate, a.StartTime, a.EndTime).Scan(&n); err != nil {
				return fmt.Errorf("allocation.Bulk duplicate check: %w", err)
			}
			if n > 0 {
				return &apierror.ConflictError{Msg: AllocationDuplicateMsg}
			}
		}

		ids := map[int]string{}
		for i, a := range req.NewAllocations {
			if _, err := tx.ExecContext(ctx, `INSERT INTO allocation (
					email, allocation_type, status, allocation_status, clearance_status, start_date, end_date,
					start_time, end_time, time_zone, added_by, added_date, engagement_id, engagement_code,
					isTravelRequired, isRecurring, comment
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), ?, ?, ?, ?, ?)`,
				a.Email, a.AllocationTypeID, a.Status, a.AllocationStatus, a.ClearanceStatus, a.StartDate, a.EndDate,
				a.StartTime, a.EndTime, a.TimeZone, req.CreatedBy, a.EngagementID, textnorm.Latin1(a.EngagementCode),
				a.IsTravelRequired, a.IsRecurring, allocationNorm(a.Comment),
			); err != nil {
				return fmt.Errorf("allocation.Bulk insert: %w", err)
			}
			if i == 0 || audit.Enabled() || allocationNeedsTravel(a) {
				id, err := allocationNewID(ctx, tx, a)
				if err != nil {
					return err
				}
				ids[i] = id
				if i == 0 {
					resp.FirstAllocationID = &id
				}
				if err := audit.Inserted("allocation", "id", id).Record(ctx, tx, allocationAuditEvent(audit.Create, req.CreatedBy)); err != nil {
					return err
				}
			}
		}
		resp.RecordsAdded = len(req.NewAllocations)

		if err := r.bulkTravel(ctx, tx, req, ids, &resp); err != nil {
			return err
		}

		for _, u := range req.UpdateAllocations {
			t, err := audit.Track(ctx, tx, "allocation", "id", `Id = ?`, u.ID)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE allocation SET start_date = ?, end_date = ?, updated_by = ?
				WHERE Id = ?`, u.StartDate, u.EndDate, req.CreatedBy, u.ID); err != nil {
				return fmt.Errorf("allocation.Bulk update: %w", err)
			}
			if err := t.Record(ctx, tx, allocationAuditEvent(audit.Update, req.CreatedBy)); err != nil {
				return err
			}
		}
		resp.RecordsUpdated = len(req.UpdateAllocations)

		for _, l := range req.AllocationLogs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO allocation_upload_log (
					email, start_date, end_date, allocation_type, engagement_code, added_by, added_date
				) VALUES (?, ?, ?, ?, ?, ?, NOW())`,
				l.Email, l.StartDate, l.EndDate, l.AllocationTypeID, textnorm.Latin1(l.EngagementCode), req.CreatedBy,
			); err != nil {
				return fmt.Errorf("allocation.Bulk upload log: %w", err)
			}
		}

		if req.RequestStatusInProgress != nil {
			t, err := audit.Track(ctx, tx, "engagement", "id", `Id = ?`, *req.RequestStatusInProgress)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE engagement SET updated_by = ?, status = ? WHERE Id = ?`,
				req.CreatedBy, allocationRequestInProgress, *req.RequestStatusInProgress); err != nil {
				return fmt.Errorf("allocation.Bulk request status: %w", err)
			}
			return t.Record(ctx, tx, audit.Event{Action: audit.Update, EntityType: "engagement", Actor: req.CreatedBy})
		}
		return nil
	})
	if err != nil {
		return domain.AllocationBulkResponse{}, err
	}
	return resp, nil
}

// allocationDuplicateKey identifies a Bulk candidate by the fields the
// duplicate check compares.
func allocationDuplicateKey(a domain.AllocationInput) string {
	opt := func(s *string) string {
		if s == nil {
			return "\x00"
		}
		return *s
	}
	return strings.Join([]string{strings.ToLower(a.Email), a.EngagementID, a.StartDate, a.EndDate, opt(a.StartTime), opt(a.EndTime)}, "|")
}

// allocationNeedsTravel reports whether a new allocation gets a travel row:
// engagement allocation, travel required, not a partner, clearance Confirmed
// or Confirmed - Visa Pending.
func allocationNeedsTravel(a domain.AllocationInput) bool {
	return !domain.AllocationIsDefault(a.EngagementID) && a.IsTravelRequired == allocationTravelRequiredYes &&
		a.ClearanceStatus != nil && !allocationIsPartner(a.Email) &&
		(*a.ClearanceStatus == domain.AllocationClearanceConfirmed ||
			*a.ClearanceStatus == domain.AllocationClearanceConfirmedVisaPending)
}

// allocationNewID reads back the id the allocation trigger gave a row just
// inserted in tx (the id is not an auto-increment column; legacy matched the
// row on its fields the same way).
func allocationNewID(ctx context.Context, tx *sql.Tx, a domain.AllocationInput) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id FROM allocation
		WHERE email = ? AND engagement_id = ? AND start_date = ? AND end_date = ? AND status = ?
		ORDER BY id DESC LIMIT 1`, a.Email, a.EngagementID, a.StartDate, a.EndDate, a.Status).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("allocation.Bulk new id: %w", err)
	}
	return id, nil
}

// bulkTravel is step 3 of Bulk (travel-requests-port-spec §3.4 path A).
// ids holds the id read back for each inserted allocation (by index), so the
// travel row is bound to the first qualifying allocation, not to a re-query.
func (r *allocationRepo) bulkTravel(ctx context.Context, tx *sql.Tx, req domain.AllocationBulkRequest, ids map[int]string, resp *domain.AllocationBulkResponse) error {
	for i, a := range req.NewAllocations {
		if !allocationNeedsTravel(a) {
			continue
		}
		var (
			typeName     *string
			country      *string
			address      *string
			customerName *string
			productID    *string
		)
		id := ids[i]
		err := tx.QueryRowContext(ctx, `SELECT aT.allocation_type, e.country, REPLACE(e.location, 'BR()', CHAR(10)), e.customer_name,
				e.opportunity_line_item_id
			FROM allocation AS a
				JOIN engagement AS e ON a.engagement_id = e.id
				JOIN allocation_type AS aT ON e.allocation_type = aT.id
			WHERE a.id = ?`,
			id,
		).Scan(&typeName, &country, &address, &customerName, &productID)
		if err != nil {
			return fmt.Errorf("allocation.Bulk travel allocation: %w", err)
		}
		code := a.EngagementCode
		job, err := r.insertTravel(ctx, allocationTravelInput{
			travellerEmail: a.Email, startDate: a.StartDate, endDate: a.EndDate,
			engagementType: typeName, engagementCode: &code, country: country, city: address,
			customerName: customerName, addedBy: req.CreatedBy, lastUpdatedBy: req.CreatedBy,
			productID: productID, engagementID: a.EngagementID, allocationID: id,
		})
		if err != nil {
			return err
		}
		resp.TravelJobNumber = &job
		return nil // legacy: only the first qualifying allocation
	}
	return nil
}

// Update ports updateAllocation for the allocation pre read: a dynamic SET
// (updated_by always), then, for an engagement allocation of an employee
// with travel required and a new clearance status (travel-requests-port-spec
// §3.4 path B):
//
//   - new status Confirmed - Visa Pending, or Confirmed when the previous
//     status was not Confirmed - Visa Pending: insert a travel row (added by
//     the allocation's added_by);
//   - otherwise, a valid status other than Confirmed when the previous one
//     was Confirmed or Confirmed - Visa Pending: cancel its travel rows.
//
// The previous status is the one read before the update.
func (r *allocationRepo) Update(ctx context.Context, pre domain.AllocationRecord, req domain.AllocationUpdateRequest) (domain.AllocationUpdateResponse, error) {
	resp := domain.AllocationUpdateResponse{PreviousClearanceStatus: pre.ClearanceStatus}
	err := WithTx(ctx, r.alloc, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "allocation", "id", `Id = ?`, pre.ID)
		if err != nil {
			return err
		}
		q := `UPDATE allocation SET updated_by = ?`
		args := []any{req.UpdatedBy}
		if req.ClearanceStatus != nil {
			q += `, clearance_status = ?`
			args = append(args, *req.ClearanceStatus)
		}
		if req.Status != nil {
			q += `, status = ?`
			args = append(args, *req.Status)
		}
		if req.Comment != nil {
			q += `, comment = ?`
			args = append(args, textnorm.Latin1(*req.Comment))
		}
		if req.StartDate != nil {
			q += `, start_date = ?`
			args = append(args, *req.StartDate)
		}
		if req.EndDate != nil {
			q += `, end_date = ?`
			args = append(args, *req.EndDate)
		}
		q += ` WHERE Id = ?`
		args = append(args, pre.ID)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("allocation.Update: %w", err)
		}
		action := audit.Update
		if req.Status != nil || req.ClearanceStatus != nil {
			action = audit.StatusChange
		}
		if err := t.Record(ctx, tx, allocationAuditEvent(action, req.UpdatedBy)); err != nil {
			return err
		}
		return r.updateTravel(ctx, pre, req, &resp)
	})
	if err != nil {
		return domain.AllocationUpdateResponse{}, err
	}
	post, err := r.Get(ctx, pre.ID)
	if err != nil {
		return domain.AllocationUpdateResponse{}, err
	}
	if post == nil {
		return domain.AllocationUpdateResponse{}, errors.New("allocation.Update: updated allocation not found")
	}
	resp.Allocation = *post
	return resp, nil
}

func (r *allocationRepo) updateTravel(ctx context.Context, pre domain.AllocationRecord, req domain.AllocationUpdateRequest, resp *domain.AllocationUpdateResponse) error {
	if pre.Engagement == nil || req.ClearanceStatus == nil || allocationIsPartner(pre.Email) ||
		pre.IsTravelRequired == nil || *pre.IsTravelRequired != allocationTravelRequiredYes {
		return nil
	}
	next := *req.ClearanceStatus
	prev := ""
	if pre.ClearanceStatus != nil {
		prev = *pre.ClearanceStatus
	}
	if next == prev {
		return nil // an unchanged status neither creates nor cancels travel
	}
	cvp := domain.AllocationClearanceConfirmedVisaPending
	confirmed := domain.AllocationClearanceConfirmed
	switch {
	case next == cvp || (prev != cvp && next == confirmed):
		e := pre.Engagement
		addedBy := ""
		if pre.AddedBy != nil {
			addedBy = *pre.AddedBy
		}
		// The travel covers the dates this update leaves on the allocation.
		start, end := pre.StartDate, pre.EndDate
		if req.StartDate != nil {
			start = *req.StartDate
		}
		if req.EndDate != nil {
			end = *req.EndDate
		}
		job, err := r.insertTravel(ctx, allocationTravelInput{
			travellerEmail: pre.Email, startDate: start, endDate: end,
			engagementType: e.AllocationTypeName, engagementCode: e.EngagementCode, country: e.Country,
			city: e.Address, customerName: e.CustomerName, addedBy: addedBy, lastUpdatedBy: addedBy,
			productID: e.ProductID, engagementID: e.ID, allocationID: pre.ID,
		})
		if err != nil {
			return err
		}
		resp.TravelJobNumber = &job
	case next != confirmed && (prev == cvp || prev == confirmed) && allocationValidClearance(next):
		cancelled, err := r.cancelTravel(ctx, pre.Email, pre.ID, req.UpdatedBy)
		if err != nil {
			return err
		}
		resp.TravelCancelled = cancelled
	}
	return nil
}

func allocationValidClearance(s string) bool {
	for _, c := range domain.AllocationClearanceStatuses {
		if c == s {
			return true
		}
	}
	return false
}

// Delete ports deleteAllocation (status Deleted only: no travel cancel).
func (r *allocationRepo) Delete(ctx context.Context, id string) error {
	return WithTx(ctx, r.alloc, func(tx *sql.Tx) error {
		t, err := audit.Track(ctx, tx, "allocation", "id", `id = ?`, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE allocation SET status = 'Deleted' WHERE id = ?`, id); err != nil {
			return fmt.Errorf("allocation.Delete: %w", err)
		}
		return t.Record(ctx, tx, allocationAuditEvent(audit.Delete, ""))
	})
}
