import { useCallback, useEffect, useState } from "react"
import { useSearchParams } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { StatusBadge } from "@/components/StatusBadge"
import { TestDetailModal } from "@/components/TestDetailModal"
import { tests as testsApi, posts as postsApi } from "@/api/client"
import type { TestResult, SosPost } from "@/api/types"
import { formatDate, humanizeTestError, isSupersededByRetry } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"
import { TableRowSkeleton } from "@/components/Skeleton"
import { TestsTimelineV2 } from "@/components/TestsTimelineV2"

// cloud-gesvial.22.0.6: filtros con label localizado. El `value` viaja al
// backend en el query string y debe matchear los enums del server (ASCII
// upper case); el `label` se renderiza en el chip.
const TEST_TYPE_FILTERS: { value: string; label: string }[] = [
  { value: "",              label: "Todos" },
  { value: "SMS",           label: "SMS" },
  { value: "CONNECTIVITY",  label: "Conectividad" },
  { value: "AUDIO_MIC",     label: "Audio (Micrófono)" },
  { value: "AUDIO_SPEAKER", label: "Audio (Parlante)" },
]
const STATUS_FILTERS: { value: string; label: string }[] = [
  { value: "",       label: "Todos" },
  { value: "PASSED", label: "Aprobado" },
  { value: "FAILED", label: "Fallido" },
  { value: "ERROR",  label: "Error" },
]
const PAGE_SIZE = 20

// cloud-gesvial.19.1 F-MED-8: build day boundaries from the user's local
// timezone, then serialise as RFC3339 with explicit offset. The server
// parses it correctly via time.RFC3339 / time.Parse — no UTC drift.
function localStartOfDayISO(yyyymmdd: string): string {
  const [y, m, d] = yyyymmdd.split("-").map(Number)
  return new Date(y, m - 1, d, 0, 0, 0, 0).toISOString()
}
function localEndOfDayISO(yyyymmdd: string): string {
  const [y, m, d] = yyyymmdd.split("-").map(Number)
  return new Date(y, m - 1, d, 23, 59, 59, 999).toISOString()
}

