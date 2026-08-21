import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../hooks/useAuth'
import { assignOrder, autoAssignOrder } from '../services/assignment'
import { ApiError } from '../services/api'
import { listAgents } from '../services/agents'
import { getOrder } from '../services/orders'
import { getOrderTracking, transitionOrderStatus } from '../services/tracking'
import type { Agent } from '../types/agent'
import type { Order } from '../types/order'
import { LEGAL_TRANSITIONS, type TrackingEvent } from '../types/tracking'
import { formatCurrency } from '../utils/currency'

// ASSIGNED is only reachable from these two statuses in M08's state
// machine (CREATED->ASSIGNED, RESCHEDULED->ASSIGNED) — the assignment
// controls only render in those states so an ADMIN is never shown a
// button that would just 409.
const ASSIGNABLE_STATUSES: Order['status'][] = ['CREATED', 'RESCHEDULED']

function badgeState(status: string): 'ok' | 'error' | 'loading' {
  if (status === 'DELIVERED') return 'ok'
  if (status === 'FAILED') return 'error'
  return 'loading'
}

// GET /orders/{id} returns 404 (never 403) for an order a CUSTOMER
// doesn't own — this page can't tell "doesn't exist" apart from "isn't
// yours" and doesn't try to; both render the same not-found message.
// GET /orders/{id}/tracking uses the identical ownership convention
// (M08), so the same ErrorBanner covers both loads.
export function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'ADMIN'
  // GET /orders/:id/tracking is ADMIN/CUSTOMER only — unchanged by M09,
  // deliberately not widened to DELIVERY_AGENT (see
  // docs/order-tracking.md) — so this page must not even attempt the
  // call for an agent viewer, let alone show its 403 as an error.
  const canViewTracking = user?.role === 'ADMIN' || user?.role === 'CUSTOMER'

  const [order, setOrder] = useState<Order | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [events, setEvents] = useState<TrackingEvent[]>([])
  const [eventsLoading, setEventsLoading] = useState(canViewTracking)
  const [eventsError, setEventsError] = useState<string | null>(null)

  const [transitionError, setTransitionError] = useState<string | null>(null)
  const [transitioning, setTransitioning] = useState<string | null>(null)

  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [assignmentError, setAssignmentError] = useState<string | null>(null)
  const [assignmentSuccess, setAssignmentSuccess] = useState<string | null>(null)
  const [assigning, setAssigning] = useState(false)

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

  useEffect(() => {
    if (!token || !id || !canViewTracking) return
    let cancelled = false
    getOrderTracking(token, id)
      .then((list) => {
        if (!cancelled) setEvents(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setEventsError(err instanceof ApiError ? err.message : 'Could not load the tracking timeline.')
      })
      .finally(() => {
        if (!cancelled) setEventsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, id, canViewTracking])

  // Reuses M03's own listAgents infrastructure (the same call
  // AgentsPage makes) rather than a new candidate-preview endpoint —
  // the finalized M09 decision explicitly rules one out. ADMIN-only:
  // GET /agents is admin-only, so this never fires for other roles.
  useEffect(() => {
    if (!token || isAdmin !== true) return
    let cancelled = false
    listAgents(token)
      .then((list) => {
        if (!cancelled) setAgents(list)
      })
      .catch(() => {
        // Non-fatal: the assignment section still renders (auto-assign
        // needs no agent list), just without a manual picker.
      })
    return () => {
      cancelled = true
    }
  }, [token, isAdmin])

  async function refreshOrderAndTracking() {
    if (!token || !id) return
    const [updatedOrder, updatedEvents] = await Promise.all([getOrder(token, id), getOrderTracking(token, id)])
    setOrder(updatedOrder)
    setEvents(updatedEvents)
  }

  async function handleManualAssign() {
    if (!token || !id || !selectedAgentId) return
    setAssignmentError(null)
    setAssignmentSuccess(null)
    setAssigning(true)
    try {
      await assignOrder(token, id, { agent_id: selectedAgentId })
      await refreshOrderAndTracking()
      setAssignmentSuccess('Order assigned.')
      setSelectedAgentId('')
    } catch (err) {
      setAssignmentError(err instanceof ApiError ? err.message : 'Could not assign this order.')
    } finally {
      setAssigning(false)
    }
  }

  async function handleAutoAssign() {
    if (!token || !id) return
    setAssignmentError(null)
    setAssignmentSuccess(null)
    setAssigning(true)
    try {
      const updated = await autoAssignOrder(token, id)
      await refreshOrderAndTracking()
      setAssignmentSuccess(`Order auto-assigned to agent ${updated.assigned_agent_id}.`)
    } catch (err) {
      setAssignmentError(err instanceof ApiError ? err.message : 'Could not auto-assign this order.')
    } finally {
      setAssigning(false)
    }
  }

  async function handleTransition(status: string) {
    if (!token || !id) return
    setTransitionError(null)
    setTransitioning(status)
    try {
      await transitionOrderStatus(token, id, { status: status as Order['status'] })
      const [updatedOrder, updatedEvents] = await Promise.all([getOrder(token, id), getOrderTracking(token, id)])
      setOrder(updatedOrder)
      setEvents(updatedEvents)
    } catch (err) {
      setTransitionError(err instanceof ApiError ? err.message : 'Could not update the order status.')
    } finally {
      setTransitioning(null)
    }
  }

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
              <StatusBadge label={order.status} state={badgeState(order.status)} />
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
              <div>
                <dt className="text-slate-500">Assigned agent</dt>
                <dd className="font-medium text-slate-900">
                  {order.assigned_agent_id
                    ? (agents.find((a) => a.id === order.assigned_agent_id)?.full_name ?? order.assigned_agent_id)
                    : 'Unassigned'}
                </dd>
              </div>
            </dl>

            {isAdmin && (ASSIGNABLE_STATUSES.includes(order.status) || assignmentSuccess || assignmentError) && (
              <div className="mt-6 border-t border-slate-100 pt-6">
                <h2 className="text-sm font-semibold text-slate-700">Assign delivery agent</h2>
                <ErrorBanner message={assignmentError} />
                {assignmentSuccess && (
                  <div role="status" className="mt-2 rounded-md border border-emerald-300 bg-emerald-50 px-4 py-2 text-sm text-emerald-800">
                    {assignmentSuccess}
                  </div>
                )}
                {ASSIGNABLE_STATUSES.includes(order.status) && (
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <select
                      aria-label="Delivery agent"
                      value={selectedAgentId}
                      onChange={(e) => setSelectedAgentId(e.target.value)}
                      disabled={assigning}
                      className="rounded-md border border-slate-300 px-3 py-2 text-sm"
                    >
                      <option value="">Select an agent…</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.full_name} ({a.availability})
                        </option>
                      ))}
                    </select>
                    <button
                      type="button"
                      onClick={() => void handleManualAssign()}
                      disabled={assigning || !selectedAgentId}
                      className="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
                    >
                      {assigning ? 'Assigning…' : 'Assign'}
                    </button>
                    <button
                      type="button"
                      onClick={() => void handleAutoAssign()}
                      disabled={assigning}
                      className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                    >
                      {assigning ? 'Assigning…' : 'Auto-assign'}
                    </button>
                  </div>
                )}
              </div>
            )}

            {isAdmin && (
              <div className="mt-6 border-t border-slate-100 pt-6">
                <h2 className="text-sm font-semibold text-slate-700">Update status</h2>
                <ErrorBanner message={transitionError} />
                {LEGAL_TRANSITIONS[order.status].length === 0 ? (
                  <p className="mt-2 text-sm text-slate-500">This order is in a terminal state — no further transitions are possible.</p>
                ) : (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {LEGAL_TRANSITIONS[order.status].map((next) => (
                      <button
                        key={next}
                        type="button"
                        onClick={() => void handleTransition(next)}
                        disabled={transitioning !== null}
                        className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                      >
                        {transitioning === next ? 'Updating…' : `Mark as ${next}`}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </div>

      {canViewTracking && (
      <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Tracking timeline</h2>
        <ErrorBanner message={eventsError} />
        {eventsLoading ? (
          <p className="mt-3 text-sm text-slate-500">Loading…</p>
        ) : events.length === 0 ? (
          <p className="mt-3 text-sm text-slate-500">No tracking events yet.</p>
        ) : (
          <ol className="mt-4 space-y-3">
            {events.map((event) => (
              <li key={event.id} className="flex items-start justify-between border-l-2 border-slate-200 pl-3 text-sm">
                <div>
                  <p className="font-medium text-slate-900">
                    {event.previous_status ? `${event.previous_status} → ${event.new_status}` : event.new_status}
                  </p>
                  <p className="text-xs text-slate-500">Actor: {event.actor_id}</p>
                </div>
                <span className="whitespace-nowrap text-xs text-slate-400">{new Date(event.created_at).toLocaleString()}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
      )}
    </Layout>
  )
}
