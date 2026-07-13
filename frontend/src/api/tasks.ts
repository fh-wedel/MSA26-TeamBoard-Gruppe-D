import { api } from './client'
import type { ApiItem, ApiList, Attachment, Comment, HistoryEntry, Priority, Task, TaskStatus } from './types'

export const tasksApi = {
  list: (boardId: string, params?: { column_id?: string; status?: string; limit?: number; cursor?: string }) => {
    const qs = new URLSearchParams()
    if (params?.column_id) qs.set('column_id', params.column_id)
    if (params?.status)    qs.set('status', params.status)
    if (params?.limit)     qs.set('limit', String(params.limit))
    if (params?.cursor)    qs.set('cursor', params.cursor)
    const q = qs.toString()
    return api.get<ApiList<Task>>(`/boards/${boardId}/tasks${q ? `?${q}` : ''}`)
  },

  get: (taskId: string) =>
    api.get<ApiItem<Task>>(`/tasks/${taskId}`),

  create: (boardId: string, data: {
    title: string
    description?: string
    priority?: Priority
    column_id?: string
    assignee_id?: string
    due_date?: string
    start_date?: string
    labels?: string[]
  }) => api.post<ApiItem<Task>>(`/boards/${boardId}/tasks`, data),

  update: (taskId: string, patch: {
    title?: string
    description?: string
    priority?: Priority
    status?: TaskStatus
    due_date?: string | null
    start_date?: string | null
    labels?: string[]
  }) => api.patch<ApiItem<Task>>(`/tasks/${taskId}`, patch),

  delete: (taskId: string) =>
    api.delete<void>(`/tasks/${taskId}`),

  move: (taskId: string, column_id: string, before_id?: string, after_id?: string) =>
    api.post<ApiItem<Task>>(`/tasks/${taskId}/move`, { column_id, before_id, after_id }),

  assign: (taskId: string, assignee_id: string | null) =>
    api.post<ApiItem<Task>>(`/tasks/${taskId}/assign`, { assignee_id }),

  listComments: (taskId: string) =>
    api.get<ApiList<Comment>>(`/tasks/${taskId}/comments`),

  createComment: (taskId: string, body: string) =>
    api.post<ApiItem<Comment>>(`/tasks/${taskId}/comments`, { body }),

  updateComment: (taskId: string, commentId: string, body: string) =>
    api.patch<ApiItem<Comment>>(`/tasks/${taskId}/comments/${commentId}`, { body }),

  deleteComment: (taskId: string, commentId: string) =>
    api.delete<void>(`/tasks/${taskId}/comments/${commentId}`),

  listAttachments: (taskId: string) =>
    api.get<ApiList<Attachment>>(`/tasks/${taskId}/attachments`),

  addAttachment: (taskId: string, document_id: string) =>
    api.post<ApiItem<Attachment>>(`/tasks/${taskId}/attachments`, { document_id }),

  removeAttachment: (taskId: string, attachmentId: string) =>
    api.delete<void>(`/tasks/${taskId}/attachments/${attachmentId}`),

  getHistory: (taskId: string) =>
    api.get<ApiList<HistoryEntry>>(`/tasks/${taskId}/history`),
}
