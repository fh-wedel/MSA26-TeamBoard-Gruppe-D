# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## Project Overview

**TeamBoard** is a web-based collaboration platform designed as a microservice system. The repository currently contains architecture specifications and design documents; the actual implementation follows these specifications.

**Tech stack:** Go 1.22+, Chi router, sqlc, PostgreSQL 16, Redis 7, RabbitMQ 3.12, Docker Compose, Traefik, GitHub Actions, AWS EC2. See `docs/specifications/deployment.md` for the CI/CD and deployment setup.

---

## Local Development Commands

All commands are run from the repo root via `make`:

```bash
make up              # Start all services + infrastructure (first run ~3 min)
make up-build        # Rebuild images and start
make down            # Stop all containers (keeps volumes/data)
make clean-volumes   # Full reset — deletes all data, prompts for confirmation
make seed            # Insert demo data (alice@teamboard.local / AliceSecret123!)
make logs            # Tail logs from all containers
make logs-<svc>      # Tail one service, e.g. make logs-task
make ps              # Show container status
make urls            # Print all service URLs
```

**Code generation** (run after changing `.sql` query files or OpenAPI specs):
```bash
make generate        # Runs sqlc generate + oapi-codegen for all services
```

**Testing:**
```bash
make test            # All tests (shared + unit + integration)
make test-unit       # Unit tests only (-short flag, skips Testcontainers)
make test-integration # Integration tests (requires Docker for Testcontainers)
make test-shared     # Shared library tests (90% coverage required)
```

**Single-service test:**
```bash
cd services/domain/task && go test -short -race ./...              # unit
cd services/domain/task && go test -race ./internal/repository/... # integration
cd services/domain/task && go test -run TestTaskService_CreateTask_HappyPath ./internal/domain/...
```

**Linting:**
```bash
make lint            # golangci-lint on all services + shared/go
make lint-fix        # with --fix
```

---

## Architecture

### High-Level Structure

Domain services behind a Traefik gateway, communicating asynchronously via RabbitMQ (`teamboard.events` topic exchange). Synchronous calls only when the user needs an immediate response.

| Service | Port | Responsibility |
|---------|------|----------------|
| Auth | 8001 | JWT issuance, user registration/login, JWKS endpoint |
| Project | 8002 | Projects, boards, memberships, **authoritative source for permissions** |
| Task | 8003 | Tasks, comments, status transitions, attachment refs |
| Document | 8004 | Metadata + S3/MinIO storage, pre-signed URLs, versioning |
| Notification | 8005 | WebSocket push + persistent notifications; stateful (Redis backplane) |
| Plugin/Webhook | 8006 | Webhook registration, delivery with retry + HMAC signing |
| Board Registry | 8007 | **Authoritative source for board-type definitions**; runtime registration of new board types |
| Gateway | 80 | Traefik routing, rate limiting, CORS, trace-ID injection |

**Board-type extensibility:** Board *types* (kanban/scrum/calendar + custom) are runtime data owned by the Board Registry service, not compile-time code. The Project Service resolves a type's default columns, default config, and JSON-schema validation from the registry's internal API (`GET /api/v1/internal/board-types/{type}`, cached, service-token auth) when creating a board, and invalidates that cache on `boardtype.*` events. Each default column carries an explicit semantic `status` (open/in_progress/blocked/done/archived) that flows via `board.created`/`column.*` events into the Task Service, which uses it directly instead of guessing from the column name (`DeriveStatus` remains a fallback).

**Board-view extensibility:** Each board-type definition also carries a declarative `presentation` spec (`view` + `view_config` + `card`), validated server-side against a host-defined meta-schema. The frontend's view registry (`frontend/src/components/board/views/`) picks a built-in renderer (`BoardView`/`CalendarView`/`GanttView`) from `presentation.view`; unknown/empty falls back to the column board. `presentation.view` is the single extension point — a future `view:"remote"` (micro-frontend) fits without a model change. See ADR 0002.

**Authorization flow:** Task, Document, Notification, and Plugin services each call `GET /api/v1/internal/projects/{id}/permissions/{userId}` on the Project Service before every write operation. Responses are cached in Redis (TTL 30s) and invalidated on `project.member.*` events.

**Outbox Pattern (mandatory):** Every service that publishes events writes to an `outbox` table in the same DB transaction as the domain mutation. The `shared/go/outbox` Worker polls and publishes to RabbitMQ. Consumers track processed `event_id`s in a `processed_events` table for idempotency.

**Database topology (local dev):** One Postgres instance with seven logical databases (`auth_db`, `project_db`, `task_db`, `document_db`, `notification_db`, `plugin_db`, `boardregistry_db`). In AWS these become separate RDS instances.

### Monorepo Layout

