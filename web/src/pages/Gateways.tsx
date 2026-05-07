import { useCallback, useEffect, useState } from "react"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { devices as devicesApi } from "@/api/client"
import type { Device } from "@/api/types"
import { timeAgo, isOnline } from "@/lib/utils"
import { usePanelEvents } from "@/hooks/usePanelEvents"

// cloud-gesvial.19.3 D7: el operador ya no ve solo "samsung/q5qxxx" + un ID
// hash. La tarjeta ahora expone:
//   - Número de teléfono de la SIM (línea desde la que el Z5 envía SMS).
//   - Modelo y versión de Android.
//   - Canal por el que el cloud lo notifica (FCM vs SSE) — distingue el caso
//     "el Z5 está conectado por SSE y no aparece en la consola Firebase" del
//     caso real "el cloud no logró contactarlo".
//   - ID corto colapsado (clic para mostrar entero), porque no es lectura
//     humana pero soporte lo pide eventualmente.

function shortId(id: string): string {
  if (id.length <= 8) return id
  return id.slice(0, 4) + "…" + id.slice(-3)
}

function formatPhone(raw: string | null | undefined): string {
  if (!raw) return "Número no reportado"
  // E.164: +56 9 1234 5678 → mostrar agrupado
  const trimmed = raw.replace(/\s/g, "")
  if (trimmed.startsWith("+56") && trimmed.length === 12) {
    return `${trimmed.slice(0, 3)} ${trimmed.slice(3, 4)} ${trimmed.slice(4, 8)} ${trimmed.slice(8)}`
  }
  return trimmed
}

export default function Gateways() {
  const [devicesList, setDevicesList] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const { data } = await devicesApi.list()
      setDevicesList(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error cargando gateways")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  usePanelEvents(["device.connected", "device.disconnected"], () => {
    void load()
  })

  if (error) return <p className="text-destructive">{error}</p>
  if (loading) return <p className="text-muted-foreground">Cargando...</p>

  const online = devicesList.filter((d) => isOnline(d.lastSeen)).length

  return (
    <div className="space-y-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-2xl font-bold">Gateways SMS</h1>
        <p className="text-sm text-muted-foreground">
          {devicesList.length} registrados · {online} en línea
        </p>
      </div>

      <div className="grid gap-3">
        {devicesList.map((d) => {
          const onLine = isOnline(d.lastSeen)
          const expanded = expandedId === d.id
          return (
            <Card key={d.id} className="p-4">
              <div className="flex items-start justify-between gap-4">
                {/* Identidad humana del gateway */}
                <div className="flex-1 min-w-0 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-base">
                      {d.name || d.model || "Gateway sin nombre"}
                    </span>
                    {d.model && d.name && d.name !== d.model && (
                      <span className="text-xs text-muted-foreground">
                        · {d.model}
                      </span>
                    )}
                    {d.osVersion && (
                      <span className="text-xs text-muted-foreground">
                        · Android {d.osVersion}
                      </span>
                    )}
                  </div>
                  <div className="text-sm font-mono">
                    {d.phoneNumber ? (
                      <span className="text-foreground">
                        📱 {formatPhone(d.phoneNumber)}
                      </span>
                    ) : (
                      <span className="text-muted-foreground italic">
                        Número de SIM no reportado por el gateway
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground">
                    <button
                      type="button"
                      onClick={() => setExpandedId(expanded ? null : d.id)}
                      className="font-mono hover:underline"
                      title="Clic para mostrar el ID completo del gateway"
                    >
                      ID: {expanded ? d.id : shortId(d.id)}
                    </button>
                    <span aria-hidden>·</span>
                    <span title="Canal que usa el cloud para notificar a este gateway">
                      Canal:{" "}
                      <span className="font-medium">
                        {d.hasPushToken ? "FCM" : "SSE"}
                      </span>
                    </span>
                  </div>
                </div>

                {/* Estado y última actividad */}
                <div className="text-right shrink-0">
                  <Badge variant={onLine ? "success" : "secondary"}>
                    {onLine ? "En línea" : "Desconectado"}
                  </Badge>
                  <p className="text-xs text-muted-foreground mt-1">
                    último contacto: {timeAgo(d.lastSeen)}
                  </p>
                </div>
              </div>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
