# ADR 0003: Remote-Board-Views (`view:"remote"`)

- **Status:** Proposed
- **Kontext-Datum:** 2026-07-16
- **Betrifft:** Board-Registry-Service (Meta-Schema), Frontend (View-Registry, neuer Host-Renderer)
- **Baut auf:** [ADR 0002](0002-board-view-extensibility.md)

## Kontext und Problemstellung

ADR 0002 machte die **Präsentation** von Boardtypen deklarativ erweiterbar: Eine Boardtyp-Definition
trägt eine `presentation`-Spec (`view` + `view_config` + `card`), und eine Frontend-**View-Registry**
(`frontend/src/components/board/views/index.ts`) wählt anhand von `presentation.view` einen
**eingebauten** Renderer (`BoardView`, `CalendarView`, `GanttView`). Das ADR benannte `presentation.view`
explizit als **einzigen Erweiterungspunkt** und hielt fest, dass ein künftiges `view:"remote"`
(Option D/E) sich „ohne Modell-Umbau" als weiterer Renderer-Eintrag ergänzen ließe.

Diese ADR beschreibt, **wie** dieser Nachtrag konkret aussehen würde. Es ist eine
Richtungsentscheidung, **keine** Implementierung: Ziel ist, den Erweiterungspfad festzuschreiben und
die offenen Design-Fragen zu benennen, bevor jemand ihn baut.

Offenes Problem aus ADR 0002 (dort als „negativ/offen"): Wirklich neue visuelle Paradigmen brauchen
weiterhin ein **Frontend-Release**. Ein zur Laufzeit registrierter Typ kann heute nur bestehende
eingebaute Views parametrieren, keine eigene UI mitbringen.

## Fundamentale Randbedingung (unverändert)

Das Frontend ist eine **kompilierte SPA**. Ein zur Laufzeit registrierter Typ darf keinen beliebigen
React-Code direkt in den Host-Bundle liefern. `view:"remote"` verlagert Fremd-UI daher hinter eine
**Host-kontrollierte Isolations- und Policy-Grenze** — der Host bleibt Autorität für Daten und Rechte.

## Entscheidung

**`view:"remote"` wird als ein weiterer, eingebauter Host-Renderer (`RemoteView`) in der bestehenden
View-Registry ergänzt.** Am Datenmodell ändert sich nichts Strukturelles: Die Registry bekommt einen
Eintrag, das Meta-Schema der Registry erlaubt zusätzlich `view:"remote"` samt zugehöriger
`view_config`-Felder.

```ts
// frontend/src/components/board/views/index.ts (Skizze — nicht umgesetzt)
const REGISTRY: Record<ViewKind, ComponentType<BoardViewProps>> = {
  board: BoardView,
  calendar: CalendarView,
  timeline: GanttView,
  remote: RemoteView,   // Host-eigener Loader/Isolator — kein Fremdcode
}
```

`RemoteView` ist ausdrücklich **kein** Drittcode, sondern ein Host-Renderer, dessen Aufgabe das Laden,
Isolieren und Vermitteln der Fremd-UI ist. Die Boardtyp-Definition referenziert nur eine URL:

```jsonc
{
  "view": "remote",
  "view_config": {
    "remote_url": "https://plugins.example/mindmap/v2/entry.js",
    "integrity": "sha384-…",        // SRI-/Signatur-Pinning
    "sandbox": "iframe",             // "iframe" | "module-federation"
    "capabilities": ["read:tasks", "write:task.status"],
    "sdk_version": "1"
  },
  "card": { "fields": ["title", "assignee"], "color_by": "priority" }
}
```

`RemoteView` erhält dieselben `BoardViewProps` (`board`, `tasks`, `presentation`) wie jeder eingebaute
Renderer und wird von `BoardPage` genauso über `resolveView` aufgelöst. Der bestehende **Fallback
greift automatisch**: Ist `remote` nicht verfügbar (Feature-Flag aus, Plugin offline, Schema-Verstoß),
fällt `resolveView` auf `BoardView` zurück — das Board bleibt nutzbar.

## Betrachtete Ausprägungen

- **Variante E — iframe-Micro-Frontend + SDK *(empfohlen als Default)*:** `RemoteView` rendert einen
  sandboxed `<iframe>` (`sandbox="allow-scripts"`, **ohne** `allow-same-origin` → eigener Origin, kein
  Zugriff auf Host-DOM/Cookies/JWT). Kommunikation über eine schmale `postMessage`-Bridge, gekapselt
  in einem Host-**Board-SDK** (`getTasks`, `onTasksChanged`, `selectTask`, `updateStatus`). Der Host
  bleibt Policy-Grenze: Schreib-Wünsche des Plugins werden gegen `capabilities` **und** die echte
  Projekt-Permission geprüft, bevor der Host den API-Call macht. Sicherheit „by construction"; Preis:
  Integrations-/UX-Aufwand (Theming, Fokus, Styling über die iframe-Grenze).
- **Variante D — Module Federation / Remote-Bundle:** `RemoteView` macht `import(remote_url)` und
  mountet die Komponente im selben React-Baum. Native Integration, kein Bridge-Overhead; Preis: Code
  läuft im **Host-Origin** → nur tragbar mit SRI-/Signatur-Pinning, strikter CSP-Allowlist, React als
  shared singleton, Error-Boundary + Timeout. Nur für **first-party/vertrauenswürdige** Bundles.

## Konsequenzen

**Positiv:** Neue visuelle Paradigmen ohne Frontend-Release möglich (schließt die offene Lücke aus
ADR 0002); additive Änderung — kein Umbau an Project-/Task-Service, Datenmodell oder den bestehenden
Views; robuster Fallback bereits vorhanden; Erweiterbarkeit reicht über **denselben Schalter**
(`presentation.view`) von „deklarativ heute" bis „beliebige Dritt-UI".

**Negativ / offen (der eigentliche Design-Aufwand — nicht der Registry-Eintrag):**

1. **Trust-Modell / Schreib-Authz der Registry:** Wer darf Remote-URLs registrieren? Hängt an der noch
   offenen Registry-Schreib-Authz (ADR 0001). Voraussichtlich Instanz-Admin oder Projekt-Owner mit
   Host-Allowlist.
2. **Capability-Vererbung:** Ein Plugin darf nie mehr können als der eingeloggte User; `capabilities`
   werden serverseitig gegen die echte Permission geschnitten.
3. **SDK als stabile öffentliche API:** Sobald Dritte darauf bauen, ist die `postMessage`-Bridge eine
   versionierte Schnittstelle (`sdk_version` in `view_config`).
4. **Isolationsgrad als Default:** iframe (E) als sicherer Default; Module Federation (D) nur für
   vertrauenswürdige first-party-Bundles.

Bis diese Punkte geklärt sind, bleibt der ADR **Proposed** und `view:"remote"` unimplementiert; die
eingebauten Views (ADR 0002) decken den aktuellen Bedarf.
