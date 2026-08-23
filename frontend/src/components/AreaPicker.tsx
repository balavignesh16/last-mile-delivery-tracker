import { Select } from './Select'
import type { Area, Zone } from '../types/zone'

// A pickup/drop area picker: pick a zone, then an area within it. There
// is no free-text address field — M04 never offered geocoding, so an
// area (an admin-configured, resolvable unit) is the smallest thing a
// customer can select. See docs/zone-management.md's "Address
// resolution" section. Shared by QuotePage (M06) and CreateOrderPage
// (M07) — both need the exact same pickup/drop selection UI, so it
// lives here once rather than being copied.
export function AreaPicker({
  idPrefix,
  label,
  zones,
  zoneId,
  onZoneChange,
  areas,
  areasLoading,
  areaId,
  onAreaChange,
}: {
  idPrefix: string
  label: string
  zones: Zone[]
  zoneId: string
  onZoneChange: (zoneId: string) => void
  areas: Area[]
  areasLoading: boolean
  areaId: string
  onAreaChange: (areaId: string) => void
}) {
  return (
    <fieldset className="grid gap-3 sm:grid-cols-2">
      <legend className="mb-1 text-sm font-semibold text-slate-700">{label}</legend>
      <div>
        <label htmlFor={`${idPrefix}_zone`} className="block text-xs font-medium text-slate-700">
          Zone
        </label>
        <Select
          id={`${idPrefix}_zone`}
          value={zoneId}
          onChange={onZoneChange}
          placeholder="Select a zone…"
          className="mt-1"
          options={zones.map((z) => ({ value: z.id, label: `${z.name}${z.active ? '' : ' (inactive)'}`, disabled: !z.active }))}
        />
      </div>
      <div>
        <label htmlFor={`${idPrefix}_area`} className="block text-xs font-medium text-slate-700">
          Area
        </label>
        <Select
          id={`${idPrefix}_area`}
          value={areaId}
          onChange={onAreaChange}
          disabled={!zoneId || areasLoading}
          placeholder={areasLoading ? 'Loading…' : 'Select an area…'}
          className="mt-1"
          options={areas.map((a) => ({ value: a.id, label: a.name }))}
        />
      </div>
    </fieldset>
  )
}
