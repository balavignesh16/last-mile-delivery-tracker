import { ArrowDown, ArrowUp, ArrowUpDown, PackageOpen, PackagePlus, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ErrorBanner } from '../components/ErrorBanner'
import { Layout } from '../components/Layout'
import { STATUS_LABEL } from '../components/order-status'
import { OrderStatusBadge } from '../components/OrderStatusBadge'
import { Select } from '../components/Select'
import { useAuth } from '../hooks/useAuth'
import { listAgents } from '../services/agents'
import { ApiError } from '../services/api'
import { listOrders } from '../services/orders'
import { listZones } from '../services/zones'
import type { Agent } from '../types/agent'
import { ORDER_STATUSES, type Order, type OrderFilter } from '../types/order'
import type { Zone } from '../types/zone'
import { formatCurrency } from '../utils/currency'

type SortKey = 'order' | 'amount' | 'status' | 'placed'

// Client-side only — sorts/searches whatever the server already
// returned for this caller (their own orders, or, for an ADMIN, the
// already status/zone/agent-filtered list). Never widens what a
// CUSTOMER/DELIVERY_AGENT can see; it only reorders and narrows it
// further within the browser.
function sortOrders(list: Order[], key: SortKey, dir: 'asc' | 'desc'): Order[] {
  const sorted = [...list].sort((a, b) => {
    let cmp = 0
    switch (key) {
      case 'order':
        cmp = a.id.localeCompare(b.id)
        break
      case 'amount':
        cmp = a.final_amount - b.final_amount
        break
      case 'status':
        cmp = ORDER_STATUSES.indexOf(a.status) - ORDER_STATUSES.indexOf(b.status)
        break
      case 'placed':
        cmp = new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
        break
    }
    return dir === 'asc' ? cmp : -cmp
  })
  return sorted
}

// A clickable column header with a sort-direction indicator — the
// button's accessible name is just the column label (e.g. "Order"),
// same as the plain <th> text it replaces, so no test/label churn.
function SortableHeader({
  sortKey,
  label,
  align,
  activeKey,
  dir,
  onSort,
}: {
  sortKey: SortKey
  label: string
  align?: 'right'
  activeKey: SortKey | null
  dir: 'asc' | 'desc'
  onSort: (key: SortKey) => void
}) {
  const isActive = activeKey === sortKey
  const Icon = isActive ? (dir === 'asc' ? ArrowUp : ArrowDown) : ArrowUpDown
  return (
    <th className={`px-6 py-3 ${align === 'right' ? 'text-right' : 'text-left'}`}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`inline-flex items-center gap-1 transition-colors hover:text-navy-900 ${align === 'right' ? 'flex-row-reverse' : ''} ${
          isActive ? 'text-navy-900' : ''
        }`}
      >
        {label}
        <Icon className={`h-3.5 w-3.5 ${isActive ? 'text-navy-600' : 'text-slate-400'}`} aria-hidden="true" />
      </button>
    </th>
  )
}

