# Board Registry Service — Detail-Design

> **Service:** `boardregistry`
> **Port:** 8007
> **Datenbank:** `boardregistry_db` (PostgreSQL)
> **Modulpfad:** `github.com/teamboard/services/boardregistry`

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet
- **Autoritative Quelle für Board-Typ-*Definitionen*** — eingebaut (`kanban`, `calendar`) plus
  zur Laufzeit registrierte Typen (z. B. `scrum`, `gantt`; siehe `docs/demo/board-types.ipynb`).
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
| `presentation` | JSONB | Deklarative Rendering-Hints fürs Frontend (s. u.) |
| `built_in` | BOOLEAN | Built-ins sind unveränderlich/nicht löschbar |
| `created_by` | UUID NULL | Registrierender User |
| `created_at`, `updated_at` | TIMESTAMPTZ | |

Plus `outbox` (Outbox-Pattern). Die eingebauten Typen `kanban` und `calendar` werden inkl.
Schema und Presentation-Spec in der Baseline-Migration `0001_init` angelegt (der aktuelle
Stand ist die erste Version; es gibt keine inkrementelle Migrationshistorie). Weitere Typen
wie `scrum` und `gantt` werden **nicht** geseedet, sondern zur Laufzeit über die API
registriert — siehe das Demo-Notebook `docs/demo/board-types.ipynb` (`make demo-board-types`).

`status` ∈ {`open`, `in_progress`, `blocked`, `done`, `archived`} (muss zum Task-Service-Statusset
passen).

### 2.1 Presentation-Spec

Das `presentation`-JSONB steuert **deklarativ**, wie ein Boardtyp im Frontend dargestellt wird —
nicht nur *welche* View, sondern *wie* sie aussieht. Es wird gegen ein **host-definiertes
Meta-Schema** validiert (anders als `config_schema`, das der Typ-Autor frei definiert). Siehe
ADR 0002.

```jsonc
{
  "view": "board",                 // board | calendar | timeline (eingebauter Renderer)
  "view_config": { ... },          // renderer-spezifisch, deklarativ
  "card": {                        // view-übergreifend: Task-Karten-Darstellung
    "fields": ["priority","due_date","labels","comment_count","attachment_count"],
    "color_by": "priority"         // priority | status | label
  }
}
```

Erlaubte `view_config`-Keys je View:
- **board:** `group_by` (column|assignee|priority), `show_wip` (bool), `swimlane_by`
- **calendar:** `date_field`, `week_start` (monday|sunday), `default_range` (month|week)
- **timeline:** `start_field`, `end_field`, `group_by`, `color_by`

Leere/unbekannte `view` → Frontend fällt auf das Spalten-Board zurück (mit Hinweis). Der
`presentation.view`-Schalter ist der einzige Erweiterungspunkt: ein künftiges `view:"remote"`
(Micro-Frontend) ließe sich ohne Modell-Umbau ergänzen.

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

`validatePresentation` prüft die optionale `presentation`-Spec gegen das host-definierte
Meta-Schema: erlaubte `view`-Werte, je-View bekannte `view_config`-Keys, `week_start`/`color_by`-
Enums sowie `card.fields`/`card.color_by`. Eine leere Spec ist gültig (Default `board`).

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
