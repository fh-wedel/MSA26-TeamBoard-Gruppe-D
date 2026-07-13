import { api } from './client'
import type { Member } from './types'

export interface Invitation {
  id: string
  project_id: string
  invitee_email: string
  role: string
  token: string
  invited_by: string
  status: 'pending' | 'accepted' | 'declined'
  created_at: string
  expires_at: string
}

export const invitationsApi = {
  create: (projectId: string, email: string, role: string) =>
    api.post<{ data: Invitation }>(`/projects/${projectId}/invitations`, { email, role }),

  list: (projectId: string) =>
    api.get<{ data: Invitation[] }>(`/projects/${projectId}/invitations`),

  get: (token: string) =>
    api.get<{ data: Invitation }>(`/invitations/${token}`),

  accept: (token: string) =>
    api.post<{ data: Member }>(`/invitations/${token}/accept`, {}),

  decline: (token: string) =>
    api.post<void>(`/invitations/${token}/decline`, {}),
}
