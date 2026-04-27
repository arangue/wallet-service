package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arangue/challenge-wallet/internal/application"
	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/arangue/challenge-wallet/internal/infrastructure/postgres"
)

const schema = `
CREATE TABLE IF NOT EXISTS wallets (
    id          UUID           PRIMARY KEY,
    owner_id    TEXT           NOT NULL UNIQUE,
    balance     NUMERIC(20, 8) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ    NOT NULL,
    updated_at  TIMESTAMPTZ    NOT NULL
);

CREATE TABLE IF NOT EXISTS transactions (
    id            UUID           PRIMARY KEY,
    wallet_id     UUID           NOT NULL REFERENCES wallets(id),
    type          TEXT           NOT NULL CHECK (type IN ('deposit', 'withdraw')),
    amount        NUMERIC(20, 8) NOT NULL CHECK (amount > 0),
    balance_after NUMERIC(20, 8) NOT NULL,
    reference     TEXT           NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ    NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_transactions_wallet_reference
    ON transactions(wallet_id, type, reference)
    WHERE reference <> '';
`

func newTestPool(tb testing.TB) *pgxpool.Pool {
	tb.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		tb.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		tb.Fatalf("connect: %v", err)
	}
	tb.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), schema); err != nil {
		tb.Fatalf("create schema: %v", err)
	}
	return pool
}

func truncate(tb testing.TB, pool *pgxpool.Pool) {
	tb.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE wallets CASCADE"); err != nil {
		tb.Fatalf("truncate: %v", err)
	}
}

func mustMoney(tb testing.TB, s string) domain.Money {
	tb.Helper()
	m, err := domain.NewMoney(s)
	if err != nil {
		tb.Fatalf("invalid money %q: %v", s, err)
	}
	return m
}

func seedWallet(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor, ownerID, initialBalance string) domain.Wallet {
	t.Helper()
	wallet, err := domain.NewWallet(ownerID)
	if err != nil {
		t.Fatalf("new wallet: %v", err)
	}
	if err := repo.Create(ctx, wallet); err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	if initialBalance != "" {
		depositor := application.NewDepositor(repo, transactor)
		if _, err := depositor.Execute(ctx, wallet.ID, mustMoney(t, initialBalance), "seed"); err != nil {
			t.Fatalf("seed deposit: %v", err)
		}
	}
	return wallet
}

// -- happy path tests --

func TestCreateWallet(t *testing.T) {
	tests := []struct {
		name    string
		ownerID string
		wantErr error
	}{
		{name: "success", ownerID: "owner-create-1"},
		{name: "duplicate owner", ownerID: "owner-create-dup", wantErr: domain.ErrWalletExists},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncate(t, pool)
			ctx := context.Background()
			repo := postgres.NewWalletRepository(pool)

			wallet, err := domain.NewWallet(tc.ownerID)
			if err != nil {
				t.Fatalf("new wallet: %v", err)
			}
			if err := repo.Create(ctx, wallet); err != nil {
				t.Fatalf("first create: %v", err)
			}

			if tc.wantErr != nil {
				dup, _ := domain.NewWallet(tc.ownerID)
				err := repo.Create(ctx, dup)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}

			got, err := repo.FindByID(ctx, wallet.ID)
			if err != nil {
				t.Fatalf("find: %v", err)
			}
			if got.OwnerID != tc.ownerID {
				t.Fatalf("expected ownerID %q, got %q", tc.ownerID, got.OwnerID)
			}
			if !got.Balance.IsZero() {
				t.Fatalf("expected zero balance, got %s", got.Balance.String())
			}
		})
	}
}

func TestDeposit(t *testing.T) {
	tests := []struct {
		name        string
		seed        string
		amount      string
		wantBalance string
		wantErr     error
	}{
		{name: "success", seed: "", amount: "100.00", wantBalance: "100"},
		{name: "accumulates", seed: "50.00", amount: "75.00", wantBalance: "125"},
		{name: "wallet not found", amount: "10.00", wantErr: domain.ErrWalletNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncate(t, pool)
			ctx := context.Background()
			repo := postgres.NewWalletRepository(pool)
			transactor := postgres.NewTransactor(pool)
			depositor := application.NewDepositor(repo, transactor)

			wallet := seedWallet(t, ctx, repo, transactor, "owner-deposit-"+tc.name, tc.seed)

			walletID := wallet.ID
			if errors.Is(tc.wantErr, domain.ErrWalletNotFound) {
				walletID = [16]byte{} // non-existent
			}

			tx, err := depositor.Execute(ctx, walletID, mustMoney(t, tc.amount), "ref-1")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if err != nil {
				return
			}

			if tx.Type != domain.TransactionTypeDeposit {
				t.Fatalf("expected deposit type, got %s", tx.Type)
			}
			if !tx.Amount.Equal(mustMoney(t, tc.amount)) {
				t.Fatalf("expected amount %s, got %s", tc.amount, tx.Amount.String())
			}

			got, err := repo.FindByID(ctx, wallet.ID)
			if err != nil {
				t.Fatalf("find wallet: %v", err)
			}
			if !got.Balance.Equal(mustMoney(t, tc.wantBalance)) {
				t.Fatalf("expected balance %s, got %s", tc.wantBalance, got.Balance.String())
			}
		})
	}
}

