import { api } from './client'
import type { ApiItem, ApiList, Webhook, WebhookDelivery } from './types'

export const webhooksApi = {
  list: (projectId: string) =>
    api.get<ApiList<Webhook>>(`/projects/${projectId}/webhooks`),

  create: (projectId: string, data: { target_url: string; description: string; event_filter: string[] }) =>
    api.post<ApiItem<Webhook & { secret: string }>>(`/projects/${projectId}/webhooks`, data),

  get: (id: string) =>
    api.get<ApiItem<Webhook>>(`/webhooks/${id}`),

  update: (id: string, patch: { target_url?: string; description?: string; event_filter?: string[]; active?: boolean }) =>
    api.patch<ApiItem<Webhook>>(`/webhooks/${id}`, patch),

  delete: (id: string) =>
    api.delete<void>(`/webhooks/${id}`),

  enable: (id: string)  => api.post<ApiItem<Webhook>>(`/webhooks/${id}/enable`, {}),
  disable: (id: string) => api.post<ApiItem<Webhook>>(`/webhooks/${id}/disable`, {}),
  test: (id: string)    => api.post<void>(`/webhooks/${id}/test`, {}),
  rotateSecret: (id: string) => api.post<ApiItem<{ secret: string }>>(`/webhooks/${id}/rotate-secret`, {}),

  listDeliveries: (id: string) =>
    api.get<ApiList<WebhookDelivery>>(`/webhooks/${id}/deliveries`),

  getDelivery: (webhookId: string, deliveryId: string) =>
    api.get<ApiItem<WebhookDelivery>>(`/webhooks/${webhookId}/deliveries/${deliveryId}`),
}
