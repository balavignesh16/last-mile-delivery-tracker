# Zone Management (M04)

## Hierarchy

```
Address (free text, entered by a customer in M07's order form)
   |
   v
Area   (admin-configured, this module)
   |
   v
Zone   (admin-configured, this module)
```

An area belongs to exactly one zone. A zone has zero or more areas. This
is the frozen architecture's exact model — no address is ever geocoded by
this system; see "Address resolution" below for why.

## Schema

```sql
CREATE TABLE zones (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE areas (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL CHECK (btrim(name) <> ''),
    zone_id    UUID NOT NULL REFERENCES zones(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (zone_id, name)
);
```

`zones.name` is globally unique — two zones with the same display name
would make admin configuration ambiguous. `areas.name` is only unique
*within its zone* (`UNIQUE (zone_id, name)`) — two different zones
legitimately having an area called "Downtown" is not a conflict.
`areas.zone_id` is `NOT NULL`: there is no "unassigned area" state,
because an area with no zone would break the resolution chain for every
later module that reads through it.

## Coordinates (post-M04, migration `0016`)

```sql
ALTER TABLE areas ADD COLUMN latitude DOUBLE PRECISION CHECK (latitude BETWEEN -90 AND 90);
ALTER TABLE areas ADD COLUMN longitude DOUBLE PRECISION CHECK (longitude BETWEEN -180 AND 180);
```

Optional, nullable, no backfill for existing rows — every area starts
with no coordinates until an admin sets real ones via
`POST .../areas` or `PUT .../areas/{areaID}`. `POST` requires both or
neither (`internal/zones.validateOptionalCoordinates`); `PUT` treats
each as independently "leave unchanged if omitted," the same contract
`active` already has. Range/finite-number validation
(`internal/geo.ValidateCoordinates`) is the same shared logic
`internal/agents`' own (required-pair) location validation delegates
to — one set of bounds, not two copies of the same rule. These
coordinates exist for exactly one consumer: `internal/assignment`'s
auto-assignment ranking, which uses a pickup area's coordinates (when
set) to rank eligible agents by real Haversine distance instead of
zone-match alone — see `docs/assignment-engine.md`. Nothing in this
module reads or interprets them itself.

## Why there's no DELETE

Zones and areas are configuration that later modules will reference:
`delivery_agents.current_zone_id`, and eventually rate cards and orders.
Physical deletion risks orphaning those references or silently
invalidating history a rate calculation or order record still points to.
The frozen architecture's blueprint lists `POST`/`GET`/`GET`/`PUT` for
zones and `POST` for areas — no `DELETE` — so none is invented here.
Administrative deactivation (`zones.active`, toggled via `PUT
/zones/{id}`) is the supported lifecycle operation instead. Areas have no
independent active flag of their own; deactivating the parent zone is
sufficient, and nothing in the assignment asks for per-area activation.

## Address resolution

STEP 9 of this module's task explicitly rules out external geocoding
(Google Maps, Mapbox, OSM, or any other provider) — the project must stay
credential-free and deterministic for evaluation. So "address resolution"
in this codebase is **not** free-text-to-coordinates geocoding. It is: a
customer picks an area from a list of admin-configured areas (an area
*is* the resolvable unit — its name is the human-readable "address" a
customer sees), and the system resolves that area's `id` to a zone.
`internal/zones.ResolveArea` and `ResolvePickupDrop`
(`internal/zones/resolution.go`) are the reusable resolution service
later modules (M06's rate engine, M07's order form) are expected to call
— the intra/inter comparison is computed once, here, never
reimplemented downstream.

