import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router-dom"
import { User, Lock, Eye, EyeOff, Loader2, AlertCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card } from "@/components/ui/card"
import { login as apiLogin, isAuthenticated } from "@/api/client"

export default function Login() {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError(null)
    try {
      await apiLogin(username, password)
      if (isAuthenticated()) {
        navigate("/")
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Error de autenticación")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen bg-gradient-to-b from-background via-background to-muted/40 px-4">
      <div className="w-full max-w-md">
        <Card className="p-8 space-y-6 shadow-lg border-border/60">
          <div className="flex flex-col items-center gap-2">
            <img
              src="/logo-gesvial.svg"
              alt="Gesvial"
              className="h-12 w-auto"
            />
            <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              SMS-SOS-GW
            </p>
          </div>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <label
                htmlFor="username"
                className="text-xs font-medium text-foreground/80"
              >
                Usuario
              </label>
              <div className="relative">
                <User className="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
                <Input
                  id="username"
                  className="pl-8"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  required
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <label
                htmlFor="password"
                className="text-xs font-medium text-foreground/80"
              >
                Contraseña
              </label>
              <div className="relative">
                <Lock className="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
                <Input
                  id="password"
                  type={showPassword ? "text" : "password"}
                  className="pl-8 pr-9"
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  required
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="absolute right-0.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? "Ocultar contraseña" : "Mostrar contraseña"}
                  aria-pressed={showPassword}
                >
                  {showPassword ? <EyeOff /> : <Eye />}
                </Button>
              </div>
            </div>
            {error && (
              <div
                role="alert"
                className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-2.5 text-sm text-destructive"
              >
                <AlertCircle className="size-4 mt-0.5 shrink-0" />
                <span>{error}</span>
              </div>
            )}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? (
                <>
                  <Loader2 className="size-4 animate-spin" />
                  Conectando...
                </>
              ) : (
                "Iniciar sesión"
              )}
            </Button>
          </form>
        </Card>
        <p className="mt-6 text-center text-xs text-muted-foreground">
          Gesvial · SMS-SOS-GW
        </p>
      </div>
    </div>
  )
}
