-- Adds optional area-level coordinates so auto-assignment can rank
-- eligible agents by real geographic distance to the pickup point
-- (Haversine, internal/geo) instead of the zone-match proxy alone, when
-- both the candidate agent and the order's pickup area have known
-- coordinates. Nullable, no backfill: this project never fabricates
-- data (see migration 0015's identical reasoning for actor_role) —
-- every existing area starts with no coordinates until an admin sets
-- real ones via the area create/update endpoints, and auto-assignment
-- falls back to its existing zone-based ranking whenever either side is
-- missing a coordinate. Mirrors delivery_agents.current_lat/current_lng
-- (migration 0002) exactly: DOUBLE PRECISION, identical range CHECKs.

ALTER TABLE areas ADD COLUMN latitude DOUBLE PRECISION CHECK (latitude BETWEEN -90 AND 90);
ALTER TABLE areas ADD COLUMN longitude DOUBLE PRECISION CHECK (longitude BETWEEN -180 AND 180);
