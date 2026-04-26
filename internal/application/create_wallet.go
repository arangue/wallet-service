package application

import (
	"context"
	"log/slog"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type WalletCreation interface {
	Execute(ctx context.Context, ownerID string) (domain.Wallet, error)
}

type walletCreator struct {
	repo domain.WalletRepository
}

func NewWalletCreator(repo domain.WalletRepository) WalletCreation {
	return &walletCreator{repo: repo}
}

func (w walletCreator) Execute(ctx context.Context, ownerID string) (domain.Wallet, error) {
	l := slog.With("owner_id", ownerID)
	l.Info("creating wallet")

	wallet, err := domain.NewWallet(ownerID)
	if err != nil {
		l.Warn("wallet creation failed: invalid domain object", "error", err)
		return domain.Wallet{}, err
	}

	if err := w.repo.Create(ctx, wallet); err != nil {
		l.Error("wallet creation failed", "error", err)
		return domain.Wallet{}, err
	}

	l.Info("wallet created successfully", "wallet_id", wallet.ID)
	return wallet, nil
}
