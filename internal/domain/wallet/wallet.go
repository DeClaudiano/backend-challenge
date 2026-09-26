package wallet

import (
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
)

type Wallet struct {
	id, playerID         string
	currency             money.Currency
	balance              money.Money
	version              int64
	createdAt, updatedAt time.Time
}
type Change struct {
	Direction     wagering.Direction
	Before, After money.Money
}

func New(id, playerID string, currency money.Currency, initial money.Money, now time.Time) (Wallet, error) {
	if id == "" || playerID == "" || now.IsZero() {
		return Wallet{}, domain.NewError(domain.ErrInvalidValue, "wallet identity and timestamp are required")
	}
	if !currency.Valid() {
		return Wallet{}, domain.NewError(domain.ErrInvalidCurrency, "invalid wallet currency")
	}
	if initial.Currency() != currency {
		return Wallet{}, domain.NewError(domain.ErrCurrencyMismatch, "initial balance currency differs")
	}
	if initial.IsNegative() {
		return Wallet{}, domain.NewError(domain.ErrNegativeAmount, "initial balance cannot be negative")
	}
	return Wallet{id: id, playerID: playerID, currency: currency, balance: initial, version: 1, createdAt: now, updatedAt: now}, nil
}
func Rehydrate(id, playerID string, currency money.Currency, balance money.Money, version int64, createdAt, updatedAt time.Time) (Wallet, error) {
	if id == "" || playerID == "" || version < 1 || createdAt.IsZero() || updatedAt.IsZero() {
		return Wallet{}, domain.NewError(domain.ErrInvalidValue, "invalid wallet state")
	}
	if !currency.Valid() || balance.Currency() != currency || balance.IsNegative() {
		return Wallet{}, domain.NewError(domain.ErrInvalidValue, "invalid wallet balance state")
	}
	return Wallet{id: id, playerID: playerID, currency: currency, balance: balance, version: version, createdAt: createdAt, updatedAt: updatedAt}, nil
}
func (w Wallet) ID() string               { return w.id }
func (w Wallet) PlayerID() string         { return w.playerID }
func (w Wallet) Currency() money.Currency { return w.currency }
func (w Wallet) Balance() money.Money     { return w.balance }
func (w Wallet) Version() int64           { return w.version }
func (w Wallet) CreatedAt() time.Time     { return w.createdAt }
func (w Wallet) UpdatedAt() time.Time     { return w.updatedAt }
func (w *Wallet) Debit(value money.Money, now time.Time) (Change, error) {
	if err := w.validateMovement(value, now); err != nil {
		return Change{}, err
	}
	if value.IsZero() || value.IsNegative() {
		return Change{}, domain.NewError(domain.ErrNonPositiveAmount, "debit must be positive")
	}
	if cmp, _ := w.balance.Compare(value); cmp < 0 {
		return Change{}, domain.NewError(domain.ErrInsufficientBalance, "wallet balance is insufficient")
	}
	after, err := w.balance.Sub(value)
	if err != nil {
		return Change{}, err
	}
	before := w.balance
	w.balance = after
	w.version++
	w.updatedAt = now
	return Change{Direction: wagering.Debit, Before: before, After: after}, nil
}
func (w *Wallet) Credit(value money.Money, now time.Time) (Change, error) {
	if err := w.validateMovement(value, now); err != nil {
		return Change{}, err
	}
	if value.IsZero() || value.IsNegative() {
		return Change{}, domain.NewError(domain.ErrNonPositiveAmount, "credit must be positive")
	}
	after, err := w.balance.Add(value)
	if err != nil {
		return Change{}, err
	}
	before := w.balance
	w.balance = after
	w.version++
	w.updatedAt = now
	return Change{Direction: wagering.Credit, Before: before, After: after}, nil
}
func (w Wallet) validateMovement(value money.Money, now time.Time) error {
	if w.id == "" || w.version < 1 || now.IsZero() {
		return domain.NewError(domain.ErrInvalidValue, "invalid wallet state")
	}
	if value.Currency() != w.currency {
		return domain.NewError(domain.ErrCurrencyMismatch, "movement currency differs from wallet")
	}
	return nil
}
