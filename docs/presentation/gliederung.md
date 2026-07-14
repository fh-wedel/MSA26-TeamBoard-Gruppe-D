Titel & Einstieg
Folie 1 — Deckblatt · 0:30

Grafik: Projektname „TeamBoard" groß und mittig, darunter Untertitel „Web-basierte Kollaborationsplattform · komponentenbasiertes Microservice-System". Als Hintergrund sehr dezent (10 % Deckkraft) das Architekturbild oder eine stilisierte Kanban-Spaltenstruktur. Fußzeile: FH Wedel · M.Sc. · Modul „Moderne Softwarearchitekturen" · SS 2026 · Prof. Dr. Hoffmann. Team-Namen als Reihe unten.
Notizen: Begrüßung, Projekt in einem Satz, Team vorstellen, Ablauf ankündigen (Vortrag + Live-Demo am Ende).

Folie 2 — Agenda · 0:30

Grafik: Horizontaler nummerierter Fortschrittsbalken mit 6 Segmenten, je ein Icon: (1) Kontext & Team, (2) Anforderungen, (3) Architektur & Komponenten, (4) Realisierung & KI, (5) Deployment & Demo, (6) Fazit. Aktives Segment im Verlauf hervorgehoben (kann auf jeder Sektions-Startfolie wiederkehren als Mini-Leiste oben).
Notizen: 30-Sekunden-Durchlauf; ankündigen, dass KI-Reflexion und die Erweiterbarkeits-Architektur die zwei inhaltlichen Schwerpunkte sind.


Einleitung
Folie 3 — Kontext & Motivation · 1:30

Grafik: Zweigeteilte Folie. Links „Ausgangslage": vier lose, unverbundene Tool-Icons (Aufgaben, Dateien, Chat, Kalender) mit gestrichelten, gekreuzten Linien = Fragmentierung. Rechts „TeamBoard": ein zentraler Knoten, aus dem sternförmig Projekte/Boards/Tasks/Dokumente/Benachrichtigungen abgehen. Pfeil dazwischen beschriftet „konsolidieren".
Notizen: Vorbild Trello/Notion/Jira-Light, aber Fokus laut Aufgabe = skalierbare, komponentenbasierte Architektur, nicht Feature-Fülle. Leitfragen der Veranstaltung nennen: Echtzeit-Events verteilen, Dienste unabhängig skalieren, Datenkonsistenz. MVP-Gedanke als roter Faden.

Folie 4 — Team & Aufgabenverteilung · 1:00

Grafik: Tabelle mit 3 Spalten (Person · Schwerpunkt · verantwortete Services), 4–5 Zeilen entlang der Service-Schnitte. Rechts als schmale Sidebar ein Callout-Kasten „KI im Projekt: Opus 4.7 durchgängig als Pair-Partner (Entwurf → Code → Doku) — Reflexion in Kap. 4".

PersonSchwerpunktServices / Bereich…Auth & Gateway, Zero-TrustAuth, Traefik, shared/go/authmiddleware…Domänen-KernProject (Permissions), Task…Storage & RealtimeDocument, Notification…Erweiterbarkeit & FrontendBoard Registry, Plugin/Webhook, React-SPA

Notizen: Schnitt entlang der Bounded Contexts. Querschnitt (shared libs, Event-Schema, Deployment) im Team gemeinsam. Callout: KI wurde deklariert eingesetzt — Details später (Prof. verlangt Reflexion explizit).


Projektbeschreibung
Folie 5 — Interpretation der Aufgabenstellung · 1:30

Grafik: Links ein „Lastenheft"-Dokument-Icon mit 4–5 Original-Stichzeilen (Projekte/Boards, Tasks mit Status/Zuweisung/Fälligkeit, Kommentare, Dokumente mit Versionierung, Echtzeit-Benachrichtigungen, Plugin-Architektur, API für Fremdsysteme). Pfeil „unsere Lesart" → rechts vier Ziel-Karten: „MVP zuerst", „Architektur vor Features", „Erweiterbarkeit als First-Class-Konzept", „API-first & JSON".
Notizen: Was wir bewusst hineingelesen haben: Schwerpunkt Architektur, nicht UI-Umfang. Bewusste Scope-Grenzen (z. B. keine Echtzeit-Kollaboration im Dokument, keine Inbound-Webhooks im MVP). „Denkbare Funktionen" der Aufgabe = Auswahl, kein Pflichtkatalog.

