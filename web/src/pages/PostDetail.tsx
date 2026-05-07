import { useCallback, useEffect, useState } from "react"
import { useParams, Link } from "react-router-dom"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/StatusBadge"
import { TestDetailModal } from "@/components/TestDetailModal"
import { useConfirm } from "@/components/ConfirmDialog"
import { ApiError, hasScope, posts as postsApi, tests as testsApi, devices as devicesApi } from "@/api/client"
import type { SosPost, TestResult, Device, TestType } from "@/api/types"
import { formatDate, humanizeTestError, isOnline, isSupersededByRetry } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"
import { Skeleton, TableRowSkeleton } from "@/components/Skeleton"

const TEST_TYPES: { value: TestType; label: string }[] = [
  { value: "SMS", label: "SMS (*#36#)" },
  { value: "CONNECTIVITY", label: "Conectividad" },
  { value: "AUDIO_MIC", label: "Audio Micrófono" },
  { value: "AUDIO_SPEAKER", label: "Audio Altavoz" },
]

export default function PostDetail() {
  // cloud-gesvial.22.0.6: las acciones admin (desactivar poste, ejecutar
  // prueba, programar regla) se gatean por scope. El monitor read-only ve
  // sólo los datos del poste y su historial.
  const canWritePosts = hasScope("posts:write")
  const canSchedule = hasScope("tests:schedule")
  const canManageSchedules = hasScope("schedules:write")
  const { id } = useParams<{ id: string }>()
  const confirm = useConfirm()
  const [post, setPost] = useState<SosPost | null>(null)
  const [testsList, setTestsList] = useState<TestResult[]>([])
  const [gateways, setGateways] = useState<Device[]>([])
  const [error, setError] = useState<string | null>(null)
  const [selectedTest, setSelectedTest] = useState<TestResult | null>(null)

  // Execute form state
  const [selectedGateway, setSelectedGateway] = useState<string>("")
  const [selectedType, setSelectedType] = useState<TestType>("SMS")
  const [executing, setExecuting] = useState(false)
  const [execMsg, setExecMsg] = useState<string | null>(null)
  const [execErr, setExecErr] = useState<string | null>(null)
  const [pendingConflict, setPendingConflict] = useState<TestResult | null>(null)

  // Disable toggle state
  const [togglingDisabled, setTogglingDisabled] = useState(false)

  const load = useCallback(async () => {
    if (!id) return
    try {
      const [p, t, d] = await Promise.all([
        postsApi.get(id),
        testsApi.list({ postId: id, limit: "20" }),
        devicesApi.list(),
      ])
      setPost(p.data)
      setTestsList(t.data)
      setGateways(d.data)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error cargando poste")
    }
  }, [id])

  useEffect(() => { load() }, [load])

  // cloud-gesvial.19.1: refresh test list + post status when the SSE bridge
  // notifies us. Pre-fix this page was the operator's main "watch a test
  // progress" view but only updated on manual reload.
  usePanelEvents(["test.scheduled", "test.completed", "post.statusChanged"], (ev) => {
    if (ev.type === "post.statusChanged" && ev.resourceId !== id) return
    void load()
  })

  if (error) return <p className="text-destructive">{error}</p>
  // cloud-gesvial.19.2 C4: skeleton estructurado para no dejar el detalle del
  // poste con un texto plano "Cargando..." mientras llegan los 3 fetches.
  if (!post) return (
    <div className="space-y-4">
      <Skeleton className="h-6 w-64" />
      <Skeleton className="h-4 w-32" />
      <div className="border rounded-md">
        <table className="w-full text-sm">
          <tbody>
            {[0, 1, 2, 3, 4].map((i) => <TableRowSkeleton key={i} cols={4} />)}
          </tbody>
        </table>
      </div>
    </div>
  )

  const onlineGateways = gateways.filter((g) => isOnline(g.lastSeen))

  async function handleExecute(force = false) {
    if (!post) return
    setExecuting(true)
    setExecMsg(null)
    setExecErr(null)
    setPendingConflict(null)
    try {
      const { data } = await testsApi.schedule(post.id, selectedType, selectedGateway, force)
      setExecMsg(
        force
          ? `Prueba reintentada (PENDING anterior cancelado). Nuevo ID: ${data.id}.`
          : `Prueba programada. ID: ${data.id}. Esperando ejecución del gateway...`,
      )
      await load()
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        setPendingConflict(e.body.existingResult as TestResult)
        setExecErr("Ya existe una prueba pendiente para este poste y tipo.")
      } else {
        setExecErr(e instanceof Error ? e.message : "Error al programar prueba")
      }
    } finally {
      setExecuting(false)
    }
  }

  async function handleToggleDisabled() {
    if (!post) return
    const nextDisabled = !post.disabled
    const ok = await confirm({
      title: nextDisabled ? "Desactivar poste" : "Reactivar poste",
      message: nextDisabled
        ? "No recibirá pruebas programadas hasta reactivarlo."
        : "Volverá a recibir pruebas programadas.",
      confirmLabel: nextDisabled ? "Desactivar" : "Reactivar",
      destructive: nextDisabled,
    })
    if (!ok) return
    setTogglingDisabled(true)
    try {
      const { data } = await postsApi.patch(post.id, { disabled: nextDisabled })
      setPost(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al cambiar estado del poste")
    } finally {
      setTogglingDisabled(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <Link to="/posts" className="text-sm text-muted-foreground hover:underline">
          &larr; Volver
        </Link>
        <h1 className="text-2xl font-bold">{post.name}</h1>
        <StatusBadge status={post.status} disabled={post.disabled} />
        {canWritePosts && (
          <div className="ml-auto">
            <Button
              variant={post.disabled ? "default" : "outline"}
              size="sm"
              onClick={handleToggleDisabled}
              disabled={togglingDisabled}
            >
              {togglingDisabled
                ? "Actualizando..."
                : post.disabled
                  ? "Reactivar poste"
                  : "Desactivar poste"}
            </Button>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="p-4">
          <p className="text-sm text-muted-foreground">Kilómetro</p>
          <p className="text-xl font-bold">KM {post.kmMarker}</p>
        </Card>
        <Card className="p-4">
          <p className="text-sm text-muted-foreground">Teléfono</p>
          <p className="text-xl font-mono">{post.phoneNumber}</p>
        </Card>
        <Card className="p-4">
          <p className="text-sm text-muted-foreground">Última prueba</p>
          <p className="text-xl">
            {post.lastTestDate ? formatDate(post.lastTestDate) : "Nunca"}
          </p>
        </Card>
      </div>

      {canSchedule && (
      <Card className="p-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-semibold">Ejecutar prueba</h2>
            <p className="text-sm text-muted-foreground">
              El servidor programa la prueba y la envía al gateway seleccionado vía SSE.
            </p>
          </div>
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            {onlineGateways.length} de {gateways.length} en línea
          </span>
        </div>

        {post.disabled && (
          <p className="mt-3 text-sm text-muted-foreground italic">
            Este poste está desactivado. Reactívalo para permitir pruebas programadas.
          </p>
        )}

        <div className="mt-4 flex flex-wrap items-end gap-3">
          <div>
            <label className="text-xs font-medium block mb-1">Gateway</label>
            <select
              value={selectedGateway}
              onChange={(e) => setSelectedGateway(e.target.value)}
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              disabled={executing}
            >
              <option value="">Todos los gateways</option>
              {gateways.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name || g.id} {isOnline(g.lastSeen) ? "" : "(offline)"}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs font-medium block mb-1">Tipo de prueba</label>
            <select
              value={selectedType}
              onChange={(e) => setSelectedType(e.target.value as TestType)}
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
              disabled={executing}
            >
              {TEST_TYPES.map((t) => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>
          <Button
            onClick={() => handleExecute(false)}
            disabled={executing || post.disabled}
          >
            {executing ? "Programando..." : "Ejecutar ahora"}
          </Button>
          {/* cloud-gesvial.19.3 D3: enlace explícito a Configuración para
              que el operador entienda que existen DOS acciones distintas:
              "Ejecutar ahora" (one-shot, inmediato) y "Programar regla
              recurrente" (cron schedule, gesvial.11). 22.0.6: gateado
              por schedules:write para no exponerlo al monitor. */}
          {canManageSchedules && (
            <Link
              to={`/settings?newSchedule=1&postId=${post.id}`}
              className="text-sm text-muted-foreground hover:text-foreground hover:underline whitespace-nowrap"
              title="Crear una regla cron que ejecute esta prueba periódicamente"
            >
              o programar regla recurrente →
            </Link>
          )}
        </div>

        {execMsg && (
          <p className="mt-3 text-sm text-green-600">{execMsg}</p>
        )}
        {execErr && (
          <div className="mt-3 text-sm">
            <p className="text-destructive">{execErr}</p>
            {pendingConflict && (
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <span className="text-muted-foreground">
                  Prueba pendiente ID: <span className="font-mono">{pendingConflict.id}</span>
                </span>
                <button
                  className="text-primary hover:underline"
                  onClick={() => setSelectedTest(pendingConflict)}
                >
                  Ver detalle
                </button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => handleExecute(true)}
                  disabled={executing}
                  title="Marca el PENDING actual como ERROR 'superseded by retry' y crea uno nuevo."
                >
                  Cancelar pendiente y reintentar
                </Button>
              </div>
            )}
          </div>
        )}
      </Card>
      )}

      <div>
        <h2 className="text-lg font-semibold mb-3">Historial de pruebas</h2>
        {testsList.length === 0 ? (
          <p className="text-muted-foreground">Sin pruebas para este poste</p>
        ) : (
          <div className="border rounded-md">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="text-left p-3">Fecha</th>
                  <th className="text-left p-3">Tipo</th>
                  <th className="text-left p-3">Estado</th>
                  <th className="text-left p-3">Error</th>
                </tr>
              </thead>
              <tbody>
                {/* cloud-gesvial.22.1.0: filtrar supersedidos por retry y
                    humanizar el texto técnico del campo `error`. */}
                {testsList.filter((t) => !isSupersededByRetry(t.error)).map((t) => (
                  <tr
                    key={t.id}
                    className="border-b last:border-0 hover:bg-muted/30 cursor-pointer"
                    onClick={() => setSelectedTest(t)}
                  >
                    <td className="p-3">{formatDate(t.CreatedAt)}</td>
                    <td className="p-3">{t.testType}</td>
                    <td className="p-3">
                      <StatusBadge
                        status={t.status}
                        failureKind={t.failureKind}
                        deliveryConfirmed={t.deliveryConfirmed}
                      />
                    </td>
                    <td className="p-3 text-xs text-muted-foreground" title={t.error || ""}>
                      {humanizeTestError(t.error)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <TestDetailModal
        test={selectedTest}
        post={post}
        onClose={() => setSelectedTest(null)}
      />
    </div>
  )
}
