import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { tasksApi } from '../../api/tasks'
import type { Board, Priority } from '../../api/types'

const PRIORITIES: Priority[] = ['low', 'medium', 'high', 'critical']

// View-agnostic task creation. Calendar/timeline views have no columns to click,
// so this gives every view a discoverable create path. The task is placed in the
// board's first column (calendar/timeline lay tasks out by date, ignoring columns).
export default function CreateTaskModal({
  board, defaultDueDate, onClose,
}: {
  board: Board
  defaultDueDate?: string // yyyy-MM-dd
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [priority, setPriority] = useState<Priority>('medium')
  const [startDate, setStartDate] = useState('')
  const [dueDate, setDueDate] = useState(defaultDueDate ?? '')
  const [error, setError] = useState('')
  const qc = useQueryClient()

  const columns = [...(board.columns ?? [])].sort((a, b) => a.position - b.position)
  const firstColumnId = columns[0]?.id

  const create = useMutation({
    mutationFn: () => tasksApi.create(board.id, {
      title,
      priority,
      column_id: firstColumnId,
      ...(startDate ? { start_date: new Date(startDate).toISOString() } : {}),
      ...(dueDate ? { due_date: new Date(dueDate).toISOString() } : {}),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['tasks', board.id] }); onClose() },
    onError: (e: Error) => setError(e.message),
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-bg-0/80 backdrop-blur-sm animate-fade-in">
      <div className="card w-full max-w-md p-6 animate-scale-in">
        <h2 className="text-base font-semibold text-text-0 mb-5">New task</h2>
        {!firstColumnId ? (
          <>
            <p className="text-sm text-text-2 mb-5">
              This board has no columns to hold tasks. Add a column first.
            </p>
            <div className="flex justify-end">
              <button onClick={onClose} className="btn-ghost">Close</button>
            </div>
          </>
        ) : (
          <form onSubmit={(e) => { e.preventDefault(); setError(''); create.mutate() }} className="space-y-4">
            {error && (
              <div className="bg-danger/10 border border-danger/30 text-danger text-xs px-3 py-2 rounded">{error}</div>
            )}
            <div>
              <label className="label block mb-1.5">Title</label>
              <input value={title} onChange={(e) => setTitle(e.target.value)}
                className="input-base w-full" placeholder="Pay invoice" required autoFocus />
            </div>
            <div>
              <label className="label block mb-2">Priority</label>
              <div className="grid grid-cols-4 gap-2">
                {PRIORITIES.map((p) => (
                  <button key={p} type="button" onClick={() => setPriority(p)}
                    className={`py-1.5 rounded border text-xs font-medium capitalize transition-colors ${
                      priority === p
                        ? 'bg-accent/10 border-accent text-accent'
                        : 'bg-bg-3 border-border-2 text-text-2 hover:border-border-3'
                    }`}>
                    {p}
                  </button>
                ))}
              </div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="label block mb-1.5">Start date</label>
                <input type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)}
                  max={dueDate || undefined} className="input-base w-full" />
              </div>
              <div>
                <label className="label block mb-1.5">Due date</label>
                <input type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)}
                  min={startDate || undefined} className="input-base w-full" />
              </div>
            </div>
            <div className="flex gap-2 justify-end pt-2">
              <button type="button" onClick={onClose} className="btn-ghost">Cancel</button>
              <button type="submit" disabled={create.isPending || !title.trim()} className="btn-primary">
                {create.isPending ? 'Creating…' : 'Create task'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}
