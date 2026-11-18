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
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/email"
)

// FinanceService is the finance screen area: the per diem, invoice, travel
// card and payment tabs of a travel request, Invoices & Payments, NS Sync
// Failures and the finance reports. It enforces the legacy record-level
// rules (any vs own, agency scope, trip status) on top of the route gates.
type FinanceService struct {
	entity     engagemententity.FinanceAPI
	hr         hrentity.Client
	people     *travelPeople
	mail       email.Sender
	recipients email.FinanceRecipients
	currency   FinanceCurrencyConverter
	now        func() time.Time
	// batchReserve is the time kept back at the end of a request: a bulk
	// push starts no new record once less than this is left.
	batchReserve time.Duration
	// audit writes the legacy email_auditlog row of a sent email; nil skips it.
	audit func(ctx context.Context, m email.Message)
	// Campaigns resolves campaign names; nil leaves them unset.
	Campaigns *CampaignNames
}

// WithEmailAudit makes every sent finance email also write its legacy
// email_auditlog row (the Travel app did for every email it generated).
func (s *FinanceService) WithEmailAudit(audit func(ctx context.Context, m email.Message)) *FinanceService {
	s.audit = audit
	return s
}

// NewFinanceService constructs a FinanceService. currency nil means the
// currency conversion service is not configured: anything that needs a
// conversion answers 503.
func NewFinanceService(entity engagemententity.FinanceAPI, hr hrentity.Client, mail email.Sender,
	recipients email.FinanceRecipients, currency FinanceCurrencyConverter, now func() time.Time) *FinanceService {
	if now == nil {
		now = time.Now
	}
	return &FinanceService{entity: entity, hr: hr, people: newTravelPeople(hr), mail: mail, recipients: recipients, currency: currency,
		now: now, batchReserve: 12 * time.Second}
}

// Legacy statuses.
const (
	financeStatusPending           = "Pending"
	financeStatusAcceptedByTravel  = "Accepted by travel"
	financeStatusAcceptedByFinance = "Accepted by finance"
	financeStatusComplete          = "Complete"
	financeStatusRejectedByTravel  = "Rejected by travel"
	financeStatusRejectedByFinance = "Rejected by finance"

	financeNSPending  = "Pending"
	financeNSComplete = "Complete"
	financeNSFailed   = "Failed"
	financeNSPushing  = "Pushing"
)

var financeAllStatuses = []string{financeStatusPending, financeStatusAcceptedByTravel, financeStatusAcceptedByFinance,
	financeStatusComplete, financeStatusRejectedByTravel, financeStatusRejectedByFinance}

var financeJobRe = regexp.MustCompile(`^TRV-\d{4}-\d{2}-\d{4}$`)

func financeForbidden() error {
	return &apierror.ForbiddenError{Msg: "You do not have permission to perform this action."}
}

func financeBad(format string, a ...any) error {
	return &apierror.ValidationError{Msg: fmt.Sprintf(format, a...)}
}

func financeStr(s string) *string { return &s }

func financeDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// financeDate is the date part of "YYYY-MM-DD HH:MM:SS".
func financeDate(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	d, _, _ := strings.Cut(*p, " ")
	return &d
}

func financeRound2(f float64) float64 { return math.Round(f*100) / 100 }

// FinancePaymentCentsMsg refuses a payment amount finer than a cent.
const FinancePaymentCentsMsg = "Payment Amount can have at most two decimal places."

// financeCents reports whether f has at most two decimal places (within
// float noise, so 100.1 passes and 100.004 does not). A positive amount
// must also be at least one cent before rounding, so neither 0.000000001
// nor 0.0099999999 passes: the entity is sent the amount as given.
func financeCents(f float64) bool {
	c := math.Round(f * 100)
	return math.Abs(f*100-c) < 1e-6 && (f <= 0 || f >= 0.01)
}

// financeOpen is the legacy UI's OPEN: In Progress or Extended.
func financeOpen(status *string) bool {
	s := financeDeref(status)
	return strings.EqualFold(s, "In Progress") || strings.EqualFold(s, "Extended")
}

func financeCampaign(c *engagemententity.FinanceCampaign) *domain.CampaignRef {
	if c == nil {
		return nil
	}
	return &domain.CampaignRef{ID: c.ID} // name: Campaigns.Fill (the entity's is the event name)
}

func financePerson(email *string) *domain.Person {
	if email == nil || *email == "" {
		return nil
	}
	return &domain.Person{Email: *email}
}

// isFinanceNotFound reports an entity 404.
func isFinanceNotFound(err error) bool {
	var up *apierror.UpstreamError
	return errors.As(err, &up) && up.StatusCode == 404
}

