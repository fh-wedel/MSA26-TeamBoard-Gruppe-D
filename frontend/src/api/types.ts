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

// Board types are runtime data owned by the Board Registry service — the set is
// open, so this is a free-form slug rather than a fixed union.
export type BoardType = string

// JSON-Schema fragment as served by the Board Registry (config_schema). Only the
// subset the frontend renders a form for is typed; unknown keys are tolerated.
export interface JSONSchema {
  type?: string
  properties?: Record<string, JSONSchemaProperty>
  required?: string[]
  additionalProperties?: boolean
}

export interface JSONSchemaProperty {
  type?: 'string' | 'integer' | 'number' | 'boolean'
  title?: string
  description?: string
  enum?: (string | number)[]
  default?: unknown
  minimum?: number
  maximum?: number
}

export interface BoardTypeColumnDef {
  name: string
  position: number
  wip_limit?: number
  status: TaskStatus
}

// ── Presentation (declarative rendering hints carried by a board type) ──────────
// `view` selects which built-in frontend renderer to use; `view_config` parametrizes
// it; `card` controls how a task card looks across views. Validated server-side
// against a host-defined meta-schema. Unknown/empty `view` falls back to the board.
export type ViewKind = 'board' | 'calendar' | 'timeline'

export type CardColorBy = 'priority' | 'status' | 'label'

export interface CardSpec {
  fields?: string[]
  color_by?: CardColorBy
}

export interface ViewConfig {
  // board
  group_by?: 'column' | 'assignee' | 'priority'
  show_wip?: boolean
  swimlane_by?: string | null
  // calendar
  date_field?: string
  week_start?: 'monday' | 'sunday'
  default_range?: 'month' | 'week'
  // timeline
  start_field?: string
  end_field?: string
  color_by?: CardColorBy
}

export interface Presentation {
  view?: ViewKind
  view_config?: ViewConfig
  card?: CardSpec
}

// A board-type definition as returned by GET /board-types (Board Registry).
export interface BoardTypeDef {
  type: string
  display_name: string
  icon: string
  default_columns: BoardTypeColumnDef[]
  default_config: Record<string, unknown>
  config_schema: JSONSchema
  presentation?: Presentation
  built_in: boolean
  created_by?: string
  created_at: string
  updated_at: string
}

export interface Column {
  id: string
  board_id: string
  name: string
  position: number
  wip_limit?: number
  status?: TaskStatus
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
  start_date?: string
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
