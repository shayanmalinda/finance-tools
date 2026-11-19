# OAuth scopes for the Engagement Entity (implementation plan)

Status: **plan, not enforced yet.** This PR adds the gateway JWT check and
client allow-list (`internal/auth`). Scopes add a second, per-operation
check on top. This document fixes the scope names, the route mapping and the
rollout, so Choreo, Asgardeo and the code are configured the same way.

## Why scopes as well as the client allow-list

`ALLOWED_CLIENT_IDS` decides *which application* may call the entity at all.
Scopes decide *which operations* that application may call. Today the only
caller is the Engagement App Backend, which needs every operation. Scopes
let a future consumer (a report job, another app) be granted, for example,
`allocations:read` only, without being able to approve invoices or edit role
privileges.

## Naming

`collection:operation`, all lower case, with the collection named after the
resource family in the URL.

- `read`: GET, and the POST searches and timelines that only read.
- `write`: create, update, delete.
- A separate operation where the risk differs from an ordinary write:
  - `invoices:approve` covers the PO and Finance approval decisions.
  - `invoices:push` and `per-diem-payments:push` cover sending to NetSuite, and re-syncing from it.
  - `role-settings:write` is privilege escalation if misused.
  - `audit-events:write` is the audit trail itself.

## Two tiers

Every operation lists **two** scopes, a fine one and a coarse one. A token
that carries **either** passes.

- **Fine-grained** scopes (the table below) are for any future consumer, each
  granted only what it needs.
- **Coarse** scopes are for the Engagement App Backend, which needs
  everything. Granting it four scopes instead of ~30 keeps the Asgardeo
  grant and `OUTBOUND_SCOPES` short, and a new route needs no new grant for
  the backend.

| Coarse scope | Covers | Why grouped |
|---|---|---|
| `engagement:read` | Every read: engagements, allocations, travel, visas and documents, finance lists, reference data, dashboards, access lookups | Reads carry the same risk class (confidential data, no state change) and every screen needs them |
| `engagement:write` | Ordinary writes: engagements, allocations, travel requests, visas, documents, per diems, travel cards, invoices and payments entry, allocation-type settings | Day-to-day operations the backend's privilege checks already gate per user |
| `engagement:finance` | Invoice approval decisions, NetSuite pushes and re-syncs, per diem payment edits | Money leaving or being committed: approvals and pushes to NetSuite. Kept apart so an app that edits travel can't move money |
| `engagement:admin` | Role privilege edits, the audit trail | Privilege escalation and tamper risk: who can do what, and the record of what was done |

The backend's application is granted only the four coarse scopes.

## Route table

The routes are listed from `internal/server/routes*.go`. `GET /health` needs no token and no scope.

