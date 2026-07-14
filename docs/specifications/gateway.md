# API Gateway — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Komponente:** API Gateway (kein eigener Service mit Code, sondern Konfiguration einer Off-the-Shelf-Komponente)  
> **Technologie:** Traefik v3.0 — **dieselbe** Instanz lokal wie in Produktion (auf der EC2-Box, via `docker-compose.prod.yml`)  
> **Port:** 80 (HTTP); TLS optional via Let's Encrypt (siehe §4)  
> **Stand:** 2026-07

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
11. [Gateway in Produktion (AWS EC2)](#11-gateway-in-produktion-aws-ec2)
12. [Observability](#12-observability)
13. [Implementierungs-Hinweise](#13-implementierungs-hinweise)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Single Entry Point** für externe Clients (Frontend, CLI, externe APIs)
- **Routing** anhand Pfad-Prefixe zu Services
- **TLS-Termination** in Produktion
- **Rate Limiting** pro IP (global am Edge). Login-/Register-spezifisches Limit
  liegt im Auth-Service (Redis-Sliding-Window), nicht am Gateway.
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

Wir bauen kein eigenes Gateway. Traefik ist eine reife, konfigurierbare Off-the-Shelf-Lösung und läuft lokal wie in Produktion identisch. Ein eigenes Gateway in Go würde Wochen kosten und keinen Mehrwert bieten — alle relevanten Features (Routing, TLS, Rate Limit, CORS) sind kommodifiziert.

### 1.4 Warum Traefik

| Option | Pro | Kontra |
|--------|-----|--------|
| **Traefik** | Native Docker-Integration via Labels, einfache Config, gutes Dashboard | weniger Plugins als Kong |
| Nginx | sehr verbreitet, performant | statische Config, kein Auto-Discovery |
| Kong | mächtige Plugin-Architektur | komplexer, eigene DB, overkill für MVP |
| Envoy | sehr leistungsstark | steile Lernkurve |

Entscheidung: Traefik — lokal **und** in Produktion (Service-Discovery via Docker-Labels passt perfekt zum Compose-Stack auf der EC2-Box; eine Konfiguration für beide Umgebungen).

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
    # Edge-Middlewares global auf alle gerouteten Services anwenden
    # (Namen aus dynamic.yml, file-Provider).
    http:
      middlewares:
        - secure-headers@file
        - cors@file
        - ratelimit@file
  websecure:
    address: ":443"

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false
    network: teamboard-net
  file:
    filename: /etc/traefik/dynamic.yml
    watch: true

log:
  level: INFO

accessLog: {}
```

> **Status (committed):** Die obige Datei entspricht dem aktuell eingecheckten
> `infra/traefik/traefik.yml` (Provider docker+file, Web-Entrypoint mit globalen
> Edge-Middlewares). **Zielkonfiguration, noch nicht aktiviert:** strukturierter
> JSON-Access-Log mit `Authorization`/`Cookie`-Redaction, ein `metrics`-Entrypoint
> (Prometheus) und OTLP-Tracing zu Jaeger. Diese werden ergänzt, sobald
> Observability am Edge benötigt wird.

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

### 3.3 Middleware-Definitionen `infra/traefik/dynamic.yml`

Drei Middlewares, vom file-Provider geladen und **global am Web-Entrypoint**
angewandt (siehe §3.1) — kein Per-Router-Label nötig:

```yaml
http:
  middlewares:
    # Security-Header. stsSeconds (HSTS) wirkt nur über HTTPS — Browser ignorieren
    # es am lokalen Plaintext-:80, daher harmlos hier und automatisch aktiv, sobald
    # TLS am Edge terminiert.
    secure-headers:
      headers:
        frameDeny: true
        contentTypeNosniff: true
        browserXssFilter: true
        referrerPolicy: "strict-origin-when-cross-origin"
        permissionsPolicy: "geolocation=(), microphone=(), camera=()"
        stsSeconds: 31536000
        stsIncludeSubdomains: true
        stsPreload: true
        customResponseHeaders:
          Server: ""
          X-Powered-By: ""

    # CORS für die Browser-SPA (beantwortet Preflight-OPTIONS für erlaubte Origins).
    cors:
      headers:
        accessControlAllowMethods: [GET, POST, PATCH, PUT, DELETE, OPTIONS]
        accessControlAllowHeaders: [Authorization, Content-Type]
        accessControlAllowOriginList:
          - "http://localhost:3000"
          - "http://localhost:5173"
        accessControlAllowCredentials: true
        accessControlMaxAge: 100
        addVaryHeader: true

    # Rate-Limit pro Quell-IP (Token-Bucket). average = nachhaltige Rate über
    # `period`, burst = kurzfristige Spitze (deckt parallele Requests beim
    # SPA-Seitenaufbau ab).
    ratelimit:
      rateLimit:
        average: 50
        period: 1s
        burst: 100
```

Der Access-Log ist JSON-formatiert und redactet `Authorization`/`Cookie`
(`infra/traefik/traefik.yml`). **Bekannte Restlücke:** als Query-Parameter
übergebene Tokens (WebSocket `/ws?token=…`) erscheinen weiterhin in der geloggten
Request-URI — Traefik kann Query-Strings nicht redacten (siehe `docs/TODO.md`).

### 3.4 Middleware-Anwendung (global am Entrypoint)

Die drei Middlewares werden **einmal** am `web`-Entrypoint gesetzt
(`entryPoints.web.http.middlewares` in `traefik.yml`, siehe §3.1) und greifen damit
für **alle** gerouteten Services. Die Service-Labels in §3.2 enthalten daher nur
`rule` + `loadbalancer` — keine Per-Router-`middlewares`.

> **Nicht im MVP (optional/geplant):** ein separates, strengeres `auth-rate-limit`
> nur für `/api/v1/auth/*`, ein `body-limit` (Request-Buffering) und eine
> CORS-Origin-Regex-Liste. Hinweis: ein **Login-Rate-Limit existiert bereits** —
> aber als Redis-Sliding-Window **im Auth-Service** (`ratelimit:login:<email>`),
> nicht am Gateway. Per-Router-Middlewares über Labels sind jederzeit additiv
> möglich, falls einzelne Routen abweichende Limits brauchen.

---

## 4. TLS und Zertifikate

### 4.1 Lokal: HTTP only

Traefik läuft auf Port 80. WSS und HTTPS sind in lokaler Entwicklung nicht nötig.

### 4.2 Produktion: TLS via Let's Encrypt (sslip.io)

Das EC2-Deployment terminiert echtes, öffentlich vertrautes TLS auf `:443` — direkt in Traefik, kein vorgelagerter Managed-Dienst. HTTP auf `:80` bleibt zusätzlich erreichbar (presigned MinIO-URLs und Reset-Links zeigen weiterhin auf `http://${PUBLIC_HOST}`).

**Problem:** Let's Encrypt stellt keine Zertifikate für nackte IPs aus. **Lösung:** ein Wildcard-DNS-Hostname, der die Elastic IP kodiert:

```
${PUBLIC_HOST}.sslip.io   ->  löst öffentlich auf ${PUBLIC_HOST} auf
```

Für diesen Hostnamen holt Traefik ein Zertifikat und löst die **ACME-TLS-ALPN-01**-Challenge selbst auf `:443`. TLS-ALPN-01 (nicht HTTP-01) ist bewusst gewählt: sie läuft auf der TLS-Ebene und berührt kein HTTP-Routing — kann also nicht vom `PathPrefix(/.well-known)`-Router des Auth-Service abgefangen werden.

Die ACME-Statik wird **prod-only** als `TRAEFIK_*`-Env-Variablen in [`docker-compose.prod.yml`](../../docker-compose.prod.yml) eingespielt (lokal bleibt `:443` self-signed und versucht keine Ausstellung); `acme.json` liegt auf einem benannten Volume (`traefik-acme`), überlebt Redeploys und bleibt so unter den LE-Rate-Limits:

```yaml
traefik:
  environment:
    TRAEFIK_CERTIFICATESRESOLVERS_letsencrypt_ACME_EMAIL: "stud106022@fh-wedel.de"
    TRAEFIK_CERTIFICATESRESOLVERS_letsencrypt_ACME_STORAGE: "/acme/acme.json"
    TRAEFIK_CERTIFICATESRESOLVERS_letsencrypt_ACME_TLSCHALLENGE: "true"
    TRAEFIK_ENTRYPOINTS_websecure_HTTP_TLS_CERTRESOLVER: "letsencrypt"
    TRAEFIK_ENTRYPOINTS_websecure_HTTP_TLS_DOMAINS_0_MAIN: "${PUBLIC_HOST}.sslip.io"
  volumes:
    - traefik-acme:/acme
```

**Zugriff:** App und MCP-Endpoint über `https://${PUBLIC_HOST}.sslip.io/` aufrufen. Die SPA nutzt relative URLs und die Settings-Seite leitet die MCP-URL vom aktuellen Hostnamen ab — beide übernehmen das gültige Zertifikat automatisch. (Die nackte IP auf `:443` liefert weiterhin Traefiks self-signed Default-Cert.)

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

## 11. Gateway in Produktion (AWS EC2)

### 11.1 Architektur

In Produktion läuft **dieselbe** Traefik-Instanz wie lokal — als Container auf der EC2-Box, hochgezogen über `docker-compose.prod.yml`. Kein Amazon API Gateway, kein CloudFront, kein zusätzlicher Managed-Dienst. Routing, Rate Limiting, CORS und WebSocket-Pass-through sind damit lokal und in Produktion identisch konfiguriert (dieselben Docker-Labels).

```
Browser
  │ HTTP (:80)   — Elastic IP der EC2-Box
  ▼
Traefik v3.0  (Container auf der Box, Docker-Provider)
  ├── /api/v1/auth, /.well-known   → auth
  ├── /api/v1/projects, /boards …  → project
  ├── /api/v1/tasks, /comments …   → task
  ├── /api/v1/documents            → document
  ├── /api/v1/notifications, /ws   → notification
  ├── /api/v1/webhooks             → plugin
  ├── /api/v1/board-types          → boardregistry
  └── /  (priority 1, Fallback)    → frontend (SPA)
```

Der einzige prod-spezifische Unterschied gegenüber lokal: das Frontend wird **durch** Traefik ausgeliefert (niedrigste Router-Priorität) statt über einen eigenen Port. Details in [`deployment.md`](deployment.md) §7.

### 11.2 Ingress & TLS

- **Ports:** `:80` (Traefik) und `:9000` (MinIO, presigned Document-Downloads direkt an den Browser) sind in der Security Group offen; `:22` für SSH-Deploy.
- **TLS:** aktuell HTTP-only; nachrüstbar direkt in Traefik via Let's Encrypt (§4.3) — kein vorgelagerter ACM/CloudFront nötig.

### 11.3 Skalierung — Ausblick

Traefik verteilt bereits per Round-Robin über Instanzen desselben Service (`docker compose up --scale task=3`, §10.3). Wächst der Bedarf über eine Box hinaus, bleibt das label-basierte Routing bestehen; getauscht wird nur der Service-Discovery-Provider (z. B. Cloud Map bei einem Umzug auf ECS). Der Skalierungs-Pfad ist in [`deployment.md`](deployment.md) §12 beschrieben — jeder Schritt ersetzt genau eine Kante, kein Rewrite.

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
