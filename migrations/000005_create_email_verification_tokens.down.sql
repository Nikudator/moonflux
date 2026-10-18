-- Remove email verification token indexes.
DROP INDEX IF EXISTS email_verification_tokens_expiry_idx;
DROP INDEX IF EXISTS email_verification_tokens_hash_uq;
DROP INDEX IF EXISTS email_verification_tokens_one_active_per_user_uq;
-- Remove email verification tokens.
DROP TABLE IF EXISTS email_verification_tokens;

-- Remove email verification state from users.
ALTER TABLE users
    DROP COLUMN IF EXISTS email_verified_at;
