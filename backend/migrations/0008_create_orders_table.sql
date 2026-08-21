-- M07: a confirmed, priced order. Every pricing-derived column here is a
-- snapshot written once at creation time from M06's rates.CalculateQuote
-- result — never recomputed on read, and never affected by a later edit
-- to the rate card it was priced against (rate cards are deactivate-only,
-- never deleted, so rate_card_id is a safe, permanent reference; see
-- 0006's comment). Deliberately no slab_id column: M05 allows slab
-- deletion specifically because nothing else references an individual
-- slab by foreign key (see 0007's comment) — adding one here would
-- silently break that.
--
-- No packages table: nothing in this project's flow implies more than
-- one package per order, so dimensions/weight are inline columns rather
-- than a separate 1:1 child table, avoiding an unneeded transaction on
-- order creation (a single INSERT is already atomic).
--
-- status starts and, as of M07, only ever is 'CREATED' — the full M08
-- state-machine value set is declared in the CHECK constraint now so a
-- later module doesn't need an ALTER TABLE just to widen it, but no
-- code in this module writes anything other than 'CREATED'.

CREATE TABLE orders (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id           UUID NOT NULL REFERENCES users(id),
    created_by            UUID NOT NULL REFERENCES users(id),
    order_type            TEXT NOT NULL CHECK (order_type IN ('B2B', 'B2C')),
    payment_type          TEXT NOT NULL CHECK (payment_type IN ('PREPAID', 'COD')),
    pickup_area_id        UUID NOT NULL REFERENCES areas(id),
    drop_area_id          UUID NOT NULL REFERENCES areas(id),
    pickup_zone_id        UUID NOT NULL REFERENCES zones(id),
    drop_zone_id          UUID NOT NULL REFERENCES zones(id),
    zone_relationship     TEXT NOT NULL CHECK (zone_relationship IN ('INTRA', 'INTER')),
    length_cm             NUMERIC(10,3) NOT NULL CHECK (length_cm > 0),
    breadth_cm            NUMERIC(10,3) NOT NULL CHECK (breadth_cm > 0),
    height_cm             NUMERIC(10,3) NOT NULL CHECK (height_cm > 0),
    actual_weight_kg      NUMERIC(10,3) NOT NULL CHECK (actual_weight_kg > 0),
    volumetric_weight_kg  NUMERIC(10,3) NOT NULL,
    chargeable_weight_kg  NUMERIC(10,3) NOT NULL,
    rate_card_id          UUID NOT NULL REFERENCES rate_cards(id),
    base_rate             NUMERIC(10,2) NOT NULL,
    cod_surcharge         NUMERIC(10,2) NOT NULL DEFAULT 0,
    final_amount          NUMERIC(10,2) NOT NULL,
    status                TEXT NOT NULL DEFAULT 'CREATED' CHECK (
        status IN (
            'CREATED',
            'ASSIGNED',
            'PICKED_UP',
            'IN_TRANSIT',
            'OUT_FOR_DELIVERY',
            'DELIVERED',
            'FAILED',
            'RESCHEDULED'
        )
    ),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_customer_id ON orders (customer_id);
