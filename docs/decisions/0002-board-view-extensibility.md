# ADR 0002: View-/Präsentations-Erweiterbarkeit von Boardtypen

- **Status:** Accepted
- **Kontext-Datum:** 2026-06-16
- **Betrifft:** Board-Registry-Service, Frontend (Board-Rendering)
- **Baut auf:** [ADR 0001](0001-board-type-extensibility.md)
- **Fortgeführt in:** [ADR 0003](0003-remote-board-views.md) (`view:"remote"`)

## Kontext und Problemstellung

ADR 0001 machte Boardtypen **datengetrieben** erweiterbar (Default-Spalten, Status, Default-Config,
Config-Schema im Board-Registry-Service). Die **Präsentation** blieb davon entkoppelt und uniform:
`frontend/src/pages/board/BoardPage.tsx` rendert *jedes* Board als Spalten-/Kanban-Board, ohne
Fallunterscheidung nach `board.type`. Folgen: ein Gantt-Typ sah aus wie Kanban, und ein Calendar-Typ
(mit 0 Spalten) blieb leer. Ziel: die Darstellung soll **möglichst präzise durch die Boardtyp-
Definition selbst** gesteuert werden — nicht nur *welche* View, sondern *wie* sie aussieht.

## Fundamentale Randbedingung

Das Frontend ist eine **kompilierte SPA**. Ein zur Laufzeit (ohne Redeploy) registrierter Typ kann
keinen beliebigen React-Code sicher mitliefern. View-Erweiterbarkeit bewegt sich daher auf einem
Spektrum von *rein deklarativ (eingebaute Renderer)* bis *Remote-Code-Ausführung*.

## Betrachtete Optionen

- **Option A — Frontend-Slug-Mapping:** Frontend mappt bekannte Slugs auf Views. Keine Steuerung
  durch die Definition; externe Typen blieben auf Kanban. Verworfen.
- **Option B — Reiner View-Hint:** Definition wählt nur einen festen Renderer (`view: …`), nicht
  parametrierbar. Zu unflexibel.
- **Option C — Deklarative Presentation-Spec *(gewählt)*:** Definition trägt `view` + `view_config` +
  `card`; eingebaute Renderer interpretieren die Spec. Hohe Steuerbarkeit (Feld-Bindung, Gruppierung,
  Card-Felder/Farben), sicher, vollständig zur Laufzeit. Neue *Paradigmen* erfordern ein
  Frontend-Release, neue *Typen auf bestehendem Paradigma* nicht.
- **Option D — Remote-Module / Module Federation:** Definition referenziert ein remote JS-Bundle, das
  im Host-Origin läuft und daher eine starke Integrity-Prüfung (SRI-/Signatur-Pinning) erfordert; volle
  beliebige UI, aber hoher Sicherheits-/Versionierungs-/Betriebsaufwand.
- **Option E — iframe-Micro-Frontend + SDK:** Externe URL, sandboxed. Stark isoliert, aber
  Integrations-/UX-Kosten.

## Entscheidung

**Option C**, vorwärtskompatibel zu D/E. Neues JSONB-Feld `presentation` an der Boardtyp-Definition,
host-seitig gegen ein Meta-Schema validiert. Im Frontend wählt eine **View-Registry**
(`components/board/views/index.ts`) anhand von `presentation.view` einen eingebauten Renderer
(`BoardView`, `CalendarView`, `GanttView`); Unbekanntes fällt auf `BoardView` zurück.

Der Kern: **`presentation.view` ist der einzige Erweiterungspunkt.** Heute wählt es eingebaute
Renderer; ein künftiges `view:"remote"` (Option D/E) ließe sich **ohne Modell-Umbau** als weiterer
Renderer-Eintrag ergänzen. Damit reicht die Erweiterbarkeit von „deklarativ heute" bis „beliebige
Dritt-UI später" über denselben Schalter.

Begleitend: der Calendar-Typ erhält eine einzelne Bucket-Spalte (`0005_calendar_default_column`),
da der Task-Service Tasks an Spalten bindet; die CalendarView ignoriert Spalten und ordnet nach
Datum. Gantt wird ein vollwertiger Built-in (`0004_seed_presentation`).

## Konsequenzen

**Positiv:** Calendar und Gantt funktionieren; Views sind deklarativ und zur Laufzeit erweiterbar;
keine Backend-Änderung am Project-/Task-Service (additives Feld); klare Vorwärtskompatibilität.

**Negativ / offen:** Wirklich neue visuelle Paradigmen brauchen weiterhin ein Frontend-Release
(bis `view:"remote"` umgesetzt ist). Präsentation ist vorerst **typ-weit**; ein Per-Board-Override
(User schaltet sein Kanban auf Kalender) über `board.config` ist Folgearbeit. Drag-to-reschedule
(Calendar) und Dependencies (Gantt) sind bewusst out of scope. Die Schreib-Authz der Registry bleibt
offen (siehe ADR 0001).
