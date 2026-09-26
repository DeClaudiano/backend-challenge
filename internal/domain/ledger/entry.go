package ledger

import (
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
)

type Entry struct {
	id, walletID, transactionID string
	direction                   wagering.Direction
	amount, before, after       money.Money
	createdAt                   time.Time
}

type WalletLedgerEntry = Entry

func New(id, walletID, transactionID string, direction wagering.Direction, amount, before, after money.Money, createdAt time.Time) (Entry, error) {
	if id == "" || walletID == "" || transactionID == "" || createdAt.IsZero() {
		return Entry{}, domain.NewError(domain.ErrInvalidValue, "ledger identity and timestamp are required")
	}
	if direction != wagering.Debit && direction != wagering.Credit {
		return Entry{}, domain.NewError(domain.ErrInvalidValue, "invalid ledger direction")
	}
	if !amount.IsPositive() {
		return Entry{}, domain.NewError(domain.ErrNonPositiveAmount, "ledger amount must be positive")
	}
	if before.Currency() != amount.Currency() || after.Currency() != amount.Currency() {
		return Entry{}, domain.NewError(domain.ErrCurrencyMismatch, "ledger currencies differ")
	}
	if before.IsNegative() || after.IsNegative() {
		return Entry{}, domain.NewError(domain.ErrInvalidValue, "ledger balances cannot be negative")
	}
	var expected money.Money
	var err error
	if direction == wagering.Debit {
		expected, err = before.Sub(amount)
	} else {
		expected, err = before.Add(amount)
	}
	if err != nil {
		return Entry{}, err
	}
	if !expected.Equal(after) {
		return Entry{}, domain.NewError(domain.ErrInvalidValue, "ledger balance equation is invalid")
	}
	return Entry{id: id, walletID: walletID, transactionID: transactionID, direction: direction, amount: amount, before: before, after: after, createdAt: createdAt}, nil
}
func (e Entry) ID() string                    { return e.id }
func (e Entry) WalletID() string              { return e.walletID }
func (e Entry) TransactionID() string         { return e.transactionID }
func (e Entry) Direction() wagering.Direction { return e.direction }
func (e Entry) Amount() money.Money           { return e.amount }
func (e Entry) BalanceBefore() money.Money    { return e.before }
func (e Entry) BalanceAfter() money.Money     { return e.after }
func (e Entry) CreatedAt() time.Time          { return e.createdAt }
