import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  X, Trash2, MessageSquare, Clock, Tag, AlertTriangle, Send, Pencil
} from 'lucide-react'
import { tasksApi } from '../../api/tasks'
import { useAuthStore } from '../../stores/authStore'
import { format, formatDistanceToNow } from 'date-fns'
import type { Priority, TaskStatus } from '../../api/types'
import clsx from 'clsx'

const STATUS_OPTIONS: { value: TaskStatus; label: string; color: string }[] = [
  { value: 'open',        label: 'Open',        color: 'text-text-2' },
  { value: 'in_progress', label: 'In Progress',  color: 'text-accent' },
  { value: 'blocked',     label: 'Blocked',      color: 'text-danger' },
  { value: 'done',        label: 'Done',         color: 'text-success' },
  { value: 'archived',    label: 'Archived',     color: 'text-text-3' },
]

const PRIORITY_OPTIONS: { value: Priority; color: string }[] = [
  { value: 'critical', color: 'text-danger' },
  { value: 'high',     color: 'text-warning' },
  { value: 'medium',   color: 'text-amber' },
  { value: 'low',      color: 'text-text-3' },
]

export default function TaskDetailPanel({ taskId, onClose }: { taskId: string; onClose: () => void }) {
  const qc = useQueryClient()
  const { user } = useAuthStore()
  const [commentBody, setCommentBody] = useState('')
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')

  const { data } = useQuery({
    queryKey: ['task', taskId],
    queryFn: () => tasksApi.get(taskId),
  })

  const { data: commentsData } = useQuery({
    queryKey: ['comments', taskId],
    queryFn: () => tasksApi.listComments(taskId),
  })

  const updateTask = useMutation({
    mutationFn: (patch: Parameters<typeof tasksApi.update>[1]) => tasksApi.update(taskId, patch),
    onSuccess: (res) => {
      qc.setQueryData(['task', taskId], res)
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })

  const postComment = useMutation({
    mutationFn: () => tasksApi.createComment(taskId, commentBody),
    onSuccess: () => { setCommentBody(''); qc.invalidateQueries({ queryKey: ['comments', taskId] }) },
  })

  const deleteComment = useMutation({
    mutationFn: (commentId: string) => tasksApi.deleteComment(taskId, commentId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['comments', taskId] }),
  })

  const task = data?.data
  const comments = commentsData?.data ?? []

  if (!task) return null

  const currentStatus = STATUS_OPTIONS.find(s => s.value === task.status)

  return (
    <>
      {/* Backdrop */}
      <div className="fixed inset-0 z-40 bg-bg-0/40 backdrop-blur-sm" onClick={onClose} />

      {/* Panel */}
      <div className="fixed right-0 top-0 bottom-0 z-50 w-[480px] bg-bg-1 border-l border-border-2 flex flex-col animate-slide-in-right">

        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border-1">
          <div className="flex items-center gap-2">
            <span className={clsx('text-xs font-medium', currentStatus?.color)}>
              {currentStatus?.label}
            </span>
            <span className="text-border-3">·</span>
            <span className="mono">{task.id.slice(0, 8)}</span>
          </div>
          <button onClick={onClose} className="btn-ghost p-1.5 rounded">
            <X size={16} />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto">
          {/* Title */}
          <div className="px-5 py-4 border-b border-border-1">
            {editingTitle ? (
              <form onSubmit={(e) => {
                e.preventDefault()
                updateTask.mutate({ title: titleDraft })
                setEditingTitle(false)
              }}>
                <input
                  value={titleDraft}
                  onChange={(e) => setTitleDraft(e.target.value)}
                  className="input-base w-full text-base font-medium"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Escape' && setEditingTitle(false)}
                  onBlur={() => { if (titleDraft.trim() && titleDraft !== task.title) updateTask.mutate({ title: titleDraft }); setEditingTitle(false) }}
                />
              </form>
            ) : (
              <div className="flex items-start gap-2 group">
                <h2 className="text-base font-medium text-text-0 flex-1 leading-snug">{task.title}</h2>
                <button onClick={() => { setTitleDraft(task.title); setEditingTitle(true) }}
                  className="opacity-0 group-hover:opacity-100 transition-opacity mt-0.5 text-text-3 hover:text-text-1">
                  <Pencil size={13} />
                </button>
              </div>
            )}
          </div>

          {/* Metadata grid */}
          <div className="px-5 py-4 border-b border-border-1 grid grid-cols-2 gap-4">
            {/* Status */}
            <div>
              <p className="label mb-1.5">Status</p>
              <select value={task.status}
                onChange={(e) => updateTask.mutate({ title: task.title })} // status update needs separate endpoint
                className="input-base w-full text-xs">
                {STATUS_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
              </select>
            </div>

            {/* Priority */}
            <div>
              <p className="label mb-1.5">Priority</p>
              <div className="flex gap-1.5">
                {PRIORITY_OPTIONS.map(p => (
                  <button key={p.value} onClick={() => updateTask.mutate({ priority: p.value as Priority })}
                    className={clsx(
                      'flex-1 py-1 rounded border text-xs font-medium capitalize transition-colors',
                      task.priority === p.value
                        ? `bg-bg-3 border-border-3 ${p.color}`
                        : 'border-border-1 text-text-3 hover:border-border-2'
                    )}>
                    {p.value.charAt(0).toUpperCase()}
                  </button>
                ))}
              </div>
            </div>

            {/* Start / Due dates */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <p className="label mb-1.5 flex items-center gap-1"><Clock size={11} /> Start date</p>
                <input type="date"
                  value={task.start_date ? task.start_date.slice(0, 10) : ''}
                  onChange={(e) => updateTask.mutate({ start_date: e.target.value ? new Date(e.target.value).toISOString() : null })}
                  className="input-base w-full text-xs" />
              </div>
              <div>
                <p className="label mb-1.5 flex items-center gap-1"><Clock size={11} /> Due date</p>
                <input type="date"
                  value={task.due_date ? task.due_date.slice(0, 10) : ''}
                  onChange={(e) => updateTask.mutate({ due_date: e.target.value ? new Date(e.target.value).toISOString() : null })}
                  className="input-base w-full text-xs" />
              </div>
            </div>

            {/* Labels */}
            <div>
              <p className="label mb-1.5 flex items-center gap-1"><Tag size={11} /> Labels</p>
              <div className="flex flex-wrap gap-1">
                {task.labels.map(l => (
                  <span key={l} className="px-1.5 py-0.5 bg-purple/10 text-purple text-[10px] rounded-full border border-purple/20">
                    {l}
                  </span>
                ))}
                {task.labels.length === 0 && <span className="text-xs text-text-3">None</span>}
              </div>
            </div>
          </div>

          {/* Description */}
          <div className="px-5 py-4 border-b border-border-1">
            <p className="label mb-2">Description</p>
            <textarea
              defaultValue={task.description}
              placeholder="Add a description…"
              rows={3}
              className="input-base w-full resize-none text-sm"
              onBlur={(e) => {
                if (e.target.value !== task.description)
                  updateTask.mutate({ description: e.target.value })
              }}
            />
          </div>

          {/* Comments */}
          <div className="px-5 py-4">
            <p className="label mb-3 flex items-center gap-1.5">
              <MessageSquare size={11} /> Comments ({comments.length})
            </p>

            <div className="space-y-3 mb-4">
              {comments.map((comment) => (
                <div key={comment.id} className="flex gap-2.5 group">
                  <div className="w-6 h-6 rounded bg-bg-3 border border-border-2 flex items-center justify-center text-[10px] font-medium text-text-1 flex-shrink-0 mt-0.5">
                    {comment.author_id.charAt(0).toUpperCase()}
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="mono">{formatDistanceToNow(new Date(comment.created_at), { addSuffix: true })}</span>
                      {comment.edited_at && <span className="mono text-text-3">(edited)</span>}
                      {comment.author_id === user?.id && (
                        <button onClick={() => deleteComment.mutate(comment.id)}
                          className="opacity-0 group-hover:opacity-100 transition-opacity ml-auto text-text-3 hover:text-danger">
                          <Trash2 size={11} />
                        </button>
                      )}
                    </div>
                    <p className="text-sm text-text-1 leading-relaxed whitespace-pre-wrap">{comment.body}</p>
                  </div>
                </div>
              ))}
            </div>

            {/* Comment input */}
            <form onSubmit={(e) => { e.preventDefault(); if (commentBody.trim()) postComment.mutate() }}>
              <div className="flex gap-2">
                <textarea
                  value={commentBody}
                  onChange={(e) => setCommentBody(e.target.value)}
                  placeholder="Write a comment…"
                  rows={2}
                  className="input-base flex-1 resize-none text-sm"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                      e.preventDefault(); if (commentBody.trim()) postComment.mutate()
                    }
                  }}
                />
                <button type="submit" disabled={!commentBody.trim() || postComment.isPending}
                  className="btn-primary px-3 self-end flex items-center gap-1">
                  <Send size={13} />
                </button>
              </div>
              <p className="text-[10px] text-text-3 mt-1">⌘ + Enter to submit</p>
            </form>
          </div>
        </div>

        {/* Footer */}
        <div className="border-t border-border-1 px-5 py-3 flex items-center justify-between">
          <span className="mono">
            {task.updated_at ? `Updated ${format(new Date(task.updated_at), 'MMM d, HH:mm')}` : ''}
          </span>
          <button className="btn-danger flex items-center gap-1.5 py-1.5">
            <Trash2 size={13} /> Delete
          </button>
        </div>
      </div>
    </>
  )
}
