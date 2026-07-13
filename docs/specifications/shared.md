# Shared Libraries — Cross-Cutting-Code

> **Pfad:** `shared/go/`  
> **Zweck:** Wiederverwendbarer Code, der von mehr als einem Service benötigt wird. Verhindert Duplikation und sichert konsistentes Verhalten.  
> **Verbindlichkeit:** Code, dessen Verhalten in den Coding Guidelines spezifiziert ist (z. B. JWT-Validation, Logging-Format, Outbox-Pattern), wird **ausschließlich** über die shared libraries verwendet — nicht pro Service nachgebaut.  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verzeichnisstruktur](#1-verzeichnisstruktur)
2. [authmiddleware](#2-authmiddleware) — JWT-Validation für HTTP
3. [eventbus](#3-eventbus) — RabbitMQ-Wrapper
4. [outbox](#4-outbox) — Outbox-Pattern-Library
5. [observability](#5-observability) — Logger und Tracer
6. [httputil](#6-httputil) — HTTP-Helpers, Error-Mapping
7. [servicetoken](#7-servicetoken) — Service-zu-Service-Auth
8. [Versionierung und Updates](#8-versionierung-und-updates)
9. [Testing der Shared Libraries](#9-testing-der-shared-libraries)

---

## 1. Verzeichnisstruktur

```
shared/go/
├── go.mod                                 # github.com/teamboard/shared/go
├── go.sum
├── README.md                              # Quick-Reference
│
├── authmiddleware/
│   ├── jwt.go                             # JWT-Middleware
│   ├── jwks_source.go                     # JWKS-Cache
│   ├── context.go                         # context-Keys + Helper
│   └── jwt_test.go
│
├── eventbus/
│   ├── publisher.go                       # Event-Publisher (RabbitMQ)
│   ├── consumer.go                        # Event-Consumer mit Idempotenz-Hook
│   ├── envelope.go                        # Envelope-Struct + Marshal
│   ├── topology.go                        # Exchange/Queue-Setup
│   └── eventbus_test.go
│
├── outbox/
│   └── outbox.go                          # Outbox-Reader-Worker (publiziert via eventbus.Publisher)
│
├── observability/
│   ├── logger.go                          # slog-Setup
│   ├── tracer.go                          # OpenTelemetry-Setup
│   ├── redact.go                          # Sensitive-Data-Filter
│   └── observability_test.go
│
├── httputil/
│   ├── problem.go                         # RFC 7807 Problem-Details
│   ├── json.go                            # JSON-Helper
│   ├── error_mapping.go                   # Domain-Error → HTTP-Status
│   ├── pagination.go                      # Cursor-Helper
│   └── httputil_test.go
│
└── servicetoken/
    ├── issuer.go                          # Service-Token erstellen
    ├── verifier.go                        # Service-Token validieren
    └── servicetoken_test.go
```

### 1.1 Modul-Pfad

`go.mod`:
```
module github.com/teamboard/shared/go

go 1.22
```

Services importieren via:
```go
import "github.com/teamboard/shared/go/authmiddleware"
```

### 1.2 Versionierung

`shared/go` ist Teil des Monorepos und wird via Replace-Directive eingebunden:

```go
// services/task/go.mod
require github.com/teamboard/shared/go v0.0.0
replace github.com/teamboard/shared/go => ../../shared/go
```

Vorteil: atomare Updates, keine Versions-Hölle. Nachteil: bei externer Wiederverwendung müsste man tags/releases einführen — im MVP nicht nötig.

---

## 2. authmiddleware

### 2.1 Verantwortung

- Validiert JWT aus `Authorization: Bearer <token>` Header
- Lädt Public Keys über JWKS-Endpoint mit Cache
- Bei gültigem Token: User-ID und E-Mail in `context.Context`
- Bei ungültigem oder fehlendem Token: 401 mit RFC 7807

### 2.2 API

```go
package authmiddleware

import (
    "context"
    "net/http"
    "time"
    
    "github.com/google/uuid"
)

// Middleware returns an HTTP middleware that validates Bearer JWTs against
// the JWKS endpoint and injects user info into the request context.
func Middleware(jwks JWKSSource, opts ...Option) func(http.Handler) http.Handler

// VerifyToken validates a raw JWT string (RS256 + exp, plus iss/aud when set via
// opts) and returns its claims. For non-HTTP paths such as WebSocket upgrades
// where the token arrives in a query parameter rather than a header.
func VerifyToken(ctx context.Context, token string, jwks JWKSSource, opts ...Option) (jwt.MapClaims, error)

// JWKSSource fetches and caches public keys for JWT validation.
type JWKSSource interface {
    Key(ctx context.Context, kid string) (any, error)
}

// NewJWKSSource creates a JWKSSource that fetches from the given URL and caches results.
func NewJWKSSource(url string, opts ...JWKSOption) JWKSSource

// Options
type Option func(*config)
type JWKSOption func(*jwksConfig)

func WithIssuer(iss string) Option
func WithAudience(aud string) Option
func WithClockSkew(skew time.Duration) Option
func WithJWKSCacheTTL(ttl time.Duration) JWKSOption

// Context-Helper
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool)
func EmailFromContext(ctx context.Context) (string, bool)
func MustUserID(ctx context.Context) uuid.UUID  // panics if not set
```

### 2.3 Verwendung in Services

```go
package main

import (
    "github.com/go-chi/chi/v5"
    "github.com/teamboard/shared/go/authmiddleware"
)

func main() {
    cfg := config.MustLoad()
    
    jwks := authmiddleware.NewJWKSSource(
        cfg.JWT.JWKSURL,
        authmiddleware.WithJWKSCacheTTL(cfg.JWT.JWKSCacheTTL),
    )
    
    auth := authmiddleware.Middleware(jwks,
        authmiddleware.WithIssuer(cfg.JWT.Issuer),
        authmiddleware.WithAudience(cfg.JWT.Audience),
        authmiddleware.WithClockSkew(30*time.Second),
    )
    
    r := chi.NewRouter()
    r.Group(func(r chi.Router) {
        r.Use(auth)  // Auth-required routes
        r.Get("/api/v1/tasks", taskHandler.list)
    })
}
```

### 2.4 Implementation-Skeleton

```go
package authmiddleware

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    
    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
    
    "github.com/teamboard/shared/go/httputil"
)

type contextKey int

const (
    contextKeyUserID contextKey = iota
    contextKeyEmail
)

type config struct {
    issuer    string
    audience  string
    clockSkew time.Duration
}

func Middleware(jwks JWKSSource, opts ...Option) func(http.Handler) http.Handler {
    cfg := &config{clockSkew: 30 * time.Second}
    for _, o := range opts {
        o(cfg)
    }
    
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tokenStr := extractBearer(r.Header.Get("Authorization"))
            if tokenStr == "" {
                httputil.WriteProblem(w, r, http.StatusUnauthorized, "missing authorization header")
                return
            }
            
            claims, err := validateToken(r.Context(), tokenStr, jwks, cfg)
            if err != nil {
                httputil.WriteProblem(w, r, http.StatusUnauthorized, err.Error())
                return
            }
            
            uid, err := uuid.Parse(claims.Subject)
            if err != nil {
                httputil.WriteProblem(w, r, http.StatusUnauthorized, "invalid user id in token")
                return
            }
            
            ctx := context.WithValue(r.Context(), contextKeyUserID, uid)
            if email, _ := claims.GetEmail(); email != "" {
                ctx = context.WithValue(ctx, contextKeyEmail, email)
            }
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func extractBearer(h string) string {
    const prefix = "Bearer "
    if !strings.HasPrefix(h, prefix) {
        return ""
    }
    return strings.TrimPrefix(h, prefix)
}

// JWKS-Cache
type jwksSource struct {
    url      string
    cacheTTL time.Duration
    
    mu     sync.RWMutex
    keys   map[string]any
    fetched time.Time
}

func (j *jwksSource) Key(ctx context.Context, kid string) (any, error) {
    j.mu.RLock()
    if k, ok := j.keys[kid]; ok && time.Since(j.fetched) < j.cacheTTL {
        j.mu.RUnlock()
        return k, nil
    }
    j.mu.RUnlock()
    
    if err := j.refresh(ctx); err != nil {
        return nil, err
    }
    j.mu.RLock()
    defer j.mu.RUnlock()
    if k, ok := j.keys[kid]; ok {
        return k, nil
    }
    return nil, fmt.Errorf("kid %s not found in JWKS", kid)
}

func (j *jwksSource) refresh(ctx context.Context) error {
    // HTTP GET, parse JWKS, ...
}
```

### 2.5 Test-Strategie

```go
func TestMiddleware_ValidToken(t *testing.T) { ... }
func TestMiddleware_ExpiredToken(t *testing.T) { ... }
func TestMiddleware_InvalidIssuer(t *testing.T) { ... }
func TestMiddleware_MissingHeader(t *testing.T) { ... }
func TestJWKSSource_CachesKeys(t *testing.T) { ... }
func TestJWKSSource_RefreshOnUnknownKID(t *testing.T) { ... }
```

---

## 3. eventbus

### 3.1 Verantwortung

- Verbindung zu RabbitMQ aufbauen und halten (mit Auto-Reconnect)
- Standardisierte Topology (Exchange `teamboard.events` topic, DLX `teamboard.events.dlx`)
- Event-Publisher-API
- Event-Consumer-API mit Idempotenz-Hook
- Envelope-Format einheitlich

### 3.2 API

```go
package eventbus

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/google/uuid"
    amqp "github.com/rabbitmq/amqp091-go"
)

// Envelope is the canonical event format. See ARCHITECTURE.md §7.3.
type Envelope struct {
    EventID       string          `json:"event_id"`
    EventType     string          `json:"event_type"`
    EventVersion  int             `json:"event_version"`
    OccurredAt    time.Time       `json:"occurred_at"`
    TraceID       string          `json:"trace_id,omitempty"`
    Producer      string          `json:"producer"`
    AggregateType string          `json:"aggregate_type"`
    AggregateID   string          `json:"aggregate_id"`
    Actor         Actor           `json:"actor"`
    Payload       json.RawMessage `json:"payload"`
}

type Actor struct {
    UserID string `json:"user_id,omitempty"`
    Type   string `json:"type"`              // "user", "service", "system"
}

// Connection wraps an AMQP connection with reconnect logic.
type Connection struct { ... }

func Connect(ctx context.Context, url string, opts ...ConnectionOption) (*Connection, error)

// Publisher sends events to the exchange.
type Publisher interface {
    Publish(ctx context.Context, env Envelope) error
    Close() error
}

func NewPublisher(conn *Connection, opts ...PublisherOption) (Publisher, error)

// Consumer subscribes to events and dispatches to handler.
type Consumer interface {
    Subscribe(ctx context.Context, handler Handler) error
    Close() error
}

// Handler is invoked for each consumed event. Return error to nack.
// Idempotency is the handler's responsibility (use IdempotentHandler wrapper).
type Handler func(ctx context.Context, env Envelope) error

func NewConsumer(conn *Connection, queue string, bindingKeys []string, opts ...ConsumerOption) (Consumer, error)

// IdempotentHandler wraps a handler with processed_events tracking.
func IdempotentHandler(inner Handler, store IdempotencyStore) Handler

type IdempotencyStore interface {
    HasProcessed(ctx context.Context, eventID string) (bool, error)
    MarkProcessed(ctx context.Context, eventID string) error
}
```

### 3.3 Verwendung — Publisher

Üblicherweise nicht direkt, sondern über `outbox`-Library (siehe Abschnitt 4). Bei direkter Nutzung:

```go
publisher, _ := eventbus.NewPublisher(conn)
defer publisher.Close()

env := eventbus.Envelope{
    EventID:      ulid.Make().String(),
    EventType:    "task.created",
    EventVersion: 1,
    OccurredAt:   time.Now().UTC(),
    Producer:     "task-service",
    AggregateType: "task",
    AggregateID:  task.ID.String(),
    Actor:        eventbus.Actor{UserID: requester.String(), Type: "user"},
    Payload:      mustMarshal(payload),
}
publisher.Publish(ctx, env)
```

### 3.4 Verwendung — Consumer

```go
consumer, _ := eventbus.NewConsumer(conn, "task-service-queue",
    []string{"user.*", "board.*", "column.*", "project.deleted", "document.deleted"})

handler := eventbus.IdempotentHandler(
    func(ctx context.Context, env eventbus.Envelope) error {
        switch env.EventType {
        case "user.deleted":
            return userHandler.Handle(ctx, env)
        case "board.deleted":
            return boardHandler.Handle(ctx, env)
        // ...
        }
        return nil
    },
    idempotencyStore,
)

go consumer.Subscribe(ctx, handler)
```

### 3.5 Topology-Setup

`eventbus.SetupTopology` erstellt benötigte Exchanges/Queues idempotent:

```go
func SetupTopology(ch *amqp.Channel, opts TopologyOptions) error {
    // Main exchange
    if err := ch.ExchangeDeclare(
        opts.Exchange, "topic", true, false, false, false, nil,
    ); err != nil { return err }
    
    // Dead-Letter Exchange
    if err := ch.ExchangeDeclare(
        opts.Exchange+".dlx", "topic", true, false, false, false, nil,
    ); err != nil { return err }
    
    // Queue mit DLX-Verknüpfung
    args := amqp.Table{
        "x-dead-letter-exchange": opts.Exchange + ".dlx",
        "x-message-ttl": int32(24 * 60 * 60 * 1000),  // 24h before DLX
    }
    if _, err := ch.QueueDeclare(opts.Queue, true, false, false, false, args); err != nil {
        return err
    }
    
    for _, key := range opts.BindingKeys {
        if err := ch.QueueBind(opts.Queue, key, opts.Exchange, false, nil); err != nil {
            return err
        }
    }
    return nil
}
```

### 3.6 Reconnect-Verhalten

`Connection` registriert sich auf `amqp.Connection.NotifyClose` und reconnected mit Exponential Backoff. Während Reconnect blockieren `Publish`-Calls für maximal 5s und returnen dann Error — Caller kann selbst entscheiden ob retry oder failure.

---

## 4. outbox

### 4.1 Verantwortung

Implementiert das Outbox-Pattern (siehe ARCHITECTURE.md §7.5):

1. Service schreibt Event in `outbox`-Tabelle innerhalb der DB-Transaktion
2. Background-Worker liest unpublished Events, sendet via `eventbus`
3. Bei Erfolg `published_at` setzen
4. Crash-resistant: Nicht publizierte Events werden nach Restart wieder versucht

### 4.2 API

```go
package outbox

import (
    "context"
    "time"
    
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/teamboard/shared/go/eventbus"
)

// Worker reads from outbox table and publishes via eventbus.
type Worker struct { ... }

type Config struct {
    Pool         *pgxpool.Pool
    TableName    string                  // default: "outbox"
    Publisher    eventbus.Publisher
    PollInterval time.Duration           // default: 1s
    BatchSize    int                     // default: 100
    Producer     string                  // service name
}

func NewWorker(cfg Config) *Worker

func (w *Worker) Run(ctx context.Context) error
func (w *Worker) Stop(ctx context.Context) error

// InsertEvent ist eine Helper-Funktion, die innerhalb einer Transaktion
// einen Event in die outbox-Tabelle schreibt. Services nutzen sie statt
// eventbus.Publish im Schreibpfad.
func InsertEvent(ctx context.Context, tx pgx.Tx, params InsertEventParams) error

type InsertEventParams struct {
    ID            uuid.UUID
    AggregateID   uuid.UUID
    EventType     string
    Payload       any        // wird zu JSON gemarshalled
}
```

### 4.3 Verwendung im Service

```go
// In Service-Methode
func (s *taskService) CreateTask(ctx context.Context, ...) (*Task, error) {
    // Permission-Check, Validierung...
    
    tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil { return nil, err }
    defer tx.Rollback(ctx)
    
    // 1. Domain-Mutation
    task, err := s.repo.WithTx(tx).CreateTask(ctx, ...)
    if err != nil { return nil, err }
    
    // 2. Outbox-Event in selber Transaktion
    err = outbox.InsertEvent(ctx, tx, outbox.InsertEventParams{
        ID:          uuid.New(),
        AggregateID: task.ID,
        EventType:   "task.created",
        Payload:     map[string]any{...},
    })
    if err != nil { return nil, err }
    
    if err := tx.Commit(ctx); err != nil { return nil, err }
    return task, nil
}

// In main.go
worker := outbox.NewWorker(outbox.Config{
    Pool:      pool,
    Publisher: publisher,
    Producer:  "task-service",
})
go worker.Run(ctx)
```

### 4.4 Worker-Implementation-Skeleton

```go
func (w *Worker) Run(ctx context.Context) error {
    ticker := time.NewTicker(w.cfg.PollInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := w.processBatch(ctx); err != nil {
                w.logger.Warn("outbox batch failed", "err", err)
            }
        }
    }
}

func (w *Worker) processBatch(ctx context.Context) error {
    tx, err := w.cfg.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
    if err != nil { return err }
    defer tx.Rollback(ctx)
    
    // FOR UPDATE SKIP LOCKED: parallele Worker stören sich nicht
    rows, err := tx.Query(ctx, `
        SELECT id, aggregate_id, event_type, payload, occurred_at
        FROM outbox WHERE published_at IS NULL
        ORDER BY occurred_at ASC, id ASC
        LIMIT $1 FOR UPDATE SKIP LOCKED`, w.cfg.BatchSize)
    if err != nil { return err }
    
    var batch []outboxRow
    for rows.Next() { /* scan into batch */ }
    rows.Close()
    
    for _, e := range batch {
        env := w.buildEnvelope(e)
        if err := w.cfg.Publisher.Publish(ctx, env); err != nil {
            return fmt.Errorf("publish %s: %w", e.id, err)
        }
        if _, err := tx.Exec(ctx,
            `UPDATE outbox SET published_at = NOW() WHERE id = $1`, e.id); err != nil {
            return err
        }
    }
    return tx.Commit(ctx)
}
```

### 4.5 Schema-Anforderungen an den Service

Jede Service-DB hat eine `outbox`-Tabelle mit dem Standard-Schema (siehe ARCHITECTURE.md §7.5). `outbox.Worker` setzt dieses Schema voraus.

### 4.6 Backpressure

Wenn Publisher länger blockiert (Broker down), bleiben Events in der DB liegen. Worker ruht beim nächsten Tick und versucht erneut. Kein Memory-Aufbau im Service.

---

## 5. observability

### 5.1 Verantwortung

- `slog`-Logger mit JSON-Format und Standard-Felder
- OpenTelemetry-Tracer-Provider mit OTLP-Export
- Trace-ID-Propagation zwischen HTTP, AMQP, Logs
- Sensitive-Data-Redaction

### 5.2 API

```go
package observability

import (
    "context"
    "log/slog"
    
    "go.opentelemetry.io/otel/trace"
)

// SetupLogger creates a slog.Logger and sets it as default.
// Returns the logger for explicit usage.
func SetupLogger(serviceName, level string) *slog.Logger

// SetupTracer initializes OpenTelemetry tracing with OTLP exporter.
// Returns shutdown function to call on service exit.
func SetupTracer(ctx context.Context, cfg TracerConfig) (shutdown func(context.Context) error, err error)

type TracerConfig struct {
    ServiceName  string
    OTLPEndpoint string
    SampleRatio  float64
}

// Middleware
func TracingMiddleware(serviceName string) func(http.Handler) http.Handler
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler
```

### 5.3 Setup in main.go

```go
func main() {
    cfg := config.MustLoad()
    
    logger := observability.SetupLogger(cfg.ServiceName, cfg.LogLevel)
    
    ctx := context.Background()
    shutdown, err := observability.SetupTracer(ctx, observability.TracerConfig{
        ServiceName:  cfg.ServiceName,
        OTLPEndpoint: cfg.Observability.OTLPEndpoint,
        SampleRatio:  1.0,
    })
    if err != nil {
        logger.Error("tracer setup failed", "err", err)
        os.Exit(1)
    }
    defer shutdown(context.Background())
    
    // ...
}
```

### 5.4 Verwendung

```go
// Logger holen — entweder über default oder via Context
logger := slog.Default()
logger.InfoContext(ctx, "task created",
    slog.String("task_id", id),
    slog.String("user_id", uid),
)

// Tracing — Spans innerhalb von Operations
ctx, span := tracer.Start(ctx, "TaskService.CreateTask")
defer span.End()

span.SetAttributes(
    attribute.String("task.id", task.ID.String()),
)
```

### 5.5 Trace-ID in Logs

`SetupLogger` registriert einen Custom-Handler, der Trace-ID und Span-ID aus `context.Context` automatisch in Log-Einträge einfügt:

```go
type tracingHandler struct {
    next slog.Handler
}

func (h *tracingHandler) Handle(ctx context.Context, r slog.Record) error {
    if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
        sc := span.SpanContext()
        r.AddAttrs(
            slog.String("trace_id", sc.TraceID().String()),
            slog.String("span_id", sc.SpanID().String()),
        )
    }
    return h.next.Handle(ctx, r)
}
```

### 5.6 Redaction

```go
package observability

var sensitiveKeys = map[string]struct{}{
    "password": {}, "passwd": {},
    "token": {}, "access_token": {}, "refresh_token": {},
    "secret": {}, "api_key": {},
    "authorization": {},
}

func RedactAttr(groups []string, a slog.Attr) slog.Attr {
    if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
        return slog.String(a.Key, "***")
    }
    return a
}
```

---

## 6. httputil

### 6.1 Verantwortung

- RFC 7807 Problem-Details schreiben
- JSON-Response-Helper mit konsistentem Wrapping (`{ "data": ... }`)
- Domain-Error → HTTP-Status-Mapping
- Cursor-Pagination-Helper

### 6.2 API

```go
package httputil

import (
    "encoding/json"
    "net/http"
    
    "go.opentelemetry.io/otel/trace"
)

// WriteJSON writes data wrapped in {"data": ...} with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) error

// WriteList writes a list response with optional pagination.
func WriteList(w http.ResponseWriter, status int, items any, pagination *Pagination) error

// WriteProblem writes an RFC 7807 problem details response.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, detail string) error
func WriteProblemFull(w http.ResponseWriter, r *http.Request, p Problem) error

// WriteError maps a domain error to HTTP status and writes problem response.
func WriteError(w http.ResponseWriter, r *http.Request, err error)

// MapErrorToHTTPStatus returns the HTTP status code for a domain error.
func MapErrorToHTTPStatus(err error) int

type Problem struct {
    Type     string                 `json:"type"`
    Title    string                 `json:"title"`
    Status   int                    `json:"status"`
    Detail   string                 `json:"detail,omitempty"`
    Instance string                 `json:"instance,omitempty"`
    TraceID  string                 `json:"trace_id,omitempty"`
    Errors   []FieldError           `json:"errors,omitempty"`
    Extra    map[string]any         `json:"-"`
}

type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

type Pagination struct {
    NextCursor *string `json:"next_cursor"`
    Limit      int     `json:"limit"`
}

// Cursor-Helper
func EncodeCursor(payload any) (string, error)
func DecodeCursor(cursor string, target any) error
```

### 6.3 Implementation — `WriteError`

```go
package httputil

import "errors"

type DomainError interface {
    error
    GetCode() string
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
    var domainErr DomainError
    if !errors.As(err, &domainErr) {
        // Unbekannter Error → 500 mit redaktiertem Detail
        WriteProblem(w, r, http.StatusInternalServerError, "internal error")
        return
    }
    
    status := mapCodeToStatus(domainErr.GetCode())
    WriteProblemFull(w, r, Problem{
        Type:    "https://teamboard.example/errors/" + domainErr.GetCode(),
        Title:   humanizeCode(domainErr.GetCode()),
        Status:  status,
        Detail:  domainErr.Error(),
        TraceID: traceIDFromContext(r.Context()),
    })
}

func mapCodeToStatus(code string) int {
    switch {
    case strings.HasSuffix(code, "_not_found"):
        return http.StatusNotFound
    case code == "permission_denied":
        return http.StatusForbidden
    case code == "validation_failed", code == "invalid_url":
        return http.StatusBadRequest
    case strings.HasPrefix(code, "already_") || code == "conflict" || code == "last_owner_protected":
        return http.StatusConflict
    case code == "rate_limited":
        return http.StatusTooManyRequests
    case code == "unauthorized" || code == "token_invalid":
        return http.StatusUnauthorized
    default:
        return http.StatusInternalServerError
    }
}
```

### 6.4 Cursor-Pagination

```go
func EncodeCursor(payload any) (string, error) {
    j, err := json.Marshal(payload)
    if err != nil { return "", err }
    return base64.URLEncoding.EncodeToString(j), nil
}

func DecodeCursor(cursor string, target any) error {
    if cursor == "" { return nil }
    j, err := base64.URLEncoding.DecodeString(cursor)
    if err != nil { return fmt.Errorf("invalid cursor: %w", err) }
    return json.Unmarshal(j, target)
}

// Verwendung
type taskCursor struct {
    Position string    `json:"position"`
    ID       uuid.UUID `json:"id"`
}

cursor := taskCursor{Position: lastTask.Position, ID: lastTask.ID}
encoded, _ := httputil.EncodeCursor(cursor)
```

### 6.5 Beispiel-Verwendung

```go
func (h *taskHandler) listTasks(w http.ResponseWriter, r *http.Request) {
    var c taskCursor
    if err := httputil.DecodeCursor(r.URL.Query().Get("cursor"), &c); err != nil {
        httputil.WriteProblem(w, r, http.StatusBadRequest, "invalid cursor")
        return
    }
    
    tasks, nextCursor, err := h.svc.ListTasks(ctx, ...)
    if err != nil {
        httputil.WriteError(w, r, err)
        return
    }
    
    pag := &httputil.Pagination{Limit: 50}
    if nextCursor != nil {
        encoded, _ := httputil.EncodeCursor(nextCursor)
        pag.NextCursor = &encoded
    }
    httputil.WriteList(w, http.StatusOK, mapTasksToDTO(tasks), pag)
}
```

---

## 7. servicetoken

### 7.1 Verantwortung

Service-zu-Service-Authentifizierung. Wenn Task Service den Project Service aufruft, braucht er ein Token mit `aud: "internal"`. Dieses Token kann nur ein Service ausstellen, der den Service-Token-Secret kennt.

### 7.2 API

```go
package servicetoken

import (
    "context"
    "time"
)

// Issuer creates short-lived (60s) HS256 JWTs for service-to-service calls.
type Issuer interface {
    Issue(ctx context.Context, callerService, audience string) (string, error)
}

func NewIssuer(secret string) Issuer

// Verifier validates incoming service tokens. The accepted audience is fixed
// at construction (e.g. "internal").
type Verifier interface {
    Verify(ctx context.Context, token string) (*Claims, error)
}

func NewVerifier(secret, audience string) Verifier

type Claims struct {
    Service  string
    Audience string
}

// Middleware for /internal/* routes. Audience is enforced by the Verifier.
func RequireServiceToken(verifier Verifier) func(http.Handler) http.Handler
```

### 7.3 Verwendung — Caller (z. B. Task Service ruft Project Service)

```go
issuer := servicetoken.NewIssuer(cfg.Security.ServiceTokenSecret)

// Pro Call:
token, _ := issuer.Issue(ctx, "task-service", "internal")
req.Header.Set("Authorization", "Bearer "+token)
```

### 7.4 Verwendung — Empfänger (z. B. Project Service)

```go
verifier := servicetoken.NewVerifier(cfg.Security.ServiceTokenSecret, "internal")

r.Group(func(r chi.Router) {
    r.Use(servicetoken.RequireServiceToken(verifier))
    r.Get("/internal/projects/{id}/permissions/{userId}", h.getPermissions)
})
```

### 7.5 Token-Format

```json
{
  "iss": "teamboard-internal",
  "sub": "task-service",
  "aud": "internal",
  "iat": 1715000000,
  "exp": 1715000060
}
```

- **Lifetime: 60 Sekunden** — kurz, weil Verfügbarkeit nicht problematisch ist (Caller stellt bei Bedarf neues aus)
- **HS256** — symmetrisch, weil beide Seiten denselben Secret kennen
- **`aud: "internal"`** — verhindert, dass User-JWTs hier akzeptiert werden

### 7.6 Verwechslungsgefahr mit User-JWT

Service-Token sind funktional ähnlich zu User-JWTs, aber:
- Andere Audience (`internal` vs `teamboard-api`)
- Anderer Algorithmus (HS256 vs RS256)
- Anderer Issuer

Verifier muss `aud` strikt prüfen, um Verwechslung auszuschließen.

---

## 8. Versionierung und Updates

### 8.1 Breaking Changes

Änderungen an public APIs der shared libraries betreffen alle Services. Daher:

- **Major-Updates erfordern ADR** (`docs/adr/`)
- **Migration aller Services in einem Pull Request** (Monorepo erlaubt das)
- **Keine deprecated-Marker im MVP** — wir refactoren konsistent

### 8.2 Wann gehört Code in shared/?

**Ja:**
- Code, der in 2+ Services identisch wäre
- Verhalten, das in Coding Guidelines spezifiziert ist
- Cross-cutting Concerns (Auth, Logging, Tracing)

**Nein:**
- Domain-Logik (gehört in den jeweiligen Service)
- "Utility"-Funktionen ohne klaren Anwendungsfall (führt zu shared-Wildwuchs)
- Service-spezifische Helpers

**Faustregel:** Bevor ein Helper in `shared/` landet, muss mindestens ein Service ihn brauchen. Beim zweiten Service mit demselben Bedarf wird der Code dorthin gehoben — nicht spekulativ vorab.

---

## 9. Testing der Shared Libraries

### 9.1 Coverage-Anforderung

`shared/go/` hat **erhöhte Coverage-Anforderung von 90%**. Begründung: Bug in shared library hat Multiplikator-Effekt über alle Services.

### 9.2 Pflicht-Tests

| Library | Pflicht-Test-Szenarien |
|---------|------------------------|
| `authmiddleware` | gültiges Token, abgelaufen, falscher Issuer, falsche Audience, fehlender Header, JWKS-Cache-Hit, JWKS-Cache-Miss-Refresh |
| `eventbus` | Publish und Konsum mit echtem RabbitMQ-Container, Reconnect bei Verbindungsverlust, Idempotency-Wrapper |
| `outbox` | Worker liest und published, FOR UPDATE SKIP LOCKED bei mehreren Workern, Crash-Recovery (publishing-Crash → bei Restart erneut versucht) |
| `observability` | JSON-Format korrekt, Trace-ID propagiert, Redaction wirkt |
| `httputil` | Problem-Details-Format, Domain-Error → Status-Mapping vollständig, Cursor-Round-Trip |
| `servicetoken` | Issue + Verify Round-Trip, falsche Audience verworfen, abgelaufenes Token verworfen |

### 9.3 CI-Integration

`shared/go/` hat eigenen Workflow-Job in `.github/workflows/ci.yml`:

```yaml
test-shared:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.22'
    - name: Test shared libraries
      run: |
        cd shared/go
        go test -race -coverprofile=coverage.out ./...
        go tool cover -func=coverage.out | grep total
```

Bei Coverage <90% bricht der Workflow.

---

## Anhang A: README.md für shared/go/

```markdown
# TeamBoard — Shared Go Libraries

Cross-cutting code shared between TeamBoard services.

## Quick Reference

| Package | Purpose |
|---------|---------|
| `authmiddleware` | JWT-Validation HTTP-Middleware |
| `eventbus` | RabbitMQ-Wrapper für Publish und Konsum |
| `outbox` | Outbox-Pattern-Worker |
| `observability` | Logger, Tracer, Redaction |
| `httputil` | RFC 7807 Problem-Details, JSON-Helper |
| `servicetoken` | Service-zu-Service-Auth |

## Usage

In a service's `go.mod`:
```
require github.com/teamboard/shared/go v0.0.0
replace github.com/teamboard/shared/go => ../../shared/go
```

See individual package docs (`go doc`) for details.

## Contributing

- Coverage requirement: 90%
- Breaking changes require ADR
- See [docs/coding-guidelines.md](../../docs/coding-guidelines.md) for code conventions
```

---

**Ende der Shared-Libraries-Spezifikation.**
