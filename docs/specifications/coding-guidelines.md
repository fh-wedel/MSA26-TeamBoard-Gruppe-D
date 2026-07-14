# TeamBoard — Coding Guidelines (Go)

> **Ziel dieses Dokuments:** Verbindliche Code-Konventionen für alle TeamBoard-Services. Adaptiert vom **[Google Go Style Guide](https://google.github.io/styleguide/go/)** mit projektspezifischen Ergänzungen.  
>
> **Geltungsbereich:** Alle Go-Module unter `services/*/` und `shared/go/`.  
> **Verbindlichkeit:** Pull Requests, die diesen Guidelines widersprechen, müssen begründet (z. B. via ADR) abweichen — sonst Block.  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Grundprinzipien](#1-grundprinzipien)
2. [Formatting und Tooling](#2-formatting-und-tooling)
3. [Naming](#3-naming)
4. [Paketstruktur und Imports](#4-paketstruktur-und-imports)
5. [Kommentare und Doc-Comments](#5-kommentare-und-doc-comments)
6. [Funktionsdesign](#6-funktionsdesign)
7. [Fehlerbehandlung](#7-fehlerbehandlung)
8. [Kontrollfluss](#8-kontrollfluss)
9. [Pointer und Werte](#9-pointer-und-werte)
10. [Interfaces](#10-interfaces)
11. [Concurrency](#11-concurrency)
12. [Context-Verwendung](#12-context-verwendung)
13. [Logging](#13-logging)
14. [Testing](#14-testing)
15. [Datenbank-Code (sqlc/pgx)](#15-datenbank-code-sqlcpgx)
16. [HTTP-Handler](#16-http-handler)
17. [Konfiguration](#17-konfiguration)
18. [Performance und Allokationen](#18-performance-und-allokationen)
19. [Security in Code](#19-security-in-code)
20. [Code-Review-Checkliste](#20-code-review-checkliste)

---

## 1. Grundprinzipien

Die fünf Leitprinzipien des Google Go Style Guide gelten 1:1 für TeamBoard:

| Prinzip | Bedeutung im Konflikt |
|---------|------------------------|
| **Clarity** | Code muss für Reviewer und zukünftige Entwickler sofort verständlich sein |
| **Simplicity** | Bevorzuge die einfachste Lösung, die funktioniert |
| **Concision** | Prägnanz, ohne Klarheit zu opfern |
| **Maintainability** | Code soll leicht änderbar sein |
| **Consistency** | Einheitlichkeit im Codebase schlägt persönliche Präferenzen |

**Im Konfliktfall:** Clarity > Simplicity > Concision. Wenn ein "cleverer" Einzeiler schwerer zu lesen ist als drei Zeilen klar geschriebenen Code, gewinnen die drei Zeilen.

**Der Konsistenzgrundsatz:** Wenn das umgebende Modul ein Pattern verwendet, das vom Style Guide abweicht, **folge dem umgebenden Pattern**, nicht dem Guide. Konsistenz innerhalb eines Moduls ist wichtiger als globale Reinheit.

---

## 2. Formatting und Tooling

### 2.1 Pflicht-Tools

Pre-Commit-Hooks führen folgendes aus:

| Tool | Zweck |
|------|-------|
| `gofmt -s` | Standard-Formatting (verbindlich) |
| `goimports` | Import-Organisation |
| `golangci-lint` | Linter-Suite (siehe `.golangci.yml` unten) |
| `sqlc generate` | Bei Änderungen in `queries/` |
| `oapi-codegen` | Bei Änderungen in `docs/api/*.openapi.yaml` |

### 2.2 `.golangci.yml` (verbindlich, identisch in allen Services)

```yaml
run:
  timeout: 5m
  tests: true

linters:
  disable-all: true
  enable:
    - errcheck         # nicht behandelte Fehler finden
    - gosimple         # vereinfachbare Konstrukte
    - govet            # Standard-Suspicious-Checks
    - ineffassign      # ungenutzte Zuweisungen
    - staticcheck      # umfassendes Static-Analysis
    - unused           # unbenutzter Code
    - bodyclose        # HTTP-Body-Close prüfen
    - contextcheck     # context.Context-Propagation
    - errorlint        # Errors korrekt wrappen
    - exhaustive       # Switch über Enums vollständig
    - gocritic         # diverse Idiom-Checks
    - gocyclo          # zyklomatische Komplexität
    - goconst          # wiederholte Strings → Konstante
    - gofmt
    - goimports
    - gosec            # Security-Checks
    - misspell         # Tippfehler in Kommentaren
    - nilerr           # nil-Check vor return err
    - prealloc         # vorab-allokierbare Slices
    - revive           # ersetzt golint
    - stylecheck       # Style-Regeln
    - unconvert        # unnötige Type-Conversions
    - whitespace

linters-settings:
  gocyclo:
    min-complexity: 15
  goconst:
    min-len: 3
    min-occurrences: 3
  errorlint:
    errorf: true
    asserts: true
    comparison: true
  revive:
    rules:
      - name: exported
        severity: warning
      - name: package-comments
      - name: error-naming
      - name: error-strings
      - name: error-return
      - name: receiver-naming
      - name: time-naming
      - name: var-declaration
      - name: var-naming
        arguments:
          - ["ID", "URL", "JWT", "API", "HTTP", "JSON", "UUID", "TLS"]

issues:
  exclude-rules:
    - path: _test\.go
      linters: [errcheck, gosec, gocyclo]
    - path: internal/repository/db/
      linters: [revive, stylecheck]   # generierter Code
    - path: internal/api/generated\.go
      linters: [revive, stylecheck]
```

### 2.3 Editor-Einstellungen

`.editorconfig` im Repo-Root:

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
indent_style = tab
indent_size = 4
insert_final_newline = true
trim_trailing_whitespace = true

[*.{yml,yaml,json,md}]
indent_style = space
indent_size = 2
```

---

## 3. Naming

### 3.1 Allgemeines

- **camelCase** für lokale Variablen, Funktions-Parameter, ungeexportierte Bezeichner
- **PascalCase** für exportierte Bezeichner (Funktionen, Typen, Konstanten)
- **Akronyme bleiben groß:** `userID`, `httpClient`, `jsonPayload`, **nicht** `userId`, `HttpClient`. Im Anfang ungeexportiert: `userID`, exportiert: `UserID`. Diese Regel ist im Linter durchgesetzt (`var-naming`).

### 3.2 Variablennamen

- **Kurze Scopes → kurze Namen:** `i`, `j`, `k` für Loop-Counter sind OK; `idx` ist auch OK; `index` in einer 3-Zeilen-Schleife ist zu viel.
- **Lange Scopes → beschreibende Namen:** Top-Level-Variable `db` ist zu wenig; `taskRepo` oder `userDB` ist besser.
- **Kein Hungarian Notation:** `iCount`, `strName`, `bDeleted` sind verboten.
- **Receiver-Namen kurz und konsistent:** Für Typ `TaskService` immer `s` (oder konsistent `t` über die ganze Datei). Niemals wechseln.

### 3.3 Funktionsnamen

- **Verben für Aktionen:** `CreateTask`, `MarkRead`
- **Substantive für Getter:** `Tasks()`, `Owner()` — **nicht** `GetTasks()` (Go-Idiom: Getter ohne `Get`-Prefix)
- **Ausnahme:** `Get`-Prefix in HTTP-Handlern und Service-zu-Service-Calls erlaubt, weil die externe Naming-Convention das verlangt: `GetPermissions`, `GetDocumentInfo`
- **`Is`/`Has`/`Can` für Boolean-Returner:** `IsAdmin()`, `HasPermission()`, `CanEdit()`

### 3.4 Typennamen

- **Suffix `-er` für Single-Method-Interfaces:** `Reader`, `Closer`, `Stringer`. Vermeide `IUser`-Style aus C#.
- **Domain-Entitäten ohne Suffix:** `Task`, `Project`, `User` — **nicht** `TaskEntity` oder `TaskModel`
- **Service-Typen:** `<Domain>Service` als Interface, `<domain>Service` als Implementation: `TaskService`/`taskService`

### 3.5 Konstanten

- **Gruppiert mit `const ()`** wenn semantisch zusammengehörig
- **PascalCase wenn exportiert,** keine `_`-Prefixe oder ALL_CAPS:
  ```go
  const MaxFileSize = 100 * 1024 * 1024  // ✓
  const MAX_FILE_SIZE = 100 * 1024 * 1024  // ✗
  ```

### 3.6 Paketnamen

- **Lowercase, ein Wort, kein Underscore:** `domain`, `repository`, `httpclient` — **nicht** `http_client` oder `httpClient`
- **Singular** wenn möglich: `model` statt `models`, `task` statt `tasks`
- **Kein generisches `util` oder `common`:** stattdessen aussagekräftiges Paket pro Verantwortlichkeit (`stringutil` ist OK; `util` nicht)

### 3.7 Datei-Naming

- **Lowercase mit Underscores:** `service_impl.go`, `task_repository.go`
- **Test-Dateien:** `<file>_test.go` — gleiche Datei wenn weiß-box, `_test`-Suffix-Paket bei black-box
- **Generierte Dateien:** `_gen.go` Suffix oder `generated.go`

---

## 4. Paketstruktur und Imports

### 4.1 Verzeichnisstruktur (verbindlich)

Identisch in jedem Service, siehe Master-Doku:

```
services/<name>/
├── cmd/server/main.go        # Wiring + nichts anderes
├── internal/
│   ├── api/                  # HTTP-Layer
│   ├── domain/               # Geschäftslogik
│   ├── repository/           # Persistenz
│   ├── events/               # Event-Konsum/Publikation
│   └── config/
```

**Regel:** `domain/` darf **niemals** aus `api/`, `repository/`, oder `events/` importieren. Andersherum sind Imports erlaubt. Das ist Hexagonal Architecture — Domain ist im Kern, Infrastruktur außen.

### 4.2 Import-Reihenfolge

Drei Gruppen, mit Leerzeile getrennt, von `goimports` automatisch sortiert:

```go
import (
    // 1. Standard Library
    "context"
    "errors"
    "fmt"
    "time"

    // 2. Externe Pakete
    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    // 3. Interne Pakete (eigenes Modul oder shared/)
    "github.com/teamboard/services/domain/task/internal/domain"
    "github.com/teamboard/shared/go/observability"
)
```

### 4.3 Verbotene Patterns

- **`.` import (Dot-Import):** ausschließlich in Test-Dateien, niemals in Produktionscode
- **`_` import (Blank-Import) für Side-Effects:** nur dokumentiert (z. B. PostgreSQL-Driver-Registrierung)
- **Zirkuläre Abhängigkeiten:** ergeben Compile-Error; das Layout oben verhindert das strukturell

### 4.4 Internal-Pakete

Alles unter `internal/` ist privat zum Service. Cross-Service-Code lebt in `shared/go/`. **Niemals** `internal/`-Code eines anderen Service importieren — nicht möglich, auch nicht mit Tricks.

---

## 5. Kommentare und Doc-Comments

### 5.1 Doc-Comments

Jeder exportierte Bezeichner braucht einen Doc-Comment. Format:

```go
// CreateTask creates a new task in the given board. It validates that the
// requester has permission to create tasks in the board's project, derives
// the task's status from the column's name, and emits a task.created event.
//
// Returns ErrBoardUnknown if the board has not yet been replicated locally
// (race condition with board.created event).
func (s *taskService) CreateTask(ctx context.Context, requester uuid.UUID, input CreateTaskInput) (*Task, error) {
    ...
}
```

**Regeln:**
- Beginnt mit dem Namen des Bezeichners
- Vollständige Sätze in Englisch (auch in TeamBoard — Code-Sprache ist Englisch)
- Beschreibt *was* die Funktion macht und *warum*, nicht *wie*
- Erwähnt nicht-offensichtliche Errors

### 5.2 Package-Comments

Jedes Paket braucht einen `// Package xxx ...`-Comment in genau einer Datei (üblicherweise gleich-benannte Datei oder `doc.go`):

```go
// Package domain contains the core business logic of the task service.
// It defines entities (Task, Comment, Attachment), use cases via the
// TaskService interface, and domain-specific errors.
//
// Domain code MUST NOT import from internal/api, internal/repository,
// or internal/events — only those packages may import this one.
package domain
```

### 5.3 Inline-Kommentare

- **Erkläre das WARUM, nicht das WAS.** `i++ // increment i` ist nutzlos.
- **TODOs immer mit Issue-Referenz und Initialen:** `// TODO(jdoe, #123): replace with proper rate limiter`
- **`// FIXME` und `// HACK`** sind Eskalations-Marker — Code-Review fragt: warum kein PR statt Marker?

---

## 6. Funktionsdesign

### 6.1 Länge

- **Soft Limit: 50 Zeilen** Funktionsbody
- **Hard Limit: 100 Zeilen** — alles darüber muss aufgeteilt werden, außer mit dokumentierter Ausnahme

### 6.2 Argument-Anzahl

- **Maximal 4 Argumente** als Reihe (`ctx` zählt nicht). Bei 5+ → Input-Struct:

```go
// Schlecht:
func CreateTask(ctx context.Context, boardID, requesterID uuid.UUID, title, description string, priority Priority, assigneeID *uuid.UUID, dueDate *time.Time, labels []string) (*Task, error)

// Gut:
func CreateTask(ctx context.Context, requester uuid.UUID, input CreateTaskInput) (*Task, error)

type CreateTaskInput struct {
    BoardID     uuid.UUID
    Title       string
    Description string
    Priority    Priority
    AssigneeID  *uuid.UUID
    DueDate     *time.Time
    Labels      []string
}
```

### 6.3 Return-Werte

- **Maximal 3 Return-Werte** (üblicherweise `(value, error)` oder `(value, cursor, error)` für Pagination)
- **Error immer als letztes**
- **Named returns sparsam:** nur wenn sie die Lesbarkeit verbessern (z. B. komplexe Funktionen mit mehreren Return-Pfaden), sonst weglassen

### 6.4 Konstruktoren

```go
// New<Type> für den Standardfall
func NewTaskService(repo Repository, projectClient ProjectClient, broker EventBroker) *taskService {
    return &taskService{
        repo:          repo,
        projectClient: projectClient,
        broker:        broker,
    }
}

// New<Type>With<Variant> für Varianten
func NewTaskServiceWithCache(repo Repository, projectClient ProjectClient, broker EventBroker, cache Cache) *taskService {
    s := NewTaskService(repo, projectClient, broker)
    s.cache = cache
    return s
}
```

### 6.5 Builder-Pattern

In Go untypisch — meist Struct-Literals besser. Ausnahme: Config-Objekte mit vielen optionalen Feldern, dann **Functional Options**:

```go
type ServerOption func(*server)

func WithTimeout(d time.Duration) ServerOption {
    return func(s *server) { s.timeout = d }
}

func WithLogger(l *slog.Logger) ServerOption {
    return func(s *server) { s.logger = l }
}

func NewServer(addr string, opts ...ServerOption) *server {
    s := &server{addr: addr, timeout: 10 * time.Second}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
```

---

## 7. Fehlerbehandlung

### 7.1 Grundregeln

- **Errors sind Werte, keine Exceptions.** Behandle sie explizit.
- **Niemals `_` für Errors** außer mit dokumentiertem Grund (`// Best effort, ignore error`).
- **Kein `panic` in Produktionscode** außer in `main.go` beim Startup, wenn Service nicht hochkommen kann.

### 7.2 Domain-Errors als Sentinel-Values

Wie in den Service-Detail-Docs spezifiziert — **immer typisierte Domain-Errors**, niemals Strings:

```go
package domain

type Error struct {
    Code    string
    Message string
    Cause   error
}

func (e *Error) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

// Sentinel-Werte
var (
    ErrTaskNotFound     = &Error{Code: "task_not_found", Message: "task not found"}
    ErrPermissionDenied = &Error{Code: "permission_denied", Message: "permission denied"}
)
```

### 7.3 Error-Wrapping

Beim Bubble-Up zwischen Layern: **Wrap mit Kontext**, ohne den ursprünglichen Error zu verlieren:

```go
// Schlecht — verliert den Original-Error
if err != nil {
    return errors.New("failed to load task")
}

// Schlecht — kein Kontext
if err != nil {
    return err
}

// Gut
if err != nil {
    return fmt.Errorf("load task %s: %w", taskID, err)
}
```

`%w` ist Pflicht für Wrapping. `%v` reicht nur für Logs, nicht für return.

### 7.4 Error-Checks

```go
// Sentinel-Check
if errors.Is(err, ErrTaskNotFound) { ... }

// Type-Assertion auf Custom-Error
var domainErr *domain.Error
if errors.As(err, &domainErr) {
    switch domainErr.Code {
    case "permission_denied": ...
    }
}

// Niemals direkt vergleichen!
if err == ErrTaskNotFound { ... }   // ✗ bricht bei wrapping
```

### 7.5 Error-Strings

- **Lowercase, kein abschließender Punkt** (Go-Konvention):
  ```go
  errors.New("task not found")           // ✓
  errors.New("Task not found.")          // ✗
  ```
- **Keine sensiblen Daten** in Error-Strings (keine Passwörter, Tokens, vollständige PII)

### 7.6 HTTP-Error-Mapping (zentral in `shared/go/httputil`)

```go
// Map domain errors to HTTP status codes
func MapErrorToHTTPStatus(err error) int {
    var domainErr *domain.Error
    if !errors.As(err, &domainErr) {
        return http.StatusInternalServerError
    }
    switch domainErr.Code {
    case "task_not_found", "project_not_found", "document_not_found":
        return http.StatusNotFound
    case "permission_denied":
        return http.StatusForbidden
    case "validation_failed":
        return http.StatusBadRequest
    case "conflict", "already_member", "last_owner_protected":
        return http.StatusConflict
    case "rate_limited":
        return http.StatusTooManyRequests
    default:
        return http.StatusInternalServerError
    }
}
```

---

## 8. Kontrollfluss

### 8.1 Early Return (Guard Clauses)

Bevorzuge frühe Returns über tiefe Verschachtelung:

```go
// Schlecht
func (s *taskService) GetTask(ctx context.Context, id, requester uuid.UUID) (*Task, error) {
    task, err := s.repo.GetTask(ctx, id)
    if err == nil {
        if !task.DeletedAt.IsZero() {
            return nil, ErrTaskNotFound
        }
        perms, err := s.projectClient.GetPermissions(ctx, task.ProjectID, requester)
        if err == nil {
            if perms.Has("task:read") {
                return task, nil
            }
            return nil, ErrPermissionDenied
        }
        return nil, err
    }
    return nil, err
}

// Gut
func (s *taskService) GetTask(ctx context.Context, id, requester uuid.UUID) (*Task, error) {
    task, err := s.repo.GetTask(ctx, id)
    if err != nil {
        return nil, err
    }
    if !task.DeletedAt.IsZero() {
        return nil, ErrTaskNotFound
    }
    perms, err := s.projectClient.GetPermissions(ctx, task.ProjectID, requester)
    if err != nil {
        return nil, fmt.Errorf("permission check: %w", err)
    }
    if !perms.Has("task:read") {
        return nil, ErrPermissionDenied
    }
    return task, nil
}
```

### 8.2 Switch über `else if`-Ketten

Drei oder mehr Bedingungen → switch:

```go
// Bevorzugt
switch {
case status == StatusDone:
    return doneStyle
case status == StatusInProgress:
    return progressStyle
default:
    return defaultStyle
}
```

### 8.3 Loops

- **Range-loop wo möglich**:
  ```go
  for i, task := range tasks {  // ✓
      ...
  }
  ```
- **`continue` und `break` mit Label** nur sparsam, nur wenn sie Klarheit bringen
- **Kein `goto`** außer in generierten Code

### 8.4 Defer

- **`defer` sofort nach Resource-Acquisition**:
  ```go
  rows, err := db.Query(ctx, q)
  if err != nil { return err }
  defer rows.Close()
  ```
- **`defer` für Locks**:
  ```go
  m.mu.Lock()
  defer m.mu.Unlock()
  ```
- **Vorsicht in Loops:** `defer` in Loop sammelt sich an. Stattdessen Helper-Funktion:
  ```go
  // Schlecht
  for _, file := range files {
      f, _ := os.Open(file)
      defer f.Close()  // sammelt sich!
      ...
  }
  
  // Gut
  for _, file := range files {
      func() {
          f, _ := os.Open(file)
          defer f.Close()
          ...
      }()
  }
  ```

---

## 9. Pointer und Werte

### 9.1 Receiver-Wahl

| Receiver-Typ | Wann |
|--------------|------|
| Value `T` | Kleine Structs (≤2 Felder), Immutable, kein Bedarf an Mutation |
| Pointer `*T` | Mutating-Methoden, große Structs, **Konsistenz innerhalb eines Typs** |

**Konsistenz-Regel:** Wenn *eine* Methode auf `*T` operiert, **alle** Methoden auf `*T`. Keine gemischten Receiver pro Typ.

### 9.2 Funktions-Argumente

- **Pointer für `out`-Parameter**: vermeide das, lieber Return-Value
- **Pointer für nullable Werte**: `*time.Time`, `*int` für "kann fehlen"
- **Slice und Map sind bereits Reference-Typen**: nicht zusätzlich `*[]string` machen
- **Großer Struct (>~100 Bytes)**: Pointer vermeidet Copy

### 9.3 Niemals Pointer auf Interface

```go
var s *Stringer  // ✗ Interface ist schon Reference
var s Stringer   // ✓
```

---

## 10. Interfaces

### 10.1 Interfaces da definieren, wo sie konsumiert werden

Nicht beim Implementer, sondern beim Aufrufer. Das vermeidet, dass jeder Service ein `UserRepository`-Interface importieren muss:

```go
// Schlecht — Interface beim Implementer
package repository

type Repository interface { /* 30 Methoden */ }
type postgresRepo struct{}
func (p *postgresRepo) ...

// Gut — Interface beim Konsumer mit minimalem Set
package domain

type taskRepository interface {
    GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
    CreateTask(ctx context.Context, t *Task) error
    // nur was Domain wirklich braucht
}

type taskService struct {
    repo taskRepository
}
```

### 10.2 Interfaces klein halten

`io.Reader` hat genau eine Methode. Single-Method-Interfaces sind Idiom. Große Interfaces (10+ Methoden) sind Code Smell — meist mehrere Verantwortlichkeiten.

Wenn ein Konsument 30 Methoden braucht, aufteilen:

```go
type taskReader interface {
    GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
    ListTasks(ctx context.Context, ...) ([]*Task, error)
}

type taskWriter interface {
    CreateTask(ctx context.Context, t *Task) error
    UpdateTask(ctx context.Context, t *Task) error
}

type taskRepository interface {
    taskReader
    taskWriter
}
```

### 10.3 `interface{}` / `any` vermeiden

- **Nur** an JSON-Marshal/Unmarshal-Grenzen, Reflection, oder genuinely-typed-as-any-Stellen
- Niemals als allgemeiner Container — Generics (Go 1.18+) bevorzugen

```go
// Schlecht
func process(items []interface{}) { ... }

// Gut
func process[T any](items []T) { ... }
```

### 10.4 Implementations-Check

```go
// Interface-Compliance compile-time prüfen
var _ TaskService = (*taskService)(nil)
```

Diese Zeile am Top der Implementation stellt sicher, dass der Compiler die Interface-Erfüllung prüft.

---

## 11. Concurrency

### 11.1 Goroutines mit klarem Lifetime

**Regel:** Jede Goroutine muss einen klar definierten Beendigungs-Mechanismus haben. Niemals "fire and forget":

```go
// Schlecht — Goroutine läuft potentiell ewig
go func() {
    for {
        s.processQueue()
        time.Sleep(1 * time.Second)
    }
}()

// Gut — Beendigung über Context
go func() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.processQueue()
        }
    }
}()
```

### 11.2 Synchronisation: Channels vs. Mutexes

**Faustregel:** Channels für *Kommunikation*, Mutexes für *Schutz von State*.

- **Mutex** wenn ein Goroutine-Set einen geteilten Datenstruktur ändert
- **Channel** wenn Goroutinen Daten weitergeben

```go
// Mutex für State-Schutz
type connectionRegistry struct {
    mu          sync.RWMutex
    connections map[string]*Connection
}

func (r *connectionRegistry) Add(c *Connection) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.connections[c.ID] = c
}

// Channel für Kommunikation
type Connection struct {
    Send chan []byte  // Outbound-Frames
}

func (c *Connection) writeLoop() {
    for msg := range c.Send {
        c.conn.WriteMessage(websocket.TextMessage, msg)
    }
}
```

### 11.3 Kein State Sharing ohne Sync

`go vet` und `-race` müssen sauber durchlaufen. CI-Pipeline führt Tests mit `-race` aus.

### 11.4 Goroutine-Leaks vermeiden

**Bei jedem `go func()` fragen:**
1. Wann beendet sich diese Goroutine?
2. Kann sie blockieren? Worauf?
3. Wird der Channel, in den sie schreibt, immer gelesen?

Häufiges Anti-Pattern:

```go
// Goroutine blockt für immer wenn niemand liest
go func() { ch <- result }()
return  // wer liest ch?
```

### 11.5 `sync/errgroup` für Parallel-Tasks

```go
import "golang.org/x/sync/errgroup"

g, gctx := errgroup.WithContext(ctx)
for _, doc := range documents {
    doc := doc  // capture
    g.Go(func() error {
        return processDocument(gctx, doc)
    })
}
if err := g.Wait(); err != nil {
    return err
}
```

---

## 12. Context-Verwendung

### 12.1 Context als erstes Argument

**Verbindlich:** `ctx context.Context` ist immer das erste Argument:

```go
func (s *taskService) CreateTask(ctx context.Context, ...) (*Task, error) { ... }
```

### 12.2 Kein Context in Structs

Context gehört in den Aufruf, nicht ins Struct:

```go
// Schlecht
type taskService struct {
    ctx context.Context
    ...
}

// Gut — Context wird pro Aufruf übergeben
type taskService struct {
    repo Repository
    ...
}
func (s *taskService) CreateTask(ctx context.Context, ...) { ... }
```

### 12.3 Context-Propagation

**Niemals** `context.Background()` oder `context.TODO()` mitten im Call-Stack. Immer den Parent-Context durchreichen:

```go
// Schlecht — neuer Background, verliert Trace-ID, Cancellation, Deadline
func (s *taskService) DoSomething(ctx context.Context) {
    s.repo.Get(context.Background(), ...)  // ✗
}

// Gut
func (s *taskService) DoSomething(ctx context.Context) {
    s.repo.Get(ctx, ...)  // ✓
}
```

**Ausnahmen, wo `context.Background()` OK ist:**
- `main.go` als Root-Context
- Background-Goroutine, die explizit über Service-Lifetime läuft

### 12.4 Context-Values

Sparsam nutzen — nur für Request-Scoped-Daten, niemals für Pflicht-Argumente:

```go
// OK: Trace-ID, User-ID aus Auth-Middleware
ctx = context.WithValue(ctx, contextKeyUserID, userID)

// Nicht OK: Geschäftsdaten
ctx = context.WithValue(ctx, "task", task)  // ✗
```

**Type-safe Context-Keys:**

```go
type contextKey int

const (
    contextKeyUserID contextKey = iota
    contextKeyTraceID
)

// Nicht
ctx = context.WithValue(ctx, "userID", id)  // String-Keys → Kollisions-Risiko
```

### 12.5 Deadlines und Timeouts

Auf jeden ausgehenden Call eine sinnvolle Deadline:

```go
ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
defer cancel()
perms, err := s.projectClient.GetPermissions(ctx, projectID, userID)
```

---

## 13. Logging

### 13.1 Verbindliche Library: `log/slog`

Stdlib seit Go 1.21. Keine externen Logger.

### 13.2 Struktur (verbindlich)

Pflicht-Felder pro Log-Entry:

```go
logger.InfoContext(ctx, "task created",
    slog.String("task_id", task.ID.String()),
    slog.String("user_id", requester.String()),
    slog.String("board_id", task.BoardID.String()),
    slog.Int64("duration_ms", duration.Milliseconds()),
)
```

`time`, `level`, `msg`, `service` (über Default-Logger gesetzt), `trace_id`, `span_id` werden automatisch von der OpenTelemetry-Bridge eingefügt.

### 13.3 Levels

| Level | Verwendung |
|-------|------------|
| `Debug` | Lokale Entwicklung, Detail-Tracing |
| `Info` | Standardmäßig sichtbar, wichtige State-Übergänge |
| `Warn` | Anomalien, die nicht zum Fehler führen, aber Aufmerksamkeit verdienen |
| `Error` | Operation gescheitert, Action erforderlich oder Retry möglich |

**Niemals `Fatal`** außer in `main.go` beim Startup.

### 13.4 Was nie geloggt wird

- **Klartext-Passwörter** (auch nicht in Errors, Debug-Dumps)
- **Tokens (JWT, Refresh, Pre-Signed-URLs)** — höchstens erste 8 Zeichen + `***`
- **Secrets, API-Keys**
- **Vollständige PII** ohne Notwendigkeit (E-Mails OK in User-Operations, aber nicht in fremden Logs)
- **Request-/Response-Bodies** unbeschränkt — bei Bedarf truncate auf 1KB

### 13.5 Error-Logging

```go
if err != nil {
    logger.ErrorContext(ctx, "create task failed",
        slog.Any("error", err),         // wraps werden automatisch entfaltet
        slog.String("user_id", uid.String()),
    )
    return err  // Error wird dort gemappt, wo's HTTP wird
}
```

**Anti-Pattern:** Error doppelt loggen. Wenn Layer A loggt und an Layer B returnt, soll Layer B nicht nochmal loggen.

### 13.6 Standard-Logger-Setup (in `shared/go/observability`)

```go
package observability

func NewLogger(serviceName, level string) *slog.Logger {
    var lvl slog.Level
    if err := lvl.UnmarshalText([]byte(strings.ToUpper(level))); err != nil {
        lvl = slog.LevelInfo
    }
    h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: lvl,
        ReplaceAttr: redactSensitiveAttrs,  // siehe Abschnitt 19
    })
    return slog.New(h).With("service", serviceName)
}
```

---

## 14. Testing

### 14.1 Pflicht-Test-Coverage

| Layer | Mindest-Coverage |
|-------|------------------|
| `internal/domain/` | 80% |
| `internal/repository/` | 70% (Integration via Testcontainers) |
| `internal/api/` | 60% (Handler + Middleware) |
| Cross-cutting (`shared/`) | 90% |

### 14.2 Test-Naming

```go
// Format: Test<Func>_<Scenario>
func TestCreateTask_HappyPath(t *testing.T) { ... }
func TestCreateTask_PermissionDenied(t *testing.T) { ... }
func TestCreateTask_BoardUnknown(t *testing.T) { ... }

// Bei Methoden-Tests: Test<Type>_<Method>_<Scenario>
func TestTaskService_CreateTask_HappyPath(t *testing.T) { ... }
```

### 14.3 Subtests via `t.Run`

```go
func TestCreateTask(t *testing.T) {
    setup := func(t *testing.T) (*taskService, *mockRepo) { ... }
    
    t.Run("happy path", func(t *testing.T) {
        svc, _ := setup(t)
        _, err := svc.CreateTask(ctx, uid, validInput)
        require.NoError(t, err)
    })
    
    t.Run("permission denied", func(t *testing.T) {
        svc, _ := setup(t)
        // ...
    })
}
```

Vorteile: gemeinsamer Setup, parallele Ausführung mit `t.Parallel()`, gefilterte Ausführung via `-run TestCreateTask/happy_path`.

### 14.4 Table-Driven Tests

Standard für mehrere Variationen derselben Logik:

```go
func TestEventFilterMatching(t *testing.T) {
    tests := []struct {
        name      string
        filter    []string
        eventType string
        want      bool
    }{
        {"exact match", []string{"task.created"}, "task.created", true},
        {"prefix wildcard", []string{"task.*"}, "task.updated", true},
        {"no match", []string{"task.created"}, "project.created", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := matchesFilter(tt.eventType, tt.filter)
            if got != tt.want {
                t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### 14.5 Assertion-Library

`github.com/stretchr/testify/require` und `.../assert`:

- **`require`** für Vorbedingungen (Test bricht ab)
- **`assert`** für eigentliche Behauptungen (Test geht weiter)

```go
require.NoError(t, err)              // bricht ab wenn Setup fehlschlägt
assert.Equal(t, expected, actual)    // sammelt alle Failures
```

### 14.6 Mocking

**Bevorzugt:** Hand-geschriebene Stubs/Fakes für Interfaces. Generierte Mocks (z. B. mockgen) nur wenn der Aufwand sich lohnt (großes Interface, viele Tests).

```go
// Stub-Implementation
type stubProjectClient struct {
    perms *PermissionSet
    err   error
}

func (s *stubProjectClient) GetPermissions(ctx context.Context, p, u uuid.UUID) (*PermissionSet, error) {
    return s.perms, s.err
}
```

### 14.7 Testcontainers für Integration-Tests

```go
func TestTaskRepository(t *testing.T) {
    if testing.Short() {
        t.Skip("integration test")
    }
    pg := startPostgresContainer(t)  // ephemeral container
    repo := NewPostgresRepository(pg.URL())
    
    t.Run("create and retrieve", func(t *testing.T) { ... })
}
```

`make test-unit` läuft `-short`, überspringt Integration-Tests. `make test-integration` läuft alle.

### 14.8 Test-Hilfsfunktionen mit `t.Helper()`

```go
func mustCreateUser(t *testing.T, repo Repository) *User {
    t.Helper()  // Reports failure at caller line
    u, err := repo.CreateUser(...)
    require.NoError(t, err)
    return u
}
```

### 14.9 Cleanup mit `t.Cleanup()`

```go
func startTestServer(t *testing.T) *httptest.Server {
    s := httptest.NewServer(handler)
    t.Cleanup(s.Close)
    return s
}
```

Bevorzugt gegenüber `defer` in Tests — `t.Cleanup` wird auch bei `t.Fatal` ausgeführt.

---

## 15. Datenbank-Code (sqlc/pgx)

### 15.1 sqlc als Single Source of Truth

- SQL-Queries leben in `queries/*.sql` mit sqlc-Annotation
- Generierter Go-Code in `internal/repository/db/` ist **read-only** — niemals manuell editieren
- Bei Query-Änderung: SQL editieren → `make generate` → committen

### 15.2 Repository-Pattern

Domain-Layer kennt nur ein **Repository-Interface**, das im Domain-Paket definiert ist (siehe Abschnitt 10.1). Implementation in `internal/repository/postgres.go` wrapped sqlc-generierten Code:

```go
// internal/domain/repository.go
type Repository interface {
    GetTask(ctx context.Context, id uuid.UUID) (*Task, error)
    CreateTask(ctx context.Context, t *Task) error
}

// internal/repository/postgres.go
type postgresRepo struct {
    q *db.Queries
}

func (r *postgresRepo) GetTask(ctx context.Context, id uuid.UUID) (*Task, error) {
    row, err := r.q.GetTask(ctx, id)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, domain.ErrTaskNotFound
        }
        return nil, fmt.Errorf("get task: %w", err)
    }
    return mapTaskFromDB(row), nil
}
```

### 15.3 Mapping-Funktionen

DB-Row → Domain-Entity in dedizierter Funktion. Keine mapping-Logik in Handlern oder Service-Methoden:

```go
func mapTaskFromDB(row db.Task) *domain.Task {
    return &domain.Task{
        ID:        row.ID,
        Title:     row.Title,
        Status:    domain.Status(row.Status),
        ...
    }
}
```

### 15.4 Transaktionen

Pattern für atomare Multi-Statement-Ops (DB-Mutation + Outbox-Insert):

```go
func (r *postgresRepo) CreateTaskWithEvent(ctx context.Context, task *Task, event *Event) error {
    tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)  // No-op if commit succeeded
    
    qtx := r.q.WithTx(tx)
    
    if err := qtx.CreateTask(ctx, taskParams(task)); err != nil {
        return fmt.Errorf("create task: %w", err)
    }
    if err := qtx.InsertOutboxEvent(ctx, eventParams(event)); err != nil {
        return fmt.Errorf("insert outbox: %w", err)
    }
    
    return tx.Commit(ctx)
}
```

### 15.5 Connection-Pool-Konfiguration

In `main.go` zentral:

```go
config, _ := pgxpool.ParseConfig(cfg.DB.URL)
config.MaxConns = int32(cfg.DB.MaxOpenConns)
config.MinConns = int32(cfg.DB.MaxIdleConns)
config.HealthCheckPeriod = 30 * time.Second
config.MaxConnLifetime = 5 * time.Minute
config.MaxConnIdleTime = 90 * time.Second

pool, err := pgxpool.NewWithConfig(ctx, config)
```

### 15.6 Niemals string-konkatenieren bei Queries

sqlc parametriert automatisch. Handgeschriebene Queries (selten nötig) immer mit `$1, $2`-Parametern, niemals Konkatenation.

---

## 16. HTTP-Handler

### 16.1 Handler-Layer ist dünn

Handler validiert nur HTTP-Spezifisches (Pfad-Parameter, Body-Parsing) und delegiert an Service:

```go
func (h *taskHandler) createTask(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    boardID, err := uuid.Parse(chi.URLParam(r, "boardId"))
    if err != nil {
        httputil.WriteProblem(w, r, http.StatusBadRequest, "invalid board id")
        return
    }
    requester := middleware.UserIDFromContext(ctx)
    
    var req CreateTaskRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.WriteProblem(w, r, http.StatusBadRequest, "invalid json")
        return
    }
    
    task, err := h.svc.CreateTask(ctx, requester, domain.CreateTaskInput{
        BoardID:     boardID,
        Title:       req.Title,
        Description: req.Description,
        // ...
    })
    if err != nil {
        httputil.WriteError(w, r, err)
        return
    }
    httputil.WriteJSON(w, http.StatusCreated, TaskResponse{Data: mapTaskToDTO(task)})
}
```

### 16.2 DTOs vs. Domain-Entities

- **Domain-Entity (`domain.Task`)**: interne Repräsentation, niemals direkt JSON-serialisiert
- **DTO (`TaskDTO`)**: API-Repräsentation, hat JSON-Tags, kann zusätzliche Computed-Fields enthalten

Mapping in dedizierten Funktionen `mapXxxToDTO`/`mapXxxFromRequest`.

### 16.3 Middleware

Reihenfolge in `router.go`:

```go
r.Use(middleware.RequestID)              // Trace-ID erzeugen oder aus Header
r.Use(middleware.Logger)                 // Request/Response-Log
r.Use(middleware.Recoverer)              // panic → 500
r.Use(middleware.Timeout(30 * time.Second))
r.Use(middleware.RealIP)
r.Use(authmiddleware.JWT(jwksSrc))       // unsere shared lib
```

### 16.4 Response-Format (verbindlich)

Erfolg:
```json
{ "data": { ... } }
```

Listen:
```json
{ "data": [...], "pagination": { "next_cursor": "...", "limit": 50 } }
```

Fehler (RFC 7807 Problem Details):
```json
{
  "type": "https://teamboard.example/errors/validation_failed",
  "title": "Validation failed",
  "status": 400,
  "detail": "field 'title' is required",
  "trace_id": "abc-123"
}
```

Helper in `shared/go/httputil`:

```go
func WriteJSON(w http.ResponseWriter, status int, body any) { ... }
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, detail string) { ... }
func WriteError(w http.ResponseWriter, r *http.Request, err error) { ... }
```

---

## 17. Konfiguration

### 17.1 Library: `github.com/caarlos0/env/v10`

Verbindlich. Keine eigene Env-Parser.

### 17.2 Pattern

Jeder Service hat ein Config-Struct in `internal/config/config.go` mit Env-Tags:

```go
type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"task-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8003"`
    
    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"50"`
    }
}

func Load() (*Config, error) {
    cfg := &Config{}
    if err := env.Parse(cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    if err := cfg.validate(); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

### 17.3 Validation

`validate()`-Methode prüft Cross-Field-Constraints:

```go
func (c *Config) validate() error {
    if c.JWT.AccessTokenTTL > c.JWT.RefreshTokenTTL {
        return errors.New("access token TTL must be shorter than refresh token TTL")
    }
    return nil
}
```

### 17.4 Secrets niemals im Code

- **Env-Variablen** für lokale Entwicklung
- **AWS Secrets Manager** für Produktion
- **Niemals** Default-Werte mit echten Secrets im `envDefault`-Tag

---

## 18. Performance und Allokationen

### 18.1 Premature Optimization vermeiden

Erst messen (`go test -bench`, `pprof`), dann optimieren. Keine Mikrooptimierung ohne Daten.

### 18.2 Slice-Allokation

Wenn die Endgröße bekannt ist, vorab allokieren:

```go
// Schlecht
result := []string{}
for _, x := range items {
    result = append(result, x.Name)
}

// Gut
result := make([]string, 0, len(items))
for _, x := range items {
    result = append(result, x.Name)
}
```

### 18.3 String-Concatenation

- **Wenig Konkatenationen:** `+` ist OK
- **Viele Konkatenationen oder Loop:** `strings.Builder`
- **Format-Strings mit Variablen:** `fmt.Sprintf`

### 18.4 JSON-Marshal-Pooling

Für Hot-Paths (z. B. WebSocket-Frame-Output): wiederverwendete Buffer mit `sync.Pool`.

### 18.5 Memory-Profile vor Production

Vor jedem Release: 1 Stunde Last-Test, `pprof heap` analysieren. Verdächtig: stetig wachsendes Heap (Leak), unerklärliche Spikes.

---

## 19. Security in Code

### 19.1 Input-Validation immer am Eintrittspunkt

HTTP-Handler validieren Input bevor sie an Service delegieren. Nicht "irgendwo später":

```go
if len(req.Title) == 0 || len(req.Title) > 500 {
    httputil.WriteProblem(w, r, 400, "title length 1-500")
    return
}
```

Validation-Library: `github.com/go-playground/validator/v10`. Struct-Tags:

```go
type CreateTaskRequest struct {
    Title string `json:"title" validate:"required,min=1,max=500"`
}
```

### 19.2 Output-Encoding

JSON-Encoding ist sicher gegen Injection. **Niemals** `template/html` mit unescaped User-Input verwenden — das ist Frontend-Aufgabe.

### 19.3 SQL-Injection

sqlc parametriert automatisch. Wenn doch handgeschriebene Queries: nur mit `$N`-Parametern, niemals Konkatenation.

### 19.4 Sensitive Data Redaction (Logging)

`shared/go/observability/redact.go`:

```go
var redactedKeys = map[string]bool{
    "password": true, "token": true, "secret": true,
    "authorization": true, "api_key": true, "refresh_token": true,
}

func redactSensitiveAttrs(groups []string, a slog.Attr) slog.Attr {
    if redactedKeys[strings.ToLower(a.Key)] {
        return slog.String(a.Key, "***")
    }
    // Auch Subfelder von structs prüfen
    return a
}
```

### 19.5 Crypto-Verwendung

- **Hashing**: `golang.org/x/crypto/argon2` für Passwörter, `crypto/sha256` für Signaturen
- **Random**: **immer** `crypto/rand`, niemals `math/rand` für Security-Zwecke
- **TLS**: nur Stdlib `crypto/tls` mit Go-Defaults
- **Niemals** eigene Crypto schreiben

### 19.6 Verbotene Funktionen

`gosec`-Linter blockt:

- `os/exec` mit User-Input (Command-Injection)
- `crypto/md5`, `crypto/sha1` für Security
- `math/rand` für Tokens
- `http.ListenAndServe` ohne Timeouts
- HTTP-Clients ohne Timeouts

---

## 20. Code-Review-Checkliste

Reviewer prüft:

### Funktionalität
- [ ] Code löst das Problem korrekt
- [ ] Edge Cases abgedeckt (nil, leere Slices, fehlende Permissions)
- [ ] Error-Pfade getestet

### Style
- [ ] `gofmt` und `goimports` sauber
- [ ] `golangci-lint` ohne Findings
- [ ] Naming: camelCase/PascalCase, Akronyme groß, sprechende Namen
- [ ] Funktionen ≤50 Zeilen, ≤4 Argumente
- [ ] Doc-Comments an exportierten Bezeichnern

### Architektur
- [ ] Domain-Layer importiert nicht aus api/repository/events
- [ ] Permissions vor jeder schreibenden Operation geprüft
- [ ] Outbox + DB-Mutation in einer Transaktion
- [ ] Idempotenz für Event-Konsumenten (`processed_events`)

### Concurrency
- [ ] Goroutinen haben Beendigungs-Mechanismus
- [ ] Mutex- oder Channel-Schutz für geteilten State
- [ ] `go vet` und `-race` sauber
- [ ] Defer in Loops vermieden

### Errors
- [ ] Domain-Errors typisiert, keine String-Errors
- [ ] `%w` für Wrapping
- [ ] Sensitive Daten nicht in Errors

### Tests
- [ ] Coverage erfüllt Mindest-Quote
- [ ] Subtests mit `t.Run`
- [ ] Table-Driven wo passend
- [ ] Cleanup mit `t.Cleanup`

### Logging
- [ ] `slog` mit strukturierten Feldern
- [ ] Trace-ID propagiert
- [ ] Keine Secrets, Tokens, Passwörter, vollständige PII
- [ ] Errors nicht doppelt geloggt

### Security
- [ ] Input am Handler validiert
- [ ] sqlc oder parametrisierte Queries
- [ ] `crypto/rand` für sicherheitskritische Werte
- [ ] Timeouts auf jeden Netzwerk-Call

### Datenbank
- [ ] sqlc-Code regeneriert wenn Queries geändert
- [ ] Repository-Interface beim Konsumer definiert
- [ ] Mapping-Funktionen DB ↔ Domain in dedizierten Funktionen
- [ ] Connection-Pool-Limits konfiguriert

### Konfiguration
- [ ] Env-Variablen über `caarlos0/env`
- [ ] `,required` für Pflichtfelder
- [ ] Keine Default-Secrets im Code

---

## Anhang A: Pflicht-Linter-Findings im CI

Pull Request wird **geblockt** bei:
- `errcheck`-Findings auf Errors aus Service-Calls
- `gosec`-Findings (außer dokumentierte Suppressions mit `//nolint:gosec` und Begründung)
- `unused`-Findings
- `gocyclo`-Komplexität >15
- Test-Coverage unter Mindest-Quote

## Anhang B: Häufige Anti-Patterns

| Anti-Pattern | Richtig |
|--------------|---------|
| `if err == nil { return ... } else { return err }` | `if err != nil { return err }; return ...` |
| `panic(err)` in Library-Code | `return err` |
| `interface{}` für alles | Konkreten Typ oder Generic |
| `time.Sleep` zur Synchronisation | Channel oder `sync.Cond` |
| `fmt.Println` für Logging | `slog` |
| `_ = err` ohne Kommentar | Begründung warum ignoriert |
| Globale Mutables Variablen | Dependency Injection |
| `init()` mit Side-Effects (DB-Connect, etc.) | Explizite `Init()`-Funktion in main |

---

**Ende der Coding Guidelines.**
