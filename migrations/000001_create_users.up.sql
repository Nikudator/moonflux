CREATE TABLE users (
    id UUID PRIMARY KEY,
    nickname VARCHAR(40) NOT NULL,
    email VARCHAR(254),
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    patronymic VARCHAR(100),
    role SMALLINT NOT NULL DEFAULT 1,
    birth_date DATE,
    about TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT users_role_allowed CHECK (role IN (0, 1, 5, 7, 9)),
    CONSTRAINT users_nickname_not_blank CHECK (length(btrim(nickname)) > 0),
    CONSTRAINT users_first_name_not_blank CHECK (length(btrim(first_name)) > 0),
    CONSTRAINT users_last_name_not_blank CHECK (length(btrim(last_name)) > 0)
);

-- Удалённые записи не участвуют в обычной уникальности. При восстановлении
-- сервис обязан проверить конфликт ника/email до снятия deleted_at.
CREATE UNIQUE INDEX users_nickname_active_uq ON users (lower(nickname)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_active_uq ON users (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX users_active_role_idx ON users (role) WHERE deleted_at IS NULL;
CREATE INDEX users_active_created_at_idx ON users (created_at DESC) WHERE deleted_at IS NULL;
