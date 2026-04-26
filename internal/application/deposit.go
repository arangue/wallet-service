package application

import (
	"context"
	"log/slog"

	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/google/uuid"
)

type Depositor interface {
	Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error)
}

type depositor struct {
	repo       domain.WalletRepository
	transactor domain.Transactor
}

func NewDepositor(repo domain.WalletRepository, transactor domain.Transactor) Depositor {
	return &depositor{repo: repo, transactor: transactor}
}

func (d depositor) Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error) {
	l := slog.With("wallet_id", walletID, "amount", amount.String(), "reference", reference)
	l.Debug("processing deposit")

	if !amount.IsPositive() {
		l.Warn("deposit failed: non-positive amount")
		return domain.Transaction{}, domain.ErrNonPositiveAmount
	}

	var tx domain.Transaction

	err := d.transactor.RunInTx(ctx, func(ctx context.Context) error {
		wallet, err := d.repo.FindByIDForUpdate(ctx, walletID)
		if err != nil {
			return err
		}

		if err := wallet.Credit(amount); err != nil {
			return err
		}

		tx = domain.NewTransaction(wallet.ID, domain.TransactionTypeDeposit, amount, wallet.Balance, reference)

		if err := d.repo.Save(ctx, wallet); err != nil {
			return err
		}

		return d.repo.CreateTransaction(ctx, tx)

	})

	if err != nil {
		l.Error("deposit failed", "error", err)
		return domain.Transaction{}, err
	}

	l.Info("deposit successful", "transaction_id", tx.ID)
	return tx, nil
}
