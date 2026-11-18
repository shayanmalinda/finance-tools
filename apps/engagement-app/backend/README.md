# Engagement App Backend

Go backend that One WSO2 calls for the Engagement App (Release 1: the legacy
Allocations and Travel apps merged into One WSO2). It owns the access checks
(the legacy privileges), workflow, email and file storage, and calls:

- the **Engagement Entity** (`entity/engagement-entity`, same Choreo project):
  every read and write of the legacy databases and NetSuite;
- the **Sales Entity Service** (digiops-sales): accounts, opportunities,
  campaigns;
- the **HR Entity**: employees and leads.

## Quick Start (local, with One WSO2 on localhost:3000)

```bash
# 1. Settings: copy the templates and fill them in (both .env files are git-ignored)
cp ../../../entity/engagement-entity/.env.local ../../../entity/engagement-entity/.env
cp .env.local .env

# 2. Start the Engagement Entity (:8290) and this backend (:8190) together
../run-local.sh

# 3. Point One WSO2 (npx vite --port 3000 --strictPort) at http://localhost:8190
#    and sign in with the staging Asgardeo client. Or call it directly:
curl http://localhost:8190/health
curl -H "Authorization: Bearer $ACCESS_TOKEN" http://localhost:8190/user-info
```

Locally One WSO2 sends the Asgardeo access token as `Authorization: Bearer`; missing groups are read from Asgardeo userinfo. No auth bypass.

## Overview

