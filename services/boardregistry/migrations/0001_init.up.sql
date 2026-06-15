CREATE TABLE board_types (
    id              UUID PRIMARY KEY,
    type            TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL,
    icon            TEXT NOT NULL DEFAULT '',
    default_columns JSONB NOT NULL DEFAULT '[]',
    default_config  JSONB NOT NULL DEFAULT '{}',
    config_schema   JSONB NOT NULL DEFAULT '{}',
    built_in        BOOLEAN NOT NULL DEFAULT FALSE,
    created_by      UUID NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT board_types_type_format CHECK (type ~ '^[a-z][a-z0-9_-]{0,49}$'),
    CONSTRAINT board_types_display_name_length CHECK (char_length(display_name) BETWEEN 1 AND 100)
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
