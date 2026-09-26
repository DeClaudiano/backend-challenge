package references

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
	applicationwagering "backend-challenge/internal/application/wagering"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/ledger"
	domainwagering "backend-challenge/internal/domain/wagering"
)

const ReferenceNotFoundFailureCode = "REFERENCE_NOT_FOUND"

type Service struct {
	unitOfWork   ports.UnitOfWork
	pending      ports.PendingReferenceRepository
	wallets      ports.WalletRepository
	transactions ports.TransactionRepository
	ledger       ports.LedgerRepository
	outbox       ports.OutboxRepository
	metrics      ports.Metrics
	now          func() time.Time
}

func NewService(unitOfWork ports.UnitOfWork, pending ports.PendingReferenceRepository, wallets ports.WalletRepository, transactions ports.TransactionRepository, ledgerRepository ports.LedgerRepository, outbox ports.OutboxRepository) *Service {
	return &Service{unitOfWork: unitOfWork, pending: pending, wallets: wallets, transactions: transactions, ledger: ledgerRepository, outbox: outbox, now: time.Now}
}

func NewServiceWithMetrics(unitOfWork ports.UnitOfWork, pending ports.PendingReferenceRepository, wallets ports.WalletRepository, transactions ports.TransactionRepository, ledgerRepository ports.LedgerRepository, outbox ports.OutboxRepository, metrics ports.Metrics) *Service {
	return &Service{unitOfWork: unitOfWork, pending: pending, wallets: wallets, transactions: transactions, ledger: ledgerRepository, outbox: outbox, metrics: metrics, now: time.Now}
}

type Config struct {
	BatchSize    int
	MaxAttempts  int
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
	ReferenceTTL time.Duration
}

func DefaultConfig() Config {
	return Config{BatchSize: 100, MaxAttempts: 8, BaseBackoff: time.Second, MaxBackoff: time.Hour, ReferenceTTL: 24 * time.Hour}
}

func (s *Service) RunOnce(ctx context.Context, cfg Config) error {
	if cfg.BatchSize <= 0 || cfg.MaxAttempts <= 0 || cfg.BaseBackoff <= 0 || cfg.MaxBackoff <= 0 || cfg.ReferenceTTL <= 0 {
		return domain.NewError(domain.ErrInvalidValue, "invalid pending reference worker configuration")
	}
	now := s.now().UTC()
	records, err := s.pending.ListPendingReferences(ctx, now, cfg.BatchSize)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := s.resume(ctx, record.Transaction.ID(), record.Attempts, record.ExpiresAt, cfg); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) resume(ctx context.Context, transactionID string, attempts int, expiresAt time.Time, cfg Config) error {
	now := s.now().UTC()
	return s.unitOfWork.Within(ctx, func(txCtx context.Context) error {
		pending, err := s.transactions.GetForUpdate(txCtx, transactionID)
		if err != nil {
			return err
		}
		if pending.Status() != domainwagering.PendingReference {
			return nil
		}
		if expiresAt.IsZero() {
			expiresAt = now.Add(cfg.ReferenceTTL)
		}
		if !now.Before(expiresAt) || attempts >= cfg.MaxAttempts {
			return s.rejectExpired(txCtx, pending, now)
		}

		wallet, err := s.wallets.GetForUpdate(txCtx, pending.WalletID())
		if err != nil {
			return err
		}
		reference, lookupErr := s.transactions.GetByExternalIDForUpdate(txCtx, pending.ProviderID(), pending.ReferenceExternalID())
		if lookupErr != nil {
			if !isNotFound(lookupErr) {
				return lookupErr
			}
			return s.schedule(txCtx, pending.ID(), attempts, now, expiresAt, cfg)
		}
		if reference.Status() != domainwagering.Processed {
			switch reference.Status() {
			case domainwagering.Pending, domainwagering.PendingReference:
				return s.schedule(txCtx, pending.ID(), attempts, now, expiresAt, cfg)
			case domainwagering.Rejected, domainwagering.Failed:
				return s.rejectReferenceNotProcessed(txCtx, pending, now)
			default:
				return s.schedule(txCtx, pending.ID(), attempts, now, expiresAt, cfg)
			}
		}

		ledgerID, err := newID()
		if err != nil {
			return err
		}
		processed, processErr := applicationwagering.Process(applicationwagering.ProcessInput{
			TransactionID: pending.ID(), LedgerID: ledgerID, ProviderID: pending.ProviderID(), ExternalTransactionID: pending.ExternalID(),
			IdempotencyKey: pending.IdempotencyKey(), PlayerID: pending.PlayerID(), WalletID: pending.WalletID(), RoundID: pending.RoundID(),
			GameID: pending.GameID(), Kind: pending.Kind(), Money: pending.Amount(), ReferenceExternalTransactionID: pending.ReferenceExternalID(),
			Now: now, PayloadHash: pending.PayloadHash(),
		}, wallet, &reference)
		if processErr != nil && processed.Transaction.ID() == "" {
			return processErr
		}
		if processed.Transaction.Status() == domainwagering.Processed {
			if err := processed.Transaction.LinkReference(reference.ID()); err != nil {
				return err
			}
		}
		if err := s.transactions.Save(txCtx, processed.Transaction); err != nil {
			return err
		}
		if processed.Ledger != nil {
			if err := s.ledger.Append(txCtx, *processed.Ledger); err != nil {
				return err
			}
		}
		if processed.Wallet.Version() != wallet.Version() {
			if err := s.wallets.Save(txCtx, processed.Wallet); err != nil {
				return err
			}
		}
		if processed.Transaction.Status() == domainwagering.Processed && reference.ReversedBy() == processed.Transaction.ID() {
			if err := s.transactions.Save(txCtx, reference); err != nil {
				return err
			}
		}
		if processErr != nil {
			return s.appendRejected(txCtx, processed.Transaction)
		}
		return s.appendProcessed(txCtx, processed.Transaction, processed.Ledger, processed.Wallet.Version())
	})
}

