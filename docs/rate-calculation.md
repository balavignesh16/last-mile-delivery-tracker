# Rate Calculation (M06)

## Scope

M06 is the **calculation engine** for pricing — given a pickup/drop area
pair, package dimensions, actual weight, order type, and payment type, it
computes exactly what an order would cost. It is deliberately
**stateless**: nothing here creates or persists anything. `POST
/api/v1/orders/quote` can be called any number of times with the same
input and always recomputes fresh against whatever rate configuration
(`internal/rates`, M05) and zone configuration (`internal/zones`, M04)
are active right now.

Per the frozen blueprint's own module split, M06 (Rate Calculation
Engine) is a separate, narrower module than M07 (Order Management) —
order identity, customer ownership, and persistence are explicitly out
of scope here and belong to a later module. This module extends
`internal/rates` (as its own package doc has committed to since M05) and
adds one endpoint; it does not touch `internal/orders`, still the
reserved-but-empty placeholder from M01.

## Reused, not duplicated

Every non-trivial piece of this calculation was already built by an
earlier module:

- **Zone resolution** — `zones.ResolvePickupDrop` (M04) resolves both
  areas to their zones and classifies the pair as `INTRA`/`INTER` in one
  call. M06 does not re-implement or wrap this; `CalculateQuote` calls it
  directly.
- **Rate card selection** — `rates.Repository.FindActiveCard(orderType,
  zoneRelationship)` (M05) was written specifically anticipating this
  consumption point. Its own doc comment named it as such since M05.
- **Slab storage** — `rates.Repository.ListSlabsByRateCard` (M05) already
  returns a rate card's slabs ordered by `min_weight`.

The only genuinely new code is the calculation itself: volumetric/
chargeable weight, slab selection by weight, and COD application — all
added to `internal/rates` (`pricing.go`), plus the HTTP handler
(`quote_handler.go`).

## Weight calculation

```go
func CalculateVolumetricWeight(lengthCM, breadthCM, heightCM float64) float64 {
    return lengthCM * breadthCM * heightCM / 5000
}

func CalculateChargeableWeight(actualWeightKG, volumetricWeightKG float64) float64 {
    return math.Max(actualWeightKG, volumetricWeightKG)
}
```

Both formulas are stated exactly by the source documents (`L × B × H ÷
5000`; chargeable weight is the higher of actual vs. volumetric) — no
invented behavior. Two things the source documents do **not** state,
flagged rather than silently decided:

