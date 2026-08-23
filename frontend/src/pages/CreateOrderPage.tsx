import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { AreaPicker } from '../components/AreaPicker'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { Select } from '../components/Select'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { createOrder } from '../services/orders'
import { requestQuote } from '../services/quote'
import { lookupCustomerByEmail } from '../services/users'
import { listAreas, listZones } from '../services/zones'
import type { CustomerLookupResult } from '../types/auth'
import type { PaymentType, QuoteResult } from '../types/quote'
import type { OrderType } from '../types/rate'
import type { Area, Zone } from '../types/zone'
import { formatCurrency } from '../utils/currency'

const ORDER_TYPES: OrderType[] = ['B2B', 'B2C']
const PAYMENT_TYPES: PaymentType[] = ['PREPAID', 'COD']

// The M07 order-creation flow: pick pickup/drop areas and package
// details (the exact same form M06's QuotePage uses — same
// AreaPicker), preview a quote, then confirm. "Confirm" is a second,
// independent network call (POST /orders) that recomputes pricing
// itself server-side — the previewed quote shown here is display-only
// and is never sent to the server as a price; see docs/order-management.md.
//
// ADMIN additionally supplies which customer the order is for. POST
// /orders itself still needs a raw customer_id (a UUID an admin has no
// realistic way to already know) — GET /api/v1/users/lookup?email=...
// resolves the one thing an admin actually knows, the customer's email,
// into that id; the backend still validates it references a real
// CUSTOMER account before accepting the order.
export function CreateOrderPage() {
  const { token, user } = useAuth()
  const navigate = useNavigate()
  const isAdmin = user?.role === 'ADMIN'

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

  const [customerEmail, setCustomerEmail] = useState('')
  const [resolvedCustomer, setResolvedCustomer] = useState<CustomerLookupResult | null>(null)
  const [lookingUpCustomer, setLookingUpCustomer] = useState(false)
  const [customerLookupError, setCustomerLookupError] = useState<string | null>(null)

  const [orderType, setOrderType] = useState<OrderType>('B2C')
  const [paymentType, setPaymentType] = useState<PaymentType>('PREPAID')
  const [lengthCm, setLengthCm] = useState('')
  const [breadthCm, setBreadthCm] = useState('')
  const [heightCm, setHeightCm] = useState('')
  const [actualWeightKg, setActualWeightKg] = useState('')

  const [previewError, setPreviewError] = useState<string | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [preview, setPreview] = useState<QuoteResult | null>(null)

  const [confirmError, setConfirmError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)

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
    if (!token || !pickupZoneId) return
    let cancelled = false
    async function load(currentToken: string, zoneId: string) {
      setPickupAreasLoading(true)
      try {
        const list = await listAreas(currentToken, zoneId)
        if (!cancelled) setPickupAreas(list)
      } catch (err) {
        if (!cancelled) setPreviewError(err instanceof ApiError ? err.message : 'Could not load pickup areas.')
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
    if (!token || !dropZoneId) return
    let cancelled = false
    async function load(currentToken: string, zoneId: string) {
      setDropAreasLoading(true)
      try {
        const list = await listAreas(currentToken, zoneId)
        if (!cancelled) setDropAreas(list)
      } catch (err) {
        if (!cancelled) setPreviewError(err instanceof ApiError ? err.message : 'Could not load drop areas.')
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
    setPreview(null)
  }

  function handleDropZoneChange(zoneId: string) {
    setDropZoneId(zoneId)
    setDropAreaId('')
    setDropAreas([])
    setPreview(null)
  }

  async function handleLookupCustomer() {
    if (!token) return
    const email = customerEmail.trim()
    if (!email) return
    setCustomerLookupError(null)
    setResolvedCustomer(null)
    setLookingUpCustomer(true)
    try {
      const found = await lookupCustomerByEmail(token, email)
      setResolvedCustomer(found)
      setPreview(null)
    } catch (err) {
      setCustomerLookupError(err instanceof ApiError ? err.message : 'Could not find a customer with that email.')
    } finally {
      setLookingUpCustomer(false)
    }
  }

  function validatePackage(): { length: number; breadth: number; height: number; weight: number } | null {
    if (!pickupAreaId || !dropAreaId) {
      setPreviewError('Select a pickup area and a drop area.')
      return null
    }
    if (isAdmin && !resolvedCustomer) {
      setPreviewError('Find a customer by email before continuing.')
      return null
    }
    const length = Number(lengthCm)
    const breadth = Number(breadthCm)
    const height = Number(heightCm)
    const weight = Number(actualWeightKg)
    if ([length, breadth, height, weight].some((v) => Number.isNaN(v) || v <= 0)) {
      setPreviewError('Length, breadth, height, and actual weight must all be numbers greater than zero.')
      return null
    }
    return { length, breadth, height, weight }
  }

  async function handlePreview(e: FormEvent) {
    e.preventDefault()
    setPreviewError(null)
    setConfirmError(null)
    setPreview(null)
    if (!token) return

    const parsed = validatePackage()
    if (!parsed) return

    setPreviewing(true)
    try {
      const result = await requestQuote(token, {
        pickup_area_id: pickupAreaId,
        drop_area_id: dropAreaId,
        order_type: orderType,
        payment_type: paymentType,
        length_cm: parsed.length,
        breadth_cm: parsed.breadth,
        height_cm: parsed.height,
        actual_weight_kg: parsed.weight,
      })
      setPreview(result)
    } catch (err) {
      setPreviewError(err instanceof ApiError ? err.message : 'Could not calculate a quote.')
    } finally {
      setPreviewing(false)
    }
  }

  async function handleConfirm() {
    setConfirmError(null)
    if (!token) return

    const parsed = validatePackage()
    if (!parsed) return

    setConfirming(true)
    try {
      const packageInput = {
        pickup_area_id: pickupAreaId,
        drop_area_id: dropAreaId,
        order_type: orderType,
        payment_type: paymentType,
        length_cm: parsed.length,
        breadth_cm: parsed.breadth,
        height_cm: parsed.height,
        actual_weight_kg: parsed.weight,
      }
      const created =
        isAdmin && resolvedCustomer
          ? await createOrder(token, { ...packageInput, customer_id: resolvedCustomer.id })
          : await createOrder(token, packageInput)
      navigate(`/orders/${created.id}`)
    } catch (err) {
      setConfirmError(err instanceof ApiError ? err.message : 'Could not create the order.')
    } finally {
      setConfirming(false)
    }
  }

  return (
    <Layout>
      <h1 className="text-xl font-semibold">{isAdmin ? 'Create an order for a customer' : 'Place an order'}</h1>
      <p className="mt-1 text-sm text-slate-500">
        Pick a pickup and drop area, enter your package details, preview the charge, then confirm. Confirming
        recalculates the price itself — the preview is never sent back as a trusted amount.
      </p>

      {/* Purely a visual progress indicator, driven by the same `preview`
          state the form/confirm logic already uses — no new business
          logic, and neither step's label duplicates existing button/
          heading text that tests already assert on. */}
      <ol className="mt-5 flex items-center gap-3 text-sm" aria-hidden="true">
        <li className="flex items-center gap-2">
          <span
            className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold ${
              preview ? 'bg-navy-600 text-white' : 'bg-amber-500 text-white'
            }`}
          >
            1
          </span>
          <span className={preview ? 'text-slate-500' : 'font-medium text-slate-900'}>Package details</span>
        </li>
        <li className="h-0.5 w-8 bg-navy-100" />
        <li className="flex items-center gap-2">
          <span
            className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold ${
              preview ? 'bg-amber-500 text-white' : 'bg-navy-100 text-navy-400'
            }`}
          >
            2
          </span>
          <span className={preview ? 'font-medium text-slate-900' : 'text-slate-400'}>Review &amp; confirm</span>
        </li>
      </ol>

      <form onSubmit={handlePreview} className="mt-6 space-y-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <ErrorBanner message={zonesError} />
        <ErrorBanner message={previewError} />

        {isAdmin && (
          <div>
            <label htmlFor="order_customer_email" className="block text-xs font-medium text-slate-700">
              Customer email
            </label>
            <div className="mt-1 flex gap-2">
              <input
                id="order_customer_email"
                type="email"
                value={customerEmail}
                onChange={(e) => {
                  setCustomerEmail(e.target.value)
                  setResolvedCustomer(null)
                  setCustomerLookupError(null)
                  setPreview(null)
                }}
                placeholder="the customer's email"
                className="w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
              />
              <button
                type="button"
                onClick={() => void handleLookupCustomer()}
                disabled={lookingUpCustomer || !customerEmail.trim()}
                className="shrink-0 rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
              >
                {lookingUpCustomer ? 'Finding…' : 'Find customer'}
              </button>
            </div>
            {customerLookupError && <p className="mt-1 text-xs text-red-600">{customerLookupError}</p>}
            {resolvedCustomer && (
              <p className="mt-1 text-xs text-emerald-700">
                Ordering for {resolvedCustomer.full_name} ({resolvedCustomer.email})
              </p>
            )}
          </div>
        )}

        <AreaPicker
          idPrefix="order_pickup"
          label="Pickup"
          zones={zones}
          zoneId={pickupZoneId}
          onZoneChange={handlePickupZoneChange}
          areas={pickupAreas}
          areasLoading={pickupAreasLoading}
          areaId={pickupAreaId}
          onAreaChange={(id) => {
            setPickupAreaId(id)
            setPreview(null)
          }}
        />

        <AreaPicker
          idPrefix="order_drop"
          label="Drop"
          zones={zones}
          zoneId={dropZoneId}
          onZoneChange={handleDropZoneChange}
          areas={dropAreas}
          areasLoading={dropAreasLoading}
          areaId={dropAreaId}
          onAreaChange={(id) => {
            setDropAreaId(id)
            setPreview(null)
          }}
        />

        <fieldset className="grid gap-3 sm:grid-cols-2">
          <legend className="mb-1 text-sm font-semibold text-slate-700">Order details</legend>
          <div>
            <label htmlFor="order_order_type" className="block text-xs font-medium text-slate-700">
              Order type
            </label>
            <Select
              id="order_order_type"
              value={orderType}
              onChange={(v) => {
                setOrderType(v as OrderType)
                setPreview(null)
              }}
              options={ORDER_TYPES.map((t) => ({ value: t, label: t }))}
              className="mt-1"
            />
          </div>
          <div>
            <label htmlFor="order_payment_type" className="block text-xs font-medium text-slate-700">
              Payment type
            </label>
            <Select
              id="order_payment_type"
              value={paymentType}
              onChange={(v) => {
                setPaymentType(v as PaymentType)
                setPreview(null)
              }}
              options={PAYMENT_TYPES.map((t) => ({ value: t, label: t }))}
              className="mt-1"
            />
          </div>
        </fieldset>

        <fieldset className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <legend className="mb-1 text-sm font-semibold text-slate-700">Package details</legend>
          <div>
            <label htmlFor="order_length" className="block text-xs font-medium text-slate-700">
              Length (cm)
            </label>
            <input
              id="order_length"
              type="number"
              min="0"
              step="0.01"
              value={lengthCm}
              onChange={(e) => {
                setLengthCm(e.target.value)
                setPreview(null)
              }}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="order_breadth" className="block text-xs font-medium text-slate-700">
              Breadth (cm)
            </label>
            <input
              id="order_breadth"
              type="number"
              min="0"
              step="0.01"
              value={breadthCm}
              onChange={(e) => {
                setBreadthCm(e.target.value)
                setPreview(null)
              }}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="order_height" className="block text-xs font-medium text-slate-700">
              Height (cm)
            </label>
            <input
              id="order_height"
              type="number"
              min="0"
              step="0.01"
              value={heightCm}
              onChange={(e) => {
                setHeightCm(e.target.value)
                setPreview(null)
              }}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label htmlFor="order_weight" className="block text-xs font-medium text-slate-700">
              Actual weight (kg)
            </label>
            <input
              id="order_weight"
              type="number"
              min="0"
              step="0.01"
              value={actualWeightKg}
              onChange={(e) => {
                setActualWeightKg(e.target.value)
                setPreview(null)
              }}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
        </fieldset>

        <button
          type="submit"
          disabled={previewing}
          className="rounded-md border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
        >
          {previewing ? 'Calculating…' : 'Preview quote'}
        </button>
      </form>

      {preview && (
        <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm" role="status">
          <h2 className="text-sm font-semibold text-slate-700">Quote preview</h2>
          <dl className="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-slate-500">Zone relationship</dt>
              <dd className="font-medium text-slate-900">{preview.zone_relationship}</dd>
            </div>
            <div>
              <dt className="text-slate-500">Chargeable weight</dt>
              <dd className="font-medium text-slate-900">{preview.chargeable_weight_kg} kg</dd>
            </div>
            <div>
              <dt className="text-slate-500">Final amount</dt>
              <dd className="text-base font-semibold text-slate-900">{formatCurrency(preview.final_amount)}</dd>
            </div>
          </dl>

          <ErrorBanner message={confirmError} />
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={confirming}
            className="mt-4 rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {confirming ? 'Placing order…' : 'Confirm & place order'}
          </button>
        </div>
      )}
    </Layout>
  )
}
