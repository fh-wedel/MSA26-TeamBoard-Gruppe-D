# Project Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Service:** `project`  
> **Port (lokal):** 8002  
> **Datenbank:** `project_db` (PostgreSQL)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Use-Cases](#2-use-cases)
3. [Datenmodell](#3-datenmodell)
4. [Domain-Modell](#4-domain-modell)
5. [Permission-Modell](#5-permission-modell)
6. [HTTP-API (OpenAPI)](#6-http-api-openapi)
7. [Interne API für Service-zu-Service](#7-interne-api-für-service-zu-service)
8. [Permission-Cache](#8-permission-cache)
9. [Board-Typen und Plugin-Hooks](#9-board-typen-und-plugin-hooks)
10. [Events](#10-events)
11. [Konfiguration](#11-konfiguration)
12. [Verzeichnisstruktur](#12-verzeichnisstruktur)
13. [sqlc-Queries](#13-sqlc-queries)
14. [Test-Strategie](#14-test-strategie)
15. [Implementierungs-Hinweise für Coding-Agents](#15-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Projekt-CRUD** mit Owner und Metadaten
- **Board-CRUD** innerhalb von Projekten (Container für Tasks)
- **Mitgliederverwaltung** mit Rollen pro Projekt
- **Authoritative Source für Permissions** — andere Services fragen hier an
- **Einladungs-Workflow** (Invite per E-Mail/User-ID)
- **Board-Spalten-Definitionen** (für Kanban: `todo`, `in_progress`, `done`; für Scrum: `backlog`, `sprint`, etc.)

### 1.2 Verantwortet NICHT

- **Tasks selbst** — gehören zum Task Service
- **Dokumente** — gehören zum Document Service  
- **Globale Benutzeridentität** — Auth Service ist Authority
- **Benutzerprofile** über User-ID und E-Mail hinaus (kommen aus User Events)

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `project_db` | hart | Persistenz |
| RabbitMQ | hart | Event-Publikation und Konsum |
| Redis | weich | Permission-Cache (Service funktioniert ohne, ist nur langsamer) |
| Auth Service | weich | JWT-Validierung über JWKS (gecached) |

---

## 2. Use-Cases

### UC-1: Projekt erstellen
**Akteur:** Eingeloggter User  
**Ablauf:**
1. Client schickt `POST /projects` mit `name`, `description`
2. Service erstellt Projekt mit `owner_id = current_user`
3. Service erstellt automatisch Mitgliedschaft `(project_id, current_user, role=owner)`
4. Outbox-Event `project.created` und `project.member.added`
5. Response: 201 mit Projekt-Response

**Vor-/Nachbedingungen:**
- Vor: User authentifiziert
- Nach: Projekt + Owner-Mitgliedschaft existieren konsistent (eine Transaktion)

### UC-2: Projekt umbenennen / aktualisieren
**Akteur:** User mit Rolle `owner` oder `editor`  
**Ablauf:**
1. `PATCH /projects/{id}` mit Teil-Update
2. Permission-Check: `project:update`
3. Update durchführen, Outbox-Event `project.updated`
4. Response: 200

### UC-3: Projekt löschen (Soft-Delete)
**Akteur:** Nur `owner`  
**Ablauf:**
1. `DELETE /projects/{id}`
2. Permission-Check: nur Owner darf
3. `deleted_at = NOW()` setzen
4. Outbox-Event `project.deleted` — andere Services hören mit und löschen ihre zugehörigen Daten (Tasks, Documents)
5. Response: 204

**Designentscheidung:** Soft-Delete im MVP. Hard-Delete später als Cleanup-Job nach Aufbewahrungsfrist (z. B. 30 Tage).

### UC-4: Mitglied hinzufügen
**Akteur:** `owner` (oder `editor` mit Erlaubnis, im MVP nur `owner`)  
**Ablauf:**
1. `POST /projects/{id}/members` mit `user_id` (oder `email`) und `role`
2. Permission-Check: `member:invite`
3. **Validierung gegen Auth Service:** Existiert User mit dieser E-Mail/ID? (Read-Only-Cache der bekannten User aus `user.registered`-Events)
4. Mitgliedschaft erstellen
5. Outbox-Event `project.member.added`
6. **Cache-Invalidierung** der Permission-Caches für diesen User auf diesem Projekt
7. Response: 201

### UC-5: Rolle ändern
**Akteur:** `owner`  
**Ablauf:**
1. `PATCH /projects/{id}/members/{userId}` mit `role`
2. Permission-Check: `member:update`
3. **Constraint:** Mindestens ein Owner muss bleiben — wenn der einzige Owner sich selbst degradieren will → 409
4. Update + Outbox-Event `project.member.role_changed`
5. Cache-Invalidierung
6. Response: 200

### UC-6: Mitglied entfernen
**Akteur:** `owner` oder das Mitglied selbst (Self-Leave)  
**Ablauf:**
1. `DELETE /projects/{id}/members/{userId}`
2. Permission-Check: `member:remove` ODER `userId == current_user`
3. Constraint wie UC-5: letzter Owner darf nicht raus
4. Soft-Delete der Mitgliedschaft (oder Hard-Delete — siehe Design-Note)
5. Outbox-Event `project.member.removed`
6. Cache-Invalidierung
7. Response: 204

**Design-Note Mitgliedschafts-Lifecycle:** Im MVP Hard-Delete. Bei Bedarf später `left_at` für Audit.

### UC-7: Board erstellen
**Akteur:** User mit `board:create`  
**Ablauf:**
1. `POST /projects/{projectId}/boards` mit `name`, `type` (Default: `kanban`), `columns` (optional)
2. Permission-Check
3. Wenn `columns` nicht gesetzt: Defaults pro `type` einsetzen (Strategy-Pattern, siehe Abschnitt 9)
4. Insert + Outbox-Event `board.created`
5. Response: 201

### UC-8: Eigene Projekte auflisten
**Akteur:** Eingeloggter User  
**Ablauf:**
1. `GET /projects` (kein Pfad-Parameter)
2. Liefert alle Projekte, in denen User Mitglied ist
3. Response: 200 mit Liste

### UC-9: Permission-Check (intern)
**Akteur:** Anderer Service (Task, Document, Notification)  
**Ablauf:**
1. `GET /internal/projects/{projectId}/permissions/{userId}`
2. Service-Token-Validation
3. Cache-Lookup in Redis
4. Bei Cache-Miss: DB-Lookup nach `(project_id, user_id)` → Rolle → Permissions ableiten
5. Response: `{role, permissions[]}` mit kurzem Cache-Header

---

## 3. Datenmodell

### 3.1 Tabellen

```sql
-- Bekannte Benutzer (Replikat aus user.registered Events)
-- Einzige sinnvolle Form von "Cross-Service-Daten" hier:
-- minimaler Spiegel der Identitäten, gefüttert durch Events
CREATE TABLE known_users (
    id              UUID PRIMARY KEY,
    email           CITEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL,
    deleted_at      TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_users_email_active 
    ON known_users (email) WHERE deleted_at IS NULL;

-- Projekte
CREATE TABLE projects (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    owner_id        UUID NOT NULL,                    -- referenziert User (kein FK auf known_users, weil User in eigenem Service)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,
    
    CONSTRAINT projects_name_length CHECK (char_length(name) BETWEEN 1 AND 200)
);

CREATE INDEX idx_projects_owner ON projects (owner_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_active ON projects (id) WHERE deleted_at IS NULL;

-- Rollen-Enum als Lookup-Tabelle (statt PostgreSQL ENUM, für einfachere Erweiterbarkeit)
CREATE TABLE roles (
    name            TEXT PRIMARY KEY,
    rank            INTEGER NOT NULL UNIQUE,           -- höher = mehr Rechte (für Constraint-Checks)
    description     TEXT NOT NULL DEFAULT ''
);

INSERT INTO roles (name, rank, description) VALUES
    ('viewer', 10, 'Read-only access'),
    ('editor', 20, 'Can modify content but not project settings'),
    ('owner',  30, 'Full control including member management and deletion');

-- Mitgliedschaften
CREATE TABLE project_members (
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL,
    role            TEXT NOT NULL REFERENCES roles(name),
    invited_by      UUID NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user ON project_members (user_id);
CREATE INDEX idx_project_members_project ON project_members (project_id);

-- Boards
CREATE TABLE boards (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL DEFAULT 'kanban',
    position        INTEGER NOT NULL DEFAULT 0,
    config          JSONB NOT NULL DEFAULT '{}'::JSONB, -- type-spezifische Konfiguration
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,
    
    CONSTRAINT boards_type CHECK (type IN ('kanban', 'scrum', 'calendar')),
    CONSTRAINT boards_name_length CHECK (char_length(name) BETWEEN 1 AND 100)
);

CREATE INDEX idx_boards_project ON boards (project_id, position) WHERE deleted_at IS NULL;

-- Board-Spalten (für Kanban/Scrum)
-- Eigenständige Tabelle, weil Spalten ändern sich häufiger als Boards selbst
CREATE TABLE board_columns (
    id              UUID PRIMARY KEY,
    board_id        UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    position        INTEGER NOT NULL,
    wip_limit       INTEGER NULL,                      -- optional: Work-in-Progress-Limit
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT board_columns_name_length CHECK (char_length(name) BETWEEN 1 AND 50),
    CONSTRAINT board_columns_unique_position UNIQUE (board_id, position)
);

CREATE INDEX idx_board_columns_board ON board_columns (board_id, position);

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

-- Idempotenz-Tracking für konsumierte Events
CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 3.2 Migrations

```
services/project/migrations/
├── 0001_init.up.sql              # Tabellen oben
├── 0001_init.down.sql
├── 0002_seed_roles.up.sql        # Rollen einfügen
└── 0002_seed_roles.down.sql
```

### 3.3 Designentscheidungen

**Warum keine FK von `projects.owner_id` auf `known_users.id`?**  
Bewusste Auflockerung. Der Owner-Bezug muss auch dann funktionieren, wenn das User-Event noch nicht angekommen ist (Race-Condition beim ersten Login → Project-Create). Die `known_users`-Tabelle ist Cache und Validierungsquelle, nicht Source of Truth. Constraints wie "Owner existiert" werden auf Service-Ebene geprüft, nicht in der DB.

**Warum Rollen als Lookup-Tabelle, nicht als PostgreSQL ENUM?**  
Erweiterbarkeit. Eine neue Rolle "guest" hinzuzufügen ist eine reine Insert-Operation, kein Schema-Change. ENUMs in PG erfordern `ALTER TYPE`, was Lock-Probleme verursachen kann.

**Warum eigene `board_columns`-Tabelle, nicht JSON-Array in `boards`?**  
Spalten haben Identität (UUID), werden referenziert von Tasks (Task hat `column_id`). JSON-Arrays sind schlecht referenzierbar. Außerdem ändern sich Spalten häufig — die Update-Granularität wäre bei JSON suboptimal.

**Warum `config JSONB` auf `boards`?**  
Type-spezifische Konfiguration (z. B. Sprint-Länge bei Scrum, Wochenstart bei Calendar). Schemaloser Bereich pro Board-Typ. Das Datenmodell selbst bleibt typ-agnostisch.

---

## 4. Domain-Modell

### 4.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Project struct {
    ID          uuid.UUID
    Name        string
    Description string
    OwnerID     uuid.UUID
    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}

type ProjectMember struct {
    ProjectID uuid.UUID
    UserID    uuid.UUID
    Role      Role
    InvitedBy *uuid.UUID
    JoinedAt  time.Time
}

type Board struct {
    ID        uuid.UUID
    ProjectID uuid.UUID
    Name      string
    Type      BoardType
    Position  int
    Config    map[string]any
    CreatedBy uuid.UUID
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt *time.Time
    Columns   []BoardColumn  // optional preloaded
}

type BoardColumn struct {
    ID       uuid.UUID
    BoardID  uuid.UUID
    Name     string
    Position int
    WIPLimit *int
}

type Role string

const (
    RoleViewer Role = "viewer"
    RoleEditor Role = "editor"
    RoleOwner  Role = "owner"
)

type BoardType string

const (
    BoardTypeKanban   BoardType = "kanban"
    BoardTypeScrum    BoardType = "scrum"
    BoardTypeCalendar BoardType = "calendar"
)
```

### 4.2 Service-Interface

```go
package domain

type ProjectService interface {
    // Projekte
    CreateProject(ctx context.Context, ownerID uuid.UUID, name, description string) (*Project, error)
    GetProject(ctx context.Context, id uuid.UUID, requester uuid.UUID) (*Project, error)
    ListProjectsForUser(ctx context.Context, userID uuid.UUID) ([]*Project, error)
    UpdateProject(ctx context.Context, id uuid.UUID, requester uuid.UUID, patch ProjectPatch) (*Project, error)
    DeleteProject(ctx context.Context, id uuid.UUID, requester uuid.UUID) error

    // Mitglieder
    AddMember(ctx context.Context, projectID uuid.UUID, requester uuid.UUID, userID uuid.UUID, role Role) (*ProjectMember, error)
    UpdateMemberRole(ctx context.Context, projectID, requester, userID uuid.UUID, newRole Role) error
    RemoveMember(ctx context.Context, projectID, requester, userID uuid.UUID) error
    ListMembers(ctx context.Context, projectID, requester uuid.UUID) ([]*ProjectMember, error)

    // Boards
    CreateBoard(ctx context.Context, projectID, requester uuid.UUID, input BoardInput) (*Board, error)
    GetBoard(ctx context.Context, boardID, requester uuid.UUID) (*Board, error)
    ListBoards(ctx context.Context, projectID, requester uuid.UUID) ([]*Board, error)
    UpdateBoard(ctx context.Context, boardID, requester uuid.UUID, patch BoardPatch) (*Board, error)
    DeleteBoard(ctx context.Context, boardID, requester uuid.UUID) error

    // Permissions (intern)
    GetPermissions(ctx context.Context, projectID, userID uuid.UUID) (*PermissionSet, error)
}

type ProjectPatch struct {
    Name        *string
    Description *string
}

type BoardInput struct {
    Name    string
    Type    BoardType
    Config  map[string]any
    Columns []BoardColumnInput  // wenn nil: Defaults pro Type
}

type BoardColumnInput struct {
    Name     string
    Position int
    WIPLimit *int
}

type BoardPatch struct {
    Name   *string
    Config map[string]any
}
```

### 4.3 Domain-Fehler

```go
package domain

var (
    ErrProjectNotFound     = &Error{Code: "project_not_found"}
    ErrBoardNotFound       = &Error{Code: "board_not_found"}
    ErrMemberNotFound      = &Error{Code: "member_not_found"}
    ErrAlreadyMember       = &Error{Code: "already_member"}
    ErrPermissionDenied    = &Error{Code: "permission_denied"}
    ErrLastOwner           = &Error{Code: "last_owner_protected"}
    ErrUnknownUser         = &Error{Code: "unknown_user"}
    ErrInvalidRole         = &Error{Code: "invalid_role"}
    ErrInvalidBoardType    = &Error{Code: "invalid_board_type"}
    ErrValidation          = &Error{Code: "validation_failed"}
)
```

---

## 5. Permission-Modell

### 5.1 Rollen-Hierarchie

| Rolle | Rang | Beschreibung |
|-------|------|--------------|
| `viewer` | 10 | Nur lesen |
| `editor` | 20 | Inhalte modifizieren |
| `owner` | 30 | Volle Kontrolle, einschließlich Mitgliederverwaltung |

### 5.2 Permissions

Permissions sind feingranular und werden aus der Rolle abgeleitet. Andere Services prüfen gegen Permissions, nicht gegen Rollen — entkoppelt von Rollen-Reorganisation.

| Permission | viewer | editor | owner |
|------------|:------:|:------:|:-----:|
| `project:read` | ✓ | ✓ | ✓ |
| `project:update` | | ✓ | ✓ |
| `project:delete` | | | ✓ |
| `board:read` | ✓ | ✓ | ✓ |
| `board:create` | | ✓ | ✓ |
| `board:update` | | ✓ | ✓ |
| `board:delete` | | | ✓ |
| `task:read` | ✓ | ✓ | ✓ |
| `task:create` | | ✓ | ✓ |
| `task:update` | | ✓ | ✓ |
| `task:delete` | | ✓ | ✓ |
| `comment:read` | ✓ | ✓ | ✓ |
| `comment:create` | | ✓ | ✓ |
| `document:read` | ✓ | ✓ | ✓ |
| `document:create` | | ✓ | ✓ |
| `document:update` | | ✓ | ✓ |
| `document:delete` | | ✓ | ✓ |
| `member:read` | ✓ | ✓ | ✓ |
| `member:invite` | | | ✓ |
| `member:update` | | | ✓ |
| `member:remove` | | | ✓ |
| `webhook:manage` | | | ✓ |

### 5.3 Mapping in Code

```go
package domain

type PermissionSet struct {
    Role        Role     `json:"role"`
    Permissions []string `json:"permissions"`
}

var rolePermissions = map[Role][]string{
    RoleViewer: {
        "project:read", "board:read", "task:read",
        "comment:read", "document:read", "member:read",
    },
    RoleEditor: {
        "project:read", "project:update",
        "board:read", "board:create", "board:update",
        "task:read", "task:create", "task:update", "task:delete",
        "comment:read", "comment:create",
        "document:read", "document:create", "document:update", "document:delete",
        "member:read",
    },
    RoleOwner: {
        // alle editor-Permissions plus:
        "project:delete",
        "board:delete",
        "member:invite", "member:update", "member:remove",
        "webhook:manage",
    },
}

func PermissionsForRole(r Role) []string {
    // Owner erbt von Editor, Editor von Viewer (im Code expandiert)
    // ...
}
```

**Designentscheidung gegen RBAC mit dynamischen Rollen:** Im MVP feste Rollen. Eine echte RBAC-Engine (z. B. Casbin, OPA) lohnt sich erst, wenn benutzerdefinierte Rollen relevant werden. Dann wäre der Wechsel ein begrenzter Refactor des `PermissionsForRole`-Function.

---

## 6. HTTP-API (OpenAPI)

Vollständige Spec in `docs/api/project.openapi.yaml`. Hier die wesentlichen Endpunkte.

### 6.1 OpenAPI 3.1 (Auszug)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Project Service
  version: 1.0.0
servers:
  - url: http://localhost:8002/api/v1

paths:
  /projects:
    get:
      summary: List projects of current user
      operationId: listProjects
      tags: [projects]
      security: [{bearerAuth: []}]
      parameters:
        - in: query
          name: limit
          schema: { type: integer, default: 50, maximum: 100 }
        - in: query
          name: cursor
          schema: { type: string }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ProjectList' }

    post:
      summary: Create a new project
      operationId: createProject
      tags: [projects]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateProjectRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ProjectResponse' }

  /projects/{projectId}:
    parameters:
      - $ref: '#/components/parameters/projectId'
    get:
      summary: Get project details
      operationId: getProject
      tags: [projects]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ProjectResponse' }
        '404': { $ref: '#/components/responses/NotFound' }
        '403': { $ref: '#/components/responses/Forbidden' }

    patch:
      summary: Update project
      operationId: updateProject
      tags: [projects]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateProjectRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ProjectResponse' }

    delete:
      summary: Delete project (soft)
      operationId: deleteProject
      tags: [projects]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /projects/{projectId}/members:
    parameters:
      - $ref: '#/components/parameters/projectId'
    get:
      summary: List members
      operationId: listMembers
      tags: [members]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/MemberList' }

    post:
      summary: Add a member
      operationId: addMember
      tags: [members]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/AddMemberRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/MemberResponse' }

  /projects/{projectId}/members/{userId}:
    parameters:
      - $ref: '#/components/parameters/projectId'
      - $ref: '#/components/parameters/userId'
    patch:
      summary: Update member role
      operationId: updateMember
      tags: [members]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateMemberRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/MemberResponse' }

    delete:
      summary: Remove member
      operationId: removeMember
      tags: [members]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Removed }

  /projects/{projectId}/boards:
    parameters:
      - $ref: '#/components/parameters/projectId'
    get:
      summary: List boards
      operationId: listBoards
      tags: [boards]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/BoardList' }
    post:
      summary: Create a board
      operationId: createBoard
      tags: [boards]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateBoardRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/BoardResponse' }

  /boards/{boardId}:
    parameters:
      - $ref: '#/components/parameters/boardId'
    get:
      summary: Get board details
      operationId: getBoard
      tags: [boards]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/BoardResponse' }

    patch:
      summary: Update board
      operationId: updateBoard
      tags: [boards]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateBoardRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/BoardResponse' }

    delete:
      summary: Delete board
      operationId: deleteBoard
      tags: [boards]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /boards/{boardId}/columns:
    parameters:
      - $ref: '#/components/parameters/boardId'
    get:
      summary: List columns
      operationId: listColumns
      tags: [boards]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ColumnList' }
    post:
      summary: Create column
      operationId: createColumn
      tags: [boards]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateColumnRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ColumnResponse' }

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
    projectId:
      in: path
      name: projectId
      required: true
      schema: { type: string, format: uuid }
    userId:
      in: path
      name: userId
      required: true
      schema: { type: string, format: uuid }
    boardId:
      in: path
      name: boardId
      required: true
      schema: { type: string, format: uuid }

  schemas:
    CreateProjectRequest:
      type: object
      required: [name]
      properties:
        name:
          type: string
          minLength: 1
          maxLength: 200
        description:
          type: string
          maxLength: 2000

    UpdateProjectRequest:
      type: object
      properties:
        name: { type: string, minLength: 1, maxLength: 200 }
        description: { type: string, maxLength: 2000 }

    Project:
      type: object
      required: [id, name, owner_id, created_at]
      properties:
        id: { type: string, format: uuid }
        name: { type: string }
        description: { type: string }
        owner_id: { type: string, format: uuid }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }

    ProjectResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Project' }

    ProjectList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Project' }
        pagination: { $ref: '#/components/schemas/Pagination' }

    AddMemberRequest:
      type: object
      required: [role]
      properties:
        user_id: { type: string, format: uuid }
        email: { type: string, format: email }
        role:
          type: string
          enum: [viewer, editor, owner]
      description: "Either user_id or email must be provided"

    UpdateMemberRequest:
      type: object
      required: [role]
      properties:
        role: { type: string, enum: [viewer, editor, owner] }

    Member:
      type: object
      required: [project_id, user_id, role, joined_at]
      properties:
        project_id: { type: string, format: uuid }
        user_id: { type: string, format: uuid }
        email: { type: string, format: email }
        role: { type: string, enum: [viewer, editor, owner] }
        invited_by: { type: string, format: uuid, nullable: true }
        joined_at: { type: string, format: date-time }

    MemberResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Member' }

    MemberList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Member' }

    CreateBoardRequest:
      type: object
      required: [name]
      properties:
        name: { type: string, minLength: 1, maxLength: 100 }
        type:
          type: string
          enum: [kanban, scrum, calendar]
          default: kanban
        config: { type: object, additionalProperties: true }
        columns:
          type: array
          items: { $ref: '#/components/schemas/CreateColumnRequest' }

    UpdateBoardRequest:
      type: object
      properties:
        name: { type: string, minLength: 1, maxLength: 100 }
        config: { type: object, additionalProperties: true }

    Board:
      type: object
      required: [id, project_id, name, type, position, created_at]
      properties:
        id: { type: string, format: uuid }
        project_id: { type: string, format: uuid }
        name: { type: string }
        type: { type: string, enum: [kanban, scrum, calendar] }
        position: { type: integer }
        config: { type: object, additionalProperties: true }
        created_by: { type: string, format: uuid }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
        columns:
          type: array
          items: { $ref: '#/components/schemas/Column' }

    BoardResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Board' }

    BoardList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Board' }

    CreateColumnRequest:
      type: object
      required: [name, position]
      properties:
        name: { type: string, minLength: 1, maxLength: 50 }
        position: { type: integer, minimum: 0 }
        wip_limit: { type: integer, minimum: 1, nullable: true }

    Column:
      type: object
      required: [id, board_id, name, position]
      properties:
        id: { type: string, format: uuid }
        board_id: { type: string, format: uuid }
        name: { type: string }
        position: { type: integer }
        wip_limit: { type: integer, nullable: true }

    ColumnResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Column' }

    ColumnList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Column' }

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

  responses:
    NotFound:
      description: Resource not found
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
    Forbidden:
      description: Permission denied
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
```

### 6.2 Endpoint-Übersicht

| Methode | Pfad | Auth | Permission | Beschreibung |
|---------|------|------|------------|--------------|
| GET | `/projects` | Bearer | — | Eigene Projekte |
| POST | `/projects` | Bearer | (eingeloggt) | Projekt erstellen |
| GET | `/projects/{id}` | Bearer | `project:read` | Projekt lesen |
| PATCH | `/projects/{id}` | Bearer | `project:update` | Projekt ändern |
| DELETE | `/projects/{id}` | Bearer | `project:delete` | Projekt löschen |
| GET | `/projects/{id}/members` | Bearer | `member:read` | Mitglieder |
| POST | `/projects/{id}/members` | Bearer | `member:invite` | Mitglied hinzufügen |
| PATCH | `/projects/{id}/members/{userId}` | Bearer | `member:update` | Rolle ändern |
| DELETE | `/projects/{id}/members/{userId}` | Bearer | `member:remove` od. self | Mitglied entfernen |
| GET | `/projects/{id}/boards` | Bearer | `board:read` | Boards |
| POST | `/projects/{id}/boards` | Bearer | `board:create` | Board erstellen |
| GET | `/boards/{id}` | Bearer | `board:read` | Board lesen |
| PATCH | `/boards/{id}` | Bearer | `board:update` | Board ändern |
| DELETE | `/boards/{id}` | Bearer | `board:delete` | Board löschen |
| GET | `/boards/{id}/columns` | Bearer | `board:read` | Spalten |
| POST | `/boards/{id}/columns` | Bearer | `board:update` | Spalte erstellen |

---

## 7. Interne API für Service-zu-Service

### 7.1 Routing

Endpunkte unter `/internal/...` werden im Gateway **nicht** nach außen exponiert. Lokal direkt erreichbar (Service Discovery via Docker-Netzwerk), in AWS über interne ALB-Listener-Rules oder Service Mesh.

### 7.2 Authentifizierung

Service-Token: kurzer JWT mit `aud: "internal"`, `sub: "<service-name>"`, signiert vom Auth Service mit demselben Key wie User-Tokens. Ausgestellt durch separaten internen Endpoint des Auth Service (`POST /internal/service-tokens`, im MVP statisch konfiguriert).

### 7.3 Endpunkte

```yaml
paths:
  /internal/projects/{projectId}/permissions/{userId}:
    parameters:
      - in: path
        name: projectId
        required: true
        schema: { type: string, format: uuid }
      - in: path
        name: userId
        required: true
        schema: { type: string, format: uuid }
    get:
      summary: Get permissions for user in project
      operationId: getPermissions
      tags: [internal]
      security: [{serviceAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                required: [role, permissions, project_exists, is_member]
                properties:
                  role:
                    type: string
                    enum: [viewer, editor, owner]
                    nullable: true
                  permissions:
                    type: array
                    items: { type: string }
                  project_exists: { type: boolean }
                  is_member: { type: boolean }
              examples:
                member:
                  value:
                    role: editor
                    permissions: [project:read, task:create, ...]
                    project_exists: true
                    is_member: true
                non_member:
                  value:
                    role: null
                    permissions: []
                    project_exists: true
                    is_member: false

  /internal/projects/{projectId}/exists:
    get:
      summary: Lightweight existence check
      operationId: projectExists
      tags: [internal]
      security: [{serviceAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                properties:
                  exists: { type: boolean }
```

### 7.4 Performance-Anforderungen

- **p99-Latenz:** < 20 ms (cached), < 50 ms (uncached)
- **Caching-Header:** `Cache-Control: max-age=30` damit Aufrufer client-seitig cachen können
- **Volumen:** Höchster Traffic im System — jeder Task-Schreibzugriff löst einen Permission-Check aus

---

## 8. Permission-Cache

### 8.1 Cache-Struktur (Redis)

Schlüsselformat: `perm:{projectId}:{userId}` → JSON `PermissionSet`  
TTL: 60 Sekunden

```redis
SET perm:550e8400-...:user-uuid '{"role":"editor","permissions":[...]}' EX 60
```

### 8.2 Cache-Invalidierung

Bei Membership-Änderungen wird der Cache-Eintrag aktiv gelöscht **bevor** das Event publiziert wird:

```go
func (s *service) AddMember(ctx context.Context, ...) error {
    // 1. DB-Update + Outbox in Transaktion
    tx, err := s.db.BeginTx(ctx, nil)
    // ...
    
    // 2. Cache invalidieren (best-effort)
    s.cache.Delete(ctx, fmt.Sprintf("perm:%s:%s", projectID, userID))
    
    // 3. Event-Publisher liest Outbox und sendet asynchron
    return nil
}
```

**Bei Cache-Server-Ausfall:** Kein Fail. Reads gehen direkt gegen DB, Latenz steigt. Ist explizit "weiche" Abhängigkeit aus Abschnitt 1.3.

### 8.3 Stale-Read-Toleranz

Es ist akzeptabel, dass für bis zu 60 Sekunden nach Permission-Änderung noch der alte Stand gelesen wird, weil:

- Permissionsänderungen sind selten (Mitglied hinzufügen, Rolle ändern)
- Aktive Cache-Invalidierung greift sofort, TTL ist nur Fallback bei verpasster Invalidierung
- Konsequenzen sind begrenzt — z. B. ein eben gewordener Editor wartet 60 s länger, bis er Tasks anlegen kann

### 8.4 Cache-Warming

Nicht im MVP. Eintrag wird beim ersten Lookup nach Cache-Miss erzeugt.

---

## 9. Board-Typen und Plugin-Hooks

> **Hinweis (Architektur-Evolution):** Board-Typen sind **keine compile-time Strategy mehr**.
> Frühere Iterationen nutzten ein in-process Strategy-Pattern bzw. eine self-registering
> `boardplugins`-Registry im Project-Service. Boardtyp-*Definitionen* sind jetzt **Laufzeit-Daten**
> im dedizierten **Board-Registry-Service** (Port 8007, `boardregistry_db`). Details:
> `docs/services/boardregistry.md` und ADR `docs/decisions/0001-board-type-extensibility.md`.

### 9.1 Laufzeit-Auflösung über die Board-Registry

Beim Erstellen eines Boards (`CreateBoard`) löst der Project-Service den Typ zur Laufzeit auf:

1. `BoardTypeRegistry.GetType(ctx, type)` — Port in `internal/domain`, implementiert vom
   `boardtypeclient` (HTTP gegen `GET /api/v1/internal/board-types/{type}` der Registry,
   Service-Token-Auth, TTL-Cache).
   - 404 → `ErrInvalidBoardType` (400 an den Client)
   - Registry nicht erreichbar → `ErrBoardTypeRegistryUnavailable` (503)
2. Aus der Definition kommen **Default-Columns** (inkl. `status`), **Default-Config** und ein
   **JSON-Schema** für die Config.
3. Die Board-Config wird gegen das JSON-Schema validiert (`ValidateBoardConfig`,
   `santhosh-tekuri/jsonschema`).
4. Default-Columns werden mit ihrem expliziten `status` persistiert und im `board.created`-Event
   (Spalten-Array) bzw. `column.created/updated`-Event mitgesendet.

**Cache-Invalidierung:** Der Project-Event-Consumer bindet `boardtype.registered/updated/deleted`
und invalidiert den `boardtypeclient`-Cache für den betroffenen Typ (gleiches Muster wie die
Permission-Cache-Invalidierung auf `project.member.*`).

### 9.2 Erweiterungspunkt

Einen neuen Board-Typ "gantt" einzuführen bedeutet **keinen Redeploy** des Project-Service mehr:

1. `POST /api/v1/board-types` an die Board-Registry (Typ-Slug, Display-Name, Icon,
   Default-Columns inkl. `status`, Default-Config, `config_schema`).
2. Frontend: passende View-Komponente.

Der DB-`CHECK`-Constraint auf `boards.type` wurde in Migration `0002_drop_board_type_check`
entfernt; `boards.type` ist ein freier Slug, dessen Gültigkeit über die Registry bestimmt wird.
**Kein Code-Change** im Project-, Task- oder Document-Service. Genau das Plugin-Pattern aus der
Aufgabenstellung — jetzt mit echter Laufzeit-Erweiterbarkeit durch Dritte.

---

## 10. Events

### 10.1 Publizierte Events

| Event-Type | Trigger | Payload |
|------------|---------|---------|
| `project.created` | UC-1 | `project_id, name, owner_id, created_at` |
| `project.updated` | UC-2 | `project_id, changes (Diff-Map)` |
| `project.deleted` | UC-3 | `project_id, deleted_at` |
| `project.member.added` | UC-4 | `project_id, user_id, role, invited_by` |
| `project.member.role_changed` | UC-5 | `project_id, user_id, old_role, new_role` |
| `project.member.removed` | UC-6 | `project_id, user_id` |
| `board.created` | UC-7 | `board_id, project_id, name, type, created_by` |
| `board.updated` | — | `board_id, project_id, changes` |
| `board.deleted` | — | `board_id, project_id` |

Vollständige Payload-Schemas in `docs/api/events.asyncapi.yaml`.

### 10.2 Konsumierte Events

| Event-Type | Source | Reaktion |
|------------|--------|----------|
| `user.registered` | auth | INSERT in `known_users` |
| `user.deleted` | auth | UPDATE `known_users.deleted_at`; alle Mitgliedschaften des Users entfernen; Outbox-Event `project.member.removed` pro betroffenem Projekt |

### 10.3 Idempotenz

Beide Konsumenten prüfen `processed_events` vor Verarbeitung. Verarbeitung + Insert in `processed_events` in einer Transaktion.

---

## 11. Konfiguration

### 11.1 Environment-Variablen

```bash
# Service
SERVICE_NAME=project-service
SERVICE_PORT=8002
LOG_LEVEL=info

# Database
DB_URL=postgres://project:project@postgres:5432/project_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
RABBITMQ_CONSUMER_QUEUE=project-service-queue

# Redis (Permission-Cache)
REDIS_URL=redis://redis:6379/1
PERMISSION_CACHE_TTL=60s
PERMISSION_CACHE_ENABLED=true

# Auth (JWKS)
JWT_JWKS_URL=http://auth:8001/.well-known/jwks.json
JWT_ISSUER=https://auth.teamboard.local
JWT_AUDIENCE=teamboard-api
JWT_JWKS_CACHE_TTL=10m

# Service-to-Service
INTERNAL_AUDIENCE=internal

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=project-service
```

### 11.2 Config-Struct

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"project-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8002"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
    }

    RabbitMQ struct {
        URL           string `env:"RABBITMQ_URL,required"`
        Exchange      string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
        ConsumerQueue string `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"project-service-queue"`
    }

    Cache struct {
        RedisURL       string        `env:"REDIS_URL"`
        PermissionTTL  time.Duration `env:"PERMISSION_CACHE_TTL" envDefault:"60s"`
        Enabled        bool          `env:"PERMISSION_CACHE_ENABLED" envDefault:"true"`
    }

    JWT struct {
        JWKSURL        string        `env:"JWT_JWKS_URL,required"`
        Issuer         string        `env:"JWT_ISSUER,required"`
        Audience       string        `env:"JWT_AUDIENCE,required"`
        JWKSCacheTTL   time.Duration `env:"JWT_JWKS_CACHE_TTL" envDefault:"10m"`
        InternalAud    string        `env:"INTERNAL_AUDIENCE" envDefault:"internal"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }
}
```

---

## 12. Verzeichnisstruktur

```
services/project/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── middleware.go               # JWT, Tracing, Service-Token
│   │   ├── handlers_projects.go
│   │   ├── handlers_members.go
│   │   ├── handlers_boards.go
│   │   ├── handlers_internal.go        # /internal/* — Permission-Endpoints
│   │   ├── handlers_health.go
│   │   ├── dto.go
│   │   └── generated.go                # oapi-codegen
│   ├── domain/
│   │   ├── project.go
│   │   ├── member.go
│   │   ├── board.go
│   │   ├── permissions.go              # Role -> Permissions Mapping
│   │   ├── board_strategies.go         # Strategy-Pattern für Board-Typen
│   │   ├── service.go                  # Interface
│   │   ├── service_impl.go             # Implementation
│   │   └── errors.go
│   ├── repository/
│   │   ├── db/                          # sqlc-generiert
│   │   ├── repository.go
│   │   └── postgres.go
│   ├── cache/
│   │   ├── cache.go                     # Interface
│   │   ├── redis.go                     # Redis-Implementation
│   │   └── noop.go                      # Fallback bei deaktiviertem Cache
│   ├── events/
│   │   ├── publisher.go                 # Outbox -> RabbitMQ
│   │   ├── consumer.go                  # user.* events
│   │   └── envelope.go
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   ├── 0001_init.down.sql
│   ├── 0002_seed_roles.up.sql
│   └── 0002_seed_roles.down.sql
├── queries/
│   ├── projects.sql
│   ├── members.sql
│   ├── boards.sql
│   ├── columns.sql
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

## 13. sqlc-Queries

### 13.1 `queries/projects.sql`

```sql
-- name: CreateProject :one
INSERT INTO projects (id, name, description, owner_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListProjectsForUser :many
SELECT p.* FROM projects p
INNER JOIN project_members m ON m.project_id = p.id
WHERE m.user_id = $1 AND p.deleted_at IS NULL
ORDER BY p.created_at DESC
LIMIT $2;

-- name: UpdateProject :one
UPDATE projects
SET name = COALESCE($2, name),
    description = COALESCE($3, description),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProject :exec
UPDATE projects SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
```

### 13.2 `queries/members.sql`

```sql
-- name: AddMember :one
INSERT INTO project_members (project_id, user_id, role, invited_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetMember :one
SELECT * FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: ListMembers :many
SELECT m.*, u.email FROM project_members m
LEFT JOIN known_users u ON u.id = m.user_id
WHERE m.project_id = $1
ORDER BY m.joined_at;

-- name: UpdateMemberRole :one
UPDATE project_members
SET role = $3
WHERE project_id = $1 AND user_id = $2
RETURNING *;

-- name: RemoveMember :exec
DELETE FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: CountOwners :one
SELECT COUNT(*) FROM project_members
WHERE project_id = $1 AND role = 'owner';

-- name: GetMemberRole :one
SELECT role FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: RemoveAllMembershipsOfUser :many
DELETE FROM project_members
WHERE user_id = $1
RETURNING project_id;
```

### 13.3 `queries/boards.sql`

```sql
-- name: CreateBoard :one
INSERT INTO boards (id, project_id, name, type, position, config, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetBoard :one
SELECT * FROM boards
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListBoardsByProject :many
SELECT * FROM boards
WHERE project_id = $1 AND deleted_at IS NULL
ORDER BY position;

-- name: UpdateBoard :one
UPDATE boards
SET name = COALESCE($2, name),
    config = COALESCE($3, config),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteBoard :exec
UPDATE boards SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetMaxBoardPosition :one
SELECT COALESCE(MAX(position), -1) FROM boards
WHERE project_id = $1 AND deleted_at IS NULL;
```

### 13.4 `queries/columns.sql`

```sql
-- name: CreateColumn :one
INSERT INTO board_columns (id, board_id, name, position, wip_limit)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListColumnsByBoard :many
SELECT * FROM board_columns
WHERE board_id = $1
ORDER BY position;

-- name: UpdateColumn :one
UPDATE board_columns
SET name = COALESCE($2, name),
    position = COALESCE($3, position),
    wip_limit = $4
WHERE id = $1
RETURNING *;

-- name: DeleteColumn :exec
DELETE FROM board_columns WHERE id = $1;
```

### 13.5 `queries/known_users.sql`

```sql
-- name: UpsertKnownUser :exec
INSERT INTO known_users (id, email, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email;

-- name: GetKnownUserByEmail :one
SELECT * FROM known_users
WHERE email = $1 AND deleted_at IS NULL;

-- name: GetKnownUserByID :one
SELECT * FROM known_users
WHERE id = $1 AND deleted_at IS NULL;

-- name: MarkKnownUserDeleted :exec
UPDATE known_users SET deleted_at = NOW() WHERE id = $1;
```

### 13.6 `queries/outbox.sql`

```sql
-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: GetUnpublishedEvents :many
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY occurred_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE outbox SET published_at = NOW() WHERE id = $1;
```

### 13.7 `queries/processed_events.sql`

```sql
-- name: WasEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1);

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1)
ON CONFLICT DO NOTHING;
```

### 13.8 `sqlc.yaml`

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

## 14. Test-Strategie

### 14.1 Unit-Tests (Domain)

**Coverage-Ziel:** ≥ 80% in `internal/domain/`

Wichtige Testfälle:

```go
func TestPermissionsForRole(t *testing.T) {
    require.Contains(t, PermissionsForRole(RoleViewer), "project:read")
    require.NotContains(t, PermissionsForRole(RoleViewer), "project:update")
    require.Contains(t, PermissionsForRole(RoleOwner), "member:invite")
}

func TestLastOwnerProtection(t *testing.T) {
    // Setup: Projekt mit nur einem Owner
    // Erwartung: Versuch Owner zu entfernen → ErrLastOwner
    // Erwartung: Versuch Owner-Rolle zu degradieren → ErrLastOwner
}

func TestBoardDefaultColumns(t *testing.T) {
    s := kanbanStrategy{}
    cols := s.DefaultColumns()
    require.Len(t, cols, 3)
    require.Equal(t, "To Do", cols[0].Name)
}
```

### 14.2 Integration-Tests

```go
func TestProjectRepository(t *testing.T) {
    pg := startPostgres(t)
    repo := NewPostgresRepository(pg.URL())

    t.Run("create and read project", func(t *testing.T) { ... })
    t.Run("soft delete excludes from queries", func(t *testing.T) { ... })
    t.Run("cascade delete of members", func(t *testing.T) { ... })
}

func TestPermissionEndpoint(t *testing.T) {
    // Setup: Projekt + Member mit Editor-Rolle
    // Call /internal/projects/{id}/permissions/{userId}
    // Erwartung: 200 mit role=editor + erwartete Permissions
}

func TestEventConsumer_UserDeleted(t *testing.T) {
    // Setup: User in 3 Projekten
    // Trigger user.deleted Event
    // Erwartung: 3 Memberships entfernt, 3 project.member.removed-Events in Outbox
    // Erwartung: known_users.deleted_at gesetzt
}
```

### 14.3 End-to-End-Tests

Postman-Collection unter `tests/postman/project.postman_collection.json`. Szenarien:

1. **Projekt-Lifecycle:** create → invite member → add board → list → update → delete
2. **Permission-Enforcement:** Viewer versucht Update → 403; Editor versucht Member-Invite → 403
3. **Last-Owner-Protection:** Owner versucht sich selbst zu degradieren → 409
4. **Self-Leave:** Mitglied entfernt sich selbst → 204

### 14.4 Performance-Tests

Permission-Endpoint unter Last (k6-Skript):

```javascript
// tests/k6/permissions.js
import http from 'k6/http';
export const options = {
  vus: 50,
  duration: '30s',
  thresholds: { http_req_duration: ['p(99)<50'] },
};
export default function () {
  http.get(`http://project:8002/internal/projects/${PID}/permissions/${UID}`, {
    headers: { Authorization: `Bearer ${SERVICE_TOKEN}` },
  });
}
```

---

## 15. Implementierungs-Hinweise für Coding-Agents

### 15.1 Implementierungsreihenfolge

1. **Migrations + sqlc-Setup** — `make generate` läuft, alle Tabellen + Rollen-Seed da
2. **Domain-Modell** (`internal/domain/`) inkl. Permissions-Map und Board-Strategies — mit Unit-Tests
3. **Repository-Layer** mit Testcontainers
4. **Cache-Layer** (`internal/cache/`) inkl. NoOp-Implementation
5. **Service-Layer** (`internal/domain/service_impl.go`) — Use-Cases UC-1..UC-9
6. **Event-Consumer** für `user.*` Events
7. **HTTP-Handler** (öffentlich + intern)
8. **Outbox-Publisher**
9. **Wiring in `main.go`**
10. **End-to-End-Tests**

### 15.2 Verbindliche Konventionen

- **Permission-Check vor jeder schreibenden Operation**, ohne Ausnahme. Wrapper-Funktion `s.requirePermission(ctx, projectID, userID, "perm:name")` nutzen.
- **Outbox + DB-Update in einer Transaktion** mit `pgx.BeginTx`. Ohne Outbox-Insert keine Datenmutation.
- **Cache-Invalidation vor Event-Publish.** Reihenfolge: 1. DB-Tx commit, 2. Cache.Delete, 3. Outbox-Worker pickt Event auf.
- **Last-Owner-Check** bei jeder Owner-Mutation, nicht nur bei Delete. Auch bei Role-Update.
- **Self-Leave separater Pfad** in `RemoveMember` — zusätzlicher Auth-Pfad, identische Validierung sonst.
- **Service-Token-Middleware** für `/internal/...` strikt trennen — keinerlei Bearer-Token-Akzeptanz dort.

### 15.3 Typische Stolperfallen

- **Cascade-Delete vs. Soft-Delete:** `project_members` hat `ON DELETE CASCADE` auf `projects.id`. Bei Soft-Delete des Projekts werden Memberships **nicht** automatisch entfernt — sie sind ja in einer eigenen, nicht-soft-delete-verknüpften Tabelle. Bei späterem Hard-Delete des Projekts tritt CASCADE in Kraft.
- **PATCH-Semantik:** `PATCH` mit `application/merge-patch+json` (RFC 7396). Felder, die `null` sein können (z. B. `wip_limit`), brauchen explizite Behandlung in der DTO.
- **JSONB-Roundtrip:** Beim Lesen von `boards.config` aus PG kommt es als `[]byte` an, beim Schreiben als JSON-Marshal. sqlc's `pgtype.JSONB` ist unhandlich — Wrapper-Funktion in Repository nutzen.
- **Cursor-Pagination:** Cursor referenziert `(created_at, id)` für stabile Sortierung. Reines `created_at` reicht nicht (Ties bei gleichzeitiger Erstellung).
- **Event-Reihenfolge:** `project.created` muss **vor** `project.member.added` aus derselben Transaktion publiziert werden. Outbox sortiert nach `occurred_at`, also Insert-Reihenfolge in der Transaktion einhalten.
- **Known_users-Race:** Wenn ein User sich registriert und sofort ein Projekt erstellt, kann das `user.registered`-Event noch nicht konsumiert sein. Daher `known_users` nicht für Existenz-Checks bei Owner verwenden, nur für Email-Lookups bei Member-Invitation.

### 15.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration migrate generate run docker-build

build:
	go build -o bin/project ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/...

test-integration:
	go test -race ./internal/repository/... ./internal/events/...

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/project.openapi.yaml > internal/api/generated.go

run:
	air

docker-build:
	docker build -t teamboard/project:latest .
```

### 15.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|-------------------|
| UC-1 Create Project | Projekt + Owner-Membership existieren atomar, beide Outbox-Events vorhanden, 201 |
| UC-2 Update Project | Permission `project:update` erforderlich, Outbox-Event mit Diff |
| UC-3 Delete Project | Nur Owner darf, Soft-Delete, `project.deleted` publiziert |
| UC-4 Add Member | E-Mail/User-ID validiert gegen `known_users`, Cache invalidiert, Event publiziert |
| UC-5 Update Role | Last-Owner-Schutz aktiv, Cache invalidiert |
| UC-6 Remove Member | Self-Leave erlaubt, Last-Owner-Schutz, Cache invalidiert |
| UC-7 Create Board | Default-Spalten gemäß Strategy, Config validiert |
| UC-8 List Projects | Nur Projekte mit aktiver Mitgliedschaft, sortiert nach `created_at DESC` |
| UC-9 Get Permissions | Cache-Hit < 20 ms p99, Cache-Miss < 50 ms p99, korrekte Permissions pro Rolle |

---

## Anhang A: Sequenzdiagramm — Permission-Check-Flow

```
Task Service          Project Service        Redis             Postgres
     │                      │                  │                  │
     │ GET /internal/        │                  │                  │
     │  projects/X/perm/Y    │                  │                  │
     ├──────────────────────>│                  │                  │
     │                      │ GET perm:X:Y     │                  │
     │                      ├─────────────────>│                  │
     │                      │ (cache miss)     │                  │
     │                      │<─────────────────┤                  │
     │                      │ SELECT role      │                  │
     │                      ├──────────────────────────────────────>│
     │                      │<──────────────────────────────────────┤
     │                      │ SET perm:X:Y EX 60                  │
     │                      ├─────────────────>│                  │
     │ {role:editor,        │                  │                  │
     │  permissions:[...]}  │                  │                  │
     │<──────────────────────┤                  │                  │
     │                      │                  │                  │
     │ GET /internal/        │                  │                  │
     │  projects/X/perm/Y    │                  │                  │
     ├──────────────────────>│                  │                  │
     │                      │ GET perm:X:Y     │                  │
     │                      ├─────────────────>│                  │
     │                      │ (cache hit)      │                  │
     │                      │<─────────────────┤                  │
     │ {role:editor, ...}    │                  │                  │
     │<──────────────────────┤                  │                  │
```

## Anhang B: Sequenzdiagramm — Member-Add mit Cache-Invalidierung

```
Client            Project Svc          Postgres          Redis           RabbitMQ
   │                  │                    │                │                  │
   │ POST .../members │                    │                │                  │
   ├─────────────────>│                    │                │                  │
   │                  │ BEGIN TX           │                │                  │
   │                  ├───────────────────>│                │                  │
   │                  │ INSERT membership  │                │                  │
   │                  ├───────────────────>│                │                  │
   │                  │ INSERT outbox      │                │                  │
   │                  ├───────────────────>│                │                  │
   │                  │ COMMIT             │                │                  │
   │                  ├───────────────────>│                │                  │
   │                  │ DEL perm:X:Y       │                │                  │
   │                  ├──────────────────────────────────>│                  │
   │ 201              │                    │                │                  │
   │<─────────────────┤                    │                │                  │
   │                  │ (Outbox-Worker     │                │                  │
   │                  │  async)            │                │                  │
   │                  │ SELECT FROM outbox │                │                  │
   │                  ├───────────────────>│                │                  │
   │                  │ Publish member.added                │                  │
   │                  ├───────────────────────────────────────────────────────>│
   │                  │ UPDATE outbox SET published_at      │                  │
   │                  ├───────────────────>│                │                  │
```

---

**Ende des Detail-Designs Project Service.**
