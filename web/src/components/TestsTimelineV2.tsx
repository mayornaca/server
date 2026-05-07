import { useCallback, useEffect, useMemo, useState } from "react"
import type { TestResult, SosPost, TestType } from "@/api/types"
import { tests as testsApi } from "@/api/client"
import { formatDate } from "@/lib/utils"

// TestsTimelineV2 — replanteo del timeline de pruebas. cloud-gesvial.21.0.
//
// Diferencias con la versión 19.3 (TestsTimeline.tsx):
//   - Fetch independiente del paginado de la tabla. El timeline carga
//     todos los tests del rango temporal seleccionado (hasta 1000), sin
//     respetar la página actual de la tabla. Esto permite comparar
//     cientos de tests a la vez.
//   - Presets temporales explícitos (1h / 8h / 24h / 7d) con fetch propia.
//   - Selector multi-poste para enfocar la comparación.
//   - Densidad inteligente: cuando una fila tiene demasiados tests para
//     mostrar como puntos individuales, agrupa en barras.
//   - Eje temporal con etiquetas reales (HH:MM o DD-HH o DD-MM según span).
//   - Tooltip rico (no solo title=) con failure_kind y carrier visibles.
//   - Anillo azul cuando delivery_confirmed=true.

type Props = {
  posts: SosPost[]
  onSelect: (test: TestResult) => void
  // Filtros heredados de Tests.tsx (tipo y status). Si están definidos se
  // aplican al fetch del timeline.
  typeFilter?: string
  statusFilter?: string
}

type Preset = "1h" | "8h" | "24h" | "7d"

const PRESETS: { value: Preset; label: string; ms: number }[] = [
  { value: "1h",  label: "Última hora",     ms: 60 * 60 * 1000 },
  { value: "8h",  label: "Últimas 8 horas", ms: 8 * 60 * 60 * 1000 },
  { value: "24h", label: "Últimas 24 horas", ms: 24 * 60 * 60 * 1000 },
  { value: "7d",  label: "Últimos 7 días",  ms: 7 * 24 * 60 * 60 * 1000 },
]

const STATUS_COLOR: Record<TestResult["status"], string> = {
  PASSED:  "bg-green-500",
  FAILED:  "bg-red-500",
  ERROR:   "bg-purple-500",
  PENDING: "bg-yellow-400",
}

const STATUS_LABEL: Record<TestResult["status"], string> = {
  PASSED:  "Aprobado",
  FAILED:  "Fallido",
  ERROR:   "Error",
  PENDING: "Pendiente",
}

const FAILURE_KIND_LABEL: Record<string, string> = {
  POST_NOT_RESPONDING: "Poste no responde",
  NETWORK_DELIVERY:    "Sin entrega",
  TIMEOUT:             "Sin evidencia",
}

const MAX_TIMELINE_RESULTS = 1000

