package services

import (
	"context"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
	applicationwallet "backend-challenge/internal/application/wallet"
	"backend-challenge/internal/domain/money"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type WalletService struct {
	unitOfWork   ports.UnitOfWork
	wallets      ports.WalletRepository
	transactions ports.TransactionRepository
	ledger       ports.LedgerRepository
	outbox       ports.OutboxRepository
	now          func() time.Time
}

func NewWalletService(unitOfWork ports.UnitOfWork, wallets ports.WalletRepository, transactions ports.TransactionRepository, ledger ports.LedgerRepository, outbox ports.OutboxRepository) *WalletService {
	return &WalletService{unitOfWork: unitOfWork, wallets: wallets, transactions: transactions, ledger: ledger, outbox: outbox, now: time.Now}
}

type OpenWalletInput struct {
	PlayerID       string
	Currency       money.Currency
	InitialBalance money.Money
}
type OpenWalletResult struct{ Wallet domainwallet.Wallet }

func (s *WalletService) Open(ctx context.Context, input OpenWalletInput) (OpenWalletResult, error) {
	walletID, err := newID()
	if err != nil {
		return OpenWalletResult{}, err
	}
	txID, err := newID()
	if err != nil {
		return OpenWalletResult{}, err
	}
	ledgerID, err := newID()
	if err != nil {
		return OpenWalletResult{}, err
	}
	now := s.now().UTC()
	var result OpenWalletResult
	err = s.unitOfWork.Within(ctx, func(txCtx context.Context) error {
		opened, err := applicationwallet.Open(applicationwallet.OpenInput{WalletID: walletID, TransactionID: txID, LedgerID: ledgerID, PlayerID: input.PlayerID, Currency: input.Currency, InitialBalance: input.InitialBalance, Now: now})
		if err != nil {
			return err
		}
		if err := s.wallets.Create(txCtx, opened.Wallet); err != nil {
			return err
		}
		if opened.Transaction != nil {
			if err := s.transactions.Create(txCtx, *opened.Transaction); err != nil {
				return err
			}
			if err := s.ledger.Append(txCtx, *opened.Ledger); err != nil {
				return err
			}
			processedEvent, err := events.Processed(*opened.Transaction, opened.Transaction.ID())
			if err != nil {
				return err
			}
			if err := s.outbox.Append(txCtx, processedEvent); err != nil {
				return err
			}
			balanceEvent, err := events.BalanceChanged(*opened.Transaction, *opened.Ledger, opened.Wallet.Version(), opened.Transaction.ID())
			if err != nil {
				return err
			}
			if err := s.outbox.Append(txCtx, balanceEvent); err != nil {
				return err
			}
		}
		result.Wallet = opened.Wallet
		return nil
	})
	return result, err
}
