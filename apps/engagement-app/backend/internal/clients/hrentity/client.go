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

// Package hrentity is a read-only client for the WSO2 HR Entity GraphQL
// service. The backend uses it for employees (allocation search by email,
// traveller details) and leads (managerEmail, for the approval routing to the
// closest approver in the reporting line).
//
// The service lives in another Choreo project, so calls go through a Choreo
// connection with OAuth2 client credentials (rest.NewOAuthHTTPClient). The
// queries follow the Travel backend Go port and the GRC platform backend.
package hrentity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/apierror"
	"github.com/wso2-open-operations/finance-tools/apps/engagement-app/backend/internal/clients/rest"
)

const serviceName = "hr-entity"

// Client is what the backend needs from the HR Entity.
type Client interface {
	// Employee returns the employee with this work email, or nil when HR has
	// none.
	Employee(ctx context.Context, email string) (*Employee, error)
	// SearchEmployees returns employees whose work email contains search,
	// limited to the given statuses (e.g. "Active", "Marked leaver").
	SearchEmployees(ctx context.Context, search string, statuses []string, limit int) ([]Employee, error)
	// ReportsTo returns the employee's reporting lead address(es): HR
	// reportsToEmail split on ",", as the allocation BFF getReportsToEmails.
	// Empty when HR records none.
	ReportsTo(ctx context.Context, email string) ([]string, error)
	// OrgData returns the organisation tree (business unit, department,
	// team) of the employees in the given statuses (allocation BFF
	// getOrgData).
	OrgData(ctx context.Context, statuses []string) ([]OrgNode, error)
}

// OrgNode is a node of HR orgData.orgStructure: a business unit at the top,
// departments below it, teams below those.
type OrgNode struct {
	Name     string    `json:"name"`
	Level    *int      `json:"level"`
	Type     *string   `json:"type"`
	TypeName *string   `json:"typeName"`
	Children []OrgNode `json:"children"`
}

// Employee is the subset of HR's Employee the Engagement App uses.
type Employee struct {
	EmployeeID   string  `json:"employeeId"`
	WorkEmail    string  `json:"workEmail"`
	FirstName    string  `json:"firstName"`
	LastName     string  `json:"lastName"`
	JobRole      *string `json:"jobRole"`
	Company      *string `json:"company"`
	Department   *string `json:"department"`
	Location     *string `json:"location"`
	ManagerEmail *string `json:"managerEmail"`
	BusinessUnit *string `json:"businessUnit"`
	Team         *string `json:"team"`
}

const employeeFields = `employeeId workEmail firstName lastName jobRole company businessUnit department team location managerEmail`

const employeeQuery = `query employee($email: String!) { employee(email: $email) { ` + employeeFields + ` } }`

const employeesQuery = `query employees($filter: EmployeeFilter, $limit: Int) {
	employees(filter: $filter, limit: $limit) { ` + employeeFields + ` }
}`

// HTTPClient implements Client against the GraphQL endpoint.
type HTTPClient struct{ rest *rest.Client }

// New returns an HTTPClient for the GraphQL endpoint graphqlURL,
// authenticating with httpClient (normally rest.NewOAuthHTTPClient).
func New(graphqlURL string, httpClient *http.Client) *HTTPClient {
	return &HTTPClient{rest: rest.New(serviceName, graphqlURL, httpClient)}
}

var _ Client = (*HTTPClient)(nil)

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *HTTPClient) query(ctx context.Context, q string, vars map[string]any, out any) error {
	var resp graphqlResponse
	if err := c.rest.Post(ctx, "", graphqlRequest{Query: q, Variables: vars}, &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return &apierror.UpstreamError{Service: serviceName, StatusCode: http.StatusBadGateway, Msg: resp.Errors[0].Message}
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return &apierror.UpstreamError{Service: serviceName, StatusCode: http.StatusBadGateway, Msg: "response has no data"}
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return &apierror.UpstreamError{Service: serviceName, StatusCode: http.StatusBadGateway, Msg: "malformed data: " + err.Error()}
	}
	return nil
}

// Employee implements Client.
func (c *HTTPClient) Employee(ctx context.Context, email string) (*Employee, error) {
	var data struct {
		Employee *Employee `json:"employee"`
	}
	if err := c.query(ctx, employeeQuery, map[string]any{"email": strings.TrimSpace(email)}, &data); err != nil {
		return nil, err
	}
	return data.Employee, nil
}

// SearchEmployees implements Client.
func (c *HTTPClient) SearchEmployees(ctx context.Context, search string, statuses []string, limit int) ([]Employee, error) {
	filter := map[string]any{"emailSearchString": search}
	if len(statuses) > 0 {
		filter["employeeStatus"] = statuses
	}
	var data struct {
		Employees []Employee `json:"employees"`
	}
	if err := c.query(ctx, employeesQuery, map[string]any{"filter": filter, "limit": limit}, &data); err != nil {
		return nil, err
	}
	return data.Employees, nil
}

