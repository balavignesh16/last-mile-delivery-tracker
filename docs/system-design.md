# System Design

## Architecture

The system is a Go modular monolith behind a single versioned REST API
(`/api/v1`), backed by PostgreSQL, with a React/TypeScript frontend. Each
business capability lives in its own `internal/` package — `auth`,
`users`, `agents`, `zones`, `rates`, `orders`, `tracking`, `assignment`,
`rescheduling`, `notifications` — mounted onto one shared chi router.
Packages depend downward only (e.g. `orders` calls into `rates`;
`rates` never calls back into `orders`), and where a later module needs
to observe an earlier one without creating a cycle, it does so through a
small, producer-owned callback type (`TransitionHook`,
`OrderCreatedHook`) rather than an event bus. No microservices, no
message queue, no cache layer — every capability is proven independently
testable through this same discipline instead.

## Rate Calculation Engine

Given pickup/drop areas, dimensions, weight, order type, and payment
type, `rates.CalculateQuote` produces the exact charge in one pass:
resolve both areas to zones, classify the route INTRA or INTER,
compute volumetric weight (`L×B×H÷5000`), take the chargeable weight as
the greater of actual and volumetric, look up the one active rate card
for `(order_type, zone_relationship)`, select the `[min, max)` slab
covering that weight, and add the card's COD surcharge if the order is
COD. Configuration (rate cards, slabs) is entirely admin-managed and
never hardcoded; the engine itself is pure computation with no
persistence side effect, so `POST /orders/quote` and order creation call
the identical function and can never drift apart. One active card per
combination and non-overlapping slabs are enforced by partial unique
indexes and `SELECT ... FOR UPDATE`, proven under concurrent writes.

## Zone Detection

An `area` belongs to exactly one `zone`; resolving a pickup/drop area to
its zone is a single indexed lookup, not geocoding or distance
estimation — the assignment never asked for real-world coordinates to
decide routing tier. INTRA/INTER is a direct comparison of the two
resolved zone ids. An inactive zone is rejected explicitly rather than
silently priced, since a deactivated zone should never route or price a
delivery.

## Order Lifecycle & Immutable Tracking

Every order moves through a closed eight-state machine
(`CREATED → ASSIGNED → PICKED_UP → IN_TRANSIT → OUT_FOR_DELIVERY →
DELIVERED`, with `FAILED → RESCHEDULED → ASSIGNED` as the recovery
cycle). A fixed table maps each edge to the roles allowed to perform it
— `ADMIN` broadly, `DELIVERY_AGENT` only the five in-flight edges,
`CUSTOMER` none directly. Every transition is written, never updated or
deleted, as a new row in an append-only `order_tracking_events` table
carrying the previous status, new status, actor, and timestamp, so the
full history is reconstructable and tamper-evident. A `SELECT ... FOR
UPDATE` lock on the order row serializes racing transition attempts;
proven with concurrent goroutines against real Postgres.

## Auto-Assignment

Manual assignment lets an admin name an agent directly, subject to an
eligibility check (active, `AVAILABLE`, has a current zone). Automatic
assignment ranks every eligible agent deterministically: same-zone
agents before cross-zone agents, then oldest `last_assigned_at` first,
with agent UUID as a final tiebreak — pure, side-effect-free ranking
over data already read under lock, so the same inputs always produce
the same choice. Both paths write through the identical, already-locked
transition path the state machine itself owns, never a second copy of
it. A partial unique index guarantees an agent can never hold two
simultaneously active assignments, even if application logic were
bypassed.

## Failed Delivery, Rescheduling & Notifications

A `FAILED` status is reached through the same state machine, carrying
an optional failure reason in its event metadata. Because the product
requires customer-initiated rescheduling but the state machine
authorizes only `ADMIN` on `FAILED → RESCHEDULED`, the rescheduling
module authorizes the request itself (ownership or admin) and then
invokes the transition as a pre-authorized internal call — the real
actor is still recorded faithfully, and zero lines of the state machine
changed. The previously assigned agent is freed back to `AVAILABLE`
inside that same transaction; reassignment is a deliberate, separate
call into the same assignment path any fresh order uses. All eight
lifecycle events, including repeated `FAILED` occurrences after a
reschedule cycle, notify the customer by email (always) and SMS (if a
phone is on file) as a synchronous, post-commit side effect —
idempotent per exact tracking-event occurrence, never per event type,
so a genuine second failure is never mistaken for a duplicate.

## Security & Dashboards

JWT-based auth carries role and identity; every endpoint pairs
route-level role gating with a handler-level ownership check, and
mismatched ownership returns 404, never 403, to avoid confirming a
resource's existence. Client-supplied prices, statuses, and actor ids
are never trusted — every derived field is computed or read from the
authenticated identity. The customer, agent, and admin dashboards are a
thin navigation and read-only composition layer over this same API,
including admin order filtering and simple status counts — they
introduce no new backend module, table, or business rule.
