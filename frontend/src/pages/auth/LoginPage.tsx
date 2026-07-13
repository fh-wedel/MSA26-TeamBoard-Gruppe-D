import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { authApi } from '../../api/auth'
import { api, ApiError, setAccessToken } from '../../api/client'
import { useAuthStore } from '../../stores/authStore'
import type { User } from '../../api/types'

export default function LoginPage() {
  const navigate = useNavigate()
  const { setAuth } = useAuthStore()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await authApi.login(email, password)
      setAccessToken(res.data.access_token)
      const me = await api.get<{ data: User }>('/auth/me')
      setAuth(me.data, res.data.access_token, res.data.refresh_token)
      navigate('/', { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-bg-0 flex items-center justify-center p-4"
      style={{ backgroundImage: 'radial-gradient(ellipse 80% 60% at 50% -10%, rgba(56,189,248,0.10) 0%, transparent 65%)' }}>

      <div className="w-full max-w-sm">
        {/* Logo */}
        <div className="mb-10 text-center">
          <div className="inline-flex items-center gap-2 mb-3">
            <div className="w-8 h-8 bg-accent rounded-sm flex items-center justify-center">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                <rect x="1" y="1" width="6" height="6" fill="#05080F" />
                <rect x="9" y="1" width="6" height="6" fill="#05080F" />
                <rect x="1" y="9" width="6" height="6" fill="#05080F" />
                <rect x="9" y="9" width="6" height="3" fill="#05080F" />
              </svg>
            </div>
            <span className="text-xl font-semibold tracking-tight text-text-0">TeamBoard</span>
          </div>
          <p className="text-text-2 text-sm">Sign in to your workspace</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="card p-6 space-y-4">
            {error && (
              <div className="bg-danger/10 border border-danger/30 text-danger text-sm px-3 py-2 rounded">
                {error}
              </div>
            )}

            <div>
              <label className="label block mb-1.5">Email</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="you@company.com"
                className="input-base w-full"
                required
                autoComplete="email"
                autoFocus
              />
            </div>

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="label">Password</label>
                <Link to="/forgot-password" className="text-xs text-accent hover:text-accent-dim transition-colors">
                  Forgot password?
                </Link>
              </div>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="input-base w-full"
                required
                autoComplete="current-password"
              />
            </div>

            <button type="submit" disabled={loading} className="btn-primary w-full">
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
          </div>

          <p className="text-center text-sm text-text-2">
            No account?{' '}
            <Link to="/register" className="text-accent hover:text-accent-dim transition-colors">
              Create one
            </Link>
          </p>
        </form>
      </div>
    </div>
  )
}