export function TestsTimelineV2({ posts, onSelect, typeFilter, statusFilter }: Props) {
  const [preset, setPreset] = useState<Preset>("8h")
  const [tests, setTests] = useState<TestResult[]>([])
  const [loading, setLoading] = useState(false)
  const [truncated, setTruncated] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedPostIds, setSelectedPostIds] = useState<Set<string>>(new Set())

  const range = useMemo(() => {
    const def = PRESETS.find((p) => p.value === preset)!
    const to = new Date()
    const from = new Date(to.getTime() - def.ms)
    return { from, to }
  }, [preset])

  // Fetch propia del timeline — independiente del paginado de Tests.tsx.
  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params: Record<string, string> = {
        from: range.from.toISOString(),
        to:   range.to.toISOString(),
        limit: String(MAX_TIMELINE_RESULTS),
      }
      if (typeFilter)   params.type = typeFilter
      if (statusFilter) params.status = statusFilter
      const { data, totalCount } = await testsApi.list(params)
      setTests(data)
      setTruncated(typeof totalCount === "number" && totalCount > MAX_TIMELINE_RESULTS)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error cargando línea de tiempo")
    } finally {
      setLoading(false)
    }
  }, [range.from, range.to, typeFilter, statusFilter])

  useEffect(() => { void load() }, [load])

  // Indexado por poste, ordenado por nombre / km
  const postById = useMemo(() => new Map(posts.map((p) => [p.id, p])), [posts])

  const rows = useMemo(() => {
    if (tests.length === 0) return []
    const byPost = new Map<string, TestResult[]>()
    for (const t of tests) {
      const arr = byPost.get(t.postId) ?? []
      arr.push(t)
      byPost.set(t.postId, arr)
    }
    const all = Array.from(byPost.entries()).map(([postId, list]) => ({
      postId,
      post: postById.get(postId),
      tests: list,
    }))
    all.sort((a, b) => {
      const ak = parseFloat(a.post?.kmMarker ?? "999999") || 999999
      const bk = parseFloat(b.post?.kmMarker ?? "999999") || 999999
      if (ak !== bk) return ak - bk
      return (a.post?.name ?? a.postId).localeCompare(b.post?.name ?? b.postId)
    })
    // Aplicar filtro multi-poste si el operador eligió alguno
    if (selectedPostIds.size === 0) return all
    return all.filter((r) => selectedPostIds.has(r.postId))
  }, [tests, postById, selectedPostIds])

  const minTs = range.from.getTime()
  const maxTs = range.to.getTime()
  const span = Math.max(maxTs - minTs, 60_000)

  // Eje temporal: tick interval depende del span
  const ticks = useMemo(() => {
    const spanH = span / (60 * 60 * 1000)
    let intervalH = 1
    if (spanH > 48) intervalH = 24       // ticks diarios
    else if (spanH > 12) intervalH = 6   // ticks cada 6h
    else if (spanH > 4) intervalH = 1    // ticks horarios
    else intervalH = 0.5                 // cada 30min
    const intervalMs = intervalH * 60 * 60 * 1000
    const out: { ts: number; label: string }[] = []
    // Empezar en el primer tick "redondo" después de minTs
    const firstTick = Math.ceil(minTs / intervalMs) * intervalMs
    for (let t = firstTick; t <= maxTs; t += intervalMs) {
      const d = new Date(t)
      let label = ""
      if (intervalH >= 24) {
        label = d.toLocaleDateString("es-CL", { day: "2-digit", month: "2-digit" })
      } else if (spanH > 12) {
        label = d.toLocaleString("es-CL", { day: "2-digit", hour: "2-digit", minute: "2-digit" })
      } else {
        label = d.toLocaleTimeString("es-CL", { hour: "2-digit", minute: "2-digit" })
      }
      out.push({ ts: t, label })
    }
    return out
  }, [minTs, maxTs, span])

  // Toggle multi-poste
  const togglePost = (postId: string) => {
    setSelectedPostIds((prev) => {
      const next = new Set(prev)
      if (next.has(postId)) next.delete(postId)
      else next.add(postId)
      return next
    })
  }

  if (loading && tests.length === 0) {
    return (
      <div className="border rounded-md p-6 text-center text-muted-foreground text-sm">
        Cargando línea de tiempo…
      </div>
    )
  }

  if (error) {
    return <p className="text-destructive">{error}</p>
  }

  return (
    <div className="space-y-3">
      {/* Controles */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">Rango:</span>
        {PRESETS.map((p) => (
          <button
            key={p.value}
            onClick={() => setPreset(p.value)}
            className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
              preset === p.value
                ? "bg-primary text-primary-foreground border-primary"
                : "bg-background hover:bg-muted border-border"
            }`}
          >
            {p.label}
          </button>
        ))}
        <span className="text-xs text-muted-foreground ml-2">
          {tests.length} pruebas en el rango
          {truncated && (
            <span className="text-yellow-700"> · ⚠ acotá el rango (&gt;{MAX_TIMELINE_RESULTS})</span>
          )}
        </span>
        {selectedPostIds.size > 0 && (
          <button
            onClick={() => setSelectedPostIds(new Set())}
            className="ml-auto text-xs text-muted-foreground hover:text-foreground"
          >
            Limpiar selección ({selectedPostIds.size})
          </button>
        )}
      </div>

      {/* Selector multi-poste */}
      {rows.length > 1 && (
        <div className="flex flex-wrap gap-1 border rounded-md p-2 bg-muted/20">
          <span className="text-xs text-muted-foreground self-center mr-1">
            Comparar postes (click para alternar):
          </span>
          {Array.from(new Set(tests.map((t) => t.postId)))
            .map((id) => postById.get(id))
            .filter((p): p is SosPost => !!p)
            .sort((a, b) => {
              const ak = parseFloat(a.kmMarker) || 0
              const bk = parseFloat(b.kmMarker) || 0
              return ak - bk
            })
            .map((p) => {
              const active = selectedPostIds.has(p.id)
              return (
                <button
                  key={p.id}
                  onClick={() => togglePost(p.id)}
                  className={`text-xs px-2 py-0.5 rounded border transition-colors ${
                    active
                      ? "bg-foreground text-background border-foreground"
                      : "bg-background hover:bg-muted border-border text-muted-foreground"
                  }`}
                >
                  {p.name}
                </button>
              )
            })}
        </div>
      )}

      {rows.length === 0 ? (
        <div className="border rounded-md p-6 text-center text-muted-foreground text-sm">
          Sin pruebas en el rango seleccionado.
        </div>
      ) : (
        <TimelineGrid rows={rows} minTs={minTs} maxTs={maxTs} span={span} ticks={ticks} onSelect={onSelect} />
      )}

      {/* Leyenda */}
      <div className="flex gap-4 pt-1 text-xs text-muted-foreground border-t pt-3 flex-wrap">
        {(["PASSED", "FAILED", "ERROR", "PENDING"] as const).map((s) => (
          <span key={s} className="flex items-center gap-1.5">
            <span className={`inline-block size-3 rounded-full ${STATUS_COLOR[s]}`} />
            {STATUS_LABEL[s]}
          </span>
        ))}
        <span className="flex items-center gap-1.5 ml-2">
          <span className="inline-block size-3 rounded-full bg-muted ring-2 ring-blue-400" />
          Entregado por carrier
        </span>
      </div>
    </div>
  )
}

// TimelineGrid renderea las filas del timeline. Densidad inteligente:
// si una fila tiene > umbral tests, los agrupa en buckets visuales.
function TimelineGrid({
  rows,
  minTs,
  maxTs,
  span,
  ticks,
  onSelect,
}: {
  rows: Array<{ postId: string; post: SosPost | undefined; tests: TestResult[] }>
  minTs: number
  maxTs: number
  span: number
  ticks: { ts: number; label: string }[]
  onSelect: (t: TestResult) => void
}) {
  return (
    <div className="border rounded-md p-3 space-y-1 overflow-x-auto">
      {/* Header con eje temporal */}
      <div className="flex items-center gap-3">
        <div className="w-44 shrink-0" />
        <div className="relative flex-1 h-5 border-b">
          {ticks.map((t) => {
            const left = ((t.ts - minTs) / span) * 100
            return (
              <span
                key={t.ts}
                className="absolute text-[10px] text-muted-foreground -translate-x-1/2"
                style={{ left: `${left}%` }}
              >
                {t.label}
              </span>
            )
          })}
        </div>
        <div className="w-24 shrink-0" />
      </div>

      {/* Filas */}
      {rows.map((row) => (
        <TimelineRow
          key={row.postId}
          row={row}
          minTs={minTs}
          maxTs={maxTs}
          span={span}
          ticks={ticks}
          onSelect={onSelect}
        />
      ))}
    </div>
  )
}

function TimelineRow({
  row,
  minTs,
  span,
  ticks,
  onSelect,
}: {
  row: { postId: string; post: SosPost | undefined; tests: TestResult[] }
  minTs: number
  maxTs: number
  span: number
  ticks: { ts: number; label: string }[]
  onSelect: (t: TestResult) => void
}) {
  // Stats inline para la columna derecha
  const passed = row.tests.filter((t) => t.status === "PASSED").length
  const total = row.tests.length
  const ratio = total > 0 ? Math.round((passed / total) * 100) : 0

  // Densidad inteligente: si hay más de un test cada ~10px, agrupar en buckets.
  // Asumimos un track de ~600px en pantallas comunes; cada bucket cubre 10px.
  const APPROX_TRACK_WIDTH = 600
  const BUCKET_PX = 10
  const NUM_BUCKETS = Math.floor(APPROX_TRACK_WIDTH / BUCKET_PX)
  const shouldBucketize = row.tests.length > NUM_BUCKETS

  return (
    <div className="flex items-center gap-3 py-1 border-b last:border-0">
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
      <div className="relative flex-1 h-7 bg-muted/30 rounded">
        {/* Líneas verticales en cada tick para referencia */}
        {ticks.map((t) => {
          const left = ((t.ts - minTs) / span) * 100
          return (
            <span
              key={t.ts}
              className="absolute top-0 bottom-0 border-l border-border/30"
              style={{ left: `${left}%` }}
            />
          )
        })}

        {/* Tests — puntos individuales o buckets agrupados */}
        {shouldBucketize ? (
          <BucketizedRow tests={row.tests} minTs={minTs} span={span} buckets={NUM_BUCKETS} onSelect={onSelect} />
        ) : (
          row.tests.map((t) => {
            const ts = new Date(t.CreatedAt).getTime()
            const left = ((ts - minTs) / span) * 100
            const ringClass = t.deliveryConfirmed === true ? "ring-2 ring-blue-400" : ""
            const tooltip = buildTooltip(t)
            return (
              <button
                key={t.id}
                onClick={() => onSelect(t)}
                style={{ left: `${left}%` }}
                className={`absolute top-1/2 -translate-x-1/2 -translate-y-1/2 size-3 rounded-full ring-offset-1 hover:ring-2 transition-shadow cursor-pointer ${STATUS_COLOR[t.status]} ${ringClass}`}
                title={tooltip}
                aria-label={tooltip}
              />
            )
          })
        )}
      </div>

      {/* Stats columna derecha */}
      <div className="w-24 shrink-0 text-right text-xs">
        <div className="text-muted-foreground">{total} pruebas</div>
        <div className={ratio >= 80 ? "text-green-600" : ratio >= 50 ? "text-yellow-700" : "text-red-600"}>
          {ratio}% OK
        </div>
      </div>
    </div>
  )
}

// BucketizedRow agrupa tests muy densos en barras verticales segmentadas.
// Cada barra cubre un bucket temporal y muestra la mayoría del status.
function BucketizedRow({
  tests,
  minTs,
  span,
  buckets,
  onSelect,
}: {
  tests: TestResult[]
  minTs: number
  span: number
  buckets: number
  onSelect: (t: TestResult) => void
}) {
  const bucketWidth = span / buckets
  const grouped: TestResult[][] = Array.from({ length: buckets }, () => [])
  for (const t of tests) {
    const ts = new Date(t.CreatedAt).getTime()
    const idx = Math.min(buckets - 1, Math.max(0, Math.floor((ts - minTs) / bucketWidth)))
    grouped[idx].push(t)
  }

  return (
    <>
      {grouped.map((bucket, idx) => {
        if (bucket.length === 0) return null
        const left = (idx / buckets) * 100
        const widthPercent = 100 / buckets
        // status mayoritario para el color del bucket
        const counts = { PASSED: 0, FAILED: 0, ERROR: 0, PENDING: 0 } as Record<string, number>
        for (const t of bucket) counts[t.status] = (counts[t.status] ?? 0) + 1
        const dominant = (Object.entries(counts).sort((a, b) => b[1] - a[1])[0][0]) as TestResult["status"]
        const tooltip = buildBucketTooltip(bucket, counts)
        return (
          <button
            key={idx}
            onClick={() => onSelect(bucket[bucket.length - 1])}  // abre el más reciente del bucket
            style={{ left: `${left}%`, width: `${widthPercent}%` }}
            className={`absolute top-1 bottom-1 ${STATUS_COLOR[dominant]} opacity-80 hover:opacity-100 rounded-sm cursor-pointer`}
            title={tooltip}
            aria-label={tooltip}
          >
            {bucket.length > 1 && (
              <span className="absolute inset-0 flex items-center justify-center text-[9px] font-bold text-white">
                {bucket.length}
              </span>
            )}
          </button>
        )
      })}
    </>
  )
}

function buildTooltip(t: TestResult): string {
  const parts = [
    `${t.testType} · ${STATUS_LABEL[t.status]}`,
    formatDate(t.CreatedAt),
  ]
  if (t.failureKind) {
    parts.push(`→ ${FAILURE_KIND_LABEL[t.failureKind] ?? t.failureKind}`)
  }
  if (t.deliveryConfirmed === true) {
    parts.push(`📡 ${t.deliveryCarrier ?? "Entregado"}`)
  } else if (t.deliveryConfirmed === false) {
    parts.push("📡 No entregado")
  }
  return parts.join(" · ")
}

function buildBucketTooltip(bucket: TestResult[], counts: Record<string, number>): string {
  const start = formatDate(bucket[0].CreatedAt)
  const end = formatDate(bucket[bucket.length - 1].CreatedAt)
  const summary = Object.entries(counts)
    .filter(([, c]) => c > 0)
    .map(([s, c]) => `${c} ${STATUS_LABEL[s as TestResult["status"]] ?? s}`)
    .join(", ")
  return `${bucket.length} pruebas (${summary}) · ${start} → ${end}`
}

// Re-export con alias para compat — el caller importa por nombre.
export default TestsTimelineV2

// Type re-export que el caller puede necesitar
export type { TestType }
