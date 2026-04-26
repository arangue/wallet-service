package domain

import (
	"time"

	"github.com/google/uuid"
)

// Wallet is the aggregate root — all balance mutations go through it.
type Wallet struct {
	ID        uuid.UUID
	OwnerID   string
	Balance   Money
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewWallet creates a new Wallet with a zero balance.
func NewWallet(ownerID string) (Wallet, error) {
	if ownerID == "" {
		return Wallet{}, ErrInvalidOwnerID
	}
	now := time.Now().UTC()
	return Wallet{
		ID:        uuid.New(),
		OwnerID:   ownerID,
		Balance:   Zero(),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (w *Wallet) Credit(amount Money) error {
	if !amount.IsPositive() {
		return ErrNonPositiveAmount
	}
	newBalance := w.Balance.Add(amount)
	if newBalance.GreaterThan(MaxMoney) {
		return ErrBalanceOverflow
	}
	w.Balance = newBalance
	w.UpdatedAt = time.Now().UTC()
	return nil
}

func (w *Wallet) Debit(amount Money) error {
	if !amount.IsPositive() {
		return ErrNonPositiveAmount
	}
	newBalance, err := w.Balance.Sub(amount)
	if err != nil {
		return err
	}
	w.Balance = newBalance
	w.UpdatedAt = time.Now().UTC()
	return nil
}
