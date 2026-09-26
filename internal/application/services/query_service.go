package services

import (
	"context"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type QueryService struct {
	wallets      ports.WalletRepository
	transactions ports.TransactionRepository
	ledger       ports.LedgerRepository
	metrics      ports.Metrics
}

func NewQueryService(wallets ports.WalletRepository, transactions ports.TransactionRepository, ledger ports.LedgerRepository) *QueryService {
	return &QueryService{wallets: wallets, transactions: transactions, ledger: ledger}
}

func NewQueryServiceWithMetrics(wallets ports.WalletRepository, transactions ports.TransactionRepository, ledger ports.LedgerRepository, metrics ports.Metrics) *QueryService {
	return &QueryService{wallets: wallets, transactions: transactions, ledger: ledger, metrics: metrics}
}
func (s *QueryService) GetWallet(ctx context.Context, id string) (domainwallet.Wallet, error) {
	return s.wallets.Get(ctx, id)
}
func (s *QueryService) GetTransaction(ctx context.Context, id string) (domainwagering.WagerTransaction, error) {
	return s.transactions.Get(ctx, id)
}
func (s *QueryService) GetExternalTransaction(ctx context.Context, providerID, externalID string) (domainwagering.WagerTransaction, error) {
	return s.transactions.GetByExternalID(ctx, providerID, externalID)
}
func (s *QueryService) ListLedger(ctx context.Context, walletID, cursor string, limit int) (ports.LedgerPage, error) {
	return s.ledger.List(ctx, walletID, cursor, limit)
}

type Reconciliation struct {
	WalletID                                     string
	StoredBalance, CalculatedBalance, Difference money.Money
	Consistent                                   bool
	CheckedEntries                               int
}

func (s *QueryService) Reconcile(ctx context.Context, walletID string) (Reconciliation, error) {
	wallet, err := s.wallets.Get(ctx, walletID)
	if err != nil {
		return Reconciliation{}, err
	}
	calculated, err := money.Zero(wallet.Currency())
	if err != nil {
		return Reconciliation{}, err
	}
	cursor := ""
	checked := 0
	for {
		page, err := s.ledger.List(ctx, walletID, cursor, 1000)
		if err != nil {
			return Reconciliation{}, err
		}
		for _, entry := range page.Entries {
			var next money.Money
			if string(entry.Direction()) == "CREDIT" {
				next, err = calculated.Add(entry.Amount())
			} else {
				next, err = calculated.Sub(entry.Amount())
			}
			if err != nil {
				return Reconciliation{}, err
			}
			calculated = next
			checked++
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	difference, err := wallet.Balance().Sub(calculated)
	if err != nil {
		return Reconciliation{}, err
	}
	consistent := difference.IsZero()
	if !consistent && s.metrics != nil {
		s.metrics.ReconciliationDivergence()
	}
	return Reconciliation{WalletID: walletID, StoredBalance: wallet.Balance(), CalculatedBalance: calculated, Difference: difference, Consistent: consistent, CheckedEntries: checked}, nil
}
