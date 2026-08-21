# Failed Delivery & Rescheduling (M10)

## Scope

M10 owns exactly two endpoints — `POST /api/v1/orders/:id/reschedule`
and `GET /api/v1/orders/:id/reschedules` — and exactly one new table
(`reschedule_requests`). It does not own the order status state machine
(M08), agent ranking/assignment (M09), or notifications (M11 — not yet
built). The `OUT_FOR_DELIVERY → FAILED` transition itself, and the
`FAILED → RESCHEDULED` and `RESCHEDULED → ASSIGNED` edges it relies on,
already existed, unmodified, before this module was written — M10 adds
*what happens around* those edges, never the edges themselves. See
"M08 integration" and "M09 integration" below.

## Failure

An order reaches `FAILED` exactly as it always has, through M08's own,
unmodified `POST /orders/:id/status` (`ADMIN` or `DELIVERY_AGENT`,
`OUT_FOR_DELIVERY → FAILED`). M10 adds no "mark failed" endpoint of its
own — there is no second way to fail a delivery, and none was needed.

## Failure reason

`transitionRequest` (M08's own DTO) already carries a free-form,
optional `metadata` field for *every* transition, including this one —
nothing about M10 was required to make this possible. A caller marking
an order `FAILED` can already attach a reason today:

```bash
curl -X POST http://localhost:8080/api/v1/orders/$ORDER_ID/status \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"FAILED","metadata":{"failure_reason":"Recipient not available"}}'
```

M10's only job here was to verify this already worked (it does — see
`docs/order-tracking.md` and this module's own integration test,
`TestRescheduleFlow_CustomerHappyPath`, which asserts a failure-reason
metadata payload survives end to end), document it, and build on it. No
`internal/tracking` file was touched to support this.

## Customer rescheduling

`POST /api/v1/orders/:id/reschedule` — `CUSTOMER` (their own order
only) or `ADMIN` (any order). Body: `{"requested_date": "YYYY-MM-DD",
"reason": "optional"}`. A reschedule request is **immediately
effective** once created — there is no pending/approved/rejected
workflow, and no endpoint to cancel or reject one.

### The architectural mismatch, and how it's resolved without touching M08

The product requirement is "a customer can reschedule." M08's own,
unmodified authorization matrix (`edgeAuthorizedRoles[{FAILED,
RESCHEDULED}]`) authorizes only `ADMIN` for the underlying transition,
and the `POST /orders/:id/status` route itself excludes `CUSTOMER`
entirely. M10 does not resolve this by widening M08's matrix or route —
that file was not touched (verified: `git diff` against
`internal/tracking/statemachine.go` and `routes.go` is empty for this
module).

Instead, `RescheduleHandler` (`internal/rescheduling/handler.go`) is
M10's own, independent authorization gate: it loads the order, checks
`CUSTOMER` ownership or `ADMIN`, and only then calls
`rescheduling.Repository.Reschedule`. That repository always calls
`tracking.Repository.TransitionTx` with `role = users.RoleAdmin` —
**not** the real caller's role — because by the time the repository
runs, M10 has already independently verified the caller is allowed to
do this. `TransitionTx`'s `role` parameter exists purely for its own
internal authorization check; it is never persisted anywhere. The
*separate* `actorID` parameter — always the real, authenticated
caller's user id — is what gets written to
`order_tracking_events.actor_id`, so the permanent record always
faithfully shows who actually requested the reschedule, customer or
admin, never a role name.

This is why authorization and identity are handled as two separate
concerns in this module: M10 asserts its own authorization once
(handler-level), then supplies the real actor separately from the
authorization value TransitionTx consults. Zero lines changed in
`internal/tracking`.

## Reschedule request persistence

A dedicated table, not a reuse of tracking-event metadata alone:

```sql
CREATE TABLE reschedule_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders(id),
    requested_by    UUID NOT NULL REFERENCES users(id),
    requested_date  DATE NOT NULL,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reschedule_requests_order_id ON reschedule_requests (order_id);
```

