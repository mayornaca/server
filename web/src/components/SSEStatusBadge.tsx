import { useEffect, useState } from "react"
import { Wifi, WifiOff, RefreshCw } from "lucide-react"

// SSEStatusBadge — pequeño indicador en el header de Monitor con el estado
// de la conexión SSE y la hora del último update visible.
//
// cloud-gesvial.21.0: pensado para que el operador del centro pueda dejar la
// pantalla puesta y, de un vistazo, saber si los datos siguen siendo
// recientes (verde + hora reciente) o si la conexión está degradada (amarillo
// reconectando) o caída (rojo polling fallback).
//
// La variable `lastEventAt` se actualiza desde Monitor.tsx cada vez que
// recibe un evento SSE. Si pasan más de `staleAfterMs` sin eventos, el
// indicador pasa a "stale" — esto es independiente del propio estado de la
// conexión SSE (que no expone `EventSource.readyState` directo en algunos
// browsers viejos).

type Props = {
  lastEventAt: number | null  // epoch millis, null si nunca llegó nada
  staleAfterMs?: number        // default 60s — si el SSE no emite en ese rato, marcar stale
}

export function SSEStatusBadge({ lastEventAt, staleAfterMs = 60_000 }: Props) {
  // Forzar re-render cada 5s para que la etiqueta "hace N segundos" se mantenga viva.
  const [, tick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 5000)
    return () => clearInterval(id)
  }, [])

  const now = Date.now()
  const ageMs = lastEventAt ? now - lastEventAt : Infinity

  let icon = <Wifi className="size-3.5" />
  let label = "Conectado"
  let className = "text-green-600 bg-green-50 border-green-200"

  if (lastEventAt === null) {
    icon = <RefreshCw className="size-3.5 animate-spin" />
    label = "Esperando primer evento…"
    className = "text-muted-foreground bg-muted border-muted-foreground/20"
  } else if (ageMs > staleAfterMs * 3) {
    icon = <WifiOff className="size-3.5" />
    label = `Sin datos hace ${formatAge(ageMs)}`
    className = "text-red-700 bg-red-50 border-red-200"
  } else if (ageMs > staleAfterMs) {
    icon = <RefreshCw className="size-3.5" />
    label = `Última actualización hace ${formatAge(ageMs)}`
    className = "text-yellow-700 bg-yellow-50 border-yellow-200"
  } else {
    label = `Última actualización ${formatTimeOfDay(lastEventAt)}`
  }

  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full border ${className}`}>
      {icon}
      {label}
    </span>
  )
}

function formatAge(ms: number): string {
  if (ms < 60_000) return `${Math.floor(ms / 1000)}s`
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`
  return `${Math.floor(ms / 3_600_000)}h`
}

function formatTimeOfDay(ts: number): string {
  const d = new Date(ts)
  // cloud-gesvial.21.0.1: forzar formato 24h para alinear con el resto del
  // panel (formatDate de lib/utils ya usa 24h). Pre-fix decía "02:15:48 p. m."
  // mientras los otros componentes mostraban "14:15", inconsistencia visual.
  return d.toLocaleTimeString("es-CL", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  })
}