func TestWithdraw(t *testing.T) {
	tests := []struct {
		name        string
		seed        string
		amount      string
		wantBalance string
		wantErr     error
	}{
		{name: "success", seed: "100.00", amount: "40.00", wantBalance: "60"},
		{name: "exact balance", seed: "100.00", amount: "100.00", wantBalance: "0"},
		{name: "insufficient funds", seed: "50.00", amount: "100.00", wantBalance: "50", wantErr: domain.ErrInsufficientFunds},
		{name: "wallet not found", seed: "", amount: "10.00", wantErr: domain.ErrWalletNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncate(t, pool)
			ctx := context.Background()
			repo := postgres.NewWalletRepository(pool)
			transactor := postgres.NewTransactor(pool)
			withdrawer := application.NewWithdrawer(repo, transactor)

			wallet := seedWallet(t, ctx, repo, transactor, "owner-withdraw-"+tc.name, tc.seed)

			walletID := wallet.ID
			if errors.Is(tc.wantErr, domain.ErrWalletNotFound) {
				walletID = [16]byte{}
			}

			tx, err := withdrawer.Execute(ctx, walletID, mustMoney(t, tc.amount), "ref-1")

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if err != nil {
				if tc.wantBalance != "" {
					got, err := repo.FindByID(ctx, wallet.ID)
					if err != nil {
						t.Fatalf("find wallet: %v", err)
					}
					if !got.Balance.Equal(mustMoney(t, tc.wantBalance)) {
						t.Fatalf("balance mutated after failed withdraw: expected %s, got %s", tc.wantBalance, got.Balance.String())
					}
				}
				return
			}

			if tx.Type != domain.TransactionTypeWithdraw {
				t.Fatalf("expected withdraw type, got %s", tx.Type)
			}
			got, err := repo.FindByID(ctx, wallet.ID)
			if err != nil {
				t.Fatalf("find wallet: %v", err)
			}
			if !got.Balance.Equal(mustMoney(t, tc.wantBalance)) {
				t.Fatalf("expected balance %s, got %s", tc.wantBalance, got.Balance.String())
			}
		})
	}
}

func TestListTransactions(t *testing.T) {
	tests := []struct {
		name      string
		deposits  int
		limit     int
		useCursor bool
		wantCount int
		wantErr   error
	}{
		{name: "returns all transactions", deposits: 3, limit: 10, wantCount: 3},
		{name: "limit is respected", deposits: 5, limit: 2, wantCount: 2},
		{name: "cursor filters older transactions", deposits: 3, limit: 2, useCursor: true, wantCount: 1},
		{name: "wallet not found", wantErr: domain.ErrWalletNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncate(t, pool)
			ctx := context.Background()
			repo := postgres.NewWalletRepository(pool)
			transactor := postgres.NewTransactor(pool)
			lister := application.NewLister(repo)
			depositor := application.NewDepositor(repo, transactor)

			wallet := seedWallet(t, ctx, repo, transactor, "owner-list-"+tc.name, "")

			for i := range tc.deposits {
				amount := mustMoney(t, "10.00")
				if _, err := depositor.Execute(ctx, wallet.ID, amount, fmt.Sprintf("ref-%d", i)); err != nil {
					t.Fatalf("deposit %d: %v", i, err)
				}
				time.Sleep(2 * time.Millisecond)
			}

			walletID := wallet.ID
			if errors.Is(tc.wantErr, domain.ErrWalletNotFound) {
				walletID = [16]byte{}
			}

			var cursor *domain.Cursor
			if tc.useCursor {
				firstPage, err := lister.Execute(ctx, walletID, tc.limit, nil)
				if err != nil {
					t.Fatalf("first page: %v", err)
				}
				last := firstPage[len(firstPage)-1]
				cursor = &domain.Cursor{Time: last.CreatedAt, ID: last.ID}
			}

			txs, err := lister.Execute(ctx, walletID, tc.limit, cursor)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if err != nil {
				return
			}
			if len(txs) != tc.wantCount {
				t.Fatalf("expected %d transactions, got %d", tc.wantCount, len(txs))
			}
		})
	}
}

// -- reference uniqueness tests --

