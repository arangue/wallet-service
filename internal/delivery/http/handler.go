package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type createWalletUseCase interface {
	Execute(ctx context.Context, ownerID string) (domain.Wallet, error)
}

type depositUseCase interface {
	Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error)
}

type checkBalanceUseCase interface {
	Execute(ctx context.Context, walletID uuid.UUID) (domain.Wallet, error)
}

type withdrawUseCase interface {
	Execute(ctx context.Context, walletID uuid.UUID, amount domain.Money, reference string) (domain.Transaction, error)
}

type listTransactionsUseCase interface {
	Execute(ctx context.Context, walletID uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error)
}

type Handler struct {
	createWallet     createWalletUseCase
	deposit          depositUseCase
	checkBalance     checkBalanceUseCase
	withdraw         withdrawUseCase
	listTransactions listTransactionsUseCase
	db               pinger
}

func NewHandler(
	createWallet createWalletUseCase,
	deposit depositUseCase,
	checkBalance checkBalanceUseCase,
	withdraw withdrawUseCase,
	listTransactions listTransactionsUseCase,
	db pinger) *Handler {
	return &Handler{
		createWallet:     createWallet,
		deposit:          deposit,
		checkBalance:     checkBalance,
		withdraw:         withdraw,
		listTransactions: listTransactions,
		db:               db,
	}
}

func (h *Handler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close() //nolint:errcheck

	req, ok := decodeJSON[createWalletRequest](w, r)
	if !ok {
		return
	}
	if len(req.OwnerID) > maxOwnerIDLen {
		writeError(w, http.StatusBadRequest, "INVALID_OWNER", "owner_id exceeds maximum length")
		return
	}

	wallet, err := h.createWallet.Execute(r.Context(), req.OwnerID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, newWalletResponse(wallet))
}

func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close() //nolint:errcheck

	walletID, ok := parseWalletID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[depositRequest](w, r)
	if !ok {
		return
	}
	if len(req.Amount) > maxAmountLen {
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amount")
		return
	}
	if len(req.Reference) > maxReferenceLen {
		writeError(w, http.StatusBadRequest, "INVALID_REFERENCE", "reference exceeds maximum length")
		return
	}

	amount, err := domain.NewMoney(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amount")
		return
	}

	tx, err := h.deposit.Execute(r.Context(), walletID, amount, req.Reference)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTransactionResponse(tx))
}

func (h *Handler) CheckBalance(w http.ResponseWriter, r *http.Request) {
	walletID, ok := parseWalletID(w, r)
	if !ok {
		return
	}

	wallet, err := h.checkBalance.Execute(r.Context(), walletID)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newWalletResponse(wallet))
}

func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close() //nolint:errcheck

	walletID, ok := parseWalletID(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[withdrawRequest](w, r)
	if !ok {
		return
	}
	if len(req.Amount) > maxAmountLen {
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amount")
		return
	}
	if len(req.Reference) > maxReferenceLen {
		writeError(w, http.StatusBadRequest, "INVALID_REFERENCE", "reference exceeds maximum length")
		return
	}

	amount, err := domain.NewMoney(req.Amount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "invalid amount")
		return
	}

	tx, err := h.withdraw.Execute(r.Context(), walletID, amount, req.Reference)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, newTransactionResponse(tx))
}

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	walletID, ok := parseWalletID(w, r)
	if !ok {
		return
	}

	limit, ok := parseLimit(w, r)
	if !ok {
		return
	}

	var cursor *domain.Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		c, err := decodeCursor(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "invalid cursor")
			return
		}
		cursor = &c
	}

	txs, err := h.listTransactions.Execute(r.Context(), walletID, limit, cursor)
	if err != nil {
		writeDomainError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newTransactionListResponse(txs, limit))
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		slog.Error("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 50, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100")
		return 0, false
	}
	return n, true
}

func parseWalletID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WALLET_ID", "invalid wallet id")
		return uuid.UUID{}, false
	}
	return id, true
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var zero T

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var v T
	if err := dec.Decode(&v); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		switch {
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, "EMPTY_BODY", "request body must not be empty")
		case errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &syntaxErr):
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body contains malformed JSON")
		case errors.As(err, &unmarshalErr):
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid value for field \""+unmarshalErr.Field+"\"")
		default:
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		}
		return zero, false
	}

	if dec.More() {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must contain a single JSON object")
		return zero, false
	}

	return v, true
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrWalletNotFound):
		writeError(w, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
	case errors.Is(err, domain.ErrWalletExists):
		writeError(w, http.StatusConflict, "WALLET_EXISTS", "wallet already exists")
	case errors.Is(err, domain.ErrInvalidOwnerID):
		writeError(w, http.StatusBadRequest, "INVALID_OWNER", "invalid owner id")
	case errors.Is(err, domain.ErrNonPositiveAmount):
		writeError(w, http.StatusBadRequest, "INVALID_AMOUNT", "amount must be positive")
	case errors.Is(err, domain.ErrInsufficientFunds):
		writeError(w, http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS", "insufficient funds")
	case errors.Is(err, domain.ErrBalanceOverflow):
		writeError(w, http.StatusUnprocessableEntity, "BALANCE_OVERFLOW", "transaction would exceed maximum wallet balance")
	case errors.Is(err, domain.ErrTransactionExists):
		writeError(w, http.StatusConflict, "TRANSACTION_EXISTS", "transaction with this reference already exists")
	default:
		slog.Error("unhandled domain error", "error", err, "request_id", RequestIDFromContext(r.Context()))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(ErrorResponse{Status: status, Code: code, Message: message}); err != nil {
		slog.Error("failed to write error response", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}
