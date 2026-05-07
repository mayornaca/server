import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Card } from "@/components/ui/card"
import { StatusBadge } from "@/components/StatusBadge"
import { PostStatusCard } from "@/components/PostStatusCard"
import { PendingCountdown } from "@/components/PendingCountdown"
import { SSEStatusBadge } from "@/components/SSEStatusBadge"
import { TestDetailModal } from "@/components/TestDetailModal"
import { posts as postsApi, tests as testsApi, devices as devicesApi } from "@/api/client"
import type { SosPost, TestResult, Device } from "@/api/types"
import { formatDate, isOnline } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"

// Monitor — vista operacional dedicada para el centro de monitoreo.
// cloud-gesvial.21.0.
//
// Diferencias vs Dashboard:
//   - Auto-refresh continuo (vía SSE + polling fallback de 30s).
//   - KPI vivo con stats de la última hora (no solo totales por status).
//   - Grilla de TODOS los postes con su estado actual (no solo 10 tests recientes).
//   - Sección dedicada a tests PENDING con countdown a TTL.
//   - Lista de tests de la última hora SIN paginar (time-bound).
//
// Pensada para que el operador la deje abierta toda la jornada.

const ONE_HOUR_MS = 60 * 60 * 1000
const POLLING_INTERVAL_MS = 30_000

