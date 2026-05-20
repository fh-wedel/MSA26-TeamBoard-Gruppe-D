# Implementation Plan — Plugin Architecture PoC

> **Status:** Draft v1.0  
> **Ziel:** Proof of Concept für eine modulare, cloud-native Plugin-Architektur auf AWS — lokal entwickelbar, vollständig per IaC deploybar, kein manuelles Klicken in der AWS Console.

---

## 1. Was wir beweisen wollen

| Ziel | Beschreibung |
|---|---|
| **Modulare Plugin-Architektur** | Plugins registrieren sich zur Laufzeit (Registry Pattern), ohne dass der Core neu deployed werden muss |
| **Plugin-Komponenten via AWS Services** | Jedes Plugin läuft als isolierter AWS-Service (Lambda, ECS Task, etc.) |
| **Persistenzschicht** | Zustandsbehaftete Daten überleben Plugin-Restarts und sind plugin-übergreifend zugreifbar |
| **Echtzeit-Support** | Events (z.B. Kanban-Ticket verschoben) propagieren sich in <500ms an alle verbundenen Clients |
| **Message Broker** | Asynchrone Entkopplung zwischen Plugins und Core über einen zentralen Event-Bus |

---

## 2. Architektur-Entscheidungen & Begründungen

### 2.1 Plugin-Laufzeit: Lambda vs. ECS

Wir verwenden **beide** — je nach Plugin-Typ:

| Plugin-Typ | Service | Warum |
|---|---|---|
| Kurzlebige Aktionen (HTTP-Handler, Webhooks) | **AWS Lambda** | Cold-Start <1s, kein Idle-Cost, perfekt für event-getriggerte Plugins |
| Lang laufende / stateful Plugins (z.B. WebSocket-Server) | **AWS ECS Fargate** | Permanenter Prozess, eigene VPC-Isolation, skalierbar |
| Plugin-Registry & Core API | **ECS Fargate** | Muss dauerhaft erreichbar sein; Lambda wäre zu komplex für Stateverwaltung |

**Warum nicht nur Lambda?** Die Plugin-Registry muss persistent sein und WebSocket-Verbindungen halten — Lambda-Timeouts (max. 15 Min.) sind dafür ungeeignet.

**Warum nicht nur ECS?** Lambda-Plugins starten on-demand, verursachen keine Kosten bei Inaktivität und können völlig unabhängig deployed werden.

### 2.2 Message Broker: Amazon MSK (Managed Kafka) vs. Amazon MQ (RabbitMQ)

**Entscheidung: Amazon MSK (Kafka)**

| Kriterium | MSK/Kafka | Amazon MQ/RabbitMQ |
|---|---|---|
| Event Replay | ✅ Retention konfigurierbar | ❌ Nachrichten weg nach Consume |
| Throughput | ✅ Sehr hoch, partitionierbar | ✅ Ausreichend für PoC |
| Fan-out (1 Event → N Plugins) | ✅ Consumer Groups nativ | ⚠️ Exchange-Konfiguration nötig |
| AWS-native Managed | ✅ MSK Serverless | ✅ Amazon MQ |
| **PoC-Komplexität** | ⚠️ Etwas höher | ✅ Einfacher |

**Für den PoC:** Wir starten mit **Amazon EventBridge** als leichtgewichtigen Event-Bus (kein Cluster-Management, native AWS-Integration, genug für Fan-out-Szenarien) und halten die Kafka-Option offen. EventBridge kann später 1:1 durch MSK ersetzt werden — die Producer/Consumer-Interfaces bleiben abstrakt.

> **Upgrade-Pfad:** EventBridge → MSK Serverless → MSK Provisioned (wenn wir Replay oder höheren Throughput brauchen)

### 2.3 Echtzeit: WebSockets

**AWS API Gateway WebSocket API** in Kombination mit einer **ECS Fargate Connection-Manager-Komponente**.

- API Gateway hält WebSocket-Verbindungen
- Connection-IDs werden in **DynamoDB** gespeichert (TTL-basiert, kein Aufräumen nötig)
- EventBridge-Events triggern eine Lambda, die über API Gateway `@connections` an alle aktiven Clients pusht

