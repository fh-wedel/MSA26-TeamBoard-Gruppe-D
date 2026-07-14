# Plugin / Webhook Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Verwandtes Dokument:** [`services/project.md`](./project.md) — Authority für Permissions  
> **Service:** `plugin`  
> **Port (lokal):** 8006  
> **Datenbank:** `plugin_db` (PostgreSQL)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Begriffe: Plugin vs. Webhook](#2-begriffe-plugin-vs-webhook)
3. [Use-Cases](#3-use-cases)
4. [Datenmodell](#4-datenmodell)
5. [Domain-Modell](#5-domain-modell)
6. [Webhook-Delivery-Pipeline](#6-webhook-delivery-pipeline)
7. [Retry-Strategie und Backoff](#7-retry-strategie-und-backoff)
8. [Circuit Breaker pro Webhook](#8-circuit-breaker-pro-webhook)
9. [HMAC-Signatur und Empfänger-Verifikation](#9-hmac-signatur-und-empfänger-verifikation)
10. [Sicherheit: SSRF-Schutz](#10-sicherheit-ssrf-schutz)
11. [HTTP-API (OpenAPI)](#11-http-api-openapi)
12. [Plugin-Erweiterungspunkt](#12-plugin-erweiterungspunkt)
13. [Events](#13-events)
14. [Konfiguration](#14-konfiguration)
15. [Verzeichnisstruktur](#15-verzeichnisstruktur)
16. [sqlc-Queries](#16-sqlc-queries)
17. [Test-Strategie](#17-test-strategie)
18. [Implementierungs-Hinweise für Coding-Agents](#18-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Webhook-Verwaltung** pro Projekt (Registrierung, Aktualisierung, Löschen)
- **Event-Filter** pro Webhook (welche Event-Types interessieren?)
- **Webhook-Auslieferung** als ausgehender HTTP-Call mit Retry, Backoff, DLQ
- **HMAC-Signatur** ausgehender Requests
- **Delivery-Audit-Log** für Operator-Sichtbarkeit
- **Circuit Breaker** pro Webhook-URL
- **Plugin-Registry** als Erweiterungspunkt für zukünftige Plugin-Typen

### 1.2 Verantwortet NICHT

- **Inbound Webhooks** (externes System ruft uns) — nicht im MVP, später als separater Endpoint möglich
- **OAuth-Flows** für Drittanbieter-Integrationen
- **Bidirektionale Sync** mit externen Tools
- **Permission-Authority** — Project Service authoritative
- **Event-Erzeugung** — wir konsumieren, leiten weiter

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `plugin_db` | hart | Webhook-Konfiguration und Delivery-Log |
| RabbitMQ | hart | Event-Konsum |
| Project Service | hart | Permission-Checks für Webhook-CRUD |
| Auth Service (JWKS) | hart | JWT-Validierung |
| Externes Internet | hart (für Delivery) | Ausgehende HTTP-Calls — bei Ausfall: Retry, DLQ |
| Redis | weich | Circuit-Breaker-State (alternative: in-memory pro Worker) |

---

## 2. Begriffe: Plugin vs. Webhook

### 2.1 Webhook

Eine konkrete Konfiguration: "Bei Event-Type X aus Projekt Y schicke einen HTTP POST an URL Z mit HMAC-Signatur." Webhooks sind der einzige Plugin-Typ im MVP.

### 2.2 Plugin (Erweiterungs-Konzept)

Ein abstrakter Output-Adapter, der Domain-Events in eine externe Aktion umsetzt. Webhooks sind Plugins vom Typ `webhook`. Zukünftige Plugin-Typen (nicht im MVP):

- `slack` — Postet in Slack-Channel über Slack-Webhook-API mit Rich-Formatting
- `teams` — Microsoft Teams Adaptive Cards
- `email` — Sendet strukturierte E-Mail (anders als Notification-Service E-Mails)
- `discord` — Discord-Bot-Webhooks
- `transformer` — verändert Event und republiziert in den eigenen Bus (Workflow-Automation)

Die **Service-Struktur muss diese Erweiterung vorsehen**, ohne sie im MVP zu implementieren.

> **Abgrenzung — Board-Typen gehören NICHT hierher:** Plugins in diesem Service sind ausschließlich
> **Output-Adapter** (Event → externe Aktion). Die Erweiterbarkeit um neue **Board-Typen** ist ein
> separates, strukturelles Konzept und liegt im dedizierten **Board-Registry-Service** (Port 8007,
> siehe `docs/services/boardregistry.md`). Der Plugin/Webhook-Service bleibt Output-Adapter.

---

## 3. Use-Cases

### UC-1: Webhook erstellen

**Akteur:** User mit `webhook:manage` (im MVP: nur `owner`)  
**Ablauf:**
1. `POST /projects/{projectId}/webhooks` mit `target_url`, `event_filter[]`, optional `description`
2. Permission-Check
3. URL-Validierung (Schema, SSRF-Schutz, siehe Abschnitt 10)
4. Secret generieren (32 Byte random, base64)
5. Insert in `webhooks`, `secret` plain (im MVP — verschlüsselt at rest in Produktion)
6. Outbox-Event `webhook.created` (intern, aktuell ohne Konsumenten)
7. Response: 201 mit Webhook-Response **und einmalig dem Secret** (danach nie wieder ausgegeben)

### UC-2: Webhook aktualisieren

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `PATCH /webhooks/{id}` mit Partial-Update (`target_url`, `event_filter`, `description`, `active`)
2. Permission-Check (über `webhook.project_id`)
3. URL-Validierung wenn URL geändert
4. Update + Outbox-Event `webhook.updated`
5. Response: 200 (ohne Secret)

### UC-3: Webhook-Secret rotieren

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `POST /webhooks/{id}/rotate-secret`
2. Permission-Check
3. Neues Secret generieren, altes überschreiben
4. Response: 200 mit neuem Secret (einmalig zurückgegeben)

**Designentscheidung:** Kein "old-and-new"-Overlap im MVP. Empfänger muss kurz neuen Secret deployen können bevor weitere Events kommen, oder nimmt 1-2 fehlgeschlagene Verifikationen in Kauf. Erweiterung später: Dual-Secret-Phase.

### UC-4: Webhook deaktivieren / aktivieren

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `POST /webhooks/{id}/disable` oder `/webhooks/{id}/enable`
2. Permission-Check
3. Update `active` Flag
4. Response: 200

Deaktivierte Webhooks werden bei Event-Konsum übersprungen.

### UC-5: Webhook löschen

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `DELETE /webhooks/{id}`
2. Permission-Check
3. Hard-Delete (Webhook-Konfiguration); Delivery-Log bleibt für Audit
4. Outbox-Event `webhook.deleted`
5. Response: 204

### UC-6: Webhooks eines Projekts auflisten

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `GET /projects/{projectId}/webhooks`
2. Permission-Check
3. Response: Liste (ohne Secrets)

### UC-7: Delivery-Log einsehen

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `GET /webhooks/{id}/deliveries?status=failed&limit=50&cursor=...`
2. Permission-Check
3. Response: Liste der Zustellversuche mit Status, Antwort-Code, Latenz, Fehlerdetails

### UC-8: Webhook manuell auslösen (Test-Delivery)

**Akteur:** User mit `webhook:manage`  
**Ablauf:**
1. `POST /webhooks/{id}/test` mit optional `event_type` (default: `webhook.test`)
2. Permission-Check
3. Test-Payload erzeugen, durch normale Delivery-Pipeline schicken (mit Retry etc.)
4. Response: 202 mit `delivery_id` zur späteren Status-Abfrage

### UC-9: Domain-Event eintreffen → Webhook-Auslieferung

**Akteur:** System  
**Ablauf:**
1. Event aus RabbitMQ konsumieren
2. Idempotenz-Check (`processed_events`)
3. Webhooks suchen, die `event_type` matchen UND `project_id == event.aggregate_project_id` UND `active = TRUE`
4. Pro Match: Delivery-Job in `webhook_deliveries` einfügen mit Status `pending`, Outbox-Event `webhook.delivery.queued` (intern)
5. Worker pickt Pending-Deliveries auf und stellt aus (siehe Abschnitt 6)

---

## 4. Datenmodell

### 4.1 Tabellen

```sql
-- Webhooks
CREATE TABLE webhooks (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL,
    target_url      TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    secret          TEXT NOT NULL,                  -- in Produktion: AES-GCM-verschlüsselt
    event_filter    TEXT[] NOT NULL DEFAULT '{}',   -- z.B. ['task.created', 'task.*'] (Wildcard erlaubt)
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT webhooks_target_url_length CHECK (char_length(target_url) BETWEEN 8 AND 2000),
    CONSTRAINT webhooks_description_length CHECK (char_length(description) <= 500)
);

CREATE INDEX idx_webhooks_project_active ON webhooks (project_id) WHERE active = TRUE;
CREATE INDEX idx_webhooks_filter ON webhooks USING GIN (event_filter);

-- Delivery-Jobs (Audit-Log + Retry-Queue)
CREATE TABLE webhook_deliveries (
    id                    UUID PRIMARY KEY,
    webhook_id            UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id              TEXT NOT NULL,                -- aus Domain-Event-Envelope
    event_type            TEXT NOT NULL,
    payload               JSONB NOT NULL,
    status                TEXT NOT NULL DEFAULT 'pending', -- pending | delivered | failed | dead
    attempt_count         INTEGER NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_response_status  INTEGER NULL,
    last_response_body    TEXT NULL,                     -- truncated to first 4KB
    last_error            TEXT NULL,
    last_attempted_at     TIMESTAMPTZ NULL,
    delivered_at          TIMESTAMPTZ NULL,
    failed_permanently_at TIMESTAMPTZ NULL,
    duration_ms           INTEGER NULL,                  -- letzte Attempt-Dauer
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT webhook_deliveries_status CHECK (status IN ('pending', 'delivered', 'failed', 'dead')),
    CONSTRAINT webhook_deliveries_attempt_positive CHECK (attempt_count >= 0)
);

-- Worker-Pickup über (status, next_attempt_at). FOR UPDATE SKIP LOCKED.
CREATE INDEX idx_deliveries_pickup 
    ON webhook_deliveries (next_attempt_at) 
    WHERE status = 'pending';

CREATE INDEX idx_deliveries_webhook 
    ON webhook_deliveries (webhook_id, created_at DESC);

CREATE INDEX idx_deliveries_event 
    ON webhook_deliveries (event_id);

-- Idempotenz für konsumierte Events
CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Outbox (für interne Events wie webhook.created)
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

-- Bekannte Projekte (Replikat aus project.created/deleted Events)
CREATE TABLE known_projects (
    id              UUID PRIMARY KEY,
    deleted_at      TIMESTAMPTZ NULL
);
```

### 4.2 Designentscheidungen

**Warum `webhook_deliveries` als Queue + Audit kombiniert?**  
Eine einzige Tabelle für beides: Worker holt `status = 'pending'` mit `FOR UPDATE SKIP LOCKED`, aktualisiert nach Versuch. Erfolgreiche und gescheiterte Deliveries bleiben für Audit erhalten — `delivered`, `failed`, `dead` sind alles Endzustände, aber bleiben sichtbar.

**Warum nicht eine separate Job-Queue (z. B. RabbitMQ Delayed Message Plugin)?**  
DB-basierte Queue hat Vorteile:
- Einheitliche Persistenz (kein "Job war in Queue, aber nicht in DB"-Inkonsistenz)
- Volle SQL-Abfragbarkeit für Operator-Tools
- Outbox-Pattern überflüssig (Delivery-Job ist bereits persistent)
- `FOR UPDATE SKIP LOCKED` macht Multi-Worker safe

Nachteil: kein "delayed dispatch" wie bei RabbitMQ. Wir simulieren das durch `next_attempt_at + 1 second polling`.

**Warum `payload` als JSONB?**  
Volles Event-Envelope wird hier gespeichert, damit Retry vollständig autonom ist (nicht "muss original-Event aus Bus rekonstruieren"). Etwas Storage-Overhead, dafür Robustheit.

**Warum `last_response_body TEXT NULL` (statt JSONB)?**  
Empfänger antwortet was er will — JSON, HTML, Text. Wir speichern roh, truncated auf 4KB, für Operator-Debugging. Nicht parsierbar erforderlich.

**Warum kein eigener Index auf `status`?**  
`idx_deliveries_pickup` filtert schon auf `status = 'pending'` (partial index). Status-basierte Queries auf andere Status sind selten und mit anderen Filtern kombiniert.

---

## 5. Domain-Modell

### 5.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Webhook struct {
    ID          uuid.UUID
    ProjectID   uuid.UUID
    TargetURL   string
    Description string
    Secret      string                 // intern; nie in Responses (außer bei Create/Rotate)
    EventFilter []string               // z. B. ["task.created", "task.*"]
    Active      bool
    CreatedBy   uuid.UUID
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Delivery struct {
    ID                  uuid.UUID
    WebhookID           uuid.UUID
    EventID             string
    EventType           string
    Payload             map[string]any
    Status              DeliveryStatus
    AttemptCount        int
    NextAttemptAt       time.Time
    LastResponseStatus  *int
    LastResponseBody    *string
    LastError           *string
    LastAttemptedAt     *time.Time
    DeliveredAt         *time.Time
    FailedPermanentlyAt *time.Time
    DurationMs          *int
    CreatedAt           time.Time
}

type DeliveryStatus string

const (
    DeliveryStatusPending   DeliveryStatus = "pending"
    DeliveryStatusDelivered DeliveryStatus = "delivered"
    DeliveryStatusFailed    DeliveryStatus = "failed"  // versucht, aber wieder pending nach Backoff
    DeliveryStatusDead      DeliveryStatus = "dead"    // max attempts erreicht, DLQ
)
```

### 5.2 Service-Interfaces

```go
package domain

// Webhook-Verwaltung (REST)
type WebhookService interface {
    CreateWebhook(ctx context.Context, requester uuid.UUID, input CreateWebhookInput) (*Webhook, string, error)  // returns webhook + plain secret
    GetWebhook(ctx context.Context, webhookID, requester uuid.UUID) (*Webhook, error)
    ListWebhooks(ctx context.Context, projectID, requester uuid.UUID) ([]*Webhook, error)
    UpdateWebhook(ctx context.Context, webhookID, requester uuid.UUID, patch WebhookPatch) (*Webhook, error)
    RotateSecret(ctx context.Context, webhookID, requester uuid.UUID) (string, error)
    EnableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
    DisableWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
    DeleteWebhook(ctx context.Context, webhookID, requester uuid.UUID) error
    
    // Test-Delivery
    TriggerTest(ctx context.Context, webhookID, requester uuid.UUID, eventType string) (*Delivery, error)
    
    // Delivery-Log
    ListDeliveries(ctx context.Context, webhookID, requester uuid.UUID, filter DeliveryFilter) ([]*Delivery, *Cursor, error)
    GetDelivery(ctx context.Context, deliveryID, requester uuid.UUID) (*Delivery, error)
}

// Dispatcher (intern, vom Event-Konsumer aufgerufen)
type DispatcherService interface {
    EnqueueForEvent(ctx context.Context, env Envelope) error
}

// Worker (intern, eigene Goroutine)
type DeliveryWorker interface {
    Run(ctx context.Context) error
    DeliverNext(ctx context.Context) (delivered bool, err error)
}

type CreateWebhookInput struct {
    ProjectID   uuid.UUID
    TargetURL   string
    Description string
    EventFilter []string
}

type WebhookPatch struct {
    TargetURL   *string
    Description *string
    EventFilter *[]string
    Active      *bool
}

type DeliveryFilter struct {
    Status *DeliveryStatus
    EventType *string
    Limit  int
    Cursor *string
}
```

### 5.3 Domain-Fehler

```go
package domain

var (
    ErrWebhookNotFound       = &Error{Code: "webhook_not_found"}
    ErrDeliveryNotFound      = &Error{Code: "delivery_not_found"}
    ErrPermissionDenied      = &Error{Code: "permission_denied"}
    ErrInvalidURL            = &Error{Code: "invalid_url"}
    ErrPrivateURLForbidden   = &Error{Code: "private_url_forbidden"}      // SSRF-Schutz
    ErrInvalidEventFilter    = &Error{Code: "invalid_event_filter"}
    ErrProjectUnknown        = &Error{Code: "project_unknown"}
    ErrCircuitOpen           = &Error{Code: "circuit_open"}
    ErrValidation            = &Error{Code: "validation_failed"}
)
```

---

## 6. Webhook-Delivery-Pipeline

### 6.1 Pipeline-Übersicht

```
Domain-Event (RabbitMQ)
        │
        ▼
  Event-Consumer
        │
        ├─► Idempotenz-Check (processed_events)
        ▼
  Match Webhooks (project_id + event_filter)
        │
        ▼
  INSERT webhook_deliveries (status=pending) [N Zeilen, eine pro Match]
        │
        ▼
  Delivery-Worker (Polling-Loop, FOR UPDATE SKIP LOCKED)
        │
        ▼
  Circuit Breaker Check
        │
        ▼
  HTTP POST mit HMAC-Signatur
        │
        ▼
  Update delivery (delivered | failed)
        │
        ├─► failed: berechne next_attempt_at, status=pending oder dead
        ▼
  (dead → optional Notification an Webhook-Owner)
```

### 6.2 Event-Filter-Matching

```go
func matchesFilter(eventType string, filter []string) bool {
    for _, pattern := range filter {
        if pattern == eventType {
            return true
        }
        // Wildcard: "task.*" matches "task.created", "task.updated", etc.
        if strings.HasSuffix(pattern, ".*") {
            prefix := pattern[:len(pattern)-1]  // "task."
            if strings.HasPrefix(eventType, prefix) {
                return true
            }
        }
        // "*" matches everything
        if pattern == "*" {
            return true
        }
    }
    return false
}
```

**Designentscheidung:** Nur Wildcard am Ende und Top-Level `*`. Keine komplexen Glob-Pattern (`task.[cu]*ed`). Hält das Mental Model einfach.

### 6.3 Project-ID-Bestimmung im Event

Nicht alle Events haben offensichtlich einen `project_id`. Strategien:

| Event-Type | Quelle für `project_id` |
|------------|-------------------------|
| `task.*` | `payload.project_id` (denormalisiert in Task Service) |
| `project.*` | `aggregate_id` ist die Project-ID |
| `board.*` | `payload.project_id` |
| `document.*` | `payload.project_id` |
| `user.*` | **kein** `project_id` — diese Events erreichen keine Webhooks im MVP |

Implementierung: pro Event-Type-Familie ein kleiner Extractor:

```go
type ProjectIDExtractor func(env Envelope) (uuid.UUID, bool)

var extractors = map[string]ProjectIDExtractor{
    "task.":     extractFromPayload("project_id"),
    "project.":  extractFromAggregate,
    "board.":    extractFromPayload("project_id"),
    "document.": extractFromPayload("project_id"),
}
```

### 6.4 Delivery-Request-Format

```http
POST <target_url> HTTP/1.1
Content-Type: application/json
User-Agent: TeamBoard-Webhook/1.0
X-TeamBoard-Event: task.created
X-TeamBoard-Event-Id: 01HABC...
X-TeamBoard-Delivery-Id: 770a-...
X-TeamBoard-Signature: sha256=<hex-hmac>
X-TeamBoard-Timestamp: 1715000000

{
  "event_id": "01HABC...",
  "event_type": "task.created",
  "occurred_at": "2026-05-07T12:00:00Z",
  "trace_id": "abc123",
  "producer": "task-service",
  "aggregate_type": "task",
  "aggregate_id": "...",
  "actor": { "user_id": "...", "type": "user" },
  "payload": { ... }
}
```

### 6.5 Erfolgs-Kriterium

**Erfolgreich:** HTTP 2xx Status-Code  
**Fehlgeschlagen:** alles andere (3xx, 4xx, 5xx, Timeout, Connection-Error, DNS-Fehler)

Sonderfall: **HTTP 410 Gone** wird als "permanent failed" behandelt — Webhook automatisch deaktivieren und `delivery.status = dead`. Empfänger signalisiert damit explizit "diese URL existiert nicht mehr".

### 6.6 Timeout-Konfiguration

Pro Delivery:
- **Connection-Timeout:** 5 Sekunden
- **Total Request-Timeout:** 30 Sekunden
- **Response-Body-Lesen:** max 1 MB, danach truncate (vor Logging)

### 6.7 Worker-Schleife

```go
func (w *deliveryWorker) Run(ctx context.Context) error {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            for {
                delivered, err := w.DeliverNext(ctx)
                if err != nil {
                    w.logger.Warn("delivery iteration failed", "err", err)
                    break
                }
                if !delivered {
                    break  // keine pending deliveries → warten auf nächsten Tick
                }
            }
        }
    }
}

func (w *deliveryWorker) DeliverNext(ctx context.Context) (bool, error) {
    // 1. SELECT ... FOR UPDATE SKIP LOCKED
    delivery, err := w.repo.PickNextPendingDelivery(ctx)
    if err == ErrNoPending { return false, nil }
    if err != nil { return false, err }
    
    // 2. Webhook laden, Circuit Breaker prüfen
    webhook, err := w.repo.GetWebhook(ctx, delivery.WebhookID)
    if err != nil || !webhook.Active {
        // Webhook gelöscht/deaktiviert — als delivered markieren (oder dead)
        return true, w.repo.MarkDeliveryDead(ctx, delivery.ID, "webhook inactive")
    }
    
    if w.breaker.IsOpen(webhook.TargetURL) {
        // Verschiebe um Backoff weiter, ohne Versuch
        return true, w.repo.RescheduleDelivery(ctx, delivery.ID, time.Now().Add(30*time.Second))
    }
    
    // 3. HTTP-Call
    result := w.httpDeliver(ctx, webhook, delivery)
    
    // 4. Update DB
    return true, w.applyResult(ctx, delivery, result)
}
```

---

## 7. Retry-Strategie und Backoff

### 7.1 Schedule

| Versuch | Delay nach vorigem Fehler |
|---------|---------------------------|
| 1 | sofort (initialer Versuch) |
| 2 | 30 Sekunden |
| 3 | 2 Minuten |
| 4 | 10 Minuten |
| 5 | 30 Minuten |
| 6 | 2 Stunden |
| 7 | 6 Stunden |
| 8 | 24 Stunden |
| 9 | DEAD (Dead-Letter) |

Maximale Lebensdauer ein Delivery in pending: ~33 Stunden. Konfigurierbar via `MAX_DELIVERY_ATTEMPTS`.

**Jitter:** ±20% pro Backoff-Wert, damit nicht alle Retries gleichzeitig laufen, wenn Empfänger gerade wieder hochkommt.

### 7.2 Retry-Logik im Code

```go
func computeNextAttempt(attemptCount int) (time.Time, bool) {
    if attemptCount >= maxAttempts {
        return time.Time{}, false  // dead
    }
    schedule := []time.Duration{
        30 * time.Second,
        2 * time.Minute,
        10 * time.Minute,
        30 * time.Minute,
        2 * time.Hour,
        6 * time.Hour,
        24 * time.Hour,
    }
    base := schedule[min(attemptCount-1, len(schedule)-1)]
    jitter := time.Duration(rand.Int63n(int64(base) / 5)) - base/10
    return time.Now().Add(base + jitter), true
}
```

### 7.3 Sofortige DLQ-Bedingungen

Nicht jeder Fehler führt zur kompletten Retry-Sequence. Sofortiger DEAD-Status bei:

- HTTP 410 Gone (Empfänger explizit "weg")
- DNS-Auflösung schlägt nach 3 Versuchen fehl (URL eindeutig invalid)
- 4xx-Fehler **außer** 408 (Request Timeout) und 429 (Rate Limited) — Empfänger sagt explizit "deine Anfrage ist falsch"

4xx, die *retried* werden:
- 408 Request Timeout
- 425 Too Early
- 429 Too Many Requests (wir respektieren `Retry-After`-Header, falls vorhanden)

5xx und Connection-Errors werden immer retried.

### 7.4 Honoring `Retry-After`

Wenn Empfänger 429 oder 503 mit `Retry-After`-Header antwortet, nutzen wir diesen Wert statt unseres Backoff (capped auf 24h):

```go
if status == 429 || status == 503 {
    if retryAfter := parseRetryAfter(headers); retryAfter > 0 {
        return time.Now().Add(min(retryAfter, 24*time.Hour)), true
    }
}
return computeNextAttempt(attemptCount)
```

### 7.5 DLQ-Notification

Wenn Delivery `dead` wird:
1. Outbox-Event `webhook.delivery.dead` mit Webhook-ID, Event-ID, letztem Fehler
2. Möglicher Konsument (Notification Service) macht daraus eine User-Notification "Webhook hat 8 Stunden lang nicht funktioniert"

Im MVP nicht zwingend implementiert — Audit-Log reicht.

---

## 8. Circuit Breaker pro Webhook

### 8.1 Motivation

Wenn ein Webhook-Empfänger dauerhaft down ist, bringt es nichts, alle 30 Sekunden einen Versuch zu starten — wir blockieren Worker und produzieren Last. Circuit Breaker erkennt persistente Fehler und überspringt zeitweise.

### 8.2 Zustände

```
   Closed ───[5 Fehler in 1 Minute]──► Open
     ▲                                    │
     │                                    │ (30s)
     │                                    ▼
   [1 Erfolg]◄──────[1 Versuch]──── Half-Open
     ▲                                    │
     │                                    │
     └──────────[1 weiterer Fehler]───────┘
```

| Zustand | Verhalten |
|---------|-----------|
| Closed | Normale Auslieferung |
| Open | Alle Deliveries werden um 30s nach hinten verschoben, ohne HTTP-Call |
| Half-Open | Genau ein Test-Delivery erlaubt; bei Erfolg → Closed, bei Fehler → Open |

### 8.3 Implementierung

Library: `github.com/sony/gobreaker` mit pro-URL-Instanz.

```go
type breakerRegistry struct {
    breakers sync.Map  // map[targetURL]*gobreaker.CircuitBreaker
}

func (r *breakerRegistry) Get(url string) *gobreaker.CircuitBreaker {
    if b, ok := r.breakers.Load(url); ok {
        return b.(*gobreaker.CircuitBreaker)
    }
    settings := gobreaker.Settings{
        Name:        url,
        MaxRequests: 1,
        Interval:    1 * time.Minute,
        Timeout:     30 * time.Second,
        ReadyToTrip: func(c gobreaker.Counts) bool {
            return c.ConsecutiveFailures >= 5
        },
    }
    b := gobreaker.NewCircuitBreaker(settings)
    actual, _ := r.breakers.LoadOrStore(url, b)
    return actual.(*gobreaker.CircuitBreaker)
}
```

### 8.4 Verteilung über mehrere Worker-Instanzen

Lokaler Breaker pro Instanz reicht im MVP. Bei Multi-Instance-Setup mit hohem Traffic: geteilter Breaker via Redis (z. B. `gobreaker` mit Custom-Backend) — aber komplexer. Für MVP **nicht nötig**.

---

## 9. HMAC-Signatur und Empfänger-Verifikation

### 9.1 Signatur-Berechnung

```go
func signPayload(secret string, body []byte, timestamp int64) string {
    mac := hmac.New(sha256.New, []byte(secret))
    fmt.Fprintf(mac, "%d.", timestamp)
    mac.Write(body)
    return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
```

Signatur über `<timestamp>.<body>` (mit Punkt-Trenner). Verhindert Replay-Angriffe — Empfänger soll Timestamp prüfen (max 5 min Skew).

### 9.2 Header

```
X-TeamBoard-Signature: sha256=<hex>
X-TeamBoard-Timestamp: <unix-seconds>
```

### 9.3 Empfänger-Verifikation (Beispiel-Doku)

**Wird im Public-Doku des Webhooks erklärt** (separates Markdown-File `docs/webhooks-receiver-guide.md`):

```python
import hmac, hashlib, time

def verify(body: bytes, header_sig: str, header_ts: str, secret: str) -> bool:
    # Reject if timestamp older than 5 minutes
    if abs(time.time() - int(header_ts)) > 300:
        return False
    expected = "sha256=" + hmac.new(
        secret.encode(),
        f"{header_ts}.".encode() + body,
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, header_sig)
```

### 9.4 Secret-Rotation Implications

Bei Rotate-Secret-Call wird das alte Secret sofort überschrieben. Empfänger der gleichzeitig Anfragen verarbeitet, sieht eine kurze Phase mit ungültigen Signaturen. Sie führt zu HTTP-401-Antworten, die als "failed" geloggt werden und nach Backoff retried — bei dem Zeitpunkt sollte der Empfänger das neue Secret deployed haben.

---

## 10. Sicherheit: SSRF-Schutz

### 10.1 Das Problem

Webhook-URLs sind benutzerdefiniert. Ohne Schutz könnte ein User `http://localhost:8002/internal/...` registrieren und unsere internen Services über den Webhook-Service angreifen (Server-Side Request Forgery).

### 10.2 Validierung beim Create/Update

```go
func validateWebhookURL(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil { return ErrInvalidURL }
    
    // 1. Schema
    if u.Scheme != "http" && u.Scheme != "https" {
        return ErrInvalidURL
    }
    if u.Scheme == "http" && !allowInsecureHTTP {
        return ErrInvalidURL  // HTTP nur in Dev erlaubt
    }
    
    // 2. Hostname auflösen
    ips, err := net.LookupIP(u.Hostname())
    if err != nil { return ErrInvalidURL }
    
    // 3. Jeder IP muss public sein
    for _, ip := range ips {
        if isPrivateOrSpecial(ip) {
            return ErrPrivateURLForbidden
        }
    }
    
    // 4. Port-Whitelist
    port := u.Port()
    if port != "" && !isAllowedPort(port) {
        return ErrInvalidURL
    }
    
    return nil
}

func isPrivateOrSpecial(ip net.IP) bool {
    return ip.IsLoopback() ||
        ip.IsPrivate() ||
        ip.IsLinkLocalUnicast() ||
        ip.IsLinkLocalMulticast() ||
        ip.IsInterfaceLocalMulticast() ||
        ip.IsMulticast() ||
        // Cloud-Metadata-IPs
        ip.Equal(net.IPv4(169, 254, 169, 254)) ||  // AWS, GCP, Azure
        ip.Equal(net.IPv4(100, 100, 100, 200))     // Alibaba
}
```

### 10.3 Re-Validierung beim Delivery

Hostname kann zwischen Create und Delivery die IP wechseln (DNS-Rebinding-Angriff). Daher beim tatsächlichen HTTP-Call **erneut** prüfen:

```go
// Custom Dialer, der vor Connect die Resolved IP prüft
dialer := &net.Dialer{
    Timeout: 5 * time.Second,
    Control: func(network, address string, c syscall.RawConn) error {
        host, _, _ := net.SplitHostPort(address)
        ip := net.ParseIP(host)
        if isPrivateOrSpecial(ip) {
            return errors.New("private IP not allowed")
        }
        return nil
    },
}

httpClient := &http.Client{
    Transport: &http.Transport{
        DialContext: dialer.DialContext,
    },
    Timeout: 30 * time.Second,
}
```

Das ist die einzige zuverlässige Variante — Validierung beim Create allein reicht nicht.

### 10.4 Port-Whitelist

Standard-Ports erlaubt: 80, 443, 8080, 8443. Alles andere (z. B. 22 SSH, 25 SMTP, 6379 Redis) verboten. Konfigurierbar via `WEBHOOK_ALLOWED_PORTS`.

### 10.5 Body-Limit

Maximaler Response-Body: 1 MB. Verhindert Memory-Angriffe durch riesige Antworten.

### 10.6 Redirect-Handling

`http.Client` folgt per Default Redirects. Wir setzen `CheckRedirect`, der jede neue URL durch denselben SSRF-Check schickt.

---

## 11. HTTP-API (OpenAPI)

### 11.1 OpenAPI 3.1 (Auszug)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Plugin / Webhook Service
  version: 1.0.0
servers:
  - url: http://localhost:8006/api/v1

paths:
  /projects/{projectId}/webhooks:
    parameters:
      - $ref: '#/components/parameters/projectId'
    get:
      summary: List webhooks for a project
      operationId: listWebhooks
      tags: [webhooks]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebhookList' }

    post:
      summary: Create webhook
      operationId: createWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/CreateWebhookRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebhookCreatedResponse' }
            description: "Includes secret (visible only once)"

  /webhooks/{webhookId}:
    parameters:
      - $ref: '#/components/parameters/webhookId'
    get:
      summary: Get webhook
      operationId: getWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebhookResponse' }

    patch:
      summary: Update webhook
      operationId: updateWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/UpdateWebhookRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/WebhookResponse' }

    delete:
      summary: Delete webhook
      operationId: deleteWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /webhooks/{webhookId}/rotate-secret:
    parameters:
      - $ref: '#/components/parameters/webhookId'
    post:
      summary: Rotate webhook secret
      operationId: rotateSecret
      tags: [webhooks]
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
                      secret: { type: string, description: "New secret (visible only once)" }

  /webhooks/{webhookId}/enable:
    parameters:
      - $ref: '#/components/parameters/webhookId'
    post:
      summary: Enable webhook
      operationId: enableWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Enabled }

  /webhooks/{webhookId}/disable:
    parameters:
      - $ref: '#/components/parameters/webhookId'
    post:
      summary: Disable webhook
      operationId: disableWebhook
      tags: [webhooks]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Disabled }

  /webhooks/{webhookId}/test:
    parameters:
      - $ref: '#/components/parameters/webhookId'
    post:
      summary: Trigger test delivery
      operationId: triggerTestDelivery
      tags: [webhooks]
      security: [{bearerAuth: []}]
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
              properties:
                event_type: { type: string, default: "webhook.test" }
      responses:
        '202':
          content:
            application/json:
              schema:
                type: object
                required: [data]
                properties:
                  data:
                    type: object
                    properties:
                      delivery_id: { type: string, format: uuid }

  /webhooks/{webhookId}/deliveries:
    parameters:
      - $ref: '#/components/parameters/webhookId'
      - in: query
        name: status
        schema: { type: string, enum: [pending, delivered, failed, dead] }
      - in: query
        name: event_type
        schema: { type: string }
      - in: query
        name: limit
        schema: { type: integer, default: 50, maximum: 200 }
      - in: query
        name: cursor
        schema: { type: string }
    get:
      summary: List delivery attempts (audit log)
      operationId: listDeliveries
      tags: [deliveries]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeliveryList' }

  /webhooks/{webhookId}/deliveries/{deliveryId}:
    parameters:
      - $ref: '#/components/parameters/webhookId'
      - in: path
        name: deliveryId
        required: true
        schema: { type: string, format: uuid }
    get:
      summary: Get delivery details
      operationId: getDelivery
      tags: [deliveries]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DeliveryResponse' }

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
    webhookId:
      in: path
      name: webhookId
      required: true
      schema: { type: string, format: uuid }

  schemas:
    CreateWebhookRequest:
      type: object
      required: [target_url, event_filter]
      properties:
        target_url:
          type: string
          format: uri
          minLength: 8
          maxLength: 2000
        description:
          type: string
          maxLength: 500
        event_filter:
          type: array
          minItems: 1
          items: { type: string, pattern: '^[a-z][a-z0-9_]*(\.[a-z0-9_*]+)*$' }
          example: ["task.created", "task.updated"]

    UpdateWebhookRequest:
      type: object
      properties:
        target_url: { type: string, format: uri }
        description: { type: string, maxLength: 500 }
        event_filter:
          type: array
          minItems: 1
          items: { type: string }
        active: { type: boolean }

    Webhook:
      type: object
      required: [id, project_id, target_url, event_filter, active, created_by, created_at]
      properties:
        id: { type: string, format: uuid }
        project_id: { type: string, format: uuid }
        target_url: { type: string, format: uri }
        description: { type: string }
        event_filter:
          type: array
          items: { type: string }
        active: { type: boolean }
        created_by: { type: string, format: uuid }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }
        # Note: secret is never returned here

    WebhookResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Webhook' }

    WebhookCreatedResponse:
      type: object
      required: [data]
      properties:
        data:
          allOf:
            - $ref: '#/components/schemas/Webhook'
            - type: object
              properties:
                secret:
                  type: string
                  description: "Visible only at creation time"

    WebhookList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Webhook' }

    Delivery:
      type: object
      required: [id, webhook_id, event_id, event_type, status, attempt_count, created_at]
      properties:
        id: { type: string, format: uuid }
        webhook_id: { type: string, format: uuid }
        event_id: { type: string }
        event_type: { type: string }
        status: { type: string, enum: [pending, delivered, failed, dead] }
        attempt_count: { type: integer }
        next_attempt_at: { type: string, format: date-time, nullable: true }
        last_response_status: { type: integer, nullable: true }
        last_response_body: { type: string, nullable: true }
        last_error: { type: string, nullable: true }
        last_attempted_at: { type: string, format: date-time, nullable: true }
        delivered_at: { type: string, format: date-time, nullable: true }
        failed_permanently_at: { type: string, format: date-time, nullable: true }
        duration_ms: { type: integer, nullable: true }
        created_at: { type: string, format: date-time }

    DeliveryResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Delivery' }

    DeliveryList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Delivery' }
        pagination:
          type: object
          properties:
            next_cursor: { type: string, nullable: true }
            limit: { type: integer }
```

### 11.2 Endpoint-Übersicht

| Methode | Pfad | Permission | Beschreibung |
|---------|------|------------|--------------|
| GET | `/projects/{id}/webhooks` | `webhook:manage` | Liste |
| POST | `/projects/{id}/webhooks` | `webhook:manage` | Erstellen |
| GET | `/webhooks/{id}` | `webhook:manage` | Details |
| PATCH | `/webhooks/{id}` | `webhook:manage` | Update |
| DELETE | `/webhooks/{id}` | `webhook:manage` | Löschen |
| POST | `/webhooks/{id}/rotate-secret` | `webhook:manage` | Secret rotieren |
| POST | `/webhooks/{id}/enable` | `webhook:manage` | Aktivieren |
| POST | `/webhooks/{id}/disable` | `webhook:manage` | Deaktivieren |
| POST | `/webhooks/{id}/test` | `webhook:manage` | Test-Delivery |
| GET | `/webhooks/{id}/deliveries` | `webhook:manage` | Audit-Log |
| GET | `/webhooks/{id}/deliveries/{deliveryId}` | `webhook:manage` | Einzelne Delivery |

---

## 12. Plugin-Erweiterungspunkt

> Dieser Erweiterungspunkt betrifft **Output-Adapter-Plugins** (slack, email, …). Board-Typen sind
> hiervon getrennt und liegen im Board-Registry-Service (`docs/services/boardregistry.md`).

### 12.1 Plugin-Interface (Vorbereitung)

```go
package plugin

// Plugin verarbeitet Domain-Events und führt typ-spezifische Aktionen aus
type Plugin interface {
    Type() string                                                       // "webhook" | "slack" | "email" | ...
    Match(env Envelope, config map[string]any) bool                     // soll dieses Event verarbeitet werden?
    Deliver(ctx context.Context, env Envelope, config map[string]any) (*DeliveryResult, error)
}

type DeliveryResult struct {
    Success      bool
    StatusCode   *int
    Body         *string
    Error        *string
    DurationMs   int
}

// Registry
type PluginRegistry struct {
    plugins map[string]Plugin
}

func (r *PluginRegistry) Register(p Plugin) {
    r.plugins[p.Type()] = p
}
```

### 12.2 MVP-Implementation: nur `webhook`

```go
type webhookPlugin struct {
    httpClient *http.Client
    breakers   *breakerRegistry
}

func (p *webhookPlugin) Type() string { return "webhook" }

func (p *webhookPlugin) Match(env Envelope, config map[string]any) bool {
    filter, _ := config["event_filter"].([]string)
    return matchesFilter(env.EventType, filter)
}

func (p *webhookPlugin) Deliver(ctx context.Context, env Envelope, config map[string]any) (*DeliveryResult, error) {
    targetURL, _ := config["target_url"].(string)
    secret, _ := config["secret"].(string)
    return p.httpDeliver(ctx, targetURL, secret, env)
}
```

### 12.3 Erweiterung-Roadmap

| Plugin-Typ | Wann sinnvoll? | Datenmodell-Änderung |
|------------|----------------|----------------------|
| `slack` | Nutzer wollen native Slack-Notifications | Neue Tabelle `slack_integrations` ODER `webhooks.type = 'slack'` mit anderem Config-Schema in `webhooks.config JSONB` |
| `transformer` | Workflow-Automation | Eigene Tabelle `event_transformers` mit Code-Snippet |
| Inbound-Webhook | externe Systeme schicken Events an uns | Eigene Endpunkte `/webhooks/inbound/...` |

**Designentscheidung MVP:** `webhooks` bleibt single-purpose. Bei Erweiterung wird entweder ein Discriminator-Feld eingeführt oder eine separate Tabelle pro Plugin-Typ. Im MVP bewusst nicht generalisiert ("YAGNI" — You Aren't Gonna Need It).

---

## 13. Events

### 13.1 Publizierte Events

Plugin Service publiziert primär interne Lifecycle-Events (für Audit). Konsumiert wird er nicht direkt, aber Events stehen für künftige Erweiterungen bereit.

| Event-Type | Trigger | Payload |
|------------|---------|---------|
| `webhook.created` | UC-1 | `webhook_id, project_id, target_url, event_filter, created_by` |
| `webhook.updated` | UC-2 | `webhook_id, project_id, changes` |
| `webhook.deleted` | UC-5 | `webhook_id, project_id, deleted_by` |
| `webhook.delivery.queued` | UC-9 | `delivery_id, webhook_id, event_id` |
| `webhook.delivery.dead` | bei DLQ | `delivery_id, webhook_id, event_id, last_error, attempts` |

### 13.2 Konsumierte Events

| Event-Type | Source | Reaktion |
|------------|--------|----------|
| `user.deleted` | auth | (optional) Webhooks, deren `created_by` gelöschter User → markieren oder lassen für Audit |
| `project.created` | project | INSERT in `known_projects` |
| `project.deleted` | project | UPDATE `known_projects.deleted_at`; alle Webhooks dieses Projekts hard-delete; offene Pending-Deliveries als dead markieren |
| **Alle anderen Events** (`task.*`, `board.*`, `document.*`, etc.) | diverse | Event-Filter-Matching → Pending-Deliveries einfügen |

### 13.3 Filter-Effizienz

Pro Event durchsuchen wir alle aktiven Webhooks des Projekts. Bei vielen Webhooks pro Projekt (>100) wäre das problematisch. Skalierungs-Optionen:

- **Index auf `event_filter` (GIN)** — bereits vorhanden, ermöglicht `WHERE event_filter @> ARRAY['task.created']`
- **Cache** der aktiven Webhooks pro Projekt-ID (TTL 60s)

Im MVP unkritisch: Webhooks per Projekt sind selten (<10).

### 13.4 Idempotenz

`processed_events`-Check vor Verarbeitung. Wenn dasselbe Event zweimal ankommt, dürfen keine doppelten Pending-Deliveries entstehen.

---

## 14. Konfiguration

### 14.1 Environment-Variablen

```bash
SERVICE_NAME=plugin-service
SERVICE_PORT=8006
LOG_LEVEL=info

# Database
DB_URL=postgres://plugin:plugin@postgres:5432/plugin_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
RABBITMQ_CONSUMER_QUEUE=plugin-service-queue
RABBITMQ_BINDING_KEYS=task.*,project.*,board.*,document.*,user.deleted

# Webhook Delivery
DELIVERY_WORKER_COUNT=5
DELIVERY_HTTP_CONNECT_TIMEOUT=5s
DELIVERY_HTTP_TOTAL_TIMEOUT=30s
DELIVERY_RESPONSE_BODY_LIMIT_BYTES=1048576
MAX_DELIVERY_ATTEMPTS=8
WEBHOOK_USER_AGENT=TeamBoard-Webhook/1.0

# Security: SSRF Protection
ALLOW_INSECURE_HTTP=false                 # true nur in Dev/Test
ALLOW_PRIVATE_URLS=false                  # true nur in Dev (für lokale Tests)
WEBHOOK_ALLOWED_PORTS=80,443,8080,8443

# Circuit Breaker
BREAKER_CONSECUTIVE_FAILURES=5
BREAKER_TIMEOUT=30s

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
DELIVERY_RETENTION_DAYS=30                # Deliveries älter als 30 Tage löschen

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=plugin-service
```

### 14.2 Config-Struct

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"plugin-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8006"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
    }

    RabbitMQ struct {
        URL           string   `env:"RABBITMQ_URL,required"`
        Exchange      string   `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
        ConsumerQueue string   `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"plugin-service-queue"`
        BindingKeys   []string `env:"RABBITMQ_BINDING_KEYS" envSeparator:","`
    }

    Delivery struct {
        WorkerCount             int           `env:"DELIVERY_WORKER_COUNT" envDefault:"5"`
        HTTPConnectTimeout      time.Duration `env:"DELIVERY_HTTP_CONNECT_TIMEOUT" envDefault:"5s"`
        HTTPTotalTimeout        time.Duration `env:"DELIVERY_HTTP_TOTAL_TIMEOUT" envDefault:"30s"`
        ResponseBodyLimitBytes  int64         `env:"DELIVERY_RESPONSE_BODY_LIMIT_BYTES" envDefault:"1048576"`
        MaxAttempts             int           `env:"MAX_DELIVERY_ATTEMPTS" envDefault:"8"`
        UserAgent               string        `env:"WEBHOOK_USER_AGENT" envDefault:"TeamBoard-Webhook/1.0"`
    }

    Security struct {
        AllowInsecureHTTP   bool   `env:"ALLOW_INSECURE_HTTP" envDefault:"false"`
        AllowPrivateURLs    bool   `env:"ALLOW_PRIVATE_URLS" envDefault:"false"`
        AllowedPorts        []int  `env:"WEBHOOK_ALLOWED_PORTS" envSeparator:"," envDefault:"80,443,8080,8443"`
        ServiceTokenSecret  string `env:"SERVICE_TOKEN_SECRET,required"`
    }

    Breaker struct {
        ConsecutiveFailures uint32        `env:"BREAKER_CONSECUTIVE_FAILURES" envDefault:"5"`
        Timeout             time.Duration `env:"BREAKER_TIMEOUT" envDefault:"30s"`
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

    Cleanup struct {
        DeliveryRetentionDays int `env:"DELIVERY_RETENTION_DAYS" envDefault:"30"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }
}
```

---

## 15. Verzeichnisstruktur

```
services/domain/plugin/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── middleware.go
│   │   ├── handlers_webhooks.go
│   │   ├── handlers_deliveries.go
│   │   ├── handlers_health.go
│   │   ├── dto.go
│   │   └── generated.go
│   ├── domain/
│   │   ├── webhook.go
│   │   ├── delivery.go
│   │   ├── filter.go                    # event_filter Matching
│   │   ├── url_validation.go            # SSRF-Schutz
│   │   ├── service.go                   # WebhookService Interface
│   │   ├── service_impl.go
│   │   └── errors.go
│   ├── repository/
│   │   ├── db/                           # sqlc-generiert
│   │   ├── repository.go
│   │   └── postgres.go
│   ├── plugin/
│   │   ├── plugin.go                    # Interface (Erweiterungspunkt)
│   │   ├── registry.go
│   │   └── webhook_plugin.go            # MVP-Implementation
│   ├── delivery/
│   │   ├── worker.go
│   │   ├── http_client.go               # Custom http.Client mit SSRF-safe Dialer
│   │   ├── signing.go                   # HMAC-Signatur
│   │   ├── retry.go                     # Backoff-Berechnung
│   │   └── breaker_registry.go          # gobreaker pro URL
│   ├── projectclient/
│   │   ├── client.go
│   │   ├── http_client.go
│   │   └── permission_cache.go
│   ├── events/
│   │   ├── consumer.go                  # shared-Envelope-Handler → Domain-Envelope-Mapping
│   │   ├── handler_projects.go
│   │   ├── handler_users.go
│   │   └── envelope.go                  # Domain-Envelope-Alias
│   │                                    #   (Publishing: shared outbox.Worker, verdrahtet in main.go)
│   ├── cleanup/
│   │   └── worker.go                    # Delivery-Retention
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   └── 0001_init.down.sql
├── queries/
│   ├── webhooks.sql
│   ├── deliveries.sql
│   ├── known_projects.sql
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

## 16. sqlc-Queries

### 16.1 `queries/webhooks.sql`

```sql
-- name: CreateWebhook :one
INSERT INTO webhooks (id, project_id, target_url, description, secret, event_filter, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = $1;

-- name: ListWebhooksByProject :many
SELECT * FROM webhooks
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: ListActiveWebhooksMatchingEvent :many
SELECT * FROM webhooks
WHERE project_id = $1
  AND active = TRUE
  AND (
    event_filter @> ARRAY[$2::TEXT]
    OR EXISTS (
      SELECT 1 FROM unnest(event_filter) AS f
      WHERE f = '*'
         OR ($2 LIKE replace(f, '.*', '.%') AND f LIKE '%.*')
    )
  );

-- name: UpdateWebhook :one
UPDATE webhooks SET
    target_url = COALESCE(sqlc.narg('target_url'), target_url),
    description = COALESCE(sqlc.narg('description'), description),
    event_filter = COALESCE(sqlc.narg('event_filter'), event_filter),
    active = COALESCE(sqlc.narg('active'), active),
    updated_at = NOW()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: UpdateWebhookSecret :one
UPDATE webhooks SET secret = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = $1;

-- name: DeleteWebhooksByProject :exec
DELETE FROM webhooks WHERE project_id = $1;

-- name: SetWebhookActive :exec
UPDATE webhooks SET active = $2, updated_at = NOW() WHERE id = $1;
```

### 16.2 `queries/deliveries.sql`

```sql
-- name: CreateDelivery :one
INSERT INTO webhook_deliveries (id, webhook_id, event_id, event_type, payload, status, next_attempt_at)
VALUES ($1, $2, $3, $4, $5, 'pending', NOW())
RETURNING *;

-- name: PickNextPendingDelivery :one
SELECT * FROM webhook_deliveries
WHERE status = 'pending' AND next_attempt_at <= NOW()
ORDER BY next_attempt_at ASC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: GetDelivery :one
SELECT * FROM webhook_deliveries WHERE id = $1;

-- name: ListDeliveriesByWebhook :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1
  AND ($2::TEXT IS NULL OR status = $2)
  AND ($3::TEXT IS NULL OR event_type = $3)
  AND ($4::TIMESTAMPTZ IS NULL OR created_at < $4)
ORDER BY created_at DESC, id DESC
LIMIT $5;

-- name: MarkDeliveryDelivered :exec
UPDATE webhook_deliveries SET
    status = 'delivered',
    attempt_count = attempt_count + 1,
    last_response_status = $2,
    last_response_body = $3,
    last_attempted_at = NOW(),
    delivered_at = NOW(),
    duration_ms = $4
WHERE id = $1;

-- name: RescheduleDelivery :exec
UPDATE webhook_deliveries SET
    status = 'pending',
    attempt_count = attempt_count + 1,
    next_attempt_at = $2,
    last_response_status = $3,
    last_response_body = $4,
    last_error = $5,
    last_attempted_at = NOW(),
    duration_ms = $6
WHERE id = $1;

-- name: MarkDeliveryDead :exec
UPDATE webhook_deliveries SET
    status = 'dead',
    attempt_count = attempt_count + 1,
    last_error = $2,
    last_attempted_at = NOW(),
    failed_permanently_at = NOW()
WHERE id = $1;

-- name: MarkPendingDeliveriesDeadByWebhook :exec
UPDATE webhook_deliveries SET
    status = 'dead',
    last_error = 'webhook deleted',
    failed_permanently_at = NOW()
WHERE webhook_id = $1 AND status = 'pending';

-- name: DeleteOldDeliveries :execrows
DELETE FROM webhook_deliveries
WHERE created_at < $1
  AND status IN ('delivered', 'dead');
```

### 16.3 `queries/known_projects.sql`

```sql
-- name: UpsertKnownProject :exec
INSERT INTO known_projects (id) VALUES ($1)
ON CONFLICT (id) DO NOTHING;

-- name: KnownProjectExists :one
SELECT EXISTS(SELECT 1 FROM known_projects WHERE id = $1 AND deleted_at IS NULL);

-- name: MarkKnownProjectDeleted :exec
UPDATE known_projects SET deleted_at = NOW() WHERE id = $1;
```

### 16.4 `queries/outbox.sql` und `queries/processed_events.sql`

Identisch zu anderen Services — siehe Master-Pattern.

### 16.5 `sqlc.yaml`

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

## 17. Test-Strategie

### 17.1 Unit-Tests

```go
func TestEventFilterMatching(t *testing.T) {
    cases := []struct{
        name     string
        filter   []string
        eventType string
        match    bool
    }{
        {"exact match", []string{"task.created"}, "task.created", true},
        {"prefix wildcard", []string{"task.*"}, "task.updated", true},
        {"all wildcard", []string{"*"}, "anything", true},
        {"no match", []string{"task.created"}, "project.created", false},
        {"multiple, one matches", []string{"task.created", "task.updated"}, "task.updated", true},
    }
    // ...
}

func TestSSRFProtection(t *testing.T) {
    cases := []struct{
        url string
        wantErr error
    }{
        {"http://localhost/x", ErrPrivateURLForbidden},
        {"http://192.168.1.1/x", ErrPrivateURLForbidden},
        {"http://169.254.169.254/latest/meta-data", ErrPrivateURLForbidden},
        {"https://example.com/webhook", nil},
        {"javascript:alert(1)", ErrInvalidURL},
        {"https://example.com:22/x", ErrInvalidURL},  // Port nicht in Allowlist
    }
    // ...
}

func TestBackoffSchedule(t *testing.T) {
    // attemptCount=1 → ~30s
    // attemptCount=8 → ~24h
    // attemptCount=9 → not retried (dead)
}

func TestHMACSignature(t *testing.T) {
    body := []byte(`{"event":"x"}`)
    sig := signPayload("secret", body, 1715000000)
    require.Equal(t, "sha256=...expected hex...", sig)
}

func TestRetryAfterHeader(t *testing.T) {
    // Response 429 with "Retry-After: 60" → next_attempt_at ~now+60s
}
```

### 17.2 Integration-Tests

```go
func TestWebhookCRUD(t *testing.T) {
    pg := startPostgres(t)
    svc := buildService(pg)
    
    t.Run("create returns secret once", func(t *testing.T) { ... })
    t.Run("get does not return secret", func(t *testing.T) { ... })
    t.Run("rotate returns new secret", func(t *testing.T) { ... })
}

func TestDeliveryWorker(t *testing.T) {
    pg := startPostgres(t)
    httpServer := startMockServer(t)  // antwortet konfigurierbar mit 200 oder 500
    
    t.Run("happy path 200", func(t *testing.T) {
        // Insert delivery, run worker, assert status=delivered
    })
    
    t.Run("retry on 500", func(t *testing.T) {
        // Mock: ersten 2 Calls 500, dritter 200
        // Run worker mehrfach (mit reduziertem Backoff für Test)
        // Assert: attempt_count=3, status=delivered
    })
    
    t.Run("dead after max attempts", func(t *testing.T) {
        // Mock: immer 500
        // Run worker max+1 mal
        // Assert: status=dead
    })
    
    t.Run("410 Gone → immediate dead", func(t *testing.T) { ... })
    
    t.Run("circuit breaker opens after 5 failures", func(t *testing.T) { ... })
    
    t.Run("Retry-After header respected", func(t *testing.T) { ... })
}

func TestEventDispatch(t *testing.T) {
    pg := startPostgres(t)
    svc := buildService(pg)
    
    // Setup: 2 webhooks for project X, one matches "task.*", one matches "task.deleted"
    // Trigger task.created event
    // Assert: 1 pending delivery (for task.* webhook)
    
    // Trigger task.deleted event
    // Assert: 2 pending deliveries
}

func TestProjectDeletedConsumer(t *testing.T) {
    // Setup: Project mit 3 webhooks und 5 pending deliveries
    // Trigger project.deleted
    // Assert: 0 webhooks, 5 deliveries als dead markiert
}
```

### 17.3 End-to-End-Tests

Postman-Collection mit Mock-Webhook-Receiver (z. B. https://webhook.site oder lokaler Test-Server):

1. **Lifecycle:** Webhook erstellen → Test-Delivery → Audit-Log prüfen
2. **Event-Trigger:** Task in zugehörigem Projekt erstellen → Webhook-Receiver bekommt Call mit korrekter Signatur
3. **Retry-Verhalten:** Receiver antwortet mit 500 → erneuter Call nach Backoff
4. **DLQ:** Receiver immer 500 → nach 8 Versuchen Status = dead
5. **SSRF:** Versuch `http://localhost:8002/...` → 400 mit `private_url_forbidden`

### 17.4 Load-Test

```javascript
// tests/k6/event_dispatch_throughput.js
// Simuliert hohe Event-Rate, prüft dass Worker mitkommen
```

Ziel: 1000 Events/sec verarbeiten ohne Pending-Queue-Aufbau.

---

## 18. Implementierungs-Hinweise für Coding-Agents

### 18.1 Implementierungsreihenfolge

1. **Migrations + sqlc-Setup**
2. **Domain-Modell** mit Filter-Matching, URL-Validation, Backoff-Berechnung — Unit-Tests
3. **Repository-Layer** mit Testcontainers
4. **HTTP-Client mit SSRF-safe Dialer** (`internal/delivery/http_client.go`) — Tests gegen lokale Mocks
5. **HMAC-Signatur** (`internal/delivery/signing.go`)
6. **Plugin-Interface + WebhookPlugin** (`internal/plugin/`)
7. **Circuit Breaker Registry**
8. **Delivery-Worker** mit Retry-Logik
9. **Project-Client**
10. **Event-Consumer + Dispatcher**
11. **HTTP-Handler** für REST-API
12. **Cleanup-Worker** für Delivery-Retention
13. **Outbox-Publisher**
14. **Wiring in `main.go`**
15. **End-to-End-Tests**

### 18.2 Verbindliche Konventionen

- **Permission-Check vor jeder Webhook-CRUD-Operation.** Auf Webhook-Operationen: über `webhook.project_id` → Project-Service-Permissions.
- **URL-Validation beim Create/Update + erneut beim Delivery** (DNS-Rebinding).
- **Kein Logging vollständiger Secrets oder Signaturen** — höchstens erste 4 Zeichen + Hash-Präfix.
- **Response-Body truncate auf 4KB** vor DB-Insert.
- **Outbox + Delivery-Insert in einer Transaktion** (Outbox für interne Events, nicht für Delivery selbst — Delivery ist primärer Persistenz-Layer).
- **Worker-Polling alle 1s + Notify via NOTIFY/LISTEN** (optional, für Latenzminimierung) — im MVP reicht reines Polling.
- **`FOR UPDATE SKIP LOCKED`** beim Worker-Pickup, damit mehrere Worker konkurrenzfrei arbeiten.

### 18.3 Typische Stolperfallen

- **PostgreSQL Array-Filter:** `event_filter @> ARRAY['task.created']::TEXT[]` — Cast notwendig. sqlc generiert nicht immer perfekt, ggf. raw SQL.
- **Wildcard-Filter im SQL-Query:** Komplexe Pattern-Matching im SQL ist umständlich. Pragmatisch: Webhooks aktiv aus DB ziehen, Filter-Matching im Code (Cache der aktiven Webhooks pro Projekt).
- **HTTP-Client mit Custom-Dialer:** Connection-Pooling kann SSRF-Schutz unterlaufen, wenn alte Connection wiederverwendet wird. Daher `MaxIdleConnsPerHost: 0` setzen, oder Dialer pro Request.
- **Redirect-Loop:** ohne `MaxRedirects` Schutz kann Empfänger einen Redirect-Loop bauen. Standard `http.Client` folgt 10 Redirects, das ist OK. Bei jedem Redirect SSRF-Check erneut.
- **Body schließen:** `resp.Body.Close()` immer in defer, sonst Connection-Leaks. Lesen mit `io.LimitReader` für Body-Limit.
- **Retry bei Timeouts:** Wichtig zu wissen, ob der Empfänger den Request schon empfangen hat oder nicht. Bei Connection-Timeout sicher nicht; bei Read-Timeout *möglicherweise schon*. Empfänger müssen idempotent sein — wir dokumentieren das.
- **Multiple Workers Race:** Wenn Worker A ein Pending zieht und vor Update crasht, bleibt der Eintrag mit altem `next_attempt_at`. `FOR UPDATE` hält Lock nur in Transaktion — bei Crash kommt nächster Worker dran. OK.
- **Retry-After-Parsing:** Header kann sowohl Sekunden (`60`) als auch HTTP-Date sein (`Wed, 21 Oct 2026 07:28:00 GMT`). Beide unterstützen.
- **Time-Drift bei HMAC-Timestamp:** Server- und Empfängeruhren können auseinanderlaufen. 5-Minuten-Toleranz im Empfänger-Code dokumentieren.

### 18.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration test-load migrate generate run docker-build

build:
	go build -o bin/plugin ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/... ./internal/delivery/... ./internal/plugin/...

test-integration:
	go test -race ./internal/repository/... ./internal/events/...

test-load:
	k6 run tests/k6/event_dispatch_throughput.js

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/plugin.openapi.yaml > internal/api/generated.go

run:
	air

docker-build:
	docker build -t teamboard/plugin:latest .
```

### 18.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|-------------------|
| UC-1 Create Webhook | URL gegen SSRF validiert, Secret generiert, exakt einmal in Response |
| UC-2 Update Webhook | URL re-validiert wenn geändert, Outbox-Event mit Diff |
| UC-3 Rotate Secret | Neues Secret persistiert, einmalig zurückgegeben |
| UC-4 Enable/Disable | `active`-Flag updated, Pending-Deliveries laufen nach Re-Enable wieder |
| UC-5 Delete | Hard-Delete, Pending-Deliveries als dead markiert, Audit bleibt |
| UC-6 List | Ohne Secrets |
| UC-7 Delivery-Log | Cursor-Pagination, Filter funktionieren |
| UC-8 Test-Delivery | Synthetic-Event durch normale Pipeline, Delivery-ID zurück |
| UC-9 Event-Dispatch | Pending-Delivery pro matching webhook, idempotent gegen duplizierte Events |
| Worker | 200→delivered, 500→retry mit Backoff+Jitter, 8 Fehlversuche→dead, 410→sofort dead |
| SSRF | Private IPs blocked auch nach DNS-Rebinding |
| Circuit Breaker | Nach 5 consecutive failures → open, alle Deliveries dieser URL um 30s verschoben |
| HMAC | Signatur reproduzierbar, Empfänger-Verify-Beispiel funktioniert |

---

## Anhang A: Sequenzdiagramm — Event → Delivery

```
RabbitMQ          Plugin Svc         Postgres        Webhook Worker      External Receiver
   │                  │                  │                  │                      │
   │ task.created     │                  │                  │                      │
   ├─────────────────>│                  │                  │                      │
   │                  │ idempotency check│                  │                      │
   │                  ├─────────────────>│                  │                      │
   │                  │ ListActive       │                  │                      │
   │                  │  Webhooks(p, ev) │                  │                      │
   │                  ├─────────────────>│                  │                      │
   │                  │ INSERT delivery  │                  │                      │
   │                  │  (status=pending)│                  │                      │
   │                  ├─────────────────>│                  │                      │
   │                  │ INSERT processed_│                  │                      │
   │                  │  events          │                  │                      │
   │                  ├─────────────────>│                  │                      │
   │ ACK              │                  │                  │                      │
   │<─────────────────┤                  │                  │                      │
   │                  │                  │                  │                      │
   │                  │     (1s tick)                       │                      │
   │                  │                  │ PickNext         │                      │
   │                  │                  │  PendingDelivery │                      │
   │                  │                  │<─────────────────┤                      │
   │                  │                  │                  │ Check breaker        │
   │                  │                  │                  │ Sign + POST          │
   │                  │                  │                  ├─────────────────────>│
   │                  │                  │                  │                      │
   │                  │                  │                  │ 200 OK               │
   │                  │                  │                  │<─────────────────────┤
   │                  │                  │ MarkDelivered    │                      │
   │                  │                  │<─────────────────┤                      │
```

## Anhang B: Sequenzdiagramm — Retry mit Backoff

```
Worker          Postgres        Receiver
   │                  │              │
   │ Pick pending     │              │
   │<─────────────────┤              │
   │ Sign + POST      │              │
   ├─────────────────────────────────>│
   │ 500 Error        │              │
   │<─────────────────────────────────┤
   │ Compute next_at  │              │
   │  (attempt=1, +30s)              │
   │ RescheduleDelivery (status=pending, next=now+30s)
   ├─────────────────>│              │
   │                  │              │
   │ ... 30s warten ... │              │
   │                  │              │
   │ Pick pending     │              │
   │<─────────────────┤              │
   │ Sign + POST      │              │
   ├─────────────────────────────────>│
   │ 200 OK           │              │
   │<─────────────────────────────────┤
   │ MarkDelivered    │              │
   ├─────────────────>│              │
```

---

**Ende des Detail-Designs Plugin / Webhook Service.**
