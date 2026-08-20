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

export interface UpdateAreaInput {
  name: string
}
