# Assignment Engine (M09)

## Scope

M09 owns delivery-agent assignment — `POST /api/v1/orders/:id/assign`
(manual) and `POST /api/v1/orders/:id/auto-assign` (automatic). It owns
exactly one schema change (`orders.assigned_agent_id`) and no new table.
It does not own pricing (M06), order persistence/ownership (M07), or the
order status state machine (M08) — every status change this module
causes goes through M08's own `tracking.Repository.TransitionTx`,
unmodified. See "M08 reuse" below.

Explicitly out of scope: reschedule-request handling (M10 — now
implemented, see `docs/failed-delivery.md`, reuses this module's own
`Assign`/`AutoAssign` for `RESCHEDULED→ASSIGNED` unmodified rather than
duplicating them), notifications (M11), dashboards/analytics, geographic-distance ranking (no coordinate
data exists anywhere in the schema for orders/zones/areas — only
`delivery_agents.current_lat`/`current_lng`, which nothing resolves a
pickup point against), an assignment-history table, status-preserving
reassignment of an already-`ASSIGNED` order, and a candidate-preview
endpoint.

## Eligibility

One rule, used identically by both manual and automatic assignment —
`internal/assignment.IsEligible`:

```go
func IsEligible(c Candidate) bool {
    return c.Active && c.Availability == agents.AvailabilityAvailable && c.CurrentZoneID != nil
}
```

`active = true`, `availability = AVAILABLE`, and a resolvable
`current_zone_id`. An admin cannot deliberately assign a `BUSY`,
`OFFLINE`, or inactive agent — manual assignment re-checks this exact
function under the same row lock automatic assignment's candidate
filtering uses, so the two paths can never diverge on what "eligible"
means.

## Auto-assignment ranking

No coordinates exist anywhere in this schema for orders, zones, or
areas — only `delivery_agents.current_lat`/`current_lng`, which nothing
ever resolves a pickup point against. True geographic-distance ranking
is therefore not computable, and this module does not invent one.
Ranking (`internal/assignment.SelectCandidate`) is a deterministic,
pure function over four ordered criteria, applied only after filtering
to eligible candidates:

1. **Same-zone preferred over cross-zone.** An agent whose
   `current_zone_id` matches the order's `pickup_zone_id` ranks ahead of
   every agent that doesn't. Cross-zone is never excluded — only ranked
   lower — since zone-match is the only locality signal this schema
   actually provides.
2. **`last_assigned_at` ascending, `NULL` first.** An agent who has
   never been assigned, or has been idle longest, is preferred — simple
   fair rotation.
3. **Agent UUID ascending** — the final, unconditional tiebreaker. This
   is what makes `SelectCandidate` a genuine strict total order: given
   the same candidate pool, it returns the same winner regardless of
   the pool's input order (`TestSelectCandidate_Deterministic` proves
   this directly, feeding the same five candidates through four
   different orderings).

`SelectCandidate` is pure and database-free — no `pgx`, no `context`,
just `[]Candidate` in, one `Candidate` out or `ErrNoEligibleCandidate`.
It is independently unit-tested (`internal/assignment/candidate_test.go`)
without any Postgres dependency, the same "pure decision function,
separately-tested database plumbing" split `rates.SelectSlab` and
`tracking.IsValidTransition` established.

## M08 reuse — non-negotiable

M09 never reimplements `IsValidTransition`, never reimplements the
per-edge role matrix, never writes `orders.status` directly, and never
inserts an `order_tracking_events` row directly. Every status change —
manual or automatic — goes through the exact same call:

```go
r.trackingRepo.TransitionTx(ctx, tx, orderID, actorID, actorRole, tracking.StatusAssigned, assignmentMetadata(agentID))
```

`TransitionTx` (M08's `Transition`, refactored to accept a caller-
supplied `pgx.Tx` instead of always opening its own — a minimal,
additive change verified to leave M08's own full test suite passing
unchanged) is the single place that validates the transition is legal,
checks role authorization, updates `orders.status`, and inserts the
tracking event. M09 supplies only the metadata payload:

```json
{"assigned_agent_id": "<uuid>"}
```

