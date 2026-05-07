import { useMemo } from "react"
import type { TestResult, SosPost } from "@/api/types"
import { formatDate } from "@/lib/utils"

// cloud-gesvial.19.3 D6: vista alternativa del historial de pruebas como
// timeline horizontal. Pensada para entrega de turno: el operador ve "qué
// pasó en mis 8 horas" como una franja temporal, una columna por hora, una
// fila por poste, una marca por prueba. La tabla paginada de Tests.tsx
// sigue siendo la vista por defecto — esta es complementaria, accesible
// desde el toggle "Línea de tiempo" en la página de Pruebas.
//
// Diseño:
//   - El eje X es el tiempo (de izquierda → derecha = pasado → presente).
//   - El eje Y es el poste (ordenados por nombre).
//   - Cada test es un círculo cuyo color refleja su estado (PASSED verde,
//     FAILED rojo, PENDING amarillo, ERROR violeta).
//   - Click sobre el círculo → callback `onSelect(test)` que abre el modal
//     de detalle (mismo modal que la tabla).
//
// Limites por diseño:
//   - Si la ventana cubre >24h, pierde resolución (los círculos se
//     solapan). El operador puede zoomar acotando "Desde/Hasta" en los
//     filtros.
//   - >50 postes la fila queda muy alta. La paginación aplica por poste
//     (no por test) — si el contrato escala, ver E1 (NetworkStatus mural).

type Props = {
  tests: TestResult[]
  posts: SosPost[]
  onSelect: (test: TestResult) => void
}

const STATUS_COLOR: Record<TestResult["status"], string> = {
  PASSED: "bg-green-500 hover:ring-green-300",
  FAILED: "bg-red-500 hover:ring-red-300",
  ERROR: "bg-purple-500 hover:ring-purple-300",
  PENDING: "bg-yellow-400 hover:ring-yellow-200",
}

const STATUS_LABEL: Record<TestResult["status"], string> = {
  PASSED: "Aprobado",
  FAILED: "Fallido",
  ERROR: "Error",
  PENDING: "Pendiente",
}

export function TestsTimeline({ tests, posts, onSelect }: Props) {
  const { rows, minTs, maxTs } = useMemo(() => {
    if (tests.length === 0) {
      return { rows: [], minTs: 0, maxTs: 0 }
    }
    // Group tests by post.
    const byPost = new Map<string, TestResult[]>()
    for (const t of tests) {
      const arr = byPost.get(t.postId) ?? []
      arr.push(t)
      byPost.set(t.postId, arr)
    }
    // Sort posts by name (or fallback to id) so the rows are stable.
    const postById = new Map(posts.map((p) => [p.id, p]))
    const rows = Array.from(byPost.entries())
      .map(([postId, list]) => ({
        postId,
        post: postById.get(postId),
        tests: list,
      }))
      .sort((a, b) => {
        const an = a.post?.name ?? a.postId
        const bn = b.post?.name ?? b.postId
        return an.localeCompare(bn)
      })
    const allTs = tests.map((t) => new Date(t.CreatedAt).getTime())
    return {
      rows,
      minTs: Math.min(...allTs),
      maxTs: Math.max(...allTs),
    }
  }, [tests, posts])

  if (rows.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        Sin pruebas en el rango seleccionado.
      </p>
    )
  }

  const span = Math.max(maxTs - minTs, 60_000) // avoid div-by-zero, min 1 min

  return (
    <div className="border rounded-md p-4 space-y-1 overflow-x-auto">
      {/* Eje temporal */}
      <div className="flex justify-between text-xs text-muted-foreground pb-2 border-b">
        <span>{formatDate(new Date(minTs).toISOString())}</span>
        <span className="font-medium">Línea de tiempo (izq. = más antiguo)</span>
        <span>{formatDate(new Date(maxTs).toISOString())}</span>
      </div>

      {rows.map((row) => (
        <div
          key={row.postId}
          className="flex items-center gap-3 py-1.5 border-b last:border-0"
        >
          {/* Etiqueta del poste */}
          <div className="w-44 shrink-0 truncate text-sm">
            {row.post ? (
              <>
                <span className="font-medium">{row.post.name}</span>
                <span className="text-muted-foreground text-xs ml-1">
                  KM {row.post.kmMarker}
                </span>
              </>
            ) : (
              <span className="font-mono text-xs text-muted-foreground">
                {row.postId.slice(0, 12)}
              </span>
            )}
          </div>

          {/* Track temporal */}
          <div className="relative flex-1 h-6 bg-muted/40 rounded">
            {row.tests.map((t) => {
              const ts = new Date(t.CreatedAt).getTime()
              const left = ((ts - minTs) / span) * 100
              // cloud-gesvial.20: a permanent ring marks tests where the
              // carrier confirmed transport-layer delivery. Combined with
              // the dot color (status) the operator sees at a glance which
              // FAILED tests are POST_NOT_RESPONDING (ring + red) vs
              // NETWORK_DELIVERY (no ring + red).
              const ringClass = t.deliveryConfirmed === true ? "ring-2 ring-blue-400" : ""
              const layerHint =
                t.deliveryConfirmed === true
                  ? ` · 📡 ${t.deliveryCarrier ?? "Entregado"}`
                  : t.deliveryConfirmed === false
                    ? " · 📡 Sin entrega"
                    : ""
              return (
                <button
                  key={t.id}
                  onClick={() => onSelect(t)}
                  style={{ left: `${left}%` }}
                  className={`absolute top-1/2 -translate-x-1/2 -translate-y-1/2 size-3 rounded-full ring-offset-1 hover:ring-2 transition-shadow cursor-pointer ${STATUS_COLOR[t.status]} ${ringClass}`}
                  title={`${t.testType} · ${STATUS_LABEL[t.status]}${layerHint} · ${formatDate(t.CreatedAt)}`}
                  aria-label={`Prueba ${t.testType} ${STATUS_LABEL[t.status]}${layerHint} ${formatDate(t.CreatedAt)}`}
                />
              )
            })}
          </div>

          {/* Conteo + último estado */}
          <div className="w-20 shrink-0 text-right text-xs text-muted-foreground">
            {row.tests.length} pruebas
          </div>
        </div>
      ))}

      {/* Leyenda */}
      <div className="flex gap-4 pt-3 text-xs text-muted-foreground border-t flex-wrap">
        {(["PASSED", "FAILED", "ERROR", "PENDING"] as const).map((s) => (
          <span key={s} className="flex items-center gap-1.5">
            <span className={`inline-block size-3 rounded-full ${STATUS_COLOR[s].split(" ")[0]}`} />
            {STATUS_LABEL[s]}
          </span>
        ))}
        {/* cloud-gesvial.20: leyenda del anillo de entrega-carrier */}
        <span className="flex items-center gap-1.5 ml-2">
          <span className="inline-block size-3 rounded-full bg-muted ring-2 ring-blue-400" />
          Entregado por carrier
        </span>
      </div>
    </div>
  )
}
