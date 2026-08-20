# API Reference

All business endpoints are versioned under `/api/v1`. `GET /health` is the
one exception — infrastructure, not business API, so it stays unversioned
at the root (see `docs/authentication.md` and the README for why).

## Conventions

- **Auth header**: `Authorization: Bearer <token>` on every endpoint marked "Auth: required".
- **Content type**: `application/json` for every request and response body.
- **Errors**: always `{"error": "human-readable message"}`, never a raw Go error, SQL error, or stack trace.
- **Status codes**:

  | Code | Meaning |
  |---|---|
  | 200 | Success (read or update) |
  | 201 | Resource created |
  | 401 | Missing, malformed, invalid, or expired token |
  | 403 | Authenticated, but the role/ownership check failed |
  | 404 | No resource at that ID |
  | 409 | Conflict — duplicate email |
  | 422 | Validation failure (bad input shape, out-of-range value, unknown field) |
  | 500 | Unexpected server error (never exposes internal detail) |

---

## Health

### `GET /health`

**Auth**: none. **Purpose**: liveness + database connectivity check.

```bash
curl http://localhost:8080/health
```

```json
{ "status": "ok", "database": "ok" }
```

---

## Authentication (M02)

### `POST /api/v1/auth/register`

**Auth**: none. **Purpose**: public self-registration — always creates a `CUSTOMER`. There is no `role` field; sending one is rejected outright (422), not silently ignored.

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"password123","full_name":"Jane Doe","phone":"555-0100"}'
```

```json
{
  "id": "8d56f3df-9039-4c98-9aae-cfb8c486dc61",
  "email": "jane@example.com",
  "full_name": "Jane Doe",
  "phone": "555-0100",
  "role": "CUSTOMER",
  "created_at": "2026-08-20T15:25:40Z"
}
```

**Errors**: `409` (email already registered), `422` (invalid email, password under 8 characters, missing full name, or an unrecognized field like `role`).

### `POST /api/v1/auth/login`

**Auth**: none. **Purpose**: exchange credentials for a JWT (24h expiry, HS256).

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"password123"}'
```

```json
{ "token": "eyJhbGciOiJIUzI1NiIs..." }
```

**Errors**: `401` with the exact same message (`"invalid email or password"`) whether the email doesn't exist or the password is wrong. `422` if either field is missing.

---

## User profile (M02 + M03)

### `GET /api/v1/users/me`

**Auth**: required (any role). **Purpose**: the caller's own profile.

```bash
curl http://localhost:8080/api/v1/users/me -H "Authorization: Bearer $TOKEN"
```

```json
{
  "id": "8d56f3df-9039-4c98-9aae-cfb8c486dc61",
  "email": "jane@example.com",
  "full_name": "Jane Doe",
  "phone": "555-0100",
  "role": "CUSTOMER",
  "created_at": "2026-08-20T15:25:40Z"
}
```

### `PUT /api/v1/users/me` (M03)

**Auth**: required (any role). **Purpose**: update the caller's own `full_name`/`phone`. Nothing else is editable — the request body has no `id`/`email`/`role`/`password_hash` field at all, and sending one is rejected outright (422), the same fail-closed pattern as registration.

```bash
curl -X PUT http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"full_name":"Jane A. Doe","phone":"555-0199"}'
```

Response: same shape as `GET /users/me`, with the updated fields.

**Errors**: `401` (no/invalid token), `422` (empty name, or an unrecognized/protected field present).

---

## Delivery agents (M03)

Agent accounts are never self-registered — they only ever come from `POST
/agents`, admin-only. There is no endpoint that creates an `ADMIN` account
at all (see `docs/authentication.md`'s demo-seed explanation).

### `POST /api/v1/agents`

**Auth**: required, **ADMIN only**. **Purpose**: provision a `DELIVERY_AGENT` — creates a `users` row and a `delivery_agents` row atomically (one transaction; if either insert fails, neither is kept). No `role` field on the request, same reasoning as registration.

```bash
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"email":"agent2@example.com","password":"password123","full_name":"Agent Two","phone":"555-0200"}'
```

```json
{
  "id": "3f9c...",
  "user_id": "a1b2...",
  "full_name": "Agent Two",
  "email": "agent2@example.com",
  "phone": "555-0200",
  "availability": "OFFLINE",
  "current_lat": null,
  "current_lng": null,
  "current_zone_id": null,
  "location_updated_at": null,
  "last_assigned_at": null,
  "active": true,
  "created_at": "2026-08-20T16:00:00Z"
}
```

**Errors**: `401` (unauthenticated), `403` (authenticated as CUSTOMER or DELIVERY_AGENT), `409` (email taken), `422` (invalid input).

### `GET /api/v1/agents`

**Auth**: required, **ADMIN only**. **Purpose**: list every agent, joined with identity fields. No filters, no pagination — M03's explicit scope is "no search/filter system," not an oversight.

```bash
curl http://localhost:8080/api/v1/agents -H "Authorization: Bearer $ADMIN_TOKEN"
```

Response: a JSON array of the same shape `POST /agents` returns.

### `GET /api/v1/agents/me`

**Auth**: required, **DELIVERY_AGENT only**. **Purpose**: the agent's self-lookup — the equivalent of `GET /users/me`, needed because a JWT only carries the caller's *user* ID, and every other agent endpoint is keyed by the agent's own `id`. The frontend calls this once to learn which agent record is "theirs" before it can call the two endpoints below.

```bash
curl http://localhost:8080/api/v1/agents/me -H "Authorization: Bearer $AGENT_TOKEN"
```

**Errors**: `404` if the caller has no agent record (shouldn't happen in practice — every DELIVERY_AGENT is created with one — but handled defensively).

### `PUT /api/v1/agents/{id}/availability`

**Auth**: required, **ADMIN or DELIVERY_AGENT**. **Purpose**: set `AVAILABLE`/`BUSY`/`OFFLINE`. An agent may only manage their own record — attempting another agent's `{id}` is `403`, checked inside the handler (not just by role), which is what actually prevents one agent editing another's state (both hold the same role, so a role check alone can't distinguish them). ADMIN may manage any agent.

```bash
curl -X PUT http://localhost:8080/api/v1/agents/$AGENT_ID/availability \
  -H "Authorization: Bearer $AGENT_TOKEN" -H 'Content-Type: application/json' \
  -d '{"availability":"AVAILABLE"}'
```

**Errors**: `401`, `403` (wrong role, or a DELIVERY_AGENT targeting another agent's `{id}`), `404` (unknown `{id}`), `422` (anything other than `AVAILABLE`/`BUSY`/`OFFLINE`).

### `PUT /api/v1/agents/{id}/location`

**Auth**: required, **DELIVERY_AGENT only** (not even ADMIN — no operational reason for anyone but the agent to report their own position). Self-only, same ownership check as availability.

```bash
curl -X PUT http://localhost:8080/api/v1/agents/$AGENT_ID/location \
  -H "Authorization: Bearer $AGENT_TOKEN" -H 'Content-Type: application/json' \
  -d '{"latitude":12.9716,"longitude":77.5946}'
```

Sets `current_lat`, `current_lng`, and `location_updated_at` (from the database's own clock — never a client-supplied timestamp).

**Errors**: `401`, `403` (another agent's `{id}`, or a non-agent role), `404`, `422` (latitude outside ±90, longitude outside ±180, missing coordinate, or a non-finite value).

---

## What's not here yet

Zones, rate cards, orders, tracking, assignment, rescheduling, notifications, and dashboards — M04 through M12. This file grows with each module.
