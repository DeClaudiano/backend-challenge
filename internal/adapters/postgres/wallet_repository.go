package postgres

import (
	"context"
	"fmt"
	"time"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	domainwallet "backend-challenge/internal/domain/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletRepository struct{ pool *pgxpool.Pool }

func NewWalletRepository(pool *pgxpool.Pool) *WalletRepository { return &WalletRepository{pool: pool} }

func (r *WalletRepository) Get(ctx context.Context, walletID string) (domainwallet.Wallet, error) {
	return r.get(ctx, walletID, false)
}

func (r *WalletRepository) GetForUpdate(ctx context.Context, walletID string) (domainwallet.Wallet, error) {
	return r.get(ctx, walletID, true)
}

func (r *WalletRepository) get(ctx context.Context, walletID string, forUpdate bool) (domainwallet.Wallet, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	row := executorFor(ctx, r.pool).QueryRow(ctx, `
		SELECT id::text, player_id::text, currency::varchar, balance_minor, version, created_at, updated_at
		FROM wallets WHERE id = $1`+lock, walletID)
	var id, playerID, currency string
	var minor, version int64
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &playerID, &currency, &minor, &version, &createdAt, &updatedAt); err != nil {
		return domainwallet.Wallet{}, mapQueryError("wallet", walletID, err)
	}
	balance, err := money.New(minor, money.Currency(currency))
	if err != nil {
		return domainwallet.Wallet{}, err
	}
	return domainwallet.Rehydrate(id, playerID, money.Currency(currency), balance, version, createdAt, updatedAt)
}

func (r *WalletRepository) Create(ctx context.Context, value domainwallet.Wallet) error {
	_, err := executorFor(ctx, r.pool).Exec(ctx, `
		INSERT INTO wallets (id, player_id, currency, balance_minor, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, value.ID(), value.PlayerID(), value.Currency().String(), value.Balance().Minor(), value.Version(), value.CreatedAt(), value.UpdatedAt())
	return mapWriteError("wallet", err)
}

func (r *WalletRepository) Save(ctx context.Context, value domainwallet.Wallet) error {
	if value.Version() < 2 {
		return domain.NewError(domain.ErrOptimisticConflict, "wallet save requires a changed aggregate version")
	}
	result, err := executorFor(ctx, r.pool).Exec(ctx, `
		UPDATE wallets SET balance_minor = $2, version = $3, updated_at = $4
		WHERE id = $1 AND version = $5`, value.ID(), value.Balance().Minor(), value.Version(), value.UpdatedAt(), value.Version()-1)
	if err != nil {
		return mapWriteError("wallet", err)
	}
	if result.RowsAffected() != 1 {
		return domain.NewError(domain.ErrOptimisticConflict, "wallet version changed concurrently")
	}
	return nil
}

func mapQueryError(entity, id string, err error) error {
	if err == pgx.ErrNoRows {
		return domain.NewError(domain.ErrNotFound, fmt.Sprintf("%s %s was not found", entity, id))
	}
	return fmt.Errorf("query %s %s: %w", entity, id, err)
}