- Default port: `:8190`
- Runtime: Go `1.25+`, standard library router (`net/http` ServeMux)
- Entry point: `cmd/server/main.go`
- Authentication: `internal/middleware/auth.go` reads the token from
  `X-Jwt-Assertion` (Choreo gateway) or, when absent, `Authorization: Bearer`
  (One WSO2 calling directly, e.g. locally), and verifies it in
  `internal/oidc`: RS256 signature against the issuer's JWKS (cached;
  refetched on an unknown key id at most every 30 s, and after 30 minutes;
  a failed fetch is 503, not 401, when the token's key is not cached; a
  cached key is still used for up to 30 more minutes while refetches fail,
  then the request is 503 until a fetch succeeds), issuer, expiry
  (required), issued-at, and audience (`JWT_AUDIENCES`, required). Trusted
  issuers: the WSO2 Asgardeo organisation (default) and, when
  `GATEWAY_JWT_ISSUER` is set, the gateway's issuer. Missing email or groups
  in a Bearer token are read from the userinfo endpoint (cached 5 minutes per
  token hash; when userinfo rejects the token the request is 401, when it
  fails otherwise 503, since the caller's access is unknown). The caller's email and groups go into the request context.
- Authorization: the legacy apps have no user-to-role table; a role is an
  Asgardeo group. `internal/service/access_service.go` reads each app's
  role-privilege map from the Engagement Entity (cached one minute), and
  resolves the caller's groups against it, case-insensitively as the legacy
  MySQL queries did. Allocations and Travel privileges are kept apart
  everywhere: the same number means different things in each app.
- Screen endpoints are registered with `a.handle(pattern, gate, h)` (or
  `a.handleIdem` for creates), which wraps them in `middleware.Require`: the
  legacy privilege gate (any of the listed ids, per app), 403 when the caller
  holds none, 503 when the role tables cannot be read. The resolved
  privileges (`domain.Perms`, including the "own" privileges without the
  super admin groups and the travel agency of an agent) go into the context
  for record-level rules.
- Double submit: create, allocate and push routes accept an
  `Idempotency-Key` header (`internal/middleware/idempotency.go`). The store
  is per replica, holds at most 100 live keys per caller, and keeps responses
  up to 256 KiB.
- Logs: request paths are logged without their query and with email
  segments replaced by `{email}` (`internal/redact`); a sent email is logged
  by type, reference and recipient counts, not addresses or subject.
- Audit: every Engagement Entity call carries the user (`X-User-Email`) and
  the correlation ID; the entity records each write. Views and downloads of
  passports, visas and travel documents and CSV exports are sent to the
  entity's `POST /audit-events` in the background
  (`internal/middleware/audit.go`); a failure is only logged.

## Prerequisites

- Go `1.25+`
- A running Engagement Entity (only for running the server; tests use fakes)

## Testing

None of the tests calls a real service: the JWT tests run a fake issuer with
its own JWKS over `httptest`, and services use a fake Engagement Entity
client.

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
gofmt -l .           # must print nothing
```

## Configuration

Settings come from environment variables; locally from a git-ignored `.env`
(copy `.env.local`). `ENGAGEMENT_ENTITY_BASE_URL`, `JWT_AUDIENCES` and
`EMAIL_SERVICE_URL` (or `EMAIL_DISABLED=true`) are required to start, plus
`ENTITY_CLIENT_ID` and `ENTITY_CLIENT_SECRET` when email is on or
`SALES_ENTITY_BASE_URL` or `HR_ENTITY_URL` is set. Every other service is
optional: unset, the backend starts and the screens that need it answer 503. Service and token
URLs must be https (http only on localhost). Never put real values in
committed files.

| Variable | Required? | Default | Where to get it (legacy key) |
|---|---|---|---|
| `ENGAGEMENT_ENTITY_BASE_URL` | Yes | – | Choreo connection to the Engagement Entity (same project); locally `http://localhost:8290` |
| `SALES_ENTITY_BASE_URL` | For accounts, opportunities, line items and campaigns | – | Choreo connection to the Go Sales Entity Service |
| `HR_ENTITY_URL` | For employee and organisation lookups | – | `[app_allocation.entity] hrEntityEndpoint` |
| `ENTITY_CLIENT_ID` | When `SALES_ENTITY_BASE_URL`, `HR_ENTITY_URL` or `EMAIL_SERVICE_URL` (email on) is set | – | `[app_allocation.entity.clientAuthConfig] clientId` (one pair for every outbound WSO2 service, as legacy) |
| `ENTITY_CLIENT_SECRET` | Same (secret) | – | `[app_allocation.entity.clientAuthConfig] clientSecret` |
| `ENTITY_TOKEN_URL` | No | `https://api.asgardeo.io/t/wso2/oauth2/token` | `[app_allocation.entity.clientAuthConfig] tokenUrl` |
| `JWT_ISSUER` | No | `https://api.asgardeo.io/t/wso2/oauth2/token` | WSO2 Asgardeo organisation |
| `JWT_JWKS_URL` | No | `https://api.asgardeo.io/t/wso2/oauth2/jwks` | – |
| `JWT_USERINFO_URL` | No | `https://api.asgardeo.io/t/wso2/oauth2/userinfo` | – |
| `JWT_AUDIENCES` | Yes | – | One WSO2 client id (comma-separated list allowed) |
| `GATEWAY_JWT_ISSUER`, `GATEWAY_JWT_JWKS_URL` | Only if the gateway's JWT is not issued by Asgardeo | – | Choreo gateway backend JWT issuer and keys (to confirm on staging) |
| `GATEWAY_JWT_AUDIENCES` | Only if the gateway's JWT names another audience | `JWT_AUDIENCES` | The `aud` of the gateway's JWT (comma-separated list allowed) |
| `JWT_CLOCK_SKEW_SECONDS` | No | `60` | – |
| `CORS_ALLOWED_ORIGINS` | Locally | – (none) | `http://localhost:3000`; behind the gateway CORS is the gateway's |
| `SERVER_PORT` | No | `8190` | – |
| `EMAIL_SERVICE_URL` | Yes, unless `EMAIL_DISABLED` | – | `[app_allocation.email.emailServiceConfig] endpointUrl` |
| `EMAIL_DISABLED` | Locally | `false` | `true` logs that an email would be sent (no addresses or subject) instead of sending it |
| `EMAIL_TEST_RECIPIENT` | No | – | Set on staging: every email goes only here (`isSendTestingEmailsOnlyVal` + `allocationTestEmail`) |
| `EMAIL_ALLOCATIONS_APP_UUID`, `EMAIL_TRAVEL_APP_UUID` | With email | – | `[app_allocation.email] appUuid`, `[app_travel.email.emailConfigurations] travelAppUUID` |
| `EMAIL_ALLOCATIONS_FROM`, `EMAIL_TRAVEL_FROM` | With email | – | `allocationManagerEmail`, `travelAppEmail` |
| `EMAIL_ALLOCATIONS_GROUP`, `EMAIL_PARTNER_ALLOCATIONS_GROUP`, `EMAIL_TRAVEL_GROUP` | With email | – | `allocationGroupEmail`, `partnerAllocationGroupEmail`, `travelGroupEmail` |
| `EMAIL_INTERNAL_APPROVERS`, `EMAIL_MARKETING_APPROVERS` | With email | – | `approveInternalEmail`, `approveMarketingEmail` (comma-separated) |
| `EMAIL_TRAVEL_LOGISTICS_GROUP`, `EMAIL_FINANCE_GROUP` | With email | – | `travelLogisticGroupEmail`, `financeGroupEmail` |
| `EMAIL_COMPANY_RECIPIENTS` | With email | – | `[app_travel.email.companyEmailRecipientsMapping]` as JSON: `{"WSO2 - UK": "a@wso2.com,b@wso2.com"}` |
| `EMAIL_SUPPRESSED` | No | – | `noEmailsTo` (comma-separated) |
| `APP_BASE_URL` | No | `https://one.wso2.com` | Link written into emails (`frontendAppBaseUrl`) |
| `EMAIL_CC_TRAVELLER` | No | `false` | `isSendEmailToTraveller` |
| `CURRENCY_CONVERT_SERVICE_URL` | For per diem payment add and edit | – (503) | `[app_travel.utils] currencyConvertServiceEndpoint` (uses the shared client pair) |
| `CURRENCY_CONVERT_CLIENT_ID` / `_SECRET` / `_TOKEN_URL` | Only if the shared pair is not subscribed to it | shared pair | `[app_travel.utils.clientAuthConfig]` |
| `TRAVEL_VISA_NOT_APPLICABLE_ID`, `TRAVEL_NO_VALID_VISA_ID` | Staging only | `332`, `333` (production) | Legacy webapp `Config.js` visa sentinels (staging 40, 41) |
| `FILE_STORAGE_ACCOUNT` | For passport and visa images | – (uploads 503) | `[app_travel.file.fileServer] accountName` |
| `FILE_STORAGE_ACCESS_KEY` | With the account (secret) | – | env `FILE_STORAGE_ACCESS_KEY` of the legacy Travel backend |

## Project Structure

```text
backend/
├── cmd/server/main.go          # Config, JWT verifier, clients, server, graceful shutdown
├── internal/
│   ├── apierror/               # Typed errors and the {code, message} body
│   ├── cache/                  # Generic in-memory TTL cache
│   ├── clients/
│   │   ├── rest/               # Shared JSON client: correlation ID, error mapping, OAuth2 client
│   │   ├── engagemententity/   # Engagement Entity client (interface + HTTP)
│   │   ├── salesentity/        # Sales Entity Service client (interface + HTTP + Unconfigured)
│   │   └── hrentity/           # HR Entity GraphQL client (interface + HTTP + Unconfigured)
│   ├── config/                 # Env var loading + Validate()
│   ├── domain/                 # API request/response types
│   ├── email/                  # Email service sender (legacy recipient rules, testing mode) + per-area templates
│   ├── filestore/              # Azure Blob store (Shared Key), legacy data-URL images, 5 MB / JPEG-PNG checks
│   ├── handler/                # HTTP handlers and error mapping
│   ├── middleware/             # CorrelationID, Recovery, Logger, CORS, Timeout, Auth, Require (gate), Idempotency, Audit
│   ├── oidc/                   # JWT verification (multi-issuer), userinfo client
│   ├── reqcontext/             # Caller (email, groups) and correlation ID in the context
│   ├── redact/                 # Strips emails and queries from logged paths
│   ├── server/                 # routes.go (shared graph) + routes_<area>.go, server.go
│   └── service/                # Access (Perms per request), then one service per screen area
├── openapi.yaml
├── project.toml                # Choreo Go buildpack: GOOGLE_BUILDABLE=./cmd/server
└── .choreo/component.yaml
```

**Request flow:**

```text
One WSO2 → API gateway (Bearer → X-Jwt-Assertion), or directly with Bearer locally
    → middleware (CorrelationID → Recovery → Logger → CORS → Timeout → Auth → Require(gate) [→ Idempotency])
    → handler → service → clients (Engagement Entity, Sales Entity, HR Entity)
```

## API Endpoints

`openapi.yaml` is the contract One WSO2 builds against; every operation lists
its legacy privilege gate in `x-privileges`. The gateway's base URL includes
the version segment; paths below are relative to it.

| Screen | Paths |
|---|---|
| Access | `GET /health` (no token), `GET /user-info` (privileges per app, own privileges, roles, agency `{id, name}`, `workQueues`) |
| Lookups | `GET /reference-data`, `/accounts` (search as you type), `/accounts/{id}/opportunities`, `/campaigns`, `/employees`, `/org-structure` |
| Dashboard, Me | `GET /dashboard`, `/dashboard/allocations-today`, `/me/overview`, `/me/travel-requests`, `/me/visas`, `/me/travel-documents`, `/me/passports` (+ POST for own visas and documents) |
| Engagements, Approvals | `GET`/`POST /engagements`, `GET /engagements/facets`, `GET`/`PATCH /engagements/{id}`, `POST /engagements/{id}/request-status`, `/engagement-status`, `/approve`, `GET /approvals` |
| Allocations | `POST /engagements/{id}/allocations`, `GET /allocation-timeline`, `GET /allocations`, `PATCH /allocations/{id}/status`, `/team-allocations` (+ `/{id}`, `/bulk/validate`, `/bulk`) |
| Travel Requests | `GET /travel-requests`, `GET`/`PATCH /travel-requests/{jobNumber}`, `/visa-options`, `/per-diems`, `/invoices`, `/travel-cards`, `/per-diem-payments` |
| Invoices & Payments | `GET /invoices`, `/invoices/{id}`, `POST /invoices/approve`, `/reject`, `/push`, `/resync`; `GET /per-diem-payments`, `GET`/`PATCH /per-diem-payments/{id}`, `POST /per-diem-payments/push`, `/resync`; `GET /netsuite-sync-failures` |
| Reports | `GET /reports/consultant-allocations`, `/reports/invoices`, `/reports/per-diem-payments` (`format=csv` for the CSV) |
| Visas & Documents | `/visas`, `/visas/{id}`, `/visas/{id}/image`, `/travel-documents`, `/travel-documents/{id}`, `/travel-documents/{id}/image`, `/travellers/{email}/passports` |
| Settings | `/role-settings/{app}`, `/role-settings/{app}/roles/{roleId}/privileges`, `/settings/allocation-types` |


Errors always have the body `{"code": <status>, "message": "..."}`. 503
means the access check or a dependency failed, or a feature is not
configured locally (Sales, HR, email, files, currency, NetSuite); it never
means "no access" (403).

## Deployment

Choreo builds the component with its Go buildpack. `project.toml` points the
buildpack at `./cmd/server` (`GOOGLE_BUILDABLE`), as
`grc-tools/apps/grc-platform/backend/project.toml` does. The endpoint is
Public: One WSO2 reaches it through the API gateway.
