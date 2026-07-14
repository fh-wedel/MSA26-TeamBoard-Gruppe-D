# Orchestrierung — Docker Compose und Makefile

> **Zweck:** Spezifikation der lokalen Entwicklungs-Orchestrierung. Erfüllt die Aufgabenstellungs-Anforderung *"sowohl das TeamBoard-System als auch die Test-Clients sollen nach dem Auschecken des Repositories mit Hilfe eines einzigen Kommandos übersetzt und dann einfach in Betrieb genommen werden können"*.  
>
> **Pfad:** `docker-compose.yml`, `docker-compose.override.yml`, `Makefile` im Repo-Root  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Designprinzipien](#1-designprinzipien)
2. [Netzwerk-Layout](#2-netzwerk-layout)
3. [Service-Inventar](#3-service-inventar)
4. [docker-compose.yml (Vollständig)](#4-docker-composeyml-vollständig)
5. [docker-compose.override.yml (Hot-Reload)](#5-docker-composeoverrideyml-hot-reload)
6. [Service-Konfiguration im Detail](#6-service-konfiguration-im-detail)
7. [Initialisierungs-Container](#7-initialisierungs-container)
8. [Makefile (Root)](#8-makefile-root)
9. [Seed-Daten](#9-seed-daten)
10. [Ports und Zugriff](#10-ports-und-zugriff)
11. [Troubleshooting](#11-troubleshooting)
12. [Implementierungs-Hinweise](#12-implementierungs-hinweise)

---

## 1. Designprinzipien

**Ein Befehl reicht.** `make up` startet die komplette Umgebung — Domain-Services, Infrastruktur (DB, Broker, Storage), Observability-Tools, Frontend, Gateway. Zweiter Aufruf ist no-op (idempotent).

**Saubere Trennung Build vs. Run.** `docker-compose.yml` definiert das Setup. `docker-compose.override.yml` überlädt für lokale Entwicklung mit Hot-Reload und Source-Mounting. CI nutzt nur die Base-Datei.

**Health-Checks überall.** Services warten auf ihre Abhängigkeiten via `depends_on: condition: service_healthy`. Kein Polling-Code in den Services für "warte bis DB ready".

**Volumes mit Sinn.** Persistente Daten (Postgres, MinIO) in benannten Volumes. Source-Code via Bind-Mount (Override-File). Keine Volumes für Code in Production-Compose.

**Eine Postgres, mehrere Datenbanken.** Im MVP: eine Postgres-Instanz mit Schema-pro-Service (`auth_db`, `project_db`, `task_db`, `document_db`, `notification_db`, `plugin_db`, `boardregistry_db`). Database-per-Service-Prinzip auf logischer Ebene erfüllt, ohne sieben Container für sieben DBs zu starten. In AWS-Deployment werden's separate RDS-Instanzen.

**Kein `latest`-Tag.** Alle Image-Versionen pinnen. Reproduzierbarkeit > minimaler Versionsaufwand.

---

## 2. Netzwerk-Layout

Ein einziges Bridge-Network `teamboard-net`. Alle Container darauf, Service-Discovery via Container-Name.

```
                     Browser / curl
                            │
                            ▼ http://localhost:80
                    ┌───────────────┐
                    │   Traefik     │ Reverse Proxy
                    │   (Gateway)   │ Port 80 publik
                    └───┬───────┬───┘
                        │       │
            ┌───────────┘       └─────────┐
            │ /api/v1/auth/*              │ /api/v1/projects/*, /tasks/*, ...
            ▼                             ▼
    ┌─────────────┐         ┌────────────────────────┐
    │ auth        │         │ project, task, document,│
    │ :8001       │         │ notification, plugin    │
    └──────┬──────┘         │ :8002, :8003, ...       │
           │                └────────────┬────────────┘
           │                             │
           ├──────────────┬──────────────┤
           ▼              ▼              ▼
    ┌──────────┐   ┌──────────┐   ┌──────────┐
    │ Postgres │   │ RabbitMQ │   │  Redis   │
    │  :5432   │   │  :5672   │   │  :6379   │
    └──────────┘   └──────────┘   └──────────┘
                                  
                   ┌──────────┐   ┌──────────┐
                   │  MinIO   │   │  Jaeger  │
                   │  :9000   │   │  :4317   │
                   └──────────┘   └──────────┘
```

**Ports nach außen veröffentlicht:**
- 80 — Traefik (HTTP)
- 3000 — Frontend (während Entwicklung)
- 5432 — Postgres (lokaler DB-Client-Zugriff)
- 6379 — Redis
- 9000 — MinIO S3-API (Browser-Direktupload via Pre-Signed URL)
- 9001 — MinIO Console
- 15672 — RabbitMQ Management UI
- 16686 — Jaeger UI
- 8080 — Traefik Dashboard

**Service-Ports intern (nicht published):**
- 8001-8006 — die sechs Services

---

## 3. Service-Inventar

Vollständige Liste aller Container im Compose-Stack:

### Infrastruktur

| Container | Image | Zweck |
|-----------|-------|-------|
| `postgres` | `postgres:16-alpine` | Alle Service-DBs |
| `redis` | `redis:7.2-alpine` | Cache, Pub/Sub für Notification-Backplane |
| `rabbitmq` | `rabbitmq:3.12-management-alpine` | Event Bus |
| `minio` | `minio/minio:RELEASE.2024-08-17T01-24-54Z` | Object Storage (S3-kompatibel) |
| `mailhog` | `mailhog/mailhog:v1.0.1` | SMTP-Server für Auth-Reset-Mails (catch-all) |
| `jaeger` | `jaegertracing/all-in-one:1.55` | Distributed Tracing |
| `traefik` | `traefik:v3.0` | API Gateway / Reverse Proxy |

### Domain-Services

| Container | Build-Context | Internal Port |
|-----------|---------------|---------------|
| `auth` | `./services/domain/auth` | 8001 |
| `project` | `./services/domain/project` | 8002 |
| `task` | `./services/domain/task` | 8003 |
| `document` | `./services/domain/document` | 8004 |
| `notification` | `./services/domain/notification` | 8005 |
| `plugin` | `./services/domain/plugin` | 8006 |
| `frontend` | `./frontend` | 3000 |

### Initialisierungs-Container (Run-once-and-exit)

| Container | Image | Zweck |
|-----------|-------|-------|
| `postgres-init` | `postgres:16-alpine` | Erstellt die sechs Databases |
| `minio-init` | `minio/mc:latest` | Bucket erstellen, CORS, Lifecycle-Rules setzen |
| `rabbitmq-init` | `rabbitmq:3.12-management-alpine` | Topology setup (optional, Services machen es selbst beim Start) |

---

## 4. docker-compose.yml (Vollständig)

```yaml
name: teamboard

networks:
  teamboard-net:
    driver: bridge

volumes:
  postgres-data:
  redis-data:
  rabbitmq-data:
  minio-data:

services:
  # =========================================================
  # INFRASTRUKTUR
  # =========================================================
  
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: teamboard
      POSTGRES_PASSWORD: teamboard
      POSTGRES_DB: teamboard  # default DB; service-DBs werden via init erstellt
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./scripts/postgres-init.sh:/docker-entrypoint-initdb.d/01-init.sh:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U teamboard"]
      interval: 5s
      timeout: 5s
      retries: 10
    networks:
      - teamboard-net

  redis:
    image: redis:7.2-alpine
    command: redis-server --appendonly yes
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5
    networks:
      - teamboard-net

  rabbitmq:
    image: rabbitmq:3.12-management-alpine
    environment:
      RABBITMQ_DEFAULT_USER: teamboard
      RABBITMQ_DEFAULT_PASS: teamboard
    ports:
      - "5672:5672"
      - "15672:15672"
    volumes:
      - rabbitmq-data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "ping"]
      interval: 10s
      timeout: 10s
      retries: 5
    networks:
      - teamboard-net

  minio:
    image: minio/minio:RELEASE.2024-08-17T01-24-54Z
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: teamboard
      MINIO_ROOT_PASSWORD: teamboard-secret
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio-data:/data
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 5s
      timeout: 5s
      retries: 10
    networks:
      - teamboard-net

  minio-init:
    image: minio/mc:latest
    depends_on:
      minio:
        condition: service_healthy
    entrypoint: /bin/sh
    command:
      - -c
      - |
        mc alias set local http://minio:9000 teamboard teamboard-secret &&
        mc mb --ignore-existing local/teamboard-documents &&
        mc anonymous set none local/teamboard-documents &&
        mc admin policy attach local readwrite --user teamboard || true &&
        echo '{"CORSRules":[{"AllowedMethods":["PUT","GET","HEAD"],"AllowedOrigins":["http://localhost:3000","http://localhost"],"AllowedHeaders":["*"],"ExposeHeaders":["ETag"],"MaxAgeSeconds":3000}]}' | mc admin config set local 'cors=*' || true &&
        echo "MinIO initialized"
    networks:
      - teamboard-net
    restart: "no"

  mailhog:
    image: mailhog/mailhog:v1.0.1
    ports:
      - "1025:1025"   # SMTP
      - "8025:8025"   # Web UI
    networks:
      - teamboard-net

  jaeger:
    image: jaegertracing/all-in-one:1.55
    environment:
      COLLECTOR_OTLP_ENABLED: "true"
    ports:
      - "16686:16686"   # UI
      - "4317:4317"     # OTLP gRPC
      - "4318:4318"     # OTLP HTTP
    networks:
      - teamboard-net

  traefik:
    image: traefik:v3.0
    command:
      - --api.insecure=true
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --entrypoints.web.address=:80
      - --accesslog=true
    ports:
      - "80:80"
      - "8080:8080"   # Traefik Dashboard
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./infra/traefik:/etc/traefik:ro
    networks:
      - teamboard-net

  # =========================================================
  # DOMAIN-SERVICES
  # =========================================================
  
  auth:
    build:
      context: ./services/domain/auth
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: auth-service
      SERVICE_PORT: 8001
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/auth_db?sslmode=disable
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      JWT_ACCESS_TOKEN_TTL: 15m
      JWT_REFRESH_TOKEN_TTL: 720h
      JWT_KEY_ROTATION_INTERVAL: 720h
      PASSWORD_MIN_LENGTH: 12
      PASSWORD_MAX_LENGTH: 128
      REDIS_URL: redis://redis:6379/0
      RATE_LIMIT_ENABLED: "true"
      SMTP_HOST: mailhog
      SMTP_PORT: 1025
      SMTP_FROM: noreply@teamboard.local
      PASSWORD_RESET_TOKEN_TTL: 1h
      PASSWORD_RESET_BASE_URL: http://localhost:3000/reset-password
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: auth-service
      ALLOW_INSECURE_HTTP: "true"
      KEY_ENCRYPTION_KEY: dGVhbWJvYXJkLWRldi1lbmNyeXB0aW9uLWtleS0zMmI=  # base64-32
    depends_on:
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8001/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.auth.rule=PathPrefix(`/api/v1/auth`) || PathPrefix(`/.well-known/jwks.json`)
      - traefik.http.services.auth.loadbalancer.server.port=8001
    networks:
      - teamboard-net

  project:
    build:
      context: ./services/domain/project
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: project-service
      SERVICE_PORT: 8002
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/project_db?sslmode=disable
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      RABBITMQ_CONSUMER_QUEUE: project-service-queue
      REDIS_URL: redis://redis:6379/1
      PERMISSION_CACHE_TTL: 60s
      PERMISSION_CACHE_ENABLED: "true"
      JWT_JWKS_URL: http://auth:8001/.well-known/jwks.json
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      INTERNAL_AUDIENCE: internal
      SERVICE_TOKEN_SECRET: shared-internal-secret-do-not-use-in-prod
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: project-service
    depends_on:
      auth:
        condition: service_healthy
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8002/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.project.rule=PathPrefix(`/api/v1/projects`) || PathPrefix(`/api/v1/boards`)
      - traefik.http.services.project.loadbalancer.server.port=8002
    networks:
      - teamboard-net

  task:
    build:
      context: ./services/domain/task
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: task-service
      SERVICE_PORT: 8003
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/task_db?sslmode=disable
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      RABBITMQ_CONSUMER_QUEUE: task-service-queue
      JWT_JWKS_URL: http://auth:8001/.well-known/jwks.json
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      PROJECT_SERVICE_URL: http://project:8002
      PROJECT_SERVICE_TIMEOUT: 200ms
      PERMISSION_CACHE_TTL: 30s
      DOCUMENT_SERVICE_URL: http://document:8004
      DOCUMENT_SERVICE_TIMEOUT: 500ms
      SERVICE_TOKEN_SECRET: shared-internal-secret-do-not-use-in-prod
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: task-service
    depends_on:
      project:
        condition: service_healthy
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8003/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.task.rule=PathPrefix(`/api/v1/tasks`) || PathPrefix(`/api/v1/comments`)
      - traefik.http.services.task.loadbalancer.server.port=8003
    networks:
      - teamboard-net

  document:
    build:
      context: ./services/domain/document
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: document-service
      SERVICE_PORT: 8004
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/document_db?sslmode=disable
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      RABBITMQ_CONSUMER_QUEUE: document-service-queue
      STORAGE_PROVIDER: minio
      STORAGE_BUCKET: teamboard-documents
      STORAGE_REGION: us-east-1
      STORAGE_ENDPOINT: http://minio:9000
      STORAGE_ACCESS_KEY: teamboard
      STORAGE_SECRET_KEY: teamboard-secret
      STORAGE_USE_PATH_STYLE: "true"
      STORAGE_PUBLIC_ENDPOINT: http://localhost:9000
      MAX_FILE_SIZE_BYTES: 104857600
      ALLOWED_CONTENT_TYPES: "application/pdf,image/png,image/jpeg,image/gif,application/zip,text/plain,text/markdown,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.openxmlformats-officedocument.presentationml.presentation"
      UPLOAD_URL_TTL: 15m
      DOWNLOAD_URL_TTL: 5m
      CLEANUP_INTERVAL: 15m
      PENDING_VERSION_TIMEOUT: 1h
      DELETED_DOCUMENT_RETENTION: 720h
      JWT_JWKS_URL: http://auth:8001/.well-known/jwks.json
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      PROJECT_SERVICE_URL: http://project:8002
      PROJECT_SERVICE_TIMEOUT: 200ms
      PERMISSION_CACHE_TTL: 30s
      SERVICE_TOKEN_SECRET: shared-internal-secret-do-not-use-in-prod
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: document-service
    depends_on:
      project:
        condition: service_healthy
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      minio:
        condition: service_healthy
      minio-init:
        condition: service_completed_successfully
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8004/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.document.rule=PathPrefix(`/api/v1/documents`) || (PathPrefix(`/api/v1/projects`) && PathRegexp(`/api/v1/projects/[^/]+/documents`))
      - traefik.http.services.document.loadbalancer.server.port=8004
    networks:
      - teamboard-net

  notification:
    build:
      context: ./services/domain/notification
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: notification-service
      SERVICE_PORT: 8005
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/notification_db?sslmode=disable
      REDIS_URL: redis://redis:6379/2
      REDIS_CONNECTION_TTL: 60s
      REDIS_HEARTBEAT_INTERVAL: 30s
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      RABBITMQ_CONSUMER_QUEUE: notification-service-queue
      RABBITMQ_BINDING_KEYS: "task.*,project.*,board.*,document.*,user.*"
      WS_PATH: /ws
      WS_READ_TIMEOUT: 60s
      WS_WRITE_TIMEOUT: 10s
      WS_PING_INTERVAL: 30s
      WS_MAX_MESSAGE_SIZE: 8192
      WS_MAX_SUBSCRIPTIONS_PER_CONNECTION: 50
      WS_MAX_CONNECTIONS_PER_USER: 10
      JWT_JWKS_URL: http://auth:8001/.well-known/jwks.json
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      PROJECT_SERVICE_URL: http://project:8002
      PROJECT_SERVICE_TIMEOUT: 200ms
      PERMISSION_CACHE_TTL: 30s
      SERVICE_TOKEN_SECRET: shared-internal-secret-do-not-use-in-prod
      NOTIFICATION_RETENTION_DAYS: 90
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: notification-service
    depends_on:
      project:
        condition: service_healthy
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8005/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.notification.rule=PathPrefix(`/api/v1/notifications`) || PathPrefix(`/ws`)
      - traefik.http.services.notification.loadbalancer.server.port=8005
    networks:
      - teamboard-net

  plugin:
    build:
      context: ./services/domain/plugin
      dockerfile: Dockerfile
    environment:
      SERVICE_NAME: plugin-service
      SERVICE_PORT: 8006
      LOG_LEVEL: info
      DB_URL: postgres://teamboard:teamboard@postgres:5432/plugin_db?sslmode=disable
      RABBITMQ_URL: amqp://teamboard:teamboard@rabbitmq:5672/
      RABBITMQ_EXCHANGE: teamboard.events
      RABBITMQ_CONSUMER_QUEUE: plugin-service-queue
      RABBITMQ_BINDING_KEYS: "task.*,project.*,board.*,document.*,user.deleted"
      DELIVERY_WORKER_COUNT: 3
      DELIVERY_HTTP_CONNECT_TIMEOUT: 5s
      DELIVERY_HTTP_TOTAL_TIMEOUT: 30s
      DELIVERY_RESPONSE_BODY_LIMIT_BYTES: 1048576
      MAX_DELIVERY_ATTEMPTS: 8
      WEBHOOK_USER_AGENT: "TeamBoard-Webhook/1.0"
      ALLOW_INSECURE_HTTP: "true"   # DEV
      ALLOW_PRIVATE_URLS: "true"    # DEV: lokale Mock-Receiver wie webhook.site oder localhost
      WEBHOOK_ALLOWED_PORTS: "80,443,8080,8443,3000"
      BREAKER_CONSECUTIVE_FAILURES: 5
      BREAKER_TIMEOUT: 30s
      JWT_JWKS_URL: http://auth:8001/.well-known/jwks.json
      JWT_ISSUER: https://auth.teamboard.local
      JWT_AUDIENCE: teamboard-api
      PROJECT_SERVICE_URL: http://project:8002
      PROJECT_SERVICE_TIMEOUT: 200ms
      PERMISSION_CACHE_TTL: 30s
      SERVICE_TOKEN_SECRET: shared-internal-secret-do-not-use-in-prod
      DELIVERY_RETENTION_DAYS: 30
      OTEL_EXPORTER_OTLP_ENDPOINT: http://jaeger:4317
      OTEL_SERVICE_NAME: plugin-service
    depends_on:
      project:
        condition: service_healthy
      postgres:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8006/health/ready"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    labels:
      - traefik.enable=true
      - traefik.http.routers.plugin.rule=PathPrefix(`/api/v1/webhooks`) || (PathPrefix(`/api/v1/projects`) && PathRegexp(`/api/v1/projects/[^/]+/webhooks`))
      - traefik.http.services.plugin.loadbalancer.server.port=8006
    networks:
      - teamboard-net

  frontend:
    build:
      context: ./frontend
      dockerfile: Dockerfile
    environment:
      VITE_API_BASE_URL: http://localhost
      VITE_WS_URL: ws://localhost/ws
    ports:
      - "3000:3000"
    depends_on:
      - traefik
    networks:
      - teamboard-net
```

---

## 5. docker-compose.override.yml (Hot-Reload)

`docker-compose.override.yml` wird automatisch von Compose geladen und überlädt das Base-Setup für lokale Entwicklung:

```yaml
services:
  auth:
    build:
      target: dev   # Multi-stage Dockerfile mit dev-Stage
    volumes:
      - ./services/domain/auth:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  project:
    build:
      target: dev
    volumes:
      - ./services/domain/project:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  task:
    build:
      target: dev
    volumes:
      - ./services/domain/task:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  document:
    build:
      target: dev
    volumes:
      - ./services/domain/document:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  notification:
    build:
      target: dev
    volumes:
      - ./services/domain/notification:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  plugin:
    build:
      target: dev
    volumes:
      - ./services/domain/plugin:/app
      - go-mod-cache:/go/pkg/mod
    command: air -c .air.toml

  frontend:
    volumes:
      - ./frontend:/app
      - /app/node_modules
    command: npm run dev

volumes:
  go-mod-cache:
```

**Multi-stage Dockerfile** (Beispiel `services/domain/auth/Dockerfile`):

```dockerfile
# ---- Stage 1: Build ----
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /bin/auth ./cmd/server

# ---- Stage 2: Dev (mit air) ----
FROM golang:1.22-alpine AS dev
RUN go install github.com/cosmtrek/air@v1.49.0
WORKDIR /app
EXPOSE 8001

# ---- Stage 3: Production ----
FROM gcr.io/distroless/static-debian12:nonroot AS production
COPY --from=build /bin/auth /bin/auth
USER nonroot:nonroot
EXPOSE 8001
ENTRYPOINT ["/bin/auth"]
```

Im Compose Base wird `target: production` gewählt, im Override `target: dev`.

---

## 6. Service-Konfiguration im Detail

### 6.1 Healthcheck-Strategie

Jeder Service exponiert `/health/ready`. Healthcheck im Compose ruft diesen Endpoint:

```yaml
healthcheck:
  test: ["CMD", "wget", "-q", "-O-", "http://localhost:8001/health/ready"]
  interval: 10s
  timeout: 5s
  retries: 5
  start_period: 20s   # Migrations können dauern
```

`/health/ready` prüft:
- DB erreichbar (kurzer SELECT 1)
- RabbitMQ-Verbindung steht
- (für Notification:) Redis erreichbar

`/health/live` immer 200, prüft nur ob Prozess läuft.

### 6.2 Reihenfolge des Service-Starts (depends_on)

```
postgres (healthy)
  ├── auth
  │     └── project
  │           ├── task
  │           ├── document
  │           ├── notification
  │           └── plugin
  └── rabbitmq, redis, minio (parallel)
        └── (alle Services warten zusätzlich auf diese)
```

### 6.3 Migrations beim Start

Jeder Service migriert seine DB beim Container-Start als ersten Schritt im Entrypoint:

```dockerfile
# Im Production-Stage Dockerfile
COPY --from=build /bin/migrate /bin/migrate
COPY --from=build /app/migrations /migrations

# Entrypoint-Script
COPY ./entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
```

`entrypoint.sh`:
```bash
#!/bin/sh
set -e
echo "Running migrations..."
/bin/migrate -path /migrations -database "$DB_URL" up
echo "Starting service..."
exec /bin/auth
```

---

## 7. Initialisierungs-Container

### 7.1 postgres-init: Service-DBs erstellen

`scripts/postgres-init.sh` wird beim ersten Postgres-Start ausgeführt (über Bind-Mount in `/docker-entrypoint-initdb.d`):

```bash
#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE project_db;
    CREATE DATABASE task_db;
    CREATE DATABASE document_db;
    CREATE DATABASE notification_db;
    CREATE DATABASE plugin_db;
    CREATE DATABASE boardregistry_db;
EOSQL

echo "All service databases created."
```

Wichtig: Wird nur beim *ersten* Postgres-Start ausgeführt (sobald `postgres-data`-Volume nicht leer ist, wird das Init-Script übersprungen).

### 7.2 minio-init: Bucket und CORS

Inline im Compose-File definiert (siehe Abschnitt 4). Läuft einmalig durch und exited.

`restart: "no"` sorgt dafür, dass es nicht nach jedem Erfolg neu startet.

---

## 8. Makefile (Root)

```makefile
.PHONY: help up up-build down restart logs ps clean clean-volumes \
        seed migrate generate test test-unit test-integration test-shared \
        lint lint-fix build build-services build-frontend \
        shell-postgres shell-redis shell-rabbit \
        urls

# Default target — `make` zeigt Hilfe
help:
	@echo "TeamBoard — Make Targets"
	@echo ""
	@echo "Lifecycle:"
	@echo "  make up                Start all services (with cached images)"
	@echo "  make up-build          Rebuild and start all services"
	@echo "  make down              Stop all services (keeps data)"
	@echo "  make restart           Restart services (down + up)"
	@echo "  make clean             Remove containers and networks"
	@echo "  make clean-volumes     Remove containers, networks, AND volumes (data loss!)"
	@echo ""
	@echo "Operations:"
	@echo "  make logs              Tail logs from all services"
	@echo "  make logs-<service>    Tail logs from one service (e.g., logs-task)"
	@echo "  make ps                Show running containers"
	@echo "  make urls              List all URLs to access services"
	@echo ""
	@echo "Development:"
	@echo "  make seed              Insert seed data (users, projects, tasks)"
	@echo "  make migrate           Run all migrations"
	@echo "  make generate          Run sqlc generate and oapi-codegen for all services"
	@echo "  make test              Run all tests"
	@echo "  make test-unit         Run unit tests"
	@echo "  make test-integration  Run integration tests"
	@echo "  make test-shared       Run shared library tests"
	@echo "  make lint              Run golangci-lint on all services"
	@echo "  make lint-fix          Run linters with --fix"
	@echo ""
	@echo "Build:"
	@echo "  make build             Build all images"
	@echo "  make build-services    Build only backend service images"
	@echo "  make build-frontend    Build only frontend image"
	@echo ""
	@echo "Shells:"
	@echo "  make shell-postgres    psql shell"
	@echo "  make shell-redis       redis-cli"
	@echo "  make shell-rabbit      rabbitmq-admin"

# ============================================================
# LIFECYCLE
# ============================================================

up:
	docker compose up -d
	@echo ""
	@echo "Waiting for services to become healthy..."
	@$(MAKE) -s wait-healthy
	@$(MAKE) -s urls

up-build:
	docker compose up -d --build
	@$(MAKE) -s wait-healthy
	@$(MAKE) -s urls

down:
	docker compose down

restart: down up

clean:
	docker compose down --remove-orphans

clean-volumes:
	@echo "WARNING: This will delete all data (databases, MinIO, etc)."
	@read -p "Continue? [y/N] " confirm; \
	if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
		docker compose down -v --remove-orphans; \
	else \
		echo "Aborted."; \
	fi

wait-healthy:
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12; do \
		unhealthy=$$(docker compose ps --format json | jq -r 'select(.Health != null and .Health != "healthy") | .Service' 2>/dev/null | wc -l); \
		if [ "$$unhealthy" = "0" ]; then \
			echo "All services healthy."; \
			exit 0; \
		fi; \
		echo "Waiting... ($$i/12)"; \
		sleep 5; \
	done; \
	echo "Some services not healthy after 60s. Check 'make ps' and 'make logs'."; \
	exit 1

# ============================================================
# OPERATIONS
# ============================================================

logs:
	docker compose logs -f --tail=100

logs-%:
	docker compose logs -f --tail=200 $*

ps:
	docker compose ps

urls:
	@echo ""
	@echo "TeamBoard is running:"
	@echo "  Frontend:           http://localhost:3000"
	@echo "  API (via Gateway):  http://localhost"
	@echo ""
	@echo "Tools:"
	@echo "  Traefik Dashboard:  http://localhost:8080"
	@echo "  RabbitMQ UI:        http://localhost:15672  (teamboard / teamboard)"
	@echo "  MinIO Console:      http://localhost:9001   (teamboard / teamboard-secret)"
	@echo "  MailHog:            http://localhost:8025"
	@echo "  Jaeger UI:          http://localhost:16686"

# ============================================================
# DEVELOPMENT
# ============================================================

SERVICES := auth project task document notification plugin boardregistry

migrate:
	@for svc in $(SERVICES); do \
		echo "Migrating $$svc..."; \
		docker compose exec -T $$svc /bin/migrate -path /migrations \
			-database "$$DB_URL" up || true; \
	done

generate:
	@for svc in $(SERVICES); do \
		echo "Generating code for $$svc..."; \
		(cd services/$$svc && sqlc generate); \
		(cd services/$$svc && oapi-codegen -package=api -generate=types,chi-server \
			../../docs/api/$$svc.openapi.yaml > internal/api/generated.go 2>/dev/null || true); \
	done
	@echo "Generated."

seed:
	@echo "Seeding test data..."
	@./scripts/seed.sh

test: test-shared test-unit test-integration

test-unit:
	@for svc in $(SERVICES); do \
		echo "=== Testing $$svc (unit) ==="; \
		(cd services/$$svc && go test -short -race -cover ./...); \
	done

test-integration:
	@for svc in $(SERVICES); do \
		echo "=== Testing $$svc (integration) ==="; \
		(cd services/$$svc && go test -race ./internal/repository/... ./internal/events/...); \
	done

test-shared:
	@echo "=== Testing shared/go ==="
	(cd shared/go && go test -race -coverprofile=coverage.out ./...)
	(cd shared/go && go tool cover -func=coverage.out | tail -1)

lint:
	@for svc in $(SERVICES); do \
		echo "=== Linting $$svc ==="; \
		(cd services/$$svc && golangci-lint run); \
	done
	(cd shared/go && golangci-lint run)

lint-fix:
	@for svc in $(SERVICES); do \
		(cd services/$$svc && golangci-lint run --fix); \
	done
	(cd shared/go && golangci-lint run --fix)

# ============================================================
# BUILD
# ============================================================

build: build-services build-frontend

build-services:
	docker compose build $(SERVICES)

build-frontend:
	docker compose build frontend

# ============================================================
# SHELLS
# ============================================================

shell-postgres:
	docker compose exec postgres psql -U teamboard

shell-redis:
	docker compose exec redis redis-cli

shell-rabbit:
	docker compose exec rabbitmq rabbitmqctl
```

---

## 9. Seed-Daten

`scripts/seed.sh`:

```bash
#!/bin/bash
set -e

API="http://localhost"

echo "Creating users..."

ALICE=$(curl -s -X POST $API/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"email":"alice@teamboard.local","password":"AliceSecret123!"}' \
    | jq -r '.data.id')
echo "  alice: $ALICE"

BOB=$(curl -s -X POST $API/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"email":"bob@teamboard.local","password":"BobSecret123!"}' \
    | jq -r '.data.id')
echo "  bob: $BOB"

echo ""
echo "Logging in as Alice..."
ALICE_TOKEN=$(curl -s -X POST $API/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"alice@teamboard.local","password":"AliceSecret123!"}' \
    | jq -r '.access_token')

echo "Creating project 'Demo Project'..."
PROJ=$(curl -s -X POST $API/api/v1/projects \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -d '{"name":"Demo Project","description":"Created by seed script"}' \
    | jq -r '.data.id')
echo "  project: $PROJ"

echo "Adding Bob as editor..."
curl -s -X POST $API/api/v1/projects/$PROJ/members \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -d "{\"user_id\":\"$BOB\",\"role\":\"editor\"}" > /dev/null

echo "Creating Kanban board..."
BOARD=$(curl -s -X POST $API/api/v1/projects/$PROJ/boards \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -d '{"name":"Sprint 1","type":"kanban"}' \
    | jq -r '.data.id')
echo "  board: $BOARD"

# Get column IDs
COLUMNS=$(curl -s $API/api/v1/boards/$BOARD/columns \
    -H "Authorization: Bearer $ALICE_TOKEN")
TODO_COL=$(echo $COLUMNS | jq -r '.data[0].id')
IN_PROGRESS_COL=$(echo $COLUMNS | jq -r '.data[1].id')

echo "Creating tasks..."
for i in 1 2 3 4 5; do
    curl -s -X POST $API/api/v1/boards/$BOARD/tasks \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $ALICE_TOKEN" \
        -d "{\"title\":\"Task $i\",\"column_id\":\"$TODO_COL\",\"priority\":\"medium\"}" \
        > /dev/null
done

echo ""
echo "Seed complete."
echo "  Login: alice@teamboard.local / AliceSecret123!"
echo "  Login: bob@teamboard.local / BobSecret123!"
echo "  Project: $PROJ"
echo "  Board:   $BOARD"
```

---

## 10. Ports und Zugriff

### 10.1 Browser-Zugriff (von Host)

| URL | Zweck |
|-----|-------|
| http://localhost | API über Gateway (für Frontend und CLI) |
| http://localhost:3000 | Frontend SPA |
| http://localhost:8080 | Traefik Dashboard |
| http://localhost:9001 | MinIO Console (Login: `teamboard` / `teamboard-secret`) |
| http://localhost:15672 | RabbitMQ Management (Login: `teamboard` / `teamboard`) |
| http://localhost:8025 | MailHog (gefangene Mails) |
| http://localhost:16686 | Jaeger UI |

### 10.2 CLI-Zugriff

```bash
# Postgres
psql -h localhost -U teamboard -d auth_db

# Redis
redis-cli -h localhost

# MinIO via mc
mc alias set local http://localhost:9000 teamboard teamboard-secret
mc ls local/teamboard-documents

# RabbitMQ
docker compose exec rabbitmq rabbitmqctl list_queues
```

### 10.3 API-Test mit curl

```bash
# Register
curl -X POST http://localhost/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"strongPassword123!"}'

# Login
TOKEN=$(curl -s -X POST http://localhost/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"strongPassword123!"}' \
    | jq -r '.access_token')

# Authenticated call
curl http://localhost/api/v1/projects -H "Authorization: Bearer $TOKEN"
```

---

## 11. Troubleshooting

### 11.1 "Service starts and immediately exits"

```bash
make logs-<service>
```

Häufigste Ursachen:
- DB-URL falsch — Service-DB existiert nicht (`postgres-init` lief nicht)
- Migrations fehlgeschlagen — typisch wenn Schema breaking changed wurde

**Fix:** `make clean-volumes && make up` (Datenverlust!)

### 11.2 "Port already in use"

```bash
# Welcher Prozess hält Port 5432?
lsof -i :5432
# Killen oder Compose-Port ändern
```

### 11.3 "Cannot connect to MinIO from browser"

CORS nicht gesetzt. `make up` erneut ausführen — `minio-init` läuft dann erneut.

### 11.4 Healthcheck schlägt fehl, Service läuft aber

`wget` ist im Distroless-Image möglicherweise nicht verfügbar. Healthcheck-Test ändern auf:
```yaml
test: ["CMD-SHELL", "/bin/auth -healthcheck || exit 1"]
```
oder Service exposed `/health/ready` und ist von außen erreichbar — `curl` vom Host:
```bash
curl http://localhost:8001/health/ready
```

### 11.5 Reset auf sauberen Zustand

```bash
make clean-volumes
make up
make seed
```

---

## 12. Implementierungs-Hinweise

### 12.1 Dateistruktur im Repo

```
teamboard/
├── docker-compose.yml
├── docker-compose.override.yml      # für lokale Entwicklung
├── docker-compose.prod.yml          # CI/Prod-Variante (optional, mit anderen Tags)
├── Makefile
├── .env.example                     # Template für lokale Overrides
├── scripts/
│   ├── postgres-init.sh             # bind-mounted in postgres container
│   ├── seed.sh
│   └── wait-healthy.sh
├── infra/
│   └── traefik/
│       └── traefik.yml              # statische Config (siehe gateway.md)
├── services/
│   ├── auth/Dockerfile
│   ├── project/Dockerfile
│   └── ...
├── shared/
│   └── go/...
└── frontend/
    └── Dockerfile
```

### 12.2 .env-File

`.env.example` als Template. Lokale Overrides via `.env`:

```bash
# .env.example
COMPOSE_PROJECT_NAME=teamboard
LOG_LEVEL=info
```

`.env` ist in `.gitignore`.

### 12.3 First-Run-Ablauf

**Neuer Entwickler, zero-state:**

```bash
git clone <repo>
cd teamboard
make up         # ~3 min (Image-Builds)
make seed       # 5 sek
make urls       # zeigt URLs
```

Browser öffnen: http://localhost:3000 → Login mit `alice@teamboard.local` / `AliceSecret123!`.

### 12.4 CI-Variante

CI nutzt nur `docker-compose.yml`, **nicht** Override (`-f` explicit):

```yaml
- name: Start services
  run: docker compose -f docker-compose.yml up -d --wait

- name: Run E2E tests
  run: docker compose exec -T frontend npm run test:e2e
```

### 12.5 Resource-Limits (optional)

Für Demo auf schwachem Laptop:

```yaml
services:
  task:
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M
```

Im MVP nicht zwingend.

---

**Ende der Orchestrierungs-Spezifikation.**
