-- M03: operational state for delivery agents, kept as its own table
-- rather than columns on users because it holds materially different
-- data (availability, location) that a customer/admin row never has.
--
-- user_id is UNIQUE, enforcing exactly one delivery_agents row per user
-- at the database level, not just in application code.
--
-- current_zone_id is deliberately a plain UUID with no foreign key yet:
-- the zones table doesn't exist until M04. M04 adds the FK constraint
-- via its own migration when that table lands — this migration must not
-- forward-reference a table that doesn't exist.
--
-- No ON DELETE CASCADE on the users FK: this project has no user-deletion
-- endpoint, and defaulting to NO ACTION means a user row can never be
-- deleted out from under an agent record it still owns, which is the
-- safer default absent an explicit reason to cascade.

CREATE TABLE delivery_agents (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL UNIQUE REFERENCES users(id),
    availability         TEXT NOT NULL DEFAULT 'OFFLINE' CHECK (availability IN ('AVAILABLE', 'BUSY', 'OFFLINE')),
    current_lat          DOUBLE PRECISION CHECK (current_lat BETWEEN -90 AND 90),
    current_lng          DOUBLE PRECISION CHECK (current_lng BETWEEN -180 AND 180),
    current_zone_id      UUID,
    location_updated_at  TIMESTAMPTZ,
    last_assigned_at     TIMESTAMPTZ,
    active               BOOLEAN NOT NULL DEFAULT true,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_agents_availability ON delivery_agents (availability);
