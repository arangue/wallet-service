package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type WalletRepository struct {
	pool *pgxpool.Pool
}

func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository {
	return &WalletRepository{pool: pool}
}

const uniqueViolation = "23505"

func (r *WalletRepository) Create(ctx context.Context, wallet domain.Wallet) error {
	_, err := executorFromContext(ctx, r.pool).Exec(ctx, `
		INSERT INTO wallets (id, owner_id, balance, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`,
		wallet.ID,
		wallet.OwnerID,
		wallet.Balance.Decimal(),
		wallet.CreatedAt,
		wallet.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.ErrWalletExists
		}
		return err
	}
	return nil
}

func (r *WalletRepository) FindByID(ctx context.Context, walletID uuid.UUID) (domain.Wallet, error) {
	row := executorFromContext(ctx, r.pool).QueryRow(ctx, `
		SELECT id, owner_id, balance, created_at, updated_at
		FROM wallets
		WHERE id = $1`,
		walletID,
	)
	return scanWallet(row)
}

func (r *WalletRepository) FindByIDForUpdate(ctx context.Context, walletID uuid.UUID) (domain.Wallet, error) {
	row := executorFromContext(ctx, r.pool).QueryRow(ctx, `
		SELECT id, owner_id, balance, created_at, updated_at
		FROM wallets
		WHERE id = $1
		FOR UPDATE`,
		walletID,
	)
	return scanWallet(row)
}

func (r *WalletRepository) Save(ctx context.Context, wallet domain.Wallet) error {
	_, err := executorFromContext(ctx, r.pool).Exec(ctx, `
		UPDATE wallets
		SET balance = $1, updated_at = $2
		WHERE id = $3`,
		wallet.Balance.Decimal(),
		wallet.UpdatedAt,
		wallet.ID,
	)
	return err
}

func (r *WalletRepository) CreateTransaction(ctx context.Context, tx domain.Transaction) error {
	_, err := executorFromContext(ctx, r.pool).Exec(ctx, `
        INSERT INTO transactions (id, wallet_id, type, amount, balance_after, reference, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `,
		tx.ID,
		tx.WalletID,
		tx.Type,
		tx.Amount.Decimal(),
		tx.BalanceAfter.Decimal(),
		tx.Reference,
		tx.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "uq_transactions_wallet_reference" {
			return domain.ErrTransactionExists
		}
		return err
	}
	return nil
}

func (r *WalletRepository) ListTransactions(ctx context.Context, walletID uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error) {
	var cursorTime *time.Time
	var cursorID *uuid.UUID
	if cursor != nil {
		cursorTime = &cursor.Time
		cursorID = &cursor.ID
	}

	rows, err := executorFromContext(ctx, r.pool).Query(ctx, `
		SELECT id, wallet_id, type, amount, balance_after, reference, created_at
		FROM transactions
		WHERE wallet_id = $1
		  AND ($2::timestamptz IS NULL OR created_at < $2 OR (created_at = $2 AND id < $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`,
		walletID, cursorTime, cursorID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []domain.Transaction
	for rows.Next() {
		var (
			tx           domain.Transaction
			txType       string
			amount       decimal.Decimal
			balanceAfter decimal.Decimal
		)
		if err := rows.Scan(&tx.ID, &tx.WalletID, &txType, &amount, &balanceAfter, &tx.Reference, &tx.CreatedAt); err != nil {
			return nil, err
		}
		tx.Type = domain.TransactionType(txType)
		tx.Amount, err = domain.NewMoneyFromDecimal(amount)
		if err != nil {
			return nil, err
		}
		tx.BalanceAfter, err = domain.NewMoneyFromDecimal(balanceAfter)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, rows.Err()
}

func scanWallet(row pgx.Row) (domain.Wallet, error) {
	var (
		w       domain.Wallet
		balance decimal.Decimal
	)
	if err := row.Scan(&w.ID, &w.OwnerID, &balance, &w.CreatedAt, &w.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Wallet{}, domain.ErrWalletNotFound
		}
		return domain.Wallet{}, err
	}
	var err error
	w.Balance, err = domain.NewMoneyFromDecimal(balance)
	if err != nil {
		return domain.Wallet{}, err
	}
	return w, nil
}
