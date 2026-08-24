import { MapPin, Navigation, Truck } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
import { Select } from '../../components/Select'
import { StatusBadge } from '../../components/StatusBadge'
import { useAuth } from '../../hooks/useAuth'
import { ApiError } from '../../services/api'
import { fetchMyAgentProfile, updateAgentAvailability, updateAgentLocation, updateAgentZone } from '../../services/agents'
import { listZones } from '../../services/zones'
import type { Agent, Availability } from '../../types/agent'
import type { Zone } from '../../types/zone'

const AVAILABILITY_OPTIONS: Availability[] = ['AVAILABLE', 'BUSY', 'OFFLINE']

const AVAILABILITY_STATE: Record<Availability, 'ok' | 'degraded' | 'error'> = {
  AVAILABLE: 'ok',
  BUSY: 'degraded',
  OFFLINE: 'error',
}

function geolocationErrorMessage(error: GeolocationPositionError): string {
  switch (error.code) {
    case error.PERMISSION_DENIED:
      return 'Location permission was denied. Enter your coordinates manually below.'
    case error.POSITION_UNAVAILABLE:
      return 'Your current location could not be determined. Enter your coordinates manually below.'
    case error.TIMEOUT:
      return 'Locating your position took too long. Enter your coordinates manually below.'
    default:
      return 'Could not determine your current location. Enter your coordinates manually below.'
  }
}

