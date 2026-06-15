# Board Registry Service — Detail-Design

> **Service:** `boardregistry`
> **Port:** 8007
> **Datenbank:** `boardregistry_db` (PostgreSQL)
> **Modulpfad:** `github.com/teamboard/services/boardregistry`

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet
- **Autoritative Quelle für Board-Typ-*Definitionen*** (kanban, scrum, calendar + benutzerdefinierte
  Typen).
- **Laufzeit-Registrierung** neuer Board-Typen durch Entwickler (REST-API) — ohne Redeploy des
  Project-Service.
- Bereitstellung von Default-Columns (inkl. Status), Default-Config und JSON-Schema je Typ.
- Publikation von `boardtype.*`-Events, damit Konsumenten (Project-Service) Caches invalidieren.

### 1.2 Verantwortet NICHT
- **Board-*Instanzen*** (Boards, Spalten, deren Persistenz) — das bleibt im Project-Service.
- Tasks, Permissions, Webhooks.

### 1.3 Abgrenzung zum Plugin/Webhook-Service
Der Plugin/Webhook-Service (8006) ist ein **Output-Adapter** (Domain-Event → externe Aktion).
Board-Typen sind ein **strukturelles Erweiterungskonzept** und liegen bewusst hier, nicht im
Plugin-Service.

---

## 2. Datenmodell

Tabelle `board_types`:

| Spalte | Typ | Notiz |
|--------|-----|-------|
| `id` | UUID PK | Aggregat-ID (für Outbox) |
| `type` | TEXT UNIQUE | Slug, `^[a-z][a-z0-9_-]{0,49}$` |
| `display_name` | TEXT | 1..100 Zeichen |
| `icon` | TEXT | |
| `default_columns` | JSONB | Array `{name, position, wip_limit?, status}` |
| `default_config` | JSONB | Typ-spezifische Default-Konfiguration |
| `config_schema` | JSONB | JSON-Schema zur Validierung der Board-Config |
| `built_in` | BOOLEAN | Built-ins sind unveränderlich/nicht löschbar |
| `created_by` | UUID NULL | Registrierender User |
| `created_at`, `updated_at` | TIMESTAMPTZ | |

Plus `outbox` (Outbox-Pattern). Built-in-Typen (kanban/scrum/calendar) werden per Seed-Migration
angelegt (`0002_seed_builtin_types`).

`status` ∈ {`open`, `in_progress`, `blocked`, `done`, `archived`} (muss zum Task-Service-Statusset
passen).

---

## 3. API

Alle Routen unter `/api/v1`. Öffentliche Routen: JWT (strukturelle Prüfung, wie Project-Service).

| Methode | Pfad | Auth | Zweck |
|---------|------|------|-------|
| GET | `/board-types` | JWT | Katalog auflisten |
| GET | `/board-types/{type}` | JWT | Einzeltyp |
| POST | `/board-types` | JWT* | Typ registrieren |
| PATCH | `/board-types/{type}` | JWT* | Typ aktualisieren (nicht built-in) |
| DELETE | `/board-types/{type}` | JWT* | Typ löschen (nicht built-in) |
| GET | `/internal/board-types` | Service-Token | Katalog (intern, vom Project-Service) |
| GET | `/internal/board-types/{type}` | Service-Token | Einzeltyp (intern) |

`*` **Authz-Offene Stelle:** Schreibzugriffe sind derzeit für jeden authentifizierten User offen.
Eine Beschränkung auf eine Admin-/Publisher-Rolle ist als Folgearbeit vorgesehen (siehe ADR 0001).

Erfolgsantworten: `{ "data": ... }`. Fehler: RFC-7807-ähnliche Problem-Details mit `code`
(`validation_failed` 400, `board_type_not_found` 404, `board_type_exists` 409,
`builtin_immutable` 403).

Service-Token = `HMAC-SHA256("internal", SERVICE_TOKEN_SECRET)` (gleiche Variante wie
Project↔Plugin/Task/Document).

---

## 4. Events

Publiziert über das Outbox-Pattern auf `teamboard.events`:

| Event-Type | Trigger | Payload |
|------------|---------|---------|
| `boardtype.registered` | POST | `type, display_name` |
| `boardtype.updated` | PATCH | `type, display_name` |
| `boardtype.deleted` | DELETE | `type, display_name` |

Konsumiert: keine. Der Project-Service konsumiert die obigen Events zur Cache-Invalidierung.

---

## 5. Validierung

`validateDefinition` prüft Slug-Format, Display-Name-Länge, Spalten (Name, eindeutige Position,
`status` im erlaubten Set, `wip_limit >= 0`), Kompilierbarkeit des `config_schema` und dass die
`default_config` das Schema erfüllt (`santhosh-tekuri/jsonschema/v5`).

---

## 6. Konfiguration (Environment)

```bash
SERVICE_PORT=8007
DB_URL=postgres://...@postgres:5432/boardregistry_db?sslmode=disable
RABBITMQ_URL=amqp://...@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
SERVICE_TOKEN_SECRET=<shared secret>
OUTBOX_INTERVAL=2s
OUTBOX_BATCH_SIZE=50
```

---

## 7. Integration mit dem Project-Service

Der Project-Service nutzt `internal/boardtypeclient` (implementiert `domain.BoardTypeRegistry`),
um Typdefinitionen über die internal-API zu beziehen (TTL-Cache, Service-Token). `CreateBoard`
verwendet Default-Columns/Config und validiert gegen das `config_schema`. Bei `boardtype.*`-Events
invalidiert der Project-Consumer den Cache. Siehe `docs/services/project.md §9`.
