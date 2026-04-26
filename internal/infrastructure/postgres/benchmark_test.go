package postgres_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/arangue/challenge-wallet/internal/application"
	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/arangue/challenge-wallet/internal/infrastructure/postgres"
)

// BenchmarkDeposit measures the round-trip latency of a single deposit,
// including the SELECT FOR UPDATE, balance update, and transaction insert.
func BenchmarkDeposit(b *testing.B) {
	pool := newTestPool(b)
	truncate(b, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)
	depositor := application.NewDepositor(repo, transactor)

	wallet, err := domain.NewWallet("bench-deposit-owner")
	if err != nil {
		b.Fatalf("new wallet: %v", err)
	}
	if err := repo.Create(ctx, wallet); err != nil {
		b.Fatalf("create wallet: %v", err)
	}

	amount := mustMoney(b, "1.00")

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		if _, err := depositor.Execute(ctx, wallet.ID, amount, fmt.Sprintf("bench-dep-%d", i)); err != nil {
			b.Fatalf("deposit: %v", err)
		}
	}
}

// BenchmarkWithdraw measures withdraw latency on a pre-funded wallet.
// The wallet is seeded with enough balance for all b.N iterations up-front
// so the benchmark body never hits ErrInsufficientFunds.
func BenchmarkWithdraw(b *testing.B) {
	pool := newTestPool(b)
	truncate(b, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)
	depositor := application.NewDepositor(repo, transactor)
	withdrawer := application.NewWithdrawer(repo, transactor)

	wallet, err := domain.NewWallet("bench-withdraw-owner")
	if err != nil {
		b.Fatalf("new wallet: %v", err)
	}
	if err := repo.Create(ctx, wallet); err != nil {
		b.Fatalf("create wallet: %v", err)
	}

	// Fund the wallet with enough balance for all iterations.
	seed := mustMoney(b, fmt.Sprintf("%d.00", b.N))
	if _, err := depositor.Execute(ctx, wallet.ID, seed, "bench-seed"); err != nil {
		b.Fatalf("seed deposit: %v", err)
	}

	amount := mustMoney(b, "1.00")

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		if _, err := withdrawer.Execute(ctx, wallet.ID, amount, fmt.Sprintf("bench-wd-%d", i)); err != nil {
			b.Fatalf("withdraw: %v", err)
		}
	}
}

// BenchmarkConcurrentDeposit measures throughput under concurrent writers on
// the same wallet, which exercises the SELECT FOR UPDATE contention path.
func BenchmarkConcurrentDeposit(b *testing.B) {
	pool := newTestPool(b)
	truncate(b, pool)

	ctx := context.Background()
	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)
	depositor := application.NewDepositor(repo, transactor)

	wallet, err := domain.NewWallet("bench-concurrent-owner")
	if err != nil {
		b.Fatalf("new wallet: %v", err)
	}
	if err := repo.Create(ctx, wallet); err != nil {
		b.Fatalf("create wallet: %v", err)
	}

	amount := mustMoney(b, "1.00")

	var seq atomic.Int64

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ref := fmt.Sprintf("bench-par-%d", seq.Add(1))
			if _, err := depositor.Execute(ctx, wallet.ID, amount, ref); err != nil {
				b.Errorf("concurrent deposit: %v", err)
			}
		}
	})
}