Deliberately **no** `status`, `approved_at`, `rejected_at`, or any other
approval-workflow field — there is no approval workflow to model.
Deliberately no `ON DELETE CASCADE` — this project has no orders- or
users-deletion endpoint, so plain `NO ACTION` is safe and consistent
with every other FK in this schema. The same `requested_date`/`reason`
also land in the paired tracking event's own `metadata`
(`{"requested_date": "...", "reason": "..."}`), so a reschedule's
context is visible from either `GET /orders/:id/reschedules` or `GET
/orders/:id/tracking` — not duplicated logic, just the same values
written to both places inside the one atomic transaction that creates
them.

## FAILED → RESCHEDULED transition

Reused, never reimplemented. `internal/rescheduling.Repository.Reschedule`
calls `tracking.Repository.TransitionTx` inside its own transaction —
the exact same call shape `internal/assignment` already established for
`→ASSIGNED` in M09. M10 never calls `UPDATE orders SET status = ...`
and never calls `INSERT INTO order_tracking_events` directly; both
remain exclusively `internal/tracking`'s job.

## Agent availability after failure

**Approved decision**: the agent who was servicing an order when it
failed is freed back to `AVAILABLE` — but not at the instant `FAILED`
is set. `OUT_FOR_DELIVERY → FAILED` still goes through M08's own,
unmodified endpoint, which this module does not wrap, intercept, or
hook — no second generic transition path exists, and none was added.
Freeing happens as part of M10's own reschedule transaction instead:
when `POST /orders/:id/reschedule` is called, `Reschedule` locks the
order's `assigned_agent_id` (read non-locking beforehand, safe because
nothing can write it while the order sits at `FAILED` — see "M09
integration" below), locks that agent's row (`SELECT ... FOR UPDATE`,
the same pattern `internal/assignment`'s own `lockCandidate` already
established against the identical table), and sets `availability =
AVAILABLE` — all inside the same transaction as the tracking transition
and the `reschedule_requests` insert.

**Consequence, and why it's a deliberate, documented choice**: a `FAILED`
order's agent remains `BUSY` until *someone actually reschedules it* —
not automatically the instant it fails. This was the only way to
satisfy "free the agent" without touching M08's `TransitionHandler`,
`routes.go`, or state machine (all explicitly frozen for this module),
and without inventing a database trigger or other hidden mechanism this
project has never used elsewhere. Every prior module's own concurrency
and side-effect logic lives in explicit, transaction-scoped Go code,
never a trigger — M10 keeps that convention. `active`, `current_zone_id`,
`current_lat`, and `current_lng` are never touched by this operation —
only `availability`.

Deliberately **not** cleared: `orders.assigned_agent_id`. It remains a
historical/current-attempt snapshot — the same "snapshot, not derived"
precedent the column's own M09 migration comment established — until
M09's own assignment path overwrites it on the next successful
`assign`/`auto-assign` call.

## Reassignment through M09

M10 never calls `assignment.Repository` and never performs candidate
selection. After a successful reschedule, the order simply sits at
`RESCHEDULED` — the exact same status `RESCHEDULED → ASSIGNED` (M09,
unmodified, already legal since M08) already knows how to consume.
Reassignment remains a **separate**, subsequent call to `POST
/orders/:id/assign` or `POST /orders/:id/auto-assign`, mirroring how a
freshly `CREATED` order already requires its own separate assignment
call today — creation and assignment have never been coupled in this
project, and rescheduling doesn't couple them either.

Because M10 only frees the agent's `availability` (never touches M09's
`IsEligible`/`SelectCandidate`/ranking), a freed agent is reconsidered
by auto-assignment on exactly the same terms as any other agent — same
zone-match, same `last_assigned_at` ordering, same UUID tiebreak. If a
*different* agent has since become more eligible (e.g. the previously
assigned agent was manually taken `BUSY` again on unrelated work before
the next auto-assign call), M09's own, unmodified ranking picks that
other agent — nothing in M10 special-cases "prefer" or "avoid" the
previous agent. This is proven directly by
`TestRescheduleFlow_PreviouslyAssignedAgentNotIncorrectlyReused`.

## Tracking history

`GET /orders/:id/tracking` (M08, unchanged) shows the `FAILED` and
`RESCHEDULED` events exactly as it always has — same actor/timestamp
capture, same immutability, same chronological ordering. M10 adds no
new tracking-event shape, only a `metadata` payload convention
(`requested_date`, `reason`) for the one event type it triggers.

