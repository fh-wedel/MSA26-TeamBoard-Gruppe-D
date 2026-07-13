import { api } from './client'
import type { ApiItem, ApiList, PersonalAccessToken } from './types'

export const tokensApi = {
  list: () =>
    api.get<ApiList<PersonalAccessToken>>('/auth/tokens'),

  create: (data: { name: string; expires_in_days: number }) =>
    api.post<ApiItem<PersonalAccessToken & { token: string }>>('/auth/tokens', data),

  revoke: (id: string) =>
    api.delete<void>(`/auth/tokens/${id}`),
}