Folie 6 — Anforderungen · 1:30

Grafik: Zwei Spalten. Funktional (Icon-Liste): Projekt-/Board-Verwaltung mit Rollen; Kanban-Tasks (Status, Zuweisung, Fälligkeit, Kommentare, Anhänge); Dokumentenablage + Versionierung; Echtzeit-Benachrichtigungen (WebSocket); Webhook-Integrationen; erweiterbare Board-Typen. Nichtfunktional (Badge-Liste, farblich abgesetzt): Zero-Trust-Sicherheit; horizontale Skalierbarkeit / Statelessness; Eventual Consistency; Erweiterbarkeit ohne Redeploy; Observability; Resilienz (Timeouts/Retry/Circuit-Breaker). Jede NFR mit kleinem Pfeil auf die spätere Architektur-Umsetzung (z. B. „Zero-Trust → JWT je Service").
Notizen: NFRs sind hier der eigentliche Kern (Architektur-Modul). Kurz je NFR erklären, wie sie im Entwurf verankert ist — schafft den Bogen zu Kap. Entwurf.

Folie 7 — User Stories · 1:00

Grafik: 4 als Kanban-Karten stilisierte Story-Karten (mit Avatar, Prioritäts-Punkt), Format „Als … möchte ich … damit …". Beispiele: (1) Projektleiter legt Board an und lädt Mitglieder mit Rollen ein; (2) Teammitglied verschiebt Task von „In Arbeit" nach „Erledigt"; (3) Autor hängt eine Datei-Version an einen Task; (4) Nutzer erhält live eine Benachrichtigung bei @Mention. Fünfte, kleinere Karte für Integrator: „CI-Pipeline registriert Webhook auf task.status.changed".
Notizen: Stories zeigen Cross-Service-Zusammenspiel (Story 2 = Task→Project-Permission→Event→Notification). Genau diese Story später in der Demo wiederverwenden.


Entwurf
Folie 8 — Architektur-Überblick · 2:30 ⭐

Grafik: Dein Architekturbild als große Schichtgrafik. Oben Browser (React-SPA) und „Externe Systeme". Darunter Traefik-Gateway-Band (Rate-Limit · CORS · Security-Header · Routing). Darunter 7 Service-Kacheln in einer Reihe: Auth · Project (★ Permission-Authority hervorgehoben) · Task · Document · Notification · Plugin/Webhook · Board Registry, jeweils mit Port. Unten Infrastruktur-Band: PostgreSQL (DB-per-Service) · Redis · RabbitMQ (teamboard.events, topic) · MinIO/S3. Drei Pfeiltypen in einer Legende unterscheiden: durchgezogen = synchron/HTTP, gestrichelt = asynchron/Event, gepunktet = WebSocket.
Notizen: Big Picture, nicht jede Linie. Fünf Kernentscheidungen benennen: Microservices entlang Bounded Contexts, DB-per-Service, Event-Driven Backbone (RabbitMQ), Gateway als Single Entry Point, Stateless als Default (nur Notification hält Zustand). Hinweis: JWT-Prüfung bewusst nicht am Gateway → Überleitung zur nächsten Folie.

Folie 9 — Leitprinzipien der Architektur · 1:30

Grafik: Raster aus 8–10 kompakten Prinzip-Kacheln, je Icon + Kurztitel + Einzeiler: Domain-Driven Design · Database-per-Service · Async-First · Stateless · Smart Endpoints/Dumb Pipes · API-first (OpenAPI/AsyncAPI) · Eventual Consistency (Outbox) · Idempotente Event-Handler · Resilience by Design · Security in Depth. Zwei, drei Kacheln (Zero-Trust, Outbox, Erweiterbarkeit) farblich als „diskussionswürdig" markiert.
Notizen: Das ist die „Diskussion der gewählten Architektur". An zwei Prinzipien Trade-offs zeigen: (a) Zero-Trust = jeder Service validiert JWT selbst (mehr Redundanz, keine implizite Netz-Vertrauensstellung); (b) Async-First = lose Kopplung, aber Eventual Consistency muss man aushalten. Abweichungen nur per ADR — Brücke zu ADR 0001/0002.

Folie 10 — Komponenten: Auth · Task · Document (Foundation) · 2:00

Grafik: Drei nebeneinanderliegende Panels gleicher Größe, je Kopf-Icon, Port, 3 Stichzeilen Verantwortung, 1 Zeile Events. Auth (8001): Registrierung/Login, RS256-JWT (Access+Refresh), JWKS-Endpoint; Events user.registered/deleted. Task (8003): Task-CRUD im Board, Statusübergänge, Kommentare, Anhang-Referenzen; Permission-Check vor jedem Schreiben; Events task.created/updated/assigned/status.changed/commented. Document (8004): Metadaten + Versionierung, Bytes fließen per Pre-signed URL direkt Client↔MinIO; Events document.uploaded/version.created. Unten je Panel ein kleiner „hängt ab von"-Chip (Auth: nur eigene DB; Task/Document: + Project für Authz).
Notizen: Bewusst kompakt — das sind die „geradlinigen" Services. Ein Detail je Service betonen: Auth = Vertrauensanker (JWKS), Task = Status-Semantik (später Board-Registry-relevant), Document = Storage läuft an den Services vorbei (Skalierung: Bytes gehen nicht durch den Service).

Folie 11 — Komponente: Project — Permission Authority · 2:00 ⭐

Grafik: Zentrale Project-Kachel (8002) in der Mitte. Von links vier eingehende Pfeile „GET /internal/…/permissions" von Task, Document, Notification, Plugin (jeder Pfeil trägt ein kleines Schloss-Symbol „Service-Token"). Rechts der Datenkern: projects · boards · project_members · roles. Darunter eine Rollen-Matrix als Mini-Tabelle: Owner (full) / Editor (Tasks+Docs full, Rest read) / Viewer (read). Unten ein Redis-Zylinder mit „Permission-Cache, TTL 30 s", eingehender gestrichelter Pfeil „project.member.* → invalidieren".
Notizen: Der architektonisch wichtigste Domänen-Service: einzige autoritative Quelle für Berechtigungen. Andere Services fragen synchron an (nur wenn der Nutzer die Antwort sofort braucht), sonst asynchron. Redis-Cache 30 s gegen Last, Event-Invalidierung statt Polling. Project ist außerdem Owner der Board-Instanzen (Boards/Spalten) — Abgrenzung zur Board Registry (Typen ≠ Instanzen). Überleitung.

