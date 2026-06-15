import { useState } from 'react'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useDroppable } from '@dnd-kit/core'
import { Plus } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { tasksApi } from '../../api/tasks'
import TaskCard from './TaskCard'
import type { Column as ColType, Task } from '../../api/types'
import clsx from 'clsx'

interface Props {
  column: ColType
  tasks: Task[]
  boardId: string
}

export default function Column({ column, tasks, boardId }: Props) {
  const { setNodeRef, isOver } = useDroppable({ id: column.id })
  const [adding, setAdding] = useState(false)
  const [title, setTitle] = useState('')
  const qc = useQueryClient()

  const createTask = useMutation({
    mutationFn: () => tasksApi.create(boardId, { title, column_id: column.id }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['tasks', boardId] }); setTitle(''); setAdding(false) },
  })

  return (
    <div className="flex flex-col w-72 flex-shrink-0">
      {/* Column header */}
      <div className="flex items-center justify-between mb-3 px-1">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-medium text-text-1">{column.name}</h3>
          <span className="text-xs text-text-3 bg-bg-3 px-1.5 py-0.5 rounded-full">{tasks.length}</span>
        </div>
        {column.wip_limit && tasks.length >= column.wip_limit && (
          <span className="text-[10px] text-warning">WIP {tasks.length}/{column.wip_limit}</span>
        )}
      </div>

      {/* Drop zone */}
      <div
        ref={setNodeRef}
        className={clsx(
          'flex-1 rounded p-2 min-h-24 transition-colors duration-100',
          isOver ? 'bg-accent/5 border border-accent/20' : 'bg-bg-0/30 border border-transparent'
        )}>
        <SortableContext items={tasks.map(t => t.id)} strategy={verticalListSortingStrategy}>
          <div className="space-y-2">
            {tasks.map((task) => (
              <TaskCard key={task.id} task={task} />
            ))}
          </div>
        </SortableContext>

        {/* Add task inline */}
        {adding ? (
          <form onSubmit={(e) => { e.preventDefault(); if (title.trim()) createTask.mutate() }}
            className="mt-2">
            <textarea
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Task title…"
              rows={2}
              className="input-base w-full resize-none text-sm"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); if (title.trim()) createTask.mutate() }
                if (e.key === 'Escape') { setAdding(false); setTitle('') }
              }}
            />
            <div className="flex gap-1.5 mt-1.5">
              <button type="submit" disabled={!title.trim()} className="btn-primary py-1 px-3 text-xs">Add</button>
              <button type="button" onClick={() => { setAdding(false); setTitle('') }} className="btn-ghost py-1 px-3 text-xs">Cancel</button>
            </div>
          </form>
        ) : (
          <button onClick={() => setAdding(true)}
            className="mt-2 w-full flex items-center gap-1.5 px-2 py-1.5 rounded text-xs text-text-3 hover:text-text-1 hover:bg-bg-3 transition-colors">
            <Plus size={13} /> Add task
          </button>
        )}
      </div>
    </div>
  )
}
