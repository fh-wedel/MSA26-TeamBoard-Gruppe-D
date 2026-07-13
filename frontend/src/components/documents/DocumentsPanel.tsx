import { useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Upload, Download, Trash2, Pencil, FileText, Check, X } from 'lucide-react'
import { documentsApi } from '../../api/documents'
import { formatDistanceToNow } from 'date-fns'

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${value.toFixed(value < 10 && unit > 0 ? 1 : 0)} ${units[unit]}`
}

export default function DocumentsPanel({ projectId, canManage }: { projectId: string; canManage: boolean }) {
  const qc = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [error, setError] = useState('')
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameDraft, setRenameDraft] = useState('')

  const { data } = useQuery({
    queryKey: ['documents', projectId],
    queryFn: () => documentsApi.list(projectId),
    enabled: !!projectId,
  })
  const documents = data?.data ?? []

  const invalidate = () => qc.invalidateQueries({ queryKey: ['documents', projectId] })

  const upload = useMutation({
    mutationFn: (file: File) => documentsApi.upload(projectId, file),
    onSuccess: invalidate,
    onError: (e: Error) => setError(e.message),
  })

  const rename = useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => documentsApi.rename(id, name),
    onSuccess: () => { setRenamingId(null); invalidate() },
    onError: (e: Error) => setError(e.message),
  })

  const remove = useMutation({
    mutationFn: (id: string) => documentsApi.delete(id),
    onSuccess: invalidate,
    onError: (e: Error) => setError(e.message),
  })

  function onPickFiles(files: FileList | null) {
    if (!files) return
    setError('')
    Array.from(files).forEach((f) => upload.mutate(f))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm text-text-2">{documents.length} {documents.length === 1 ? 'Datei' : 'Dateien'}</p>
        {canManage && (
          <>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => { onPickFiles(e.target.files); e.target.value = '' }}
            />
            <button onClick={() => fileInputRef.current?.click()} disabled={upload.isPending}
              className="btn-primary flex items-center gap-2">
              <Upload size={14} /> {upload.isPending ? 'Wird hochgeladen…' : 'Datei hochladen'}
            </button>
          </>
        )}
      </div>

      {error && (
        <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded mb-4">{error}</div>
      )}

      <div className="card divide-y divide-border-1">
        {documents.map((doc) => (
          <div key={doc.id} className="flex items-center justify-between px-4 py-3 gap-3">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <div className="w-8 h-8 rounded bg-bg-3 border border-border-2 flex items-center justify-center text-text-2 flex-shrink-0">
                <FileText size={15} />
              </div>
              <div className="min-w-0 flex-1">
                {renamingId === doc.id ? (
                  <form
                    onSubmit={(e) => { e.preventDefault(); if (renameDraft.trim()) rename.mutate({ id: doc.id, name: renameDraft.trim() }) }}
                    className="flex items-center gap-1.5">
                    <input value={renameDraft} onChange={(e) => setRenameDraft(e.target.value)}
                      className="input-base text-sm py-1 flex-1" autoFocus
                      onKeyDown={(e) => e.key === 'Escape' && setRenamingId(null)} />
                    <button type="submit" className="text-success hover:opacity-80 p-1"><Check size={14} /></button>
                    <button type="button" onClick={() => setRenamingId(null)} className="text-text-3 hover:text-text-1 p-1"><X size={14} /></button>
                  </form>
                ) : (
                  <>
                    <p className="text-sm text-text-0 truncate">{doc.name}</p>
                    <p className="mono">
                      {doc.content_type} · {formatDistanceToNow(new Date(doc.updated_at), { addSuffix: true })}
                    </p>
                  </>
                )}
              </div>
            </div>

            {renamingId !== doc.id && (
              <div className="flex items-center gap-1 flex-shrink-0">
                <button onClick={() => documentsApi.download(doc.id)} title="Herunterladen"
                  className="text-text-3 hover:text-accent p-1.5 rounded transition-colors">
                  <Download size={14} />
                </button>
                {canManage && (
                  <>
                    <button onClick={() => { setRenamingId(doc.id); setRenameDraft(doc.name) }} title="Umbenennen"
                      className="text-text-3 hover:text-text-1 p-1.5 rounded transition-colors">
                      <Pencil size={13} />
                    </button>
                    <button
                      onClick={() => { if (window.confirm(`„${doc.name}" wirklich löschen?`)) remove.mutate(doc.id) }}
                      title="Löschen"
                      className="text-text-3 hover:text-danger p-1.5 rounded transition-colors">
                      <Trash2 size={14} />
                    </button>
                  </>
                )}
              </div>
            )}
          </div>
        ))}
        {documents.length === 0 && (
          <div className="text-center py-16 text-sm text-text-3">
            Noch keine Dateien. {canManage ? 'Lade die erste Datei hoch.' : ''}
          </div>
        )}
      </div>
    </div>
  )
}
