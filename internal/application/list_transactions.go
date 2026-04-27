package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type Lister interface {
	Execute(ctx context.Context, walletID uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error)
}

type lister struct {
	repo domain.WalletRepository
}

func NewLister(repo domain.WalletRepository) Lister {
	return &lister{repo: repo}
}

func (l lister) Execute(ctx context.Context, walletID uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error) {
	if _, err := l.repo.FindByID(ctx, walletID); err != nil {
		return nil, err
	}
	return l.repo.ListTransactions(ctx, walletID, limit, cursor)
}
