# Demo-Notebooks

Interaktive Beispiele gegen den laufenden TeamBoard-Stack.

## `board-types.ipynb` — Runtime-Registrierung von Board-Typen

Demonstriert das Kernfeature des Board-Registry-Service: neue Board-*Typen* zur Laufzeit über
die öffentliche REST-API zu registrieren, ohne Redeploy oder DB-Migration.

- **Eingebaut (aus der Migration):** `kanban`, `calendar` — `built_in`, unveränderlich.
- **Im Notebook registriert:** `scrum` (Board-View mit `config_schema`) und `gantt`
  (`timeline`-View) — als normale, editier-/löschbare Typen.

### Voraussetzungen
```bash
make up          # Stack starten (liefert kanban + calendar)
# optional: make clean-volumes && make up   # frische DB, damit scrum/gantt noch nicht existieren
```

### Ausführen
Headless über Make (benötigt Jupyter, `pip install jupyter`):
```bash
make demo-board-types
```
Oder interaktiv: `jupyter lab docs/demo/board-types.ipynb` und Zellen der Reihe nach ausführen.

Das Notebook nutzt nur die Python-Standardbibliothek (`urllib`) und meldet bereits vorhandene
Typen sauber als `409` (übersprungen), ist also gefahrlos wiederholbar.

## `webhooks.ipynb` — Webhook-Lebenszyklus

Demonstriert den Plugin/Webhook-Service (Output-Adapter): Webhook registrieren, HMAC-Signatur
verifizieren, Test-Zustellung auslösen, Deliveries inspizieren, aktualisieren,
deaktivieren/aktivieren, Secret rotieren und löschen.

### Voraussetzungen
```bash
make up && make seed     # Stack + Demo-User (alice) und Demo-Projekt
pip install requests     # das Notebook nutzt requests
```
Zusätzlich eine Empfänger-URL (z. B. von [webhook.site](https://webhook.site)) in `TARGET_URL`
eintragen. Interaktiv ausführen: `jupyter lab docs/demo/webhooks.ipynb`.
