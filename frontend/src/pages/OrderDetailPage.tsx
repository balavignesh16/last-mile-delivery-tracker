import { Check, Copy, RotateCcw, Truck, UserCheck } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { STATUS_ICON } from '../components/order-status'
import { OrderStatusBadge } from '../components/OrderStatusBadge'
import { useAuth } from '../hooks/useAuth'
import { assignOrder, autoAssignOrder } from '../services/assignment'
import { ApiError } from '../services/api'
import { listAgents } from '../services/agents'
import { getOrder } from '../services/orders'
import { getReschedules, rescheduleOrder } from '../services/rescheduling'
import { getOrderTracking, transitionOrderStatus } from '../services/tracking'
import type { Agent } from '../types/agent'
import type { Order } from '../types/order'
import type { Reschedule } from '../types/reschedule'
import { LEGAL_TRANSITIONS, transitionsForRole, type TrackingEvent } from '../types/tracking'
import { formatCurrency } from '../utils/currency'

// ASSIGNED is only reachable from these two statuses in M08's state
// machine (CREATED->ASSIGNED, RESCHEDULED->ASSIGNED) — the assignment
// controls only render in those states so an ADMIN is never shown a
// button that would just 409.
const ASSIGNABLE_STATUSES: Order['status'][] = ['CREATED', 'RESCHEDULED']

