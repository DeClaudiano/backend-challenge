package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandlerExposesCounters(t *testing.T) {
	m := NewMetrics()
	m.TransactionStatus("PROCESSED")
	m.IncIdempotentReplay()
	m.IncRetry()
	m.IncDLQ()
	m.IncConcurrencyConflict()
	m.OutboxPublished()
	m.OutboxFailed()
	m.ReconciliationDivergence()
	m.ObserveProcessing(2 * time.Second)
	m.ObserveOutboxLag(3 * time.Second)

	recorder := httptest.NewRecorder()
	m.Handler(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, expected := range []string{
		`wager_transaction_total{status="PROCESSED"} 1`,
		"wager_idempotent_replay_total 1",
		"wager_retry_total 1",
		"wager_dlq_total 1",
		"wager_concurrency_conflict_total 1",
		"wager_outbox_published_total 1",
		"wager_outbox_failed_total 1",
		"wager_reconciliation_divergence_total 1",
		"wager_processing_seconds_total 2",
		"wager_outbox_lag_seconds 3",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q: %s", expected, body)
		}
	}
}
