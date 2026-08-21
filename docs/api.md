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

## Zones and areas (M04, read access widened in M06)

Geographic configuration: `zones` are the top-level unit, `areas` belong to
exactly one zone. Every mutation endpoint below is **ADMIN only**; the three
`GET` endpoints are **ADMIN or CUSTOMER** (widened in M06 so a customer can
pick a pickup/drop area when requesting a quote — see
`docs/zone-management.md`'s RBAC section for the full reasoning). `DELIVERY_AGENT`
gets `403` on every endpoint in this section.

### `POST /api/v1/zones`

**Auth**: required, **ADMIN only**. **Purpose**: create a zone.

```bash
curl -X POST http://localhost:8080/api/v1/zones \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"North"}'
```

```json
{ "id": "1f2e...", "name": "North", "active": true, "created_at": "2026-08-20T16:00:00Z" }
```

**Errors**: `401`, `403`, `409` (name already taken), `422` (empty/whitespace name, or name over 100 characters).

### `GET /api/v1/zones`

**Auth**: required, **ADMIN or CUSTOMER**. **Purpose**: list every zone, active or not — no filter for "active only" exists yet.

```bash
curl http://localhost:8080/api/v1/zones -H "Authorization: Bearer $TOKEN"
```

### `GET /api/v1/zones/{id}`

**Auth**: required, **ADMIN or CUSTOMER**. **Errors**: `404` if unknown.

### `PUT /api/v1/zones/{id}`

**Auth**: required, **ADMIN only**. **Purpose**: rename a zone and/or toggle `active`. This is also how a zone is activated/deactivated — there is no separate endpoint for that. `name` is always required (send the zone's current name back if you only mean to change `active`); `active` is optional — omitting it leaves the current value unchanged.

```bash
curl -X PUT http://localhost:8080/api/v1/zones/$ZONE_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"North","active":false}'
```

**Errors**: `401`, `403`, `404`, `409` (renamed to a name already taken), `422`.

### `POST /api/v1/zones/{zoneID}/areas`

**Auth**: required, **ADMIN only**. **Purpose**: create an area under the zone named in the URL. The request body has no `zone_id` field — sending one is rejected outright (422, unknown field), the same fail-closed pattern as agent creation having no `role` field. The zone in the path is the only source of the relationship.

```bash
curl -X POST http://localhost:8080/api/v1/zones/$ZONE_ID/areas \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Downtown"}'
```

```json
{ "id": "9a1b...", "name": "Downtown", "zone_id": "1f2e...", "created_at": "2026-08-20T16:05:00Z" }
```

**Errors**: `401`, `403`, `404` (unknown zone), `409` (an area with this name already exists *in this zone* — the same name is fine in a different zone), `422`.

### `GET /api/v1/zones/{zoneID}/areas`

**Auth**: required, **ADMIN or CUSTOMER**. **Purpose**: list the areas belonging to one zone — this is what populates the pickup/drop area pickers on the M06 quote form. `404` if the zone itself doesn't exist (distinct from `200` with an empty array, which means the zone exists but has no areas yet — the frontend needs that distinction for its empty state).

### `PUT /api/v1/zones/{zoneID}/areas/{areaID}`

**Auth**: required, **ADMIN only**. **Purpose**: rename an area. Moving an area to a different zone is not supported — this endpoint only renames. If `{areaID}` exists but belongs to a different zone than `{zoneID}` names, the response is `404` (treated identically to "doesn't exist," not a more specific error, so this endpoint never confirms an area's existence under the wrong zone).

**Errors**: `401`, `403`, `404`, `409` (name taken within the zone), `422`.

---

## Rate cards and slabs (M05)

Admin-configured pricing profiles, one per `(order_type,
zone_relationship)` combination, plus their weight-tiered slabs. Every
endpoint below is **ADMIN only**, including `GET` — see
`docs/rate-configuration.md` for the full design (schema, `[min, max)`
boundary convention, concurrency, why cards can't be deleted but slabs
can). **These endpoints only store configuration — no calculation
happens here.** Selecting a slab for a chargeable weight, applying COD
surcharge, and producing a quote are all M06 (`POST /orders/quote`,
below) — the customer never sees a rate card or slab directly, only the
quote they produce.

### `POST /api/v1/rates`

**Auth**: required, **ADMIN only**. **Purpose**: create a rate card. New cards always start `"active": false` — there is no `active` field on this request at all; activate one via `PUT` once it's ready.

```bash
curl -X POST http://localhost:8080/api/v1/rates \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"order_type":"B2B","zone_relationship":"INTRA","cod_surcharge":15}'
```

```json
{ "id": "4f1a...", "order_type": "B2B", "zone_relationship": "INTRA", "cod_surcharge": 15, "active": false, "created_at": "2026-08-21T10:00:00Z" }
```

**Errors**: `401`, `403`, `422` (invalid `order_type`/`zone_relationship`, negative `cod_surcharge`, or an unrecognized field like `active`).

### `GET /api/v1/rates`

**Auth**: required, **ADMIN only**. **Purpose**: list every rate card, active or not.

### `GET /api/v1/rates/{id}`

**Auth**: required, **ADMIN only**. **Errors**: `404` if unknown.

### `PUT /api/v1/rates/{id}`

**Auth**: required, **ADMIN only**. **Purpose**: update `cod_surcharge` and/or toggle `active` — this is also how a card is activated/deactivated; there is no separate endpoint. `order_type`/`zone_relationship` are not editable (a card's identity). `cod_surcharge` is always resent (send the current value back if only toggling `active`); `active` is optional — omitting it leaves the current value unchanged.

```bash
curl -X PUT http://localhost:8080/api/v1/rates/$RATE_CARD_ID \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"cod_surcharge":15,"active":true}'
```

**Errors**: `401`, `403`, `404`, `409` (activating this card would leave two active cards for the same `order_type`+`zone_relationship`), `422`.

### `POST /api/v1/rates/{rateCardID}/slabs`

**Auth**: required, **ADMIN only**. **Purpose**: create a weight slab under the rate card named in the URL. The request body has no `rate_card_id` field — sending one is rejected outright (422, unknown field), the same fail-closed pattern as area creation having no `zone_id` field. Omit `max_weight` for an open-ended top slab (at most one per card).

```bash
curl -X POST http://localhost:8080/api/v1/rates/$RATE_CARD_ID/slabs \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"min_weight":0,"max_weight":5,"price":50}'
```

```json
{ "id": "9c3e...", "rate_card_id": "4f1a...", "min_weight": 0, "max_weight": 5, "price": 50, "created_at": "2026-08-21T10:05:00Z" }
```

A chargeable weight in `[min_weight, max_weight)` costs exactly `price` — a flat amount, never multiplied by weight.

**Errors**: `401`, `403`, `404` (unknown rate card), `409` (overlaps an existing slab in this card, or would be a second open-ended slab), `422` (missing/negative `min_weight`/`price`, or `max_weight` not greater than `min_weight`).

### `GET /api/v1/rates/{rateCardID}/slabs`

**Auth**: required, **ADMIN only**. **Purpose**: list the slabs belonging to one rate card. `404` if the rate card itself doesn't exist (distinct from `200` with an empty array).

### `PUT /api/v1/rates/{rateCardID}/slabs/{slabID}`

**Auth**: required, **ADMIN only**. **Purpose**: update a slab's `min_weight`/`max_weight`/`price`. If `{slabID}` exists but belongs to a different rate card than `{rateCardID}` names, the response is `404` — treated identically to "doesn't exist."

**Errors**: `401`, `403`, `404`, `409` (overlap/open-ended conflict), `422`.

### `DELETE /api/v1/rates/{rateCardID}/slabs/{slabID}`

**Auth**: required, **ADMIN only**. **Purpose**: remove a slab. Unlike zones/areas/rate cards, slabs may be deleted outright — nothing else in the schema references an individual slab, so removing a misconfigured one can't orphan anything (see `docs/rate-configuration.md`). Same path-ownership check as `PUT`: wrong `{rateCardID}` → `404`. Returns `204 No Content` on success.

**Errors**: `401`, `403`, `404`.

---

## Rate calculation / quotes (M06)

Given pickup/drop areas, package dimensions, actual weight, order type,
and payment type, calculates the exact charge a customer would pay —
zone resolution (M04) → rate card selection (M05) → volumetric/chargeable
weight → slab selection → COD surcharge. See `docs/rate-calculation.md`
for the full design (weight formula, `[min, max)` slab-selection
algorithm, why nothing is persisted here).

### `POST /api/v1/orders/quote`

**Auth**: required, **ADMIN or CUSTOMER** (`DELIVERY_AGENT` → `403`). **Purpose**: calculate a quote. Stateless — nothing is created or persisted; calling this twice with the same input always recomputes fresh against whatever rate configuration is active right now. The request body has no field for anything the backend derives itself (`customer_id`, `pickup_zone_id`, `drop_zone_id`, `zone_relationship`, `rate_card_id`, `volumetric_weight`, `chargeable_weight`, `base_rate`, `cod_surcharge`, `final_amount`, `status`) — sending one is rejected outright (`422`, unknown field), the same fail-closed pattern every other module in this API uses.

```bash
curl -X POST http://localhost:8080/api/v1/orders/quote \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"pickup_area_id":"...","drop_area_id":"...","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":7}'
```

```json
{
  "pickup_area_id": "...", "pickup_zone_id": "...",
  "drop_area_id": "...", "drop_zone_id": "...",
  "zone_relationship": "INTRA",
  "order_type": "B2C", "payment_type": "COD",
  "length_cm": 10, "breadth_cm": 10, "height_cm": 10, "actual_weight_kg": 7,
  "volumetric_weight_kg": 0.2, "chargeable_weight_kg": 7,
  "rate_card_id": "...",
  "base_rate": 80, "cod_surcharge": 20, "final_amount": 100
}
```

Dimensions are assumed centimeters, weight kilograms — the universal
convention for the `L × B × H ÷ 5000` volumetric formula, though never
stated explicitly in the source documents (flagged, not invented, in
`docs/rate-calculation.md`).

**Errors**: `401`, `403` (`DELIVERY_AGENT` or unauthenticated), `422` for every one of: malformed body, unknown field, non-positive/non-finite dimension or weight, invalid `order_type`/`payment_type`, an unknown `pickup_area_id`/`drop_area_id`, a pickup/drop area whose zone is inactive, no active rate card for the resolved `(order_type, zone_relationship)`, or no slab covering the calculated chargeable weight.

---

## Orders (M07)

Order persistence and retrieval. Pricing is never computed here — every
request below calls M06's `rates.CalculateQuote` internally and stores
its result as an immutable snapshot; see `docs/order-management.md` for
the full design (customer-vs-admin creation, ownership, why there's no
`packages` table, why `orders.rate_card_id` is safe but `slab_id` isn't).

### `POST /api/v1/orders`

**Auth**: required, **ADMIN or CUSTOMER** (`DELIVERY_AGENT` → `403`). **Purpose**: create and price an order. The accepted request shape depends on the caller's role:

- **CUSTOMER** — `pickup_area_id`, `drop_area_id`, `order_type`, `payment_type`, `length_cm`, `breadth_cm`, `height_cm`, `actual_weight_kg`. No `customer_id` field exists on this shape at all; the order's owner is always the caller's own JWT identity. Sending `customer_id` anyway is rejected (`422`, unknown field) — this is the mechanism that makes "a customer cannot create an order for another customer" true.
- **ADMIN** — the same fields, plus a required `customer_id` naming who the order is for. The id must reference an existing user whose role is `CUSTOMER`; naming an `ADMIN` or `DELIVERY_AGENT` account is rejected.

```bash
# As a customer
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"pickup_area_id":"...","drop_area_id":"...","order_type":"B2C","payment_type":"COD","length_cm":10,"breadth_cm":10,"height_cm":10,"actual_weight_kg":3}'

# As an admin, on behalf of a customer
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"customer_id":"...","pickup_area_id":"...","drop_area_id":"...","order_type":"B2C","payment_type":"PREPAID","length_cm":1,"breadth_cm":1,"height_cm":1,"actual_weight_kg":8}'
```

```json
{
  "id": "...", "customer_id": "...", "created_by": "...",
  "order_type": "B2C", "payment_type": "COD",
  "pickup_area_id": "...", "drop_area_id": "...",
  "pickup_zone_id": "...", "drop_zone_id": "...", "zone_relationship": "INTRA",
  "length_cm": 10, "breadth_cm": 10, "height_cm": 10, "actual_weight_kg": 3,
  "volumetric_weight_kg": 0.2, "chargeable_weight_kg": 3,
  "rate_card_id": "...", "base_rate": 55, "cod_surcharge": 18, "final_amount": 73,
  "assigned_agent_id": null,
  "status": "CREATED", "created_at": "2026-08-21T09:21:32Z"
}
```

The request body has no field for anything server-derived — `id`,
`created_by`, `pickup_zone_id`, `drop_zone_id`, `zone_relationship`,
`volumetric_weight_kg`, `chargeable_weight_kg`, `rate_card_id`,
`base_rate`, `cod_surcharge`, `final_amount`, `status`, `created_at` —
sending any of them is rejected outright (`422`, unknown field).

**Errors**: `401`, `403`, `422` for: malformed body, an unknown field, a `CUSTOMER` sending `customer_id`, a missing `customer_id` from an `ADMIN`, a `customer_id` that doesn't exist or isn't a `CUSTOMER`, invalid `order_type`/`payment_type`, non-positive/non-finite dimensions or weight, an unknown or inactive-zone pickup/drop area, no active rate card for the resolved combination, or no slab covering the calculated chargeable weight (the same pricing-failure modes `POST /orders/quote` has, since both call the identical `CalculateQuote`).

### `GET /api/v1/orders`

**Auth**: required, **ADMIN, CUSTOMER, or DELIVERY_AGENT** (widened from ADMIN/CUSTOMER by M09). **Purpose**: list orders — every order for `ADMIN`, only the caller's own for `CUSTOMER`, only the orders currently assigned to them for `DELIVERY_AGENT` (never every customer's order). No query-parameter filters (by status or zone) — not in the endpoint list; agent-scoping is implicit in the `DELIVERY_AGENT` role's own view, not a filter any role can pass.

```bash
curl http://localhost:8080/api/v1/orders -H "Authorization: Bearer $TOKEN"
```

Response: a JSON array of the same shape `POST /orders` returns, now also carrying `assigned_agent_id` (`null` until M09 assigns an agent).

### `GET /api/v1/orders/{id}`

**Auth**: required, **ADMIN, CUSTOMER, or DELIVERY_AGENT** (widened by M09). **Purpose**: retrieve one order. `ADMIN` may retrieve any order; `CUSTOMER` only their own; `DELIVERY_AGENT` only an order currently assigned to them — requesting an order the caller doesn't own/isn't assigned returns `404`, never `403` or any other signal that the order exists, the same ownership convention M04's area-vs-zone and M05's slab-vs-rate-card path checks use.

**Errors**: `401`, `404` (unknown id, a `CUSTOMER` requesting an order they don't own, or a `DELIVERY_AGENT` requesting an order not assigned to them).

---

## Order tracking (M08)

The order status state machine and its immutable event log. Pricing,
persistence, and ownership stay exactly as M06/M07 left them — this
section only adds status transitions and their history. See
`docs/order-tracking.md` for the full design (the state diagram, the
per-edge authorization matrix, the concurrency mechanism, why the
initial `CREATED` event is written by order creation itself).

### `POST /api/v1/orders/:id/status`

**Auth**: required, **ADMIN or DELIVERY_AGENT** (`CUSTOMER` → `403` — customers have no status-transition authority in this module). **Purpose**: perform one legal state transition. Which of `ADMIN`/`DELIVERY_AGENT` may use a given edge depends on the edge itself, not just the route — see the authorization matrix in `docs/order-tracking.md`. `DELIVERY_AGENT` authority remains unscoped by ownership even after M09: `orders.assigned_agent_id` now exists, but this endpoint's own authorization was deliberately left unmodified (a finalized M09 decision — M08 remains the single source of truth, unchanged), so any authenticated agent may still perform an agent-tier edge on any order.

```bash
curl -X POST http://localhost:8080/api/v1/orders/$ORDER_ID/status \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"PICKED_UP"}'
```

```json
{
  "id": "...", "order_id": "...",
  "previous_status": "ASSIGNED", "new_status": "PICKED_UP",
  "actor_id": "...", "metadata": null,
  "created_at": "2026-08-21T13:27:07Z"
}
```

`metadata` is an optional, free-form JSON object, stored verbatim —
no required shape is enforced.

The request body has no field for anything server-derived (`id`,
`order_id`, `actor_id`, `previous_status`, `created_at`) — sending any
of them is rejected outright (`422`, unknown field).

**Errors**: `401`, `403` (`CUSTOMER`, unauthenticated, or a role not authorized for this specific edge), `404` (unknown order id), `409` (the requested status is a recognized value but not a legal transition from the order's current status — including a same-status no-op, which is never legal), `422` (malformed body, unknown field, or a status string that isn't one of the eight recognized values).

### `GET /api/v1/orders/:id/tracking`

**Auth**: required, **ADMIN or CUSTOMER** (`DELIVERY_AGENT` → `403`, matching M07's blanket "agents have no order visibility yet" stance). **Purpose**: the full, chronological (oldest first) event history for one order. `ADMIN` may view any order; `CUSTOMER` only their own.

```bash
curl http://localhost:8080/api/v1/orders/$ORDER_ID/tracking -H "Authorization: Bearer $TOKEN"
```

Response: a JSON array of the same event shape `POST .../status` returns. The first entry always has `"previous_status": null` — order creation itself is the first tracking event.

**Errors**: `401`, `403` (`DELIVERY_AGENT`), `404` (unknown id, or a `CUSTOMER` requesting an order they don't own — same hide-existence convention as `GET /orders/{id}`).

---

## Assignment (M09)

Manual and automatic delivery-agent assignment. Owns exactly one new
column (`orders.assigned_agent_id`) and no new table — assignment
history lives in M08's own tracking-event metadata. See
`docs/assignment-engine.md` for the full design (the ranking algorithm,
eligibility rule, concurrency strategy, and why M08's state machine is
reused rather than duplicated).

### `POST /api/v1/orders/:id/assign`

**Auth**: required, **ADMIN only** (`CUSTOMER`/`DELIVERY_AGENT` → `403`). **Purpose**: manually assign one named delivery agent to an order. Body: `{"agent_id": "<uuid>"}` — no other field; the order id comes from the URL, the actor from the JWT.

```bash
curl -X POST http://localhost:8080/api/v1/orders/$ORDER_ID/assign \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"agent_id":"..."}'
```

```json
{
  "id": "...", "customer_id": "...", "created_by": "...",
  "order_type": "B2C", "payment_type": "COD",
  "pickup_area_id": "...", "drop_area_id": "...",
  "pickup_zone_id": "...", "drop_zone_id": "...", "zone_relationship": "INTRA",
  "length_cm": 10, "breadth_cm": 10, "height_cm": 10, "actual_weight_kg": 3,
  "volumetric_weight_kg": 0.2, "chargeable_weight_kg": 3,
  "rate_card_id": "...", "base_rate": 55, "cod_surcharge": 18, "final_amount": 73,
  "assigned_agent_id": "...",
  "status": "ASSIGNED", "created_at": "2026-08-21T09:21:32Z"
}
```

The response is the updated order — the same shape every other order
endpoint returns, not a separate assignment resource. The tracking
event this transition also produces (with `metadata:
{"assigned_agent_id": "..."}`) is available, unchanged, via `GET
/orders/:id/tracking`.

**Errors**: `401`, `403` (non-`ADMIN`), `404` (unknown order or unknown `agent_id`), `409` (the named agent exists but fails the eligibility rule — inactive, not `AVAILABLE`, or no usable `current_zone_id` — or the order isn't currently in a status M08 permits `→ASSIGNED` from, including an order that is already `ASSIGNED`), `422` (malformed body, missing `agent_id`, or an unknown field).

### `POST /api/v1/orders/:id/auto-assign`

**Auth**: required, **ADMIN only**. **Purpose**: automatically select and assign the best-ranked eligible agent. No request body fields — every input the ranking algorithm needs is read fresh from the order (its pickup zone) and the `delivery_agents` table.

```bash
curl -X POST http://localhost:8080/api/v1/orders/$ORDER_ID/auto-assign \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

Response: the same updated-order shape as `POST .../assign`.

**Errors**: `401`, `403` (non-`ADMIN`), `404` (unknown order), `409` (no agent satisfies the eligibility rule, or the order isn't currently in a status M08 permits `→ASSIGNED` from).

---

## Failed delivery & rescheduling (M10)

Customer- (and admin-) initiated rescheduling of a failed delivery. Owns
exactly one new table (`reschedule_requests`) and reuses M08's own
`FAILED → RESCHEDULED` edge and M09's own reassignment endpoints
unmodified. See `docs/failed-delivery.md` for the full design (the
architectural mismatch between "customer can reschedule" and M08's
ADMIN-only transition matrix, and how it's resolved without touching
M08; agent-availability behavior after a failure; why no
`delivery_attempts` table exists).

### `POST /api/v1/orders/:id/reschedule`

**Auth**: required, **CUSTOMER (own order only) or ADMIN (any order)** (`DELIVERY_AGENT` → `403`). **Purpose**: reschedule a `FAILED` order for a new delivery date. Body: `{"requested_date": "YYYY-MM-DD", "reason": "optional"}` — no other field; the order id comes from the URL, the actor from the JWT.

```bash
curl -X POST http://localhost:8080/api/v1/orders/$ORDER_ID/reschedule \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"requested_date":"2099-01-01","reason":"Not available"}'
```

```json
{
  "id": "...", "customer_id": "...", "created_by": "...",
  "order_type": "B2C", "payment_type": "COD",
  "pickup_area_id": "...", "drop_area_id": "...",
  "pickup_zone_id": "...", "drop_zone_id": "...", "zone_relationship": "INTRA",
  "length_cm": 10, "breadth_cm": 10, "height_cm": 10, "actual_weight_kg": 3,
  "volumetric_weight_kg": 0.2, "chargeable_weight_kg": 3,
  "rate_card_id": "...", "base_rate": 55, "cod_surcharge": 18, "final_amount": 73,
  "assigned_agent_id": "...",
  "status": "RESCHEDULED", "created_at": "2026-08-21T09:21:32Z"
}
```

The response is the updated order — the same shape every other order
endpoint returns. `assigned_agent_id` is deliberately left unchanged
(the previously assigned agent, now freed back to `AVAILABLE` — see
`docs/failed-delivery.md`); it is overwritten only by a later, separate
`POST /orders/:id/assign`/`auto-assign` call. The reschedule record
itself and the paired tracking event are available, unchanged, via `GET
/orders/:id/reschedules` and `GET /orders/:id/tracking`.

**Errors**: `401`, `403` (`DELIVERY_AGENT`), `404` (unknown order, or a `CUSTOMER` requesting an order they don't own), `409` (the order is not currently `FAILED`), `422` (malformed body, missing/invalid `requested_date`, or an unknown field).

### `GET /api/v1/orders/:id/reschedules`

**Auth**: required, **CUSTOMER (own order only) or ADMIN (any order)** (`DELIVERY_AGENT` → `403`). **Purpose**: the full, chronological (oldest first) reschedule-request history for one order.

```bash
curl http://localhost:8080/api/v1/orders/$ORDER_ID/reschedules -H "Authorization: Bearer $TOKEN"
```

```json
[
  {
    "id": "...", "order_id": "...", "requested_by": "...",
    "requested_date": "2099-01-01", "reason": "Not available",
    "created_at": "2026-08-21T09:21:32Z"
  }
]
```

**Errors**: `401`, `403` (`DELIVERY_AGENT`), `404` (unknown id, or a `CUSTOMER` requesting an order they don't own — same hide-existence convention as `GET /orders/{id}`).

---

## What's not here yet

Notifications and dashboards — M11 and M12. This file grows with each module.
