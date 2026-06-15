# API Gateway — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Komponente:** API Gateway (kein eigener Service mit Code, sondern Konfiguration einer Off-the-Shelf-Komponente)  
> **Technologie lokal:** Traefik v3.0  
> **Technologie AWS:** Amazon API Gateway (HTTP API + WebSocket API)  
> **Port (lokal):** 80 (HTTP), 443 (HTTPS in Produktion)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Routing-Regeln](#2-routing-regeln)
3. [Traefik-Konfiguration (Lokal)](#3-traefik-konfiguration-lokal)
4. [TLS und Zertifikate](#4-tls-und-zertifikate)
5. [Rate Limiting](#5-rate-limiting)
6. [CORS](#6-cors)
7. [WebSocket-Upgrade-Handling](#7-websocket-upgrade-handling)
8. [Trace-ID-Initiierung](#8-trace-id-initiierung)
9. [Authentication am Gateway?](#9-authentication-am-gateway)
10. [Health-Checks und Service Discovery](#10-health-checks-und-service-discovery)
11. [AWS-Variante](#11-aws-variante)
12. [Observability](#12-observability)
13. [Implementierungs-Hinweise](#13-implementierungs-hinweise)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Single Entry Point** für externe Clients (Frontend, CLI, externe APIs)
- **Routing** anhand Pfad-Prefixe zu Services
- **TLS-Termination** in Produktion
- **Rate Limiting** pro IP und für sensitive Endpoints (Login, Register)
- **CORS** für Browser-Clients
- **WebSocket-Upgrade** transparent durchreichen
- **Trace-ID-Initiierung** wenn nicht vom Client mitgeliefert
- **Edge-Logging** für Operator-Sichtbarkeit
- **Service Discovery** über Docker-Labels (lokal)

### 1.2 Verantwortet NICHT

- **Authentication** — pro Service, JWT-Validation gegen JWKS
- **Authorization** — Project Service ist Authority
- **Geschäftslogik, Aggregation, Transformation** (Smart Endpoints, Dumb Pipes)
- **Caching von Service-Responses**
- **Service-spezifische Validierung**

### 1.3 Off-the-Shelf statt Custom

Wir bauen kein eigenes Gateway. Traefik (lokal) und AWS API Gateway (Produktion) sind reife, konfigurierbare Lösungen. Eigenes Gateway in Go würde Wochen kosten und keinen Mehrwert bieten — alle relevanten Features (Routing, TLS, Rate Limit, CORS) sind kommodifiziert.

### 1.4 Warum Traefik

| Option | Pro | Kontra |
|--------|-----|--------|
| **Traefik** | Native Docker-Integration via Labels, einfache Config, gutes Dashboard | weniger Plugins als Kong |
| Nginx | sehr verbreitet, performant | statische Config, kein Auto-Discovery |
| Kong | mächtige Plugin-Architektur | komplexer, eigene DB, overkill für MVP |
| Envoy | sehr leistungsstark | steile Lernkurve |

Entscheidung: Traefik lokal (Service-Discovery via Labels passt perfekt zu Docker Compose), AWS API Gateway in Produktion (managed, kein Betrieb).

---

## 2. Routing-Regeln

### 2.1 Pfad-zu-Service-Mapping

| Pfad-Pattern | Service | Beispiel-Endpoints |
|--------------|---------|---------------------|
| `/api/v1/auth/*` | auth | `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/me` |
| `/.well-known/jwks.json` | auth | JWKS für andere Services und externe Clients |
| `/api/v1/projects/*` | project / document / plugin | siehe Disambiguation unten |
| `/api/v1/boards/*` | project / task | siehe Disambiguation |
| `/api/v1/tasks/*` | task | `/tasks/{id}`, `/tasks/{id}:move`, `/tasks/{id}/comments` |
| `/api/v1/comments/*` | task | `/comments/{id}` |
| `/api/v1/documents/*` | document | `/documents/{id}`, `/documents/{id}/download` |
| `/api/v1/webhooks/*` | plugin | `/webhooks/{id}`, `/webhooks/{id}/deliveries` |
| `/api/v1/board-types/*` | boardregistry | `/board-types`, `/board-types/{type}` (Katalog + Registrierung) |
| `/api/v1/notifications/*` | notification | `/notifications`, `/notifications/{id}/read` |
| `/ws` | notification | WebSocket-Upgrade |

### 2.2 Pfad-Disambiguation

Mehrere Services haben Routes unter `/api/v1/projects/{projectId}/...`:

| Subpfad | Ziel-Service |
|---------|--------------|
| `/api/v1/projects/{id}/boards` | project |
| `/api/v1/projects/{id}/members` | project |
| `/api/v1/projects/{id}/documents` | document |
| `/api/v1/projects/{id}/webhooks` | plugin |

Lösung über **Regex-Matching mit Priorität:**

- Höhere Priorität: `PathRegexp` für spezifische Sub-Pattern (`document`, `plugin`)
- Niedrigere Priorität: `PathPrefix` als Fallback (`project`)

Analog für `/api/v1/boards`:

- `PathRegexp(`^/api/v1/boards/[^/]+/tasks`)` → task
- `PathPrefix(`/api/v1/boards`)` → project

### 2.3 Boards-Routes Detail

```
/api/v1/boards/{boardId}                       → project (board details)
/api/v1/boards/{boardId}/columns               → project (column CRUD)
/api/v1/boards/{boardId}/tasks                 → task    (task list + create)
```

Der dritte Endpoint ist semantisch task-spezifisch und liegt deshalb beim Task Service.

---

## 3. Traefik-Konfiguration (Lokal)

### 3.1 Statische Konfiguration `infra/traefik/traefik.yml`

```yaml
api:
  dashboard: true
  insecure: true            # nur lokal — in Produktion via Auth absichern

entryPoints:
  web:
    address: ":80"
    forwardedHeaders:
      insecure: true
  metrics:
    address: ":8082"

providers:
  docker:
    exposedByDefault: false
    network: teamboard_teamboard-net
  file:
    directory: /etc/traefik/dynamic
    watch: true

log:
  level: INFO
  format: json

accessLog:
  format: json
  filters:
    statusCodes:
      - "400-599"
  fields:
    headers:
      defaultMode: keep
      names:
        Authorization: redact
        Cookie: redact

metrics:
  prometheus:
    entryPoint: metrics

tracing:
  serviceName: traefik-gateway
  otlp:
    grpc:
      endpoint: jaeger:4317
      insecure: true
```

### 3.2 Dynamische Konfiguration über Docker-Labels

Bereits in `docker-compose.yml` (siehe `orchestration.md`). Hier kompakt:

```yaml
services:
  auth:
    labels:
      - traefik.enable=true
      - traefik.http.routers.auth.rule=PathPrefix(`/api/v1/auth`) || PathPrefix(`/.well-known/jwks.json`)
      - traefik.http.routers.auth.priority=10
      - traefik.http.services.auth.loadbalancer.server.port=8001

  task:
    labels:
      - traefik.enable=true
      - traefik.http.routers.task.rule=PathPrefix(`/api/v1/tasks`) || PathPrefix(`/api/v1/comments`) || PathRegexp(`^/api/v1/boards/[^/]+/tasks`)
      - traefik.http.routers.task.priority=20
      - traefik.http.services.task.loadbalancer.server.port=8003

  project:
    labels:
      - traefik.enable=true
      - traefik.http.routers.project.rule=PathPrefix(`/api/v1/projects`) || PathPrefix(`/api/v1/boards`)
      - traefik.http.routers.project.priority=10
      - traefik.http.services.project.loadbalancer.server.port=8002

  document:
    labels:
      - traefik.enable=true
      - traefik.http.routers.document.rule=PathPrefix(`/api/v1/documents`) || PathRegexp(`^/api/v1/projects/[^/]+/documents`)
      - traefik.http.routers.document.priority=20
      - traefik.http.services.document.loadbalancer.server.port=8004

  notification:
    labels:
      - traefik.enable=true
      - traefik.http.routers.notification.rule=PathPrefix(`/api/v1/notifications`) || PathPrefix(`/ws`)
      - traefik.http.routers.notification.priority=10
      - traefik.http.services.notification.loadbalancer.server.port=8005

  plugin:
    labels:
      - traefik.enable=true
      - traefik.http.routers.plugin.rule=PathPrefix(`/api/v1/webhooks`) || PathRegexp(`^/api/v1/projects/[^/]+/webhooks`)
      - traefik.http.routers.plugin.priority=20
      - traefik.http.services.plugin.loadbalancer.server.port=8006

  boardregistry:
    labels:
      - traefik.enable=true
      - traefik.http.routers.boardregistry.rule=PathPrefix(`/api/v1/board-types`)
      - traefik.http.services.boardregistry.loadbalancer.server.port=8007
```

**Priority-Logik:** Höhere Priority gewinnt. `task` (20) > `project` (10) für `/boards/{id}/tasks`. `document` und `plugin` (jeweils 20) > `project` (10) für ihre Project-Subpfade.

### 3.3 Middleware-Definitionen `infra/traefik/dynamic/middlewares.yml`

```yaml
http:
  middlewares:
    rate-limit:
      rateLimit:
        average: 100
        period: 1s
        burst: 200
        sourceCriterion:
          ipStrategy:
            depth: 1

    auth-rate-limit:
      rateLimit:
        average: 5
        period: 60s
        burst: 10

    cors-default:
      headers:
        accessControlAllowMethods:
          - GET
          - POST
          - PUT
          - PATCH
          - DELETE
          - OPTIONS
        accessControlAllowHeaders:
          - "*"
        accessControlAllowOriginListRegex:
          - "^http://localhost:3000$"
          - "^http://localhost$"
          - "^https://teamboard\\.example$"
        accessControlAllowCredentials: true
        accessControlMaxAge: 3600
        addVaryHeader: true

    security-headers:
      headers:
        frameDeny: true
        contentTypeNosniff: true
        browserXssFilter: true
        referrerPolicy: "strict-origin-when-cross-origin"
        stsSeconds: 31536000
        stsIncludeSubdomains: true

    body-limit:
      buffering:
        maxRequestBodyBytes: 10485760
        memRequestBodyBytes: 1048576
```

### 3.4 Middleware-Anwendung über Service-Labels

```yaml
services:
  auth:
    labels:
      - traefik.http.routers.auth.middlewares=auth-rate-limit@file,cors-default@file,security-headers@file,body-limit@file

  task:
    labels:
      - traefik.http.routers.task.middlewares=rate-limit@file,cors-default@file,security-headers@file,body-limit@file

  notification:
    labels:
      # WebSocket braucht kein body-limit (Frames sind kein "Body")
      - traefik.http.routers.notification.middlewares=rate-limit@file,cors-default@file,security-headers@file
```

---

## 4. TLS und Zertifikate

### 4.1 Lokal: HTTP only

Traefik läuft auf Port 80. WSS und HTTPS sind in lokaler Entwicklung nicht nötig.

### 4.2 Produktion: AWS Certificate Manager

In AWS übernimmt **AWS Certificate Manager + CloudFront** TLS-Termination. Backend (ECS via API Gateway) sieht nur HTTP. Zertifikat ist kostenlos und auto-renewable.

### 4.3 Produktion: Let's Encrypt (eigener Server)

```yaml
certificatesResolvers:
  letsencrypt:
    acme:
      email: ops@teamboard.example
      storage: /letsencrypt/acme.json
      httpChallenge:
        entryPoint: web
```

---

## 5. Rate Limiting

### 5.1 Drei-Schichten-Strategie

| Schicht | Limit | Wo |
|---------|-------|-----|
| **Edge (Gateway)** | 100 req/s pro IP, 5 req/min für Auth | Traefik |
| **Service-spezifisch** | 5 Login-Versuche/15min pro E-Mail | Auth Service mit Redis |
| **WebSocket-Subscriptions** | 50 Channels pro Connection | Notification Service |

Edge-Limit schützt vor Brute-Force und einfachen DoS. Service-spezifisches Limit kann auf User-Identität targeten, was Gateway nicht kann.

### 5.2 Rate-Limit-Response

Bei Überschreitung: `429 Too Many Requests` mit `Retry-After`-Header. Traefik macht das automatisch.

### 5.3 IP-Erkennung hinter Proxies

```yaml
entryPoints:
  web:
    forwardedHeaders:
      trustedIPs:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
```

---

## 6. CORS

### 6.1 Whitelist statt Wildcard

`accessControlAllowOrigin: "*"` ist mit Credentials nicht zulässig (Browser blockt). Daher `accessControlAllowOriginListRegex` mit konkreten Origins.

### 6.2 Preflight (OPTIONS)

Traefik mit `cors-default`-Middleware handled OPTIONS automatisch. Service muss keine eigene CORS-Logic haben.

### 6.3 Credentials

`accessControlAllowCredentials: true` ist nötig für Cookies und `Authorization`-Header.

### 6.4 WebSocket und CORS

WebSocket-Upgrade-Requests haben `Origin`-Header. Notification Service prüft den Origin selbst (siehe `notification.md` §18.3). Gateway leitet nur durch.

---

## 7. WebSocket-Upgrade-Handling

### 7.1 Pass-Through

Traefik leitet WebSocket-Upgrades transparent durch — keine spezielle Config nötig. `Connection: Upgrade`-Header bleiben erhalten.

### 7.2 Timeouts

WebSocket-Verbindungen sind langlebig. Standard-Timeouts würden sie kappen:

```yaml
http:
  serversTransports:
    websocket-transport:
      forwardingTimeouts:
        dialTimeout: "10s"
        responseHeaderTimeout: "0s"      # 0 = kein Timeout
        idleConnTimeout: "0s"
```

In Service-Labels:

```yaml
notification:
  labels:
    - traefik.http.services.notification.loadbalancer.serverstransport=websocket-transport@file
```

### 7.3 Sticky Sessions

Nicht nötig wenn Notification-Service-Backplane (siehe `notification.md` §3) korrekt ist — Events werden via Redis Pub/Sub an alle Instanzen verteilt.

Falls dennoch gewünscht:

```yaml
notification:
  labels:
    - traefik.http.services.notification.loadbalancer.sticky.cookie.name=teamboard_ws_session
    - traefik.http.services.notification.loadbalancer.sticky.cookie.secure=true
```

---

## 8. Trace-ID-Initiierung

### 8.1 W3C Trace Context

Wir nutzen den Standard `traceparent`-Header (RFC 9110):

```
traceparent: 00-<trace-id>-<parent-span-id>-<flags>
traceparent: 00-0af7651916cd43dd8448eb211c80319c-b9c7c989f97918e1-01
```

### 8.2 Verhalten am Gateway

- Wenn Client `traceparent` mitschickt → durchreichen
- Wenn nicht → Gateway erzeugt einen neuen
- In jedem Fall: an Service durchgeben

Traefik mit OTLP-Tracing macht das nativ.

### 8.3 Korrelation in Logs

Services parsen `traceparent`, extrahieren Trace-ID, fügen sie als `trace_id` in alle Log-Einträge ein. Siehe `shared.md` §5.

---

## 9. Authentication am Gateway?

### 9.1 Designentscheidung: NEIN

Authentication passiert **in jedem Service**, nicht am Gateway. Begründung:

- **Konsistenz:** Service-zu-Service-Calls (Task → Project) müssen auch JWTs validieren. Wenn nur Gateway das tut, müssten Services bei internen Calls vertrauen — gefährlich.
- **Trace-Bezug:** Im Service-Log will man wissen, welcher User die Operation ausgelöst hat. Wenn Gateway User-ID extrahiert, müsste sie als Header durchgereicht werden — fehleranfällig.
- **Token-Inhalt:** Services brauchen oft mehr als "User ist authenticated" — z. B. die User-ID. Das müsste sowieso aus dem Token kommen.

**Konsequenz:** Gateway leitet `Authorization`-Header durch. Jeder Service nutzt `authmiddleware` aus `shared/go/`.

### 9.2 Was bleibt am Gateway

- TLS-Termination
- Rate Limiting (auch unauthenticated Traffic muss begrenzt werden)
- CORS-Preflight (kein Auth-Token im OPTIONS-Request)

### 9.3 Optionale Erweiterung: Pre-Validation

Defense-in-Depth-Variante: Gateway nutzt ForwardAuth-Middleware oder Plugin für Token-Validierung als zusätzliche Schicht. Würde 401 für offensichtlich invalid Tokens vor Service-Eintritt liefern. Im MVP nicht nötig.

---

## 10. Health-Checks und Service Discovery

### 10.1 Service Discovery

Traefik liest Docker-Labels und entdeckt neue Services automatisch. Beim Start eines Service-Containers ist die Route innerhalb 1-2 Sekunden aktiv.

### 10.2 Health-Check vom Gateway

```yaml
labels:
  - traefik.http.services.task.loadbalancer.healthcheck.path=/health/ready
  - traefik.http.services.task.loadbalancer.healthcheck.interval=10s
  - traefik.http.services.task.loadbalancer.healthcheck.timeout=5s
```

Bei Fehler: Service wird aus Routing entfernt, kein Traffic.

### 10.3 Multi-Instance / Skalierung

`docker compose up --scale task=3` startet drei Backends. Standard-Load-Balancing ist Round-Robin.

---

## 11. AWS-Variante

### 11.1 Architektur

```
Browser
  │ HTTPS
  ▼
CloudFront (CDN, optional)
  │
  ▼
AWS WAF (DDoS, SQLi, XSS)
  │
  ▼
Amazon API Gateway
  ├── HTTP API (REST)
  └── WebSocket API
  │
  ▼ VPC Link
ECS Services (Fargate)
```

### 11.2 HTTP API (CDK-Skeleton)

```typescript
// infra/cdk/lib/api-gateway-stack.ts

const api = new HttpApi(this, 'TeamBoardApi', {
  apiName: 'teamboard',
  corsPreflight: {
    allowOrigins: ['https://teamboard.example'],
    allowMethods: [CorsHttpMethod.ANY],
    allowHeaders: ['*'],
    allowCredentials: true,
    maxAge: Duration.hours(1),
  },
});

const authIntegration = new HttpServiceDiscoveryIntegration(
  'AuthIntegration',
  authService.cloudMapService,
);

api.addRoutes({
  path: '/api/v1/auth/{proxy+}',
  methods: [HttpMethod.ANY],
  integration: authIntegration,
});

api.addRoutes({
  path: '/.well-known/jwks.json',
  methods: [HttpMethod.GET],
  integration: authIntegration,
});

// Analog für project, task, document, notification, plugin
```

### 11.3 WebSocket API

```typescript
const wsApi = new WebSocketApi(this, 'TeamBoardWsApi', {
  connectRouteOptions: {
    integration: new WebSocketLambdaIntegration('Connect', wsConnectFn),
    authorizer: new WebSocketLambdaAuthorizer('JWTAuth', jwtAuthorizerFn),
  },
  disconnectRouteOptions: {
    integration: new WebSocketLambdaIntegration('Disconnect', wsDisconnectFn),
  },
  defaultRouteOptions: {
    integration: new WebSocketLambdaIntegration('Default', wsMessageFn),
  },
});
```

### 11.4 Rate Limiting in AWS

```typescript
const stage = new HttpStage(this, 'ProdStage', {
  httpApi: api,
  throttle: {
    rateLimit: 1000,
    burstLimit: 2000,
  },
});
```

WAF für anspruchsvollere Limits (per IP, Geo-Block, etc.).

### 11.5 Trade-offs

| Aspekt | Traefik (lokal) | AWS API Gateway |
|--------|-----------------|-----------------|
| Konfig-Sprache | YAML / Labels | CDK / Terraform |
| Service Discovery | Docker-Labels | Cloud Map / Service Connect |
| Skalierung | manuell pro Service | automatisch |
| Kosten | Container-Resource | Pay-per-Request |
| WebSockets | nativ pass-through | separate WebSocket API |
| Ops-Aufwand | Container betreiben | managed |

---

## 12. Observability

### 12.1 Access Logs

Traefik schreibt JSON-strukturierte Access Logs nach stdout:

```json
{
  "ClientHost": "172.20.0.1",
  "DownstreamStatus": 200,
  "DownstreamContentSize": 1234,
  "Duration": 23456789,
  "RequestMethod": "POST",
  "RequestPath": "/api/v1/tasks",
  "RouterName": "task@docker",
  "ServiceName": "task@docker",
  "StartUTC": "2026-05-07T12:00:00.000Z"
}
```

### 12.2 Prometheus Metrics

Traefik exposed `/metrics` auf Port 8082:

- `traefik_service_request_duration_seconds`
- `traefik_service_requests_total`
- `traefik_router_requests_total`
- `traefik_entrypoint_requests_total`

### 12.3 Tracing

Traefik OTLP-Export an Jaeger ist konfiguriert. Jeder Request erscheint als Span in Jaeger UI, korreliert mit Service-Spans über `trace_id`.

### 12.4 Dashboard

Traefik Dashboard auf `http://localhost:8080` zeigt: konfigurierte Routes, Service-Health, Live-Stats, Middleware-Chain. In Produktion: hinter Auth oder gar nicht exposed.

---

## 13. Implementierungs-Hinweise

### 13.1 Verzeichnisstruktur

```
infra/traefik/
├── traefik.yml                    # statische Config
└── dynamic/
    ├── middlewares.yml            # CORS, Rate-Limit, Security
    ├── tls.yml                    # TLS-Optionen (für Production)
    └── routes.yml                 # statische Routes (alternativ zu Docker Labels)
```

### 13.2 Implementations-Reihenfolge

1. **Statische Config** (`traefik.yml`) erstellen
2. **Service-Labels** in `docker-compose.yml` ergänzen
3. **Middleware** in `middlewares.yml` definieren
4. **Lokal testen** mit `make up` + `curl http://localhost/api/v1/auth/login`
5. **Disambiguation testen** — alle Routes aus Tabelle 2 manuell durchgehen
6. **WebSocket-Test** — DevTools Connection auf `ws://localhost/ws`
7. **CORS-Test** — Browser-Fetch von `http://localhost:3000`

### 13.3 Häufige Stolperfallen

- **Priority falsch herum:** Höhere Priority gewinnt — `task` muss höher als `project` sein für `/boards/{id}/tasks`. Wenn Routes scheinbar wahllos zugeordnet werden: Priorities prüfen.
- **Network-Name falsch:** `traefik.yml` braucht `network: <project>_<network>`. Default-Compose-Project-Name ist Verzeichnisname. Konsistent setzen via `name: teamboard` im Compose-Top-Level.
- **CORS Preflight schlägt fehl:** OPTIONS-Response in DevTools prüfen. Häufig: Origin nicht in Whitelist, oder Middleware nicht angehängt.
- **WebSocket schließt nach 60s:** Idle-Connection-Timeout — siehe §7.2.
- **`accessControlAllowCredentials: true` mit `*`:** Browser blockt. Konkrete Origins listen.
- **Trailing Slashes:** `PathPrefix(`/api/v1/auth`)` matcht auch `/api/v1/authentication`. Bei breit gefassten Pattern beachten.
- **Health-Check-Resolution:** Traefik prüft Service-URLs aus dem Container-Netzwerk. Container-Name nutzen, nicht `localhost`.

### 13.4 Routing-Test-Skript

```bash
#!/bin/bash
# scripts/test-routing.sh
set -e
BASE="http://localhost"

check() {
  local path="$1"
  local expected_service="$2"
  actual=$(curl -s -o /dev/null -w "%{http_code}" "$BASE$path")
  echo "  $path → HTTP $actual (expected service: $expected_service)"
}

echo "Testing routing rules..."
check "/api/v1/auth/login" "auth"
check "/.well-known/jwks.json" "auth"
check "/api/v1/projects" "project"
check "/api/v1/projects/abc/boards" "project"
check "/api/v1/projects/abc/documents" "document"
check "/api/v1/projects/abc/webhooks" "plugin"
check "/api/v1/boards/abc" "project"
check "/api/v1/boards/abc/tasks" "task"
check "/api/v1/tasks/abc" "task"
check "/api/v1/comments/abc" "task"
check "/api/v1/documents/abc" "document"
check "/api/v1/webhooks/abc" "plugin"
check "/api/v1/notifications" "notification"
```

### 13.5 Acceptance Criteria

| Kriterium | Erfüllt wenn |
|-----------|--------------|
| Routing | Alle Pfade aus Tabelle 2.1 erreichen den korrekten Service |
| TLS | (Prod) Routes nur via HTTPS, HTTP→HTTPS-Redirect aktiv |
| Rate Limit | Auth-Endpoint nach 10 Requests/min antwortet 429 |
| CORS | Browser von `localhost:3000` kann mit Credentials anfragen |
| WebSocket | Connection auf `/ws` bleibt >5min offen, Frames bidirektional |
| Trace | Eingehender Request ohne `traceparent` bekommt einen — alle nachgelagerten Logs zeigen dieselbe Trace-ID |
| Health | Service-Failure entfernt aus Routing innerhalb 30s |
| Service Discovery | Neuer Service-Container ist innerhalb 5s erreichbar |

---

**Ende des Gateway-Designs.**
