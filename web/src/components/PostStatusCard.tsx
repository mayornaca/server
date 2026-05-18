import { Link } from "react-router-dom"
import { CheckCircle2, XCircle, Timer, Radio } from "lucide-react"
import type { ReactNode } from "react"
import type { SosPost, TestResult } from "@/api/types"
import { formatDate } from "@/lib/utils"

// PostStatusCard — mini-tarjeta para la grilla "Estado actual de la flota"
// en la pantalla de Monitoreo. Densa, una por poste, color del status como
// fondo dominante para que el operador vea de un vistazo el estado del campo.
//
// cloud-gesvial.21.0.

type Props = {
  post: SosPost
  lastTest?: TestResult | null
}

const statusBg: Record<string, string> = {
  OK:      "bg-green-50 border-green-300 hover:bg-green-100",
  FAIL:    "bg-red-50 border-red-300 hover:bg-red-100",
  TESTING: "bg-yellow-50 border-yellow-300 hover:bg-yellow-100",
  UNKNOWN: "bg-muted/30 border-border hover:bg-muted/50",
}

const statusLabel: Record<string, string> = {
  OK:      "Activo",
  FAIL:    "Falla",
  TESTING: "En prueba",
  UNKNOWN: "Sin info",
}

// Iconos lucide-react reemplazan los emojis previos (checkmark, X, timer) — Fase 6 plan QA.
const statusIcon: Record<string, ReactNode> = {
  OK:      <CheckCircle2 className="w-4 h-4 text-green-700" aria-hidden />,
  FAIL:    <XCircle      className="w-4 h-4 text-red-700"   aria-hidden />,
  TESTING: <Timer        className="w-4 h-4 text-yellow-700" aria-hidden />,
  UNKNOWN: <span aria-hidden>—</span>,
}

const failureKindLabel: Record<string, string> = {
  POST_NOT_RESPONDING: "Poste no responde",
  NETWORK_DELIVERY:    "Sin entrega",
  TIMEOUT:             "Sin SMS recibidos",
}

export function PostStatusCard({ post, lastTest }: Props) {
  if (post.disabled) {
    return (
      <Link
        to={`/posts/${post.id}`}
        className="block border rounded-md p-2.5 bg-muted/30 border-dashed text-muted-foreground hover:bg-muted/50 transition-colors"
      >
        <div className="font-medium text-sm">{post.name}</div>
        <div className="text-xs">KM {post.kmMarker}</div>
        <div className="text-xs italic mt-1">Desactivado</div>
      </Link>
    )
  }

  const bg = statusBg[post.status] ?? statusBg.UNKNOWN
  const label = statusLabel[post.status] ?? post.status
  const icon: ReactNode = statusIcon[post.status] ?? <span aria-hidden>—</span>

  // Subtítulo: razón del fail o carrier de delivery
  let subtitle: React.ReactNode = null
  if (lastTest) {
    if (lastTest.status === "FAILED" && lastTest.failureKind) {
      subtitle = (
        <div className="text-xs text-red-700 mt-0.5 truncate">
          {failureKindLabel[lastTest.failureKind] ?? lastTest.failureKind}
        </div>
      )
    } else if (lastTest.status === "PASSED" && lastTest.deliveryCarrier) {
      subtitle = (
        <div className="text-xs text-green-700 mt-0.5 truncate flex items-center gap-1">
          <Radio className="w-3 h-3" aria-hidden /> {lastTest.deliveryCarrier}
        </div>
      )
    } else if (lastTest.status === "PASSED" && lastTest.deliveryConfirmed === true) {
      subtitle = (
        <div className="text-xs text-green-700 mt-0.5 truncate flex items-center gap-1">
          <Radio className="w-3 h-3" aria-hidden /> Entregado
        </div>
      )
    } else if (lastTest.status === "ERROR") {
      // cloud-gesvial.22.0.4: no exponer el `error` técnico en la grilla. Texto
      // como "timeout sin respuesta del gateway" o stack-traces son metadata
      // del backend, no info útil para el operador en una mini-tarjeta. El
      // detalle completo vive en el modal del test.
      subtitle = (
        <div className="text-xs text-purple-700 mt-0.5">
          Error en la prueba
        </div>
      )
    }
  }

  return (
    <Link
      to={`/posts/${post.id}`}
      className={`block border rounded-md p-2.5 transition-colors ${bg}`}
      title={post.name}
    >
      <div className="flex items-baseline justify-between gap-1">
        <span className="font-semibold text-sm truncate">{post.name}</span>
        <span className="text-xs text-muted-foreground whitespace-nowrap">KM {post.kmMarker}</span>
      </div>
      <div className="mt-1 text-sm font-medium flex items-center gap-1">
        <span>{icon}</span>
        <span>{label}</span>
      </div>
      {subtitle}
      <div className="text-xs text-muted-foreground mt-1">
        {post.lastTestDate ? formatDate(post.lastTestDate) : "Nunca probado"}
      </div>
    </Link>
  )
}
