package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/arangue/challenge-wallet/internal/domain"
	"github.com/google/uuid"
)

func TestNewChecker(t *testing.T) {
	type args struct {
		repo domain.WalletRepository
	}
	tests := []struct {
		name string
		args args
		want Checker
	}{
		{
			name: "should return a new Checker instance with the provided repository",
			args: args{repo: &mockRepo{}},
			want: &checker{repo: &mockRepo{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewChecker(tt.args.repo); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewChecker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_checker_Execute(t *testing.T) {
	mustMoney := func(s string) domain.Money {
		m, _ := domain.NewMoney(s)
		return m
	}

	walletID := uuid.New()

	type fields struct {
		repo domain.WalletRepository
	}
	type args struct {
		ctx      context.Context
		walletID uuid.UUID
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantID  uuid.UUID
		wantBal domain.Money
		wantErr bool
		errIs   error
	}{
		{
			name: "success: returns wallet for valid ID",
			fields: fields{
				repo: &mockRepo{
					findByIDFn: func(_ context.Context, id uuid.UUID) (domain.Wallet, error) {
						return domain.Wallet{ID: id, Balance: mustMoney("100.00")}, nil
					},
				},
			},
			args:    args{ctx: context.Background(), walletID: walletID},
			wantID:  walletID,
			wantBal: mustMoney("100.00"),
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
			args:    args{ctx: context.Background(), walletID: walletID},
			wantErr: true,
			errIs:   domain.ErrWalletNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := checker{repo: tt.fields.repo}
			got, err := c.Execute(tt.args.ctx, tt.args.walletID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("Execute() error = %v, wantErrIs %v", err, tt.errIs)
			}
			if !tt.wantErr {
				if got.ID != tt.wantID {
					t.Errorf("Execute() got.ID = %v, want %v", got.ID, tt.wantID)
				}
				if !got.Balance.Equal(tt.wantBal) {
					t.Errorf("Execute() got.Balance = %v, want %v", got.Balance, tt.wantBal)
				}
			}
		})
	}
}
