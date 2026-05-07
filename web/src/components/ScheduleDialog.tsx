import { useEffect, useState, type FormEvent } from "react"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useConfirm } from "@/components/ConfirmDialog"
import { schedules as schedulesApi, devices as devicesApi } from "@/api/client"
import type { TestSchedule, TestScheduleInput, TestType, Device } from "@/api/types"

// 5-field cron validation: 5 tokens separated by whitespace.
const CRON_REGEX = /^\s*(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$/

const CRON_PRESETS: { label: string; expr: string }[] = [
  { label: "Cada minuto (testing)", expr: "* * * * *" },
  { label: "Cada 15 minutos", expr: "*/15 * * * *" },
  { label: "Cada hora", expr: "0 * * * *" },
  { label: "Cada 6 horas", expr: "0 */6 * * *" },
  { label: "Diario a las 08:00", expr: "0 8 * * *" },
  { label: "Lunes a viernes 08:00", expr: "0 8 * * 1-5" },
]

const TEST_TYPES: { value: TestType; label: string }[] = [
  { value: "SMS", label: "SMS" },
  { value: "CONNECTIVITY", label: "Conectividad" },
  { value: "AUDIO_MIC", label: "Audio Micrófono" },
  { value: "AUDIO_SPEAKER", label: "Audio Altavoz" },
]

interface Props {
  open: boolean
  schedule: TestSchedule | null
  onClose: () => void
  onSaved: () => void
}

export function ScheduleDialog({ open, schedule, onClose, onSaved }: Props) {
  const isEdit = !!schedule
  const confirm = useConfirm()
  const [form, setForm] = useState<TestScheduleInput>({
    name: "",
    cronExpression: "0 */6 * * *",
    testType: "SMS",
    enabled: true,
    onlyEnabledPosts: true,
    cancelPending: false,
    filterPostIds: null,
    deviceId: null,
  })
  const [gateways, setGateways] = useState<Device[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    if (schedule) {
      setForm({
        name: schedule.name,
        cronExpression: schedule.cronExpression,
        testType: schedule.testType,
        enabled: schedule.enabled,
        onlyEnabledPosts: schedule.onlyEnabledPosts,
        cancelPending: schedule.cancelPending ?? false,
        filterPostIds: schedule.filterPostIds ?? null,
        deviceId: schedule.deviceId ?? null,
      })
    } else {
      setForm({
        name: "",
        cronExpression: "0 */6 * * *",
        testType: "SMS",
        enabled: true,
        onlyEnabledPosts: true,
        cancelPending: false,
        filterPostIds: null,
        deviceId: null,
      })
    }
    setError(null)
    // cloud-gesvial.19.1 F-MED-4: log the error so a permanently empty
    // gateway dropdown isn't a silent mystery. UI behaviour unchanged
    // (empty array is the right fallback — "Todos" still works).
    devicesApi
      .list()
      .then((r) => setGateways(r.data))
      .catch((e) => {
        console.warn("[ScheduleDialog] failed to load gateways:", e)
        setGateways([])
      })
  }, [open, schedule])

  const cronValid = CRON_REGEX.test(form.cronExpression)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!cronValid) {
      setError("Expresión cron inválida (deben ser 5 campos separados por espacio)")
      return
    }
    setSaving(true)
    try {
      if (isEdit && schedule) {
        await schedulesApi.update(schedule.id, form)
      } else {
        await schedulesApi.create(form)
      }
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al guardar")
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    if (!schedule) return
    const ok = await confirm({
      title: "Eliminar regla programada",
      message: `Se eliminará "${schedule.name}". Esta acción no se puede deshacer.`,
      confirmLabel: "Eliminar",
      destructive: true,
    })
    if (!ok) return
    setSaving(true)
    try {
      await schedulesApi.delete(schedule.id)
      onSaved()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error al eliminar")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? "Editar regla programada" : "Nueva regla programada"}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-3">
          <div>
            <label className="text-xs font-medium block mb-1">Nombre</label>
            <Input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              maxLength={64}
              required
              placeholder="Ej: Chequeo cada 6h"
            />
          </div>

          <div>
            <label className="text-xs font-medium block mb-1">Preset</label>
            <select
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm w-full"
              onChange={(e) => e.target.value && setForm({ ...form, cronExpression: e.target.value })}
              value=""
            >
              <option value="">Seleccionar preset...</option>
              {CRON_PRESETS.map((p) => (
                <option key={p.expr} value={p.expr}>{p.label} ({p.expr})</option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-xs font-medium block mb-1">
              Expresión cron (5 campos: min hora día mes semana)
            </label>
            <Input
              value={form.cronExpression}
              onChange={(e) => setForm({ ...form, cronExpression: e.target.value })}
              className="font-mono"
              required
            />
            {!cronValid && form.cronExpression && (
              <p className="text-xs text-destructive mt-1">
                Formato inválido. Debe tener 5 campos.
              </p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium block mb-1">Tipo</label>
              <select
                value={form.testType}
                onChange={(e) => setForm({ ...form, testType: e.target.value as TestType })}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm w-full"
              >
                {TEST_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs font-medium block mb-1">Gateway</label>
              <select
                value={form.deviceId ?? ""}
                onChange={(e) => setForm({ ...form, deviceId: e.target.value || null })}
                className="h-9 rounded-md border border-input bg-transparent px-3 text-sm w-full"
              >
                <option value="">Todos</option>
                {gateways.map((g) => (
                  <option key={g.id} value={g.id}>{g.name || g.id}</option>
                ))}
              </select>
            </div>
          </div>

          <div className="space-y-2 pt-1">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              Habilitada
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.onlyEnabledPosts}
                onChange={(e) => setForm({ ...form, onlyEnabledPosts: e.target.checked })}
              />
              Solo postes activos (excluir desactivados)
            </label>
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.cancelPending}
                onChange={(e) => setForm({ ...form, cancelPending: e.target.checked })}
                className="mt-0.5"
              />
              <span>
                Cancelar pendientes y reintentar
                <span className="block text-xs text-muted-foreground">
                  Si está activo, cuando el cron dispare cancela cualquier prueba PENDING
                  vigente para el mismo poste+tipo (la marca como ERROR "superseded by retry")
                  y crea una nueva. Útil cuando los SMS llegan tarde y querés que el cron
                  siempre corra a tiempo. Default OFF mantiene el dedupe original.
                </span>
              </span>
            </label>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <DialogFooter>
            {isEdit && (
              <Button
                type="button"
                variant="destructive"
                onClick={handleDelete}
                disabled={saving}
              >
                Eliminar
              </Button>
            )}
            <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
              Cancelar
            </Button>
            <Button type="submit" disabled={saving || !cronValid}>
              {saving ? "Guardando..." : isEdit ? "Guardar" : "Crear"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
