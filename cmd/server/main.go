package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arangue/challenge-wallet/internal/application"
	httpdelivery "github.com/arangue/challenge-wallet/internal/delivery/http"
	"github.com/arangue/challenge-wallet/internal/infrastructure/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL is not set")
		os.Exit(1)
	}

	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		slog.Error("failed to parse database URL", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("database is unreachable", "error", err)
		os.Exit(1)
	}

	if err := runMigrations(dbURL); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	repo := postgres.NewWalletRepository(pool)
	transactor := postgres.NewTransactor(pool)

	createWallet := application.NewWalletCreator(repo)
	deposit := application.NewDepositor(repo, transactor)
	checkBalance := application.NewChecker(repo)
	withdraw := application.NewWithdrawer(repo, transactor)
	listTransactions := application.NewLister(repo)

	handler := httpdelivery.NewHandler(createWallet, deposit, checkBalance, withdraw, listTransactions, pool)
	router := httpdelivery.NewRouter(handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server started", "port", port)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func runMigrations(dbURL string) error {
	src, err := iofs.New(postgres.Migrations, "migrations")
	if err != nil {
		return err
	}
	migrateURL := strings.Replace(dbURL, "postgres://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	slog.Info("migrations applied")
	return nil
}
