# MSA26 TeamBoard – Gruppe D

TeamBoard ist eine Web-basierte Kollaborationsplattform (Projekte, Kanban-Boards, Tasks, Kommentare, Dokumente).
Diese Iteration legt das Microservice-Grundgerüst auf Basis von **FastAPI**, **PostgreSQL**, **Docker Compose** und einem statischen HTML/JS-Frontend.

## Quickstart

Voraussetzung: Docker + Docker Compose.

```bash
cp .env.example .env       # einmalig
docker compose up --build  # alles bauen und starten
```

- Web-UI: http://localhost:8080/
- Swagger pro Service (über Gateway):
  - http://localhost:8080/api/auth/docs
  - http://localhost:8080/api/projects/docs
  - http://localhost:8080/api/tasks/docs
  - http://localhost:8080/api/documents/docs
  - http://localhost:8080/api/ai/docs

Stoppen: `docker compose down`. Kompletter Reset (inkl. DB + Uploads): `make clean`.

## Architektur (Iteration 1)

```
Browser ── HTTP ──▶ gateway (8080)
                     ├─▶ auth-service        (Schema "auth")
                     ├─▶ projects-service    (Schema "projects")
                     ├─▶ tasks-service ──HTTP──▶ projects-service
                     ├─▶ documents-service ─HTTP──▶ projects-service
                     └─▶ ai-service ────────HTTP──▶ projects/tasks/documents
                                             └────▶ AWS Bedrock
                          alle ──▶ postgres (eine DB, vier Schemas)
```

- **Gateway** (`services/gateway`): FastAPI-Reverse-Proxy. Nimmt `/api/<service>/...` entgegen, leitet via `httpx` an die Backend-Services weiter und liefert das statische Frontend (`frontend/`) auf `/` aus.
- **Auth-Service** (`services/auth`): Registrierung, Login, JWT (`HS256`), `/me`. Tabellen im Schema `auth`.
- **Projects-Service** (`services/projects`): Projekte, Mitgliedschaften (Rollen `owner`/`member`), Boards, Spalten. Beim Anlegen eines Kanban-Boards werden default `To Do / In Progress / Done` erzeugt.
- **Tasks-Service** (`services/tasks`): Tasks (Status, Zuweisung, Fälligkeitsdatum, Spalte) und Kommentare. Prüft Mitgliedschaft per HTTP-Call gegen `projects-service` (Endpunkt `/internal/...`).
- **Documents-Service** (`services/documents`): Multipart-Upload, Metadaten in Postgres, Dateien auf Volume `./data/documents`. Mitgliedschaftsprüfung wie tasks-service.
- **AI-Service** (`services/ai`): Aggregiert Projektdaten aus Projects/Tasks/Documents und erzeugt über AWS Bedrock eine Projektzusammenfassung mit Risiken und nächsten Schritten.
- **Postgres**: ein Container, eine DB, vier Schemas (siehe `db/init.sql`). DB-per-Service-Trennung ist vorbereitet (jeder Service nutzt nur sein Schema, keine cross-schema FKs).

JWT wird im Auth-Service signiert und in allen anderen Services lokal verifiziert (gemeinsames Secret aus `.env`). Das Gateway reicht den Token transparent weiter.

## API-Überblick

Alle Endpunkte werden vom Gateway unter `/api/<service>/...` veröffentlicht. Auth via `Authorization: Bearer <jwt>`.

**auth** (`/api/auth/...`)

- `POST /register` `{email, password, display_name}`
- `POST /login` `{email, password}` → `{access_token}`
- `GET  /me`
- `GET  /internal/users/{user_id}` (intern, ohne Auth)

**projects** (`/api/projects/...`)

- `GET/POST  /projects`
- `GET       /projects/{id}`
- `GET/POST  /projects/{id}/members`
- `GET/POST  /projects/{id}/boards`
- `GET       /boards/{id}`
- `GET/POST  /boards/{id}/columns`
- `GET       /internal/projects/{id}/members/{user_id}` (intern)
- `GET       /internal/boards/{id}` (intern)

**tasks** (`/api/tasks/...`)

- `GET/POST  /boards/{board_id}/tasks`
- `GET/PATCH/DELETE /tasks/{id}`
- `GET/POST  /tasks/{id}/comments`

**documents** (`/api/documents/...`)

- `GET/POST  /projects/{project_id}/documents` (POST: multipart)
- `GET       /documents/{id}`
- `GET       /documents/{id}/content`
- `DELETE    /documents/{id}`

**ai** (`/api/ai/...`)

- `POST /projects/{project_id}/summary` `{include_comments, include_documents, focus?}` → Bedrock-basierte Projektzusammenfassung

OpenAPI/Swagger jeweils unter `/api/<service>/docs`.

## AWS-Integration

AWS wird dort eingesetzt, wo es für TeamBoard fachlich und architektonisch gut passt:

- **Bedrock**: `ai-service` für Projektzusammenfassungen, Risiken und nächste Schritte.
- **S3**: optionales Backend für Dokument-Binaries im `documents-service`.
- **DynamoDB**: optionales Audit-/Event-Log für schreibende API-Aufrufe im `gateway`.
- **ECS Fargate**: empfohlene Zielplattform für die FastAPI-Container.
- **API Gateway / ALB**: empfohlener öffentlicher Einstiegspunkt vor dem Gateway-Service.
- **Lambda**: sinnvoll für asynchrone S3-Dokument-Nachverarbeitung.
- **AppSync**: aktuell nicht im Runtime-Pfad, aber sinnvoll für spätere Realtime-/GraphQL-Subscriptions.

