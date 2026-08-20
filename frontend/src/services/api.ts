// Minimal fetch wrapper. Deliberately not a state-management library or an
// HTTP client dependency — a single `fetch` call is enough for M01's one
// endpoint, and later modules can extend this file as real API needs
// (auth headers, retries) actually arise.

const API_BASE = '/api'

export class ApiError extends Error {
  status?: number

  constructor(message: string, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function apiGet<T>(path: string): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`)
  } catch {
    throw new ApiError('Could not reach the backend. Is it running?')
  }

  if (!response.ok) {
    throw new ApiError(`Request failed with status ${response.status}`, response.status)
  }

  return (await response.json()) as T
}
