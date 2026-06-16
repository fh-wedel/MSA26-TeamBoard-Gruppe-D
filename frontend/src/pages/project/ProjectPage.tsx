import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Kanban, Plus, Users, Settings, ChevronRight, Mail, Clock } from 'lucide-react'
import { projectsApi } from '../../api/projects'
import { invitationsApi } from '../../api/invitations'
import type { BoardTypeDef } from '../../api/types'
import { formatDistanceToNow } from 'date-fns'
import SchemaForm from '../../components/board/SchemaForm'
import { useBoardTypes } from '../../hooks/useBoardTypes'

// Registry icons are emoji strings; fall back to a Lucide glyph when absent.
function BoardTypeIcon({ icon, size = 16 }: { icon?: string; size?: number }) {
  if (icon && icon.trim() !== '') return <span style={{ fontSize: size }}>{icon}</span>
  return <Kanban size={size} />
}

function InviteModal({ projectId, onClose }: { projectId: string; onClose: () => void }) {
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'viewer' | 'editor'>('viewer')
  const [error, setError] = useState('')
  const qc = useQueryClient()

  const invite = useMutation({
    mutationFn: () => invitationsApi.create(projectId, email, role),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['invitations', projectId] }); onClose() },
    onError: (e: Error) => setError(e.message),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg-0/80 backdrop-blur-sm animate-fade-in">
      <div className="card w-full max-w-md p-6 animate-scale-in">
        <h2 className="text-base font-semibold text-text-0 mb-1">Mitglied einladen</h2>
        <p className="text-sm text-text-2 mb-5">
          Der Benutzer muss bereits registriert sein. Er erhält eine Benachrichtigung und kann die Einladung annehmen oder ablehnen.
        </p>
        <form onSubmit={(e) => { e.preventDefault(); setError(''); invite.mutate() }} className="space-y-4">
          {error && (
            <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded">{error}</div>
          )}
          <div>
            <label className="label block mb-1.5">E-Mail-Adresse</label>
            <input value={email} onChange={(e) => setEmail(e.target.value)}
              type="email" className="input-base w-full" placeholder="name@example.com" required autoFocus />
          </div>
          <div>
            <label className="label block mb-2">Rolle</label>
            <div className="grid grid-cols-2 gap-2">
              {(['viewer', 'editor'] as const).map((r) => (
                <button key={r} type="button" onClick={() => setRole(r)}
                  className={`py-2 rounded border text-sm font-medium capitalize transition-colors ${
                    role === r
                      ? 'bg-accent/10 border-accent text-accent'
                      : 'bg-bg-3 border-border-2 text-text-2 hover:border-border-3'
                  }`}>
                  {r === 'viewer' ? '👁 Betrachter' : '✏️ Bearbeiter'}
                </button>
              ))}
            </div>
          </div>
          <div className="flex gap-2 justify-end pt-2">
            <button type="button" onClick={onClose} className="btn-ghost">Abbrechen</button>
            <button type="submit" disabled={invite.isPending || !email.trim()} className="btn-primary flex items-center gap-2">
              <Mail size={14} /> {invite.isPending ? 'Wird gesendet…' : 'Einladung senden'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function CreateBoardModal({ projectId, onClose }: { projectId: string; onClose: () => void }) {
  const [name, setName] = useState('')
  const [type, setType] = useState<string>('')
  const [config, setConfig] = useState<Record<string, unknown>>({})
  const [error, setError] = useState('')
  const qc = useQueryClient()

  const { data: typesData, isLoading: typesLoading } = useBoardTypes()
  const boardTypes = typesData?.data ?? []
  const selected: BoardTypeDef | undefined = boardTypes.find((t) => t.type === type)

  // When a type is picked, seed the config form with its default_config.
  function pickType(def: BoardTypeDef) {
    setType(def.type)
    setConfig({ ...(def.default_config ?? {}) })
  }

  const create = useMutation({
    mutationFn: () => projectsApi.createBoard(projectId, name, type, config),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['boards', projectId] }); onClose() },
    onError: (e: Error) => setError(e.message),
  })

  const hasSchemaFields = selected && Object.keys(selected.config_schema?.properties ?? {}).length > 0

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg-0/80 backdrop-blur-sm animate-fade-in">
      <div className="card w-full max-w-md p-6 animate-scale-in max-h-[90vh] overflow-y-auto">
        <h2 className="text-base font-semibold text-text-0 mb-5">New Board</h2>
        <form onSubmit={(e) => { e.preventDefault(); setError(''); create.mutate() }} className="space-y-4">
          {error && (
            <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded">{error}</div>
          )}
          <div>
            <label className="label block mb-1.5">Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)}
              className="input-base w-full" placeholder="Sprint 1" required autoFocus />
          </div>
          <div>
            <label className="label block mb-2">Type</label>
            {typesLoading ? (
              <p className="text-xs text-text-3">Loading board types…</p>
            ) : boardTypes.length === 0 ? (
              <p className="text-xs text-text-3">No board types available.</p>
            ) : (
              <div className="grid grid-cols-3 gap-2">
                {boardTypes.map((t) => (
                  <button key={t.type} type="button" onClick={() => pickType(t)}
                    className={`flex flex-col items-center gap-1.5 p-3 rounded border text-xs font-medium transition-colors ${
                      type === t.type
                        ? 'bg-accent/10 border-accent text-accent'
                        : 'bg-bg-3 border-border-2 text-text-2 hover:border-border-3'
                    }`}>
                    <BoardTypeIcon icon={t.icon} size={18} />
                    <span className="text-center leading-tight">{t.display_name || t.type}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Config form driven by the selected type's JSON-Schema. */}
          {hasSchemaFields && selected && (
            <div className="border-t border-border-1 pt-4">
              <p className="label mb-3">Configuration</p>
              <SchemaForm schema={selected.config_schema} value={config} onChange={setConfig} />
            </div>
          )}

          <div className="flex gap-2 justify-end pt-2">
            <button type="button" onClick={onClose} className="btn-ghost">Cancel</button>
            <button type="submit" disabled={create.isPending || !name.trim() || !type} className="btn-primary">
              {create.isPending ? 'Creating…' : 'Create board'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default function ProjectPage() {
  const { projectId } = useParams<{ projectId: string }>()
  const [showCreateBoard, setShowCreateBoard] = useState(false)
  const [activeTab, setActiveTab] = useState<'boards' | 'members' | 'settings'>('boards')

  const { data: projectData } = useQuery({
    queryKey: ['project', projectId],
    queryFn: () => projectsApi.get(projectId!),
    enabled: !!projectId,
  })

  const { data: boardsData } = useQuery({
    queryKey: ['boards', projectId],
    queryFn: () => projectsApi.listBoards(projectId!),
    enabled: !!projectId,
  })

  const { data: membersData } = useQuery({
    queryKey: ['members', projectId],
    queryFn: () => projectsApi.listMembers(projectId!),
    enabled: !!projectId && activeTab === 'members',
  })

  const { data: invitationsData } = useQuery({
    queryKey: ['invitations', projectId],
    queryFn: () => invitationsApi.list(projectId!),
    enabled: !!projectId && activeTab === 'members',
  })

  const { data: typesData } = useBoardTypes()
  const boardTypeByType = new Map((typesData?.data ?? []).map((t) => [t.type, t]))

  const [showInviteModal, setShowInviteModal] = useState(false)

  const project = projectData?.data
  const boards = boardsData?.data ?? []
  const members = membersData?.data ?? []
  const invitations = invitationsData?.data ?? []
  const pendingInvitations = invitations.filter(i => i.status === 'pending')

  if (!project) return null

  return (
    <div className="p-6 max-w-5xl mx-auto">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-lg font-semibold text-text-0">{project.name}</h1>
        {project.description && <p className="text-sm text-text-2 mt-1">{project.description}</p>}
      </div>

      {/* Tabs */}
      <div className="flex gap-0.5 mb-6 border-b border-border-1">
        {[
          { id: 'boards', label: 'Boards', icon: <Kanban size={14} /> },
          { id: 'members', label: `Members${pendingInvitations.length > 0 ? ` (${pendingInvitations.length} pending)` : ''}`, icon: <Users size={14} /> },
          { id: 'settings', label: 'Settings', icon: <Settings size={14} /> },
        ].map((tab) => (
          <button key={tab.id} onClick={() => setActiveTab(tab.id as typeof activeTab)}
            className={`flex items-center gap-1.5 px-4 py-2.5 text-sm border-b-2 transition-colors ${
              activeTab === tab.id
                ? 'border-accent text-accent'
                : 'border-transparent text-text-2 hover:text-text-1'
            }`}>
            {tab.icon} {tab.label}
          </button>
        ))}
      </div>

      {/* Boards Tab */}
      {activeTab === 'boards' && (
        <>
          <div className="flex justify-end mb-4">
            <button onClick={() => setShowCreateBoard(true)} className="btn-primary flex items-center gap-2">
              <Plus size={14} /> New board
            </button>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {boards.map((board) => (
              <Link key={board.id} to={`/projects/${projectId}/boards/${board.id}`}
                className="card p-5 hover:border-border-3 hover:bg-bg-3 transition-all group">
                <div className="flex items-start justify-between mb-3">
                  <div className="text-accent"><BoardTypeIcon icon={boardTypeByType.get(board.type)?.icon} /></div>
                  <ChevronRight size={14} className="text-text-3 group-hover:text-text-1 transition-colors" />
                </div>
                <h3 className="font-medium text-text-0 text-sm mb-1">{board.name}</h3>
                <p className="text-xs text-text-3 capitalize">{boardTypeByType.get(board.type)?.display_name ?? board.type}</p>
                {board.columns && (
                  <div className="flex gap-1 mt-3 flex-wrap">
                    {board.columns.map((col) => (
                      <span key={col.id} className="px-1.5 py-0.5 bg-bg-3 rounded text-[10px] text-text-3">
                        {col.name}
                      </span>
                    ))}
                  </div>
                )}
              </Link>
            ))}
            {boards.length === 0 && (
              <div className="col-span-3 text-center py-16 text-sm text-text-3">
                No boards yet. Create your first board.
              </div>
            )}
          </div>
        </>
      )}

      {/* Members Tab */}
      {activeTab === 'members' && (
        <>
          <div className="flex justify-end mb-4">
            <button onClick={() => setShowInviteModal(true)} className="btn-primary flex items-center gap-2">
              <Mail size={14} /> Einladen
            </button>
          </div>

          {/* Active members */}
          <div className="card divide-y divide-border-1 mb-4">
            {members.map((m) => (
              <div key={m.user_id} className="flex items-center justify-between px-4 py-3">
                <div className="flex items-center gap-3">
                  <div className="w-7 h-7 rounded bg-bg-3 border border-border-2 flex items-center justify-center text-xs font-medium text-text-1">
                    {m.email?.charAt(0).toUpperCase() ?? '?'}
                  </div>
                  <div>
                    <p className="text-sm text-text-0">{m.email ?? m.user_id}</p>
                    <p className="mono">{new Date(m.joined_at).toLocaleDateString()}</p>
                  </div>
                </div>
                <span className={`status-badge text-xs px-2 py-0.5 rounded ${
                  m.role === 'owner' ? 'bg-amber/10 text-amber border border-amber/20' :
                  m.role === 'editor' ? 'bg-accent/10 text-accent border border-accent/20' :
                  'bg-bg-3 text-text-2 border border-border-2'
                }`}>
                  {m.role}
                </span>
              </div>
            ))}
          </div>

          {/* Pending invitations */}
          {pendingInvitations.length > 0 && (
            <div>
              <p className="label mb-2 flex items-center gap-1.5"><Clock size={11} /> Ausstehende Einladungen</p>
              <div className="card divide-y divide-border-1">
                {pendingInvitations.map((inv) => (
                  <div key={inv.id} className="flex items-center justify-between px-4 py-3">
                    <div className="flex items-center gap-3">
                      <div className="w-7 h-7 rounded bg-accent/10 border border-accent/20 flex items-center justify-center">
                        <Mail size={12} className="text-accent" />
                      </div>
                      <div>
                        <p className="text-sm text-text-0">{inv.invitee_email}</p>
                        <p className="mono">Läuft ab {formatDistanceToNow(new Date(inv.expires_at), { addSuffix: true })}</p>
                      </div>
                    </div>
                    <span className="text-xs px-2 py-0.5 rounded bg-accent/10 text-accent border border-accent/20 capitalize">
                      {inv.role} · ausstehend
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {showInviteModal && projectId && (
            <InviteModal projectId={projectId} onClose={() => setShowInviteModal(false)} />
          )}
        </>
      )}

      {/* Settings Tab */}
      {activeTab === 'settings' && (
        <div className="card p-6 text-sm text-text-2">
          <p className="font-mono text-xs text-text-3 mb-2">Project ID</p>
          <code className="mono text-text-1">{project.id}</code>
        </div>
      )}

      {showCreateBoard && projectId && (
        <CreateBoardModal projectId={projectId} onClose={() => setShowCreateBoard(false)} />
      )}
    </div>
  )
}
