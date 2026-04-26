package http

import "net/http"

func NewRouter(h *Handler) http.Handler {
	r := http.NewServeMux()

	r.HandleFunc("POST /wallets", h.CreateWallet)
	r.HandleFunc("POST /wallets/{id}/deposit", h.Deposit)
	r.HandleFunc("GET /wallets/{id}", h.CheckBalance)
	r.HandleFunc("POST /wallets/{id}/withdraw", h.Withdraw)
	r.HandleFunc("GET /wallets/{id}/transactions", h.ListTransactions)

	r.HandleFunc("GET /health", h.Health)

	return CorrelationIDMiddleware(RecoveryMiddleware(LoggingMiddleware(r)))
}
