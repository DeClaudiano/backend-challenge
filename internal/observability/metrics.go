package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	transactionStatus sync.Map
	idempotentReplay  atomic.Uint64
	retries           atomic.Uint64
	dlq               atomic.Uint64
	dlqDepth          atomic.Uint64
	concurrency       atomic.Uint64
	outboxPublished   atomic.Uint64
	outboxFailed      atomic.Uint64
	reconciliationBad atomic.Uint64
	processingCount   atomic.Uint64
	processingNanos   atomic.Uint64
	outboxLagNanos    atomic.Uint64
}

func NewMetrics() *Metrics { return &Metrics{} }

func (m *Metrics) TransactionStatus(status string) {
	if status == "" {
		return
	}
	value, _ := m.transactionStatus.LoadOrStore(status, new(atomic.Uint64))
	value.(*atomic.Uint64).Add(1)
}
func (m *Metrics) IncIdempotentReplay()      { m.idempotentReplay.Add(1) }
func (m *Metrics) IncRetry()                 { m.retries.Add(1) }
func (m *Metrics) IncDLQ()                   { m.dlq.Add(1) }
func (m *Metrics) SetDLQDepth(value uint64)  { m.dlqDepth.Store(value) }
func (m *Metrics) IncConcurrencyConflict()   { m.concurrency.Add(1) }
func (m *Metrics) OutboxPublished()          { m.outboxPublished.Add(1) }
func (m *Metrics) OutboxFailed()             { m.outboxFailed.Add(1) }
func (m *Metrics) ReconciliationDivergence() { m.reconciliationBad.Add(1) }
func (m *Metrics) ObserveProcessing(d time.Duration) {
	m.processingCount.Add(1)
	m.processingNanos.Add(uint64(d.Nanoseconds()))
}
func (m *Metrics) ObserveOutboxLag(d time.Duration) {
	if d > 0 {
		m.outboxLagNanos.Store(uint64(d.Nanoseconds()))
	}
}

func (m *Metrics) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	var b strings.Builder
	writeCounter := func(name string, value uint64) { fmt.Fprintf(&b, "%s %d\n", name, value) }
	statuses := make(map[string]uint64)
	m.transactionStatus.Range(func(key, value any) bool { statuses[key.(string)] = value.(*atomic.Uint64).Load(); return true })
	keys := make([]string, 0, len(statuses))
	for key := range statuses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "wager_transaction_total{status=%q} %d\n", key, statuses[key])
	}
	writeCounter("wager_idempotent_replay_total", m.idempotentReplay.Load())
	writeCounter("wager_retry_total", m.retries.Load())
	writeCounter("wager_dlq_total", m.dlq.Load())
	writeCounter("wager_dlq_depth", m.dlqDepth.Load())
	writeCounter("wager_concurrency_conflict_total", m.concurrency.Load())
	writeCounter("wager_outbox_published_total", m.outboxPublished.Load())
	writeCounter("wager_outbox_failed_total", m.outboxFailed.Load())
	writeCounter("wager_reconciliation_divergence_total", m.reconciliationBad.Load())
	writeCounter("wager_processing_count", m.processingCount.Load())
	writeCounter("wager_processing_seconds_total", m.processingNanos.Load()/uint64(time.Second))
	writeCounter("wager_outbox_lag_seconds", m.outboxLagNanos.Load()/uint64(time.Second))
	_, _ = w.Write([]byte(b.String()))
}

func (m *Metrics) Snapshot() map[string]uint64 {
	result := map[string]uint64{
		"idempotent_replay":         m.idempotentReplay.Load(),
		"retries":                   m.retries.Load(),
		"dlq":                       m.dlq.Load(),
		"dlq_depth":                 m.dlqDepth.Load(),
		"concurrency_conflicts":     m.concurrency.Load(),
		"outbox_published":          m.outboxPublished.Load(),
		"outbox_failed":             m.outboxFailed.Load(),
		"reconciliation_divergence": m.reconciliationBad.Load(),
		"processing_count":          m.processingCount.Load(),
	}
	return result
}
