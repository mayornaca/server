import { useEffect, useRef } from "react"

// PanelEvent mirrors the Go-side paneleventsbus.Event. cloud-gesvial.19+.
export type PanelEventType =
  | "test.scheduled"
  | "test.completed"
  | "post.statusChanged"
  | "device.connected"
  | "device.disconnected"
  | "schedule.fired"

export interface PanelEvent {
  type: PanelEventType
  userId?: string
  resourceId?: string
  status?: string
  at: string
}

type Handler = (event: PanelEvent) => void

function readAccessToken(): string | null {
  const raw = localStorage.getItem("auth")
  if (!raw) return null
  try {
    const tokens = JSON.parse(raw) as { access_token?: string }
    return tokens.access_token ?? null
  } catch {
    return null
  }
}

/**
 * usePanelEvents subscribes to /api/3rdparty/v1/events/panel via EventSource
 * and invokes `onEvent` for every event whose type appears in `types`. The
 * connection is opened on mount, closed on unmount.
 *
 * cloud-gesvial.19.1: pre-fix the access token was captured once at mount
 * and never refreshed — when the JWT TTL expired the browser auto-reconnected
 * with a stale token, the server 401'd every time, and the panel silently
 * stopped receiving events until a full reload. Now `onerror` reads a fresh
 * token from localStorage and recreates the EventSource if it changed (which
 * is what apiFetch will have written after a successful refresh).
 */
export function usePanelEvents(types: PanelEventType[], onEvent: Handler): void {
  const handlerRef = useRef<Handler>(onEvent)
  handlerRef.current = onEvent

  // Snapshot the types list as a comma-joined key so we don't re-open on
  // each render even if the parent passes a freshly-allocated array.
  const typesKey = types.join(",")

  useEffect(() => {
    let es: EventSource | null = null
    let currentToken: string | null = null
    let reopenTimer: ReturnType<typeof setTimeout> | null = null
    let cancelled = false

    const subscribed = typesKey.split(",")

    function open() {
      if (cancelled) return
      currentToken = readAccessToken()
      if (!currentToken) {
        // No session yet — try again shortly. Cheap; the browser would also
        // retry on its own and 401 forever otherwise.
        reopenTimer = setTimeout(open, 5000)
        return
      }
      const url = new URL("/api/3rdparty/v1/events/panel", window.location.origin)
      url.searchParams.set("token", currentToken)
      es = new EventSource(url.toString(), { withCredentials: true })

      for (const t of subscribed) {
        es.addEventListener(t, (msg: MessageEvent) => {
          try {
            const data = JSON.parse(msg.data) as PanelEvent
            handlerRef.current(data)
          } catch {
            // hello / : ping — ignore
          }
        })
      }

      es.onerror = () => {
        // Most onerror events are transient network blips that the browser
        // already auto-retries. We only intervene when the token has rotated
        // (apiFetch refreshed it after a 401 elsewhere): close the dead
        // EventSource and reopen with the new token.
        const fresh = readAccessToken()
        if (fresh && fresh !== currentToken) {
          es?.close()
          es = null
          if (reopenTimer) clearTimeout(reopenTimer)
          reopenTimer = setTimeout(open, 250)
        }
      }
    }

    open()

    return () => {
      cancelled = true
      if (reopenTimer) clearTimeout(reopenTimer)
      es?.close()
    }
  }, [typesKey])
}
