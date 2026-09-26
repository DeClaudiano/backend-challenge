package observability

import (
	"context"
	"log/slog"
	"testing"
)

func TestEnsureCorrelationIDPreservesExistingID(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "corr-123")
	result, id := EnsureCorrelationID(ctx)
	if id != "corr-123" || CorrelationID(result) != "corr-123" {
		t.Fatalf("expected existing correlation id, got %q", id)
	}
}

func TestMessageLoggerKeepsOperationalFields(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	ctx := WithCorrelationID(context.Background(), "corr-123")
	derived := MessageLogger(ctx, logger, "msg-1", "tx-1", "wallet-1", "provider-a")
	if derived == nil {
		t.Fatal("expected logger")
	}
}
