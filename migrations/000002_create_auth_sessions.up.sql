CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    refresh_token_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT auth_sessions_expiry_after_creation CHECK (expires_at > created_at)
);

CREATE INDEX auth_sessions_active_user_idx ON auth_sessions (user_id) WHERE revoked_at IS NULL AND deleted_at IS NULL;
CREATE INDEX auth_sessions_active_expiry_idx ON auth_sessions (expires_at) WHERE revoked_at IS NULL AND deleted_at IS NULL;
