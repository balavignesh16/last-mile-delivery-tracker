# Documentation

This directory holds detailed, per-module documentation as each module
lands: `architecture.md`, `database.md`, `rate-engine.md`,
`assignment-engine.md`, `order-lifecycle.md`, `failed-delivery.md`,
`notifications.md`, `testing.md`, and the required `system-design.md`
write-up (≤ 800 words).

Written so far:

- [`api.md`](./api.md) — full endpoint reference (request/response
  examples, auth, roles, errors) for every route that exists so far.
- [`authentication.md`](./authentication.md) — roles, the users table,
  password hashing, JWT design, RBAC middleware, and the frontend auth
  flow (M02).
- [`user-agent-management.md`](./user-agent-management.md) — the
  delivery_agents schema, `active` vs `availability`, transactional agent
  provisioning, the two-layer IDOR protection on agent endpoints, and
  M09 forward-compatibility notes (M03).
- [`zone-management.md`](./zone-management.md) — the zones/areas schema
  and hierarchy, why there's no DELETE, address/area resolution and why
  it isn't geocoding, INTRA/INTER determination, inactive-zone behavior,
  and completing the `delivery_agents.current_zone_id` foreign key (M04).
- [`rate-configuration.md`](./rate-configuration.md) — the rate_cards/
  rate_card_slabs schema, why cards start inactive, flat-per-band
  pricing, the `[min, max)` boundary convention, why slabs (unlike every
  other config entity) can be deleted, one-active-card-per-combination,
  and the two concurrency mechanisms that protect activation and slab
  writes under real concurrent load (M05).
- [`rate-calculation.md`](./rate-calculation.md) — the M06 quote engine:
  volumetric/chargeable weight, the `[min, max)` slab-selection
  algorithm, COD surcharge application, why nothing is persisted, the
  narrow customer-facing zone/area read-RBAC widening it required, and
  why `POST /orders/quote` is the one authoritative pricing path M07's
  order creation reuses (M06).
- [`order-management.md`](./order-management.md) — the orders schema,
  why there's no `packages` table, customer-vs-admin creation and
  ownership, the pricing snapshot (and why `rate_card_id` is safe on
  orders but `slab_id` isn't), why every order starts and only M07 ever
  writes `CREATED`, and exactly how order creation reuses M06's
  `rates.CalculateQuote` without a second pricing implementation (M07).
- [`order-tracking.md`](./order-tracking.md) — the M08 state machine
  (every legal edge and why every other pair is rejected), the per-edge
  authorization matrix, the `SELECT ... FOR UPDATE` concurrency
  mechanism and its proof, why order creation itself writes the first
  tracking event, the `order_tracking_events` schema, and exactly what
  was deliberately left to M09/M10/M11 (M08).
- [`assignment-engine.md`](./assignment-engine.md) — the M09 manual and
  automatic assignment endpoints, the eligibility rule and deterministic
  ranking algorithm (and why no geographic-distance ranking exists), why
  M08's state machine is reused rather than duplicated, the four
  concurrency races and their PostgreSQL-level protection, the
  `orders.assigned_agent_id` schema addition, and the widened
  `DELIVERY_AGENT` order-visibility rules (M09).
- [`failed-delivery.md`](./failed-delivery.md) — the M10 reschedule
  endpoints, the architectural mismatch between "customer can
  reschedule" and M08's ADMIN-only transition matrix and how it's
  resolved without modifying M08, the `reschedule_requests` schema,
  agent-availability behavior after a failure, why reassignment stays a
  separate M09 call, and why no `delivery_attempts` table exists (M10).
- [`notifications.md`](./notifications.md) — the M11 Notification
  Service: the eight lifecycle events, why the customer is the only
  recipient, the email/SMS provider abstraction and its log-based MVP
  implementation, the post-commit hook pattern that avoids an import
  cycle with M07/M08/M09/M10, failure containment, why idempotency is
  anchored on the exact `tracking_event_id` (not `(order_id, event,
  channel)`), and why there is no REST API, no frontend UI, no
  retries, and no queues/workers (M11).

The rest don't exist yet — the root [README.md](../README.md) carries each
completed module's summary directly, and links into this directory as
each file is written.
