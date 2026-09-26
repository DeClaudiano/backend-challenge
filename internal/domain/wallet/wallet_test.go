package wallet

import (
	"testing"
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
)

func wm(t *testing.T, amount string) money.Money {
	t.Helper()
	v, err := money.Parse(amount, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestWalletDebitCreditAndVersion(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	w, err := New("w1", "p1", money.BRL, wm(t, "100.00"), now)
	if err != nil {
		t.Fatal(err)
	}
	change, err := w.Debit(wm(t, "80.00"), now.Add(time.Second))
	if err != nil || change.After.Amount() != "20.00" {
		t.Fatalf("change=%+v err=%v", change, err)
	}
	if w.Version() != 2 {
		t.Fatalf("version=%d", w.Version())
	}
	if _, err := w.Debit(wm(t, "20.01"), now); !hasCode(err, domain.ErrInsufficientBalance) {
		t.Fatalf("want insufficient, got %v", err)
	}
	if _, err := w.Credit(wm(t, "1.00"), now); err != nil {
		t.Fatal(err)
	}
	if w.Balance().Amount() != "21.00" || w.Version() != 3 {
		t.Fatalf("balance=%s version=%d", w.Balance().Amount(), w.Version())
	}
}
func TestWalletRejectsInvalidMovement(t *testing.T) {
	now := time.Unix(100, 0)
	w, _ := New("w1", "p1", money.BRL, wm(t, "0.00"), now)
	usd, _ := money.Parse("1.00", money.Currency("USD"))
	if _, err := w.Credit(usd, now); !hasCode(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("got %v", err)
	}
	if _, err := w.Credit(wm(t, "0.00"), now); !hasCode(err, domain.ErrNonPositiveAmount) {
		t.Fatalf("got %v", err)
	}
	if _, err := New("w", "p", money.BRL, func() money.Money { v, _ := money.New(-1, money.BRL); return v }(), now); !hasCode(err, domain.ErrNegativeAmount) {
		t.Fatalf("got %v", err)
	}
}
func hasCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}
