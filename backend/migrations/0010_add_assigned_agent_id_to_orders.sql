-- M09: current assignment state. No new table — assignment *history* is
-- not duplicated here: every CREATED/RESCHEDULED -> ASSIGNED tracking
-- event (M08) already carries `metadata: {"assigned_agent_id": "..."}`,
-- so order_tracking_events already IS the assignment history. This
-- column only ever needs to hold the *current* value, the same
-- "snapshot, not derived" precedent orders.status itself set.
--
-- No ON DELETE behavior: this project has no agent-deletion endpoint
-- (agents are provisioned, never deleted — same precedent as
-- users/zones/rate cards), so plain NO ACTION is safe and consistent
-- with every other FK in this schema.
ALTER TABLE orders
    ADD COLUMN assigned_agent_id UUID REFERENCES delivery_agents(id);

CREATE INDEX idx_orders_assigned_agent_id ON orders (assigned_agent_id);

-- The concurrency backstop: at most one order may sit in a non-terminal,
-- "an agent is actively working this" status for a given agent at a
-- time. Same partial-unique-index mechanism (and same reasoning) as
-- idx_rate_cards_one_active_per_combination (M05) — this is what makes
-- "an agent must never end up with two simultaneously active assigned
-- orders" true even if application-level locking were somehow bypassed,
-- not just true because the code is careful.
CREATE UNIQUE INDEX idx_orders_one_active_assignment_per_agent
    ON orders (assigned_agent_id)
    WHERE status IN ('ASSIGNED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY');
