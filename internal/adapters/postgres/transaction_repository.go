package postgres

import (
	"context"
	"fmt"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionRepository struct{ pool *pgxpool.Pool }

func NewTransactionRepository(pool *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{pool: pool}
}

const transactionSelect = `
	SELECT id::text, source, kind, status, COALESCE(provider_id, ''), COALESCE(external_transaction_id, ''),
	       COALESCE(idempotency_key, ''), COALESCE(payload_hash, ''), player_id::text, wallet_id::text,
	       COALESCE(round_id, ''), COALESCE(game_id, ''), amount_minor, currency::varchar,
	       COALESCE(reference_external_transaction_id, ''), reference_transaction_id::text,
	       reversed_by_transaction_id::text, reversal_kind, COALESCE(failure_code, ''),
	       result_balance_minor, result_currency::varchar, created_at, updated_at
	FROM wager_transactions`

type transactionScanner interface{ Scan(dest ...any) error }

func (r *TransactionRepository) Get(ctx context.Context, transactionID string) (domainwagering.WagerTransaction, error) {
	return r.scan(ctx, executorFor(ctx, r.pool).QueryRow(ctx, transactionSelect+` WHERE id = $1`, transactionID), transactionID)
}
func (r *TransactionRepository) GetForUpdate(ctx context.Context, transactionID string) (domainwagering.WagerTransaction, error) {
	return r.scan(ctx, executorFor(ctx, r.pool).QueryRow(ctx, transactionSelect+` WHERE id = $1 FOR UPDATE`, transactionID), transactionID)
}
func (r *TransactionRepository) GetByIdempotency(ctx context.Context, providerID, key string) (domainwagering.WagerTransaction, error) {
	return r.scan(ctx, executorFor(ctx, r.pool).QueryRow(ctx, transactionSelect+` WHERE provider_id = $1 AND idempotency_key = $2`, providerID, key), providerID+":"+key)
}
func (r *TransactionRepository) GetByExternalID(ctx context.Context, providerID, externalID string) (domainwagering.WagerTransaction, error) {
	return r.scan(ctx, executorFor(ctx, r.pool).QueryRow(ctx, transactionSelect+` WHERE provider_id = $1 AND external_transaction_id = $2`, providerID, externalID), providerID+":"+externalID)
}
func (r *TransactionRepository) GetByExternalIDForUpdate(ctx context.Context, providerID, externalID string) (domainwagering.WagerTransaction, error) {
	return r.scan(ctx, executorFor(ctx, r.pool).QueryRow(ctx, transactionSelect+` WHERE provider_id = $1 AND external_transaction_id = $2 FOR UPDATE`, providerID, externalID), providerID+":"+externalID)
}

func (r *TransactionRepository) ListPendingReferences(ctx context.Context, now time.Time, limit int) ([]ports.PendingReferenceRecord, error) {
	if now.IsZero() || limit <= 0 {
		return nil, domain.NewError(domain.ErrInvalidValue, "invalid pending reference query")
	}
	rows, err := executorFor(ctx, r.pool).Query(ctx, `
		SELECT id::text, reference_attempts, reference_next_attempt_at, reference_expires_at
		FROM wager_transactions
		WHERE status = 'PENDING_REFERENCE' AND reference_next_attempt_at <= $1
		ORDER BY reference_next_attempt_at, updated_at, id
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending references: %w", err)
	}
	defer rows.Close()
	result := make([]ports.PendingReferenceRecord, 0, limit)
	for rows.Next() {
		var id string
		var attempts int
		var nextAttemptAt, expiresAt time.Time
		if err := rows.Scan(&id, &attempts, &nextAttemptAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan pending reference: %w", err)
		}
		transaction, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, ports.PendingReferenceRecord{Transaction: transaction, Attempts: attempts, NextAttemptAt: nextAttemptAt, ExpiresAt: expiresAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending references: %w", err)
	}
	return result, nil
}

func (r *TransactionRepository) SchedulePendingReference(ctx context.Context, transactionID string, attempts int, nextAttemptAt, expiresAt time.Time) error {
	if transactionID == "" || attempts < 0 || nextAttemptAt.IsZero() || expiresAt.IsZero() || expiresAt.Before(nextAttemptAt) {
		return domain.NewError(domain.ErrInvalidValue, "invalid pending reference schedule")
	}
	result, err := executorFor(ctx, r.pool).Exec(ctx, `
		UPDATE wager_transactions
		SET reference_attempts=$2, reference_next_attempt_at=$3, reference_expires_at=$4, updated_at=$3
		WHERE id=$1 AND status='PENDING_REFERENCE'`, transactionID, attempts, nextAttemptAt, expiresAt)
	if err != nil {
		return mapWriteError("pending reference schedule", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrNotFound, "pending reference was not found")
	}
	return nil
}

func (r *TransactionRepository) scan(ctx context.Context, row transactionScanner, identity string) (domainwagering.WagerTransaction, error) {
	var id, source, kind, status, providerID, externalID, idempotencyKey, payloadHash string
	var playerID, walletID, roundID, gameID, currency, referenceExternalID string
	var referenceID, reversedBy, reversalKind, failureCode *string
	var minor int64
	var resultMinor *int64
	var resultCurrency *string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &source, &kind, &status, &providerID, &externalID, &idempotencyKey, &payloadHash, &playerID, &walletID, &roundID, &gameID, &minor, &currency, &referenceExternalID, &referenceID, &reversedBy, &reversalKind, &failureCode, &resultMinor, &resultCurrency, &createdAt, &updatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return domainwagering.WagerTransaction{}, domain.NewError(domain.ErrNotFound, fmt.Sprintf("transaction %s was not found", identity))
		}
		return domainwagering.WagerTransaction{}, fmt.Errorf("scan wager transaction %s: %w", identity, err)
	}
	amount, err := money.New(minor, money.Currency(currency))
	if err != nil {
		return domainwagering.WagerTransaction{}, err
	}
	var resultBalance *money.Money
	if resultMinor != nil || resultCurrency != nil {
		if resultMinor == nil || resultCurrency == nil {
			return domainwagering.WagerTransaction{}, domain.NewError(domain.ErrInvalidValue, "persisted result balance is incomplete")
		}
		value, err := money.New(*resultMinor, money.Currency(*resultCurrency))
		if err != nil {
			return domainwagering.WagerTransaction{}, err
		}
		resultBalance = &value
	}
	return domainwagering.Rehydrate(id, providerID, externalID, idempotencyKey, payloadHash, playerID, walletID, roundID, gameID, domainwagering.Kind(kind), domainwagering.Source(source), amount, referenceExternalID, nullableValue(referenceID), domainwagering.Status(status), nullableValue(failureCode), resultBalance, createdAt, updatedAt, nullableValue(reversedBy), domainwagering.Kind(nullableValue(reversalKind)))
}

func (r *TransactionRepository) Create(ctx context.Context, value domainwagering.WagerTransaction) error {
	_, err := executorFor(ctx, r.pool).Exec(ctx, `
		INSERT INTO wager_transactions
		(id, source, kind, status, provider_id, external_transaction_id, idempotency_key, payload_hash,
		 wallet_id, player_id, round_id, game_id, amount_minor, currency, reference_external_transaction_id,
		 reference_transaction_id, reversed_by_transaction_id, reversal_kind, failure_code,
		 result_balance_minor, result_currency, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		value.ID(), value.Source(), value.Kind(), value.Status(), nullableString(value.ProviderID()), nullableString(value.ExternalID()), nullableString(value.IdempotencyKey()), nullableString(value.PayloadHash()), value.WalletID(), value.PlayerID(), nullableString(value.RoundID()), nullableString(value.GameID()), value.Amount().Minor(), value.Amount().Currency().String(), nullableString(value.ReferenceExternalID()), nullableString(value.ReferenceID()), nullableString(value.ReversedBy()), nullableKind(value.ReversalKind()), nullableString(value.FailureCode()), resultMinor(value), resultCurrency(value), value.CreatedAt(), value.UpdatedAt())
	return mapWriteError("wager transaction", err)
}

func (r *TransactionRepository) Save(ctx context.Context, value domainwagering.WagerTransaction) error {
	result, err := executorFor(ctx, r.pool).Exec(ctx, `
		UPDATE wager_transactions SET status=$2, reference_transaction_id=$3, failure_code=$4,
		 result_balance_minor=$5, result_currency=$6, updated_at=$7, reversed_by_transaction_id=$8, reversal_kind=$9
		WHERE id=$1`, value.ID(), value.Status(), nullableString(value.ReferenceID()), nullableString(value.FailureCode()), resultMinor(value), resultCurrency(value), value.UpdatedAt(), nullableString(value.ReversedBy()), nullableKind(value.ReversalKind()))
	if err != nil {
		return mapWriteError("wager transaction", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrNotFound, "wager transaction was not found")
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func nullableKind(value domainwagering.Kind) any {
	if value == "" {
		return nil
	}
	return string(value)
}
func resultMinor(value domainwagering.WagerTransaction) any {
	balance, ok := value.ResultBalance()
	if !ok {
		return nil
	}
	return balance.Minor()
}
func resultCurrency(value domainwagering.WagerTransaction) any {
	balance, ok := value.ResultBalance()
	if !ok {
		return nil
	}
	return balance.Currency().String()
}

var _ ports.PendingReferenceRepository = (*TransactionRepository)(nil)
