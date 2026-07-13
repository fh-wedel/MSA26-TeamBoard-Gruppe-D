import { useLocation, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import { projectsApi, boardsApi } from '../../api/projects'
import NotificationBell from '../notifications/NotificationBell'

export default function TopBar() {
  const location = useLocation()
  const { projectId, boardId } = useParams()

  const { data: project } = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => projectsApi.get(projectId!),
    enabled: !!projectId,
  })

  const { data: board } = useQuery({
    queryKey: ['board', boardId],
    queryFn: () => boardsApi.get(boardId!),
    enabled: !!boardId,
  })

  const crumbs: { label: string; href?: string }[] = []

  if (project) {
    crumbs.push({ label: project.data.name, href: `/projects/${projectId}` })
    if (board) crumbs.push({ label: board.data.name })
  } else if (location.pathname === '/') {
    crumbs.push({ label: 'Dashboard' })
  }

  return (
    <header className="h-12 flex items-center justify-between px-5 border-b border-border-1 bg-bg-0/50 backdrop-blur-sm flex-shrink-0">
      <nav className="flex items-center gap-1 text-sm">
        {crumbs.map((crumb, i) => (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <ChevronRight size={12} className="text-text-3" />}
            {crumb.href
              ? <a href={crumb.href} className="text-text-2 hover:text-text-0 transition-colors">{crumb.label}</a>
              : <span className="text-text-0 font-medium">{crumb.label}</span>
            }
          </span>
        ))}
      </nav>

      <div className="flex items-center gap-2">
        <NotificationBell />
      </div>
    </header>
  )
}
