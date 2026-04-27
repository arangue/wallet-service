package application

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type Withdrawer interface {
	Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error)
}

type withdrawer struct {
	repo       domain.WalletRepository
	transactor domain.Transactor
}

func NewWithdrawer(repo domain.WalletRepository, transactor domain.Transactor) Withdrawer {
	return &withdrawer{repo: repo,
		transactor: transactor,
	}
}

func (w withdrawer) Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error) {
	l := slog.With("wallet_id", walletID, "amount", amount.String(), "reference", reference)
	l.Debug("processing withdrawal")

	var tx domain.Transaction

	err := w.transactor.RunInTx(ctx, func(ctx context.Context) error {
		wallet, err := w.repo.FindByIDForUpdate(ctx, walletID)
		if err != nil {
			return err
		}

		if err := wallet.Debit(amount); err != nil {
			return err
		}

		tx = domain.NewTransaction(wallet.ID, domain.TransactionTypeWithdraw, amount, wallet.Balance, reference)

		if err := w.repo.Save(ctx, wallet); err != nil {
			return err
		}

		return w.repo.CreateTransaction(ctx, tx)

	})

	if err != nil {
		if errors.Is(err, domain.ErrWalletNotFound) ||
			errors.Is(err, domain.ErrInsufficientFunds) ||
			errors.Is(err, domain.ErrTransactionExists) {
			l.Warn("withdrawal failed", "error", err)
		} else {
			l.Error("withdrawal failed", "error", err)
		}
		return domain.Transaction{}, err
	}

	l.Info("withdrawal successful", "transaction_id", tx.ID)
	return tx, nil
}
