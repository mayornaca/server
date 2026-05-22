import type { TokenResponse } from "./types"

const API_BASE = "/api/3rdparty/v1"

// ApiError carries the HTTP status and parsed body so callers can branch on
// specific codes (e.g. 409 for PENDING dedupe) and read the server payload.
export class ApiError extends Error {
  status: number
  body: any
  constructor(status: number, body: any, message?: string) {
    super(message ?? body?.error ?? body?.message ?? `HTTP ${status}`)
    this.status = status
    this.body = body
  }
}

function getTokens() {
  const raw = localStorage.getItem("auth")
  if (!raw) return null
  // cloud-gesvial.19.1: a corrupted localStorage entry used to throw
  // SyntaxError out of every API call. Treat it as "no session".
  try {
    return JSON.parse(raw) as TokenResponse
  } catch {
    localStorage.removeItem("auth")
    return null
  }
}

// cloud-gesvial.22.0.5: scopes del JWT actual, decodificados client-side.
// El server es la autoridad — esto es UX (ocultar acciones que la API
// rechazaría con 403 igualmente). Si el JWT no tiene `scopes` o no parsea,
// devuelve [] (= ninguna acción restringida visible).
export function getScopes(): string[] {
  const tokens = getTokens()
  if (!tokens?.access_token) return []
  try {
    const payload = JSON.parse(atob(tokens.access_token.split(".")[1]))
    return Array.isArray(payload.scopes) ? payload.scopes : []
  } catch {
    return []
  }
}

export function hasScope(scope: string): boolean {
  return getScopes().includes(scope)
}

function setTokens(tokens: TokenResponse) {
  localStorage.setItem("auth", JSON.stringify(tokens))
}

function clearTokens() {
  localStorage.removeItem("auth")
}

// cloud-gesvial.19.1: serialise concurrent refresh attempts. Refresh tokens
// are single-use; without this, parallel 401s (Promise.all on Dashboard) all
// independently kick off /auth/token/refresh, the second consumes a token
// already burned by the first and force-logs the user out every hour.
let refreshInflight: Promise<boolean> | null = null

function refreshAccessToken(): Promise<boolean> {
  if (refreshInflight) return refreshInflight
  refreshInflight = (async () => {
    try {
      const tokens = getTokens()
      if (!tokens?.refresh_token) return false

      const res = await fetch(`${API_BASE}/auth/token/refresh`, {
        method: "POST",
        headers: { Authorization: `Bearer ${tokens.refresh_token}` },
      })

      if (!res.ok) {
        clearTokens()
        return false
      }

      const newTokens: TokenResponse = await res.json()
      setTokens(newTokens)
      return true
    } finally {
      refreshInflight = null
    }
  })()
  return refreshInflight
}

async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
): Promise<{ data: T; totalCount?: number }> {
  const tokens = getTokens()

  const headers: Record<string, string> = {
    ...((options.headers as Record<string, string>) || {}),
  }

  if (tokens?.access_token) {
    headers["Authorization"] = `Bearer ${tokens.access_token}`
  }

  if (options.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json"
  }

  let res = await fetch(`${API_BASE}${path}`, { ...options, headers })

  // Auto-refresh on 401
  if (res.status === 401 && tokens?.refresh_token) {
    const refreshed = await refreshAccessToken()
    if (refreshed) {
      const newTokens = getTokens()!
      headers["Authorization"] = `Bearer ${newTokens.access_token}`
      res = await fetch(`${API_BASE}${path}`, { ...options, headers })
    } else {
      // Refresh failed — session expired, redirect to login
      window.location.href = "/login"
      throw new Error("Sesión expirada")
    }
  }

  if (res.status === 401) {
    // Still 401 after refresh attempt — force logout
    clearTokens()
    window.location.href = "/login"
    throw new Error("Sesión expirada")
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ message: res.statusText }))
    throw new ApiError(res.status, err, err.message || err.error)
  }

  const totalCount = res.headers.get("X-Total-Count")

  // cloud-gesvial.22.0.7: tolerar respuestas sin body (204 No Content,
  // o 200 con Content-Length: 0). Pre-fix `auth.changePassword` reventaba
  // con "Failed to execute 'json' on 'Response': Unexpected end of JSON
  // input" porque el handler PATCH /auth/password devuelve 204 y `.json()`
  // sobre body vacío tira SyntaxError. Cualquier endpoint que devuelva
  // 204 caía en el mismo agujero.
  const contentLength = res.headers.get("Content-Length")
  if (res.status === 204 || contentLength === "0") {
    return {
      data: undefined as unknown as T,
      totalCount: totalCount ? parseInt(totalCount, 10) : undefined,
    }
  }

  const data: T = await res.json()

  return {
    data,
    totalCount: totalCount ? parseInt(totalCount, 10) : undefined,
  }
}

