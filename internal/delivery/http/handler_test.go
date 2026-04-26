package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	httpdelivery "github.com/arangue/challenge-wallet/internal/delivery/http"
	"github.com/arangue/challenge-wallet/internal/domain"
)

// -- mocks --

type mockCreateWallet struct {
	wallet domain.Wallet
	err    error
}

func (m mockCreateWallet) Execute(_ context.Context, ownerID string) (domain.Wallet, error) {
	return m.wallet, m.err
}

type mockDeposit struct {
	tx  domain.Transaction
	err error
}

func (m mockDeposit) Execute(_ context.Context, _ uuid.UUID, _ domain.Money, _ string) (domain.Transaction, error) {
	return m.tx, m.err
}

type mockCheckBalance struct {
	wallet domain.Wallet
	err    error
}

func (m mockCheckBalance) Execute(_ context.Context, _ uuid.UUID) (domain.Wallet, error) {
	return m.wallet, m.err
}

type mockWithdraw struct {
	tx  domain.Transaction
	err error
}

func (m mockWithdraw) Execute(_ context.Context, _ uuid.UUID, _ domain.Money, _ string) (domain.Transaction, error) {
	return m.tx, m.err
}

type mockPinger struct{ err error }

func (m mockPinger) Ping(_ context.Context) error { return m.err }

type mockListTransactions struct {
	txs []domain.Transaction
	err error
}

func (m mockListTransactions) Execute(_ context.Context, _ uuid.UUID, _ int, _ *domain.Cursor) ([]domain.Transaction, error) {
	return m.txs, m.err
}

func newRouter(cw mockCreateWallet, d mockDeposit, cb mockCheckBalance, w mockWithdraw, lt mockListTransactions, p mockPinger) http.Handler {
	h := httpdelivery.NewHandler(cw, d, cb, w, lt, p)
	return httpdelivery.NewRouter(h)
}

func do(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("expected status %d, got %d — body: %s", want, rec.Code, rec.Body.String())
	}
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != want {
		t.Fatalf("expected error code %q, got %q", want, resp.Code)
	}
}

func aWallet() domain.Wallet {
	w, _ := domain.NewWallet("test-owner")
	return w
}

func aTransaction() domain.Transaction {
	amount, _ := domain.NewMoney("10.00")
	balance, _ := domain.NewMoney("90.00")
	return domain.NewTransaction(uuid.New(), domain.TransactionTypeDeposit, amount, balance, "ref")
}

// -- tests --

func TestCreateWalletHandler(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		mock        mockCreateWallet
		wantStatus  int
		wantErrCode string
	}{
		{
			name:       "success",
			body:       `{"owner_id":"owner-1"}`,
			mock:       mockCreateWallet{wallet: aWallet()},
			wantStatus: http.StatusCreated,
		},
		{
			name:        "empty body",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "EMPTY_BODY",
		},
		{
			name:        "malformed json",
			body:        "{bad",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_JSON",
		},
		{
			name:        "wallet exists",
			body:        `{"owner_id":"owner-1"}`,
			mock:        mockCreateWallet{err: domain.ErrWalletExists},
			wantStatus:  http.StatusConflict,
			wantErrCode: "WALLET_EXISTS",
		},
		{
			name:        "invalid owner",
			body:        `{"owner_id":""}`,
			mock:        mockCreateWallet{err: domain.ErrInvalidOwnerID},
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_OWNER",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newRouter(tc.mock, mockDeposit{}, mockCheckBalance{}, mockWithdraw{}, mockListTransactions{}, mockPinger{})
			rec := do(t, router, http.MethodPost, "/wallets", tc.body)
			assertStatus(t, rec, tc.wantStatus)
			if tc.wantErrCode != "" {
				assertErrorCode(t, rec, tc.wantErrCode)
			}
		})
	}
}

func TestDepositHandler(t *testing.T) {
	validID := uuid.New().String()
	validBody := `{"amount":"10.00","reference":"ref-1"}`

	tests := []struct {
		name        string
		path        string
		body        string
		mock        mockDeposit
		wantStatus  int
		wantErrCode string
	}{
		{
			name:       "success",
			path:       "/wallets/" + validID + "/deposit",
			body:       validBody,
			mock:       mockDeposit{tx: aTransaction()},
			wantStatus: http.StatusCreated,
		},
		{
			name:        "invalid wallet id",
			path:        "/wallets/not-a-uuid/deposit",
			body:        validBody,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_WALLET_ID",
		},
		{
			name:        "empty body",
			path:        "/wallets/" + validID + "/deposit",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "EMPTY_BODY",
		},
		{
			name:        "trailing json",
			path:        "/wallets/" + validID + "/deposit",
			body:        validBody + "{}",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_JSON",
		},
		{
			name:        "wallet not found",
			path:        "/wallets/" + validID + "/deposit",
			body:        validBody,
			mock:        mockDeposit{err: domain.ErrWalletNotFound},
			wantStatus:  http.StatusNotFound,
			wantErrCode: "WALLET_NOT_FOUND",
		},
		{
			name:        "transaction exists",
			path:        "/wallets/" + validID + "/deposit",
			body:        validBody,
			mock:        mockDeposit{err: domain.ErrTransactionExists},
			wantStatus:  http.StatusConflict,
			wantErrCode: "TRANSACTION_EXISTS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newRouter(mockCreateWallet{}, tc.mock, mockCheckBalance{}, mockWithdraw{}, mockListTransactions{}, mockPinger{})
			rec := do(t, router, http.MethodPost, tc.path, tc.body)
			assertStatus(t, rec, tc.wantStatus)
			if tc.wantErrCode != "" {
				assertErrorCode(t, rec, tc.wantErrCode)
			}
		})
	}
}

