# Notification Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Verwandtes Dokument:** [`services/project.md`](./project.md) — Authority für Permissions  
> **Service:** `notification`  
> **Port (lokal):** 8005  
> **Datenbank:** `notification_db` (PostgreSQL)  
> **Cache & Backplane:** Redis  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Zwei Notification-Typen: Real-Time und Persistent](#2-zwei-notification-typen-real-time-und-persistent)
3. [Skalierungs-Architektur (Backplane)](#3-skalierungs-architektur-backplane)
4. [Channel-Subscription-Model](#4-channel-subscription-model)
5. [Use-Cases](#5-use-cases)
6. [Datenmodell](#6-datenmodell)
7. [Domain-Modell](#7-domain-modell)
8. [WebSocket-Protokoll](#8-websocket-protokoll)
9. [Connection-Lifecycle](#9-connection-lifecycle)
10. [Event-zu-Notification-Mapping](#10-event-zu-notification-mapping)
11. [HTTP-API (OpenAPI)](#11-http-api-openapi)
12. [Permission-Filterung](#12-permission-filterung)
13. [Konfiguration](#13-konfiguration)
14. [Verzeichnisstruktur](#14-verzeichnisstruktur)
15. [sqlc-Queries](#15-sqlc-queries)
16. [Test-Strategie](#16-test-strategie)
17. [Performance und Skalierung](#17-performance-und-skalierung)
18. [Implementierungs-Hinweise für Coding-Agents](#18-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **WebSocket-Endpoint** für Browser-Clients
- **Connection-Tracking** (welcher User ist auf welcher Instanz verbunden?)
- **Channel-Subscription-Management** (welcher Client hört welche Channels?)
- **Event-Konsum** aus dem zentralen Event Bus (RabbitMQ)
- **Permission-Filterung** vor dem Push (User darf Event sehen?)
- **Real-Time-Push** an verbundene Clients
- **Persistente Notifications** für offline-fähige Events (z. B. Mentions)
- **Notification-API** zum Lesen, Markieren als gelesen, Löschen

### 1.2 Verantwortet NICHT

- **Notification-Erzeugung als Authority** — Events kommen aus anderen Services
- **E-Mail- oder Push-Notifications** (Mobile Push, Slack, etc.) — eigener "Notification-Channel-Service" später; im MVP nur In-App
- **User-Settings** für Notification-Präferenzen — im MVP nicht; später ergänzbar
- **Berechtigungen ableiten** — Project Service ist Authority

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `notification_db` | hart | Persistente Notifications |
| Redis | **hart** | Connection-Tracking + Pub/Sub-Backplane (kritisch für Multi-Instance) |
| RabbitMQ | hart | Event-Konsum |
| Project Service | hart | Permission-Checks pro Event |
| Auth Service (JWKS) | hart | JWT-Validierung beim Connect |

**Wichtig:** Redis ist im Notification Service kein "weiches" Cache wie bei anderen Services — ohne Redis funktioniert die Multi-Instance-Skalierung nicht. Bei Single-Instance-Deployment kann Redis durch In-Memory ersetzt werden (Fallback-Mode).

---

## 2. Zwei Notification-Typen: Real-Time und Persistent

### 2.1 Real-Time Notifications

**Charakteristik:** Live-Updates, die nur Sinn machen, wenn der User gerade aktiv ist.

**Beispiele:**
- Task wird in andere Spalte verschoben (Drag-and-Drop sichtbar bei allen Mitgliedern)
- Task-Status ändert sich
- Neuer Comment erscheint im Task
- User tippt im Task gerade einen Comment (Typing-Indicator, Erweiterung)

**Verhalten:** Werden via WebSocket gepusht. Wenn User offline ist, **werden sie verworfen** — kein Speichern. Der nächste Page-Refresh holt den aktuellen Stand sowieso über REST.

### 2.2 Persistent Notifications

**Charakteristik:** Wichtig genug, dass User sie auch sehen sollen, wenn er offline war.

**Beispiele:**
- `@mention` in einem Comment (User wurde explizit angesprochen)
- Task wurde mir zugewiesen
- Mein Task wurde gelöscht / verschoben
- Mein Comment wurde gelöscht (Moderation)

**Verhalten:** Werden in `notifications`-Tabelle gespeichert. Wenn User online: zusätzlich Live-Push. Wenn offline: bei nächstem Login über `GET /notifications` abrufbar mit `read_at = NULL`.

### 2.3 Designentscheidung

Beide Typen laufen über denselben WebSocket-Channel — der Client unterscheidet sie über `event_type` und `persistent: true|false` Flag im Payload. Das hält den Client-Code einfach: ein Subscriber für alles, UI entscheidet, ob Banner-Notification oder nur Live-Update auf der Board-View.

---

## 3. Skalierungs-Architektur (Backplane)

### 3.1 Das Problem

WebSocket-Verbindungen sind langlebig und bleiben an einer konkreten Instanz haften. Bei mehreren Instanzen:

```
User A ──┐                    
         ├── (Load Balancer) ──┬── Instanz 1 (hält Verbindung von User A)
User B ──┘                    └── Instanz 2 (hält Verbindung von User B)
```

Wenn auf Instanz 1 ein Event ankommt, das an User B gehen soll, muss es Instanz 2 erreichen. Klassisches verteiltes Pub/Sub-Problem.

### 3.2 Die Lösung: Redis Pub/Sub als Backplane

```
                 RabbitMQ (Domain-Events)
                       │
            ┌──────────┴──────────┐
            ▼                     ▼
      Instanz 1             Instanz 2
            │                     │
            └──────┬──────────────┘
                   ▼
            Redis Pub/Sub
            (channel "broadcast")
                   │
            ┌──────┴──────┐
            ▼             ▼
      Instanz 1     Instanz 2
            │             │
            ▼             ▼
        User A        User B
```

**Flow:**
1. Instanz 1 konsumiert Event aus RabbitMQ
2. Instanz 1 prüft Permissions, erzeugt Notifications
3. Instanz 1 schreibt persistente Notifications in DB
4. Instanz 1 publiziert Notification an Redis Pub/Sub-Channel `notify:user:{userId}`
5. **Alle Instanzen** abonnieren `notify:user:*` per Redis Pattern Subscribe
6. Jede Instanz prüft: hält sie eine Connection für diesen User? Wenn ja, push via WebSocket.

### 3.3 Alternative: Direct Routing über Connection-Registry

```
Redis HSET connections {userId} -> {instanceId}
```

Wenn Instanz 1 ein Event hat, schaut sie nach: "User B ist auf Instanz 2 → schick direkt an Instanz 2 via internem RPC."

**Trade-off:**
- **Pub/Sub-Variante:** einfacher, jede Instanz bekommt jede Notification (mehr Netzwerk-Traffic, aber Filterung lokal)
- **Direct-Routing:** weniger Netzwerk, aber benötigt Service-zu-Service-Kommunikation und Connection-Registry

**MVP-Entscheidung:** **Pub/Sub.** Einfacher, weniger bewegliche Teile. Bei sehr großen User-Zahlen (>10k concurrent) lohnt der Wechsel zu Direct-Routing.

### 3.4 Connection-Tracking in Redis

Jede Instanz registriert ihre Connections:

```
SET conn:{userId}:{connId} {instanceId} EX 60       # TTL 60s, refresh alle 30s
SADD user_conns:{userId} {connId}
```

Bei Disconnect:
```
DEL conn:{userId}:{connId}
SREM user_conns:{userId} {connId}
```

Heartbeat-Refresh hält Einträge frisch. Wenn Instanz crasht, expirieren Einträge automatisch.

---

## 4. Channel-Subscription-Model

### 4.1 Channel-Schema

Channels sind String-Identifier, denen ein Client beitreten kann. Permission-bezogen:

| Channel | Bedeutung | Permission-Check |
|---------|-----------|------------------|
| `user:{userId}` | Persönliche Notifications (Mentions, Assignments) | `userId == self` |
| `project:{projectId}` | Alle Events eines Projekts | `member of project` |
| `board:{boardId}` | Alle Events eines Boards | `member of project (board belongs to)` |
| `task:{taskId}` | Updates eines spezifischen Tasks (Comments, Status) | `member of project (task belongs to)` |

Ein Client subscribed mehrere Channels gleichzeitig — typisch:
- Sein `user:{self}`-Channel (immer)
- `board:{currentlyViewedBoard}` solange er auf der Board-View ist
- `project:{currentProject}` für übergeordnete Events

### 4.2 Subscribe-Flow

Client schickt nach Connect:
```json
{
  "type": "subscribe",
  "channels": ["board:550e...", "project:660f..."]
}
```

Server:
1. Pro Channel Permission-Check
2. Bei Erfolg: Channel-Membership in Redis `SADD channel:{name} {connId}`
3. Bei Fehlschlag: Antwort `subscribe_denied` für den Channel
4. `subscribe_ack` mit erfolgreichen Channels

### 4.3 Filterung beim Push

Wenn Event ankommt, das z. B. zu Board X gehört:
- Server schaut: welche Connections sind in `channel:board:X`?
- Pro Connection: ist sie auf dieser Instanz? Wenn ja → push
- Wenn auf anderer Instanz: Pub/Sub kümmert sich

### 4.4 Designentscheidung gegen "alles abonnieren"

Alternative wäre: Client bekommt automatisch alle Events seiner Projekte. Vorteile: einfacher Client. Nachteile: hoher Traffic, Privacy-Probleme (Client sieht Events, die er gar nicht braucht).

**MVP:** Explizite Subscriptions. Client kontrolliert seinen Traffic, Server filtert.

---

## 5. Use-Cases

### UC-1: WebSocket-Connect

**Akteur:** Eingeloggter User über Browser  
**Ablauf:**
1. Client öffnet `WSS /ws?token=<jwt>` (Token im Query, weil Browser keine Header bei WS-Upgrade setzen können)
2. Server validiert JWT
3. Server registriert Connection in Redis (`conn:{userId}:{connId}`)
4. Server schickt `connected`-Frame mit `connection_id` und `server_time`
5. Server abonniert auto-channel `user:{userId}` (jeder bekommt seinen persönlichen Channel)
6. Heartbeat-Loop startet (siehe Abschnitt 9)

### UC-2: Channel subscribieren / unsubscribieren

**Akteur:** Verbundener Client  
**Ablauf:**
1. Client schickt `{type: "subscribe", channels: [...]}`
2. Server prüft pro Channel Permission
3. Bei Erfolg `SADD channel:{c} {connId}` und Antwort `subscribe_ack`
4. Bei Fehlschlag `subscribe_denied` mit Grund

Analog für `unsubscribe`.

### UC-3: Event-Push

**Akteur:** System (kein direkter User-Akteur)  
**Ablauf:**
1. Domain-Event aus RabbitMQ empfangen
2. Event-Mapping bestimmt: welcher Channel(s)? Welche User?
3. Bei persistenter Notification: in DB speichern
4. Über Redis Pub/Sub broadcasten an `broadcast`
5. Jede Notification-Service-Instanz prüft, ob sie betroffene Connections hält
6. Wenn ja: WebSocket-Frame senden

### UC-4: Persistente Notifications lesen

**Akteur:** User (online oder offline)  
**Ablauf:**
1. `GET /notifications?unread_only=true&limit=50&cursor=...`
2. JWT-Validierung
3. Liste aus DB (sortiert by `created_at` DESC)
4. Response: 200 mit Liste

### UC-5: Notification als gelesen markieren

**Akteur:** User  
**Ablauf:**
1. `POST /notifications/{id}/read` oder `POST /notifications/read-all`
2. Update `read_at = NOW()` für betroffene Notifications
3. Outbox-Event `notification.read` (intern; informiert ggf. andere offene Tabs des Users)
4. Response: 204

### UC-6: Notification löschen

**Akteur:** User  
**Ablauf:**
1. `DELETE /notifications/{id}`
2. Hard-Delete (Notification ist eh nicht semantisch wertvoll nach dem User-Dismiss)
3. Response: 204

### UC-7: Disconnect

**Akteur:** Client schließt Verbindung (oder Server detected stale Connection via Heartbeat-Timeout)  
**Ablauf:**
1. Connection cleanup: `DEL conn:{userId}:{connId}`, `SREM user_conns:{userId} {connId}`, Channel-Memberships entfernen
2. Logging mit Disconnect-Reason

---

## 6. Datenmodell

### 6.1 Tabellen

```sql
-- Persistente Notifications
CREATE TABLE notifications (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL,                  -- Empfänger
    type            TEXT NOT NULL,                   -- z. B. 'mention', 'task_assigned', 'comment_on_my_task'
    payload         JSONB NOT NULL,                  -- typ-spezifische Daten
    project_id      UUID NULL,                       -- für Filter / Cleanup bei project.deleted
    source_event_id TEXT NULL,                       -- Idempotenz: wenn Event 2x ankommt, keine Duplikate
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at         TIMESTAMPTZ NULL,
    
    CONSTRAINT notifications_type CHECK (type IN (
        'mention',
        'task_assigned',
        'task_unassigned',
        'task_due_soon',
        'task_deleted',
        'comment_on_my_task',
        'document_shared',
        'project_member_added',
        'project_member_removed'
    ))
);

CREATE INDEX idx_notifications_user_unread 
    ON notifications (user_id, created_at DESC) 
    WHERE read_at IS NULL;

CREATE INDEX idx_notifications_user_all 
    ON notifications (user_id, created_at DESC);

CREATE INDEX idx_notifications_project 
    ON notifications (project_id) 
    WHERE project_id IS NOT NULL;

CREATE UNIQUE INDEX idx_notifications_idempotency 
    ON notifications (user_id, source_event_id) 
    WHERE source_event_id IS NOT NULL;

-- Idempotenz für konsumierte Events (Service-übergreifend)
CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Outbox (für eigene Events)
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;
```

### 6.2 Designentscheidungen

**Warum `idx_notifications_idempotency` als unique index?**  
Verhindert duplizierte Notifications, wenn dasselbe RabbitMQ-Event zweimal ankommt (Redelivery, Retries). Bei N Empfängern eines Events gibt es N Notifications mit demselben `source_event_id`, daher Composite mit `user_id`.

**Warum `payload JSONB` statt typisierte Spalten?**  
Notification-Typen haben unterschiedliche Strukturen (Mention braucht `comment_id, task_id, author_id`, Assignment braucht `task_id, assigned_by`). JSONB ist hier idiomatisch — die Konsistenz wird im Code-Layer erzwungen, nicht im Schema.

**Warum keine Notification-Settings?**  
Zukunftsfähig wäre eine `notification_preferences`-Tabelle mit "Welche Typen will ich, in welchem Kanal?". Bewusst aus dem MVP rausgehalten — alle Notifications werden ohne Settings als In-App-Push geliefert. Erweiterung später durch Filter im Push-Pfad.

**Warum keine `connections`-Tabelle in PostgreSQL?**  
Connection-State gehört nicht in die DB — flüchtig, hochfrequent updated, nur für die aktive Session relevant. Redis ist genau richtig dafür.

---

## 7. Domain-Modell

### 7.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Notification struct {
    ID            uuid.UUID
    UserID        uuid.UUID
    Type          NotificationType
    Payload       map[string]any
    ProjectID     *uuid.UUID
    SourceEventID *string
    CreatedAt     time.Time
    ReadAt        *time.Time
}

type NotificationType string

const (
    TypeMention                NotificationType = "mention"
    TypeTaskAssigned           NotificationType = "task_assigned"
    TypeTaskUnassigned         NotificationType = "task_unassigned"
    TypeTaskDueSoon            NotificationType = "task_due_soon"
    TypeTaskDeleted            NotificationType = "task_deleted"
    TypeCommentOnMyTask        NotificationType = "comment_on_my_task"
    TypeDocumentShared         NotificationType = "document_shared"
    TypeProjectMemberAdded     NotificationType = "project_member_added"
    TypeProjectMemberRemoved   NotificationType = "project_member_removed"
)

// Connection-Repräsentation (in-memory, nicht in DB)
type Connection struct {
    ID         string
    UserID     uuid.UUID
    InstanceID string
    Subscribed map[string]bool  // Channel-Set
    Send       chan []byte      // Outbound-Frames
    LastPing   time.Time
}

// Channel-Identifier
type Channel string

func UserChannel(userID uuid.UUID) Channel    { return Channel("user:" + userID.String()) }
func ProjectChannel(projectID uuid.UUID) Channel { return Channel("project:" + projectID.String()) }
func BoardChannel(boardID uuid.UUID) Channel  { return Channel("board:" + boardID.String()) }
func TaskChannel(taskID uuid.UUID) Channel    { return Channel("task:" + taskID.String()) }
```

### 7.2 Service-Interfaces

```go
package domain

// NotificationService — REST-API
type NotificationService interface {
    ListNotifications(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]*Notification, *Cursor, error)
    GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error)
    MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error
    MarkAllRead(ctx context.Context, userID uuid.UUID) error
    DeleteNotification(ctx context.Context, notificationID, userID uuid.UUID) error
}

// PushService — Echtzeit-Layer
type PushService interface {
    PushToUser(ctx context.Context, userID uuid.UUID, frame Frame) error
    PushToChannel(ctx context.Context, channel Channel, frame Frame) error
    Subscribe(ctx context.Context, conn *Connection, channels []Channel) ([]Channel, []Channel, error) // ok, denied
    Unsubscribe(ctx context.Context, conn *Connection, channels []Channel) error
    RegisterConnection(ctx context.Context, conn *Connection) error
    DeregisterConnection(ctx context.Context, conn *Connection) error
}

// EventDispatcher — Event-zu-Notification-Mapping
type EventDispatcher interface {
    Dispatch(ctx context.Context, envelope Envelope) error
}

type ListFilter struct {
    UnreadOnly bool
    Type       *NotificationType
    Limit      int
    Cursor     *string
}
```

### 7.3 Domain-Fehler

```go
package domain

var (
    ErrNotificationNotFound  = &Error{Code: "notification_not_found"}
    ErrPermissionDenied      = &Error{Code: "permission_denied"}
    ErrInvalidChannel        = &Error{Code: "invalid_channel"}
    ErrSubscriptionDenied    = &Error{Code: "subscription_denied"}
    ErrConnectionNotFound    = &Error{Code: "connection_not_found"}
    ErrUnauthorized          = &Error{Code: "unauthorized"}
)
```

---

## 8. WebSocket-Protokoll

### 8.1 Frame-Format

Alle Frames sind JSON. Verbindliches Top-Level-Schema:

```json
{
  "type": "<frame-type>",
  "id": "<correlation-id, optional>",
  "data": { ... }
}
```

### 8.2 Client → Server Frames

#### `subscribe`
```json
{
  "type": "subscribe",
  "id": "client-correlation-1",
  "data": {
    "channels": ["board:550e...", "task:660f..."]
  }
}
```

#### `unsubscribe`
```json
{
  "type": "unsubscribe",
  "data": {
    "channels": ["task:660f..."]
  }
}
```

#### `ping`
```json
{ "type": "ping" }
```

### 8.3 Server → Client Frames

#### `connected` (initial nach erfolgreichem Connect)
```json
{
  "type": "connected",
  "data": {
    "connection_id": "conn-abc-123",
    "server_time": "2026-05-07T12:00:00Z",
    "auto_subscribed": ["user:<userId>"]
  }
}
```

#### `subscribe_ack`
```json
{
  "type": "subscribe_ack",
  "id": "client-correlation-1",
  "data": {
    "subscribed": ["board:550e..."],
    "denied": [
      {"channel": "task:660f...", "reason": "permission_denied"}
    ]
  }
}
```

#### `event` — domain event, der den Client interessiert
```json
{
  "type": "event",
  "data": {
    "channel": "board:550e...",
    "event_type": "task.moved",
    "occurred_at": "2026-05-07T12:00:01Z",
    "trace_id": "abc123",
    "payload": {
      "task_id": "660f...",
      "from_column_id": "...",
      "to_column_id": "...",
      "position": "Um"
    }
  }
}
```

#### `notification` — persistent notification (auch via REST abrufbar)
```json
{
  "type": "notification",
  "data": {
    "notification_id": "770a...",
    "notification_type": "mention",
    "created_at": "2026-05-07T12:00:01Z",
    "payload": {
      "task_id": "...",
      "comment_id": "...",
      "author_id": "...",
      "body_excerpt": "Hey @alice, can you check this?"
    }
  }
}
```

#### `pong`
```json
{ "type": "pong", "data": { "server_time": "..." } }
```

#### `error`
```json
{
  "type": "error",
  "id": "client-correlation-1",
  "data": {
    "code": "invalid_channel",
    "message": "Channel format invalid"
  }
}
```

### 8.4 Verbindungs-Schließen

Server schließt mit Standard-Close-Codes:

| Code | Bedeutung |
|------|-----------|
| 1000 | Normal | 
| 1008 | Policy Violation (z. B. zu viele Subscribes) |
| 4001 | Custom: Auth Failure (Token expired during session) |
| 4002 | Custom: Idle Timeout (kein Ping in 60s) |

---

## 9. Connection-Lifecycle

### 9.1 Connect-Phase

```
Client                     Notification Svc           Auth Svc          Redis
   │                              │                       │                │
   │ WSS /ws?token=<jwt>          │                       │                │
   ├─────────────────────────────>│                       │                │
   │                              │ Validate JWT (JWKS)   │                │
   │                              ├──────────────────────>│                │
   │                              │<──────────────────────┤                │
   │                              │ Generate connId       │                │
   │                              │ SET conn:{u}:{c}      │                │
   │                              ├───────────────────────────────────────>│
   │                              │ SADD user_conns:{u}   │                │
   │                              ├───────────────────────────────────────>│
   │                              │ Auto-subscribe        │                │
   │                              │  user:{userId}        │                │
   │                              │ SADD channel:user:{u} │                │
   │                              ├───────────────────────────────────────>│
   │ {type: connected, ...}       │                       │                │
   │<─────────────────────────────┤                       │                │
```

### 9.2 Heartbeat

Bidirektional, alle 30 Sekunden:

- **Client → Server:** `{"type": "ping"}` alle 30s
- **Server → Client:** WebSocket Ping-Frame (Protokoll-Level) alle 30s
- **Connection-Refresh in Redis:** Server refresht TTL alle 30s (`EXPIRE conn:{u}:{c} 60`)

### 9.3 Idle-Timeout

Wenn Server in 60s kein Ping/Pong empfangen hat → schließe Verbindung mit Code 4002.

### 9.4 Token-Expiry

JWT hat 15min TTL. Wenn Token während Session abläuft:
- Server prüft beim Subscribe **erneut** (Permissions können sich geändert haben)
- Token-Validity wird nicht aktiv im laufenden Connection geprüft (zu teuer)
- **MVP-Ansatz:** Long-lived WebSocket akzeptiert, dass JWT abläuft. Permission-Checks hängen am Project-Service-Cache, nicht am JWT.
- **Erweitert:** Reconnect alle 14min (Client-Logik), bei jedem Connect frisches Token.

### 9.5 Reconnect

Client-seitig: Bei Connection-Loss exponential backoff (1s, 2s, 4s, ..., max 30s) + neues Token holen via Refresh.

Bei Reconnect: alle Subscriptions müssen neu eingerichtet werden — Server speichert keine Subscriptions über Disconnects hinweg.

### 9.6 Disconnect-Cleanup

```go
func (c *Connection) cleanup(ctx context.Context, redis *redis.Client) {
    // Unsubscribe von allen Channels
    for ch := range c.Subscribed {
        redis.SRem(ctx, "channel:"+ch, c.ID)
    }
    // Connection-Eintrag entfernen
    redis.Del(ctx, fmt.Sprintf("conn:%s:%s", c.UserID, c.ID))
    redis.SRem(ctx, fmt.Sprintf("user_conns:%s", c.UserID), c.ID)
    // Outbound-Channel schließen
    close(c.Send)
}
```

---

## 10. Event-zu-Notification-Mapping

### 10.1 Mapping-Tabelle

Jeder konsumierte Domain-Event wird zu einem oder mehreren Notifications + Channel-Events verarbeitet.

| Domain-Event | → Channel-Push | → Persistente Notification (an wen) |
|--------------|----------------|-------------------------------------|
| `task.created` | `board:{boardId}` | — |
| `task.updated` | `board:{boardId}`, `task:{taskId}` | — |
| `task.moved` | `board:{boardId}`, `task:{taskId}` | — |
| `task.assigned` | `task:{taskId}` | `task_assigned` an neuen Assignee (wenn ≠ assigner) |
| `task.unassigned` | `task:{taskId}` | `task_unassigned` an alten Assignee (wenn ≠ unassigner) |
| `task.status.changed` | `board:{boardId}`, `task:{taskId}` | — |
| `task.deleted` | `board:{boardId}`, `task:{taskId}` | `task_deleted` an Assignee (wenn vorhanden, ≠ deleter) |
| `task.commented` | `task:{taskId}` | für jede `mentions[]`: `mention` an User<br>+ `comment_on_my_task` an Task-Assignee (wenn ≠ author und nicht in mentions) |
| `task.attachment.added` | `task:{taskId}` | — |
| `document.uploaded` | `project:{projectId}` | — |
| `project.member.added` | `project:{projectId}` | `project_member_added` an neues Member |
| `project.member.removed` | `project:{projectId}` | `project_member_removed` an entferntes Member |
| `project.deleted` | `project:{projectId}` | — |
| `board.created` | `project:{projectId}` | — |

### 10.2 Mapping-Implementation

```go
package events

type DispatchHandler interface {
    Handle(ctx context.Context, env Envelope) (*DispatchResult, error)
}

type DispatchResult struct {
    ChannelPushes []ChannelPush
    Notifications []NotificationToCreate
}

type ChannelPush struct {
    Channel domain.Channel
    Frame   wsproto.Frame
}

type NotificationToCreate struct {
    UserID   uuid.UUID
    Type     domain.NotificationType
    Payload  map[string]any
    ProjectID *uuid.UUID
}

// Handler-Registry pro Event-Type
type Dispatcher struct {
    handlers map[string]DispatchHandler
}

func (d *Dispatcher) Dispatch(ctx context.Context, env Envelope) error {
    handler, ok := d.handlers[env.EventType]
    if !ok {
        // Unbekannter Event-Type → einfach ignorieren (forward-compatible)
        return nil
    }
    
    result, err := handler.Handle(ctx, env)
    if err != nil { return err }
    
    // 1. Persistente Notifications in DB schreiben (transaktional, mit Idempotenz)
    for _, n := range result.Notifications {
        // INSERT ... ON CONFLICT DO NOTHING
    }
    
    // 2. Channel-Pushes via Redis Pub/Sub broadcasten
    for _, p := range result.ChannelPushes {
        d.broadcast.Publish(ctx, "broadcast:"+string(p.Channel), p.Frame)
    }
    
    // 3. Notifications zusätzlich als Push (an betroffene User)
    for _, n := range result.Notifications {
        d.broadcast.Publish(ctx, "broadcast:user:"+n.UserID.String(), notificationFrame(n))
    }
    
    return nil
}
```

### 10.3 Beispiel-Handler: `task.commented`

```go
type taskCommentedHandler struct {
    projectClient projectclient.Client
}

func (h *taskCommentedHandler) Handle(ctx context.Context, env Envelope) (*DispatchResult, error) {
    var p TaskCommentedPayload
    if err := json.Unmarshal(env.Payload, &p); err != nil { return nil, err }
    
    result := &DispatchResult{}
    
    // 1. Channel-Push an alle Task-Subscriber
    result.ChannelPushes = append(result.ChannelPushes, ChannelPush{
        Channel: domain.TaskChannel(p.TaskID),
        Frame: wsproto.EventFrame(env, "task.commented", p),
    })
    
    // 2. Mention-Notifications
    for _, mentionedUserID := range p.Mentions {
        if mentionedUserID == p.AuthorID { continue }  // self-mention skip
        result.Notifications = append(result.Notifications, NotificationToCreate{
            UserID: mentionedUserID,
            Type: domain.TypeMention,
            Payload: map[string]any{
                "task_id": p.TaskID,
                "comment_id": p.CommentID,
                "author_id": p.AuthorID,
                "body_excerpt": p.BodyExcerpt,
            },
            ProjectID: &p.ProjectID,
        })
    }
    
    // 3. Notification an Task-Assignee (wenn nicht author und nicht in mentions)
    if p.AssigneeID != nil && *p.AssigneeID != p.AuthorID && !contains(p.Mentions, *p.AssigneeID) {
        result.Notifications = append(result.Notifications, NotificationToCreate{
            UserID: *p.AssigneeID,
            Type: domain.TypeCommentOnMyTask,
            Payload: map[string]any{
                "task_id": p.TaskID,
                "comment_id": p.CommentID,
                "author_id": p.AuthorID,
            },
            ProjectID: &p.ProjectID,
        })
    }
    
    return result, nil
}
```

---

## 11. HTTP-API (OpenAPI)

### 11.1 OpenAPI 3.1 (Auszug)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Notification Service
  version: 1.0.0
servers:
  - url: http://localhost:8005/api/v1

paths:
  /notifications:
    get:
      summary: List notifications for current user
      operationId: listNotifications
      tags: [notifications]
      security: [{bearerAuth: []}]
      parameters:
        - in: query
          name: unread_only
          schema: { type: boolean, default: false }
        - in: query
          name: type
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
              schema: { $ref: '#/components/schemas/NotificationList' }

  /notifications/unread-count:
    get:
      summary: Get unread count
      operationId: getUnreadCount
      tags: [notifications]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                required: [data]
                properties:
                  data:
                    type: object
                    properties:
                      unread_count: { type: integer }

  /notifications/{id}/read:
    parameters:
      - in: path
        name: id
        required: true
        schema: { type: string, format: uuid }
    post:
      summary: Mark single notification as read
      operationId: markRead
      tags: [notifications]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Marked }

  /notifications/read-all:
    post:
      summary: Mark all notifications as read
      operationId: markAllRead
      tags: [notifications]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: All marked }

  /notifications/{id}:
    parameters:
      - in: path
        name: id
        required: true
        schema: { type: string, format: uuid }
    delete:
      summary: Delete notification
      operationId: deleteNotification
      tags: [notifications]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /ws:
    get:
      summary: WebSocket upgrade endpoint
      operationId: openWebSocket
      tags: [websocket]
      parameters:
        - in: query
          name: token
          required: true
          schema: { type: string }
          description: JWT (Access Token)
      responses:
        '101':
          description: Switching Protocols (WebSocket established)
        '401':
          description: Invalid or expired token

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
        '503': { description: Not ready (DB or Redis unreachable) }

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

  schemas:
    Notification:
      type: object
      required: [id, type, payload, created_at]
      properties:
        id: { type: string, format: uuid }
        type:
          type: string
          enum:
            - mention
            - task_assigned
            - task_unassigned
            - task_due_soon
            - task_deleted
            - comment_on_my_task
            - document_shared
            - project_member_added
            - project_member_removed
        payload: { type: object, additionalProperties: true }
        project_id: { type: string, format: uuid, nullable: true }
        created_at: { type: string, format: date-time }
        read_at: { type: string, format: date-time, nullable: true }

    NotificationList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Notification' }
        pagination:
          type: object
          properties:
            next_cursor: { type: string, nullable: true }
            limit: { type: integer }
        meta:
          type: object
          properties:
            unread_count: { type: integer }
```

### 11.2 Endpoint-Übersicht

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|--------------|
| GET | `/notifications` | Bearer | Liste persistenter Notifications |
| GET | `/notifications/unread-count` | Bearer | Unread-Counter |
| POST | `/notifications/{id}/read` | Bearer | Eine als gelesen |
| POST | `/notifications/read-all` | Bearer | Alle als gelesen |
| DELETE | `/notifications/{id}` | Bearer | Löschen |
| GET | `/ws?token=<jwt>` | Query-Token | WebSocket-Upgrade |
| GET | `/health/{live,ready}` | — | Health-Checks |

### 11.3 AsyncAPI für WebSocket

In `docs/api/notification.asyncapi.yaml` wird die WebSocket-Schnittstelle separat dokumentiert (analog zu OpenAPI für REST). Frame-Schemas dort definiert.

---

## 12. Permission-Filterung

### 12.1 Wann wird geprüft?

**Bei Subscribe** (UC-2): Vor jedem Channel-Beitritt prüfen, ob User den Channel sehen darf.

**Bei Push** (UC-3): Theoretisch nicht mehr nötig, weil Channel-Membership beim Subscribe geprüft wurde. **Praktisch aber doch**, weil:
- Permissions sich nach Subscribe ändern können (Member entfernt aus Projekt)
- Persistente Notifications adressieren explizite User — kein Channel-basierter Filter nötig

### 12.2 Re-Verifikation bei Push

**Channel-Push:** Vertraut auf Subscribe-Permission. Wenn ein User entfernt wird (`project.member.removed`), verarbeitet der Notification Service dieses Event und kickt den User aus betroffenen Channels.

**Re-Permission auf jede Notification (pessimistisch):** zu teuer, hot path.

### 12.3 Auto-Cleanup bei Permission-Loss

```go
// Handler für project.member.removed
func (h *memberRemovedHandler) Handle(ctx context.Context, env Envelope) (*DispatchResult, error) {
    var p MemberRemovedPayload
    json.Unmarshal(env.Payload, &p)
    
    // Kicke User aus allen Project-/Board-/Task-Channels dieses Projekts
    h.pushService.UnsubscribeUserFromProject(ctx, p.UserID, p.ProjectID)
    
    return &DispatchResult{
        Notifications: []NotificationToCreate{{
            UserID: p.UserID,
            Type: domain.TypeProjectMemberRemoved,
            ...
        }},
    }, nil
}
```

`UnsubscribeUserFromProject` muss alle Connections des Users finden und die zugehörigen Channels (project, alle boards, alle tasks dieses Projekts) entfernen.

### 12.4 Project-Service-Caching

Permission-Lookups über Project-Service-Client mit denselben Einstellungen wie Task/Document Service:
- 30s lokaler Cache
- 200ms Timeout
- Circuit Breaker

---

## 13. Konfiguration

### 13.1 Environment-Variablen

```bash
SERVICE_NAME=notification-service
SERVICE_PORT=8005
SERVICE_INSTANCE_ID=                          # auto-generated UUID at startup, oder via K8s Pod-Name
LOG_LEVEL=info

# Database
DB_URL=postgres://notification:notification@postgres:5432/notification_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# Redis
REDIS_URL=redis://redis:6379/2
REDIS_CONNECTION_TTL=60s
REDIS_HEARTBEAT_INTERVAL=30s

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
RABBITMQ_CONSUMER_QUEUE=notification-service-queue
RABBITMQ_BINDING_KEYS=task.*,project.*,board.*,document.*,user.*

# WebSocket
WS_PATH=/ws
WS_READ_TIMEOUT=60s
WS_WRITE_TIMEOUT=10s
WS_PING_INTERVAL=30s
WS_MAX_MESSAGE_SIZE=8192
WS_MAX_SUBSCRIPTIONS_PER_CONNECTION=50
WS_MAX_CONNECTIONS_PER_USER=10

# Auth (JWKS)
JWT_JWKS_URL=http://auth:8001/.well-known/jwks.json
JWT_ISSUER=https://auth.teamboard.local
JWT_AUDIENCE=teamboard-api

# Project Service
PROJECT_SERVICE_URL=http://project:8002
PROJECT_SERVICE_TIMEOUT=200ms
PERMISSION_CACHE_TTL=30s

# Service-Token
SERVICE_TOKEN_SECRET=<from-secrets-manager>

# Cleanup
NOTIFICATION_RETENTION_DAYS=90              # Notifications älter als das werden gelöscht

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=notification-service
```

### 13.2 Config-Struct

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"notification-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8005"`
    InstanceID  string `env:"SERVICE_INSTANCE_ID"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
    }

    Redis struct {
        URL               string        `env:"REDIS_URL,required"`
        ConnectionTTL     time.Duration `env:"REDIS_CONNECTION_TTL" envDefault:"60s"`
        HeartbeatInterval time.Duration `env:"REDIS_HEARTBEAT_INTERVAL" envDefault:"30s"`
    }

    RabbitMQ struct {
        URL           string   `env:"RABBITMQ_URL,required"`
        Exchange      string   `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
        ConsumerQueue string   `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"notification-service-queue"`
        BindingKeys   []string `env:"RABBITMQ_BINDING_KEYS" envSeparator:","`
    }

    WS struct {
        Path                       string        `env:"WS_PATH" envDefault:"/ws"`
        ReadTimeout                time.Duration `env:"WS_READ_TIMEOUT" envDefault:"60s"`
        WriteTimeout               time.Duration `env:"WS_WRITE_TIMEOUT" envDefault:"10s"`
        PingInterval               time.Duration `env:"WS_PING_INTERVAL" envDefault:"30s"`
        MaxMessageSize             int64         `env:"WS_MAX_MESSAGE_SIZE" envDefault:"8192"`
        MaxSubscriptionsPerConn    int           `env:"WS_MAX_SUBSCRIPTIONS_PER_CONNECTION" envDefault:"50"`
        MaxConnectionsPerUser      int           `env:"WS_MAX_CONNECTIONS_PER_USER" envDefault:"10"`
    }

    JWT struct {
        JWKSURL  string `env:"JWT_JWKS_URL,required"`
        Issuer   string `env:"JWT_ISSUER,required"`
        Audience string `env:"JWT_AUDIENCE,required"`
    }

    ProjectService struct {
        URL                string        `env:"PROJECT_SERVICE_URL,required"`
        Timeout            time.Duration `env:"PROJECT_SERVICE_TIMEOUT" envDefault:"200ms"`
        PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" envDefault:"30s"`
    }

    Security struct {
        ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`
    }

    Cleanup struct {
        NotificationRetentionDays int `env:"NOTIFICATION_RETENTION_DAYS" envDefault:"90"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }
}
```

---

## 14. Verzeichnisstruktur

```
services/domain/notification/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── middleware.go
│   │   ├── handlers_notifications.go
│   │   ├── handlers_health.go
│   │   ├── ws_handler.go                # WebSocket-Upgrade-Handler
│   │   ├── dto.go
│   │   └── generated.go
│   ├── ws/
│   │   ├── connection.go                # Connection struct + read/write loops
│   │   ├── hub.go                       # Lokales Connection-Registry pro Instanz
│   │   ├── proto.go                     # Frame-Definitionen + Marshal
│   │   └── proto_test.go
│   ├── push/
│   │   ├── service.go                   # PushService Interface
│   │   ├── service_impl.go
│   │   ├── backplane.go                 # Redis Pub/Sub Wrapper
│   │   └── connections_registry.go      # Redis Connection-Tracking
│   ├── domain/
│   │   ├── notification.go
│   │   ├── channel.go
│   │   ├── service.go                   # NotificationService Interface
│   │   ├── service_impl.go
│   │   └── errors.go
│   ├── repository/
│   │   ├── db/                          # sqlc-generiert
│   │   ├── repository.go
│   │   └── postgres.go
│   ├── projectclient/
│   │   ├── client.go
│   │   ├── http_client.go
│   │   └── permission_cache.go
│   ├── events/
│   │   ├── consumer.go                  # RabbitMQ-Konsument
│   │   ├── dispatcher.go                # Event-zu-Notification-Mapping
│   │   ├── handlers/
│   │   │   ├── task_created.go
│   │   │   ├── task_updated.go
│   │   │   ├── task_assigned.go
│   │   │   ├── task_commented.go        # mit Mention-Auswertung
│   │   │   ├── task_moved.go
│   │   │   ├── task_deleted.go
│   │   │   ├── document_uploaded.go
│   │   │   ├── project_member_added.go
│   │   │   ├── project_member_removed.go # incl. Channel-Cleanup
│   │   │   └── ...
│   │   └── envelope.go                  # (Publishing: shared outbox.Worker, verdrahtet in main.go)
│   ├── cleanup/
│   │   └── worker.go                    # Notification-Retention
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   └── 0001_init.down.sql
├── queries/
│   ├── notifications.sql
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

## 15. sqlc-Queries

### 15.1 `queries/notifications.sql`

```sql
-- name: CreateNotification :one
INSERT INTO notifications (id, user_id, type, payload, project_id, source_event_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, source_event_id) WHERE source_event_id IS NOT NULL
DO NOTHING
RETURNING *;

-- name: GetNotification :one
SELECT * FROM notifications
WHERE id = $1 AND user_id = $2;

-- name: ListNotifications :many
SELECT * FROM notifications
WHERE user_id = $1
  AND ($2::BOOLEAN = FALSE OR read_at IS NULL)
  AND ($3::TEXT IS NULL OR type = $3)
  AND ($4::TIMESTAMPTZ IS NULL OR created_at < $4)
ORDER BY created_at DESC, id DESC
LIMIT $5;

-- name: CountUnread :one
SELECT COUNT(*) FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkRead :exec
UPDATE notifications SET read_at = NOW()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllRead :exec
UPDATE notifications SET read_at = NOW()
WHERE user_id = $1 AND read_at IS NULL;

-- name: DeleteNotification :exec
DELETE FROM notifications
WHERE id = $1 AND user_id = $2;

-- name: DeleteNotificationsByProject :exec
DELETE FROM notifications WHERE project_id = $1;

-- name: DeleteOldNotifications :execrows
DELETE FROM notifications
WHERE created_at < $1;
```

### 15.2 `queries/outbox.sql`

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

### 15.3 `queries/processed_events.sql`

```sql
-- name: WasEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1);

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1)
ON CONFLICT DO NOTHING;
```

### 15.4 `sqlc.yaml`

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

## 16. Test-Strategie

### 16.1 Unit-Tests

```go
func TestChannelParsing(t *testing.T) {
    require.Equal(t, "user", parseChannelType("user:abc"))
    require.Error(t, validateChannel("invalid"))
}

func TestEventDispatcher_TaskCommented(t *testing.T) {
    dispatcher := buildDispatcher()
    env := Envelope{
        EventType: "task.commented",
        Payload: marshal(TaskCommentedPayload{
            TaskID: tid, ProjectID: pid, AuthorID: alice,
            AssigneeID: &bob,
            Mentions: []uuid.UUID{charlie},
            BodyExcerpt: "Hey @charlie",
        }),
    }
    
    result, err := dispatcher.Handle(ctx, env)
    require.NoError(t, err)
    
    // 2 Notifications: mention für Charlie, comment_on_my_task für Bob
    require.Len(t, result.Notifications, 2)
    require.Equal(t, charlie, result.Notifications[0].UserID)
    require.Equal(t, domain.TypeMention, result.Notifications[0].Type)
    require.Equal(t, bob, result.Notifications[1].UserID)
    require.Equal(t, domain.TypeCommentOnMyTask, result.Notifications[1].Type)
    
    // 1 Channel-Push
    require.Len(t, result.ChannelPushes, 1)
    require.Equal(t, domain.TaskChannel(tid), result.ChannelPushes[0].Channel)
}

func TestSelfMentionSkipped(t *testing.T) {
    // Author mentions himself → keine Mention-Notification
}

func TestChannelPermissionMapping(t *testing.T) {
    require.Equal(t, "project:550e", channelForBoardEvent("550e"))
}
```

### 16.2 Integration-Tests

```go
func TestPersistentNotificationLifecycle(t *testing.T) {
    pg := startPostgres(t)
    redis := startRedis(t)
    svc := buildService(pg, redis)
    
    t.Run("create unique by (user_id, source_event_id)", func(t *testing.T) {
        // Insert, then insert with same source_event_id → no duplicate
    })
    
    t.Run("mark read updates read_at", func(t *testing.T) { ... })
    t.Run("list filters by unread", func(t *testing.T) { ... })
}

func TestWebSocketConnectAndSubscribe(t *testing.T) {
    // Start full stack with testcontainers
    // Open WS connection
    // Subscribe to channel
    // Trigger event via RabbitMQ
    // Assert WS frame received
}

func TestBackplaneFanout(t *testing.T) {
    // Two service instances on same Redis
    // Connection from User A on Instance 1
    // Event arrives on Instance 2 → must reach User A's WS
}

func TestProjectMemberRemovedKicksUser(t *testing.T) {
    // User subscribed to project:X channel
    // project.member.removed event
    // Assert user got unsubscribed (no more events on that channel)
}
```

### 16.3 End-to-End-Tests

Postman-Collection für REST + eigenes Skript für WebSocket:

```javascript
// tests/e2e/ws-flow.js (Node.js)
const ws = new WebSocket(`ws://localhost:8005/ws?token=${TOKEN}`);

ws.on('message', (msg) => {
    const frame = JSON.parse(msg);
    if (frame.type === 'connected') {
        ws.send(JSON.stringify({
            type: 'subscribe',
            channels: [`board:${BOARD_ID}`],
        }));
    }
    // weitere Assertions
});
```

### 16.4 Load Test

```javascript
// tests/k6/ws_concurrent.js
import ws from 'k6/ws';
export const options = {
    scenarios: {
        connections: {
            executor: 'ramping-vus',
            stages: [
                { target: 500, duration: '30s' },
                { target: 500, duration: '5m' },
            ],
        },
    },
};
export default function () {
    ws.connect(`ws://localhost:8005/ws?token=${TOKEN}`, {}, (socket) => {
        socket.on('open', () => {
            socket.send(JSON.stringify({ type: 'subscribe', channels: ['user:...'] }));
            socket.setInterval(() => socket.send(JSON.stringify({ type: 'ping' })), 30000);
        });
        socket.setTimeout(() => socket.close(), 300000);
    });
}
```

Erwartung: 500 gleichzeitige Connections pro Instanz ohne Probleme. Memory ~100MB pro Instanz. Ping-RTT < 50ms.

---

## 17. Performance und Skalierung

### 17.1 Connections pro Instanz

| Aspekt | Wert |
|--------|------|
| Memory pro Connection | ~50KB (Buffer + Subscription-Set) |
| Goroutines pro Connection | 2 (Reader + Writer) |
| Praktisches Limit pro Instanz | 5.000–10.000 |
| Erweiterung | horizontal: weitere Instanzen, Load Balancer mit Sticky Sessions oder Reconnect-tolerantes Frontend |

### 17.2 Event-Throughput

| Aspekt | Wert |
|--------|------|
| Events/sec pro Instanz | ~5.000 (mit Permission-Cache-Hit) |
| Bottleneck | Redis Pub/Sub (Single-Node ~100k msg/s, reicht weit) |
| Skalierung | Redis Cluster für > 10 Instanzen |

### 17.3 Hot-Path-Optimierungen

1. **Permission-Cache lokal** (nicht nur Redis) — pro Worker 30s TTL
2. **Frame-Marshalling cachen** für Channel-Pushes — wenn 10 User in `board:X` sind, JSON nur einmal serialisieren
3. **Backpressure am Send-Channel** — wenn Client zu langsam, Connection schließen (kein Memory-Build-up)
4. **Batch-Verarbeitung** im Outbox-Publisher

### 17.4 Skalierungs-Achsen

| Last steigt durch | Skalierungs-Antwort |
|-------------------|---------------------|
| Mehr User-Connections | Mehr Service-Instanzen |
| Mehr Events/sec | Mehr Service-Instanzen + ggf. Redis Cluster |
| Mehr persistente Notifications | DB-Optimierung (Read-Replicas), TTL-basierter Cleanup |

---

## 18. Implementierungs-Hinweise für Coding-Agents

### 18.1 Implementierungsreihenfolge

1. **Migrations + sqlc-Setup** — Tabellen, `make generate` läuft
2. **Domain-Modell** (`internal/domain/`) — Notification, Channel, Errors
3. **Repository-Layer** mit Testcontainers
4. **WS-Proto** (`internal/ws/proto.go`) — Frame-Definitionen mit Tests
5. **Hub** (`internal/ws/hub.go`) — Lokales Connection-Registry, Goroutine-Sicherheit
6. **Connection** (`internal/ws/connection.go`) — Read/Write-Loops mit gorilla/websocket
7. **Backplane** (`internal/push/backplane.go`) — Redis Pub/Sub
8. **Connections-Registry in Redis** mit Heartbeat-Refresh
9. **PushService** — kombiniert Hub + Backplane + Connections-Registry
10. **Project-Client** für Permission-Checks
11. **Event-Dispatcher** (`internal/events/dispatcher.go`) — Mapping-Engine
12. **Handler pro Event-Type**
13. **WebSocket-Handler** im API-Layer
14. **REST-Handler** für `/notifications/*`
15. **Cleanup-Worker** für Retention
16. **Wiring in `main.go`**
17. **End-to-End-Tests**

### 18.2 Verbindliche Konventionen

- **WebSocket-Goroutinen sauber beenden** — jede Connection hat ein `context.Context` mit Cancel. Bei Disconnect alle Goroutinen über Context-Cancel beenden, nicht über Goroutine-Killing.
- **Send-Channel buffered** mit max 64 Frames. Wenn voll: Connection schließen (Slow Consumer).
- **Connection-Registry-Schreibvorgänge sind atomar** — Race zwischen Disconnect und Push wird durch `redis.SADD/SREM` mit korrektem Locking vermieden.
- **Permission-Check beim Subscribe**, nicht beim Push.
- **Channel-Membership-Cleanup bei `project.member.removed`** ist Pflicht — sonst leakt der Ex-Member weiter Events.
- **Idempotenz für Notifications** über `(user_id, source_event_id)` — `ON CONFLICT DO NOTHING`.
- **Token-Validierung passiert nur beim Connect**, nicht im laufenden Frame-Handling. Falls Permissions sich ändern, treibt das Project Service via Events.

### 18.3 Typische Stolperfallen

- **WebSocket-Library:** `github.com/gorilla/websocket` ist Standard. `nhooyr/websocket` ist moderner, aber `gorilla` hat mehr Beispiele und ist battle-tested. **Empfehlung: gorilla.**
- **JSON über WebSocket:** Frames können fragmentiert ankommen. `gorilla/websocket` reassembliert das automatisch beim `ReadMessage()`. Nicht selber aus Stream lesen.
- **Concurrent writes auf eine Connection** sind **nicht erlaubt** — `websocket.Conn` ist nicht goroutine-safe für Writes. Lösung: ein dedizierter Writer-Goroutine pro Connection, der alle Writes über einen Channel serialisiert.
- **Redis Pub/Sub ist fire-and-forget** — wenn ein Subscriber temporär nicht da ist, geht die Message verloren. Für unsere Use-Cases ok (Live-Push), aber nicht für persistente Notifications (die werden separat in DB persistiert).
- **Reconnect-Welle:** Wenn alle Clients gleichzeitig reconnecten (z. B. nach Service-Restart), staut sich die Last. Frontend-Backoff mit Jitter implementieren.
- **CORS für WebSocket:** WebSocket hat keine echte CORS-Prüfung wie HTTP — der Server prüft `Origin`-Header beim Upgrade selbst. Whitelist konfigurieren.
- **Token im Query-String:** Wird in Server-Logs landen. Logger muss `token=*` rausfiltern. Alternative: First-Frame-Auth (Client schickt erstes Frame mit Token), aber das macht das Protokoll komplexer.
- **Project-Replikat fehlt:** Anders als Task/Document Service brauchen wir keine `known_*`-Replikate, weil Permissions live beim Project Service abgefragt werden. Das ist OK, weil Notification-Events seltener sind als Task-Operationen.
- **Outbox vs direct Publish:** Notification Service erzeugt eigene Events (`notification.delivered`, `notification.read`) primär für Audit/Future-Use. Outbox-Pattern wie überall.

### 18.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration test-load migrate generate run docker-build

build:
	go build -o bin/notification ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/... ./internal/ws/... ./internal/events/...

test-integration:
	go test -race ./internal/repository/... ./internal/push/...

test-load:
	k6 run tests/k6/ws_concurrent.js

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/notification.openapi.yaml > internal/api/generated.go

run:
	air

docker-build:
	docker build -t teamboard/notification:latest .
```

### 18.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|-------------------|
| UC-1 Connect | JWT validiert, Connection in Redis, auto-subscribe `user:{id}`, `connected`-Frame |
| UC-2 Subscribe | Per-Channel Permission-Check, `subscribe_ack` mit ok/denied-Listen |
| UC-3 Event-Push | RabbitMQ-Event → Permission-Filter → Persistente Notifications in DB → Pub/Sub-Broadcast → WS-Frame an User |
| UC-4 List Notifications | Pagination, Filter (unread, type), Sort DESC |
| UC-5 Mark Read | Idempotent (mehrfaches Markieren OK), `read_at` gesetzt |
| UC-6 Delete | Hard-Delete, nur eigene Notifications |
| UC-7 Disconnect | Connection-Cleanup in Redis, alle Channel-Memberships entfernt, Goroutinen beendet |
| Multi-Instance | Event auf Instanz A, Connection auf Instanz B → User bekommt Push (Pub/Sub funktioniert) |
| Permission-Loss | `project.member.removed` → User wird aus allen Project-Channels entfernt |
| Heartbeat | Ohne Ping in 60s → Connection geschlossen mit Code 4002 |

---

## Anhang A: Sequenzdiagramm — Event-Push Multi-Instance

```
RabbitMQ        Notif Inst 1         Postgres        Redis           Notif Inst 2          User B (Inst 2)
   │                  │                  │              │                  │                      │
   │ task.commented   │                  │              │                  │                      │
   ├─────────────────>│                  │              │                  │                      │
   │                  │ Validate event   │              │                  │                      │
   │                  │ Check perms cache│              │                  │                      │
   │                  │ INSERT notify    │              │                  │                      │
   │                  │ (Charlie mention)│              │                  │                      │
   │                  ├─────────────────>│              │                  │                      │
   │                  │ INSERT processed_events         │                  │                      │
   │                  ├─────────────────>│              │                  │                      │
   │                  │ PUBLISH          │              │                  │                      │
   │                  │  broadcast:      │              │                  │                      │
   │                  │  user:charlie    │              │                  │                      │
   │                  ├─────────────────────────────────>│                  │                      │
   │                  │                  │              │ FANOUT to all subs│                      │
   │                  │                  │              ├─────────────────>│                      │
   │                  │ (Inst 1 hat Charlie nicht)      │                  │ Charlie ist hier     │
   │                  │                  │              │                  ├──────────────────────>│
   │                  │                  │              │                  │ {type: notification, │
   │                  │                  │              │                  │  notification_type:  │
   │                  │                  │              │                  │  mention, ...}       │
   │                  │ ACK              │              │                  │                      │
   │<─────────────────┤                  │              │                  │                      │
```

## Anhang B: Sequenzdiagramm — Subscribe mit Permission-Check

```
Client          WS-Handler          Push Svc           Project Svc      Redis
   │                 │                  │                    │              │
   │ {subscribe:     │                  │                    │              │
   │  ["board:X",    │                  │                    │              │
   │   "task:Y"]}    │                  │                    │              │
   ├────────────────>│                  │                    │              │
   │                 │ Subscribe(conn,  │                    │              │
   │                 │   channels)      │                    │              │
   │                 ├─────────────────>│                    │              │
   │                 │                  │ Lookup project for │              │
   │                 │                  │  board:X (cache)   │              │
   │                 │                  │ GetPermissions(p,u)│              │
   │                 │                  ├───────────────────>│              │
   │                 │                  │<───────────────────┤              │
   │                 │                  │ board:read? ✓      │              │
   │                 │                  │ Lookup project for │              │
   │                 │                  │  task:Y → ✗ no perm│              │
   │                 │                  │ SADD channel:board │              │
   │                 │                  │  :X conn-123       │              │
   │                 │                  ├───────────────────────────────────>│
   │                 │ {ok:[board:X],   │                    │              │
   │                 │  denied:[task:Y]}│                    │              │
   │                 │<─────────────────┤                    │              │
   │ {subscribe_ack: │                  │                    │              │
   │  ok:[board:X],  │                  │                    │              │
   │  denied:[...]}  │                  │                    │              │
   │<────────────────┤                  │                    │              │
```

---

**Ende des Detail-Designs Notification Service.**
