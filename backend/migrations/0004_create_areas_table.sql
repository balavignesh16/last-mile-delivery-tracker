-- M04: an area belongs to exactly one zone (zone_id NOT NULL, no
-- nullable "unassigned area" state — the frozen architecture's hierarchy
-- is Address -> Area -> Zone, and an area with no zone would break that
-- chain for every later module that resolves through it).
--
-- UNIQUE (zone_id, name): area names only need to be distinct within
-- their own zone, not globally — two different zones legitimately having
-- an area called e.g. "Downtown" is not a conflict.
--
-- No ON DELETE CASCADE on the zones FK, and this module implements no
-- DELETE endpoint for either table: zones and areas are configuration
-- that later modules (rates, orders, agent current_zone_id) will
-- reference, so physical deletion risks orphaning those references or
-- silently invalidating history. Administrative deactivation
-- (zones.active) is the supported lifecycle operation instead; areas
-- have no independent active flag of their own — deactivating the
-- parent zone is sufficient, and the assignment does not ask for
-- per-area activation.

CREATE TABLE areas (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL CHECK (btrim(name) <> ''),
    zone_id    UUID NOT NULL REFERENCES zones(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (zone_id, name)
);

CREATE INDEX idx_areas_zone_id ON areas (zone_id);
