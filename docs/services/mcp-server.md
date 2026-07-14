# MCP Server — Detail-Design

> **Komponente:** `mcp-server`
> **Port (HTTP-Transport):** 8080 (intern; extern über Traefik unter `/mcp`)
> **Datenbank:** keine
> **Modulpfad:** `github.com/teamboard/services/other/mcp-server`
> **Bedienung/Setup:** [`mcp-server/README.md`](../../services/other/mcp-server/README.md)

---

## 1. Verantwortung und Abgrenzung

Der MCP-Server ist ein **Protokoll-Adapter** — konkret ein *Backend-for-Frontend (BFF)* bzw.
*Anti-Corruption Layer* für MCP-Clients: er übersetzt das MCP-/Tool-Calling-Modell in die internen
REST-Verträge des Systems und ist damit bewusst **kein Domain-Service** (kein eigener Bounded
Context, kein DB, keine Events — vgl. §1.2). Das „Frontend", für das er dient, ist hier der
LLM-Client statt einer Web- oder Mobile-App.

Funktional stellt er TeamBoard-Daten als [MCP](https://modelcontextprotocol.io)-Tools bereit,
damit ein MCP-Client (Claude Desktop, Claude Code) Projekte, Boards, Tasks und Board-Typen eines
Users **lesen und teilweise verwalten** kann — authentifiziert über ein
[Personal Access Token (PAT)](auth.md#17-personal-access-tokens-pat) statt über eine Login-Session.
Weil er ausschließlich das User-PAT nutzt (on-behalf-of) und **kein** internes Service-Token, bleibt
er ein Edge-Consumer im Zero-Trust-Sinn und erhält keine erweiterte Vertrauensstellung.

### 1.1 Verantwortet
- Übersetzung von MCP-Tool-Aufrufen in HTTP-Requests gegen die **Gateway-Routen** (`/api/v1/...`),
  exakt die, die auch das Frontend nutzt (`frontend/src/api/client.ts`).
- Zwei Transporte (siehe §3): multi-tenant **HTTP** (deployt) und single-user **stdio** (lokal).
- Durchreichen des PAT als `Authorization: Bearer`-Header an das Gateway.

### 1.2 Verantwortet NICHT
- **Kein Domain-Service:** keine eigene DB, keine Events (weder Publisher noch Consumer), kein
  Eintrag im Aggregat-/Event-Katalog. Er hält keinerlei Zustand.
- **Keine eigene Authentifizierung/Autorisierung.** Der Server validiert das PAT nicht selbst und
  kennt keine Permissions — er reicht das Token weiter. Gültigkeit (Introspection) und
  Berechtigungen prüfen die Domain-Services hinter dem Gateway. Ein Aufruf sieht deshalb **genau
  das, was der Token-Eigentümer sieht** (dieselben Memberships/Rollen wie im Web).
- **Keine PAT-Ausgabe/-Verwaltung** — das liegt im Auth-Service (`/auth/tokens`, Frontend →
  Settings). Siehe [`auth.md` §17](auth.md#17-personal-access-tokens-pat).

---

## 2. Tools

Registriert in `internal/tools/tools.go`; der HTTP-Client liegt in `internal/teamboard/`.

| Tool | Art | Ziel-Route (Gateway) | Zweck |
|------|-----|----------------------|-------|
| `list_projects` | read | `GET /api/v1/projects` | Projekte, in denen der User Mitglied ist |
| `list_boards` | read | `GET /api/v1/projects/{id}/boards` | Boards, optional auf ein Projekt beschränkt |
| `get_board` | read | `GET /api/v1/boards/{id}` | Board-Detail (Spalten, Typ) |
| `list_tasks` | read | `GET /api/v1/boards/{id}/tasks` | Tasks eines Boards, optional nach Status/Spalte gefiltert |
| `get_task` | read | `GET /api/v1/tasks/{id}` | Task-Detail |
| `list_board_types` | read | `GET /api/v1/board-types` | Board-Typ-Katalog (built-in + custom) |
| `register_board_type` | **write** | `POST /api/v1/board-types` | Neuen Custom-Board-Typ registrieren |
| `delete_board_type` | **write** | `DELETE /api/v1/board-types/{type}` | Custom-Board-Typ löschen |

Die beiden Schreib-Tools spiegeln, was `docs/demo/presentation-board-registry.ipynb` interaktiv
tut — dieselben Board-Registry-Routen, nur aus einem LLM heraus aufrufbar. Für sie gilt dieselbe
offene Authz-Stelle wie für alle Board-Registry-Schreibrouten: derzeit für **jeden**
authentifizierten Aufrufer offen (siehe [`boardregistry.md` §3](boardregistry.md) und `docs/TODO.md`).
Ein PAT hat volle User-Rechte, kein Scope — die Tools sind dadurch nicht enger begrenzt als der
Web-Client desselben Users.

---

## 3. Transporte

Ausgewählt über `MCP_TRANSPORT` in `cmd/mcp-server/main.go`.

### 3.1 HTTP (multi-tenant, deployt) — `MCP_TRANSPORT=http`
Ein einziger Server hinter Traefik unter `/mcp`. **Streamable-HTTP-Transport** des Go-MCP-SDK;
pro Request wird der PAT aus dem `Authorization`-Header gelesen und ein frischer, an dieses Token
gebundener `mcp.Server` erzeugt. Dadurch bedient eine laufende Instanz beliebig viele Nutzer —
niemand muss ein lokales Binary bauen. Das ist die Variante, die im Docker/EC2-Deployment läuft.

> `DisableLocalhostProtection: true` ist gesetzt, weil der Server hinter Traefik auf einem
> öffentlichen Host sitzt; der DNS-Rebinding-/Localhost-Schutz des SDK würde die proxied
> Host-Header sonst ablehnen. Die Absicherung erfolgt über das Per-Request-PAT, **nicht** über die
> Netzwerkherkunft.

### 3.2 stdio (single-user, lokal) — `MCP_TRANSPORT=stdio` (Default)
Ein Prozess, den der Client spawnt; der PAT kommt einmalig über `TEAMBOARD_TOKEN`. Praktisch für
lokale Entwicklung ohne Gateway-HTTPS.

### 3.3 Konfiguration (Environment)

| Var | Modus | Default | Bedeutung |
|-----|-------|---------|-----------|
| `MCP_TRANSPORT` | beide | `stdio` | `http` (deployt, multi-tenant) oder `stdio` (lokal, single-user) |
| `TEAMBOARD_API_URL` | beide | `http://localhost` | Gateway-Basis-URL (in Compose: `http://traefik`) |
| `MCP_HTTP_ADDR` | http | `:8080` | Listen-Adresse des HTTP-Transports |
| `TEAMBOARD_TOKEN` | stdio | — | Das PAT (im http-Modus stattdessen pro Request aus dem Header) |

---

## 4. Authentifizierung (PAT-Fluss)

```
MCP-Client ──Bearer tbpat_…──▶ Traefik /mcp ──▶ mcp-server
                                                    │  reicht denselben Bearer durch
                                                    ▼
                             Traefik /api/v1/… ──▶ Domain-Service (project/task/boardregistry)
                                                    │  authmiddleware erkennt "tbpat_"-Präfix
                                                    ▼
                             POST /internal/tokens/introspect ──▶ Auth-Service → {user_id, email}
```

Der Präfix `tbpat_` erlaubt `shared/go/authmiddleware`, das Token ohne JWT-Parse-Versuch an die
Introspection zu routen (`WithPATIntrospector`). Die Domain-Services cachen das Ergebnis 30s und
sind über einen Circuit-Breaker gegen einen Auth-Ausfall abgesichert. Format, Speicherung
(SHA-256-Hash), Endpunkte, TTL und die Eventual-Consistency-Eigenschaft bei Revocation sind in
[`auth.md` §17](auth.md#17-personal-access-tokens-pat) beschrieben — hier nicht dupliziert.

**Warum opaque statt langlebigem JWT:** Ein PAT ist ein langlebiges, vom User verwalt- und
**revozierbares** Credential für einen fremden Client. Ein selbst-validierendes JWT wäre bis `exp`
nicht widerrufbar; für die Settings-UI (Liste, `last_used_at`, Revoke) braucht es ohnehin
Server-State. Damit folgt der Entwurf dem Access-/Refresh-Token-Muster (kurzlebiges RS256-JWT für
die Session, opakes Token für langlebige Credentials) — konsistent mit den bestehenden
Refresh-Tokens des Auth-Service.

---

## 5. Deployment und Gateway-Anbindung

Als Compose-Service `mcp-server` (`target: production`), von Traefik geroutet:

```yaml
labels:
  - traefik.enable=true
  - "traefik.http.routers.mcp.rule=PathPrefix(`/mcp`)"
  - "traefik.http.routers.mcp.middlewares=mcp-strip"
  - "traefik.http.middlewares.mcp-strip.stripprefix.prefixes=/mcp"
  - traefik.http.services.mcp.loadbalancer.server.port=8080
```

Erreichbar auf `:80` **und** `:443`. HTTPS ist nötig, weil der HTTP-Transport von Claude Code eine
`https://`-URL verlangt; Traefik terminiert TLS mit seinem eingebauten **self-signed**-Zertifikat
(eine Public-CA-Ausstellung via Let's Encrypt bräuchte eine echte Domain — die Box wird per IP
erreicht). Clients müssen das Zertifikat akzeptieren; Details und der Weg zu einem echten Zertifikat
stehen in [`mcp-server/README.md`](../../services/other/mcp-server/README.md) und
[`gateway.md` §4](../specifications/gateway.md#4-tls-und-zertifikate).

`TEAMBOARD_API_URL=http://traefik` — der Server ruft **über** das Gateway zurück (nicht direkt die
Services an), damit dieselben Routing-, CORS- und Rate-Limit-Regeln greifen.

---

## 6. Verwandte Dokumente
- [`mcp-server/README.md`](../../services/other/mcp-server/README.md) — Setup für Claude Desktop/Code, Env-Referenz
- [`auth.md` §17](auth.md#17-personal-access-tokens-pat) — PAT-Format, -Speicherung, -Introspection
- [`boardregistry.md`](boardregistry.md) — Ziel der `register_board_type`/`delete_board_type`-Tools
- [`gateway.md`](../specifications/gateway.md) — `/mcp`-Route und TLS/self-signed
