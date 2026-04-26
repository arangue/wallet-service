package domain_test

import (
	"errors"
	"testing"

	"github.com/arangue/challenge-wallet/internal/domain"
)

func TestNewWallet(t *testing.T) {
	tests := []struct {
		name    string
		ownerID string
		wantErr error
	}{
		{name: "success", ownerID: "owner-1"},
		{name: "empty owner id", ownerID: "", wantErr: domain.ErrInvalidOwnerID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, err := domain.NewWallet(tc.ownerID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if err == nil {
				if w.OwnerID != tc.ownerID {
					t.Fatalf("expected ownerID %q, got %q", tc.ownerID, w.OwnerID)
				}
				if !w.Balance.IsZero() {
					t.Fatalf("expected zero balance, got %s", w.Balance.String())
				}
			}
		})
	}
}

func TestWallet_Credit(t *testing.T) {
	tests := []struct {
		name    string
		amount  string
		wantBal string
		wantErr error
	}{
		{name: "success", amount: "100.00", wantBal: "100"},
		{name: "zero amount", amount: "0", wantErr: domain.ErrNonPositiveAmount, wantBal: "0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := domain.NewWallet("owner-1")
			amount, _ := domain.NewMoney(tc.amount)

			err := w.Credit(amount)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			want, _ := domain.NewMoney(tc.wantBal)
			if !w.Balance.Equal(want) {
				t.Fatalf("expected balance %s, got %s", tc.wantBal, w.Balance.String())
			}
		})
	}
}

func TestWallet_Debit(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		amount  string
		wantBal string
		wantErr error
	}{
		{name: "success", initial: "100.00", amount: "40.00", wantBal: "60"},
		{name: "exact amount", initial: "100.00", amount: "100.00", wantBal: "0"},
		{name: "insufficient funds", initial: "50.00", amount: "100.00", wantBal: "50", wantErr: domain.ErrInsufficientFunds},
		{name: "zero amount", initial: "100.00", amount: "0", wantBal: "100", wantErr: domain.ErrNonPositiveAmount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := domain.NewWallet("owner-1")
			initial, _ := domain.NewMoney(tc.initial)
			_ = w.Credit(initial)

			amount, _ := domain.NewMoney(tc.amount)
			err := w.Debit(amount)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			want, _ := domain.NewMoney(tc.wantBal)
			if !w.Balance.Equal(want) {
				t.Fatalf("expected balance %s, got %s", tc.wantBal, w.Balance.String())
			}
		})
	}
}