## Customer/Admin permissions

| Endpoint | CUSTOMER | ADMIN | DELIVERY_AGENT | Unauthenticated |
|---|---|---|---|---|
| `POST /orders/:id/reschedule` | own order only | any order | `403` | `401` |
| `GET /orders/:id/reschedules` | own order only | any order | `403` | `401` |

Ownership mismatch for `CUSTOMER` is `404`, never `403` — the same
hide-existence convention `GET /orders/{id}` and `GET
/orders/:id/tracking` already use.

## Delivery-agent restrictions

`DELIVERY_AGENT` has no role in rescheduling at all — not requesting
one, not approving one, not viewing history. This mirrors the product
requirement directly (only "customer can reschedule" is stated
anywhere) and the project's own precedent of excluding agents from
`GET /orders/:id/tracking` even after M09 widened other order-visibility
rules. An agent's only involvement in this lifecycle remains marking an
order `FAILED` in the first place — an M08 capability, unchanged.

## Notification boundary

M10 does **not** send notifications of any kind — no email, no SMS, no
push. `docs/order-tracking.md`'s and `docs/assignment-engine.md`'s own
notification boundary applies identically here: a `FAILED` or
`RESCHEDULED` event exists and is queryable via `GET
/orders/:id/tracking`; *acting* on that event to notify anyone is M11's
job, not yet built. M10 imports nothing from, and depends on nothing in,
any notification package.

## Delivery attempts

No `delivery_attempts` table exists, and none was added. The blueprint's
own field list for it (`attempt_id`, `agent_id`, `attempt_number`,
`status`, `failure_reason`, `started_at`, `completed_at`) is, on
inspection, already representable from existing, unmodified tracking
history:

- **Which agent serviced a given attempt** — the `assigned_agent_id`
  metadata on that attempt's own `ASSIGNED` tracking event.
- **When an attempt started/completed** — the `ASSIGNED` event's
  `created_at` and the terminal (`DELIVERED`/`FAILED`) event's
  `created_at` for that same attempt.
- **Failure reason** — the `FAILED` event's own `metadata` (see
  "Failure reason" above).
- **Attempt number** — derived, not stored: counting how many
  `→ASSIGNED` transitions an order's own `GET /orders/:id/tracking`
  history contains gives the attempt number directly (the first
  `CREATED→ASSIGNED` is attempt 1; each subsequent
  `RESCHEDULED→ASSIGNED` is the next attempt). This is never persisted
  as a mutable counter anywhere — it's arithmetic over an already
  immutable, already-correct event log, computed at read time, exactly
  the same way `docs/order-management.md` already avoided a redundant
  `packages` table and `docs/assignment-engine.md` already avoided a
  redundant `order_assignments` table.

## Security model

| Threat | How it's closed |
|---|---|
| Mass assignment (`actor_id`, `requested_by`, `order_id`, `status`, `created_at`, `requested_at`) | `rescheduleRequest` has exactly two fields (`requested_date`, `reason`); `DisallowUnknownFields` rejects the whole request (`422`) |
| Privilege escalation (`DELIVERY_AGENT` rescheduling) | Route-level `RequireRole(ADMIN, CUSTOMER)` — the same two-gate pattern (route + handler ownership) every RBAC-scoped route in this project uses |
| A customer rescheduling another customer's order | Ownership check in the handler, `404` on mismatch — never confirms an order exists under an id the caller doesn't own |
| Server-derived actor identity | `actor_id`/`requested_by` always `auth.IdentityFromContext`'s `UserID`, never a request field |
| Client-controlled status/timestamps | No such fields exist on the DTO; `status`/`previous_status`/`created_at` are entirely `TransitionTx`'s and Postgres's own output |
| SQL injection | Every query in `internal/rescheduling/repository.go` is parameterized |
| Tampering with reschedule/tracking history | No update/delete route exists for either table |

## Concurrency

