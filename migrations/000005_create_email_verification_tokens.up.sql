-- Add email verification state to users.
ALTER TABLE users
    ADD COLUMN email_verified_at TIMESTAMPTZ;

-- Store only token hashes, never raw verification tokens.
CREATE TABLE email_verification_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    token_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT email_verification_tokens_expiry_after_creation
        CHECK (expires_at > created_at),
    CONSTRAINT email_verification_tokens_use_after_creation
        CHECK (used_at IS NULL OR used_at >= created_at),
    CONSTRAINT email_verification_tokens_revoke_after_creation
        CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CONSTRAINT email_verification_tokens_use_or_revoke
        CHECK (used_at IS NULL OR revoked_at IS NULL)
);

-- A token hash must be unique.
CREATE UNIQUE INDEX email_verification_tokens_hash_uq
    ON email_verification_tokens (token_hash);
-- Allow only one active verification token per user.
CREATE UNIQUE INDEX email_verification_tokens_one_active_per_user_uq
    ON email_verification_tokens (user_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;

-- Support finding expired tokens for maintenance.
CREATE INDEX email_verification_tokens_expiry_idx
    ON email_verification_tokens (expires_at)
    WHERE used_at IS NULL AND revoked_at IS NULL;