### 2.4 Persistenz: Zwei Schichten

| Schicht | Service | Verwendung |
|---|---|---|
| **Primary Store** | Amazon RDS (PostgreSQL, Aurora Serverless v2) | Strukturierte Plugin-Daten, Konfiguration, Audit-Log |
| **Cache / Session** | Amazon ElastiCache (Redis) | Plugin-Registry-State, Rate-Limiting, Pub/Sub für interne Events |
| **Connection State** | Amazon DynamoDB | WebSocket Connection-IDs (TTL = Session-Ende) |
| **Blob/Assets** | Amazon S3 | Plugin-Bundles, Uploads |

### 2.5 IaC: AWS CloudFormation + CDK

**Entscheidung: AWS CDK (TypeScript)** — generiert CloudFormation, aber mit echter Programmiersprache:

- Keine YAML-Hölle
- Reuse von Constructs über Plugins hinweg
- CDK Stacks pro Plugin → unabhängig deploybar
- `cdk synth` erzeugt CloudFormation für Review/Audit

### 2.6 CI/CD: GitHub Actions

- Branch `main` → Deploy nach `staging`
- Tags `v*.*.*` → Deploy nach `production`
- Jedes Plugin hat seinen eigenen Workflow (kann unabhängig deployed werden)
- Shared Workflows als reusable Actions

---

## 3. System-Übersicht

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client (Browser)                         │
│              REST / WebSocket / SSE                              │
└──────────────────┬───────────────────────┬──────────────────────┘
                   │                       │
         ┌─────────▼──────────┐   ┌────────▼────────────┐
         │  API Gateway (REST) │   │ API Gateway (WS)    │
         └─────────┬──────────┘   └────────┬────────────┘
                   │                       │
         ┌─────────▼──────────────────────▼─────────────┐
         │              Core API (ECS Fargate)            │
         │   - Plugin Registry (Runtime-Registration)     │
         │   - Auth / Routing                             │
         │   - Plugin Proxy                               │
         └──────┬──────────────────────┬─────────────────┘
                │                      │
    ┌───────────▼───────┐    ┌─────────▼──────────────┐
    │   EventBridge      │    │   Redis (ElastiCache)   │
    │   (Event Bus)      │    │   Plugin Registry Cache │
    └────────┬───────────┘    └────────────────────────┘
             │
    ┌────────┴──────────────────────────────────┐
    │           Plugin-Layer                     │
    │                                            │
    │  ┌──────────────┐  ┌──────────────────┐   │
    │  │ Plugin A     │  │ Plugin B          │   │
    │  │ (Lambda)     │  │ (ECS Fargate)     │   │
    │  └──────┬───────┘  └──────────┬───────┘   │
    └─────────┼─────────────────────┼───────────┘
              │                     │
    ┌─────────▼─────────────────────▼───────────┐
    │              Persistenz-Layer              │
    │  RDS PostgreSQL │ DynamoDB │ S3            │
    └────────────────────────────────────────────┘
```

---

## 4. Plugin-Registry Pattern (Detail)

### Konzept: Self-Registration on Startup

Jedes Plugin registriert sich beim Start selbst beim Core:

```
Plugin startet (Lambda cold start oder ECS Task start)
    → POST /internal/registry/register
        {
          "pluginId": "kanban-board",
          "version": "1.2.0",
          "capabilities": ["read", "write", "realtime"],
          "endpoint": "https://kanban.internal.example.com",
          "eventSubscriptions": ["ticket.moved", "ticket.assigned"],
          "healthCheck": "/health"
        }
    → Core speichert in Redis (TTL: 30s)
    → Plugin sendet Heartbeat alle 10s
    → Kein Heartbeat → automatisch deregistriert
