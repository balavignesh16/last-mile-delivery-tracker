-- M11: a customer notification attempt for one exact tracking-event
-- occurrence, on one channel. tracking_event_id (not order_id + event
-- type) is the idempotency anchor — an order can legitimately produce
-- the *same* event type more than once (e.g. FAILED -> RESCHEDULED ->
-- ASSIGNED -> ... -> FAILED again), and each occurrence is a distinct
-- row in order_tracking_events with its own id; keying on that id
-- (rather than on (order_id, event)) is what lets a second, genuine
-- FAILED occurrence still notify, while a retried/duplicate attempt at
-- the *same* occurrence does not.
--
-- order_id is denormalized from tracking_event_id's own
-- order_tracking_events.order_id — a deliberate, minor redundancy (the
-- same kind orders.pickup_zone_id/drop_zone_id already accept) so any
-- future "all notifications for this order" query needs no join.
--
-- status starts 'PENDING' the instant a (tracking_event_id, channel)
-- slot is claimed (INSERT ... ON CONFLICT DO NOTHING), before the
-- provider is ever called, and is updated to 'SENT'/'FAILED' once the
-- provider attempt resolves — this is what makes the unique index below
-- prevent a *duplicate send*, not merely a duplicate row: a second,
-- concurrent claim attempt for the same (tracking_event_id, channel)
-- finds the slot already taken and never calls the provider at all.
--
-- No ON DELETE CASCADE on either FK: this project has no orders- or
-- tracking-event-deletion endpoint, so plain NO ACTION is safe and
-- consistent with every other FK in this schema.
CREATE TABLE notifications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracking_event_id  UUID NOT NULL REFERENCES order_tracking_events(id),
    order_id           UUID NOT NULL REFERENCES orders(id),
    event              TEXT NOT NULL CHECK (event IN (
        'ORDER_CREATED', 'AGENT_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT',
        'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED', 'RESCHEDULED'
    )),
    channel            TEXT NOT NULL CHECK (channel IN ('EMAIL', 'SMS')),
    recipient          TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (status IN ('PENDING', 'SENT', 'FAILED')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The idempotency backstop: at most one notification row per exact
-- tracking-event occurrence per channel.
CREATE UNIQUE INDEX idx_notifications_tracking_event_channel ON notifications (tracking_event_id, channel);

CREATE INDEX idx_notifications_order_id ON notifications (order_id);
