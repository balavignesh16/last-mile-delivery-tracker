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
        <select
          id={`${idPrefix}_zone`}
          value={zoneId}
          onChange={(e) => onZoneChange(e.target.value)}
          className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
        >
          <option value="">Select a zone…</option>
          {zones.map((z) => (
            <option key={z.id} value={z.id} disabled={!z.active}>
              {z.name} {z.active ? '' : '(inactive)'}
            </option>
          ))}
        </select>
      </div>
      <div>
        <label htmlFor={`${idPrefix}_area`} className="block text-xs font-medium text-slate-700">
          Area
        </label>
        <select
          id={`${idPrefix}_area`}
          value={areaId}
          onChange={(e) => onAreaChange(e.target.value)}
          disabled={!zoneId || areasLoading}
          className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm disabled:bg-slate-50"
        >
          <option value="">{areasLoading ? 'Loading…' : 'Select an area…'}</option>
          {areas.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </div>
    </fieldset>
  )
}
