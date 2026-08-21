-- M05: weight-tiered flat pricing rows under one rate card. A slab
-- covers [min_weight, max_weight) — min inclusive, max exclusive — so a
-- chargeable weight equal to a slab's max_weight belongs to the *next*
-- slab, never this one. max_weight = NULL represents an open-ended top
-- slab ("and above"); idx_rate_card_slabs_one_open_ended limits a rate
-- card to at most one such row.
--
-- Overlap between slabs (including two slabs that both claim to be
-- open-ended) cannot be expressed as a plain constraint without the
-- btree_gist extension, which this project deliberately avoids — see
-- docs/rate-configuration.md. Overlap is prevented by the application,
-- which locks the parent rate_cards row (SELECT ... FOR UPDATE) before
-- checking a new/edited slab against every existing slab in the same
-- rate card, closing the check-then-insert race a plain SELECT-then-
-- INSERT would leave open under concurrent requests.
--
-- No ON DELETE CASCADE on the rate_cards FK, consistent with every
-- other FK in this project — rate cards are never deleted (see
-- 0006's comment and docs/rate-configuration.md), so this never
-- triggers in practice, but NO ACTION remains the safer default absent
-- a reason to cascade.

CREATE TABLE rate_card_slabs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rate_card_id UUID NOT NULL REFERENCES rate_cards(id),
    min_weight   NUMERIC(10,3) NOT NULL CHECK (min_weight >= 0),
    max_weight   NUMERIC(10,3) CHECK (max_weight IS NULL OR max_weight > min_weight),
    price        NUMERIC(10,2) NOT NULL CHECK (price >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_rate_card_slabs_rate_card_id ON rate_card_slabs (rate_card_id);

CREATE UNIQUE INDEX idx_rate_card_slabs_one_open_ended
    ON rate_card_slabs (rate_card_id) WHERE max_weight IS NULL;