export default function Monitor() {
  const [postsList, setPostsList] = useState<SosPost[]>([])
  const [recentTests, setRecentTests] = useState<TestResult[]>([])
  const [pendingTests, setPendingTests] = useState<TestResult[]>([])
  const [gateways, setGateways] = useState<Device[]>([])
  const [error, setError] = useState<string | null>(null)
  const [selectedTest, setSelectedTest] = useState<TestResult | null>(null)
  const [lastEventAt, setLastEventAt] = useState<number | null>(null)

  const load = useCallback(async () => {
    try {
      const fromIso = new Date(Date.now() - ONE_HOUR_MS).toISOString()
      const [p, recent, pending, devs] = await Promise.all([
        postsApi.list({ limit: "500" }),
        testsApi.list({ from: fromIso, limit: "500" }),
        testsApi.list({ status: "PENDING", limit: "100" }),
        devicesApi.list(),
      ])
      setPostsList(p.data)
      setRecentTests(recent.data)
      setPendingTests(pending.data)
      setGateways(devs.data)
      setLastEventAt(Date.now())
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error cargando monitoreo")
    }
  }, [])

  // Carga inicial + polling fallback
  useEffect(() => {
    load()
    const id = setInterval(load, POLLING_INTERVAL_MS)
    return () => clearInterval(id)
  }, [load])

  // SSE: cualquier evento relevante → recarga + actualiza el badge "última actualización"
  usePanelEvents(
    ["test.scheduled", "test.completed", "post.statusChanged", "device.connected", "device.disconnected"],
    () => {
      setLastEventAt(Date.now())
      void load()
    },
  )

  // Indexar el último test FINALIZADO (no PENDING) por postId para alimentar las cards
  const lastTestByPost = useMemo(() => {
    const m = new Map<string, TestResult>()
    // recentTests viene ordenado por created_at DESC desde el server (Repository.Select).
    // El primero que aparece para cada postId es el más reciente — pero filtramos:
    //   - PENDING: no representa estado conocido, está corriendo.
    //   - "superseded by retry": cloud-gesvial.22.0 — cuando el cron de retry
    //     marca un test viejo como superseded para crear uno nuevo, ese ERROR
    //     es metadata del retry, NO el estado real del poste. Debe mostrarse
    //     el último test ANTERIOR a ese (el resultado real previo al retry).
    for (const t of recentTests) {
      if (t.status === "PENDING") continue
      if (t.error === "superseded by retry") continue
      if (!m.has(t.postId)) m.set(t.postId, t)
    }
    return m
  }, [recentTests])

  // Stats agregadas de la última hora.
  // cloud-gesvial.22.0.4: dos fixes de coherencia.
  //  1. Filtrar `superseded by retry` igual que la lista "Actividad reciente".
  //     Pre-fix: stats contaba 3 finalised (incluyendo el supersedido) pero la
  //     lista mostraba 2 → counts inconsistentes.
  //  2. byFailureKind ahora sólo cuenta tests con status=FAILED. El test
  //     supersedido (status=ERROR) hereda el `failureKind` del original, así
  //     que sin este filtro el "1 fallidas · 2 poste no responde" — más kinds
  //     que fallidas — se imprimía absurdo.
  const stats = useMemo(() => {
    const finalised = recentTests.filter(
      (t) => t.status !== "PENDING" && t.error !== "superseded by retry",
    )
    const passed = finalised.filter((t) => t.status === "PASSED").length
    const failed = finalised.filter((t) => t.status === "FAILED").length
    const errored = finalised.filter((t) => t.status === "ERROR").length
    const total = finalised.length
    const passedRatio = total > 0 ? Math.round((passed / total) * 100) : 0
    const byFailureKind = {
      POST_NOT_RESPONDING: 0,
      NETWORK_DELIVERY: 0,
      TIMEOUT: 0,
    } as Record<string, number>
    for (const t of finalised) {
      if (
        t.status === "FAILED" &&
        t.failureKind &&
        byFailureKind[t.failureKind] !== undefined
      ) {
        byFailureKind[t.failureKind]++
      }
    }
    return { passed, failed, errored, total, passedRatio, byFailureKind }
  }, [recentTests])

  const gatewaysOnline = gateways.filter((g) => isOnline(g.lastSeen)).length

  // Indexar postes por status para el KPI superior
  const visiblePosts = useMemo(() => postsList.filter((p) => !p.disabled), [postsList])
  const postsByStatus = useMemo(() => {
    const counts = { OK: 0, FAIL: 0, UNKNOWN: 0, TESTING: 0 } as Record<string, number>
    for (const p of visiblePosts) counts[p.status] = (counts[p.status] ?? 0) + 1
    return counts
  }, [visiblePosts])

  const sortedVisiblePosts = useMemo(() => {
    return [...visiblePosts].sort((a, b) => {
      const akm = parseFloat(a.kmMarker) || 0
      const bkm = parseFloat(b.kmMarker) || 0
      if (akm !== bkm) return akm - bkm
      return a.name.localeCompare(b.name)
    })
  }, [visiblePosts])

  // Auto-scroll de la lista de tests de última hora cuando llega uno nuevo
  const recentTestsRef = useRef<HTMLDivElement | null>(null)
  const prevTopId = useRef<string | null>(null)
  useEffect(() => {
    const topId = recentTests[0]?.id ?? null
    if (topId && topId !== prevTopId.current && recentTestsRef.current) {
      recentTestsRef.current.scrollTop = 0
      prevTopId.current = topId
    }
  }, [recentTests])

  const postById = useMemo(() => new Map(postsList.map((p) => [p.id, p])), [postsList])

  if (error) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-bold">Monitoreo en vivo</h1>
        <p className="text-destructive">{error}</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header con título + estado de conexión */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h1 className="text-2xl font-bold">Monitor de Postes SOS</h1>
        <SSEStatusBadge lastEventAt={lastEventAt} />
      </div>

      {/* KPI vivo */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">Postes activos</p>
          <p className="text-3xl font-bold text-green-600">
            {postsByStatus.OK} <span className="text-base text-muted-foreground">/ {visiblePosts.length}</span>
          </p>
        </Card>
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">En falla</p>
          <p className="text-3xl font-bold text-red-600">{postsByStatus.FAIL}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">Pruebas en curso</p>
          <p className="text-3xl font-bold text-yellow-600">{pendingTests.length}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">Gateways en línea</p>
          <p className="text-3xl font-bold">
            {gatewaysOnline}
            <span className="text-base text-muted-foreground"> / {gateways.length}</span>
          </p>
        </Card>
      </div>

      {/* KPI hora */}
      <Card className="p-4">
        <div className="flex flex-wrap gap-x-8 gap-y-2 items-baseline">
          <div>
            <span className="text-xs text-muted-foreground">Última hora · </span>
            <span className="font-semibold">{stats.total} pruebas finalizadas</span>
          </div>
          {stats.total > 0 && (
            <>
              <div>
                <span className="text-green-700 font-medium">{stats.passed} aprobadas</span>
                <span className="text-xs text-muted-foreground"> ({stats.passedRatio}%)</span>
              </div>
              <div>
                <span className="text-red-700 font-medium">{stats.failed} fallidas</span>
                {stats.byFailureKind.POST_NOT_RESPONDING > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {" "}· {stats.byFailureKind.POST_NOT_RESPONDING} poste no responde
                  </span>
                )}
                {stats.byFailureKind.NETWORK_DELIVERY > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {" "}· {stats.byFailureKind.NETWORK_DELIVERY} sin entrega
                  </span>
                )}
                {stats.byFailureKind.TIMEOUT > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {" "}· {stats.byFailureKind.TIMEOUT} sin evidencia
                  </span>
                )}
              </div>
              {stats.errored > 0 && (
                <div className="text-purple-700 font-medium">{stats.errored} con error</div>
              )}
            </>
          )}
          {stats.total === 0 && (
            <span className="text-muted-foreground italic">
              Sin pruebas en la última hora
            </span>
          )}
        </div>
      </Card>

      {/* Estado actual de los Postes SOS — grilla por KM */}
      <div>
        <h2 className="text-lg font-semibold mb-2 flex items-baseline gap-2 flex-wrap">
          <span>Estado de los Postes SOS</span>
          {sortedVisiblePosts.length > 0 && (
            <span className="text-sm font-normal text-muted-foreground">
              {sortedVisiblePosts.length} postes ·{" "}
              <span className="text-green-700">{postsByStatus.OK ?? 0} activos</span> ·{" "}
              <span className="text-red-700">{postsByStatus.FAIL ?? 0} en falla</span>
              {(postsByStatus.UNKNOWN ?? 0) > 0 && (
                <> · <span className="text-yellow-700">{postsByStatus.UNKNOWN} sin info</span></>
              )}
            </span>
          )}
        </h2>
        {sortedVisiblePosts.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            No hay postes activos. Crear postes desde la sección Postes SOS o reactivar los desactivados.
          </p>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-2">
            {sortedVisiblePosts.map((p) => (
              <PostStatusCard key={p.id} post={p} lastTest={lastTestByPost.get(p.id)} />
            ))}
          </div>
        )}
      </div>

      {/* Tests en curso (PENDING) — debajo de la grilla para que el operador
          primero vea el estado consolidado y después los tests vivos. */}
      {pendingTests.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold mb-2">Pruebas en curso ({pendingTests.length})</h2>
          <div className="space-y-1">
            {pendingTests.map((t) => {
              const post = postById.get(t.postId)
              return (
                <Card
                  key={t.id}
                  className="p-3 flex items-center gap-3 flex-wrap cursor-pointer hover:bg-muted/30"
                  onClick={() => setSelectedTest(t)}
                >
                  <span className="font-medium">{post?.name ?? t.postId.slice(0, 12)}</span>
                  <span className="text-xs text-muted-foreground">
                    KM {post?.kmMarker ?? "—"} · {t.testType}
                  </span>
                  <span className="ml-auto">
                    <PendingCountdown startedAt={t.startedAt ?? t.CreatedAt} />
                  </span>
                </Card>
              )
            })}
          </div>
        </div>
      )}

      {/* Actividad reciente — tests finalizados de la última hora, sin paginar.
          cloud-gesvial.22.0: filtra "superseded by retry" para que el listado
          muestre sólo resultados reales, no metadata del cron de retry. */}
      {(() => {
        const finalised = recentTests.filter(
          (t) => t.status !== "PENDING" && t.error !== "superseded by retry",
        )
        return (
          <div>
            <h2 className="text-lg font-semibold mb-2 flex items-baseline gap-2 flex-wrap">
              <span>Actividad reciente</span>
              <span className="text-sm font-normal text-muted-foreground">
                {finalised.length} pruebas en la última hora
              </span>
            </h2>
            {finalised.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                Sin pruebas finalizadas en la última hora.
              </p>
            ) : (
              <div
                ref={recentTestsRef}
                className="border rounded-md max-h-96 overflow-y-auto"
              >
                <table className="w-full text-sm">
                  <thead className="sticky top-0 bg-muted/95 backdrop-blur z-10">
                    <tr className="border-b">
                      <th className="text-left p-2 text-xs">Hora</th>
                      <th className="text-left p-2 text-xs">Poste</th>
                      <th className="text-left p-2 text-xs">Tipo</th>
                      <th className="text-left p-2 text-xs">Resultado</th>
                    </tr>
                  </thead>
                  <tbody>
                    {finalised.map((t) => {
                      const post = postById.get(t.postId)
                      return (
                        <tr
                          key={t.id}
                          className="border-b last:border-0 hover:bg-muted/30 cursor-pointer"
                          onClick={() => setSelectedTest(t)}
                        >
                          <td className="p-2 text-xs text-muted-foreground whitespace-nowrap">
                            {formatDate(t.CreatedAt)}
                          </td>
                          <td className="p-2 whitespace-nowrap">
                            {post ? (
                              <>
                                <span className="font-medium">{post.name}</span>
                                <span className="text-xs text-muted-foreground ml-1">
                                  KM {post.kmMarker}
                                </span>
                              </>
                            ) : (
                              <span className="font-mono text-xs">{t.postId.slice(0, 12)}</span>
                            )}
                          </td>
                          <td className="p-2 text-xs">{t.testType}</td>
                          <td className="p-2">
                            <StatusBadge
                              status={t.status}
                              failureKind={t.failureKind}
                              deliveryConfirmed={t.deliveryConfirmed}
                            />
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )
      })()}

      <TestDetailModal
        test={selectedTest}
        post={selectedTest ? postById.get(selectedTest.postId) || null : null}
        onClose={() => setSelectedTest(null)}
      />
    </div>
  )
}
