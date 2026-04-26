package domain

import "errors"

var (
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrNegativeAmount    = errors.New("amount must be non-negative")
	ErrNonPositiveAmount = errors.New("amount must be positive")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidOwnerID    = errors.New("invalid owner id")
	ErrWalletNotFound    = errors.New("wallet not found")
	ErrWalletExists      = errors.New("wallet already exists")
	ErrTransactionExists = errors.New("transaction with this reference already exists")
	ErrBalanceOverflow   = errors.New("transaction would exceed maximum wallet balance")
)
