-- Go validation (auth.Validate, auth.IssueWithVersion) requires
-- token_version >= 1. The previous CHECK from migration 000004 allowed 0.
-- Repair existing rows first, then tighten the constraint.

UPDATE users SET token_version = 1 WHERE token_version < 1;

ALTER TABLE users DROP CONSTRAINT users_token_version_non_negative;
ALTER TABLE users ADD CONSTRAINT users_token_version_positive
    CHECK (token_version >= 1);