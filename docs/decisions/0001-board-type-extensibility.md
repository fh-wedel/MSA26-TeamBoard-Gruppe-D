# ADR 0001: Erweiterbarkeit von Boardtypen

- **Status:** Accepted
- **Kontext-Datum:** 2026-06-15
- **Betrifft:** Project-Service, neuer Board-Registry-Service, Task-Service

## Kontext und Problemstellung

Neue Boardtypen sollen ergänzbar sein, **ohne** Änderungen an Task-, Document- oder anderen Services. In der ursprünglichen Umsetzung waren Boardtypen jedoch
nur **compile-time** ergänzbar: je Typ ein Go-Package unter
`services/project/internal/boardplugins/` mit `init()`→`Register()`, aktiviert via Blank-Imports in
`main.go`. Externe Entwickler konnten somit keine Boardtypen hinzufügen, ohne den Project-Service neu
zu bauen und zu deployen.

Zusätzliche bestehende Schwäche: Der Task-Service leitete den Task-Status aus dem **Spaltennamen** ab
(`DeriveStatus`, Keyword-Matching). Boardtyp-Semantik leckte dadurch über Spaltennamen; benutzer­
definierte Spalten (z. B. „Shipped") wurden unzuverlässig auf `open` gemappt.

## Entscheidungstreiber

- Echte Laufzeit-Erweiterbarkeit durch Dritte (ohne Redeploy)
- Minimale zusätzliche Komplexität, bestehende Mechanik **ersetzen statt ergänzen**
- Saubere Concern-Trennung (Board-Struktur vs. Output-Adapter)
- Konsistenz mit bestehender Architektur (Outbox, Service-Token, gecachte Synchron-Calls)
- Korrekte Spalte→Status-Sematik über Servicegrenzen hinweg

## Betrachtete Optionen

- **Option 0 — Compile-time Registry (Status quo):** typsicher, einfach; aber neuer Typ = Recompile +
  Redeploy des Project-Service; nicht durch Dritte erweiterbar.
- **Option A — Daten-/configgetrieben im Project-Service:** Boardtypen als DB/Config + JSON-Schema im
  Project-Service. Kein Cross-Service-Call; aber vermischt Registry-Verantwortung mit dem
  Board-Owner.
- **Option B — Dedizierter Board-Registry-Service (HTTP):** Boardtyp-Definitionen in eigenem Service;
  Project-Service bezieht sie zur Laufzeit (Cache + Service-Token). Echte Laufzeit-Erweiterbarkeit,
  saubere Trennung; Preis: zusätzlicher Service + Synchron-Call (gecached) im Board-Erstellungspfad.
- **Option C — Go plugin (.so) / WASM:** Out-of-tree-Code-Plugins. Maximale Mächtigkeit, aber
  Linux-only/Build-fragil (Dev ist Windows) bzw. hoher WASM-Aufwand; höchstes Security-/Betriebsrisiko.

## Entscheidung

Gewählt wird **Option B — dedizierter `boardregistry`-Service** (Port 8007, `boardregistry_db`).

- Der Project-Service löst Boardtypen zur Laufzeit über die internal-API der Registry auf
  (`boardtypeclient`, TTL-Cache, kurzlebiges `servicetoken`-JWT mit `aud:"internal"`) und validiert die Board-Config
  gegen das vom Typ gelieferte **JSON-Schema**.
- Die alte `boardplugins`-Registry und das Legacy-Strategy-Interface wurden **entfernt**; Built-in-
  Typen (kanban/scrum/calendar) wurden als Seed in die Registry migriert.
- Cache-Invalidierung über `boardtype.*`-Events (gleiches Muster wie Permission-Cache).
- **Spalte→Status sauber gelöst:** Boardtyp-Default-Columns tragen einen expliziten `status`, der via
  `board.created`/`column.*`-Events in `known_columns.status` des Task-Service fließt und dort direkt
  genutzt wird; `DeriveStatus` bleibt nur Fallback.

## Konsequenzen

**Positiv:** Neue Boardtypen ohne Redeploy registrierbar; klare Concern-Trennung; korrektes
Status-Mapping für beliebige Spaltennamen; Project-Service bleibt Owner der Board-*Instanzen*.

**Negativ / offen:**
- Neuer Synchron-Abhängigkeit (gecached, mit klaren Fehlercodes: `invalid_board_type` 400,
  `board_type_registry_unavailable` 503) im `CreateBoard`-Pfad.
- **Authz der Schreib-Endpoints** ist aktuell „jeder authentifizierte User"; Beschränkung auf eine
  Admin-/Publisher-Rolle ist Folgearbeit.
- **AWS/CDK** für den neuen Service (RDS, ECS) ist als Folgearbeit zu ergänzen.

## Referenzen
- `docs/services/boardregistry.md`
- `docs/services/project.md §9`
- `docs/services/task.md §6.2`