// trip reads a travel request; a bad job number is 400, none is 404.
func (s *FinanceService) trip(ctx context.Context, jobNumber string) (engagemententity.FinanceTrip, error) {
	if !financeJobRe.MatchString(jobNumber) {
		return engagemententity.FinanceTrip{}, financeBad("Invalid job number format")
	}
	t, err := s.entity.FinanceTrip(ctx, jobNumber)
	if isFinanceNotFound(err) {
		return t, &apierror.NotFoundError{Msg: "Travel request not found."}
	}
	return t, err
}

// financeScope is how a caller sees a trip's finance tab.
type financeScope string

const (
	financeScopeFull   financeScope = "full"
	financeScopeAgency financeScope = "agency"
	financeScopeOwn    financeScope = "own"
)

// viewScope ports the legacy two-layer check of a trip tab: the route gate
// passed one of anyPrivs or ownPriv; holders of anyPrivs see any trip, the
// others only their own (the legacy getTravels({jobNumber, email}) check:
// the caller is the traveller). Release 1 scopes agents to their own
// agency's trips, and agentsAllowed false hides the tab from agents
// (payments and travel cards). The agent page (18) gives no full view on
// its own: a holder of 18 who is not an agent (no agency row) and does not
// see every request (Travel 1 or 2) is scoped as travelScope does.
func viewScope(p domain.Perms, trip engagemententity.FinanceTrip, agentsAllowed bool, anyPrivs []int, ownPriv int) (financeScope, error) {
	if p.IsAgent() {
		if !agentsAllowed || !p.Travel.Any(anyPrivs...) || trip.AgencyID == nil || *trip.AgencyID != p.Agency.ID {
			return "", financeForbidden()
		}
		return financeScopeAgency, nil
	}
	if financeFullPrivs(p, anyPrivs) {
		return financeScopeFull, nil
	}
	if p.Travel.Has(ownPriv) && strings.EqualFold(trip.TravellerEmail, p.Email) {
		return financeScopeOwn, nil
	}
	return "", financeForbidden()
}

// financeFullPrivs reports whether a non-agent holds one of anyPrivs, the
// agent page (18) counting only for those who see every request.
func financeFullPrivs(p domain.Perms, anyPrivs []int) bool {
	for _, id := range anyPrivs {
		if id == domain.TravelPrivAgentTravelRequests && !p.Travel.Any(travelPrivNewRequests, travelPrivAllRequests) {
			continue
		}
		if p.Travel.Has(id) {
			return true
		}
	}
	return false
}

// writeScope refuses an agent acting on another agency's trip (Release 1
// agency scoping; the legacy let agents act on any trip by number).
func writeScope(p domain.Perms, trip engagemententity.FinanceTrip) error {
	if p.IsAgent() && (trip.AgencyID == nil || *trip.AgencyID != p.Agency.ID) {
		return financeForbidden()
	}
	return nil
}

// travellerCompany is the traveller's HR company (legacy employeeCompany),
// "" when HR has none or cannot be reached. It is read through the cached
// people lookup, so a batch of emails for one traveller costs one HR call.
func (s *FinanceService) travellerCompany(ctx context.Context, traveller string) string {
	if traveller == "" {
		return ""
	}
	return strings.TrimSpace(financeDeref(s.people.get(ctx, traveller).Company))
}

// notify sends a travel finance email; a failure is logged, never returned
// (legacy: the action stands).
func (s *FinanceService) notify(ctx context.Context, kind, jobNumber, traveller, tab string) {
	m := email.FinanceTravelMessage(s.recipients, kind, jobNumber, s.travellerCompany(ctx, traveller), tab)
	ctx = context.WithoutCancel(ctx)
	if err := s.mail.Send(ctx, m); err != nil {
		slog.ErrorContext(ctx, "finance email not sent", "kind", kind, "job", jobNumber, "err", err)
		return
	}
	if s.audit != nil {
		s.audit(ctx, m)
	}
}

// ── Per diems ───────────────────────────────────────────────────────────

func perDiemOf(pd engagemententity.FinancePerDiem, canDelete bool) domain.PerDiem {
	out := domain.PerDiem{ID: pd.ID, JobNumber: pd.JobNumber, Currency: financeDeref(pd.Currency), Rate: pd.PerDiemValue,
		Total: pd.Total, CanDelete: canDelete}
	split := func(p *string) (string, string) {
		d, t, _ := strings.Cut(financeDeref(p), " ")
		if len(t) >= 5 {
			t = t[:5]
		}
		return d, t
	}
	out.StartDate, out.StartTime = split(pd.StartDate)
	out.EndDate, out.EndTime = split(pd.EndDate)
	if pd.NoOfDays != nil {
		out.Days = float64(*pd.NoOfDays)
	}
	return out
}