export default function Tests() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [testsList, setTestsList] = useState<TestResult[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [allPosts, setAllPosts] = useState<SosPost[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedTest, setSelectedTest] = useState<TestResult | null>(null)
  // cloud-gesvial.19.3 D6: vista de historial. "table" = paginada (default,
  // útil para auditoría puntual); "timeline" = horizontal por poste, útil
  // para entrega de turno.
  const [view, setView] = useState<"table" | "timeline">("table")

  const typeFilter = searchParams.get("type") || ""
  const statusFilter = searchParams.get("status") || ""
  const postIdFilter = searchParams.get("postId") || ""
  const fromFilter = searchParams.get("from") || ""
  const toFilter = searchParams.get("to") || ""
  const page = parseInt(searchParams.get("page") || "1", 10)

  function setFilter(key: string, value: string) {
    const params = new URLSearchParams(searchParams)
    if (value) params.set(key, value)
    else params.delete(key)
    params.set("page", "1")
    setSearchParams(params)
  }

  useEffect(() => {
    // cloud-gesvial.19.1: cap to 500 posts so the dropdown stays bounded even
    // if a future deploy holds thousands. F-MED-1.
    postsApi
      .list({ limit: "500" })
      .then(({ data }) => setAllPosts(data))
      .catch((e) => {
        // F-MED-2: at least surface to console so an empty dropdown isn't
        // mysterious during dev. We don't bubble to the user — the post
        // filter is non-essential for browsing tests.
        console.warn("[Tests] failed to load posts for filter:", e)
      })
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params: Record<string, string> = {
        limit: String(PAGE_SIZE),
        offset: String((page - 1) * PAGE_SIZE),
      }
      if (typeFilter) params.type = typeFilter
      if (statusFilter) params.status = statusFilter
      if (postIdFilter) params.postId = postIdFilter
      if (fromFilter) params.from = fromFilter
      if (toFilter) params.to = toFilter

      const { data, totalCount: tc } = await testsApi.list(params)
      setTestsList(data)
      setTotalCount(tc || data.length)
    } catch (e) {
      // cloud-gesvial.19.1: pre-fix this throw left the page in "Cargando..."
      // forever with no recovery path on any 5xx / network error.
      setError(e instanceof Error ? e.message : "Error cargando pruebas")
    } finally {
      setLoading(false)
    }
  }, [typeFilter, statusFilter, postIdFilter, fromFilter, toFilter, page])

  useEffect(() => {
    load()
  }, [load])

  // Auto-refresh on panel events: refetch when a test is scheduled or
  // completed (cloud-gesvial.19+). The hook only fires when the EventSource
  // delivers a matching event — no polling.
  usePanelEvents(["test.scheduled", "test.completed"], () => {
    void load()
  })

  const totalPages = Math.ceil(totalCount / PAGE_SIZE)
  const postMap = new Map(allPosts.map((p) => [p.id, p]))

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Pruebas</h1>

      {/* Filters */}
      <div className="space-y-2">
        <div className="flex gap-2 flex-wrap items-center">
          <span className="text-sm font-medium">Tipo:</span>
          {TEST_TYPE_FILTERS.map((t) => (
            <Badge
              key={t.value || "all"}
              variant={typeFilter === t.value ? "default" : "outline"}
              className="cursor-pointer"
              onClick={() => setFilter("type", t.value)}
            >
              {t.label}
            </Badge>
          ))}
        </div>
        <div className="flex gap-2 flex-wrap items-center">
          <span className="text-sm font-medium">Estado:</span>
          {STATUS_FILTERS.map((s) => (
            <Badge
              key={s.value || "all"}
              variant={statusFilter === s.value ? "default" : "outline"}
              className="cursor-pointer"
              onClick={() => setFilter("status", s.value)}
            >
              {s.label}
            </Badge>
          ))}
        </div>
        <div className="flex gap-2 items-center">
          <span className="text-sm font-medium">Poste:</span>
          <select
            className="border rounded px-2 py-1 text-sm bg-background"
            value={postIdFilter}
            onChange={(e) => setFilter("postId", e.target.value)}
          >
            <option value="">Todos los postes</option>
            {allPosts.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} (KM {p.kmMarker})
              </option>
            ))}
          </select>
          <span className="text-sm font-medium ml-4">Desde:</span>
          <Input
            type="date"
            className="w-40"
            value={fromFilter.split("T")[0]}
            max={toFilter.split("T")[0] || undefined}
            onChange={(e) =>
              // cloud-gesvial.19.1 F-MED-8: build the boundary in the user's
              // local timezone instead of forcing UTC. In Chile (UTC-3 / -4)
              // pre-fix `2026-04-28T00:00:00Z` was actually 21:00 of Apr 27
              // local time — tests after midnight local were missing.
              setFilter(
                "from",
                e.target.value ? localStartOfDayISO(e.target.value) : "",
              )
            }
          />
          <span className="text-sm font-medium">Hasta:</span>
          <Input
            type="date"
            className="w-40"
            value={toFilter.split("T")[0]}
            min={fromFilter.split("T")[0] || undefined}
            onChange={(e) =>
              setFilter(
                "to",
                e.target.value ? localEndOfDayISO(e.target.value) : "",
              )
            }
          />
          {(fromFilter || toFilter) && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                // cloud-gesvial.19.1: clear both keys atomically. Two
                // sequential setFilter() calls each captured the same stale
                // searchParams snapshot, so the second one overwrote the
                // first — `from` silently survived.
                const p = new URLSearchParams(searchParams)
                p.delete("from")
                p.delete("to")
                p.set("page", "1")
                setSearchParams(p)
              }}
            >
              Limpiar fechas
            </Button>
          )}
        </div>
      </div>

      {/* Results header */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">{totalCount} resultados</p>
        {/* cloud-gesvial.19.3 D6: toggle de vista. La tabla es default; la
            línea de tiempo es la opción visual para entrega de turno. */}
        <div className="inline-flex rounded-md border bg-background p-0.5">
          <button
            type="button"
            onClick={() => setView("table")}
            className={`px-3 py-1 text-xs rounded ${view === "table" ? "bg-muted font-medium" : "text-muted-foreground"}`}
            aria-pressed={view === "table"}
          >
            Tabla
          </button>
          <button
            type="button"
            onClick={() => setView("timeline")}
            className={`px-3 py-1 text-xs rounded ${view === "timeline" ? "bg-muted font-medium" : "text-muted-foreground"}`}
            aria-pressed={view === "timeline"}
          >
            Línea de tiempo
          </button>
        </div>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}

      {loading ? (
        <div className="border rounded-md">
          <table className="w-full text-sm">
            <tbody>
              {[0, 1, 2, 3, 4, 5, 6, 7].map((i) => <TableRowSkeleton key={i} cols={5} />)}
            </tbody>
          </table>
        </div>
      ) : testsList.length === 0 ? (
        <p className="text-muted-foreground">Sin tests</p>
      ) : view === "timeline" ? (
        <TestsTimelineV2
          posts={allPosts}
          onSelect={setSelectedTest}
          typeFilter={typeFilter}
          statusFilter={statusFilter}
        />
      ) : (
        <div className="border rounded-md">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="text-left p-3">Fecha</th>
                <th className="text-left p-3">Poste</th>
                <th className="text-left p-3">Tipo</th>
                <th className="text-left p-3">Estado</th>
                <th className="text-left p-3">Error</th>
              </tr>
            </thead>
            <tbody>
              {/* cloud-gesvial.22.1.0: filtrar tests "superseded by retry" del
                  historial. Son metadata interna del cron que oscurece el
                  historial real desde la perspectiva del operador. */}
              {testsList.filter((t) => !isSupersededByRetry(t.error)).map((t) => {
                const post = postMap.get(t.postId)
                return (
                  <tr
                    key={t.id}
                    className="border-b last:border-0 hover:bg-muted/30 cursor-pointer"
                    onClick={() => setSelectedTest(t)}
                  >
                    <td className="p-3">{formatDate(t.CreatedAt)}</td>
                    <td className="p-3">{post ? `${post.name} (KM ${post.kmMarker})` : t.postId.slice(0, 12)}</td>
                    <td className="p-3">{t.testType}</td>
                    <td className="p-3">
                      <StatusBadge
                        status={t.status}
                        failureKind={t.failureKind}
                        deliveryConfirmed={t.deliveryConfirmed}
                      />
                    </td>
                    <td className="p-3 text-xs text-muted-foreground max-w-48 truncate" title={t.error || ""}>
                      {humanizeTestError(t.error)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <TestDetailModal
        test={selectedTest}
        post={selectedTest ? postMap.get(selectedTest.postId) || null : null}
        onClose={() => setSelectedTest(null)}
      />

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => {
              const p = new URLSearchParams(searchParams)
              p.set("page", String(page - 1))
              setSearchParams(p)
            }}
          >
            Anterior
          </Button>
          <span className="text-sm text-muted-foreground">
            Página {page} de {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => {
              const p = new URLSearchParams(searchParams)
              p.set("page", String(page + 1))
              setSearchParams(p)
            }}
          >
            Siguiente
          </Button>
        </div>
      )}
    </div>
  )
}
