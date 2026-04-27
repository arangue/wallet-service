CREATE UNIQUE INDEX uq_transactions_wallet_reference
    ON transactions(wallet_id, type, reference)
    WHERE reference <> '';
