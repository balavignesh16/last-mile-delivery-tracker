# User & Agent Management (M03)

## Domain separation

```
users              = identity/profile (all three roles)
delivery_agents    = operational state, delivery agents only
```

`delivery_agents` never duplicates `full_name`/`email`/`phone`/`role` —
those stay in `users`. Every API response that describes an agent is a
join of the two tables (see `AgentWithUser` in `internal/agents`), never a
copy.

## Schema

```sql
CREATE TABLE delivery_agents (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL UNIQUE REFERENCES users(id),
    availability         TEXT NOT NULL DEFAULT 'OFFLINE'
                          CHECK (availability IN ('AVAILABLE', 'BUSY', 'OFFLINE')),
    current_lat          DOUBLE PRECISION CHECK (current_lat BETWEEN -90 AND 90),
    current_lng          DOUBLE PRECISION CHECK (current_lng BETWEEN -180 AND 180),
    current_zone_id      UUID,
    location_updated_at  TIMESTAMPTZ,
    last_assigned_at     TIMESTAMPTZ,
    active               BOOLEAN NOT NULL DEFAULT true,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`user_id UNIQUE` enforces one agent profile per user at the database
level — not just checked in application code. `current_zone_id` has no
foreign key yet: `zones` doesn't exist until M04, and this migration must
not forward-reference a table that isn't there. M04 adds the constraint
when it adds the table. No `ON DELETE CASCADE` on the `users` foreign
key — there is no user-deletion endpoint anywhere in this project, and the
safer default is that a user row can never be deleted out from under an
agent record that still references it.

## `active` vs `availability`

Two independent flags, not one:

- **`availability`** — the agent's current operational state:
  `AVAILABLE` / `BUSY` / `OFFLINE`. Changes often, agent- or admin-managed.
- **`active`** — whether the agent is operationally enabled at all.
  Rarely changes, admin-managed (M03 exposes it for visibility; no
  endpoint to *change* it yet — that's the first plausible M09-adjacent
  addition, not built ahead of need).

`active = false` with `availability = AVAILABLE` must **not** make an
agent eligible for assignment once M09's candidate-selection algorithm
exists — M03 establishes the data and the distinction; M09 owns the
eligibility check itself.

## Agent provisioning

`POST /api/v1/agents` is the only way a `DELIVERY_AGENT` account is ever
created (see `docs/authentication.md` for why `ADMIN` accounts have no
creation endpoint at all). It creates a `users` row and a
`delivery_agents` row in one transaction — if either insert fails, both
roll back. No orphaned `delivery_agents` row is ever left behind, and no
`users` row is ever created without its matching agent record. Verified
directly in `tests/integration/agents_integration_test.go`
(`TestAgentCreation_TransactionalAtomicity`), which forces the failure
case (duplicate email) and confirms the agent count is unchanged
afterward.

Password hashing and input validation reuse `internal/auth`'s exported
`HashPassword`/`ValidateRegistration`/`NormalizeEmail` — agent creation's
input shape is identical to registration's (email, password, full name,
optional phone), so the same rules apply without duplicating them.

## Two-layer authorization

Every write endpoint under `/agents` is protected twice:

1. **Route-level role gate** (`internal/agents/routes.go`) — coarse: is
   this role allowed to call this endpoint *at all*.
2. **Handler-level ownership check** — fine: for `DELIVERY_AGENT`, is the
   `{id}` in the URL the caller's *own* agent record.

The second layer exists because the first alone cannot stop Agent A from
editing Agent B's data — both hold the identical `DELIVERY_AGENT` role, so
a role check can't distinguish them. Only comparing `identity.UserID`
against the looked-up agent's `UserID` can. This is the direct fix for the
IDOR (insecure direct object reference) class of bug the task's security
review explicitly calls out, and it's tested directly:
`TestUpdateAvailabilityHandler_AnotherAgentForbidden` and
`TestUpdateLocationHandler_AnotherAgentForbidden` in
`internal/agents/handler_test.go`, plus the integration-level
`TestCreateAgent_DeliveryAgentCannotCreateAnotherAgent`.

Availability additionally allows `ADMIN` (operational oversight is a
reasonable admin capability); location does not, even for `ADMIN` — there
is no stated operational reason for anyone but the agent to report their
own position, and extending that reach wasn't asked for.

## Why `GET /api/v1/agents/me` exists

A discovery gap surfaced while building the frontend: an agent's JWT
carries their `user_id`, but every write endpoint is keyed by the agent's
own `id` (the `delivery_agents` primary key) — and nothing gave the
frontend a way to learn that `id`. `GET /agents/me` closes that gap, the
same way `GET /users/me` does for identity. `internal/agents.Repository`
gained a `FindByUserID` method to back it.

## Forward compatibility with M09

Fields M09's candidate-selection algorithm will read are already present
and typed for that purpose: `availability` (constrained to exactly the
three values M09 will filter on), `active`, `current_zone_id`,
`current_lat`/`current_lng` (`float64`, matching Haversine-distance
arithmetic), and `last_assigned_at` (a natural tie-break signal — earliest
`last_assigned_at` first, for fair round-robin — though M03 does not write
to this field; that starts in M09). `location_updated_at` is always set
from the database's own clock (`now()`), never a client-supplied value,
so it can be trusted later for staleness checks without re-verifying it.

M03 explicitly does **not** implement: `SELECT ... FOR UPDATE SKIP
LOCKED`, Haversine ranking, or either assignment path (manual or
automatic) — those are M09's algorithm, not this module's data layer.
