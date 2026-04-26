package http

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type cursorPayload struct {
	Before   time.Time `json:"before"`
	BeforeID uuid.UUID `json:"before_id"`
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(cursorPayload{Before: t, BeforeID: id})
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (domain.Cursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return domain.Cursor{}, err
	}
	var p cursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return domain.Cursor{}, err
	}
	return domain.Cursor{Time: p.Before, ID: p.BeforeID}, nil
}

type transactionListResponse struct {
	Data       []transactionResponse `json:"data"`
	NextCursor *string               `json:"next_cursor,omitempty"`
}

func newTransactionListResponse(txs []domain.Transaction, limit int) transactionListResponse {
	data := make([]transactionResponse, len(txs))
	for i, t := range txs {
		data[i] = newTransactionResponse(t)
	}

	var nextCursor *string
	if len(txs) == limit {
		last := txs[len(txs)-1]
		s := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &s
	}

	return transactionListResponse{Data: data, NextCursor: nextCursor}
}

type ErrorResponse struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type walletResponse struct {
	ID        uuid.UUID `json:"id"`
	OwnerID   string    `json:"owner_id"`
	Balance   string    `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newWalletResponse(w domain.Wallet) walletResponse {
	return walletResponse{
		ID:        w.ID,
		OwnerID:   w.OwnerID,
		Balance:   w.Balance.String(),
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}

type transactionResponse struct {
	ID           uuid.UUID `json:"id"`
	WalletID     uuid.UUID `json:"wallet_id"`
	Type         string    `json:"type"`
	Amount       string    `json:"amount"`
	BalanceAfter string    `json:"balance_after"`
	Reference    string    `json:"reference"`
	CreatedAt    time.Time `json:"created_at"`
}

func newTransactionResponse(tx domain.Transaction) transactionResponse {
	return transactionResponse{
		ID:           tx.ID,
		WalletID:     tx.WalletID,
		Type:         string(tx.Type),
		Amount:       tx.Amount.String(),
		BalanceAfter: tx.BalanceAfter.String(),
		Reference:    tx.Reference,
		CreatedAt:    tx.CreatedAt,
	}
}