func TestWithdrawHandler(t *testing.T) {
	validID := uuid.New().String()
	validBody := `{"amount":"10.00","reference":"ref-1"}`

	tests := []struct {
		name        string
		path        string
		body        string
		mock        mockWithdraw
		wantStatus  int
		wantErrCode string
	}{
		{
			name:       "success",
			path:       "/wallets/" + validID + "/withdraw",
			body:       validBody,
			mock:       mockWithdraw{tx: aTransaction()},
			wantStatus: http.StatusCreated,
		},
		{
			name:        "invalid wallet id",
			path:        "/wallets/not-a-uuid/withdraw",
			body:        validBody,
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_WALLET_ID",
		},
		{
			name:        "empty body",
			path:        "/wallets/" + validID + "/withdraw",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "EMPTY_BODY",
		},
		{
			name:        "insufficient funds",
			path:        "/wallets/" + validID + "/withdraw",
			body:        validBody,
			mock:        mockWithdraw{err: domain.ErrInsufficientFunds},
			wantStatus:  http.StatusUnprocessableEntity,
			wantErrCode: "INSUFFICIENT_FUNDS",
		},
		{
			name:        "wallet not found",
			path:        "/wallets/" + validID + "/withdraw",
			body:        validBody,
			mock:        mockWithdraw{err: domain.ErrWalletNotFound},
			wantStatus:  http.StatusNotFound,
			wantErrCode: "WALLET_NOT_FOUND",
		},
		{
			name:        "transaction exists",
			path:        "/wallets/" + validID + "/withdraw",
			body:        validBody,
			mock:        mockWithdraw{err: domain.ErrTransactionExists},
			wantStatus:  http.StatusConflict,
			wantErrCode: "TRANSACTION_EXISTS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newRouter(mockCreateWallet{}, mockDeposit{}, mockCheckBalance{}, tc.mock, mockListTransactions{}, mockPinger{})
			rec := do(t, router, http.MethodPost, tc.path, tc.body)
			assertStatus(t, rec, tc.wantStatus)
			if tc.wantErrCode != "" {
				assertErrorCode(t, rec, tc.wantErrCode)
			}
		})
	}
}

func TestCheckBalanceHandler(t *testing.T) {
	validID := uuid.New().String()

	tests := []struct {
		name        string
		path        string
		mock        mockCheckBalance
		wantStatus  int
		wantErrCode string
	}{
		{
			name:       "success",
			path:       "/wallets/" + validID,
			mock:       mockCheckBalance{wallet: aWallet()},
			wantStatus: http.StatusOK,
		},
		{
			name:        "invalid wallet id",
			path:        "/wallets/not-a-uuid",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_WALLET_ID",
		},
		{
			name:        "wallet not found",
			path:        "/wallets/" + validID,
			mock:        mockCheckBalance{err: domain.ErrWalletNotFound},
			wantStatus:  http.StatusNotFound,
			wantErrCode: "WALLET_NOT_FOUND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newRouter(mockCreateWallet{}, mockDeposit{}, tc.mock, mockWithdraw{}, mockListTransactions{}, mockPinger{})
			rec := do(t, router, http.MethodGet, tc.path, "")
			assertStatus(t, rec, tc.wantStatus)
			if tc.wantErrCode != "" {
				assertErrorCode(t, rec, tc.wantErrCode)
			}
		})
	}
}

func TestListTransactionsHandler(t *testing.T) {
	validID := uuid.New().String()
	base := "/wallets/" + validID + "/transactions"

	tests := []struct {
		name        string
		path        string
		mock        mockListTransactions
		wantStatus  int
		wantErrCode string
	}{
		{
			name:       "success",
			path:       base,
			mock:       mockListTransactions{txs: []domain.Transaction{aTransaction()}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "success empty list",
			path:       base,
			mock:       mockListTransactions{txs: []domain.Transaction{}},
			wantStatus: http.StatusOK,
		},
		{
			name:        "invalid wallet id",
			path:        "/wallets/not-a-uuid/transactions",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_WALLET_ID",
		},
		{
			name:        "invalid limit",
			path:        base + "?limit=999",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_LIMIT",
		},
		{
			name:        "invalid cursor",
			path:        base + "?cursor=not-valid-base64!!!",
			wantStatus:  http.StatusBadRequest,
			wantErrCode: "INVALID_CURSOR",
		},
		{
			name:        "wallet not found",
			path:        base,
			mock:        mockListTransactions{err: domain.ErrWalletNotFound},
			wantStatus:  http.StatusNotFound,
			wantErrCode: "WALLET_NOT_FOUND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := newRouter(mockCreateWallet{}, mockDeposit{}, mockCheckBalance{}, mockWithdraw{}, tc.mock, mockPinger{})
			rec := do(t, router, http.MethodGet, tc.path, "")
			assertStatus(t, rec, tc.wantStatus)
			if tc.wantErrCode != "" {
				assertErrorCode(t, rec, tc.wantErrCode)
			}
		})
	}
}
