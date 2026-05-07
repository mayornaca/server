import { useEffect, useState } from "react"

// Toast unificado para feedback transitorio (success/info/error). Reemplaza el
// patrón ad-hoc setError/setBatchMsg que dejaba mensajes pegados en la zona
// del formulario, sin trazabilidad temporal y sin diferenciar entre
// "operación terminó OK" y "operación falló". cloud-gesvial.19.2 C3.
//
// Uso:
//
//   const toast = useToast()
//   toast.success("Lote enviado: 3 creadas")
//   toast.error("No se pudo programar la prueba")
//   toast.info("Sincronizando…")
//
// El componente <ToastHost /> debe montarse UNA vez cerca del root del app
// (junto a <ConfirmHost />). El hook se conecta al host por una variable
// de módulo, idéntico al patrón de ConfirmDialog.

type ToastKind = "success" | "error" | "info"

type Toast = {
  id: number
  kind: ToastKind
  message: string
}

let pushToast: ((kind: ToastKind, message: string) => void) | null = null

let nextId = 1

export function useToast() {
  return {
    success: (message: string) => pushToast?.("success", message),
    error: (message: string) => pushToast?.("error", message),
    info: (message: string) => pushToast?.("info", message),
  }
}

const KIND_STYLE: Record<ToastKind, string> = {
  success: "bg-green-50 border-green-300 text-green-900 dark:bg-green-950/40 dark:border-green-700 dark:text-green-100",
  error: "bg-red-50 border-red-300 text-red-900 dark:bg-red-950/40 dark:border-red-700 dark:text-red-100",
  info: "bg-blue-50 border-blue-300 text-blue-900 dark:bg-blue-950/40 dark:border-blue-700 dark:text-blue-100",
}

const KIND_PREFIX: Record<ToastKind, string> = {
  success: "✓",
  error: "✕",
  info: "i",
}

const AUTODISMISS_MS = 6000

export function ToastHost() {
  const [toasts, setToasts] = useState<Toast[]>([])

  useEffect(() => {
    pushToast = (kind, message) => {
      const id = nextId++
      setToasts((curr) => [...curr, { id, kind, message }])
      // Auto-dismiss tras AUTODISMISS_MS. El cleanup en unmount NO cancela
      // los timers pendientes — son self-removing y el host vive todo el
      // app, así que en práctica nunca se desmonta.
      setTimeout(() => {
        setToasts((curr) => curr.filter((t) => t.id !== id))
      }, AUTODISMISS_MS)
    }
    return () => {
      pushToast = null
    }
  }, [])

  function dismiss(id: number) {
    setToasts((curr) => curr.filter((t) => t.id !== id))
  }

  if (toasts.length === 0) return null

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none">
      {toasts.map((t) => (
        <div
          key={t.id}
          role={t.kind === "error" ? "alert" : "status"}
          className={`pointer-events-auto rounded-md border px-4 py-3 shadow-md text-sm flex items-start gap-3 ${KIND_STYLE[t.kind]}`}
        >
          <span aria-hidden className="font-semibold mt-0.5">{KIND_PREFIX[t.kind]}</span>
          <span className="flex-1">{t.message}</span>
          <button
            type="button"
            onClick={() => dismiss(t.id)}
            aria-label="Cerrar notificación"
            className="text-xs opacity-60 hover:opacity-100 mt-0.5"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  )
}