There is deliberately **no HTTP endpoint** for resolution in M04: nothing
in the frontend needs it yet (M04's UI only manages zones/areas, it
doesn't create orders), and the blueprint's API list doesn't request one.
Adding a public endpoint nothing calls would be exactly the kind of
speculative addition the frozen architecture asks not to make. The
resolution functions are Go-level, covered directly by unit tests
(`internal/zones/resolution_test.go`) and an integration test running
against real Postgres (`TestResolution_IntraAndInterAgainstRealDatabase`).

## INTRA vs INTER

```go
func DetermineZoneRelationship(pickupZoneID, dropZoneID string) ZoneRelationship {
    if pickupZoneID == dropZoneID {
        return RelationshipIntra
    }
    return RelationshipInter
}
```

Compares stable zone **IDs**, never names. A zone's name is a mutable
display string an admin can rename at any time via `PUT /zones/{id}`, so
it cannot be what later pricing logic keys off.

## Inactive-zone behavior

A deliberate M04 decision, not deferred to a later module:
`ResolveArea`/`ResolvePickupDrop` return `ErrZoneInactive` if the
resolved area's zone has been deactivated. An admin deactivates a zone
specifically to take it out of operational use — resolving an order
through it silently would defeat the purpose of the flag. This is a
judgment call the task explicitly asked to be made and documented rather
than left implicit; a later module that genuinely needs a "resolve
anyway, decide downstream" mode would have to introduce that as its own
explicit change. Covered by
`TestResolveArea_InactiveZoneIsRejected`.

Resolving an unknown area fails explicitly (`ErrAreaNotFound`) — the
resolution service never guesses, never falls back to a default zone,
and never silently returns `INTRA`. If an area's `zone_id` somehow points
to a zone that no longer exists (unreachable in practice given the `areas
-> zones` foreign key, but handled rather than assumed away), that is
treated as a data-integrity error, not a not-found.

## Completing `delivery_agents.current_zone_id`

M03 added `current_zone_id` as a plain nullable `UUID` with **no** foreign
key, because `zones` didn't exist yet — see M03's own migration comment.
M04's third migration closes that gap:

```sql
ALTER TABLE delivery_agents
    ADD CONSTRAINT fk_delivery_agents_current_zone_id
    FOREIGN KEY (current_zone_id) REFERENCES zones(id);
```

No M03 handler ever wrote to this column — `PUT
/agents/{id}/location` only ever touches `current_lat`/`current_lng` —
so this migration has no existing write path to break. It simply closes
the constraint gap for whichever later module (plausibly M09) starts
writing to it. Verified against real PostgreSQL in
`TestDeliveryAgentsCurrentZoneFK`: a valid zone id is accepted, a
nonexistent one is rejected, and `NULL` remains valid.

## RBAC

Every zone/area **mutation** endpoint is `ADMIN`-only — the task
specification's default ("unless the assignment explicitly states
otherwise"), and nothing states otherwise for M04.

The three **read** endpoints (`GET /zones`, `GET /zones/{id}`, `GET
/zones/{zoneID}/areas`) additionally admit `CUSTOMER`, a narrow M06
change: a customer requesting a quote needs to pick a real pickup/drop
area from a list, and M04 had no reason to anticipate that read need at
the time. See `docs/rate-calculation.md` for why this was necessary
rather than optional.

The same three reads were further widened to admit `DELIVERY_AGENT`: an
agent needs the zone list to pick their own `current_zone_id` via `PUT
/agents/{id}/zone` (see `docs/user-agent-management.md`) — the endpoint
that closes the gap where `current_zone_id`, the column M09's
`assignment.IsEligible` requires, had no application write path at all.
Same shape as the M06 widening: no mutation route touched, no Go code in
this module's handlers/resolution/repository changed — only the RBAC
middleware list in `routes.go`'s three `GET` registrations grew one more
role.

`DELIVERY_AGENT` still gets `403` on every **mutation** endpoint in this
module (zone/area management stays `ADMIN`-only); unauthenticated
requests get `401` everywhere. Verified in
`tests/integration/zones_integration_test.go`
(`TestZoneEndpoints_RoleGating`, `TestAreaEndpoints_RoleGating`,
`TestAreaEndpoints_GetRoleGating`).

## Mass-assignment protection

`POST /zones/{zoneID}/areas`'s request DTO has no `zone_id` field —
structurally, not just by validation. A client attempting
`{"name":"X","zone_id":"<some-other-zone>"}` gets the whole request
rejected (`422`, unknown field), the same fail-closed pattern
`auth.RegisterHandler` uses for a client-supplied `role`. The area's zone
is always the `{zoneID}` path segment. Tested at both the handler level
(`TestCreateAreaHandler_ZoneIDInBodyIsRejected`) and against the real
router/database (`TestAreaCreate_ZoneIDBodyTamperCannotOverridePath`).
