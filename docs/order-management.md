# Order Management (M07)

## Scope

M07 owns order **persistence and retrieval** — the `orders` table, `POST
/api/v1/orders`, `GET /api/v1/orders`, `GET /api/v1/orders/{id}`. It
owns none of the pricing calculation: zone resolution, rate-card
selection, weight calculation, slab selection, and COD application are
all M06's job (`internal/rates`), unchanged and untouched by this
module except for one small, deliberate refactor (see "Reuse, not
duplication" below).

Explicitly out of scope, per the frozen module split: order status
transitions and the M08 state machine, delivery-agent assignment (M09),
tracking, notifications, order filtering by status/zone/agent, order
editing/cancellation/deletion, payment processing.

## Reuse, not duplication: `rates.CalculateQuote`

`POST /api/v1/orders`'s entire pricing step is one call:

```go
quote, err := rates.CalculateQuote(r.Context(), zonesRepo, ratesRepo, quoteInput)
```

This is the exact same function `POST /api/v1/orders/quote` (M06) calls.
Nothing in `internal/orders` computes volumetric weight, chargeable
weight, selects a rate card, selects a slab, or applies a COD surcharge
— there is no second implementation of any of that logic anywhere in
this module. The order handler's job is entirely: figure out which
customer this order is for and validate the caller is allowed to create
it on that customer's behalf, build a `rates.QuoteInput` from the
request, call `CalculateQuote`, and persist the `rates.QuoteResult` it
returns.

Two small, deliberate refactors were made to `internal/rates` to make
this reuse possible without duplicating validation or error-mapping
logic either — both are pure extractions with **zero behavior change**
to `POST /orders/quote` (verified: M06's full test suite passes
unchanged before and after):

- `validateQuoteRequest` (HTTP-DTO-shaped, unexported) became
  `ValidateQuoteFields` (raw-fields-shaped, exported) — M06's own
  `QuoteHandler` now calls it with its DTO's fields unpacked; M07's
  `CreateOrderHandler` calls the identical function with its own DTO's
  fields unpacked. One validation implementation, two callers.
- `mapQuoteDomainError` (unexported) became `MapQuoteError` (exported)
  — same reasoning, for turning `CalculateQuote`'s domain errors
  (`zones.ErrAreaNotFound`, `zones.ErrZoneInactive`,
  `rates.ErrRateCardNotFound`, `rates.ErrNoMatchingSlab`) into the same
  `(422, message)` pairs in both handlers.

## Why "never trust a client-supplied price" is actually true here

`POST /orders/quote` and `POST /orders` accept the same raw inputs
(pickup/drop area, dimensions, weight, order type, payment type) and
each independently calls `CalculateQuote` fresh. There is no quote
token, quote ID, or cached quote passed from the first call to the
second — nothing to go stale, and nothing for a client to tamper with
in between. If the active rate card changes between a customer's
`POST /orders/quote` preview and their `POST /orders` confirmation, the
confirmation simply prices against whatever is active *at that moment*
— this is correct behavior, not a bug to guard against.

## Customer vs. admin creation

Two separate request DTOs, decoded based on the caller's role — not one
shared struct with an optional field:

```go
// CUSTOMER caller — no customer_id field exists in this type at all.
type customerCreateOrderRequest struct {
    PickupAreaID, DropAreaID, OrderType, PaymentType string
    LengthCM, BreadthCM, HeightCM, ActualWeightKG     *float64
}

// ADMIN caller — the one additional field.
type adminCreateOrderRequest struct {
    CustomerID string
    // ...the same fields as above
}
```

For a `CUSTOMER` caller, `customer_id` is always `identity.UserID` from
the JWT (`auth.IdentityFromContext`) — never read from the request body.
Because `customerCreateOrderRequest` has no `customer_id` field
structurally, a customer attempting to send one doesn't get silently
ignored — `DisallowUnknownFields` rejects the whole request (`422`).
This is what makes "a customer cannot create an order for another
customer" true by construction rather than by a runtime check that could
be forgotten.

For an `ADMIN` caller, `customer_id` is required and validated two ways
before being trusted:

1. `users.Repository.FindByID` — the id must reference a real user.
2. `user.Role == users.RoleCustomer` — an admin cannot create an order
   naming another `ADMIN` or a `DELIVERY_AGENT` as its customer; both are
   rejected with `422`.

`created_by` is always `identity.UserID` — the caller who actually
submitted the request (the customer themself, or the acting admin) —
and is never a request field for either DTO.

## Pricing snapshot

Every pricing-derived column on `orders` (`zone_relationship`,
`volumetric_weight_kg`, `chargeable_weight_kg`, `rate_card_id`,
`base_rate`, `cod_surcharge`, `final_amount`, plus the resolved
`pickup_zone_id`/`drop_zone_id`) is written **once**, directly from the
`rates.QuoteResult` `CalculateQuote` returned, and never recomputed on
read. A later admin edit to the rate card that priced an order (a new
`cod_surcharge`, a changed slab price) has no effect on that order's
already-stored `final_amount` — it was captured as a value, not derived
through a live reference.

`rate_card_id` is stored as an FK — safe, because M05 rate cards are
never deleted, only deactivated (`docs/rate-configuration.md`).
**Deliberately no `slab_id` column.** M05 allows slab deletion
specifically because nothing else references an individual slab by
foreign key; adding `orders.slab_id` would silently break that
guarantee (an admin correcting a rate card could no longer delete a
slab that had ever priced an order). The slab's price at order time is
already captured in `base_rate` — there's no need to reference the slab
row itself to reproduce the charge later.

## Why no `packages` table

The blueprint's own ER diagram shows `orders` and `packages` as
separate entities, but nothing in the actual order flow implies more
than one package per order — there's no "add another package" step
anywhere in the source documents. Dimensions and weight are inline
columns on `orders` instead — the pricing side of order creation
(`CalculateQuote`, then the `INSERT` of the priced row) never needed a
second table to stay in sync with.

**Updated by M08**: `CreateOrder` does now run inside a transaction —
not because of `packages`, but because M08 requires order creation to
also record its own opening tracking event (see "Status" below). The
`INSERT INTO orders` and the paired `INSERT INTO order_tracking_events`
must succeed or fail together; this is the one transaction M07's own
design note said wouldn't be needed, added later by a different,
unrelated requirement.

## Status

Every order is created with `status = 'CREATED'` — the only value this
module writes directly to `orders.status`. The full M08 state-machine
value set (`ASSIGNED`, `PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`,
`DELIVERED`, `FAILED`, `RESCHEDULED`) already existed in the
`orders.status` `CHECK` constraint since migration `0008`, so M08
needed no schema change to `orders` itself to add its transitions.

**M08 is now implemented** — `POST /api/v1/orders/:id/status`
transitions an order (state-machine-validated, role-authorized per
edge) and `GET /api/v1/orders/:id/tracking` returns its full event
history. `internal/orders` still only ever writes `CREATED`; every
later transition belongs to `internal/tracking`. See
`docs/order-tracking.md` for the full M08 design.

## Security model

| Threat | How it's closed |
|---|---|
| Privilege escalation — customer creates an order for another customer | `customerCreateOrderRequest` has no `customer_id` field; sending one is a `422` unknown-field rejection, not a silently dropped value |
| Mass assignment of any server-derived field (`id`, `created_by`, `pickup_zone_id`, `drop_zone_id`, `zone_relationship`, `rate_card_id`, `volumetric_weight_kg`, `chargeable_weight_kg`, `base_rate`, `cod_surcharge`, `final_amount`, `status`, `created_at`) | None of these fields exist on either request DTO; `DisallowUnknownFields` rejects the whole request |
| Admin assigns an order to a non-customer account | `users.FindByID` + a `Role == CUSTOMER` check, both before any pricing or persistence happens |
| IDOR on `GET /orders/{id}` | Ownership check (`order.CustomerID == identity.UserID`) for `CUSTOMER` callers, `404` on mismatch — never `403`, so the endpoint never confirms an order exists under an id the caller doesn't own. Same convention as M04's area-vs-zone and M05's slab-vs-rate-card path checks |
| `DELIVERY_AGENT` order visibility | `403` on all three endpoints — nothing in any source document gives an agent order access before assignment exists (M09) |
| Price tampering | Impossible by construction — no pricing field is ever accepted as input on either DTO; every one is `CalculateQuote`'s output |
| SQL injection | Every query in `internal/orders/repository.go` is parameterized; no string concatenation into SQL anywhere |

## API

See `docs/api.md`'s "Orders" section for full request/response examples
and the RBAC table. In short:

| Endpoint | Role |
|---|---|
| `POST /api/v1/orders` | `ADMIN`, `CUSTOMER` |
| `GET /api/v1/orders` | `ADMIN` (all), `CUSTOMER` (own only) |
| `GET /api/v1/orders/{id}` | `ADMIN` (any), `CUSTOMER` (own only, else `404`) |

No status/zone/agent filter query parameters — nothing in this
milestone produces fields worth filtering on yet (every order is
`CREATED`, no agent is assigned). Filtering arrives when M08/M09 do.

## Testing

- **Unit** (`internal/orders/handler_test.go`, using in-memory fakes for
  `users.Repository`/`zones.Repository`/`rates.Repository`/this
  package's own `Repository`): customer and admin creation happy paths,
  identity-from-JWT (not body), admin-target-validation (nonexistent
  user, non-customer role), every field-validation and mass-assignment
  case, ownership on list/get, and an explicit assertion that
  `CalculateQuote` actually ran (`base_rate`/`cod_surcharge`/
  `final_amount`/`zone_relationship` in the response match the
  configured rate card, not a hardcoded value).
- **Integration** (`tests/integration/orders_integration_test.go`,
  against real Postgres): full customer and admin-on-behalf-of-customer
  flows, a dedicated test asserting a quote and an order created from
  the identical input produce byte-identical pricing fields (the
  concrete "M06 is reused, not duplicated" check), list isolation, IDOR,
  admin-sees-all, full RBAC matrix, foreign-key and check-constraint
  tests against the real schema, and a default-status test.
- **Frontend** (`CreateOrderPage.test.tsx`, `OrdersPage.test.tsx`,
  `OrderDetailPage.test.tsx`, `orders.test.ts`): role-conditional UI (no
  `customer_id` field for a `CUSTOMER`), the preview-then-confirm flow,
  request-body shape assertions, list/detail loading and error states.
- **Regression**: the complete M01–M06 backend and frontend suites
  re-verified green after every change in this module, including a
  repeated (`-count=3`) integration run to rule out ordering-dependent
  flakiness.
