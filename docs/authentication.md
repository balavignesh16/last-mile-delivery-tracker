# Authentication & RBAC (M02)

## Roles

Exactly three, per the assignment: `CUSTOMER`, `DELIVERY_AGENT`, `ADMIN`. Stored as a
plain `TEXT` column constrained by a `CHECK` constraint (not a Postgres `ENUM`
type), so adding a role later never needs an `ALTER TYPE` migration.

## The users table

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    phone         TEXT,
    role          TEXT NOT NULL CHECK (role IN ('CUSTOMER', 'DELIVERY_AGENT', 'ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

One table backs all three roles — the frozen architecture folded an earlier
separate `customers` table into `users`, since no customer-only field was ever
identified. `delivery_agents`-specific operational data (availability,
location) is a later module's table (M03), not this one.

Migrations are embedded SQL files (`backend/migrations/*.sql`, via
`go:embed`) applied automatically on every backend startup, tracked in a
`schema_migrations` table — no external migration tool, no manual step.

## Who can create what kind of account

**Public registration (`POST /api/v1/auth/register`) always creates a
`CUSTOMER` account.** There is no code path from that endpoint to any other
role — the request DTO has no `role` field at all, and the decoder rejects
any request containing an unrecognized field (including `"role"`) with a
422, rather than silently ignoring it. A client sending
`{"role":"ADMIN", ...}` gets its entire request refused; no account, of any
role, is created from that request.

`DELIVERY_AGENT` accounts are provisioned by an admin — that's `POST
/agents`, which belongs to M03 and does not exist yet. `ADMIN` accounts are
never created through any HTTP endpoint in this project.

So how does the first admin account come to exist? Through a fixed,
version-controlled **demo seed step** (`internal/auth.SeedDemoUsers`),
run automatically alongside migrations on every backend startup. It is
idempotent — creates each demo account only if it doesn't already exist —
and creates exactly one demo account per role:

| Email | Password | Role |
|---|---|---|
| `admin@lastmile.test` | `Admin123!` | ADMIN |
| `agent@lastmile.test` | `Agent123!` | DELIVERY_AGENT |
| `customer@lastmile.test` | `Customer123!` | CUSTOMER |

These are intentionally public, hard-coded, demo-only credentials — the
whole point is that an evaluator can log in as any role without a real
provisioning flow existing yet. Never reuse this pattern for production
credentials.

## Password hashing

bcrypt (`golang.org/x/crypto/bcrypt`), default cost. Registration hashes
before the first write; login compares via `bcrypt.CompareHashAndPassword`.
Two hashes of the same password are never equal (bcrypt salts per call).
`password_hash` is never included in any API response — every response is
built from an explicit response DTO that simply has no such field, rather
than by remembering to strip one from a larger struct.

Password policy: minimum 8 characters. No complexity rules — the assignment
specifies none, and inventing uppercase/digit/symbol requirements would add
friction without a stated reason.

## JWT

- Algorithm: HS256 (symmetric) — the right choice here since one backend
  both issues and verifies every token; there's no separate issuer that
  would justify RS256's asymmetric keypair.
- Claims: subject (user ID), role, issued-at, expiration. Nothing else — no
  permissions list, no session ID.
- Expiration: 24 hours. No refresh tokens — out of scope per the frozen
  architecture; a longer session just means logging in again.
- Secret: `JWT_SECRET` environment variable, required at startup (the
  server fails to start with a clear error if it's missing) — never
  hard-coded, never has a functional default.

`Authorization: Bearer <token>` is the only accepted format. Every failure
mode — missing header, malformed header, invalid signature, expired token,
unsupported algorithm (e.g. `alg: none`) — is rejected identically with a
generic 401. The middleware never reports which check failed; that detail
is only useful to an attacker probing the token format.

## RBAC

Two composable middlewares, in `internal/auth`:

```
RequireAuth(jwtSecret)   // validates the token, puts Identity{UserID, Role} in context
RequireRole(roles ...)   // must run after RequireAuth; 403 if the role isn't in the list
```

Chained via chi's `.With(...)`:

```go
v1.With(RequireAuth(jwtSecret)).Get("/users/me", GetMeHandler(usersRepo))
```

`RequireRole` used without `RequireAuth` ahead of it returns 401 (no
identity to be forbidden), not 403 — a defensive fallback, not the normal
path. This is the one centralized place role checks live; handlers never
contain `if user.Role == "ADMIN"` logic themselves.

M02 has no real role-gated production endpoint yet (M03+ adds those), so
`RequireRole`'s behavior is proven with a minimal test-only router in
`internal/auth/middleware_test.go` rather than by inventing a fake business
endpoint.

## Endpoints

| Method | Path | Auth | Notes |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | none | Always creates a `CUSTOMER`. 201 / 409 (duplicate email) / 422 (validation). |
| `POST` | `/api/v1/auth/login` | none | 200 with `{"token": "..."}` / 401 (generic "invalid email or password" for both wrong password and unknown email) / 422 (missing fields). |
| `GET` | `/api/v1/users/me` | Bearer token | Returns the caller's own profile. 401 if missing/invalid/expired token. |

`GET /users/me` (not `GET /auth/me`) is the canonical current-user endpoint
per the frozen architecture — `/auth/*` is reserved for credential
lifecycle actions (register, login), not resource retrieval.

Its handler lives in `internal/auth`, not `internal/users`, despite the URL.
`internal/auth` already depends on `internal/users` for the repository;
putting the handler in `internal/users` would require `users` to import
`auth` right back for the `Identity` type — a real import cycle. Rather
than invent a third package to hold just a context helper, the handler
stays next to the rest of the auth flow it reads from. M03 can relocate it
when it adds `PUT /users/me` and other profile-management endpoints.

## Frontend

- `contexts/AuthContext.tsx` + `hooks/useAuth.ts`: token, current user, and
  `status` (`loading` / `authenticated` / `unauthenticated`), plus
  `login`/`register`/`logout` actions.
- `components/ProtectedRoute.tsx`: redirects to `/login` when
  unauthenticated. This is a UX convenience, not a security boundary — the
  backend's middleware is authoritative. Editing frontend state cannot grant
  access to anything the backend wouldn't otherwise allow, because every
  protected page's data still comes from an API call the backend checks
  independently.
- Token storage: `sessionStorage`. Simple, needs no backend cookie/CSRF
  handling, survives a page refresh within the tab — but, like any
  JavaScript-readable storage, it is vulnerable to token theft via XSS. A
  production system would use an httpOnly cookie instead, which trades that
  weakness for real added complexity (`Set-Cookie` handling, `SameSite`/CSRF
  protection) that this project's scope doesn't call for. This is a
  deliberate, bounded tradeoff, stated plainly rather than glossed over.

## What this module deliberately does not include

OAuth, social login, Redis-backed sessions, refresh tokens, a permissions
database, a policy engine, an external identity provider, MFA, or email
verification. The assignment asks for role-based authentication; this
module builds that, not an identity platform.
