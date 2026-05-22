import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatDate(dateStr: string | null | undefined): string {
  if (!dateStr) return "—"
  return new Date(dateStr).toLocaleString('es-CL', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

// timeAgo formats a relative time consistently. cloud-gesvial.19.3 D5: pre-fix
// the format mixed units ("min" / "h" / "d") and capitalization rules in a way
// the operator manual could not document accurately. Now every output uses the
// same shape "hace N <unidad>" with full Spanish words, plus the special case
// "recién" (< 1 min) for very fresh data.
export function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  if (diff < 0) return "en un momento" // clock skew between server and panel
  const seconds = Math.floor(diff / 1000)
  if (seconds < 45) return "recién"
  const mins = Math.floor(diff / 60000)
  if (mins < 60) return `hace ${mins} ${mins === 1 ? "minuto" : "minutos"}`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `hace ${hours} ${hours === 1 ? "hora" : "horas"}`
  const days = Math.floor(hours / 24)
  if (days < 30) return `hace ${days} ${days === 1 ? "día" : "días"}`
  const months = Math.floor(days / 30)
  if (months < 12) return `hace ${months} ${months === 1 ? "mes" : "meses"}`
  const years = Math.floor(days / 365)
  return `hace ${years} ${years === 1 ? "año" : "años"}`
}

// isOnline returns true when the device's last_seen is within ONLINE_THRESHOLD_MS.
// Exposed as a constant so the UI tooltip and the operator manual can reference
// the same source of truth. cloud-gesvial.19.3 D2.
export const ONLINE_THRESHOLD_MS = 5 * 60 * 1000

export function isOnline(lastSeen: string): boolean {
  return Date.now() - new Date(lastSeen).getTime() < ONLINE_THRESHOLD_MS
}

// cloud-gesvial.22.1.0: humaniza textos técnicos del campo `error` de
// test_results. El backend genera frases tipo "timeout sin respuesta del
// gateway" o "superseded by retry" — útiles en logs / tickets pero ruido
// para el operador del panel. Cualquier valor desconocido pasa por igual,
// para no esconder un error verdadero del backend.
export function humanizeTestError(raw: string | null | undefined): string {
  if (!raw) return "—"
  const trimmed = raw.trim()
  if (!trimmed) return "—"
  const map: Record<string, string> = {
    "superseded by retry": "Reintentado por el sistema",
    "timeout sin respuesta del gateway": "El gateway no respondió a tiempo",
    "carrier confirmó entrega pero el poste no respondió": "Carrier entregó pero el poste no respondió",
    "ventana cerró sin SMS recibidos del poste o del carrier": "Ventana cerró sin SMS recibidos",
  }
  return map[trimmed] ?? trimmed
}

// `superseded by retry` es metadata que crea el cron de retry cuando supersede
// un test viejo para crear uno nuevo. El operador no necesita verla en las
// tablas — la usa sólo el equipo backend para auditoría.
export function isSupersededByRetry(error: string | null | undefined): boolean {
  return error?.trim() === "superseded by retry"
}
