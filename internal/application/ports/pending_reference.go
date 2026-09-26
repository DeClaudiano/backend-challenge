package ports

import (
	"context"
	"time"

	"backend-challenge/internal/domain/wagering"
)

type PendingReferenceRecord struct {
	Transaction   wagering.WagerTransaction
	Attempts      int
	NextAttemptAt time.Time
	ExpiresAt     time.Time
}

type PendingReferenceRepository interface {
	ListPendingReferences(ctx context.Context, now time.Time, limit int) ([]PendingReferenceRecord, error)
	SchedulePendingReference(ctx context.Context, transactionID string, attempts int, nextAttemptAt, expiresAt time.Time) error
}
