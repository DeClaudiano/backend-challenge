package publisher

import (
	"context"
	"fmt"
	"math"
	"time"

	"backend-challenge/internal/application/ports"
)

const (
	defaultBatchSize     = 10
	defaultLeaseDuration = 30 * time.Second
	maxRetryBackoff      = 1 * time.Minute
)

type OutboxPublisher struct {
	repository    ports.OutboxRepository
	broker        ports.OutboxPublisher
	now           func() time.Time
	leaseDuration time.Duration
	batchSize     int
	metrics       ports.Metrics
}

func NewOutboxPublisher(repository ports.OutboxRepository, broker ports.OutboxPublisher) *OutboxPublisher {
	return NewOutboxPublisherWithConfig(repository, broker, defaultBatchSize, defaultLeaseDuration)
}

func NewOutboxPublisherWithConfig(repository ports.OutboxRepository, broker ports.OutboxPublisher, batchSize int, leaseDuration time.Duration) *OutboxPublisher {
	return &OutboxPublisher{repository: repository, broker: broker, now: time.Now, leaseDuration: leaseDuration, batchSize: batchSize}
}

func NewOutboxPublisherWithMetrics(repository ports.OutboxRepository, broker ports.OutboxPublisher, metrics ports.Metrics) *OutboxPublisher {
	return NewOutboxPublisherWithConfigAndMetrics(repository, broker, defaultBatchSize, defaultLeaseDuration, metrics)
}

func NewOutboxPublisherWithConfigAndMetrics(repository ports.OutboxRepository, broker ports.OutboxPublisher, batchSize int, leaseDuration time.Duration, metrics ports.Metrics) *OutboxPublisher {
	return &OutboxPublisher{repository: repository, broker: broker, metrics: metrics, now: time.Now, leaseDuration: leaseDuration, batchSize: batchSize}
}

func (p *OutboxPublisher) PublishBatch(ctx context.Context) error {
	now := p.now().UTC()
	records, err := p.repository.ClaimPending(ctx, now, now.Add(p.leaseDuration), p.batchSize)
	if err != nil {
		return err
	}
	var firstErr error
	for _, record := range records {
		if record.LeaseUntil == nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("outbox record %s has no lease", record.EventID)
			}
			continue
		}
		if err := p.broker.Publish(ctx, record); err != nil {
			if p.metrics != nil {
				p.metrics.OutboxFailed()
				p.metrics.IncRetry()
			}
			next := now.Add(retryBackoff(record.Attempts + 1))
			if failureErr := p.repository.RecordFailure(ctx, record.EventID, *record.LeaseUntil, next, err.Error()); failureErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("record outbox failure %s: %w", record.EventID, failureErr)
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if p.metrics != nil {
			p.metrics.OutboxPublished()
			p.metrics.ObserveOutboxLag(now.Sub(record.OccurredAt))
		}
		if err := p.repository.MarkPublished(ctx, record.EventID, *record.LeaseUntil, now); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("mark outbox event %s published: %w", record.EventID, err)
		}
	}
	return firstErr
}

func (p *OutboxPublisher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = p.PublishBatch(ctx)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := math.Pow(2, float64(attempt-1))
	backoff := time.Duration(seconds) * time.Second
	if backoff > maxRetryBackoff {
		return maxRetryBackoff
	}
	return backoff
}
