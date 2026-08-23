import { ChevronRight, Plus, Search, Wallet } from 'lucide-react'
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
import { createRateCard, createSlab, deleteSlab, listRateCards, listSlabs, updateRateCard, updateSlab } from '../../services/rates'
import type { OrderType, RateCard, Slab, ZoneRelationship } from '../../types/rate'

const ORDER_TYPES: OrderType[] = ['B2B', 'B2C']
const ZONE_RELATIONSHIPS: ZoneRelationship[] = ['INTRA', 'INTER']

function formatWeightRange(slab: Slab): string {
  const max = slab.max_weight === null ? '∞' : `${slab.max_weight} kg`
  return `${slab.min_weight} kg – ${max}`
}

function cardLabel(card: RateCard): string {
  return `${card.order_type} · ${card.zone_relationship}`
}

// Rows shown per page — beyond this, rate cards paginate out of view
// instead of piling into one long, ever-growing list.
const PAGE_SIZE = 20

// Admin rate-card management (Round 6): a full-width pricing workspace —
// search/filter toolbar, dense table, and a full-width pricing-
// configuration workspace that replaces the table (not a permanently-
// reserved empty side panel) once a card is selected. Creation moved
// from a page-dominating form into a modal. Scope is otherwise
// unchanged from M05: create/manage (order_type, zone_relationship)
// rate cards and their weight slabs — no calculator, no quote preview
// (that's M06's QuotePage's job, not this page's).
export function RatesPage() {
  const { token } = useAuth()

  const [cards, setCards] = useState<RateCard[]>([])
  const [cardsLoading, setCardsLoading] = useState(true)
  const [cardsError, setCardsError] = useState<string | null>(null)

  // Real slab counts, derived from the same GET /rates/:id/slabs
  // endpoint the detail view already calls — fetched once per card
  // after the card list loads, not fabricated.
  const [slabCounts, setSlabCounts] = useState<Record<string, number>>({})

  const [cardSearch, setCardSearch] = useState('')
  const [orderTypeFilter, setOrderTypeFilter] = useState('')
  const [zoneRelFilter, setZoneRelFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)

  const [createOpen, setCreateOpen] = useState(false)
  const [newOrderType, setNewOrderType] = useState<OrderType>('B2B')
  const [newZoneRelationship, setNewZoneRelationship] = useState<ZoneRelationship>('INTRA')
  const [newCodSurcharge, setNewCodSurcharge] = useState('0')
  const [createError, setCreateError] = useState<string | null>(null)
  const [creatingCard, setCreatingCard] = useState(false)

  const [selectedCardId, setSelectedCardId] = useState<string | null>(null)
  const [editCodSurcharge, setEditCodSurcharge] = useState('0')
  const [cardEditError, setCardEditError] = useState<string | null>(null)
  const [savingCard, setSavingCard] = useState(false)

  const [slabs, setSlabs] = useState<Slab[]>([])
  const [slabsLoading, setSlabsLoading] = useState(false)
  const [slabsError, setSlabsError] = useState<string | null>(null)

  const [newMinWeight, setNewMinWeight] = useState('')
  const [newMaxWeight, setNewMaxWeight] = useState('')
  const [newPrice, setNewPrice] = useState('')
  const [slabCreateError, setSlabCreateError] = useState<string | null>(null)
  const [creatingSlab, setCreatingSlab] = useState(false)

  const [editingSlabId, setEditingSlabId] = useState<string | null>(null)
  const [editMinWeight, setEditMinWeight] = useState('')
  const [editMaxWeight, setEditMaxWeight] = useState('')
  const [editPrice, setEditPrice] = useState('')
  const [slabEditError, setSlabEditError] = useState<string | null>(null)

  const [slabDeleteError, setSlabDeleteError] = useState<string | null>(null)

  const selectedCard = cards.find((c) => c.id === selectedCardId) ?? null

  const displayedCards = useMemo(() => {
    const q = cardSearch.trim().toLowerCase()
    return cards.filter((c) => {
      if (q && !cardLabel(c).toLowerCase().includes(q) && !String(c.cod_surcharge).includes(q)) return false
      if (orderTypeFilter && c.order_type !== orderTypeFilter) return false
      if (zoneRelFilter && c.zone_relationship !== zoneRelFilter) return false
      if (statusFilter === 'active' && !c.active) return false
      if (statusFilter === 'inactive' && c.active) return false
      return true
    })
  }, [cards, cardSearch, orderTypeFilter, zoneRelFilter, statusFilter])

  const hasActiveFilter = cardSearch.trim() !== '' || orderTypeFilter !== '' || zoneRelFilter !== '' || statusFilter !== ''

  // A new search/filter can change how many pages exist — always land
  // back on page 1 rather than risk stranding the viewer on a now-empty
  // later page. Adjusted during render (React's documented pattern for
  // resetting state when an input changes) rather than in an effect.
  const filterKey = `${cardSearch}|${orderTypeFilter}|${zoneRelFilter}|${statusFilter}`
  const [prevFilterKey, setPrevFilterKey] = useState(filterKey)
  if (filterKey !== prevFilterKey) {
    setPrevFilterKey(filterKey)
    setPage(1)
  }

  const pagedCards = useMemo(() => displayedCards.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE), [displayedCards, page])

  useEffect(() => {
    if (!token) return
    let cancelled = false
    listRateCards(token)
      .then((list) => {
        if (!cancelled) setCards(list)
      })
      .catch((err: unknown) => {
        if (!cancelled) setCardsError(err instanceof ApiError ? err.message : 'Could not load rate cards.')
      })
      .finally(() => {
        if (!cancelled) setCardsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  useEffect(() => {
    if (!token || cards.length === 0) return
    let cancelled = false
    Promise.allSettled(cards.map((c) => listSlabs(token, c.id).then((list) => [c.id, list.length] as const))).then((results) => {
      if (cancelled) return
      const counts: Record<string, number> = {}
      for (const result of results) {
        if (result.status === 'fulfilled') counts[result.value[0]] = result.value[1]
      }
      setSlabCounts((prev) => ({ ...prev, ...counts }))
    })
    return () => {
      cancelled = true
    }
  }, [token, cards])

  useEffect(() => {
    if (!token || !selectedCardId) return
    let cancelled = false

    async function loadSlabs(currentToken: string, rateCardId: string) {
      setSlabsLoading(true)
      setSlabsError(null)
      try {
        const list = await listSlabs(currentToken, rateCardId)
        if (!cancelled) setSlabs(list)
      } catch (err) {
        if (!cancelled) setSlabsError(err instanceof ApiError ? err.message : 'Could not load slabs.')
      } finally {
        if (!cancelled) setSlabsLoading(false)
      }
    }

    void loadSlabs(token, selectedCardId)
    return () => {
      cancelled = true
    }
  }, [token, selectedCardId])

  function handleSelectCard(card: RateCard) {
    setSelectedCardId(card.id)
    setEditCodSurcharge(String(card.cod_surcharge))
    setCardEditError(null)
    setSlabs([])
    setNewMinWeight('')
    setNewMaxWeight('')
    setNewPrice('')
    setSlabCreateError(null)
    setEditingSlabId(null)
    setSlabDeleteError(null)
  }

  async function handleCreateCard(e: FormEvent) {
    e.preventDefault()
    setCreateError(null)
    if (!token) return
    const surcharge = Number(newCodSurcharge)
    if (Number.isNaN(surcharge) || surcharge < 0) {
      setCreateError('COD surcharge must be a non-negative number.')
      return
    }
    setCreatingCard(true)
    try {
      const created = await createRateCard(token, {
        order_type: newOrderType,
        zone_relationship: newZoneRelationship,
        cod_surcharge: surcharge,
      })
      setCards((prev) => [...prev, created])
      setSlabCounts((prev) => ({ ...prev, [created.id]: 0 }))
      setNewCodSurcharge('0')
      setCreateOpen(false)
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Could not create rate card.')
    } finally {
      setCreatingCard(false)
    }
  }

  async function handleSaveCard(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedCard) return
    setCardEditError(null)
    const surcharge = Number(editCodSurcharge)
    if (Number.isNaN(surcharge) || surcharge < 0) {
      setCardEditError('COD surcharge must be a non-negative number.')
      return
    }
    setSavingCard(true)
    try {
      const updated = await updateRateCard(token, selectedCard.id, { cod_surcharge: surcharge })
      setCards((prev) => prev.map((c) => (c.id === updated.id ? updated : c)))
    } catch (err) {
      setCardEditError(err instanceof ApiError ? err.message : 'Could not update rate card.')
    } finally {
      setSavingCard(false)
    }
  }

  // Toggles active using the card's currently saved cod_surcharge, not
  // whatever is sitting in the edit field — activating/deactivating
  // must never have an unintended side effect of also changing the
  // surcharge.
  async function handleToggleActive() {
    if (!token || !selectedCard) return
    setCardEditError(null)
    try {
      const updated = await updateRateCard(token, selectedCard.id, {
        cod_surcharge: selectedCard.cod_surcharge,
        active: !selectedCard.active,
      })
      setCards((prev) => prev.map((c) => (c.id === updated.id ? updated : c)))
    } catch (err) {
      setCardEditError(err instanceof ApiError ? err.message : 'Could not update rate card.')
    }
  }

  function parseSlabFields(minRaw: string, maxRaw: string, priceRaw: string): { min: number; max?: number; price: number } | string {
    const min = Number(minRaw)
    const price = Number(priceRaw)
    if (minRaw.trim() === '' || Number.isNaN(min) || min < 0) return 'Min weight must be a non-negative number.'
    if (priceRaw.trim() === '' || Number.isNaN(price) || price < 0) return 'Price must be a non-negative number.'
    if (maxRaw.trim() === '') return { min, price }
    const max = Number(maxRaw)
    if (Number.isNaN(max) || max <= min) return 'Max weight must be greater than min weight, or left blank for open-ended.'
    return { min, max, price }
  }

  async function handleCreateSlab(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedCard) return
    setSlabCreateError(null)
    const parsed = parseSlabFields(newMinWeight, newMaxWeight, newPrice)
    if (typeof parsed === 'string') {
      setSlabCreateError(parsed)
      return
    }
    setCreatingSlab(true)
    try {
      const created = await createSlab(token, selectedCard.id, { min_weight: parsed.min, max_weight: parsed.max, price: parsed.price })
      setSlabs((prev) => [...prev, created].sort((a, b) => a.min_weight - b.min_weight))
      setSlabCounts((prev) => ({ ...prev, [selectedCard.id]: (prev[selectedCard.id] ?? 0) + 1 }))
      setNewMinWeight('')
      setNewMaxWeight('')
      setNewPrice('')
    } catch (err) {
      setSlabCreateError(err instanceof ApiError ? err.message : 'Could not create slab.')
    } finally {
      setCreatingSlab(false)
    }
  }

  function startEditSlab(slab: Slab) {
    setEditingSlabId(slab.id)
    setEditMinWeight(String(slab.min_weight))
    setEditMaxWeight(slab.max_weight === null ? '' : String(slab.max_weight))
    setEditPrice(String(slab.price))
    setSlabEditError(null)
  }

  async function handleSaveSlab(e: FormEvent) {
    e.preventDefault()
    if (!token || !selectedCard || !editingSlabId) return
    setSlabEditError(null)
    const parsed = parseSlabFields(editMinWeight, editMaxWeight, editPrice)
    if (typeof parsed === 'string') {
      setSlabEditError(parsed)
      return
    }
    try {
      const updated = await updateSlab(token, selectedCard.id, editingSlabId, { min_weight: parsed.min, max_weight: parsed.max, price: parsed.price })
      setSlabs((prev) => prev.map((s) => (s.id === updated.id ? updated : s)).sort((a, b) => a.min_weight - b.min_weight))
      setEditingSlabId(null)
    } catch (err) {
      setSlabEditError(err instanceof ApiError ? err.message : 'Could not update slab.')
    }
  }

  async function handleDeleteSlab(slab: Slab) {
    if (!token || !selectedCard) return
    setSlabDeleteError(null)
    try {
      await deleteSlab(token, selectedCard.id, slab.id)
      setSlabs((prev) => prev.filter((s) => s.id !== slab.id))
      setSlabCounts((prev) => ({ ...prev, [selectedCard.id]: Math.max(0, (prev[selectedCard.id] ?? 1) - 1) }))
    } catch (err) {
      setSlabDeleteError(err instanceof ApiError ? err.message : 'Could not delete slab.')
    }
  }

  const createButton = (
    <button
      type="button"
      onClick={() => setCreateOpen(true)}
      className="flex items-center gap-1.5 rounded-md bg-navy-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-navy-700"
    >
      <Plus className="h-4 w-4" aria-hidden="true" />
      Create rate card
    </button>
  )

  return (
    <Layout>
      <PageHeader icon={Wallet} title="Rate Cards" description="Manage delivery pricing rules and weight slabs." action={createButton} />

      {selectedCard ? (
        <div className="mt-6">
          <button type="button" onClick={() => setSelectedCardId(null)} className="text-sm text-slate-500 hover:text-slate-800">
            ← Back to rate cards
          </button>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <h2 className="text-lg font-semibold text-slate-900">{cardLabel(selectedCard)}</h2>
            <div className="flex items-center gap-2">
              <StatusBadge label={selectedCard.active ? 'Active' : 'Inactive'} state={selectedCard.active ? 'ok' : 'error'} />
              <button
                type="button"
                onClick={() => void handleToggleActive()}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                {selectedCard.active ? 'Deactivate' : 'Activate'}
              </button>
            </div>
          </div>

          <div className="mt-6 rounded-lg border border-navy-100 bg-white shadow-sm">
            <h3 className="border-b border-navy-100 px-6 py-4 text-sm font-semibold text-slate-700">Pricing configuration</h3>
            <div className="px-6 py-4">
              <form onSubmit={handleSaveCard} className="flex flex-wrap items-end gap-3">
                <ErrorBanner message={cardEditError} />
                <div>
                  <label htmlFor="rate_edit_cod_surcharge" className="block text-xs font-medium text-slate-700">
                    COD surcharge
                  </label>
                  <input
                    id="rate_edit_cod_surcharge"
                    type="number"
                    min="0"
                    step="0.01"
                    value={editCodSurcharge}
                    onChange={(e) => setEditCodSurcharge(e.target.value)}
                    className="mt-1 w-32 rounded-md border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <button
                  type="submit"
                  disabled={savingCard}
                  className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
                >
                  {savingCard ? 'Saving…' : 'Save'}
                </button>
              </form>

              <h4 className="mt-6 text-sm font-semibold text-slate-700">Weight slabs{slabs.length > 0 ? ` (${slabs.length})` : ''}</h4>
              <ErrorBanner message={slabsError} />
              <ErrorBanner message={slabDeleteError} />
              {slabsLoading ? (
                <p className="mt-3 text-sm text-slate-500">Loading…</p>
              ) : slabs.length === 0 ? (
                <p className="mt-3 text-sm text-slate-500">No slabs configured yet.</p>
              ) : (
                <table className="mt-3 w-full text-left text-sm">
                  <thead className="text-xs font-medium tracking-wide text-slate-400 uppercase">
                    <tr>
                      <th className="py-2">Weight range</th>
                      <th className="py-2">Price</th>
                      <th className="py-2 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {slabs.map((slab) =>
                      editingSlabId === slab.id ? (
                        <tr key={slab.id} className="border-t border-slate-100">
                          <td colSpan={3} className="py-2">
                            <form onSubmit={handleSaveSlab} className="flex flex-wrap items-center gap-2">
                              <ErrorBanner message={slabEditError} />
                              <input
                                aria-label="Edit min weight"
                                type="number"
                                value={editMinWeight}
                                onChange={(e) => setEditMinWeight(e.target.value)}
                                className="w-20 rounded-md border border-slate-300 px-2 py-1 text-sm"
                              />
                              <span className="text-slate-400">–</span>
                              <input
                                aria-label="Edit max weight"
                                type="number"
                                placeholder="open-ended"
                                value={editMaxWeight}
                                onChange={(e) => setEditMaxWeight(e.target.value)}
                                className="w-28 rounded-md border border-slate-300 px-2 py-1 text-sm"
                              />
                              <input
                                aria-label="Edit price"
                                type="number"
                                value={editPrice}
                                onChange={(e) => setEditPrice(e.target.value)}
                                className="w-20 rounded-md border border-slate-300 px-2 py-1 text-sm"
                              />
                              <button
                                type="submit"
                                className="rounded-md border border-slate-300 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-100"
                              >
                                Save
                              </button>
                              <button
                                type="button"
                                onClick={() => setEditingSlabId(null)}
                                className="rounded-md px-2 py-1 text-xs text-slate-500 hover:text-slate-700"
                              >
                                Cancel
                              </button>
                            </form>
                          </td>
                        </tr>
                      ) : (
                        <tr key={slab.id} className="border-t border-slate-100">
                          <td className="py-2.5 font-medium text-slate-900">{formatWeightRange(slab)}</td>
                          <td className="py-2.5 tabular-nums text-slate-700">₹{slab.price}</td>
                          <td className="py-2.5 text-right">
                            <span className="inline-flex gap-3">
                              <button
                                type="button"
                                onClick={() => startEditSlab(slab)}
                                className="text-xs font-medium text-slate-500 hover:text-slate-800"
                              >
                                Edit
                              </button>
                              <button
                                type="button"
                                onClick={() => void handleDeleteSlab(slab)}
                                className="text-xs font-medium text-red-500 hover:text-red-700"
                              >
                                Delete
                              </button>
                            </span>
                          </td>
                        </tr>
                      ),
                    )}
                  </tbody>
                </table>
              )}

              <form onSubmit={handleCreateSlab} className="mt-4 flex flex-wrap items-end gap-3 border-t border-slate-100 pt-4">
                <ErrorBanner message={slabCreateError} />
                <div>
                  <label htmlFor="slab_min_weight" className="block text-xs font-medium text-slate-700">
                    Min weight (kg)
                  </label>
                  <input
                    id="slab_min_weight"
                    type="number"
                    value={newMinWeight}
                    onChange={(e) => setNewMinWeight(e.target.value)}
                    className="mt-1 w-24 rounded-md border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label htmlFor="slab_max_weight" className="block text-xs font-medium text-slate-700">
                    Max weight (kg)
                  </label>
                  <input
                    id="slab_max_weight"
                    type="number"
                    placeholder="open-ended"
                    value={newMaxWeight}
                    onChange={(e) => setNewMaxWeight(e.target.value)}
                    className="mt-1 w-28 rounded-md border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label htmlFor="slab_price" className="block text-xs font-medium text-slate-700">
                    Price
                  </label>
                  <input
                    id="slab_price"
                    type="number"
                    value={newPrice}
                    onChange={(e) => setNewPrice(e.target.value)}
                    className="mt-1 w-24 rounded-md border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <button
                  type="submit"
                  disabled={creatingSlab}
                  className="rounded-md bg-navy-600 px-3 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
                >
                  {creatingSlab ? 'Adding…' : 'Add slab'}
                </button>
              </form>
            </div>
          </div>
        </div>
      ) : (
        <>
          <div className="mt-6 flex flex-wrap items-end gap-3 rounded-lg border border-navy-100 bg-white p-4 shadow-sm">
            <div className="min-w-56 flex-1">
              <label htmlFor="rate-search" className="block text-xs font-medium text-slate-500">
                Search
              </label>
              <div className="relative mt-1">
                <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
                <input
                  id="rate-search"
                  type="text"
                  aria-label="Search rate cards"
                  value={cardSearch}
                  onChange={(e) => setCardSearch(e.target.value)}
                  placeholder="Search by type, zone relationship, or surcharge…"
                  className="w-full rounded-md border border-slate-300 py-2 pr-3 pl-9 text-sm transition-colors focus:border-navy-500"
                />
              </div>
            </div>
            <div>
              <label htmlFor="rate-order-type-filter" className="block text-xs font-medium text-slate-500">
                Order type
              </label>
              <Select
                id="rate-order-type-filter"
                value={orderTypeFilter}
                onChange={setOrderTypeFilter}
                placeholder="All types"
                className="mt-1 w-32"
                options={ORDER_TYPES.map((t) => ({ value: t, label: t }))}
              />
            </div>
            <div>
              <label htmlFor="rate-zone-rel-filter" className="block text-xs font-medium text-slate-500">
                Zone relationship
              </label>
              <Select
                id="rate-zone-rel-filter"
                value={zoneRelFilter}
                onChange={setZoneRelFilter}
                placeholder="All"
                className="mt-1 w-36"
                options={ZONE_RELATIONSHIPS.map((r) => ({ value: r, label: r }))}
              />
            </div>
            <div>
              <label htmlFor="rate-status-filter" className="block text-xs font-medium text-slate-500">
                Status
              </label>
              <Select
                id="rate-status-filter"
                value={statusFilter}
                onChange={setStatusFilter}
                placeholder="All statuses"
                className="mt-1 w-36"
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
                  setCardSearch('')
                  setOrderTypeFilter('')
                  setZoneRelFilter('')
                  setStatusFilter('')
                }}
                className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                Clear filters
              </button>
            )}
          </div>

          <div className="mt-6 overflow-hidden rounded-lg border border-navy-100 bg-white shadow-sm">
            <ErrorBanner message={cardsError} />
            {cardsLoading ? (
              <div className="space-y-3 p-6">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-14 animate-pulse rounded-md bg-slate-100" />
                ))}
              </div>
            ) : cards.length === 0 ? (
              <EmptyState icon={Wallet} title="No rate cards yet" description="Create your first rate card to start pricing deliveries." />
            ) : displayedCards.length === 0 ? (
              <EmptyState icon={Search} title="No rate cards match your search." description="Try a different type, relationship, or filter." />
            ) : (
              <>
                <div className="hidden sm:block">
                  <table className="w-full text-left text-sm">
                    <thead className="border-b border-navy-100 bg-navy-50/95 text-xs font-medium tracking-wide text-navy-700 uppercase">
                      <tr>
                        <th className="px-6 py-3">Type</th>
                        <th className="px-6 py-3">Zone relationship</th>
                        <th className="px-6 py-3 text-right">COD surcharge</th>
                        <th className="px-6 py-3 text-right">Slabs</th>
                        <th className="px-6 py-3">Status</th>
                        <th className="px-6 py-3" aria-hidden="true" />
                      </tr>
                    </thead>
                    <tbody>
                      {pagedCards.map((card) => (
                        <tr
                          key={card.id}
                          onClick={() => handleSelectCard(card)}
                          className="group cursor-pointer border-t border-slate-100 transition-colors hover:bg-navy-50/50"
                        >
                          <td className="px-6 py-3">
                            <button type="button" onClick={() => handleSelectCard(card)} className="font-medium text-navy-700 hover:underline">
                              {card.order_type}
                            </button>
                          </td>
                          <td className="px-6 py-3 text-slate-600">{card.zone_relationship}</td>
                          <td className="px-6 py-3 text-right tabular-nums text-slate-600">₹{card.cod_surcharge}</td>
                          <td className="px-6 py-3 text-right tabular-nums text-slate-600">{slabCounts[card.id] ?? '—'}</td>
                          <td className="px-6 py-3">
                            <StatusBadge label={card.active ? 'Active' : 'Inactive'} state={card.active ? 'ok' : 'error'} />
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
                  {pagedCards.map((card) => (
                    <button
                      key={card.id}
                      type="button"
                      onClick={() => handleSelectCard(card)}
                      className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left transition-colors hover:bg-navy-50/50"
                    >
                      <span>
                        <span className="block font-medium text-slate-900">{cardLabel(card)}</span>
                        <span className="block text-xs text-slate-500">
                          COD +₹{card.cod_surcharge} · {slabCounts[card.id] ?? '—'} slab{slabCounts[card.id] === 1 ? '' : 's'}
                        </span>
                      </span>
                      <StatusBadge label={card.active ? 'Active' : 'Inactive'} state={card.active ? 'ok' : 'error'} />
                    </button>
                  ))}
                </div>

                <Pagination page={page} totalItems={displayedCards.length} pageSize={PAGE_SIZE} onPageChange={setPage} />
              </>
            )}
          </div>
        </>
      )}

      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Create rate card">
        <form onSubmit={handleCreateCard} className="space-y-4">
          <ErrorBanner message={createError} />

          <div>
            <label htmlFor="rate_order_type" className="block text-sm font-medium text-slate-700">
              Order type
            </label>
            <Select
              id="rate_order_type"
              value={newOrderType}
              onChange={(v) => setNewOrderType(v as OrderType)}
              options={ORDER_TYPES.map((t) => ({ value: t, label: t }))}
              className="mt-1"
            />
          </div>

          <div>
            <label htmlFor="rate_zone_relationship" className="block text-sm font-medium text-slate-700">
              Zone relationship
            </label>
            <Select
              id="rate_zone_relationship"
              value={newZoneRelationship}
              onChange={(v) => setNewZoneRelationship(v as ZoneRelationship)}
              options={ZONE_RELATIONSHIPS.map((r) => ({ value: r, label: r }))}
              className="mt-1"
            />
          </div>

          <div>
            <label htmlFor="rate_cod_surcharge" className="block text-sm font-medium text-slate-700">
              COD surcharge
            </label>
            <input
              id="rate_cod_surcharge"
              type="number"
              min="0"
              step="0.01"
              value={newCodSurcharge}
              onChange={(e) => setNewCodSurcharge(e.target.value)}
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <p className="text-xs text-slate-500">New rate cards start inactive — activate one after creating it, once it's ready.</p>

          <button
            type="submit"
            disabled={creatingCard}
            className="w-full rounded-md bg-navy-600 px-4 py-2 text-sm font-medium text-white hover:bg-navy-700 disabled:opacity-50"
          >
            {creatingCard ? 'Creating…' : 'Create rate card'}
          </button>
        </form>
      </Modal>
    </Layout>
  )
}
