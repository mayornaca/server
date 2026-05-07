import { useEffect, useRef, useState, type FormEvent } from "react"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { ScheduleDialog } from "@/components/ScheduleDialog"
import { auth, getCurrentUser, hasScope, logout, schedules as schedulesApi } from "@/api/client"
import type { TestSchedule } from "@/api/types"
import { formatDate } from "@/lib/utils"

export default function Settings() {
  // cloud-gesvial.22.0.5: la sección "Pruebas programadas" requiere
  // schedules:write. Sin scope, esa parte de la página no se renderiza ni
  // se llama a schedules.list() (que devolvería 403 + ruido en consola).
  // Para usuarios `monitor` la página queda como "Mi cuenta" minimal.
  const canManageSchedules = hasScope("schedules:write")

  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [scheds, setScheds] = useState<TestSchedule[]>([])
  const [schedsLoading, setSchedsLoading] = useState(true)
  const [schedsError, setSchedsError] = useState<string | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<TestSchedule | null>(null)
  // F-MED-5: track which schedule is currently being toggled so the
  // operator can't double-click and fire two PATCHes.
  const [togglingId, setTogglingId] = useState<string | null>(null)

  // F-MED-7: clean up the post-password-change logout timer if the user
  // navigates away before it fires.
  const logoutTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => {
    return () => {
      if (logoutTimerRef.current) clearTimeout(logoutTimerRef.current)
    }
  }, [])

  const user = getCurrentUser()

  async function loadSchedules() {
    setSchedsLoading(true)
    setSchedsError(null)
    try {
      const { data } = await schedulesApi.list()
      setScheds(data)
    } catch (e) {
      setSchedsError(e instanceof Error ? e.message : "Error cargando reglas")
    } finally {
      setSchedsLoading(false)
    }
  }

  useEffect(() => {
    if (canManageSchedules) loadSchedules()
    else setSchedsLoading(false)
  }, [canManageSchedules])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setMsg(null)

    if (newPassword !== confirmPassword) {
      setError("Las contraseñas nuevas no coinciden")
      return
    }
    if (newPassword.length < 8) {
      setError("La contraseña debe tener al menos 8 caracteres")
      return
    }

    setSaving(true)
    try {
      await auth.changePassword(currentPassword, newPassword)
      setMsg("Contraseña actualizada. Iniciando sesión nuevamente...")
      // F-MED-7: store the timer so unmount can cancel it.
      logoutTimerRef.current = setTimeout(() => logout(), 2000)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al cambiar contraseña")
    } finally {
      setSaving(false)
    }
  }

  function handleNewSchedule() {
    setEditing(null)
    setDialogOpen(true)
  }

  function handleEditSchedule(s: TestSchedule) {
    setEditing(s)
    setDialogOpen(true)
  }

  async function handleToggleEnabled(s: TestSchedule) {
    if (togglingId) return // F-MED-5: ignore re-entrant clicks
    setTogglingId(s.id)
    try {
      await schedulesApi.update(s.id, {
        name: s.name,
        cronExpression: s.cronExpression,
        testType: s.testType,
        enabled: !s.enabled,
        onlyEnabledPosts: s.onlyEnabledPosts,
        cancelPending: s.cancelPending ?? false,
        filterPostIds: s.filterPostIds ?? null,
        deviceId: s.deviceId ?? null,
      })
      await loadSchedules()
    } catch (e) {
      setSchedsError(e instanceof Error ? e.message : "Error al actualizar regla")
    } finally {
      setTogglingId(null)
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">
        {canManageSchedules ? "Configuración" : "Mi cuenta"}
      </h1>

      {canManageSchedules && (
      <Card className="p-6">
        <div className="flex items-start justify-between gap-4 mb-4">
          <div>
            <h2 className="text-lg font-semibold">Pruebas programadas</h2>
            <p className="text-sm text-muted-foreground">
              Reglas de cron que ejecuta el servidor. Compatibles con expresiones estándar de 5 campos.
            </p>
          </div>
          <Button onClick={handleNewSchedule}>+ Nueva regla</Button>
        </div>

        {schedsError && <p className="text-sm text-destructive mb-3">{schedsError}</p>}

        {schedsLoading ? (
          <p className="text-muted-foreground">Cargando...</p>
        ) : scheds.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            No hay reglas configuradas. Las pruebas se ejecutan solo bajo demanda.
          </p>
        ) : (
          <div className="border rounded-md">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="text-left p-3">Nombre</th>
                  <th className="text-left p-3">Cron</th>
                  <th className="text-left p-3">Tipo</th>
                  <th className="text-left p-3">Estado</th>
                  <th className="text-left p-3">Última ejecución</th>
                  <th className="p-3"></th>
                </tr>
              </thead>
              <tbody>
                {scheds.map((s) => (
                  <tr key={s.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="p-3 font-medium">{s.name}</td>
                    <td className="p-3 font-mono text-xs">{s.cronExpression}</td>
                    <td className="p-3">{s.testType}</td>
                    <td className="p-3">
                      <button
                        onClick={() => handleToggleEnabled(s)}
                        disabled={togglingId !== null}
                        className="cursor-pointer disabled:opacity-50"
                        title="Click para alternar"
                      >
                        <Badge variant={s.enabled ? "default" : "outline"}>
                          {togglingId === s.id
                            ? "..."
                            : s.enabled
                              ? "Habilitada"
                              : "Pausada"}
                        </Badge>
                      </button>
                    </td>
                    <td className="p-3 text-muted-foreground text-xs">
                      {s.lastRunAt ? formatDate(s.lastRunAt) : "Nunca"}
                    </td>
                    <td className="p-3">
                      <button
                        onClick={() => handleEditSchedule(s)}
                        className="text-sm text-primary hover:underline"
                      >
                        Editar
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      )}

      <Card className="p-6 max-w-md">
        <h2 className="text-lg font-semibold mb-1">Cuenta</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Usuario: <span className="font-mono">{user}</span>
        </p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <h3 className="text-sm font-semibold">Cambiar contraseña</h3>

          <div>
            <label className="text-sm font-medium mb-1 block">Contraseña actual</label>
            <Input
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
            />
          </div>
          <div>
            <label className="text-sm font-medium mb-1 block">Contraseña nueva</label>
            <Input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              minLength={8}
              required
            />
            <p className="text-xs text-muted-foreground mt-1">Mínimo 8 caracteres</p>
          </div>
          <div>
            <label className="text-sm font-medium mb-1 block">Confirmar contraseña</label>
            <Input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              minLength={8}
              required
            />
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
          {msg && <p className="text-sm text-green-600">{msg}</p>}

          <Button type="submit" disabled={saving}>
            {saving ? "Guardando..." : "Cambiar contraseña"}
          </Button>
        </form>
      </Card>

      {canManageSchedules && (
        <ScheduleDialog
          open={dialogOpen}
          schedule={editing}
          onClose={() => setDialogOpen(false)}
          onSaved={loadSchedules}
        />
      )}
    </div>
  )
}