```
teamboard/
├── services/
│   ├── domain/<name>/        # One Go module per DOMAIN service (auth, project, task,
│   │   │                     #   document, notification, plugin, boardregistry)
│   │   │                     #   module: github.com/teamboard/services/domain/<name>
│   │   ├── cmd/server/main.go # Wiring only — no logic
│   │   ├── internal/
│   │   │   ├── api/          # HTTP handlers, DTOs, middleware
│   │   │   ├── domain/       # Business logic — no imports from api/repository/events
│   │   │   ├── repository/   # Repository interface (defined in domain/) + postgres impl
│   │   │   ├── events/       # Publisher + consumer
│   │   │   └── config/
│   │   ├── migrations/       # golang-migrate SQL files
│   │   └── queries/          # sqlc .sql input files
│   └── other/                # Non-domain services (protocol adapters etc.)
│       └── mcp-server/       # MCP protocol adapter (BFF) — no DB, no events
│                             #   module: github.com/teamboard/services/other/mcp-server
├── shared/go/                # Cross-cutting libraries used by all services
│   ├── authmiddleware/       # JWT validation middleware + JWKS cache
│   ├── eventbus/             # RabbitMQ wrapper (Envelope, Publisher, Consumer)
│   ├── outbox/               # Outbox pattern worker
│   ├── observability/        # slog JSON logger, OTel tracer, sensitive-data redaction
│   ├── httputil/             # RFC 7807 problem details, JSON helpers, cursor pagination
│   └── servicetoken/         # Service-to-service JWT (HS256, aud: "internal")
├── docs/
│   ├── ARCHITECTURE.md       # Master reference — read before generating any code
│   ├── specifications/       # Coding guidelines, shared lib spec, orchestration spec, gateway spec
│   └── services/             # Per-service detail docs (DB schema, OpenAPI, use-cases)
└── infra/
    └── traefik/              # Gateway config (local + prod)
```

---

## Mandatory Code Conventions

### Service Layer Rules

- `domain/` **must never import** from `api/`, `repository/`, or `events/`. Dependency flows inward only.
- **Repository interfaces** are defined in `domain/`, implemented in `repository/`.
- **DTOs** (`api/dto.go`) are distinct from domain entities; mapping functions are in `api/`.
- **HTTP handlers** are thin: parse params → call domain service → map to DTO → write response.

### Error Handling

Domain errors are typed structs, not strings:
```go
var ErrTaskNotFound = &domain.Error{Code: "task_not_found", Message: "task not found"}
```
Use `errors.Is` / `errors.As` for checks. Always wrap with `%w`. `shared/go/httputil.WriteError` handles HTTP mapping automatically.

### Logging

Use `log/slog` (stdlib) via `observability.SetupLogger`. Every log entry must carry `task_id`, `user_id`, and duration where applicable. Never log passwords, tokens, or full PII. Never use `fmt.Println`.

### API Conventions

- URL: `/api/v1/<resources>` (plural), cursor-based pagination (`?cursor=<base64>&limit=50`)
- Success: `{ "data": {...} }` or `{ "data": [...], "pagination": {...} }`
- Errors: RFC 7807 Problem Details with `trace_id`
- Sub-actions: `/tasks/{id}:assign` (colon) or PATCH

### Code Generation Workflow

OpenAPI spec → `oapi-codegen` → generated server stubs → handwritten handlers implement the interfaces.  
SQL queries → `sqlc generate` → typed Go code in `internal/repository/db/` (never edit generated files).

### Testing

- Format: `TestCreateTask_HappyPath`, `TestTaskService_CreateTask_PermissionDenied`
- Use `t.Run` subtests and table-driven tests for variations
- `require` for preconditions (stops test), `assert` for assertions (continues)
- `t.Cleanup` instead of `defer` in tests
- Testcontainers for DB/event integration tests; skip with `-short` for unit tests
- Coverage minimums: `domain/` ≥ 80%, `repository/` ≥ 70%, `api/` ≥ 60%, `shared/go/` ≥ 90%

### Key Go Idioms Enforced by Linter

- Acronyms uppercase: `userID`, `httpClient`, `jwksURL` (not `userId`, `HttpClient`)
- No `init()` with side effects — use explicit `Init()` called from `main.go`
- Goroutines must have a defined termination mechanism (context cancellation)
- `context.Context` always the first argument, never stored in structs
- Interfaces defined at the consumer, not the implementer; keep them small
- Prefer early returns (guard clauses) over nested if-else

### Shared Libraries Usage

Never re-implement what exists in `shared/go/`. Key imports:
- `authmiddleware.Middleware(jwks)` — JWT validation Chi middleware
- `authmiddleware.MustUserID(ctx)` — extract user UUID from context
- `outbox.InsertEvent(ctx, tx, params)` — write event in same DB transaction
- `httputil.WriteError(w, r, err)` — maps domain errors to RFC 7807 responses
- `eventbus.IdempotentHandler(inner, store)` — wraps consumer with idempotency check
- `servicetoken.RequireServiceToken(verifier, "internal")` — protects `/internal/` routes

Services include `shared/go` via replace directive in `go.mod`:
```
replace github.com/teamboard/shared/go => ../../../shared/go
```

---

## Key Documents

- `docs/ARCHITECTURE.md` — master reference for all design decisions, event catalog, data model
- `docs/specifications/coding-guidelines.md` — binding Go style rules with code examples
- `docs/specifications/shared.md` — shared library APIs with usage examples
- `docs/specifications/orchestration.md` — full Docker Compose config, Makefile, and seed script
- `docs/specifications/gateway.md` — Traefik routing rules, rate limiting, CORS config
- `docs/services/*.md` — per-service detail: DB schema, OpenAPI spec, use-cases, test strategy
- `docs/TODO.md` — **backlog of not-yet-implemented changes** (gateway/observability hardening, missing tests, authz follow-ups). Check here before assuming a documented feature is built; update it when closing an item.