- **Units.** Centimeters for dimensions, kilograms for the result, is
  the universal real-world convention for this exact divisor (and
  matches the blueprint's own worked example: 50×40×30 → 12), but no
  source document says so explicitly.
- **Rounding.** Not specified anywhere, so none is applied — values are
  carried at full `float64` precision through the calculation; only the
  final currency amounts round to two decimals, and that rounding comes
  from the database's `NUMERIC(10,2)` columns on `rate_cards`/
  `rate_card_slabs`, not from anything M06 does itself.

`length_cm`, `breadth_cm`, `height_cm`, and `actual_weight_kg` must each
be strictly greater than zero (not `>= 0`) — a package cannot physically
have a zero dimension or zero weight, unlike a rate slab's `min_weight`,
which is a legitimate range boundary starting at 0. Non-finite values
(`NaN`/`Infinity`) are rejected the same way `internal/rates`' existing
slab validation already rejects them.

## Slab selection

`SelectSlab(chargeableWeight, slabs)` is a single linear scan applying
M05's `[min_weight, max_weight)` convention uniformly:

```go
for _, s := range slabs {
    if chargeableWeight >= s.MinWeight && (s.MaxWeight == nil || chargeableWeight < *s.MaxWeight) {
        return s, nil
    }
}
return Slab{}, ErrNoMatchingSlab
```

No special-casing is needed for any of the boundary scenarios — they are
all just different shapes of "does this `[min, max)` window contain the
value":

| Scenario | Outcome |
|---|---|
| Exact `min_weight` (e.g. `5.000` when a slab starts at 5) | Matches — inclusive floor |
| Exact `max_weight` (e.g. `5.000` when a slab ends at 5) | Does **not** match this slab — falls through to the next one starting there |
| Weight in a gap between two slabs | Matches neither — `ErrNoMatchingSlab` |
| Weight below the lowest configured slab | Same — M05 never required slabs to start at 0 |
| Weight above every closed slab, no open-ended slab configured | Same |
| Open-ended slab (`max_weight = NULL`) | Matches any weight `>= min_weight` |
| Rate card has no slabs at all | Loop body never runs — `ErrNoMatchingSlab` immediately |
| Overlapping slabs | Cannot occur — already excluded by M05's own validation and partial unique index |

This module deliberately does **not** modify M05's overlap-prevention
mechanism, slab storage, or deletion semantics — `SelectSlab` only reads.

## Pricing

```
base_rate    = selected_slab.price                          (flat, never × weight — M05's finalized model)
cod_surcharge = payment_type == COD ? rate_card.cod_surcharge : 0
final_amount = base_rate + cod_surcharge
```

No invented behavior for `PREPAID` beyond "no surcharge" — the source
documents state exactly this and nothing more.

## Why "never trust a client-supplied price" actually holds

The blueprint requires quote and order-confirmation to be two separate
operations, with confirmation never blindly trusting a client-supplied
price. This module makes that true by construction, not by convention:
`CalculateQuote` is the **one** pricing function in the codebase, and it
always recomputes from raw inputs (`pickup_area_id`, `drop_area_id`,
`order_type`, `payment_type`, dimensions, weight) — never from a stored
or cached quote. `POST /orders/quote` calls it today; whichever later
module adds order confirmation is expected to call the exact same
function with the exact same input shape, not a second, parallel pricing
path. There is no quote token, quote ID, or server-side quote cache
anywhere — a "stale quote" cannot exist because nothing is ever stored
to go stale.

## What is *not* persisted

Nothing. `POST /orders/quote` performs zero writes — no order row, no
quote-history row, no cache entry. A later module that needs to persist
a *confirmed* order's pricing is expected to store its own snapshot
values (not a foreign key to an individual slab, since M05 allows slab
deletion and a stored `slab_id` would silently break that) rather than
relying on this endpoint's output being retrievable later.

## API

See `docs/api.md`'s "Rate calculation / quotes" section for the full
request/response shape, RBAC, and error mapping. In short: `POST
/api/v1/orders/quote`, `ADMIN` or `CUSTOMER`, `DELIVERY_AGENT` → `403`,
every calculation failure (unknown/inactive area, no active rate card,
no matching slab) → `422`.

## The one M04 change this module required

Every M04 zone/area endpoint was `ADMIN`-only, including reads — correct
at the time, since nothing needed customer read access. This module's
quote form needs a customer to pick a real pickup/drop area from a list,
and there is no other resolution mechanism (no geocoding — see
`docs/zone-management.md`'s "Address resolution" section). The fix is
the narrowest one available: `GET /zones`, `GET /zones/{id}`, and `GET
/zones/{zoneID}/areas` now additionally admit `CUSTOMER`. No mutation
route changed, no `internal/zones` Go code changed — only the RBAC
middleware list on three existing routes. `internal/rates`' own HTTP
endpoints needed no such change: a customer never sees a rate card or
slab directly, only the quote they produce.

## Mass-assignment protection

`quoteRequest`'s fields are exactly the client-suppliable inputs listed
above — there is no field, anywhere in the struct, for `customer_id`,
`pickup_zone_id`, `drop_zone_id`, `zone_relationship`, `rate_card_id`,
`volumetric_weight`, `chargeable_weight`, `base_rate`, `cod_surcharge`,
`final_amount`, or `status`. `DisallowUnknownFields` rejects the whole
request (`422`) if a client sends any of them — the same fail-closed
pattern every prior module in this project uses, never a silently
dropped field.

## Testing

- **Unit** (`internal/rates/pricing_test.go`, `quote_handler_test.go`):
  pure calculation functions (volumetric/chargeable weight, `SelectSlab`
  across every boundary case above), `CalculateQuote` end-to-end against
  in-memory fakes for both `zones.Repository` and `rates.Repository`,
  handler-level validation/mass-assignment/error-mapping tests.
- **Integration** (`tests/integration/quote_integration_test.go`): the
  same scenarios again through the real router and real Postgres — golden
  B2B/B2C × INTRA/INTER × PREPAID/COD scenarios, exact slab boundaries,
  missing rate card, missing slab, unknown/inactive area, full RBAC
  matrix, mass-assignment attempts for every forbidden field.
- **Regression**: M01–M05's full test suite re-verified green after this
  module's changes, including the widened zone RBAC tests
  (`TestZoneEndpoints_RoleGating`, `TestAreaEndpoints_GetRoleGating`).
