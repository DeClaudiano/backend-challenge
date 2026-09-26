package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"backend-challenge/internal/application/ports"
	"backend-challenge/internal/domain/ledger"
	"backend-challenge/internal/domain/money"
	"backend-challenge/internal/domain/wagering"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LedgerRepository struct{ pool *pgxpool.Pool }

func NewLedgerRepository(pool *pgxpool.Pool) *LedgerRepository { return &LedgerRepository{pool: pool} }

func (r *LedgerRepository) Append(ctx context.Context, value ledger.Entry) error {
	_, err := executorFor(ctx, r.pool).Exec(ctx, `
		INSERT INTO wallet_ledger_entries
		(id, wallet_id, transaction_id, direction, amount_minor, currency, balance_before_minor, balance_after_minor, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, value.ID(), value.WalletID(), value.TransactionID(), value.Direction(), value.Amount().Minor(), value.Amount().Currency().String(), value.BalanceBefore().Minor(), value.BalanceAfter().Minor(), value.CreatedAt())
	return mapWriteError("wallet ledger entry", err)
}

type ledgerCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

func (r *LedgerRepository) List(ctx context.Context, walletID, cursor string, limit int) (ports.LedgerPage, error) {
	if limit < 1 || limit > 1000 {
		return ports.LedgerPage{}, fmt.Errorf("ledger limit must be between 1 and 1000")
	}
	args := []any{walletID, limit + 1}
	where := "wallet_id = $1"
	if cursor != "" {
		value, err := decodeCursor(cursor)
		if err != nil {
			return ports.LedgerPage{}, err
		}
		args = []any{walletID, value.CreatedAt, value.ID, limit + 1}
		where += " AND (created_at, id) < ($2, $3)"
	}
	query := `SELECT id::text, wallet_id::text, transaction_id::text, direction, amount_minor, currency::varchar,
		balance_before_minor, balance_after_minor, created_at
		FROM wallet_ledger_entries WHERE ` + where + ` ORDER BY created_at DESC, id DESC LIMIT $` + fmt.Sprint(len(args))
	rows, err := executorFor(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return ports.LedgerPage{}, fmt.Errorf("list wallet ledger: %w", err)
	}
	defer rows.Close()
	entries := make([]ledger.Entry, 0, limit)
	for rows.Next() {
		var id, rowWalletID, transactionID, direction, currency string
		var amountMinor, beforeMinor, afterMinor int64
		var createdAt time.Time
		if err := rows.Scan(&id, &rowWalletID, &transactionID, &direction, &amountMinor, &currency, &beforeMinor, &afterMinor, &createdAt); err != nil {
			return ports.LedgerPage{}, fmt.Errorf("scan wallet ledger: %w", err)
		}
		amount, err := money.New(amountMinor, money.Currency(currency))
		if err != nil {
			return ports.LedgerPage{}, err
		}
		before, err := money.New(beforeMinor, money.Currency(currency))
		if err != nil {
			return ports.LedgerPage{}, err
		}
		after, err := money.New(afterMinor, money.Currency(currency))
		if err != nil {
			return ports.LedgerPage{}, err
		}
		entry, err := ledger.New(id, rowWalletID, transactionID, wagering.Direction(direction), amount, before, after, createdAt)
		if err != nil {
			return ports.LedgerPage{}, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return ports.LedgerPage{}, fmt.Errorf("iterate wallet ledger: %w", err)
	}
	page := ports.LedgerPage{Entries: entries}
	if len(entries) > limit {
		last := entries[limit-1]
		page.Entries = entries[:limit]
		page.NextCursor = encodeCursor(ledgerCursor{CreatedAt: last.CreatedAt(), ID: last.ID()})
	}
	return page, nil
}

func encodeCursor(value ledgerCursor) string {
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeCursor(value string) (ledgerCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return ledgerCursor{}, fmt.Errorf("invalid ledger cursor")
	}
	var cursor ledgerCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
		return ledgerCursor{}, fmt.Errorf("invalid ledger cursor")
	}
	return cursor, nil
}
