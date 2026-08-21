# Rate Configuration (M05)

## Scope

M05 is the **admin configuration side** of pricing — it stores rate
cards and their weight slabs, and enforces that configuration is
internally consistent (no overlapping slabs, at most one active card per
combination). It does **not** calculate anything: chargeable weight,
slab selection for a given weight, and COD surcharge application are all
M06's job (Rate Calculation Engine), a separate, later module. Every
place this document says "M05 only stores X" means exactly that —
consuming X to produce a price is out of scope here.

## Schema

```sql
CREATE TABLE rate_cards (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_type        TEXT NOT NULL CHECK (order_type IN ('B2B', 'B2C')),
    zone_relationship TEXT NOT NULL CHECK (zone_relationship IN ('INTRA', 'INTER')),
    cod_surcharge     NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (cod_surcharge >= 0),
    active            BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_rate_cards_one_active_per_combination
    ON rate_cards (order_type, zone_relationship) WHERE active;

CREATE TABLE rate_card_slabs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rate_card_id UUID NOT NULL REFERENCES rate_cards(id),
    min_weight   NUMERIC(10,3) NOT NULL CHECK (min_weight >= 0),
    max_weight   NUMERIC(10,3) CHECK (max_weight IS NULL OR max_weight > min_weight),
    price        NUMERIC(10,2) NOT NULL CHECK (price >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_rate_card_slabs_one_open_ended
    ON rate_card_slabs (rate_card_id) WHERE max_weight IS NULL;
```

`zone_relationship` reuses M04's exact `zones.ZoneRelationship` values
(`INTRA`/`INTER`) at the Go layer — `internal/rates.ZoneRelationship` is
a type alias for it, not a redeclaration (`internal/rates/zone_relationship.go`).

## Why new rate cards start inactive

Unlike every other `active` flag in this project (zones, agents),
`rate_cards.active` defaults to `false`, and `CreateRateCardInput` has no
`Active` field at all — a card only becomes active through a later,
explicit `PUT /rates/{id}` call.

This is a deliberate concurrency-driven design decision, not an
oversight: if creation defaulted to `active = true`, an admin could never
create a *second* card for a combination that already has an active one
— not even as a draft — without first deactivating the live card, which
would leave that combination with zero active cards (unresolvable by
M06) during the edit. Defaulting to inactive removes that gap: an admin
can always stage a replacement card, verify its slabs, and only then
flip it active — at which point the swap is atomic from the database's
perspective (see "Concurrency" below).

## Flat price per slab, not per-kg

A chargeable weight falling in `[min_weight, max_weight)` costs exactly
`price` — not `price` multiplied by the weight. `rate_card_slabs.price`
is a flat amount for the whole band, confirmed as the pricing model for
this project.

## `[min_weight, max_weight)` boundary convention

A slab covers its `min_weight` (inclusive) up to but not including its
`max_weight` (exclusive). Concretely, for the demo configuration:

```
0–5 kg    → slab A
5–10 kg   → slab B
10–15 kg  → slab C
15–20 kg  → slab D
```

- `4.999 kg` belongs to slab A.
- `5.000 kg` belongs to slab B, **not** slab A — a slab's upper boundary
  belongs to the next slab.
- `9.999 kg` belongs to slab B.
- `10.000 kg` belongs to slab C.

`max_weight = NULL` represents an open-ended top slab: `min_weight <=
weight`, no upper bound. At most one such slab is permitted per rate
card, enforced both by `idx_rate_card_slabs_one_open_ended` and by the
application check in `validateSlabAgainstExisting`.

The weight-to-slab *lookup* itself (given a chargeable weight, which
slab applies) is M06's responsibility, not implemented here — this
module only stores data shaped so that lookup is unambiguous once M06
performs it.

## Preventing overlapping slabs

Postgres can express range-overlap-exclusion natively, but only via the
`btree_gist` extension — deliberately not used here, consistent with
this project's "PostgreSQL-only, no extra extensions" bias from the
architecture freeze. Overlap is instead prevented at the application
layer (`internal/rates/slab_validation.go`), inside a transaction that
takes a `SELECT ... FOR UPDATE` lock on the parent `rate_cards` row
before checking a new/edited slab's range against every existing slab in
that card. See "Concurrency" for why the lock, not just the check, is
what actually makes this safe.

## COD surcharge: stored, not applied

`cod_surcharge` is a flat amount, one column on `rate_cards` — a
combination's COD surcharge is per `(order_type, zone_relationship)`,
which is a superset of "per order type" (an admin can set the same value
on both the intra and inter card of one order type to get exactly that
narrower behavior, without being forced into a separate five-row config
entity nothing else requires). **M05 only stores this value.** Adding it
to an order's charge when the order's payment type is COD is M06's
job — this module has no code path that reads `cod_surcharge` for any
purpose other than storing and returning it.