This is also why there is no separate assignment-history table:
`order_tracking_events` already recorded every `ASSIGNED` transition
before M09 existed; M09 just started putting something meaningful in
that event's `metadata` column.

## Why no status-preserving reassignment

M08's state machine has no `ASSIGNED → ASSIGNED` self-loop — reassigning
an order that is already `ASSIGNED` to a different agent without first
changing its status is structurally impossible without modifying M08,
which the finalized M09 decisions explicitly forbid. The only
reassignment path that exists is the one M08 already had:
`FAILED → RESCHEDULED → ASSIGNED`. `Assign`/`AutoAssign` naturally
support this cycle because it is just another `→ASSIGNED` transition —
no separate reassignment endpoint or reassignment-specific logic exists.

## Manual assignment (`Assign`)

1. Confirm the order exists (`orders.ErrOrderNotFound` → `404` if not).
2. Begin a transaction.
3. `SELECT ... FOR UPDATE` the named agent row (`lockCandidate`) —
   `404` if no such agent exists.
4. Re-check `IsEligible` against the locked row — `409` if it fails.
5. `TransitionTx` the order to `ASSIGNED` (M08's own validation/
   authorization/write, unmodified).
6. Write `orders.assigned_agent_id` and mark the agent `BUSY` with
   `last_assigned_at = now()`.
7. Re-select the full order row inside the same transaction and commit.

Any failure at any step rolls the entire transaction back — no partial
state (an order marked `ASSIGNED` with no `assigned_agent_id`, or an
agent marked `BUSY` with no order referencing them) is ever observable.

## Automatic assignment (`AutoAssign`)

1. Read the order (non-locking) and fast-fail via
   `tracking.IsValidTransition` reused directly (a cheap pre-check —
   the real, race-safe enforcement still happens inside `TransitionTx`).
2. Bulk-read every agent (`agentsRepo.List`) and rank the pool with
   `SelectCandidate`.
3. Begin a transaction. Lock the top-ranked candidate and re-check
   `IsEligible` under the lock.
4. **If the top candidate was raced away** (eligible at the bulk read,
   not eligible under lock — e.g. another request assigned them in the
   interim) — drop it from the pool and retry `SelectCandidate` with the
   remainder, looping until a locked-and-eligible candidate is found or
   the pool is exhausted (`ErrNoEligibleCandidate`, `409`). Auto-
   assignment never guesses or falls back to an ineligible agent.
5. Same `TransitionTx` → write assignment → re-select → commit sequence
   as manual assignment.

## Concurrency

Four races, all closed by PostgreSQL row locks inside one transaction
per assignment attempt, proven against real Postgres (not mocks) with
concurrent goroutines, repeated (`-count=5`) to rule out flakiness:

