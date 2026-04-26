package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/google/uuid"
)

func TestNewWithdrawer(t *testing.T) {
	type args struct {
		repo       domain.WalletRepository
		transactor domain.Transactor
	}
	tests := []struct {
		name string
		args args
		want Withdrawer
	}{
		{
			name: "returns a new withdrawer instance",
			args: args{
				repo:       &mockRepo{},
				transactor: &mockTransactor{},
			},
			want: &withdrawer{
				repo:       &mockRepo{},
				transactor: &mockTransactor{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewWithdrawer(tt.args.repo, tt.args.transactor); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWithdrawer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_withdrawer_Execute(t *testing.T) {
	mustMoney := func(s string) domain.Money {
		m, _ := domain.NewMoney(s)
		return m
	}

	walletID := uuid.New()
	amount := mustMoney("50.00")

	fundedWallet := func(id uuid.UUID) domain.Wallet {
		w := domain.Wallet{ID: id, Balance: mustMoney("100.00")}
		return w
	}

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
			name: "success: withdraws from a funded wallet",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return fundedWallet(id), nil
					},
				},
				transactor: &mockTransactor{},
			},
			args: args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-1"},
		},
		{
			name: "fail: non-positive amount rejected before hitting repo",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (domain.Wallet, error) {
						t.Error("FindByIDForUpdate should NOT be called for non-positive amount")
						return domain.Wallet{}, nil
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: mustMoney("0"), reference: "ref-1"},
			wantErr: true,
			errIs:   domain.ErrNonPositiveAmount,
		},
		{
			name: "fail: wallet not found",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{}, domain.ErrWalletNotFound
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-1"},
			wantErr: true,
			errIs:   domain.ErrWalletNotFound,
		},
		{
			name: "fail: insufficient funds",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: mustMoney("10.00")}, nil
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: mustMoney("50.00"), reference: "ref-1"},
			wantErr: true,
			errIs:   domain.ErrInsufficientFunds,
		},
		{
			name: "fail: save fails after debit",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return fundedWallet(id), nil
					},
					saveFn: func(_ context.Context, _ domain.Wallet) error {
						return errors.New("db write error")
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-1"},
			wantErr: true,
		},
		{
			name: "fail: duplicate transaction reference",
			fields: fields{
				repo: &mockRepo{
					findByIDForUpdateFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return fundedWallet(id), nil
					},
					createTransactionFn: func(_ context.Context, _ domain.Transaction) error {
						return domain.ErrTransactionExists
					},
				},
				transactor: &mockTransactor{},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "dup-ref"},
			wantErr: true,
			errIs:   domain.ErrTransactionExists,
		},
		{
			name: "fail: transaction fails to start",
			fields: fields{
				repo: &mockRepo{},
				transactor: &mockTransactor{
					runInTxFn: func(_ context.Context, _ func(context.Context) error) error {
						return errors.New("tx start failed")
					},
				},
			},
			args:    args{ctx: context.Background(), walletID: walletID, amount: amount, reference: "ref-1"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := withdrawer{repo: tt.fields.repo, transactor: tt.fields.transactor}
			got, err := w.Execute(tt.args.ctx, tt.args.walletID, tt.args.amount, tt.args.reference)
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
				if got.Type != domain.TransactionTypeWithdraw {
					t.Errorf("Execute() got.Type = %v, want %v", got.Type, domain.TransactionTypeWithdraw)
				}
			}
		})
	}
}
