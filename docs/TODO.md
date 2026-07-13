# TeamBoard — Offene TODOs / Backlog

Sammelstelle für **noch nicht umgesetzte** Änderungen und bekannte Lücken. Gepflegt
als Gegenstück zu den Spezifikationen: Was in den Docs als „geplant", „Folgearbeit",
„nicht im MVP" oder „Zielkonfiguration" markiert ist, steht hier konsolidiert.

Status-Legende: `[ ]` offen · `[~]` teilweise · `[x]` erledigt (dann zeitnah entfernen).
Stand: 2026-06-30.

---

## 1. Auth & Gateway (Rest aus dem Zero-Trust-Hardening)

Der Auth-Kern ist umgesetzt (RS256-JWT pro Service via `shared/go/authmiddleware`,
kurzlebige `servicetoken`-JWTs intern, Edge-Middlewares CORS/Rate-Limit/Security-Header).
Offen bleibt:

- [x] ~~**HSTS am TLS-Edge:** `secure-headers.stsSeconds` aktiviert (wirkt nur über HTTPS).~~ — erledigt
- [x] ~~**Access-Log-Redaction:** JSON-Format + `Authorization`/`Cookie` redactet.~~ — erledigt
- [x] ~~**Rate-Limit-Tuning:** deliberater Wert (`average 50 / period 1s / burst 100`) + Doku.~~ — erledigt
- [ ] **WS-Query-Token-Leak:** `/ws?token=<JWT>` landet weiterhin in der geloggten Request-URI
  (Traefik redactet keine Query-Strings). Fix: WS-Token aus dem Query-String holen (z. B.
  `Sec-WebSocket-Protocol`-Header) statt `?token=`. — `traefik.yml`, `notification/ws_handler.go`
- [ ] **Edge-Observability (Rest):** `metrics`-Entrypoint (Prometheus) + OTLP-Tracing zu Jaeger
  am Gateway. — `gateway.md §3.1` (Status-Note)
- [ ] **Optionale Gateway-Middlewares:** separates strengeres `auth-rate-limit` nur für
  `/api/v1/auth/*`, `body-limit` (Request-Buffering), CORS-Origin-**Regex**-Liste statt
  fester Origins. — `gateway.md §3.4` (Note)
- [ ] **TLS-Termination:** lokal nur HTTP; in AWS via API Gateway / ACM.

## 2. Tests & Coverage (`shared/go` ≥ 90 %)

- [x] ~~**`shared/go/httputil/httputil_test.go`**~~ — erledigt (Coverage 94.8 %; nebenbei
  `invalid_credentials`→401-Mapping-Bug in `error_mapping.go` gefixt, war durch `invalid_`-Prefix auf 400 verschattet).
- [x] ~~**`shared/go/observability/observability_test.go`**~~ — angelegt (redact/logger/tracing-handler/
  no-op-Tracer/Middleware). Coverage 55.6 %.
- [ ] **`observability` Coverage → 90 %:** Rest ist der **OTLP-Export-Pfad** in `SetupTracer`
  (echter gRPC-Exporter/Provider) — braucht einen Test-Collector/Bufconn, nicht trivial.
- [x] ~~**Document-Test-Failure** `TestInitiateUpload_UnsupportedContentType`~~ — erledigt via #6a
  (octet-stream aus Allowlist entfernt); live verifiziert (422).
- [~] `eventbus`-Integrationstests laufen nur mit laufendem Broker (sonst `t.Skip`); unter
  `-short` übersprungen — bewusst, aber Coverage dort entsprechend niedrig.
- [ ] **Deprecation:** `observability/tracer.go` nutzt `trace.NewNoopTracerProvider` (deprecated)
  → auf `trace/noop.NewTracerProvider` umstellen.

## 3. Authorization-Härtung

- [ ] **Board-Registry-Schreibrouten** (`POST/PATCH/DELETE /api/v1/board-types`) sind derzeit
  für **jeden authentifizierten User** offen. Auf Admin-/Publisher-Rolle beschränken. —
  `boardregistry.md` (Authz-Offene Stelle), `ADR 0001`
- [ ] **WS-Board-/Task-Channel-Subscription** (`board:{id}`, `task:{id}`) wird derzeit für
  **jeden authentifizierten User** erlaubt, da der Notification-Service keine
  Board→Projekt-Zuordnung kennt und Membership nicht synchron prüfen kann. Härtung: aus
  `board.created`/`column.*`-Events eine `known_boards`-Map persistieren und in
  `checkChannelPermission` das Projekt auflösen + Membership prüfen. —
  `notification/internal/push/service.go`

## 4. Infrastruktur / Deployment

> Das aktuelle Deployment (einzelne EC2-Box, Docker-Compose-Stack, GitHub Actions → GHCR) ist
> in [`specifications/deployment.md`](specifications/deployment.md) beschrieben. Die folgenden
> Punkte sind **keine Blocker** — sie betreffen erst das Herauslösen einzelner Bausteine in
> dedizierte Managed-Services und lohnen sich erst bei entsprechender Last (siehe
> `deployment.md` §12 Skalierungs-Pfad).

- [ ] **(Niedrige Prio, spätere Erweiterung)** Zustands-Dienste in Managed-AWS-Ressourcen
  herauslösen (Postgres → RDS, Redis → ElastiCache, MinIO → S3, RabbitMQ → Amazon MQ) und/oder
  Services auf ECS/Fargate umziehen. Der `boardregistry`-Service **läuft bereits** im
  gemeinsamen Compose-Stack; ein eigener RDS-/ECS-Split ist optional. — `ADR 0001`, `deployment.md §12`

## 5. Features (bewusst nicht im MVP)

- [ ] **Board-View-Live-Switch** über `board.config` (z. B. Kanban → Kalender) + Drag-to-reschedule. — `ADR 0002`
- [ ] **Inbound-Webhooks** (externes System ruft uns) + weitere Plugin-Typen jenseits `webhook`. — `plugin.md`

---

> Pflegehinweis: Wird ein Punkt umgesetzt, hier streichen **und** die zugehörige
> „geplant/Folgearbeit/Zielkonfiguration"-Notiz in der jeweiligen Spec entfernen, damit
> Doku und Code synchron bleiben.