Two reschedule requests racing the same `FAILED` order are serialized
by the same `SELECT ... FOR UPDATE` lock `TransitionTx` already takes on
the order row — the second re-reads the now-`RESCHEDULED` status and
fails `IsValidTransition`, `409`. A reschedule racing an M09 assignment
attempt that happens to target the same (being-freed) agent is
serialized by `freeAgent`'s own `SELECT ... FOR UPDATE`, acquired
**before** the order lock — the same agent-then-order lock ordering
`internal/assignment` already established, reused here rather than
inventing a different sequence, which is what actually rules out a
cross-module deadlock. Proven with real concurrent goroutines against
real Postgres, repeated (`-count=5`):
`TestRescheduleConcurrency_SameOrderRacedTwice` and
`TestRescheduleConcurrency_DoesNotDeadlockWithAssignment`.

## What this module deliberately does not do

- No approval/rejection workflow, no reschedule-cancellation endpoint.
- No `delivery_attempts` table — see "Delivery attempts" above.
- No modification to M08's state machine, transition validation, or
  authorization matrix.
- No modification to M09's candidate ranking, eligibility rule, or
  assignment endpoints.
- No automatic reassignment — `RESCHEDULED → ASSIGNED` remains a
  separate, subsequent `POST /orders/:id/assign`/`auto-assign` call.
- No notifications (M11) or dashboards/analytics (M12).

## Frontend

- **`OrderDetailPage`** — a "Reschedule delivery" section, visible to
  `CUSTOMER` and `ADMIN` once `order.status === 'FAILED'`: a native
  `<input type="date">` (no date-picker dependency added, per the
  project's own "keep dependencies minimal" convention), an optional
  reason field, a submit button, and loading/error/success states,
  mirroring the exact structure `handleManualAssign`/`handleAutoAssign`
  (M09) already established. A "Reschedule history" section, visible to
  the same `ADMIN`/`CUSTOMER` scope as the tracking timeline (never
  `DELIVERY_AGENT` — this page still never even calls `GET
  /orders/:id/tracking` for an agent viewer, and the reschedule-history
  fetch is gated by the identical flag).
- **`services/rescheduling.ts`**, **`types/reschedule.ts`** — new,
  minimal, one-file-per-module precedent matching `services/assignment.ts`.
- Reused, not duplicated: `getOrder`/the `Order` type (already carries
  `status: 'RESCHEDULED'` and `assigned_agent_id` since M07/M09), the
  same `ErrorBanner`/success-banner pattern every other write action on
  this page already uses.

## Testing

- **Unit** (`internal/rescheduling/reschedule_test.go`): every date
  validation boundary (valid future date, today/same-day, yesterday,
  clearly past, malformed, empty, leap day, year boundary) —
  `ParseRequestedDate`/`ValidateRescheduleDate` are pure, deterministic,
  and take "today" as an injected parameter rather than reading the
  clock themselves.
- **Unit** (`internal/rescheduling/handler_test.go`, fake `Repository` +
  fake `orders.Repository`): every RBAC/ownership/validation/mass-
  assignment case, and an explicit assertion that the real caller's
  identity — never a request-body value — is what reaches the
  repository as `actor_id`.
- **Integration** (`tests/integration/rescheduling_integration_test.go`,
  against real Postgres): customer and admin happy paths, reschedule-
  record and tracking-event persistence (including `actor_id` and
  `metadata` content), failure-reason metadata surviving end to end,
  ownership/RBAC/IDOR, non-`FAILED` and unknown-order rejection, invalid
  date and unknown-field rejection, a rejected repeat reschedule leaving
  no orphan row, the full `FAILED→RESCHEDULED→ASSIGNED→...→FAILED→RESCHEDULED`
  cycle working twice, deterministic chronological history ordering,
  agent-freeing behavior, a subsequent M09 auto-assign correctly
  re-selecting a freed agent (or a different, more eligible one), and
  both concurrency races proven with real goroutines, repeated
  (`-count=5`).
- **Frontend**: role-gated reschedule form visibility, native date input
  behavior, request-payload assertions, loading/error/success states,
  reschedule-history rendering and its own empty/error states, refresh
  after a successful reschedule.
- **Regression**: the complete M01–M09 backend and frontend suites
  re-verified green after every change in this module — confirmed via
  `git diff` showing zero changes to any `internal/tracking` or
  `internal/assignment` file.
