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
    -- Declarative rendering hints for the frontend (which built-in view renderer to
    -- use and how to parametrize it). Validated against a host-defined meta-schema
    -- in the domain layer, not author-defined. See ADR 0002.
    presentation    JSONB NOT NULL DEFAULT '{}'::jsonb,

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

-- Built-in board types shipped out of the box: kanban + calendar. Each default
-- column carries an explicit semantic status so the task service does not have to
-- guess it from the column name. The presentation spec selects a built-in frontend
-- renderer and parametrizes it declaratively (validated against the host meta-schema).
--
-- These are marked built_in (immutable/non-deletable). Additional types such as
-- scrum and gantt are NOT seeded here: they are registered at runtime via the public
-- API to demonstrate the registry's extensibility — see docs/demo/board-types.ipynb.
INSERT INTO board_types (id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in)
VALUES
(
    gen_random_uuid(), 'kanban', 'Kanban Board', '📋',
    '[
        {"name":"To Do","position":0,"status":"open"},
        {"name":"In Progress","position":1,"wip_limit":3,"status":"in_progress"},
        {"name":"Done","position":2,"status":"done"}
    ]'::jsonb,
    '{}'::jsonb,
    '{}'::jsonb,
    '{
        "view":"board",
        "view_config":{"group_by":"column","show_wip":true},
        "card":{"fields":["priority","due_date","labels","comment_count","attachment_count"],"color_by":"priority"}
    }'::jsonb,
    TRUE
),
(
    gen_random_uuid(), 'calendar', 'Calendar', '📅',
    '[{"name":"Scheduled","position":0,"status":"open"}]'::jsonb,
    '{"week_start":"monday"}'::jsonb,
    '{"type":"object","properties":{"week_start":{"type":"string","enum":["monday","sunday"]}},"additionalProperties":false}'::jsonb,
    '{
        "view":"calendar",
        "view_config":{"date_field":"due_date","week_start":"monday"},
        "card":{"fields":["priority","labels"],"color_by":"priority"}
    }'::jsonb,
    TRUE
);
