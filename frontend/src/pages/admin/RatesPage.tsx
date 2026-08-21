import { useEffect, useState, type FormEvent } from 'react'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Layout } from '../../components/Layout'
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

// Minimal admin rate-card management, per M05's explicit scope: create
// and manage the (order_type, zone_relationship) rate cards and their
// weight slabs. No calculator, no quote preview — computing a charge
// from these numbers is M06's job, not this page's.
export function RatesPage() {
  const { token } = useAuth()

  const [cards, setCards] = useState<RateCard[]>([])
  const [cardsLoading, setCardsLoading] = useState(true)
  const [cardsError, setCardsError] = useState<string | null>(null)

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
      setNewCodSurcharge('0')
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
    } catch (err) {
      setSlabDeleteError(err instanceof ApiError ? err.message : 'Could not delete slab.')
    }
  }

  return (
    <Layout>
      <h1 className="text-xl font-semibold">Rate Cards</h1>

      <div className="mt-6 rounded-lg border border-slate-200 bg-white p-6 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700">Create a new rate card</h2>
        <form onSubmit={handleCreateCard} className="mt-4 flex flex-wrap items-end gap-4">
          <ErrorBanner message={createError} />

          <div>
            <label htmlFor="rate_order_type" className="block text-sm font-medium text-slate-700">
              Order type
            </label>
            <select
              id="rate_order_type"
              value={newOrderType}
              onChange={(e) => setNewOrderType(e.target.value as OrderType)}
              className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              {ORDER_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label htmlFor="rate_zone_relationship" className="block text-sm font-medium text-slate-700">
              Zone relationship
            </label>
            <select
              id="rate_zone_relationship"
              value={newZoneRelationship}
              onChange={(e) => setNewZoneRelationship(e.target.value as ZoneRelationship)}
              className="mt-1 rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              {ZONE_RELATIONSHIPS.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
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
              className="mt-1 w-32 rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>

          <button
            type="submit"
            disabled={creatingCard}
            className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
          >
            {creatingCard ? 'Creating…' : 'Create rate card'}
          </button>
        </form>
        <p className="mt-2 text-xs text-slate-500">New rate cards start inactive — activate one below once it's ready.</p>
      </div>

      <div className="mt-6 grid gap-6 md:grid-cols-2">
        <div className="rounded-lg border border-slate-200 bg-white shadow-sm">
          <h2 className="border-b border-slate-200 px-6 py-4 text-sm font-semibold text-slate-700">All rate cards</h2>
          <ErrorBanner message={cardsError} />
          {cardsLoading ? (
            <p className="px-6 py-4 text-sm text-slate-500">Loading…</p>
          ) : cards.length === 0 ? (
            <p className="px-6 py-4 text-sm text-slate-500">No rate cards yet.</p>
          ) : (
            <ul>
              {cards.map((card) => (
                <li key={card.id} className="border-t border-slate-100 first:border-t-0">
                  <button
                    type="button"
                    onClick={() => handleSelectCard(card)}
                    className={`flex w-full items-center justify-between px-6 py-3 text-left text-sm hover:bg-slate-50 ${
                      card.id === selectedCardId ? 'bg-slate-50' : ''
                    }`}
                  >
                    <span>
                      {card.order_type} · {card.zone_relationship}
                      <span className="ml-2 text-slate-400">COD +{card.cod_surcharge}</span>
                    </span>
                    <StatusBadge label={card.active ? 'Active' : 'Inactive'} state={card.active ? 'ok' : 'error'} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="rounded-lg border border-slate-200 bg-white shadow-sm">
          {!selectedCard ? (
            <p className="px-6 py-8 text-center text-sm text-slate-500">Select a rate card to manage its slabs.</p>
          ) : (
            <>
              <div className="border-b border-slate-200 px-6 py-4">
                <h2 className="text-sm font-semibold text-slate-700">
                  Manage {selectedCard.order_type} · {selectedCard.zone_relationship}
                </h2>
                <form onSubmit={handleSaveCard} className="mt-3 flex flex-wrap items-end gap-3">
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
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={handleToggleActive}
                    className="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100"
                  >
                    {selectedCard.active ? 'Deactivate' : 'Activate'}
                  </button>
                </form>
              </div>

              <div className="px-6 py-4">
                <h3 className="text-sm font-semibold text-slate-700">Weight slabs</h3>
                <ErrorBanner message={slabsError} />
                <ErrorBanner message={slabDeleteError} />
                {slabsLoading ? (
                  <p className="mt-3 text-sm text-slate-500">Loading…</p>
                ) : slabs.length === 0 ? (
                  <p className="mt-3 text-sm text-slate-500">No slabs configured yet.</p>
                ) : (
                  <ul className="mt-3 space-y-2">
                    {slabs.map((slab) =>
                      editingSlabId === slab.id ? (
                        <li key={slab.id}>
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
                        </li>
                      ) : (
                        <li key={slab.id} className="flex items-center justify-between rounded-md border border-slate-100 px-3 py-2 text-sm">
                          <span>
                            {formatWeightRange(slab)} <span className="text-slate-400">→ ₹{slab.price}</span>
                          </span>
                          <span className="flex gap-3">
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
                        </li>
                      ),
                    )}
                  </ul>
                )}

                <form onSubmit={handleCreateSlab} className="mt-4 flex flex-wrap items-end gap-3">
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
                    className="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 disabled:opacity-50"
                  >
                    {creatingSlab ? 'Adding…' : 'Add slab'}
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