// PerDiems is GET /travel-requests/{jobNumber}/per-diems. "Any" holds for
// View Perdiems (28) or the agent page (18), as in the legacy handler.
func (s *FinanceService) PerDiems(ctx context.Context, p domain.Perms, jobNumber string) ([]domain.PerDiem, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return nil, err
	}
	if _, err := viewScope(p, trip, true, []int{domain.TravelPrivViewPerDiems, domain.TravelPrivAgentTravelRequests},
		domain.TravelPrivViewOwnPerDiems); err != nil {
		return nil, err
	}
	rows, err := s.entity.FinancePerDiems(ctx, jobNumber)
	if err != nil {
		return nil, err
	}
	canDelete := p.Travel.Has(domain.TravelPrivDeletePerDiems) && financeOpen(trip.Status) && writeScope(p, trip) == nil
	out := make([]domain.PerDiem, 0, len(rows))
	for _, r := range rows {
		out = append(out, perDiemOf(r, canDelete))
	}
	return out, nil
}

// AddPerDiem is POST /travel-requests/{jobNumber}/per-diems. Days and total
// follow the legacy client rule: days = date difference + 1; total =
// (first + last + days - 2) × rate, first = 1 when the start time is at or
// before 12:00 else 0.5, last = 1 when the end time is at or after 12:00
// else 0.5. Every per diem of a trip has the same currency.
func (s *FinanceService) AddPerDiem(ctx context.Context, p domain.Perms, jobNumber string, in domain.PerDiemCreate) (domain.PerDiem, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.PerDiem{}, err
	}
	if err := writeScope(p, trip); err != nil {
		return domain.PerDiem{}, err
	}
	if !financeOpen(trip.Status) {
		return domain.PerDiem{}, financeBad("Per diems can be added only while the travel request is In Progress or Extended.")
	}
	var missing []string
	for name, v := range map[string]string{"Start Date": in.StartDate, "Start Time": in.StartTime, "End Date": in.EndDate,
		"End Time": in.EndTime, "Currency": in.Currency} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, name)
		}
	}
	if in.Rate <= 0 {
		missing = append(missing, "Per Diem Amount")
	}
	if len(missing) > 0 {
		verb := "is"
		if len(missing) > 1 {
			verb = "are"
		}
		return domain.PerDiem{}, financeBad("Please re-check the information. %s %s mandatory.", strings.Join(financeSorted(missing), ", "), verb)
	}
	start, err1 := time.Parse("2006-01-02 15:04", in.StartDate+" "+in.StartTime)
	end, err2 := time.Parse("2006-01-02 15:04", in.EndDate+" "+in.EndTime)
	if err1 != nil || err2 != nil {
		return domain.PerDiem{}, financeBad("Dates must be YYYY-MM-DD and times HH:MM.")
	}
	if !end.After(start) {
		return domain.PerDiem{}, financeBad("Please check the dates again")
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if len(cur) != 3 {
		return domain.PerDiem{}, financeBad("Currency must be a three-letter currency code.")
	}
	existing, err := s.entity.FinancePerDiems(ctx, jobNumber)
	if err != nil {
		return domain.PerDiem{}, err
	}
	for _, e := range existing {
		if c := financeDeref(e.Currency); c != "" && !strings.EqualFold(c, cur) {
			return domain.PerDiem{}, financeBad("Currency mismatch! All per diems in a travel request must use the same currency.")
		}
	}
	sd, _ := time.Parse("2006-01-02", in.StartDate)
	ed, _ := time.Parse("2006-01-02", in.EndDate)
	days := int(ed.Sub(sd).Hours()/24) + 1
	first, last := 1.0, 1.0
	// Compare the parsed clock times: a non-canonical "9:00" parses but sorts
	// after "12:00" as a string.
	noon := 12 * 60
	if start.Hour()*60+start.Minute() > noon {
		first = 0.5
	}
	if end.Hour()*60+end.Minute() < noon {
		last = 0.5
	}
	total := financeRound2((first + last + float64(days) - 2) * in.Rate)
	created, err := s.entity.FinanceAddPerDiem(ctx, jobNumber, engagemententity.FinancePerDiemCreate{
		Currency: cur, PerDiemValue: in.Rate, Total: total,
		StartDate: start.Format("2006-01-02 15:04:05"), EndDate: end.Format("2006-01-02 15:04:05"),
		NoOfDays: days, CreatedBy: p.Email,
	})
	if err != nil {
		return domain.PerDiem{}, err
	}
	s.notify(ctx, email.FinancePerDiemsAdded, jobNumber, trip.TravellerEmail, "per-diems")
	out := perDiemOf(created, p.Travel.Has(domain.TravelPrivDeletePerDiems))
	out.AddedBy = financeStr(p.Email)
	return out, nil
}

