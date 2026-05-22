import { useEffect, useMemo, useState } from "react"
import { Link } from "react-router-dom"
import i18n from "@/i18n"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { StatusBadge } from "@/components/StatusBadge"
import { PostFormDialog } from "@/components/PostFormDialog"
import { useConfirm } from "@/components/ConfirmDialog"
import { useToast } from "@/components/Toast"
import { TableRowSkeleton } from "@/components/Skeleton"
import { hasScope, posts as postsApi, tests as testsApi } from "@/api/client"
import type { SosPost, TestType } from "@/api/types"
import { formatDate } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"

const STATUS_FILTERS = ["", "OK", "FAIL", "UNKNOWN"] as const
const STATUS_OPTIONS: SosPost["status"][] = ["OK", "FAIL", "UNKNOWN", "TESTING"]
const TEST_TYPES: { value: TestType; label: string }[] = [
  { value: "SMS", label: "SMS" },
  { value: "CONNECTIVITY", label: "Conectividad" },
  { value: "AUDIO_MIC", label: "Audio Mic" },
  { value: "AUDIO_SPEAKER", label: "Audio Altavoz" },
]

export default function Posts() {
  // cloud-gesvial.22.0.6: gate write operations behind the corresponding scope.
  // El server las rechazaría con 403 igualmente — esto es UX para que un
  // monitor read-only no vea botones que no puede usar.
  const canWritePosts = hasScope("posts:write")
  const canSchedule = hasScope("tests:schedule")
  const canBulk = canWritePosts || canSchedule
  const confirm = useConfirm()
  const toast = useToast()
  const [postsList, setPostsList] = useState<SosPost[]>([])
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState("")
  const [showDisabled, setShowDisabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingPost, setEditingPost] = useState<SosPost | null>(null)

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [batchTestType, setBatchTestType] = useState<TestType>("SMS")
  const [batchBusy, setBatchBusy] = useState(false)
  const [batchMsg, setBatchMsg] = useState<string | null>(null)

  const [editingStatusId, setEditingStatusId] = useState<string | null>(null)

  async function load() {
    setLoading(true)
    setError(null)
    try {
      const params: Record<string, string> = {}
      if (search) params.search = search
      if (statusFilter) params.status = statusFilter
      // cloud-gesvial.20.1: backend defaults to enabled-only. The toggle now
      // requests disabled posts via `?includeDisabled=true` instead of
      // filtering client-side after fetching everything. This keeps the
      // dataset consistent with the rest of the panel (Dashboard counts and
      // Tests post-selector also see only enabled posts by default).
      if (showDisabled) params.includeDisabled = "true"
      const { data } = await postsApi.list(params)
      setPostsList(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : i18n.t("errors.loading.posts"))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    // cloud-gesvial.19.1: drop the previous selection when filters change.
    // Pre-fix `selectedIds` survived across filter switches, so a batch
    // operation could fire against rows the user no longer saw.
    // cloud-gesvial.20.1: also re-fetch when the disabled toggle flips, since
    // the backend now drives the filter.
    setSelectedIds(new Set())
    setBatchMsg(null)
    load() /* eslint-disable-next-line */
  }, [search, statusFilter, showDisabled])

  // Auto-refresh on panel events that mutate post-level state. cloud-gesvial.19+.
  usePanelEvents(["post.statusChanged", "test.completed"], () => {
    void load()
  })

  const visiblePosts = useMemo(
    () => postsList.filter((p) => showDisabled || !p.disabled),
    [postsList, showDisabled],
  )

  const allSelected =
    visiblePosts.length > 0 && visiblePosts.every((p) => selectedIds.has(p.id))

  function toggleAll() {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(visiblePosts.map((p) => p.id)))
    }
  }

  function toggleOne(id: string) {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelectedIds(next)
  }

  async function handleStatusChange(post: SosPost, status: SosPost["status"]) {
    setEditingStatusId(null)
    if (status === post.status) return
    try {
      await postsApi.patch(post.id, { status })
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al actualizar estado")
    }
  }

  async function handleBatchTest() {
    if (selectedIds.size === 0) return
    // cloud-gesvial.19.2 C2: confirmar antes de enviar el lote. Cuando son
    // 10+ postes el operador a veces clickea sin querer mientras está en
    // medio de una alarma — el confirm da el segundo de pausa que falta.
    const ok = await confirm({
      title: `Programar ${selectedIds.size} prueba${selectedIds.size === 1 ? "" : "s"}`,
      message: `Se enviará una prueba de tipo ${batchTestType} a ${selectedIds.size} poste${selectedIds.size === 1 ? "" : "s"} seleccionado${selectedIds.size === 1 ? "" : "s"}. Cada prueba dispara un SMS al poste real.`,
      confirmLabel: "Enviar",
    })
    if (!ok) return
    setBatchBusy(true)
    setBatchMsg(null)
    try {
      const { data } = await testsApi.scheduleBatch(
        Array.from(selectedIds),
        batchTestType,
      )
      const created = data.results.filter((r) => r.status === "PENDING").length
      const existing = data.results.filter((r) => r.status === "PENDING_EXISTING").length
      const errs = data.results.filter((r) => r.status === "ERROR").length
      const summary = `Lote enviado: ${created} creadas, ${existing} ya pendientes, ${errs} con error.`
      // cloud-gesvial.19.2 C3: el toast garantiza visibilidad incluso si el
      // operador navegó a otra página antes de que termine el batch.
      // setBatchMsg se mantiene para compat con el render legacy debajo.
      if (errs > 0 && created === 0 && existing === 0) {
        toast.error(summary)
      } else {
        toast.success(summary)
      }
      setBatchMsg(summary)
      setSelectedIds(new Set())
      await load()
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Error al programar lote"
      toast.error(msg)
      setError(msg)
    } finally {
      setBatchBusy(false)
    }
  }

  async function handleBatchDisable(disabled: boolean) {
    if (selectedIds.size === 0) return
    // cloud-gesvial.19.2 C2: solo el flujo "desactivar" pide confirmación.
    // Reactivar nunca daña operacionalmente; desactivar puede dejar postes
    // sin monitoreo (los crons los saltean) — más riesgo, más pausa.
    if (disabled) {
      const ok = await confirm({
        title: `Desactivar ${selectedIds.size} poste${selectedIds.size === 1 ? "" : "s"}`,
        message: `Los postes desactivados NO reciben pruebas de los crons ni del operador. Es reversible (botón "Reactivar"), pero mientras estén desactivados quedan fuera del monitoreo.`,
        confirmLabel: "Desactivar",
        destructive: true,
      })
      if (!ok) return
    }
    setBatchBusy(true)
    setBatchMsg(null)
    setError(null)
    // cloud-gesvial.19.1: Promise.allSettled so a partial failure no longer
    // hides successful PATCHes behind a single error toast. Pre-fix the
    // operator saw "Error al cambiar..." with no clue how many succeeded.
    const ids = Array.from(selectedIds)
    const results = await Promise.allSettled(
      ids.map((id) => postsApi.patch(id, { disabled })),
    )
    const ok = results.filter((r) => r.status === "fulfilled").length
    const failed = results.length - ok
    const verb = disabled ? "desactivados" : "reactivados"
    if (failed === 0) {
      const msg = `${ok} postes ${verb}.`
      toast.success(msg)
      setBatchMsg(msg)
    } else if (ok === 0) {
      const msg = `No se pudo cambiar ningún poste (${failed} fallaron).`
      toast.error(msg)
      setError(msg)
    } else {
      const msg = `${ok} ${verb}, ${failed} con error.`
      toast.info(msg)
      setBatchMsg(msg)
    }
    setSelectedIds(new Set())
    try {
      await load()
    } catch {
      // load() ya setea error si falla
    }
    setBatchBusy(false)
  }

  function handleNew() {
    setEditingPost(null)
    setDialogOpen(true)
  }

  function handleEdit(p: SosPost, e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setEditingPost(p)
    setDialogOpen(true)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Postes SOS</h1>
        {canWritePosts && (
          <Button onClick={handleNew}>+ Nuevo Poste</Button>
        )}
      </div>

      <div className="flex gap-2 items-center flex-wrap">
        <Input
          placeholder="Buscar por nombre, km o teléfono..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-sm"
        />
        <div className="flex gap-1">
          {STATUS_FILTERS.map((s) => (
            <Badge
              key={s || "all"}
              variant={statusFilter === s ? "default" : "outline"}
              className="cursor-pointer"
              onClick={() => setStatusFilter(s)}
            >
              {s || "Todos"}
            </Badge>
          ))}
        </div>
        <label className="flex items-center gap-2 text-sm ml-auto">
          <input
            type="checkbox"
            checked={showDisabled}
            onChange={(e) => setShowDisabled(e.target.checked)}
          />
          Mostrar desactivados
        </label>
      </div>

      {canBulk && selectedIds.size > 0 && (
        <div className="flex flex-wrap items-center gap-3 border rounded-md bg-muted/40 p-3">
          <span className="text-sm font-medium">
            {selectedIds.size} seleccionado{selectedIds.size === 1 ? "" : "s"}
          </span>
          {/* cloud-gesvial.22.1.0: cada botón se gatea por su propio scope.
              monitor tiene tests:schedule para pruebas manuales pero no
              posts:write — ve "Probar seleccionados" pero no
              "Desactivar"/"Reactivar". */}
          {canSchedule && (
            <>
              <select
                value={batchTestType}
                onChange={(e) => setBatchTestType(e.target.value as TestType)}
                className="h-8 rounded-md border border-input bg-transparent px-2 text-sm"
                disabled={batchBusy}
              >
                {TEST_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
              <Button size="sm" onClick={handleBatchTest} disabled={batchBusy}>
                {batchBusy ? "Enviando..." : "Probar seleccionados"}
              </Button>
            </>
          )}
          {canWritePosts && (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={() => handleBatchDisable(true)}
                disabled={batchBusy}
              >
                Desactivar
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => handleBatchDisable(false)}
                disabled={batchBusy}
              >
                Reactivar
              </Button>
            </>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setSelectedIds(new Set())}
            disabled={batchBusy}
          >
            Cancelar selección
          </Button>
        </div>
      )}

      {batchMsg && <p className="text-sm text-green-600">{batchMsg}</p>}
      {error && <p className="text-destructive">{error}</p>}

      {loading ? (
        <div className="border rounded-md">
          <table className="w-full text-sm">
            <tbody>
              {[0, 1, 2, 3, 4, 5].map((i) => <TableRowSkeleton key={i} cols={7} />)}
            </tbody>
          </table>
        </div>
      ) : visiblePosts.length === 0 ? (
        <p className="text-muted-foreground">Sin postes</p>
      ) : (
        <div className="border rounded-md">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                {canBulk && (
                  <th className="p-3 w-8">
                    <input
                      type="checkbox"
                      checked={allSelected}
                      onChange={toggleAll}
                      aria-label="Seleccionar todos"
                    />
                  </th>
                )}
                <th className="text-left p-3">Nombre</th>
                <th className="text-left p-3">KM</th>
                <th className="text-left p-3">Teléfono</th>
                <th className="text-left p-3">Estado</th>
                <th className="text-left p-3">Último test</th>
                <th className="p-3"></th>
              </tr>
            </thead>
            <tbody>
              {visiblePosts.map((p) => (
                <tr
                  key={p.id}
                  className={`border-b last:border-0 hover:bg-muted/30 ${p.disabled ? "opacity-60" : ""}`}
                >
                  {canBulk && (
                    <td className="p-3">
                      <input
                        type="checkbox"
                        checked={selectedIds.has(p.id)}
                        onChange={() => toggleOne(p.id)}
                        aria-label={`Seleccionar ${p.name}`}
                      />
                    </td>
                  )}
                  <td className="p-3 font-medium">{p.name}</td>
                  <td className="p-3">{p.kmMarker}</td>
                  <td className="p-3 font-mono text-xs">{p.phoneNumber}</td>
                  <td className="p-3">
                    {canWritePosts && editingStatusId === p.id && !p.disabled ? (
                      <select
                        autoFocus
                        value={p.status}
                        // cloud-gesvial.19.1: only `onChange` — the previous
                        // (defaultValue + onBlur + onChange) combo fired the
                        // PATCH twice on keyboard selection. `onBlur` closes
                        // the editor without persisting; choosing a value
                        // dispatches once.
                        onChange={(e) =>
                          handleStatusChange(p, e.target.value as SosPost["status"])
                        }
                        onBlur={() => setEditingStatusId(null)}
                        className="h-7 rounded-md border border-input bg-transparent px-2 text-xs"
                      >
                        {STATUS_OPTIONS.map((s) => (
                          <option key={s} value={s}>{s}</option>
                        ))}
                      </select>
                    ) : canWritePosts ? (
                      <button
                        onClick={() => !p.disabled && setEditingStatusId(p.id)}
                        className="cursor-pointer"
                        title={p.disabled ? "Reactive el poste para cambiar estado" : "Clic para cambiar estado"}
                      >
                        <StatusBadge status={p.status} disabled={p.disabled} />
                      </button>
                    ) : (
                      <StatusBadge status={p.status} disabled={p.disabled} />
                    )}
                  </td>
                  <td className="p-3 text-muted-foreground text-xs">
                    {p.lastTestDate ? formatDate(p.lastTestDate) : <span className="italic">Sin pruebas aún</span>}
                  </td>
                  <td className="p-3 flex gap-3">
                    <Link
                      to={`/posts/${p.id}`}
                      className="text-sm text-primary hover:underline"
                    >
                      Ver
                    </Link>
                    {canWritePosts && (
                      <button
                        onClick={(e) => handleEdit(p, e)}
                        className="text-sm text-muted-foreground hover:underline"
                      >
                        Editar
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <PostFormDialog
        open={dialogOpen}
        post={editingPost}
        onClose={() => setDialogOpen(false)}
        onSaved={load}
      />
    </div>
  )
}
