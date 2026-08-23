-- Revisits the 0004 migration's original decision that "deactivating the
-- parent zone is sufficient" for areas. In practice an admin needs to
-- retire a single area (e.g. it's no longer serviced) without taking
-- every other area in the same zone offline too — so areas now get their
-- own independent active flag, mirroring zones.active exactly: default
-- true, never deleted, toggled via the same UPDATE .../areas/{id}
-- endpoint that already handles renaming (see zones.UpdateAreaHandler).
-- Same deletion-semantics rationale as 0004 still applies unchanged —
-- orders.pickup_area_id/drop_area_id are NOT NULL FKs with no ON DELETE
-- CASCADE, so physical deletion remains unsupported for areas.

ALTER TABLE areas ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
