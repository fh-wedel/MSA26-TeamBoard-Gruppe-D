// ── Auth ──────────────────────────────────────────────────────────────────────
export interface User {
  id: string
  email: string
  created_at: string
  last_login_at?: string
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

// ── Projects ──────────────────────────────────────────────────────────────────
export type Role = 'owner' | 'editor' | 'viewer'

export interface Project {
  id: string
  name: string
  description: string
  owner_id: string
  created_at: string
  updated_at: string
}

export interface Member {
  project_id: string
  user_id: string
  email?: string
  role: Role
  invited_by?: string
  joined_at: string
}

export type BoardType = 'kanban' | 'scrum' | 'calendar'

export interface Column {
  id: string
  board_id: string
  name: string
  position: number
  wip_limit?: number
}

export interface Board {
  id: string
  project_id: string
  name: string
  type: BoardType
  position: number
  config: Record<string, unknown>
  created_by: string
  created_at: string
  updated_at: string
  columns?: Column[]
}

// ── Tasks ─────────────────────────────────────────────────────────────────────
export type TaskStatus = 'open' | 'in_progress' | 'blocked' | 'done' | 'archived'
export type Priority = 'low' | 'medium' | 'high' | 'critical'

export interface Task {
  id: string
  board_id: string
  project_id: string
  column_id?: string
  title: string
  description: string
  status: TaskStatus
  priority: Priority
  assignee_id?: string
  due_date?: string
  labels: string[]
  position: string
  created_by: string
  created_at: string
  updated_at: string
  comment_count: number
  attachment_count: number
}

export interface Comment {
  id: string
  task_id: string
  author_id: string
  body: string
  mentions: string[]
  edited_at?: string
  created_at: string
}

export interface Attachment {
  id: string
  task_id: string
  document_id: string
  added_by: string
  added_at: string
}

export interface HistoryEntry {
  id: string
  task_id: string
  actor_id: string
  change_type: string
  diff: Record<string, unknown>
  occurred_at: string
}

// ── Documents ─────────────────────────────────────────────────────────────────
export interface Document {
  id: string
  project_id: string
  name: string
  content_type: string
  current_version?: number
  status: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface DocumentVersion {
  id: string
  document_id: string
  version_number: number
  content_type: string
  size_bytes: number
  status: string
  uploaded_by: string
  uploaded_at?: string
  created_at: string
}

export interface UploadInitiation {
  document: Document
  version: DocumentVersion
  upload_url: string
  expires_at: string
}

export interface DownloadInfo {
  document: Document
  version: DocumentVersion
  download_url: string
  expires_at: string
}

// ── Notifications ─────────────────────────────────────────────────────────────
export interface Notification {
  id: string
  type: string
  payload: Record<string, unknown>
  project_id?: string
  created_at: string
  read_at?: string
}

// ── Webhooks ──────────────────────────────────────────────────────────────────
export interface Webhook {
  id: string
  project_id: string
  target_url: string
  description: string
  event_filter: string[]
  active: boolean
  created_by: string
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  webhook_id: string
  event_id: string
  event_type: string
  status: string
  attempt_count: number
  next_attempt_at?: string
  last_response_status?: number
  last_response_body?: string
  last_error?: string
  last_attempted_at?: string
  delivered_at?: string
  duration_ms?: number
  created_at: string
}

// ── API wrappers ──────────────────────────────────────────────────────────────
export interface ApiList<T> {
  data: T[]
  pagination?: { next_cursor?: string }
}

export interface ApiItem<T> {
  data: T
}
