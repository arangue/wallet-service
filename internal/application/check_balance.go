package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type Checker interface {
	Execute(ctx context.Context, walletID uuid.UUID) (domain.Wallet, error)
}

type checker struct {
	repo domain.WalletRepository
}

func NewChecker(repo domain.WalletRepository) Checker {
	return &checker{
		repo: repo,
	}
}

func (c checker) Execute(ctx context.Context, walletID uuid.UUID) (domain.Wallet, error) {
	return c.repo.FindByID(ctx, walletID)
}
