package wagering

import (
	"sync"
	"testing"
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
)

func tm(t *testing.T, amount string) money.Money {
	t.Helper()
	v, err := money.Parse(amount, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestTransactionTransitionsAndTerminalState(t *testing.T) {
	now := time.Unix(10, 0)
	tx, err := NewExternal(ExternalInput{ID: "t1", ProviderID: "p", ExternalTransactionID: "e", IdempotencyKey: "k", PlayerID: "pl", WalletID: "w", RoundID: "r", GameID: "g", Kind: Bet, Money: tm(t, "1.00")}, "hash", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.MarkPendingReference(now); err != nil {
		t.Fatal(err)
	}
	if err := tx.MarkProcessed(tm(t, "0.00"), now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Reject("X", now); !hasCode(err, domain.ErrTerminalTransaction) {
		t.Fatalf("got %v", err)
	}
}
func TestTransactionRules(t *testing.T) {
	now := time.Unix(10, 0)
	if _, err := NewExternal(ExternalInput{ID: "t", ProviderID: "p", ExternalTransactionID: "e", IdempotencyKey: "k", PlayerID: "pl", WalletID: "w", Kind: Opening, Money: tm(t, "1.00")}, "h", now); !hasCode(err, domain.ErrInvalidOperation) {
		t.Fatalf("got %v", err)
	}
	if _, err := NewExternal(ExternalInput{ID: "t", ProviderID: "p", ExternalTransactionID: "e", IdempotencyKey: "k", PlayerID: "pl", WalletID: "w", Kind: Loss, Money: tm(t, "1.00")}, "h", now); !hasCode(err, domain.ErrInvalidOperation) {
		t.Fatalf("got %v", err)
	}
	if _, err := NewExternal(ExternalInput{ID: "t", ProviderID: "p", ExternalTransactionID: "e", IdempotencyKey: "k", PlayerID: "pl", WalletID: "w", Kind: Refund, Money: tm(t, "1.00")}, "h", now); !hasCode(err, domain.ErrReferenceRequired) {
		t.Fatalf("got %v", err)
	}
	opening, err := NewOpening(OpeningInput{ID: "o", WalletID: "w", PlayerID: "pl", Money: tm(t, "1.00"), Now: now})
	if err != nil || opening.Source() != Internal || opening.Kind() != Opening {
		t.Fatalf("opening=%+v err=%v", opening, err)
	}
	bet, _ := NewExternal(ExternalInput{ID: "b", ProviderID: "p", ExternalTransactionID: "be", IdempotencyKey: "bk", PlayerID: "pl", WalletID: "w", Kind: Bet, Money: tm(t, "1.00")}, "h", now)
	_ = bet.MarkProcessed(tm(t, "1.00"), now)
	if err := bet.ReserveReversal("r1", Refund); err != nil {
		t.Fatal(err)
	}
	if err := bet.ReserveReversal("r2", Rollback); !hasCode(err, domain.ErrReversalAlreadyApplied) {
		t.Fatalf("got %v", err)
	}
}

func TestConcurrentReversalClaimsAllowOneWinner(t *testing.T) {
	now := time.Unix(10, 0)
	tx, err := NewExternal(ExternalInput{ID: "t", ProviderID: "p", ExternalTransactionID: "e", IdempotencyKey: "k", PlayerID: "pl", WalletID: "w", Kind: Bet, Money: tm(t, "1.00")}, "h", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.MarkProcessed(tm(t, "1.00"), now); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	results := make(chan error, 2)
	for _, id := range []string{"r1", "r2"} {
		go func(id string) {
			mu.Lock()
			defer mu.Unlock()
			results <- tx.ReserveReversal(id, Refund)
		}(id)
	}
	var success, rejected int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case hasCode(err, domain.ErrReversalAlreadyApplied):
			rejected++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("success=%d rejected=%d", success, rejected)
	}
}
func hasCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}