func (s *Service) schedule(ctx context.Context, transactionID string, attempts int, now, expiresAt time.Time, cfg Config) error {
	nextAttempts := attempts + 1
	backoff := cfg.BaseBackoff
	for i := 1; i < nextAttempts && backoff < cfg.MaxBackoff; i++ {
		backoff *= 2
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}
	next := now.Add(backoff)
	if next.After(expiresAt) {
		next = expiresAt
	}
	if s.metrics != nil {
		s.metrics.IncRetry()
	}
	return s.pending.SchedulePendingReference(ctx, transactionID, nextAttempts, next, expiresAt)
}

func (s *Service) rejectReferenceNotProcessed(ctx context.Context, transaction domainwagering.WagerTransaction, now time.Time) error {
	const failureCode = "REFERENCE_NOT_PROCESSED"
	if err := transaction.Reject(failureCode, now); err != nil {
		return err
	}
	if err := s.transactions.Save(ctx, transaction); err != nil {
		return err
	}
	return s.appendRejected(ctx, transaction)
}

func (s *Service) rejectExpired(ctx context.Context, transaction domainwagering.WagerTransaction, now time.Time) error {
	if err := transaction.Reject(ReferenceNotFoundFailureCode, now); err != nil {
		return err
	}
	if err := s.transactions.Save(ctx, transaction); err != nil {
		return err
	}
	return s.appendRejected(ctx, transaction)
}

func (s *Service) appendRejected(ctx context.Context, transaction domainwagering.WagerTransaction) error {
	event, err := events.Rejected(transaction, transaction.ID())
	if err != nil {
		return err
	}
	return s.outbox.Append(ctx, event)
}

func (s *Service) appendProcessed(ctx context.Context, transaction domainwagering.WagerTransaction, entry *ledger.Entry, version int64) error {
	event, err := events.Processed(transaction, transaction.ID())
	if err != nil {
		return err
	}
	if err := s.outbox.Append(ctx, event); err != nil {
		return err
	}
	if entry != nil {
		balanceEvent, err := events.BalanceChanged(transaction, *entry, version, transaction.ID())
		if err != nil {
			return err
		}
		return s.outbox.Append(ctx, balanceEvent)
	}
	return nil
}

func isNotFound(err error) bool {
	value, ok := err.(*domain.Error)
	return ok && value.Code == domain.ErrNotFound
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
