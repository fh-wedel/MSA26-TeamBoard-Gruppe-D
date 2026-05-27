# CLAUDE.md

Plugin Architecture PoC. Local-first development, AWS-targeted via CDK.
**Read this file before exploring** — it captures decisions and gotchas that
are not obvious from the code alone.

## Authoritative spec

- `implementation_plan.md` — full architectural spec (German). Sections to know:
  - §2: technology decisions and rationale
  - §6: canonical repo layout
  - §11: **explicitly out of scope** — do not implement these unless asked
- `README.md` — setup, smoke-test commands, demo flow

## What lives where

| Path | Purpose |
|---|---|
| `services/core/` | Fastify Core API on :3000. Plugin Registry, Reverse Proxy, Admin routes |
| `services/plugins/kanban/` | Fastify Kanban plugin on :3001. Self-registers; tickets in Postgres locally, DynamoDB in AWS |
| `services/frontend/` | Vite + React + TS + Tailwind on :5173. Routes `/` (user kanban) and `/admin` (architecture dashboard) |
| `lambdas/ws-broadcaster/` | Dual-mode: AWS Lambda handler (`handler.ts`) + local Fastify (`local.ts`) on :3002 |
| `infrastructure/` | AWS CDK (TypeScript). 5 stacks: Network → Persistence → Core → Realtime/Kanban |
| `.github/workflows/` | `ci.yml`, `deploy-staging.yml`, `deploy-production.yml` + composite action |

### Key files for common questions

