CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE known_boards (
    id         UUID        PRIMARY KEY,
    project_id UUID        NOT NULL,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_boards_project ON known_boards (project_id) WHERE deleted_at IS NULL;

CREATE TABLE known_columns (
    id       UUID    PRIMARY KEY,
    board_id UUID    NOT NULL REFERENCES known_boards(id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    position INTEGER NOT NULL,
    -- Explicit semantic status, propagated from the board type via
    -- board.created / column.* events. Task status is derived from this instead
    -- of guessing from the column name (DeriveStatus remains a fallback).
    status   TEXT    NOT NULL DEFAULT 'open'
);

CREATE INDEX idx_known_columns_board ON known_columns (board_id, position);

CREATE TABLE known_users (
    id         UUID   PRIMARY KEY,
    email      CITEXT NOT NULL UNIQUE,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_users_email_active ON known_users (email) WHERE deleted_at IS NULL;

CREATE TABLE tasks (
    id          UUID        PRIMARY KEY,
    board_id    UUID        NOT NULL REFERENCES known_boards(id),
    project_id  UUID        NOT NULL,
    column_id   UUID        NULL REFERENCES known_columns(id),
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'open',
    priority    TEXT        NOT NULL DEFAULT 'medium',
    assignee_id UUID        NULL,
    due_date    TIMESTAMPTZ NULL,
    labels      TEXT[]      NOT NULL DEFAULT '{}',
    position    TEXT        NOT NULL,
    created_by  UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL,
    -- Optional start date, enabling a real start..end span in the timeline (Gantt)
    -- view. Tasks without it fall back to created_at when charted.
    start_date  TIMESTAMPTZ NULL,

    CONSTRAINT tasks_title_length       CHECK (char_length(title) BETWEEN 1 AND 500),
    CONSTRAINT tasks_description_length CHECK (char_length(description) <= 10000),
    CONSTRAINT tasks_status             CHECK (status IN ('open','in_progress','blocked','done','archived')),
    CONSTRAINT tasks_priority           CHECK (priority IN ('low','medium','high','critical'))
);

CREATE INDEX idx_tasks_board_active ON tasks (board_id, column_id, position) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_assignee     ON tasks (assignee_id)                    WHERE deleted_at IS NULL AND assignee_id IS NOT NULL;
CREATE INDEX idx_tasks_project      ON tasks (project_id)                     WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_due_date     ON tasks (due_date)                       WHERE deleted_at IS NULL AND due_date IS NOT NULL;
CREATE INDEX idx_tasks_labels       ON tasks USING GIN (labels)               WHERE deleted_at IS NULL;

CREATE TABLE task_comments (
    id         UUID        PRIMARY KEY,
    task_id    UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_id  UUID        NOT NULL,
    body       TEXT        NOT NULL,
    edited_at  TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,

    CONSTRAINT task_comments_body_length CHECK (char_length(body) BETWEEN 1 AND 10000)
);

CREATE INDEX idx_task_comments_task ON task_comments (task_id, created_at) WHERE deleted_at IS NULL;

CREATE TABLE comment_mentions (
    comment_id UUID NOT NULL REFERENCES task_comments(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,

    PRIMARY KEY (comment_id, user_id)
);

CREATE INDEX idx_comment_mentions_user ON comment_mentions (user_id);

CREATE TABLE comment_history (
    id          UUID        PRIMARY KEY,
    comment_id  UUID        NOT NULL REFERENCES task_comments(id) ON DELETE CASCADE,
    body        TEXT        NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_attachments (
    id          UUID        PRIMARY KEY,
    task_id     UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    document_id UUID        NOT NULL,
    added_by    UUID        NOT NULL,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (task_id, document_id)
);

CREATE INDEX idx_task_attachments_task     ON task_attachments (task_id);
CREATE INDEX idx_task_attachments_document ON task_attachments (document_id);

CREATE TABLE task_history (
    id          UUID        PRIMARY KEY,
    task_id     UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    actor_id    UUID        NOT NULL,
    change_type TEXT        NOT NULL,
    diff        JSONB       NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT task_history_change_type CHECK (change_type IN (
        'created','updated','moved','assigned','unassigned',
        'commented','attachment_added','attachment_removed','deleted'
    ))
);

CREATE INDEX idx_task_history_task ON task_history (task_id, occurred_at DESC);

CREATE TABLE outbox (
    id           UUID        PRIMARY KEY,
    aggregate_id UUID        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;

CREATE TABLE processed_events (
    event_id     TEXT        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
