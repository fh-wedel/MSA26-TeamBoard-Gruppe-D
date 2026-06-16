import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { MessageSquare, Paperclip, Calendar, AlertCircle } from 'lucide-react'
import { format, isPast } from 'date-fns'
import { useUIStore } from '../../stores/uiStore'
import type { CardSpec, Task } from '../../api/types'
import clsx from 'clsx'

const PRIORITY_COLOR: Record<string, string> = {
  critical: 'bg-danger',
  high:     'bg-warning',
  medium:   'bg-amber',
  low:      'bg-border-3',
}

const PRIORITY_LABEL_COLOR: Record<string, string> = {
  critical: 'text-danger',
  high:     'text-warning',
  medium:   'text-amber',
  low:      'text-text-3',
}

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-border-3', in_progress: 'bg-accent', blocked: 'bg-danger',
  done: 'bg-success', archived: 'bg-text-3',
}

const DEFAULT_FIELDS = ['priority', 'due_date', 'labels', 'comment_count', 'attachment_count']

export default function TaskCard({ task, card }: { task: Task; card?: CardSpec }) {
  const { selectTask } = useUIStore()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: task.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
  }

  const fields = card?.fields ?? DEFAULT_FIELDS
  const show = (f: string) => fields.includes(f)
  const stripeColor = card?.color_by === 'status'
    ? (STATUS_COLOR[task.status] ?? 'bg-border-3')
    : (PRIORITY_COLOR[task.priority] ?? 'bg-border-3')

  const isOverdue = task.due_date && isPast(new Date(task.due_date)) && task.status !== 'done'

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      onClick={() => selectTask(task.id)}
      className={clsx(
        'bg-bg-2 border border-border-1 rounded p-3 cursor-pointer',
        'hover:border-border-3 hover:bg-bg-3 transition-all duration-100',
        'active:scale-[0.99] select-none',
        isDragging && 'shadow-xl border-accent/30',
      )}>

      {/* Color stripe */}
      <div className={clsx('w-full h-0.5 rounded-full mb-2.5 opacity-60', stripeColor)} />

      {/* Title */}
      <p className="text-sm text-text-0 leading-snug mb-2 line-clamp-2">{task.title}</p>

      {/* Labels */}
      {show('labels') && task.labels.length > 0 && (
        <div className="flex gap-1 flex-wrap mb-2">
          {task.labels.slice(0, 3).map((label) => (
            <span key={label} className="px-1.5 py-0.5 bg-purple/10 text-purple text-[10px] rounded-full border border-purple/20">
              {label}
            </span>
          ))}
        </div>
      )}

      {/* Footer */}
      <div className="flex items-center justify-between mt-2">
        {show('priority') ? (
          <span className={clsx('text-[10px] font-medium uppercase tracking-wide', PRIORITY_LABEL_COLOR[task.priority])}>
            {task.priority}
          </span>
        ) : <span />}

        <div className="flex items-center gap-2">
          {show('due_date') && task.due_date && (
            <span className={clsx('flex items-center gap-0.5 text-[10px]', isOverdue ? 'text-danger' : 'text-text-3')}>
              {isOverdue && <AlertCircle size={10} />}
              <Calendar size={10} />
              {format(new Date(task.due_date), 'MMM d')}
            </span>
          )}
          {show('comment_count') && task.comment_count > 0 && (
            <span className="flex items-center gap-0.5 text-[10px] text-text-3">
              <MessageSquare size={10} /> {task.comment_count}
            </span>
          )}
          {show('attachment_count') && task.attachment_count > 0 && (
            <span className="flex items-center gap-0.5 text-[10px] text-text-3">
              <Paperclip size={10} /> {task.attachment_count}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
