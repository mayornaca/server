import type { HTMLAttributes } from "react"

// Reemplaza el "Cargando..." plano que dejaba al operador sin saber si la
// página se estaba demorando porque el server tarda o porque está colgado.
// cloud-gesvial.19.2 C4. La animación pulse es de tailwind/shadcn ya
// configurada en `tailwind.config.js`.

export function Skeleton({ className = "", ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      role="status"
      aria-label="Cargando contenido"
      className={`animate-pulse rounded-md bg-muted ${className}`}
      {...props}
    />
  )
}

// CardSkeleton — usado por Dashboard cards (4 cuadrados resumen).
export function CardSkeleton() {
  return (
    <div className="rounded-md border p-4 space-y-2">
      <Skeleton className="h-4 w-24" />
      <Skeleton className="h-8 w-16" />
    </div>
  )
}

// TableRowSkeleton — usado por Posts/Tests/Dashboard listas.
export function TableRowSkeleton({ cols = 6 }: { cols?: number }) {
  return (
    <tr className="border-b last:border-0">
      {Array.from({ length: cols }).map((_, i) => (
        <td key={i} className="p-3">
          <Skeleton className="h-4 w-full max-w-[160px]" />
        </td>
      ))}
    </tr>
  )
}
