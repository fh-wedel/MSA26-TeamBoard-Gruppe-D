CREATE TABLE webhooks (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL,
    target_url      TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    secret          TEXT NOT NULL,
    event_filter    TEXT[] NOT NULL DEFAULT '{}',
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT webhooks_target_url_length CHECK (char_length(target_url) <= 2000),
    CONSTRAINT webhooks_description_length CHECK (char_length(description) <= 500)
);

CREATE INDEX idx_webhooks_project_active ON webhooks (project_id) WHERE active = TRUE;
CREATE INDEX idx_webhooks_filter ON webhooks USING GIN (event_filter);

CREATE TABLE webhook_deliveries (
    id                    UUID PRIMARY KEY,
    webhook_id            UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id              TEXT NOT NULL,
    event_type            TEXT NOT NULL,
    payload               JSONB NOT NULL,
    status                TEXT NOT NULL DEFAULT 'pending',
    attempt_count         INTEGER NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_response_status  INTEGER NULL,
    last_response_body    TEXT NULL,
    last_error            TEXT NULL,
    last_attempted_at     TIMESTAMPTZ NULL,
    delivered_at          TIMESTAMPTZ NULL,
    failed_permanently_at TIMESTAMPTZ NULL,
    duration_ms           INTEGER NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT webhook_deliveries_status CHECK (status IN ('pending', 'delivered', 'failed', 'dead')),
    CONSTRAINT webhook_deliveries_attempt_positive CHECK (attempt_count >= 0)
);

CREATE INDEX idx_deliveries_pickup
    ON webhook_deliveries (next_attempt_at)
    WHERE status = 'pending';

CREATE INDEX idx_deliveries_webhook
    ON webhook_deliveries (webhook_id, created_at DESC);

CREATE INDEX idx_deliveries_event
    ON webhook_deliveries (event_id);

CREATE TABLE processed_events (
    event_id        TEXT PRIMARY KEY,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

CREATE TABLE known_projects (
    id              UUID PRIMARY KEY,
    deleted_at      TIMESTAMPTZ NULL
);
