package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/google/uuid"
)

func TestNewLister(t *testing.T) {
	type args struct {
		repo domain.WalletRepository
	}
	tests := []struct {
		name string
		args args
		want Lister
	}{
		{
			name: "returns a new lister with the given repository",
			args: args{repo: &mockWalletRepo{}},
			want: &lister{repo: &mockWalletRepo{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLister(tt.args.repo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLister() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_lister_Execute(t *testing.T) {
	walletID := uuid.New()
	someTx := domain.Transaction{ID: uuid.New(), WalletID: walletID}
	type fields struct {
		repo domain.WalletRepository
	}
	type args struct {
		ctx      context.Context
		walletID uuid.UUID
		limit    int
		cursor   *domain.Cursor
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []domain.Transaction
		wantErr bool
		errIs   error
	}{
		{
			name: "success: returns transactions for existing wallet",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id}, nil
					},
					listTransactionsFn: func(_ context.Context, _ uuid.UUID, _ int, _ *domain.Cursor) ([]domain.Transaction, error) {
						return []domain.Transaction{someTx}, nil
					},
				},
			},
			args: args{ctx: context.Background(), walletID: walletID, limit: 10},
			want: []domain.Transaction{someTx},
		},
		{
			name: "success: empty result when wallet has no transactions",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id}, nil
					},
					listTransactionsFn: func(_ context.Context, _ uuid.UUID, _ int, _ *domain.Cursor) ([]domain.Transaction, error) {
						return nil, nil
					},
				},
			},
			args: args{ctx: context.Background(), walletID: walletID, limit: 10},
			want: nil,
		},
		{
			name: "success: cursor is forwarded to repository",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id}, nil
					},
					listTransactionsFn: func(_ context.Context, _ uuid.UUID, _ int, c *domain.Cursor) ([]domain.Transaction, error) {
						if c == nil {
							t.Error("expected cursor to be forwarded to ListTransactions")
						}
						return []domain.Transaction{someTx}, nil
					},
				},
			},
			args: args{ctx: context.Background(), walletID: walletID, limit: 10, cursor: &domain.Cursor{Time: time.Now().UTC(), ID: uuid.New()}},
			want: []domain.Transaction{someTx},
		},
		{
			name: "fail: wallet not found",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, _ uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{}, domain.ErrWalletNotFound
					},
				},
			},
			args:    args{ctx: context.Background(), walletID: walletID, limit: 10},
			wantErr: true,
			errIs:   domain.ErrWalletNotFound,
		},
		{
			name: "fail: repository error on list",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id}, nil
					},
					listTransactionsFn: func(_ context.Context, _ uuid.UUID, _ int, _ *domain.Cursor) ([]domain.Transaction, error) {
						return nil, errors.New("db error")
					},
				},
			},
			args:    args{ctx: context.Background(), walletID: walletID, limit: 10},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lister{repo: tt.fields.repo}
			got, err := l.Execute(tt.args.ctx, tt.args.walletID, tt.args.limit, tt.args.cursor)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("Execute() error = %v, wantErrIs %v", err, tt.errIs)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Execute() got = %v, want %v", got, tt.want)
			}
		})
	}
}
