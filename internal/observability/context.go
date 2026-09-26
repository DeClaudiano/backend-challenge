package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
)

type contextKey string

const correlationKey contextKey = "correlation_id"

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}
func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey).(string)
	return value
}
func EnsureCorrelationID(ctx context.Context) (context.Context, string) {
	if id := CorrelationID(ctx); id != "" {
		return ctx, id
	}
	var value [16]byte
	id := "correlation-unknown"
	if _, err := rand.Read(value[:]); err == nil {
		id = hex.EncodeToString(value[:])
	}
	return WithCorrelationID(ctx, id), id
}
func Logger(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if id := CorrelationID(ctx); id != "" {
		return logger.With("correlationId", id)
	}
	return logger
}
func MessageLogger(ctx context.Context, logger *slog.Logger, messageID, transactionID, walletID, providerID string) *slog.Logger {
	logger = Logger(ctx, logger)
	attrs := make([]any, 0, 8)
	if messageID != "" {
		attrs = append(attrs, "messageId", messageID)
	}
	if transactionID != "" {
		attrs = append(attrs, "transactionId", transactionID)
	}
	if walletID != "" {
		attrs = append(attrs, "walletId", walletID)
	}
	if providerID != "" {
		attrs = append(attrs, "providerId", providerID)
	}
	return logger.With(attrs...)
}
func NewCorrelationID() string {
	ctx, id := EnsureCorrelationID(context.Background())
	_ = ctx
	return fmt.Sprint(id)
}