func financeSorted(v []string) []string {
	order := map[string]int{"Start Date": 0, "Start Time": 1, "End Date": 2, "End Time": 3, "Currency": 4, "Per Diem Amount": 5}
	out := append([]string{}, v...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && order[out[j]] < order[out[j-1]]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// FinanceDeleteProcessedMsg is the legacy refusal of a processed per diem.
const FinanceDeleteProcessedMsg = "Deletion not allowed. This per diem has already been processed."

// DeletePerDiem is DELETE /travel-requests/{jobNumber}/per-diems/{id}: once
// payments exist, a per diem larger than the remaining balance (legacy:
// first currency's total minus Complete Currency invoices, travel cards and
// payments, each converted to the per diem's currency at its added date)
// cannot be deleted.
func (s *FinanceService) DeletePerDiem(ctx context.Context, p domain.Perms, jobNumber string, id int) error {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return err
	}
	if err := writeScope(p, trip); err != nil {
		return err
	}
	if !financeOpen(trip.Status) {
		return financeBad("Per diems can be deleted only while the travel request is In Progress or Extended.")
	}
	pd, err := s.entity.FinancePerDiem(ctx, jobNumber, id)
	if isFinanceNotFound(err) {
		return &apierror.NotFoundError{Msg: "Per diem not found."}
	}
	if err != nil {
		return err
	}
	basis, err := s.entity.FinancePaymentBasis(ctx, jobNumber)
	if err != nil {
		return err
	}
	if basis.PaymentCount > 0 {
		if len(basis.PerDiemTotals) == 0 || financeDeref(pd.Currency) == "" {
			return financeBad("Error while validating the deletion")
		}
		fx := newFinanceFX(s.currency)
		cur := *pd.Currency
		var used float64
		for _, group := range [][]engagemententity.FinanceAmount{basis.CurrencyInvoices, basis.TravelCards, basis.Payments} {
			for _, a := range group {
				v, err := fx.convert(ctx, a.Currency, cur, a.Amount, a.AddedDate)
				if err != nil {
					return err
				}
				used += v
			}
		}
		if financeRound2(pd.Total) > financeRound2(basis.PerDiemTotals[0].TotalAmount-used) {
			return financeBad(FinanceDeleteProcessedMsg)
		}
	}
	err = s.entity.FinanceDeletePerDiem(ctx, jobNumber, id)
	if isFinanceNotFound(err) {
		return &apierror.NotFoundError{Msg: "Per diem not found."}
	}
	return err
}

// ── Travel cards ────────────────────────────────────────────────────────

func travelCardOf(c engagemententity.FinanceTravelCard) domain.TravelCard {
	return domain.TravelCard{ID: c.ID, JobNumber: c.JobNumber, Currency: c.Currency, Amount: c.Amount, AddedBy: c.AddedBy,
		AddedOn: financeDeref(financeDate(&c.AddedDate))}
}

// TravelCards is GET /travel-requests/{jobNumber}/travel-cards.
func (s *FinanceService) TravelCards(ctx context.Context, p domain.Perms, jobNumber string) ([]domain.TravelCard, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return nil, err
	}
	if _, err := viewScope(p, trip, false, []int{domain.TravelPrivViewTravelCards}, domain.TravelPrivViewOwnTravelCards); err != nil {
		return nil, err
	}
	rows, err := s.entity.FinanceTravelCards(ctx, jobNumber)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TravelCard, 0, len(rows))
	for _, c := range rows {
		out = append(out, travelCardOf(c))
	}
	return out, nil
}

// AddTravelCard is POST /travel-requests/{jobNumber}/travel-cards (USD or
// GBP, the trip open, no payment yet; the entity refuses once payments
// exist).
func (s *FinanceService) AddTravelCard(ctx context.Context, p domain.Perms, jobNumber string, in domain.TravelCardCreate) (domain.TravelCard, error) {
	trip, err := s.trip(ctx, jobNumber)
	if err != nil {
		return domain.TravelCard{}, err
	}
	if err := writeScope(p, trip); err != nil {
		return domain.TravelCard{}, err
	}
	if !financeOpen(trip.Status) {
		return domain.TravelCard{}, financeBad("Travel cards can be added only while the travel request is In Progress or Extended.")
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if cur != "USD" && cur != "GBP" {
		return domain.TravelCard{}, financeBad("Travel Card Currency must be USD or GBP.")
	}
	if in.Amount <= 0 {
		return domain.TravelCard{}, financeBad("Travel Card Amount must be more than 0.")
	}
	c, err := s.entity.FinanceAddTravelCard(ctx, jobNumber, engagemententity.FinanceTravelCardCreate{Currency: cur, Amount: in.Amount, CreatedBy: p.Email})
	if err != nil {
		return domain.TravelCard{}, err
	}
	return travelCardOf(c), nil
}
