CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             VARCHAR(255) NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    role_id           SMALLINT NOT NULL REFERENCES roles(id),
    is_active         BOOLEAN NOT NULL DEFAULT true,

    totp_enabled      BOOLEAN NOT NULL DEFAULT false,
    totp_secret_enc   TEXT,
    totp_confirmed_at TIMESTAMPTZ,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_role_id ON users (role_id);
