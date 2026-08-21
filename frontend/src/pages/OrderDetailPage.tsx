import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../hooks/useAuth'
import { ApiError } from '../services/api'
import { getOrder } from '../services/orders'
import type { Order } from '../types/order'
import { formatCurrency } from '../utils/currency'

// GET /orders/{id} returns 404 (never 403) for an order a CUSTOMER
// doesn't own — this page can't tell "doesn't exist" apart from "isn't
// yours" and doesn't try to; both render the same not-found message.
export function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { token } = useAuth()

  const [order, setOrder] = useState<Order | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token || !id) return
    let cancelled = false
    getOrder(token, id)
      .then((o) => {
        if (!cancelled) setOrder(o)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load this order.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, id])

  return (
    <Layout>
      <Link to="/orders" className="text-sm text-slate-500 hover:text-slate-800">
        ← Back to orders
      </Link>

      <div className="mt-4 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : !order ? null : (
          <>
            <div className="flex items-center justify-between">
              <h1 className="text-xl font-semibold">Order {order.id}</h1>
              <StatusBadge label={order.status} state={order.status === 'DELIVERED' ? 'ok' : order.status === 'FAILED' ? 'error' : 'loading'} />
            </div>

            <dl className="mt-6 grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-3">
              <div>
                <dt className="text-slate-500">Order type</dt>
                <dd className="font-medium text-slate-900">{order.order_type}</dd>
              </div>
              <div>
                <dt className="text-slate-500">Payment type</dt>
                <dd className="font-medium text-slate-900">{order.payment_type}</dd>
              </div>
              <div>
                <dt className="text-slate-500">Zone relationship</dt>
                <dd className="font-medium text-slate-900">{order.zone_relationship}</dd>
              </div>
              <div>
                <dt className="text-slate-500">Dimensions</dt>
                <dd className="font-medium text-slate-900">
                  {order.length_cm} × {order.breadth_cm} × {order.height_cm} cm
                </dd>
              </div>
              <div>
                <dt className="text-slate-500">Actual weight</dt>
                <dd className="font-medium text-slate-900">{order.actual_weight_kg} kg</dd>
              </div>
              <div>
                <dt className="text-slate-500">Volumetric weight</dt>
                <dd className="font-medium text-slate-900">{order.volumetric_weight_kg} kg</dd>
              </div>
              <div>
                <dt className="text-slate-500">Chargeable weight</dt>
                <dd className="font-medium text-slate-900">{order.chargeable_weight_kg} kg</dd>
              </div>
              <div>
                <dt className="text-slate-500">Base rate</dt>
                <dd className="font-medium text-slate-900">{formatCurrency(order.base_rate)}</dd>
              </div>
              <div>
                <dt className="text-slate-500">COD surcharge</dt>
                <dd className="font-medium text-slate-900">{formatCurrency(order.cod_surcharge)}</dd>
              </div>
              <div>
                <dt className="text-slate-500">Final amount</dt>
                <dd className="text-base font-semibold text-slate-900">{formatCurrency(order.final_amount)}</dd>
              </div>
              <div>
                <dt className="text-slate-500">Placed</dt>
                <dd className="font-medium text-slate-900">{new Date(order.created_at).toLocaleString()}</dd>
              </div>
            </dl>
          </>
        )}
      </div>
    </Layout>
  )
}
