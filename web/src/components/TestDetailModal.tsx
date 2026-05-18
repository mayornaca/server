import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { StatusBadge } from "@/components/StatusBadge"
import { formatDate } from "@/lib/utils"
import { Radio, CheckCircle2, XCircle, AlertTriangle, Hourglass } from "lucide-react"
import type { TestResult, SosPost, FailureKind } from "@/api/types"

interface Props {
  test: TestResult | null
  post: SosPost | null
  onClose: () => void
}

// cloud-gesvial.20: action recommendations per failure kind. Operator-facing
// hint that complements the dual badge. Mirrors manual-operador.md §6.3.
const FAILURE_KIND_HINT: Record<FailureKind, { label: string; action: string }> = {
  POST_NOT_RESPONDING: {
    label: "Poste no responde",
    action: "El SMS llegó al destino pero el módulo no contestó. Escalar a equipo de campo para revisar el módulo SOS.",
  },
  NETWORK_DELIVERY: {
    label: "Falla de transporte",
    action: "El SMS no llegó al destino. Revisar gateway / cobertura del carrier / portabilidad de la SIM.",
  },
  TIMEOUT: {
    label: "Sin evidencia (timeout)",
    action: "La ventana de espera cerró sin recibir ningún SMS. El reintento automático se ejecutará si está habilitado.",
  },
}

