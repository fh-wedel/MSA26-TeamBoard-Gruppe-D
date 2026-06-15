import { useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  DndContext, DragOverlay, PointerSensor, useSensor, useSensors,
  type DragEndEvent, type DragStartEvent, closestCorners,
} from '@dnd-kit/core'
import { useState } from 'react'
import { boardsApi } from '../../api/projects'
import { tasksApi } from '../../api/tasks'
import { useUIStore } from '../../stores/uiStore'
import Column from '../../components/board/Column'
import TaskCard from '../../components/board/TaskCard'
import TaskDetailPanel from '../../components/task/TaskDetailPanel'
import type { Task } from '../../api/types'

export default function BoardPage() {
  const { boardId } = useParams<{ boardId: string }>()
  const { selectedTaskId, selectTask } = useUIStore()
  const qc = useQueryClient()
  const [activeTask, setActiveTask] = useState<Task | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } })
  )

  const { data: boardData } = useQuery({
    queryKey: ['board', boardId],
    queryFn: () => boardsApi.get(boardId!),
    enabled: !!boardId,
  })

  const { data: tasksData } = useQuery({
    queryKey: ['tasks', boardId],
    queryFn: () => tasksApi.list(boardId!),
    enabled: !!boardId,
  })

  const moveTask = useMutation({
    mutationFn: ({ taskId, columnId, beforeId }: { taskId: string; columnId: string; beforeId?: string }) =>
      tasksApi.move(taskId, columnId, beforeId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks', boardId] }),
  })

  const board = boardData?.data
  const allTasks = tasksData?.data ?? []
  const columns = board?.columns ?? []

  function tasksByColumn(columnId: string) {
    return allTasks.filter(t => t.column_id === columnId)
  }

  function handleDragStart(event: DragStartEvent) {
    const task = allTasks.find(t => t.id === event.active.id)
    if (task) setActiveTask(task)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveTask(null)
    const { active, over } = event
    if (!over || active.id === over.id) return

    const taskId = String(active.id)
    const task = allTasks.find(t => t.id === taskId)
    if (!task) return

    // over could be a column ID or a task ID
    const overTask = allTasks.find(t => t.id === over.id)
    const targetColumnId = overTask ? overTask.column_id! : String(over.id)

    if (!targetColumnId) return
    moveTask.mutate({ taskId, columnId: targetColumnId, beforeId: overTask?.id })
  }

  if (!board) return null

  return (
    <div className="h-full flex flex-col">
      {/* Board toolbar */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-border-1 flex-shrink-0">
        <span className="text-xs text-text-3">{allTasks.length} tasks</span>
      </div>

      {/* Columns */}
      <DndContext sensors={sensors} collisionDetection={closestCorners}
        onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        <div className="flex gap-5 p-6 overflow-x-auto flex-1 items-start">
          {columns
            .sort((a, b) => a.position - b.position)
            .map((col) => (
              <Column
                key={col.id}
                column={col}
                tasks={tasksByColumn(col.id)}
                boardId={boardId!}
              />
            ))}
        </div>

        <DragOverlay>
          {activeTask && (
            <div className="rotate-1 scale-105">
              <TaskCard task={activeTask} />
            </div>
          )}
        </DragOverlay>
      </DndContext>

      {/* Task detail slide-over */}
      {selectedTaskId && (
        <TaskDetailPanel taskId={selectedTaskId} onClose={() => selectTask(null)} />
      )}
    </div>
  )
}
