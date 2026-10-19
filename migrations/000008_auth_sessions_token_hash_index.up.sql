-- refresh_token_hash is looked up on every /auth/refresh request.
-- Without an index that becomes a full table scan.
CREATE UNIQUE INDEX auth_sessions_token_hash_uq
    ON auth_sessions (refresh_token_hash);