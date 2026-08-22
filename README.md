# Last-Mile Delivery Tracker

## Overview

A delivery management platform where customers and admins create orders
with auto-calculated charges, delivery agents are assigned to orders
(manually or automatically), and customers are notified at every step of
the delivery journey. Built as a Go + PostgreSQL backend behind a REST API,
with a React + TypeScript frontend, structured as a modular monolith.

## Live Application

- **App**: <https://last-mile-delivery-tracker-blush.vercel.app/>
- **API**: <https://lastmile-backend-pvzk.onrender.com> (`/health` for a
  liveness check)

Frontend on Vercel, backend + PostgreSQL on Render — see
[`docs/deployment.md`](docs/deployment.md) for the full setup (why CORS
and a build-time API base URL are both required across two origins,
every environment variable, and how this deployment itself was
verified). Real email notifications are live via
[Resend](https://resend.com) (see "Notification Service (M11)" below).
Demo credentials are in "Authentication" below.

The backend's free-tier instance spins down after inactivity — the
first request after a quiet period can take 30–60 seconds to wake it
back up.

## Current Status

**Post-M12 hardening.** A full audit after M12 (see "Evaluation Matrix"
below) identified the gap between what M01–M12 build and what the
assignment's own deliverables require: a hosted application URL, and a
closer reading of "email notifications are sent" against the
assignment's own "free-tier service" wording. Addressed:

- **CORS** (`internal/server/cors.go`) — a small, hand-rolled middleware
  (no new dependency), wrapped around the router only in
  `cmd/server/main.go`, off by default (`CORS_ALLOWED_ORIGINS` unset =
  no header added, today's behavior unchanged). Required the moment
  frontend and backend are hosted on different origins — verified live
  against a running container, both the allow-path and the deny-path.
- **Real email, opt-in** — `notifications.ResendEmailProvider`
  (`internal/notifications/resend.go`) sends real email via
  [Resend](https://resend.com)'s free tier, behind the exact same
  `EmailProvider` interface `LogEmailProvider` already satisfies.
  `EMAIL_PROVIDER` defaults to `log` (zero credentials, zero external
  calls, unchanged from M11); set it to `resend` plus `RESEND_API_KEY`/
  `RESEND_FROM_EMAIL` to send real mail — the backend fails fast at
  startup if those are missing while `resend` is selected.
- **Real SMS, opt-in** — `notifications.TwilioSmsProvider`
  (`internal/notifications/twilio.go`) sends real SMS via
  [Twilio](https://www.twilio.com)'s free trial, behind the exact same
  `SmsProvider` interface `LogSmsProvider` already satisfies — the
  identical pattern as real email, one provider swap, zero change to
  `internal/notifications`' dispatch/idempotency logic. `SMS_PROVIDER`
  defaults to `log`; set it to `twilio` plus `TWILIO_ACCOUNT_SID`/
  `TWILIO_AUTH_TOKEN`/`TWILIO_FROM_NUMBER` to send real texts — same
  fail-fast-at-startup guarantee if those are missing. A Twilio trial
  account can only text phone numbers verified in the Twilio console;
  delivery to Indian numbers additionally requires DLT registration
  under Indian telecom regulation (verified directly against the real
  Twilio API — the request is correctly built and authenticated, and
  rejected by Twilio's own regional policy, not by this code) — see
  `docs/notifications.md` for the full, honestly-documented limitation.
- **A configurable frontend API base URL** (`VITE_API_BASE_URL`,
  `frontend/services/api.ts`) — the one change deployment actually
  required: every request in the frontend already funneled through one
  function, so this is a few-line, single-file change, not a rework.
  Empty by default (today's relative-path/dev-proxy behavior,
  unchanged).
- **CI** (`.github/workflows/ci.yml`) — backend `gofmt`/`go vet`/
  `go build`/`go test` (unit, integration, e2e, against a real Postgres
  service container) and frontend `tsc`/`oxlint`/`vitest`/`vite build`,
  on every push/PR to `main`.
- **An automated OpenAPI-contract test**
  (`TestOpenAPIContract_DocumentedPathsMatchRealRoutes`) — walks the
  real, fully-mounted router via chi's own route introspection and
  diffs it against `docs/openapi.yaml`, so the document and the router
  can never silently drift apart again.
- **Three supporting indexes** (migration `0013`) on
  `orders.status`/`pickup_zone_id`/`drop_zone_id` — the two columns
  `GET /orders`'s M12 filter can now query that didn't already have one
  (`customer_id` and `assigned_agent_id` already did, from M07/M09).
- **`docs/deployment.md`** — the concrete deployment guide tying CORS,
  the base-URL config, and both platforms' required environment
  variables together, plus how to verify a live deployment.
- **`PUT /api/v1/agents/{id}/zone`** — a real functional gap, not a
  polish item: `current_zone_id` (the column M09's
  `assignment.IsEligible` requires to be non-nil) had *no application
  write path at all* until this endpoint, so no real delivery agent
  using the app could ever become eligible for auto-assignment. A plain
  dropdown of real zones (`GET /zones`, read-RBAC widened to
  `DELIVERY_AGENT`), not GPS-derived — there is no zone/area boundary
  geometry anywhere in this schema, and no order pickup coordinate
  either, so true geofencing was never computable here (see
  `docs/assignment-engine.md`). `internal/assignment/candidate.go`
  (M09, frozen) is untouched.

None of this touched `internal/tracking`, `internal/assignment/candidate.go`,
`internal/rescheduling`, or `internal/notifications`' own dispatch/
idempotency logic — see the Evaluation Matrix for the full evidence
trail.

**M12 — Dashboards & Evaluation Layer.** A role-specific dashboard
landing page for each of `CUSTOMER` (`/customer/dashboard`),
`DELIVERY_AGENT` (`/agent/dashboard`), and `ADMIN` (`/admin/dashboard`)
— a thin navigation/composition layer over pages M01–M11 already built,
introducing no new backend module and no new database table. Two real
gaps were closed. First: `GET /api/v1/orders` gained optional
`?status=&zone=&agent=` query parameters, honored for `ADMIN` only
(silently ignored for `CUSTOMER`/`DELIVERY_AGENT`, who can never have
their view widened by a filter) — a source-required capability M07
explicitly deferred; a zero-value filter is byte-identical to the
pre-M12 unfiltered call, so every existing caller is unaffected. Second,
and more consequential: a `DELIVERY_AGENT` previously had **no frontend
path at all** to update an order's status, despite the backend
authorizing five transition edges for that role since M08 —
`OrderDetailPage`'s status-update section rendered only for `isAdmin`.
M12 widens that render condition and adds a frontend mirror of the
backend's per-edge role authorization (`transitionsForRole`), touching
zero lines of `internal/tracking/statemachine.go`; fixing this also
surfaced and fixed a real latent bug, where the same transition handler
unconditionally refetched the tracking timeline afterward — an endpoint
`DELIVERY_AGENT` was never authorized to call. The admin dashboard adds
a simple order-statistics widget (counts by status, computed from real,
unfiltered order data, never hardcoded). Also added: a static,
hand-authored OpenAPI 3.0 document (`docs/openapi.yaml`) mirroring every
real endpoint; the required ≤ 800-word `docs/system-design.md`;
and `backend/tests/e2e`'s first real full-stack HTTP flow tests
(register → quote → order → assign → agent status updates → delivered,
and the failed → reschedule → reassign → second-lifecycle variant),
replacing the placeholder that had sat empty since M01. **No charts,
maps, search, pagination, real-time updates, or notification UI were
added** — all explicit, approved out-of-scope decisions; see
`docs/dashboards.md`.

**M11 — Notification Service.** The customer is now notified — by
email always, and by SMS when a phone number is on file — for all
eight order-lifecycle events (`ORDER_CREATED`, `AGENT_ASSIGNED`,
`PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, `FAILED`,
`RESCHEDULED`). This is entirely a post-commit side effect of the
endpoints M07–M10 already expose — no new endpoint, and no line of
M08's transition logic, M09's ranking, or M10's reschedule
authorization was changed. `internal/notifications` depends on
`orders`/`users`/`tracking` to resolve a customer's contact details,
but nothing in those packages depends back on it: `tracking` and
`orders` each export a tiny, nil-safe callback type of their own
(`tracking.TransitionHook`, `orders.OrderCreatedHook`), and
`cmd/server/main.go` is the only place that wires the real
`notifications.Service` into them. Idempotency is anchored on the
*exact* `order_tracking_events.id` — never `(order_id, event,
channel)` — because an order can legitimately produce the same event
type more than once (a second `FAILED` after a
`FAILED→RESCHEDULED→ASSIGNED→...` cycle is a distinct, independently
notify-able occurrence); a `Repository.Claim`
(`INSERT ... ON CONFLICT (tracking_event_id, channel) DO NOTHING`)
executed *before* any provider call, backed by a real unique index, is
what actually prevents a duplicate send under concurrent load, proven
with real goroutines against real Postgres. `EmailProvider`/
`SmsProvider` are two narrow interfaces; the only implementations that
ship are log-based (`LogEmailProvider`/`LogSmsProvider`) — no external
account, credential, or SDK dependency of any kind, by design for this
MVP. A provider failure is caught, logged, and recorded as that one
notification's own `FAILED` status; it can never roll back or fail the
order/tracking/assignment/reschedule operation that triggered it.
**M11 adds no REST API and no frontend UI** — both are explicit,
approved scope decisions, not omissions; see `docs/notifications.md`.

**M10 — Failed Delivery & Rescheduling.** A `FAILED` order can now be
rescheduled: `POST /api/v1/orders/:id/reschedule` (`CUSTOMER`, own order
only, or `ADMIN`, any order — body `{"requested_date": "YYYY-MM-DD",
"reason": "optional"}`) and `GET /api/v1/orders/:id/reschedules` (the
same two roles). This resolves a real architectural tension without
touching M08: the product requirement is "a customer can reschedule,"
but M08's own, unmodified authorization matrix permits only `ADMIN` on
the underlying `FAILED → RESCHEDULED` edge. M10's own handler
authorizes the call independently (ownership or admin), then invokes
M08's `TransitionTx` asserting its own authority — while still recording
the *real* caller's user id as `actor_id`, never a role name, so the
permanent tracking record stays honest about who actually asked. Zero
lines changed in `internal/tracking`. A dedicated `reschedule_requests`
table (`requested_date`, `reason`, `requested_by`, `created_at` — no
approval workflow, no status column: a reschedule is immediately
effective) persists each request alongside the paired tracking event's
own metadata. The delivery agent servicing a failed order is freed back
to `AVAILABLE` as part of the reschedule transaction (agent-then-order
lock ordering, reused from M09, so the two modules can't deadlock);
`assigned_agent_id` itself is deliberately left unchanged, a historical
snapshot until a later, *separate* `POST /orders/:id/assign`/
`auto-assign` call overwrites it — M10 never calls into M09's own
ranking code. No `delivery_attempts` table: failure reason travels in
the existing `FAILED` event's own metadata (already-supported, unmodified
M08 behavior), and "attempt number" is derived by counting `→ASSIGNED`
transitions in an order's own tracking history rather than stored as a
mutable counter. The frontend adds a "Reschedule delivery" control
(native `<input type="date">`, no new dependency) and a reschedule-history
section to the order detail page, visible to `CUSTOMER`/`ADMIN` only.

| Module | Status |
|---|---|
| M01 — Foundation & Infrastructure | ✅ Done |
| M02 — Authentication & RBAC | ✅ Done |
| M03 — User & Agent Management | ✅ Done |
| M04 — Zone Management | ✅ Done |
| M05 — Rate Configuration | ✅ Done |
| M06 — Rate Calculation Engine | ✅ Done |
| M07 — Order Management | ✅ Done |
| M08 — Tracking & Order Lifecycle | ✅ Done |
| M09 — Assignment Engine | ✅ Done |
| M10 — Failed Delivery & Rescheduling | ✅ Done |
| M11 — Notification Service | ✅ Done |
| M12 — Dashboards & Evaluation Layer | ✅ Done |

## Tech Stack

Everything below is an **engineering decision** made for this project —
none of it is mandated by the assignment, which only requires a backend
API, a frontend, a database, and role-based authentication. Where the
assignment does require something specific (e.g. the volumetric weight
formula, the five delivery statuses), that requirement is documented in
`docs/` as the relevant module lands.

| Layer | Choice |
|---|---|
| Frontend | React, TypeScript, Vite, Tailwind CSS |
| Backend | Go, [chi](https://github.com/go-chi/chi) router, REST API, OpenAPI docs (added when the first real endpoints land) |
| Database | PostgreSQL, [pgx](https://github.com/jackc/pgx), SQL migrations |
| Auth | JWT ([golang-jwt/jwt](https://github.com/golang-jwt/jwt)), bcrypt ([golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto/bcrypt)) |
| Infrastructure | Docker, Docker Compose |
| Testing | Go `testing` + `httptest`, integration tests, full-stack flow tests |
| Architecture | Modular monolith — twelve modules in one deployable backend, not microservices |

## Prerequisites

- Go 1.24+
- Node.js 20+ and npm
- Docker and Docker Compose

## Local Setup

```bash
git clone <repository-url>
cd last-mile-delivery-tracker
cp .env.example .env        # then edit DB_PASSWORD and JWT_SECRET to real local values
docker compose up -d        # starts PostgreSQL + the backend
cd frontend && npm install && npm run dev
```

This is honestly a **two-command** setup, not one: `docker compose up`
brings up PostgreSQL and the backend; the frontend is run separately with
`npm run dev`. It isn't in Compose because a dockerized Vite dev server
needs extra volume-mount and file-watching configuration to support hot
reload reliably — complexity not worth adding while the frontend is still
this small.

The backend applies its database migrations and seeds three demo accounts
(one per role) automatically on startup — no separate migration or seed
command to run. See [Authentication](#authentication) below for the demo
credentials.

Once both are running:

- Backend health check: http://localhost:8080/health
- Frontend: http://localhost:5173

## Environment Configuration

All configuration is read from environment variables — nothing is
hard-coded, and startup fails with a clear error listing exactly which
variables are missing. Copy `.env.example` to `.env` and adjust as needed;
`.env` is gitignored and must never be committed.

| Variable | Purpose | Default |
|---|---|---|
| `APP_ENV` | Application environment label | `development` |
| `SERVER_HOST` | Host the backend binds to | `0.0.0.0` |
| `SERVER_PORT` | Port the backend listens on | `8080` |
| `DB_HOST` | PostgreSQL host | *(required)* |
| `DB_PORT` | PostgreSQL port | *(required)* |
| `DB_NAME` | PostgreSQL database name | *(required)* |
| `DB_USER` | PostgreSQL user | *(required)* |
| `DB_PASSWORD` | PostgreSQL password | *(required)* |
| `DB_SSLMODE` | PostgreSQL SSL mode | `disable` |
| `JWT_SECRET` | Signs and verifies JWTs. Use a long, random value outside your own machine. | *(required)* |

## Running PostgreSQL

```bash
docker compose up -d postgres
```

Runs PostgreSQL 16 with a persistent named volume (`postgres_data`) and a
`pg_isready` health check that `docker compose up` waits on before
starting the backend.

## Running Backend

Via Docker Compose (recommended — matches how it will actually run):

```bash
docker compose up -d backend
```

Directly on the host, against the dockerized database (useful for fast
iteration without rebuilding the image each time):

```bash
cd backend
go run ./cmd/server
```

Either way, on startup the backend applies any unapplied SQL migrations
from `backend/migrations/` (tracked in a `schema_migrations` table, so this
is safe to do on every start) and seeds the three demo accounts if they
don't already exist. There is no separate `migrate` command.

## Running Frontend

```bash
cd frontend
npm install
npm run dev
```

The dev server proxies `/api/*` requests to `http://localhost:8080`
(configured in `vite.config.ts`), so the frontend never needs to know the
backend's origin and the backend needs no CORS configuration in
development.

## Running Tests

`.github/workflows/ci.yml` runs every command below automatically on
every push/PR to `main` (backend against a real Postgres service
container, frontend on Node 24) — see `docs/deployment.md`'s own CI
section.

**Backend:**

```bash
cd backend

# Unit tests — no external dependencies required
go test ./...

# Integration tests — requires a running PostgreSQL instance
docker compose up -d postgres
TEST_DATABASE_URL="postgres://lastmile:<your-password>@localhost:5432/lastmile?sslmode=disable" \
  go test -tags=integration ./tests/integration/...

# End-to-end tests (M12) — same database, full-stack HTTP flows through
# the real router, register through delivered/rescheduled
TEST_DATABASE_URL="postgres://lastmile:<your-password>@localhost:5432/lastmile?sslmode=disable" \
  go test -tags=e2e ./tests/e2e/...
```

**Frontend:**

```bash
cd frontend
npm test        # vitest, single run
npm run lint     # oxlint
npm run build    # type-check (tsc -b) + production build
```

Frontend tests render real components (login/auth state transitions,
protected-route redirects) via `@testing-library/react` on a `jsdom`
environment — not just pure-function tests — reflecting that M02
introduces real stateful UI behavior worth asserting on.

## Authentication

Three roles: `CUSTOMER`, `DELIVERY_AGENT`, `ADMIN`. Full design detail
(schema, JWT claims, RBAC middleware, token-storage tradeoff) is in
[`docs/authentication.md`](docs/authentication.md); this is the short
version.

- **Registration** (`POST /api/v1/auth/register`) always creates a
  `CUSTOMER` account — there is no way to request another role through
  this endpoint. A request containing a `role` field is rejected outright
  (422), not silently downgraded.
- **Login** (`POST /api/v1/auth/login`) returns a JWT on success, or a
  generic "invalid email or password" on failure — the same message
  whether the email doesn't exist or the password is wrong.
- **JWT**: HS256, signed with `JWT_SECRET`, 24-hour expiration, containing
  only user ID, role, issued-at, and expiration. No refresh tokens.
- **RBAC**: `RequireAuth` + `RequireRole` middleware in `internal/auth`,
  composed via chi's `.With(...)`. Centralized — handlers never check
  `user.Role` themselves.
- **`GET /api/v1/users/me`** (not `/auth/me`) returns the authenticated
  caller's own profile. Requires a valid bearer token.
- **`PUT /api/v1/users/me`** (M03) updates `full_name`/`phone` only — the
  request has no `id`/`email`/`role`/`password_hash` field at all, so
  role-tampering through this endpoint is structurally impossible, not
  just filtered out.

**Demo accounts** (seeded automatically on backend startup — see
[Local Setup](#local-setup)):

| Email | Password | Role |
|---|---|---|
| `admin@lastmile.test` | `Admin123!` | ADMIN |
| `agent@lastmile.test` | `Agent123!` | DELIVERY_AGENT |
| `customer@lastmile.test` | `Customer123!` | CUSTOMER |

These are intentionally public demo-only credentials, not production
secrets — the point is that an evaluator can log in as any role without a
real account-provisioning flow existing yet (that's M03's `POST /agents`
for agents; admin accounts are never created through any HTTP endpoint at
all).

## Delivery Agents

Full design detail (schema, the `active`/`availability` distinction,
transactional provisioning, IDOR protection, M09 forward-compatibility) is
in [`docs/user-agent-management.md`](docs/user-agent-management.md);
endpoint examples are in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/agents`** (ADMIN only) provisions a `DELIVERY_AGENT` —
  creates the linked `users` row and `delivery_agents` row in one
  transaction. No `role` field on the request, same server-controlled
  pattern as registration.
- **`GET /api/v1/agents`** (ADMIN only) lists every agent with their
  operational state. No filters — not an oversight, out of M03's scope.
- **`GET /api/v1/agents/me`** (DELIVERY_AGENT only) is an agent's
  self-lookup, the same role `GET /users/me` plays for identity.
- **`PUT /api/v1/agents/{id}/availability`** (ADMIN or the agent
  themselves) sets `AVAILABLE`/`BUSY`/`OFFLINE`.
- **`PUT /api/v1/agents/{id}/location`** (the agent themselves only, not
  even ADMIN) sets current latitude/longitude — validated (±90/±180,
  finite), with `location_updated_at` always set from the database's own
  clock, never a client-supplied timestamp.
- **`PUT /api/v1/agents/{id}/zone`** (the agent themselves only) sets
  `current_zone_id` — the column M09's `assignment.IsEligible` requires
  to be non-nil. Until this endpoint existed, nothing in the application
  ever wrote to it (only direct SQL, in tests and seed data), so no real
  agent using the app could ever become eligible for auto-assignment.
  Validates the zone is real and active via `zones.Repository` before
  writing (`422` otherwise) — a plain dropdown, not a location derived
  from lat/lng, since there's no zone-boundary geometry anywhere in this
  schema to geofence against. See `docs/user-agent-management.md`.

Every agent-management write is protected twice: a route-level role gate,
then a handler-level ownership check comparing the caller's own user ID
against the target agent's — the second layer exists because two agents
share the identical `DELIVERY_AGENT` role, so a role check alone cannot
stop Agent A from editing Agent B's data. See
`docs/user-agent-management.md` for the full reasoning and the tests that
verify it (`TestUpdateAvailabilityHandler_AnotherAgentForbidden`,
`TestUpdateLocationHandler_AnotherAgentForbidden`, and the integration-level
`TestCreateAgent_DeliveryAgentCannotCreateAnotherAgent`).

## Zones

Full design detail (hierarchy, schema, why there's no DELETE, address
resolution and why it isn't geocoding, INTRA/INTER, inactive-zone
behavior, completing the `current_zone_id` FK) is in
[`docs/zone-management.md`](docs/zone-management.md); endpoint examples
are in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/zones`**, **`GET /api/v1/zones`**, **`GET
  /api/v1/zones/{id}`**, **`PUT /api/v1/zones/{id}`** (all ADMIN only) —
  zone CRUD. `PUT` is also how a zone is activated/deactivated; there is
  no separate endpoint.
- **`POST /api/v1/zones/{zoneID}/areas`**, **`GET
  /api/v1/zones/{zoneID}/areas`**, **`PUT
  /api/v1/zones/{zoneID}/areas/{areaID}`** (all ADMIN only) — areas are
  always created and addressed under their zone's URL; the request body
  has no `zone_id` field, so a client cannot redirect an area to a
  different zone than the path names.
- **Deletion**: neither zones nor areas can be deleted — only zones can
  be deactivated (`active = false`). Zones and areas are configuration
  later modules will reference; physical deletion risks orphaning those
  references.
- **Resolution**: `internal/zones.ResolvePickupDrop` resolves a
  pickup/drop area pair to their zones and classifies the pair as
  `INTRA` (same zone) or `INTER` (different zones) — comparing zone IDs,
  never names. No HTTP endpoint exposes this in M04; it's a Go-level
  service M06's `CalculateQuote` and M07's order creation both call
  directly. Resolving an unknown area fails explicitly; resolving
  through a deactivated zone fails explicitly too (`ErrZoneInactive`) —
  the resolver never guesses a zone or silently returns `INTRA`.
- **`delivery_agents.current_zone_id`** now has a real foreign key to
  `zones.id` — the constraint M03's own migration comment said would
  land in M04.

## Rate Cards

Full design detail (schema, why cards start inactive, flat-per-band
pricing, the `[min, max)` boundary convention, why slabs can be deleted
but cards can't, concurrency mechanisms) is in
[`docs/rate-configuration.md`](docs/rate-configuration.md); endpoint
examples are in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/rates`**, **`GET /api/v1/rates`**, **`GET
  /api/v1/rates/{id}`**, **`PUT /api/v1/rates/{id}`** (all ADMIN only) —
  rate card CRUD. New cards always start `active: false`; `PUT` is the
  only way to activate one (and the only way to deactivate one — there
  is no `DELETE /rates/{id}`, deliberately, mirroring M04's zones).
- **`POST /api/v1/rates/{rateCardID}/slabs`**, **`GET
  /api/v1/rates/{rateCardID}/slabs`**, **`PUT
  /api/v1/rates/{rateCardID}/slabs/{slabID}`**, **`DELETE
  /api/v1/rates/{rateCardID}/slabs/{slabID}`** (all ADMIN only) — slabs
  are always created and addressed under their rate card's URL; the
  request body has no `rate_card_id` field. Unlike every other config
  entity in this project, slabs **can** be deleted — nothing else
  references an individual slab by foreign key, so removing a
  misconfigured one can't orphan anything.
- **Pricing model**: each slab is a flat price for its whole
  `[min_weight, max_weight)` band, not a per-kg rate. `max_weight = null`
  means open-ended ("and above"); at most one such slab is allowed per
  card.
- **One active card per combination**: enforced by a partial unique
  index (`(order_type, zone_relationship) WHERE active`), not just
  application code — this is what makes M06's "select the rate card"
  step (`FindActiveCard`) deterministic.
- **Concurrency, proven under real load, not just asserted**: activating
  two cards for the same combination at once is resolved by the unique
  index itself (`TestConcurrentActivation_OnlyOneWins` fires simultaneous
  requests and checks exactly one wins); concurrent slab creation under
  one rate card is serialized by a `SELECT ... FOR UPDATE` lock on the
  parent card (`TestConcurrentSlabCreation_OverlapPreventedUnderRace`),
  closing a check-then-insert race a plain read-then-write would leave
  open. Both tests run against real PostgreSQL with actual concurrent
  goroutines, not sequential calls.

## Rate Calculation (M06)

Full design detail (weight formula, unit assumptions, the `[min, max)`
slab-selection algorithm and every boundary case, why nothing is
persisted, the one M04 RBAC change this required) is in
[`docs/rate-calculation.md`](docs/rate-calculation.md); endpoint
reference is in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/orders/quote`** (ADMIN or CUSTOMER; DELIVERY_AGENT →
  403) — the one authoritative pricing path. Resolves pickup/drop areas
  via M04's `zones.ResolvePickupDrop`, selects the active rate card via
  M05's `rates.FindActiveCard`, computes `volumetric = L×B×H÷5000`,
  `chargeable = max(actual, volumetric)`, picks the matching slab, and
  adds the COD surcharge only when `payment_type = COD`. Nothing is
  persisted — calling this again always recomputes fresh, which is what
  actually makes "never trust a client-supplied price" true rather than
  just documented.
- **No new database table.** M06 is pure calculation over M04/M05's
  existing schema — extending `internal/rates` exactly as its own
  package doc committed to since M05, not a new package.
- **Mass-assignment protection**: the request has no field for anything
  server-derived (`customer_id`, `pickup_zone_id`, `drop_zone_id`,
  `zone_relationship`, `rate_card_id`, `volumetric_weight`,
  `chargeable_weight`, `base_rate`, `cod_surcharge`, `final_amount`,
  `status`) — sending one is rejected outright (422, unknown field).
- **One narrow M04 change**: `GET /zones`, `GET /zones/{id}`, and `GET
  /zones/{zoneID}/areas` now also admit `CUSTOMER` (previously
  ADMIN-only), so a customer can pick a real pickup/drop area for a
  quote. No mutation route changed.
- **Order creation (M07) reuses this exact function** — `CalculateQuote`
  — rather than a second implementation; see below.

## Order Management (M07)

Full design detail (customer-vs-admin creation, ownership, the pricing
snapshot, why there's no `packages` table, why `rate_card_id` is safe on
an order but `slab_id` isn't) is in
[`docs/order-management.md`](docs/order-management.md); endpoint
reference is in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/orders`** (ADMIN or CUSTOMER; DELIVERY_AGENT → 403) —
  builds a `rates.QuoteInput` from the request and calls
  `rates.CalculateQuote` — M06's exact, unmodified pricing path — then
  persists the returned `QuoteResult` as an immutable snapshot.
  `status` is always `CREATED`; no other value is ever written here.
- **Customer vs. admin request shapes are two different Go structs, not
  one shared struct with an optional field.** A `CUSTOMER`'s DTO has no
  `customer_id` field at all — sending one is a `422` unknown-field
  rejection, which is what makes "a customer cannot create an order for
  another customer" true by construction. An `ADMIN`'s DTO requires
  `customer_id`, validated to reference a real user with role
  `CUSTOMER` (an `ADMIN`/`DELIVERY_AGENT` target is rejected).
  `created_by` is always the caller's own JWT identity on both paths.
- **`GET /api/v1/orders`** — every order for `ADMIN`, only the caller's
  own for `CUSTOMER`. **`GET /api/v1/orders/{id}`** — any order for
  `ADMIN`; a `CUSTOMER` requesting another customer's order gets `404`,
  never `403`, the same ownership convention M04/M05 established.
- **Pricing snapshot, not a live reference**: `base_rate`,
  `cod_surcharge`, `final_amount`, `zone_relationship`, and the resolved
  weights are written once from `QuoteResult` and never recomputed — a
  later edit to the rate card that priced an order never changes that
  order's stored amount. `rate_card_id` is stored (rate cards are
  deactivate-only, never deleted); `slab_id` deliberately is not (M05
  allows slab deletion, and nothing else may reference one by FK).
- **One new table**: `orders` (migration `0008`), 20 columns, 7 foreign
  keys, 8 CHECK constraints matching the full M08 status enum
  (schema-only at the time — M07 itself only ever wrote `CREATED`). No
  `packages` table — dimensions are inline columns on `orders`.
- **Updated by M08**: `CreateOrder` now runs inside a transaction — not
  for `packages` (still doesn't exist), but because order creation must
  also atomically write its own opening tracking event (see below).
- **Out of scope, by design**: order filtering by status/zone/agent,
  order editing/cancellation — later modules. Status transitions
  themselves are M08, immediately below.

## Order Tracking (M08)

Full design detail (the state machine, the per-edge authorization
matrix, the concurrency proof, why order creation writes the first
tracking event) is in
[`docs/order-tracking.md`](docs/order-tracking.md); endpoint reference
is in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/orders/:id/status`** (ADMIN or DELIVERY_AGENT;
  CUSTOMER → 403) — validates one transition against a closed 8-edge
  table (`CREATED→ASSIGNED→PICKED_UP→IN_TRANSIT→OUT_FOR_DELIVERY→
  {DELIVERED|FAILED→RESCHEDULED→ASSIGNED}`, `DELIVERED` terminal),
  checks the caller's role is authorized for that *specific* edge (not
  just the route), updates `orders.status`, and inserts an immutable
  `order_tracking_events` row — all inside one transaction, under a
  `SELECT ... FOR UPDATE` lock on the order.
- **Authorization is per-edge, not per-role-per-route**: ADMIN is
  authorized on all 8 edges; DELIVERY_AGENT only on the 5 agent-tier
  ones (`ASSIGNED→PICKED_UP` through `OUT_FOR_DELIVERY→{DELIVERED,
  FAILED}`); CUSTOMER on none. A DELIVERY_AGENT attempting
  `CREATED→ASSIGNED` or `FAILED→RESCHEDULED` gets `403` even though the
  route itself admits their role.
- **Resolved by M09, deliberately not here**: `orders.assigned_agent_id`
  now exists, but this endpoint's own authorization was intentionally
  left unmodified — any authenticated DELIVERY_AGENT can still perform
  an agent-tier edge on any order via this endpoint directly. What M09
  actually scoped is `GET /orders`/`GET /orders/:id` (an agent's own
  frontend view), not this transition endpoint's authorization.
- **`GET /api/v1/orders/:id/tracking`** (ADMIN any order; CUSTOMER own
  only, `404` otherwise; DELIVERY_AGENT → 403, unchanged by M09) — the full,
  chronological event history. The first entry always has
  `previous_status: null` — order creation itself is the first
  tracking event, written atomically by `internal/orders.CreateOrder`.
- **Concurrency, proven under real load**: two conflicting transitions
  fired simultaneously at the same order (`OUT_FOR_DELIVERY→DELIVERED`
  vs. `OUT_FOR_DELIVERY→FAILED`) — exactly one commits, the other gets
  `409`, verified by `TestConcurrentTransition_OnlyOneWins` (real
  goroutines, real Postgres) and by firing two real, simultaneous curl
  requests manually.
- **One new table, no `orders` schema change**: `order_tracking_events`
  (migration `0009`) — `orders.status`'s CHECK constraint already
  covered the full M08 value set since M07, so no `ALTER` was needed.
  No `slab_id`. Genuinely append-only — no update/delete route exists
  for this table at all.
- **Out of scope, by design**: agent assignment/auto-assignment (now
  M09, below), reschedule-date capture, notifications, status/zone
  filtering.

## Assignment Engine (M09)

Full design detail (the eligibility rule, the deterministic ranking
algorithm and why no geographic-distance ranking exists, why M08's
state machine is reused rather than duplicated, the four concurrency
races and their protection) is in
[`docs/assignment-engine.md`](docs/assignment-engine.md); endpoint
reference is in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/orders/:id/assign`** (ADMIN only) — manually assign
  one named agent (`{"agent_id": "<uuid>"}`). Locks the agent row,
  re-checks eligibility under lock, transitions the order to `ASSIGNED`
  via M08's own `TransitionTx` (never reimplemented), writes
  `assigned_agent_id`, marks the agent `BUSY` — all in one transaction.
- **`POST /api/v1/orders/:id/auto-assign`** (ADMIN only, no body) —
  ranks every eligible agent (active, `AVAILABLE`, a usable
  `current_zone_id`) by same-zone-first, then `last_assigned_at`
  ascending (`NULL` first), then agent UUID as the final tiebreak;
  locks the winner and re-checks eligibility, retrying the next-ranked
  candidate if it was raced away since the bulk read. **No geographic-
  distance ranking** — no coordinate data exists anywhere in the schema
  for orders or zones to rank against.
- **Eligibility is one rule, shared by both paths**: `active = true`,
  `availability = AVAILABLE`, a resolvable `current_zone_id` — an admin
  cannot deliberately assign a `BUSY`/`OFFLINE`/inactive agent any more
  than auto-assignment could pick one.
- **No status-preserving reassignment**: M08 has no `ASSIGNED→ASSIGNED`
  edge, and this module doesn't add one — the only reassignment path is
  the existing `FAILED→RESCHEDULED→ASSIGNED` cycle, which both endpoints
  already support since it's just another `→ASSIGNED` transition.
- **Concurrency, proven under real load**: two admins racing the same
  order, manual racing auto-assign, two orders racing the same agent,
  and assignment racing an unrelated M08 transition — all closed by
  consistent agent-then-order lock ordering plus a partial unique index
  backstop (`idx_orders_one_active_assignment_per_agent`: at most one
  active — `ASSIGNED`/`PICKED_UP`/`IN_TRANSIT`/`OUT_FOR_DELIVERY` —
  assignment per agent). Verified with real concurrent goroutines
  against real Postgres, repeated (`-count=5`) to rule out flakiness.
- **One new column, no new table**: `orders.assigned_agent_id`
  (migration `0010`) plus two indexes. Assignment history lives entirely
  in M08's own `order_tracking_events.metadata` — each `ASSIGNED` event
  now carries `{"assigned_agent_id": "..."}`.
- **`GET /orders`/`GET /orders/:id` widen to admit DELIVERY_AGENT**,
  scoped strictly to their own assigned orders (never every customer's
  order); `GET /orders/:id/tracking` stays ADMIN/CUSTOMER-only,
  deliberately unchanged.
- **Frontend**: `OrdersPage` shows "My assigned orders" for
  DELIVERY_AGENT (no "New order" action); `OrderDetailPage` shows the
  assigned agent and, for ADMIN on an assignable order, manual (agent
  picker, reusing M03's `GET /agents`) and auto-assign controls with
  success/error states.
- **Out of scope, by design**: reschedule-request handling (now M10,
  below), notifications, dashboards, geographic-distance infrastructure,
  an assignment-history table, a candidate-preview endpoint, and any
  change to M08's own state machine or authorization.

## Failed Delivery & Rescheduling (M10)

Full design detail (the architectural mismatch between "customer can
reschedule" and M08's ADMIN-only transition matrix and how it's
resolved without modifying M08, the `reschedule_requests` schema,
agent-availability behavior after a failure, why no `delivery_attempts`
table exists) is in
[`docs/failed-delivery.md`](docs/failed-delivery.md); endpoint reference
is in [`docs/api.md`](docs/api.md). Short version:

- **`POST /api/v1/orders/:id/reschedule`** (CUSTOMER, own order only, or
  ADMIN, any order — DELIVERY_AGENT `403`) — reschedule a `FAILED` order.
  Body: `{"requested_date": "YYYY-MM-DD", "reason": "optional"}`.
  Returns the updated order (`status: RESCHEDULED`).
- **The architectural mismatch, resolved additively**: M08's own,
  unmodified authorization matrix permits only ADMIN on
  `FAILED→RESCHEDULED`, but the product requirement is customer-
  initiated rescheduling. `RescheduleHandler` is M10's own,
  independent authorization gate (ownership or admin, checked before
  M08 is ever called); `Repository.Reschedule` then calls M08's
  `TransitionTx` asserting `RoleAdmin` — a value TransitionTx's own
  check consumes but never persists — while passing the *real* caller's
  user id as the separate `actorID` parameter, which *is* what lands in
  `order_tracking_events.actor_id`. Authorization and identity are two
  parameters, not one; zero lines changed in `internal/tracking`.
- **Reschedule persistence**: a dedicated `reschedule_requests` table
  (migration `0011`) — `order_id`, `requested_by`, `requested_date`,
  `reason`, `created_at`. Deliberately no status/approval columns: a
  reschedule is immediately effective, there is no pending/approved/
  rejected workflow. The same `requested_date`/`reason` also land in the
  paired `RESCHEDULED` tracking event's own metadata.
- **Failure reason**: no new mechanism needed — M08's own
  `transitionRequest.metadata` already accepted arbitrary context on
  every transition, including `OUT_FOR_DELIVERY→FAILED`, before M10
  existed. `{"status":"FAILED","metadata":{"failure_reason":"..."}}`
  against the existing, unmodified `POST /orders/:id/status` already
  worked; M10 verified, tested, and documented it rather than building
  something new.
- **Agent availability after failure**: the previously assigned agent is
  freed to `AVAILABLE` as part of M10's own reschedule transaction (not
  at the instant `FAILED` is set — that transition still goes through
  M08's own endpoint, which this module doesn't hook or wrap). Lock
  ordering matches M09 exactly (agent row first, then the order row via
  `TransitionTx`), which is what rules out a cross-module deadlock.
  `assigned_agent_id` itself is deliberately never cleared — a
  historical snapshot until a later, separate M09 assignment call
  overwrites it.
- **Reassignment stays M09's job**: M10 never calls
  `assignment.Repository` and never ranks candidates. After a
  reschedule, the order simply sits at `RESCHEDULED`, exactly the status
  `RESCHEDULED→ASSIGNED` (M09, unmodified) already knew how to consume —
  a *separate*, subsequent `POST /orders/:id/assign`/`auto-assign` call,
  the same two-step pattern `CREATED`→(separate)→`ASSIGNED` already
  uses. A freed agent is reconsidered on exactly the same eligibility/
  ranking terms as any other agent — no special-casing.
- **No `delivery_attempts` table**: failure reason lives in the
  existing `FAILED` event's metadata; "attempt number" is derived by
  counting `→ASSIGNED` transitions in an order's own tracking history at
  read time, never a stored, mutable counter.
- **Concurrency, proven under real load**: two reschedule requests
  racing the same `FAILED` order, and a reschedule racing an unrelated
  M09 assignment attempt for consistent lock-ordering deadlock safety —
  both closed by the same `SELECT ... FOR UPDATE` + transaction pattern
  every prior module uses, verified with real concurrent goroutines,
  repeated (`-count=5`).
- **Frontend**: `OrderDetailPage` gains a "Reschedule delivery" section
  (native `<input type="date">`, no new dependency; visible to
  CUSTOMER/ADMIN when `status === FAILED`) and a "Reschedule history"
  section (same ADMIN/CUSTOMER scope as the tracking timeline).
- **Out of scope, by design**: an approval/rejection workflow, a
  reschedule-cancellation endpoint, a `delivery_attempts` table,
  automatic reassignment, notifications, dashboards, and any change to
  M08's or M09's own code.

## Notification Service (M11)

Full design detail (the post-commit hook pattern and why it avoids an
import cycle, the provider abstraction and its log-based MVP
implementation, failure containment, and why idempotency is anchored
on the exact `tracking_event_id`) is in
[`docs/notifications.md`](docs/notifications.md); `docs/api.md`
explicitly states M11 adds no REST API. Short version:

- **What it does**: for all eight order-lifecycle events
  (`ORDER_CREATED`, `AGENT_ASSIGNED`, `PICKED_UP`, `IN_TRANSIT`,
  `OUT_FOR_DELIVERY`, `DELIVERED`, `FAILED`, `RESCHEDULED`), the
  order's own customer is emailed (always) and texted (only when a
  phone number is on file) — the only recipient this module ever
  resolves.
- **How it integrates without an import cycle**: `internal/notifications`
  depends on `orders`/`users`/`tracking`, but nothing in those packages
  depends back on it. `tracking.TransitionHook` and
  `orders.OrderCreatedHook` are small, nil-safe callback types the
  *producer* packages own; `tracking.TransitionHandler`,
  `assignment.Assign`/`AutoAssign`, `rescheduling.Reschedule`, and
  `orders.CreateOrderHandler` all invoke one after their own
  transaction has already committed. Only `cmd/server/main.go`
  constructs a real `notifications.Service` and wires its methods in —
  no other package knows this module exists.
- **Idempotency, the critical design constraint**: a notification is
  identified by the exact `order_tracking_events.id` (a specific
  occurrence) plus channel — never `(order_id, event, channel)` —
  because the same event type can legitimately recur (a second
  `FAILED` after a full `FAILED→RESCHEDULED→ASSIGNED→...` cycle is a
  new, independently notify-able occurrence). `Repository.Claim`
  (`INSERT ... ON CONFLICT (tracking_event_id, channel) DO NOTHING`)
  runs *before* any provider call; a real Postgres unique index is the
  actual backstop against a duplicate send, proven with concurrent
  goroutines against real Postgres.
- **Provider abstraction**: `EmailProvider`/`SmsProvider`, two narrow
  interfaces. `LogEmailProvider`/`LogSmsProvider` are the zero-config
  default; `ResendEmailProvider` and `TwilioSmsProvider` (both
  post-M12) send real email/SMS behind the identical interfaces,
  opt-in via `EMAIL_PROVIDER=resend`/`SMS_PROVIDER=twilio` — see
  "Post-M12 hardening" above and `docs/notifications.md`.
- **Failure containment**: a provider error is caught, logged, and
  recorded as that one notification's `FAILED` status; a panicking
  provider is recovered. Neither can ever roll back or fail the
  order/tracking/assignment/reschedule operation that triggered it —
  proven directly by forcing a real provider failure mid-lifecycle and
  asserting the HTTP call still commits.
- **No retries**: a `FAILED` notification attempt is never
  automatically retried.
- **Out of scope, by design**: no REST API of any kind, no frontend
  notification UI (no bell icon, no history page, no preferences), no
  queues/Kafka/RabbitMQ/Redis/background workers/polling/webhooks, no
  recipients other than the order's own customer, and no change to
  M08's, M09's, or M10's own logic.

## Dashboards & Evaluation Layer (M12)

Full design detail (why no new backend module or database table was
needed, the exact agent status-update fix, the filter/statistics
design) is in [`docs/dashboards.md`](docs/dashboards.md); the required
system-design write-up is [`docs/system-design.md`](docs/system-design.md)
and the OpenAPI document is [`docs/openapi.yaml`](docs/openapi.yaml).
Short version:

- **Three dashboards, one pattern**: `pages/customer/DashboardPage.tsx`
  (`/customer/dashboard`), `pages/agent/DashboardPage.tsx`
  (`/agent/dashboard`), and `pages/admin/DashboardPage.tsx`
  (`/admin/dashboard`) each link into pages that already exist and are
  already fully tested — none of them duplicate `OrdersPage` or
  `OrderDetailPage`'s own content. A new `DashboardLink` card component
  is the only new shared UI piece.
- **The real gap this milestone closes**: `internal/tracking`'s
  `edgeAuthorizedRoles` has always authorized `DELIVERY_AGENT` for five
  status-transition edges, but `OrderDetailPage`'s status-update section
  rendered only for `isAdmin` — an agent had a fully backend-supported
  capability with no UI path to use it. M12 widens that one render
  condition and adds `transitionsForRole` (`types/tracking.ts`), a
  frontend-only mirror of the backend's per-edge authorization — the
  same "UX convenience, not a security boundary" disclaimer
  `LEGAL_TRANSITIONS` already carries. Zero lines of
  `internal/tracking/statemachine.go` changed. Making this path
  reachable surfaced a real, previously-latent bug — `handleTransition`
  unconditionally refetched the ADMIN/CUSTOMER-only tracking timeline
  after every transition, which would have 403'd for an agent — now
  fixed to only refetch when the viewer is authorized to.
- **Admin order filtering**: `GET /orders?status=&zone=&agent=`, three
  independently optional, combinable query parameters, honored for
  `ADMIN` only — a `CUSTOMER`/`DELIVERY_AGENT` supplying them gets them
  silently ignored, never a widened view. `internal/orders`'s new
  `OrderFilter` struct and `ListAllOrders(ctx, filter)` build the
  `WHERE` clause dynamically from whichever fields are non-empty; a
  zero-value filter is the exact unfiltered query every prior milestone
  relied on. `zone` matches an order's pickup *or* drop zone; an
  invalid `status` is `422`; an unknown `zone`/`agent` id is an empty
  result, not an error.
- **Order statistics**: simple counts by status on the admin dashboard,
  computed client-side from a real, unfiltered `GET /orders` call —
  never hardcoded, never a new aggregation endpoint.
- **OpenAPI & system design**: `docs/openapi.yaml` is a static,
  hand-authored OpenAPI 3.0 document (no runtime reflection/Swagger-UI
  dependency added) mirroring every real route — kept honest by
  `TestOpenAPIContract_DocumentedPathsMatchRealRoutes` (post-M12), which
  walks the real router via chi's own route introspection and diffs it
  against the document on every CI run. `docs/system-design.md` is the
  required ≤ 800-word write-up covering the rate engine, zone detection,
  auto-assignment, and failed-delivery handling.
- **E2E tests**: `backend/tests/e2e/` — empty since M01 — now has real,
  full-stack HTTP flow tests exercising the entire real router
  end-to-end: register → quote → order → assign → agent-driven status
  updates → delivered, and the failed → reschedule → auto-reassign →
  second-lifecycle-to-delivered variant, both asserting real tracking
  history, real actor ids, and that the M11 notification side effect
  actually fires at every step.
- **Out of scope, by design**: charts, graphs, maps, search, pagination,
  real-time/WebSocket updates, a notification bell/history/preferences
  UI (M11 stays untouched), export functionality, extra analytics or
  KPIs beyond simple status counts, a new dashboard backend module or
  database table, and any new frontend dependency.

## Deployment

Full guide — why CORS and a build-time API base URL are both required,
required environment variables per platform, and how to verify a live
deployment — is in [`docs/deployment.md`](docs/deployment.md). Short
version: the backend's existing `Dockerfile` deploys as-is to
Render/Railway/similar; the frontend deploys to Vercel/similar as a
static Vite build. Set `CORS_ALLOWED_ORIGINS` on the backend to the
frontend's real URL, and `VITE_API_BASE_URL` on the frontend to the
backend's real URL, at build/deploy time — both are unset (and
harmless) by default, matching today's same-origin local-dev behavior
exactly.

## Repository Structure

```text
last-mile-delivery-tracker/
├── README.md
├── LICENSE
├── .gitignore
├── .env.example                  # +CORS_ALLOWED_ORIGINS, EMAIL_PROVIDER, RESEND_API_KEY, RESEND_FROM_EMAIL (post-M12)
├── docker-compose.yml
├── .github/workflows/ci.yml      # backend fmt/vet/build/unit/integration/e2e + frontend tsc/lint/test/build, on every push/PR (post-M12)
│
├── backend/
│   ├── go.mod
│   ├── Dockerfile
│   ├── cmd/server/main.go        # entry point: config → DB pool → router → graceful shutdown; CORS wrapper + EmailProvider selection (post-M12)
│   ├── internal/
│   │   ├── config/               # environment-based configuration (M01, M02: +JWT_SECRET; post-M12: +CORSAllowedOrigins, NotificationsConfig)
│   │   ├── database/             # pgx connection pool + migration runner (M01, M02)
│   │   ├── server/               # router, middleware, /health (M01); cors.go — hand-rolled CORS middleware, wrapped around the router only in main.go (post-M12)
│   │   ├── auth/                 # password hashing, JWT, RBAC middleware, register/login/GET+PUT me handlers, demo seed (M02, M03)
│   │   ├── users/                # User domain model + Postgres repository, incl. Update (M02, M03)
│   │   ├── agents/               # delivery_agents domain, repository (transactional creation), handlers, routes (M03)
│   │   ├── zones/                # zones/areas domain, repository, resolution service, handlers, routes (M04)
│   │   ├── rates/                # rate_cards/rate_card_slabs domain, concurrency-safe repository, handlers, routes (M05); pricing.go/quote_handler.go add the M06 calculation engine + POST /orders/quote
│   │   ├── orders/                # orders domain, repository, handlers, routes (M07) — calls rates.CalculateQuote, never reimplements pricing; OrderFilter + admin status/zone/agent query-param filtering (M12)
│   │   ├── tracking/              # status state machine, order_tracking_events domain, repository, handlers, routes (M08)
│   │   ├── assignment/            # candidate ranking, eligibility, repository (reuses tracking.TransitionTx), handlers, routes (M09)
│   │   ├── rescheduling/          # reschedule domain/validation, repository (reuses tracking.TransitionTx, frees the previous agent), handlers, routes (M10)
│   │   └── notifications/         # event->content mapping, provider abstraction + log-based MVP providers, claim-then-resolve repository, Service (M11); resend.go — real, opt-in EmailProvider via Resend's free tier (post-M12); twilio.go — real, opt-in SmsProvider via Twilio's free trial (post-M12)
│   ├── migrations/                # embedded SQL migrations (go:embed): 0001 users (M02), 0002 delivery_agents (M03),
│   │                               #   0003 zones, 0004 areas, 0005 delivery_agents.current_zone_id FK (M04),
│   │                               #   0006 rate_cards, 0007 rate_card_slabs (M05); M06 added none — pure calculation, no new schema;
│   │                               #   0008 orders (M07); 0009 order_tracking_events (M08, no ALTER to orders);
│   │                               #   0010 orders.assigned_agent_id + indexes (M09); 0011 reschedule_requests (M10);
│   │                               #   0012 notifications (M11); M12 added none — no schema change required;
│   │                               #   0013 status/pickup_zone_id/drop_zone_id indexes for the M12 order filter (post-M12)
│   └── tests/
│       ├── unit/                  # convention note — unit tests are co-located with source
│       ├── integration/           # DB-backed integration tests (build tag: integration)
│       └── e2e/                   # full-stack HTTP flow tests (build tag: e2e) — happy path and failed/reschedule/reassign, incl. M11 notification side effects (M12); openapi_contract_test.go — real router vs. docs/openapi.yaml drift check (post-M12)
│
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── .env.example               # VITE_API_BASE_URL — unset for local dev/same-origin deployments (post-M12)
│   └── src/
│       ├── vite-env.d.ts          # ImportMetaEnv typing for VITE_API_BASE_URL (post-M12)
│       ├── components/            # Layout (role-aware nav, +Dashboard link, M12), StatusBadge, ErrorBanner, ProtectedRoute (+roles), AreaPicker (M06, shared with M07), DashboardLink (M12)
│       ├── pages/                 # Home, LoginPage, RegisterPage, Account (profile edit);
│       │                          #   admin/AgentsPage, agent/OperationsPage (M03); admin/ZonesPage (M04); admin/RatesPage (M05); QuotePage (M06);
│       │                          #   CreateOrderPage, OrdersPage (+"My assigned orders" for DELIVERY_AGENT, M09; +admin status/zone/agent filters, M12), OrderDetailPage
│       │                          #   (M07; +tracking timeline & admin transition control, M08; +assigned-agent display & admin assign/auto-assign controls, M09;
│       │                          #   +"Reschedule delivery" control & reschedule-history section, M10; +DELIVERY_AGENT status-update control, M12);
│       │                          #   customer/DashboardPage, agent/DashboardPage, admin/DashboardPage (+order statistics) (M12)
│       ├── services/              # api.ts (+apiPut, +apiDelete; +API_BASE_URL prefix, post-M12), health.ts, auth.ts (+updateProfile), agents.ts (M03), zones.ts (M04), rates.ts (M05), quote.ts (M06), orders.ts (M07; +status/zone/agent filter params, M12), tracking.ts (M08), assignment.ts (M09), rescheduling.ts (M10)
│       ├── hooks/                 # useHealthCheck, useAuth
│       ├── contexts/              # AuthContext.tsx (provider) + auth-context.ts (context object)
│       ├── types/                 # HealthResponse, auth.ts (+ProfileUpdateInput), agent.ts (M03), zone.ts (M04), rate.ts (M05), quote.ts (M06), order.ts (M07; +assigned_agent_id, M09; +OrderFilter/ORDER_STATUSES, M12), tracking.ts (M08; +transitionsForRole, M12), assignment.ts (M09), reschedule.ts (M10)
│       ├── test/                  # setup.ts — React Testing Library cleanup
│       └── utils/                 # currency.ts (formatCurrency, shared M06/M07)
│
├── docs/                          # detailed per-module docs, written as each module lands
│   ├── api.md                     # full endpoint reference, every route so far
│   ├── authentication.md          # roles, schema, JWT, RBAC, token-storage tradeoff (M02)
│   ├── user-agent-management.md   # delivery_agents schema, IDOR protection, M09 notes (M03)
│   ├── zone-management.md         # zones/areas schema, resolution, INTRA/INTER, current_zone_id FK (M04)
│   ├── rate-configuration.md      # rate_cards/rate_card_slabs schema, boundaries, concurrency (M05)
│   ├── rate-calculation.md        # quote engine: weight formula, slab selection, COD, RBAC widening (M06)
│   ├── order-management.md        # orders schema, ownership, pricing snapshot, CalculateQuote reuse (M07)
│   ├── order-tracking.md          # state machine, per-edge authorization, concurrency proof, initial event (M08)
│   ├── assignment-engine.md       # eligibility rule, ranking algorithm, M08 reuse, concurrency proof, assigned_agent_id schema (M09)
│   ├── failed-delivery.md         # reschedule endpoints, the CUSTOMER-vs-M08-matrix resolution, reschedule_requests schema, agent-freeing, M09 reuse (M10)
│   ├── notifications.md           # 8 lifecycle events, post-commit hook pattern, provider abstraction, tracking_event_id idempotency, no API/UI (M11)
│   ├── dashboards.md              # 3 dashboards, admin filters/statistics, the agent status-update UI fix, why no new module/table (M12)
│   ├── openapi.yaml               # static, hand-authored OpenAPI 3.0 document mirroring api.md exactly (M12); kept honest by a real-router diff test (post-M12)
│   ├── system-design.md           # required ≤800-word system-design write-up (M12)
│   └── deployment.md              # CORS + VITE_API_BASE_URL, per-platform env vars, live-deployment verification (post-M12)
│
└── scripts/
    └── seed/                      # reserved for later modules' larger seed data (M02's demo users seed from main.go instead — see its README)
```

## Architecture

Modular monolith: twelve modules, one deployable Go backend, no
microservices. Full module-by-module design, database schema, and API
structure live in `docs/architecture.md` once enough modules exist to make
that worth writing — for now, each module's own doc
([`docs/authentication.md`](docs/authentication.md),
[`docs/user-agent-management.md`](docs/user-agent-management.md),
[`docs/zone-management.md`](docs/zone-management.md),
[`docs/rate-configuration.md`](docs/rate-configuration.md)) covers its
own design, and [`docs/api.md`](docs/api.md) is the running endpoint
reference across all of them.

Two source-document points worth recording now, since they affect the
whole project's submission, not just one module:

- **Deliverable format.** The assignment's problem statement lists a ZIP
  file as a deliverable. The separate submission guidelines specify a
  public GitHub repository on `main` as the format to submit through. Both
  requirements are recorded as they are, unmodified; for the actual
  submission we follow the GitHub-only instruction given to us. This is a
  choice of which channel to submit through, not a claim that the
  repository fulfills the ZIP deliverable.
- **Admin status override.** The assignment requires invalid order-status
  jumps to be rejected, and separately allows admin to override order
  status. Both requirements are honored through two endpoints — a strict
  normal-lifecycle endpoint and a separate, bounded admin-override
  endpoint — documented in full in `docs/order-lifecycle.md` once M08
  lands.

## Evaluation Matrix

Grows with each module.

| Requirement | Implementation | Test | Documentation |
|---|---|---|---|
| Application health / DB connectivity check | `backend/internal/server/health.go` | `backend/internal/server/health_test.go` | this README |
| Role-based authentication (Customer/Agent/Admin) | `backend/internal/auth/`, `backend/internal/users/` | `internal/auth/*_test.go`, `tests/integration/auth_integration_test.go` | `docs/authentication.md` |
| Password hashing (bcrypt, never plaintext) | `backend/internal/auth/password.go` | `password_test.go`, `security_test.go` | `docs/authentication.md` |
| JWT issuance and validation | `backend/internal/auth/jwt.go` | `jwt_test.go` | `docs/authentication.md` |
| RBAC middleware (`RequireAuth`, `RequireRole`) | `backend/internal/auth/middleware.go` | `middleware_test.go` | `docs/authentication.md` |
| Public registration cannot create ADMIN/AGENT | `backend/internal/auth/handler.go` (`RegisterHandler`) | `TestRegisterHandler_ClientSuppliedRoleIsRejectedNotHonored` | `docs/authentication.md` |
| `GET /users/me` (not `/auth/me`) | `backend/internal/auth/handler.go` (`GetMeHandler`) | `handler_test.go`, integration flow test | `docs/authentication.md` |
| `users` table: unique email, role CHECK constraint | `backend/migrations/0001_create_users_table.sql` | `TestUsersTable_EmailUniqueConstraint`, `TestUsersTable_RoleCheckConstraint` | `docs/authentication.md` |
| Frontend login/register/logout, protected route | `frontend/src/pages/`, `contexts/AuthContext.tsx`, `components/ProtectedRoute.tsx` | `AuthContext.test.tsx`, `ProtectedRoute.test.tsx`, `auth.test.ts` | `docs/authentication.md` |
| `PUT /users/me` profile editing, mass-assignment safe | `backend/internal/auth/handler.go` (`UpdateMeHandler`) | `TestUpdateMeHandler_CannotMassAssignProtectedFields` | `docs/api.md` |
| `delivery_agents` table: unique user_id, FK, availability/coordinate CHECKs | `backend/migrations/0002_create_delivery_agents_table.sql` | `TestDeliveryAgentsTable_*` (4 tests) | `docs/user-agent-management.md` |
| Admin-only agent provisioning, transactional | `backend/internal/agents/repository.go` (`Create`) | `TestAgentCreation_TransactionalAtomicity`, `TestCreateAgent_RoleGating` | `docs/user-agent-management.md` |
| Agent availability/location management + IDOR protection | `backend/internal/agents/handler.go` | `TestUpdate{Availability,Location}Handler_AnotherAgentForbidden` | `docs/user-agent-management.md` |
| Frontend profile edit, admin agent management, agent operations | `frontend/src/pages/Account.tsx`, `pages/admin/AgentsPage.tsx`, `pages/agent/OperationsPage.tsx` | `Account.test.tsx`, `AgentsPage.test.tsx`, `OperationsPage.test.tsx` | `docs/user-agent-management.md` |
| `zones`/`areas` tables: name UNIQUE/CHECK, area→zone FK, per-zone area UNIQUE | `backend/migrations/0003_create_zones_table.sql`, `0004_create_areas_table.sql` | `TestZonesTable_*`, `TestAreasTable_*` | `docs/zone-management.md` |
| Admin-only zone/area CRUD, no cross-zone mass assignment | `backend/internal/zones/handler.go`, `routes.go` | `TestZoneEndpoints_RoleGating`, `TestAreaCreate_ZoneIDBodyTamperCannotOverridePath` | `docs/zone-management.md` |
| Deterministic address→area→zone resolution, INTRA/INTER classification | `backend/internal/zones/resolution.go` (`ResolvePickupDrop`, `DetermineZoneRelationship`) | `resolution_test.go`, `TestResolution_IntraAndInterAgainstRealDatabase` | `docs/zone-management.md` |
| Inactive-zone resolution explicitly rejected (not silently allowed) | `backend/internal/zones/resolution.go` (`ErrZoneInactive`) | `TestResolveArea_InactiveZoneIsRejected` | `docs/zone-management.md` |
| `delivery_agents.current_zone_id` foreign key completed | `backend/migrations/0005_add_zone_fk_to_delivery_agents.sql` | `TestDeliveryAgentsCurrentZoneFK` | `docs/zone-management.md` |
| Frontend admin zone/area management, loading/empty/error states | `frontend/src/pages/admin/ZonesPage.tsx` | `ZonesPage.test.tsx`, `services/zones.test.ts` | `docs/zone-management.md` |
| `rate_cards`/`rate_card_slabs` tables: enum/amount CHECKs, FK, one-active-per-combination + one-open-ended-per-card partial unique indexes | `backend/migrations/0006_create_rate_cards_table.sql`, `0007_create_rate_card_slabs_table.sql` | `TestRateCardsTable_*`, `TestRateCardSlabsTable_*` | `docs/rate-configuration.md` |
| Admin-only rate card/slab CRUD incl. slab DELETE, path-ownership enforced | `backend/internal/rates/handler.go`, `routes.go` | `TestSlabEndpoints_RoleGating`, `TestUpdateSlab_WrongRateCardInPathRejected`, `TestDeleteSlab_WrongRateCardInPathRejected` | `docs/rate-configuration.md` |
| `[min, max)` slab semantics, overlap rejection, at most one open-ended slab | `backend/internal/rates/slab_validation.go` | `TestSlabRangesOverlap`, `TestValidateSlabAgainstExisting_*` (incl. exact-boundary and demo-config cases) | `docs/rate-configuration.md` |
| One active rate card per (order_type, zone_relationship), race-safe | `backend/internal/rates/repository.go` (`UpdateRateCard`) | `TestConcurrentActivation_OnlyOneWins` (real concurrent requests vs. real Postgres) | `docs/rate-configuration.md` |
| Concurrent slab writes serialized, race-safe | `backend/internal/rates/repository.go` (`lockRateCard`, `CreateSlab`/`UpdateSlab`) | `TestConcurrentSlabCreation_OverlapPreventedUnderRace` | `docs/rate-configuration.md` |
| Frontend admin rate card/slab management, loading/empty/error states | `frontend/src/pages/admin/RatesPage.tsx` | `RatesPage.test.tsx`, `services/rates.test.ts` | `docs/rate-configuration.md` |
| `POST /orders/quote`: volumetric/chargeable weight, `[min,max)` slab selection, COD surcharge | `backend/internal/rates/pricing.go` (`CalculateQuote`) | `TestQuote_*` (unit + `quote_integration_test.go` golden cases) | `docs/rate-calculation.md` |
| `orders` table: FKs, CHECKs, default `status='CREATED'` | `backend/migrations/0008_create_orders_table.sql` | `TestOrdersTable_*` | `docs/order-management.md` |
| Order creation reuses `CalculateQuote`, never re-implements pricing | `backend/internal/orders/handler.go` (`CreateOrderHandler`) | `TestOrderCreate_PricingSnapshotMatchesM06Calculation` | `docs/order-management.md` |
| Customer-vs-admin order creation DTOs, no `customer_id` mass assignment | `backend/internal/orders/handler.go` | `TestCreateOrderHandler_CustomerIdentityComesFromJWTNotBody`, `TestOrderCreate_CustomerCannotSpecifyCustomerID` | `docs/order-management.md` |
| Order list/get ownership (ADMIN all, CUSTOMER own only, 404 not 403) | `backend/internal/orders/handler.go` | `TestOrderGet_CustomerOwnershipIDOR`, `TestOrderList_CustomerIsolation` | `docs/order-management.md` |
| Frontend order creation/list/detail, loading/empty/error states | `frontend/src/pages/CreateOrderPage.tsx`, `OrdersPage.tsx`, `OrderDetailPage.tsx` | `CreateOrderPage.test.tsx`, `OrdersPage.test.tsx`, `OrderDetailPage.test.tsx` | `docs/order-management.md` |
| `order_tracking_events` table: append-only, no update/delete route | `backend/migrations/0009_create_order_tracking_events_table.sql` | `TestOrderTrackingEventsTable_*`, `TestTrackingEvents_NoUpdateOrDeleteRouteExists` | `docs/order-tracking.md` |
| Closed 8-edge state machine, per-edge role authorization | `backend/internal/tracking/statemachine.go` | `TestIsValidTransition_*`, `TestIsRoleAuthorized_FullMatrix` | `docs/order-tracking.md` |
| Concurrent conflicting transitions serialized, race-safe | `backend/internal/tracking/repository.go` (`Transition`/`TransitionTx`) | `TestConcurrentTransition_OnlyOneWins` | `docs/order-tracking.md` |
| Order creation atomically writes its own opening tracking event | `backend/internal/orders/repository.go` (`CreateOrder`) | `TestOrderCreate_InitialTrackingEventRecorded` | `docs/order-tracking.md` |
| Frontend tracking timeline + admin-only transition control | `frontend/src/pages/OrderDetailPage.tsx` | `OrderDetailPage.test.tsx` | `docs/order-tracking.md` |
| `orders.assigned_agent_id` + partial unique index (one active assignment per agent) | `backend/migrations/0010_add_assigned_agent_id_to_orders.sql` | `TestOrdersTable_*`, `TestAssignmentConcurrency_SameAgentRacedByTwoOrders` | `docs/assignment-engine.md` |
| Deterministic candidate ranking (same-zone, `last_assigned_at`, UUID tiebreak), pure/DB-free | `backend/internal/assignment/candidate.go` (`SelectCandidate`) | `TestSelectCandidate_*`, `TestSelectCandidate_Deterministic` | `docs/assignment-engine.md` |
| Manual/auto-assignment reuse M08's `TransitionTx`, never reimplement the state machine | `backend/internal/assignment/repository.go` (`Assign`, `AutoAssign`) | `TestAssignmentFlow_*` (integration) | `docs/assignment-engine.md` |
| Assignment concurrency (agent-then-order lock ordering + DB backstop) | `backend/internal/assignment/repository.go` (`lockCandidate`) | `TestAssignmentConcurrency_SameOrderRacedByTwoAdmins`, `TestAssignmentConcurrency_SameAgentRacedByTwoOrders` (repeated `-count=5`) | `docs/assignment-engine.md` |
| `GET /orders`/`GET /orders/:id` scoped to DELIVERY_AGENT's own assigned orders | `backend/internal/orders/handler.go` | `TestListOrdersHandler_DeliveryAgentSeesOnlyAssignedOrders`, `TestOrderList_DeliveryAgentSeesOnlyAssignedOrders` | `docs/assignment-engine.md` |
| Frontend "My assigned orders" view, admin manual/auto-assign controls | `frontend/src/pages/OrdersPage.tsx`, `OrderDetailPage.tsx` | `OrdersPage.test.tsx`, `OrderDetailPage.test.tsx`, `services/assignment.test.ts` | `docs/assignment-engine.md` |
| `reschedule_requests` table: FKs, no approval/status columns | `backend/migrations/0011_create_reschedule_requests_table.sql` | integration schema/persistence tests in `rescheduling_integration_test.go` | `docs/failed-delivery.md` |
| CUSTOMER-initiated reschedule resolved without modifying M08's ADMIN-only matrix | `backend/internal/rescheduling/handler.go`, `repository.go` | `TestRescheduleFlow_CustomerHappyPath`, `TestRescheduleFlow_AdminHappyPath` | `docs/failed-delivery.md` |
| Reschedule reuses M08's `TransitionTx`, never reimplements the state machine | `backend/internal/rescheduling/repository.go` (`Reschedule`) | `TestRescheduleFlow_*` (integration) | `docs/failed-delivery.md` |
| Reschedule ownership/RBAC (CUSTOMER own order, ADMIN any, DELIVERY_AGENT 403, 404 hides existence) | `backend/internal/rescheduling/handler.go` | `TestRescheduleHandler_*`, `TestRescheduleFlow_CustomerOwnershipEnforced`, `TestRescheduleFlow_DeliveryAgentForbidden` | `docs/failed-delivery.md` |
| Previously assigned agent freed to AVAILABLE, atomically with the transition | `backend/internal/rescheduling/repository.go` (`freeAgent`) | `TestRescheduleFlow_CustomerHappyPath`, `TestRescheduleFlow_PreviouslyAssignedAgentNotIncorrectlyReused` | `docs/failed-delivery.md` |
| Reschedule concurrency (same-order race, cross-module lock-ordering vs. M09) | `backend/internal/rescheduling/repository.go` | `TestRescheduleConcurrency_SameOrderRacedTwice`, `TestRescheduleConcurrency_DoesNotDeadlockWithAssignment` (repeated `-count=5`) | `docs/failed-delivery.md` |
| Pure, deterministic date validation (past-date rejection, same-day allowed) | `backend/internal/rescheduling/reschedule.go` (`ValidateRescheduleDate`) | `TestValidateRescheduleDate_*`, `TestParseRequestedDate_*` | `docs/failed-delivery.md` |
| Frontend reschedule control (native date input) + reschedule history | `frontend/src/pages/OrderDetailPage.tsx` | `OrderDetailPage.test.tsx`, `services/rescheduling.test.ts` | `docs/failed-delivery.md` |
| `notifications` table: FKs, event/channel/status CHECKs, `(tracking_event_id, channel)` unique index | `backend/migrations/0012_create_notifications_table.sql` | `TestNotificationsTable_SchemaAndUniqueConstraint` (integration) | `docs/notifications.md` |
| All 8 lifecycle events notify the customer (email always, SMS when phone on file) | `backend/internal/notifications/service.go` (`NotifyTransition`, `NotifyOrderCreated`) | `TestNotifyTransition_EveryLifecycleEventRecognized`, `TestNotificationFlow_FullLifecycleEveryEventNotifies` (integration) | `docs/notifications.md` |
| Post-commit hooks avoid an M08/M09/M10/M11 import cycle, zero lines changed in those modules | `backend/internal/tracking/event.go` (`TransitionHook`), `backend/internal/orders/order.go` (`OrderCreatedHook`), `backend/cmd/server/main.go` | `TestTransitionHandler_TransitionHookFiresAfterSuccess`, `TestCreateOrderHandler_OrderCreatedHookFiresAfterSuccess` | `docs/notifications.md` |
| Idempotency anchored on the exact `tracking_event_id`, not `(order_id, event, channel)` — a repeated FAILED after a reschedule cycle independently notifies | `backend/internal/notifications/repository.go` (`Claim`) | `TestNotifyTransition_RepeatedFailedOccurrencesEachNotify`, `TestNotificationFlow_SecondFailedOccurrenceCreatesNewRow` (integration) | `docs/notifications.md` |
| Notification concurrency: claim-before-send + DB unique index prevents a duplicate send | `backend/internal/notifications/repository.go` (`Claim`) | `TestNotificationConcurrency_ConcurrentIdenticalAttemptsClaimExactlyOnce` (repeated `-count=5`) | `docs/notifications.md` |
| Provider failure/panic never breaks the triggering lifecycle commit | `backend/internal/notifications/service.go` (`dispatch`, `safeSend`) | `TestNotifyTransition_ProviderPanicContained`, `TestNotificationFlow_ProviderFailureDoesNotBreakLifecycleCommit` (integration) | `docs/notifications.md` |
| No REST API and no frontend UI, by design | — (no new routes, no new frontend files) | `TestNotifications_NoPublicEndpointsExist` | `docs/api.md`, `docs/notifications.md` |
| Customer/Agent/Admin dashboard landing pages, role-gated | `frontend/src/pages/{customer,agent,admin}/DashboardPage.tsx`, `App.tsx` | `customer/DashboardPage.test.tsx`, `agent/DashboardPage.test.tsx`, `admin/DashboardPage.test.tsx` | `docs/dashboards.md` |
| Admin order filtering by status/zone/agent, combinable, non-admin roles unaffected | `backend/internal/orders/repository.go` (`OrderFilter`, `ListAllOrders`), `handler.go` (`parseOrderFilter`) | `TestListOrdersHandler_Admin*Filter*`, `TestOrderList_AdminFiltering` (integration) | `docs/dashboards.md`, `docs/api.md` |
| Delivery-agent status-update UI reaches an already-authorized backend capability, zero state-machine changes | `frontend/src/pages/OrderDetailPage.tsx`, `types/tracking.ts` (`transitionsForRole`) | `OrderDetailPage.test.tsx` (agent transition tests) | `docs/dashboards.md` |
| Admin order statistics computed from real, unfiltered order data | `frontend/src/pages/admin/DashboardPage.tsx` | `admin/DashboardPage.test.tsx` | `docs/dashboards.md` |
| OpenAPI documentation matches the real API surface, evaluator-inspectable | `docs/openapi.yaml` | validated as parseable OpenAPI 3.0 YAML; cross-checked path-by-path against `docs/api.md` | `docs/api.md`, `docs/openapi.yaml` |
| Required system-design write-up, within the 800-word limit | `docs/system-design.md` | `wc -w` verified ≤ 800 | `docs/system-design.md` |
| Full-stack E2E flows: happy path and failed/reschedule/reassign, real router, real DB, real M11 notification side effects | `backend/tests/e2e/lifecycle_test.go` | `TestE2E_HappyPath_RegisterQuoteOrderAssignDeliver`, `TestE2E_FailedDeliveryRescheduleReassignContinues` (`-tags=e2e`) | `README.md`, `docs/dashboards.md` |
| CORS: off by default, explicit allowlist, no cookie-based credentials header | `backend/internal/server/cors.go` | `TestCORS_*` (5 unit tests); live-verified against a real container (allow-path and deny-path both) | `docs/deployment.md` |
| Real, opt-in email delivery via Resend, fails fast at startup if misconfigured, 10s timeout independent of request context | `backend/internal/notifications/resend.go`, `cmd/server/main.go` | `TestResendEmailProvider_*` (unit, incl. unreachable-server case) | `docs/notifications.md` |
| Real, opt-in SMS delivery via Twilio, fails fast at startup if misconfigured, 10s timeout independent of request context, auth token never leaks into errors | `backend/internal/notifications/twilio.go`, `cmd/server/main.go` | `TestTwilioSmsProvider_*` (unit, incl. unreachable-server and secret-hygiene cases) | `docs/notifications.md` |
| Frontend API base URL configurable at build time, empty-default unchanged behavior | `frontend/src/services/api.ts`, `vite-env.d.ts` | `api.test.ts` (`API_BASE_URL` describe block) | `docs/deployment.md` |
| CI: backend fmt/vet/build/unit/integration/e2e + frontend tsc/lint/test/build on every push/PR | `.github/workflows/ci.yml` | the workflow itself, run on GitHub | `README.md` |
| OpenAPI document cannot silently drift from the real router | `backend/tests/e2e/openapi_contract_test.go` | `TestOpenAPIContract_DocumentedPathsMatchRealRoutes` (`-tags=e2e`) | `docs/openapi.yaml` |
| Supporting indexes on every column the M12 order filter can query | `backend/migrations/0013_add_order_filter_indexes.sql` | verified present via `\d orders` against a fresh database | `README.md` |
| `current_zone_id` write path — a real agent can now become eligible for auto-assignment via the app itself, not just direct SQL | `backend/internal/agents/handler.go` (`UpdateZoneHandler`), `routes.go`, `repository.go` | `TestUpdateZoneHandler_*` (unit), `TestUpdateAgentZone_*` (integration), live-verified against a running container (success, unknown-zone 422, admin-forbidden 403) | `docs/user-agent-management.md` |
