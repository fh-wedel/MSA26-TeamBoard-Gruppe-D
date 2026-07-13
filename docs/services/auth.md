# Auth Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Service:** `auth`  
> **Port (lokal):** 8001  
> **Datenbank:** `auth_db` (PostgreSQL)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Use-Cases](#2-use-cases)
3. [Datenmodell](#3-datenmodell)
4. [Domain-Modell](#4-domain-modell)
5. [HTTP-API (OpenAPI)](#5-http-api-openapi)
6. [JWT-Spezifikation](#6-jwt-spezifikation)
7. [Schlüsselverwaltung und Rotation](#7-schlüsselverwaltung-und-rotation)
8. [Refresh-Token-Strategie](#8-refresh-token-strategie)
9. [Passwort-Handling](#9-passwort-handling)
10. [Events](#10-events)
11. [Sicherheit](#11-sicherheit)
12. [Konfiguration](#12-konfiguration)
13. [Verzeichnisstruktur](#13-verzeichnisstruktur)
14. [sqlc-Queries](#14-sqlc-queries)
15. [Test-Strategie](#15-test-strategie)
16. [Implementierungs-Hinweise für Coding-Agents](#16-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Identitätsverwaltung:** Registrierung neuer Nutzer mit E-Mail/Passwort
- **Authentifizierung:** Login → JWT-Ausstellung (Access + Refresh)
- **Token-Lifecycle:** Refresh, Logout, Revocation
- **Passwort-Reset:** Token-basierter Flow mit zeitlicher Begrenzung
- **Schlüsselverwaltung:** Rotation der Signing-Keys, Bereitstellung via JWKS
- **Public-Key-Distribution:** `/.well-known/jwks.json` für andere Services

### 1.2 Verantwortet NICHT

- **Projekt-spezifische Rollen** → Project Service ist Authority
- **Profile-Daten** über die Identität hinaus (kein Avatar, kein Name in Langform) — nur was zur Authentifizierung nötig ist
- **MFA, Social Login, SSO** — bewusst aus dem MVP ausgeklammert, später ergänzbar
- **Session-Management auf Server-Seite** — JWTs sind self-contained, kein Server-State pro Session

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `auth_db` | hart | persistente Speicherung User/Tokens/Keys |
| RabbitMQ | weich | Event-Publikation (`user.registered`, `user.deleted`) |
| SMTP / SES | weich | Versand von Reset-Mails (in Dev: nur loggen) |

"Weich" = Service kann ohne diese Abhängigkeit Kernfunktionen liefern, allerdings degradiert.

---

## 2. Use-Cases

### UC-1: Registrierung
**Akteur:** Anonymer Client  
**Vorbedingung:** E-Mail noch nicht registriert, Passwort erfüllt Policy  
**Ablauf:**
1. Client schickt `POST /auth/register` mit `email`, `password`
2. Service prüft Policy (Länge, Komplexität)
3. Service prüft Eindeutigkeit der E-Mail
4. Argon2id-Hash des Passworts erzeugen
5. User-Datensatz speichern (in einer Transaktion mit Outbox-Event `user.registered`)
6. Response: 201 mit User-ID
7. *Kein automatischer Login* — Client muss sich danach explizit einloggen

**Fehlerfälle:**
- E-Mail bereits vergeben → 409 `email_taken`
- Passwort zu schwach → 400 `password_too_weak`

### UC-2: Login
**Akteur:** Registrierter User  
**Vorbedingung:** User existiert, Account nicht gesperrt  
**Ablauf:**
1. Client schickt `POST /auth/login` mit `email`, `password`
2. User per E-Mail laden
3. Argon2id-Verify gegen gespeicherten Hash
4. Bei Erfolg: Access-Token (15 min) + Refresh-Token (30 Tage) ausstellen
5. Refresh-Token-Hash in DB speichern
6. Login-Versuch im Audit-Log festhalten
7. Response: `{ access_token, refresh_token, expires_in }`

**Fehlerfälle:**
- E-Mail/Passwort falsch → 401 `invalid_credentials` (gleiche Fehlermeldung für beide Fälle, um User-Enumeration zu verhindern)
- Zu viele Fehlversuche (>5 in 15 min) → 429 `rate_limited`

### UC-3: Token-Refresh
**Akteur:** Client mit gültigem Refresh-Token  
**Ablauf:**
1. Client schickt `POST /auth/refresh` mit Refresh-Token
2. Service hasht den Token, sucht in `refresh_tokens`
3. Prüft `expires_at` und `revoked_at`
4. Bei Erfolg:
   - Alten Refresh-Token revozieren (`revoked_at = NOW()`)
   - Neuen Access- und Refresh-Token ausstellen (Rotation)
5. Response: neue Tokens

**Designentscheidung — Rotation:** Jeder Refresh erzeugt einen neuen Refresh-Token, der alte wird sofort invalide. Vorteile: gestohlener Token wird beim nächsten Refresh des echten Users entdeckt (Refresh-Token-Reuse-Detection).

### UC-4: Logout
**Akteur:** Eingeloggter User  
**Ablauf:**
1. Client schickt `POST /auth/logout` mit Refresh-Token
2. Refresh-Token wird revoziert
3. Access-Token bleibt bis Ablauf gültig (kein Server-State, aber kurze Lifetime von 15min macht das unkritisch)
4. Response: 204

**Optional für später:** Token-Blocklist via Redis für sofortiges Invalidieren auch des Access-Tokens.

### UC-5: Passwort-Reset anfordern
**Akteur:** User, der sein Passwort vergessen hat  
**Ablauf:**
1. Client schickt `POST /auth/password-reset/request` mit `email`
2. Service prüft, ob E-Mail existiert
3. *Unabhängig vom Ergebnis:* Service antwortet mit 202 (verhindert User-Enumeration)
4. Wenn E-Mail existiert: Reset-Token generieren (random 32 Byte, hex-codiert), Hash in DB speichern, gültig 1h
5. E-Mail mit Reset-Link versenden

### UC-6: Passwort-Reset durchführen
**Akteur:** User mit Reset-Token  
**Ablauf:**
1. Client schickt `POST /auth/password-reset/confirm` mit `token`, `new_password`
2. Token-Hash suchen, `expires_at` prüfen, `used_at` prüfen
3. Neues Passwort hashen, User-Datensatz aktualisieren
4. Reset-Token als verwendet markieren
5. **Alle Refresh-Tokens des Users revozieren** (sicher ist sicher)
6. Response: 204

### UC-7: JWKS-Bereitstellung
**Akteur:** Andere Services im System  
**Ablauf:**
1. Service ruft `GET /.well-known/jwks.json`
2. Response: Liste aller aktiven Public Keys im JWKS-Format
3. Andere Services cachen das (TTL 10 min) und nutzen die Keys zur JWT-Validierung

---

## 3. Datenmodell

### 3.1 Tabellen

```sql
-- Users
CREATE TABLE users (
    id              UUID PRIMARY KEY,
    email           CITEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,
    last_login_at   TIMESTAMPTZ NULL
);

CREATE INDEX idx_users_email_active ON users (email) WHERE deleted_at IS NULL;

-- Refresh Tokens
CREATE TABLE refresh_tokens (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT NOT NULL UNIQUE,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ NULL,
    replaced_by     UUID NULL REFERENCES refresh_tokens(id),
    user_agent      TEXT NULL,
    ip_address      INET NULL
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_active 
    ON refresh_tokens (token_hash) 
    WHERE revoked_at IS NULL;

-- Password Reset Tokens
CREATE TABLE password_reset_tokens (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT NOT NULL UNIQUE,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ NULL
);

CREATE INDEX idx_password_reset_active 
    ON password_reset_tokens (token_hash) 
    WHERE used_at IS NULL;

-- Login Attempts (Audit + Rate Limiting Support)
CREATE TABLE login_attempts (
    id              UUID PRIMARY KEY,
    email           CITEXT NOT NULL,
    success         BOOLEAN NOT NULL,
    ip_address      INET NULL,
    user_agent      TEXT NULL,
    attempted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_login_attempts_email_time 
    ON login_attempts (email, attempted_at DESC);

-- Signing Keys
CREATE TABLE signing_keys (
    id              UUID PRIMARY KEY,
    kid             TEXT NOT NULL UNIQUE,           -- Key ID für JWKS
    algorithm       TEXT NOT NULL DEFAULT 'RS256',
    private_key_pem TEXT NOT NULL,                  -- verschlüsselt at rest
    public_key_pem  TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at    TIMESTAMPTZ NOT NULL,
    retired_at      TIMESTAMPTZ NULL,               -- nicht mehr zum Signieren
    deleted_at      TIMESTAMPTZ NULL                -- nicht mehr zum Validieren
);

CREATE INDEX idx_signing_keys_active 
    ON signing_keys (activated_at) 
    WHERE deleted_at IS NULL;

-- Outbox (siehe ARCHITECTURE.md, Abschnitt 7.5)
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished 
    ON outbox (occurred_at) 
    WHERE published_at IS NULL;
```

### 3.2 Konvention CITEXT

`CITEXT` (case-insensitive text) für E-Mails — `User@Example.com` und `user@example.com` werden als identisch behandelt. Erfordert `CREATE EXTENSION citext;` in der Migration.

### 3.3 Migration-Dateien

```
services/auth/migrations/
├── 0001_init.up.sql      # Extensions + Tabellen oben
├── 0001_init.down.sql
├── 0002_seed_signing_key.up.sql   # Initialer Schlüssel
└── 0002_seed_signing_key.down.sql
```

---

## 4. Domain-Modell

### 4.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type User struct {
    ID           uuid.UUID
    Email        string
    PasswordHash string
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    *time.Time
    LastLoginAt  *time.Time
}

type RefreshToken struct {
    ID         uuid.UUID
    UserID     uuid.UUID
    TokenHash  string
    IssuedAt   time.Time
    ExpiresAt  time.Time
    RevokedAt  *time.Time
    ReplacedBy *uuid.UUID
}

type PasswordResetToken struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    TokenHash string
    IssuedAt  time.Time
    ExpiresAt time.Time
    UsedAt    *time.Time
}

type SigningKey struct {
    ID            uuid.UUID
    KID           string
    Algorithm     string
    PrivateKeyPEM string
    PublicKeyPEM  string
    CreatedAt     time.Time
    ActivatedAt   time.Time
    RetiredAt     *time.Time
    DeletedAt     *time.Time
}
```

### 4.2 Service-Layer (Use-Cases)

```go
package domain

type AuthService interface {
    Register(ctx context.Context, email, password string) (*User, error)
    Login(ctx context.Context, email, password, ip, userAgent string) (*TokenPair, error)
    Refresh(ctx context.Context, refreshToken string) (*TokenPair, error)
    Logout(ctx context.Context, refreshToken string) error
    RequestPasswordReset(ctx context.Context, email string) error
    ConfirmPasswordReset(ctx context.Context, token, newPassword string) error
    GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
}

type TokenPair struct {
    AccessToken  string
    RefreshToken string
    ExpiresIn    int // seconds
    TokenType    string // "Bearer"
}
```

### 4.3 Domain-Fehler

```go
package domain

var (
    ErrEmailTaken         = &Error{Code: "email_taken", Message: "email already registered"}
    ErrInvalidCredentials = &Error{Code: "invalid_credentials", Message: "invalid email or password"}
    ErrPasswordTooWeak    = &Error{Code: "password_too_weak", Message: "password does not meet policy"}
    ErrTokenInvalid       = &Error{Code: "token_invalid", Message: "token is invalid or expired"}
    ErrTokenRevoked       = &Error{Code: "token_revoked", Message: "token has been revoked"}
    ErrUserNotFound       = &Error{Code: "user_not_found", Message: "user not found"}
    ErrRateLimited        = &Error{Code: "rate_limited", Message: "too many attempts"}
)
```

---

## 5. HTTP-API (OpenAPI)

Vollständige Spezifikation in `docs/api/auth.openapi.yaml`. Hier kompakt mit den wichtigsten Schemas.

### 5.1 OpenAPI 3.1 (Auszug — komplettes File separat)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Auth Service
  version: 1.0.0
  description: Identity and authentication service for TeamBoard
servers:
  - url: http://localhost:8001/api/v1
    description: Local development
paths:
  /auth/register:
    post:
      summary: Register a new user
      operationId: registerUser
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RegisterRequest'
      responses:
        '201':
          description: User created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/UserResponse'
        '400': { $ref: '#/components/responses/ValidationError' }
        '409': { $ref: '#/components/responses/Conflict' }

  /auth/login:
    post:
      summary: Login with email and password
      operationId: loginUser
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/LoginRequest'
      responses:
        '200':
          description: Login successful
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TokenPair'
        '401': { $ref: '#/components/responses/Unauthorized' }
        '429': { $ref: '#/components/responses/RateLimited' }

  /auth/refresh:
    post:
      summary: Refresh access token
      operationId: refreshToken
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RefreshRequest'
      responses:
        '200':
          description: Tokens refreshed
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TokenPair'
        '401': { $ref: '#/components/responses/Unauthorized' }

  /auth/logout:
    post:
      summary: Logout (revoke refresh token)
      operationId: logout
      tags: [auth]
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RefreshRequest'
      responses:
        '204': { description: Logged out }

  /auth/me:
    get:
      summary: Get current user info
      operationId: getCurrentUser
      tags: [auth]
      security:
        - bearerAuth: []
      responses:
        '200':
          description: Current user
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/UserResponse'
        '401': { $ref: '#/components/responses/Unauthorized' }

  /auth/password-reset/request:
    post:
      summary: Request password reset
      operationId: requestPasswordReset
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PasswordResetRequestBody'
      responses:
        '202':
          description: Request accepted (always 202 to prevent enumeration)

  /auth/password-reset/confirm:
    post:
      summary: Confirm password reset with token
      operationId: confirmPasswordReset
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PasswordResetConfirmBody'
      responses:
        '204': { description: Password reset successful }
        '400': { $ref: '#/components/responses/ValidationError' }

  /.well-known/jwks.json:
    get:
      summary: Public keys for JWT validation
      operationId: getJWKS
      tags: [keys]
      responses:
        '200':
          description: JWKS document
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/JWKS'

  /health/live:
    get:
      summary: Liveness probe
      operationId: getLiveness
      tags: [health]
      responses:
        '200': { description: Service is live }

  /health/ready:
    get:
      summary: Readiness probe
      operationId: getReadiness
      tags: [health]
      responses:
        '200': { description: Service is ready }
        '503': { description: Service not ready }

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

  schemas:
    RegisterRequest:
      type: object
      required: [email, password]
      properties:
        email:
          type: string
          format: email
          maxLength: 255
        password:
          type: string
          minLength: 12
          maxLength: 128

    LoginRequest:
      type: object
      required: [email, password]
      properties:
        email: { type: string, format: email }
        password: { type: string }

    RefreshRequest:
      type: object
      required: [refresh_token]
      properties:
        refresh_token: { type: string }

    PasswordResetRequestBody:
      type: object
      required: [email]
      properties:
        email: { type: string, format: email }

    PasswordResetConfirmBody:
      type: object
      required: [token, new_password]
      properties:
        token: { type: string }
        new_password:
          type: string
          minLength: 12
          maxLength: 128

    TokenPair:
      type: object
      required: [access_token, refresh_token, expires_in, token_type]
      properties:
        access_token: { type: string }
        refresh_token: { type: string }
        expires_in: { type: integer, description: "Access token lifetime in seconds" }
        token_type: { type: string, enum: [Bearer] }

    UserResponse:
      type: object
      required: [data]
      properties:
        data:
          type: object
          required: [id, email, created_at]
          properties:
            id: { type: string, format: uuid }
            email: { type: string, format: email }
            created_at: { type: string, format: date-time }
            last_login_at: { type: string, format: date-time, nullable: true }

    JWKS:
      type: object
      required: [keys]
      properties:
        keys:
          type: array
          items: { $ref: '#/components/schemas/JWK' }

    JWK:
      type: object
      required: [kty, kid, use, alg, n, e]
      properties:
        kty: { type: string, enum: [RSA] }
        kid: { type: string }
        use: { type: string, enum: [sig] }
        alg: { type: string, enum: [RS256] }
        n: { type: string, description: "RSA modulus (base64url)" }
        e: { type: string, description: "RSA exponent (base64url)" }

    Problem:
      type: object
      required: [type, title, status]
      properties:
        type: { type: string, format: uri }
        title: { type: string }
        status: { type: integer }
        detail: { type: string }
        trace_id: { type: string }
        errors:
          type: array
          items:
            type: object
            properties:
              field: { type: string }
              message: { type: string }

  responses:
    ValidationError:
      description: Validation failed
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
    Unauthorized:
      description: Authentication failed
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
    Conflict:
      description: Resource conflict
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
    RateLimited:
      description: Too many requests
      content:
        application/problem+json:
          schema: { $ref: '#/components/schemas/Problem' }
```

### 5.2 Endpoint-Übersicht

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|--------------|
| POST | `/auth/register` | öffentlich | User registrieren |
| POST | `/auth/login` | öffentlich | Login |
| POST | `/auth/refresh` | öffentlich (Refresh-Token im Body) | Token erneuern |
| POST | `/auth/logout` | Bearer | Logout |
| GET | `/auth/me` | Bearer | Aktueller User |
| POST | `/auth/password-reset/request` | öffentlich | Reset anfordern |
| POST | `/auth/password-reset/confirm` | öffentlich | Reset durchführen |
| GET | `/.well-known/jwks.json` | öffentlich | JWKS |
| POST | `/auth/tokens` | Bearer | Personal Access Token erstellen (§17) |
| GET | `/auth/tokens` | Bearer | Eigene PATs auflisten (§17) |
| DELETE | `/auth/tokens/{id}` | Bearer | PAT revozieren (§17) |
| POST | `/internal/tokens/introspect` | Service-Token | PAT auflösen, für andere Services (§17) |
| GET | `/health/live` | öffentlich | Liveness |
| GET | `/health/ready` | öffentlich | Readiness |

---

## 6. JWT-Spezifikation

### 6.1 Access-Token

**Algorithmus:** RS256  
**Lifetime:** 15 Minuten  
**Header:**
```json
{
  "alg": "RS256",
  "typ": "JWT",
  "kid": "key-2026-05-01"
}
```
**Payload:**
```json
{
  "iss": "https://auth.teamboard.local",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "aud": ["teamboard-api"],
  "iat": 1715000000,
  "exp": 1715000900,
  "jti": "01HABC...",
  "email": "user@example.com",
  "scope": "user"
}
```

**Pflicht-Claims:**
- `iss` — Issuer, fester String aus Config
- `sub` — User-UUID
- `aud` — Audience, `["teamboard-api"]`
- `iat`, `exp` — Standard-Zeiten
- `jti` — Eindeutige JWT-ID (ULID)
- `email` — User-E-Mail (Convenience)
- `scope` — `user` (kein RBAC im Token, Permissions kommen vom Project Service)

### 6.2 Refresh-Token

**Format:** Opaque, kryptographisch zufällig (32 Byte, hex-codiert = 64 Zeichen)  
**Speicherung:** Nur Hash (SHA-256) in DB  
**Lifetime:** 30 Tage  
**Eigenschaft:** Single-use (Rotation bei jedem Refresh)

Begründung gegen JWT-Refresh-Tokens: Refresh-Tokens müssen revokierbar sein. Bei JWT bräuchte man eine Blocklist. Opaque + DB ist einfacher und sicherer.

---

## 7. Schlüsselverwaltung und Rotation

### 7.1 Schlüssel-Lifecycle

Jeder Signing-Key durchläuft drei Phasen:

| Phase | Status-Felder | Bedeutung |
|-------|---------------|-----------|
| **Active (signing)** | `activated_at` gesetzt, `retired_at` NULL | Wird zum Signieren neuer Tokens verwendet |
| **Retired (validation only)** | `retired_at` gesetzt, `deleted_at` NULL | Wird nicht mehr zum Signieren genutzt, aber noch als gültig akzeptiert (für noch nicht abgelaufene Tokens) |
| **Deleted** | `deleted_at` gesetzt | Wird nicht mehr exposed, Validierung schlägt fehl |

### 7.2 Rotations-Algorithmus

**Standard-Rotation alle 30 Tage:**
1. Neuen Key generieren (RSA 2048)
2. Mit `activated_at = NOW()` einfügen
3. Bisherigen aktiven Key auf `retired_at = NOW()` setzen
4. Keys mit `retired_at < NOW() - 30min` (= max Access-Token-Lifetime + Puffer) auf `deleted_at = NOW()` setzen
5. Cron-Job in CronJob-Service oder als Goroutine

### 7.3 Auswahl beim Signieren

```sql
SELECT * FROM signing_keys
WHERE retired_at IS NULL AND deleted_at IS NULL
ORDER BY activated_at DESC
LIMIT 1;
```

### 7.4 JWKS-Endpoint

Liefert alle Keys mit `deleted_at IS NULL`:

```sql
SELECT kid, algorithm, public_key_pem
FROM signing_keys
WHERE deleted_at IS NULL
ORDER BY activated_at DESC;
```

PEM in JWK-Format konvertieren (Bibliothek: `github.com/lestrrat-go/jwx/v2/jwk`).

### 7.5 Verschlüsselung des Private Keys at rest

**MVP:** Private Key in PEM in der DB (akzeptabel, weil DB-Zugriff selbst geschützt).  
**Produktion:** AES-GCM-Verschlüsselung mit Master-Key aus Secrets Manager / KMS. Spalte heißt schon richtig "private_key_pem" — wird transparent beim Lesen entschlüsselt.

---

## 8. Refresh-Token-Strategie

### 8.1 Generierung

```go
func generateRefreshToken() (token, hash string, err error) {
    bytes := make([]byte, 32)
    if _, err := rand.Read(bytes); err != nil {
        return "", "", err
    }
    token = hex.EncodeToString(bytes)
    sum := sha256.Sum256([]byte(token))
    hash = hex.EncodeToString(sum[:])
    return token, hash, nil
}
```

### 8.2 Refresh-Token-Reuse-Detection

**Szenario:** Angreifer stiehlt Refresh-Token, nutzt ihn. Echter User loggt sich später ein und nutzt den nun ungültigen alten Token → Service entdeckt Reuse.

**Implementierung:**
1. Beim Refresh: alten Token suchen
2. Wenn gefunden, aber `revoked_at IS NOT NULL` und `replaced_by IS NOT NULL` → **Reuse erkannt**
3. Aktion: **alle aktiven Refresh-Tokens des Users revozieren** (Sicherheitsmaßnahme)
4. Event publizieren `auth.suspicious_activity` (für Notification an User)

### 8.3 Cleanup

Cron-Job entfernt Tokens, die seit >30 Tagen abgelaufen oder revoziert sind:

```sql
DELETE FROM refresh_tokens
WHERE (expires_at < NOW() - INTERVAL '30 days')
   OR (revoked_at < NOW() - INTERVAL '30 days');
```

---

## 9. Passwort-Handling

### 9.1 Hashing

**Argon2id** mit folgenden Parametern (RFC 9106, OWASP-Empfehlung 2024):

```go
// Argon2id-Parameter (Memory-hard)
memory      = 64 * 1024  // 64 MiB
iterations  = 3
parallelism = 2
saltLength  = 16
keyLength   = 32
```

Bibliothek: `golang.org/x/crypto/argon2`

**Hash-Format:** PHC-String wie `$argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>` — enthält Parameter, ermöglicht zukünftige Migration.

### 9.2 Passwort-Policy

**MVP-Regeln:**
- Mindestens 12 Zeichen
- Maximal 128 Zeichen
- Keine spezifischen Komplexitätsregeln (wie "muss Sonderzeichen enthalten")

**Begründung:** NIST SP 800-63B-Empfehlungen. Lange Passphrasen sind sicherer als kurze, komplexe. Längere Mindestlänge ersetzt Komplexitätsregeln.

**Erweiterung später:** Abgleich gegen "Have I Been Pwned"-API (k-anonymity).

### 9.3 Timing-Angriffs-Schutz

Bei nicht existierender E-Mail trotzdem einen Argon2-Hash berechnen (gegen einen Dummy-Hash), damit Antwortzeit identisch ist:

```go
func (s *service) Login(ctx context.Context, email, password string) (*TokenPair, error) {
    user, err := s.repo.GetUserByEmail(ctx, email)
    if err == ErrUserNotFound {
        // Dummy-Verify, damit Timing identisch ist
        argon2.IDKey([]byte(password), dummySalt, 3, 64*1024, 2, 32)
        return nil, ErrInvalidCredentials
    }
    if err != nil { return nil, err }
    if !verifyPassword(password, user.PasswordHash) {
        return nil, ErrInvalidCredentials
    }
    // ...
}
```

---

## 10. Events

Verwendet Outbox-Pattern (siehe ARCHITECTURE.md, Abschnitt 7.5).

### 10.1 `user.registered`

Publiziert bei UC-1 (Registrierung).

```json
{
  "event_id": "01HABC...",
  "event_type": "user.registered",
  "event_version": 1,
  "occurred_at": "2026-05-06T12:34:56.789Z",
  "trace_id": "abc123",
  "producer": "auth-service",
  "aggregate_type": "user",
  "aggregate_id": "550e8400-...",
  "actor": { "user_id": "550e8400-...", "type": "user" },
  "payload": {
    "user_id": "550e8400-...",
    "email": "user@example.com",
    "created_at": "2026-05-06T12:34:56.789Z"
  }
}
```

### 10.2 `user.deleted`

Publiziert bei Soft-Delete eines Users (späterer Use-Case, hier nur Schema).

```json
{
  "event_type": "user.deleted",
  "event_version": 1,
  "payload": {
    "user_id": "550e8400-...",
    "deleted_at": "2026-05-06T12:34:56.789Z"
  }
}
```

### 10.3 Konsumierte Events

**Keine** — der Auth Service ist Source of Truth für Identität, andere Services konsumieren seine Events, nicht umgekehrt.

---

## 11. Sicherheit

### 11.1 Rate Limiting

**Pro IP + E-Mail-Kombination:**

| Endpoint | Limit |
|----------|-------|
| `/auth/login` | 5 Versuche / 15 min |
| `/auth/register` | 3 / Stunde |
| `/auth/password-reset/request` | 3 / Stunde |

**Implementation MVP:** Sliding-Window-Counter in Redis. Wenn Redis nicht verfügbar → fail open (keine Begrenzung) mit Log-Warning.

### 11.2 HTTPS only

Alle Production-Calls über HTTPS. In Dev erlaubt der Service auch HTTP, aber das Setting ist explicit (`ALLOW_INSECURE_HTTP=true`).

### 11.3 Secrets

| Secret | Speicherort |
|--------|-------------|
| DB-Passwort | Secrets Manager / .env (lokal) |
| Master-Key für Private-Key-Encryption | Secrets Manager / KMS |
| SMTP-Credentials | Secrets Manager |

Niemals in Git, niemals im Image.

### 11.4 Audit-Log

Alle sicherheitsrelevanten Events werden geloggt:
- Registrierung
- Login (erfolgreich/fehlgeschlagen)
- Passwort-Reset angefordert
- Passwort-Reset durchgeführt
- Refresh-Token-Reuse erkannt
- Schlüssel-Rotation

Format: strukturiert via `slog`, mit Trace-ID, User-ID, IP, User-Agent.

### 11.5 OWASP-Checkliste

| Schutz gegen | Maßnahme |
|--------------|----------|
| Brute-Force | Rate Limiting + Argon2id |
| User-Enumeration | Identische Antwort und Timing für unbekannte/falsche E-Mail |
| Replay | JWT mit `exp`, Refresh-Token Single-Use |
| CSRF | Tokens im Authorization-Header (kein Cookie-basierter Auth-Flow) |
| Session-Hijacking | kurze Access-Token-Lifetime, IP/UA bei Refresh-Token loggen |
| Password-Spraying | Rate Limiting + Account-Lockout-Optionen für später |

---

## 12. Konfiguration

### 12.1 Environment-Variablen

```bash
# Service
SERVICE_NAME=auth-service
SERVICE_PORT=8001
LOG_LEVEL=info

# Database
DB_URL=postgres://auth:auth@postgres:5432/auth_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events

# JWT
JWT_ISSUER=https://auth.teamboard.local
JWT_AUDIENCE=teamboard-api
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=720h         # 30 Tage
JWT_KEY_ROTATION_INTERVAL=720h     # 30 Tage

# Password Policy
PASSWORD_MIN_LENGTH=12
PASSWORD_MAX_LENGTH=128

# Rate Limiting
REDIS_URL=redis://redis:6379/0
RATE_LIMIT_ENABLED=true

# Mail
SMTP_HOST=mailhog
SMTP_PORT=1025
SMTP_FROM=noreply@teamboard.local
PASSWORD_RESET_TOKEN_TTL=1h
PASSWORD_RESET_BASE_URL=http://localhost:3000/reset-password

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=auth-service

# Security
ALLOW_INSECURE_HTTP=false
KEY_ENCRYPTION_KEY=<base64-32bytes>  # für Private-Key-Verschlüsselung
```

### 12.2 Config-Struct (Go)

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"auth-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8001"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
    }

    RabbitMQ struct {
        URL      string `env:"RABBITMQ_URL,required"`
        Exchange string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
    }

    JWT struct {
        Issuer              string        `env:"JWT_ISSUER,required"`
        Audience            string        `env:"JWT_AUDIENCE,required"`
        AccessTokenTTL      time.Duration `env:"JWT_ACCESS_TOKEN_TTL" envDefault:"15m"`
        RefreshTokenTTL     time.Duration `env:"JWT_REFRESH_TOKEN_TTL" envDefault:"720h"`
        KeyRotationInterval time.Duration `env:"JWT_KEY_ROTATION_INTERVAL" envDefault:"720h"`
    }

    Password struct {
        MinLength int `env:"PASSWORD_MIN_LENGTH" envDefault:"12"`
        MaxLength int `env:"PASSWORD_MAX_LENGTH" envDefault:"128"`
    }

    Redis struct {
        URL              string `env:"REDIS_URL"`
        RateLimitEnabled bool   `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
    }

    SMTP struct {
        Host     string `env:"SMTP_HOST"`
        Port     int    `env:"SMTP_PORT" envDefault:"1025"`
        From     string `env:"SMTP_FROM,required"`
        Username string `env:"SMTP_USERNAME"`
        Password string `env:"SMTP_PASSWORD"`
    }

    PasswordReset struct {
        TokenTTL time.Duration `env:"PASSWORD_RESET_TOKEN_TTL" envDefault:"1h"`
        BaseURL  string        `env:"PASSWORD_RESET_BASE_URL,required"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }

    Security struct {
        AllowInsecureHTTP   bool   `env:"ALLOW_INSECURE_HTTP" envDefault:"false"`
        KeyEncryptionKeyB64 string `env:"KEY_ENCRYPTION_KEY,required"`
    }
}
```

---

## 13. Verzeichnisstruktur

```
services/auth/
├── cmd/
│   └── server/
│       └── main.go                      # Wiring + HTTP-Server
├── internal/
│   ├── api/
│   │   ├── router.go                    # Chi-Router-Setup
│   │   ├── middleware.go                # Auth, Logging, Tracing
│   │   ├── handlers_auth.go             # /auth/* Endpoints
│   │   ├── handlers_jwks.go             # /.well-known/jwks.json
│   │   ├── handlers_health.go           # /health/*
│   │   ├── dto.go                       # Request/Response-Strukturen
│   │   └── generated.go                 # oapi-codegen Output
│   ├── domain/
│   │   ├── user.go                      # User-Entity
│   │   ├── token.go                     # RefreshToken, PasswordResetToken
│   │   ├── signing_key.go               # SigningKey
│   │   ├── service.go                   # AuthService Interface
│   │   ├── service_impl.go              # AuthService-Implementation
│   │   ├── password.go                  # Argon2id-Hashing/Verify
│   │   ├── jwt.go                       # JWT-Erzeugung
│   │   └── errors.go                    # Domain-Errors
│   ├── repository/
│   │   ├── db/                          # sqlc-generiert (read-only)
│   │   │   ├── querier.go
│   │   │   ├── models.go
│   │   │   ├── users.sql.go
│   │   │   ├── refresh_tokens.sql.go
│   │   │   ├── password_reset.sql.go
│   │   │   ├── signing_keys.sql.go
│   │   │   └── outbox.sql.go
│   │   ├── repository.go                # Interface
│   │   └── postgres.go                  # Implementation
│   ├── keys/
│   │   ├── manager.go                   # Key-Rotation + JWKS-Build
│   │   └── crypto.go                    # AES-GCM für Private-Key-Encryption
│   ├── ratelimit/
│   │   └── redis_limiter.go             # Sliding-Window-Counter
│   ├── mail/
│   │   ├── sender.go                    # Interface
│   │   ├── smtp.go                      # SMTP-Implementation
│   │   └── templates/
│   │       └── password_reset.html
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   ├── 0001_init.down.sql
│   ├── 0002_seed_signing_key.up.sql
│   └── 0002_seed_signing_key.down.sql
├── queries/                              # sqlc-Input
│   ├── users.sql
│   ├── refresh_tokens.sql
│   ├── password_reset.sql
│   ├── signing_keys.sql
│   ├── login_attempts.sql
│   └── outbox.sql
├── sqlc.yaml
├── Dockerfile
├── go.mod
├── go.sum
├── .air.toml                             # Hot-Reload-Config
├── Makefile                              # Service-spezifische Targets
└── README.md
```

---

## 14. sqlc-Queries

### 14.1 `queries/users.sql`

```sql
-- name: CreateUser :one
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateLastLogin :exec
UPDATE users SET last_login_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: UpdatePasswordHash :exec
UPDATE users SET password_hash = $2, updated_at = NOW()
WHERE id = $1;

-- name: SoftDeleteUser :exec
UPDATE users SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1;
```

### 14.2 `queries/refresh_tokens.sql`

```sql
-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, user_agent, ip_address)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserTokens :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW() - INTERVAL '30 days'
   OR revoked_at < NOW() - INTERVAL '30 days';
```

### 14.3 `queries/password_reset.sql`

```sql
-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPasswordResetTokenByHash :one
SELECT * FROM password_reset_tokens
WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW();

-- name: MarkPasswordResetTokenUsed :exec
UPDATE password_reset_tokens
SET used_at = NOW()
WHERE id = $1;
```

### 14.4 `queries/signing_keys.sql`

```sql
-- name: CreateSigningKey :one
INSERT INTO signing_keys (id, kid, algorithm, private_key_pem, public_key_pem, activated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
RETURNING *;

-- name: GetActiveSigningKey :one
SELECT * FROM signing_keys
WHERE retired_at IS NULL AND deleted_at IS NULL
ORDER BY activated_at DESC
LIMIT 1;

-- name: ListValidatingSigningKeys :many
SELECT * FROM signing_keys
WHERE deleted_at IS NULL
ORDER BY activated_at DESC;

-- name: GetSigningKeyByKID :one
SELECT * FROM signing_keys
WHERE kid = $1 AND deleted_at IS NULL;

-- name: RetireSigningKey :exec
UPDATE signing_keys
SET retired_at = NOW()
WHERE id = $1 AND retired_at IS NULL;

-- name: DeleteRetiredSigningKeys :exec
UPDATE signing_keys
SET deleted_at = NOW()
WHERE retired_at IS NOT NULL
  AND retired_at < NOW() - INTERVAL '30 minutes'
  AND deleted_at IS NULL;
```

### 14.5 `queries/login_attempts.sql`

```sql
-- name: RecordLoginAttempt :exec
INSERT INTO login_attempts (id, email, success, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5);

-- name: CountRecentFailedAttempts :one
SELECT COUNT(*) FROM login_attempts
WHERE email = $1
  AND success = FALSE
  AND attempted_at > NOW() - INTERVAL '15 minutes';
```

### 14.6 `queries/outbox.sql`

```sql
-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: GetUnpublishedEvents :many
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY occurred_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE outbox SET published_at = NOW()
WHERE id = $1;
```

### 14.7 `sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries"
    schema: "migrations"
    gen:
      go:
        package: "db"
        out: "internal/repository/db"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_types: true
        emit_interface: true
```

---

## 15. Test-Strategie

### 15.1 Unit-Tests (Domain)

**Coverage-Ziel:** ≥ 80% in `internal/domain/`

**Beispielhafte Test-Cases:**

```go
func TestPasswordPolicy(t *testing.T) {
    tests := []struct{
        name     string
        password string
        wantErr  error
    }{
        {"too short", "abc", ErrPasswordTooWeak},
        {"acceptable", "correct horse battery staple", nil},
        {"too long", strings.Repeat("a", 129), ErrPasswordTooWeak},
    }
    // ...
}

func TestArgon2idVerify(t *testing.T) {
    hash, err := HashPassword("hello world 12345")
    require.NoError(t, err)
    require.True(t, VerifyPassword("hello world 12345", hash))
    require.False(t, VerifyPassword("wrong password", hash))
}

func TestJWTSignAndVerify(t *testing.T) {
    // Generiert Key, signiert Token, verifiziert mit Public Key
}
```

### 15.2 Integration-Tests (Repository + Events)

**Werkzeug:** `testcontainers-go` startet ephemere PostgreSQL-Instanz pro Testlauf.

```go
func TestUserRepository(t *testing.T) {
    pg := startPostgres(t)
    defer pg.Terminate()
    repo := NewPostgresRepository(pg.ConnectionString())

    t.Run("create and get user", func(t *testing.T) {
        u, err := repo.CreateUser(ctx, "test@example.com", "hash")
        require.NoError(t, err)
        got, err := repo.GetUserByEmail(ctx, "test@example.com")
        require.NoError(t, err)
        require.Equal(t, u.ID, got.ID)
    })

    t.Run("duplicate email returns error", func(t *testing.T) {
        // ...
    })
}
```

### 15.3 End-to-End-Tests

**Werkzeug:** Postman-Collection unter `tests/postman/auth.postman_collection.json` mit folgenden Szenarien:

1. **Happy Path Registrierung → Login → /me → Logout**
2. **Register mit dupliziertem E-Mail → 409**
3. **Login mit falschem Passwort → 401**
4. **Refresh-Token-Flow**
5. **Refresh-Token-Reuse-Detection** (alten Token zweimal nutzen → alle Tokens revoked)
6. **Passwort-Reset-Flow**

### 15.4 Test-Daten / Seed

`scripts/seed-auth.sh`:
```bash
#!/bin/bash
curl -X POST http://localhost:8001/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@teamboard.local","password":"strongPassword123!"}'
curl -X POST http://localhost:8001/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"bob@teamboard.local","password":"strongPassword123!"}'
```

---

## 16. Implementierungs-Hinweise für Coding-Agents

### 16.1 Implementierungsreihenfolge

Empfohlene Reihenfolge für TDD-orientiertes Vorgehen:

1. **Migrations + sqlc-Setup** (`migrations/`, `queries/`, `sqlc.yaml`) → `make generate` läuft
2. **Domain-Modell** (`internal/domain/`) inkl. Errors und Password-Hashing — **mit Unit-Tests**
3. **Repository-Layer** (`internal/repository/`) mit Testcontainers-Integration-Test
4. **JWT + Key-Manager** (`internal/keys/`, `internal/domain/jwt.go`)
5. **Service-Layer** (`internal/domain/service_impl.go`) mit Mock-Repository
6. **HTTP-Handler** (`internal/api/`)
7. **Outbox-Publishing** — shared `outbox.Worker` + `eventbus.NewPublisher`, verdrahtet in `main.go` (kein service-eigener Publisher mehr)
8. **Wiring in `main.go`**
9. **End-to-End-Test gegen Docker-Compose-Stack**

### 16.2 Verbindliche Konventionen

- **Niemals** Klartext-Passwörter loggen, weder im Request-Body noch in Errors
- **Niemals** vollständige Tokens loggen, nur die ersten 8 Zeichen + Hash-Präfix
- **Immer** `pgx.BeginTx` für Operationen, die DB-Update + Outbox-Insert kombinieren
- **Immer** Response-Bodys gegen das OpenAPI-Schema validieren (im Test)
- **Niemals** `interface{}` oder `any` außerhalb von JSON-Marshalling-Grenzen

### 16.3 Typische Stolperfallen

- **CITEXT-Extension** muss in der Migration aktiviert werden, sonst schlägt `email CITEXT` fehl
- **pgx und sqlc:** Bei nullable-Feldern generiert sqlc Pointer-Typen. Konsistent verwenden, nicht zu/von `sql.NullString` konvertieren
- **JWT-`aud`-Claim:** ist Array vs. String — bibliotheksabhängig, immer als Array `[]string{"teamboard-api"}` setzen
- **Argon2id-Parameter:** beim Validieren aus dem PHC-String parsen, nicht hardcoden, sonst bricht künftige Migration auf höhere Parameter
- **Outbox-Worker:** muss `FOR UPDATE SKIP LOCKED` nutzen, sonst mehrfache Publikation bei mehreren Replikas
- **Key-Rotation beim Boot:** wenn keine Keys in DB, generiere ersten beim Start (Migration 0002 macht das)

### 16.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration migrate generate run docker-build

build:
	go build -o bin/auth ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/...

test-integration:
	go test -race ./internal/repository/...

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/auth.openapi.yaml > internal/api/generated.go

run:
	air

docker-build:
	docker build -t teamboard/auth:latest .
```

### 16.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|------|
| UC-1 Register | User in DB, Outbox-Event vorhanden, 201 mit User-Response, Passwort als Argon2id |
| UC-2 Login | TokenPair valide, Access-Token verifiziert mit Public Key, Login-Attempt geloggt |
| UC-3 Refresh | Alter Token revoziert mit `replaced_by`, neuer Token aktiv |
| UC-3.1 Reuse | Verwendung eines bereits-revozierten Tokens revoziert alle aktiven Tokens des Users |
| UC-4 Logout | Refresh-Token revoziert, 204 |
| UC-5 Reset Request | Antwort 202 unabhängig von Existenz der E-Mail |
| UC-6 Reset Confirm | Passwort aktualisiert, alle Refresh-Tokens revoziert, Reset-Token als used markiert |
| UC-7 JWKS | Liefert alle Keys mit `deleted_at IS NULL`, korrektes JWK-Format |

---

## 17. Personal Access Tokens (PAT)

### 17.1 Zweck

Langlebige, vom User selbst verwaltete Credentials für externe Clients, die sich nicht über den
normalen Login-Flow anmelden können — z. B. den TeamBoard-MCP-Server (`mcp-server/`) für Claude
Desktop. Erstellung/Verwaltung im Frontend unter **Settings**.

### 17.2 Format und Speicherung

**Format:** `tbpat_` + 32 zufällige Bytes (hex-codiert). Der `tbpat_`-Präfix ist nicht geheim —
er erlaubt `shared/go/authmiddleware`, ein Token ohne JWT-Parse-Versuch direkt an die
Introspection zu routen.

**Speicherung:** Nur SHA-256-Hash in `personal_access_tokens` (analog `refresh_tokens`, **nicht**
analog dem Webhook-Secret im Plugin-Service — letzteres muss server-seitig reproduzierbar sein,
ein PAT wird nur verglichen). Zusätzlich `token_prefix` (erste 12 Zeichen) für die Anzeige in der
Token-Liste.

```sql
personal_access_tokens (
    id, user_id, name, token_hash, token_prefix,
    created_at, expires_at, revoked_at, last_used_at
)
```

**Rechte:** Ein PAT hat dieselben Permissions wie ein normaler Login (kein eigenes RBAC/Scope —
davon gibt es aktuell nirgends im System eines). **TTL:** vom User bei Erstellung gewählt (30 /
90 / 365 Tage).

### 17.3 Endpunkte

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|--------------|
| POST | `/api/v1/auth/tokens` | Bearer | Neues PAT erstellen — Rohwert nur in dieser Response |
| GET | `/api/v1/auth/tokens` | Bearer | Eigene PATs auflisten (nie mit Rohwert) |
| DELETE | `/api/v1/auth/tokens/{id}` | Bearer | PAT revozieren (idempotent) |
| POST | `/api/v1/internal/tokens/introspect` | Service-Token | PAT → `{user_id, email}` auflösen |

### 17.4 Validierung durch andere Services

PATs sind **opaque**, nicht selbst-validierend wie Access-JWTs — andere Services können sie also
nicht rein über die JWKS prüfen. Project-, Task- und Board-Registry-Service akzeptieren sie
stattdessen über denselben Mechanismus, der bereits für Permission-Checks existiert: ein
`authclient.Client` pro Service (Kopie von `projectclient.Client`s Aufbau — In-Process-Cache
30s TTL + Circuit-Breaker) ruft `POST /api/v1/internal/tokens/introspect` synchron auf.
`shared/go/authmiddleware.Middleware` erkennt den `tbpat_`-Präfix und routet automatisch dorthin,
wenn ein Service `authmiddleware.WithPATIntrospector(...)` konfiguriert hat — ohne diese Option
bleibt das Verhalten für andere Services unverändert (nur RS256-JWTs).

**Eventual Consistency bei Revocation:** Eine Revocation braucht bis zu 30s (Cache-TTL), um bei
den aufrufenden Services anzukommen — keine Events, keine aktive Invalidierung. Dieselbe Klasse
von Verzögerung akzeptiert das System bereits beim Project-Service-Permission-Cache (P8).

---

## Anhang: Beispiel-Sequenzdiagramm Login-Flow

```
Client                  Gateway              Auth Service             Postgres        RabbitMQ
  │                        │                        │                        │              │
  │ POST /auth/login       │                        │                        │              │
  ├───────────────────────>│                        │                        │              │
  │                        │ POST /auth/login       │                        │              │
  │                        ├───────────────────────>│                        │              │
  │                        │                        │ GetUserByEmail         │              │
  │                        │                        ├───────────────────────>│              │
  │                        │                        │<───────────────────────┤              │
  │                        │                        │ Argon2id verify        │              │
  │                        │                        │ Generate JWT (RS256)   │              │
  │                        │                        │ INSERT refresh_token   │              │
  │                        │                        ├───────────────────────>│              │
  │                        │                        │ INSERT outbox (login)  │              │
  │                        │                        ├───────────────────────>│              │
  │                        │                        │ UPDATE last_login_at   │              │
  │                        │                        ├───────────────────────>│              │
  │                        │ 200 TokenPair          │                        │              │
  │                        │<───────────────────────┤                        │              │
  │ 200 TokenPair          │                        │                        │              │
  │<───────────────────────┤                        │                        │              │
  │                        │                        │ (Outbox-Worker        │              │
  │                        │                        │  liest async)          │              │
  │                        │                        ├───────────────────────>│              │
  │                        │                        │ Publish user.login     │              │
  │                        │                        ├──────────────────────────────────────>│
```

---

**Ende des Detail-Designs Auth Service.**
