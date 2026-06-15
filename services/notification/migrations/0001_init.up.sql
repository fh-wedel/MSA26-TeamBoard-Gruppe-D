CREATE TABLE notifications (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL,
    type            TEXT NOT NULL,
    payload         JSONB NOT NULL,
    project_id      UUID NULL,
    source_event_id TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at         TIMESTAMPTZ NULL,

    CONSTRAINT notifications_type CHECK (type IN (
        'mention',
        'task_assigned',
        'task_unassigned',
        'task_due_soon',
        'task_deleted',
        'comment_on_my_task',
        'document_shared',
        'project_member_added',
        'project_member_removed',
        'project_invitation_sent',
        'project_invitation_accepted',
        'project_invitation_declined'
    ))
);

CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id, created_at DESC)
    WHERE read_at IS NULL;

CREATE INDEX idx_notifications_user_all
    ON notifications (user_id, created_at DESC);

CREATE INDEX idx_notifications_project
    ON notifications (project_id)
    WHERE project_id IS NOT NULL;

CREATE UNIQUE INDEX idx_notifications_idempotency
    ON notifications (user_id, source_event_id)
    WHERE source_event_id IS NOT NULL;

CREATE TABLE processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type   TEXT NOT NULL,
    payload      JSONB NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;
