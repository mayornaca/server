import { NavLink, Outlet } from "react-router-dom"
import { Activity, LayoutDashboard, MapPin, FlaskConical, Smartphone, Settings as SettingsIcon, User } from "lucide-react"
import type { LucideIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { logout, getCurrentUser, hasScope } from "@/api/client"

// cloud-gesvial.21.0: Monitor (vista operacional en vivo) es home del panel.
// cloud-gesvial.22.0.5: el sidebar se filtra por scopes del JWT — un usuario
// `monitor` con scopes restringidos no debería ver entradas que sólo sirven
// a un admin, ni siquiera para entrar a una vista vacía. La autoridad sigue
// siendo el server (intercepta acciones con RequireScope), esto es la capa
// UX que esconde lo que el usuario no puede usar.
type NavItem = {
  to: string
  label: string
  Icon: LucideIcon
  // Si true, el item solo se renderiza para usuarios con `schedules:write`
  // (alias práctico de "es admin del sistema, no monitor read-only").
  // Decisión operacional 22.0.6: el monitor sólo necesita Monitor + Postes
  // + Pruebas para su trabajo diario; Resumen/Gateways/Configuración son
  // herramientas de admin.
  adminOnly?: boolean
}

const navItems: NavItem[] = [
  { to: "/",          label: "Monitor de Postes SOS", Icon: Activity },
  { to: "/dashboard", label: "Resumen",               Icon: LayoutDashboard, adminOnly: true },
  { to: "/posts",     label: "Postes SOS",            Icon: MapPin },
  { to: "/tests",     label: "Pruebas",               Icon: FlaskConical },
  { to: "/gateways",  label: "Gateways",              Icon: Smartphone, adminOnly: true },
  { to: "/settings",  label: "Configuración",         Icon: SettingsIcon, adminOnly: true },
]

export default function Layout() {
  const user = getCurrentUser()
  const isAdmin = hasScope("schedules:write")
  const visibleNavItems = navItems.filter((i) => !i.adminOnly || isAdmin)
  // Si Configuración (admin) está oculta, exponer "Mi cuenta" para que el
  // usuario monitor pueda cambiar su contraseña sin acceso al cron config.
  const showAccountLink = !isAdmin

  return (
    <div className="flex min-h-screen">
      <aside className="w-56 border-r bg-sidebar text-sidebar-foreground flex flex-col">
        <div className="p-4">
          {/* cloud-gesvial.22.0.4: logo + título integrados en bloque único.
              Se elimina la repetición "Gesvial" del subtítulo (el logo ya lo
              comunica). Logo a h-9 con shrink-0 para que no se deforme y
              min-w-0 en el texto para que el sidebar no se desborde. */}
          <div className="flex items-center gap-2.5">
            <img
              src="/logo-gesvial.svg"
              alt="Gesvial"
              className="h-9 w-auto shrink-0"
            />
            <div className="leading-tight min-w-0">
              <div className="font-semibold text-sm">SOS Gateway</div>
              <div className="text-[11px] text-muted-foreground">Monitoreo de Postes</div>
            </div>
          </div>
        </div>
        <Separator />
        <nav className="flex-1 p-2 space-y-1">
          {visibleNavItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) =>
                `flex items-center gap-2 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                    : "hover:bg-sidebar-accent/50"
                }`
              }
            >
              <item.Icon className="size-4 shrink-0" />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <Separator />
        <div className="p-3 space-y-2">
          {user && (
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-full bg-primary text-primary-foreground flex items-center justify-center text-xs font-bold">
                {user.slice(0, 2)}
              </div>
              <span className="text-xs font-medium">{user}</span>
            </div>
          )}
          {showAccountLink && (
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                `flex items-center gap-2 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                    : "hover:bg-sidebar-accent/50"
                }`
              }
            >
              <User className="size-4 shrink-0" />
              Mi cuenta
            </NavLink>
          )}
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start text-destructive hover:text-destructive"
            onClick={logout}
          >
            Cerrar sesión
          </Button>
        </div>
      </aside>
      <main className="flex-1 p-6 bg-background overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}