const thumbnailQuery = `query employeeThumbnail($email: String!) { employee(email: $email) { employeeThumbnail } }`

// ThumbnailReader is the optional part of Client that reads an employee's
// profile photo (a data URI or an https URL; "" when HR has none). It is a
// separate query, so the other employee reads stay as light as they were.
type ThumbnailReader interface {
	Thumbnail(ctx context.Context, email string) (string, error)
}

var _ ThumbnailReader = (*HTTPClient)(nil)

// Thumbnail implements ThumbnailReader.
func (c *HTTPClient) Thumbnail(ctx context.Context, email string) (string, error) {
	var data struct {
		Employee *struct {
			EmployeeThumbnail *string `json:"employeeThumbnail"`
		} `json:"employee"`
	}
	if err := c.query(ctx, thumbnailQuery, map[string]any{"email": strings.TrimSpace(email)}, &data); err != nil {
		return "", err
	}
	if data.Employee == nil || data.Employee.EmployeeThumbnail == nil {
		return "", nil
	}
	return *data.Employee.EmployeeThumbnail, nil
}

const reportsToQuery = `query employeeQuery($email: String!) { employee(email: $email) { reportsToEmail } }`

// orgDataQuery is the allocation BFF orgDataQuery (three levels).
const orgDataQuery = `query orgDataQuery($employeeStatuses: [String!]) {
	orgData(employeeStatuses: $employeeStatuses) {
		orgStructure {
			name level type typeName
			children { name level type typeName children { name level type typeName } }
		}
	}
}`