// GET /orders/{id} returns 404 (never 403) for an order a CUSTOMER
// doesn't own — this page can't tell "doesn't exist" apart from "isn't
// yours" and doesn't try to; both render the same not-found message.
// GET /orders/{id}/tracking uses the identical ownership convention
// (M08), so the same ErrorBanner covers both loads.
export function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { token, user } = useAuth()
  const isAdmin = user?.role === 'ADMIN'
  const isCustomer = user?.role === 'CUSTOMER'
  // M12: a DELIVERY_AGENT may update the status of an order assigned to
  // them — the backend has always authorized the five agent-tier edges
  // (internal/tracking/statemachine.go); this page simply never exposed
  // a control for it before now. GET /orders/:id already 404s an agent
  // viewer out of any order that isn't assigned to them (M09), so by the
  // time `order` is loaded here for a DELIVERY_AGENT, ownership is
  // already backend-guaranteed — no extra ownership check is needed on
  // this page.
  const isDeliveryAgent = user?.role === 'DELIVERY_AGENT'
  // GET /orders/:id/tracking is ADMIN/CUSTOMER only — unchanged by M09,
  // deliberately not widened to DELIVERY_AGENT (see
  // docs/order-tracking.md) — so this page must not even attempt the
  // call for an agent viewer, let alone show its 403 as an error.
  // GET /orders/:id/reschedules (M10) uses the identical ADMIN/CUSTOMER
  // scope, so the same flag gates both.
  const canViewTracking = isAdmin || isCustomer

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

  const [reschedules, setReschedules] = useState<Reschedule[]>([])
  const [reschedulesLoading, setReschedulesLoading] = useState(canViewTracking)
  const [reschedulesError, setReschedulesError] = useState<string | null>(null)

  const [requestedDate, setRequestedDate] = useState('')
  const [reason, setReason] = useState('')
  const [rescheduleError, setRescheduleError] = useState<string | null>(null)
  const [rescheduleSuccess, setRescheduleSuccess] = useState<string | null>(null)
  const [rescheduling, setRescheduling] = useState(false)

  const [copied, setCopied] = useState(false)

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

  useEffect(() => {
    if (!token || !id || !canViewTracking) return
    let cancelled = false
    getReschedules(token, id)
      .then((list) => {
        if (!cancelled) setReschedules(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setReschedulesError(err instanceof ApiError ? err.message : 'Could not load the reschedule history.')
      })
      .finally(() => {
        if (!cancelled) setReschedulesLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, id, canViewTracking])

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

  async function handleReschedule(e: FormEvent) {
    e.preventDefault()
    if (!token || !id) return
    setRescheduleError(null)
    setRescheduleSuccess(null)
    setRescheduling(true)
    try {
      await rescheduleOrder(token, id, { requested_date: requestedDate, reason: reason.trim() === '' ? undefined : reason })
      const [updatedOrder, updatedReschedules] = await Promise.all([getOrder(token, id), getReschedules(token, id)])
      setOrder(updatedOrder)
      setReschedules(updatedReschedules)
      setRescheduleSuccess('Reschedule request submitted.')
      setRequestedDate('')
      setReason('')
    } catch (err) {
      setRescheduleError(err instanceof ApiError ? err.message : 'Could not reschedule this order.')
    } finally {
      setRescheduling(false)
    }
  }

  async function handleTransition(status: string) {
    if (!token || !id) return
    setTransitionError(null)
    setTransitioning(status)
    try {
      await transitionOrderStatus(token, id, { status: status as Order['status'] })
      const updatedOrder = await getOrder(token, id)
      setOrder(updatedOrder)
      // GET /orders/:id/tracking is ADMIN/CUSTOMER only (unchanged by
      // M12 — see canViewTracking's own doc comment above); a
      // DELIVERY_AGENT performing a transition must still only ever
      // refresh the order itself, never attempt a call this role was
      // never authorized for.
      if (canViewTracking) {
        setEvents(await getOrderTracking(token, id))
      }
    } catch (err) {
      setTransitionError(err instanceof ApiError ? err.message : 'Could not update the order status.')
    } finally {
      setTransitioning(null)
    }
  }

  function handleCopyId() {
    if (!order) return
    void navigator.clipboard.writeText(order.id)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <Layout>
      <Link to="/orders" className="text-sm text-slate-500 hover:text-slate-800">
        ← Back to orders
      </Link>

      <div className="mt-4 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : !order ? null : (
          <>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <h1 className="text-xl font-semibold">Order {order.id}</h1>
                <button
                  type="button"
                  onClick={handleCopyId}
                  aria-label="Copy order ID"
                  className="text-slate-400 transition-colors hover:text-navy-600"
                >
                  {copied ? <Check className="h-4 w-4 text-emerald-600" aria-hidden="true" /> : <Copy className="h-4 w-4" aria-hidden="true" />}
                </button>
              </div>
              <OrderStatusBadge status={order.status} />
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
                      className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
                    >
                      <UserCheck className="h-4 w-4" aria-hidden="true" />
                      {assigning ? 'Assigning…' : 'Assign'}
                    </button>
                    <button
                      type="button"
                      onClick={() => void handleAutoAssign()}
                      disabled={assigning}
                      className="flex items-center gap-1.5 rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                    >
                      <Truck className="h-4 w-4" aria-hidden="true" />
                      {assigning ? 'Assigning…' : 'Auto-assign'}
                    </button>
                  </div>
                )}
              </div>
            )}

            {(isAdmin || isCustomer) && (order.status === 'FAILED' || rescheduleSuccess || rescheduleError) && (
              <div className="mt-6 border-t border-slate-100 pt-6">
                <h2 className="text-sm font-semibold text-slate-700">Reschedule delivery</h2>
                <ErrorBanner message={rescheduleError} />
                {rescheduleSuccess && (
                  <div role="status" className="mt-2 rounded-md border border-emerald-300 bg-emerald-50 px-4 py-2 text-sm text-emerald-800">
                    {rescheduleSuccess}
                  </div>
                )}
                {order.status === 'FAILED' && (
                  <form onSubmit={(e) => void handleReschedule(e)} className="mt-3 flex flex-wrap items-end gap-2">
                    <div>
                      <label htmlFor="requested-date" className="block text-xs font-medium text-slate-500">
                        New delivery date
                      </label>
                      <input
                        id="requested-date"
                        type="date"
                        required
                        value={requestedDate}
                        onChange={(e) => setRequestedDate(e.target.value)}
                        disabled={rescheduling}
                        className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
                      />
                    </div>
                    <div>
                      <label htmlFor="reschedule-reason" className="block text-xs font-medium text-slate-500">
                        Reason (optional)
                      </label>
                      <input
                        id="reschedule-reason"
                        type="text"
                        value={reason}
                        onChange={(e) => setReason(e.target.value)}
                        disabled={rescheduling}
                        className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
                      />
                    </div>
                    <button
                      type="submit"
                      disabled={rescheduling || !requestedDate}
                      className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
                    >
                      <RotateCcw className="h-4 w-4" aria-hidden="true" />
                      {rescheduling ? 'Rescheduling…' : 'Reschedule'}
                    </button>
                  </form>
                )}
              </div>
            )}

            {(isAdmin || isDeliveryAgent) && user && (
              <div className="mt-6 border-t border-slate-100 pt-6">
                <h2 className="text-sm font-semibold text-slate-700">Update status</h2>
                <ErrorBanner message={transitionError} />
                {(() => {
                  const available = transitionsForRole(order.status, user.role)
                  if (available.length > 0) {
                    return (
                      <div className="mt-3 flex flex-wrap gap-2">
                        {available.map((next) => (
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
                    )
                  }
                  // Two distinct empty states: a genuinely terminal
                  // status (no role has any further edge, e.g.
                  // DELIVERED) vs. a status with legal next edges that
                  // simply aren't authorized for this viewer's role
                  // (e.g. a DELIVERY_AGENT viewing a FAILED order, where
                  // only ADMIN may move it to RESCHEDULED).
                  return (
                    <p className="mt-2 text-sm text-slate-500">
                      {LEGAL_TRANSITIONS[order.status].length === 0
                        ? 'This order is in a terminal state — no further transitions are possible.'
                        : 'No status update is available to you for this order right now.'}
                    </p>
                  )
                })()}
              </div>
            )}
          </>
        )}
      </div>

      {canViewTracking && (
      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Tracking timeline</h2>
        <ErrorBanner message={eventsError} />
        {eventsLoading ? (
          <p className="mt-3 text-sm text-slate-500">Loading…</p>
        ) : events.length === 0 ? (
          <p className="mt-3 text-sm text-slate-500">No tracking events yet.</p>
        ) : (
          <ol className="mt-4">
            {events.map((event, i) => {
              const Icon = STATUS_ICON[event.new_status]
              const isCurrent = i === events.length - 1
              return (
                <li key={event.id} className="flex gap-4 pb-6 text-sm last:pb-0">
                  <div className="flex flex-col items-center">
                    <span
                      className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-white ${
                        isCurrent ? 'bg-amber-500 ring-4 ring-amber-100' : 'bg-navy-600'
                      }`}
                    >
                      <Icon className="h-4 w-4" aria-hidden="true" />
                    </span>
                    {i < events.length - 1 && <span className="mt-1 w-0.5 flex-1 bg-navy-100" aria-hidden="true" />}
                  </div>
                  <div className="flex flex-1 items-start justify-between pt-1">
                    <div>
                      <p className="font-medium text-slate-900">
                        {event.previous_status ? `${event.previous_status} → ${event.new_status}` : event.new_status}
                      </p>
                      <p className="text-xs text-slate-500">Actor: {event.actor_id}</p>
                    </div>
                    <span className="whitespace-nowrap text-xs text-slate-400">{new Date(event.created_at).toLocaleString()}</span>
                  </div>
                </li>
              )
            })}
          </ol>
        )}
      </div>
      )}

      {canViewTracking && (
      <div className="mt-6 rounded-lg border border-navy-100 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Reschedule history</h2>
        <ErrorBanner message={reschedulesError} />
        {reschedulesLoading ? (
          <p className="mt-3 text-sm text-slate-500">Loading…</p>
        ) : reschedules.length === 0 ? (
          <p className="mt-3 text-sm text-slate-500">No reschedule requests yet.</p>
        ) : (
          <ol className="mt-4 space-y-3">
            {reschedules.map((re) => (
              <li key={re.id} className="flex items-start justify-between border-l-2 border-navy-100 pl-3 text-sm">
                <div>
                  <p className="font-medium text-slate-900">New date: {re.requested_date}</p>
                  {re.reason && <p className="text-xs text-slate-500">Reason: {re.reason}</p>}
                  <p className="text-xs text-slate-500">Requested by: {re.requested_by}</p>
                </div>
                <span className="whitespace-nowrap text-xs text-slate-400">{new Date(re.created_at).toLocaleString()}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
      )}
    </Layout>
  )
}
