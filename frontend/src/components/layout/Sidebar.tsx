import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { FolderOpen, LayoutGrid, LogOut, Plus, Settings } from 'lucide-react'
import { projectsApi } from '../../api/projects'
import { authApi } from '../../api/auth'
import { useAuthStore } from '../../stores/authStore'
import { useUIStore } from '../../stores/uiStore'
import clsx from 'clsx'

export default function Sidebar() {
  const location = useLocation()
  const navigate = useNavigate()
  const { user, clearAuth } = useAuthStore()
  const { openCreateProject } = useUIStore()

  const { data } = useQuery({
    queryKey: ['projects'],
    queryFn: () => projectsApi.list(),
  })

  async function logout() {
    const rt = localStorage.getItem('refresh_token')
    if (rt) { try { await authApi.logout(rt) } catch {} }
    clearAuth()
    navigate('/login')
  }

  const initials = user?.email?.charAt(0).toUpperCase() ?? '?'

  return (
    <aside className="w-56 flex-shrink-0 flex flex-col bg-bg-0 border-r border-border-1">
      {/* Brand */}
      <div className="px-4 py-4 border-b border-border-1">
        <Link to="/" className="flex items-center gap-2">
          <div className="w-6 h-6 bg-accent rounded-sm flex items-center justify-center flex-shrink-0">
            <LayoutGrid size={12} color="#05080F" strokeWidth={2.5} />
          </div>
          <span className="font-semibold text-sm text-text-0 tracking-tight">TeamBoard</span>
        </Link>
      </div>

      {/* Nav */}
      <div className="flex-1 overflow-y-auto py-3 px-2">
        <div className="mb-4">
          <div className="flex items-center justify-between px-2 mb-1">
            <span className="label">Projects</span>
            <button onClick={openCreateProject}
              className="text-text-3 hover:text-accent transition-colors p-0.5 rounded">
              <Plus size={14} />
            </button>
          </div>

          {data?.data.map((project) => {
            const isActive = location.pathname.startsWith(`/projects/${project.id}`)
            return (
              <Link key={project.id} to={`/projects/${project.id}`}
                className={clsx(
                  'flex items-center gap-2 px-2 py-1.5 rounded text-sm transition-colors',
                  isActive
                    ? 'bg-bg-3 text-text-0'
                    : 'text-text-2 hover:bg-bg-2 hover:text-text-1'
                )}>
                <FolderOpen size={14} className={isActive ? 'text-accent' : ''} />
                <span className="truncate">{project.name}</span>
              </Link>
            )
          })}

          {!data?.data.length && (
            <p className="px-2 py-1 text-xs text-text-3">No projects yet</p>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="border-t border-border-1 p-3">
        <div className="flex items-center gap-2 px-1 mb-2">
          <div className="w-6 h-6 rounded-sm bg-bg-3 border border-border-2 flex items-center justify-center text-xs font-medium text-text-1 flex-shrink-0">
            {initials}
          </div>
          <span className="text-xs text-text-2 truncate flex-1">{user?.email}</span>
        </div>
        <button onClick={logout}
          className="flex items-center gap-2 w-full px-2 py-1.5 rounded text-xs text-text-3 hover:text-danger hover:bg-danger/5 transition-colors">
          <LogOut size={13} />
          Sign out
        </button>
        {import.meta.env.VITE_BUILD_TIME && (
          <p className="px-2 pt-2 text-[10px] text-text-3 truncate" title={import.meta.env.VITE_BUILD_SHA}>
            Deployed {new Date(import.meta.env.VITE_BUILD_TIME).toLocaleString()}
            {import.meta.env.VITE_BUILD_SHA && ` · ${import.meta.env.VITE_BUILD_SHA.slice(0, 7)}`}
          </p>
        )}
      </div>
    </aside>
  )
}
