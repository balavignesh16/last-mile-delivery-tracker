export interface Zone {
  id: string
  name: string
  active: boolean
  created_at: string
}

export interface Area {
  id: string
  name: string
  zone_id: string
  active: boolean
  created_at: string
}

export interface CreateZoneInput {
  name: string
}

// active is optional: omitting it (undefined -> not sent, per apiPut's
// JSON.stringify) leaves the zone's active state unchanged on the
// backend — see zones.ZoneUpdate's doc on the Go side.
export interface UpdateZoneInput {
  name: string
  active?: boolean
}

export interface CreateAreaInput {
  name: string
}

// active is optional: omitting it leaves the area's active state
// unchanged on the backend — same "nil means unchanged" contract as
// UpdateZoneInput.active.
export interface UpdateAreaInput {
  name: string
  active?: boolean
}
