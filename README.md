# MSA26-TeamBoard — Gruppe D

**TeamBoard** ist eine web-basierte Kollaborationsplattform für Teams: Projekt- und
Board-Verwaltung mit rollenbasierten Berechtigungen, Aufgaben (Status, Zuweisung,
Kommentare, Anhänge), Dokumentenablage mit Versionierung, Echtzeit-Benachrichtigungen
und Webhook-Integrationen. Umgesetzt als Microservice-System nach Domain-Driven Design.

> Architektur-Detailreferenz: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
> Verbindliche Coding-Konventionen für dieses Repo: [`CLAUDE.md`](CLAUDE.md).

---

## Tech-Stack

Go 1.22+ · Chi-Router · sqlc · PostgreSQL 16 · Redis 7 · RabbitMQ 3.12 ·
Docker Compose · Traefik · React/TypeScript (Frontend) · GitHub Actions · AWS EC2 (Prod).

---

## Quickstart (lokal)

**Voraussetzungen:** Docker + Docker Compose, Make, Python 3 (für Seed-/Helper-Skripte).
Go 1.22+ und Node.js 20+ nur nötig, wenn Services bzw. Frontend außerhalb von Docker
gebaut werden.

```bash
make up         # startet alle Services + Infrastruktur (erster Lauf ~3 min)
make seed       # Demo-Daten: alice@teamboard.local / AliceSecret123!
make urls       # listet alle erreichbaren URLs
make logs       # aggregierte Logs (oder make logs-<svc>, z. B. logs-task)
make down       # stoppt alles (Daten bleiben erhalten)
```

Nach `make up` erreichbar:

| Was | URL | Zugang |
|-----|-----|--------|
| Frontend | http://localhost:3000 | alice@teamboard.local / `AliceSecret123!` |
| API (über Gateway) | http://localhost | JWT vom Auth-Service |
| API-Doku Plugin/Webhook | http://localhost/plugin/docs | — (Swagger UI) |
| API-Doku Board Registry | http://localhost/board-registry/docs | — (Swagger UI) |
| Traefik Dashboard | http://localhost:8080 | — |
| RabbitMQ UI | http://localhost:15672 | teamboard / teamboard |
| MinIO Console | http://localhost:9001 | teamboard / teamboard-secret |
| MailHog | http://localhost:8025 | — |
| Jaeger UI | http://localhost:16686 | — |

`make clean-volumes` setzt alles zurück (löscht **alle** Daten, fragt vorher nach).

---

## Services

Domain-Services hinter einem Traefik-Gateway, asynchron gekoppelt über RabbitMQ
(`teamboard.events`, Topic-Exchange). Jeder Service hat eine eigene logische DB
(eine Postgres-Instanz lokal, separate RDS in AWS).

| Service | Port | Verantwortung | Doku |
|---------|------|---------------|------|
| Gateway (Traefik) | 80 | Routing, Rate Limiting, CORS, Trace-ID | [gateway.md](docs/specifications/gateway.md) |
| Auth | 8001 | JWT-Ausstellung, Registrierung/Login, JWKS | [auth.md](docs/services/auth.md) |
| Project | 8002 | Projekte, Boards, Mitglieder, **Permission-Authority** | [project.md](docs/services/project.md) |
| Task | 8003 | Tasks, Kommentare, Statusübergänge, Anhang-Refs | [task.md](docs/services/task.md) |
| Document | 8004 | Metadaten + S3/MinIO, Pre-Signed URLs, Versionierung | [document.md](docs/services/document.md) |
| Notification | 8005 | WebSocket-Push + persistente Notifications (stateful) | [notification.md](docs/services/notification.md) |
| Plugin/Webhook | 8006 | Webhook-Registrierung, Auslieferung mit Retry + HMAC | [plugin.md](docs/services/plugin.md) |
| Board Registry | 8007 | **Authority für Board-Typ-Definitionen**, Laufzeit-Registrierung | [boardregistry.md](docs/services/boardregistry.md) |

**Board-Typen** (kanban/scrum/calendar/gantt + eigene) sind Laufzeit-Daten der Board
Registry, kein Compile-Zeit-Code. Der Project-Service löst Default-Columns, Default-Config
und JSON-Schema beim Board-Erstellen über die interne API der Registry auf (gecacht,
Service-Token) und invalidiert den Cache auf `boardtype.*`-Events. Siehe
[ADR 0001](docs/decisions/0001-board-type-extensibility.md) und
[ADR 0002](docs/decisions/0002-board-view-extensibility.md).

---

## Entwicklung

```bash
make generate          # sqlc + oapi-codegen nach Änderungen an .sql / OpenAPI
make test              # alle Tests (shared + unit + integration)
make test-unit         # nur Unit-Tests (-short, ohne Testcontainers)
make lint              # golangci-lint über alle Services + shared/go
make demo-board-types  # registriert scrum + gantt zur Laufzeit (Notebook-Demo)
```

Einzelner Service:

```bash
cd services/task && go test -short -race ./...               # Unit
cd services/task && go test -race ./internal/repository/...  # Integration (Docker)
```

Vollständige Befehlsliste: `make help`.

---

## Repository-Struktur

```
.
├── services/<name>/        # ein Go-Modul pro Service (auth, project, task,
│                           #   document, notification, plugin, boardregistry)
│   ├── cmd/server/main.go  # Wiring
│   ├── internal/{api,domain,repository,events,config}/
│   ├── migrations/         # golang-migrate
│   └── queries/            # sqlc-Input
├── shared/go/              # Cross-Cutting-Libraries (authmiddleware, eventbus,
│                           #   outbox, observability, httputil, servicetoken)
├── frontend/               # React/TypeScript SPA
├── docs/                   # ARCHITECTURE.md, decisions/ (ADRs), services/, specifications/
├── infra/                  # Traefik-Config (lokal + Prod)
└── scripts/                # seed.py, wait-healthy.py, …
```

---

## Dokumentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Master-Referenz: Service-Katalog, Event-Schema, Datenmodell
- [`docs/decisions/`](docs/decisions) — Architecture Decision Records (Board-Typ- & View-Extensibility)
- [`docs/services/`](docs/services) — Detail pro Service: DB-Schema, OpenAPI, Use-Cases, Test-Strategie
- [`docs/specifications/`](docs/specifications) — Coding-Guidelines, Shared-Libs, Orchestrierung, Gateway
- [`docs/specifications/deployment.md`](docs/specifications/deployment.md) — CI/CD-Pipeline (GitHub Actions → GHCR) und AWS-EC2-Deployment
- [`docs/demo/`](docs/demo) — Notebooks (Board-Typen zur Laufzeit, Webhooks)