| Method and path | Fine scope | Coarse scope |
|---|---|---|
| GET /access/{app}/roles | access:read | engagement:read |
| GET /access/{app}/privileges | access:read | engagement:read |
| GET /access/{app}/role-privileges | access:read | engagement:read |
| GET /access/travel/agency | access:read | engagement:read |
| GET /engagement-types | reference-data:read | engagement:read |
| GET /allocation-types | reference-data:read | engagement:read |
| GET /reference-data | reference-data:read | engagement:read |
| GET /travel-agencies | reference-data:read | engagement:read |
| GET /finance/invoice-cost-types | reference-data:read | engagement:read |
| GET /finance/subsidiaries/{company} | reference-data:read | engagement:read |
| POST /engagements/search | engagements:read | engagement:read |
| GET /engagements/facets | engagements:read | engagement:read |
| GET /engagements/{id} | engagements:read | engagement:read |
| GET /engagements/{id}/roster | engagements:read | engagement:read |
| POST /engagements | engagements:write | engagement:write |
| PATCH /engagements/{id} | engagements:write | engagement:write |
| GET /allocations/engagements/{id} | allocations:read | engagement:read |
| GET /allocations/visas | allocations:read | engagement:read |
| POST /allocations/timeline | allocations:read | engagement:read |
| POST /allocations/search | allocations:read | engagement:read |
| GET /allocations/{id} | allocations:read | engagement:read |
| GET /consultant-allocations | allocations:read | engagement:read |
| GET /consultant-allocations/non-allocated | allocations:read | engagement:read |
| GET /consultant-allocations/team-defaults | allocations:read | engagement:read |
| GET /consultant-allocations/engagement-codes | allocations:read | engagement:read |
| POST /allocations/bulk | allocations:write | engagement:write |
| PATCH /allocations/{id} | allocations:write | engagement:write |
| DELETE /allocations/{id} | allocations:write | engagement:write |
| POST /travel-requests/search | travel-requests:read | engagement:read |
| GET /travel-requests/{jobNumber} | travel-requests:read | engagement:read |
| GET /travel-requests/{jobNumber}/active-per-diems | travel-requests:read | engagement:read |
| GET /finance/travel-requests/{jobNumber} | travel-requests:read | engagement:read |
| GET /finance/travel-requests/{jobNumber}/payment-basis | travel-requests:read | engagement:read |
| PATCH /travel-requests/{jobNumber} | travel-requests:write | engagement:write |
| POST /travel-email-audit-logs | travel-requests:write | engagement:write |
| GET /visas | visas:read | engagement:read |
| GET /visas/{id} | visas:read | engagement:read |
| POST /visas | visas:write | engagement:write |
| PUT /visas/{id} | visas:write | engagement:write |
| GET /travel-documents | travel-documents:read | engagement:read |
| GET /travel-documents/{id} | travel-documents:read | engagement:read |
| POST /travel-documents | travel-documents:write | engagement:write |
| PUT /travel-documents/{id} | travel-documents:write | engagement:write |
| GET /finance/travel-requests/{jobNumber}/per-diems | per-diems:read | engagement:read |
| GET /finance/travel-requests/{jobNumber}/per-diems/{id} | per-diems:read | engagement:read |
| POST /finance/travel-requests/{jobNumber}/per-diems | per-diems:write | engagement:write |
| DELETE /finance/travel-requests/{jobNumber}/per-diems/{id} | per-diems:write | engagement:write |
| GET /finance/travel-requests/{jobNumber}/travel-cards | travel-cards:read | engagement:read |
| POST /finance/travel-requests/{jobNumber}/travel-cards | travel-cards:write | engagement:write |
| GET /finance/invoices | invoices:read | engagement:read |
| GET /finance/invoices/counts | invoices:read | engagement:read |
| GET /finance/invoices/{id} | invoices:read | engagement:read |
| GET /finance/invoices/{id}/netsuite-reference | invoices:read | engagement:read |
| POST /finance/travel-requests/{jobNumber}/invoices | invoices:write | engagement:write |
| POST /finance/invoices/decisions | invoices:approve | engagement:finance |
| POST /finance/invoices/{id}/push | invoices:push | engagement:finance |
| POST /finance/invoices/{id}/resync | invoices:push | engagement:finance |
| GET /finance/per-diem-payments | per-diem-payments:read | engagement:read |
| GET /finance/per-diem-payments/counts | per-diem-payments:read | engagement:read |
| GET /finance/per-diem-payments/{perDiemNumber} | per-diem-payments:read | engagement:read |
| GET /finance/per-diem-payments/{perDiemNumber}/netsuite-reference | per-diem-payments:read | engagement:read |
| POST /finance/travel-requests/{jobNumber}/per-diem-payments | per-diem-payments:write | engagement:write |
| PATCH /finance/per-diem-payments/{perDiemNumber} | per-diem-payments:write | engagement:finance |
| POST /finance/per-diem-payments/{perDiemNumber}/push | per-diem-payments:push | engagement:finance |
| POST /finance/per-diem-payments/{perDiemNumber}/resync | per-diem-payments:push | engagement:finance |
| GET /dashboard/allocations-today | dashboard:read | engagement:read |
| GET /dashboard/my-work | dashboard:read | engagement:read |
| GET /dashboard/queues | dashboard:read | engagement:read |
| GET /dashboard/travel | dashboard:read | engagement:read |
| GET /dashboard/engagements | dashboard:read | engagement:read |
| GET /dashboard/agency | dashboard:read | engagement:read |
| GET /role-settings/{app} | role-settings:read | engagement:admin |
| PUT /role-settings/{app}/roles/{roleId}/privileges | role-settings:write | engagement:admin |
| POST /settings/allocation-types | settings:write | engagement:write |
| PUT /settings/allocation-types/{id} | settings:write | engagement:write |
| DELETE /settings/allocation-types/{id} | settings:write | engagement:write |
| POST /audit-events | audit-events:write | engagement:admin |

