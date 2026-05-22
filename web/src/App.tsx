import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom"
import { isAuthenticated } from "@/api/client"
import Layout from "@/components/Layout"
import { ConfirmHost } from "@/components/ConfirmDialog"
import { ToastHost } from "@/components/Toast"
import Login from "@/pages/Login"
import Monitor from "@/pages/Monitor"
import Dashboard from "@/pages/Dashboard"
import Posts from "@/pages/Posts"
import PostDetail from "@/pages/PostDetail"
import Tests from "@/pages/Tests"
import Gateways from "@/pages/Gateways"
import Settings from "@/pages/Settings"

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  if (!isAuthenticated()) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <ProtectedRoute>
              <Layout />
            </ProtectedRoute>
          }
        >
          {/* cloud-gesvial.21.0: Monitor es la nueva home — vista operacional
              en vivo. Dashboard queda accesible vía /dashboard como "Resumen". */}
          <Route path="/" element={<Monitor />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/posts" element={<Posts />} />
          <Route path="/posts/:id" element={<PostDetail />} />
          <Route path="/tests" element={<Tests />} />
          <Route path="/gateways" element={<Gateways />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      <ConfirmHost />
      <ToastHost />
    </BrowserRouter>
  )
}
