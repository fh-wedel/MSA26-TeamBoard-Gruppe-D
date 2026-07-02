# TeamBoard — Architektur und System-Design

> **Zweck dieses Dokuments:** Master-Referenz für die Implementierung des TeamBoard-Systems. Dieses Dokument ist die zentrale Grundlage für KI-gestützte Entwicklung (Agentic Coding) und für menschliche Entwickler. Jeder Coding-Task referenziert dieses Dokument als Source of Truth.
>
> **Stand:** 2026-05  
> **Tech-Stack:** Go 1.22+, Chi-Router, sqlc, PostgreSQL 16, Redis 7, RabbitMQ 3.12, Docker, AWS ECS Fargate

---

## Inhaltsverzeichnis

1. [Systemüberblick](#1-systemüberblick)
2. [Architekturprinzipien](#2-architekturprinzipien)
3. [Service-Katalog](#3-service-katalog)
4. [Repository-Struktur](#4-repository-struktur)
5. [Service-übergreifende Konventionen](#5-service-übergreifende-konventionen)
6. [API-Konventionen (REST)](#6-api-konventionen-rest)
7. [Event-Schema und Messaging](#7-event-schema-und-messaging)
8. [Datenmodell-Übersicht](#8-datenmodell-übersicht)
9. [Authentifizierung und Autorisierung](#9-authentifizierung-und-autorisierung)
10. [Observability](#10-observability)
11. [Lokale Entwicklung](#11-lokale-entwicklung)
12. [Deployment](#12-deployment)
13. [Glossar](#13-glossar)

---

## 1. Systemüberblick

### 1.1 Was ist TeamBoard?

TeamBoard ist eine Web-basierte Kollaborationsplattform für Teams mit folgenden Kernfunktionen:

- **Projekt- und Board-Verwaltung** mit rollenbasierten Berechtigungen
- **Kanban-Boards** mit Aufgaben (Status, Zuweisung, Fälligkeit, Kommentare, Anhänge)
- **Dokumentenablage** mit Versionierung
- **Echtzeit-Benachrichtigungen** via WebSockets
- **Webhook-Integrationen** zu externen Systemen
- **Plugin-Architektur** für neue Board-Typen (Kanban, Scrum, Kalender)

### 1.2 Architektur-Stil

Komponentenbasiertes Microservice-System mit folgenden Eigenschaften:

- **Domain-Driven Design** — Services geschnitten entlang fachlicher Bounded Contexts
- **Event-Driven Backbone** — RabbitMQ als zentraler Event Bus, Services kommunizieren bevorzugt asynchron
- **Database-per-Service** — jede Komponente hat eigene Datenhoheit
- **API Gateway als Single Entry Point** — Authentifizierung, Routing, Rate Limiting zentral
- **Statelessness als Default** — nur Notification Service hält bewusst Zustand (WebSocket-Connections)

### 1.3 High-Level-Architektur

```
┌─────────────────────────────────────────────────────────────────┐
│                          CLIENTS                                 │
│   Web-SPA (React/TS)    │   Externe Systeme (CI/CD, Webhooks)   │
└──────────┬──────────────────────────────────────┬───────────────┘
           │ HTTPS/WSS                            │ HTTPS
           ▼                                      ▼
┌─────────────────────────────────────────────────────────────────┐
│              API GATEWAY (Traefik / AWS API Gateway)             │
│         TLS-Termination, JWT-Validation, Routing, Rate Limit     │
└──┬───────┬───────┬───────┬───────┬───────┬───────┬──────────────┘
   │       │       │       │       │       │       │
   ▼       ▼       ▼       ▼       ▼       ▼       ▼
┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐
│Auth│ │Proj│ │Task│ │Doc │ │Notf│ │Plug│ │... │
└─┬──┘ └─┬──┘ └─┬──┘ └─┬──┘ └─┬──┘ └─┬──┘
  │      │      │      │      │      │
  │ DB   │ DB   │ DB   │ DB   │Redis │ DB
  ▼      ▼      ▼      ▼      ▼      ▼
                                 │
        ┌─────────────────────────────────────┐
        │   RabbitMQ (Event Bus)              │
        │   Topics: task.*, project.*, ...    │
        └─────────────────────────────────────┘
                          │
                          ▼
                ┌─────────────────┐
                │  S3 / MinIO     │  Dokumente
                └─────────────────┘
```

---

## 2. Architekturprinzipien

Bindende Designentscheidungen für alle Services. Abweichungen erfordern ein dokumentiertes ADR (Architecture Decision Record) unter `docs/adr/`.

| ID | Prinzip | Konsequenz |
|----|---------|------------|
| P1 | Domain-Driven Design | Services entlang fachlicher Grenzen, eigene Sprache (Ubiquitous Language) pro Bounded Context |
| P2 | Database-per-Service | Kein direkter DB-Zugriff zwischen Services, nur über APIs/Events |
| P3 | Loose Coupling | Services kennen voneinander nur stabile, versionierte Schnittstellen |
| P4 | Async-First | Inter-Service-Kommunikation bevorzugt über Events, synchron nur wenn der Benutzer die Antwort sofort braucht |
| P5 | Stateless wo möglich | Service-Instanzen austauschbar, Zustand in DB/Cache/Broker |
| P6 | Smart Endpoints, Dumb Pipes | Geschäftslogik in Services, Infrastruktur dazwischen bleibt dumm |
| P7 | API-First | OpenAPI/AsyncAPI-Spezifikation vor Implementierung |
| P8 | Eventual Consistency | Strikte ACID nur innerhalb eines Service. Outbox-Pattern für DB-Event-Konsistenz |
| P9 | Idempotente Event-Handler | Jedes Event darf mehrfach ankommen, ohne Schaden anzurichten |
| P10 | Resilience by Design | Timeouts, Retries, Circuit Breaker auf jedem ausgehenden Call |
| P11 | Security in Depth | TLS, JWT, IAM, Secrets Manager, Audit-Logs |
| P12 | Observability First-Class | Strukturierte Logs, Metrics, Distributed Tracing in jedem Service |

---

## 3. Service-Katalog

Das System besteht aus sieben Domain-Services (Auth, Project, Task, Document, Notification, Plugin/Webhook, Board Registry) plus dem API-Gateway. Jeder Service hat einen klar abgegrenzten Verantwortungsbereich; die Board Registry (§3.8) ist nach den ursprünglichen sechs Services hinzugekommen.

### 3.1 Auth Service

**Zweck:** Identitätsverwaltung — Registrierung, Login, JWT-Ausstellung.

**Verantwortet:**
- Benutzerregistrierung mit E-Mail/Passwort
- Login → Issue JWT (Access + Refresh Token)
- Token-Refresh
- Passwort-Reset-Flow
- Public-Key-Endpoint (JWKS) für andere Services zur Token-Validierung

**Verantwortet NICHT:**
- Projekt-spezifische Rollen (gehören zum Project Service)
- Profile-Daten über Identität hinaus

**Datenmodell:** `users`, `refresh_tokens`, `password_reset_tokens`

**Skalierung:** Stateless, horizontal beliebig

**Externe Abhängigkeiten:** Nur seine eigene PostgreSQL-DB, optional E-Mail-Service (SES/SMTP) für Reset-Mails

**Events publiziert:** `user.registered`, `user.deleted`

**Events konsumiert:** keine

**Port (lokal):** 8001

---

### 3.2 Project Service

**Zweck:** Projekte, Boards, Mitgliedschaften, Berechtigungen.

**Verantwortet:**
- Projekt-CRUD
- Board-CRUD innerhalb eines Projekts
- Mitgliederverwaltung pro Projekt mit Rollen (Owner, Editor, Viewer)
- **Authoritative Source für Permissions** — andere Services fragen hier an, ob ein User eine Operation darf

**Verantwortet NICHT:**
- Tasks (gehören zum Task Service)
- Globale Benutzerverwaltung (Auth Service)

**Datenmodell:** `projects`, `boards`, `project_members`, `roles`

**Skalierung:** Stateless, horizontal beliebig. Bei hoher Authorization-Last → Permission-Cache via Redis.

**Externe Abhängigkeiten:** PostgreSQL, RabbitMQ

**Events publiziert:** `project.created`, `project.deleted`, `project.member.added`, `project.member.removed`, `board.created`, `board.deleted`

**Events konsumiert:** `user.deleted` (Cleanup von Mitgliedschaften)

**Port (lokal):** 8002

---

### 3.3 Task Service

**Zweck:** Aufgabenverwaltung — Tasks, Kommentare, Status, Zuweisungen.

**Verantwortet:**
- Task-CRUD innerhalb eines Boards
- Statusübergänge (z. B. `todo` → `in_progress` → `done`)
- Zuweisung an Benutzer
- Fälligkeitsdaten
- Kommentar-CRUD an Tasks
- Anhang-Referenzen (verlinkt nur auf Document Service)

**Datenmodell:** `tasks`, `task_comments`, `task_attachments`, `task_history` (Audit-Trail)

**Skalierung:** Stateless, horizontal beliebig. Bei hoher Last → DB-Sharding nach `project_id`.

**Authorization-Strategie:** Vor jeder schreibenden Operation Permission-Check beim Project Service. Lesen darf, wer Mitglied im Projekt ist.

**Externe Abhängigkeiten:** PostgreSQL, RabbitMQ, Project Service (Authorization)

**Events publiziert:** `task.created`, `task.updated`, `task.deleted`, `task.assigned`, `task.status.changed`, `task.commented`

**Events konsumiert:** `project.deleted` (Cleanup zugehöriger Tasks)

**Port (lokal):** 8003

---

### 3.4 Document Service

**Zweck:** Dokumentenablage mit Versionierung.

**Verantwortet:**
- Dokumente als Metadaten + Verweis auf Object Storage
- Versionierung (jede neue Version = neuer S3-Object-Key)
- Pre-Signed URLs für Up- und Download
- Lifecycle (Soft-Delete, später Hard-Delete via Cleanup-Job)

**Verantwortet NICHT:**
- Inhaltliche Auswertung der Dokumente
- Echtzeit-Kollaboration auf Dokumenten (späterer Service)

**Datenmodell:** `documents`, `document_versions`

**Skalierung:** Stateless, horizontal beliebig. Bytes fließen direkt zwischen Client und S3.

**Externe Abhängigkeiten:** PostgreSQL, RabbitMQ, S3 / MinIO, Project Service (Authorization)

**Events publiziert:** `document.uploaded`, `document.version.created`, `document.deleted`

**Events konsumiert:** `project.deleted` (Cleanup)

**Port (lokal):** 8004

---

### 3.5 Notification Service

**Zweck:** Echtzeit-Push und persistente Benachrichtigungen.

**Verantwortet:**
- WebSocket-Endpoint für Clients
- Verbindungs-Tracking in Redis
- Konsumiert Domain-Events, filtert nach Berechtigungen, pusht an verbundene User
- Persistente Notifications (DB) — z. B. `@mentions` in Kommentaren
- API zum Lesen/Markieren von Notifications

**Bewusste Stateful-Komponente:** Hält offene WebSocket-Verbindungen.

**Skalierungsstrategie:**
- Mehrere Instanzen, Redis Pub/Sub als Backplane
- Event kommt auf Instanz A an, wird via Redis allen Instanzen zugestellt, jede prüft ihre lokalen Connections
- Sticky Sessions am Gateway oder Reconnect-tolerantes Frontend

**Datenmodell:** `notifications` (persistent in PostgreSQL), Connection-Tracking in Redis

**Externe Abhängigkeiten:** PostgreSQL, Redis, RabbitMQ, Project Service (Authorization)

**Events publiziert:** `notification.delivered`, `notification.read`

**Events konsumiert:** Praktisch alle `task.*`, `project.*`, `document.*` Events

**Port (lokal):** 8005

---

### 3.6 Plugin / Webhook Service

**Zweck:** Externe Webhook-Integrationen + Plugin-Backbone.

**Verantwortet:**
- Webhook-Registrierung pro Projekt mit Event-Filter
- Webhook-Auslieferung mit Retry, Exponential Backoff, Dead-Letter-Handling
- HMAC-Signatur ausgehender Requests für Empfänger-Verifikation
- Audit-Log aller Zustellversuche

**Datenmodell:** `webhooks`, `webhook_deliveries`

**Skalierung:** Stateless Worker, horizontal beliebig. Job-Queue in RabbitMQ.

**Externe Abhängigkeiten:** PostgreSQL, RabbitMQ, Internet (ausgehende HTTP-Calls)

**Events publiziert:** keine domain events
**Events konsumiert:** alle abonnierten Domain-Events

**Port (lokal):** 8006

---

### 3.7 API Gateway

**Zweck:** Single Entry Point für alle Clients.

**Verantwortet:**
- TLS-Termination (in AWS; lokal HTTP)
- Routing zu den Domain-Services
- Rate Limiting pro IP (Traefik `ratelimit`-Middleware)
- CORS (Traefik `headers`-Middleware)
- Security-Header (X-Frame-Options, X-Content-Type-Options, Referrer-/Permissions-Policy)
- Request-Logging

**Verantwortet NICHT:**
- **JWT-Validierung** — bewusst **nicht** am Gateway, sondern in **jedem** Service
  unabhängig (Zero-Trust: keine implizite Vertrauensstellung durch Netzwerklage).
  Jeder Service prüft die RS256-Signatur selbst gegen die JWKS des Auth Service
  via `shared/go/authmiddleware`. Interne Service-zu-Service-Calls tragen ein
  separates, kurzlebiges `servicetoken`-JWT.
- Geschäftslogik (Smart Endpoints, Dumb Pipes!)
- Response-Aggregation

**Technologie:** Lokal Traefik, in AWS Amazon API Gateway

**Skalierung:** Stateless, horizontal beliebig

**Port (lokal):** 80 (HTTP), 443 (HTTPS in Produktion)

---

### 3.8 Service-Übersicht (kompakt)

| Service | Port | DB | Stateful? | Skalierung |
|---------|------|-----|-----------|------------|
| API Gateway | 80 | — | nein | horizontal |
| Auth | 8001 | Postgres `auth_db` | nein | horizontal |
| Project | 8002 | Postgres `project_db` | nein | horizontal |
| Task | 8003 | Postgres `task_db` | nein | horizontal |
| Document | 8004 | Postgres `document_db` + S3 | nein | horizontal |
| Notification | 8005 | Postgres `notification_db` + Redis | **ja (WS)** | horizontal mit Backplane |
| Plugin/Webhook | 8006 | Postgres `plugin_db` | nein | horizontal |
| Board Registry | 8007 | Postgres `boardregistry_db` | nein | horizontal |

**Board Registry (8007):** Autoritative Quelle für Board-Typ-Definitionen (Default-Columns inkl.
Status, Default-Config, JSON-Schema). Der Project-Service löst Typen zur Laufzeit über die
internal-API auf (Service-Token, Cache) und invalidiert den Cache auf `boardtype.*`-Events. Details:
`docs/services/boardregistry.md`.

---

## 4. Repository-Struktur

**Empfehlung: Monorepo** mit klarer Modulgrenze pro Service. Vorteile: gemeinsames Tooling, atomare Cross-Service-Änderungen, einfacheres Onboarding.

```
teamboard/
├── README.md
├── Makefile                          # make up, make test, make build, ...
├── docker-compose.yml                # lokale Entwicklung (alle Services + Infra)
├── docker-compose.override.yml       # lokale Overrides (Hot-Reload, Debug)
├── .github/
│   └── workflows/
│       ├── ci.yml                    # Lint, Test, Build pro Service
│       ├── deploy-staging.yml
│       └── deploy-prod.yml
├── docs/
│   ├── ARCHITECTURE.md               # ← dieses Dokument
│   ├── decisions/                    # Architecture Decision Records (ADRs)
│   │   ├── 0001-board-type-extensibility.md
│   │   └── 0002-board-view-extensibility.md
│   ├── services/                     # Detail pro Service: DB-Schema, OpenAPI, Use-Cases
│   │   ├── auth.md
│   │   ├── project.md
│   │   └── ...                       # OpenAPI-Specs sind hier eingebettet (kein eigenes docs/api/)
│   ├── specifications/               # coding-guidelines, shared, orchestration, gateway
│   └── demo/                         # Notebooks (Board-Typen, Webhooks)
├── services/
│   ├── auth/
│   │   ├── cmd/server/main.go
│   │   ├── internal/
│   │   │   ├── api/                  # HTTP-Handler (Chi)
│   │   │   ├── domain/               # Geschäftslogik (sprachlich rein)
│   │   │   ├── repository/           # DB-Zugriff (sqlc-generiert + Wrapper)
│   │   │   ├── events/               # Event-Publisher
│   │   │   └── config/
│   │   ├── migrations/               # golang-migrate
│   │   ├── queries/                  # sqlc-Input (.sql-Dateien)
│   │   ├── sqlc.yaml
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── README.md
│   ├── project/      (gleiche Struktur)
│   ├── task/         (gleiche Struktur)
│   ├── document/     (gleiche Struktur)
│   ├── notification/ (gleiche Struktur)
│   ├── plugin/       (gleiche Struktur)
│   └── boardregistry/ (gleiche Struktur)
├── shared/
│   └── go/
│       ├── authmiddleware/           # JWT-Validierung als Library
│       ├── eventbus/                 # RabbitMQ-Wrapper (Envelope, Publisher, Consumer)
│       ├── outbox/                   # Outbox-Worker (poll → publish)
│       ├── servicetoken/             # Service-to-Service-JWT (aud: "internal")
│       ├── observability/            # OTel-Setup, Logger
│       └── httputil/                 # Common HTTP-Helpers, Error-Mapping
├── frontend/
│   ├── src/
│   ├── package.json
│   └── Dockerfile
├── infra/
│   ├── cdk/                          # AWS CDK (TypeScript)
│   │   ├── bin/
│   │   ├── lib/
│   │   └── package.json
│   └── traefik/                      # lokale Gateway-Konfiguration
└── scripts/
    ├── postgres-init.sh          # DB-Initialisierung (6 Datenbanken anlegen)
    ├── run-migrations.sh         # golang-migrate auf alle Service-DBs
    ├── seed.py                   # Demo-Daten einspielen (plattformübergreifend)
    ├── seed.sh                   # Bash-Variante des Seed-Skripts
    └── wait-healthy.py           # Wartet bis alle Docker-Services healthy sind
```

### Begründung Monorepo

- **Atomare Änderungen** über Service-Grenzen hinweg (z. B. Event-Schema-Änderung)
- **Geteilter Code** in `shared/go/` 
- **CI/CD** kann Service-spezifische Builds über Path-Filter triggern
- **Onboarding:** ein `git clone`, ein `make up`, läuft

---

## 5. Service-übergreifende Konventionen

### 5.1 Sprache und Frameworks

| Layer | Technologie |
|-------|-------------|
| Sprache | Go 1.22+ |
| HTTP-Router | `github.com/go-chi/chi/v5` |
| DB-Zugriff | sqlc (generierter Code) + `pgx/v5` |
| Migrations | `golang-migrate/migrate/v4` |
| Validation | `github.com/go-playground/validator/v10` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| Messaging | `github.com/rabbitmq/amqp091-go` |
| Logging | `log/slog` (Standard Library) |
| Tracing | `go.opentelemetry.io/otel` |
| Testing | Standard `testing` + `github.com/stretchr/testify` |
| Test-DB | `github.com/testcontainers/testcontainers-go` |

### 5.2 Service-Layout (innerhalb eines Service)

```
services/<name>/
├── cmd/server/main.go        # Entry Point: Wiring, HTTP-Server starten
├── internal/
│   ├── api/                  # HTTP-Handler, kein Domain-Wissen
│   │   ├── router.go
│   │   ├── handlers_<resource>.go
│   │   ├── middleware.go
│   │   └── dto.go            # Request/Response-Strukturen
│   ├── domain/               # Geschäftslogik, sprachlich rein
│   │   ├── <entity>.go       # Domain-Modell
│   │   ├── service.go        # Use-Cases
│   │   └── errors.go         # Domain-Fehler
│   ├── repository/
│   │   ├── db/               # sqlc-generierter Code
│   │   └── repository.go     # Interface + Implementierung
│   ├── events/
│   │   └── consumer.go       # Incoming Events (falls relevant); Publishing via shared outbox.Worker in main.go
│   └── config/
│       └── config.go         # Env-basierte Config
```

**Architektur-Stil:** Hexagonal / Clean Architecture leicht. `domain/` darf keine Imports aus `api/` oder `repository/` haben — nur Interfaces. Dependency-Injection im `main.go`.

### 5.3 Konfiguration

Ausschließlich über Environment-Variablen, geladen über `github.com/caarlos0/env/v10`.

Konvention: `<SERVICE>_<KEY>` — Beispiel: `TASK_DB_URL`, `TASK_RABBITMQ_URL`.

Pflicht-Variablen pro Service (Auszug):

```
SERVICE_NAME=task-service
SERVICE_PORT=8003
LOG_LEVEL=info
DB_URL=postgres://user:pass@localhost:5432/task_db?sslmode=disable
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
JWT_JWKS_URL=http://auth:8001/.well-known/jwks.json
PROJECT_SERVICE_URL=http://project:8002
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
```

### 5.4 Fehlerbehandlung

**Domain-Fehler** als typisierte Werte, nicht als Strings:

```go
package domain

type ErrorCode string

const (
    ErrNotFound          ErrorCode = "not_found"
    ErrPermissionDenied  ErrorCode = "permission_denied"
    ErrValidation        ErrorCode = "validation_failed"
    ErrConflict          ErrorCode = "conflict"
)

type Error struct {
    Code    ErrorCode
    Message string
    Cause   error
}

func (e *Error) Error() string { ... }
```

**HTTP-Mapping** in `shared/go/httputil/`:
- `ErrNotFound` → 404
- `ErrPermissionDenied` → 403
- `ErrValidation` → 400
- `ErrConflict` → 409
- alles andere → 500 (mit Trace-ID im Response, ohne interne Details)

### 5.5 Logging

Strukturiertes JSON via `log/slog`. Pflichtfelder pro Log-Eintrag:

```json
{
  "time": "2026-05-06T12:34:56Z",
  "level": "info",
  "msg": "task created",
  "service": "task-service",
  "trace_id": "abc123",
  "span_id": "def456",
  "user_id": "uuid-...",
  "task_id": "uuid-...",
  "duration_ms": 12
}
```

Sensible Felder (Passwörter, Tokens, E-Mails) niemals loggen.

### 5.6 Testing-Pyramide

| Ebene | Anteil | Werkzeug |
|-------|--------|----------|
| Unit-Tests (Domain-Logik) | 70% | `testing` + `testify` |
| Integration-Tests (DB, Events) | 25% | Testcontainers |
| End-to-End | 5% | Postman-Collection / Playwright |

**Pflicht:** Jeder Service hat einen `make test`-Target, der alle Tests ausführt. CI bricht bei <70% Coverage in Domain-Layer.

---

## 6. API-Konventionen (REST)

### 6.1 Versionierung

URL-basiert: `/api/v1/...`. Breaking Changes erhöhen die Version, alte Version bleibt mindestens ein Release parallel verfügbar.

### 6.2 URL-Struktur

- Ressourcen plural: `/projects`, `/tasks`
- Hierarchien: `/projects/{projectId}/boards`
- Sub-Aktionen sparsam: `/tasks/{taskId}:assign` (mit Doppelpunkt) oder als PATCH

### 6.3 HTTP-Methoden

| Methode | Verwendung |
|---------|------------|
| GET | Lesen, niemals Seiteneffekte |
| POST | Erzeugen oder Aktion ohne natürliche PUT-Semantik |
| PUT | Vollständiges Ersetzen |
| PATCH | Teil-Update (`application/merge-patch+json`) |
| DELETE | Löschen (Soft- oder Hard-Delete domain-abhängig) |

### 6.4 Standard-Response-Format

**Erfolg:**
```json
{
  "data": { ... } 
}
```

oder bei Listen:
```json
{
  "data": [ ... ],
  "pagination": {
    "next_cursor": "eyJpZCI6...",
    "limit": 50
  }
}
```

**Fehler** (RFC 7807 Problem Details):
```json
{
  "type": "https://teamboard.example/errors/validation_failed",
  "title": "Validation failed",
  "status": 400,
  "detail": "field 'title' is required",
  "trace_id": "abc123",
  "errors": [
    { "field": "title", "message": "required" }
  ]
}
```

### 6.5 Pagination

Cursor-basiert (nicht offset-basiert):

```
GET /tasks?board_id=...&limit=50&cursor=eyJpZCI6IjEyMyJ9
```

Cursor ist Base64-kodiertes JSON `{"id": "...", "created_at": "..."}`. Verhindert Skip-Probleme bei häufig wachsenden Listen.

### 6.6 Idempotency

Schreibende Operationen unterstützen optional `Idempotency-Key`-Header. Server speichert Response für 24h und gibt bei Wiederholung dieselbe Antwort.

### 6.7 OpenAPI-Workflow

**Spec-First:** Schemas leben in `docs/api/<service>.openapi.yaml`. Aus der Spec wird mit `oapi-codegen` der Server-Stub generiert:

```bash
oapi-codegen -package=api -generate=types,chi-server \
  docs/api/task.openapi.yaml > services/task/internal/api/generated.go
```

Die Spec ist Source of Truth — handgeschriebener Code implementiert die generierten Interfaces.

---

## 7. Event-Schema und Messaging

### 7.1 Broker

**Lokal:** RabbitMQ 3.12 mit Management-Plugin.  
**AWS:** Amazon MQ (managed RabbitMQ) oder Migration zu EventBridge + SQS.

### 7.2 Topology

- **Exchange:** `teamboard.events`, Typ `topic`, durable
- **Routing-Keys:** `<aggregate>.<event>` — z. B. `task.created`, `project.member.added`
- **Queues pro Subscriber:** durable, mit Dead-Letter-Exchange `teamboard.events.dlx`
- **Beispiel-Bindings:**
  - `notification-service-queue` ← `task.*`, `project.*`, `document.*`
  - `plugin-service-queue` ← `*.*` (oder konfigurierbar pro Webhook)

### 7.3 Event-Envelope

**Verbindlich für alle Events:**

```json
{
  "event_id": "01H...",
  "event_type": "task.created",
  "event_version": 1,
  "occurred_at": "2026-05-06T12:34:56.789Z",
  "trace_id": "abc123",
  "producer": "task-service",
  "aggregate_type": "task",
  "aggregate_id": "uuid-...",
  "actor": {
    "user_id": "uuid-...",
    "type": "user"
  },
  "payload": { ... }
}
```

- `event_id`: UUID (Outbox-Zeilen-ID), idempotenz-tauglich
- `event_version`: erlaubt Schema-Evolution
- `payload`: event-spezifisch (siehe unten)

**Transport (verbindlich):** Dieser Envelope ist der **vollständige AMQP-Message-Body**
(JSON). Implementiert über die Shared-Libs `shared/go/eventbus` (Publisher/Consumer)
und `shared/go/outbox` (Worker) — jeder Service nutzt sie, kein service-eigener
Publisher/Consumer mehr.

- **`MessageId`** der AMQP-Nachricht trägt zusätzlich die `event_id` (Idempotenz-Key
  der Consumer, `processed_events`).
- **Routing-Key** = `event_type`.
- **`traceparent`-Header** trägt die `trace_id` (Trace-Propagation über die async-Grenze).
- Der Publisher wartet auf den **Publisher-Confirm** des Brokers, bevor die
  Outbox-Zeile als `published_at` markiert wird (kein Event-Verlust bei Broker-Nack).

### 7.4 Event-Katalog (Auszug)

| Event-Type | Producer | Payload-Felder |
|------------|----------|----------------|
| `user.registered` | auth | `user_id`, `email`, `created_at` |
| `user.deleted` | auth | `user_id` |
| `project.created` | project | `project_id`, `name`, `owner_id` |
| `project.deleted` | project | `project_id` |
| `project.member.added` | project | `project_id`, `user_id`, `role` |
| `project.member.removed` | project | `project_id`, `user_id` |
| `board.created` | project | `board_id`, `project_id`, `name`, `type`, `columns[]` (inkl. `status`) |
| `column.created` / `column.updated` | project | `column_id`, `board_id`, `name`, `position`, `status` |
| `boardtype.registered` / `boardtype.updated` / `boardtype.deleted` | boardregistry | `type`, `display_name` |
| `task.created` | task | `task_id`, `board_id`, `title`, `created_by` |
| `task.updated` | task | `task_id`, `changes` (Diff) |
| `task.assigned` | task | `task_id`, `assignee_id`, `assigned_by` |
| `task.status.changed` | task | `task_id`, `from`, `to` |
| `task.commented` | task | `task_id`, `comment_id`, `author_id`, `mentions[]` |
| `document.uploaded` | document | `document_id`, `project_id`, `version` |
| `document.version.created` | document | `document_id`, `version`, `size_bytes` |

Vollständiger Katalog in `docs/api/events.asyncapi.yaml`.

### 7.5 Outbox-Pattern (verbindlich)

Jeder Service, der Domain-Events produziert, schreibt sie in eine `outbox`-Tabelle in derselben DB-Transaktion wie die Datenänderung. Der gemeinsame `shared/go/outbox`-`Worker` pollt die Outbox (`FOR UPDATE SKIP LOCKED`), verpackt jede Zeile in den Envelope (§7.3) und publiziert via `shared/go/eventbus`-Publisher an RabbitMQ. Die Outbox-Zeile wird erst nach bestätigtem Publisher-Confirm in derselben Transaktion als `published_at` markiert — schlägt der Publish fehl, wird die Transaktion zurückgerollt und beim nächsten Poll erneut versucht.

```sql
CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type   TEXT NOT NULL,
    payload      JSONB NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at)
    WHERE published_at IS NULL;
```

### 7.6 Idempotente Konsumenten (verbindlich)

Jeder Konsument speichert verarbeitete `event_id`s in einer `processed_events`-Tabelle. Vor Verarbeitung wird geprüft, ob die ID schon existiert.

```sql
CREATE TABLE processed_events (
    event_id    TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## 8. Datenmodell-Übersicht

### 8.1 Geteilte Konventionen

- **Primärschlüssel:** UUID v7 (zeitlich sortierbar) als `uuid` in PostgreSQL
- **Zeitstempel:** `created_at`, `updated_at` als `TIMESTAMPTZ`
- **Soft-Delete:** `deleted_at TIMESTAMPTZ NULL` (statt physisches DELETE bei Domain-Entitäten)
- **Audit-Felder:** `created_by`, `updated_by` als UUID (User-ID aus Auth)

### 8.2 Aggregate-Übersicht (vereinfacht)

```
Auth Service:
  users (id, email, password_hash, created_at, ...)
  refresh_tokens (id, user_id, token_hash, expires_at, ...)

Project Service:
  projects (id, name, description, owner_id, created_at, ...)
  boards (id, project_id, name, type, position, ...)
  project_members (project_id, user_id, role, joined_at)

Task Service:
  tasks (id, board_id, title, description, status, assignee_id,
         due_date, position, created_by, ...)
  task_comments (id, task_id, author_id, body, created_at)
  task_attachments (id, task_id, document_id, added_at)
  task_history (id, task_id, change_type, before, after, actor, at)

Document Service:
  documents (id, project_id, name, current_version, ...)
  document_versions (id, document_id, version_number,
                     storage_key, size_bytes, content_type,
                     uploaded_by, uploaded_at)

Notification Service:
  notifications (id, user_id, type, payload, read_at, created_at)

Plugin Service:
  webhooks (id, project_id, target_url, event_filter,
            secret, active, created_by)
  webhook_deliveries (id, webhook_id, event_id, status_code,
                      attempt, delivered_at, error)
```

Detaillierte Schemas inklusive Constraints und Indexes folgen in den Service-spezifischen Design-Dokumenten.

---

## 9. Authentifizierung und Autorisierung

### 9.1 Authentifizierung

**JWT mit RS256.** Auth Service signiert mit Private Key, alle anderen Services validieren über JWKS-Endpoint.

**Token-Inhalt (Claims):**
```json
{
  "iss": "https://auth.teamboard.local",
  "sub": "user-uuid",
  "aud": "teamboard-api",
  "iat": 1715000000,
  "exp": 1715003600,
  "email": "user@example.com",
  "scope": "user"
}
```

**Refresh-Tokens:** Opaque Strings, gespeichert als Hash in DB. Nur über `/auth/refresh` einlösbar.

### 9.2 Authorisierung

**Project Service ist Authority** für projektbezogene Permissions. Andere Services rufen synchron an:

```
GET /api/v1/internal/projects/{projectId}/permissions/{userId}
→ { "role": "editor", "permissions": ["task:read", "task:write", ...] }
```

Mit Cache (Redis, TTL 30s) zur Lastreduktion. Cache wird bei `project.member.*` Events invalidiert.

**Rollen-Matrix:**

| Rolle | Tasks | Documents | Members | Projekt-Settings |
|-------|-------|-----------|---------|------------------|
| Owner | full | full | full | full |
| Editor | full | full | read | read |
| Viewer | read | read | read | read |

### 9.3 Service-to-Service-Authentifizierung

Interne Endpunkte unter `/api/v1/internal/...` erwarten zusätzlich einen Service-Token:
ein kurzlebiges HS256-JWT (`aud: "internal"`, 60 s TTL) aus `shared/go/servicetoken`. Der
**aufrufende** Service stellt es pro Request mit dem geteilten `SERVICE_TOKEN_SECRET` aus
(`servicetoken.Issuer`); der Empfänger verifiziert mit `servicetoken.RequireServiceToken`.
Verhindert, dass externe Clients interne Routes ansprechen — diese Routen sind zudem in keiner
Gateway-`rule` und daher am Edge gar nicht erreichbar (Defense-in-Depth).

---

## 10. Observability

### 10.1 Logs

Strukturiert (JSON) → CloudWatch (AWS) oder Loki (lokal). Trace-ID in jedem Log-Eintrag.

### 10.2 Metrics

Prometheus-Format unter `/metrics` pro Service. Standard-Metriken:

- `http_requests_total{service, method, route, status}`
- `http_request_duration_seconds{service, route}` (Histogram)
- `event_published_total{service, event_type}`
- `event_consumed_total{service, event_type, status}`
- `outbox_unpublished_count{service}`

### 10.3 Tracing

OpenTelemetry mit OTLP-Export. Trace-ID wird vom Gateway gesetzt und durch alle Services weitergereicht (HTTP-Header `traceparent`, AMQP-Header `traceparent`).

### 10.4 Health-Checks

Pflicht pro Service:

- `GET /health/live` — Liveness (Service läuft)
- `GET /health/ready` — Readiness (DB, RabbitMQ, externe Abhängigkeiten erreichbar)

---

## 11. Lokale Entwicklung

### 11.1 Voraussetzungen

- Docker + Docker Compose
- Make
- Go 1.22+
- Node.js 20+ (für Frontend)

### 11.2 Start

```bash
git clone https://github.com/fh-wedel/MSA-26-TeamBoard
cd teamboard
make up         # startet alle Services + Infrastruktur
make seed       # füllt Datenbanken mit Beispieldaten
make logs       # zeigt aggregierte Logs
make down       # fährt alles herunter
```

### 11.3 Docker-Compose-Architektur

`docker-compose.yml` enthält:

- **Infrastruktur:** Postgres (eine Instanz mit DBs pro Service für Entwicklung), Redis, RabbitMQ, MinIO, Jaeger
- **Services:** alle 7 Services + Frontend
- **Gateway:** Traefik mit lokalem Self-Signed-Cert
- **Helper:** Adminer (DB-UI), MinIO Console, RabbitMQ Management

### 11.4 Hot-Reload für Go-Services

`docker-compose.override.yml` mountet Source-Code, nutzt `air` für Auto-Reload bei Code-Änderungen.

---

## 12. Deployment

### 12.1 CI (GitHub Actions)

`.github/workflows/ci.yml` läuft bei Push und PR:

1. **Detect Changes** — Path-Filter ermittelt geänderte Services
2. **Lint** — `golangci-lint`, `eslint`
3. **Test** — `make test` pro geändertem Service
4. **Build** — Container-Image bauen
5. **Push** — bei `main`-Branch in ECR

### 12.2 CD (Staging/Prod)

Trigger bei Tag `v*.*.*`:

1. CDK Synth + Deploy zu Staging
2. Smoke-Tests
3. Manuelle Approval-Stage
4. Deploy zu Prod (Blue/Green via ECS)

### 12.3 AWS-Targets

Siehe separates Dokument `docs/aws-deployment.md`. Zusammenfassung:

- ECS Fargate für Services
- RDS für Postgres
- ElastiCache für Redis
- Amazon MQ für RabbitMQ
- S3 für Dokumente
- API Gateway + CloudFront
- Cognito für Auth (ersetzt eigenen Auth Service in Produktion optional)

---

## 13. Glossar

| Begriff | Bedeutung |
|---------|-----------|
| **Aggregate** | DDD-Begriff: Konsistenz-Cluster (z. B. Task mit Comments) |
| **Bounded Context** | Fachlich abgegrenzter Bereich, eigener Service |
| **CQRS** | Command-Query-Responsibility-Segregation |
| **DLQ** | Dead Letter Queue — Queue für nicht zustellbare Nachrichten |
| **Idempotenz** | Mehrfachausführung produziert dasselbe Ergebnis |
| **JWKS** | JSON Web Key Set — öffentliche Keys zur JWT-Validierung |
| **Outbox-Pattern** | Pattern zur transaktionssicheren Event-Publikation |
| **Saga** | Pattern für lange laufende, kompensierbare verteilte Transaktionen |
| **ULID** | Universally Unique Lexicographically Sortable Identifier |

---

## Anhang A: Coding-Agent-Hinweise

> **Für KI-Agenten, die Code in diesem Repository generieren:**
>
> 1. **Lies dieses Dokument vollständig**, bevor du Code generierst.
> 2. **Befolge die Service-Layout-Konvention** aus Abschnitt 5.2.
> 3. **Nutze die Shared-Libraries** aus `shared/go/` statt Code zu duplizieren.
> 4. **Implementiere Outbox-Pattern** für jeden Service, der Events produziert.
> 5. **Schreibe immer** OpenAPI-Spec → sqlc-Queries → generierten Code → Handler.
> 6. **Tests sind Pflicht** — Unit-Tests für Domain, Integration-Tests für Repository.
> 7. **Frage nach**, wenn das Design lückenhaft ist. Erfinde keine Konventionen.
> 8. **Halte die Service-Grenzen ein** — keine Cross-Service-DB-Zugriffe, keine geteilten Datenmodelle.
> 9. **Logge strukturiert** mit `slog`, niemals mit `fmt.Println`.
> 10. **Fehler typisiert** als Domain-Errors, niemals als Strings.

## Anhang B: Nächste Detaildokumente

Folgende Detaildokumente werden pro Service ergänzt:

- `docs/services/auth.md`
- `docs/services/project.md`
- `docs/services/task.md`
- `docs/services/document.md`
- `docs/services/notification.md`
- `docs/services/plugin.md`

Jedes Detaildokument enthält:
- Vollständiges Datenbankschema mit DDL
- Vollständige OpenAPI-Spec
- Domain-Modell und Use-Cases
- Event-Definitionen mit Payload-Schemas
- Test-Strategie
- Beispiel-Implementierung der wichtigsten Handler

## Anhang C: Weitere Spezifikationen
- `docs/specifications/coding-guidelines.md`
- `docs/specifications/orchestration.md`
- `docs/specifications/shared.md`
