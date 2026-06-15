CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE known_users (
    id         UUID    PRIMARY KEY,
    email      CITEXT  NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_known_users_email_active ON known_users (email) WHERE deleted_at IS NULL;

CREATE TABLE projects (
    id          UUID    PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    owner_id    UUID    NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL,

    CONSTRAINT projects_name_length CHECK (char_length(name) BETWEEN 1 AND 200)
);

CREATE INDEX idx_projects_owner  ON projects (owner_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_active ON projects (id) WHERE deleted_at IS NULL;

CREATE TABLE roles (
    name        TEXT    PRIMARY KEY,
    rank        INTEGER NOT NULL UNIQUE,
    description TEXT    NOT NULL DEFAULT ''
);

INSERT INTO roles (name, rank, description) VALUES
    ('viewer', 10, 'Read-only access'),
    ('editor', 20, 'Can modify content but not project settings'),
    ('owner',  30, 'Full control including member management and deletion');

CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    role       TEXT NOT NULL REFERENCES roles(name),
    invited_by UUID NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user    ON project_members (user_id);
CREATE INDEX idx_project_members_project ON project_members (project_id);

CREATE TABLE boards (
    id         UUID    PRIMARY KEY,
    project_id UUID    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    type       TEXT    NOT NULL DEFAULT 'kanban',
    position   INTEGER NOT NULL DEFAULT 0,
    config     JSONB   NOT NULL DEFAULT '{}'::JSONB,
    created_by UUID    NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,

    CONSTRAINT boards_type        CHECK (type IN ('kanban', 'scrum', 'calendar')),
    CONSTRAINT boards_name_length CHECK (char_length(name) BETWEEN 1 AND 100)
);

CREATE INDEX idx_boards_project ON boards (project_id, position) WHERE deleted_at IS NULL;

CREATE TABLE board_columns (
    id         UUID    PRIMARY KEY,
    board_id   UUID    NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    position   INTEGER NOT NULL,
    wip_limit  INTEGER NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT board_columns_name_length     CHECK (char_length(name) BETWEEN 1 AND 50),
    CONSTRAINT board_columns_unique_position UNIQUE (board_id, position)
);

CREATE INDEX idx_board_columns_board ON board_columns (board_id, position);

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
