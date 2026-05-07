import { Badge } from "@/components/ui/badge"

// cloud-gesvial.22.1.0: estados positivos pasan a `success` (verde) en lugar
// de `default` (negro/primario). El default visual chocaba con la convención
// semántica del resto del panel — "Aprobado/Activo" en negro daba la impresión
// de "neutro" en vez de "OK".
const statusConfig: Record<string, { label: string; variant: "default" | "destructive" | "secondary" | "outline" | "success" }> = {
  OK: { label: "Activo", variant: "success" },
  FAIL: { label: "Falla", variant: "destructive" },
  UNKNOWN: { label: "Desconocido", variant: "secondary" },
  TESTING: { label: "En Prueba", variant: "outline" },
  PASSED: { label: "Aprobado", variant: "success" },
  FAILED: { label: "Fallido", variant: "destructive" },
  ERROR: { label: "Error", variant: "destructive" },
  PENDING: { label: "Pendiente", variant: "outline" },
  DISABLED: { label: "Desactivado", variant: "outline" },
}

// cloud-gesvial.20.4: etiqueta humana del subtipo de FAILED. Mostrarse al
// lado del badge principal cuando un test está FAILED para que el operador
// sepa por qué falló sin abrir el modal.
//
// cloud-gesvial.21.0.1: agregar tooltip informativo con acción sugerida —
// "Sin entrega" y "Sin evidencia" eran ambiguas para operador nuevo.
const failureKindLabel: Record<string, { label: string; hint: string }> = {
  POST_NOT_RESPONDING: {
    label: "Poste no responde",
    hint:  "El SMS llegó al teléfono del módulo pero el firmware no respondió. Escalar a equipo en terreno para revisar el módulo SOS físico.",
  },
  NETWORK_DELIVERY: {
    label: "Sin entrega",
    hint:  "Ni el carrier confirmó entrega ni el poste respondió. Verificar gateway, cobertura y SIM del carrier.",
  },
  TIMEOUT: {
    label: "Sin SMS recibidos",
    hint:  "Ventana de 90 s cerró sin recibir ningún SMS desde el número del poste. Posibles causas: SIM del módulo desconectada, sin cobertura, o el carrier no emitió delivery receipt en este intento. El reintento automático se ejecutará si está habilitado.",
  },
}

export function StatusBadge({
  status,
  disabled,
  failureKind,
  deliveryConfirmed,
}: {
  status: string
  disabled?: boolean
  failureKind?: string | null
  deliveryConfirmed?: boolean | null
}) {
  if (disabled) {
    return (
      <Badge variant="outline" className="text-muted-foreground border-dashed">
        Desactivado
      </Badge>
    )
  }

  // cloud-gesvial.20.5: nunca renderizar un badge sin texto. Si el status
  // viene vacío o desconocido, mostrar "Sin estado" en gris para que el
  // operador sepa que algo no llegó bien — pre-fix se veía un badge gris
  // pequeño sin texto que parecía error visual.
  if (!status) {
    return (
      <Badge variant="outline" className="text-muted-foreground border-dashed">
        Sin estado
      </Badge>
    )
  }

  const config = statusConfig[status] ?? { label: status, variant: "secondary" as const }
  const subEntry = failureKind ? failureKindLabel[failureKind] : null
  const subLabel = subEntry?.label ?? failureKind ?? null
  const subHint = subEntry?.hint ?? ""

  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <Badge variant={config.variant}>{config.label}</Badge>
      {subLabel && (
        <Badge
          variant="outline"
          className="text-xs text-muted-foreground cursor-help"
          title={subHint}
        >
          {subLabel}
        </Badge>
      )}
      {/* cloud-gesvial.21.0.1: 📡 SOLO cuando hay confirmación efectiva
          de entrega. Pre-fix mostrar 📡 con opacity-50 para
          delivery_confirmed=false era visualmente indistinguible del
          icono sólido — el operador veía "Fallido + Sin evidencia + 📡"
          y razonaba "si tiene 📡 algo llegó", lo cual es falso. Ahora:
          delivery_confirmed=true → 📡 visible
          delivery_confirmed=false/null → nada (la ausencia de 📡 +
          la etiqueta "Sin evidencia" comunican el mensaje correcto). */}
      {deliveryConfirmed === true && (
        <span title="Carrier confirmó entrega al teléfono" className="text-xs">📡</span>
      )}
    </span>
  )
}
