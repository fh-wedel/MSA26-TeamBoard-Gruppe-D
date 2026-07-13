import { api } from './client'
import type { ApiItem, ApiList, Document, DocumentVersion, DownloadInfo, UploadInitiation } from './types'

export const documentsApi = {
  list: (projectId: string, cursor?: string) => {
    const qs = new URLSearchParams({ project_id: projectId })
    if (cursor) qs.set('cursor', cursor)
    return api.get<ApiList<Document>>(`/documents?${qs}`)
  },

  get: (id: string) =>
    api.get<ApiItem<Document>>(`/documents/${id}`),

  initiateUpload: (project_id: string, name: string, content_type: string, size_bytes: number) =>
    api.post<ApiItem<UploadInitiation>>('/documents', { project_id, name, content_type, size_bytes }),

  confirmUpload: (documentId: string, version: number) =>
    api.post<ApiItem<Document>>(`/documents/${documentId}/versions/${version}:confirm`, {}),

  rename: (id: string, name: string) =>
    api.patch<ApiItem<Document>>(`/documents/${id}`, { name }),

  delete: (id: string) =>
    api.delete<void>(`/documents/${id}`),

  listVersions: (id: string) =>
    api.get<ApiList<DocumentVersion>>(`/documents/${id}/versions`),

  restoreVersion: (id: string, version: number) =>
    api.post<ApiItem<Document>>(`/documents/${id}/versions/${version}:restore`, {}),

  getDownloadUrl: (id: string) =>
    api.get<ApiItem<DownloadInfo>>(`/documents/${id}/download`),
}