## Deletion semantics

- **Rate cards are never deleted.** Only deactivated
  (`active = false`), via `PUT /rates/{id}` — no `DELETE /rates/{id}`
  endpoint exists. This preserves historical configuration and matches
  M04's deactivate-instead-of-delete philosophy for configuration
  entities other modules may reference.
- **Slabs may be deleted** (`DELETE
  /rates/{rateCardID}/slabs/{slabID}`) — a genuine deviation from M04's
  "no DELETE anywhere" pattern, made deliberately: unlike zones/areas
  (referenced by `delivery_agents.current_zone_id`), nothing in this
  schema holds a foreign key to an individual slab, so removing a
  misconfigured one cannot orphan anything. An admin correcting a rate
  card's tiers needs a way to remove a wrong slab outright, not just
  avoid creating it.

## One active card per combination

`idx_rate_cards_one_active_per_combination` is a partial unique index on
`(order_type, zone_relationship) WHERE active` — at most one row may be
active for a given combination at any time. This is what makes M06's
"select the rate card for this order" step deterministic; without it,
two active `B2B`+`INTRA` cards would leave no principled way to choose
between them. Any number of *inactive* rows for the same combination may
coexist (drafts, history) — the constraint only ever restricts `active`
rows.

## Concurrency

Two independent race conditions exist in this module, and each is closed
by a different mechanism — see `internal/rates/repository.go`'s doc
comments for the full reasoning:

1. **Activating two different cards for the same combination at once.**
   Closed by the partial unique index itself: `UpdateRateCard`'s
   `UPDATE ... SET active = true` either commits or fails with a real
   Postgres unique-violation, atomically, with no pre-check that could
   race. The application maps that violation to `ErrActiveCombinationExists`
   (`409`) rather than a raw `500`. Verified under real concurrent load
   in `TestConcurrentActivation_OnlyOneWins` (`tests/integration`) — N
   simultaneous activation requests for two candidate cards, asserting
   exactly one commits and the database shows exactly one active row
   afterward.
2. **Two concurrent slab writes to the same rate card racing past the
   overlap check.** A plain "read existing slabs, check in Go, insert"
   sequence would let two concurrent requests both read the pre-insert
   state, both pass the check, and both commit — producing overlapping
   slabs the check was supposed to prevent. Closed by `SELECT ... FOR
   UPDATE` on the parent `rate_cards` row inside the same transaction as
   the check and the write, serializing all slab mutations for *that*
   rate card (writes to a different rate card are unaffected). Verified
   under real concurrent load in
   `TestConcurrentSlabCreation_OverlapPreventedUnderRace` — N
   simultaneous requests to create the same overlapping range, asserting
   exactly one succeeds and exactly one slab row exists afterward.

Both mechanisms are pure PostgreSQL (a unique index, a row lock) —
neither needs Redis, an application-level mutex, or any coordination
outside the database, consistent with the architecture freeze's explicit
"Postgres-only concurrency mechanisms" decision.

## RBAC

Every `/rates` and `/rates/.../slabs` endpoint is `ADMIN`-only —
`CUSTOMER`/`DELIVERY_AGENT` get `403`, unauthenticated requests get
`401`. This includes `GET` endpoints: M06's future read access happens
through `internal/rates`'s Go-level `Repository` interface (in
particular `FindActiveCard`), not through these HTTP routes, the same
pattern M04 established for `internal/zones.ResolvePickupDrop`. There is
no scenario where `CUSTOMER` needs to call `/rates` directly.

## Mass-assignment protection

`POST /rates/{rateCardID}/slabs`'s request body has no `rate_card_id`
field — structurally, not just by validation. A client attempting
`{"min_weight":0,"max_weight":5,"price":50,"rate_card_id":"<some-other-card>"}`
gets the whole request rejected (`422`, unknown field). The slab's
parent is always the `{rateCardID}` path segment. Similarly,
`PUT`/`DELETE .../slabs/{slabID}` verify the slab actually belongs to
the rate card named in the URL before acting — a slab addressed through
the wrong rate card's URL gets `404`, identical to "doesn't exist,"
never a more specific error.

## Consumption by M06 (forward compatibility)

`internal/rates.Repository` exposes `FindActiveCard(orderType,
zoneRelationship)` specifically for M06 to call directly — the
Go-level consumption point, not an HTTP endpoint (see "RBAC" above).
`internal/rates` deliberately does **not** implement:

- chargeable-weight calculation (`max(actual, volumetric)`)
- volumetric-weight calculation (`L × B × H ÷ 5000`)
- selecting which slab matches a given chargeable weight
- COD surcharge application to an order's total
- any quote/pricing endpoint

All of the above are M06's flagship responsibility per the blueprint's
own module split, and are left entirely unimplemented here.