```

### Registry-API (Core)

| Endpoint | Methode | Beschreibung |
|---|---|---|
| `/internal/registry/register` | POST | Plugin registriert sich |
| `/internal/registry/heartbeat/:pluginId` | PUT | Liveness-Signal |
| `/internal/registry/deregister/:pluginId` | DELETE | Graceful shutdown |
| `/api/plugins` | GET | Öffentliche Plugin-Liste (für UI) |
| `/api/plugins/:pluginId/proxy/*` | ALL | Transparent zu Plugin weiterleiten |

---

## 5. Echtzeit-Flow (Kanban-Beispiel)

```
1. User zieht Ticket in neue Spalte
   → PUT /api/plugins/kanban/tickets/123
   → Kanban-Plugin updated DB

2. Kanban-Plugin published Event:
   EventBridge Event {
     source: "plugin.kanban",
     detail-type: "ticket.moved",
     detail: { ticketId: 123, from: "todo", to: "in-progress", boardId: 42 }
   }

3. EventBridge → Lambda (WebSocket Broadcaster)
   → Liest Connection-IDs aus DynamoDB (gefiltert nach boardId)
   → POST API Gateway /@connections/{connectionId}
   → Jeder verbundene Client empfängt das Event

4. Client-UI updated Kanban-Board ohne Reload
```

---

## 6. Repository-Struktur

```
/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                    # Lint, Test, Build (alle PRs)
│   │   ├── deploy-staging.yml        # Push auf main
│   │   ├── deploy-production.yml     # Tag v*.*.*
│   │   └── plugin-kanban.yml        # Plugin-spezifischer Deploy
│   └── actions/
│       └── deploy-cdk-stack/        # Reusable Action
│
├── infrastructure/                   # CDK Root
│   ├── bin/app.ts                   # CDK App Entry
│   ├── lib/
│   │   ├── stacks/
│   │   │   ├── network-stack.ts     # VPC, Subnets, Security Groups
│   │   │   ├── core-stack.ts        # ECS Core, API Gateway, EventBridge
│   │   │   ├── persistence-stack.ts # RDS, ElastiCache, DynamoDB, S3
│   │   │   ├── realtime-stack.ts    # WebSocket API Gateway, Broadcaster Lambda
│   │   │   └── plugins/
│   │   │       └── kanban-stack.ts  # Plugin-spezifischer Stack
│   │   └── constructs/
│   │       ├── plugin-lambda.ts     # Reusable: Lambda Plugin
│   │       └── plugin-ecs.ts        # Reusable: ECS Plugin
│   └── cdk.json
│
├── services/
│   ├── core/                         # Core API (Node.js/TypeScript)
│   │   ├── src/
│   │   │   ├── registry/            # Plugin Registry Logic
│   │   │   ├── proxy/               # Plugin Request Proxying
│   │   │   ├── realtime/            # WebSocket Connection Manager
│   │   │   └── api/                 # REST Endpoints
│   │   ├── Dockerfile
│   │   └── package.json
│   │
│   └── plugins/
│       └── kanban/                   # Beispiel-Plugin (ECS)
│           ├── src/
│           │   ├── index.ts         # Plugin Entry + Self-Registration
│           │   ├── routes/
│           │   └── events/          # EventBridge Publisher
│           ├── Dockerfile
│           └── package.json
│
├── lambdas/
│   └── ws-broadcaster/              # WebSocket Fan-out Lambda
│       ├── handler.ts
│       └── package.json
│
├── docker-compose.yml               # Lokale Entwicklung
├── docker-compose.override.yml      # Lokale Overrides (kein Commit nötig)
├── .env.example
└── README.md
```

---

## 7. Lokale Entwicklung

Alles läuft lokal via **Docker Compose** — kein AWS-Account nötig für den Dev-Loop.

### Lokale Service-Ersetzungen

| AWS Service | Lokal |
|---|---|
| API Gateway | Kong Gateway oder direktes Express-Routing |
| EventBridge | LocalStack oder Redis Pub/Sub (leichtgewichtiger) |
| RDS PostgreSQL | `postgres:16` Docker Image |
| ElastiCache Redis | `redis:7` Docker Image |
| DynamoDB | DynamoDB Local (`amazon/dynamodb-local`) |
| S3 | MinIO |
| ECS | Direkt als Docker Services |
| Lambda | AWS SAM CLI oder einfach als Express-Endpoint |

### `docker-compose.yml` (Auszug)

```yaml
services:
  core:
    build: ./services/core
    ports: ["3000:3000"]
    environment:
      - REDIS_URL=redis://redis:6379
      - DATABASE_URL=postgresql://postgres:postgres@postgres:5432/poc
      - EVENT_BUS=redis  # lokal: redis pub/sub statt EventBridge
    depends_on: [postgres, redis]

  plugin-kanban:
    build: ./services/plugins/kanban
    environment:
      - CORE_URL=http://core:3000
      - DATABASE_URL=postgresql://postgres:postgres@postgres:5432/poc

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: postgres

  redis:
    image: redis:7-alpine

  dynamodb-local:
    image: amazon/dynamodb-local
    ports: ["8000:8000"]

  minio:
    image: minio/minio
    command: server /data
    ports: ["9000:9000", "9001:9001"]
```

### Lokaler Dev-Start

```bash
cp .env.example .env
docker compose up --build

# Core API:       http://localhost:3000
# Kanban Plugin:  http://localhost:3001
# DynamoDB Local: http://localhost:8000
# MinIO Console:  http://localhost:9001
```

---

## 8. Infrastructure as Code (CDK)

### Stack-Dependencies

```
NetworkStack
    └── PersistenceStack (RDS, Redis, DynamoDB, S3)
            └── CoreStack (ECS Core API, API Gateway REST + WS, EventBridge)
                    └── RealtimeStack (Broadcaster Lambda)
                    └── PluginStacks/* (je Plugin ein Stack)
```

### Deploy-Befehle

```bash
# Erstmalig (Bootstrap AWS Account für CDK)
npx cdk bootstrap aws://ACCOUNT_ID/eu-central-1

# Alle Stacks deployen
npx cdk deploy --all

# Nur ein Plugin deployen (unabhängig!)
npx cdk deploy KanbanPluginStack

# Diff anzeigen vor Deploy
npx cdk diff
```

---

## 9. GitHub Actions CI/CD

### Pipeline-Übersicht

```
PR → ci.yml
    ├── Lint & Type-check (alle Services)
    ├── Unit Tests
    ├── CDK Synth (CloudFormation validieren)
    └── Docker Build (kein Push)

Push auf main → deploy-staging.yml
    ├── ci.yml (reuse)
    ├── Docker Images → ECR (staging tag)
    └── cdk deploy --all (staging)

Tag v*.*.* → deploy-production.yml
    ├── ci.yml (reuse)
    ├── Docker Images → ECR (version tag + latest)
    └── cdk deploy --all (production)
```

### Environments & Secrets (GitHub Secrets)

```
AWS_ROLE_ARN_STAGING       # OIDC Role für staging
AWS_ROLE_ARN_PRODUCTION    # OIDC Role für production
AWS_REGION                 # z.B. eu-central-1
```

> **Kein statischer AWS_ACCESS_KEY!** Wir nutzen **GitHub OIDC → AWS IAM Role** (best practice, kein Secret-Rotation-Problem).

### Beispiel `deploy-staging.yml`

```yaml
name: Deploy Staging
on:
  push:
    branches: [main]

permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN_STAGING }}
          aws-region: ${{ secrets.AWS_REGION }}

      - name: Build & Push Docker Images
        run: |
          aws ecr get-login-password | docker login --username AWS --password-stdin $ECR_REGISTRY
          docker build -t $ECR_REGISTRY/core:$GITHUB_SHA ./services/core
          docker push $ECR_REGISTRY/core:$GITHUB_SHA

      - name: CDK Deploy
        run: |
          npm ci --prefix infrastructure
          npx cdk deploy --all --require-approval never \
            -c imageTag=$GITHUB_SHA \
            -c environment=staging
```

---

## 10. PoC Demo-Szenario: Kanban Board

Das PoC demonstriert alle 5 Ziele anhand eines Kanban-Boards:

1. **Plugin-Architektur:** Kanban läuft als eigenständiger ECS Service, registriert sich beim Core-Start
2. **AWS Services:** Core = ECS, Kanban-Plugin = ECS Fargate, Broadcaster = Lambda
3. **Persistenz:** Tickets in RDS PostgreSQL, Board-State in Redis, Attachments in S3
4. **Echtzeit:** User A verschiebt Ticket → User B sieht es sofort (WebSocket)
5. **EventBridge:** `ticket.moved` Event → Broadcaster Lambda → alle Board-Member

**Demo-Flow:**
```
Browser Tab 1 (User A) + Browser Tab 2 (User B) öffnen dasselbe Board
→ User A verschiebt Ticket von "Todo" → "In Progress"
→ User B sieht die Änderung ohne Reload in <500ms
→ CloudWatch Logs zeigen den kompletten Event-Flow
```

---

## 11. Was noch fehlt / bewusst ausgeklammert

| Thema | Status | Begründung |
|---|---|---|
| Auth / JWT / Cognito | ⏳ Phase 2 | PoC fokussiert auf Architektur, nicht Auth |
| Multi-Tenancy | ⏳ Phase 2 | Tenant-Isolation ist eigenes Thema |
| Plugin Sandboxing / Permissions | ⏳ Phase 2 | IAM Roles pro Plugin sind vorbereitet, aber nicht voll umgesetzt |
| Monitoring / Alerting (CloudWatch, Alarms) | ⏳ Phase 2 | CDK-Grundstruktur ist vorbereitet |
| Kafka/MSK | ⏳ Upgrade-Pfad | EventBridge reicht für PoC; MSK wenn Replay/höherer Throughput nötig |
| Blue/Green Deployment | ⏳ Phase 2 | ECS unterstützt es nativ, CDK vorbereiten |
| Plugin Versioning / Rollback | ⏳ Phase 2 | Wichtig für Produktion, zu komplex für PoC |

---

## 12. Nächste Schritte (Prio-Reihenfolge)

- [ ] **Repo aufsetzen** — Struktur wie in Abschnitt 6 anlegen
- [ ] **CDK NetworkStack + PersistenceStack** — VPC, RDS, Redis, DynamoDB lokal testen
- [ ] **Core Service** — Plugin Registry (Express + Redis), `/internal/registry/*` Endpoints
- [ ] **docker-compose.yml** — Lokaler Dev-Stack lauffähig
- [ ] **Kanban Plugin** — Self-Registration, CRUD auf Tickets, EventBridge Publisher
- [ ] **WebSocket Broadcaster Lambda** — Connection-Manager + Fan-out
- [ ] **CDK CoreStack + RealtimeStack** — API Gateway (REST + WS), EventBridge Rule
- [ ] **GitHub Actions** — OIDC Setup, CI + Staging Deploy
- [ ] **End-to-End Demo** — Echtzeit-Kanban über zwei Browser-Tabs
- [ ] **README** — Setup-Dokumentation für neue Team-Member

---

## 13. Tech Stack Zusammenfassung

| Bereich | Technologie |
|---|---|
| Sprache | TypeScript (Node.js 20) |
| Core Framework | Express.js oder Fastify |
| IaC | AWS CDK (TypeScript) → CloudFormation |
| Container Registry | Amazon ECR |
| Container Runtime (Core, Plugins) | AWS ECS Fargate |
| Serverless (Broadcaster, kleine Plugins) | AWS Lambda |
| Event Bus | Amazon EventBridge (→ MSK Upgrade-Pfad) |
| WebSocket | API Gateway WebSocket API |
| Primary DB | Amazon RDS PostgreSQL (Aurora Serverless v2) |
| Cache / Registry State | Amazon ElastiCache (Redis) |
| Connection State | Amazon DynamoDB |
| Blob Storage | Amazon S3 |
| CI/CD | GitHub Actions + OIDC |
| Lokale Deps | Docker Compose, DynamoDB Local, MinIO, Redis |
| Monitoring | Amazon CloudWatch (vorbereitet) |
| Region | eu-central-1 (Frankfurt) |
