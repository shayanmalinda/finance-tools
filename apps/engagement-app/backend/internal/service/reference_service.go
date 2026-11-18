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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/cache"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/engagemententity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/hrentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/salesentity"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/domain"
)

// EngagementEntityAPI is what the engagements area needs from the
// Engagement Entity.
type EngagementEntityAPI interface {
	engagemententity.EngagementAPI
	engagemententity.AccessAPI
}

// referenceTTL bounds how stale the reference lists may be. The Allocation
// Types settings writes clear the cache of the replica that made them.
const referenceTTL = time.Minute

// referenceEmployeeStatuses are the HR statuses the person pickers and org
// pickers use (legacy employeeStatuses).
var referenceEmployeeStatuses = []string{"Active", "Marked leaver"}

// ReferenceService serves the lookups: reference data, Salesforce accounts,
// opportunities and campaigns, HR employees and the org structure.
type ReferenceService interface {
	ReferenceData(ctx context.Context) (domain.ReferenceData, error)
	Accounts(ctx context.Context, search string, limit int) ([]domain.ReferenceAccount, error)
	Opportunities(ctx context.Context, accountID string, engagementTypeID int) ([]domain.ReferenceOpportunity, error)
	Campaigns(ctx context.Context, search string, limit int) ([]domain.ReferenceCampaign, error)
	Employees(ctx context.Context, search string, limit int) ([]domain.ReferenceEmployee, error)
	OrgStructure(ctx context.Context) (domain.ReferenceOrgStructure, error)
	// Account reads one Salesforce account by id; nil when there is none.
	Account(ctx context.Context, id string) (*salesentity.Customer, error)
	// Campaign finds the active campaign with the id among those named like
	// name (the Sales Entity Service cannot search by id); nil when none.
	Campaign(ctx context.Context, id, name string) (*domain.ReferenceCampaign, error)
	// Invalidate drops the cached reference lists.
	Invalidate()
}

type referenceService struct {
	entity EngagementEntityAPI
	sales  salesentity.Client
	hr     hrentity.Client
	lists  *cache.Cache[string, domain.ReferenceData]
}

// NewReferenceService constructs a ReferenceService.
func NewReferenceService(entity EngagementEntityAPI, sales salesentity.Client, hr hrentity.Client) ReferenceService {
	return &referenceService{entity: entity, sales: sales, hr: hr, lists: cache.New[string, domain.ReferenceData](referenceTTL)}
}

func (s *referenceService) Invalidate() { s.lists.Delete("all") }

