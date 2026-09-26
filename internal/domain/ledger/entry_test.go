package ledger

import (
	"testing"
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
)

func lm(t *testing.T, value string) money.Money {
	t.Helper()
	m, err := money.Parse(value, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestEntryValidDebitAndCredit(t *testing.T) {
	now := time.Unix(1, 0)
	if _, err := New("e1", "w1", "t1", wagering.Debit, lm(t, "80.00"), lm(t, "100.00"), lm(t, "20.00"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := New("e2", "w1", "t2", wagering.Credit, lm(t, "80.00"), lm(t, "20.00"), lm(t, "100.00"), now); err != nil {
		t.Fatal(err)
	}
}
func TestEntryRejectsInvalidEquationAndAmount(t *testing.T) {
	if _, err := New("e", "w", "t", wagering.Debit, lm(t, "80.00"), lm(t, "100.00"), lm(t, "30.00"), time.Now()); !hasCode(err, domain.ErrInvalidValue) {
		t.Fatalf("got %v", err)
	}
	if _, err := New("e", "w", "t", wagering.Debit, lm(t, "0.00"), lm(t, "100.00"), lm(t, "100.00"), time.Now()); !hasCode(err, domain.ErrNonPositiveAmount) {
		t.Fatalf("got %v", err)
	}
}
func hasCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}
