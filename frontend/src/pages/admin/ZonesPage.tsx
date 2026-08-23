import { MapPin } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { createArea, createZone, listAreas, listZones, updateArea, updateZone } from '../../services/zones'
import type { Area, Zone } from '../../types/zone'

// Minimal admin zone/area management, per M04's explicit scope: create
// and rename zones and areas, and toggle a zone's active state. No maps,
// no geofencing, no bulk import — those are out of scope by design (see
// docs/zone-management.md).
export function ZonesPage() {
  const { token } = useAuth()

  const [zones, setZones] = useState<Zone[]>([])
  const [zonesLoading, setZonesLoading] = useState(true)
  const [zonesError, setZonesError] = useState<string | null>(null)

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
  const [areaCreateError, setAreaCreateError] = useState<string | null>(null)
  const [creatingArea, setCreatingArea] = useState(false)

  const [editingAreaId, setEditingAreaId] = useState<string | null>(null)
  const [areaEditName, setAreaEditName] = useState('')
  const [areaEditError, setAreaEditError] = useState<string | null>(null)

  const selectedZone = zones.find((z) => z.id === selectedZoneId) ?? null

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
      setNewZoneName('')
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
    setCreatingArea(true)
    try {
      const created = await createArea(token, selectedZone.id, { name })
      setAreas((prev) => [...prev, created])
      setNewAreaName('')
    } catch (err) {
      setAreaCreateError(err instanceof ApiError ? err.message : 'Could not create area.')
    } finally {
      setCreatingArea(false)
    }
  }

  function startEditArea(area: Area) {
    setEditingAreaId(area.id)
    setAreaEditName(area.name)
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
    try {
      const updated = await updateArea(token, selectedZone.id, editingAreaId, { name })
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

  return (
    <Layout>
      <div className="flex items-center gap-2.5">
        <span className="flex h-9 w-9 items-center justify-center rounded-md bg-navy-50 text-navy-600">
          <MapPin className="h-5 w-5" aria-hidden="true" />
        </span>
        <h1 className="text-xl font-semibold">Zones</h1>
      </div>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Create a new zone</h2>
        <form onSubmit={handleCreateZone} className="mt-4 flex flex-wrap items-end gap-4">
          <ErrorBanner message={zoneCreateError} />
          <div className="flex-1">
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
            className="rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {creatingZone ? 'Creating…' : 'Create zone'}
          </button>
        </form>
      </div>

      <div className="mt-6 grid gap-6 md:grid-cols-2">
        <div className="rounded-lg border border-navy-100 bg-white shadow-sm">
          <h2 className="border-b border-navy-100 px-6 py-4 text-sm font-semibold text-slate-700">All zones</h2>
          <ErrorBanner message={zonesError} />
          {zonesLoading ? (
            <p className="px-6 py-4 text-sm text-slate-500">Loading…</p>
          ) : zones.length === 0 ? (
            <p className="px-6 py-4 text-sm text-slate-500">No zones yet.</p>
          ) : (
            <ul>
              {zones.map((zone) => (
                <li key={zone.id} className="border-t border-slate-100 first:border-t-0">
                  <button
                    type="button"
                    onClick={() => handleSelectZone(zone)}
                    className={`flex w-full items-center justify-between px-6 py-3 text-left text-sm transition-colors hover:bg-navy-50 ${
                      zone.id === selectedZoneId ? 'bg-navy-50' : ''
                    }`}
                  >
                    <span>{zone.name}</span>
                    <StatusBadge label={zone.active ? 'Active' : 'Inactive'} state={zone.active ? 'ok' : 'error'} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="rounded-lg border border-navy-100 bg-white shadow-sm">
          {!selectedZone ? (
            <p className="px-6 py-8 text-center text-sm text-slate-500">Select a zone to manage its areas.</p>
          ) : (
            <>
              <div className="border-b border-navy-100 px-6 py-4">
                <h2 className="text-sm font-semibold text-slate-700">Manage zone</h2>
                <form onSubmit={handleRenameZone} className="mt-3 flex flex-wrap items-end gap-3">
                  <ErrorBanner message={zoneEditError} />
                  <div className="flex-1">
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
                    className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                  >
                    Save name
                  </button>
                  <button
                    type="button"
                    onClick={handleToggleActive}
                    className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
                  >
                    {selectedZone.active ? 'Deactivate' : 'Activate'}
                  </button>
                </form>
              </div>

              <div className="px-6 py-4">
                <h3 className="text-sm font-semibold text-slate-700">Areas in {selectedZone.name}</h3>
                <ErrorBanner message={areasError} />
                {areasLoading ? (
                  <p className="mt-3 text-sm text-slate-500">Loading…</p>
                ) : areas.length === 0 ? (
                  <p className="mt-3 text-sm text-slate-500">No areas in this zone yet.</p>
                ) : (
                  <ul className="mt-3 space-y-2">
                    {areas.map((area) =>
                      editingAreaId === area.id ? (
                        <li key={area.id}>
                          <form onSubmit={handleSaveArea} className="flex items-center gap-2">
                            <ErrorBanner message={areaEditError} />
                            <input
                              aria-label="Edit area name"
                              type="text"
                              value={areaEditName}
                              onChange={(e) => setAreaEditName(e.target.value)}
                              className="flex-1 rounded-md border border-slate-300 px-3 py-1 text-sm"
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
                        </li>
                      ) : (
                        <li key={area.id} className="flex items-center justify-between gap-2 rounded-md border border-slate-100 px-3 py-2 text-sm">
                          <span className="flex items-center gap-2">
                            {area.name}
                            {!area.active && <StatusBadge label="Inactive" state="error" />}
                          </span>
                          <span className="flex shrink-0 items-center gap-3">
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
                        </li>
                      ),
                    )}
                  </ul>
                )}

                <form onSubmit={handleCreateArea} className="mt-4 flex items-end gap-3">
                  <ErrorBanner message={areaCreateError} />
                  <div className="flex-1">
                    <label htmlFor="area_name" className="block text-xs font-medium text-slate-700">
                      New area name
                    </label>
                    <input
                      id="area_name"
                      type="text"
                      value={newAreaName}
                      onChange={(e) => setNewAreaName(e.target.value)}
                      className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
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
            </>
          )}
        </div>
      </div>
    </Layout>
  )
}
