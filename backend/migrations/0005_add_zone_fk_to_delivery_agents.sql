-- M04: completes the FK that M03's delivery_agents migration
-- deliberately deferred (see 0002's comment) because the zones table
-- didn't exist yet. From here on, current_zone_id can only ever hold
-- NULL or a real zones.id — never an arbitrary UUID.
--
-- No M03 handler writes to current_zone_id today (PUT
-- .../agents/{id}/location only ever touches current_lat/current_lng),
-- so this migration has no existing write path to break; it simply
-- closes the constraint gap for whichever later module starts writing
-- to the column.

ALTER TABLE delivery_agents
    ADD CONSTRAINT fk_delivery_agents_current_zone_id
    FOREIGN KEY (current_zone_id) REFERENCES zones(id);
