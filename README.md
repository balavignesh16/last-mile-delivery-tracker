# Last-Mile Delivery Tracker

## Overview

A delivery management platform where customers and admins create orders
with auto-calculated charges, delivery agents are assigned to orders
(manually or automatically), and customers are notified at every step of
the delivery journey. Built as a Go + PostgreSQL backend behind a REST API,
with a React + TypeScript frontend, structured as a modular monolith.

## Current Status

**M01 — Foundation & Infrastructure.** This is the technical foundation
only: a running backend, a running frontend shell, a database connection,
and a health check. **No business functionality exists yet** — no
authentication, no orders, no zones, no rates, no assignment, no tracking.
Those arrive in M02 through M12, in order, each expanding this README and
`docs/` as it lands.

| Module | Status |
|---|---|
| M01 — Foundation & Infrastructure | ✅ Done |
| M02 — Authentication & RBAC | Not started |
| M03 — User & Agent Management | Not started |
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
| Auth | JWT, bcrypt (added in M02) |
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
cp .env.example .env        # then edit DB_PASSWORD to a real local value
docker compose up -d        # starts PostgreSQL + the backend
cd frontend && npm install && npm run dev
```

This is honestly a **two-command** setup, not one: `docker compose up`
brings up PostgreSQL and the backend; the frontend is run separately with
`npm run dev` (see [Docker](#docker) below for why it isn't in Compose).

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
│   │   ├── config/               # environment-based configuration (M01)
│   │   ├── database/             # pgx connection pool foundation (M01)
│   │   ├── server/               # router, middleware, /health (M01)
│   │   ├── auth/                 # M02 — reserved, empty
│   │   ├── users/                # M03 — reserved, empty
│   │   ├── agents/               # M03 — reserved, empty
│   │   ├── zones/                # M04 — reserved, empty
│   │   ├── rates/                # M05/M06 — reserved, empty
│   │   ├── orders/                # M07 — reserved, empty
│   │   ├── tracking/              # M08 — reserved, empty
│   │   ├── assignment/            # M09 — reserved, empty
│   │   ├── rescheduling/          # M10 — reserved, empty
│   │   └── notifications/         # M11 — reserved, empty
│   ├── migrations/                # SQL migrations — first one lands with M02
│   └── tests/
│       ├── unit/                  # convention note — unit tests are co-located with source
│       ├── integration/           # DB-backed integration tests (build tag: integration)
│       └── e2e/                   # reserved — full-stack flow tests start around M06/M07
│
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   └── src/
│       ├── components/            # Layout, StatusBadge
│       ├── pages/                 # Home (health-check shell page)
│       ├── services/              # api.ts, health.ts
│       ├── hooks/                 # useHealthCheck
│       ├── contexts/              # reserved — no cross-cutting state yet
│       ├── types/                 # HealthResponse
│       └── utils/                 # reserved — no shared utility yet
│
├── docs/                          # detailed per-module docs, written as each module lands
│
└── scripts/
    └── seed/                      # reserved — demo seed data starts once tables exist
```

## Architecture

Modular monolith: twelve modules, one deployable Go backend, no
microservices. Full module-by-module design, database schema, API
structure, and the resolved design decisions from the pre-implementation
architecture audit live in `docs/architecture.md` once M02 begins — M01
has no business modules to document yet.

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

Grows with each module. Seeded now with the one capability M01 actually
implements, to show the pattern the rest of the table will follow:

| Requirement | Implementation | Test | Documentation |
|---|---|---|---|
| Application health / DB connectivity check | `backend/internal/server/health.go` | `backend/internal/server/health_test.go` | this section |
