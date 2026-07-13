# Deployment & CI/CD — GitHub Actions und AWS EC2

> **Zweck:** Spezifikation der tatsächlich implementierten Continuous-Delivery-Pipeline und des Produktions-Deployments. Beschreibt, wie ein Push automatisch Images baut, in die Registry lädt und auf eine öffentliche AWS-EC2-Instanz ausrollt.
>
> **Pfad:** `.github/workflows/deploy.yml`, `deploy.sh`, `docker-compose.prod.yml`, `.env.prod.example` im Repo-Root
> **Stand:** 2026-07
>
> **Deployment-Modell:** Eine einzelne, selbst-provisionierende AWS-EC2-Box fährt den kompletten Docker-Compose-Stack. GitHub Actions baut alle Images nach GHCR und rollt per SSH aus. Bewusst schlank gehalten (MVP): der komplette Deploy-Pfad steckt in zwei lesbaren Dateien (`deploy.yml`, `deploy.sh`).

---

## Inhaltsverzeichnis

1. [Überblick](#1-überblick)
2. [Designprinzipien](#2-designprinzipien)
3. [CI/CD-Pipeline (`deploy.yml`)](#3-cicd-pipeline-deployyml)
4. [Build- & Push-Job](#4-build--push-job)
5. [Deploy-Job (SSH)](#5-deploy-job-ssh)
6. [Deploy-Skript (`deploy.sh`)](#6-deploy-skript-deploysh)
7. [Produktions-Overlay (`docker-compose.prod.yml`)](#7-produktions-overlay-docker-composeprodyml)
8. [Secrets & Konfiguration](#8-secrets--konfiguration)
9. [AWS-Infrastruktur](#9-aws-infrastruktur)
10. [Erstinbetriebnahme (Bootstrap)](#10-erstinbetriebnahme-bootstrap)
11. [Betrieb & Troubleshooting](#11-betrieb--troubleshooting)
12. [Skalierungs-Pfad](#12-skalierungs-pfad)

---

## 1. Überblick

```
  git push  ─────────────►  GitHub Actions
                                 │
              ┌──────────────────┴───────────────────┐
              │  Job 1: build (matrix × 8)            │
              │  Go/Frontend-Images bauen             │
              │  → push zu GHCR (:<sha> + :latest)    │
              └──────────────────┬───────────────────┘
                                 │  needs: build
              ┌──────────────────┴───────────────────┐
              │  Job 2: deploy (SSH)                  │
              │  appleboy/ssh-action → EC2            │
              │  provisioniert Box, checkout <sha>,   │
              │  ruft deploy.sh auf                   │
              └──────────────────┬───────────────────┘
                                 │
                        AWS EC2 (Amazon Linux)
              ┌──────────────────┴───────────────────┐
              │  deploy.sh                            │
              │  docker compose pull && up -d         │
              │  (base + prod overlay, GHCR-Images)   │
              └───────────────────────────────────────┘
```

**Zwei Artefakt-Ströme:** Der **Image-Strom** geht über die GitHub Container Registry (GHCR, `ghcr.io/<owner>/teamboard-<service>`). Der **Konfigurations-Strom** (Compose-Dateien, Migrations, Traefik-Config, Init-Skripte) kommt über einen `git checkout` desselben Commits direkt auf die Box. Beide werden über den Git-SHA als gemeinsamen `TAG` verklammert, sodass Images und Config immer zur selben Revision passen.

---

## 2. Designprinzipien

**Push-to-Deploy, ein Environment.** Jeder Push auf **jeden** Branch baut und deployt auf dieselbe Box — sie fährt immer den zuletzt gepushten Stand (`on: push`). Das ist bewusst für ein MVP/Demo-Setup gewählt. Einschränkung auf `main` ist ein Einzeiler (`on: { push: { branches: [main] } }`, siehe Kommentar im Workflow).

**Ein Deploy zur Zeit.** Auf CI-Ebene serialisiert eine `concurrency`-Gruppe (`cancel-in-progress: true`) — ein neuerer Push canceld einen laufenden. Auf der Box serialisiert zusätzlich ein `flock` in `deploy.sh` (Wartezeit bis 10 min), falls sich Läufe überlappen.

**Selbst-provisionierend.** Eine frische oder zurückgesetzte EC2-Box braucht **kein** manuelles Setup. Der SSH-Schritt installiert Git, Docker und das Compose-Plugin idempotent, klont das Repo und `deploy.sh` generiert beim ersten Lauf `/opt/teamboard/.env` mit zufälligen Secrets.

**Reproduzierbare Verklammerung.** Images tragen den Git-SHA als Tag; die Box checkt exakt denselben SHA aus. Kein Drift zwischen laufendem Code und Config-Files.

**Kein Build auf der Box.** Domain-Services und Frontend laufen in Produktion ausschließlich aus vorgebauten GHCR-Images (`image:` statt `build:` im Prod-Overlay). Die Box zieht nur.

---

## 3. CI/CD-Pipeline (`deploy.yml`)

Datei: `.github/workflows/deploy.yml`

```yaml
on:
  push:                 # jeder Branch
  workflow_dispatch:    # manueller Trigger

concurrency:
  group: build-deploy
  cancel-in-progress: true
```

Zwei Jobs, sequenziell verknüpft:

| Job | Runner | Zweck |
|-----|--------|-------|
| `build` | `ubuntu-latest` (Matrix × 8) | Images bauen und nach GHCR pushen |
| `deploy` | `ubuntu-latest` | Per SSH auf die EC2-Box ausrollen (`needs: build`) |

---

## 4. Build- & Push-Job

Läuft als **Matrix** über acht Ziele:

```
auth · project · task · document · notification · plugin · boardregistry · frontend
```

`fail-fast: false` — schlägt ein Service-Build fehl, laufen die anderen weiter.

Schritte pro Matrix-Zelle:

1. **Checkout** (`actions/checkout@v4`).
2. **Lowercase owner** — GHCR verlangt einen kleingeschriebenen Namespace; `GITHUB_REPOSITORY_OWNER` wird nach `$OWNER` normalisiert.
3. **Login zu GHCR** (`docker/login-action@v3`) mit dem eingebauten `GITHUB_TOKEN` (Permission `packages: write`) — **keine** langlebigen Registry-Credentials nötig.
4. **Buildx** einrichten (`docker/setup-buildx-action@v3`).
5. **Build & Push** (`docker/build-push-action@v6`):
   - **Context/Dockerfile:** Frontend baut aus `./frontend` + `frontend/Dockerfile`; alle Backend-Services aus dem Repo-Root mit `services/<name>/Dockerfile` (Zugriff auf `shared/go` via Root-Context).
   - **Target:** `production` (Multi-Stage-Build).
   - **Tags:** `ghcr.io/<owner>/teamboard-<service>:<git-sha>` **und** `:latest`.
   - **Cache:** GitHub-Actions-Cache pro Service-Scope (`cache-from`/`cache-to type=gha, mode=max, scope=<service>`).

Ergebnis: acht Images unter `ghcr.io/<owner>/teamboard-*`, jeweils mit SHA- und `latest`-Tag.

---

## 5. Deploy-Job (SSH)

`needs: build` — startet erst, wenn **alle** Images gepusht sind. Permissions: `packages: read` (die Box zieht mit einem ephemeren Token aus GHCR).

Verbindung via `appleboy/ssh-action@v1`:

| Parameter | Wert |
|-----------|------|
| `host` | `secrets.EC2_HOST` |
| `username` | `ec2-user` |
| `key` | `secrets.EC2_SSH_KEY` |
| `command_timeout` | `15m` |

An die Remote-Shell durchgereichte Env-Variablen (`envs:`): `TAG` (Git-SHA), `REGISTRY` (`ghcr.io/<owner>`), `GHCR_USER` (`github.actor`), `GHCR_TOKEN` (`GITHUB_TOKEN`), `REPO`.

Das inline `script:` provisioniert die Box **idempotent**:

1. **Tools** — `git`, `docker` und das Compose-Plugin per `dnf` installieren, falls nicht vorhanden; Docker-Dienst aktivieren; den User zur `docker`-Gruppe hinzufügen.
2. **Repo** — bei fehlendem `/opt/teamboard/.git`: Klon über `https://x-access-token:${GHCR_TOKEN}@github.com/${REPO}.git` (funktioniert auch für private Repos). Danach Remote-URL mit dem aktuellen ephemeren Token auffrischen, `fetch --all`, `checkout --force "$TAG"`.
3. **Deploy** — `bash /opt/teamboard/deploy.sh`. Ist die frisch hinzugefügte `docker`-Gruppe in der aktuellen Session noch nicht aktiv (`docker info` schlägt fehl), wird via `sg docker -c ...` gewrappt.

> **Wichtig:** `GHCR_TOKEN` ist das eingebaute `GITHUB_TOKEN` — kurzlebig, endet mit dem Workflow-Lauf. Es wird sowohl zum Klonen des privaten Repos als auch für den GHCR-Login auf der Box verwendet. Es liegt zu keinem Zeitpunkt dauerhaft auf der Box.

---

## 6. Deploy-Skript (`deploy.sh`)

Läuft **auf der Box** als `ec2-user`, aufgerufen vom SSH-Schritt. Erwartet die Env-Variablen `TAG`, `REGISTRY`, `GHCR_USER`, `GHCR_TOKEN`. Idempotent.

Ablauf:

1. **Lock** — `flock -w 600` auf `/tmp/teamboard-deploy.lock` serialisiert parallele Deploys (Wartezeit 10 min, sonst Abbruch).
2. **Secrets bei Erstlauf** — existiert `/opt/teamboard/.env` nicht, wird es mit zufälligen Secrets generiert (`openssl rand`): Postgres-, RabbitMQ-, MinIO-Passwörter (hex → URL-safe für Connection-Strings), `KEY_ENCRYPTION_KEY` (base64-32), `SERVICE_TOKEN_SECRET` (hex-32), Seed-Passwörter. `chmod 600`. Die Datei ist gitignored und **überlebt** damit jeden `git reset`.
3. **Config-Sync** — `git fetch --all` + `git reset --hard "$TAG"` (Fallback `origin/$TAG`). Holt Compose-Files, Migrations, Init-Skripte und Traefik-Config passend zu den Images. `.env` bleibt unberührt (gitignored).
4. **Config laden** — `.env` sourcen (`set -a`), `TAG`/`REGISTRY` exportieren.
5. **`PUBLIC_HOST` ableiten** — falls nicht in `.env` gesetzt, via **IMDSv2** aus EC2-Instance-Metadata (`http://169.254.169.254/.../public-ipv4`). Wird für presigned MinIO-URLs und Passwort-Reset-Links gebraucht.
6. **GHCR-Login** — `docker login ghcr.io` mit dem durchgereichten `GHCR_TOKEN` (nur wenn gesetzt).
7. **Deploy** — `docker compose -f docker-compose.yml -f docker-compose.prod.yml pull && up -d --remove-orphans`.
8. **Aufräumen** — `docker image prune -a -f` gibt Speicher alter SHA-getaggter Images frei (relevant auf kleinen EC2-Volumes).

---

## 7. Produktions-Overlay (`docker-compose.prod.yml`)

Wird **zusätzlich** zur Basis (`docker-compose.yml`) geladen und überschreibt nur die produktionsrelevanten Unterschiede. Die Basisdatei liefert die komplette Infrastruktur (Postgres, Redis, RabbitMQ, MinIO, Traefik, Migrate/Init-Container) — siehe [orchestration.md](orchestration.md).

Unterschiede zum lokalen Dev-Setup:

| Aspekt | Dev (`override`) | Prod (`prod`) |
|--------|------------------|---------------|
| Service-Images | lokaler `build:` | `${REGISTRY}/teamboard-<svc>:${TAG}` aus GHCR |
| Frontend | eigener Port `3000` | **durch Traefik** auf `:80` (kein eigener Public-Port), Router `PathPrefix(/)` mit `priority=1` (niedrigste, damit `/api/*` und `/ws` gewinnen) |
| Webhook-SSRF-Guards | `ALLOW_INSECURE_HTTP=true`, `ALLOW_PRIVATE_URLS=true` | **beide `false`** (öffentliche Box, SSRF-Schutz aktiv) |
| MinIO presigned URLs | `localhost` | `S3_PUBLIC_ENDPOINT=http://${PUBLIC_HOST}:9000` |
| Passwort-Reset-Link | `localhost:3000` | `http://${PUBLIC_HOST}/reset-password` |

Per-Deploy injizierte Variablen: `REGISTRY` und `TAG` (von CI), `PUBLIC_HOST` (auto-abgeleitet oder in `.env`). App-Secrets kommen aus `/opt/teamboard/.env`.

---

## 8. Secrets & Konfiguration

**GitHub Repository Secrets** (Settings → Secrets and variables → Actions):

| Secret | Zweck |
|--------|-------|
| `EC2_HOST` | Öffentliche IP/DNS der EC2-Box (SSH-Ziel) |
| `EC2_SSH_KEY` | Privater SSH-Schlüssel für `ec2-user` |
| `GITHUB_TOKEN` | **eingebaut**, kein manuelles Anlegen — GHCR-Push/-Pull + privater Repo-Klon |

**Box-Secrets** (`/opt/teamboard/.env`, siehe `.env.prod.example`): werden bei Erstlauf von `deploy.sh` zufällig generiert und liegen `chmod 600`, gitignored. Enthalten Postgres-/RabbitMQ-/MinIO-Credentials, `KEY_ENCRYPTION_KEY` (AES-256 für RSA-Signing-Keys at rest), `SERVICE_TOKEN_SECRET` (HS256, von allen Services geteilt für Service-Token) und Seed-Passwörter.

> **Nie committen:** Ein echtes `.env` gehört niemals ins Repo. `.env.prod.example` dokumentiert nur die Struktur.

---

## 9. AWS-Infrastruktur

Das umgesetzte MVP läuft auf **einer** EC2-Instanz — kein ECS, kein RDS, kein CDK.

| Ressource | Rolle |
|-----------|-------|
| **EC2-Instanz** (Amazon Linux, `dnf`-basiert) | Einziger Compute-Host; fährt den kompletten Docker-Compose-Stack (7 Services + Frontend + Postgres/Redis/RabbitMQ/MinIO/Traefik) |
| **Elastic IP** (empfohlen) | Stabile `PUBLIC_HOST` für presigned URLs und Reset-Links; sonst wechselt die IP bei jedem Stop/Start |
| **Security Group** | Eingehend nötig: **22** (SSH, idealerweise auf GitHub-Actions-/Admin-IPs beschränkt), **80** (Traefik/HTTP), **9000** (MinIO — presigned Document-Downloads landen direkt beim Browser) |
| **IAM / Instance-Metadata** | IMDSv2 wird für die `PUBLIC_HOST`-Ableitung genutzt (keine speziellen IAM-Rechte erforderlich) |
| **EBS-Volume** | Persistente Docker-Volumes (`postgres-data`, `redis-data`, `rabbitmq-data`, `minio-data`); `docker image prune` hält den Verbrauch in Schach |

**Datenhaltung:** Postgres, Redis, RabbitMQ und MinIO laufen als Container **auf** der Box (nicht als Managed Services), mit benannten Docker-Volumes auf dem EBS-Volume. Eine Postgres-Instanz mit sieben logischen Datenbanken (`auth_db` … `boardregistry_db`).

**Netzwerk-Ingress:** Traefik ist der einzige HTTP-Eintrittspunkt (`:80`) und routet per Docker-Provider-Labels an die Services; der Frontend-Router hat die niedrigste Priorität und fängt alles ab, was nicht `/api/*` oder `/ws` ist. Port `9000` ist zusätzlich offen, weil der Browser presigned MinIO-URLs direkt lädt.

---

## 10. Erstinbetriebnahme (Bootstrap)

Für eine **frische** Box ist praktisch nichts manuell zu tun — das ist der Kern des selbst-provisionierenden Designs:

1. EC2-Instanz starten (Amazon Linux), Security Group mit Ports 22/80/9000, SSH-Keypair hinterlegen.
2. (Empfohlen) Elastic IP zuweisen.
3. In GitHub die Secrets `EC2_HOST` und `EC2_SSH_KEY` setzen.
4. Beliebigen Commit pushen (oder Workflow manuell via `workflow_dispatch` starten).

Der Deploy-Job installiert dann Docker/Git/Compose, klont das Repo nach `/opt/teamboard`, generiert `.env` mit Secrets und bringt den Stack hoch. Migrations und MinIO-Bucket werden von den Init-Containern der Basis-Compose (`migrate`, `minio-init`) erledigt.

Optional danach: `make seed` bzw. das Seed-Skript für Demo-Daten (`alice@teamboard.local`).

---

## 11. Betrieb & Troubleshooting

Alle Kommandos auf der Box im Verzeichnis `/opt/teamboard`:

```bash
# Status / Logs
docker compose -f docker-compose.yml -f docker-compose.prod.yml ps
docker compose -f docker-compose.yml -f docker-compose.prod.yml logs -f <service>

# Manueller Re-Deploy eines Tags (setzt CI-Env-Vars voraus)
TAG=<git-sha> REGISTRY=ghcr.io/<owner> \
  GHCR_USER=<user> GHCR_TOKEN=<token> bash deploy.sh
```

| Symptom | Ursache / Fix |
|---------|---------------|
| Deploy-Job hängt / Timeout | `command_timeout: 15m` überschritten — meist langsamer Image-Pull; Box-Netzwerk/Disk prüfen |
| „another deploy is already running“ | `flock` — ein Vorgänger-Deploy läuft noch; warten oder Lock (`/tmp/teamboard-deploy.lock`) prüfen |
| presigned Document-Download 403/timeout | `PUBLIC_HOST` falsch oder Port 9000 in der Security Group zu; `S3_PUBLIC_ENDPOINT` im Prod-Overlay prüfen |
| Reset-Link zeigt auf `localhost` | `PUBLIC_HOST` nicht abgeleitet (nicht auf EC2 oder IMDSv2 blockiert) → in `.env` setzen |
| Login schlägt nach Deploy fehl (401) | JWT-`iss`/`aud`-Mismatch zwischen Deploy-Config und Code-Defaults — siehe Memory *JWT issuer/audience values* |
| Volle Disk | Alte Images; `docker image prune -a -f` (läuft automatisch am Ende von `deploy.sh`) |

---

## 12. Skalierungs-Pfad

Das aktuelle Modell ist bewusst auf **eine** Box optimiert. Wenn Last oder Verfügbarkeitsanforderungen es erzwingen, sind die naheliegenden Schritte — ohne den Anwendungscode anzufassen, da die Services zustandslos hinter Traefik hängen und Konfiguration ausschließlich über Env-Variablen läuft:

| Engpass | Nächster Schritt |
|---------|------------------|
| Zustands-Container konkurrieren mit Services um Box-Ressourcen | Postgres → **RDS**, Redis → **ElastiCache**, MinIO → **S3**, RabbitMQ → **Amazon MQ** herausziehen; Services zeigen per geänderter `*_URL`-Env-Var dorthin |
| Eine Box ist Single Point of Failure | Mehrere Instanzen hinter einem Load Balancer; Traefik-Routing bleibt, Service-Discovery-Provider tauschen |
| Manuelle Kapazitätssteuerung | Container-Orchestrierung (ECS/Fargate oder Kubernetes); die vorhandenen `production`-Images laufen unverändert weiter |
| Ein gemeinsames Environment | Branch-Filter im Workflow + zweite Box/Stage für Staging vs. Prod |

Kein Schritt ist ein Rewrite — jeder ersetzt genau eine Kante (Registry, Datenbank-Endpoint, Orchestrator) im hier dokumentierten Aufbau.
