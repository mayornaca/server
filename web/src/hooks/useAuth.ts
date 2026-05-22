import { useState, useCallback } from "react"
import { login as apiLogin, logout as apiLogout, isAuthenticated } from "@/api/client"

export function useAuth() {
  const [authenticated, setAuthenticated] = useState(isAuthenticated())
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const login = useCallback(async (username: string, password: string) => {
    setLoading(true)
    setError(null)
    try {
      await apiLogin(username, password)
      setAuthenticated(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error de autenticación")
      setAuthenticated(false)
    } finally {
      setLoading(false)
    }
  }, [])

  const logout = useCallback(() => {
    apiLogout()
    setAuthenticated(false)
  }, [])

  return { authenticated, login, logout, error, loading }
}
