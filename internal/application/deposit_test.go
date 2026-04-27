package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

func Test_depositor_Execute(t *testing.T) {
	mustMoney := func(s string) domain.Money {
		m, _ := domain.NewMoney(s)
		return m
	}

	walletID := uuid.New()
	amount := mustMoney("100.00")

	type fields struct {
		repo       domain.WalletRepository
		transactor domain.Transactor
	}

	type args struct {
		ctx       context.Context
		walletID  uuid.UUID
		amount    domain.Money
		reference string
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		errIs   error
	}{
		{
			name: "Success: should deposit funds into an existing wallet",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: domain.Zero()}, nil
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-123"},
			wantErr: false,
		},
		{
			name: "Fail: should return error when wallet ID is not found",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{}, domain.ErrWalletNotFound
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-123"},
			wantErr: true,
			errIs:   domain.ErrWalletNotFound,
		},
		{
			name: "Fail: should reject non-positive amounts (domain error check)",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: domain.Zero()}, nil
					},
					saveFn: func(ctx context.Context, w domain.Wallet) error {
						t.Error("Save should NOT be called if domain logic fails")
						return nil
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: mustMoney("0"), reference: "ref-123"},
			wantErr: true,
			errIs:   domain.ErrNonPositiveAmount,
		},
		{
			name: "Fail: should return error for duplicate transaction reference (idempotency)",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: domain.Zero()}, nil
					},
					createTransactionFn: func(ctx context.Context, tx domain.Transaction) error {
						return domain.ErrTransactionExists
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "duplicate-ref"},
			wantErr: true,
			errIs:   domain.ErrTransactionExists,
		},
		{
			name: "Fail: should return error when save fails after credit",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: domain.Zero()}, nil
					},
					saveFn: func(ctx context.Context, w domain.Wallet) error {
						return errors.New("db write error")
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-123"},
			wantErr: true,
		},
		{
			name: "Fail: should return error when database transaction fails to start",
			fields: fields{
				repo: &mockRepo{},
				transactor: &mockTransactor{
					runInTxFn: func(ctx context.Context, fn func(ctx context.Context) error) error {
						return errors.New("transaction start failed")
					},
				},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-123"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := depositor{repo: tt.fields.repo, transactor: tt.fields.transactor}
			got, err := d.Execute(tt.args.ctx, tt.args.walletID, tt.args.amount, tt.args.reference)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("Execute() error = %v, wantErrIs %v", err, tt.errIs)
			}
			if !tt.wantErr {
				if got.WalletID != tt.args.walletID {
					t.Errorf("Execute() got.WalletID = %v, want %v", got.WalletID, tt.args.walletID)
				}
				if !got.Amount.Equal(tt.args.amount) {
					t.Errorf("Execute() got.Amount = %v, want %v", got.Amount, tt.args.amount)
				}
				if got.Reference != tt.args.reference {
					t.Errorf("Execute() got.Reference = %v, want %v", got.Reference, tt.args.reference)
				}
				if got.Type != domain.TransactionTypeDeposit {
					t.Errorf("Execute() got.Type = %v, want %v", got.Type, domain.TransactionTypeDeposit)
				}
			}
		})
	}
}
