package postgres

import (
	"errors"
	"fmt"
	"strings"

	"backend-challenge/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func mapWriteError(entity string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if strings.Contains(pgErr.ConstraintName, "processed_reference") {
				return domain.NewError(domain.ErrReversalAlreadyApplied, "the referenced transaction already has a processed reversal")
			}
			if strings.Contains(pgErr.ConstraintName, "idempotency") || strings.Contains(pgErr.ConstraintName, "external") || strings.Contains(pgErr.ConstraintName, "reference") {
				return domain.NewError(domain.ErrIdentityConflict, "a persisted business identity already exists")
			}
			return domain.NewError(domain.ErrIdentityConflict, fmt.Sprintf("%s violates unique constraint %s", entity, pgErr.ConstraintName))
		case "23503", "23514", "23502", "22P02":
			return domain.WrapError(domain.ErrInvalidValue, fmt.Sprintf("invalid %s persistence state", entity), err)
		}
	}
	return fmt.Errorf("write %s: %w", entity, err)
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
