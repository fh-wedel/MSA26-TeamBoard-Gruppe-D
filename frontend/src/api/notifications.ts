import { api } from './client'
import type { ApiItem, ApiList, Notification } from './types'

export const notificationsApi = {
  list: (cursor?: string) => {
    const q = cursor ? `?cursor=${cursor}` : ''
    return api.get<ApiList<Notification>>(`/notifications${q}`)
  },

  unreadCount: () =>
    api.get<ApiItem<{ count: number }>>('/notifications/unread-count'),

  markRead: (id: string) =>
    api.post<void>(`/notifications/${id}/read`),

  markAllRead: () =>
    api.post<void>('/notifications/read-all'),

  delete: (id: string) =>
    api.delete<void>(`/notifications/${id}`),
}
