package money

import (
	"encoding/json"
	"math"
	"testing"

	"backend-challenge/internal/domain"
)

func must(t *testing.T, amount string, currency Currency) Money {
	t.Helper()
	m, err := Parse(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestParseAndSerialize(t *testing.T) {
	m := must(t, "25.00", BRL)
	if m.Minor() != 2500 || m.Amount() != "25.00" || m.Currency() != BRL {
		t.Fatalf("unexpected money: %#v", m)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"amount":"25.00","currency":"BRL"}` {
		t.Fatalf("json = %s", data)
	}
}
func TestParseRejectsInvalidExternalAmounts(t *testing.T) {
	for _, amount := range []string{"", "25", "25.0", "25.000", "-1.00", "+1.00", "1e2", "NaN", "Infinity", "01.00"} {
		if _, err := Parse(amount, BRL); err == nil {
			t.Errorf("Parse(%q) accepted", amount)
		}
	}
	if _, err := Parse("92233720368547758.08", BRL); err == nil {
		t.Error("expected overflow")
	}
}
func TestArithmeticAndCurrencyMismatch(t *testing.T) {
	a := must(t, "10.25", BRL)
	b := must(t, "2.75", BRL)
	sum, err := a.Add(b)
	if err != nil || sum.Amount() != "13.00" {
		t.Fatalf("sum=%v err=%v", sum, err)
	}
	diff, err := a.Sub(b)
	if err != nil || diff.Amount() != "7.50" {
		t.Fatalf("diff=%v err=%v", diff, err)
	}
	neg, err := b.Neg()
	if err != nil || neg.Amount() != "-2.75" {
		t.Fatalf("neg=%v err=%v", neg, err)
	}
	usd := must(t, "1.00", Currency("USD"))
	if _, err := a.Add(usd); err == nil {
		t.Fatal("expected currency mismatch")
	}
	if _, err := must(t, "1.00", BRL).Compare(usd); err == nil {
		t.Fatal("expected currency mismatch")
	}
}
func TestOverflow(t *testing.T) {
	max, err := New(math.MaxInt64, BRL)
	if err != nil {
		t.Fatal(err)
	}
	one, _ := New(1, BRL)
	if _, err := max.Add(one); !hasCode(err, domain.ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
	min, _ := New(math.MinInt64, BRL)
	if _, err := min.Neg(); !hasCode(err, domain.ErrOverflow) {
		t.Fatalf("want overflow, got %v", err)
	}
}
func hasCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}

func TestCurrencyRejectsUnknownISOCode(t *testing.T) {
	if _, err := NewCurrency("ZZZ"); err == nil {
		t.Fatal("expected unknown currency code to be rejected")
	}
	if _, err := NewCurrency("USD"); err != nil {
		t.Fatalf("expected USD to be accepted: %v", err)
	}
}
