package postgres

import (
	"context"
	"fmt"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InboxRepository struct{ pool *pgxpool.Pool }

func NewInboxRepository(pool *pgxpool.Pool) *InboxRepository { return &InboxRepository{pool: pool} }

const inboxSelect = `SELECT consumer_name, message_id, payload_hash, received_at, completed_at FROM inbox_messages`

func (r *InboxRepository) Claim(ctx context.Context, consumerName, messageID, payloadHash string) (ports.InboxClaim, error) {
	if consumerName == "" || messageID == "" || payloadHash == "" {
		return ports.InboxClaim{}, domain.NewError(domain.ErrInvalidValue, "consumer name, message ID and payload hash are required")
	}
	row := executorFor(ctx, r.pool).QueryRow(ctx, `
		INSERT INTO inbox_messages (consumer_name, message_id, payload_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (consumer_name, message_id) DO NOTHING
		RETURNING consumer_name, message_id, payload_hash, received_at, completed_at`, consumerName, messageID, payloadHash)
	record, err := scanInbox(row)
	if err == nil {
		return ports.InboxClaim{Record: record, Created: true}, nil
	}
	if err != pgx.ErrNoRows {
		return ports.InboxClaim{}, fmt.Errorf("claim inbox message: %w", err)
	}

	record, err = r.getForUpdate(ctx, consumerName, messageID)
	if err != nil {
		return ports.InboxClaim{}, err
	}
	if record.PayloadHash != payloadHash {
		return ports.InboxClaim{}, domain.NewError(domain.ErrIdentityConflict, "inbox message was redelivered with a different payload hash")
	}
	return ports.InboxClaim{Record: record, Redelivery: true}, nil
}

func (r *InboxRepository) Get(ctx context.Context, consumerName, messageID string) (ports.InboxRecord, error) {
	return r.getForUpdate(ctx, consumerName, messageID)
}

func (r *InboxRepository) getForUpdate(ctx context.Context, consumerName, messageID string) (ports.InboxRecord, error) {
	row := executorFor(ctx, r.pool).QueryRow(ctx, inboxSelect+` WHERE consumer_name = $1 AND message_id = $2 FOR UPDATE`, consumerName, messageID)
	record, err := scanInbox(row)
	if err == pgx.ErrNoRows {
		return ports.InboxRecord{}, domain.NewError(domain.ErrNotFound, "inbox message was not found")
	}
	if err != nil {
		return ports.InboxRecord{}, fmt.Errorf("query inbox message: %w", err)
	}
	return record, nil
}

func (r *InboxRepository) MarkCompleted(ctx context.Context, consumerName, messageID string, completedAt time.Time) error {
	if completedAt.IsZero() {
		return domain.NewError(domain.ErrInvalidValue, "completion timestamp is required")
	}
	result, err := executorFor(ctx, r.pool).Exec(ctx, `
		UPDATE inbox_messages SET completed_at = COALESCE(completed_at, $3)
		WHERE consumer_name = $1 AND message_id = $2`, consumerName, messageID, completedAt)
	if err != nil {
		return mapWriteError("inbox message", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrNotFound, "inbox message was not found")
	}
	return nil
}

func scanInbox(row interface{ Scan(...any) error }) (ports.InboxRecord, error) {
	var record ports.InboxRecord
	if err := row.Scan(&record.ConsumerName, &record.MessageID, &record.PayloadHash, &record.ReceivedAt, &record.CompletedAt); err != nil {
		return ports.InboxRecord{}, err
	}
	return record, nil
}

var _ ports.InboxRepository = (*InboxRepository)(nil)
