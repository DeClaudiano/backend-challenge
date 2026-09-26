package wagering

import (
	"testing"
	"time"

	"backend-challenge/internal/application/wallet"
	"backend-challenge/internal/domain"
	domainmoney "backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

func pm(t *testing.T, value string) domainmoney.Money {
	t.Helper()
	m, err := domainmoney.Parse(value, domainmoney.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func testWallet(t *testing.T) domainwallet.Wallet {
	w, err := domainwallet.New("wallet", "player", domainmoney.BRL, pm(t, "100.00"), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func input(kind domainwagering.Kind, amount string) ProcessInput {
	return ProcessInput{TransactionID: string(kind) + "-id", ProviderID: "provider", ExternalTransactionID: string(kind) + "-external", IdempotencyKey: string(kind) + "-key", PlayerID: "player", WalletID: "wallet", RoundID: "round", GameID: "game", Kind: kind, Money: mustMoney(amount), Now: time.Unix(2, 0), PayloadHash: "hash"}
}
func mustMoney(value string) domainmoney.Money {
	m, _ := domainmoney.Parse(value, domainmoney.BRL)
	return m
}

func TestProcessBetWinLossAndInsufficientBalance(t *testing.T) {
	bet, err := Process(input(domainwagering.Bet, "80.00"), testWallet(t), nil)
	if err != nil || bet.Transaction.Status() != domainwagering.Processed || bet.Wallet.Balance().Amount() != "20.00" || bet.Ledger == nil {
		t.Fatalf("bet=%+v err=%v", bet, err)
	}
	bad := input(domainwagering.Bet, "30.00")
	bad.TransactionID = "bad"
	bad.ExternalTransactionID = "bad-external"
	bad.IdempotencyKey = "bad-key"
	rejected, err := Process(bad, bet.Wallet, nil)
	if !hasProcessCode(err, domain.ErrInsufficientBalance) || rejected.Transaction.Status() != domainwagering.Rejected || rejected.Transaction.FailureCode() != string(domain.ErrInsufficientBalance) {
		t.Fatalf("rejected=%+v err=%v", rejected, err)
	}
	win, err := Process(input(domainwagering.Win, "10.00"), bet.Wallet, nil)
	if err != nil || win.Wallet.Balance().Amount() != "30.00" || win.Ledger.Direction() != domainwagering.Credit {
		t.Fatalf("win=%+v err=%v", win, err)
	}
	lossInput := input(domainwagering.Loss, "0.00")
	loss, err := Process(lossInput, bet.Wallet, nil)
	if err != nil || loss.Ledger != nil || loss.Wallet.Version() != bet.Wallet.Version() {
		t.Fatalf("loss=%+v err=%v", loss, err)
	}
}

func TestOpeningAndReversals(t *testing.T) {
	opening, err := wallet.Open(wallet.OpenInput{WalletID: "wallet", TransactionID: "opening", PlayerID: "player", Currency: domainmoney.BRL, InitialBalance: pm(t, "100.00"), Now: time.Unix(1, 0)})
	if err != nil || opening.Transaction.Status() != domainwagering.Processed || opening.Ledger == nil {
		t.Fatalf("opening=%+v err=%v", opening, err)
	}
	zero, err := wallet.Open(wallet.OpenInput{WalletID: "zero", TransactionID: "", PlayerID: "player", Currency: domainmoney.BRL, InitialBalance: pm(t, "0.00"), Now: time.Unix(1, 0)})
	if err != nil || zero.Transaction != nil || zero.Ledger != nil {
		t.Fatalf("zero=%+v err=%v", zero, err)
	}
	bet, err := Process(input(domainwagering.Bet, "25.00"), testWallet(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	refundInput := input(domainwagering.Refund, "25.00")
	refundInput.TransactionID, refundInput.ExternalTransactionID, refundInput.IdempotencyKey, refundInput.ReferenceExternalTransactionID = "refund-id", "refund-ext", "refund-key", "BET-external"
	refund, err := Process(refundInput, bet.Wallet, &bet.Transaction)
	if err != nil || refund.Wallet.Balance().Amount() != "100.00" || refund.Ledger.Direction() != domainwagering.Credit {
		t.Fatalf("refund=%+v err=%v", refund, err)
	}
	rollbackInput := input(domainwagering.Rollback, "25.00")
	rollbackInput.TransactionID, rollbackInput.ExternalTransactionID, rollbackInput.IdempotencyKey, rollbackInput.ReferenceExternalTransactionID = "rollback-id", "rollback-ext", "rollback-key", "refund-ext"
	rollback, err := Process(rollbackInput, refund.Wallet, &refund.Transaction)
	if err != nil || rollback.Wallet.Balance().Amount() != "75.00" || rollback.Ledger.Direction() != domainwagering.Debit {
		t.Fatalf("rollback=%+v err=%v", rollback, err)
	}
	secondRollback := rollbackInput
	secondRollback.TransactionID, secondRollback.ExternalTransactionID, secondRollback.IdempotencyKey = "rollback2", "rollback2-ext", "rollback2-key"
	if _, err := Process(secondRollback, refund.Wallet, &refund.Transaction); !hasProcessCode(err, domain.ErrReversalAlreadyApplied) {
		t.Fatalf("got %v", err)
	}
	secondBet, err := Process(input(domainwagering.Bet, "25.00"), testWallet(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	rollbackBet := input(domainwagering.Rollback, "25.00")
	rollbackBet.TransactionID, rollbackBet.ExternalTransactionID, rollbackBet.IdempotencyKey, rollbackBet.ReferenceExternalTransactionID = "rollback-bet", "rollback-bet-ext", "rollback-bet-key", "BET-external"
	secondBetCopy := secondBet.Transaction
	rolledBackBet, err := Process(rollbackBet, secondBet.Wallet, &secondBetCopy)
	if err != nil || rolledBackBet.Wallet.Balance().Amount() != "100.00" || rolledBackBet.Ledger.Direction() != domainwagering.Credit {
		t.Fatalf("rollback bet=%+v err=%v", rolledBackBet, err)
	}
}

func TestPendingReferenceAndIncompatibleReference(t *testing.T) {
	in := input(domainwagering.Refund, "10.00")
	in.ReferenceExternalTransactionID = "missing"
	pending, err := Process(in, testWallet(t), nil)
	if err != nil || pending.Transaction.Status() != domainwagering.PendingReference {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	bet, _ := Process(input(domainwagering.Bet, "10.00"), testWallet(t), nil)
	bad := in
	bad.TransactionID, bad.ExternalTransactionID, bad.IdempotencyKey = "r", "r-ext", "r-key"
	bad.ReferenceExternalTransactionID = "bad"
	if _, err := Process(bad, bet.Wallet, &bet.Transaction); !hasProcessCode(err, domain.ErrReferenceIncompatible) {
		t.Fatalf("got %v", err)
	}
}

func TestRollbackInsufficientBalanceHasSpecificFailureCode(t *testing.T) {
	zeroWallet, _ := domainwallet.New("wallet", "player", domainmoney.BRL, mustMoney("0.00"), time.Unix(3, 0))
	win, err := Process(input(domainwagering.Win, "80.00"), zeroWallet, nil)
	if err != nil {
		t.Fatal(err)
	}
	rollback := input(domainwagering.Rollback, "80.00")
	rollback.TransactionID, rollback.ExternalTransactionID, rollback.IdempotencyKey, rollback.ReferenceExternalTransactionID = "rollback", "rollback-ext", "rollback-key", "WIN-external"
	_, err = Process(rollback, zeroWallet, &win.Transaction)
	if !hasProcessCode(err, domain.ErrInsufficientReversal) {
		t.Fatalf("want reversal-specific failure, got %v", err)
	}
}

func hasProcessCode(err error, code domain.ErrorCode) bool {
	e, ok := err.(*domain.Error)
	return ok && e.Code == code
}