A few rows need explaining:

- `PATCH /finance/per-diem-payments/{n}` edits a payment's amounts, so it
  sits under `engagement:finance`, not `write`.
- `GET /role-settings/{app}` sits under `engagement:admin`, so reading the
  privilege matrix is also an admin act.
- `POST /audit-events` sits under `engagement:admin` because a forged event
  defeats the audit trail. The backend holds `engagement:admin`, so its audit
  calls keep working.
- There is no separate `reports` collection. The backend builds its report
  downloads from the reads above (`allocations:read`, `invoices:read` and
  `per-diem-payments:read`).

## Enforcement points

1. **Choreo gateway.** On the entity endpoint, go to Operation Level Security,
   then Manage Permissions. Define every scope above as a permission and assign
   each operation **both** its fine and its coarse scope. The gateway then
   refuses a token that has neither.
2. **Entity middleware**, as defence in depth after the gateway JWT check from
   this PR:
   - It reads the scopes from the gateway JWT. The expected claim is `scope`,
     space-separated; this is an open question, see below.
   - It looks up the route (method plus `http.ServeMux` pattern) in a table
     that mirrors the one above, and passes the request when the token carries
     the fine or the coarse scope.
   - It answers **403** otherwise, and logs the method, the route pattern and
     the required scopes. It never logs token values or the path's identifiers.
   - A route missing from the table is refused (403), so a new route can't
     ship without a scope. A unit test walks the mux and fails when a route is
     unmapped.
   - It is switched on by `ENFORCE_SCOPES=true` and stays **off** until Choreo
     and Asgardeo are configured. When it's off, the middleware logs a startup
     warning and checks nothing.
3. **Engagement App Backend.** Set `OUTBOUND_SCOPES` to
   `engagement:read engagement:write engagement:finance engagement:admin`.
   This PR adds that setting. One token serves every outbound service, so the
   list must also cover any scopes other subscribed APIs enforce.
4. **Asgardeo.** An admin grants the backend's application the four coarse
   scopes. This is done outside the code.

## Rollout (staging first, then production)

1. In Choreo, define the permissions (every scope in the table) on the
   entity's Stage endpoint.
2. In Asgardeo, grant the backend's application the four coarse scopes.
3. On the backend, set `OUTBOUND_SCOPES` and redeploy.
4. Check that the backend's tokens and the gateway JWT the entity receives
   carry the scopes. Log the claim names once at DEBUG; never the token.
5. In Choreo, assign the permissions to the operations (fine plus coarse each).
   Exercise every screen and confirm there are no 403s.
6. Implement the entity middleware above, deploy it, and turn on
   `ENFORCE_SCOPES=true`.
7. Repeat steps 1–6 in production.

## Open question

Which claim does the gateway JWT carry the scopes in? `scope`
(space-separated, RFC 8693 and RFC 9068) is expected. Confirm it on staging in
step 4 before the middleware is written; a list-valued claim or a
WSO2-namespaced name would change the parser.
