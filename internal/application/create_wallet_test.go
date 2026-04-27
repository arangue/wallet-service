package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/arangue/challenge-wallet/internal/domain"
)

// mockWalletRepo aliases mockRepo so auto-generated tests referencing it compile.
type mockWalletRepo = mockRepo

type mockRepo struct {
	domain.WalletRepository
	createFn            func(ctx context.Context, w domain.Wallet) error
	findByIDFn          func(ctx context.Context, id uuid.UUID) (domain.Wallet, error)
	findByIDForUpdateFn func(ctx context.Context, id uuid.UUID) (domain.Wallet, error)
	saveFn              func(ctx context.Context, w domain.Wallet) error
	createTransactionFn func(ctx context.Context, tx domain.Transaction) error
	listTransactionsFn  func(ctx context.Context, id uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error)
}

func (m *mockRepo) Create(ctx context.Context, w domain.Wallet) error {
	if m.createFn != nil {
		return m.createFn(ctx, w)
	}
	return nil
}

func (m *mockRepo) FindByID(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return domain.Wallet{}, nil
}

func (m *mockRepo) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (domain.Wallet, error) {
	if m.findByIDForUpdateFn != nil {
		return m.findByIDForUpdateFn(ctx, id)
	}
	return domain.Wallet{}, nil
}

func (m *mockRepo) Save(ctx context.Context, w domain.Wallet) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, w)
	}
	return nil
}

func (m *mockRepo) CreateTransaction(ctx context.Context, tx domain.Transaction) error {
	if m.createTransactionFn != nil {
		return m.createTransactionFn(ctx, tx)
	}
	return nil
}

func (m *mockRepo) ListTransactions(ctx context.Context, id uuid.UUID, limit int, cursor *domain.Cursor) ([]domain.Transaction, error) {
	if m.listTransactionsFn != nil {
		return m.listTransactionsFn(ctx, id, limit, cursor)
	}
	return nil, nil
}

type mockTransactor struct {
	runInTxFn func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockTransactor) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if m.runInTxFn != nil {
		return m.runInTxFn(ctx, fn)
	}
	return fn(ctx)
}

func Test_walletCreator_Execute(t *testing.T) {
	type fields struct {
		repo domain.WalletRepository
	}
	type args struct {
		ctx     context.Context
		ownerID string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		errIs   error
	}{
		{
			name: "success: creates and returns wallet for valid owner",
			fields: fields{
				repo: &mockRepo{},
			},
			args:    args{ctx: context.Background(), ownerID: "owner-1"},
			wantErr: false,
		},
		{
			name: "fail: empty owner ID rejected before hitting repo",
			fields: fields{
				repo: &mockRepo{
					createFn: func(ctx context.Context, w domain.Wallet) error {
						t.Error("Create should NOT be called for invalid domain object")
						return nil
					},
				},
			},
			args:    args{ctx: context.Background(), ownerID: ""},
			wantErr: true,
			errIs:   domain.ErrInvalidOwnerID,
		},
		{
			name: "fail: repo returns wallet already exists",
			fields: fields{
				repo: &mockRepo{
					createFn: func(ctx context.Context, w domain.Wallet) error {
						return domain.ErrWalletExists
					},
				},
			},
			args:    args{ctx: context.Background(), ownerID: "owner-dup"},
			wantErr: true,
			errIs:   domain.ErrWalletExists,
		},
		{
			name: "fail: repo returns generic error",
			fields: fields{
				repo: &mockRepo{
					createFn: func(ctx context.Context, w domain.Wallet) error {
						return errors.New("db unavailable")
					},
				},
			},
			args:    args{ctx: context.Background(), ownerID: "owner-1"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wc := walletCreator{repo: tt.fields.repo}
			got, err := wc.Execute(tt.args.ctx, tt.args.ownerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Errorf("Execute() error = %v, wantErrIs %v", err, tt.errIs)
			}
			if !tt.wantErr {
				if got.OwnerID != tt.args.ownerID {
					t.Errorf("Execute() got.OwnerID = %v, want %v", got.OwnerID, tt.args.ownerID)
				}
				if got.ID == (uuid.UUID{}) {
					t.Error("Execute() got.ID should not be zero")
				}
				if !got.Balance.IsZero() {
					t.Errorf("Execute() got.Balance = %v, want zero", got.Balance)
				}
			}
		})
	}
}
