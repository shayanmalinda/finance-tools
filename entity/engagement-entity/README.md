# Engagement Entity

Go REST service that owns database and NetSuite access for the Engagement App
(Release 1: the legacy Allocations and Travel apps merged into One WSO2). It is
the only component that reads and writes the legacy databases:

- `allocation_db` (read/write): engagements, allocations, engagement and
  allocation types, Allocations roles and privileges.
- `travelapp` (read/write): travel requests, invoices, per diems, passports and
  visas, Travel roles and privileges.
- `expense_app` (read-only): card spend for the travel cost dashboard.

Its one caller is the Engagement App Backend (`apps/engagement-app/backend`).

## Quick Start

```bash
# from entity/engagement-entity/
cp .env.local .env   # then fill in DB_HOST, DB_USER, DB_PASSWORD
go run ./cmd/api
```

Service starts at `http://localhost:8290`. To run it together with the
backend, use `apps/engagement-app/run-local.sh`.

## Overview

- Default port: `:8290`
- Runtime: Go `1.25+`, standard library router (`net/http` ServeMux)
- Entry point: `cmd/api/main.go`
- Authentication: the Engagement App Backend reaches the entity through its
  Choreo **Organization** endpoint (`.choreo/component.yaml`), with its
  client-credentials token. The gateway checks that token and forwards a JWT
  it signs in `X-JWT-Assertion`; the `auth` middleware verifies it (RS256
  against `GATEWAY_JWT_JWKS_URL`, issuer, expiry) and admits only the client
  applications in `ALLOWED_CLIENT_IDS`: 401 without a valid assertion, 403 for
  another application. Only `GET /health` is open. The backend is the trusted
  caller: it verifies the user's JWT and enforces the legacy
  privileges, then passes the acting user's email in the JSON body of writes
  (`createdBy` / `updatedBy`). Handler code never reads identity from headers.
  The one use of the `X-User-Email` header is the audit trail: the `Actor`
  middleware, which runs after `auth`, puts it on the request context and `audit_event` rows take their
  actor from it, falling back to the body's user.
- The two legacy apps keep separate roles, privileges and `role_privilege`
  tables. Every access endpoint takes the app (`allocations` or `travel`) in
  the path, and the two apps' privilege ids are never merged: Allocations
  privilege 1 and Travel privilege 1 are different things.

## Prerequisites

