-- M10: a customer's (or admin's) request to reschedule a FAILED order for
-- a new delivery date. requested_by and created_at are already exactly
-- "who requested this" and "when" — no separate approval actor is ever
-- recorded, because there is no approval workflow: a reschedule request
-- is immediately effective once created (see docs/failed-delivery.md).
-- Deliberately no status/approved_at/rejected_at columns for that reason.
--
-- No CHECK constraint ties this row to the order's status at request
-- time — that lifecycle rule (the order must be FAILED) is enforced by
-- M08's own tracking.TransitionTx at write time (FAILED -> RESCHEDULED),
-- inside the same transaction that inserts this row; a static schema
-- constraint can't express "was FAILED a moment ago, now RESCHEDULED."
--
-- No ON DELETE CASCADE on either FK: this project has no orders- or
-- users-deletion endpoint, so plain NO ACTION is safe and consistent
-- with every other FK in this schema.
CREATE TABLE reschedule_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders(id),
    requested_by    UUID NOT NULL REFERENCES users(id),
    requested_date  DATE NOT NULL,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reschedule_requests_order_id ON reschedule_requests (order_id);