func TestReferenceUniqueness(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor)
	}{
		{
			name: "empty reference allows multiple transactions on same wallet",
			run: func(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor) {
				wallet := seedWallet(t, ctx, repo, transactor, "owner-empty-ref", "")
				depositor := application.NewDepositor(repo, transactor)
				amount := mustMoney(t, "10.00")
				for i := range 3 {
					if _, err := depositor.Execute(ctx, wallet.ID, amount, ""); err != nil {
						t.Fatalf("deposit %d with empty reference: %v", i, err)
					}
				}
			},
		},
		{
			name: "duplicate non-empty reference on same wallet is rejected",
			run: func(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor) {
				wallet := seedWallet(t, ctx, repo, transactor, "owner-dup-ref", "")
				depositor := application.NewDepositor(repo, transactor)
				amount := mustMoney(t, "10.00")
				if _, err := depositor.Execute(ctx, wallet.ID, amount, "ref-abc"); err != nil {
					t.Fatalf("first deposit: %v", err)
				}
				_, err := depositor.Execute(ctx, wallet.ID, amount, "ref-abc")
				if !errors.Is(err, domain.ErrTransactionExists) {
					t.Fatalf("expected ErrTransactionExists, got %v", err)
				}
			},
		},
		{
			name: "same reference on different wallets succeeds",
			run: func(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor) {
				walletA := seedWallet(t, ctx, repo, transactor, "owner-wallet-a", "")
				walletB := seedWallet(t, ctx, repo, transactor, "owner-wallet-b", "")
				depositor := application.NewDepositor(repo, transactor)
				amount := mustMoney(t, "10.00")
				if _, err := depositor.Execute(ctx, walletA.ID, amount, "shared-ref"); err != nil {
					t.Fatalf("deposit wallet A: %v", err)
				}
				if _, err := depositor.Execute(ctx, walletB.ID, amount, "shared-ref"); err != nil {
					t.Fatalf("deposit wallet B with same reference: %v", err)
				}
			},
		},
		{
			name: "same reference on deposit and withdrawal succeeds",
			run: func(t *testing.T, ctx context.Context, repo *postgres.WalletRepository, transactor domain.Transactor) {
				wallet := seedWallet(t, ctx, repo, transactor, "owner-cross-type-ref", "100.00")
				depositor := application.NewDepositor(repo, transactor)
				withdrawer := application.NewWithdrawer(repo, transactor)
				amount := mustMoney(t, "10.00")
				if _, err := depositor.Execute(ctx, wallet.ID, amount, "ref-cross"); err != nil {
					t.Fatalf("deposit with ref-cross: %v", err)
				}
				if _, err := withdrawer.Execute(ctx, wallet.ID, amount, "ref-cross"); err != nil {
					t.Fatalf("withdraw with same ref-cross should succeed: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newTestPool(t)
			truncate(t, pool)
			repo := postgres.NewWalletRepository(pool)
			transactor := postgres.NewTransactor(pool)
			tc.run(t, context.Background(), repo, transactor)
		})
	}
}

// -- concurrency test --

func TestConcurrency_DoubleSpend(t *testing.T) {
	pool := newTestPool(t)
	truncate(t, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)

	wallet := seedWallet(t, ctx, repo, transactor, "owner-concurrency", "100.00")
	withdrawer := application.NewWithdrawer(repo, transactor)
	amount := mustMoney(t, "100.00")

	const goroutines = 10
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := withdrawer.Execute(ctx, wallet.ID, amount, fmt.Sprintf("withdraw-%d", i))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if errors.Is(err, domain.ErrInsufficientFunds) {
				failures++
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 successful withdrawal, got %d", successes)
	}
	if failures != goroutines-1 {
		t.Errorf("expected %d failed withdrawals, got %d", goroutines-1, failures)
	}
}

func TestConcurrency_ConcurrentDeposits(t *testing.T) {
	pool := newTestPool(t)
	truncate(t, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)

	wallet := seedWallet(t, ctx, repo, transactor, "owner-concurrent-deposits", "")
	depositor := application.NewDepositor(repo, transactor)

	const goroutines = 10
	amount := mustMoney(t, "10.00")

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			if _, err := depositor.Execute(ctx, wallet.ID, amount, fmt.Sprintf("dep-%d", i)); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()

	got, err := repo.FindByID(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}
	want := mustMoney(t, "100.00")
	if !got.Balance.Equal(want) {
		t.Errorf("expected balance %s after %d concurrent deposits, got %s", want, goroutines, got.Balance)
	}
}

func TestConcurrency_IdempotentDeposit(t *testing.T) {
	pool := newTestPool(t)
	truncate(t, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)

	wallet := seedWallet(t, ctx, repo, transactor, "owner-idempotent", "")
	depositor := application.NewDepositor(repo, transactor)
	amount := mustMoney(t, "50.00")

	const goroutines = 10
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		successes  int
		duplicates int
	)

	for range goroutines {
		wg.Go(func() {
			_, err := depositor.Execute(ctx, wallet.ID, amount, "idempotent-ref")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if errors.Is(err, domain.ErrTransactionExists) {
				duplicates++
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 successful deposit, got %d", successes)
	}
	if duplicates != goroutines-1 {
		t.Errorf("expected %d duplicate rejections, got %d", goroutines-1, duplicates)
	}

	got, err := repo.FindByID(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}
	if !got.Balance.Equal(amount) {
		t.Errorf("expected balance %s (credited once), got %s", amount, got.Balance)
	}
}
