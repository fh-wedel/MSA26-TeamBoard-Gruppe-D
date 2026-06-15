import { api } from './client'
import type { ApiItem, ApiList, Board, BoardType, Column, Member, Project, Role } from './types'

export const projectsApi = {
  list: () =>
    api.get<ApiList<Project>>('/projects'),

  get: (id: string) =>
    api.get<ApiItem<Project>>(`/projects/${id}`),

  create: (name: string, description: string) =>
    api.post<ApiItem<Project>>('/projects', { name, description }),

  update: (id: string, patch: { name?: string; description?: string }) =>
    api.patch<ApiItem<Project>>(`/projects/${id}`, patch),

  delete: (id: string) =>
    api.delete<void>(`/projects/${id}`),

  listMembers: (projectId: string) =>
    api.get<ApiList<Member>>(`/projects/${projectId}/members`),

  addMember: (projectId: string, email: string, role: Role) =>
    api.post<ApiItem<Member>>(`/projects/${projectId}/members`, { email, role }),

  updateMemberRole: (projectId: string, userId: string, role: Role) =>
    api.patch<ApiItem<Member>>(`/projects/${projectId}/members/${userId}/role`, { role }),

  removeMember: (projectId: string, userId: string) =>
    api.delete<void>(`/projects/${projectId}/members/${userId}`),

  listBoards: (projectId: string) =>
    api.get<ApiList<Board>>(`/projects/${projectId}/boards`),

  createBoard: (projectId: string, name: string, type: BoardType, columns: { name: string; position: number }[]) =>
    api.post<ApiItem<Board>>(`/projects/${projectId}/boards`, { name, type, columns }),
}

export const boardsApi = {
  get: (boardId: string) =>
    api.get<ApiItem<Board>>(`/boards/${boardId}`),

  update: (boardId: string, patch: { name?: string; config?: Record<string, unknown> }) =>
    api.patch<ApiItem<Board>>(`/boards/${boardId}`, patch),

  delete: (boardId: string) =>
    api.delete<void>(`/boards/${boardId}`),

  listColumns: (boardId: string) =>
    api.get<ApiList<Column>>(`/boards/${boardId}/columns`),

  createColumn: (boardId: string, name: string, position: number, wip_limit?: number) =>
    api.post<ApiItem<Column>>(`/boards/${boardId}/columns`, { name, position, wip_limit }),

  listBoardTypes: () =>
    api.get<ApiList<{ type: string; label: string }>>('/board-types'),
}
