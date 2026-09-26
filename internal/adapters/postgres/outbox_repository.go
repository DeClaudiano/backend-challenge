package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend-challenge/internal/application/events"
	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct{ pool *pgxpool.Pool }

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository { return &OutboxRepository{pool: pool} }

func (r *OutboxRepository) Append(ctx context.Context, event events.Envelope) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}
	_, err = executorFor(ctx, r.pool).Exec(ctx, `
		INSERT INTO outbox_events (event_id, aggregate_id, event_type, event_version, payload, occurred_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`, event.EventID, event.AggregateID, event.EventType, event.Version, payload, event.OccurredAt.UTC())
	return mapWriteError("outbox event", err)
}

func (r *OutboxRepository) ClaimPending(ctx context.Context, now, leaseUntil time.Time, limit int) ([]ports.OutboxRecord, error) {
	if now.IsZero() || leaseUntil.IsZero() || !leaseUntil.After(now) || limit < 1 {
		return nil, domain.NewError(domain.ErrInvalidValue, "invalid outbox claim parameters")
	}
	rows, err := executorFor(ctx, r.pool).Query(ctx, `
		WITH candidates AS (
			SELECT event_id FROM outbox_events
			WHERE published_at IS NULL
			  AND next_attempt_at <= $1
			  AND (lease_until IS NULL OR lease_until <= $1)
			ORDER BY next_attempt_at, event_id
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE outbox_events AS outbox
		SET lease_until = $2
		FROM candidates
		WHERE outbox.event_id = candidates.event_id
		RETURNING outbox.event_id::text, outbox.event_type, outbox.aggregate_id::text,
		          outbox.event_version, outbox.payload, outbox.occurred_at,
		          outbox.attempts, outbox.next_attempt_at, outbox.lease_until, outbox.last_error`, now.UTC(), leaseUntil.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()
	result := make([]ports.OutboxRecord, 0, limit)
	for rows.Next() {
		var record ports.OutboxRecord
		if err := rows.Scan(&record.EventID, &record.EventType, &record.AggregateID, &record.Version, &record.Payload, &record.OccurredAt, &record.Attempts, &record.NextAttemptAt, &record.LeaseUntil, &record.LastError); err != nil {
			return nil, fmt.Errorf("scan claimed outbox event: %w", err)
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed outbox events: %w", err)
	}
	return result, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string, leaseUntil, publishedAt time.Time) error {
	if eventID == "" || leaseUntil.IsZero() || publishedAt.IsZero() {
		return domain.NewError(domain.ErrInvalidValue, "event ID, lease and publication timestamp are required")
	}
	result, err := executorFor(ctx, r.pool).Exec(ctx, `UPDATE outbox_events SET published_at = $3, lease_until = NULL WHERE event_id = $1 AND lease_until = $2 AND published_at IS NULL`, eventID, leaseUntil.UTC(), publishedAt.UTC())
	if err != nil {
		return mapWriteError("outbox event", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrNotFound, "pending outbox event was not found")
	}
	return nil
}

func (r *OutboxRepository) RecordFailure(ctx context.Context, eventID string, leaseUntil, nextAttemptAt time.Time, lastError string) error {
	if eventID == "" || leaseUntil.IsZero() || nextAttemptAt.IsZero() || lastError == "" {
		return domain.NewError(domain.ErrInvalidValue, "event ID, lease, next attempt and error are required")
	}
	result, err := executorFor(ctx, r.pool).Exec(ctx, `
		UPDATE outbox_events SET attempts = attempts + 1, next_attempt_at = $2, lease_until = NULL, last_error = $3
		WHERE event_id = $1 AND lease_until = $4 AND published_at IS NULL`, eventID, nextAttemptAt.UTC(), lastError, leaseUntil.UTC())
	if err != nil {
		return mapWriteError("outbox event", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrNotFound, "pending outbox event was not found")
	}
	return nil
}

var _ ports.OutboxRepository = (*OutboxRepository)(nil)
