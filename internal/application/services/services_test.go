package services

import (
	"context"
	"testing"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type fakeUOW struct{ calls int }

func (f *fakeUOW) Within(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(ctx)
}

type fakeWallets struct {
	values map[string]domainwallet.Wallet
}

func (f *fakeWallets) Get(_ context.Context, id string) (domainwallet.Wallet, error) {
	return f.get(id)
}
func (f *fakeWallets) GetForUpdate(_ context.Context, id string) (domainwallet.Wallet, error) {
	return f.get(id)
}
func (f *fakeWallets) get(id string) (domainwallet.Wallet, error) {
	value, ok := f.values[id]
	if !ok {
		return domainwallet.Wallet{}, domain.NewError(domain.ErrNotFound, "wallet not found")
	}
	return value, nil
}
func (f *fakeWallets) Create(_ context.Context, value domainwallet.Wallet) error {
	if _, ok := f.values[value.ID()]; ok {
		return domain.NewError(domain.ErrIdentityConflict, "wallet exists")
	}
	f.values[value.ID()] = value
	return nil
}
func (f *fakeWallets) Save(_ context.Context, value domainwallet.Wallet) error {
	f.values[value.ID()] = value
	return nil
}

type fakeTransactions struct {
	values map[string]domainwagering.WagerTransaction
}

func (f *fakeTransactions) Get(_ context.Context, id string) (domainwagering.WagerTransaction, error) {
	value, ok := f.values[id]
	if !ok {
		return domainwagering.WagerTransaction{}, domain.NewError(domain.ErrNotFound, "transaction not found")
	}
	return value, nil
}
func (f *fakeTransactions) GetForUpdate(ctx context.Context, id string) (domainwagering.WagerTransaction, error) {
	return f.Get(ctx, id)
}
func (f *fakeTransactions) GetByIdempotency(_ context.Context, provider, key string) (domainwagering.WagerTransaction, error) {
	for _, value := range f.values {
		if value.ProviderID() == provider && value.IdempotencyKey() == key {
			return value, nil
		}
	}
	return domainwagering.WagerTransaction{}, domain.NewError(domain.ErrNotFound, "transaction not found")
}
func (f *fakeTransactions) GetByExternalID(_ context.Context, provider, external string) (domainwagering.WagerTransaction, error) {
	for _, value := range f.values {
		if value.ProviderID() == provider && value.ExternalID() == external {
			return value, nil
		}
	}
	return domainwagering.WagerTransaction{}, domain.NewError(domain.ErrNotFound, "transaction not found")
}
func (f *fakeTransactions) GetByExternalIDForUpdate(ctx context.Context, provider, external string) (domainwagering.WagerTransaction, error) {
	return f.GetByExternalID(ctx, provider, external)
}
func (f *fakeTransactions) Create(_ context.Context, value domainwagering.WagerTransaction) error {
	f.values[value.ID()] = value
	return nil
}
func (f *fakeTransactions) Save(_ context.Context, value domainwagering.WagerTransaction) error {
	f.values[value.ID()] = value
	return nil
}

type fakeLedger struct{ values []ledger.Entry }

func (f *fakeLedger) Append(_ context.Context, value ledger.Entry) error {
	f.values = append(f.values, value)
	return nil
}
func (f *fakeLedger) List(_ context.Context, walletID, _ string, _ int) (ports.LedgerPage, error) {
	result := make([]ledger.Entry, 0)
	for _, value := range f.values {
		if value.WalletID() == walletID {
			result = append(result, value)
		}
	}
	return ports.LedgerPage{Entries: result}, nil
}

type fakeOutbox struct{ values []events.Envelope }

func (f *fakeOutbox) Append(_ context.Context, value events.Envelope) error {
	f.values = append(f.values, value)
	return nil
}
func (f *fakeOutbox) ClaimPending(context.Context, time.Time, time.Time, int) ([]ports.OutboxRecord, error) {
	return nil, nil
}
func (f *fakeOutbox) MarkPublished(context.Context, string, time.Time, time.Time) error { return nil }
func (f *fakeOutbox) RecordFailure(context.Context, string, time.Time, time.Time, string) error {
	return nil
}

func testMoney(t *testing.T, value string) money.Money {
	t.Helper()
	result, err := money.Parse(value, money.BRL)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func newServices() (*fakeUOW, *fakeWallets, *fakeTransactions, *fakeLedger, *WalletService, *TransactionService, *QueryService) {
	uow := &fakeUOW{}
	wallets := &fakeWallets{values: map[string]domainwallet.Wallet{}}
	transactions := &fakeTransactions{values: map[string]domainwagering.WagerTransaction{}}
	entries := &fakeLedger{}
	outbox := &fakeOutbox{}
	return uow, wallets, transactions, entries, NewWalletService(uow, wallets, transactions, entries, outbox), NewTransactionService(uow, wallets, transactions, entries, outbox), NewQueryService(wallets, transactions, entries)
}

func TestWalletServiceOpenPersistsAtomicSet(t *testing.T) {
	uow, wallets, transactions, entries, service, _, _ := newServices()
	service.now = func() time.Time { return time.Unix(1, 0).UTC() }
	result, err := service.Open(context.Background(), OpenWalletInput{PlayerID: "player", Currency: money.BRL, InitialBalance: testMoney(t, "100.00")})
	if err != nil {
		t.Fatal(err)
	}
	if uow.calls != 1 || len(wallets.values) != 1 || len(transactions.values) != 1 || len(entries.values) != 1 {
		t.Fatalf("calls=%d wallets=%d transactions=%d ledger=%d", uow.calls, len(wallets.values), len(transactions.values), len(entries.values))
	}
	if result.Wallet.Balance().Amount() != "100.00" || result.Wallet.Version() != 1 {
		t.Fatalf("wallet=%+v", result.Wallet)
	}
	if len(service.outbox.(*fakeOutbox).values) != 2 {
		t.Fatalf("opening should emit processed and balance events, got %d", len(service.outbox.(*fakeOutbox).values))
	}
}

func TestTransactionServicePersistsAndReplays(t *testing.T) {
	uow, wallets, _, _, _, service, queries := newServices()
	initial, err := domainwallet.New("wallet", "player", money.BRL, testMoney(t, "100.00"), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	wallets.values[initial.ID()] = initial
	service.now = func() time.Time { return time.Unix(2, 0).UTC() }
	input := ProcessTransactionInput{ProviderID: "provider", ExternalTransactionID: "external", IdempotencyKey: "key", PlayerID: "player", WalletID: "wallet", RoundID: "round", GameID: "game", Kind: domainwagering.Bet, Money: testMoney(t, "25.00")}
	first, err := service.Process(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.IdempotentReplay || first.Balance.Amount() != "75.00" {
		t.Fatalf("first=%+v", first)
	}
	second, err := service.Process(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.IdempotentReplay || second.Balance.Amount() != "75.00" {
		t.Fatalf("replay=%+v", second)
	}
	stored, err := queries.GetWallet(context.Background(), "wallet")
	if err != nil || stored.Balance().Amount() != "75.00" {
		t.Fatalf("stored=%v err=%v", stored, err)
	}
	if uow.calls != 1 {
		t.Fatalf("replay should not open transaction, calls=%d", uow.calls)
	}
	if len(service.outbox.(*fakeOutbox).values) != 2 {
		t.Fatalf("replay should not emit new events, got %d", len(service.outbox.(*fakeOutbox).values))
	}
}

func TestQueryServiceReconcilesLedger(t *testing.T) {
	_, wallets, _, entries, walletService, _, queries := newServices()
	walletService.now = func() time.Time { return time.Unix(1, 0).UTC() }
	opened, err := walletService.Open(context.Background(), OpenWalletInput{PlayerID: "player", Currency: money.BRL, InitialBalance: testMoney(t, "100.00")})
	if err != nil {
		t.Fatal(err)
	}
	_ = entries
	reconciliation, err := queries.Reconcile(context.Background(), opened.Wallet.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !reconciliation.Consistent || reconciliation.CalculatedBalance.Amount() != "100.00" || reconciliation.CheckedEntries != 1 {
		t.Fatalf("reconciliation=%+v", reconciliation)
	}
	if _, ok := wallets.values[opened.Wallet.ID()]; !ok {
		t.Fatal("wallet was not persisted")
	}
}