// Agent-side operational controls: view/change own availability, report
// current location, and pick a current operating zone. Location is a
// manual latitude/longitude form, optionally pre-filled via the
// browser's native Geolocation API (no maps, no third-party service, no
// new dependency) — an agent can still type coordinates by hand if they
// decline the permission prompt or their browser/context doesn't
// support it. Zone selection is a plain dropdown of real
// zones (GET /zones, read-access widened for DELIVERY_AGENT) rather than
// deriving a zone from lat/lng — there is no zone-boundary geometry
// anywhere in this schema to geofence against (see
// docs/assignment-engine.md), so an agent states their zone directly.
// current_zone_id is what assignment.IsEligible (M09) actually requires
// to be non-nil; without this control an agent using the app could never
// become eligible for auto-assignment.
export function OperationsPage() {
  const { token } = useAuth()
  const geolocationSupported = typeof navigator !== 'undefined' && !!navigator.geolocation

  const [agent, setAgent] = useState<Agent | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [availabilityError, setAvailabilityError] = useState<string | null>(null)
  const [savingAvailability, setSavingAvailability] = useState(false)

  const [latitude, setLatitude] = useState('')
  const [longitude, setLongitude] = useState('')
  const [locationError, setLocationError] = useState<string | null>(null)
  const [locationSuccess, setLocationSuccess] = useState(false)
  const [savingLocation, setSavingLocation] = useState(false)
  const [locatingViaGPS, setLocatingViaGPS] = useState(false)

  const [zones, setZones] = useState<Zone[]>([])
  const [selectedZoneId, setSelectedZoneId] = useState('')
  const [zoneError, setZoneError] = useState<string | null>(null)
  const [zoneSuccess, setZoneSuccess] = useState(false)
  const [savingZone, setSavingZone] = useState(false)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    fetchMyAgentProfile(token)
      .then((profile) => {
        if (cancelled) return
        setAgent(profile)
        if (profile.current_lat != null) setLatitude(String(profile.current_lat))
        if (profile.current_lng != null) setLongitude(String(profile.current_lng))
        if (profile.current_zone_id != null) setSelectedZoneId(profile.current_zone_id)
      })
      .catch((err: unknown) => {
        if (!cancelled) setLoadError(err instanceof ApiError ? err.message : 'Could not load agent profile.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    listZones(token)
      .then((list) => {
        if (!cancelled) setZones(list.filter((z) => z.active))
      })
      .catch(() => {
        // Zone list is a convenience for the dropdown below; a failure
        // here shouldn't block availability/location, which don't need it.
      })
    return () => {
      cancelled = true
    }
  }, [token])

  async function handleAvailabilityChange(next: Availability) {
    setAvailabilityError(null)
    if (!token || !agent) return

    setSavingAvailability(true)
    try {
      const updated = await updateAgentAvailability(token, agent.id, next)
      setAgent(updated)
    } catch (err) {
      setAvailabilityError(err instanceof ApiError ? err.message : 'Could not update availability.')
    } finally {
      setSavingAvailability(false)
    }
  }

  async function handleLocationSubmit(e: FormEvent) {
    e.preventDefault()
    setLocationError(null)
    setLocationSuccess(false)
    if (!token || !agent) return

    const lat = Number(latitude)
    const lng = Number(longitude)
    if (latitude.trim() === '' || longitude.trim() === '' || Number.isNaN(lat) || Number.isNaN(lng)) {
      setLocationError('Latitude and longitude are both required.')
      return
    }
    if (lat < -90 || lat > 90) {
      setLocationError('Latitude must be between -90 and 90.')
      return
    }
    if (lng < -180 || lng > 180) {
      setLocationError('Longitude must be between -180 and 180.')
      return
    }

    setSavingLocation(true)
    try {
      const updated = await updateAgentLocation(token, agent.id, lat, lng)
      setAgent(updated)
      setLocationSuccess(true)
    } catch (err) {
      setLocationError(err instanceof ApiError ? err.message : 'Could not update location.')
    } finally {
      setSavingLocation(false)
    }
  }

  function handleUseCurrentLocation() {
    setLocationError(null)
    setLocationSuccess(false)
    setLocatingViaGPS(true)
    navigator.geolocation.getCurrentPosition(
      (position) => {
        setLatitude(String(position.coords.latitude))
        setLongitude(String(position.coords.longitude))
        setLocatingViaGPS(false)
      },
      (error) => {
        setLocationError(geolocationErrorMessage(error))
        setLocatingViaGPS(false)
      },
      { timeout: 10000 },
    )
  }

  async function handleZoneSubmit(e: FormEvent) {
    e.preventDefault()
    setZoneError(null)
    setZoneSuccess(false)
    if (!token || !agent) return

    if (selectedZoneId === '') {
      setZoneError('Choose a zone.')
      return
    }

    setSavingZone(true)
    try {
      const updated = await updateAgentZone(token, agent.id, selectedZoneId)
      setAgent(updated)
      setZoneSuccess(true)
    } catch (err) {
      setZoneError(err instanceof ApiError ? err.message : 'Could not update zone.')
    } finally {
      setSavingZone(false)
    }
  }

  if (loading) {
    return (
      <Layout>
        <p className="text-sm text-slate-500">Loading…</p>
      </Layout>
    )
  }

  if (loadError || !agent) {
    return (
      <Layout>
        <ErrorBanner message={loadError ?? 'No agent profile found for this account.'} />
      </Layout>
    )
  }

  return (
    <Layout>
      <div className="flex items-center gap-2.5">
        <span className="flex h-9 w-9 items-center justify-center rounded-md bg-navy-50 text-navy-600">
          <Truck className="h-5 w-5" aria-hidden="true" />
        </span>
        <h1 className="text-xl font-semibold">Delivery Operations</h1>
      </div>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-slate-700">Availability</h2>
          <StatusBadge label={agent.availability} state={AVAILABILITY_STATE[agent.availability]} />
        </div>
        <ErrorBanner message={availabilityError} />
        <div className="mt-4 flex gap-2">
          {AVAILABILITY_OPTIONS.map((option) => (
            <button
              key={option}
              type="button"
              disabled={savingAvailability || agent.availability === option}
              onClick={() => handleAvailabilityChange(option)}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {option}
            </button>
          ))}
        </div>
      </div>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-700">
          <MapPin className="h-4 w-4 text-navy-500" aria-hidden="true" />
          Current zone
        </h2>
        <p className="mt-1 text-xs text-slate-500">
          The zone you're currently operating in. This is what makes you eligible for auto-assignment — orders in
          this zone are offered to you first.
        </p>
        <form onSubmit={handleZoneSubmit} className="mt-4 flex flex-wrap items-end gap-3">
          <ErrorBanner message={zoneError} />
          {zoneSuccess && (
            <div role="status" className="w-full rounded-md border border-emerald-300 bg-emerald-50 px-4 py-2 text-sm text-emerald-800">
              Zone updated.
            </div>
          )}
          <div className="min-w-48">
            <label htmlFor="zone" className="block text-sm font-medium text-slate-700">
              Zone
            </label>
            <Select
              id="zone"
              value={selectedZoneId}
              onChange={setSelectedZoneId}
              placeholder="Choose a zone…"
              className="mt-1"
              options={zones.map((z) => ({ value: z.id, label: z.name }))}
            />
          </div>
          <button
            type="submit"
            disabled={savingZone}
            className="rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {savingZone ? 'Saving…' : 'Update zone'}
          </button>
        </form>
      </div>

      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-700">
          <Navigation className="h-4 w-4 text-navy-500" aria-hidden="true" />
          Current location
        </h2>
        {geolocationSupported && (
          <button
            type="button"
            onClick={handleUseCurrentLocation}
            disabled={locatingViaGPS || savingLocation}
            className="mt-3 flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
          >
            <Navigation className="h-3.5 w-3.5" aria-hidden="true" />
            {locatingViaGPS ? 'Locating…' : 'Use my current location'}
          </button>
        )}
        <form onSubmit={handleLocationSubmit} className="mt-4 grid gap-4 sm:grid-cols-2">
          <ErrorBanner message={locationError} />
          {locationSuccess && (
            <div role="status" className="sm:col-span-2 rounded-md border border-emerald-300 bg-emerald-50 px-4 py-2 text-sm text-emerald-800">
              Location updated.
            </div>
          )}

          <div>
            <label htmlFor="latitude" className="block text-sm font-medium text-slate-700">
              Latitude
            </label>
            <input
              id="latitude"
              type="number"
              step="any"
              value={latitude}
              onChange={(e) => setLatitude(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label htmlFor="longitude" className="block text-sm font-medium text-slate-700">
              Longitude
            </label>
            <input
              id="longitude"
              type="number"
              step="any"
              value={longitude}
              onChange={(e) => setLongitude(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <div className="sm:col-span-2">
            <button
              type="submit"
              disabled={savingLocation}
              className="rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
            >
              {savingLocation ? 'Saving…' : 'Update location'}
            </button>
          </div>
        </form>
        {agent.location_updated_at && (
          <p className="mt-3 text-xs text-slate-500">
            Last reported: {new Date(agent.location_updated_at).toLocaleString()}
          </p>
        )}
      </div>
    </Layout>
  )
}