// ReportsTo implements Client.
func (c *HTTPClient) ReportsTo(ctx context.Context, email string) ([]string, error) {
	var data struct {
		Employee *struct {
			ReportsToEmail *string `json:"reportsToEmail"`
		} `json:"employee"`
	}
	if err := c.query(ctx, reportsToQuery, map[string]any{"email": strings.TrimSpace(email)}, &data); err != nil {
		return nil, err
	}
	out := []string{}
	if data.Employee == nil || data.Employee.ReportsToEmail == nil {
		return out, nil
	}
	for _, a := range strings.Split(*data.Employee.ReportsToEmail, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out, nil
}

// OrgData implements Client.
func (c *HTTPClient) OrgData(ctx context.Context, statuses []string) ([]OrgNode, error) {
	var data struct {
		OrgData struct {
			OrgStructure []OrgNode `json:"orgStructure"`
		} `json:"orgData"`
	}
	vars := map[string]any{"employeeStatuses": nil}
	if len(statuses) > 0 {
		vars["employeeStatuses"] = statuses
	}
	if err := c.query(ctx, orgDataQuery, vars, &data); err != nil {
		return nil, err
	}
	return data.OrgData.OrgStructure, nil
}

// Unconfigured is the Client used when HR_ENTITY_GRAPHQL_URL is unset: the
// backend starts, and every call answers 503.
type Unconfigured struct{}

var _ Client = Unconfigured{}

func notConfigured() error {
	return &apierror.UpstreamError{Service: serviceName, Msg: "not configured (HR_ENTITY_GRAPHQL_URL is unset)"}
}

// Employee implements Client.
func (Unconfigured) Employee(context.Context, string) (*Employee, error) {
	return nil, notConfigured()
}

// SearchEmployees implements Client.
func (Unconfigured) SearchEmployees(context.Context, string, []string, int) ([]Employee, error) {
	return nil, notConfigured()
}

// ReportsTo implements Client.
func (Unconfigured) ReportsTo(context.Context, string) ([]string, error) {
	return nil, notConfigured()
}

// OrgData implements Client.
func (Unconfigured) OrgData(context.Context, []string) ([]OrgNode, error) {
	return nil, notConfigured()
}

// ── Allocations area ─────────────────────────────────────────────────────────
// Added by the allocation screen area (team pickers, bulk upload, report,
// allocation emails). A separate interface, so fakes of Client elsewhere keep
// compiling; HTTPClient and Unconfigured implement it.

// OrgClient lists employees by organisation and reads one employee's org,
// employment and reporting-line record, as the legacy Allocations backend
// did (GET /employees, GET /employees/{email}, getEmployeeReportsTo).
type OrgClient interface {
	// EmployeesByOrg returns every employee matching the filter, read in
	// pages until an empty page (legacy getEmployees).
	EmployeesByOrg(ctx context.Context, f OrgEmployeeFilter) ([]OrgEmployee, error)
	// EmployeeRecord returns the employee's org, final day and reporting
	// lead(s), or nil when HR has no such employee.
	EmployeeRecord(ctx context.Context, email string) (*EmployeeRecord, error)
}

// OrgEmployeeFilter is the legacy EmployeeFilter.
type OrgEmployeeFilter struct {
	Statuses                []string
	ExcludedEmploymentTypes []string
	BusinessUnit            string
	Department              string
	Team                    string
}

// OrgEmployee is an employee in an org listing.
type OrgEmployee struct {
	WorkEmail    string  `json:"workEmail"`
	FirstName    string  `json:"firstName"`
	LastName     string  `json:"lastName"`
	JobRole      *string `json:"jobRole"`
	Team         *string `json:"team"`
	BusinessUnit *string `json:"businessUnit"`
	Department   *string `json:"department"`
	Company      *string `json:"company"`
	Location     *string `json:"location"`
}

// EmployeeRecord is one employee's org, final day of employment and
// reporting lead(s) (reportsToEmail, comma-separated in HR).
type EmployeeRecord struct {
	BusinessUnit         *string `json:"businessUnit"`
	Department           *string `json:"department"`
	Team                 *string `json:"team"`
	Company              *string `json:"company"`
	FinalDayOfEmployment *string `json:"finalDayOfEmployment"`
	ReportsToEmail       *string `json:"reportsToEmail"`
}

const orgEmployeesPageSize = 1000

// orgEmployeesMaxRows stops a listing that never reaches an empty page
// (far above the head count). orgEmployeesMaxPages bounds the calls on
// their own, and still reads that many rows when HR caps a page at 100.
// One more call after the last data page checks that the listing ended.
const (
	orgEmployeesMaxRows  = 100_000
	orgEmployeesMaxPages = 1000
)

const orgEmployeesQuery = `query employeesQuery($filter: EmployeeFilter!, $limit: Int!, $offset: Int!) {
	employees(filter: $filter, limit: $limit, offset: $offset) {
		workEmail firstName lastName jobRole team businessUnit department company location
	}
}`

const employeeRecordQuery = `query employeeQuery($email: String!) {
	employee(email: $email) { businessUnit department team company finalDayOfEmployment reportsToEmail }
}`

var _ OrgClient = (*HTTPClient)(nil)
var _ OrgClient = Unconfigured{}

// EmployeesByOrg implements OrgClient.
func (c *HTTPClient) EmployeesByOrg(ctx context.Context, f OrgEmployeeFilter) ([]OrgEmployee, error) {
	filter := map[string]any{}
	if len(f.Statuses) > 0 {
		filter["employeeStatus"] = f.Statuses
	}
	if len(f.ExcludedEmploymentTypes) > 0 {
		filter["excludedEmploymentTypes"] = f.ExcludedEmploymentTypes
	}
	for k, v := range map[string]string{"businessUnit": f.BusinessUnit, "department": f.Department, "team": f.Team} {
		if v != "" {
			filter[k] = v
		}
	}
	// Read until an empty page, as the legacy did: HR may cap the limit
	// below orgEmployeesPageSize, so a short page is not the last one.
	out := []OrgEmployee{}
	var prev []OrgEmployee
	for page := 0; page <= orgEmployeesMaxPages && len(out) <= orgEmployeesMaxRows; page++ {
		var data struct {
			Employees []OrgEmployee `json:"employees"`
		}
		vars := map[string]any{"filter": filter, "limit": orgEmployeesPageSize, "offset": len(out)}
		if err := c.query(ctx, orgEmployeesQuery, vars, &data); err != nil {
			return nil, err
		}
		if len(data.Employees) == 0 {
			return out, nil
		}
		if orgSamePage(prev, data.Employees) {
			return nil, &apierror.UpstreamError{Service: serviceName, StatusCode: http.StatusBadGateway,
				Msg: "employee listing returned the same page twice (offset ignored)"}
		}
		out = append(out, data.Employees...)
		prev = data.Employees
	}
	return nil, &apierror.UpstreamError{Service: serviceName, StatusCode: http.StatusBadGateway,
		Msg: "employee listing did not end within the page limit"}
}

// orgSamePage reports whether two pages list the same employees in order.
func orgSamePage(a, b []OrgEmployee) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i].WorkEmail, b[i].WorkEmail) {
			return false
		}
	}
	return true
}

// EmployeeRecord implements OrgClient.
func (c *HTTPClient) EmployeeRecord(ctx context.Context, email string) (*EmployeeRecord, error) {
	var data struct {
		Employee *EmployeeRecord `json:"employee"`
	}
	if err := c.query(ctx, employeeRecordQuery, map[string]any{"email": strings.TrimSpace(email)}, &data); err != nil {
		return nil, err
	}
	return data.Employee, nil
}

// EmployeesByOrg implements OrgClient.
func (Unconfigured) EmployeesByOrg(context.Context, OrgEmployeeFilter) ([]OrgEmployee, error) {
	return nil, notConfigured()
}

// EmployeeRecord implements OrgClient.
func (Unconfigured) EmployeeRecord(context.Context, string) (*EmployeeRecord, error) {
	return nil, notConfigured()
}
