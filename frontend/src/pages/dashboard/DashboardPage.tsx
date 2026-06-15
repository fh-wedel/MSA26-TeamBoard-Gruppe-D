import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { FolderPlus, LayoutGrid, Users, Clock, ChevronRight } from 'lucide-react'
import { projectsApi } from '../../api/projects'
import { useUIStore } from '../../stores/uiStore'
import { formatDistanceToNow } from 'date-fns'

function CreateProjectModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const qc = useQueryClient()

  const create = useMutation({
    mutationFn: () => projectsApi.create(name, description),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['projects'] }); onClose() },
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg-0/80 backdrop-blur-sm animate-fade-in">
      <div className="card w-full max-w-md p-6 animate-scale-in">
        <h2 className="text-base font-semibold text-text-0 mb-5">New Project</h2>
        <form onSubmit={(e) => { e.preventDefault(); create.mutate() }} className="space-y-4">
          <div>
            <label className="label block mb-1.5">Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)}
              className="input-base w-full" placeholder="My Project" required autoFocus />
          </div>
          <div>
            <label className="label block mb-1.5">Description</label>
            <textarea value={description} onChange={(e) => setDescription(e.target.value)}
              className="input-base w-full resize-none h-20" placeholder="What is this project about?" />
          </div>
          <div className="flex gap-2 justify-end pt-2">
            <button type="button" onClick={onClose} className="btn-ghost">Cancel</button>
            <button type="submit" disabled={create.isPending || !name.trim()} className="btn-primary">
              {create.isPending ? 'Creating…' : 'Create project'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default function DashboardPage() {
  const { createProjectModal, openCreateProject, closeCreateProject } = useUIStore()

  const { data, isLoading } = useQuery({
    queryKey: ['projects'],
    queryFn: () => projectsApi.list(),
  })

  const projects = data?.data ?? []

  return (
    <div className="p-6 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-lg font-semibold text-text-0">Projects</h1>
          <p className="text-sm text-text-2 mt-0.5">
            {projects.length} project{projects.length !== 1 ? 's' : ''}
          </p>
        </div>
        <button onClick={openCreateProject} className="btn-primary flex items-center gap-2">
          <FolderPlus size={15} />
          New project
        </button>
      </div>

      {/* Grid */}
      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="card h-36 animate-pulse bg-bg-2" />
          ))}
        </div>
      )}

      {!isLoading && projects.length === 0 && (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="w-16 h-16 bg-bg-2 rounded border border-border-1 flex items-center justify-center mb-4">
            <LayoutGrid size={24} className="text-text-3" />
          </div>
          <h3 className="text-sm font-medium text-text-1 mb-1">No projects yet</h3>
          <p className="text-sm text-text-3 mb-5">Create your first project to get started</p>
          <button onClick={openCreateProject} className="btn-primary flex items-center gap-2">
            <FolderPlus size={15} /> New project
          </button>
        </div>
      )}

      {!isLoading && projects.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {projects.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`}
              className="card p-5 hover:border-border-3 hover:bg-bg-3 transition-all duration-150 group">
              <div className="flex items-start justify-between mb-3">
                <div className="w-8 h-8 bg-accent/10 border border-accent/20 rounded flex items-center justify-center">
                  <LayoutGrid size={14} className="text-accent" />
                </div>
                <ChevronRight size={14} className="text-text-3 group-hover:text-text-1 transition-colors mt-1" />
              </div>

              <h3 className="font-medium text-text-0 text-sm mb-1 truncate">{project.name}</h3>
              {project.description && (
                <p className="text-xs text-text-2 line-clamp-2 mb-3">{project.description}</p>
              )}

              <div className="flex items-center gap-3 mt-auto pt-2 border-t border-border-1">
                <span className="flex items-center gap-1 mono">
                  <Clock size={11} />
                  {formatDistanceToNow(new Date(project.updated_at), { addSuffix: true })}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}

      {createProjectModal && <CreateProjectModal onClose={closeCreateProject} />}
    </div>
  )
}
