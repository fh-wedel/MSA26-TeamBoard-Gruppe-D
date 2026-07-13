import { useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Paperclip, Download, X, Plus } from 'lucide-react'
import { tasksApi } from '../../api/tasks'
import { documentsApi } from '../../api/documents'

export default function TaskAttachments({ taskId, projectId }: { taskId: string; projectId: string }) {
  const qc = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [error, setError] = useState('')

  const { data: attachmentsData } = useQuery({
    queryKey: ['attachments', taskId],
    queryFn: () => tasksApi.listAttachments(taskId),
  })

  // Resolve document names/types for display — attachments only carry document_id.
  const { data: documentsData } = useQuery({
    queryKey: ['documents', projectId],
    queryFn: () => documentsApi.list(projectId),
    enabled: !!projectId,
  })

  const attachments = attachmentsData?.data ?? []
  const docById = new Map((documentsData?.data ?? []).map((d) => [d.id, d]))

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['attachments', taskId] })
    qc.invalidateQueries({ queryKey: ['task', taskId] })
  }

  const attachNew = useMutation({
    mutationFn: async (file: File) => {
      const doc = await documentsApi.upload(projectId, file)
      return tasksApi.addAttachment(taskId, doc.id)
    },
    onSuccess: invalidate,
    onError: (e: Error) => setError(e.message),
  })

  const removeAttachment = useMutation({
    mutationFn: (attachmentId: string) => tasksApi.removeAttachment(taskId, attachmentId),
    onSuccess: invalidate,
    onError: (e: Error) => setError(e.message),
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <p className="label flex items-center gap-1.5">
          <Paperclip size={11} /> Anhänge ({attachments.length})
        </p>
        <input
          ref={fileInputRef}
          type="file"
          className="hidden"
          onChange={(e) => { setError(''); const f = e.target.files?.[0]; if (f) attachNew.mutate(f); e.target.value = '' }}
        />
        <button onClick={() => fileInputRef.current?.click()} disabled={attachNew.isPending}
          className="btn-ghost text-xs flex items-center gap-1 py-1">
          <Plus size={12} /> {attachNew.isPending ? 'Wird hochgeladen…' : 'Datei anhängen'}
        </button>
      </div>

      {error && (
        <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded mb-3">{error}</div>
      )}

      <div className="space-y-1.5">
        {attachments.map((att) => {
          const doc = docById.get(att.document_id)
          return (
            <div key={att.id} className="flex items-center gap-2 px-2.5 py-2 rounded bg-bg-2 border border-border-1 group">
              <Paperclip size={13} className="text-text-3 flex-shrink-0" />
              <span className="text-sm text-text-1 truncate flex-1">{doc?.name ?? att.document_id.slice(0, 8)}</span>
              <button onClick={() => documentsApi.download(att.document_id)} title="Herunterladen"
                className="text-text-3 hover:text-accent p-1 rounded transition-colors">
                <Download size={13} />
              </button>
              <button onClick={() => removeAttachment.mutate(att.id)} title="Entfernen"
                className="opacity-0 group-hover:opacity-100 transition-opacity text-text-3 hover:text-danger p-1 rounded">
                <X size={13} />
              </button>
            </div>
          )
        })}
        {attachments.length === 0 && (
          <p className="text-xs text-text-3">Keine Anhänge.</p>
        )}
      </div>
    </div>
  )
}