| Race | Protection |
|---|---|
| Two admins manually assign the same order to two different agents | Both attempts call `TransitionTx`, which locks the *order* row (`SELECT ... FOR UPDATE` inside `tracking.Repository`); the second to arrive re-reads the already-`ASSIGNED` status and gets `409` |
| Manual assignment races auto-assignment on the same order | Same mechanism — whichever transaction's `TransitionTx` call commits first wins; the other's re-read sees the new status and fails |
| Two different orders both try to assign the same agent | Both attempts lock the *agent* row (`lockCandidate`'s `SELECT ... FOR UPDATE`) before touching the order; the second blocks until the first commits, then re-checks `IsEligible` under lock and sees `BUSY` — `409` |
| Assignment races an unrelated M08 status transition on the same order | Both go through `TransitionTx`'s own order-row lock — no separate code path exists for M09 to bypass it |

**Lock ordering.** Every assignment code path — `Assign` and
`AutoAssign` alike — acquires the agent lock first, then the order lock
(via `TransitionTx`), in that exact same sequence. This consistent
ordering is what actually prevents a circular-wait deadlock between two
concurrent assignment transactions; locking alone isn't sufficient if
two transactions could take the same two locks in opposite order.

**Database-level backstop.** Migration `0010` adds a partial unique
index:

```sql
CREATE UNIQUE INDEX idx_orders_one_active_assignment_per_agent
    ON orders (assigned_agent_id)
    WHERE status IN ('ASSIGNED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY');
```

Same philosophy as M05's `idx_rate_cards_one_active_per_combination`: the
application-level locking above is the primary mechanism, and this index
is what actually guarantees the invariant even if some future code path
bypassed it — an agent can never end up with two simultaneously active
assigned orders, enforced by the database itself, not just by
convention.

Proven in `tests/integration/assignment_integration_test.go`:
`TestAssignmentConcurrency_SameOrderRacedByTwoAdmins` and
`TestAssignmentConcurrency_SameAgentRacedByTwoOrders`, each firing two
real concurrent goroutines against real Postgres and asserting exactly
one success, the loser gets `409` (not a hang, not a corrupted write),
and — for the same-agent race — a direct `count(*)` query against
`orders` confirms exactly one active assignment exists for that agent
afterward.

## Schema

```sql
ALTER TABLE orders ADD COLUMN assigned_agent_id UUID REFERENCES delivery_agents(id);
CREATE INDEX idx_orders_assigned_agent_id ON orders (assigned_agent_id);
CREATE UNIQUE INDEX idx_orders_one_active_assignment_per_agent
    ON orders (assigned_agent_id)
    WHERE status IN ('ASSIGNED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY');
```

`assigned_agent_id` is current-state-only — nullable, holds only the
*currently* assigned agent. Assignment history (which agent was
assigned when, by whom) lives entirely in `order_tracking_events`'s own
`metadata` on each `ASSIGNED` event, which M08 already made immutable
and append-only. No new table was needed to add that history.

## DELIVERY_AGENT order visibility

`GET /orders` and `GET /orders/{id}` (`internal/orders`) widened to
admit `DELIVERY_AGENT`, scoped strictly to their own assigned orders:

- `ListOrdersHandler` resolves the caller's own `delivery_agents.id`
  from their JWT's user id (`agentsRepo.FindByUserID` — the same
  problem M03's `GET /agents/me` already solved), then calls
  `ListOrdersForAgent(agentID)`. A `DELIVERY_AGENT` with no agent
  record (shouldn't happen — M03 provisions both atomically — but fails
  safe) gets an empty list, not an error.
- `GetOrderHandler` checks `order.AssignedAgentID == agent.ID`; a
  mismatch is `404`, the same hide-existence convention `CUSTOMER`
  ownership already uses on this endpoint.

`ADMIN` (all orders) and `CUSTOMER` (own orders only) visibility is
unchanged. An agent is never shown every customer's order — only the
ones assigned to them.

`GET /orders/:id/tracking` (M08) is **not** widened — it remains
`ADMIN`/`CUSTOMER`-only, unmodified, per the finalized decision not to
touch M08's own authorization. See `docs/order-tracking.md`.

## Security model

| Threat | How it's closed |
|---|---|
| Mass assignment (`status`, `assigned_agent_id` on the request, or any field beyond `agent_id`) | `assignRequest` has exactly one field; `DisallowUnknownFields` rejects the whole request (`422`) |
| Privilege escalation (`CUSTOMER`/`DELIVERY_AGENT` assigning an order) | Both routes are `RequireRole(ADMIN)`-only — no per-edge nuance the way M08's transition endpoint needs, since assignment has exactly one authorized actor |
| Assigning a deliberately ineligible agent | `IsEligible` re-checked under lock on every path, including manual assignment where an admin names the agent directly — the check runs regardless of who selected the agent |
| IDOR on `GET /orders/{id}` for `DELIVERY_AGENT` | Ownership check against `assigned_agent_id`, `404` on mismatch — never confirms an order exists under an id the caller isn't assigned to |
| An agent seeing another customer's or agent's orders | `ListOrdersForAgent` filters at the SQL layer (`WHERE assigned_agent_id = $1`), never a client-suppliable filter |
| SQL injection | Every query in `internal/assignment/repository.go` is parameterized |

## What this module deliberately does not do

- No geographic-distance ranking or coordinate infrastructure — the
  schema has none to rank against (see "Auto-assignment ranking").
- No assignment-history table — `order_tracking_events.metadata`
  already provides it.
- No status-preserving reassignment of an `ASSIGNED` order, no separate
  reassignment endpoint — only the existing
  `FAILED → RESCHEDULED → ASSIGNED` cycle.
- No candidate-preview endpoint — the frontend's manual-assignment
  picker reuses M03's existing `GET /agents` listing directly.
- No changes to M08's state machine, transition validation, or
  authorization matrix.
- No reschedule-request handling (M10, see `docs/failed-delivery.md`),
  notifications (M11), or dashboards.

## Frontend

- **`OrdersPage`** — `DELIVERY_AGENT` sees "My assigned orders" (same
  component, same `GET /orders` call; the backend decides the scope,
  the frontend never sends a filter) with the "New order" action hidden
  for that role (agents cannot create orders).
- **`OrderDetailPage`** — an "Assigned agent" field in the order
  breakdown for every role (agent's name once resolved, or
  "Unassigned"); for `ADMIN`, on an order in `CREATED` or `RESCHEDULED`
  status, an "Assign delivery agent" section: a `<select>` populated by
  reusing M03's existing `listAgents` call (no new candidate-preview
  endpoint), an "Assign" button, and an "Auto-assign" button, with
  success/error banners. `DELIVERY_AGENT` never sees this section, and
  — since `GET /orders/:id/tracking` stays `ADMIN`/`CUSTOMER`-only —
  never sees the tracking timeline either; the page skips that call
  entirely for that role rather than surfacing its `403`.
