package domain

import "github.com/shopspring/decimal"

type Money struct {
	amount decimal.Decimal
}

var maxMoneyDecimal, _ = decimal.NewFromString("999999999999.99999999")
var MaxMoney = Money{amount: maxMoneyDecimal}

func NewMoney(s string) (Money, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Money{}, ErrInvalidAmount
	}
	if d.IsNegative() {
		return Money{}, ErrNegativeAmount
	}
	if d.Exponent() < -8 {
		return Money{}, ErrInvalidAmount
	}
	if d.GreaterThan(maxMoneyDecimal) {
		return Money{}, ErrInvalidAmount
	}
	return Money{amount: d}, nil
}

func NewMoneyFromDecimal(d decimal.Decimal) (Money, error) {
	if d.IsNegative() {
		return Money{}, ErrNegativeAmount
	}
	return Money{amount: d}, nil
}

func Zero() Money {
	return Money{amount: decimal.Zero}
}

func (m Money) Add(other Money) Money {
	return Money{amount: m.amount.Add(other.amount)}
}

func (m Money) Sub(other Money) (Money, error) {
	result := m.amount.Sub(other.amount)
	if result.IsNegative() {
		return Money{}, ErrInsufficientFunds
	}
	return Money{amount: result}, nil
}

func (m Money) IsZero() bool {
	return m.amount.IsZero()
}

func (m Money) IsPositive() bool {
	return m.amount.IsPositive()
}

func (m Money) Equal(other Money) bool {
	return m.amount.Equal(other.amount)
}

func (m Money) GreaterThan(other Money) bool {
	return m.amount.GreaterThan(other.amount)
}

func (m Money) GreaterThanOrEqual(other Money) bool {
	return m.amount.GreaterThanOrEqual(other.amount)
}

func (m Money) Decimal() decimal.Decimal {
	return m.amount
}

func (m Money) String() string {
	return m.amount.String()
}
