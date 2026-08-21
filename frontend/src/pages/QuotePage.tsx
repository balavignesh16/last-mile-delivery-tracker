import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { AreaPicker } from '../components/AreaPicker'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { requestQuote } from '../services/quote'
import { listAreas, listZones } from '../services/zones'
import type { OrderType } from '../types/rate'
import type { PaymentType, QuoteResult } from '../types/quote'
import type { Area, Zone } from '../types/zone'
import { formatCurrency } from '../utils/currency'

const ORDER_TYPES: OrderType[] = ['B2B', 'B2C']
const PAYMENT_TYPES: PaymentType[] = ['PREPAID', 'COD']

// The M06 quote flow: pick pickup/drop areas, enter package details,
// request a quote, see the full breakdown. This page never creates or
// persists an order — POST /orders/quote is stateless, and order
// confirmation/persistence is M07's job, not this one. Available to
// both CUSTOMER (requesting their own quote) and ADMIN (previewing one),
// matching the backend's RBAC on this endpoint.
export function QuotePage() {
  const { token } = useAuth()

  const [zones, setZones] = useState<Zone[]>([])
  const [zonesError, setZonesError] = useState<string | null>(null)

  const [pickupZoneId, setPickupZoneId] = useState('')
  const [pickupAreaId, setPickupAreaId] = useState('')
  const [pickupAreas, setPickupAreas] = useState<Area[]>([])
  const [pickupAreasLoading, setPickupAreasLoading] = useState(false)

  const [dropZoneId, setDropZoneId] = useState('')
  const [dropAreaId, setDropAreaId] = useState('')
  const [dropAreas, setDropAreas] = useState<Area[]>([])
  const [dropAreasLoading, setDropAreasLoading] = useState(false)

  const [orderType, setOrderType] = useState<OrderType>('B2C')
  const [paymentType, setPaymentType] = useState<PaymentType>('PREPAID')
  const [lengthCm, setLengthCm] = useState('')
  const [breadthCm, setBreadthCm] = useState('')
  const [heightCm, setHeightCm] = useState('')
  const [actualWeightKg, setActualWeightKg] = useState('')

  const [submitError, setSubmitError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [quote, setQuote] = useState<QuoteResult | null>(null)

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
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    if (!token || !pickupZoneId) {
      return
    }
    let cancelled = false
    async function load(currentToken: string, zoneId: string) {
      setPickupAreasLoading(true)
      try {
        const list = await listAreas(currentToken, zoneId)
        if (!cancelled) setPickupAreas(list)
      } catch (err) {
        if (!cancelled) setSubmitError(err instanceof ApiError ? err.message : 'Could not load pickup areas.')
      } finally {
        if (!cancelled) setPickupAreasLoading(false)
      }
    }
    void load(token, pickupZoneId)
    return () => {
      cancelled = true
    }
  }, [token, pickupZoneId])

  useEffect(() => {
    if (!token || !dropZoneId) {
      return
    }
    let cancelled = false
    async function load(currentToken: string, zoneId: string) {
      setDropAreasLoading(true)
      try {
        const list = await listAreas(currentToken, zoneId)
        if (!cancelled) setDropAreas(list)
      } catch (err) {
        if (!cancelled) setSubmitError(err instanceof ApiError ? err.message : 'Could not load drop areas.')
      } finally {
        if (!cancelled) setDropAreasLoading(false)
      }
    }
    void load(token, dropZoneId)
    return () => {
      cancelled = true
    }
  }, [token, dropZoneId])

  function handlePickupZoneChange(zoneId: string) {
    setPickupZoneId(zoneId)
    setPickupAreaId('')
    setPickupAreas([])
  }

  function handleDropZoneChange(zoneId: string) {
    setDropZoneId(zoneId)
    setDropAreaId('')
    setDropAreas([])
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitError(null)
    setQuote(null)
    if (!token) return

    if (!pickupAreaId || !dropAreaId) {
      setSubmitError('Select a pickup area and a drop area.')
      return
    }
    const length = Number(lengthCm)
    const breadth = Number(breadthCm)
    const height = Number(heightCm)
    const weight = Number(actualWeightKg)
    if ([length, breadth, height, weight].some((v) => Number.isNaN(v) || v <= 0)) {
      setSubmitError('Length, breadth, height, and actual weight must all be numbers greater than zero.')
      return
    }

    setSubmitting(true)
    try {
      const result = await requestQuote(token, {
        pickup_area_id: pickupAreaId,
        drop_area_id: dropAreaId,
        order_type: orderType,
        payment_type: paymentType,
        length_cm: length,
        breadth_cm: breadth,
        height_cm: height,
        actual_weight_kg: weight,
      })
      setQuote(result)
    } catch (err) {
      setSubmitError(err instanceof ApiError ? err.message : 'Could not calculate a quote.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Layout>
      <h1 className="text-xl font-semibold">Get a delivery quote</h1>
      <p className="mt-1 text-sm text-slate-500">
        Pick a pickup and drop area, enter your package details, and see the calculated charge before placing an
        order.
      </p>

      <form onSubmit={handleSubmit} className="mt-6 space-y-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <ErrorBanner message={zonesError} />
        <ErrorBanner message={submitError} />

        <AreaPicker
          idPrefix="pickup"
          label="Pickup"
          zones={zones}
          zoneId={pickupZoneId}
          onZoneChange={handlePickupZoneChange}
          areas={pickupAreas}
          areasLoading={pickupAreasLoading}
          areaId={pickupAreaId}
          onAreaChange={setPickupAreaId}
        />

        <AreaPicker
          idPrefix="drop"
          label="Drop"
          zones={zones}
          zoneId={dropZoneId}
          onZoneChange={handleDropZoneChange}
          areas={dropAreas}
          areasLoading={dropAreasLoading}
          areaId={dropAreaId}
          onAreaChange={setDropAreaId}
        />

        <fieldset className="grid gap-3 sm:grid-cols-2">
          <legend className="mb-1 text-sm font-semibold text-slate-700">Order details</legend>
          <div>
            <label htmlFor="quote_order_type" className="block text-xs font-medium text-slate-700">
              Order type
            </label>
            <select
              id="quote_order_type"
              value={orderType}
              onChange={(e) => setOrderType(e.target.value as OrderType)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              {ORDER_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="quote_payment_type" className="block text-xs font-medium text-slate-700">
              Payment type
            </label>
            <select
              id="quote_payment_type"
              value={paymentType}
              onChange={(e) => setPaymentType(e.target.value as PaymentType)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              {PAYMENT_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
        </fieldset>

        <fieldset className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <legend className="mb-1 text-sm font-semibold text-slate-700">Package details</legend>
          <div>
            <label htmlFor="quote_length" className="block text-xs font-medium text-slate-700">
              Length (cm)
            </label>
            <input
              id="quote_length"
              type="number"
              min="0"
              step="0.01"
              value={lengthCm}
              onChange={(e) => setLengthCm(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="quote_breadth" className="block text-xs font-medium text-slate-700">
              Breadth (cm)
            </label>
            <input
              id="quote_breadth"
              type="number"
              min="0"
              step="0.01"
              value={breadthCm}
              onChange={(e) => setBreadthCm(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="quote_height" className="block text-xs font-medium text-slate-700">
              Height (cm)
            </label>
            <input
              id="quote_height"
              type="number"
              min="0"
              step="0.01"
              value={heightCm}
              onChange={(e) => setHeightCm(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="quote_weight" className="block text-xs font-medium text-slate-700">
              Actual weight (kg)
            </label>
            <input
              id="quote_weight"
              type="number"
              min="0"
              step="0.01"
              value={actualWeightKg}
              onChange={(e) => setActualWeightKg(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
        </fieldset>

        <button
          type="submit"
          disabled={submitting}
          className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
        >
          {submitting ? 'Calculating…' : 'Get quote'}
        </button>
      </form>

      {quote && (
        <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm" role="status">
          <h2 className="text-sm font-semibold text-slate-700">Quote</h2>
          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-slate-500">Zone relationship</dt>
              <dd className="font-medium text-slate-900">{quote.zone_relationship}</dd>
            </div>
            <div>
              <dt className="text-slate-500">Volumetric weight</dt>
              <dd className="font-medium text-slate-900">{quote.volumetric_weight_kg} kg</dd>
            </div>
            <div>
              <dt className="text-slate-500">Chargeable weight</dt>
              <dd className="font-medium text-slate-900">{quote.chargeable_weight_kg} kg</dd>
            </div>
            <div>
              <dt className="text-slate-500">Base rate</dt>
              <dd className="font-medium text-slate-900">{formatCurrency(quote.base_rate)}</dd>
            </div>
            <div>
              <dt className="text-slate-500">COD surcharge</dt>
              <dd className="font-medium text-slate-900">{formatCurrency(quote.cod_surcharge)}</dd>
            </div>
            <div>
              <dt className="text-slate-500">Final amount</dt>
              <dd className="text-base font-semibold text-slate-900">{formatCurrency(quote.final_amount)}</dd>
            </div>
          </dl>
          <p className="mt-4 text-xs text-slate-500">
            This is a preview only — nothing has been booked yet.{' '}
            <Link to="/orders/new" className="font-medium text-slate-900 underline">
              Continue to place an order
            </Link>{' '}
            (the order form recalculates this quote itself — nothing shown here is reused as-is).
          </p>
        </div>
      )}
    </Layout>
  )
}