- **`App.tsx`** — `/orders` and `/orders/:id` widened to admit
  `DELIVERY_AGENT`; `/orders/new` stays `ADMIN`/`CUSTOMER`-only.
- **`Layout`** — a "My Assigned Orders" nav link for `DELIVERY_AGENT`,
  alongside the existing "Operations" link (availability/location, M03).
- Reused deliberately, not duplicated: `listAgents`
  (`services/agents.ts`), `getOrder`/the `Order` type (widened with
  `assigned_agent_id`), `getOrderTracking`.

## Testing

- **Unit** (`internal/assignment/candidate_test.go`): every eligibility
  exclusion (inactive, `BUSY`, `OFFLINE`, no zone), same-zone-preferred,
  cross-zone-accepted-when-no-same-zone-candidate,
  `last_assigned_at` ordering including `NULL`-first, the UUID
  tiebreak, no-eligible-candidate returning `ErrNoEligibleCandidate`,
  and determinism across four different input orderings of the same
  pool.
- **Unit** (`internal/assignment/handler_test.go`, fake `Repository`):
  admin happy path for both endpoints, `CUSTOMER`/`DELIVERY_AGENT`/
  unauthenticated rejection, missing/unknown-field/malformed body,
  every sentinel-error-to-status-code mapping (404/409/500).
- **Integration** (`tests/integration/assignment_integration_test.go`,
  against real Postgres): manual and auto-assign happy paths, same-zone-
  wins and cross-zone-accepted at the HTTP level, every ineligibility
  case (inactive/`BUSY`/`OFFLINE`/no-zone), unknown agent/order, full
  RBAC matrix, an already-`ASSIGNED` order rejected, a `DELIVERED`
  (terminal) order rejected, the `FAILED→RESCHEDULED→ASSIGNED` path
  working end to end, no-eligible-candidate rejected, a rejected
  assignment leaving no partial state (order status/agent availability
  both unchanged, no stray tracking event), and both concurrency races
  proven with real goroutines against real Postgres, repeated
  (`-count=5`).
- **Frontend**: `services/assignment.test.ts` (request shape, error
  mapping); `OrderDetailPage.test.tsx` (assigned-agent display,
  `ADMIN`-only controls hidden from `CUSTOMER`, controls hidden once the
  order leaves an assignable status, manual assign posts the right body
  and refreshes, auto-assign posts and refreshes, error banner on
  rejection, a `DELIVERY_AGENT` viewing their own assigned order without
  the tracking timeline); `OrdersPage.test.tsx` ("My assigned orders"
  title, no "New order" link for `DELIVERY_AGENT`).
- **Regression**: the complete M01–M08 backend and frontend suites
  re-verified green after every change in this module.
