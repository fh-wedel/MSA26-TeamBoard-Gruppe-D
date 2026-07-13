import {
  DndContext, DragOverlay, PointerSensor, useSensor, useSensors,
  type DragEndEvent, type DragStartEvent, closestCorners,
} from '@dnd-kit/core'
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { tasksApi } from '../../../api/tasks'
import Column from '../Column'
import TaskCard from '../TaskCard'
import type { BoardViewProps } from './index'
import type { Task } from '../../../api/types'

// The default renderer: a column/Kanban board with drag-and-drop. Honors
// presentation.card for how task cards look. group_by other than "column" falls
// back to the board's columns (extension point for assignee/priority swimlanes).
export default function BoardView({ board, tasks, presentation }: BoardViewProps) {
  const qc = useQueryClient()
  const [activeTask, setActiveTask] = useState<Task | null>(null)
  const card = presentation?.card

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  const moveTask = useMutation({
    mutationFn: ({ taskId, columnId, beforeId }: { taskId: string; columnId: string; beforeId?: string }) =>
      tasksApi.move(taskId, columnId, beforeId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks', board.id] }),
  })

  const columns = [...(board.columns ?? [])].sort((a, b) => a.position - b.position)

  function tasksByColumn(columnId: string) {
    return tasks.filter((t) => t.column_id === columnId)
  }

  function handleDragStart(event: DragStartEvent) {
    const task = tasks.find((t) => t.id === event.active.id)
    if (task) setActiveTask(task)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveTask(null)
    const { active, over } = event
    if (!over || active.id === over.id) return

    const taskId = String(active.id)
    const task = tasks.find((t) => t.id === taskId)
    if (!task) return

    const overTask = tasks.find((t) => t.id === over.id)
    const targetColumnId = overTask ? overTask.column_id! : String(over.id)
    if (!targetColumnId) return
    moveTask.mutate({ taskId, columnId: targetColumnId, beforeId: overTask?.id })
  }

  if (columns.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center text-sm text-text-3">
        This board has no columns.
      </div>
    )
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCorners}
      onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex gap-5 p-6 overflow-x-auto flex-1 items-start">
        {columns.map((col) => (
          <Column key={col.id} column={col} tasks={tasksByColumn(col.id)} boardId={board.id} card={card} />
        ))}
      </div>

      <DragOverlay>
        {activeTask && (
          <div className="rotate-1 scale-105">
            <TaskCard task={activeTask} card={card} />
          </div>
        )}
      </DragOverlay>
    </DndContext>
  )
}