- Plugin registry interface + backends: `services/core/src/registry/registry.ts` (interface + `RedisPluginRegistry`), `services/core/src/registry/dynamo-registry.ts` (`DynamoPluginRegistry`), `services/core/src/registry/factory.ts` (selection)
- Reverse proxy (buffers bodies — see gotcha #2): `services/core/src/proxy/proxy.ts`
- Event bus abstraction: `services/core/src/eventbus/` (`RedisPubSubBus` local, `EventBridgeBus` AWS, factory.ts picks)
- Admin SSE + architecture endpoint: `services/core/src/api/admin-routes.ts`
- Kanban ticket store: `services/plugins/kanban/src/db/tickets.ts` (interface + `PostgresTicketRepository`), `services/plugins/kanban/src/db/tickets-dynamo.ts` (`DynamoTicketRepository`), `services/plugins/kanban/src/db/factory.ts`
- Kanban self-registration & heartbeat: `services/plugins/kanban/src/registry-client.ts`
- WS broadcaster local mode: `lambdas/ws-broadcaster/src/local.ts`
- CDK entry: `infrastructure/bin/app.ts`
- Architecture graph rendering: `services/frontend/src/components/ArchitectureGraph.tsx`

## Tech stack

TypeScript strict everywhere. Node 20.
Backend: Fastify, ioredis (local), pg (local), undici, pino, @fastify/cors, @fastify/websocket.
AWS data path: `@aws-sdk/client-dynamodb` + `@aws-sdk/lib-dynamodb`, `@aws-sdk/client-eventbridge`.
Frontend: React 18, react-router v6, @dnd-kit/core, reactflow, Tailwind.
AWS SDK v3, AWS CDK (TypeScript), esbuild for Lambda bundling.

## Workspaces

Root `package.json` is an npm workspace covering:
`services/core`, `services/plugins/kanban`, `services/frontend`,
`lambdas/ws-broadcaster`, `infrastructure`. Run `npm install` at the root.

## Local dev

```
docker compose up --build       # full stack
docker compose down -v          # wipe pg/minio volumes (needed after schema changes)
```

URLs after start:
- `:3000` — Core API. **No `/` route**. Use `/health`, `/api/plugins`, `/api/admin/architecture`, `/api/admin/events/stream`.
- `:3001` — Kanban direct (`/health`, `/tickets`)
- `:3002` — Broadcaster (`/health`, `/events/recent`, `/ws`)
- `:5173` — Frontend (`/` and `/admin`)

Smoke test:
```
curl -X POST http://localhost:3000/api/plugins/kanban-board/proxy/tickets \
  -H 'content-type: application/json' \
  -d '{"title":"Test","status":"todo","boardId":42}'
curl http://localhost:3002/events/recent
```

## AWS deploy

```
cd infrastructure
npm install                                       # ALWAYS first; node_modules gitignored
npx cdk bootstrap aws://<account>/eu-central-1    # one-time per account/region
npx cdk synth                                     # validate
npx cdk deploy --all -c environment=staging -c imageTag=<tag>
```

Stacks deploy in this dependency order (enforced in `bin/app.ts`):
Network → Persistence → Core → (Realtime + Kanban).

GitHub Actions: CI runs on PRs (no AWS needed); staging deploys on push to `main`;
production deploys on `v*.*.*` tags. Requires:
- GitHub OIDC IAM identity provider in AWS
- Two IAM roles trusted to the repo (staging + production refs)
- Secrets: `AWS_ROLE_ARN_STAGING`, `AWS_ROLE_ARN_PRODUCTION`, `AWS_REGION`, `ECR_REGISTRY`
- GitHub Environments `staging` and `production`

## Non-obvious decisions (read before editing)

1. **Per-service `tsconfig.json` is SELF-CONTAINED.** It does NOT extend
   `tsconfig.base.json` at the repo root. Docker build context is per-service
   (`./services/core` etc.), so `extends` would fail at build time. If you
   change one of these tsconfigs, keep the others in sync.

2. **Reverse proxy buffers responses to `Buffer`** before `reply.send`
   (`services/core/src/proxy/proxy.ts`). Do NOT "optimise" to streaming —
   undici's `BodyReadable` piped to Fastify produces empty bodies and
   `content-length: 0` on JSON POST/PUT, breaking the frontend silently.

3. **Vite proxy target ≠ browser target.** `vite.config.ts` uses
   `CORE_PROXY_TARGET` / `BROADCASTER_PROXY_TARGET` (container DNS, e.g.
   `http://core:3000`) for the server-side proxy. `VITE_CORE_URL` /
   `VITE_BROADCASTER_URL` are injected into the bundle for the browser
   (`http://localhost:3000`). Inside a container, `localhost` is the container
   itself — using it as proxy target produces `ECONNREFUSED 127.0.0.1:3000`.

4. **CDK security-group ingress rules used to live in consumer stacks**
   (Core, Kanban) — producer-side placement created Network → Persistence →
   Core → Persistence dependency cycles. With Aurora/ElastiCache removed,
   no SG ingress is needed anymore (DynamoDB/EventBridge are accessed via
   IAM, not network ACLs). Re-introducing VPC-bound stores would mean
   re-applying this rule.

5. **Lambda bundling uses esbuild** (devDep in `infrastructure/`). Don't
   remove it — CDK silently falls back to Docker bundling, which slows synth
   dramatically and requires Docker daemon during `cdk deploy`.

6. **Event-bus `tap(handler)` is implementation-only**, not on the `EventBus`
   interface. It exists only on `RedisPubSubBus` (uses `PSUBSCRIBE plugin.*`)
   and powers the admin SSE stream. **In AWS the SSE endpoint returns 503**:
   events flow `EventBridge → Lambda` and don't loop back to Core. To support
   live admin events in AWS you'd need an `EventBridge → Lambda → Core` relay
   (e.g. via WebSocket from the Lambda). Out of scope for the PoC.

7. **Plugin registration carries optional UI metadata** (`runtime`,
   `awsService`, `description`). The admin graph uses these. New plugins
   should send all three for nice display in `/admin`.

8. **Frontend env split:** `VITE_*` vars are inlined into the browser
   bundle (public). `CORE_PROXY_TARGET`/`BROADCASTER_PROXY_TARGET` go only to
   the Vite dev server (never reach the browser). Don't confuse the two.

9. **Storage abstraction via env flags.** Services pick their persistence
   backend at startup from env vars — not bundled per-build. The Kanban
   plugin reads `STORAGE=postgres|dynamodb`, Core reads
   `REGISTRY_BACKEND=redis|dynamodb` and `EVENT_BUS=redis|eventbridge`.
   Local docker-compose pins all three to the Redis/Postgres options;
   the AWS CDK stacks pin them to DynamoDB/EventBridge. Same container
   image runs in both environments.

10. **Ticket IDs are strings, not numbers.** DynamoDB has no SERIAL — IDs
    are UUIDs in AWS, numeric strings ("42") in Postgres. The API surface
    (`Ticket.id`) is always `string`. Frontend already typed accordingly.

## Common failure modes

| Symptom | Cause | Fix |
|---|---|---|
| `Cannot read file '/tsconfig.base.json'` in Docker build | Forgot decision #1 | Keep service tsconfigs self-contained |
| `ECONNREFUSED 127.0.0.1:3000` in frontend logs | Vite proxy target wrong | See decision #3 |
| Postgres exits with "database files are incompatible" | Stale volume from earlier PG version | `docker compose down -v` |
| `Cannot find module 'aws-cdk-lib'` on `cdk synth` | `infrastructure/node_modules` missing | `cd infrastructure && npm install` |
| CDK `No bootstrap stack found` | First-time deploy in account/region | `cdk bootstrap aws://acct/region` |
| CDK dependency cycle error | SG ingress placed in producer stack | See decision #4 |
| Frontend ticket POST returns empty body | Proxy "optimised" back to streaming | See decision #2 |

## Conventions

- TS strict, no `any` without a comment explaining why.
- Structured logging via pino in all backend services. Frontend uses `console`.
- German UI text and German comments are intentional (project context: FH course).
- Don't create `.md` files unless asked. Don't add emojis to files unless asked.
- Don't commit unless explicitly told.
- Treat `implementation_plan.md` §11 as a hard "do not build" list for this PoC.

## Out of scope (Phase 2)

Auth/JWT/Cognito, multi-tenancy, plugin sandboxing & per-plugin IAM,
MSK/Kafka migration, blue-green deploy, plugin versioning/rollback,
monitoring & alerting beyond CDK skeleton. All documented in plan §11.