Folie 12 — Komponente: Notification — Realtime & bewusst Stateful · 2:00 ⭐

Grafik: Oben mehrere Browser-Clients mit gepunkteten WebSocket-Linien zu zwei Notification-Instanzen (8005) — zeigt Skalierung. Zwischen beiden Instanzen ein Redis-Pub/Sub-Band („Backplane"). Von links unten kommt RabbitMQ mit task.* · project.* · document.* in eine Instanz; von dort ein Pfeil in die Backplane, dann an alle Instanzen, jede prüft „meine lokalen Connections?" und pusht. Rechts ein DB-Zylinder notifications (persistent, z. B. @Mentions).
Notizen: Die einzige bewusst zustandsbehaftete Komponente (offene WS-Verbindungen). Kernproblem: Event trifft Instanz A, betroffener User hängt an Instanz B → Redis-Backplane verteilt an alle. Vor dem Push wird gegen Project-Permissions gefiltert (kein Leak). Persistente Notifications zusätzlich in DB (Lesen/Markieren via REST). Skalierung: Sticky Sessions oder reconnect-tolerantes Frontend.

Folie 13 — Komponente: Plugin/Webhook — Delivery-Pipeline · 2:00 ⭐

Grafik: Vertikale Pipeline von oben nach unten, jede Stufe ein Kasten mit kurzem Label: RabbitMQ-Event → Idempotenz-Check (processed_events) → Webhook-Matching (project_id + event_filter, Wildcard task.*/*) → INSERT webhook_deliveries (Status pending, eine Zeile je Treffer) → Delivery-Worker (Poll alle 1 s, FOR UPDATE SKIP LOCKED) → HTTP POST an Empfänger → Update (delivered / failed). Rechts ein Kasten „Design: DB als Queue und Audit-Log — kein separater Job-Broker, volle SQL-Abfragbarkeit". Unten der Delivery-Request skizziert mit Headern X-TeamBoard-Event, -Delivery-Id, -Signature, -Timestamp.
Notizen: Zweck: Domain-Events → externe HTTP-Calls (CI/CD, Fremdsysteme). Abgrenzung schärfen: Output-Adapter, nicht Board-Typen (die liegen in der Registry). Warum DB-Queue statt RabbitMQ-Delayed-Plugin: eine Persistenz, kein „Job in Queue, aber nicht in DB"-Split, Multi-Worker-safe via SKIP LOCKED. Volles Event-Envelope wird gespeichert → Retry ist autonom.

Folie 14 — Plugin/Webhook — Resilienz, Sicherheit & Erweiterung · 2:00 ⭐

Grafik: Vier Quadranten. (1) Retry/Backoff: Zeitachse mit Marken 30 s → 2 min → 10 min → 30 min → 2 h → 6 h → 24 h, Klammer „±20 % Jitter, max. ~33 h → DLQ". (2) Circuit Breaker: Zustandsdiagramm Closed —[5 Fehler in Folge]→ Open —(30 s)→ Half-Open, mit Rückkanten [1 Erfolg]→Closed und [1 Fehler]→Open; Notiz „pro Ziel-URL, gobreaker". (3) HMAC: Kasten sha256(secret, "<ts>.<body>"), Header X-TeamBoard-Signature/-Timestamp, „5-min-Skew, Secret nur einmal sichtbar". (4) SSRF-Schutz: Schild-Icon, „Private/Loopback/Link-Local + Cloud-Metadata-IP (169.254.169.254) blockiert; Port-Whitelist 80/443/8080/8443; Re-Check beim Delivery gegen DNS-Rebinding". Darunter ein schmales Band „Plugin-Erweiterungspunkt": Interface Plugin{ Type() · Match() · Deliver() } + Registry; heute nur webhook, vorgesehen slack · teams · email · discord · transformer · inbound.
Notizen: Hier steckt die eigentliche Engineering-Tiefe. Sofort-DLQ-Fälle nennen (410 Gone, harte 4xx außer 408/425/429), Retry-After respektiert. SSRF ist der kritische Punkt: Validierung beim Anlegen reicht nicht, deshalb Custom-Dialer prüft die aufgelöste IP erneut beim Call. Erweiterbarkeit: Struktur sieht neue Plugin-Typen vor, ohne sie im MVP zu bauen (YAGNI) — Discriminator-Feld oder Tabelle je Typ.

Folie 15 — Komponente: Board Registry & Erweiterbarkeit · 2:30 ⭐

Grafik: Zweiteilig. Oben „Runtime-Registrierung" (ADR 0001): Vorher/Nachher — links „vorher: compile-time boardplugins/, neuer Typ = Recompile+Redeploy", rechts „nachher: Board Registry (8007), POST /board-types zur Laufzeit". Ein Entwickler-Icon schickt per REST einen neuen Typ (scrum, gantt) rein; Registry hält board_types (default_columns inkl. status, default_config, config_schema als JSON-Schema, presentation). Pfeil „boardtype.*-Event" → Project invalidiert seinen TTL-Cache (boardtypeclient, Service-Token). Unten „Präsentation & Remote-Roadmap" (ADR 0002): die presentation-Spec (view = board/calendar/timeline, view_config, card) → Frontend-View-Registry wählt Renderer (BoardView/CalendarView/GanttView), Unbekanntes → Fallback BoardView. Rechts abgesetzt und gestrichelt ein Zukunftskasten view:"remote" → Micro-Frontend / Module-Federation → „beliebige Dritt-UI ohne SPA-Redeploy".
Notizen: Das architektonische Alleinstellungsmerkmal und direkte Antwort auf die geforderte Plugin-Architektur. Kern: presentation.view ist der einzige Erweiterungspunkt — heute wählt er eingebaute Renderer, morgen remote, ohne Modell-Umbau (vorwärtskompatibel). Nebenbei sauber gelöst: Spalte→Status leckte früher über Spaltennamen (DeriveStatus); jetzt trägt jede Default-Spalte einen expliziten status, der via board.created/column.* in den Task-Service fließt. Ehrlich: Schreib-Authz der Registry ist noch offen (jeder Auth-User) → Backlog. Die geplante Remote-Erweiterung ist der spannende Ausblick.

Folie 16 — Zusammenspiel: Permission & Event-Flow · 2:00

Grafik: Zwei kompakte Sequenzdiagramme nebeneinander. Links „Synchron: Permission-Check" — Task → GET /internal/…/permissions/{userId} → Project; Project prüft Redis (Hit/Miss), antwortet {role, permissions}; Service-Token (HS256, aud:internal, 60 s) an der Pfeilspitze; Fußnote „Cache TTL 30 s, invalidiert per Event". Rechts „Asynchron: Outbox → Event" — Service schreibt Daten + Outbox in einer Transaktion → shared/go/outbox-Worker pollt (SKIP LOCKED) → publiziert Envelope an teamboard.events (topic) → wartet auf Publisher-Confirm → markiert published_at; Consumer (durable Queue + DLX) idempotent via processed_events (MessageId = event_id).
Notizen: Die zwei Kommunikationsmuster gegenübergestellt: synchron nur, wenn der Nutzer sofort eine Antwort braucht; sonst Event. Outbox garantiert „kein Event-Verlust, keine Geister-Events" (Confirm vor Commit). Idempotenz, weil Events mehrfach ankommen dürfen (P9). Ein Envelope-Beispiel kurz zeigen (event_id, type, version, trace_id, actor, payload).

Folie 17 — Repository & Artefakte · 0:45

Grafik: Links ein Verzeichnisbaum-Ausschnitt des Monorepos: services/{auth,project,…,boardregistry}/, shared/go/{authmiddleware,eventbus,outbox,servicetoken,observability,httputil}, docs/{ARCHITECTURE.md, decisions/ADR-000x, services/*.md}, frontend/, infra/traefik/, Makefile, docker-compose*.yml. Rechts großer QR-Code zum GitHub-Repo fh-wedel/MSA-26-TeamBoard + Liste der auffindbaren Artefakte (Service-Detaildocs, ADRs, OpenAPI/AsyncAPI, Demo-Notebooks).
Notizen: Monorepo-Begründung in einem Satz (atomare Cross-Service-Änderungen, geteilter Code, ein git clone/make up). Betonen: die gesamte hier gezeigte Doku ist versioniert und war Grundlage, nicht Nachdokumentation → Brücke zur KI-Reflexion.


Realisierung / Implementierung
Folie 18 — Software-Stack · 1:00

Grafik: Logo-/Badge-Raster in 5 Gruppen mit Überschrift: Backend (Go 1.22, Chi-Router, sqlc + pgx/v5, golang-migrate). Daten (PostgreSQL 16, Redis 7, RabbitMQ 3.12, MinIO/S3). Edge (Traefik lokal / AWS API Gateway). Frontend (React + TypeScript, Vite). Ops (Docker Compose, GitHub Actions, GHCR, AWS EC2, OpenTelemetry/Jaeger).
Notizen: Nicht vorlesen — zwei Begründungen: Go für schlanke, gut nebenläufige Services; sqlc = typsichere Queries ohne ORM-Magie. Query-Layer generiert, Handler handgeschrieben gegen OpenAPI-Stubs (Spec-first). Ein shared/go gegen Code-Duplikation.

Folie 19 — Vorgehen bei der Programmerstellung · 1:30

Grafik: Kreislauf-Diagramm mit vier Stationen und Pfeilen im Kreis: Planen (Feature-Design im Team) → Dokumentieren (Spec/ADR/OpenAPI aktualisieren) → Implementieren (Agent + Review) → Doku synchronisieren (Learnings zurückschreiben) → zurück zu Planen. In die Mitte des Kreises ein „Source of Truth: ARCHITECTURE.md + Service-Docs". Am Rand ein kleiner Marker „Spec-first: OpenAPI → sqlc → Handler → Tests".
Notizen: Kernprinzip Doku-first: vor dem ersten Code standen Architektur, alle Microservice-Specs, Code-Style, Doc-Style, Test-Strategie und das Zusammenspiel. Jedes Feature erst planen, dann bauen. Tests Pflicht (Domain-Unit + Repository-Integration mit Testcontainers). Das ist die Voraussetzung für die nächste Folie.

Folie 20 — Agentic Engineering: Herangehensweise · 2:00 ⭐

Grafik: Horizontale Timeline mit drei Phasen-Blöcken. (1) Gemeinsame Planung — Team + Opus 4.7 zusammen (Icon Mensch+LLM), Komponenten-Zuschnitt schon im Dialog mit dem Modell. (2) Architektur vor Implementierung — Dokument-Stapel „alle Microservices · Code-Style · Doc-Style · Testing · Architekturzusammenspiel" bevor Code entsteht. (3) Feature-Loop — wiederholtes „plan → build → doc" (verweist auf Folie 19). Unter der Timeline ein Leitsatz-Band: „Doku als Vertrag für den Agent — nicht Nachdokumentation."
Notizen: Bewusste Entscheidung, das LLM schon in der Planungsphase einzusetzen, nicht erst beim Coden. Umfangreiche Vorab-Doku diente dem Agent als verbindlicher Kontext. Learning-by-doing: der Prozess selbst wurde im Verlauf geschärft. Das erklärt, warum der Start so tragfähig war (nächste Folie).

Folie 21 — Agentic Engineering: Reflexion · 2:30 ⭐

Grafik: Zwei Spalten. Links „Lief gut" (grüne Haken): (a) Durch ausführliche Vorab-Doku direkt ein — aus unserer Sicht — gut strukturiertes und funktional relativ vollständiges Grundgerüst; (b) Struktur erlaubte schnelles Anfügen neuer Features; (c) bei regelmäßigem Doku-Update wenige Probleme durch Blind Spots. Rechts „Lief nicht so gut" (rote Warnungen): (a) Gerade nach den ersten Änderungen viele Blind Spots — Doku wurde häufig nicht automatisch mitgezogen; (b) Agent setzte anfangs nicht alles wie geplant um, teils nur „Quick Solutions", unvermerkt; (c) bei größeren Umstellungen (v. a. Frontend) kommentarlos entfernte Funktionalität trotz gegenteiliger Vorgabe. Unten quer ein hervorgehobener Fallstudien-Kasten: „Beispiel Auth: zu Beginn nur Pseudo-Validierung der Tokens — später aufwendig auf echtes RS256/JWKS nachgezogen." Ganz unten ein Learning-Band: „Agent braucht enge Reviews + Doku-Sync als Disziplin, nicht als Automatismus."
Notizen: Ehrliche Bilanz (genau das verlangt die Aufgabe). Die Kausalität betonen: guter Start wegen Doku, spätere Reibung wegen nicht-nachgezogener Doku → bestätigt das Prinzip von Folie 20 negativ wie positiv. Auth-Beispiel als konkreter Beleg (schlägt den Bogen zu Folie 10). Konsequenz: Reviews verschärft, Doku-Update als fester Schritt im Feature-Loop.

Folie 22 — Herausforderungen (jenseits KI) · 1:00

Grafik: Vier Icon-Karten: Koordination im Monorepo (mehrere Leute, ein Repo, Event-Schema als geteilter Vertrag), Verteiltes Debugging (ein Fehler über Service-Grenzen — Trace-ID hilft), Scope/Zeit (MVP vs. Nice-to-have priorisieren), Konsistenz (Eventual Consistency mental modellieren). Jede Karte mit einer Zeile „wie begegnet".
Notizen: Projektmanagement-Sicht. Ein konkretes Beispiel je Karte (z. B. Trace-ID durch Gateway → alle Services → RabbitMQ-Header machte verteiltes Debugging überhaupt handhabbar). Ehrlich: Priorisierung war der härteste Teil, TODO-Backlog dokumentiert bewusst Ausgelassenes.


Deployment / Evaluation
Folie 23 — Deployment: ein Kommando · 1:00

Grafik: Links Terminal-Mockup mit git clone …/MSA-26-TeamBoard und make up (darunter klein make seed, make logs, make down). Rechts zwei gestapelte Ziel-Umgebungen: Lokal — eine Docker-Compose-Box mit allen 7 Services + Frontend + Traefik + Postgres/Redis/RabbitMQ/MinIO/Jaeger (+ Adminer/MinIO-Console/RabbitMQ-Mgmt als Helfer), Hot-Reload via air. AWS — GitHub-Actions-Pipeline (Build-Matrix ×8 → GHCR, getaggt SHA+latest) → SSH-Deploy auf eine EC2-Box → deploy.sh (flock, .env-Secrets generieren, git reset --hard, docker compose pull && up -d). Kleiner Hinweis „Postgres/Redis/RabbitMQ/MinIO laufen als Container, nicht managed — Skalierungspfad optional".
Notizen: Aufgaben-Anforderung „ein Kommando" erfüllt: make up bringt alles hoch. Lokal HTTP, TLS erst in AWS. Ehrlich einordnen: einzelne EC2-Box, Compose-Stack — bewusst simpel; Auslagern in RDS/ElastiCache/ECS ist niedrig priorisierte Folgearbeit, kein Blocker (siehe Backlog).

Folie 24 — Laufendes System (Live-Demo) · 3:00 ⭐

Notizen: Die User-Story von Folie 7 live nachspielen. Zweites Browserfenster für den Realtime-Push (WebSocket). Wenn Zeit: Runtime-Registrierung eines neuen Board-Typs via make demo-board-types zeigen — das macht die Erweiterbarkeit greifbar. Screenshots nur, falls die Demo klemmt.

Folie 25 — Realisierter Funktionsumfang · 1:00

Grafik: Zweispaltige Checkliste. Umgesetzt (grün): alle 7 Services + Gateway; Zero-Trust-Auth (RS256/JWKS, Service-Token); Kanban-Boards mit Tasks/Kommentaren/Anhängen; Dokument-Versionierung; Realtime-Notifications; Webhooks mit Retry/Circuit-Breaker/HMAC/SSRF; Runtime-Board-Typen + Presentation-Spec; Ein-Kommando-Deployment + CI/CD. Bewusst offen / Backlog (grau): Schreib-Authz der Registry (Admin-Rolle); view:"remote" Micro-Frontends; Per-Board-View-Switch; Inbound-Webhooks + weitere Plugin-Typen; Edge-Observability am Gateway; Managed-AWS-Split.
Notizen: Klare, ehrliche Grenze MVP vs. Backlog (aus TODO.md). Betonen: „offen" heißt bewusst priorisiert, nicht vergessen — Struktur trägt diese Erweiterungen bereits.


Abschluss
Folie 26 — Fazit & Ausblick · 1:30

Grafik: Zwei Spalten. Erreicht: tragfähige Microservice-Architektur entlang Bounded Contexts; Erweiterbarkeit als First-Class-Konzept (Board Registry, Plugin-Backbone); Doku-getriebenes agentisches Vorgehen. Nächste Schritte: Remote-Board-Views (view:"remote"), Rollen-Authz-Härtung, Managed-Services/ECS, Inbound-Integrationen, Edge-Observability. In der Mitte ein hervorgehobener Kernsatz: „Doku-getriebenes Agentic Engineering skaliert — aber nur mit menschlicher Kontrolle."
Notizen: Zusammenfassende Bewertung: Architektur-Ziele des Moduls erreicht (Skalierung, Event-Verteilung, Konsistenz beantwortet). Wichtigstes Learning wiederholen. Was wir beim nächsten Mal anders machen (Doku-Sync erzwingen, kleinere Agent-Schritte reviewen).

Folie 27 — Quellenverzeichnis · 0:15

Grafik: Schlichte, zweispaltige Liste. Standards/Tools: JSON, OpenAPI/Swagger, AsyncAPI, RabbitMQ, Traefik, sqlc, Google Go Style Guide, RFC 7807. Projekt-intern: ARCHITECTURE.md, ADR 0001/0002, Service-Detaildocs, GitHub-Repo. KI: eingesetztes Modell (Opus 4.7) + Werkzeugkontext.
Notizen: Kurz stehen lassen. Explizit die KI-Werkzeuge nennen (Prof. verlangt die Angabe „welche Werkzeuge, wie eingesetzt").