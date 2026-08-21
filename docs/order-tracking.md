# Order Tracking & Status Lifecycle (M08)

## Scope

M08 owns the order status state machine and its immutable event log —
`POST /api/v1/orders/:id/status` (perform one transition) and `GET
/api/v1/orders/:id/tracking` (retrieve the full history). It does not
own pricing (M06), order persistence/ownership (M07), agent assignment
(M09 — now implemented, see `docs/assignment-engine.md`, without
modifying anything in this module), or reschedule-date capture (M10 —
not yet built). See "What this module deliberately does not do" below.

## State machine

The blueprint's own diagram, implemented exactly as eight edges:

```
CREATED → ASSIGNED
ASSIGNED → PICKED_UP
PICKED_UP → IN_TRANSIT
IN_TRANSIT → OUT_FOR_DELIVERY
OUT_FOR_DELIVERY → DELIVERED
OUT_FOR_DELIVERY → FAILED
FAILED → RESCHEDULED
RESCHEDULED → ASSIGNED
```

`DELIVERED` has no outgoing edges — fully terminal. Every pair not
listed above is illegal: same-status transitions (no self-loops
exist), backward jumps, and skipped jumps, including the blueprint's
own named counterexample, `CREATED → DELIVERED`.

`internal/tracking.IsValidTransition(from, to)` is the single source
of truth for this — a plain lookup table
(`internal/tracking/statemachine.go`), not a hand-rolled sequence of
`if` statements, so the full edge set is visible at a glance and
`TestIsValidTransition_EveryOtherPairIsIllegal` can assert every
64-pair truth table entry exhaustively rather than hand-picking
counterexamples.

## Authorization matrix

A **per-edge** table, not a per-route role check — the same actor
(`ADMIN`) is authorized everywhere, while `DELIVERY_AGENT` is
authorized only for the five agent-tier edges:

| Edge | Authorized roles |
|---|---|
| `CREATED → ASSIGNED` | `ADMIN` |
| `ASSIGNED → PICKED_UP` | `ADMIN`, `DELIVERY_AGENT` |
| `PICKED_UP → IN_TRANSIT` | `ADMIN`, `DELIVERY_AGENT` |
| `IN_TRANSIT → OUT_FOR_DELIVERY` | `ADMIN`, `DELIVERY_AGENT` |
| `OUT_FOR_DELIVERY → DELIVERED` | `ADMIN`, `DELIVERY_AGENT` |
| `OUT_FOR_DELIVERY → FAILED` | `ADMIN`, `DELIVERY_AGENT` |
| `FAILED → RESCHEDULED` | `ADMIN` |
| `RESCHEDULED → ASSIGNED` | `ADMIN` |

`CUSTOMER` has no status-transition authority anywhere in M08 — the
route itself (`routes.go`) only admits `ADMIN`/`DELIVERY_AGENT` to
`POST /orders/:id/status`, so a `CUSTOMER` gets `403` before the
handler, let alone the per-edge table, is ever consulted.

