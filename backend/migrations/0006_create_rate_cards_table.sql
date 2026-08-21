-- M05: admin-configured pricing profile for one (order_type,
-- zone_relationship) combination. zone_relationship reuses the exact
-- string values M04's zones.ZoneRelationship already defined
-- ("INTRA"/"INTER") — the Go layer imports that type directly rather
-- than redeclaring it, so this column's CHECK is the only place those
-- values are spelled out on the database side.
--
-- New rows are always created with active = false — both at the
-- application layer (internal/rates.CreateRateCard never accepts an
-- "active" field on creation) and as this column's own default, so a
-- hypothetical direct SQL insert that forgets to specify it fails safe
-- too. An admin activates a card as a deliberate second step via PUT.
-- Defaulting to inactive (rather than active, like every other
-- "active" flag in this project) is deliberate here specifically: an
-- INSERT that defaulted to active=true would collide with the unique
-- index below the moment a second card for an already-active
-- combination is created, blocking the exact "stage a replacement
-- before swapping" workflow an admin needs.
--
-- idx_rate_cards_one_active_per_combination is the mechanism that makes
-- M06's "select the active rate card for this combination" step
-- deterministic: at most one row may be active for a given
-- (order_type, zone_relationship) at a time. Any number of inactive
-- rows for the same combination may coexist (drafts, history).

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
