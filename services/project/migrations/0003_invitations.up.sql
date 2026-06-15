CREATE TABLE invitations (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    UUID         NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    invitee_email TEXT         NOT NULL,
    role          TEXT         NOT NULL DEFAULT 'viewer',
    token         TEXT         NOT NULL UNIQUE,
    invited_by    UUID         NOT NULL,
    status        TEXT         NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ  NOT NULL DEFAULT (now() + interval '7 days'),
    responded_at  TIMESTAMPTZ
);

CREATE INDEX ON invitations(token);
CREATE INDEX ON invitations(project_id);
CREATE INDEX ON invitations(invitee_email, status);