func (s *referenceService) ReferenceData(ctx context.Context) (domain.ReferenceData, error) {
	if v, ok := s.lists.Get("all"); ok {
		return v, nil
	}
	l, err := s.entity.ReferenceData(ctx)
	if err != nil {
		return domain.ReferenceData{}, err
	}
	out := domain.ReferenceData{
		EngagementTypes:         make([]domain.ReferenceEngagementType, 0, len(l.EngagementTypes)),
		AllocationTypes:         make([]domain.ReferenceAllocationType, 0, len(l.AllocationTypes)),
		BusinessUnits:           make([]domain.ReferenceNamedItem, 0, len(l.BusinessUnits)),
		ProductUnits:            make([]domain.ReferenceProductUnit, 0, len(l.ProductUnits)),
		Regions:                 make([]domain.ReferenceNamedItem, 0, len(l.Regions)),
		SubRegions:              make([]domain.ReferenceSubRegion, 0, len(l.SubRegions)),
		Countries:               domain.ReferenceCountries,
		EngagementNatures:       domain.ReferenceEngagementNatures,
		EventNatures:            domain.ReferenceEventNatures,
		RequestStatuses:         domain.ReferenceRequestStatuses,
		EngagementStatuses:      domain.ReferenceEngagementStatuses,
		ClearanceStatuses:       domain.ReferenceClearanceStatuses,
		ClearanceTransitions:    domain.ReferenceClearanceTransitions,
		TravelStatuses:          domain.ReferenceTravelStatuses,
		TravelStatusTransitions: domain.ReferenceTravelStatusTransitions,
		TravelAgencies:          make([]domain.ReferenceTravelAgency, 0, len(l.TravelAgencies)),
		CostTypes:               make([]string, 0, len(l.CostTypes)),
		PerDiemRates:            make([]domain.ReferencePerDiemRate, 0, len(l.PerDiemRates)),
		Currencies:              domain.ReferenceCurrencies,
		InvoiceCurrencies:       domain.ReferenceInvoiceCurrencies,
		TravelCardCurrencies:    domain.ReferenceTravelCardCurrencies,
		VisaTypes:               domain.ReferenceVisaTypes,
		EntryTypes:              domain.ReferenceEntryTypes,
		TravelDocumentTypes:     domain.ReferenceTravelDocumentTypes,
		TimeZones:               domain.ReferenceTimeZones,
	}
	for _, t := range l.EngagementTypes {
		out.EngagementTypes = append(out.EngagementTypes, domain.ReferenceEngagementType(t))
	}
	for _, t := range l.AllocationTypes {
		out.AllocationTypes = append(out.AllocationTypes, domain.ReferenceAllocationType(t))
	}
	for _, b := range l.BusinessUnits {
		out.BusinessUnits = append(out.BusinessUnits, domain.ReferenceNamedItem(b))
	}
	for _, r := range l.Regions {
		out.Regions = append(out.Regions, domain.ReferenceNamedItem(r))
	}
	for _, p := range l.ProductUnits {
		out.ProductUnits = append(out.ProductUnits, domain.ReferenceProductUnit(p))
	}
	for _, sr := range l.SubRegions {
		out.SubRegions = append(out.SubRegions, domain.ReferenceSubRegion(sr))
	}
	for _, a := range l.TravelAgencies {
		out.TravelAgencies = append(out.TravelAgencies, domain.ReferenceTravelAgency(a))
	}
	for _, c := range l.CostTypes {
		out.CostTypes = append(out.CostTypes, c.Name)
	}
	for _, r := range l.PerDiemRates {
		out.PerDiemRates = append(out.PerDiemRates, domain.ReferencePerDiemRate(r))
	}
	s.lists.Set("all", out)
	return out, nil
}

func referenceLimit(limit, def, maxLimit int) (int, error) {
	if limit == 0 {
		return def, nil
	}
	if limit < 1 || limit > maxLimit {
		return 0, &apierror.ValidationError{Msg: fmt.Sprintf("limit must be between 1 and %d.", maxLimit)}
	}
	return limit, nil
}

func referenceStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func referencePtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// referenceAddress joins the account's billing address lines.
func referenceAddress(a *salesentity.CustomerAddress) (address, country string) {
	if a == nil {
		return "", ""
	}
	country = referenceStr(a.BillingCountry)
	if country == "" {
		country = referenceStr(a.Country)
	}
	var lines []string
	for _, v := range []string{referenceStr(a.BillingStreet), referenceStr(a.BillingCity),
		strings.TrimSpace(referenceStr(a.BillingState) + " " + referenceStr(a.BillingPostalCode)), country} {
		if v != "" {
			lines = append(lines, v)
		}
	}
	return strings.Join(lines, "\n"), country
}