// ADMIN sees every order (optionally narrowed by the status/zone/agent
// filters below, M12), CUSTOMER sees only their own, and (since M09)
// DELIVERY_AGENT sees only orders currently assigned to them — the
// backend decides which by the caller's role (GET /orders); a CUSTOMER/
// DELIVERY_AGENT can never widen their own view by supplying a filter
// (see internal/orders/handler.go's own doc comment), so this page only
// ever renders the filter controls for an ADMIN viewer.
export function OrdersPage() {
  const { token, user } = useAuth()
  const navigate = useNavigate()
  const isAdmin = user?.role === 'ADMIN'
  const isDeliveryAgent = user?.role === 'DELIVERY_AGENT'

  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [statusFilter, setStatusFilter] = useState('')
  const [zoneFilter, setZoneFilter] = useState('')
  const [agentFilter, setAgentFilter] = useState('')

  const [zones, setZones] = useState<Zone[]>([])
  const [agents, setAgents] = useState<Agent[]>([])

  const [searchQuery, setSearchQuery] = useState('')
  const [sortKey, setSortKey] = useState<SortKey | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  useEffect(() => {
    if (!token || !isAdmin) return
    let cancelled = false
    Promise.all([listZones(token), listAgents(token)])
      .then(([zoneList, agentList]) => {
        if (!cancelled) {
          setZones(zoneList)
          setAgents(agentList)
        }
      })
      .catch(() => {
        // Non-fatal: the order list itself still loads and works; only
        // the filter dropdowns' option lists would stay empty.
      })
    return () => {
      cancelled = true
    }
  }, [token, isAdmin])

  useEffect(() => {
    if (!token) return
    let cancelled = false
    const filter: OrderFilter | undefined = isAdmin
      ? {
          status: statusFilter ? (statusFilter as OrderFilter['status']) : undefined,
          zone: zoneFilter || undefined,
          agent: agentFilter || undefined,
        }
      : undefined
    listOrders(token, filter)
      .then((list) => {
        if (!cancelled) setOrders(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Could not load orders.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, isAdmin, statusFilter, zoneFilter, agentFilter])

  const hasActiveFilter = statusFilter !== '' || zoneFilter !== '' || agentFilter !== ''
  const hasActiveSearch = searchQuery.trim() !== ''

  // Search is client-side over whatever the server already returned for
  // this caller — order id, type, route, payment type, and status label.
  // Not customer name/email: no admin endpoint returns that alongside an
  // order, so searching by it would silently do nothing rather than
  // actually work — scoped honestly to what's really there.
  const displayedOrders = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    let list = orders
    if (q) {
      list = list.filter(
        (o) =>
          o.id.toLowerCase().includes(q) ||
          o.order_type.toLowerCase().includes(q) ||
          o.zone_relationship.toLowerCase().includes(q) ||
          o.payment_type.toLowerCase().includes(q) ||
          STATUS_LABEL[o.status].toLowerCase().includes(q),
      )
    }
    return sortKey ? sortOrders(list, sortKey, sortDir) : list
  }, [orders, searchQuery, sortKey, sortDir])

  function handleSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir(key === 'status' ? 'asc' : 'desc')
    }
  }

  return (
    <Layout>
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{isAdmin ? 'All orders' : isDeliveryAgent ? 'My assigned orders' : 'My orders'}</h1>
        {!isDeliveryAgent && (
          <Link
            to="/orders/new"
            className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700"
          >
            <PackagePlus className="h-4 w-4" aria-hidden="true" />
            New order
          </Link>
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-end gap-3 rounded-lg border border-navy-100 bg-white p-4 shadow-sm">
        <div className="min-w-56 flex-1">
          <label htmlFor="order-search" className="block text-xs font-medium text-slate-500">
            Search
          </label>
          <div className="relative mt-1">
            <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
            <input
              id="order-search"
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Order ID, type, route, or status…"
              className="w-full rounded-md border border-slate-300 py-2 pr-3 pl-9 text-sm transition-colors focus:border-navy-500"
            />
          </div>
        </div>
        {isAdmin && (
          <>
            <div>
              <label htmlFor="status-filter" className="block text-xs font-medium text-slate-500">
                Status
              </label>
              <Select
                id="status-filter"
                value={statusFilter}
                onChange={setStatusFilter}
                placeholder="All statuses"
                className="mt-1 w-40"
                options={ORDER_STATUSES.map((s) => ({ value: s, label: s }))}
              />
            </div>
            <div>
              <label htmlFor="zone-filter" className="block text-xs font-medium text-slate-500">
                Zone
              </label>
              <Select
                id="zone-filter"
                value={zoneFilter}
                onChange={setZoneFilter}
                placeholder="All zones"
                className="mt-1 w-40"
                options={zones.map((z) => ({ value: z.id, label: z.name }))}
              />
            </div>
            <div>
              <label htmlFor="agent-filter" className="block text-xs font-medium text-slate-500">
                Agent
              </label>
              <Select
                id="agent-filter"
                value={agentFilter}
                onChange={setAgentFilter}
                placeholder="All agents"
                className="mt-1 w-40"
                options={agents.map((a) => ({ value: a.id, label: a.full_name }))}
              />
            </div>
          </>
        )}
        {(hasActiveFilter || hasActiveSearch) && (
          <button
            type="button"
            onClick={() => {
              setStatusFilter('')
              setZoneFilter('')
              setAgentFilter('')
              setSearchQuery('')
            }}
            className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
          >
            Clear filters
          </button>
        )}
      </div>

      <div className="mt-6 overflow-hidden rounded-lg border border-navy-100 bg-white shadow-sm">
        <ErrorBanner message={error} />
        {loading ? (
          <div className="space-y-3 p-6">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
            ))}
          </div>
        ) : orders.length === 0 ? (
          <div className="px-6 py-14 text-center">
            <PackageOpen className="mx-auto h-10 w-10 text-slate-300" aria-hidden="true" />
            <p className="mt-3 text-sm font-medium text-slate-700">{hasActiveFilter ? 'No orders match these filters.' : 'No orders yet.'}</p>
            <p className="mt-1 text-sm text-slate-500">
              {hasActiveFilter ? 'Try clearing a filter to see more results.' : 'Your orders will appear here once you create a delivery.'}
            </p>
          </div>
        ) : displayedOrders.length === 0 ? (
          <div className="px-6 py-14 text-center">
            <PackageOpen className="mx-auto h-10 w-10 text-slate-300" aria-hidden="true" />
            <p className="mt-3 text-sm font-medium text-slate-700">No orders match your search.</p>
            <p className="mt-1 text-sm text-slate-500">Try a different order ID, type, route, or status.</p>
          </div>
        ) : (
          <div className="max-h-[32rem] overflow-auto">
            <table className="w-full text-left text-sm">
              <thead className="sticky top-0 z-[1] border-b border-navy-100 bg-navy-50/95 text-xs font-medium tracking-wide text-navy-700 uppercase backdrop-blur-sm">
                <tr>
                  <SortableHeader sortKey="order" label="Order" activeKey={sortKey} dir={sortDir} onSort={handleSort} />
                  <th className="px-6 py-3">Route</th>
                  <th className="px-6 py-3">Payment</th>
                  <SortableHeader sortKey="amount" label="Amount" align="right" activeKey={sortKey} dir={sortDir} onSort={handleSort} />
                  <SortableHeader sortKey="status" label="Status" activeKey={sortKey} dir={sortDir} onSort={handleSort} />
                  <SortableHeader sortKey="placed" label="Placed" activeKey={sortKey} dir={sortDir} onSort={handleSort} />
                </tr>
              </thead>
              <tbody>
                {displayedOrders.map((o) => (
                  <tr
                    key={o.id}
                    onClick={() => navigate(`/orders/${o.id}`)}
                    className="cursor-pointer border-t border-slate-100 transition-colors hover:bg-navy-50/50"
                  >
                    <td className="px-6 py-3">
                      <Link to={`/orders/${o.id}`} className="font-medium text-navy-700 hover:underline">
                        {o.order_type} · {o.id.slice(0, 8)}
                      </Link>
                    </td>
                    <td className="px-6 py-3 text-slate-600">{o.zone_relationship}</td>
                    <td className="px-6 py-3 text-slate-600">{o.payment_type}</td>
                    <td className="px-6 py-3 text-right font-medium tabular-nums text-slate-900">{formatCurrency(o.final_amount)}</td>
                    <td className="px-6 py-3">
                      <OrderStatusBadge status={o.status} />
                    </td>
                    <td className="px-6 py-3 whitespace-nowrap text-slate-500">{new Date(o.created_at).toLocaleDateString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Layout>
  )
}
