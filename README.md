# Last-Mile Delivery Tracker

## Overview

A delivery management platform where customers and admins create orders
with auto-calculated charges, delivery agents are assigned to orders
(manually or automatically), and customers are notified at every step of
the delivery journey. Built as a Go + PostgreSQL backend behind a REST API,
with a React + TypeScript frontend, structured as a modular monolith.

## Current Status

**M03 — User & Agent Management.** The backend now also has a
`delivery_agents` table, admin-only agent provisioning, agent
availability/location management, and profile editing (`PUT
/users/me`). The frontend has a real profile-edit form, an admin agent
management screen, and an agent operational screen. **No other business
functionality exists yet** — no zones, no rates, no orders, no assignment
algorithm, no tracking. Those arrive in M04 through M12, in order, each
expanding this README and `docs/` as they land.

| Module | Status |
|---|---|
| M01 — Foundation & Infrastructure | ✅ Done |
| M02 — Authentication & RBAC | ✅ Done |
| M03 — User & Agent Management | ✅ Done |
| M04 — Zone Management | Not started |
| M05 — Rate Configuration | Not started |
| M06 — Rate Calculation Engine | Not started |
| M07 — Order Management | Not started |
| M08 — Tracking & Order Lifecycle | Not started |
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
│   │   ├── zones/                # M04 — reserved, empty
│   │   ├── rates/                # M05/M06 — reserved, empty
│   │   ├── orders/                # M07 — reserved, empty
│   │   ├── tracking/              # M08 — reserved, empty
│   │   ├── assignment/            # M09 — reserved, empty
│   │   ├── rescheduling/          # M10 — reserved, empty
│   │   └── notifications/         # M11 — reserved, empty
│   ├── migrations/                # embedded SQL migrations (go:embed): 0001 users (M02), 0002 delivery_agents (M03)
│   └── tests/
│       ├── unit/                  # convention note — unit tests are co-located with source
│       ├── integration/           # DB-backed integration tests (build tag: integration)
│       └── e2e/                   # reserved — full-stack flow tests start around M06/M07
│
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   └── src/
│       ├── components/            # Layout (role-aware nav), StatusBadge, ErrorBanner, ProtectedRoute (+roles)
│       ├── pages/                 # Home, LoginPage, RegisterPage, Account (profile edit);
│       │                          #   admin/AgentsPage, agent/OperationsPage (M03)
│       ├── services/              # api.ts (+apiPut), health.ts, auth.ts (+updateProfile), agents.ts (M03)
│       ├── hooks/                 # useHealthCheck, useAuth
│       ├── contexts/              # AuthContext.tsx (provider) + auth-context.ts (context object)
│       ├── types/                 # HealthResponse, auth.ts (+ProfileUpdateInput), agent.ts (M03)
│       ├── test/                  # setup.ts — React Testing Library cleanup
│       └── utils/                 # reserved — no shared utility yet
│
├── docs/                          # detailed per-module docs, written as each module lands
│   ├── api.md                     # full endpoint reference, every route so far
│   ├── authentication.md          # roles, schema, JWT, RBAC, token-storage tradeoff (M02)
│   └── user-agent-management.md   # delivery_agents schema, IDOR protection, M09 notes (M03)
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
[`docs/user-agent-management.md`](docs/user-agent-management.md)) covers
its own design, and [`docs/api.md`](docs/api.md) is the running endpoint
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
