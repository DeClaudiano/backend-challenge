package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
)

func TestEnvelopeSerializesVersionAndMoneyAsStrings(t *testing.T) {
	value, err := money.Parse("25.00", money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := wagering.NewExternal(wagering.ExternalInput{ID: "tx", ProviderID: "provider", ExternalTransactionID: "external", IdempotencyKey: "key", PlayerID: "player", WalletID: "wallet", Kind: wagering.Bet, Money: value}, "hash", time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.MarkProcessed(value, time.Unix(2, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	event, err := Processed(tx, tx.ID())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	if event.Version != Version || event.EventID == "" || !strings.Contains(serialized, `"amount":"25.00"`) || !strings.Contains(serialized, `"occurredAt":"1970-01-01T00:00:02Z"`) {
		t.Fatalf("event=%s", serialized)
	}
}

func TestBalanceChangedPreservesStableEventIDInEnvelope(t *testing.T) {
	value, _ := money.Parse("1.00", money.BRL)
	tx, err := wagering.NewExternal(wagering.ExternalInput{ID: "tx", ProviderID: "provider", ExternalTransactionID: "external", IdempotencyKey: "key", PlayerID: "player", WalletID: "wallet", Kind: wagering.Bet, Money: value}, "hash", time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := money.Zero(money.BRL)
	after, _ := before.Add(value)
	entry, err := ledger.New("entry", "wallet", "tx", wagering.Credit, value, before, after, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	first, err := BalanceChanged(tx, entry, 1, tx.ID())
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEnvelope(first.EventID, first.EventType, first.AggregateID, first.CorrelationID, first.CausationID, first.OccurredAt, first.Data)
	if err != nil {
		t.Fatal(err)
	}
	if first.EventID != second.EventID {
		t.Fatalf("event ID changed: %s != %s", first.EventID, second.EventID)
	}
}
