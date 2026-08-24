import { ChevronRight, MapPin, Plus, Search } from 'lucide-react'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { Modal } from '../../components/Modal'
import { PageHeader } from '../../components/PageHeader'
import { Pagination } from '../../components/Pagination'
import { Select } from '../../components/Select'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { createArea, createZone, listAreas, listZones, updateArea, updateZone } from '../../services/zones'
import type { Area, Zone } from '../../types/zone'

// Rows shown per page — beyond this, zones paginate out of view
// instead of piling into one long, ever-growing list.
const PAGE_SIZE = 20

// Both fields blank is valid (no coordinates set — the normal case);
// exactly one filled is rejected client-side before ever reaching the
// backend's own "both or neither" rule (zones.validateOptionalCoordinates).
function parseOptionalCoordinates(latRaw: string, lngRaw: string): { latitude?: number; longitude?: number; error?: string } {
  const lat = latRaw.trim()
  const lng = lngRaw.trim()
  if (!lat && !lng) return {}
  if (!lat || !lng) return { error: 'Latitude and longitude must both be provided together.' }
  const latitude = Number(lat)
  const longitude = Number(lng)
  if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) {
    return { error: 'Latitude and longitude must be valid numbers.' }
  }
  return { latitude, longitude }
}

// Admin zone/area management (Round 6): a full-width zone workspace —
// search/status toolbar, dense table, and a full-width area-management
// workspace that replaces the table (not a permanently-reserved empty
// side panel) once a zone is selected. Creation moved from a
// page-dominating form into a modal. Scope is otherwise unchanged from
// M04: create/rename zones and areas, toggle active state — no maps, no
// geofencing, no bulk import (see docs/zone-management.md).
export function ZonesPage() {
  const { token } = useAuth()

  const [zones, setZones] = useState<Zone[]>([])
  const [zonesLoading, setZonesLoading] = useState(true)
  const [zonesError, setZonesError] = useState<string | null>(null)

  // Real area counts, derived from the same GET /zones/:id/areas
  // endpoint the detail view already calls — fetched once per zone
  // after the zone list loads, not fabricated. A zone whose count
  // couldn't be loaded just shows no count rather than a wrong one.
  const [areaCounts, setAreaCounts] = useState<Record<string, number>>({})

  const [zoneSearch, setZoneSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)

  const [createOpen, setCreateOpen] = useState(false)
  const [newZoneName, setNewZoneName] = useState('')
  const [zoneCreateError, setZoneCreateError] = useState<string | null>(null)
  const [creatingZone, setCreatingZone] = useState(false)

  const [selectedZoneId, setSelectedZoneId] = useState<string | null>(null)
  const [zoneEditName, setZoneEditName] = useState('')
  const [zoneEditError, setZoneEditError] = useState<string | null>(null)
  const [savingZone, setSavingZone] = useState(false)

  const [areas, setAreas] = useState<Area[]>([])
  const [areasLoading, setAreasLoading] = useState(false)
  const [areasError, setAreasError] = useState<string | null>(null)

  const [newAreaName, setNewAreaName] = useState('')
  // Optional — both left blank means "no coordinates yet" (the normal
  // case). Kept as strings since these are plain text inputs; parsed to
  // numbers only at submit time (see handleCreateArea/handleSaveArea).
  const [newAreaLat, setNewAreaLat] = useState('')
  const [newAreaLng, setNewAreaLng] = useState('')
  const [areaCreateError, setAreaCreateError] = useState<string | null>(null)
  const [creatingArea, setCreatingArea] = useState(false)

  const [editingAreaId, setEditingAreaId] = useState<string | null>(null)
  const [areaEditName, setAreaEditName] = useState('')
  const [areaEditLat, setAreaEditLat] = useState('')
  const [areaEditLng, setAreaEditLng] = useState('')
  const [areaEditError, setAreaEditError] = useState<string | null>(null)

  const selectedZone = zones.find((z) => z.id === selectedZoneId) ?? null

  const displayedZones = useMemo(() => {
    const q = zoneSearch.trim().toLowerCase()
    return zones.filter((z) => {
      if (q && !z.name.toLowerCase().includes(q)) return false
      if (statusFilter === 'active' && !z.active) return false
      if (statusFilter === 'inactive' && z.active) return false
      return true
    })
  }, [zones, zoneSearch, statusFilter])

  const hasActiveFilter = zoneSearch.trim() !== '' || statusFilter !== ''

  // A new search/filter can change how many pages exist — always land
  // back on page 1 rather than risk stranding the viewer on a now-empty
  // later page. Adjusted during render (React's documented pattern for
  // resetting state when an input changes) rather than in an effect.
  const filterKey = `${zoneSearch}|${statusFilter}`
  const [prevFilterKey, setPrevFilterKey] = useState(filterKey)
  if (filterKey !== prevFilterKey) {
    setPrevFilterKey(filterKey)
    setPage(1)
  }

  const pagedZones = useMemo(() => displayedZones.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE), [displayedZones, page])

  useEffect(() => {
    if (!token) return
    let cancelled = false
    listZones(token)
      .then((list) => {
        if (!cancelled) setZones(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setZonesError(err instanceof ApiError ? err.message : 'Could not load zones.')
      })
      .finally(() => {
        if (!cancelled) setZonesLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    if (!token || zones.length === 0) return
    let cancelled = false
    Promise.allSettled(zones.map((z) => listAreas(token, z.id).then((list) => [z.id, list.length] as const))).then((results) => {
      if (cancelled) return
      const counts: Record<string, number> = {}
      for (const result of results) {
        if (result.status === 'fulfilled') counts[result.value[0]] = result.value[1]
      }
      setAreaCounts((prev) => ({ ...prev, ...counts }))
    })
    return () => {
      cancelled = true
    }
  }, [token, zones])

  useEffect(() => {
    if (!token || !selectedZoneId) return
    let cancelled = false

    async function loadAreas(currentToken: string, zoneId: string) {
      setAreasLoading(true)
      setAreasError(null)
      try {
        const list = await listAreas(currentToken, zoneId)
        if (!cancelled) setAreas(list)
      } catch (err) {
        if (!cancelled) setAreasError(err instanceof ApiError ? err.message : 'Could not load areas.')
      } finally {
        if (!cancelled) setAreasLoading(false)
      }
    }

    void loadAreas(token, selectedZoneId)
    return () => {
      cancelled = true
    }
  }, [token, selectedZoneId])

  function handleSelectZone(zone: Zone) {
    setSelectedZoneId(zone.id)
    setZoneEditName(zone.name)
    setZoneEditError(null)
    setAreas([])
    setNewAreaName('')
    setAreaCreateError(null)
    setEditingAreaId(null)
  }

  async function handleCreateZone(e: FormEvent) {
    e.preventDefault()
    setZoneCreateError(null)
    if (!token) return
    const name = newZoneName.trim()
    if (!name) {
      setZoneCreateError('Name is required.')
      return
    }
    setCreatingZone(true)
    try {
      const created = await createZone(token, { name })
      setZones((prev) => [...prev, created])
      setAreaCounts((prev) => ({ ...prev, [created.id]: 0 }))
      setNewZoneName('')
      setCreateOpen(false)
    } catch (err) {
      setZoneCreateError(err instanceof ApiError ? err.message : 'Could not create zone.')
    } finally {
      setCreatingZone(false)
    }
  }

  async function handleRenameZone(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedZone) return
    setZoneEditError(null)
    const name = zoneEditName.trim()
    if (!name) {
      setZoneEditError('Name is required.')
      return
    }
    setSavingZone(true)
    try {
      const updated = await updateZone(token, selectedZone.id, { name })
      setZones((prev) => prev.map((z) => (z.id === updated.id ? updated : z)))
    } catch (err) {
      setZoneEditError(err instanceof ApiError ? err.message : 'Could not update zone.')
    } finally {
      setSavingZone(false)
    }
  }

  // Toggles active using the zone's currently saved name, not whatever is
  // sitting in the rename field — activating/deactivating must never have
  // an unintended side effect of also renaming the zone.
  async function handleToggleActive() {
    if (!token || !selectedZone) return
    setZoneEditError(null)
    try {
      const updated = await updateZone(token, selectedZone.id, {
        name: selectedZone.name,
        active: !selectedZone.active,
      })
      setZones((prev) => prev.map((z) => (z.id === updated.id ? updated : z)))
    } catch (err) {
      setZoneEditError(err instanceof ApiError ? err.message : 'Could not update zone.')
    }
  }

  async function handleCreateArea(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedZone) return
    setAreaCreateError(null)
    const name = newAreaName.trim()
    if (!name) {
      setAreaCreateError('Name is required.')
      return
    }
    const coords = parseOptionalCoordinates(newAreaLat, newAreaLng)
    if (coords.error) {
      setAreaCreateError(coords.error)
      return
    }
    setCreatingArea(true)
    try {
      const created = await createArea(token, selectedZone.id, { name, latitude: coords.latitude, longitude: coords.longitude })
      setAreas((prev) => [...prev, created])
      setAreaCounts((prev) => ({ ...prev, [selectedZone.id]: (prev[selectedZone.id] ?? 0) + 1 }))
      setNewAreaName('')
      setNewAreaLat('')
      setNewAreaLng('')
    } catch (err) {
      setAreaCreateError(err instanceof ApiError ? err.message : 'Could not create area.')
    } finally {
      setCreatingArea(false)
    }
  }

  function startEditArea(area: Area) {
    setEditingAreaId(area.id)
    setAreaEditName(area.name)
    setAreaEditLat(area.latitude != null ? String(area.latitude) : '')
    setAreaEditLng(area.longitude != null ? String(area.longitude) : '')
    setAreaEditError(null)
  }

  async function handleSaveArea(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedZone || !editingAreaId) return
    setAreaEditError(null)
    const name = areaEditName.trim()
    if (!name) {
      setAreaEditError('Name is required.')
      return
    }
    const coords = parseOptionalCoordinates(areaEditLat, areaEditLng)
    if (coords.error) {
      setAreaEditError(coords.error)
      return
    }
    try {
      const updated = await updateArea(token, selectedZone.id, editingAreaId, { name, latitude: coords.latitude, longitude: coords.longitude })
      setAreas((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
      setEditingAreaId(null)
    } catch (err) {
      setAreaEditError(err instanceof ApiError ? err.message : 'Could not update area.')
    }
  }

  // Toggles active using the area's currently saved name, not whatever is
  // sitting in the rename field — same reasoning as handleToggleActive
  // for zones: activating/deactivating must never have an unintended
  // side effect of also renaming.
  async function handleToggleAreaActive(area: Area) {
    if (!token || !selectedZone) return
    setAreaEditError(null)
    try {
      const updated = await updateArea(token, selectedZone.id, area.id, { name: area.name, active: !area.active })
      setAreas((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
    } catch (err) {
      setAreaEditError(err instanceof ApiError ? err.message : 'Could not update area.')
    }
  }

  const createButton = (
    <button
      type="button"
      onClick={() => setCreateOpen(true)}
      className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-navy-700"
    >
      <Plus className="h-4 w-4" aria-hidden="true" />
      Create zone
    </button>
  )

  return (
    <Layout>
      <PageHeader icon={MapPin} title="Zones" description="Manage service zones and their delivery areas." action={createButton} />

      {selectedZone ? (
        <div className="mt-6">
          <button type="button" onClick={() => setSelectedZoneId(null)} className="text-sm text-slate-500 hover:text-slate-800">
            ← Back to zones
          </button>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-lg font-semibold text-slate-900">{selectedZone.name}</h2>
            <div className="flex items-center gap-2">
              <StatusBadge label={selectedZone.active ? 'Active' : 'Inactive'} state={selectedZone.active ? 'ok' : 'error'} />
              <button
                type="button"
                onClick={() => void handleToggleActive()}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                {selectedZone.active ? 'Deactivate' : 'Activate'}
              </button>
            </div>
          </div>

          <div className="mt-6 grid gap-6 lg:grid-cols-3">
            <div className="rounded-lg border border-navy-100 bg-white p-6 shadow-sm lg:col-span-1">
              <h3 className="text-sm font-semibold text-slate-700">Zone details</h3>
              <form onSubmit={handleRenameZone} className="mt-4 space-y-3">
                <ErrorBanner message={zoneEditError} />
                <div>
                  <label htmlFor="zone_edit_name" className="block text-xs font-medium text-slate-700">
                    Name
                  </label>
                  <input
                    id="zone_edit_name"
                    type="text"
                    value={zoneEditName}
                    onChange={(e) => setZoneEditName(e.target.value)}
                    className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <button
                  type="submit"
                  disabled={savingZone}
                  className="w-full rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                >
                  {savingZone ? 'Saving…' : 'Save name'}
                </button>
              </form>
            </div>

            <div className="rounded-lg border border-navy-100 bg-white shadow-sm lg:col-span-2">
              <h3 className="border-b border-navy-100 px-6 py-4 text-sm font-semibold text-slate-700">
                Areas{areas.length > 0 ? ` (${areas.length})` : ''}
              </h3>
              <div className="px-6 py-4">
                <ErrorBanner message={areasError} />
                {areasLoading ? (
                  <p className="text-sm text-slate-500">Loading…</p>
                ) : areas.length === 0 ? (
                  <p className="text-sm text-slate-500">No areas in this zone yet.</p>
                ) : (
                  <table className="w-full text-left text-sm">
                    <thead className="text-xs font-medium tracking-wide text-slate-400 uppercase">
                      <tr>
                        <th className="py-2">Area name</th>
                        <th className="py-2">Status</th>
                        <th className="py-2 text-right">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {areas.map((area) =>
                        editingAreaId === area.id ? (
                          <tr key={area.id} className="border-t border-slate-100">
                            <td colSpan={3} className="py-2">
                              <form onSubmit={handleSaveArea} className="flex flex-wrap items-center gap-2">
                                <ErrorBanner message={areaEditError} />
                                <input
                                  aria-label="Edit area name"
                                  type="text"
                                  value={areaEditName}
                                  onChange={(e) => setAreaEditName(e.target.value)}
                                  className="flex-1 rounded-md border border-slate-300 px-3 py-1 text-sm"
                                />
                                <input
                                  aria-label="Edit area latitude"
                                  type="text"
                                  inputMode="decimal"
                                  placeholder="Latitude"
                                  value={areaEditLat}
                                  onChange={(e) => setAreaEditLat(e.target.value)}
                                  className="w-28 rounded-md border border-slate-300 px-3 py-1 text-sm"
                                />
                                <input
                                  aria-label="Edit area longitude"
                                  type="text"
                                  inputMode="decimal"
                                  placeholder="Longitude"
                                  value={areaEditLng}
                                  onChange={(e) => setAreaEditLng(e.target.value)}
                                  className="w-28 rounded-md border border-slate-300 px-3 py-1 text-sm"
                                />
                                <button
                                  type="submit"
                                  className="rounded-md border border-slate-300 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100"
                                >
                                  Save
                                </button>
                                <button
                                  type="button"
                                  onClick={() => setEditingAreaId(null)}
                                  className="rounded-md px-2 py-1 text-xs text-slate-500 hover:text-slate-700"
                                >
                                  Cancel
                                </button>
                              </form>
                            </td>
                          </tr>
                        ) : (
                          <tr key={area.id} className="border-t border-slate-100">
                            <td className="py-2.5 font-medium text-slate-900">
                              {area.name}
                              {area.latitude != null && area.longitude != null && (
                                <div className="mt-0.5 text-xs font-normal text-slate-400">
                                  {area.latitude.toFixed(4)}, {area.longitude.toFixed(4)}
                                </div>
                              )}
                            </td>
                            <td className="py-2.5">
                              <StatusBadge label={area.active ? 'Active' : 'Inactive'} state={area.active ? 'ok' : 'error'} />
                            </td>
                            <td className="py-2.5 text-right">
                              <span className="inline-flex gap-3">
                                <button
                                  type="button"
                                  onClick={() => startEditArea(area)}
                                  className="text-xs font-medium text-slate-500 hover:text-slate-800"
                                >
                                  Rename
                                </button>
                                <button
                                  type="button"
                                  onClick={() => void handleToggleAreaActive(area)}
                                  className="text-xs font-medium text-slate-500 hover:text-slate-800"
                                >
                                  {area.active ? 'Deactivate' : 'Activate'}
                                </button>
                              </span>
                            </td>
                          </tr>
                        ),
                      )}
                    </tbody>
                  </table>
                )}

                <form onSubmit={handleCreateArea} className="mt-4 flex flex-wrap items-end gap-3 border-t border-slate-100 pt-4">
                  <div className="min-w-40 flex-1">
                    <label htmlFor="area_name" className="block text-xs font-medium text-slate-700">
                      New area name
                    </label>
                    <ErrorBanner message={areaCreateError} />
                    <input
                      id="area_name"
                      type="text"
                      value={newAreaName}
                      onChange={(e) => setNewAreaName(e.target.value)}
                      className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
                    />
                  </div>
                  <div>
                    <label htmlFor="area_latitude" className="block text-xs font-medium text-slate-700">
                      Latitude (optional)
                    </label>
                    <input
                      id="area_latitude"
                      type="text"
                      inputMode="decimal"
                      value={newAreaLat}
                      onChange={(e) => setNewAreaLat(e.target.value)}
                      className="mt-1 w-28 rounded-md border border-slate-300 px-3 py-2 text-sm"
                    />
                  </div>
                  <div>
                    <label htmlFor="area_longitude" className="block text-xs font-medium text-slate-700">
                      Longitude (optional)
                    </label>
                    <input
                      id="area_longitude"
                      type="text"
                      inputMode="decimal"
                      value={newAreaLng}
                      onChange={(e) => setNewAreaLng(e.target.value)}
                      className="mt-1 w-28 rounded-md border border-slate-300 px-3 py-2 text-sm"
                    />
                  </div>
                  <button
                    type="submit"
                    disabled={creatingArea}
                    className="rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
                  >
                    {creatingArea ? 'Adding…' : 'Add area'}
                  </button>
                </form>
              </div>
            </div>
          </div>
        </div>
      ) : (
        <>
          <div className="mt-6 flex flex-wrap items-end gap-3 rounded-lg border border-navy-100 bg-white p-4 shadow-sm">
            <div className="min-w-56 flex-1">
              <label htmlFor="zone-search" className="block text-xs font-medium text-slate-500">
                Search
              </label>
              <div className="relative mt-1">
                <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <input
                  id="zone-search"
                  type="text"
                  aria-label="Search zones"
                  value={zoneSearch}
                  onChange={(e) => setZoneSearch(e.target.value)}
                  placeholder="Search by zone name…"
                  className="w-full rounded-md border border-slate-300 py-2 pr-3 pl-9 text-sm transition-colors focus:border-navy-500"
                />
              </div>
            </div>
            <div>
              <label htmlFor="zone-status-filter" className="block text-xs font-medium text-slate-500">
                Status
              </label>
              <Select
                id="zone-status-filter"
                value={statusFilter}
                onChange={setStatusFilter}
                placeholder="All statuses"
                className="mt-1 w-40"
                options={[
                  { value: 'active', label: 'Active' },
                  { value: 'inactive', label: 'Inactive' },
                ]}
              />
            </div>
            {hasActiveFilter && (
              <button
                type="button"
                onClick={() => {
                  setZoneSearch('')
                  setStatusFilter('')
                }}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                Clear filters
              </button>
            )}
          </div>

          <div className="mt-6 overflow-hidden rounded-lg border border-navy-100 bg-white shadow-sm">
            <ErrorBanner message={zonesError} />
            {zonesLoading ? (
              <div className="space-y-3 p-6">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
                ))}
              </div>
            ) : zones.length === 0 ? (
              <EmptyState icon={MapPin} title="No zones yet" description="Create your first zone to start organizing pickup and drop areas." />
            ) : displayedZones.length === 0 ? (
              <EmptyState icon={Search} title="No zones match your search." description="Try a different name or filter." />
            ) : (
              <>
                <div className="hidden sm:block">
                  <table className="w-full text-left text-sm">
                    <thead className="border-b border-navy-100 bg-navy-50/95 text-xs font-medium tracking-wide text-navy-700 uppercase">
                      <tr>
                        <th className="px-6 py-3">Zone</th>
                        <th className="px-6 py-3">Areas</th>
                        <th className="px-6 py-3">Status</th>
                        <th className="px-6 py-3" aria-hidden="true" />
                      </tr>
                    </thead>
                    <tbody>
                      {pagedZones.map((zone) => (
                        <tr
                          key={zone.id}
                          onClick={() => handleSelectZone(zone)}
                          className="group cursor-pointer border-t border-slate-100 transition-colors hover:bg-navy-50/50"
                        >
                          <td className="max-w-md px-6 py-3">
                            <button
                              type="button"
                              onClick={() => handleSelectZone(zone)}
                              title={zone.name}
                              className="block truncate font-medium text-navy-700 hover:underline"
                            >
                              {zone.name}
                            </button>
                          </td>
                          <td className="px-6 py-3 tabular-nums text-slate-600">{areaCounts[zone.id] ?? '—'}</td>
                          <td className="px-6 py-3">
                            <StatusBadge label={zone.active ? 'Active' : 'Inactive'} state={zone.active ? 'ok' : 'error'} />
                          </td>
                          <td className="px-6 py-3 text-right">
                            <ChevronRight className="h-4 w-4 text-slate-300 transition-colors group-hover:text-navy-500" aria-hidden="true" />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <div className="divide-y divide-slate-100 sm:hidden">
                  {pagedZones.map((zone) => (
                    <button
                      key={zone.id}
                      type="button"
                      onClick={() => handleSelectZone(zone)}
                      className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left transition-colors hover:bg-navy-50/50"
                    >
                      <span>
                        <span className="block font-medium text-slate-900">{zone.name}</span>
                        <span className="block text-xs text-slate-500">
                          {areaCounts[zone.id] ?? '—'} area{areaCounts[zone.id] === 1 ? '' : 's'}
                        </span>
                      </span>
                      <StatusBadge label={zone.active ? 'Active' : 'Inactive'} state={zone.active ? 'ok' : 'error'} />
                    </button>
                  ))}
                </div>

                <Pagination page={page} totalItems={displayedZones.length} pageSize={PAGE_SIZE} onPageChange={setPage} />
              </>
            )}
          </div>
        </>
      )}

      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Create zone">
        <form onSubmit={handleCreateZone} className="space-y-4">
          <ErrorBanner message={zoneCreateError} />
          <div>
            <label htmlFor="zone_name" className="block text-sm font-medium text-slate-700">
              Zone name
            </label>
            <input
              id="zone_name"
              type="text"
              value={newZoneName}
              onChange={(e) => setNewZoneName(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <button
            type="submit"
            disabled={creatingZone}
            className="w-full rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {creatingZone ? 'Creating…' : 'Create zone'}
          </button>
        </form>
      </Modal>
    </Layout>
  )
}