Details stehen in `docs/aws-architecture.md`.

Alle AWS-Zugriffe nutzen `boto3` und damit die normale AWS Credential Provider Chain. Es werden keine AWS-Zugangsdaten hardcodiert.

Für lokale Entwicklung mit AWS-Profil:

```bash
cp .env.example .env
# in .env:
AWS_PROFILE_NAME=dein-profilname
AWS_REGION=eu-central-1
BEDROCK_MODEL_ID=anthropic.claude-3-haiku-20240307-v1:0
docker compose up --build
```

Die Compose-Datei mountet `${USERPROFILE}/.aws` read-only nach `/root/.aws`, damit Profile im Container verfügbar sind. Alternativ können Standard-AWS-Umgebungsvariablen oder auf AWS später IAM Roles/Task Roles genutzt werden.

S3-Dokumentablage aktivieren:

```bash
DOCUMENT_STORAGE_BACKEND=s3
DOCUMENT_S3_BUCKET=dein-teamboard-bucket
DOCUMENT_S3_PREFIX=teamboard/documents
```

DynamoDB-Audit-Logging aktivieren:

```bash
AUDIT_LOG_BACKEND=dynamodb
AUDIT_DYNAMODB_TABLE=teamboard-audit-events
```

Für Offline-Tests ohne AWS:

```bash
AI_PROVIDER=mock docker compose up --build
```

## Test-Clients

### 1. Web-UI

`http://localhost:8080/` – Login/Register, Projekte, Boards, Kanban-Tasks (Spalten-Wechsel via Dropdown), Kommentare, Dokumenten-Upload/-Download und AI-Projektzusammenfassung über Bedrock.

### 2. Smoke-Test

Voraussetzung: Python 3.10+ und `pip install httpx`.

```bash
make test
# oder direkt:
python tests/smoke_test.py http://localhost:8080
```

Geht den kompletten Flow durch (Register → Login → Projekt → Board → Spalte → Task → Move → Kommentar → Upload → Download).

Optional inklusive AI-Service:

```bash
TEAMBOARD_SMOKE_AI=1 python tests/smoke_test.py http://localhost:8080
```

### 3. Postman

`postman/TeamBoard.postman_collection.json` importieren. Variable `baseUrl` ist auf `http://localhost:8080` voreingestellt; Token, Projekt-, Board-, Column-, Task- und Document-IDs werden in der Reihenfolge der Requests automatisch in Collection-Variablen geschrieben.

## Repository-Struktur

```
.
├── docker-compose.yml
├── Makefile
├── .env.example
├── db/init.sql
├── services/
│   ├── gateway/      # API-Gateway + statisches Frontend
│   ├── auth/         # Registrierung, Login, JWT
│   ├── projects/     # Projekte, Members, Boards, Columns
│   ├── tasks/        # Tasks, Comments
│   ├── documents/    # Datei-Upload/-Download
│   └── ai/           # AWS Bedrock Projektassistent
├── docs/aws-architecture.md
├── infra/lambda/     # Beispiel-Lambda für S3-Dokument-Events
├── frontend/         # statisches UI (HTML/CSS/Vanilla-JS)
├── tests/smoke_test.py
└── postman/TeamBoard.postman_collection.json
```

## Make-Targets

- `make up`       – `docker compose up --build -d`
- `make down`     – Container stoppen
- `make logs`     – Logs aller Services
- `make rebuild`  – Images ohne Cache neu bauen
- `make test`     – Smoke-Test gegen `http://localhost:8080`
- `make clean`    – Container + Volumes + lokale Dokumente löschen

## CI

Eine minimale GitHub-Actions-Pipeline liegt unter `.github/workflows/ci.yml`. Sie prüft Python-Syntax, die Postman-Collection und die Docker-Compose-Konfiguration.

## Bewusst (noch) nicht enthalten

Folgendes ist für Folgeiterationen geplant und in Iteration 1 weggelassen:

- Echtzeit-Benachrichtigungen (WebSockets/SSE, Message Broker/AppSync Subscriptions).
- Versionierung in der Dokumentenablage.
- Plugin-Architektur (weitere Board-Typen, Webhooks).
- Feiner aufgelöstes Rechte-/Rollensystem.
- Eigene Postgres-DB pro Service, Alembic-Migrationen.
- Automatisches Deployment auf einen Internet-Server.

## KI-Einsatz im Entwicklungsprozess (Reflexion – Stub)

Der initiale Aufbau dieses Repositories wurde mit einem KI-Coding-Assistenten (Cascade / Claude) erarbeitet:

- Architektur-Entwurf, Service-Zuschnitt und Datenmodell wurden im Dialog erarbeitet.
- Boilerplate (FastAPI-Skelette, SQLAlchemy-Modelle, Pydantic-Schemas, Docker-Setup, Frontend-Grundgerüst) wurde KI-generiert und manuell geprüft.
- Zusätzlich wurde AWS Bedrock als AI-Service integriert, um Projektdaten zusammenzufassen, Risiken zu erkennen und nächste Schritte vorzuschlagen.
- AWS-Services wurden nach Best-Fit-Prinzip ergänzt: S3 für Dokumente, DynamoDB für Audit-Events, ECS/API Gateway/Lambda/AppSync als Zielarchitektur bzw. Extension Points.
- Grenzen: Cross-Service-Konsistenz (z. B. tatsächliches Verhalten der Member-Prüfung), Login/UX-Edgecases sowie Performance-/Lasttests wurden nicht von der KI verifiziert und müssen manuell getestet werden.

Diese Reflexion wird in den Folgeiterationen erweitert.
