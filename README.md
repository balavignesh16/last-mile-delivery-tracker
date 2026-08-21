# Last-Mile Delivery Tracker

## Overview

A delivery management platform where customers and admins create orders
with auto-calculated charges, delivery agents are assigned to orders
(manually or automatically), and customers are notified at every step of
the delivery journey. Built as a Go + PostgreSQL backend behind a REST API,
with a React + TypeScript frontend, structured as a modular monolith.

## Current Status

**M08 — Tracking & Order Lifecycle.** Orders now have a real status
state machine: `POST /api/v1/orders/:id/status` validates and applies
one legal transition (`CREATED → ASSIGNED → PICKED_UP → IN_TRANSIT →
OUT_FOR_DELIVERY → {DELIVERED | FAILED → RESCHEDULED → ASSIGNED}`,
`DELIVERED` terminal, every other pair — including same-status and the
blueprint's own `CREATED → DELIVERED` counterexample — rejected `409`),
and `GET /api/v1/orders/:id/tracking` returns the full, immutable,
chronological event history. Authorization is per-edge, not per-route:
`ADMIN` may perform any transition, `DELIVERY_AGENT` only the five
agent-tier ones, `CUSTOMER` none. Concurrent conflicting transitions on
the same order are serialized by a `SELECT ... FOR UPDATE` lock — proven
under real concurrent load, not just asserted. Order creation itself
(M07) now writes the order's opening `CREATED` tracking event
atomically, inside the same transaction as the order row. The frontend
adds a tracking timeline to the order detail page (both `CUSTOMER` and
`ADMIN`) and an `ADMIN`-only status-transition control. **Agent
assignment, reschedule-date capture, and notifications are explicitly
out of scope here** — no `assigned_agent_id` column or assignment logic
was added; `DELIVERY_AGENT` transition authority is deliberately
unscoped in this module (documented, temporary) until M09 supplies that
relationship. Those arrive in M09 through M12, in order, each expanding
this README and `docs/` as they land.

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
| M09 — Assignment Engine | Not started |
| M10 — Failed Delivery & Rescheduling | Not started |
| M11 — Notification Service | Not started |
| M12 — Dashboards & Evaluation Layer | Not started |

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

**Backend:**

```bash
cd backend

# Unit tests — no external dependencies required
go test ./...

# Integration tests — requires a running PostgreSQL instance
docker compose up -d postgres
TEST_DATABASE_URL="postgres://lastmile:<your-password>@localhost:5432/lastmile?sslmode=disable" \
  go test -tags=integration ./tests/integration/...
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
- **Known, documented, temporary gap**: no `assigned_agent_id` exists
  yet (that's M09's own `POST /orders/:id/assign`) — so any
  authenticated DELIVERY_AGENT may perform an agent-tier edge on *any*
  order, not only one assigned to them. A finalized M08 decision, not
  an oversight; M09 tightens this once the assignment relationship
  exists.
- **`GET /api/v1/orders/:id/tracking`** (ADMIN any order; CUSTOMER own
  only, `404` otherwise; DELIVERY_AGENT → 403) — the full,
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
  No `assigned_agent_id`, no `slab_id`. Genuinely append-only — no
  update/delete route exists for this table at all.
- **Out of scope, by design**: agent assignment/auto-assignment,
  reschedule-date capture, notifications, status/zone/agent filtering,
  any agent-facing order list/detail UI — all later modules.

## Repository Structure

```text
last-mile-delivery-tracker/
├── README.md
├── LICENSE
├── .gitignore
├── .env.example
├── docker-compose.yml
│
├── backend/
│   ├── go.mod
│   ├── Dockerfile
│   ├── cmd/server/main.go        # entry point: config → DB pool → router → graceful shutdown
│   ├── internal/
│   │   ├── config/               # environment-based configuration (M01, M02: +JWT_SECRET)
│   │   ├── database/             # pgx connection pool + migration runner (M01, M02)
│   │   ├── server/               # router, middleware, /health (M01)
│   │   ├── auth/                 # password hashing, JWT, RBAC middleware, register/login/GET+PUT me handlers, demo seed (M02, M03)
│   │   ├── users/                # User domain model + Postgres repository, incl. Update (M02, M03)
│   │   ├── agents/               # delivery_agents domain, repository (transactional creation), handlers, routes (M03)
│   │   ├── zones/                # zones/areas domain, repository, resolution service, handlers, routes (M04)
│   │   ├── rates/                # rate_cards/rate_card_slabs domain, concurrency-safe repository, handlers, routes (M05); pricing.go/quote_handler.go add the M06 calculation engine + POST /orders/quote
│   │   ├── orders/                # orders domain, repository, handlers, routes (M07) — calls rates.CalculateQuote, never reimplements pricing
│   │   ├── tracking/              # status state machine, order_tracking_events domain, repository, handlers, routes (M08)
│   │   ├── assignment/            # M09 — reserved, empty
│   │   ├── rescheduling/          # M10 — reserved, empty
│   │   └── notifications/         # M11 — reserved, empty
│   ├── migrations/                # embedded SQL migrations (go:embed): 0001 users (M02), 0002 delivery_agents (M03),
│   │                               #   0003 zones, 0004 areas, 0005 delivery_agents.current_zone_id FK (M04),
│   │                               #   0006 rate_cards, 0007 rate_card_slabs (M05); M06 added none — pure calculation, no new schema;
│   │                               #   0008 orders (M07); 0009 order_tracking_events (M08, no ALTER to orders)
│   └── tests/
│       ├── unit/                  # convention note — unit tests are co-located with source
│       ├── integration/           # DB-backed integration tests (build tag: integration)
│       └── e2e/                   # reserved — full-stack flow tests start around M09
│
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   └── src/
│       ├── components/            # Layout (role-aware nav), StatusBadge, ErrorBanner, ProtectedRoute (+roles), AreaPicker (M06, shared with M07)
│       ├── pages/                 # Home, LoginPage, RegisterPage, Account (profile edit);
│       │                          #   admin/AgentsPage, agent/OperationsPage (M03); admin/ZonesPage (M04); admin/RatesPage (M05); QuotePage (M06);
│       │                          #   CreateOrderPage, OrdersPage, OrderDetailPage (M07; +tracking timeline & admin transition control, M08)
│       ├── services/              # api.ts (+apiPut, +apiDelete), health.ts, auth.ts (+updateProfile), agents.ts (M03), zones.ts (M04), rates.ts (M05), quote.ts (M06), orders.ts (M07), tracking.ts (M08)
│       ├── hooks/                 # useHealthCheck, useAuth
│       ├── contexts/              # AuthContext.tsx (provider) + auth-context.ts (context object)
│       ├── types/                 # HealthResponse, auth.ts (+ProfileUpdateInput), agent.ts (M03), zone.ts (M04), rate.ts (M05), quote.ts (M06), order.ts (M07), tracking.ts (M08)
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
│   └── order-tracking.md          # state machine, per-edge authorization, concurrency proof, initial event (M08)
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
