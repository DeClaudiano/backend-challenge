package ports

import (
	"context"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/wagering"
	"backend-challenge/internal/domain/wallet"
)

type WalletRepository interface {
	Get(ctx context.Context, walletID string) (wallet.Wallet, error)
	GetForUpdate(ctx context.Context, walletID string) (wallet.Wallet, error)
	Create(ctx context.Context, value wallet.Wallet) error
	Save(ctx context.Context, value wallet.Wallet) error
}
type TransactionRepository interface {
	Get(ctx context.Context, transactionID string) (wagering.WagerTransaction, error)
	GetForUpdate(ctx context.Context, transactionID string) (wagering.WagerTransaction, error)
	GetByIdempotency(ctx context.Context, providerID, key string) (wagering.WagerTransaction, error)
	GetByExternalID(ctx context.Context, providerID, externalID string) (wagering.WagerTransaction, error)
	GetByExternalIDForUpdate(ctx context.Context, providerID, externalID string) (wagering.WagerTransaction, error)
	Create(ctx context.Context, value wagering.WagerTransaction) error
	Save(ctx context.Context, value wagering.WagerTransaction) error
}
type LedgerRepository interface {
	Append(ctx context.Context, value ledger.Entry) error
	List(ctx context.Context, walletID, cursor string, limit int) (LedgerPage, error)
}

type LedgerPage struct {
	Entries    []ledger.Entry
	NextCursor string
}

type InboxRecord struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	ReceivedAt   time.Time
	CompletedAt  *time.Time
}

type InboxClaim struct {
	Record     InboxRecord
	Created    bool
	Redelivery bool
}

type InboxRepository interface {
	Claim(ctx context.Context, consumerName, messageID, payloadHash string) (InboxClaim, error)
	Get(ctx context.Context, consumerName, messageID string) (InboxRecord, error)
	MarkCompleted(ctx context.Context, consumerName, messageID string, completedAt time.Time) error
}

type OutboxRecord struct {
	EventID       string
	EventType     string
	AggregateID   string
	Version       int
	Payload       []byte
	OccurredAt    time.Time
	Attempts      int
	NextAttemptAt time.Time
	LeaseUntil    *time.Time
	LastError     *string
}

type OutboxRepository interface {
	Append(ctx context.Context, event events.Envelope) error
	ClaimPending(ctx context.Context, now, leaseUntil time.Time, limit int) ([]OutboxRecord, error)
	MarkPublished(ctx context.Context, eventID string, leaseUntil, publishedAt time.Time) error
	RecordFailure(ctx context.Context, eventID string, leaseUntil, nextAttemptAt time.Time, lastError string) error
}

type OutboxPublisher interface {
	Publish(ctx context.Context, record OutboxRecord) error
}

type UnitOfWork interface {
	Within(ctx context.Context, fn func(context.Context) error) error
}

type Metrics interface {
	TransactionStatus(status string)
	IncIdempotentReplay()
	IncRetry()
	IncDLQ()
	SetDLQDepth(uint64)
	IncConcurrencyConflict()
	OutboxPublished()
	OutboxFailed()
	ReconciliationDivergence()
	ObserveProcessing(time.Duration)
	ObserveOutboxLag(time.Duration)
}