**Resolved by M09, deliberately not here**: M08 itself still performs
*no* ownership check against `assigned_agent_id` — `Transition`'s
authorization matrix above is unchanged, exactly as the finalized M09
decisions required ("M08 remains the single source of truth for order
lifecycle transitions... do not modify M08's authorization matrix").
Any authenticated `DELIVERY_AGENT` can still perform an agent-tier edge
on any order via `POST /orders/:id/status` directly. What M09 actually
added is a *second*, narrower path into the same edges — an agent's
own frontend only ever shows orders `GET /orders` already scopes to
them (see `docs/assignment-engine.md`'s "DELIVERY_AGENT order
visibility") — not a change to this endpoint's own authorization.

## Why the per-edge check lives inside `Transition`, not the handler

Route-level RBAC (`ADMIN`/`DELIVERY_AGENT` may call the endpoint at
all) is necessary but not sufficient — which *specific* edge a caller
may use depends on the order's *current* status, and that isn't known
until the row is locked. `Repository.Transition` performs both the
transition-legality check and the role-authorization check inside the
same locked transaction that reads the current status — the same
reasoning M05's slab-overlap validation uses for needing a
consistent, locked read before deciding. A handler-level pre-check
would either need a separate, racy read first, or would have to
duplicate the lock itself.

## Concurrency

Two callers racing conflicting transitions on the same order (e.g.
simultaneous `OUT_FOR_DELIVERY → DELIVERED` and `OUT_FOR_DELIVERY →
FAILED`) is closed by `SELECT ... FROM orders WHERE id = $1 FOR
UPDATE` inside the same transaction that validates and writes —
identical mechanism to M05's `lockRateCard`. The full sequence:

1. Begin transaction.
2. `SELECT status FROM orders WHERE id = $1 FOR UPDATE` — locks the row.
3. Check `IsValidTransition(current, requested)` — `409` if not.
4. Check `IsRoleAuthorized(current, requested, role)` — `403` if not.
5. `UPDATE orders SET status = $1`.
6. `INSERT INTO order_tracking_events (...)`.
7. Commit.

A second concurrent request blocks at step 2 until the first commits,
then re-reads the *already-changed* status — its own validation runs
against reality, not a stale read. Proven under real concurrent
goroutine load against real Postgres in
`TestConcurrentTransition_OnlyOneWins` (`tests/integration`): eight
simultaneous requests split between `→DELIVERED` and `→FAILED`, exactly
one commits, the rest get `409`, and exactly one terminal tracking
event exists afterward — not merely asserted by calling the handler
twice in sequence. Also verified manually (see the M08 completion
report) by firing two real, simultaneous curl requests.

## The initial tracking event

The blueprint's own worked example opens an order's timeline with a
`CREATED` entry — order *creation* is itself the first tracking event,
not something a later call produces. `internal/orders.CreateOrder`
therefore runs inside a transaction (added by M08; M07 originally
didn't need one) that inserts both the `orders` row and the paired
`order_tracking_events` row (`previous_status = NULL`, `new_status =
'CREATED'`, `actor_id` = the actual authenticated creator — the
customer's own id for a self-placed order, the acting admin's id for
an admin-created one) atomically. An order can never exist without its
opening event, and no orphaned event can reference a nonexistent
order.

This is the **one** place `internal/orders` writes to
`order_tracking_events` directly. Every later transition belongs to
`internal/tracking`.

## Schema

```sql
CREATE TABLE order_tracking_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID NOT NULL REFERENCES orders(id),
    previous_status  TEXT CHECK (previous_status IS NULL OR previous_status IN (...)),
    new_status       TEXT NOT NULL CHECK (new_status IN (...)),
    actor_id         UUID NOT NULL REFERENCES users(id),
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_tracking_events_order_id ON order_tracking_events (order_id);
```

`previous_status` is nullable — `NULL` only for the initial event;
every subsequent row always has one. Both status columns reuse
`orders.status`'s exact value list (migration `0008`) rather than a
new enum. `orders.status` itself needed **no** `ALTER` — its `CHECK`
constraint already covered the full M08 value set since M07.
`metadata` is free-form, optional `JSONB` — M08 defines no required
shape for it; a later module may attach its own context (a failure
reason, a reschedule date) without this table changing.

No `assigned_agent_id`, no `slab_id`, and no update/delete path exists
anywhere for this table — it is genuinely append-only, not just
append-only by convention. `TestTrackingEvents_NoUpdateOrDeleteRouteExists`
confirms a `PUT`/`DELETE` against either M08 route returns the
router's generic `405`, because no such route is registered at all.

## API

See `docs/api.md`'s "Order tracking" section for full request/response
examples. In short:

| Endpoint | Role |
|---|---|
| `POST /api/v1/orders/:id/status` | `ADMIN`, `DELIVERY_AGENT` (per-edge authorized, see matrix); `CUSTOMER` → `403` |
| `GET /api/v1/orders/:id/tracking` | `ADMIN` (any order), `CUSTOMER` (own only, else `404`); `DELIVERY_AGENT` → `403` |

## Security model

| Threat | How it's closed |
|---|---|
| Mass assignment (`actor_id`, `previous_status`, `id`, `created_at`, `order_id`) | None of these fields exist on `transitionRequest`; `DisallowUnknownFields` rejects the whole request |
| Privilege escalation (an agent performing an admin-only edge) | Per-edge authorization inside `Transition`, checked under lock |
| IDOR on `GET /orders/:id/tracking` | Ownership check (`customer_id == identity.UserID` for `CUSTOMER`), `404` on mismatch — never `403`, never reveals existence |
| Tampering with history | No update/delete route exists; the table is insert-only from the application's perspective |
| SQL injection | Every query in `internal/tracking/repository.go` is parameterized |

## What this module deliberately does not do

- No `assigned_agent_id` column, no assignment creation, no
  candidate-ranking/distance logic, no marking an agent `BUSY` — all
  M09 (`POST /orders/:id/assign`, `POST /orders/:id/auto-assign`, see
  `docs/assignment-engine.md`). M09 calls into this module's own
  `TransitionTx` for the `CREATED→ASSIGNED`/`RESCHEDULED→ASSIGNED`
  writes rather than duplicating them — M08 remains the only place
  that validates a transition or writes `orders.status`.
- No reschedule-date capture, no customer-initiated reschedule request
  flow — M10 (`POST /orders/:id/reschedule`,
  `GET /orders/:id/reschedules`). M08 only needs the
  `FAILED → RESCHEDULED` *edge* to be legal and loggable.
- No email/SMS notification on any transition — M11.
- No status/zone/agent filtering on any listing endpoint, no analytics
  or dashboard views over tracking events.
- No agent-facing order list/detail UI — see "Frontend" below.

## Frontend

`OrderDetailPage` (existing since M07) gained two things:

- A tracking timeline, visible to both `CUSTOMER` (own orders) and
  `ADMIN` (any order) — `GET /orders/:id/tracking`, rendered oldest
  first, matching the endpoint's own ordering.
- An `ADMIN`-only status-transition control, showing buttons only for
  the legal next statuses from the order's current status (mirroring
  `internal/tracking`'s own edge table in
  `frontend/src/types/tracking.ts`'s `LEGAL_TRANSITIONS` — a UX
  convenience, not a security boundary; the backend re-validates and
  re-authorizes every transition regardless of what the frontend
  offers, the same disclaimer `ProtectedRoute` already carries for
  role-based routing).

**Was deliberately no agent-facing UI in M08** — `GET /orders` excluded
`DELIVERY_AGENT` entirely, since giving every agent a list of every
customer's order would be exactly the kind of overexposure M07's RBAC
was designed to avoid, and scoping it to "an agent's *own* orders"
needed the assignment relationship M09 later added. **M09 now provides
that entry point** (`OrdersPage`'s "My assigned orders" view,
`OrderDetailPage`'s order-detail breakdown) — see
`docs/assignment-engine.md`'s "Frontend" section. `GET
/orders/:id/tracking` itself is **still** `ADMIN`/`CUSTOMER`-only,
unchanged by M09 (this module's own route-level RBAC was never
widened) — `OrderDetailPage` accounts for this by never calling that
endpoint for a `DELIVERY_AGENT` viewer, rather than surfacing its `403`
as an error. This module's own `LEGAL_TRANSITIONS`-driven status-update
control also remains `ADMIN`-only in the frontend, unchanged; an agent
still has API-level transition authority per the matrix above without a
corresponding frontend control, which remains out of this module's
scope to add.

## Testing

- **Unit** (`internal/tracking/statemachine_test.go`,
  `handler_test.go`): every legal edge accepted, an exhaustive 8×8
  truth table proving every other pair is rejected (including
  same-status and the blueprint's `CREATED→DELIVERED` counterexample),
  the full role×edge authorization matrix, handler-level validation/
  RBAC/ownership/mass-assignment/actor/timestamp/metadata cases,
  chronological-order preservation.
- **Integration** (`tests/integration/tracking_integration_test.go`):
  the initial-event-on-creation behavior (including actor = customer
  vs. actor = admin depending on who created the order), the full
  happy-path lifecycle, the `FAILED→RESCHEDULED→ASSIGNED` path, invalid
  jump/same-status/unknown-status rejection, the full RBAC matrix
  against the real router, customer IDOR on tracking, immutability
  (`405` on `PUT`/`DELETE`), FK/CHECK constraint tests on the real
  schema, and the concurrent-transition race proof.
- **Frontend**: timeline rendering, loading/empty/error states, the
  admin control only offering legal next states, request-shape
  assertions, and a full transition-then-refresh round trip.
- **Regression**: the complete M01–M07 backend and frontend suites
  re-verified green after every change in this module, including a
  repeated (`-count=3`) run of every concurrency test (M05's two plus
  M08's one) to rule out flakiness.
