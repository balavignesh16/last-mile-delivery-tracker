// Minimal fetch wrapper. Deliberately not an HTTP client dependency (no
// axios) or a data-fetching library — plain `fetch` plus a thin error
// mapping is enough for this project's scope.
//
// Callers pass the full path (e.g. '/health', '/api/v1/auth/login') —
// there is no automatic prefixing, since the backend's own route
// structure already distinguishes unversioned infrastructure endpoints
// (/health) from versioned business endpoints (/api/v1/*).

export class ApiError extends Error {
  status?: number

  constructor(message: string, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, init)
  } catch {
    throw new ApiError('Could not reach the backend. Is it running?')
  }

  if (!response.ok) {
    throw new ApiError(await extractErrorMessage(response), response.status)
  }

  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

// The backend always responds with { "error": "..." } on failure (see
// internal/server.WriteError) — surfacing that message lets forms show
// the user something meaningful ("invalid email or password") instead of
// a generic "request failed" for every kind of error.
async function extractErrorMessage(response: Response): Promise<string> {
  try {
    const body: unknown = await response.json()
    if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') {
      return body.error
    }
  } catch {
    // Response body wasn't JSON — fall through to the generic message.
  }
  return `Request failed with status ${response.status}`
}

export function apiGet<T>(path: string, token?: string): Promise<T> {
  return apiRequest<T>(path, { headers: authHeaders(token) })
}

export function apiPost<T>(path: string, body: unknown, token?: string): Promise<T> {
  return apiRequest<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export function apiPut<T>(path: string, body: unknown, token?: string): Promise<T> {
  return apiRequest<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', ...authHeaders(token) },
    body: JSON.stringify(body),
  })
}

export function apiDelete<T>(path: string, token?: string): Promise<T> {
  return apiRequest<T>(path, { method: 'DELETE', headers: authHeaders(token) })
}

function authHeaders(token?: string): HeadersInit {
  return token ? { Authorization: `Bearer ${token}` } : {}
}
