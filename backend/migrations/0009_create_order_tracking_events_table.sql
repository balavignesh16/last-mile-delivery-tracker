-- M08: the immutable audit log of every order status transition.
-- previous_status is NULL only for the very first event any order ever
-- gets (NULL -> CREATED, inserted atomically by internal/orders.CreateOrder
-- itself, not by this module's transition endpoint). Every subsequent
-- row is written exclusively by internal/tracking's POST
-- /orders/:id/status handler, inside the same transaction that updates
-- orders.status, under a SELECT ... FOR UPDATE lock on the parent order
-- — see docs/order-tracking.md for the concurrency reasoning.
--
-- orders.status's CHECK constraint (migration 0008) already covers the
-- full M08 value set, so this table's own CHECK constraints reuse the
-- identical value list rather than a new enum — no ALTER to orders is
-- needed for M08.
--
-- No update/delete path exists anywhere in the application for this
-- table — it is genuinely append-only, matching the blueprint's own
-- "tracking history should not have normal application-level
-- update/delete operations" requirement literally, not just by
-- convention.
--
-- metadata is a free-form, optional JSONB payload — M08 defines no
-- required shape for it (the blueprint does not specify one); a later
-- module may choose to attach its own context (e.g. a failure reason,
-- a reschedule date) without this table needing to change.

CREATE TABLE order_tracking_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID NOT NULL REFERENCES orders(id),
    previous_status  TEXT CHECK (
        previous_status IS NULL OR previous_status IN (
            'CREATED', 'ASSIGNED', 'PICKED_UP', 'IN_TRANSIT',
            'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED', 'RESCHEDULED'
        )
    ),
    new_status       TEXT NOT NULL CHECK (
        new_status IN (
            'CREATED', 'ASSIGNED', 'PICKED_UP', 'IN_TRANSIT',
            'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED', 'RESCHEDULED'
        )
    ),
    actor_id         UUID NOT NULL REFERENCES users(id),
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_tracking_events_order_id ON order_tracking_events (order_id);
