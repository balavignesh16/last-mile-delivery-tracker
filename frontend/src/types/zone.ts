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
  // Both null until an admin sets real coordinates — used by
  // auto-assignment to rank agents by real distance to this area when
  // it's a pickup point (see docs/assignment-engine.md). Never
  // geocoded; always a real value an admin entered.
  latitude: number | null
  longitude: number | null
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

// latitude/longitude are optional, and must be provided together —
// see backend zones.validateOptionalCoordinates.
export interface CreateAreaInput {
  name: string
  latitude?: number
  longitude?: number
}

// active/latitude/longitude are optional: omitting any of them leaves
// that field unchanged on the backend — same "nil means unchanged"
// contract as UpdateZoneInput.active.
export interface UpdateAreaInput {
  name: string
  active?: boolean
  latitude?: number
  longitude?: number
}
