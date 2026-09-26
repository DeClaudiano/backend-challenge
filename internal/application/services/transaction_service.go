package services

import (
	"context"
	"errors"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
	applicationwagering "backend-challenge/internal/application/wagering"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	domainwallet "backend-challenge/internal/domain/wallet"
)

type TransactionService struct {
	unitOfWork   ports.UnitOfWork
	wallets      ports.WalletRepository
	transactions ports.TransactionRepository
	ledger       ports.LedgerRepository
	outbox       ports.OutboxRepository
	metrics      ports.Metrics
	now          func() time.Time
}

func NewTransactionService(unitOfWork ports.UnitOfWork, wallets ports.WalletRepository, transactions ports.TransactionRepository, ledger ports.LedgerRepository, outbox ports.OutboxRepository) *TransactionService {
	return &TransactionService{unitOfWork: unitOfWork, wallets: wallets, transactions: transactions, ledger: ledger, outbox: outbox, now: time.Now}
}

func NewTransactionServiceWithMetrics(unitOfWork ports.UnitOfWork, wallets ports.WalletRepository, transactions ports.TransactionRepository, ledger ports.LedgerRepository, outbox ports.OutboxRepository, metrics ports.Metrics) *TransactionService {
	return &TransactionService{unitOfWork: unitOfWork, wallets: wallets, transactions: transactions, ledger: ledger, outbox: outbox, metrics: metrics, now: time.Now}
}

type ProcessTransactionInput struct {
	ProviderID, ExternalTransactionID, IdempotencyKey string
	PlayerID, WalletID, RoundID, GameID               string
	Kind                                              domainwagering.Kind
	Money                                             money.Money
	ReferenceExternalTransactionID                    string
}
type ProcessTransactionResult struct {
	Transaction      domainwagering.WagerTransaction
	Wallet           domainwallet.Wallet
	Ledger           *ledger.Entry
	Balance          money.Money
	HasBalance       bool
	IdempotentReplay bool
}

