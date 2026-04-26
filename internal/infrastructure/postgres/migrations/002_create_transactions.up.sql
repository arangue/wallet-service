CREATE TABLE IF NOT EXISTS transactions (
    id            UUID           PRIMARY KEY,
    wallet_id     UUID           NOT NULL REFERENCES wallets(id),
    type          TEXT           NOT NULL CHECK (type IN ('deposit', 'withdraw')),
    amount        NUMERIC(20, 8) NOT NULL CHECK (amount > 0),
    balance_after NUMERIC(20, 8) NOT NULL,
    reference     TEXT           NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transactions_wallet_id ON transactions(wallet_id);
