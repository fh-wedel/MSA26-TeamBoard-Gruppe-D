import { useEffect } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { authApi } from './api/auth'
import { setAccessToken } from './api/client'
import { useAuthStore } from './stores/authStore'
import AppShell from './components/layout/AppShell'
import LoginPage from './pages/auth/LoginPage'
import RegisterPage from './pages/auth/RegisterPage'
import DashboardPage from './pages/dashboard/DashboardPage'
import ProjectPage from './pages/project/ProjectPage'
import BoardPage from './pages/board/BoardPage'
import SettingsPage from './pages/settings/SettingsPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { user, ready } = useAuthStore()
  if (!ready) return <div className="flex items-center justify-center h-screen text-text-2 text-sm">Loading…</div>
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  const { setAuth, clearAuth, setReady } = useAuthStore()

  useEffect(() => {
    const rt = localStorage.getItem('refresh_token')
    if (!rt) { setReady(); return }

    authApi.refresh(rt)
      .then(async (res) => {
        setAccessToken(res.data.access_token)
        localStorage.setItem('refresh_token', res.data.refresh_token)
        const me = await authApi.me()
        setAuth(me.data, res.data.access_token, res.data.refresh_token)
      })
      .catch(() => { clearAuth() })
      .finally(() => setReady())
  }, [setAuth, clearAuth, setReady])

  useEffect(() => {
    const handler = () => clearAuth()
    window.addEventListener('auth:logout', handler)
    return () => window.removeEventListener('auth:logout', handler)
  }, [clearAuth])

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login"    element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route element={<RequireAuth><AppShell /></RequireAuth>}>
          <Route index element={<DashboardPage />} />
          <Route path="/projects/:projectId" element={<ProjectPage />} />
          <Route path="/projects/:projectId/boards/:boardId" element={<BoardPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
