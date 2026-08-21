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
  why `POST /orders/quote` is the one authoritative pricing path a later
  order-confirmation step is expected to reuse (M06).

The rest don't exist yet — the root [README.md](../README.md) carries each
completed module's summary directly, and links into this directory as
each file is written.
