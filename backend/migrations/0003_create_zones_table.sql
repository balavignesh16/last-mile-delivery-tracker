-- M04: zones are the top-level geographic configuration unit. Areas (next
-- migration) belong to exactly one zone; orders and rates in later
-- modules key off a zone's stable id, never its name.
--
-- name is UNIQUE: two zones with the same display name would make admin
-- configuration ambiguous, and nothing in the assignment requires
-- allowing duplicates.
--
-- active has no cascading behavior of its own in M04 — zones are never
-- deleted (see the areas migration for the full deletion-semantics
-- rationale), so "deactivate" is the only lifecycle transition this
-- module supports. Later modules decide what an inactive zone means for
-- orders/rates; M04 itself only stores and exposes the flag.

CREATE TABLE zones (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
