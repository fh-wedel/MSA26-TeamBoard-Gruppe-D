# Task Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Verwandtes Dokument:** [`services/project.md`](./project.md) — Authority für Permissions  
> **Service:** `task`  
> **Port (lokal):** 8003  
> **Datenbank:** `task_db` (PostgreSQL)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Use-Cases](#2-use-cases)
3. [Datenmodell](#3-datenmodell)
4. [Domain-Modell](#4-domain-modell)
5. [Reihenfolge (Ordering) — Fractional Ranks](#5-reihenfolge-ordering--fractional-ranks)
6. [Status vs. Column](#6-status-vs-column)
7. [Authorization-Strategie](#7-authorization-strategie)
8. [Mentions in Kommentaren](#8-mentions-in-kommentaren)
9. [Anhänge (Attachments)](#9-anhänge-attachments)
10. [HTTP-API (OpenAPI)](#10-http-api-openapi)
11. [Events](#11-events)
12. [Konfiguration](#12-konfiguration)
13. [Verzeichnisstruktur](#13-verzeichnisstruktur)
14. [sqlc-Queries](#14-sqlc-queries)
15. [Test-Strategie](#15-test-strategie)
16. [Performance-Überlegungen](#16-performance-überlegungen)
17. [Implementierungs-Hinweise für Coding-Agents](#17-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Tasks** mit Titel, Beschreibung, Status, Zuweisung, Fälligkeitsdatum, Priorität, Labels
- **Statusübergänge** und Spaltenbewegungen (Drag-and-Drop in Kanban)
- **Reihenfolge innerhalb einer Spalte/Column** (Ordering)
- **Kommentare** an Tasks mit Mention-Extraktion
- **Anhang-Referenzen** auf Document Service (keine Bytes)
- **Audit-Historie** aller Task-Änderungen
- **Replikat-Cache der Boards/Spalten** für Validierung (kein erneutes DB-Lookup pro Operation beim Project Service)

### 1.2 Verantwortet NICHT

- **Boards selbst** — gehören zum Project Service
- **Board-Spalten** — gehören zum Project Service (Replikat hier nur read-only)
- **Dokumente** — gehören zum Document Service
- **User-Profile** — Auth Service ist Authority
- **Berechtigungen** — Project Service ist Authority

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `task_db` | hart | Persistenz |
| RabbitMQ | hart | Events publizieren + konsumieren |
| Project Service | hart bei Schreibops | Permission-Check (`/internal/projects/{id}/permissions/{userId}`) |
| Auth Service (JWKS) | hart | JWT-Validierung |
| Document Service | weich | Attachment-Validierung (Existenz + Project-Match) |

---

## 2. Use-Cases

### UC-1: Task erstellen
**Akteur:** User mit `task:create`  
**Ablauf:**
1. `POST /boards/{boardId}/tasks` mit `title`, `column_id`, optional `description`, `assignee_id`, `due_date`, `priority`, `labels`
2. Permission-Check beim Project Service (Board → Project-Lookup über lokales Replikat)
3. Validierung: `column_id` gehört zu `board_id`
4. `position` berechnen (Anhängen ans Ende der Spalte, fractional rank)
5. `status` ableiten aus `column_id` (siehe Abschnitt 6)
6. Insert in `tasks` + Outbox-Event `task.created` + History-Eintrag — alles in einer Transaktion
7. Response: 201 mit Task-Response

### UC-2: Task aktualisieren (allgemein)
**Akteur:** User mit `task:update`  
**Ablauf:**
1. `PATCH /tasks/{id}` mit Partial-Update
2. Permission-Check
3. Aktuellen Task laden, Diff berechnen
4. Update + Outbox-Event `task.updated` mit `changes`-Diff + History-Eintrag
5. Response: 200

**Special-Case:** Wenn `assignee_id` geändert wird, zusätzlich Event `task.assigned`. Wenn `column_id` geändert wird, zusätzlich Event `task.status.changed`.

### UC-3: Task in andere Spalte verschieben
**Akteur:** User mit `task:update`  
**Ablauf:**
1. `POST /tasks/{id}:move` mit `column_id`, `position` (fractional rank zwischen zwei bestehenden Tasks)
2. Permission-Check
3. Validierung: `column_id` gehört zu selbem Board
4. Update `column_id`, `position`, `status` (abgeleitet)
5. Outbox-Events `task.updated` + `task.status.changed` (falls Status sich ändert)
6. Response: 200

**Designentscheidung — separater Move-Endpoint:** Move ist die häufigste Operation bei Drag-and-Drop und verdient klare Semantik. Außerdem wird `position` nie "freistehend" gesetzt, sondern immer relativ zu Nachbarn — separate Endpoint vereinfacht den Vertrag.

### UC-4: Task zuweisen / Zuweisung entfernen
**Akteur:** User mit `task:update`  
**Ablauf:**
1. `POST /tasks/{id}:assign` mit `assignee_id` (oder `null` zum Entfernen)
2. Permission-Check
3. Validierung: `assignee_id` ist Mitglied im Projekt (über internen Project-Service-Call)
4. Update + Outbox-Events `task.updated` + `task.assigned` (oder `task.unassigned`)
5. Response: 200

### UC-5: Task löschen
**Akteur:** User mit `task:delete`  
**Ablauf:**
1. `DELETE /tasks/{id}`
2. Permission-Check
3. Soft-Delete (`deleted_at = NOW()`)
4. Outbox-Event `task.deleted`
5. Kommentare und Attachment-Refs bleiben technisch erhalten (für Audit), sind aber nur über deleted Task erreichbar
6. Response: 204

### UC-6: Tasks in Board auflisten
**Akteur:** User mit `task:read`  
**Ablauf:**
1. `GET /boards/{boardId}/tasks`
2. Optional: Filter `status`, `assignee_id`, `column_id`, `label`
3. Sortierung: nach `column_id`, dann `position`
4. Pagination: cursor-basiert
5. Response: 200 mit Liste

### UC-7: Einzelnen Task lesen
**Akteur:** User mit `task:read`  
**Ablauf:**
1. `GET /tasks/{id}`
2. Permission-Check
3. Response: 200 mit Task inkl. eingebetteter `comment_count` und `attachment_count`

### UC-8: Kommentar erstellen
**Akteur:** User mit `comment:create`  
**Ablauf:**
1. `POST /tasks/{id}/comments` mit `body` (Markdown-fähig)
2. Permission-Check
3. **Mention-Extraktion** aus `body` (Regex `@\b[\w.-]+\b` mit anschließender User-Auflösung — siehe Abschnitt 8)
4. Insert + Outbox-Event `task.commented` mit `mentions[]` + History-Eintrag
5. Response: 201

### UC-9: Kommentare auflisten
**Akteur:** User mit `comment:read`  
**Ablauf:**
1. `GET /tasks/{id}/comments`
2. Sortierung: chronologisch ASC
3. Response: 200

### UC-10: Kommentar bearbeiten/löschen
**Akteur:** Author des Kommentars ODER User mit `task:delete` (für Moderation)  
**Ablauf:**
1. `PATCH /comments/{id}` oder `DELETE /comments/{id}`
2. Author-Check ODER Permission-Check
3. Bei Edit: `edited_at` setzen, Original-Body in `comment_history`
4. Bei Delete: Soft-Delete
5. Response: 200/204

### UC-11: Anhang verknüpfen
**Akteur:** User mit `task:update`  
**Ablauf:**
1. `POST /tasks/{id}/attachments` mit `document_id`
2. Permission-Check
3. **Validierung beim Document Service:** Existiert Dokument? Gehört es zum selben Projekt? (Cross-Project-Attachments verhindern)
4. Insert in `task_attachments` + Outbox-Event `task.attachment.added`
5. Response: 201

### UC-12: Anhang entfernen
**Akteur:** User mit `task:update`  
**Ablauf:**
1. `DELETE /tasks/{id}/attachments/{attachmentId}`
2. Permission-Check
3. Hard-Delete der Verknüpfung (Dokument selbst bleibt unangetastet)
4. Outbox-Event `task.attachment.removed`
5. Response: 204

### UC-13: History eines Tasks ansehen
**Akteur:** User mit `task:read`  
**Ablauf:**
1. `GET /tasks/{id}/history`
2. Permission-Check
3. Response: chronologische Liste der Änderungen

---

## 3. Datenmodell

### 3.1 Tabellen

```sql
-- Replikat: Boards (gefüttert durch board.created/board.deleted Events)
-- Wird gespiegelt, um column-Validierung lokal zu ermöglichen ohne Cross-Service-DB-Zugriff
CREATE TABLE known_boards (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL,
    deleted_at      TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_boards_project ON known_boards (project_id) WHERE deleted_at IS NULL;

-- Replikat: Board-Spalten
CREATE TABLE known_columns (
    id              UUID PRIMARY KEY,
    board_id        UUID NOT NULL REFERENCES known_boards(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    position        INTEGER NOT NULL
);

CREATE INDEX idx_known_columns_board ON known_columns (board_id, position);

-- Replikat: Bekannte User (für Assignee-Validierung)
CREATE TABLE known_users (
    id              UUID PRIMARY KEY,
    email           CITEXT NOT NULL UNIQUE,
    deleted_at      TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_users_email_active ON known_users (email) WHERE deleted_at IS NULL;

-- Tasks
CREATE TABLE tasks (
    id              UUID PRIMARY KEY,
    board_id        UUID NOT NULL REFERENCES known_boards(id),
    project_id      UUID NOT NULL,                          -- denormalisiert für Authorization-Lookups
    column_id       UUID NULL REFERENCES known_columns(id), -- NULL bei Calendar-Boards
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    priority        TEXT NOT NULL DEFAULT 'medium',
    assignee_id     UUID NULL,
    due_date        TIMESTAMPTZ NULL,
    labels          TEXT[] NOT NULL DEFAULT '{}',
    position        TEXT NOT NULL,                          -- fractional rank, sortable lexicographically
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,
    
    CONSTRAINT tasks_title_length CHECK (char_length(title) BETWEEN 1 AND 500),
    CONSTRAINT tasks_description_length CHECK (char_length(description) <= 10000),
    CONSTRAINT tasks_status CHECK (status IN ('open', 'in_progress', 'blocked', 'done', 'archived')),
    CONSTRAINT tasks_priority CHECK (priority IN ('low', 'medium', 'high', 'critical'))
);

CREATE INDEX idx_tasks_board_active ON tasks (board_id, column_id, position) 
    WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_assignee ON tasks (assignee_id) 
    WHERE deleted_at IS NULL AND assignee_id IS NOT NULL;
CREATE INDEX idx_tasks_project ON tasks (project_id) 
    WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_due_date ON tasks (due_date) 
    WHERE deleted_at IS NULL AND due_date IS NOT NULL;
CREATE INDEX idx_tasks_labels ON tasks USING GIN (labels) 
    WHERE deleted_at IS NULL;

-- Task-Kommentare
CREATE TABLE task_comments (
    id              UUID PRIMARY KEY,
    task_id         UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL,
    body            TEXT NOT NULL,
    edited_at       TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,
    
    CONSTRAINT task_comments_body_length CHECK (char_length(body) BETWEEN 1 AND 10000)
);

CREATE INDEX idx_task_comments_task ON task_comments (task_id, created_at) WHERE deleted_at IS NULL;

-- Mentions in Kommentaren (1:n, für effiziente Lookups "alle Mentions an mich")
CREATE TABLE comment_mentions (
    comment_id      UUID NOT NULL REFERENCES task_comments(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    
    PRIMARY KEY (comment_id, user_id)
);

CREATE INDEX idx_comment_mentions_user ON comment_mentions (user_id);

-- Comment-Edit-History (für Audit + Anti-Abuse)
CREATE TABLE comment_history (
    id              UUID PRIMARY KEY,
    comment_id      UUID NOT NULL REFERENCES task_comments(id) ON DELETE CASCADE,
    body            TEXT NOT NULL,
    archived_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Anhänge (Verknüpfung Task <-> Document)
CREATE TABLE task_attachments (
    id              UUID PRIMARY KEY,
    task_id         UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    document_id     UUID NOT NULL,
    added_by        UUID NOT NULL,
    added_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE (task_id, document_id)
);

CREATE INDEX idx_task_attachments_task ON task_attachments (task_id);
CREATE INDEX idx_task_attachments_document ON task_attachments (document_id);

-- Task-History (Audit-Trail für alle Änderungen)
CREATE TABLE task_history (
    id              UUID PRIMARY KEY,
    task_id         UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id        UUID NOT NULL,
    change_type     TEXT NOT NULL,                          -- 'created', 'updated', 'moved', 'assigned', 'commented', 'deleted'
    diff            JSONB NOT NULL,                          -- {"field": {"from": "...", "to": "..."}}
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT task_history_change_type CHECK (change_type IN (
        'created', 'updated', 'moved', 'assigned', 'unassigned',
        'commented', 'attachment_added', 'attachment_removed', 'deleted'
    ))
);

CREATE INDEX idx_task_history_task ON task_history (task_id, occurred_at DESC);

-- Outbox
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

-- Idempotenz für konsumierte Events
CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 3.2 Migrations

```
services/task/migrations/
├── 0001_init.up.sql
└── 0001_init.down.sql
```

### 3.3 Designentscheidungen

**Warum `project_id` denormalisiert in `tasks`?**  
Authorization-Calls werden auf `(project_id, user_id)` getätigt. Wenn `project_id` nur über `boards.project_id` erreichbar wäre, bräuchte jeder Permission-Check einen JOIN. Denormalisierung kostet einen Wert pro Task und spart einen Index-Lookup pro Authorization. Konsistenz wird beim Board-Replikat garantiert (`board.project_id` ändert sich nie nach Erstellung).

**Warum sind Spalten als Replikat hier?**  
Validierung "gehört Spalte zum Board?" muss synchron beim Task-Create passieren. Cross-Service-Call wäre langsamer und macht Schreibpfade brüchig bei Project-Service-Ausfall. Replikat über Events ist eventually consistent — bei kurzen Race Conditions (neue Spalte vs. neuer Task) im schlechtesten Fall 400-Validation-Error, der vom Client nochmal versucht werden kann.

**Warum `position` als TEXT, nicht INTEGER?**  
Fractional Ranks (siehe Abschnitt 5). Mit INTEGER bräuchte man bei jedem Drag-and-Drop Renumbering aller nachfolgenden Tasks — quadratisches Verhalten. Mit Lexicographic-Strings (z. B. `"0|hzzzzz:"`) ist das O(1).

**Warum Hard-Delete für Comments und Attachments via `ON DELETE CASCADE`, aber Soft-Delete für Comments selbst?**  
Cascade tritt nur bei *Hard-Delete* eines Tasks ein. Da Tasks Soft-Delete nutzen, bleiben Comments und Attachments bei normaler Löschung erhalten. Cascade ist defensive Maßnahme für seltene Fälle (Compliance-bedingter Hard-Delete via Cleanup-Job).

**Warum `labels` als TEXT[] statt eigene Tabelle?**  
Labels sind im MVP einfache String-Tags ohne Metadaten. Postgres' Array-Typ mit GIN-Index ist effizient für Filter wie "alle Tasks mit Label 'urgent'". Eine eigene Tabelle wäre Overengineering. Wenn später strukturierte Labels (Farbe, Beschreibung) gebraucht werden, Migration zu eigener Tabelle möglich.

---

## 4. Domain-Modell

### 4.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Task struct {
    ID          uuid.UUID
    BoardID     uuid.UUID
    ProjectID   uuid.UUID
    ColumnID    *uuid.UUID
    Title       string
    Description string
    Status      Status
    Priority    Priority
    AssigneeID  *uuid.UUID
    DueDate     *time.Time
    Labels      []string
    Position    string                  // fractional rank
    CreatedBy   uuid.UUID
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}

type Comment struct {
    ID        uuid.UUID
    TaskID    uuid.UUID
    AuthorID  uuid.UUID
    Body      string
    Mentions  []uuid.UUID
    EditedAt  *time.Time
    CreatedAt time.Time
    DeletedAt *time.Time
}

type Attachment struct {
    ID         uuid.UUID
    TaskID     uuid.UUID
    DocumentID uuid.UUID
    AddedBy    uuid.UUID
    AddedAt    time.Time
}

type HistoryEntry struct {
    ID         uuid.UUID
    TaskID     uuid.UUID
    ActorID    uuid.UUID
    ChangeType ChangeType
    Diff       map[string]any
    OccurredAt time.Time
}

type Status string

const (
    StatusOpen       Status = "open"
    StatusInProgress Status = "in_progress"
    StatusBlocked    Status = "blocked"
    StatusDone       Status = "done"
    StatusArchived   Status = "archived"
)

type Priority string

const (
    PriorityLow      Priority = "low"
    PriorityMedium   Priority = "medium"
    PriorityHigh     Priority = "high"
    PriorityCritical Priority = "critical"
)

type ChangeType string

const (
    ChangeCreated           ChangeType = "created"
    ChangeUpdated           ChangeType = "updated"
    ChangeMoved             ChangeType = "moved"
    ChangeAssigned          ChangeType = "assigned"
    ChangeUnassigned        ChangeType = "unassigned"
    ChangeCommented         ChangeType = "commented"
    ChangeAttachmentAdded   ChangeType = "attachment_added"
    ChangeAttachmentRemoved ChangeType = "attachment_removed"
    ChangeDeleted           ChangeType = "deleted"
)
```

### 4.2 Service-Interface

```go
package domain

type TaskService interface {
    // Tasks
    CreateTask(ctx context.Context, requester uuid.UUID, input CreateTaskInput) (*Task, error)
    GetTask(ctx context.Context, taskID, requester uuid.UUID) (*Task, error)
    ListTasks(ctx context.Context, boardID, requester uuid.UUID, filter TaskFilter, page Pagination) ([]*Task, *Cursor, error)
    UpdateTask(ctx context.Context, taskID, requester uuid.UUID, patch TaskPatch) (*Task, error)
    MoveTask(ctx context.Context, taskID, requester uuid.UUID, columnID uuid.UUID, beforeID, afterID *uuid.UUID) (*Task, error)
    AssignTask(ctx context.Context, taskID, requester uuid.UUID, assigneeID *uuid.UUID) (*Task, error)
    DeleteTask(ctx context.Context, taskID, requester uuid.UUID) error
    
    // Comments
    CreateComment(ctx context.Context, taskID, requester uuid.UUID, body string) (*Comment, error)
    ListComments(ctx context.Context, taskID, requester uuid.UUID) ([]*Comment, error)
    UpdateComment(ctx context.Context, commentID, requester uuid.UUID, body string) (*Comment, error)
    DeleteComment(ctx context.Context, commentID, requester uuid.UUID) error
    
    // Attachments
    AddAttachment(ctx context.Context, taskID, requester, documentID uuid.UUID) (*Attachment, error)
    ListAttachments(ctx context.Context, taskID, requester uuid.UUID) ([]*Attachment, error)
    RemoveAttachment(ctx context.Context, attachmentID, requester uuid.UUID) error
    
    // History
    GetTaskHistory(ctx context.Context, taskID, requester uuid.UUID) ([]*HistoryEntry, error)
}

type CreateTaskInput struct {
    BoardID     uuid.UUID
    ColumnID    *uuid.UUID
    Title       string
    Description string
    Priority    Priority
    AssigneeID  *uuid.UUID
    DueDate     *time.Time
    Labels      []string
}

type TaskPatch struct {
    Title       *string
    Description *string
    Priority    *Priority
    DueDate     *time.Time      // explizit nullable: pointer-to-pointer wäre overkill, "DueDateSet" Flag in Custom-Patch
    DueDateSet  bool            // true wenn DueDate im Patch enthalten (auch bei nil = Löschen)
    Labels      *[]string
}

type TaskFilter struct {
    Status     *Status
    AssigneeID *uuid.UUID
    ColumnID   *uuid.UUID
    Label      *string
}

type Pagination struct {
    Limit  int
    Cursor *string
}

type Cursor struct {
    Position string
    ID       uuid.UUID
}
```

### 4.3 Domain-Fehler

```go
package domain

var (
    ErrTaskNotFound             = &Error{Code: "task_not_found"}
    ErrCommentNotFound          = &Error{Code: "comment_not_found"}
    ErrAttachmentNotFound       = &Error{Code: "attachment_not_found"}
    ErrBoardUnknown             = &Error{Code: "board_unknown"}              // Replikat noch nicht da
    ErrColumnNotInBoard         = &Error{Code: "column_not_in_board"}
    ErrAssigneeNotMember        = &Error{Code: "assignee_not_member"}
    ErrPermissionDenied         = &Error{Code: "permission_denied"}
    ErrNotCommentAuthor         = &Error{Code: "not_comment_author"}
    ErrDocumentNotInProject     = &Error{Code: "document_not_in_project"}
    ErrDocumentUnreachable      = &Error{Code: "document_unreachable"}        // Document Service down
    ErrInvalidPosition          = &Error{Code: "invalid_position"}
    ErrValidation               = &Error{Code: "validation_failed"}
)
```

---

## 5. Reihenfolge (Ordering) — Fractional Ranks

### 5.1 Problem

Drag-and-Drop in Kanban-Boards erfordert Reordering von Tasks innerhalb einer Spalte. Naive INTEGER-`position` zwingt zu Renumbering aller nachfolgenden Tasks beim Einfügen — O(n) pro Move, schlecht skalierbar bei großen Spalten.

### 5.2 Lösung: Lexicographic Fractional Ranks

Position als String, der lexikographisch sortierbar ist. Zwischen zwei Positionen lässt sich immer eine neue dazwischen erzeugen, ohne andere zu ändern.

**Bibliothek-Empfehlung:** `github.com/jbenet/go-base58` mit eigenem Wrapper, oder Lexorank-artige Implementierung.

**Beispiel-Algorithmus (vereinfacht, Base-62):**

```go
package ordering

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Between erzeugt eine Position zwischen prev und next.
// Beide können leer sein (= "anfang" oder "ende").
func Between(prev, next string) (string, error) {
    if prev == "" && next == "" {
        return "U", nil  // Mitte des Alphabets
    }
    if next == "" {
        return increment(prev), nil
    }
    if prev == "" {
        return decrement(next), nil
    }
    return midpoint(prev, next)
}

// Initial: erstes Element bekommt "U" (mitten im Alphabet)
// Anhängen ans Ende: increment der letzten Position
// Zwischen "U" und "V": "Um" (echter Mittelwert)
```

**Edge-Case Rebalancing:** Bei extrem dichter Packung (z. B. 100x in dieselbe Position einfügen) wachsen Strings unbegrenzt. Periodisches Rebalancing (Cleanup-Job): alle Positions in einer Spalte neu vergeben mit gleichen Abständen. Im MVP unkritisch.

### 5.3 API-Vertrag für Move

Client schickt **nicht** die neue Position, sondern den **Kontext** — welcher Task ist davor, welcher dahinter:

```http
POST /tasks/{id}:move
Content-Type: application/json

{
  "column_id": "uuid",
  "before_task_id": "uuid",   // Task, vor dem dieser eingefügt wird (optional)
  "after_task_id": "uuid"     // Task, nach dem dieser eingefügt wird (optional)
}
```

Server berechnet `position = Between(after.position, before.position)`. Vorteile:
- Server hat volle Kontrolle über das Ranking-Schema
- Client muss keine Position-Strings kennen
- Race Conditions sind unproblematisch — selbst wenn parallel zwei Tasks zwischen A und B eingefügt werden, bekommen sie unterschiedliche, gültige Positionen

### 5.4 Initialposition bei Create

Neuer Task wird ans Ende der Zielspalte angehängt: `position = Between(lastPosition, "")`.

---

## 6. Status vs. Column

### 6.1 Doppelter Wert

`tasks.column_id` und `tasks.status` sind verwandte, aber nicht identische Konzepte:

- **`column_id`** ist die UI-Position auf einem Board (Kanban: "To Do", "In Progress", "Done")
- **`status`** ist der semantische Zustand der Aufgabe (`open`, `in_progress`, `done`, …)

Bei einem Kanban-Board sind sie meistens deckungsgleich, aber:
- Calendar-Boards haben **keine Spalten** (`column_id IS NULL`), aber Tasks haben trotzdem einen Status
- Eine Spalte "Review" könnte semantisch immer noch `in_progress` sein
- Suchen/Filtern nach Status sollte boardunabhängig funktionieren

### 6.2 Mapping-Strategie

Beim Verschieben in eine Spalte wird der Status anhand des Spaltennamens abgeleitet (best-effort):

```go
func deriveStatus(columnName string) Status {
    name := strings.ToLower(columnName)
    switch {
    case strings.Contains(name, "done") || strings.Contains(name, "complete"):
        return StatusDone
    case strings.Contains(name, "progress") || strings.Contains(name, "doing"):
        return StatusInProgress
    case strings.Contains(name, "block"):
        return StatusBlocked
    case strings.Contains(name, "archive"):
        return StatusArchived
    default:
        return StatusOpen
    }
}
```

**Override:** Wenn Client beim Move explizit `status` setzt, hat das Vorrang. Sonst wird abgeleitet.

**Designentscheidung:** Bewusst einfach gehalten. Eine echte Lösung wäre konfigurierbares Mapping pro Board (`column_id` → `status`), z. B. in `boards.config`. Im MVP zu früh — beobachten, ob Bedarf entsteht.

---

## 7. Authorization-Strategie

### 7.1 Permission-Check vor jeder Operation

Helper-Funktion im Service:

```go
func (s *taskService) requirePermission(ctx context.Context, projectID, userID uuid.UUID, perm string) error {
    perms, err := s.projectClient.GetPermissions(ctx, projectID, userID)
    if err != nil {
        return fmt.Errorf("permission check: %w", err)
    }
    if !perms.Has(perm) {
        return ErrPermissionDenied
    }
    return nil
}
```

### 7.2 Project-Service-Client

```go
package projectclient

type Client interface {
    GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
}

type httpClient struct {
    baseURL    string
    httpClient *http.Client
    tokenSource ServiceTokenSource
    breaker    *gobreaker.CircuitBreaker
}
```

**Pflicht-Eigenschaften:**
- **Service-Token** im `Authorization`-Header (siehe Project Service Abschnitt 7.2)
- **Timeout** 200 ms (kurz, weil hot path)
- **Retry** maximal 1x bei Netzwerkfehlern, mit 50 ms Backoff
- **Circuit Breaker** öffnet bei >50% Fehlerrate über 30s — danach kurze Trip-Phase
- **Local Cache** in-memory mit TTL 30 s pro `(project_id, user_id)` (nicht Redis — der Project Service hat schon einen Redis-Cache, hier reicht Per-Pod-In-Memory)

### 7.3 Fail-Closed bei Project-Service-Ausfall

Wenn `GetPermissions` final fehlschlägt (alle Retries durch, Circuit Breaker offen):

- **Read-Operationen:** ggf. fail-open mit Logging als Warning erwägen — aber im MVP fail-closed (503) für Konsistenz
- **Write-Operationen:** **immer** fail-closed (503)

Alternative: Stale-Cache-Reads mit längerer TTL als Fallback. Bewusst nicht im MVP, weil das Stale-Permissions in Sicherheitsentscheidungen einführt.

### 7.4 Special-Cases

- **Ressourcen-Owner:** Comment-Author darf eigenen Comment editieren ohne `comment:create` Permission (allerdings nicht löschen — das braucht `task:delete` als Moderation oder Author-Status). MVP-Entscheidung: Author-Check zuerst, dann Permission.
- **Self-Assign:** Bei UC-4 darf User sich selbst zuweisen, auch ohne dass er sonst irgendwo Editor-Rechte braucht? **Nein**, im MVP nicht. Self-Assign kommt mit `task:update`.

---

## 8. Mentions in Kommentaren

### 8.1 Extraktion

Beim `CreateComment`/`UpdateComment` werden Mentions aus `body` extrahiert:

```go
var mentionRegex = regexp.MustCompile(`@([\w.-]+)`)

func extractMentions(body string) []string {
    matches := mentionRegex.FindAllStringSubmatch(body, -1)
    handles := make([]string, 0, len(matches))
    seen := make(map[string]bool)
    for _, m := range matches {
        h := m[1]
        if !seen[h] {
            handles = append(handles, h)
            seen[h] = true
        }
    }
    return handles
}
```

### 8.2 Auflösung zu User-IDs

Aktuell hat das System keine Handle-Spalte (User haben nur E-Mail). Drei Strategien:

**Strategie A (MVP):** `@user@example.com` — vollständige E-Mail. Hässlich, aber funktioniert ohne Schema-Erweiterung.

**Strategie B:** Lokal-Part der E-Mail als Handle (`@alice` für `alice@teamboard.local`). Konflikte bei gleichem Lokal-Part möglich.

**Strategie C (best):** `display_name`/`handle` Feld im Auth Service — erfordert dortige Erweiterung.

**MVP-Entscheidung:** Strategie A. Der Frontend-Editor präsentiert E-Mail-Picker, fügt komplette E-Mail nach `@` ein. Nicht schön zu lesen, aber eindeutig. Strategie C als Erweiterung dokumentiert.

### 8.3 Validierung

Nicht jede `@xyz` muss matchen. Wenn keine User-ID gefunden wird → Mention ignorieren, kein Fehler. Body bleibt unverändert (UI rendert nicht-aufgelöste Mentions als Plain-Text).

### 8.4 Speicherung

Aufgelöste User-IDs in `comment_mentions` einfügen. Im `task.commented`-Event wird `mentions: [user_id, ...]` mitgeschickt — der Notification Service nutzt das als Trigger für Push-Notifications.

---

## 9. Anhänge (Attachments)

### 9.1 Vertrag

Task Service speichert nur `(task_id, document_id)` Paare. Die Bytes liegen im Document Service / S3.

### 9.2 Validierung beim Hinzufügen

```go
func (s *taskService) AddAttachment(ctx context.Context, taskID, requester, documentID uuid.UUID) (*Attachment, error) {
    // 1. Task laden, Permission-Check
    task, err := s.repo.GetTask(ctx, taskID)
    if err != nil { return nil, err }
    if err := s.requirePermission(ctx, task.ProjectID, requester, "task:update"); err != nil {
        return nil, err
    }
    
    // 2. Document Service fragen: existiert? gehört zum selben Projekt?
    docInfo, err := s.documentClient.GetDocumentInfo(ctx, documentID)
    if err != nil {
        return nil, ErrDocumentUnreachable
    }
    if docInfo.ProjectID != task.ProjectID {
        return nil, ErrDocumentNotInProject
    }
    
    // 3. Insert + Outbox-Event in Transaktion
    return s.repo.CreateAttachment(ctx, taskID, documentID, requester)
}
```

### 9.3 Auswirkung von Document-Delete

Document Service publiziert `document.deleted`. Task Service konsumiert (optional, im MVP nicht zwingend) und entfernt verwaiste Attachments. Alternative: Beim Lesen von Attachments Filter "noch existierende Documents" — minimaler Aufwand. **MVP: Konsumieren und cleanup**, weil der Listener ohnehin existiert.

---

## 10. HTTP-API (OpenAPI)

Vollständige Spec in `docs/api/task.openapi.yaml`.

### 10.1 OpenAPI 3.1 (Auszug)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Task Service
  version: 1.0.0
servers:
  - url: http://localhost:8003/api/v1

paths:
  /boards/{boardId}/tasks:
    parameters:
      - $ref: '#/components/parameters/boardId'
    get:
      summary: List tasks in a board
      operationId: listTasks
      tags: [tasks]
      security: [{bearerAuth: []}]
      parameters:
        - in: query
          name: status
          schema: { type: string, enum: [open, in_progress, blocked, done, archived] }
        - in: query
          name: assignee_id
          schema: { type: string, format: uuid }
        - in: query
          name: column_id
          schema: { type: string, format: uuid }
        - in: query
          name: label
          schema: { type: string }
        - in: query
          name: limit
          schema: { type: integer, default: 50, maximum: 200 }
        - in: query
          name: cursor
          schema: { type: string }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskList' }
    post:
      summary: Create a task in a board
      operationId: createTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateTaskRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskResponse' }

  /tasks/{taskId}:
    parameters:
      - $ref: '#/components/parameters/taskId'
    get:
      summary: Get task details
      operationId: getTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskResponse' }
    patch:
      summary: Update task
      operationId: updateTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateTaskRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskResponse' }
    delete:
      summary: Delete task (soft)
      operationId: deleteTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /tasks/{taskId}:move:
    parameters:
      - $ref: '#/components/parameters/taskId'
    post:
      summary: Move task to another column / position
      operationId: moveTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/MoveTaskRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskResponse' }

  /tasks/{taskId}:assign:
    parameters:
      - $ref: '#/components/parameters/taskId'
    post:
      summary: Assign or unassign a task
      operationId: assignTask
      tags: [tasks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/AssignTaskRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/TaskResponse' }

  /tasks/{taskId}/comments:
    parameters:
      - $ref: '#/components/parameters/taskId'
    get:
      summary: List comments
      operationId: listComments
      tags: [comments]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CommentList' }
    post:
      summary: Create comment
      operationId: createComment
      tags: [comments]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateCommentRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CommentResponse' }

  /comments/{commentId}:
    parameters:
      - $ref: '#/components/parameters/commentId'
    patch:
      summary: Edit comment (author only)
      operationId: updateComment
      tags: [comments]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateCommentRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/CommentResponse' }
    delete:
      summary: Delete comment (author or moderator)
      operationId: deleteComment
      tags: [comments]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /tasks/{taskId}/attachments:
    parameters:
      - $ref: '#/components/parameters/taskId'
    get:
      summary: List attachments
      operationId: listAttachments
      tags: [attachments]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AttachmentList' }
    post:
      summary: Add attachment (link to existing document)
      operationId: addAttachment
      tags: [attachments]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/AddAttachmentRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/AttachmentResponse' }

  /tasks/{taskId}/attachments/{attachmentId}:
    parameters:
      - $ref: '#/components/parameters/taskId'
      - $ref: '#/components/parameters/attachmentId'
    delete:
      summary: Remove attachment
      operationId: removeAttachment
      tags: [attachments]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Removed }

  /tasks/{taskId}/history:
    parameters:
      - $ref: '#/components/parameters/taskId'
    get:
      summary: Get task change history
      operationId: getTaskHistory
      tags: [tasks]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/HistoryList' }

  /health/live:
    get:
      operationId: getLiveness
      responses:
        '200': { description: Live }

  /health/ready:
    get:
      operationId: getReadiness
      responses:
        '200': { description: Ready }
        '503': { description: Not ready }

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

  parameters:
    boardId:
      in: path
      name: boardId
      required: true
      schema: { type: string, format: uuid }
    taskId:
      in: path
      name: taskId
      required: true
      schema: { type: string, format: uuid }
    commentId:
      in: path
      name: commentId
      required: true
      schema: { type: string, format: uuid }
    attachmentId:
      in: path
      name: attachmentId
      required: true
      schema: { type: string, format: uuid }

  schemas:
    CreateTaskRequest:
      type: object
      required: [title]
      properties:
        title: { type: string, minLength: 1, maxLength: 500 }
        description: { type: string, maxLength: 10000 }
        column_id: { type: string, format: uuid }
        priority:
          type: string
          enum: [low, medium, high, critical]
          default: medium
        assignee_id: { type: string, format: uuid, nullable: true }
        due_date: { type: string, format: date-time, nullable: true }
        labels:
          type: array
          items: { type: string, maxLength: 50 }

    UpdateTaskRequest:
      type: object
      properties:
        title: { type: string, minLength: 1, maxLength: 500 }
        description: { type: string, maxLength: 10000 }
        priority: { type: string, enum: [low, medium, high, critical] }
        due_date: { type: string, format: date-time, nullable: true }
        labels:
          type: array
          items: { type: string, maxLength: 50 }

    MoveTaskRequest:
      type: object
      required: [column_id]
      properties:
        column_id: { type: string, format: uuid }
        before_task_id: { type: string, format: uuid, nullable: true }
        after_task_id: { type: string, format: uuid, nullable: true }

    AssignTaskRequest:
      type: object
      properties:
        assignee_id:
          type: string
          format: uuid
          nullable: true
          description: "null to unassign"

    CreateCommentRequest:
      type: object
      required: [body]
      properties:
        body: { type: string, minLength: 1, maxLength: 10000 }

    UpdateCommentRequest:
      type: object
      required: [body]
      properties:
        body: { type: string, minLength: 1, maxLength: 10000 }

    AddAttachmentRequest:
      type: object
      required: [document_id]
      properties:
        document_id: { type: string, format: uuid }

    Task:
      type: object
      required: [id, board_id, project_id, title, status, priority, position, created_by, created_at]
      properties:
        id: { type: string, format: uuid }
        board_id: { type: string, format: uuid }
        project_id: { type: string, format: uuid }
        column_id: { type: string, format: uuid, nullable: true }
        title: { type: string }
        description: { type: string }
        status: { type: string, enum: [open, in_progress, blocked, done, archived] }
        priority: { type: string, enum: [low, medium, high, critical] }
        assignee_id: { type: string, format: uuid, nullable: true }
        due_date: { type: string, format: date-time, nullable: true }
        labels:
          type: array
          items: { type: string }
        position: { type: string }
        created_by: { type: string, format: uuid }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
        comment_count: { type: integer }
        attachment_count: { type: integer }

    TaskResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Task' }

    TaskList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Task' }
        pagination: { $ref: '#/components/schemas/Pagination' }

    Comment:
      type: object
      required: [id, task_id, author_id, body, created_at]
      properties:
        id: { type: string, format: uuid }
        task_id: { type: string, format: uuid }
        author_id: { type: string, format: uuid }
        body: { type: string }
        mentions:
          type: array
          items: { type: string, format: uuid }
        edited_at: { type: string, format: date-time, nullable: true }
        created_at: { type: string, format: date-time }

    CommentResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Comment' }

    CommentList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Comment' }

    Attachment:
      type: object
      required: [id, task_id, document_id, added_by, added_at]
      properties:
        id: { type: string, format: uuid }
        task_id: { type: string, format: uuid }
        document_id: { type: string, format: uuid }
        added_by: { type: string, format: uuid }
        added_at: { type: string, format: date-time }

    AttachmentResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Attachment' }

    AttachmentList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Attachment' }

    HistoryEntry:
      type: object
      required: [id, task_id, actor_id, change_type, occurred_at]
      properties:
        id: { type: string, format: uuid }
        task_id: { type: string, format: uuid }
        actor_id: { type: string, format: uuid }
        change_type:
          type: string
          enum: [created, updated, moved, assigned, unassigned, commented, attachment_added, attachment_removed, deleted]
        diff: { type: object, additionalProperties: true }
        occurred_at: { type: string, format: date-time }

    HistoryList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/HistoryEntry' }

    Pagination:
      type: object
      properties:
        next_cursor: { type: string, nullable: true }
        limit: { type: integer }

    Problem:
      type: object
      required: [type, title, status]
      properties:
        type: { type: string, format: uri }
        title: { type: string }
        status: { type: integer }
        detail: { type: string }
        trace_id: { type: string }
```

### 10.2 Endpoint-Übersicht

| Methode | Pfad | Permission | Beschreibung |
|---------|------|------------|--------------|
| GET | `/boards/{boardId}/tasks` | `task:read` | Tasks im Board |
| POST | `/boards/{boardId}/tasks` | `task:create` | Task erstellen |
| GET | `/tasks/{taskId}` | `task:read` | Einzelner Task |
| PATCH | `/tasks/{taskId}` | `task:update` | Task ändern |
| DELETE | `/tasks/{taskId}` | `task:delete` | Task löschen |
| POST | `/tasks/{taskId}:move` | `task:update` | Verschieben |
| POST | `/tasks/{taskId}:assign` | `task:update` | Zuweisen |
| GET | `/tasks/{taskId}/comments` | `comment:read` | Kommentare |
| POST | `/tasks/{taskId}/comments` | `comment:create` | Kommentar erstellen |
| PATCH | `/comments/{commentId}` | Author oder `task:delete` | Edit |
| DELETE | `/comments/{commentId}` | Author oder `task:delete` | Löschen |
| GET | `/tasks/{taskId}/attachments` | `task:read` | Anhänge |
| POST | `/tasks/{taskId}/attachments` | `task:update` | Anhang hinzufügen |
| DELETE | `/tasks/{taskId}/attachments/{attachmentId}` | `task:update` | Entfernen |
| GET | `/tasks/{taskId}/history` | `task:read` | Historie |

---

## 11. Events

### 11.1 Publizierte Events

| Event-Type | Trigger | Payload-Felder |
|------------|---------|----------------|
| `task.created` | UC-1 | `task_id, board_id, project_id, column_id, title, assignee_id, due_date, priority, labels, created_by` |
| `task.updated` | UC-2 | `task_id, project_id, changes` (Diff-Map) |
| `task.moved` | UC-3 | `task_id, project_id, from_column_id, to_column_id, position` |
| `task.assigned` | UC-4 | `task_id, project_id, assignee_id, previous_assignee_id, assigned_by` |
| `task.unassigned` | UC-4 (mit null) | `task_id, project_id, previous_assignee_id, by` |
| `task.status.changed` | UC-2/UC-3 wenn status wechselt | `task_id, project_id, from, to` |
| `task.deleted` | UC-5 | `task_id, project_id, deleted_by` |
| `task.commented` | UC-8 | `task_id, project_id, comment_id, author_id, body_excerpt, mentions[]` |
| `task.comment.updated` | UC-10 | `task_id, project_id, comment_id` |
| `task.comment.deleted` | UC-10 | `task_id, project_id, comment_id` |
| `task.attachment.added` | UC-11 | `task_id, project_id, attachment_id, document_id, added_by` |
| `task.attachment.removed` | UC-12 | `task_id, project_id, attachment_id, document_id` |

**`body_excerpt`:** Erste 200 Zeichen des Comment-Body. Reicht für Notification-Preview, vermeidet PII-Volumen im Event-Bus.

### 11.2 Konsumierte Events

| Event-Type | Source | Reaktion |
|------------|--------|----------|
| `user.registered` | auth | INSERT in `known_users` |
| `user.deleted` | auth | UPDATE `known_users.deleted_at`. Tasks bleiben, aber `assignee_id` wird auf NULL gesetzt für betroffene Tasks (mit `task.unassigned`-Event) |
| `board.created` | project | INSERT in `known_boards` |
| `board.updated` | project | UPDATE in `known_boards` |
| `board.deleted` | project | UPDATE `known_boards.deleted_at`. Tasks im Board werden soft-deleted (mit `task.deleted` pro Task) |
| `column.created` | project | INSERT in `known_columns` |
| `column.updated` | project | UPDATE |
| `column.deleted` | project | DELETE; Tasks in Spalte → `column_id` auf NULL |
| `project.deleted` | project | UPDATE alle Tasks dieses Projekts auf `deleted_at = NOW()`, Outbox-Events `task.deleted` |
| `document.deleted` | document | DELETE betroffene `task_attachments`, Outbox-Event `task.attachment.removed` |

### 11.3 Idempotenz

Alle Konsumenten prüfen `processed_events` vor Verarbeitung.

### 11.4 Event-Reihenfolge

Bei Multi-Event-Operationen (z. B. Move generiert sowohl `task.moved` als auch `task.status.changed`) ist die Reihenfolge in der Outbox **wichtig**: Notification Service erwartet `task.moved` zuerst, dann optional `task.status.changed`. Outbox sortiert nach `occurred_at` (Microsecond-Auflösung in PostgreSQL `TIMESTAMPTZ` reicht in der Praxis).

---

## 12. Konfiguration

### 12.1 Environment-Variablen

```bash
SERVICE_NAME=task-service
SERVICE_PORT=8003
LOG_LEVEL=info

# Database
DB_URL=postgres://task:task@postgres:5432/task_db?sslmode=disable
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=10

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
RABBITMQ_CONSUMER_QUEUE=task-service-queue

# Auth (JWKS)
JWT_JWKS_URL=http://auth:8001/.well-known/jwks.json
JWT_ISSUER=https://auth.teamboard.local
JWT_AUDIENCE=teamboard-api
JWT_JWKS_CACHE_TTL=10m

# Project Service
PROJECT_SERVICE_URL=http://project:8002
PROJECT_SERVICE_TIMEOUT=200ms
PERMISSION_CACHE_TTL=30s

# Document Service
DOCUMENT_SERVICE_URL=http://document:8004
DOCUMENT_SERVICE_TIMEOUT=500ms

# Service-Token (für Service-zu-Service)
SERVICE_TOKEN_SECRET=<from-secrets-manager>

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=task-service
```

### 12.2 Config-Struct

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"task-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8003"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"50"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
    }

    RabbitMQ struct {
        URL           string `env:"RABBITMQ_URL,required"`
        Exchange      string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
        ConsumerQueue string `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"task-service-queue"`
    }

    JWT struct {
        JWKSURL      string        `env:"JWT_JWKS_URL,required"`
        Issuer       string        `env:"JWT_ISSUER,required"`
        Audience     string        `env:"JWT_AUDIENCE,required"`
        JWKSCacheTTL time.Duration `env:"JWT_JWKS_CACHE_TTL" envDefault:"10m"`
    }

    ProjectService struct {
        URL              string        `env:"PROJECT_SERVICE_URL,required"`
        Timeout          time.Duration `env:"PROJECT_SERVICE_TIMEOUT" envDefault:"200ms"`
        PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" envDefault:"30s"`
    }

    DocumentService struct {
        URL     string        `env:"DOCUMENT_SERVICE_URL,required"`
        Timeout time.Duration `env:"DOCUMENT_SERVICE_TIMEOUT" envDefault:"500ms"`
    }

    Security struct {
        ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }
}
```

---

## 13. Verzeichnisstruktur

```
services/task/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── middleware.go
│   │   ├── handlers_tasks.go
│   │   ├── handlers_comments.go
│   │   ├── handlers_attachments.go
│   │   ├── handlers_history.go
│   │   ├── handlers_health.go
│   │   ├── dto.go
│   │   └── generated.go                # oapi-codegen
│   ├── domain/
│   │   ├── task.go
│   │   ├── comment.go
│   │   ├── attachment.go
│   │   ├── history.go
│   │   ├── status_mapping.go            # column-name -> status
│   │   ├── mention.go                   # @-Mention-Parser
│   │   ├── service.go                   # Interface
│   │   ├── service_tasks.go             # Task-Use-Cases
│   │   ├── service_comments.go          # Comment-Use-Cases
│   │   ├── service_attachments.go
│   │   └── errors.go
│   ├── ordering/
│   │   ├── fractional_rank.go           # Lexorank-artige Implementierung
│   │   └── fractional_rank_test.go
│   ├── repository/
│   │   ├── db/                          # sqlc-generiert
│   │   ├── repository.go
│   │   └── postgres.go
│   ├── projectclient/
│   │   ├── client.go                    # Interface
│   │   ├── http_client.go               # HTTP-Implementation mit Circuit Breaker
│   │   └── permission_cache.go          # In-Memory-Cache
│   ├── documentclient/
│   │   ├── client.go
│   │   └── http_client.go
│   ├── events/
│   │   ├── publisher.go                 # Outbox -> RabbitMQ
│   │   ├── consumer.go                  # Routing zu Handlern
│   │   ├── handler_users.go             # user.* Events
│   │   ├── handler_boards.go            # board.* / column.*
│   │   ├── handler_projects.go          # project.deleted
│   │   ├── handler_documents.go         # document.deleted
│   │   └── envelope.go
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   └── 0001_init.down.sql
├── queries/
│   ├── tasks.sql
│   ├── comments.sql
│   ├── attachments.sql
│   ├── history.sql
│   ├── known_boards.sql
│   ├── known_columns.sql
│   ├── known_users.sql
│   ├── outbox.sql
│   └── processed_events.sql
├── sqlc.yaml
├── Dockerfile
├── go.mod
├── go.sum
├── .air.toml
├── Makefile
└── README.md
```

---

## 14. sqlc-Queries

### 14.1 `queries/tasks.sql`

```sql
-- name: CreateTask :one
INSERT INTO tasks (
    id, board_id, project_id, column_id, title, description,
    status, priority, assignee_id, due_date, labels, position, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTaskWithCounts :one
SELECT
    t.*,
    (SELECT COUNT(*) FROM task_comments c WHERE c.task_id = t.id AND c.deleted_at IS NULL) AS comment_count,
    (SELECT COUNT(*) FROM task_attachments a WHERE a.task_id = t.id) AS attachment_count
FROM tasks t
WHERE t.id = $1 AND t.deleted_at IS NULL;

-- name: ListTasksByBoard :many
SELECT * FROM tasks
WHERE board_id = $1 AND deleted_at IS NULL
  AND ($2::TEXT IS NULL OR status = $2)
  AND ($3::UUID IS NULL OR assignee_id = $3)
  AND ($4::UUID IS NULL OR column_id = $4)
  AND ($5::TEXT IS NULL OR $5 = ANY(labels))
  AND ($6::TEXT IS NULL OR position > $6)
ORDER BY position ASC, id ASC
LIMIT $7;

-- name: GetLastPositionInColumn :one
SELECT COALESCE(MAX(position), '') FROM tasks
WHERE board_id = $1 AND column_id = $2 AND deleted_at IS NULL;

-- name: GetPositionForRefs :one
-- Liefert Position-Strings der Referenz-Tasks für Move-Berechnung
SELECT
    (SELECT position FROM tasks WHERE id = $1 AND deleted_at IS NULL) AS before_pos,
    (SELECT position FROM tasks WHERE id = $2 AND deleted_at IS NULL) AS after_pos;

-- name: UpdateTask :one
UPDATE tasks SET
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    priority = COALESCE(sqlc.narg('priority'), priority),
    due_date = CASE WHEN sqlc.arg('due_date_set')::BOOLEAN THEN sqlc.narg('due_date') ELSE due_date END,
    labels = COALESCE(sqlc.narg('labels'), labels),
    updated_at = NOW()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: MoveTask :one
UPDATE tasks SET
    column_id = $2,
    position = $3,
    status = $4,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: AssignTask :one
UPDATE tasks SET
    assignee_id = $2,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteTask :exec
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteTasksByProject :many
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE project_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: SoftDeleteTasksByBoard :many
UPDATE tasks SET deleted_at = NOW(), updated_at = NOW()
WHERE board_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: ClearAssigneeForUser :many
UPDATE tasks SET assignee_id = NULL, updated_at = NOW()
WHERE assignee_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: NullifyColumnReferences :exec
UPDATE tasks SET column_id = NULL, updated_at = NOW()
WHERE column_id = $1;
```

### 14.2 `queries/comments.sql`

```sql
-- name: CreateComment :one
INSERT INTO task_comments (id, task_id, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetComment :one
SELECT * FROM task_comments
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListCommentsByTask :many
SELECT * FROM task_comments
WHERE task_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: UpdateComment :one
UPDATE task_comments SET body = $2, edited_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteComment :exec
UPDATE task_comments SET deleted_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: ArchiveCommentBody :exec
INSERT INTO comment_history (id, comment_id, body)
VALUES ($1, $2, $3);

-- name: AddMention :exec
INSERT INTO comment_mentions (comment_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ListMentionsByComment :many
SELECT user_id FROM comment_mentions
WHERE comment_id = $1;

-- name: ClearMentions :exec
DELETE FROM comment_mentions
WHERE comment_id = $1;
```

### 14.3 `queries/attachments.sql`

```sql
-- name: CreateAttachment :one
INSERT INTO task_attachments (id, task_id, document_id, added_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM task_attachments WHERE id = $1;

-- name: ListAttachmentsByTask :many
SELECT * FROM task_attachments
WHERE task_id = $1
ORDER BY added_at ASC;

-- name: DeleteAttachment :exec
DELETE FROM task_attachments WHERE id = $1;

-- name: DeleteAttachmentsByDocument :many
DELETE FROM task_attachments
WHERE document_id = $1
RETURNING id, task_id;
```

### 14.4 `queries/history.sql`

```sql
-- name: CreateHistoryEntry :exec
INSERT INTO task_history (id, task_id, actor_id, change_type, diff)
VALUES ($1, $2, $3, $4, $5);

-- name: ListTaskHistory :many
SELECT * FROM task_history
WHERE task_id = $1
ORDER BY occurred_at DESC
LIMIT $2;
```

### 14.5 `queries/known_boards.sql`

```sql
-- name: UpsertKnownBoard :exec
INSERT INTO known_boards (id, project_id, name, type)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, type = EXCLUDED.type;

-- name: GetKnownBoard :one
SELECT * FROM known_boards
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkBoardDeleted :exec
UPDATE known_boards SET deleted_at = NOW() WHERE id = $1;
```

### 14.6 `queries/known_columns.sql`

```sql
-- name: UpsertKnownColumn :exec
INSERT INTO known_columns (id, board_id, name, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, position = EXCLUDED.position;

-- name: GetKnownColumn :one
SELECT * FROM known_columns WHERE id = $1;

-- name: ColumnBelongsToBoard :one
SELECT EXISTS(SELECT 1 FROM known_columns WHERE id = $1 AND board_id = $2);

-- name: DeleteKnownColumn :exec
DELETE FROM known_columns WHERE id = $1;
```

### 14.7 `queries/known_users.sql`

```sql
-- name: UpsertKnownUser :exec
INSERT INTO known_users (id, email)
VALUES ($1, $2)
ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email;

-- name: KnownUserExists :one
SELECT EXISTS(SELECT 1 FROM known_users WHERE id = $1 AND deleted_at IS NULL);

-- name: MarkKnownUserDeleted :exec
UPDATE known_users SET deleted_at = NOW() WHERE id = $1;
```

### 14.8 `queries/outbox.sql`

```sql
-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: GetUnpublishedEvents :many
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY occurred_at ASC, id ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE outbox SET published_at = NOW() WHERE id = $1;
```

### 14.9 `queries/processed_events.sql`

```sql
-- name: WasEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1);

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1)
ON CONFLICT DO NOTHING;
```

### 14.10 `sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries"
    schema: "migrations"
    gen:
      go:
        package: "db"
        out: "internal/repository/db"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_types: true
        emit_interface: true
```

---

## 15. Test-Strategie

### 15.1 Unit-Tests (Domain)

**Coverage-Ziel:** ≥ 80% in `internal/domain/` und 100% in `internal/ordering/`

```go
func TestFractionalRankBetween(t *testing.T) {
    cases := []struct{
        prev, next, want string
    }{
        {"", "", "U"},                  // beide leer → Mitte
        {"U", "", anything},             // ans Ende
        {"", "U", anything},             // an den Anfang
        {"U", "V", "Um"},                // exakte Mitte
    }
    // Verifizieren: prev < got < next (lexikographisch)
}

func TestStatusMapping(t *testing.T) {
    require.Equal(t, StatusDone, deriveStatus("Done"))
    require.Equal(t, StatusInProgress, deriveStatus("In Progress"))
    require.Equal(t, StatusOpen, deriveStatus("Random"))
}

func TestExtractMentions(t *testing.T) {
    require.Equal(t, []string{"alice@x.de", "bob@y.de"},
        extractMentions("ping @alice@x.de and @bob@y.de again"))
    require.Equal(t, []string{"alice"}, extractMentions("@alice @alice"))
    require.Empty(t, extractMentions("no mentions here"))
}
```

### 15.2 Integration-Tests

```go
func TestTaskRepository_LifeCycle(t *testing.T) {
    pg := startPostgres(t)
    repo := NewPostgresRepository(pg.URL())
    
    t.Run("create with valid column", func(t *testing.T) { ... })
    t.Run("list filters by status and assignee", func(t *testing.T) { ... })
    t.Run("move updates position and status", func(t *testing.T) { ... })
    t.Run("soft delete excludes from list", func(t *testing.T) { ... })
}

func TestEventConsumer_BoardDeleted(t *testing.T) {
    // Setup: Board mit 5 Tasks
    // Trigger: board.deleted
    // Erwartung: 5 task.deleted Events in Outbox, alle Tasks soft-deleted
}

func TestEventConsumer_ProjectDeleted(t *testing.T) {
    // Setup: 3 Boards mit insgesamt 15 Tasks
    // Trigger: project.deleted
    // Erwartung: 15 task.deleted Events
}

func TestProjectClient_CircuitBreaker(t *testing.T) {
    // Mock Project Service der immer 500 zurückgibt
    // Nach 5 Calls: Circuit offen, 6. Call schlägt sofort fehl
}
```

### 15.3 End-to-End-Tests

Postman-Collection: `tests/postman/task.postman_collection.json`

**Pflichtszenarien:**

1. **Happy Path:** Login → Project erstellen → Board erstellen → Task erstellen → Comment hinzufügen → Move → Assign → Delete
2. **Permission-Enforcement:** Viewer versucht Task zu erstellen → 403
3. **Position-Math:** 3 Tasks erstellen, mittleren in Mitte verschieben, Reihenfolge prüfen
4. **Mentions:** Comment mit `@alice@example.com` erstellen, Event prüft `mentions` enthält Alice's User-ID
5. **Cross-Service-Cleanup:** Project löschen, prüfen dass alle Tasks soft-deleted sind
6. **Attachment-Validierung:** Document aus anderem Project verlinken → `document_not_in_project`

### 15.4 Load-Testing

`tests/k6/task_create_load.js`:

```javascript
import http from 'k6/http';
export const options = {
    scenarios: {
        ramping: {
            executor: 'ramping-arrival-rate',
            startRate: 10,
            timeUnit: '1s',
            stages: [
                { target: 100, duration: '30s' },
                { target: 100, duration: '60s' },
            ],
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],
        http_req_failed: ['rate<0.01'],
    },
};

export default function () {
    http.post(`${BASE_URL}/api/v1/boards/${BOARD_ID}/tasks`, JSON.stringify({
        title: `Load test ${__ITER}`,
    }), {
        headers: {
            Authorization: `Bearer ${TOKEN}`,
            'Content-Type': 'application/json',
        },
    });
}
```

---

## 16. Performance-Überlegungen

### 16.1 Hot Path: Task-List

`GET /boards/{boardId}/tasks` ist der häufigste Read. Optimierungen:

- **Index** `idx_tasks_board_active` mit `(board_id, column_id, position)` für direktes Sortier-Lookup ohne Sort-Operation
- **Cursor-Pagination** statt OFFSET (vermeidet "tiefe" Pagination-Probleme)
- **Counts (`comment_count`, `attachment_count`) per Subquery** akzeptabel bei <50 Tasks pro Page; bei größeren Boards ggf. asynchron pre-aggregiert

### 16.2 Hot Path: Task-Create

Pro Create:
- 1 Permission-Check (cached → sub-millisecond bei Hit)
- 1 INSERT in `tasks`
- 1 INSERT in `task_history`
- 1 INSERT in `outbox`

Alle in einer Transaktion. Erwartete p99 < 50 ms bei normaler Last.

### 16.3 Outbox-Worker-Performance

Outbox-Worker liest in Batches (z. B. 100 Events) und publiziert. RabbitMQ-Confirm-Mode für Verlässlichkeit. Bei sehr hohen Schreibraten (>1000 Events/s) horizontale Skalierung des Workers — `FOR UPDATE SKIP LOCKED` macht das automatisch sicher.

### 16.4 Indizes-Pflicht

| Index | Zweck |
|-------|-------|
| `(board_id, column_id, position)` | Task-List in Board |
| `assignee_id` | "Meine Tasks"-View (über alle Projekte) |
| `project_id` | Cleanup bei project.deleted, Authorization-Lookups |
| `due_date` | "Bald fällige Tasks"-Reports |
| `labels` GIN | Filter nach Label |

---

## 17. Implementierungs-Hinweise für Coding-Agents

### 17.1 Implementierungsreihenfolge

1. **Migrations + sqlc-Setup** — alle Tabellen, `make generate` läuft
2. **Ordering-Library** (`internal/ordering/`) — eigene Lib mit umfangreichen Property-Tests (lexikographisch zwischen prev und next)
3. **Domain-Modell ohne Service** — Entitäten, Mention-Parser, Status-Mapping mit Unit-Tests
4. **Repository-Layer** mit Testcontainers
5. **Project-Client** und **Document-Client** als Interfaces (echter HTTP-Client + Test-Stub)
6. **Event-Consumer** für `user.*`, `board.*`, `column.*`, `project.deleted`, `document.deleted`
7. **Service-Layer** (Use-Cases UC-1..UC-13) — pro UC ein Test
8. **HTTP-Handler**
9. **Outbox-Publisher**
10. **Wiring in `main.go`**
11. **End-to-End-Tests im Compose-Stack**

### 17.2 Verbindliche Konventionen

- **Permission-Check vor jeder schreibenden Operation.** Keine Ausnahme. Helper-Funktion benutzen.
- **Outbox + DB-Mutation in einer Transaktion.** Auch History-Eintrag in derselben Transaktion.
- **Status-Ableitung beim Move/Update**, wenn `column_id` sich ändert. Override durch explizit gesetzten Status erlaubt.
- **Mention-Extraktion strikt**: Nur Mentions, deren Handle/E-Mail in `known_users` auflösbar ist, werden gespeichert.
- **`project_id` immer aus dem Board ableiten** beim Create, niemals vom Client akzeptieren — Client kennt nur `board_id`.
- **Cursor-Format:** Base64-codiertes JSON `{"position": "...", "id": "..."}`. Bei Pagination beide Werte für stable ordering.

### 17.3 Typische Stolperfallen

- **`column_id` NULL bei Calendar-Boards:** Validierungslogik muss das berücksichtigen — `column_id` ist nicht immer Pflicht.
- **Move ohne `before` und `after`:** Bedeutet "ans Ende der neuen Spalte". Edge-Case explizit testen.
- **Race beim Replikat-Update:** Wenn `task.create` ankommt, bevor `column.created`-Event verarbeitet wurde → 400 mit `column_unknown`. Client soll retry mit kurzem Delay.
- **`due_date` Null-Semantik im PATCH:** "Feld nicht im Body" vs. "Feld auf null gesetzt = Datum löschen". `application/merge-patch+json` macht das mit JSON `null` explizit; im Go-Modell separates `DueDateSet bool`.
- **Comment-Edit ändert `mentions`:** Beim Edit alle alten Mentions löschen, neue extrahieren. Sonst sammeln sich verwaiste Mentions.
- **Position-Strings in Cursor:** Beim Vergleich `position > $cursor_pos` sortiert lexikographisch — was wir wollen, weil unser Ranking lexikographisch ist.
- **Attachment auf gelöschtes Document:** Konsumer-Race: Wenn `task.attachment.added` und `document.deleted` zeitnah, muss Konsumer von `document.deleted` idempotent sein.
- **Self-Assign-Permission:** Im MVP nicht erlaubt ohne `task:update`. Wenn später relevant: separater Use-Case.

### 17.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration test-ordering migrate generate run docker-build

build:
	go build -o bin/task ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/... ./internal/ordering/...

test-integration:
	go test -race ./internal/repository/... ./internal/events/...

test-ordering:
	go test -count=100 -run TestFractional ./internal/ordering/...

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/task.openapi.yaml > internal/api/generated.go

run:
	air

docker-build:
	docker build -t teamboard/task:latest .
```

### 17.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|-------------------|
| UC-1 Create Task | Permission geprüft, Spalte gehört zu Board, Position ans Ende, Status abgeleitet, Outbox-Event + History-Eintrag |
| UC-2 Update Task | Diff korrekt berechnet, mehrere Outbox-Events bei Multi-Field-Update (z. B. assignment + status) |
| UC-3 Move Task | Position lexikographisch zwischen `before` und `after`, Status ggf. neu abgeleitet |
| UC-4 Assign Task | Assignee ist Projekt-Mitglied (über internen Project-Service-Call), `task.assigned`-Event |
| UC-5 Delete Task | Soft-Delete, Comments und Attachments unangetastet |
| UC-6 List Tasks | Filter funktionieren, Cursor-Pagination liefert konsistente Reihenfolge |
| UC-7 Get Task | Counts (comments, attachments) korrekt |
| UC-8 Create Comment | Mentions extrahiert und gespeichert, im Event mitgeschickt |
| UC-9 List Comments | Sortiert nach `created_at ASC` |
| UC-10 Update/Delete Comment | Author oder `task:delete`, Edit archiviert alten Body |
| UC-11 Add Attachment | Document Service bestätigt Existenz und gleiches Projekt |
| UC-12 Remove Attachment | Hard-Delete der Verknüpfung, Dokument bleibt |
| UC-13 History | Chronologisch DESC, vollständige Diffs |

---

## Anhang A: Sequenzdiagramm — Task erstellen

```
Client          Gateway       Task Svc      Project Svc     Postgres        RabbitMQ
   │                │              │              │              │              │
   │ POST .../tasks │              │              │              │              │
   ├───────────────>│              │              │              │              │
   │                │ + JWT-Verify │              │              │              │
   │                ├─────────────>│              │              │              │
   │                │              │ Get column from known_columns               │
   │                │              ├─────────────────────────────>│              │
   │                │              │ Get permissions (cache miss)│              │
   │                │              ├─────────────>│              │              │
   │                │              │<─────────────┤              │              │
   │                │              │ BEGIN TX                    │              │
   │                │              │ Calc position (last+1)      │              │
   │                │              │ INSERT task                 │              │
   │                │              ├─────────────────────────────>│              │
   │                │              │ INSERT history              │              │
   │                │              ├─────────────────────────────>│              │
   │                │              │ INSERT outbox(task.created) │              │
   │                │              ├─────────────────────────────>│              │
   │                │              │ COMMIT                      │              │
   │                │              ├─────────────────────────────>│              │
   │                │ 201 + Task   │              │              │              │
   │                │<─────────────┤              │              │              │
   │ 201 + Task     │              │              │              │              │
   │<───────────────┤              │              │              │              │
   │                │              │ Outbox-Worker async         │              │
   │                │              │ SELECT FROM outbox          │              │
   │                │              ├─────────────────────────────>│              │
   │                │              │ Publish task.created                       │
   │                │              ├──────────────────────────────────────────>│
   │                │              │ UPDATE outbox SET published_at              │
   │                │              ├─────────────────────────────>│              │
```

## Anhang B: Sequenzdiagramm — Move Task mit Statuswechsel

```
Client       Task Svc                 Postgres                    RabbitMQ
   │              │                       │                            │
   │ POST .../move│                       │                            │
   ├─────────────>│                       │                            │
   │              │ requirePermission     │                            │
   │              │ (cache hit, ok)       │                            │
   │              │ GET task              │                            │
   │              ├──────────────────────>│                            │
   │              │ GET before/after pos  │                            │
   │              ├──────────────────────>│                            │
   │              │ Calc position (Between│                            │
   │              │  prev, next)          │                            │
   │              │ deriveStatus(newCol)  │                            │
   │              │ → status changed!     │                            │
   │              │ BEGIN TX              │                            │
   │              │ UPDATE task           │                            │
   │              ├──────────────────────>│                            │
   │              │ INSERT history(moved) │                            │
   │              ├──────────────────────>│                            │
   │              │ INSERT outbox         │                            │
   │              │  (task.moved)         │                            │
   │              ├──────────────────────>│                            │
   │              │ INSERT outbox         │                            │
   │              │  (task.status.changed)│                            │
   │              ├──────────────────────>│                            │
   │              │ COMMIT                │                            │
   │              ├──────────────────────>│                            │
   │ 200 + Task   │                       │                            │
   │<─────────────┤                       │                            │
   │              │                       │                            │
   │              │ Worker publishes      │                            │
   │              │ in occurred_at order: │                            │
   │              │ 1. task.moved         │                            │
   │              ├───────────────────────────────────────────────────>│
   │              │ 2. task.status.changed│                            │
   │              ├───────────────────────────────────────────────────>│
```

---

**Ende des Detail-Designs Task Service.**
