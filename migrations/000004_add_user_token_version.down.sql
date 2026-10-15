ALTER TABLE users
    DROP CONSTRAINT users_token_version_non_negative,
    DROP COLUMN token_version;