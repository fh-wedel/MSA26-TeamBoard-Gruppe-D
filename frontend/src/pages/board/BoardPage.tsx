import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Info, Plus } from 'lucide-react'
import { boardsApi } from '../../api/projects'
import { tasksApi } from '../../api/tasks'
import { useUIStore } from '../../stores/uiStore'
import { useBoardType } from '../../hooks/useBoardTypes'
import { useWebSocket } from '../../hooks/useWebSocket'
import { resolveView } from '../../components/board/views'
import CreateTaskModal from '../../components/board/CreateTaskModal'
import TaskDetailPanel from '../../components/task/TaskDetailPanel'

const TASK_EVENT_TYPES = new Set([
  'task.created', 'task.updated', 'task.moved', 'task.status.changed',
  'task.assigned', 'task.unassigned', 'task.deleted',
])

export default function BoardPage() {
  const { boardId } = useParams<{ boardId: string }>()
  const { selectedTaskId, selectTask } = useUIStore()
  const [showCreateTask, setShowCreateTask] = useState(false)
  const qc = useQueryClient()

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

  // Live updates: another tab/user moving, editing, or deleting a task on this
  // board refreshes the list here too.
  useWebSocket((msg) => {
    if (msg.type === 'event' && TASK_EVENT_TYPES.has(msg.data?.event_type)) {
      qc.invalidateQueries({ queryKey: ['tasks', boardId] })
    }
  }, !!boardId, boardId ? [`board:${boardId}`] : [])

  const board = boardData?.data
  const tasks = tasksData?.data ?? []

  const boardType = useBoardType(board?.type)
  const presentation = boardType?.presentation
  const { Component: View, known } = resolveView(presentation?.view)

  if (!board) return null

  const viewLabel = presentation?.view ?? 'board'

  return (
    <div className="h-full flex flex-col">
      {/* Board toolbar */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-border-1 flex-shrink-0">
        <span className="text-xs text-text-3">{tasks.length} tasks</span>
        <span className="text-[11px] text-text-3 capitalize bg-bg-3 px-1.5 py-0.5 rounded">{viewLabel} view</span>
        {!known && presentation?.view && (
          <span className="flex items-center gap-1 text-[11px] text-amber">
            <Info size={11} /> Unknown view "{presentation.view}" — showing board
          </span>
        )}
        <button onClick={() => setShowCreateTask(true)} className="btn-primary flex items-center gap-1.5 ml-auto py-1 px-3 text-xs">
          <Plus size={13} /> New task
        </button>
      </div>

      {/* Selected renderer */}
      <View board={board} tasks={tasks} presentation={presentation} />

      {/* Create task (view-agnostic) */}
      {showCreateTask && (
        <CreateTaskModal board={board} onClose={() => setShowCreateTask(false)} />
      )}

      {/* Task detail slide-over (shared across all views) */}
      {selectedTaskId && (
        <TaskDetailPanel taskId={selectedTaskId} onClose={() => selectTask(null)} />
      )}
    </div>
  )
}
