CREATE TABLE IF NOT EXISTS wallets (
    id          UUID           PRIMARY KEY,
    owner_id    TEXT           NOT NULL UNIQUE,
    balance     NUMERIC(20, 8) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ    NOT NULL,
    updated_at  TIMESTAMPTZ    NOT NULL
);
