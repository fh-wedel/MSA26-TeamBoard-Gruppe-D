CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

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

CREATE TABLE refresh_tokens (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT NOT NULL UNIQUE,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ NULL,
    replaced_by     UUID NULL REFERENCES refresh_tokens(id),
    user_agent      TEXT NULL,
    ip_address      TEXT NULL
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_active ON refresh_tokens (token_hash) WHERE revoked_at IS NULL;

CREATE TABLE password_reset_tokens (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT NOT NULL UNIQUE,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ NULL
);

CREATE INDEX idx_password_reset_active ON password_reset_tokens (token_hash) WHERE used_at IS NULL;

CREATE TABLE login_attempts (
    id              UUID PRIMARY KEY,
    email           CITEXT NOT NULL,
    success         BOOLEAN NOT NULL,
    ip_address      TEXT NULL,
    user_agent      TEXT NULL,
    attempted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_login_attempts_email_time ON login_attempts (email, attempted_at DESC);

CREATE TABLE signing_keys (
    id              UUID PRIMARY KEY,
    kid             TEXT NOT NULL UNIQUE,
    algorithm       TEXT NOT NULL DEFAULT 'RS256',
    private_key_pem TEXT NOT NULL,
    public_key_pem  TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at    TIMESTAMPTZ NOT NULL,
    retired_at      TIMESTAMPTZ NULL,
    deleted_at      TIMESTAMPTZ NULL
);

CREATE INDEX idx_signing_keys_active ON signing_keys (activated_at) WHERE deleted_at IS NULL;

CREATE TABLE outbox (
    id              UUID PRIMARY KEY,
    aggregate_id    UUID NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at    TIMESTAMPTZ NULL
);

CREATE INDEX idx_outbox_unpublished ON outbox (occurred_at) WHERE published_at IS NULL;
