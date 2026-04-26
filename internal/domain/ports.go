package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Cursor marks the position in a paginated transaction list.
type Cursor struct {
	Time time.Time
	ID   uuid.UUID
}

type WalletRepository interface {
	Create(ctx context.Context, wallet Wallet) error
	FindByID(ctx context.Context, walletID uuid.UUID) (Wallet, error)
	FindByIDForUpdate(ctx context.Context, walletID uuid.UUID) (Wallet, error)
	Save(ctx context.Context, wallet Wallet) error
	CreateTransaction(ctx context.Context, tx Transaction) error
	ListTransactions(ctx context.Context, walletID uuid.UUID, limit int, cursor *Cursor) ([]Transaction, error)
}

type Transactor interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