// Auth
export async function login(
  username: string,
  password: string,
): Promise<TokenResponse> {
  const basicAuth = btoa(`${username}:${password}`)
  const res = await fetch(`${API_BASE}/auth/token`, {
    method: "POST",
    headers: {
      Authorization: `Basic ${basicAuth}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      scopes: [
        "posts:list", "posts:read", "posts:write", "posts:delete",
        "tests:list", "tests:read", "tests:schedule",
        "schedules:list", "schedules:read", "schedules:write", "schedules:delete",
        "devices:list", "devices:delete",
        "tokens:manage",
        "admin:all",
      ],
      ttl: 3600,
    }),
  })

  if (!res.ok) {
    throw new Error("Credenciales inválidas")
  }

  const tokens: TokenResponse = await res.json()
  setTokens(tokens)
  return tokens
}

export function logout() {
  clearTokens()
  window.location.href = "/login"
}

// cloud-gesvial.19.1: also verify the JWT hasn't expired. Pre-fix
// ProtectedRoute let the user into protected pages on a stale token, the
// first API call then 401'd and force-logged them out — visible as a brief
// flash of authenticated UI before the redirect. Returns false on any
// parse error too (corrupted token).
export function isAuthenticated(): boolean {
  const tokens = getTokens()
  if (!tokens?.access_token) return false
  try {
    const payload = JSON.parse(atob(tokens.access_token.split(".")[1]))
    if (typeof payload.exp === "number" && payload.exp * 1000 <= Date.now()) {
      // Expired but a refresh token may still rescue the session — keep
      // tokens in storage; apiFetch will handle the 401/refresh dance.
      return !!tokens.refresh_token
    }
    return true
  } catch {
    return false
  }
}

export function getCurrentUser(): string | null {
  const tokens = getTokens()
  if (!tokens?.access_token) return null
  try {
    const payload = JSON.parse(atob(tokens.access_token.split(".")[1]))
    return payload.user_id || payload.sub || null
  } catch {
    return null
  }
}

// Posts
export const posts = {
  list: (params?: Record<string, string>) => {
    const qs = params ? "?" + new URLSearchParams(params).toString() : ""
    return apiFetch<import("./types").SosPost[]>(`/posts${qs}`)
  },
  get: (id: string) =>
    apiFetch<import("./types").SosPost>(`/posts/${id}`),
  create: (data: Partial<import("./types").SosPost>) =>
    apiFetch<import("./types").SosPost>("/posts", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  update: (id: string, data: Partial<import("./types").SosPost>) =>
    apiFetch<import("./types").SosPost>(`/posts/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  patch: (id: string, data: import("./types").PostPatch) =>
    apiFetch<import("./types").SosPost>(`/posts/${id}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  delete: (id: string) =>
    apiFetch<void>(`/posts/${id}`, { method: "DELETE" }),
  summary: () =>
    apiFetch<import("./types").PostsSummary>("/posts/summary"),
}

// Devices
export const devices = {
  list: () => apiFetch<import("./types").Device[]>("/devices"),
}

// Password
export const auth = {
  changePassword: (currentPassword: string, newPassword: string) =>
    apiFetch<void>("/auth/password", {
      method: "PATCH",
      body: JSON.stringify({ currentPassword, newPassword }),
    }),
}

// Tests
export const tests = {
  list: (params?: Record<string, string>) => {
    const qs = params ? "?" + new URLSearchParams(params).toString() : ""
    return apiFetch<import("./types").TestResult[]>(`/tests${qs}`)
  },
  get: (id: string) =>
    apiFetch<import("./types").TestResult>(`/tests/${id}`),
  // cloud-gesvial.17: opcional `force` cancela cualquier PENDING vigente
  // para el mismo (postId, testType) y crea uno nuevo. El PENDING viejo
  // queda en el audit trail con error="superseded by retry". Se usa desde
  // el botón "Cancelar pendiente y reintentar" del 409.
  schedule: (postId: string, testType: string, deviceId?: string, force?: boolean) =>
    apiFetch<import("./types").TestResult>("/tests/schedule", {
      method: "POST",
      body: JSON.stringify({ postId, testType, deviceId: deviceId || undefined, force: !!force }),
    }),
  scheduleBatch: (postIds: string[], testType: string, deviceId?: string, force?: boolean) =>
    apiFetch<import("./types").BatchScheduleResponse>("/tests/schedule/batch", {
      method: "POST",
      body: JSON.stringify({ postIds, testType, deviceId: deviceId || undefined, force: !!force }),
    }),
  // cloud-gesvial.17: cancela un PENDING (lo marca ERROR "cancelled by operator")
  // para liberar el dedupe del schedule sin esperar el TTL de 10 min.
  cancel: (id: string) =>
    apiFetch<import("./types").TestResult>(`/tests/${id}`, { method: "DELETE" }),
}

// Test schedules (gesvial.11)
export const schedules = {
  list: () => apiFetch<import("./types").TestSchedule[]>("/schedules"),
  get: (id: string) =>
    apiFetch<import("./types").TestSchedule>(`/schedules/${id}`),
  create: (data: import("./types").TestScheduleInput) =>
    apiFetch<import("./types").TestSchedule>("/schedules", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  update: (id: string, data: import("./types").TestScheduleInput) =>
    apiFetch<import("./types").TestSchedule>(`/schedules/${id}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: (id: string) =>
    apiFetch<void>(`/schedules/${id}`, { method: "DELETE" }),
}
