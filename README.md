# msa2 — Plugin Architecture PoC

A modular, cloud-native plugin architecture for AWS. Plugins register themselves
at runtime against a Core service, persist their own state, and communicate with
other plugins through a shared event bus. Locally everything runs in Docker
Compose; in AWS the same code is deployed via CDK.

See [`implementation_plan.md`](./implementation_plan.md) for the full design.

---

## Architecture (high level)

```
                Client (Browser)
                 REST     WebSocket
                  |         |
            API Gateway (REST / WS)
                  |         |
                  v         v
            Core API (ECS Fargate)
        +-- Plugin Registry (Redis) --+
        +-- Reverse Proxy (Fastify) --+
                |               |
                v               v
        EventBridge / Redis     |
                |               |
        +-------+-------+       |
        |       |       |       |
        v       v       v       v
   Plugin A  Plugin B  Plugin C
   (ECS or Lambda)
        |
        v
   RDS PostgreSQL / DynamoDB / S3
```

Realtime: plugins publish events (`ticket.moved`, ...). On AWS, an EventBridge
rule triggers the **ws-broadcaster** Lambda which fans out via the API Gateway
WebSocket `@connections` API. Locally, the broadcaster container subscribes to
Redis Pub/Sub and logs the deliveries (see smoke test below).

---

## Repository layout

```
.
├── services/
│   ├── core/                   Fastify + ioredis: plugin registry, proxy, event bus
│   └── plugins/
│       └── kanban/             Fastify + pg: ticket CRUD, event publisher
├── lambdas/
│   └── ws-broadcaster/         AWS Lambda handler + local Fastify variant
├── infrastructure/             AWS CDK app (5 stacks)
├── .github/                    CI + staging/production deploy workflows
├── docker-compose.yml          Full local stack
├── docker-compose.override.yml.example
└── .env.example
```

---

## Prerequisites

- Node.js 20+
- Docker Desktop / Docker Engine with Compose v2
- (Optional, for AWS deploys) an AWS account + `aws` CLI + CDK bootstrap

---

## Local development

```bash
cp .env.example .env
docker compose up --build
```

| Service          | URL                       |
|------------------|---------------------------|
| Frontend (User)  | http://localhost:5173     |
| Frontend (Admin) | http://localhost:5173/admin |
| Core API         | http://localhost:3000     |
| Kanban plugin    | http://localhost:3001     |
| WS Broadcaster   | http://localhost:3002     |
| Postgres         | localhost:5432            |
| Redis            | localhost:6379            |
| DynamoDB Local   | http://localhost:8000     |
| MinIO console    | http://localhost:9001     |

Local mode uses **Redis Pub/Sub** as the event bus (`EVENT_BUS=redis`); AWS mode
uses EventBridge (`EVENT_BUS=eventbridge`). The same `EventBus` interface backs
both implementations — see `services/core/src/eventbus/`.

### Smoke test

In one terminal:

```bash
docker compose up --build
```

Wait until you see `core service ready` and `kanban plugin ready`.

In a second terminal — verify the plugin registered itself:

```bash
curl -s http://localhost:3000/api/plugins | jq
# -> { "plugins": [ { "pluginId": "kanban-board", "endpoint": "http://plugin-kanban:3001", ... } ] }
```

Tail the broadcaster logs (it subscribes to Redis Pub/Sub):

```bash
docker compose logs -f ws-broadcaster
```

Create a ticket through the Core's reverse proxy:

```bash
curl -s -X POST http://localhost:3000/api/plugins/kanban-board/proxy/tickets \
  -H "content-type: application/json" \
  -d '{"boardId": 1, "title": "First ticket", "status": "todo"}' | jq
```

Move the ticket — this should emit a `ticket.moved` event:

```bash
curl -s -X PUT http://localhost:3000/api/plugins/kanban-board/proxy/tickets/1 \
  -H "content-type: application/json" \
  -d '{"status": "in-progress"}' | jq
```

In the broadcaster logs you'll see:

```
event received  source=plugin.kanban-board  detailType=ticket.moved  recipients=0
```

(`recipients=0` is expected — no WebSocket clients are registered against
DynamoDB Local in the smoke test. The full demo with real connections requires
the AWS deploy.)

Recent broadcaster events are also queryable:

```bash
curl -s http://localhost:3002/events/recent | jq
```

