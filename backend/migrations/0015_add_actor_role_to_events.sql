-- Both order_tracking_events and reschedule_requests record who acted
-- (actor_id / requested_by, a users.id) but not what role they held at
-- the time — the frontend has no reliable way to show "updated by an
-- admin" vs "updated by the assigned agent" vs "updated by the
-- customer" without it (actor_id alone is ambiguous: an admin and the
-- order's own customer/agent are all just UUIDs). Both columns are
-- nullable and get no backfill for existing rows — this project never
-- fabricates data, and there is no way to know a historical actor's
-- role after the fact, so those rows honestly show as unknown rather
-- than guessed. Every new event/request going forward is written with
-- a role by the same handler code that already knows it from the
-- caller's verified JWT.

ALTER TABLE order_tracking_events ADD COLUMN actor_role TEXT;
ALTER TABLE reschedule_requests ADD COLUMN requested_by_role TEXT;
