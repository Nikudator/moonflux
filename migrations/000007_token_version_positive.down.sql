ALTER TABLE users DROP CONSTRAINT users_token_version_positive;
ALTER TABLE users ADD CONSTRAINT users_token_version_non_negative
    CHECK (token_version >= 0);