- Go `1.25+` ([install](https://go.dev/doc/install))
- MySQL 8 reachable over TLS (never needed for the tests)

## Testing

The tests use `go-sqlmock`; none connects to a database.

```bash
go test ./...              # all tests
go test -race ./...        # with the race detector
go vet ./...
go build ./...
gofmt -l .                 # must print nothing
```

## Configuration

Settings come from environment variables; locally from a git-ignored `.env`
(copy `.env.local`). The database host, user and password must be set, and
the gateway JWT settings unless `AUTH_DISABLED=true` (local run only: the
server then listens on `127.0.0.1`, so a deployment that sets it by mistake is
unreachable, not open).
Never put real values in committed files.

| Variable | Required? | Default | Where to get it (legacy key) |
|---|---|---|---|
| `DB_HOST` | Yes | – | MySQL host of the legacy schemas (`[finance_entity.database.allocationDatabase] hostname`) |
| `DB_USER` | Yes | – | One user for all schemas: read/write on `allocation_db` and `travelapp`; `SELECT` on `expense_app` (Infra) |
| `DB_PASSWORD` | Yes (secret) | – | That user's password |
| `DB_PORT` | No | `3306` | – |
| `DB_TLS` | No | empty = verified TLS (`tls=true`) | Set `false` only for a local MySQL without TLS. `skip-verify` does not check the certificate: non-production only (a startup warning says so) |
| `DB_TLS_CA` | When the MySQL certificate is from an internal CA | – | Path to a PEM CA bundle, mounted with Choreo "File and Certificate Mount". Verified TLS against it (TLS 1.2+); an unreadable file or one with no certificate stops startup |
| `DB_TLS_SERVER_NAME` | When connecting by IP | – | The host name the MySQL certificate carries, when `DB_HOST` is an IP the certificate doesn't list |
| `NON_PRODUCTION` | Staging and locally | `false` | `true` allows `DB_TLS` `false`/`skip-verify`, `AUDIT_OPTIONAL`, `AUTH_DISABLED` and no `GATEWAY_JWT_API_CONTEXTS`; otherwise startup fails |
| `AUDIT_OPTIONAL` | Locally | `false` | `true` lets the entity start without the `audit_event` tables; otherwise startup fails and names the migration to run |
| `SERVER_PORT` | No | `8290` | – |
| `GATEWAY_JWT_JWKS_URL` | Yes, unless `AUTH_DISABLED` | – | The Choreo gateway's keys, `https://<gateway-domain>/.wellknown/jwks` (staging `https://apis-stg.wso2.com/.wellknown/jwks`) |
| `GATEWAY_JWT_ISSUER` | No | `wso2.org/products/am` | `iss` of the gateway's JWT |
| `ALLOWED_CLIENT_IDS` | Yes, unless `AUTH_DISABLED` | – | Comma-separated client ids let in: the Engagement App Backend's `OUTBOUND_CLIENT_ID`. Read from `client_id`, else `azp`, else `http://wso2.org/claims/client_id`, the same order as the Customer Entity's `AUTH_INTERNAL_CLIENT_IDS` (cs-tools-membership-scope `entity-service`) |
| `CLIENT_ID_CLAIMS` | No | `client_id,azp,http://wso2.org/claims/client_id` | Comma-separated claims that name the calling application, tried in order. Change it only if the gateway JWT names the client elsewhere: a refused call logs the claim names it carried (never values) |
| `GATEWAY_JWT_API_CONTEXTS` | Recommended | – | Comma-separated API contexts the assertion must be issued for, e.g. `/wutu/engagement-entity/v1.0`, so an assertion the gateway issued for another API (Sales or HR entity, Email Service) is refused (403). Unset skips the check, with a startup warning |
| `API_CONTEXT_CLAIMS` | No | `http://wso2.org/claims/apicontext` | Claims that name that API, tried in order |
| `AUTH_DISABLED` | Locally | `false` | `true` skips the JWT check and binds to `127.0.0.1` |
| `NETSUITE_PUSH_ENABLED` | No | `false` | `true` turns NetSuite on (invoice and per diem push, re-sync lookups, Marketing Event creation); then the settings below are required |
| `NETSUITE_ACCOUNT_ID` | When on | – | `[finance_entity_service.netsuite.netSuiteConfigs] accountId` |
| `NETSUITE_CONSUMER_KEY` / `_SECRET` | When on (secret) | – | `consumerId` / `consumerSecret` |
| `NETSUITE_TOKEN_ID` / `_SECRET` | When on (secret) | – | `tokenId` / `tokenSecret` |
| `NETSUITE_PURCHASE_ORDER_TAX_CODE` | When on | – | `[app_travel.operation] taxCode` |
| `NETSUITE_CREATE_PURCHASE_ORDER_SCRIPT_ID`, `NETSUITE_GET_PURCHASE_ORDER_SCRIPT_ID` | When on | – | `netSuiteConfigs.createPurchaseOrder` / `getPurchaseOrder` `scriptId` |
| `NETSUITE_CREATE_VENDOR_BILL_SCRIPT_ID`, `NETSUITE_GET_VENDOR_BILL_SCRIPT_ID` | When on | – | `netSuiteConfigs.insertPerDiemPayments` / `getVendorBill` `scriptId` |
| `NETSUITE_CREATE_MARKETING_EVENT_SCRIPT_ID` | When on | – | `[finance_entity.netsuite.netSuiteConfigs.createMarketingEvent] scriptId` |
| `NETSUITE_*_DEPLOY_ID` (one per script) | No | `1` | the script's `deploymentId` |
| `NETSUITE_RESTLETS_BASE_URL` | No | `https://<account>.restlets.api.netsuite.com` | `restletsBaseUrl` |

Fixed, not settings: schema names `allocation_db`, `travelapp`,
`expense_app`; pool sizes (20 open for allocation_db and
travelapp, 5 for the others); connection lifetimes.

How connections are made, as in the GRC compliance entity:

- **TLS:** verified TLS by default, no plaintext fallback.
- **UTC:** every session is pinned to UTC (`time_zone='+00:00'`, `loc=UTC`).
- **Read-only:** the `expense_app` pool sets `transaction_read_only=1` on
  every connection, so a write through it fails even if the user had more
  than `SELECT`.
- **Campaign column:** until the migration adds `engagement.campaign_id`, campaigns read as empty.
- **Row counts:** `clientFoundRows=true`, so re-saving an unchanged row
  counts as found (as the legacy Ballerina client did).

## Project Structure

```text
engagement-entity/
├── cmd/api/main.go            # Entry point: config, DB pools, server, graceful shutdown
├── Resources/                 # Hand-run schema changes (see "Schema changes")
├── internal/
│   ├── apierror/              # Typed API errors and the {code, message} body
│   ├── audit/                 # audit_event rows, written in the transaction of each change
│   ├── auth/                  # Gateway JWT (X-JWT-Assertion) check and client allow-list
│   ├── cache/                 # Generic in-memory TTL cache
│   ├── config/                # Env var loading + Validate()
│   ├── db/                    # One MySQL pool per schema; TLS default, UTC, read-only
│   ├── domain/                # Request/response types, one file per area
│   ├── handler/               # HTTP handlers, one file per area
│   ├── middleware/            # CorrelationID, Actor (X-User-Email), Recovery, Logger, Timeout
│   ├── netsuite/              # RESTlet client: OAuth 1.0a TBA (HMAC-SHA256), PO, VB, marketing event, lookups
│   ├── repository/            # SQL per area and pool, no business rules; sqlutil.go (WithTx, Placeholders)
│   ├── server/                # routes.go + routes_<area>.go (dependency graph + routes), server.go
│   ├── service/               # Validation, defaults, multi-step writes
│   └── textnorm/              # Latin1(): makes text fit the legacy latin1 columns
├── openapi.yaml
├── project.toml               # Choreo Go buildpack: GOOGLE_BUILDABLE=./cmd/api
└── .choreo/component.yaml
```

**Request flow:**

```text
HTTP request (from the Engagement App Backend)
    → middleware (CorrelationID → Recovery → Logger → Timeout 30 s)
    → handler    (parse request, call service, write response)
    → service    (validation, defaults, transactions across pools)
    → repository (SQL)
    → MySQL
```

## API Endpoints

`openapi.yaml` is the full contract. Paths are relative to the service root
(no version prefix); every error body is `{"code": <status>, "message": "..."}`.

| Area | Paths |
|---|---|
| Health, access | `GET /health`, `GET /access/{app}/roles`, `/privileges`, `/role-privileges`, `GET /access/travel/agency?email=` |
| Lookups | `GET /engagement-types`, `GET /allocation-types`, `GET /reference-data` |
| Engagements | `POST /engagements/search`, `GET /engagements/facets`, `POST /engagements`, `GET`/`PATCH /engagements/{id}` (Cancelled cascades to allocations and travel), `GET /engagements/{id}/roster` |
| Settings | `GET /role-settings/{app}`, `PUT /role-settings/{app}/roles/{roleId}/privileges`, `/settings/allocation-types` CRUD |
| Allocations | `GET /allocations/engagements/{id}`, `GET`/`PATCH`/`DELETE /allocations/{id}`, `POST /allocations/search`, `/allocations/timeline`, `/allocations/bulk`, `/allocations/visas`, `GET /consultant-allocations` (+ `/non-allocated`, `/team-defaults`, `/engagement-codes`) |
| Travel | `POST /travel-requests/search`, `GET`/`PATCH /travel-requests/{jobNumber}`, `/active-per-diems`, `GET /travel-agencies`, `POST /travel-email-audit-logs`, `/visas` and `/travel-documents` (list, add with the override rule, get, update) |
| Finance (`/finance/...`) | per diems, travel cards, invoices (add, decisions, push, re-sync, NetSuite reference), per diem payments (add, edit, push, re-sync), counts, cost types, subsidiaries |
| Dashboard (`/dashboard/...`) | `allocations-today`, `my-work`, `queues`, `travel`, `engagements`, `agency` |
| Audit | `POST /audit-events` (views, downloads and exports; writes record their own `audit_event` rows) |

### NetSuite push and NS Sync Failures

One push per record at a time, with a NetSuite lookup before each push; a failed push is marked in existing columns (invoice `reject_reason` prefix, payment `ns_status = 'Failed'`) and re-sync is lookup only.

### Examples

```bash
curl http://localhost:8290/health
curl http://localhost:8290/access/allocations/role-privileges
curl 'http://localhost:8290/engagement-types?type=Internal&isActive=true'
```

## Text normalisation (latin1 columns)

The legacy text columns are MySQL `latin1` (Windows-1252). A character they
cannot hold makes MySQL reject the whole write (LEGACY-ISSUES #1: "→" in an
engagement name). Every repository write of free text to a legacy column must
pass the value through `textnorm.Latin1`. It keeps every character the column
can store (accented letters such as "é", curly quotes, en and em dashes, "€",
"…", "™"), replaces the others with the closest plain equivalent ("→" → "->",
"ő" → "o") and drops the rest (emoji, invisible characters).

## Schema changes

As in `grc-tools/apps/grc-platform/backend/Resources`, schema changes are
SQL scripts that Infra runs by hand before the deploy that needs them (the
`audit_event` scripts are safe to re-run, the `campaign_id` one is not).
Nothing runs them automatically.

Each script holds one statement, so it runs with Ctrl+Enter or as a script.

| Script | Change |
|---|---|
| `Resources/allocation_db_table_update_v1.0.0.sql` | Adds nullable `engagement.campaign_id VARCHAR(18)` (Salesforce Campaign of a Marketing Event). The engagement and finance queries read the column, so run it before deploying. Run it once; a second run fails with a duplicate-column error. |
| `Resources/allocation_db_table_update_v1.1.0.sql` | Adds the `audit_event` table to allocation_db. Safe to re-run. |
| `Resources/travelapp_table_update_v1.0.0.sql` | Adds the `audit_event` table to travelapp. Safe to re-run. Until both schemas have it, the entity refuses to start (unless `AUDIT_OPTIONAL=true`, local runs only). |

## Scopes (planned)

Per-operation OAuth scopes on top of the client allow-list are planned in
[`docs/scopes.md`](docs/scopes.md): scope names, the route-to-scope table
(fine-grained and coarse), enforcement at the Choreo gateway and in the
entity (`ENFORCE_SCOPES`), and the rollout order. Not enforced yet.

## Deployment

Go-live checklist (production, and staging configured the same way):

- Both `audit_event` migrations above have run; the entity does not start
  without them.
- MySQL TLS is verified: `DB_TLS` empty, with `DB_TLS_CA` (the internal CA,
  mounted as a file) and `DB_TLS_SERVER_NAME` when `DB_HOST` is an IP. Never
  `skip-verify`.
- `GATEWAY_JWT_JWKS_URL` and `ALLOWED_CLIENT_IDS` are set; `AUTH_DISABLED`
  and `AUDIT_OPTIONAL` are not.

Choreo builds the component with its Go buildpack. `project.toml` points the
buildpack at `./cmd/api` (`GOOGLE_BUILDABLE`), as
`grc-tools/apps/grc-platform/backend/project.toml` does for `./cmd/server`.
The endpoint is visible to the Choreo project and the organization. Callers
need the gateway: the Organization endpoint forwards `X-JWT-Assertion` (enable
"Pass end-user attributes to upstream" on the endpoint); a direct Project
URL call has none and gets 401.
