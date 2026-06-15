const BASE = '/api/v1'

let accessToken: string | null = null

export function setAccessToken(t: string | null) { accessToken = t }
export function getAccessToken() { return accessToken }

async function refreshTokens(): Promise<boolean> {
  const rt = localStorage.getItem('refresh_token')
  if (!rt) return false
  try {
    const res = await fetch(`${BASE}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt }),
    })
    if (!res.ok) { localStorage.removeItem('refresh_token'); return false }
    const { data } = await res.json()
    accessToken = data.access_token
    localStorage.setItem('refresh_token', data.refresh_token)
    return true
  } catch { return false }
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init: RequestInit = {}, retry = true): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init.headers as Record<string, string>),
  }
  if (accessToken) headers['Authorization'] = `Bearer ${accessToken}`

  const res = await fetch(`${BASE}${path}`, { ...init, headers })

  if (res.status === 401 && retry) {
    const ok = await refreshTokens()
    if (ok) return request<T>(path, init, false)
    accessToken = null
    localStorage.removeItem('refresh_token')
    window.dispatchEvent(new Event('auth:logout'))
    throw new ApiError(401, 'unauthorized', 'Session expired')
  }

  if (!res.ok) {
    let code = 'error'
    let message = res.statusText
    try {
      const body = await res.json()
      code = body.code ?? body.type ?? code
      message = body.detail ?? body.title ?? message
    } catch {}
    throw new ApiError(res.status, code, message)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  get:    <T>(path: string)               => request<T>(path, { method: 'GET' }),
  post:   <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST',   body: body ? JSON.stringify(body) : undefined }),
  patch:  <T>(path: string, body?: unknown) => request<T>(path, { method: 'PATCH',  body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string)               => request<T>(path, { method: 'DELETE' }),
}