func (s *TransactionService) Process(ctx context.Context, input ProcessTransactionInput) (ProcessTransactionResult, error) {
	started := time.Now()
	defer func() {
		if s.metrics != nil {
			s.metrics.ObserveProcessing(time.Since(started))
		}
	}()
	if input.IdempotencyKey == "" {
		return ProcessTransactionResult{}, domain.NewError(domain.ErrInvalidValue, "idempotency key is required")
	}
	business := applicationwagering.BusinessInput{ProviderID: input.ProviderID, ExternalTransactionID: input.ExternalTransactionID, PlayerID: input.PlayerID, WalletID: input.WalletID, RoundID: input.RoundID, GameID: input.GameID, Kind: input.Kind, Money: input.Money, ReferenceExternalTransactionID: input.ReferenceExternalTransactionID}
	fingerprint, err := applicationwagering.Fingerprint(business)
	if err != nil {
		return ProcessTransactionResult{}, err
	}
	if existing, found, err := s.findExisting(ctx, input); err != nil {
		return ProcessTransactionResult{}, err
	} else if found {
		result, replayErr := s.replay(existing, business, input.IdempotencyKey)
		if replayErr == nil && s.metrics != nil {
			s.metrics.IncIdempotentReplay()
			s.metrics.TransactionStatus(string(existing.Status()))
		}
		return result, replayErr
	}

	transactionID, err := newID()
	if err != nil {
		return ProcessTransactionResult{}, err
	}
	ledgerID, err := newID()
	if err != nil {
		return ProcessTransactionResult{}, err
	}
	now := s.now().UTC()
	var result ProcessTransactionResult
	var operationErr error
	err = s.unitOfWork.Within(ctx, func(txCtx context.Context) error {
		current, err := s.wallets.GetForUpdate(txCtx, input.WalletID)
		if err != nil {
			return err
		}
		var reference *domainwagering.WagerTransaction
		if input.ReferenceExternalTransactionID != "" {
			value, lookupErr := s.transactions.GetByExternalIDForUpdate(txCtx, input.ProviderID, input.ReferenceExternalTransactionID)
			if lookupErr == nil {
				reference = &value
			} else if !isNotFound(lookupErr) {
				return lookupErr
			}
		}
		processed, processErr := applicationwagering.Process(applicationwagering.ProcessInput{TransactionID: transactionID, LedgerID: ledgerID, ProviderID: input.ProviderID, ExternalTransactionID: input.ExternalTransactionID, IdempotencyKey: input.IdempotencyKey, PlayerID: input.PlayerID, WalletID: input.WalletID, RoundID: input.RoundID, GameID: input.GameID, Kind: input.Kind, Money: input.Money, ReferenceExternalTransactionID: input.ReferenceExternalTransactionID, Now: now, PayloadHash: fingerprint}, current, reference)
		if processErr != nil && processed.Transaction.ID() == "" {
			return processErr
		}
		if err := s.transactions.Create(txCtx, processed.Transaction); err != nil {
			return err
		}
		if processed.Ledger != nil {
			if err := s.ledger.Append(txCtx, *processed.Ledger); err != nil {
				return err
			}
		}
		if processed.Wallet.Version() != current.Version() {
			if err := s.wallets.Save(txCtx, processed.Wallet); err != nil {
				return err
			}
		}
		if reference != nil && reference.ReversedBy() == processed.Transaction.ID() {
			if err := s.transactions.Save(txCtx, *reference); err != nil {
				return err
			}
		}
		switch processed.Transaction.Status() {
		case domainwagering.Processed:
			event, err := events.Processed(processed.Transaction, processed.Transaction.ID())
			if err != nil {
				return err
			}
			if err := s.outbox.Append(txCtx, event); err != nil {
				return err
			}
			if processed.Ledger != nil {
				balanceEvent, err := events.BalanceChanged(processed.Transaction, *processed.Ledger, processed.Wallet.Version(), processed.Transaction.ID())
				if err != nil {
					return err
				}
				if err := s.outbox.Append(txCtx, balanceEvent); err != nil {
					return err
				}
			}
		case domainwagering.Rejected:
			event, err := events.Rejected(processed.Transaction, processed.Transaction.ID())
			if err != nil {
				return err
			}
			if err := s.outbox.Append(txCtx, event); err != nil {
				return err
			}
		case domainwagering.PendingReference:
			event, err := events.PendingReference(processed.Transaction, processed.Transaction.ID())
			if err != nil {
				return err
			}
			if err := s.outbox.Append(txCtx, event); err != nil {
				return err
			}
		}
		result = ProcessTransactionResult{Transaction: processed.Transaction, Wallet: processed.Wallet, Ledger: processed.Ledger}
		if balance, ok := processed.Transaction.ResultBalance(); ok {
			result.Balance, result.HasBalance = balance, true
		}
		operationErr = processErr
		return nil
	})
	if err != nil {
		if s.metrics != nil && isIdentityConflict(err) {
			s.metrics.IncConcurrencyConflict()
		}
		if isIdentityConflict(err) {
			if existing, found, lookupErr := s.findExisting(ctx, input); lookupErr == nil && found {
				return s.replay(existing, business, input.IdempotencyKey)
			}
		}
		return ProcessTransactionResult{}, err
	}
	if s.metrics != nil {
		s.metrics.TransactionStatus(string(result.Transaction.Status()))
	}
	return result, operationErr
}

func (s *TransactionService) findExisting(ctx context.Context, input ProcessTransactionInput) (domainwagering.WagerTransaction, bool, error) {
	existing, err := s.transactions.GetByIdempotency(ctx, input.ProviderID, input.IdempotencyKey)
	if err == nil {
		return existing, true, nil
	}
	if !isNotFound(err) {
		return domainwagering.WagerTransaction{}, false, err
	}
	existing, err = s.transactions.GetByExternalID(ctx, input.ProviderID, input.ExternalTransactionID)
	if err == nil {
		return existing, true, nil
	}
	if isNotFound(err) {
		return domainwagering.WagerTransaction{}, false, nil
	}
	return domainwagering.WagerTransaction{}, false, err
}

func (s *TransactionService) replay(existing domainwagering.WagerTransaction, business applicationwagering.BusinessInput, key string) (ProcessTransactionResult, error) {
	if _, err := applicationwagering.ResolveIdempotency(existing, business.ProviderID, key, business); err != nil {
		return ProcessTransactionResult{}, err
	}
	result := ProcessTransactionResult{Transaction: existing, IdempotentReplay: true}
	if balance, ok := existing.ResultBalance(); ok {
		result.Balance, result.HasBalance = balance, true
	}
	return result, nil
}

func isNotFound(err error) bool {
	var value *domain.Error
	return errors.As(err, &value) && value.Code == domain.ErrNotFound
}
func isIdentityConflict(err error) bool {
	var value *domain.Error
	return errors.As(err, &value) && value.Code == domain.ErrIdentityConflict
}
