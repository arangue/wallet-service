package domain

import (
	"time"

	"github.com/google/uuid"
)

type TransactionType string

const (
	TransactionTypeDeposit  TransactionType = "deposit"
	TransactionTypeWithdraw TransactionType = "withdraw"
)

type Transaction struct {
	ID           uuid.UUID
	WalletID     uuid.UUID
	Type         TransactionType
	Amount       Money
	BalanceAfter Money
	Reference    string
	CreatedAt    time.Time
}

func NewTransaction(
	walletID uuid.UUID,
	txType TransactionType,
	amount Money,
	balanceAfter Money,
	reference string,
) Transaction {
	return Transaction{
		ID:           uuid.New(),
		WalletID:     walletID,
		Type:         txType,
		Amount:       amount,
		BalanceAfter: balanceAfter,
		Reference:    reference,
		CreatedAt:    time.Now().UTC(),
	}
}
