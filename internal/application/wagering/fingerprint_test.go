package wagering

import (
	"testing"
	"time"

	"backend-challenge/internal/domain"
	domainmoney "backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
)

func fm(t *testing.T, value string) domainmoney.Money {
	t.Helper()
	m, err := domainmoney.Parse(value, domainmoney.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func fi() BusinessInput {
	return BusinessInput{ProviderID: "provider-a", ExternalTransactionID: "transaction-123", PlayerID: "player", WalletID: "wallet", RoundID: "round", GameID: "game", Kind: domainwagering.Bet, Money: fmTest("25.00"), ReferenceExternalTransactionID: ""}
}
func fmTest(v string) domainmoney.Money { m, _ := domainmoney.Parse(v, domainmoney.BRL); return m }
func TestCanonicalBusinessJSONAndFingerprint(t *testing.T) {
	input := fi()
	data, err := CanonicalBusinessJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"externalTransactionId":"transaction-123","gameId":"game","kind":"BET","money":{"amount":"25.00","currency":"BRL"},"playerId":"player","providerId":"provider-a","referenceExternalTransactionId":"","roundId":"round","walletId":"wallet"}`
	if string(data) != want {
		t.Fatalf("canonical=%s want=%s", data, want)
	}
	first, _ := Fingerprint(input)
	second, _ := Fingerprint(input)
	if first != second {
		t.Fatal("fingerprint is not stable")
	}
	input.Kind = domainwagering.Win
	changed, _ := Fingerprint(input)
	if first == changed {
		t.Fatal("different business content has same fingerprint")
	}
}
func TestReplayAndIdentityConflicts(t *testing.T) {
	input := fi()
	hash, _ := Fingerprint(input)
	tx, err := domainwagering.NewExternal(domainwagering.ExternalInput{ID: "id", ProviderID: input.ProviderID, ExternalTransactionID: input.ExternalTransactionID, IdempotencyKey: "key", PlayerID: input.PlayerID, WalletID: input.WalletID, RoundID: input.RoundID, GameID: input.GameID, Kind: input.Kind, Money: input.Money}, hash, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := ResolveIdempotency(tx, input.ProviderID, "key", input)
	if err != nil || !replay {
		t.Fatalf("replay=%v err=%v", replay, err)
	}
	input.Kind = domainwagering.Win
	if _, err := ResolveIdempotency(tx, "provider-a", "key", input); !hasCode(err, domain.ErrIdentityConflict) {
		t.Fatalf("got %v", err)
	}
	if _, err := ResolveExternalIdentity(tx, "provider-a", input.ExternalTransactionID, "other-key", fi()); !hasCode(err, domain.ErrIdentityConflict) {
		t.Fatalf("got %v", err)
	}
}
func hasCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}
