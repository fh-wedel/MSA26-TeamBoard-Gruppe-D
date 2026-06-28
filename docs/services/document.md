# Document Service — Detail-Design

> **Verwandtes Dokument:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) — Master-Architektur  
> **Verwandtes Dokument:** [`services/project.md`](./project.md) — Authority für Permissions  
> **Service:** `document`  
> **Port (lokal):** 8004  
> **Datenbank:** `document_db` (PostgreSQL)  
> **Object Storage:** MinIO (lokal) / S3 (AWS)  
> **Stand:** 2026-05

---

## Inhaltsverzeichnis

1. [Verantwortung und Abgrenzung](#1-verantwortung-und-abgrenzung)
2. [Architektur-Pattern: Pre-Signed URLs](#2-architektur-pattern-pre-signed-urls)
3. [Use-Cases](#3-use-cases)
4. [Datenmodell](#4-datenmodell)
5. [Domain-Modell](#5-domain-modell)
6. [Versionierungs-Strategie](#6-versionierungs-strategie)
7. [Two-Phase Upload und Cleanup](#7-two-phase-upload-und-cleanup)
8. [HTTP-API (OpenAPI)](#8-http-api-openapi)
9. [Interne API für Service-zu-Service](#9-interne-api-für-service-zu-service)
10. [Object-Storage-Integration](#10-object-storage-integration)
11. [Events](#11-events)
12. [Konfiguration](#12-konfiguration)
13. [Verzeichnisstruktur](#13-verzeichnisstruktur)
14. [sqlc-Queries](#14-sqlc-queries)
15. [Test-Strategie](#15-test-strategie)
16. [Sicherheit](#16-sicherheit)
17. [Implementierungs-Hinweise für Coding-Agents](#17-implementierungs-hinweise-für-coding-agents)

---

## 1. Verantwortung und Abgrenzung

### 1.1 Verantwortet

- **Dokument-Metadaten** (Name, MIME-Type, Größe, aktuelle Version, Projekt-Zugehörigkeit)
- **Versionierung** — jede Version ein eigener Storage-Eintrag, keine Überschreibung
- **Pre-Signed URLs** für Upload und Download (Direkt-Transfer Client ↔ S3)
- **Upload-Bestätigung** und Cleanup verwaister Pending-Uploads
- **Soft-Delete** mit späterem Hard-Delete-Cleanup (Storage-Bytes erst dann freigegeben)
- **Internal-API** für andere Services (Existenz- und Project-Membership-Checks)

### 1.2 Verantwortet NICHT

- **Inhaltliche Verarbeitung** der Dokumente (kein Text-Extract, kein OCR, kein Thumbnailing — kommt ggf. als separater Worker-Service später)
- **Echtzeit-Kollaboration** an Dokumenten (keine Live-Edits — separater Service später)
- **Permissions** — Project Service ist Authority
- **Anhang-Verknüpfungen zu Tasks** — Task Service hat eigene `task_attachments`-Tabelle, hier nur Existenz-Checks
- **Bytes** — der Service liest und schreibt niemals Dokumentinhalte

### 1.3 Abhängigkeiten

| Abhängigkeit | Typ | Zweck |
|--------------|-----|-------|
| PostgreSQL `document_db` | hart | Metadaten-Persistenz |
| MinIO/S3 | hart | Bytes-Storage und Pre-Signed URLs |
| RabbitMQ | hart | Event-Publikation und -Konsum |
| Project Service | hart bei Schreibops | Permission-Checks |
| Auth Service (JWKS) | hart | JWT-Validierung |

---

## 2. Architektur-Pattern: Pre-Signed URLs

### 2.1 Das Problem

Klassische Implementierung: Client uploadet zum Service, Service streamt zu S3. Probleme:
- Service muss die volle Bandbreite verarbeiten
- Bei großen Dateien blockieren Worker-Threads
- Memory-Druck beim Buffering
- Doppelte Egress-Kosten in der Cloud

### 2.2 Die Lösung

S3 (und MinIO) bieten **Pre-Signed URLs**: zeitlich begrenzte, signierte URLs, die einem Client direkt Up- oder Download erlauben — ohne dass der Service den Datenstrom sieht.

**Upload-Flow:**
```
Client                  Document Service           PostgreSQL          S3/MinIO
   │                          │                        │                  │
   │ POST /documents          │                        │                  │
   ├─────────────────────────>│                        │                  │
   │                          │ INSERT document         │                  │
   │                          │ INSERT version (pending)│                  │
   │                          ├───────────────────────>│                  │
   │                          │ Generate Pre-Signed URL                  │
   │                          ├───────────────────────────────────────────>│
   │                          │<───────────────────────────────────────────┤
   │ {document_id, upload_url}│                        │                  │
   │<─────────────────────────┤                        │                  │
   │                          │                        │                  │
   │ PUT upload_url + Bytes   │                        │                  │
   ├──────────────────────────────────────────────────────────────────────>│
   │ 200 OK                   │                        │                  │
   │<──────────────────────────────────────────────────────────────────────┤
   │                          │                        │                  │
   │ POST /docs/{id}/confirm  │                        │                  │
   ├─────────────────────────>│                        │                  │
   │                          │ HEAD object (verify)                     │
   │                          ├───────────────────────────────────────────>│
   │                          │<───────────────────────────────────────────┤
   │                          │ UPDATE version (uploaded)                  │
   │                          ├───────────────────────>│                  │
   │                          │ Outbox event           │                  │
   │ 200 + final document     │                        │                  │
   │<─────────────────────────┤                        │                  │
```

**Download-Flow** ist deutlich kürzer:
```
Client                  Document Service           S3/MinIO
   │                          │                        │
   │ GET /documents/{id}      │                        │
   ├─────────────────────────>│                        │
   │                          │ Generate Pre-Signed URL│
   │                          ├───────────────────────>│
   │                          │<───────────────────────┤
   │ {metadata, download_url} │                        │
   │<─────────────────────────┤                        │
   │ GET download_url         │                        │
   ├──────────────────────────────────────────────────>│
   │ Bytes                    │                        │
   │<──────────────────────────────────────────────────┤
```

### 2.3 Lifetime der URLs

| Operation | Lifetime | Begründung |
|-----------|---------:|------------|
| Upload-URL | 15 min | Genug Zeit auch für große Files; Limit gegen Token-Reuse |
| Download-URL | 5 min | Kurzlebig; Client soll bei Bedarf neue holen |

---

## 3. Use-Cases

### UC-1: Dokument hochladen (Two-Phase)

**Phase 1: Upload-URL anfordern**

**Akteur:** User mit `document:create`  
**Ablauf:**
1. `POST /projects/{projectId}/documents` mit `name`, `content_type`, `size_bytes`
2. Permission-Check beim Project Service
3. Validierung: `content_type` in Allowlist, `size_bytes ≤ MAX_FILE_SIZE`
4. Insert in `documents` (Status `pending`) + Insert in `document_versions` (version 1, Status `pending`)
5. Pre-Signed Upload URL für Storage-Key generieren
6. Outbox-Event `document.upload.initiated`
7. Response: 201 mit `{document_id, version, upload_url, expires_at}`

**Phase 2: Upload bestätigen**

**Ablauf:**
3. Client hat Bytes via PUT zur URL hochgeladen
4. Client schickt `POST /documents/{id}/versions/{version}:confirm`
5. Service ruft `HEAD object` bei S3 — verifiziert Existenz und Größe
6. Bei Mismatch (Größe weicht ab) → 409 + Pending bleibt für Cleanup
7. Update `documents.status = active`, `document_versions.status = uploaded` + `current_version` setzen
8. Outbox-Event `document.uploaded`
9. Response: 200 mit Document-Response

**Fehlerfälle:**
- Upload nie bestätigt → Cleanup-Job entfernt Pending-Versionen nach 1h, S3-Object wird per S3-Lifecycle-Rule entfernt (siehe Abschnitt 7)

### UC-2: Neue Version hochladen

**Akteur:** User mit `document:update`  
**Ablauf:**
1. `POST /documents/{id}/versions` mit `content_type`, `size_bytes`
2. Permission-Check
3. Insert neue Zeile in `document_versions` mit `version_number = current + 1` (Status `pending`)
4. Pre-Signed URL generieren
5. Outbox-Event `document.upload.initiated` (mit `version: N`)
6. Response: 201 mit Upload-URL

Bestätigung wie UC-1.

### UC-3: Dokument-Metadaten abrufen

**Akteur:** User mit `document:read`  
**Ablauf:**
1. `GET /documents/{id}`
2. Permission-Check
3. Response: Metadaten (ohne Download-URL)

### UC-4: Dokument herunterladen (Download-URL)

**Akteur:** User mit `document:read`  
**Ablauf:**
1. `GET /documents/{id}/download` (optional: `?version=N` für historische Version)
2. Permission-Check
3. Pre-Signed Download URL generieren
4. Response: 200 mit `{download_url, expires_at, version}`

### UC-5: Versionsliste

**Akteur:** User mit `document:read`  
**Ablauf:**
1. `GET /documents/{id}/versions`
2. Permission-Check
3. Response: Liste mit `version_number, size_bytes, content_type, uploaded_by, uploaded_at`

### UC-6: Dokument umbenennen

**Akteur:** User mit `document:update`  
**Ablauf:**
1. `PATCH /documents/{id}` mit `name`
2. Permission-Check
3. Update `documents.name`
4. Outbox-Event `document.updated`
5. Response: 200

### UC-7: Dokumente eines Projekts auflisten

**Akteur:** User mit `document:read`  
**Ablauf:**
1. `GET /projects/{projectId}/documents`
2. Permission-Check
3. Cursor-Pagination
4. Response: Liste

### UC-8: Dokument löschen (Soft-Delete)

**Akteur:** User mit `document:delete`  
**Ablauf:**
1. `DELETE /documents/{id}`
2. Permission-Check
3. `documents.deleted_at = NOW()`
4. Outbox-Event `document.deleted`
5. Response: 204

**Note:** Bytes in S3 bleiben zunächst erhalten. Hard-Delete-Cleanup-Job (täglich) entfernt nach 30 Tagen Aufbewahrungsfrist die S3-Objects und DB-Zeilen aller `document_versions`.

### UC-9: Versions-Rollback

**Akteur:** User mit `document:update`  
**Ablauf:**
1. `POST /documents/{id}/versions/{n}:restore`
2. Permission-Check
3. Setze `current_version = n` (alte Version wird wieder aktuell)
4. Outbox-Event `document.version.restored`
5. Response: 200

**Designentscheidung:** Rollback macht keine neue Version-Kopie, sondern setzt nur den Pointer um. Das spart Storage und ist wiederherstellbar.

---

## 4. Datenmodell

### 4.1 Tabellen

```sql
-- Bekannte Projekte (Replikat aus project.created Events)
-- Für Validierung "ist Dokument-Owner Projekt-Mitglied"
CREATE TABLE known_projects (
    id              UUID PRIMARY KEY,
    deleted_at      TIMESTAMPTZ NULL
);

-- Bekannte Benutzer
CREATE TABLE known_users (
    id              UUID PRIMARY KEY,
    deleted_at      TIMESTAMPTZ NULL
);

-- Dokumente (Aggregat-Wurzel)
CREATE TABLE documents (
    id                UUID PRIMARY KEY,
    project_id        UUID NOT NULL,
    name              TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    current_version   INTEGER NULL,                 -- NULL solange erste Version pending
    status            TEXT NOT NULL DEFAULT 'pending', -- pending | active | deleted
    created_by        UUID NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ NULL,
    
    CONSTRAINT documents_name_length CHECK (char_length(name) BETWEEN 1 AND 255),
    CONSTRAINT documents_status CHECK (status IN ('pending', 'active', 'deleted')),
    CONSTRAINT documents_content_type_length CHECK (char_length(content_type) <= 200)
);

CREATE INDEX idx_documents_project_active ON documents (project_id, created_at DESC) 
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_documents_pending ON documents (created_at) 
    WHERE status = 'pending';

-- Versionen (1..n pro Dokument)
CREATE TABLE document_versions (
    id                UUID PRIMARY KEY,
    document_id       UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version_number    INTEGER NOT NULL,
    storage_key       TEXT NOT NULL,                -- S3-Key, z.B. "projects/<proj>/documents/<doc>/v<n>/<filename>"
    content_type      TEXT NOT NULL,
    size_bytes        BIGINT NOT NULL,
    checksum_sha256   TEXT NULL,                    -- nach Confirm gefüllt
    status            TEXT NOT NULL DEFAULT 'pending', -- pending | uploaded | failed
    uploaded_by       UUID NOT NULL,
    uploaded_at       TIMESTAMPTZ NULL,             -- erst beim Confirm gesetzt
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT document_versions_status CHECK (status IN ('pending', 'uploaded', 'failed')),
    CONSTRAINT document_versions_size_positive CHECK (size_bytes > 0),
    CONSTRAINT document_versions_unique UNIQUE (document_id, version_number)
);

CREATE INDEX idx_document_versions_doc ON document_versions (document_id, version_number DESC);
CREATE INDEX idx_document_versions_pending ON document_versions (created_at) 
    WHERE status = 'pending';
CREATE INDEX idx_document_versions_storage_key ON document_versions (storage_key);

-- Pending-Cleanup-Tracking: orphaned uploads
-- Cleanup-Job: alle pending versions, die älter als 1h sind → status = 'failed' setzen
-- S3-Bytes werden über S3-Lifecycle-Policy nach 24h auf "incomplete" gelöscht

-- Outbox
CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

-- Idempotenz für konsumierte Events
CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 4.2 Storage-Key-Schema

```
projects/<project-uuid>/documents/<document-uuid>/v<version>/<sanitized-name>
```

Beispiel:
```
projects/550e8400-e29b-41d4-a716-446655440000/documents/660f9511-.../v3/architecture.pdf
```

**Begründung:**
- Project-Prefix ermöglicht Bulk-Operationen pro Projekt (z. B. Backup, IAM-Policies)
- Version im Pfad — auch ohne DB-Lookup ist erkennbar, welche Version
- Sanitized Name am Ende — Original-Name wird in DB gespeichert, im Pfad kollisionsfrei

### 4.3 Designentscheidungen

**Warum `documents.current_version` nullable?**  
Für Phase 1 von UC-1: Dokument existiert in DB, Bytes noch nicht hochgeladen. `current_version IS NULL` markiert "erstes Upload läuft". Erst nach Confirm wird sie gesetzt.

**Warum `status` zusätzlich zu `deleted_at`?**  
`status = pending` ist ein anderer Zustand als `status = active, deleted_at IS NULL`. Pending-Dokumente werden in keiner Liste angezeigt, sind aber nicht "gelöscht". Drei klar separierte States.

**Warum `checksum_sha256` nullable?**  
Optional. Beim Confirm berechnen wir es nicht selbst (zu teuer), aber S3 liefert ETag, das bei Single-Part-Uploads dem MD5 entspricht. SHA256 wäre via S3 Checksum API (`x-amz-checksum-sha256`) verfügbar, im MVP ausgespart.

**Warum kein eigener `download_count` o.Ä.?**  
Outscope. Wenn nötig, später als Read-Model aus `document.downloaded`-Events.

---

## 5. Domain-Modell

### 5.1 Entitäten

```go
package domain

import (
    "time"
    "github.com/google/uuid"
)

type Document struct {
    ID             uuid.UUID
    ProjectID      uuid.UUID
    Name           string
    ContentType    string
    CurrentVersion *int
    Status         DocStatus
    CreatedBy      uuid.UUID
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
}

type DocumentVersion struct {
    ID             uuid.UUID
    DocumentID     uuid.UUID
    VersionNumber  int
    StorageKey     string
    ContentType    string
    SizeBytes      int64
    ChecksumSHA256 *string
    Status         VersionStatus
    UploadedBy     uuid.UUID
    UploadedAt     *time.Time
    CreatedAt      time.Time
}

type DocStatus string

const (
    DocStatusPending DocStatus = "pending"
    DocStatusActive  DocStatus = "active"
    DocStatusDeleted DocStatus = "deleted"
)

type VersionStatus string

const (
    VersionStatusPending  VersionStatus = "pending"
    VersionStatusUploaded VersionStatus = "uploaded"
    VersionStatusFailed   VersionStatus = "failed"
)

// Result-Typen mit Pre-Signed URLs
type UploadInitiation struct {
    Document   *Document
    Version    *DocumentVersion
    UploadURL  string
    ExpiresAt  time.Time
}

type DownloadInfo struct {
    Document    *Document
    Version     *DocumentVersion
    DownloadURL string
    ExpiresAt   time.Time
}
```

### 5.2 Service-Interface

```go
package domain

type DocumentService interface {
    // Upload
    InitiateUpload(ctx context.Context, requester uuid.UUID, input InitiateUploadInput) (*UploadInitiation, error)
    InitiateNewVersion(ctx context.Context, documentID, requester uuid.UUID, input NewVersionInput) (*UploadInitiation, error)
    ConfirmUpload(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error)
    
    // Read
    GetDocument(ctx context.Context, documentID, requester uuid.UUID) (*Document, error)
    GetDownloadURL(ctx context.Context, documentID, requester uuid.UUID, version *int) (*DownloadInfo, error)
    ListVersions(ctx context.Context, documentID, requester uuid.UUID) ([]*DocumentVersion, error)
    ListDocuments(ctx context.Context, projectID, requester uuid.UUID, page Pagination) ([]*Document, *Cursor, error)
    
    // Modify
    RenameDocument(ctx context.Context, documentID, requester uuid.UUID, newName string) (*Document, error)
    DeleteDocument(ctx context.Context, documentID, requester uuid.UUID) error
    RestoreVersion(ctx context.Context, documentID, requester uuid.UUID, versionNumber int) (*Document, error)
    
    // Internal (Service-zu-Service)
    GetDocumentInfo(ctx context.Context, documentID uuid.UUID) (*DocumentInfo, error)
}

type InitiateUploadInput struct {
    ProjectID   uuid.UUID
    Name        string
    ContentType string
    SizeBytes   int64
}

type NewVersionInput struct {
    ContentType string
    SizeBytes   int64
}

type DocumentInfo struct {
    ID        uuid.UUID
    ProjectID uuid.UUID
    Name      string
    Exists    bool
    Active    bool
}

type Pagination struct {
    Limit  int
    Cursor *string
}

type Cursor struct {
    CreatedAt time.Time
    ID        uuid.UUID
}
```

### 5.3 Domain-Fehler

```go
package domain

var (
    ErrDocumentNotFound       = &Error{Code: "document_not_found"}
    ErrVersionNotFound        = &Error{Code: "version_not_found"}
    ErrDocumentNotActive      = &Error{Code: "document_not_active"}
    ErrVersionNotPending      = &Error{Code: "version_not_pending"}
    ErrVersionNotUploaded     = &Error{Code: "version_not_uploaded"}
    ErrPermissionDenied       = &Error{Code: "permission_denied"}
    ErrFileTooLarge           = &Error{Code: "file_too_large"}
    ErrUnsupportedContentType = &Error{Code: "unsupported_content_type"}
    ErrUploadSizeMismatch     = &Error{Code: "upload_size_mismatch"}
    ErrUploadNotFound         = &Error{Code: "upload_not_found_in_storage"}
    ErrProjectUnknown         = &Error{Code: "project_unknown"}
    ErrValidation             = &Error{Code: "validation_failed"}
)
```

---

## 6. Versionierungs-Strategie

### 6.1 Verhalten

- **Initial-Upload:** version = 1
- **Neue Version:** `MAX(version_number) + 1` für das Dokument
- **`current_version`** zeigt auf die "aktuelle" Version (typisch die neueste, kann nach Rollback auf eine alte zeigen)
- **Versionen sind immutable** — einmal `uploaded`, niemals geändert. Rename des Dokuments ändert nur Metadata, nicht Versionen.

### 6.2 Storage-Implikationen

Jede Version ein eigener S3-Object-Key. Vorteile:
- Truly immutable Versionen
- Trivial parallel zugreifbar
- Rollback durch Pointer-Update, kein Daten-Move
- Bytes der alten Versionen können bei Bedarf in günstigeren Storage-Klassen (S3 Standard-IA, Glacier) lifecycle-managed werden

Nachteil: Storage-Kosten skalieren mit Versionsanzahl. Im MVP keine Begrenzung — Erweiterung wäre konfigurierbares "max retained versions" pro Projekt.

### 6.3 Rollback-Semantik

Beim Restore von Version N wird die Version selbst nicht verändert — nur `documents.current_version = N`. Spätere neue Versionen erhöhen weiter (`N+1`, `N+2`...), nicht "über" der gerollbackten Version. Das macht die History lückenlos und nachvollziehbar.

---

## 7. Two-Phase Upload und Cleanup

### 7.1 Das Problem

Pre-Signed URLs erlauben dem Client, beliebig lange (bis zur URL-Expiry) hochzuladen. Drei Probleme:

1. **Client lädt nie hoch** → Pending-Zeile bleibt in DB, kein S3-Object
2. **Client lädt teilweise hoch** → Pending-Zeile bleibt, S3-Object existiert mit falscher Größe
3. **Client bestätigt nie** → Pending-Zeile bleibt, S3-Object möglicherweise OK

### 7.2 Cleanup-Strategie

**Pending-Versionen, älter als 1 Stunde:**
- DB: `UPDATE document_versions SET status = 'failed' WHERE status = 'pending' AND created_at < NOW() - INTERVAL '1 hour'`
- Wenn Dokument noch keine erste `uploaded` Version hat (`documents.current_version IS NULL`) → `documents.status = 'deleted'` setzen, dann Lifecycle-Cleanup

**S3-Bytes:**
- Storage-Bucket bekommt **S3 Lifecycle Rule:** "incomplete multipart uploads → Abbruch nach 24h"
- Für komplette aber ungenutzte Objekte: Cleanup-Job listet `failed`-Versionen und löscht zugehörige Storage-Keys

**Hard-Delete-Cleanup für gelöschte Dokumente:**
- Täglicher Job: alle `documents.deleted_at < NOW() - INTERVAL '30 days'` → DB-Cascade-Delete + S3-Object-Removal pro Version

### 7.3 Confirm-Validierung

Bei `POST /documents/{id}/versions/{v}:confirm`:

```go
func (s *service) ConfirmUpload(ctx context.Context, docID uuid.UUID, requester uuid.UUID, versionNumber int) (*Document, error) {
    // 1. Permission-Check
    // 2. Version laden, sicherstellen status=pending
    version, err := s.repo.GetVersion(ctx, docID, versionNumber)
    if version.Status != VersionStatusPending { return nil, ErrVersionNotPending }
    
    // 3. HEAD object bei S3
    info, err := s.storage.HeadObject(ctx, version.StorageKey)
    if err != nil {
        return nil, ErrUploadNotFound  // Bytes existieren nicht
    }
    
    // 4. Größenprüfung
    if info.SizeBytes != version.SizeBytes {
        return nil, ErrUploadSizeMismatch
    }
    
    // 5. Update in Transaktion: version uploaded, document active, current_version
    // 6. Outbox: document.uploaded
    // ...
}
```

### 7.4 Cleanup-Worker

Eigenständiger Goroutine im Service oder separater Container:

```go
func (w *cleanupWorker) Run(ctx context.Context) {
    ticker := time.NewTicker(15 * time.Minute)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.cleanupPendingVersions(ctx)
            w.cleanupOrphanedDocuments(ctx)
        }
    }
}
```

---

## 8. HTTP-API (OpenAPI)

Vollständige Spec in `docs/api/document.openapi.yaml`.

### 8.1 OpenAPI 3.1 (Auszug)

```yaml
openapi: 3.1.0
info:
  title: TeamBoard Document Service
  version: 1.0.0
servers:
  - url: http://localhost:8004/api/v1

paths:
  /projects/{projectId}/documents:
    parameters:
      - $ref: '#/components/parameters/projectId'
    get:
      summary: List documents in a project
      operationId: listDocuments
      tags: [documents]
      security: [{bearerAuth: []}]
      parameters:
        - in: query
          name: limit
          schema: { type: integer, default: 50, maximum: 200 }
        - in: query
          name: cursor
          schema: { type: string }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DocumentList' }

    post:
      summary: Initiate document upload (Phase 1)
      operationId: initiateUpload
      tags: [documents]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/InitiateUploadRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/UploadInitiationResponse' }

  /documents/{documentId}:
    parameters:
      - $ref: '#/components/parameters/documentId'
    get:
      summary: Get document metadata
      operationId: getDocument
      tags: [documents]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DocumentResponse' }

    patch:
      summary: Rename document
      operationId: renameDocument
      tags: [documents]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema: { $ref: '#/components/schemas/RenameDocumentRequest' }
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DocumentResponse' }

    delete:
      summary: Soft-delete document
      operationId: deleteDocument
      tags: [documents]
      security: [{bearerAuth: []}]
      responses:
        '204': { description: Deleted }

  /documents/{documentId}/download:
    parameters:
      - $ref: '#/components/parameters/documentId'
      - in: query
        name: version
        schema: { type: integer, minimum: 1 }
        description: Specific version (defaults to current_version)
    get:
      summary: Get pre-signed download URL
      operationId: getDownloadURL
      tags: [documents]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DownloadResponse' }

  /documents/{documentId}/versions:
    parameters:
      - $ref: '#/components/parameters/documentId'
    get:
      summary: List versions
      operationId: listVersions
      tags: [versions]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/VersionList' }

    post:
      summary: Initiate new version upload (Phase 1)
      operationId: initiateNewVersion
      tags: [versions]
      security: [{bearerAuth: []}]
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/NewVersionRequest' }
      responses:
        '201':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/UploadInitiationResponse' }

  /documents/{documentId}/versions/{version}:confirm:
    parameters:
      - $ref: '#/components/parameters/documentId'
      - in: path
        name: version
        required: true
        schema: { type: integer, minimum: 1 }
    post:
      summary: Confirm upload completion (Phase 2)
      operationId: confirmUpload
      tags: [versions]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DocumentResponse' }
        '404':
          description: Upload not found in storage
        '409':
          description: Size mismatch or wrong version status

  /documents/{documentId}/versions/{version}:restore:
    parameters:
      - $ref: '#/components/parameters/documentId'
      - in: path
        name: version
        required: true
        schema: { type: integer, minimum: 1 }
    post:
      summary: Set version as current
      operationId: restoreVersion
      tags: [versions]
      security: [{bearerAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DocumentResponse' }

  /health/live:
    get:
      operationId: getLiveness
      responses:
        '200': { description: Live }

  /health/ready:
    get:
      operationId: getReadiness
      responses:
        '200': { description: Ready }
        '503': { description: Not ready }

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT

  parameters:
    projectId:
      in: path
      name: projectId
      required: true
      schema: { type: string, format: uuid }
    documentId:
      in: path
      name: documentId
      required: true
      schema: { type: string, format: uuid }

  schemas:
    InitiateUploadRequest:
      type: object
      required: [name, content_type, size_bytes]
      properties:
        name: { type: string, minLength: 1, maxLength: 255 }
        content_type: { type: string, maxLength: 200 }
        size_bytes: { type: integer, format: int64, minimum: 1 }

    NewVersionRequest:
      type: object
      required: [content_type, size_bytes]
      properties:
        content_type: { type: string, maxLength: 200 }
        size_bytes: { type: integer, format: int64, minimum: 1 }

    RenameDocumentRequest:
      type: object
      required: [name]
      properties:
        name: { type: string, minLength: 1, maxLength: 255 }

    UploadInitiationResponse:
      type: object
      required: [data]
      properties:
        data:
          type: object
          required: [document_id, version, upload_url, expires_at]
          properties:
            document_id: { type: string, format: uuid }
            version: { type: integer }
            upload_url: { type: string, format: uri }
            expires_at: { type: string, format: date-time }
            method:
              type: string
              enum: [PUT]
              default: PUT

    DownloadResponse:
      type: object
      required: [data]
      properties:
        data:
          type: object
          required: [document_id, version, download_url, expires_at]
          properties:
            document_id: { type: string, format: uuid }
            version: { type: integer }
            download_url: { type: string, format: uri }
            expires_at: { type: string, format: date-time }
            content_type: { type: string }
            size_bytes: { type: integer, format: int64 }

    Document:
      type: object
      required: [id, project_id, name, content_type, status, created_by, created_at]
      properties:
        id: { type: string, format: uuid }
        project_id: { type: string, format: uuid }
        name: { type: string }
        content_type: { type: string }
        current_version: { type: integer, nullable: true }
        size_bytes: { type: integer, format: int64, description: "Of current version" }
        status: { type: string, enum: [pending, active, deleted] }
        created_by: { type: string, format: uuid }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }

    DocumentResponse:
      type: object
      required: [data]
      properties:
        data: { $ref: '#/components/schemas/Document' }

    DocumentList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Document' }
        pagination: { $ref: '#/components/schemas/Pagination' }

    Version:
      type: object
      required: [version_number, size_bytes, content_type, status, uploaded_by, created_at]
      properties:
        version_number: { type: integer }
        size_bytes: { type: integer, format: int64 }
        content_type: { type: string }
        checksum_sha256: { type: string, nullable: true }
        status: { type: string, enum: [pending, uploaded, failed] }
        uploaded_by: { type: string, format: uuid }
        uploaded_at: { type: string, format: date-time, nullable: true }
        created_at: { type: string, format: date-time }
        is_current: { type: boolean }

    VersionList:
      type: object
      required: [data]
      properties:
        data:
          type: array
          items: { $ref: '#/components/schemas/Version' }

    Pagination:
      type: object
      properties:
        next_cursor: { type: string, nullable: true }
        limit: { type: integer }
```

### 8.2 Endpoint-Übersicht

| Methode | Pfad | Permission | Beschreibung |
|---------|------|------------|--------------|
| GET | `/projects/{projectId}/documents` | `document:read` | Dokumentenliste |
| POST | `/projects/{projectId}/documents` | `document:create` | Upload Phase 1 |
| GET | `/documents/{documentId}` | `document:read` | Metadaten |
| PATCH | `/documents/{documentId}` | `document:update` | Umbenennen |
| DELETE | `/documents/{documentId}` | `document:delete` | Soft-Delete |
| GET | `/documents/{documentId}/download` | `document:read` | Download-URL |
| GET | `/documents/{documentId}/versions` | `document:read` | Versionsliste |
| POST | `/documents/{documentId}/versions` | `document:update` | Neue Version Phase 1 |
| POST | `/documents/{documentId}/versions/{v}:confirm` | `document:update` | Phase 2 |
| POST | `/documents/{documentId}/versions/{v}:restore` | `document:update` | Rollback |

---

## 9. Interne API für Service-zu-Service

### 9.1 Endpoint

```yaml
paths:
  /internal/documents/{documentId}:
    parameters:
      - in: path
        name: documentId
        required: true
        schema: { type: string, format: uuid }
    get:
      summary: Get document info for cross-service validation
      operationId: getDocumentInfo
      tags: [internal]
      security: [{serviceAuth: []}]
      responses:
        '200':
          content:
            application/json:
              schema:
                type: object
                required: [exists, active]
                properties:
                  document_id: { type: string, format: uuid }
                  project_id: { type: string, format: uuid, nullable: true }
                  name: { type: string, nullable: true }
                  exists: { type: boolean }
                  active: { type: boolean, description: "exists AND status=active AND deleted_at IS NULL" }
        '404':
          description: Document does not exist
```

### 9.2 Verwendung

Task Service ruft diesen Endpoint vor `AddAttachment`-Operation auf:

```
GET /internal/documents/{documentId}
→ {project_id, exists: true, active: true}
```

Task Service prüft dann selbst, ob `project_id` mit dem Task-Projekt übereinstimmt.

### 9.3 Performance

p99 < 30 ms. Kein Caching im MVP — wird selten genug aufgerufen, weil Task-Attachments seltener sind als Task-Reads.

---

## 10. Object-Storage-Integration

### 10.1 Abstraktion

```go
package storage

type ObjectStorage interface {
    PutPresignedURL(ctx context.Context, key string, contentType string, sizeBytes int64, ttl time.Duration) (string, time.Time, error)
    GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
    HeadObject(ctx context.Context, key string) (*ObjectInfo, error)
    DeleteObject(ctx context.Context, key string) error
    DeleteObjects(ctx context.Context, keys []string) error  // Batch
}

type ObjectInfo struct {
    Key         string
    SizeBytes   int64
    ContentType string
    ETag        string
    LastModified time.Time
}
```

### 10.2 S3/MinIO-Implementation

Library: `github.com/aws/aws-sdk-go-v2/service/s3` (funktioniert auch gegen MinIO mit `EndpointResolver`).

```go
package storage

import (
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Storage struct {
    client     *s3.Client
    bucket     string
}

func (s *s3Storage) PutPresignedURL(ctx context.Context, key string, ct string, size int64, ttl time.Duration) (string, time.Time, error) {
    presigner := s3.NewPresignClient(s.client)
    req, err := presigner.PresignPutObject(ctx, &s3.PutObjectInput{
        Bucket:        aws.String(s.bucket),
        Key:           aws.String(key),
        ContentType:   aws.String(ct),
        ContentLength: aws.Int64(size),
    }, s3.WithPresignExpires(ttl))
    if err != nil { return "", time.Time{}, err }
    return req.URL, time.Now().Add(ttl), nil
}
```

### 10.3 MinIO-Setup für lokale Entwicklung

`docker-compose.yml` (Auszug):

```yaml
services:
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: teamboard
      MINIO_ROOT_PASSWORD: teamboard-secret
    ports:
      - "9000:9000"     # S3-API
      - "9001:9001"     # Console
    volumes:
      - minio-data:/data
  
  minio-init:
    image: minio/mc:latest
    depends_on: [minio]
    entrypoint: >
      /bin/sh -c "
        mc alias set local http://minio:9000 teamboard teamboard-secret;
        mc mb --ignore-existing local/teamboard-documents;
        mc anonymous set none local/teamboard-documents;
      "
```

### 10.4 Bucket-Lifecycle-Policy

Für inkomplette Multipart Uploads automatisch nach 24h:

```json
{
  "Rules": [
    {
      "ID": "abort-incomplete-multipart-uploads",
      "Status": "Enabled",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 1
      }
    }
  ]
}
```

Diese Lifecycle-Rule wird im CDK / Terraform mit Bucket erstellt. In MinIO-Dev über `mc ilm` oder UI.

### 10.5 CORS am Bucket

Damit Browser die Pre-Signed URL direkt nutzen können:

```json
{
  "CORSRules": [
    {
      "AllowedMethods": ["PUT", "GET", "HEAD"],
      "AllowedOrigins": ["http://localhost:3000", "https://teamboard.example"],
      "AllowedHeaders": ["*"],
      "ExposeHeaders": ["ETag"],
      "MaxAgeSeconds": 3000
    }
  ]
}
```

---

## 11. Events

### 11.1 Publizierte Events

| Event-Type | Trigger | Payload |
|------------|---------|---------|
| `document.upload.initiated` | UC-1 Phase 1 / UC-2 | `document_id, project_id, version, name, content_type, size_bytes, initiated_by` |
| `document.uploaded` | UC-1 Phase 2 / UC-2 Confirm | `document_id, project_id, version, content_type, size_bytes, uploaded_by` |
| `document.updated` | UC-6 Rename | `document_id, project_id, changes` |
| `document.deleted` | UC-8 | `document_id, project_id, deleted_by` |
| `document.version.restored` | UC-9 | `document_id, project_id, restored_version, previous_version` |
| `document.upload.failed` | Cleanup-Worker | `document_id, version, reason` |

### 11.2 Konsumierte Events

| Event-Type | Source | Reaktion |
|------------|--------|----------|
| `user.registered` | auth | INSERT in `known_users` |
| `user.deleted` | auth | UPDATE `known_users.deleted_at` (keine Doc-Daten ändern — Audit erhalten) |
| `project.created` | project | INSERT in `known_projects` |
| `project.deleted` | project | UPDATE `known_projects.deleted_at`; alle Dokumente des Projekts soft-deleten; pro Doc Outbox-Event `document.deleted` |

### 11.3 Idempotenz

`processed_events`-Check vor Verarbeitung.

### 11.4 Wichtig: keine Bytes-Events

Events tragen niemals Inhalts-Daten — nur Metadaten. Wer Bytes braucht, holt Download-URL über die API. Hält den Event-Bus schlank.

---

## 12. Konfiguration

### 12.1 Environment-Variablen

```bash
SERVICE_NAME=document-service
SERVICE_PORT=8004
LOG_LEVEL=info

# Database
DB_URL=postgres://document:document@postgres:5432/document_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
RABBITMQ_EXCHANGE=teamboard.events
RABBITMQ_CONSUMER_QUEUE=document-service-queue

# Object Storage
STORAGE_PROVIDER=s3                          # s3 | minio (gleicher Code, andere Endpoints)
STORAGE_BUCKET=teamboard-documents
STORAGE_REGION=us-east-1
STORAGE_ENDPOINT=http://minio:9000           # nur für MinIO; bei S3 leer
STORAGE_ACCESS_KEY=teamboard
STORAGE_SECRET_KEY=teamboard-secret
STORAGE_USE_PATH_STYLE=true                  # true für MinIO, false für S3
STORAGE_PUBLIC_ENDPOINT=http://localhost:9000 # was Clients sehen (vs. interner Service-Name)

# Upload Limits
MAX_FILE_SIZE_BYTES=104857600                # 100 MB
ALLOWED_CONTENT_TYPES=application/pdf,image/png,image/jpeg,image/gif,application/zip,text/plain,text/markdown,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,application/vnd.openxmlformats-officedocument.presentationml.presentation
UPLOAD_URL_TTL=15m
DOWNLOAD_URL_TTL=5m

# Cleanup
CLEANUP_INTERVAL=15m
PENDING_VERSION_TIMEOUT=1h
DELETED_DOCUMENT_RETENTION=720h              # 30 Tage

# Auth (JWKS)
JWT_JWKS_URL=http://auth:8001/.well-known/jwks.json
JWT_ISSUER=https://auth.teamboard.local
JWT_AUDIENCE=teamboard-api

# Project Service
PROJECT_SERVICE_URL=http://project:8002
PROJECT_SERVICE_TIMEOUT=200ms
PERMISSION_CACHE_TTL=30s

# Service-Token
SERVICE_TOKEN_SECRET=<from-secrets-manager>

# Observability
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=document-service
```

### 12.2 Config-Struct

```go
package config

type Config struct {
    ServiceName string `env:"SERVICE_NAME" envDefault:"document-service"`
    Port        int    `env:"SERVICE_PORT" envDefault:"8004"`
    LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

    DB struct {
        URL          string `env:"DB_URL,required"`
        MaxOpenConns int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
        MaxIdleConns int    `env:"DB_MAX_IDLE_CONNS" envDefault:"5"`
    }

    RabbitMQ struct {
        URL           string `env:"RABBITMQ_URL,required"`
        Exchange      string `env:"RABBITMQ_EXCHANGE" envDefault:"teamboard.events"`
        ConsumerQueue string `env:"RABBITMQ_CONSUMER_QUEUE" envDefault:"document-service-queue"`
    }

    Storage struct {
        Provider        string `env:"STORAGE_PROVIDER" envDefault:"s3"`
        Bucket          string `env:"STORAGE_BUCKET,required"`
        Region          string `env:"STORAGE_REGION" envDefault:"us-east-1"`
        Endpoint        string `env:"STORAGE_ENDPOINT"`
        AccessKey       string `env:"STORAGE_ACCESS_KEY,required"`
        SecretKey       string `env:"STORAGE_SECRET_KEY,required"`
        UsePathStyle    bool   `env:"STORAGE_USE_PATH_STYLE" envDefault:"false"`
        PublicEndpoint  string `env:"STORAGE_PUBLIC_ENDPOINT"`
    }

    Upload struct {
        MaxFileSizeBytes      int64         `env:"MAX_FILE_SIZE_BYTES" envDefault:"104857600"`
        AllowedContentTypes   []string      `env:"ALLOWED_CONTENT_TYPES" envSeparator:","`
        UploadURLTTL          time.Duration `env:"UPLOAD_URL_TTL" envDefault:"15m"`
        DownloadURLTTL        time.Duration `env:"DOWNLOAD_URL_TTL" envDefault:"5m"`
    }

    Cleanup struct {
        Interval                 time.Duration `env:"CLEANUP_INTERVAL" envDefault:"15m"`
        PendingVersionTimeout    time.Duration `env:"PENDING_VERSION_TIMEOUT" envDefault:"1h"`
        DeletedDocumentRetention time.Duration `env:"DELETED_DOCUMENT_RETENTION" envDefault:"720h"`
    }

    JWT struct {
        JWKSURL  string `env:"JWT_JWKS_URL,required"`
        Issuer   string `env:"JWT_ISSUER,required"`
        Audience string `env:"JWT_AUDIENCE,required"`
    }

    ProjectService struct {
        URL                string        `env:"PROJECT_SERVICE_URL,required"`
        Timeout            time.Duration `env:"PROJECT_SERVICE_TIMEOUT" envDefault:"200ms"`
        PermissionCacheTTL time.Duration `env:"PERMISSION_CACHE_TTL" envDefault:"30s"`
    }

    Security struct {
        ServiceTokenSecret string `env:"SERVICE_TOKEN_SECRET,required"`
    }

    Observability struct {
        OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
    }
}
```

---

## 13. Verzeichnisstruktur

```
services/document/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   │   ├── router.go
│   │   ├── middleware.go
│   │   ├── handlers_documents.go
│   │   ├── handlers_versions.go
│   │   ├── handlers_internal.go         # /internal/* — Document-Info
│   │   ├── handlers_health.go
│   │   ├── dto.go
│   │   └── generated.go
│   ├── domain/
│   │   ├── document.go
│   │   ├── version.go
│   │   ├── content_types.go             # Allowlist-Validation
│   │   ├── storage_key.go               # Key-Generierung
│   │   ├── service.go                   # Interface
│   │   ├── service_upload.go            # Initiate/Confirm
│   │   ├── service_read.go              # Get/List/Download
│   │   ├── service_modify.go            # Rename/Delete/Restore
│   │   └── errors.go
│   ├── repository/
│   │   ├── db/                           # sqlc-generiert
│   │   ├── repository.go
│   │   └── postgres.go
│   ├── storage/
│   │   ├── storage.go                   # Interface
│   │   ├── s3.go                        # S3/MinIO-Impl
│   │   └── inmemory.go                  # Test-Stub
│   ├── projectclient/
│   │   ├── client.go
│   │   ├── http_client.go
│   │   └── permission_cache.go
│   ├── cleanup/
│   │   ├── worker.go                    # Pending- und Hard-Delete-Cleanup
│   │   └── worker_test.go
│   ├── events/
│   │   ├── consumer.go                  # shared-Envelope-Handler (project.* / user.*)
│   │   └── util.go                      # (Publishing: shared outbox.Worker, verdrahtet in main.go)
│   └── config/
│       └── config.go
├── migrations/
│   ├── 0001_init.up.sql
│   └── 0001_init.down.sql
├── queries/
│   ├── documents.sql
│   ├── versions.sql
│   ├── known_projects.sql
│   ├── known_users.sql
│   ├── outbox.sql
│   └── processed_events.sql
├── sqlc.yaml
├── Dockerfile
├── go.mod
├── go.sum
├── .air.toml
├── Makefile
└── README.md
```

---

## 14. sqlc-Queries

### 14.1 `queries/documents.sql`

```sql
-- name: CreateDocument :one
INSERT INTO documents (id, project_id, name, content_type, status, created_by)
VALUES ($1, $2, $3, $4, 'pending', $5)
RETURNING *;

-- name: GetDocument :one
SELECT * FROM documents
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetDocumentInternal :one
SELECT * FROM documents
WHERE id = $1;

-- name: ListDocumentsByProject :many
SELECT * FROM documents
WHERE project_id = $1
  AND status = 'active'
  AND deleted_at IS NULL
  AND ($2::TIMESTAMPTZ IS NULL OR created_at < $2)
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: UpdateDocumentName :one
UPDATE documents
SET name = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: ActivateDocument :one
UPDATE documents
SET status = 'active', current_version = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCurrentVersion :one
UPDATE documents
SET current_version = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteDocument :exec
UPDATE documents
SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteDocumentsByProject :many
UPDATE documents
SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
WHERE project_id = $1 AND deleted_at IS NULL
RETURNING id, project_id;

-- name: ListPendingDocumentsOlderThan :many
SELECT * FROM documents
WHERE status = 'pending' AND created_at < $1;

-- name: ListSoftDeletedOlderThan :many
SELECT * FROM documents
WHERE deleted_at IS NOT NULL AND deleted_at < $1
LIMIT $2;

-- name: HardDeleteDocument :exec
DELETE FROM documents WHERE id = $1;
```

### 14.2 `queries/versions.sql`

```sql
-- name: CreateVersion :one
INSERT INTO document_versions (
    id, document_id, version_number, storage_key, content_type, size_bytes, status, uploaded_by
) VALUES (
    $1, $2, $3, $4, $5, $6, 'pending', $7
)
RETURNING *;

-- name: GetVersion :one
SELECT * FROM document_versions
WHERE document_id = $1 AND version_number = $2;

-- name: GetVersionByID :one
SELECT * FROM document_versions WHERE id = $1;

-- name: GetCurrentVersion :one
SELECT v.* FROM document_versions v
INNER JOIN documents d ON d.id = v.document_id
WHERE d.id = $1 AND v.version_number = d.current_version;

-- name: ListVersionsByDocument :many
SELECT * FROM document_versions
WHERE document_id = $1
ORDER BY version_number DESC;

-- name: GetMaxVersionNumber :one
SELECT COALESCE(MAX(version_number), 0)::INTEGER FROM document_versions
WHERE document_id = $1;

-- name: MarkVersionUploaded :one
UPDATE document_versions
SET status = 'uploaded', uploaded_at = NOW(), checksum_sha256 = $2
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: MarkVersionFailed :exec
UPDATE document_versions
SET status = 'failed'
WHERE id = $1 AND status = 'pending';

-- name: ListPendingVersionsOlderThan :many
SELECT * FROM document_versions
WHERE status = 'pending' AND created_at < $1;

-- name: ListFailedVersions :many
SELECT * FROM document_versions
WHERE status = 'failed'
LIMIT $1;

-- name: DeleteVersion :exec
DELETE FROM document_versions WHERE id = $1;
```

### 14.3 `queries/known_projects.sql`

```sql
-- name: UpsertKnownProject :exec
INSERT INTO known_projects (id) VALUES ($1)
ON CONFLICT (id) DO NOTHING;

-- name: KnownProjectExists :one
SELECT EXISTS(SELECT 1 FROM known_projects WHERE id = $1 AND deleted_at IS NULL);

-- name: MarkKnownProjectDeleted :exec
UPDATE known_projects SET deleted_at = NOW() WHERE id = $1;
```

### 14.4 `queries/known_users.sql`

```sql
-- name: UpsertKnownUser :exec
INSERT INTO known_users (id) VALUES ($1)
ON CONFLICT (id) DO NOTHING;

-- name: MarkKnownUserDeleted :exec
UPDATE known_users SET deleted_at = NOW() WHERE id = $1;
```

### 14.5 `queries/outbox.sql`

```sql
-- name: InsertOutboxEvent :exec
INSERT INTO outbox (id, aggregate_id, event_type, payload)
VALUES ($1, $2, $3, $4);

-- name: GetUnpublishedEvents :many
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY occurred_at ASC, id ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkEventPublished :exec
UPDATE outbox SET published_at = NOW() WHERE id = $1;
```

### 14.6 `queries/processed_events.sql`

```sql
-- name: WasEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1);

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1)
ON CONFLICT DO NOTHING;
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

### 15.1 Unit-Tests

```go
func TestStorageKey(t *testing.T) {
    key := buildStorageKey(projectID, docID, 3, "Architecture Doc.pdf")
    require.Equal(t,
        "projects/550e8400-.../documents/660f9511-.../v3/architecture-doc.pdf",
        key)
}

func TestContentTypeAllowlist(t *testing.T) {
    cfg := []string{"application/pdf", "image/png"}
    require.NoError(t, validateContentType("application/pdf", cfg))
    require.Error(t, validateContentType("application/x-malware", cfg))
}

func TestVersionNumberAssignment(t *testing.T) {
    // Mock-Repo: existing versions [1, 2, 3]
    // → next = 4
}
```

### 15.2 Integration-Tests

```go
func TestUploadConfirmCycle(t *testing.T) {
    pg := startPostgres(t)
    minio := startMinIO(t)
    svc := buildService(pg, minio)
    
    t.Run("happy path: initiate -> upload -> confirm", func(t *testing.T) {
        init, err := svc.InitiateUpload(ctx, userID, InitiateUploadInput{
            ProjectID: projID, Name: "test.pdf", ContentType: "application/pdf", SizeBytes: 1024,
        })
        require.NoError(t, err)
        
        // Simulate client uploading via pre-signed URL
        uploadBytes(t, init.UploadURL, make([]byte, 1024))
        
        doc, err := svc.ConfirmUpload(ctx, init.Document.ID, userID, init.Version.VersionNumber)
        require.NoError(t, err)
        require.Equal(t, DocStatusActive, doc.Status)
        require.Equal(t, 1, *doc.CurrentVersion)
    })
    
    t.Run("size mismatch on confirm", func(t *testing.T) {
        // Initiate with size_bytes=1024
        // Upload only 500 bytes
        // Confirm → ErrUploadSizeMismatch
    })
    
    t.Run("confirm without bytes uploaded", func(t *testing.T) {
        // Initiate
        // Skip upload
        // Confirm → ErrUploadNotFound
    })
}

func TestCleanupWorker(t *testing.T) {
    // Setup: pending version aged 2 hours
    // Run worker
    // Expect: status = failed, document soft-deleted (because no current_version)
}

func TestProjectDeletedConsumer(t *testing.T) {
    // Setup: project with 5 documents
    // Trigger project.deleted event
    // Expect: 5 documents soft-deleted, 5 document.deleted events in outbox
}
```

### 15.3 End-to-End-Tests

Postman-Collection mit mehrstufigen Requests (Postman unterstützt Variable-Capturing zwischen Requests):

1. **Upload-Lifecycle:** initiate → PUT to upload_url → confirm → download → list-versions
2. **New Version:** existing doc → initiate version 2 → upload → confirm → check current_version=2
3. **Restore:** create v1, v2, v3 → restore v1 → check current_version=1
4. **Permission:** Viewer initiate-upload → 403
5. **Size-Mismatch:** initiate 1MB → upload 500KB → confirm → 409

### 15.4 Load Test

```javascript
// tests/k6/document_initiate.js
import http from 'k6/http';
export const options = {
    vus: 20, duration: '60s',
    thresholds: { http_req_duration: ['p(99)<300'] },
};
export default function () {
    http.post(`${BASE_URL}/api/v1/projects/${PID}/documents`, JSON.stringify({
        name: `file-${__VU}-${__ITER}.pdf`,
        content_type: 'application/pdf',
        size_bytes: 100000,
    }), {
        headers: { Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json' },
    });
}
```

---

## 16. Sicherheit

### 16.1 Pre-Signed-URL-Sicherheit

- **Kurze Lifetime:** 15min Upload, 5min Download
- **Method-bound:** Upload-URL akzeptiert nur PUT, Download nur GET
- **Content-Length-bound:** Upload-URL für Single-Part hat `Content-Length` als Signatur-Bestandteil — Client kann nicht mehr hochladen als angegeben
- **Content-Type-bound (optional):** Verhindert MIME-Smuggling
- **Bucket nicht öffentlich:** Default Block all public access; Zugriff nur via signierte URLs

### 16.2 Content-Type-Allowlist

Nicht jede Datei darf hochgeladen werden. Allowlist statt Denylist. Aufgenommen sind:
- Office-Formate (docx, xlsx, pptx)
- PDF
- Bilder (png, jpeg, gif)
- Text und Markdown
- ZIP

**NICHT erlaubt:** ausführbare Formate (`application/x-msdownload`, `application/x-executable`), Skripte ohne klaren Use-Case.

### 16.3 Filename-Sanitization

Original-Name in DB OK, in Storage-Key sanitized:

```go
func sanitizeFilename(name string) string {
    // Lowercase, ersetze nicht-alphanumerische durch '-', max 100 Zeichen
    // ".pdf" Endung erhalten
    re := regexp.MustCompile(`[^a-z0-9.\-]+`)
    base := strings.ToLower(name)
    return re.ReplaceAllString(base, "-")
}
```

Verhindert Path-Traversal (`../`) und exotische Zeichen, die in S3 problematisch wären.

### 16.4 Authorization

- **Lese-URL nur an Mitglieder:** Permission-Check auf jeden `/download`-Call
- **Pre-Signed-URL ist nicht an User gebunden:** Wer die URL hat, kann zugreifen — ist explizit so gewünscht (z. B. zum Teilen mit kurzlebigen Links). Daher Lifetime niedrig.
- **Kein "shareable link"-Feature im MVP:** Nur authentifizierte User bekommen URLs. Sharing-Feature später als separate Funktionalität mit Audit.

### 16.5 Größenlimit

`MAX_FILE_SIZE_BYTES = 100 MB` Default. Größere Files würden Multipart Upload erfordern — Erweiterung für später.

---

## 17. Implementierungs-Hinweise für Coding-Agents

### 17.1 Implementierungsreihenfolge

1. **Migrations + sqlc-Setup** — Tabellen, `make generate` läuft
2. **Storage-Abstraktion** (`internal/storage/`) — S3-Impl gegen MinIO testen, In-Memory-Stub für Tests
3. **Domain-Modell ohne Service** — Entitäten, Storage-Key-Builder, Content-Type-Allowlist mit Unit-Tests
4. **Repository-Layer** mit Testcontainers
5. **Project-Client** für Permission-Checks
6. **Service-Layer Upload-Pfad** (Initiate + Confirm) — die zentralen Use-Cases
7. **Service-Layer Read-Pfad** (Get / Download / List)
8. **Service-Layer Modify-Pfad** (Rename / Delete / Restore)
9. **Cleanup-Worker** mit Tests
10. **Event-Consumer** für `user.*`, `project.*`
11. **HTTP-Handler** öffentlich + intern
12. **Outbox-Publisher**
13. **Wiring in `main.go`**
14. **End-to-End-Tests im Compose-Stack**

### 17.2 Verbindliche Konventionen

- **Service liest niemals Bytes.** Wenn Code anfängt `io.Copy` mit Document-Bytes zu machen, Designfehler.
- **Permission-Check vor jeder schreibenden Operation.** Ohne Ausnahme.
- **`HEAD object` vor Confirm**, niemals "vertraue dem Client" bei Größe.
- **Outbox + DB-Mutation in einer Transaktion.**
- **Pre-Signed URLs niemals loggen** (sie sind kurzlebige Credentials).
- **Storage-Keys in Logs maskieren** auf `projects/{proj}/documents/{doc}/v{n}/...`.
- **Storage-Calls außerhalb der DB-Transaktion** — keine Locks halten während externer I/O.

### 17.3 Typische Stolperfallen

- **MinIO vs S3 Endpoint-Resolver:** MinIO braucht `UsePathStyle: true` und `EndpointResolver`. Code muss beides unterstützen, abhängig von Config.
- **Public vs. Internal Endpoint:** Service ruft S3 über internen Docker-Hostnamen `http://minio:9000`. Aber die Pre-Signed URL muss den Hostnamen enthalten, den der Browser erreicht — also `http://localhost:9000`. Zwei separate Konfig-Werte (`STORAGE_ENDPOINT` für Service, `STORAGE_PUBLIC_ENDPOINT` für URL-Generierung).
- **CORS:** Browser werfen ohne CORS-Header keinen direkten PUT zu MinIO durch. Setup-Script muss CORS auf Bucket setzen.
- **Confirm-Idempotenz:** Wenn Client zweimal Confirm aufruft, zweiter Call findet `version.status = uploaded` → 200 mit aktuellem Stand zurück, kein Fehler. Ist eindeutig idempotent.
- **Upload-URL-Reuse:** URL ist wie ein Token. Wenn Client sie zweimal nutzt, ist das technisch erlaubt, überschreibt aber die Bytes. Kein Fehler beim Service, aber unsauber. Im MVP nicht aktiv verhindert.
- **Projekt-Replikat-Race:** `project.created`-Event noch nicht konsumiert, aber User versucht Dokument hochzuladen → `ErrProjectUnknown`. Project-Service fragt aber synchron Permissions an, wodurch dort meist eine 404 zurückkommt — also doppelt abgesichert.
- **Size-Validation:** S3 ETag bei Single-Part-Upload entspricht MD5 — kann zu Vergleich genutzt werden, aber bei Multipart-Uploads ist das nicht mehr MD5. Im MVP nur Größenvergleich, kein Hash.
- **Soft-Delete-vs-Hard-Delete:** Bei Soft-Delete bleiben S3-Bytes. Bei Hard-Delete (nach 30 Tagen) müssen sowohl DB-Zeilen als auch ALLE Versionen-Storage-Keys entfernt werden. Cleanup-Job muss sorgfältig sein.

### 17.4 Make-Targets

```makefile
.PHONY: build test test-unit test-integration migrate generate run docker-build init-bucket

build:
	go build -o bin/document ./cmd/server

test: test-unit test-integration

test-unit:
	go test -short -race -cover ./internal/domain/... ./internal/storage/...

test-integration:
	go test -race ./internal/repository/... ./internal/events/... ./internal/cleanup/...

migrate:
	migrate -path ./migrations -database "$$DB_URL" up

generate:
	sqlc generate
	oapi-codegen -package=api -generate=types,chi-server \
		../../docs/api/document.openapi.yaml > internal/api/generated.go

init-bucket:
	mc alias set local $$STORAGE_ENDPOINT $$STORAGE_ACCESS_KEY $$STORAGE_SECRET_KEY
	mc mb --ignore-existing local/$$STORAGE_BUCKET
	mc ilm import local/$$STORAGE_BUCKET < scripts/lifecycle.json

run:
	air

docker-build:
	docker build -t teamboard/document:latest .
```

### 17.5 Acceptance Criteria pro Use-Case

| UC | Erfolgs-Kriterien |
|----|-------------------|
| UC-1.1 Initiate Upload | Permission OK, Doc + Version pending, Pre-Signed URL gültig 15min, `document.upload.initiated`-Event |
| UC-1.2 Confirm Upload | HEAD object erfolgreich, Größe matches, Doc active, current_version=1, `document.uploaded`-Event |
| UC-2 New Version | Permission OK, version_number = max+1, Pre-Signed URL, Confirm aktualisiert current_version |
| UC-3 Get Document | Metadata zurück, kein Storage-Call |
| UC-4 Download URL | Pre-Signed URL gültig 5min, korrekte Version (default current) |
| UC-5 List Versions | Alle Versionen DESC, mit `is_current`-Flag |
| UC-6 Rename | Update name, Outbox-Event mit Diff |
| UC-7 List Documents | Cursor-Pagination, nur active+nicht-deleted |
| UC-8 Delete | Soft-Delete, alle Versionen bleiben in DB+S3 (Hard-Delete erst nach Retention) |
| UC-9 Restore | Pointer auf alte Version, Outbox-Event |
| Cleanup | Pending older than 1h → failed; Soft-deleted older than 30d → hard-deleted |

---

## Anhang A: Sequenzdiagramm — Upload Two-Phase

```
Client          Doc Service          Project Svc      Postgres        S3            RabbitMQ
   │                │                      │              │            │                │
   │ POST /docs     │                      │              │            │                │
   │ {name, ct, size}│                     │              │            │                │
   ├───────────────>│                      │              │            │                │
   │                │ Permission(create)   │              │            │                │
   │                ├─────────────────────>│              │            │                │
   │                │<─────────────────────┤              │            │                │
   │                │ BEGIN TX             │              │            │                │
   │                │ INSERT document      │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ INSERT version v1    │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ INSERT outbox        │              │            │                │
   │                │  (upload.initiated)  │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ COMMIT               │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ Generate Pre-Signed  │              │            │                │
   │                │  PUT URL             │              │            │                │
   │                ├──────────────────────────────────────────────────>│                │
   │                │<──────────────────────────────────────────────────┤                │
   │ 201 {doc_id,   │                      │              │            │                │
   │   v=1, url}    │                      │              │            │                │
   │<───────────────┤                      │              │            │                │
   │                │                      │              │            │                │
   │ PUT url        │                      │              │            │                │
   │ + bytes        │                      │              │            │                │
   ├──────────────────────────────────────────────────────────────────>│                │
   │ 200 OK         │                      │              │            │                │
   │<──────────────────────────────────────────────────────────────────┤                │
   │                │                      │              │            │                │
   │ POST /docs/{id}│                      │              │            │                │
   │  /versions/1   │                      │              │            │                │
   │  :confirm      │                      │              │            │                │
   ├───────────────>│                      │              │            │                │
   │                │ Permission(create)   │              │            │                │
   │                ├─────────────────────>│              │            │                │
   │                │ HEAD object          │              │            │                │
   │                ├──────────────────────────────────────────────────>│                │
   │                │ {size, etag}         │              │            │                │
   │                │<──────────────────────────────────────────────────┤                │
   │                │ Validate size match  │              │            │                │
   │                │ BEGIN TX             │              │            │                │
   │                │ UPDATE version       │              │            │                │
   │                │  status=uploaded     │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ UPDATE document      │              │            │                │
   │                │  status=active       │              │            │                │
   │                │  current_version=1   │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ INSERT outbox        │              │            │                │
   │                │  (uploaded)          │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │                │ COMMIT               │              │            │                │
   │                ├─────────────────────────────────────>│            │                │
   │ 200 + Document │                      │              │            │                │
   │<───────────────┤                      │              │            │                │
   │                │ Outbox-Worker async  │              │            │                │
   │                │ Publish              │              │            │                │
   │                ├───────────────────────────────────────────────────────────────────>│
```

## Anhang B: Sequenzdiagramm — Cleanup verwaister Pending-Versionen

```
                 Cleanup Worker          Postgres          S3
                       │                    │              │
   (alle 15 min)       │                    │              │
                       │ ListPendingOlder   │              │
                       │  Than(1h)          │              │
                       ├───────────────────>│              │
                       │ [v1, v2, v3]       │              │
                       │<───────────────────┤              │
                       │                    │              │
                       │ Für jede Version:  │              │
                       │ MarkVersionFailed  │              │
                       ├───────────────────>│              │
                       │ INSERT outbox      │              │
                       │  (upload.failed)   │              │
                       ├───────────────────>│              │
                       │                    │              │
                       │ Wenn Doc keine     │              │
                       │  uploaded versions │              │
                       │  hat: SoftDelete   │              │
                       ├───────────────────>│              │
                       │                    │              │
                       │ S3-Lifecycle-Rule  │              │
                       │  räumt incomplete  │              │
                       │  uploads nach 24h  │              │
                       │  automatisch       │              │
                       │                    │   ─ ─ ─ ─ ─ ─│
```

---

**Ende des Detail-Designs Document Service.**