// Accounts is the Release 1 account search as you type (LEGACY-ISSUES F1):
// at least 2 characters, the synced Salesforce accounts whose name contains
// the text, at most limit (default 8, max 25).
func (s *referenceService) Accounts(ctx context.Context, search string, limit int) ([]domain.ReferenceAccount, error) {
	search = strings.TrimSpace(search)
	if len([]rune(search)) < 2 {
		return nil, &apierror.ValidationError{Msg: "Type at least 2 characters to search accounts."}
	}
	limit, err := referenceLimit(limit, 8, 25)
	if err != nil {
		return nil, err
	}
	customers, err := s.sales.SearchCustomers(ctx, salesentity.CustomerFilter{CustomerNameLike: search, IsRealTime: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := []domain.ReferenceAccount{}
	for _, c := range customers {
		if referenceStr(c.Name) == "" {
			continue
		}
		out = append(out, referenceAccountOf(c))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func referenceAccountOf(c salesentity.Customer) domain.ReferenceAccount {
	address, country := referenceAddress(c.Address)
	if bc := referenceStr(c.BillingCountry); bc != "" {
		country = bc
	}
	return domain.ReferenceAccount{
		ID: c.ID, Name: referenceStr(c.Name), Country: referencePtr(country),
		SalesRegion: c.SalesRegion, SubRegion: c.SubRegion, Address: referencePtr(address),
	}
}

// Account reads one account by id (create form validation).
func (s *referenceService) Account(ctx context.Context, id string) (*salesentity.Customer, error) {
	customers, err := s.sales.SearchCustomers(ctx, salesentity.CustomerFilter{IDs: []string{id}, IsRealTime: true, Limit: 1})
	if err != nil {
		return nil, err
	}
	for i := range customers {
		if customers[i].ID == id {
			return &customers[i], nil
		}
	}
	return nil, nil
}

func referenceLineItemOf(li salesentity.SalesLineItem) domain.ReferenceLineItem {
	return domain.ReferenceLineItem{
		ID: referenceStr(li.ID), Product: referenceStr(li.Name), EngagementCode: li.EngagementCodeAuto,
		StartDate: li.ServiceStartDate, EndDate: li.ServiceEndDate, Description: li.Description,
		BusinessUnit: li.BusinessUnit, ProductUnit: li.ProductUnit, IsTentativeDates: li.IsTentativeDates,
	}
}

// referenceNeedsLineItems reports whether the create form shows line items
// for the type: not for Non-Paid Pre Sale (3) nor the internal kinds.
func referenceNeedsLineItems(engagementTypeID int) bool {
	switch engagementTypeID {
	case 0, domain.EngagementTypePaid, domain.EngagementTypeNonPaidPost, domain.EngagementTypePaidPostSale, domain.EngagementTypePaidPreSale:
		return true
	}
	return false
}

// Opportunities ports GET customers/{id}/opportunities (active only, except
// for Non-Paid Post Sale, as the legacy form asked) with each
// opportunity's line items (legacy GET opportunities/{id}/line-items).
func (s *referenceService) Opportunities(ctx context.Context, accountID string, engagementTypeID int) ([]domain.ReferenceOpportunity, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, &apierror.ValidationError{Msg: "Select an account from Salesforce."}
	}
	opps, err := s.sales.Opportunities(ctx, accountID, engagementTypeID != domain.EngagementTypeNonPaidPost)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ReferenceOpportunity, len(opps))
	for i, o := range opps {
		out[i] = domain.ReferenceOpportunity{
			ID: o.ID, Name: o.Name, TechnicalOwner: o.TechnicalOwner, IsWon: o.IsWon, Address: o.CustomerAddress,
			PreSalesCode: o.PreSalesCode, PostSalesCode: o.PostSalesEngagementCode, LineItems: []domain.ReferenceLineItem{},
		}
		if o.Owner != nil {
			out[i].Owner = o.Owner.Name
		}
	}
	if !referenceNeedsLineItems(engagementTypeID) || len(out) == 0 {
		return out, nil
	}
	// Line items per opportunity, four at a time; the first failure cancels
	// the calls still running and skips those not started.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, 4)
	)
	for i := range out {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			items, err := s.sales.OpportunityLineItems(ctx, out[i].ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				return
			}
			for _, li := range items {
				out[i].LineItems = append(out[i].LineItems, referenceLineItemOf(li))
			}
		}(i)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	// A cancelled request can skip lookups without an error of their own:
	// their line items were never read, so don't answer with empty ones.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Campaigns searches active Salesforce campaigns by name (Release 1).
func (s *referenceService) Campaigns(ctx context.Context, search string, limit int) ([]domain.ReferenceCampaign, error) {
	limit, err := referenceLimit(limit, 20, 100)
	if err != nil {
		return nil, err
	}
	cs, err := s.sales.SearchCampaigns(ctx, salesentity.CampaignFilter{NameLike: strings.TrimSpace(search), Limit: limit})
	if err != nil {
		return nil, err
	}
	out := []domain.ReferenceCampaign{}
	for _, c := range cs {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		out = append(out, domain.ReferenceCampaign{ID: c.ID, Name: c.Name, Type: c.Type, Status: c.Status, StartDate: c.StartDate, EndDate: c.EndDate})
	}
	return out, nil
}

// Campaign is the active campaign with this id, or nil. It is looked up by
// id; name (optional) is searched too, for a Sales Entity without the ids
// filter.
func (s *referenceService) Campaign(ctx context.Context, id, name string) (*domain.ReferenceCampaign, error) {
	id, name = strings.TrimSpace(id), strings.TrimSpace(name)
	if id == "" {
		return nil, nil
	}
	active := true
	cs, err := s.sales.SearchCampaigns(ctx, salesentity.CampaignFilter{IDs: []string{id}, IsActive: &active, Limit: 1})
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c.ID == id {
			return &domain.ReferenceCampaign{ID: c.ID, Name: c.Name, Type: c.Type, Status: c.Status, StartDate: c.StartDate, EndDate: c.EndDate}, nil
		}
	}
	if name == "" {
		return nil, nil
	}
	found, err := s.Campaigns(ctx, name, 100)
	if err != nil {
		return nil, err
	}
	for i := range found {
		if found[i].ID == id {
			return &found[i], nil
		}
	}
	return nil, nil
}

// Employees searches Active and Marked leaver employees by work email.
func (s *referenceService) Employees(ctx context.Context, search string, limit int) ([]domain.ReferenceEmployee, error) {
	search = strings.TrimSpace(search)
	if len([]rune(search)) < 2 {
		return nil, &apierror.ValidationError{Msg: "Type at least 2 characters to search people."}
	}
	limit, err := referenceLimit(limit, 20, 100)
	if err != nil {
		return nil, err
	}
	es, err := s.hr.SearchEmployees(ctx, search, referenceEmployeeStatuses, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ReferenceEmployee, 0, len(es))
	for _, e := range es {
		out = append(out, domain.ReferenceEmployee{
			Email: e.WorkEmail, Name: strings.TrimSpace(e.FirstName + " " + e.LastName), EmployeeID: referencePtr(e.EmployeeID),
			JobRole: e.JobRole, BusinessUnit: e.BusinessUnit, Department: e.Department, Team: e.Team,
			Company: e.Company, Location: e.Location, ManagerEmail: e.ManagerEmail,
		})
	}
	return out, nil
}

// OrgStructure flattens HR orgData into business unit → department → team.
func (s *referenceService) OrgStructure(ctx context.Context) (domain.ReferenceOrgStructure, error) {
	nodes, err := s.hr.OrgData(ctx, referenceEmployeeStatuses)
	if err != nil {
		return domain.ReferenceOrgStructure{}, err
	}
	out := domain.ReferenceOrgStructure{BusinessUnits: []domain.ReferenceBusinessUnit{}}
	for _, bu := range nodes {
		b := domain.ReferenceBusinessUnit{Name: bu.Name, Departments: []domain.ReferenceDepartment{}}
		for _, d := range bu.Children {
			dep := domain.ReferenceDepartment{Name: d.Name, Teams: []string{}}
			for _, t := range d.Children {
				dep.Teams = append(dep.Teams, t.Name)
			}
			b.Departments = append(b.Departments, dep)
		}
		out.BusinessUnits = append(out.BusinessUnits, b)
	}
	return out, nil
}
