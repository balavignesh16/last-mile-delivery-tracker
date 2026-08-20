import { useEffect, useState } from 'react'
import { ApiError } from '../services/api'
import { getHealth } from '../services/health'
import type { HealthResponse } from '../types/health'

type HealthCheckState =
  | { status: 'loading' }
  | { status: 'success'; data: HealthResponse }
  | { status: 'error'; message: string }

export function useHealthCheck(): HealthCheckState {
  const [state, setState] = useState<HealthCheckState>({ status: 'loading' })

  useEffect(() => {
    let cancelled = false

    getHealth()
      .then((data) => {
        if (!cancelled) setState({ status: 'success', data })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        const message = err instanceof ApiError ? err.message : 'Unexpected error checking backend health.'
        setState({ status: 'error', message })
      })

    return () => {
      cancelled = true
    }
  }, [])

  return state
}
