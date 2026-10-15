ALTER TABLE users
    ADD COLUMN token_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT users_token_version_non_negative
        CHECK (token_version >= 0);