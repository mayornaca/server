import { useEffect, useState } from "react"

// PendingCountdown — para tests en estado PENDING, muestra cuánto tiempo
// llevan en cola y cuánto falta para que `Service.ExpirePending` los marque
// ERROR(timeout). El TTL del cloud es `PendingTTL` (30 min en gesvial.17+).
//
// cloud-gesvial.21.0: pensado para la sección "Tests en curso" de Monitor.
// Re-renderea cada segundo para que el countdown viva.

type Props = {
  startedAt: string  // ISO timestamp del momento en que el cloud creó el PENDING
  ttlMinutes?: number // default 30 (matches Service.PendingTTL)
}

export function PendingCountdown({ startedAt, ttlMinutes = 30 }: Props) {
  // tick cada segundo
  const [, tick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [])

  const startedMs = new Date(startedAt).getTime()
  const elapsed = Date.now() - startedMs
  const remainingMs = ttlMinutes * 60_000 - elapsed

  if (remainingMs <= 0) {
    return (
      <span className="text-red-600 font-mono text-xs">
        ⏱ TTL vencido — esperando expiración server
      </span>
    )
  }

  const elapsedSec = Math.floor(elapsed / 1000)
  const remainSec = Math.floor(remainingMs / 1000)
  const remainStr =
    remainSec >= 60 ? `${Math.floor(remainSec / 60)}m ${remainSec % 60}s` : `${remainSec}s`
  const elapsedStr =
    elapsedSec >= 60 ? `${Math.floor(elapsedSec / 60)}m ${elapsedSec % 60}s` : `${elapsedSec}s`

  // Color según urgencia
  const isWarning = remainingMs < 5 * 60_000  // <5min restantes
  const isCritical = remainingMs < 60_000     // <1min restante
  const className = isCritical
    ? "text-red-600"
    : isWarning
      ? "text-yellow-700"
      : "text-muted-foreground"

  return (
    <span className={`font-mono text-xs ${className}`}>
      ⏱ {elapsedStr} · TTL en {remainStr}
    </span>
  )
}