---

## Frontend

A single Vite + React + TypeScript app (`services/frontend`) ships with the
stack. After `docker compose up --build`, two routes are available:

| URL                            | What you get                                                    |
|--------------------------------|-----------------------------------------------------------------|
| http://localhost:5173          | **User view** — live plugin list + Kanban board (`kanban-board` plugin) |
| http://localhost:5173/admin    | **Admin view** — architecture graph, component detail, live event stream |

The user view auto-refreshes the registered plugins every 5 seconds, renders a
3-column Kanban board with `@dnd-kit`-powered drag-and-drop against the
proxied plugin (`/api/plugins/kanban-board/proxy/tickets`), and subscribes to
the broadcaster WebSocket at `ws://localhost:3002/ws` for live `ticket.*`
events. The admin view fetches `/api/admin/architecture` plus `/api/plugins`,
merges both into a React Flow graph, and tails `/api/admin/events/stream`
(Server-Sent Events) for a live event log of the last 30 seconds.

_(Screenshot placeholder: an image of the User view and Admin view could live
under `docs/screenshots/` once captured.)_

---

## Deploying to AWS

```bash
cd infrastructure
npm install

# One-time per account/region
npx cdk bootstrap aws://ACCOUNT_ID/eu-central-1

# Preview everything
npx cdk synth

# Deploy all stacks for the staging environment
npx cdk deploy --all -c environment=staging -c imageTag=latest

# Deploy a single stack
npx cdk deploy Msa2-Staging-PluginKanban -c environment=staging
```

### Stack dependency graph

```
Msa2-{env}-Network
    └── Msa2-{env}-Persistence  (Aurora + Redis + DynamoDB + S3)
            └── Msa2-{env}-Core        (ECS cluster, Core service, ALB, REST API, EventBridge)
                    ├── Msa2-{env}-Realtime      (WebSocket API + Broadcaster Lambda)
                    └── Msa2-{env}-PluginKanban  (Kanban ECS service)
```

### CI/CD

| Trigger              | Workflow                          | Effect                                    |
|----------------------|-----------------------------------|-------------------------------------------|
| PR / push            | `.github/workflows/ci.yml`        | Typecheck + test + cdk synth + docker build |
| Push to `main`       | `deploy-staging.yml`              | OIDC → ECR push → `cdk deploy --all` (staging) |
| Tag `v*.*.*`         | `deploy-production.yml`           | Same flow for production                  |

Required GitHub Secrets: `AWS_ROLE_ARN_STAGING`, `AWS_ROLE_ARN_PRODUCTION`,
`AWS_REGION`, `ECR_REGISTRY` (e.g. `123456789012.dkr.ecr.eu-central-1.amazonaws.com`).

---

## Pragmatic design notes

- **Fastify over Express** — same ergonomics, ~2x throughput, better TypeScript types.
- **Plugin registry** stored in Redis with a 30s TTL keyed on `plugin:registry:<id>`,
  plus a `SET` index for fast listing. Heartbeats refresh the TTL.
- **Reverse proxy** uses `undici` (Node's built-in HTTP client) rather than
  `@fastify/http-proxy`, to keep the dep graph small and the body forwarding
  explicit.
- **Event bus is an interface** — `RedisPubSubBus` and `EventBridgeBus`
  implement the same contract; the choice is a single env var (`EVENT_BUS`).
- **The broadcaster has two entry points** — `handler.ts` is the EventBridge
  Lambda for AWS; `local.ts` is a Fastify process that subscribes to Redis
  Pub/Sub for the docker-compose flow. They share `connections.ts` (the
  DynamoDB store).
- **Aurora Serverless v2** with a min ACU of 0.5 to keep PoC costs low.
- **CDK assets** — stacks default to `ContainerImage.fromAsset(...)` so a
  fresh deploy works without pre-pushing images. The production workflow
  overrides this by pushing tagged images to ECR first.
- **Auth, multi-tenancy, sandboxing** are deliberately deferred (see
  implementation plan §11).

---

## Useful commands

```bash
# Whole-monorepo lint / typecheck / test
npm run typecheck
npm run test

# Per workspace
npm --workspace services/core run dev
npm --workspace services/plugins/kanban run dev

# CDK
npm --workspace infrastructure run synth
npm --workspace infrastructure run diff

# Validate compose file
docker compose config
```
