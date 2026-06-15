import { api } from './client'
import type { ApiItem, TokenPair, User } from './types'

export const authApi = {
  register: (email: string, password: string) =>
    api.post<ApiItem<{ user: User; tokens: TokenPair }>>('/auth/register', { email, password }),

  login: (email: string, password: string) =>
    api.post<ApiItem<TokenPair>>('/auth/login', { email, password }),

  refresh: (refresh_token: string) =>
    api.post<ApiItem<TokenPair>>('/auth/refresh', { refresh_token }),

  logout: (refresh_token: string) =>
    api.post<void>('/auth/logout', { refresh_token }),

  me: () =>
    api.get<ApiItem<User>>('/auth/me'),

  requestPasswordReset: (email: string) =>
    api.post<void>('/auth/password-reset/request', { email }),

  confirmPasswordReset: (token: string, new_password: string) =>
    api.post<void>('/auth/password-reset/confirm', { token, new_password }),
}
