CREATE TABLE known_projects (
    id         UUID PRIMARY KEY,
    deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE known_users (
    id         UUID PRIMARY KEY,
    deleted_at TIMESTAMPTZ NULL
);

CREATE TABLE documents (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL,
    name            TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    current_version INTEGER NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ NULL,

    CONSTRAINT documents_name_length    CHECK (char_length(name) BETWEEN 1 AND 255),
    CONSTRAINT documents_status         CHECK (status IN ('pending', 'active', 'deleted')),
    CONSTRAINT documents_content_type   CHECK (char_length(content_type) <= 200)
);

CREATE INDEX idx_documents_project_active ON documents (project_id, created_at DESC)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_documents_pending ON documents (created_at)
    WHERE status = 'pending';

CREATE TABLE document_versions (
    id              UUID PRIMARY KEY,
    document_id     UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version_number  INTEGER NOT NULL,
    storage_key     TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    checksum_sha256 TEXT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    uploaded_by     UUID NOT NULL,
    uploaded_at     TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT document_versions_status    CHECK (status IN ('pending', 'uploaded', 'failed')),
    CONSTRAINT document_versions_positive  CHECK (size_bytes > 0),
    CONSTRAINT document_versions_unique    UNIQUE (document_id, version_number)
);

CREATE INDEX idx_document_versions_doc     ON document_versions (document_id, version_number DESC);
CREATE INDEX idx_document_versions_pending ON document_versions (created_at) WHERE status = 'pending';
CREATE INDEX idx_document_versions_key     ON document_versions (storage_key);

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type   TEXT NOT NULL,
    payload      JSONB NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

CREATE TABLE processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