export function TestDetailModal({ test, post, onClose }: Props) {
  if (!test) return null

  let parsedDetails: Record<string, any> = {}
  if (test.details) {
    try {
      parsedDetails = JSON.parse(test.details)
    } catch {
      parsedDetails = { raw: test.details }
    }
  }

  const parsed = parsedDetails.parsed || {}
  const hasParsedData = Object.keys(parsed).length > 0

  // cloud-gesvial.20: dual badge state. `delivery_confirmed === undefined/null`
  // → legacy row, render "—". Otherwise show one badge per layer.
  const hasClassification =
    test.deliveryConfirmed != null || !!test.deliveryCarrier || !!test.failureKind
  const failureHint = test.failureKind ? FAILURE_KIND_HINT[test.failureKind] : null

  return (
    <Dialog open={!!test} onOpenChange={(open) => !open && onClose()}>
      {/*
        sm:max-w-3xl: el dialog base aplica `sm:max-w-sm` (384px) por default.
        Tailwind genera las clases responsive después de las base, por lo que
        un `max-w-3xl` sin prefijo NO override en pantallas >= 640px. Hay que
        usar la variante `sm:` con la misma especificidad para ganarle al default.
      */}
      <DialogContent className="w-[95vw] sm:max-w-6xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Detalle de Prueba</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <p className="text-xs text-muted-foreground">Tipo</p>
              <p className="font-medium">{test.testType}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Estado</p>
              <StatusBadge status={test.status} />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Poste</p>
              <p className="font-medium">
                {post ? `${post.name} (KM ${post.kmMarker})` : test.postId}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Teléfono</p>
              <p className="font-mono text-xs">{post?.phoneNumber || "—"}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Gateway</p>
              <p className="font-mono text-xs">{test.deviceId || "Desde servidor"}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Creado</p>
              <p>{formatDate(test.CreatedAt)}</p>
            </div>
            {test.startedAt && (
              <div>
                <p className="text-xs text-muted-foreground">Iniciado</p>
                <p>{formatDate(test.startedAt)}</p>
              </div>
            )}
            {test.completedAt && (
              <div>
                <p className="text-xs text-muted-foreground">Completado</p>
                <p>{formatDate(test.completedAt)}</p>
              </div>
            )}
          </div>

          {/* cloud-gesvial.20: dual-evidence layer breakdown. Two columns —
              transport (carrier delivery) + application (post response) —
              so the operator sees BOTH states for the same logical test
              instead of a single confused status. */}
          {hasClassification && (
            <div className="border-t pt-3">
              <p className="text-xs text-muted-foreground mb-2 font-semibold">
                Evidencia de la prueba
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {/* Transport layer — carrier delivery receipt */}
                <div className="rounded border p-3 bg-muted/20">
                  <p className="text-xs text-muted-foreground mb-1">
                    Transporte (carrier)
                  </p>
                  {test.deliveryConfirmed === true ? (
                    <>
                      <p className="font-medium flex items-center gap-1.5">
                        <Radio className="w-4 h-4" aria-hidden />
                        Entregado{test.deliveryCarrier ? ` por ${test.deliveryCarrier}` : ""}
                      </p>
                      {test.deliveryAt && (
                        <p className="text-xs text-muted-foreground mt-1">
                          {formatDate(test.deliveryAt)}
                        </p>
                      )}
                    </>
                  ) : test.deliveryConfirmed === false ? (
                    <p className="text-destructive font-medium flex items-center gap-1.5">
                      <Radio className="w-4 h-4" aria-hidden />
                      No llegó al destino
                    </p>
                  ) : (
                    <p className="text-muted-foreground italic flex items-center gap-1.5">
                      <Radio className="w-4 h-4" aria-hidden />
                      Sin información de transporte
                    </p>
                  )}
                </div>

                {/* Application layer — post response */}
                <div className="rounded border p-3 bg-muted/20">
                  <p className="text-xs text-muted-foreground mb-1">
                    Aplicación (poste)
                  </p>
                  {test.status === "PASSED" ? (
                    <p className="font-medium flex items-center gap-1.5">
                      <CheckCircle2 className="w-4 h-4 text-green-700" aria-hidden />
                      El poste respondió
                    </p>
                  ) : test.status === "FAILED" ? (
                    <p className="text-destructive font-medium flex items-center gap-1.5">
                      <XCircle className="w-4 h-4" aria-hidden />
                      {failureHint?.label ?? "Sin respuesta del poste"}
                    </p>
                  ) : test.status === "ERROR" ? (
                    <p className="text-destructive font-medium flex items-center gap-1.5">
                      <AlertTriangle className="w-4 h-4" aria-hidden /> Error
                    </p>
                  ) : (
                    <p className="text-muted-foreground italic flex items-center gap-1.5">
                      <Hourglass className="w-4 h-4" aria-hidden /> Pendiente
                    </p>
                  )}
                </div>
              </div>
              {failureHint && (
                <p className="text-xs text-muted-foreground mt-2">
                  <span className="font-semibold">Acción sugerida:</span> {failureHint.action}
                </p>
              )}
            </div>
          )}

          {hasParsedData && (
            <div className="border-t pt-3">
              <p className="text-xs text-muted-foreground mb-2 font-semibold">
                Respuesta del Poste
              </p>
              <div className="grid grid-cols-2 gap-3">
                {parsed.firmware && (
                  <div>
                    <p className="text-xs text-muted-foreground">Firmware</p>
                    <p className="font-mono text-xs">{parsed.firmware}</p>
                  </div>
                )}
                {parsed.batteryVoltage != null && (
                  <div>
                    <p className="text-xs text-muted-foreground">Batería</p>
                    <p className="font-medium">{parsed.batteryVoltage}V</p>
                  </div>
                )}
                {parsed.signalLevel != null && (
                  <div>
                    <p className="text-xs text-muted-foreground">Nivel de Señal</p>
                    <p className="font-medium">{parsed.signalLevel}</p>
                  </div>
                )}
                {parsed.network && (
                  <div>
                    <p className="text-xs text-muted-foreground">Red</p>
                    <p>{parsed.network}</p>
                  </div>
                )}
                {parsed.networkMode && (
                  <div>
                    <p className="text-xs text-muted-foreground">Modo</p>
                    <p>{parsed.networkMode}</p>
                  </div>
                )}
                {parsed.time && (
                  <div>
                    <p className="text-xs text-muted-foreground">Hora del Poste</p>
                    <p>{parsed.time}</p>
                  </div>
                )}
              </div>
            </div>
          )}

          {parsedDetails.responseText && (
            <div className="border-t pt-3">
              <p className="text-xs text-muted-foreground mb-1 font-semibold">
                SMS Recibido
              </p>
              <pre className="text-xs bg-muted p-2 rounded whitespace-pre-wrap font-mono">
                {parsedDetails.responseText}
              </pre>
            </div>
          )}

          {test.error && (
            <div className="border-t pt-3">
              <p className="text-xs text-muted-foreground mb-1 font-semibold text-destructive">
                Error
              </p>
              <p className="text-destructive text-xs">{test.error}</p>
            </div>
          )}

          <details className="border-t pt-3">
            <summary className="text-xs text-muted-foreground cursor-pointer">
              JSON completo
            </summary>
            <pre className="text-xs bg-muted p-2 rounded mt-2 overflow-x-auto">
              {JSON.stringify({ ...test, parsedDetails }, null, 2)}
            </pre>
          </details>
        </div>
      </DialogContent>
    </Dialog>
  )
}
